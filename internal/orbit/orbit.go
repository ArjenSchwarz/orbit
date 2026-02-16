// Package orbit provides the main orchestration loop for running Claude Code sessions.
package orbit

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	output "github.com/ArjenSchwarz/go-output/v2"
	"github.com/arjenschwarz/orbit/internal/agents"
	_ "github.com/arjenschwarz/orbit/internal/agents/claudecode" // Register claudecode agent
	"github.com/arjenschwarz/orbit/internal/comparison"
	"github.com/arjenschwarz/orbit/internal/cost"
	"github.com/arjenschwarz/orbit/internal/debug"
	"github.com/arjenschwarz/orbit/internal/display"
	"github.com/arjenschwarz/orbit/internal/logs"
	"github.com/arjenschwarz/orbit/internal/registry"
	"github.com/arjenschwarz/orbit/internal/rune"
	"github.com/arjenschwarz/orbit/internal/variants"
)

const (
	maxRetries = 5
)

// Clock interface for time operations.
// This allows tests to inject a fake clock for deterministic timing tests.
type Clock interface {
	Now() time.Time
	Sleep(d time.Duration)
}

// RealClock uses actual time functions.
type RealClock struct{}

// Now returns the current time.
func (RealClock) Now() time.Time { return time.Now() }

// Sleep pauses execution for the specified duration.
func (RealClock) Sleep(d time.Duration) { time.Sleep(d) }

// AgentResolver looks up agents by name.
// This allows tests to inject mock agents without modifying the global registry.
type AgentResolver interface {
	GetAgent(name string, cfg agents.AgentConfig) (agents.Agent, error)
}

// registryResolver implements AgentResolver using the global agent registry.
type registryResolver struct{}

// GetAgent returns an agent from the global registry.
func (registryResolver) GetAgent(name string, cfg agents.AgentConfig) (agents.Agent, error) {
	return agents.Get(name, cfg)
}

// DefaultAgentResolver uses the global agent registry.
var DefaultAgentResolver AgentResolver = registryResolver{}

// getCostValue extracts the primary cost value from a RunResult, returning 0 if cost is nil.
// Checks CostUSD first, then Credits, then PremiumRequests.
func getCostValue(result *agents.RunResult) float64 {
	if result == nil || result.Cost == nil {
		return 0
	}
	if result.Cost.CostUSD > 0 {
		return result.Cost.CostUSD
	}
	if result.Cost.Credits > 0 {
		return result.Cost.Credits
	}
	return result.Cost.PremiumRequests
}

// formatCost returns a human-readable cost string from a RunResult.
// Uses the cost package to format costs according to their unit type.
// Returns "-" if no cost information is available.
func formatCost(result *agents.RunResult) string {
	if result == nil || result.Cost == nil {
		return "-"
	}

	unit := result.Cost.CostUnit
	var value float64

	switch unit {
	case cost.UnitCredits:
		value = result.Cost.Credits
	case cost.UnitPremiumRequests:
		value = result.Cost.PremiumRequests
	default:
		value = result.Cost.CostUSD
		unit = cost.UnitUSD
	}

	return cost.Format(value, unit)
}

// getSessionDuration returns the session duration for display.
// Uses agent-reported SessionDuration if available, otherwise falls back to measured Duration.
func getSessionDuration(result *agents.RunResult) time.Duration {
	if result.Cost != nil && result.Cost.SessionDuration != nil {
		return *result.Cost.SessionDuration
	}
	return result.Duration
}

// Config holds the orchestrator configuration.
type Config struct {
	TasksFile        string
	LogDir           string
	BranchName       string
	SkipPermissions  bool
	Verbose          bool
	DryRun           bool
	Debug            bool   // Enable debug logging for troubleshooting
	CentralizedLog   bool   // Enable centralized file logging
	RunID            string // UUID for this orchestration run
	Version          string // Orbit version for logging
	WorkingDir       string
	Command          string        // Custom phase command
	PrePrompt        string        // AI prompt before phases start (empty = disabled)
	PostPrompt       string        // Post-completion AI prompt (renamed from PostCommand, empty = disabled)
	AgentPreCommand  string        // Shell command before first phase (from agent config)
	AgentPostCommand string        // Shell command after last phase (from agent config)
	CommandTimeout   time.Duration // Timeout for shell commands (default 5m)
	DateSubdirs      bool          // If true, use timestamped subdirectories for logs
	ContinueSession  bool          // If true, continue existing Claude sessions when resuming

	// Agent configuration
	Agent        string                        // Agent name (claude-code, codex, kiro, copilot)
	AgentConfig  agents.AgentConfig            // Agent-specific configuration for default agent
	AgentConfigs map[string]agents.AgentConfig // Per-agent configs from config file (for variants)

	// Variant configuration for multi-spec comparison
	VariantCount   int      // Number of variants (0 = single-run mode)
	Parallel       bool     // Run variants in parallel
	MaxParallel    int      // Maximum parallel variants
	BranchPrefix   string   // Branch naming prefix
	Guidance       []string // Per-variant guidance from file
	CompareCommand string   // Custom comparison command
	GlobalGuidance string   // Global guidance applied to all variants
	SpecDir        string   // Spec directory for variant worktrees
	RepoRoot       string   // Repository root directory
	VariantAgents  []string // Per-variant agents (cycles if fewer than variants) [Req 10.1]

	// Auto-consolidation configuration
	AutoConsolidate        bool   // If true, run consolidation after comparison
	AllowDirty             bool   // If true, allow consolidation even with uncommitted changes
	PostConsolidateCommand string // Shell command to run after consolidation completes

	// Testing support
	Clock         Clock         // Optional clock for time operations (defaults to RealClock{})
	AgentResolver AgentResolver // Optional agent resolver (defaults to DefaultAgentResolver)
}

// Orbit orchestrates Claude Code sessions to implement spec phases.
type Orbit struct {
	config               Config
	runeClient           *rune.Client
	agent                agents.Agent           // Agent interface for multi-agent support
	errorClassifier      agents.ErrorClassifier // Agent-specific error classifier
	logManager           *logs.Manager
	phaseSummaries       []rune.PhaseSummary
	spinner              *display.Spinner
	shutdownCtx          context.Context
	shutdownCancel       context.CancelFunc
	registry             *registry.Registry // Web interface run registry
	runID                string             // UUID of the current run in registry
	currentPhaseRunCount int                // Track retry count for current phase
	debug                *debug.Logger      // Debug logger
	variantManager       *variants.Manager  // Variant lifecycle manager (nil for single-run mode)
	comparisonResult     *comparison.Result // Comparison result for report generation
	variantRunID         string             // Shared ID to group variant registry entries
	variantRegistryIDs   map[int]string     // Maps variant ID to registry entry ID
	prePromptSessionID   string             // Session ID from pre-prompt to pass to phase 1
}

// New creates a new Orbit instance.
func New(config Config) (*Orbit, error) {
	// Create debug logger with optional centralized file output
	dbg, err := debug.NewLogger(debug.LoggerConfig{
		StderrEnabled: config.Debug,
		FileEnabled:   config.CentralizedLog,
		RunID:         config.RunID,
		Prefix:        "orbit",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	// Output log path if centralized logging is enabled
	if config.CentralizedLog && dbg.Path() != "" {
		log.Printf("Centralized log: %s", dbg.Path())
	}

	runeClient := rune.NewClient(config.TasksFile)
	runeClient.SetDebug(config.Debug)

	// Set default clock for time operations
	if config.Clock == nil {
		config.Clock = RealClock{}
	}

	// Set default agent resolver
	if config.AgentResolver == nil {
		config.AgentResolver = DefaultAgentResolver
	}

	// Initialize agent and error classifier
	// Use the configured agent or default to Claude Code
	agentName := config.Agent
	if agentName == "" {
		agentName = "claude-code"
	}

	// Merge agent config with SkipPermissions
	agentConfig := config.AgentConfig
	if config.SkipPermissions {
		agentConfig.AutoApprove = true
	}

	agent, err := config.AgentResolver.GetAgent(agentName, agentConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent %q: %w", agentName, err)
	}

	// Get the error classifier for this agent
	errorClassifier := agents.GetClassifier(agentName)

	// Log configuration if debug enabled
	if config.Debug {
		dbg.LogConfig("TasksFile", config.TasksFile)
		dbg.LogConfig("LogDir", config.LogDir)
		dbg.LogConfig("BranchName", config.BranchName)
		dbg.LogConfig("SkipPermissions", config.SkipPermissions)
		dbg.LogConfig("Verbose", config.Verbose)
		dbg.LogConfig("DryRun", config.DryRun)
		dbg.LogConfig("WorkingDir", config.WorkingDir)
		dbg.LogConfig("Command", config.Command)
		dbg.LogConfig("PostPrompt", config.PostPrompt)
		dbg.LogConfig("DateSubdirs", config.DateSubdirs)
		dbg.LogConfig("ContinueSession", config.ContinueSession)
		// Variant configuration
		dbg.LogConfig("VariantCount", config.VariantCount)
		dbg.LogConfig("Parallel", config.Parallel)
		dbg.LogConfig("MaxParallel", config.MaxParallel)
		dbg.LogConfig("BranchPrefix", config.BranchPrefix)
		dbg.LogConfig("GlobalGuidance", config.GlobalGuidance)
	}

	var logManager *logs.Manager
	if !config.DryRun {
		var err error
		opts := logs.ManagerOptions{
			UseSubdirs: config.DateSubdirs,
		}
		logManager, err = logs.NewManagerWithOptions(config.LogDir, config.BranchName, config.WorkingDir, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to create log manager: %w", err)
		}
		// Set agent info for session logging
		var model string
		if agentConfig.Options != nil {
			model = agentConfig.Options["model"]
		}
		logManager.SetAgentInfo(agentName, agent.Name(), model)
		if config.Verbose {
			log.Printf("Logs will be saved to: %s", logManager.SessionDir())
			// Log resolved agent configuration
			log.Printf("Agent: %s (type: %s)", agentName, agent.Name())
			if model != "" {
				log.Printf("Model: %s", model)
			}
		}
	}

	// Create spinner (nil in dry-run mode or if not a TTY)
	var spin *display.Spinner
	if !config.DryRun {
		spin = display.NewSpinner()
	}

	// Set up graceful shutdown context for signal handling
	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)

	// Initialize registry for web interface integration
	// Failures are non-fatal (requirement 3.7)
	var reg *registry.Registry
	if !config.DryRun {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			registryDir := homeDir + "/.orbit/runs"
			reg, err = registry.New(registryDir)
			if err != nil {
				log.Printf("Warning: failed to initialize registry: %v", err)
				reg = nil
			}
		} else {
			log.Printf("Warning: failed to get home directory for registry: %v", err)
		}
	}

	// Initialize variant manager if variant mode is enabled
	var variantMgr *variants.Manager
	if config.VariantCount > 0 && !config.DryRun {
		// Validate required config for variant mode
		if config.SpecDir == "" {
			cancel()
			return nil, fmt.Errorf("SpecDir is required for variant mode")
		}
		if _, err := os.Stat(config.SpecDir); os.IsNotExist(err) {
			cancel()
			return nil, fmt.Errorf("spec directory does not exist: %s", config.SpecDir)
		}

		variantCfg := variants.Config{
			Count:        config.VariantCount,
			Parallel:     config.Parallel,
			MaxParallel:  config.MaxParallel,
			BranchPrefix: config.BranchPrefix,
			Guidance:     config.Guidance,
		}
		// Default branch prefix if not set
		if variantCfg.BranchPrefix == "" {
			variantCfg.BranchPrefix = "orbit-impl"
		}
		// Default max parallel if not set
		if variantCfg.MaxParallel == 0 {
			variantCfg.MaxParallel = 3
		}

		gitClient := variants.NewGit(config.RepoRoot)
		var err error
		// Derive spec name from the spec directory, not the branch name
		// e.g., "specs/enhanced-status" -> "enhanced-status"
		specName := filepath.Base(config.SpecDir)
		variantMgr, err = variants.NewManager(variantCfg, specName, config.SpecDir, config.RepoRoot, gitClient)
		if err != nil {
			cancel() // Clean up context
			return nil, fmt.Errorf("failed to create variant manager: %w", err)
		}
		// Load existing metadata if present
		if err := variantMgr.Load(); err != nil {
			cancel()
			return nil, fmt.Errorf("failed to load variant metadata: %w", err)
		}
	}

	return &Orbit{
		config:          config,
		runeClient:      runeClient,
		agent:           agent,
		errorClassifier: errorClassifier,
		logManager:      logManager,
		spinner:         spin,
		shutdownCtx:     ctx,
		shutdownCancel:  cancel,
		registry:        reg,
		debug:           dbg,
		variantManager:  variantMgr,
	}, nil
}

// Close releases resources and should be called via defer in main().
// Idempotent: calling Close() multiple times is safe.
func (o *Orbit) Close() {
	if o.shutdownCancel != nil {
		o.shutdownCancel()
	}
	if o.spinner != nil {
		o.spinner.Stop()
	}
	if o.debug != nil {
		o.debug.Close()
	}
}

// Run executes the orchestration loop until all tasks are complete.
func (o *Orbit) Run() error {
	log.Println("Starting Orbit orchestration...")
	log.Printf("Tasks file: %s", o.config.TasksFile)

	// Write startup entry to centralized log (must be first entry - Req 5.3)
	o.debug.LogStartup(debug.StartupConfig{
		OrbitVersion:     o.config.Version,
		Agent:            o.config.Agent,
		TasksFile:        o.config.TasksFile,
		WorkingDirectory: o.config.WorkingDir,
		BranchName:       o.config.BranchName,
	})

	// Log configuration sources to centralized log (Req 3.7)
	// This happens after LogStartup to ensure schema_version is in the first entry
	o.debug.LogStructured("info", "Configuration loaded", map[string]any{
		"agent":           o.config.Agent,
		"tasks_file":      o.config.TasksFile,
		"working_dir":     o.config.WorkingDir,
		"centralized_log": o.config.CentralizedLog,
	})

	// Check for variant mode
	if o.variantManager != nil {
		log.Printf("Variant mode enabled: %d variants", o.config.VariantCount)
		if o.config.Parallel {
			log.Printf("Running variants in parallel (max %d)", o.config.MaxParallel)
		}
		return o.runWithVariants(o.shutdownCtx)
	}

	// Single-run mode (existing behavior)
	return o.runSingle()
}

// getPhaseNumber returns the phase number for a given phase name.
// Returns 0 if the phase is not found.
func (o *Orbit) getPhaseNumber(phaseName string) int {
	for _, s := range o.phaseSummaries {
		if s.Name == phaseName {
			return s.Order
		}
	}
	return 0
}

// displayPhaseOverview shows a table of all phases with their status and task counts.
func (o *Orbit) displayPhaseOverview() error {
	summaries, err := o.runeClient.GetPhaseSummaries()
	if err != nil {
		return err
	}

	// Cache summaries for phase number lookup
	o.phaseSummaries = summaries

	if len(summaries) == 0 {
		return nil
	}

	// Build table data
	rows := make([]map[string]any, 0, len(summaries))
	for _, s := range summaries {
		status := string(s.Status)
		if status == "" {
			status = "-"
		}
		rows = append(rows, map[string]any{
			"#":         s.Order,
			"Phase":     s.Name,
			"Tasks":     s.Total,
			"Completed": s.Completed,
			"Pending":   s.Pending,
			"Status":    status,
		})
	}

	doc := output.New().
		Table("Phase Overview", rows, output.WithKeys("#", "Phase", "Tasks", "Completed", "Pending", "Status")).
		Build()

	out := output.NewOutput(
		output.WithFormat(output.Table()),
		output.WithWriter(output.NewStdoutWriter()),
	)

	fmt.Println() // Add blank line before table
	if err := out.Render(context.Background(), doc); err != nil {
		return fmt.Errorf("failed to render phase table: %w", err)
	}
	fmt.Println() // Add blank line after table

	return nil
}

// fail marks the orchestration as failed and returns the error.
func (o *Orbit) fail(err error) error {
	// Stop spinner before printing links
	if o.spinner != nil {
		o.spinner.Stop()
	}

	// Write shutdown entry to centralized log
	o.debug.LogShutdown("failed")

	// Update registry status to failed (req 3.3)
	o.updateRunStatus(registry.StatusFailed)

	if o.logManager != nil {
		_ = o.logManager.Fail(err)
		// Print index links even on failure for debugging
		display.PrintIndexLinks(o.logManager.SessionDir())
	}
	return err
}

