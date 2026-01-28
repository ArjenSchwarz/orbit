// Package orbit provides the main orchestration loop for running Claude Code sessions.
package orbit

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	output "github.com/ArjenSchwarz/go-output/v2"
	"github.com/arjenschwarz/orbit/internal/agents"
	_ "github.com/arjenschwarz/orbit/internal/agents/claudecode" // Register claudecode agent
	"github.com/arjenschwarz/orbit/internal/claude"
	"github.com/arjenschwarz/orbit/internal/comparison"
	"github.com/arjenschwarz/orbit/internal/debug"
	"github.com/arjenschwarz/orbit/internal/display"
	orberrors "github.com/arjenschwarz/orbit/internal/errors"
	"github.com/arjenschwarz/orbit/internal/logs"
	"github.com/arjenschwarz/orbit/internal/registry"
	"github.com/arjenschwarz/orbit/internal/report"
	"github.com/arjenschwarz/orbit/internal/rune"
	"github.com/arjenschwarz/orbit/internal/variants"
	"github.com/google/uuid"
)

const (
	maxRetries = 5
)

// claudeRunner is an interface for running Claude sessions.
// This allows for mocking in tests.
type claudeRunner interface {
	RunPhase(sessionID string, resume bool) (*agents.RunResult, error)
	RunCustomPrompt(prompt string) (*agents.RunResult, error)
	RunCustomPromptWithSession(prompt, sessionID string, resume bool) (*agents.RunResult, error)
}

// getCostUSD extracts the cost in USD from a RunResult, returning 0 if cost is nil.
func getCostUSD(result *agents.RunResult) float64 {
	if result == nil || result.Cost == nil {
		return 0
	}
	return result.Cost.CostUSD
}

// Config holds the orchestrator configuration.
type Config struct {
	TasksFile       string
	LogDir          string
	BranchName      string
	SkipPermissions bool
	Verbose         bool
	DryRun          bool
	Debug           bool // Enable debug logging for troubleshooting
	WorkingDir      string
	Command         string // Custom phase command
	PostCommand     string // Post-completion command (empty = disabled)
	DateSubdirs     bool   // If true, use timestamped subdirectories for logs
	ContinueSession bool   // If true, continue existing Claude sessions when resuming

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
}

// Orbit orchestrates Claude Code sessions to implement spec phases.
type Orbit struct {
	config               Config
	runeClient           *rune.Client
	claudeClient         claudeRunner
	agent                agents.Agent           // Agent interface for multi-agent support
	errorClassifier      agents.ErrorClassifier // Agent-specific error classifier
	logManager           *logs.Manager
	phaseSummaries       []rune.PhaseSummary
	spinner              *display.Spinner
	shutdownCtx          context.Context
	shutdownCancel       context.CancelFunc
	registry             *registry.Registry     // Web interface run registry
	runID                string                 // UUID of the current run in registry
	currentPhaseRunCount int                    // Track retry count for current phase
	debug                *debug.Logger          // Debug logger
	variantManager       *variants.Manager      // Variant lifecycle manager (nil for single-run mode)
	rawClaudeClient      *claude.Client         // Raw Claude client for variant mode
	comparisonResult     *comparison.Result     // Comparison result for report generation
	variantRunID         string                 // Shared ID to group variant registry entries
	variantRegistryIDs   map[int]string         // Maps variant ID to registry entry ID
}

// New creates a new Orbit instance.
func New(config Config) (*Orbit, error) {
	// Create debug logger
	dbg := debug.New(config.Debug, "orbit")

	runeClient := rune.NewClient(config.TasksFile)
	runeClient.SetDebug(config.Debug)

	claudeClient := claude.NewClient(claude.Config{
		SkipPermissions: config.SkipPermissions,
		WorkingDir:      config.WorkingDir,
		Prompt:          config.Command,
		Debug:           config.Debug,
	})

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

	agent, err := agents.Get(agentName, agentConfig)
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
		dbg.LogConfig("PostCommand", config.PostCommand)
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
		claudeClient:    claudeClient,
		agent:           agent,
		errorClassifier: errorClassifier,
		logManager:      logManager,
		spinner:         spin,
		shutdownCtx:     ctx,
		shutdownCancel:  cancel,
		registry:        reg,
		debug:           dbg,
		variantManager:  variantMgr,
		rawClaudeClient: claudeClient,
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
}

// Run executes the orchestration loop until all tasks are complete.
func (o *Orbit) Run() error {
	log.Println("Starting Orbit orchestration...")
	log.Printf("Tasks file: %s", o.config.TasksFile)

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

// runSingle executes the single-run orchestration loop (existing behavior).
func (o *Orbit) runSingle() error {
	// Display phase overview at startup
	if err := o.displayPhaseOverview(); err != nil {
		log.Printf("Warning: could not display phase overview: %v", err)
	}

	// Register run in web interface registry (req 3.1)
	if runID, err := o.registerRun(); err == nil && runID != "" {
		o.runID = runID
		if o.config.Verbose {
			log.Printf("Registered run: %s", runID)
		}
	}

	for {
		// Check for shutdown signal between phases
		select {
		case <-o.shutdownCtx.Done():
			o.debug.Log("Shutdown signal received")
			return o.fail(fmt.Errorf("interrupted by user"))
		default:
		}

		// Check for remaining tasks
		o.debug.Log("Checking for pending tasks...")
		pending, err := o.runeClient.ListPending()
		if err != nil {
			o.debug.Log("Failed to list pending tasks: %v", err)
			return o.fail(fmt.Errorf("failed to check pending tasks: %w", err))
		}
		o.debug.Log("Found %d pending tasks", len(pending))

		if len(pending) == 0 {
			log.Println("All tasks complete!")
			return o.complete()
		}

		// Get the next phase info
		o.debug.Log("Getting next phase...")
		nextPhase, err := o.runeClient.GetNextPhase()
		if err != nil {
			o.debug.Log("Failed to get next phase: %v", err)
			return o.fail(fmt.Errorf("failed to get next phase: %w", err))
		}
		o.debug.Log("Next phase: name=%s tasks=%d all_complete=%v", nextPhase.PhaseName, len(nextPhase.Tasks), nextPhase.AllComplete)

		if nextPhase.AllComplete {
			log.Println("All phases complete!")
			return o.complete()
		}

		// Get the actual phase number from the tasks file
		phaseNum := o.getPhaseNumber(nextPhase.PhaseName)

		if o.config.DryRun {
			log.Printf("[DRY RUN] Would execute phase %d with %d pending tasks", phaseNum, len(pending))
			log.Printf("[DRY RUN] Next phase: %s with %d tasks", nextPhase.PhaseName, len(nextPhase.Tasks))
			log.Printf("[DRY RUN] Phase command: %s", o.config.Command)
			if o.config.PostCommand != "" {
				log.Printf("[DRY RUN] Post-command: %s", o.config.PostCommand)
			} else {
				log.Printf("[DRY RUN] Post-command: (disabled)")
			}
			return nil
		}

		// Log phase start
		log.Printf("Starting phase %d: %s (%d tasks)", phaseNum, nextPhase.PhaseName, len(nextPhase.Tasks))

		// Run the phase
		if err := o.runPhaseWithRetry(phaseNum); err != nil {
			return o.fail(err)
		}

		log.Printf("Completed phase %d: %s", phaseNum, nextPhase.PhaseName)
	}
}

// complete handles successful orchestration completion, including post-command execution.
func (o *Orbit) complete() error {
	// Run post-command if configured
	if o.config.PostCommand != "" {
		log.Println("Running post-completion command...")
		if err := o.runPostCommandWithRetry(); err != nil {
			log.Printf("Orchestration succeeded but post-command failed: %v", err)
			return o.fail(err)
		}
		log.Println("Post-completion command finished")
	}

	// Update registry status to completed (req 3.2)
	o.updateRunStatus(registry.StatusCompleted)

	if o.logManager != nil {
		if err := o.logManager.Complete(); err != nil {
			return err
		}
		// Print index links after successful completion
		display.PrintIndexLinks(o.logManager.SessionDir())
	}
	return nil
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

// runPhaseWithRetry executes a phase with retry logic for transient errors.
func (o *Orbit) runPhaseWithRetry(phase int) error {
	var lastErr error
	o.currentPhaseRunCount = 0

	o.debug.Log("Starting phase %d with up to %d retries", phase, maxRetries)

	for attempt := 0; attempt < maxRetries; attempt++ {
		o.currentPhaseRunCount++
		o.debug.Log("Phase %d attempt %d/%d", phase, attempt+1, maxRetries)

		// Update phase status to running (req 3.5)
		o.updatePhaseStatus(phase, registry.PhaseStatusRunning, o.currentPhaseRunCount)

		err := o.runPhase(phase)
		if err == nil {
			o.debug.Log("Phase %d completed successfully on attempt %d", phase, attempt+1)
			// Update phase status to completed (req 3.6)
			o.updatePhaseStatus(phase, registry.PhaseStatusCompleted, o.currentPhaseRunCount)
			return nil
		}

		o.debug.Log("Phase %d attempt %d failed: %v", phase, attempt+1, err)

		// Handle agent-specific classified errors
		classified, ok := err.(*agents.ClassifiedError)
		if !ok {
			o.debug.Log("Error is not a ClassifiedError, not retrying: %T", err)
			// Unknown error type, don't retry
			return err
		}

		o.debug.LogError(classified.Class.String(), classified.Message, classified.Class.IsRetryable())
		lastErr = err

		if !classified.Class.IsRetryable() {
			o.debug.Log("Error class %s is not retryable, stopping", classified.Class)
			// Non-retryable error
			return err
		}

		// Pause spinner before logging to prevent visual artifacts
		if o.spinner != nil {
			o.spinner.Pause()
		}

		// Determine wait time using RetryAfter from classifier, with fallback to exponential backoff
		var waitTime time.Duration
		if classified.RetryAfter > 0 {
			waitTime = classified.RetryAfter
			log.Printf("Retryable error (attempt %d/%d). Waiting %s before retry...", attempt+1, maxRetries, waitTime)
		} else {
			waitTime = orberrors.BackoffDuration(attempt)
			log.Printf("Error (attempt %d/%d). Waiting %s before retry...", attempt+1, maxRetries, waitTime)
		}

		o.debug.LogRetry(attempt+1, maxRetries, classified.Class.String(), waitTime.String())

		// Resume spinner with wait countdown during retry wait
		if o.spinner != nil {
			o.spinner.UpdateWait(waitTime)
			o.spinner.Resume()
		}

		time.Sleep(waitTime)

		// Stop spinner before next phase attempt (runPhase will start it again)
		if o.spinner != nil {
			o.spinner.Stop()
		}
	}

	o.debug.Log("Phase %d failed after %d attempts", phase, maxRetries)
	// Update phase status to failed after max retries (req 3.6)
	o.updatePhaseStatus(phase, registry.PhaseStatusFailed, o.currentPhaseRunCount)

	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// runPhase executes a single phase.
func (o *Orbit) runPhase(phase int) error {
	startTime := time.Now()
	o.debug.Log("runPhase(%d) starting", phase)

	// Start spinner before Claude execution
	if o.spinner != nil {
		o.spinner.Start(phase)
	}

	// Get session ID and determine if resuming (req 3.1-3.3)
	var sessionID string
	var isResume bool
	if o.logManager != nil {
		var err error
		sessionID, isResume, err = o.logManager.StartPhase(phase, o.config.ContinueSession)
		if err != nil {
			o.debug.Log("Failed to start phase in log manager: %v", err)
			return o.fail(fmt.Errorf("failed to start phase: %w", err))
		}
		o.debug.LogSession(sessionID, isResume, "obtained from log manager")
	} else {
		sessionID = uuid.NewString()
		isResume = false
		o.debug.LogSession(sessionID, isResume, "generated new (no log manager)")
	}

	o.debug.Log("Executing Claude for phase %d...", phase)
	result, err := o.claudeClient.RunPhase(sessionID, isResume)
	o.debug.Log("Claude execution completed: err=%v", err)

	// Stop spinner after Claude returns
	if o.spinner != nil {
		o.spinner.Stop()
	}

	// Handle resume failure (req 3.7-3.9)
	if err != nil && isResume && isSessionInvalidError(result) {
		o.debug.Log("Session resume failed, detected invalid session error")
		log.Printf("Warning: Session resume failed, starting fresh session")
		sessionID = uuid.NewString()
		o.debug.LogSession(sessionID, false, "retrying with fresh session")
		if o.logManager != nil {
			if setErr := o.logManager.SetCurrentPhaseSessionID(sessionID); setErr != nil {
				log.Printf("Warning: failed to update session ID: %v", setErr)
				o.debug.Log("Failed to update session ID in log manager: %v", setErr)
			}
		}
		result, err = o.claudeClient.RunPhase(sessionID, false)
		o.debug.Log("Fresh session execution completed: err=%v", err)
	}

	if err != nil {
		o.debug.Log("Phase execution failed: %v", err)

		// Save the failed session for debugging
		if o.logManager != nil && result != nil {
			o.debug.Log("Saving failed session for debugging")
			_ = o.logManager.SaveSession(phase, result, startTime)
		}

		// Classify using agent-specific classifier
		o.debug.Log("Classifying error from stderr=%d bytes, output=%d bytes, errors=%v",
			len(result.Stderr), len(result.Output), result.Errors)
		classified := o.errorClassifier.Classify(1, result.Stderr, result.Output, result.Errors)
		o.debug.LogError(classified.Class.String(), classified.Message, classified.Class.IsRetryable())
		return classified
	}

	// Reconcile session ID if Claude returned a different one (req 2.5, 2.6)
	if o.logManager != nil && result.SessionID != sessionID {
		o.debug.Log("Session ID changed: expected=%s got=%s", sessionID, result.SessionID)
		o.logManager.ReconcileSessionID(result.SessionID)
	}

	// Check if Claude reported an error in its output
	if result.IsError {
		o.debug.Log("Claude reported error in output (IsError=true)")
		if o.logManager != nil {
			_ = o.logManager.SaveSession(phase, result, startTime)
		}
		classified := o.errorClassifier.Classify(1, result.Stderr, result.Output, result.Errors)
		o.debug.LogError(classified.Class.String(), classified.Message, classified.Class.IsRetryable())
		return classified
	}

	// Save successful session
	if o.logManager != nil {
		if err := o.logManager.SaveSession(phase, result, startTime); err != nil {
			log.Printf("Warning: failed to save session log: %v", err)
			o.debug.Log("Failed to save session log: %v", err)
		}
		// Complete phase (req 2.7)
		if err := o.logManager.CompletePhase(); err != nil {
			log.Printf("Warning: failed to complete phase: %v", err)
			o.debug.Log("Failed to complete phase in log manager: %v", err)
		}
	}

	// Export session for agents that require explicit export (e.g., Kiro) [Decision 8]
	if exporter, ok := o.agent.(agents.SessionExporter); ok {
		exportFilename := o.generateSessionExportFilename(phase)
		o.debug.Log("Exporting session to %s", exportFilename)
		if err := exporter.ExportSession(o.shutdownCtx, exportFilename); err != nil {
			// Handle export failures gracefully - log warning but don't fail orchestration
			log.Printf("Warning: failed to export session: %v", err)
			o.debug.Log("Session export failed: %v", err)
		} else {
			o.debug.Log("Session exported successfully to %s", exportFilename)
		}
	}

	o.debug.Log("Phase %d completed successfully: cost=$%.4f duration=%s turns=%d",
		phase, getCostUSD(result), result.Duration, result.NumTurns)

	if o.config.Verbose {
		log.Printf("Phase %d: cost=$%.4f, duration=%s, turns=%d",
			phase, getCostUSD(result), result.Duration, result.NumTurns)
	}

	return nil
}

// runPostCommand executes the post-completion command.
func (o *Orbit) runPostCommand() error {
	startTime := time.Now()
	o.debug.Log("runPostCommand starting")

	// Start spinner for post-completion
	if o.spinner != nil {
		o.spinner.StartPostCompletion()
	}

	// Get session ID and determine if resuming
	var sessionID string
	var isResume bool
	if o.logManager != nil {
		var err error
		sessionID, isResume, err = o.logManager.StartPostCompletion(o.config.ContinueSession)
		if err != nil {
			o.debug.Log("Failed to start post-completion in log manager: %v", err)
			return o.fail(fmt.Errorf("failed to start post-completion: %w", err))
		}
		o.debug.LogSession(sessionID, isResume, "post-completion obtained from log manager")
	} else {
		sessionID = uuid.NewString()
		isResume = false
		o.debug.LogSession(sessionID, isResume, "generated new (no log manager)")
	}

	o.debug.Log("Executing post-completion command with agent %s...", o.agent.Name())

	// Execute using the configured agent (not hardcoded to Claude)
	opts := agents.RunOptions{
		Prompt:    o.config.PostCommand,
		WorkDir:   o.config.WorkingDir,
		SessionID: sessionID,
	}

	var result *agents.RunResult
	var err error
	if isResume {
		result, err = o.agent.Resume(o.shutdownCtx, sessionID, opts)
	} else {
		result, err = o.agent.Run(o.shutdownCtx, opts)
	}
	o.debug.Log("Post-completion execution completed: err=%v", err)

	// Stop spinner after agent returns
	if o.spinner != nil {
		o.spinner.Stop()
	}

	// Handle resume failure
	if err != nil && isResume && isSessionInvalidError(result) {
		o.debug.Log("Post-completion session resume failed, detected invalid session error")
		log.Printf("Warning: Post-completion session resume failed, starting fresh session")
		sessionID = uuid.NewString()
		o.debug.LogSession(sessionID, false, "post-completion retrying with fresh session")
		if o.logManager != nil {
			if setErr := o.logManager.SetPostCompletionSessionID(sessionID); setErr != nil {
				log.Printf("Warning: failed to update post-completion session ID: %v", setErr)
				o.debug.Log("Failed to update post-completion session ID: %v", setErr)
			}
		}
		opts.SessionID = sessionID
		result, err = o.agent.Run(o.shutdownCtx, opts)
		o.debug.Log("Fresh post-completion execution completed: err=%v", err)
	}

	if err != nil {
		o.debug.Log("Post-completion execution failed: %v", err)
		// Save the failed session for debugging
		if o.logManager != nil && result != nil {
			o.debug.Log("Saving failed post-completion session for debugging")
			_ = o.logManager.SavePostCompletionSession(result, startTime)
		}
		classified := o.errorClassifier.Classify(1, result.Stderr, result.Output, result.Errors)
		o.debug.LogError(classified.Class.String(), classified.Message, classified.Class.IsRetryable())
		return classified
	}

	// Reconcile session ID if agent returned a different one
	if o.logManager != nil && result.SessionID != sessionID {
		o.debug.Log("Post-completion session ID changed: expected=%s got=%s", sessionID, result.SessionID)
		o.logManager.ReconcilePostCompletionSessionID(result.SessionID)
	}

	// Check if agent reported an error in its output
	if result.IsError {
		o.debug.Log("Agent reported error in post-completion output (IsError=true)")
		if o.logManager != nil {
			_ = o.logManager.SavePostCompletionSession(result, startTime)
		}
		classified := o.errorClassifier.Classify(1, result.Stderr, result.Output, result.Errors)
		o.debug.LogError(classified.Class.String(), classified.Message, classified.Class.IsRetryable())
		return classified
	}

	// Save successful session
	if o.logManager != nil {
		if err := o.logManager.SavePostCompletionSession(result, startTime); err != nil {
			log.Printf("Warning: failed to save post-completion log: %v", err)
			o.debug.Log("Failed to save post-completion log: %v", err)
		}
		// Complete post-completion
		if err := o.logManager.CompletePostCompletion(); err != nil {
			log.Printf("Warning: failed to complete post-completion: %v", err)
			o.debug.Log("Failed to complete post-completion in log manager: %v", err)
		}
	}

	o.debug.Log("Post-completion completed successfully: cost=$%.4f duration=%s turns=%d",
		getCostUSD(result), result.Duration, result.NumTurns)

	if o.config.Verbose {
		log.Printf("Post-completion: cost=$%.4f, duration=%s, turns=%d",
			getCostUSD(result), result.Duration, result.NumTurns)
	}

	return nil
}

// runPostCommandWithRetry executes the post-command with retry logic for transient errors.
func (o *Orbit) runPostCommandWithRetry() error {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		err := o.runPostCommand()
		if err == nil {
			return nil
		}

		// Handle agent-specific classified errors
		classified, ok := err.(*agents.ClassifiedError)
		if !ok {
			return err
		}

		lastErr = err

		if !classified.Class.IsRetryable() {
			return err
		}

		// Pause spinner before logging to prevent visual artifacts
		if o.spinner != nil {
			o.spinner.Pause()
		}

		// Determine wait time using RetryAfter from classifier, with fallback to exponential backoff
		var waitTime time.Duration
		if classified.RetryAfter > 0 {
			waitTime = classified.RetryAfter
			log.Printf("Retryable error (attempt %d/%d). Waiting %s before retry...", attempt+1, maxRetries, waitTime)
		} else {
			waitTime = orberrors.BackoffDuration(attempt)
			log.Printf("Error (attempt %d/%d). Waiting %s before retry...", attempt+1, maxRetries, waitTime)
		}

		// Resume spinner with wait countdown during retry wait
		if o.spinner != nil {
			o.spinner.UpdateWait(waitTime)
			o.spinner.Resume()
		}

		time.Sleep(waitTime)

		// Stop spinner before next attempt (runPostCommand will start it again)
		if o.spinner != nil {
			o.spinner.Stop()
		}
	}

	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// fail marks the orchestration as failed and returns the error.
func (o *Orbit) fail(err error) error {
	// Stop spinner before printing links
	if o.spinner != nil {
		o.spinner.Stop()
	}

	// Update registry status to failed (req 3.3)
	o.updateRunStatus(registry.StatusFailed)

	if o.logManager != nil {
		_ = o.logManager.Fail(err)
		// Print index links even on failure for debugging
		display.PrintIndexLinks(o.logManager.SessionDir())
	}
	return err
}

// isSessionInvalidError checks if the result contains a session-related error.
// This is used to detect when a session resume has failed and a fresh session
// should be started instead.
func isSessionInvalidError(result *agents.RunResult) bool {
	if result == nil {
		return false
	}

	// Check for session-related error messages
	combined := result.Stderr + result.Output
	sessionErrors := []string{
		"session not found",
		"invalid session",
		"session expired",
		"no such session",
	}

	combinedLower := strings.ToLower(combined)
	for _, msg := range sessionErrors {
		if strings.Contains(combinedLower, msg) {
			return true
		}
	}

	return false
}

// generateSessionExportFilename creates a filename for session export.
// The filename is placed in the log directory with the pattern: phase-N-agent-session.json
func (o *Orbit) generateSessionExportFilename(phase int) string {
	if o.logManager == nil {
		// Fallback if no log manager
		return fmt.Sprintf("phase-%d-%s-session.json", phase, o.agent.Name())
	}
	return filepath.Join(o.logManager.SessionDir(), fmt.Sprintf("phase-%d-run-%d-%s-session.json", phase, o.currentPhaseRunCount, o.agent.Name()))
}

// registerRun creates a new registry entry for this orchestration run.
// Returns the run ID and any error. Errors are logged but not fatal (req 3.7).
func (o *Orbit) registerRun() (string, error) {
	if o.registry == nil {
		return "", nil
	}

	entry := registry.NewRunEntry()
	entry.Name = o.config.BranchName
	entry.Repository = registry.GetRepository(o.config.WorkingDir)
	entry.Branch = o.config.BranchName
	entry.Status = registry.StatusRunning
	entry.StartedAt = time.Now()

	// Set log directory (convert to absolute path for web interface access)
	var logDir string
	if o.logManager != nil {
		logDir = o.logManager.SessionDir()
	} else {
		logDir = o.config.LogDir
	}
	absLogDir, err := filepath.Abs(logDir)
	if err != nil {
		log.Printf("Warning: failed to get absolute path for log dir: %v", err)
		absLogDir = logDir // Fall back to original
	}
	entry.LogDir = absLogDir

	// Set run number for file naming (defaults to 1 if no log manager)
	if o.logManager != nil {
		entry.RunNumber = o.logManager.RunNumber()
	} else {
		entry.RunNumber = 1
	}

	// Set PID for auto-registered runs
	pid := os.Getpid()
	entry.PID = &pid

	if err := o.registry.Register(entry); err != nil {
		log.Printf("Warning: failed to register run: %v", err)
		return "", nil
	}

	return entry.ID, nil
}

// registerVariantRun creates registry entries for each variant.
// Each variant gets its own entry with variant-specific metadata.
// Returns the shared variant run ID. Errors are logged but not fatal.
func (o *Orbit) registerVariantRun(variantList []*variants.Variant) string {
	if o.registry == nil {
		return ""
	}

	// Generate a shared ID to group all variants from this run
	variantRunID := uuid.NewString()
	o.variantRunID = variantRunID
	o.variantRegistryIDs = make(map[int]string)

	pid := os.Getpid()
	now := time.Now()
	variantTotal := len(variantList)

	for _, v := range variantList {
		entry := registry.NewRunEntry()
		entry.Name = fmt.Sprintf("%s [variant %d/%d]", o.config.BranchName, v.ID, variantTotal)
		entry.Repository = registry.GetRepository(o.config.WorkingDir)
		entry.Branch = o.config.BranchName
		entry.Status = registry.StatusRunning
		entry.StartedAt = now
		entry.PID = &pid
		entry.RunNumber = 1

		// Set variant-specific fields
		entry.IsVariant = true
		entry.VariantID = v.ID
		entry.VariantRunID = variantRunID
		entry.VariantTotal = variantTotal
		entry.VariantAgent = v.Agent
		entry.VariantBranch = v.Branch

		// Set log directory to this variant's log directory
		variantLogDir := filepath.Join(o.config.SpecDir, ".orbit", "logs", fmt.Sprintf("variant-%d", v.ID))
		absLogDir, err := filepath.Abs(variantLogDir)
		if err != nil {
			log.Printf("Warning: failed to get absolute path for variant %d log dir: %v", v.ID, err)
			absLogDir = variantLogDir
		}
		entry.LogDir = absLogDir

		if err := o.registry.Register(entry); err != nil {
			log.Printf("Warning: failed to register variant %d: %v", v.ID, err)
			continue
		}

		o.variantRegistryIDs[v.ID] = entry.ID
	}

	return variantRunID
}

// updateVariantRegistryStatus updates a variant's status in the registry.
// Failures are logged but not fatal.
func (o *Orbit) updateVariantRegistryStatus(variantID int, status registry.RunStatus) {
	if o.registry == nil || o.variantRegistryIDs == nil {
		return
	}

	registryID, ok := o.variantRegistryIDs[variantID]
	if !ok {
		return
	}

	entry, err := o.registry.Get(registryID)
	if err != nil {
		log.Printf("Warning: failed to get variant %d registry entry: %v", variantID, err)
		return
	}
	if entry == nil {
		return
	}

	entry.Status = status
	if status == registry.StatusCompleted || status == registry.StatusFailed {
		now := time.Now()
		entry.FinishedAt = &now
	}

	if err := o.registry.Register(entry); err != nil {
		log.Printf("Warning: failed to update variant %d status: %v", variantID, err)
	}
}

// updatePhaseStatus updates the phase status in the registry.
// Failures are logged but not fatal (req 3.7).
func (o *Orbit) updatePhaseStatus(phaseNum int, status registry.PhaseStatus, runCount int) {
	if o.registry == nil || o.runID == "" {
		return
	}

	phase := registry.Phase{
		Number:   phaseNum,
		Status:   status,
		RunCount: runCount,
	}

	if err := o.registry.UpdatePhase(o.runID, phase); err != nil {
		log.Printf("Warning: failed to update phase status: %v", err)
	}
}

// updateRunStatus updates the run status in the registry.
// Failures are logged but not fatal (req 3.7).
func (o *Orbit) updateRunStatus(status registry.RunStatus) {
	if o.registry == nil || o.runID == "" {
		return
	}

	entry, err := o.registry.Get(o.runID)
	if err != nil {
		log.Printf("Warning: failed to get run for status update: %v", err)
		return
	}
	if entry == nil {
		log.Printf("Warning: run entry not found for status update: %s", o.runID)
		return
	}

	entry.Status = status
	now := time.Now()
	entry.FinishedAt = &now

	if err := o.registry.Register(entry); err != nil {
		log.Printf("Warning: failed to update run status: %v", err)
	}
}

// runWithVariants orchestrates multi-variant execution.
func (o *Orbit) runWithVariants(ctx context.Context) error {
	// Check for existing run and prompt user
	continueExisting := false
	if o.variantManager.HasExistingRun() {
		cont, proceed := o.promptContinueOrRestart()
		if !proceed {
			return fmt.Errorf("variant run canceled by user")
		}
		continueExisting = cont
	}

	// Setup worktrees
	if err := o.variantManager.Setup(ctx, continueExisting); err != nil {
		return fmt.Errorf("setup variants: %w", err)
	}

	// Snapshot variants slice under lock to avoid race condition during parallel execution
	variantList := o.variantManager.GetVariantsSnapshot()

	// Assign agents to variants [Req 10.3]
	variants.AssignVariantAgents(variantList, o.config.VariantAgents, o.config.Agent)

	// Register each variant in web interface registry
	variantRunID := o.registerVariantRun(variantList)
	if variantRunID != "" && o.config.Verbose {
		log.Printf("Registered %d variants with run ID: %s", len(variantList), variantRunID)
	}

	log.Printf("Running %d variants...", len(variantList))

	// Create context with cancellation for interrupt handling
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Handle SIGINT gracefully
	go func() {
		select {
		case <-o.shutdownCtx.Done():
			log.Println("Interrupt received, stopping new phases (running phases will complete)")
			cancel()
		case <-ctx.Done():
		}
	}()

	if o.config.Parallel {
		log.Printf("Starting parallel execution of %d variants...", len(variantList))
		o.runVariantsParallel(ctx, variantList)
		log.Println("All variant goroutines completed")
	} else {
		o.runVariantsSequential(ctx, variantList)
	}

	// Count successes
	successCount := o.variantManager.CountByStatus(variants.StatusCompleted)
	failedCount := o.variantManager.CountByStatus(variants.StatusFailed)
	canceledCount := o.variantManager.CountByStatus(variants.StatusCanceled)

	log.Printf("Variant execution complete: %d succeeded, %d failed, %d canceled",
		successCount, failedCount, canceledCount)

	// Generate report based on outcomes
	if successCount == 0 {
		log.Println("All variants failed; generating partial report")
		return o.generatePartialReport()
	}

	if successCount == 1 {
		log.Println("Only one variant succeeded; skipping comparison")
		return o.generateReport()
	}

	// Compare multiple successful variants
	if err := o.runComparison(ctx); err != nil {
		log.Printf("Comparison failed: %v", err)
		// Still try to generate a report without comparison
	}

	return o.generateReport()
}

// promptContinueOrRestart asks the user whether to continue an existing run or start fresh.
// Returns (continueExisting, shouldProceed). If shouldProceed is false, the run should be aborted.
func (o *Orbit) promptContinueOrRestart() (bool, bool) {
	fmt.Println("\nExisting variant run detected.")
	fmt.Println("  [c] Continue existing run")
	fmt.Println("  [n] Start new run (cleanup and recreate)")
	fmt.Println("  [q] Cancel")
	fmt.Print("\nChoice [c/n/q]: ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, false
	}

	input = strings.TrimSpace(strings.ToLower(input))
	switch input {
	case "c", "continue":
		return true, true
	case "n", "new":
		return false, true
	case "q", "quit", "cancel":
		return false, false
	default:
		fmt.Println("Invalid choice, canceling")
		return false, false
	}
}

// runVariantsSequential runs variants one at a time.
func (o *Orbit) runVariantsSequential(ctx context.Context, variantList []*variants.Variant) {
	for _, v := range variantList {
		select {
		case <-ctx.Done():
			_ = o.variantManager.UpdateStatus(v.ID, variants.StatusCanceled, nil)
			continue
		default:
		}

		// Skip variants that are already completed (for continue mode)
		if v.Status == variants.StatusCompleted {
			log.Printf("Variant %d: already completed, skipping", v.ID)
			continue
		}

		if err := o.runVariant(ctx, v); err != nil {
			log.Printf("Variant %d failed: %v", v.ID, err)
		}
	}
}

// runVariantsParallel runs variants concurrently with semaphore limiting.
func (o *Orbit) runVariantsParallel(ctx context.Context, variantList []*variants.Variant) {
	sem := make(chan struct{}, o.config.MaxParallel)
	var wg sync.WaitGroup

	for _, v := range variantList {
		// Skip variants that are already completed (for continue mode)
		if v.Status == variants.StatusCompleted {
			log.Printf("Variant %d: already completed, skipping", v.ID)
			continue
		}

		wg.Add(1)
		go func(variant *variants.Variant) {
			defer wg.Done()

			// Check for cancellation before acquiring semaphore
			select {
			case <-ctx.Done():
				_ = o.variantManager.UpdateStatus(variant.ID, variants.StatusCanceled, nil)
				return
			default:
			}

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			// Check again after acquiring semaphore
			select {
			case <-ctx.Done():
				_ = o.variantManager.UpdateStatus(variant.ID, variants.StatusCanceled, nil)
				return
			default:
			}

			if err := o.runVariant(ctx, variant); err != nil {
				log.Printf("Variant %d failed: %v", variant.ID, err)
			}
		}(v)
	}

	wg.Wait()
}

// runVariant executes all spec phases for a single variant in its worktree.
func (o *Orbit) runVariant(ctx context.Context, v *variants.Variant) error {
	startTime := time.Now()
	log.Printf("Starting variant %d (branch: %s, agent: %s)", v.ID, v.Branch, v.Agent)

	// Mark variant as running
	if err := o.variantManager.UpdateStatus(v.ID, variants.StatusRunning, nil); err != nil {
		log.Printf("Warning: failed to update variant %d status: %v", v.ID, err)
	}
	// Registry status is already set to running during registration

	// Create the agent for this variant using its assigned agent alias
	agentAlias := v.Agent
	if agentAlias == "" {
		agentAlias = "claude-code" // Default to Claude Code
	}

	// Get agent config from config file, falling back to defaults
	variantAgentConfig := o.getAgentConfig(agentAlias)
	// Ensure AutoApprove is set based on SkipPermissions flag
	if o.config.SkipPermissions {
		variantAgentConfig.AutoApprove = true
	}

	// Resolve the agent type: use Type from config, or fall back to alias name
	// (for backwards compatibility when alias name equals type name)
	agentType := variantAgentConfig.Type
	if agentType == "" {
		agentType = agentAlias
	}

	variantAgent, err := agents.Get(agentType, variantAgentConfig)
	if err != nil {
		return fmt.Errorf("failed to get agent %q (type: %s) for variant %d: %w", agentAlias, agentType, v.ID, err)
	}

	// agentType is already resolved above from config or alias
	var model string
	if variantAgentConfig.Options != nil {
		model = variantAgentConfig.Options["model"]
	}
	if err := o.variantManager.UpdateAgentInfo(v.ID, agentAlias, agentType, model); err != nil {
		log.Printf("Warning: failed to update agent info for variant %d: %v", v.ID, err)
	}

	// Log resolved agent configuration in verbose mode
	if o.config.Verbose {
		if model != "" {
			log.Printf("Variant %d agent config: alias=%s, type=%s, model=%s", v.ID, agentAlias, agentType, model)
		} else {
			log.Printf("Variant %d agent config: alias=%s, type=%s", v.ID, agentAlias, agentType)
		}
	}

	// Build the prompt for this variant
	variantPrompt := o.buildVariantPrompt(v)

	// Create a rune client for this variant's worktree
	// The tasks file path needs to be adjusted for the worktree
	tasksFile := o.config.TasksFile
	if o.config.RepoRoot != "" {
		// Convert to absolute path first if relative
		absTasksFile := tasksFile
		if !filepath.IsAbs(tasksFile) {
			absTasksFile = filepath.Join(o.config.RepoRoot, tasksFile)
		}
		// Get path relative to repo root, then rebase to worktree
		relPath, err := filepath.Rel(o.config.RepoRoot, absTasksFile)
		if err == nil {
			tasksFile = filepath.Join(v.WorktreePath, relPath)
		}
	}
	o.debug.Log("Variant %d using tasks file: %s", v.ID, tasksFile)
	variantRuneClient := rune.NewClient(tasksFile)
	variantRuneClient.SetDebug(o.config.Debug)

	// Create log manager for this variant
	// Logs are stored in the main spec directory under variant-specific subdirectory
	variantLogDir := filepath.Join(o.config.SpecDir, ".orbit", "logs", fmt.Sprintf("variant-%d", v.ID))
	variantBranchName := fmt.Sprintf("variant-%d", v.ID)
	variantLogManager, err := logs.NewManagerWithOptions(variantLogDir, variantBranchName, v.WorktreePath, logs.ManagerOptions{
		UseSubdirs: false, // Use flat structure for variant logs
	})
	if err != nil {
		log.Printf("Warning: failed to create log manager for variant %d: %v", v.ID, err)
		// Continue without logging - variant execution is more important
		variantLogManager = nil
	}
	if variantLogManager != nil {
		// Set agent info for session logging
		variantLogManager.SetAgentInfo(agentAlias, agentType, model)
	}

	// Track total metrics across all phases
	var totalCost float64
	var totalTurns int

	// Track last logged phase and per-phase cost
	var lastLoggedPhase string
	var phaseCost float64

	// Track phase number for logging
	phaseNum := 0

	// Get phase summaries for phase number lookup
	phaseSummaries, err := variantRuneClient.GetPhaseSummaries()
	if err != nil {
		o.debug.Log("Variant %d: failed to get phase summaries: %v", v.ID, err)
		// Continue without phase numbers - logging will use incremental numbers
	}

	// Helper to get phase number from name
	getPhaseNumber := func(phaseName string) int {
		for _, s := range phaseSummaries {
			if s.Name == phaseName {
				return s.Order
			}
		}
		return 0
	}

	// Run all phases in this variant
	for {
		// Check for cancellation
		select {
		case <-ctx.Done():
			if err := o.variantManager.UpdateStatus(v.ID, variants.StatusCanceled, nil); err != nil {
				log.Printf("Warning: failed to update variant %d status: %v", v.ID, err)
			}
			o.updateVariantRegistryStatus(v.ID, registry.StatusFailed) // Canceled = failed in registry
			return ctx.Err()
		default:
		}

		// Get next phase
		nextPhase, err := variantRuneClient.GetNextPhase()
		if err != nil {
			variantErr := fmt.Errorf("get next phase: %w", err)
			if updateErr := o.variantManager.UpdateStatus(v.ID, variants.StatusFailed, variantErr); updateErr != nil {
				log.Printf("Warning: failed to update variant %d status: %v", v.ID, updateErr)
			}
			return variantErr
		}

		if nextPhase.AllComplete {
			break
		}

		totalTasks := len(nextPhase.Tasks)
		currentPhase := nextPhase.PhaseName
		phaseNum = getPhaseNumber(currentPhase)
		if phaseNum == 0 {
			phaseNum++ // Fallback to incremental if phase not found in summaries
		}

		// Log when entering a new phase (and log completion of previous phase)
		if currentPhase != lastLoggedPhase {
			if lastLoggedPhase != "" {
				log.Printf("Variant %d: finished phase %s (cost=$%.4f)", v.ID, lastLoggedPhase, phaseCost)
				phaseCost = 0 // Reset for new phase
			}
			log.Printf("Variant %d: starting phase %s (%d tasks)", v.ID, currentPhase, totalTasks)
			lastLoggedPhase = currentPhase
		}

		phaseStartTime := time.Now()

		// Run the phase with retry
		phaseResult, err := o.runVariantPhaseWithRetry(ctx, v, variantAgent, variantPrompt)
		if err != nil {
			// Save failed session for debugging
			if variantLogManager != nil && phaseResult != nil {
				_ = variantLogManager.SaveSession(phaseNum, phaseResult, phaseStartTime)
			}
			variantErr := fmt.Errorf("phase %s: %w", currentPhase, err)
			if updateErr := o.variantManager.UpdateStatus(v.ID, variants.StatusFailed, variantErr); updateErr != nil {
				log.Printf("Warning: failed to update variant %d status: %v", v.ID, updateErr)
			}
			o.updateVariantRegistryStatus(v.ID, registry.StatusFailed)
			if variantLogManager != nil {
				_ = variantLogManager.Fail(variantErr)
			}
			return variantErr
		}

		// Save successful session
		if variantLogManager != nil && phaseResult != nil {
			if saveErr := variantLogManager.SaveSession(phaseNum, phaseResult, phaseStartTime); saveErr != nil {
				log.Printf("Warning: variant %d failed to save session log: %v", v.ID, saveErr)
			}
		}

		// Accumulate metrics
		if phaseResult != nil {
			phaseCost += getCostUSD(phaseResult)
			totalCost += getCostUSD(phaseResult)
			totalTurns += phaseResult.NumTurns
		}
	}

	// Log final phase completion
	if lastLoggedPhase != "" && phaseCost > 0 {
		log.Printf("Variant %d: finished phase %s (cost=$%.4f)", v.ID, lastLoggedPhase, phaseCost)
	}

	// Run post-completion command if configured
	if o.config.PostCommand != "" {
		log.Printf("Variant %d: running post-completion command...", v.ID)
		postStartTime := time.Now()
		postResult, err := o.runVariantPostCompletion(ctx, v, variantAgent)
		if err != nil {
			// Save failed post-completion session for debugging
			if variantLogManager != nil && postResult != nil {
				_ = variantLogManager.SavePostCompletionSession(postResult, postStartTime)
			}
			variantErr := fmt.Errorf("post-completion: %w", err)
			if updateErr := o.variantManager.UpdateStatus(v.ID, variants.StatusFailed, variantErr); updateErr != nil {
				log.Printf("Warning: failed to update variant %d status: %v", v.ID, updateErr)
			}
			o.updateVariantRegistryStatus(v.ID, registry.StatusFailed)
			if variantLogManager != nil {
				_ = variantLogManager.Fail(variantErr)
			}
			return variantErr
		}
		// Save successful post-completion session
		if variantLogManager != nil && postResult != nil {
			if saveErr := variantLogManager.SavePostCompletionSession(postResult, postStartTime); saveErr != nil {
				log.Printf("Warning: variant %d failed to save post-completion log: %v", v.ID, saveErr)
			}
		}
		if postResult != nil {
			totalCost += getCostUSD(postResult)
			totalTurns += postResult.NumTurns
		}
		log.Printf("Variant %d: post-completion finished", v.ID)
	}

	// Mark variant as completed
	duration := time.Since(startTime)
	if err := o.variantManager.UpdateStatus(v.ID, variants.StatusCompleted, nil); err != nil {
		log.Printf("Warning: failed to update variant %d status: %v", v.ID, err)
	}
	if err := o.variantManager.UpdateMetrics(v.ID, totalCost, duration, totalTurns); err != nil {
		log.Printf("Warning: failed to update variant %d metrics: %v", v.ID, err)
	}
	o.updateVariantRegistryStatus(v.ID, registry.StatusCompleted)

	// Mark variant log as complete
	if variantLogManager != nil {
		if completeErr := variantLogManager.Complete(); completeErr != nil {
			log.Printf("Warning: variant %d failed to complete log: %v", v.ID, completeErr)
		}
	}

	log.Printf("Variant %d completed: cost=$%.4f, duration=%s, turns=%d",
		v.ID, totalCost, duration.Round(time.Second), totalTurns)

	return nil
}

// buildVariantPrompt constructs the phase prompt with variant-specific guidance.
func (o *Orbit) buildVariantPrompt(v *variants.Variant) string {
	basePrompt := o.config.Command

	// Add variant-specific guidance
	if v.Guidance != "" {
		basePrompt = fmt.Sprintf(`%s

## Guidance for this Implementation

%s`, basePrompt, v.Guidance)
	}

	// Add global guidance if present
	if o.config.GlobalGuidance != "" {
		basePrompt = fmt.Sprintf(`%s

## Global Guidance

%s`, basePrompt, o.config.GlobalGuidance)
	}

	return basePrompt
}

// runVariantPhaseWithRetry executes a single phase with retry logic.
func (o *Orbit) runVariantPhaseWithRetry(ctx context.Context, v *variants.Variant, agent agents.Agent, prompt string) (*agents.RunResult, error) {
	var lastErr error
	var lastResult *agents.RunResult

	for attempt := 0; attempt < maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Execute the agent with the variant's prompt and working directory.
		// Each phase gets a fresh session ID intentionally - variants are isolated
		// implementations that don't share session history. This ensures clean
		// comparison between variants without cross-contamination from previous phases.
		opts := agents.RunOptions{
			Prompt:    prompt,
			SessionID: uuid.NewString(),
			WorkDir:   v.WorktreePath,
		}
		result, err := agent.Run(ctx, opts)
		if err == nil && result != nil && !result.IsError {
			return result, nil
		}

		lastResult = result
		if err != nil {
			lastErr = err
		} else if result != nil && result.IsError {
			lastErr = fmt.Errorf("agent reported error")
		}

		// Classify the error using agent-specific classifier
		classifier := agents.GetClassifier(agent.Name())
		classified := classifier.Classify(1, result.Stderr, result.Output, result.Errors)
		if !classified.Class.IsRetryable() {
			return result, classified
		}

		// Determine wait time using RetryAfter from classifier, with fallback to exponential backoff
		var waitTime time.Duration
		if classified.RetryAfter > 0 {
			waitTime = classified.RetryAfter
			log.Printf("Variant %d: retryable error, waiting %s (attempt %d/%d)",
				v.ID, waitTime, attempt+1, maxRetries)
		} else {
			waitTime = orberrors.BackoffDuration(attempt)
			log.Printf("Variant %d: error, waiting %s (attempt %d/%d)",
				v.ID, waitTime, attempt+1, maxRetries)
		}

		select {
		case <-ctx.Done():
			return lastResult, ctx.Err()
		case <-time.After(waitTime):
		}
	}

	return lastResult, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// runVariantPostCompletion executes the post-completion command for a variant.
func (o *Orbit) runVariantPostCompletion(ctx context.Context, v *variants.Variant, agent agents.Agent) (*agents.RunResult, error) {
	var lastErr error
	var lastResult *agents.RunResult

	for attempt := 0; attempt < maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Execute the post-completion command in the variant's worktree
		opts := agents.RunOptions{
			Prompt:    o.config.PostCommand,
			SessionID: uuid.NewString(),
			WorkDir:   v.WorktreePath,
		}
		result, err := agent.Run(ctx, opts)
		if err == nil && result != nil && !result.IsError {
			return result, nil
		}

		lastResult = result
		if err != nil {
			lastErr = err
		} else if result != nil && result.IsError {
			lastErr = fmt.Errorf("agent reported error in post-completion")
		}

		// Classify the error using agent-specific classifier
		classifier := agents.GetClassifier(agent.Name())
		classified := classifier.Classify(1, result.Stderr, result.Output, result.Errors)
		if !classified.Class.IsRetryable() {
			return result, classified
		}

		// Determine wait time using RetryAfter from classifier, with fallback to exponential backoff
		var waitTime time.Duration
		if classified.RetryAfter > 0 {
			waitTime = classified.RetryAfter
			log.Printf("Variant %d post-completion: retryable error, waiting %s (attempt %d/%d)",
				v.ID, waitTime, attempt+1, maxRetries)
		} else {
			waitTime = orberrors.BackoffDuration(attempt)
			log.Printf("Variant %d post-completion: error, waiting %s (attempt %d/%d)",
				v.ID, waitTime, attempt+1, maxRetries)
		}

		select {
		case <-ctx.Done():
			return lastResult, ctx.Err()
		case <-time.After(waitTime):
		}
	}

	return lastResult, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// runComparison performs the comparison of successful variants.
func (o *Orbit) runComparison(ctx context.Context) error {
	log.Println("Running comparison of successful variants...")

	metadata := o.variantManager.GetMetadata()
	if metadata == nil {
		return fmt.Errorf("no variant metadata available")
	}

	// Gather summaries from successful variants (not full diffs - they use too much context)
	gitClient := variants.NewGit(o.config.RepoRoot)
	diffGatherer := comparison.NewDiffGatherer(gitClient)
	variantList := o.variantManager.GetVariantsSnapshot()

	variantData, err := diffGatherer.GatherSummaries(ctx, metadata.BaseCommit, variantList)
	if err != nil {
		return fmt.Errorf("gather summaries: %w", err)
	}

	if len(variantData) < 2 {
		log.Println("Not enough successful variants for comparison (need at least 2)")
		return nil
	}

	// Read spec context for additional context
	specContext := o.readSpecContext()

	// Create comparator and run comparison using summaries only (diffs excluded to save context)
	comparator := comparison.NewComparator(o.rawClaudeClient, o.config.CompareCommand)
	result, err := comparator.CompareWithSummaries(ctx, o.config.BranchName, variantData, specContext)
	if err != nil {
		return fmt.Errorf("compare variants: %w", err)
	}

	log.Printf("Comparison complete: recommends variant %d (confidence: %s)",
		result.Recommendation, result.Confidence)
	log.Printf("Summary: %s", result.Summary)

	// Store comparison result for report generation
	o.comparisonResult = result

	return nil
}

// generateReport creates the HTML comparison report.
func (o *Orbit) generateReport() error {
	log.Println("Generating comparison report...")

	metadata := o.variantManager.GetMetadata()
	if metadata == nil {
		return fmt.Errorf("no variant metadata available")
	}

	// Gather diffs for report
	gitClient := variants.NewGit(o.config.RepoRoot)
	diffGatherer := comparison.NewDiffGatherer(gitClient)
	variantList := o.variantManager.GetVariantsSnapshot()

	variantData, err := diffGatherer.GatherDiffs(context.Background(), metadata.BaseCommit, variantList)
	if err != nil {
		log.Printf("Warning: could not gather diffs for report: %v", err)
	}

	// Build variant data map for lookup
	variantDiffs := make(map[int]string)
	for _, vd := range variantData {
		variantDiffs[vd.ID] = vd.Diff
	}

	// Build report data
	reportVariants := make([]report.VariantReportData, 0, len(variantList))
	for _, v := range variantList {
		reportVariants = append(reportVariants, report.VariantReportData{
			ID:       v.ID,
			Branch:   v.Branch,
			Status:   string(v.Status),
			Error:    v.Error,
			Diff:     variantDiffs[v.ID],
			Agent:    v.Agent,
			Metrics: report.VariantMetrics{
				Cost:     v.Cost,
				Duration: v.Duration.Round(time.Second).String(),
				NumTurns: v.NumTurns,
			},
		})
	}

	reportData := &report.ReportData{
		SpecName:       o.config.BranchName,
		GeneratedAt:    time.Now(),
		Variants:       reportVariants,
		Comparison:     o.comparisonResult,
		BaseCommit:     metadata.BaseCommit,
		OriginalBranch: metadata.OriginalBranch,
		VariantCommits: o.variantManager.GetVariantCommits(),
	}

	// Create report in comparison-report directory under spec
	reportDir := filepath.Join(o.config.SpecDir, "comparison-report")
	generator := report.NewGenerator(reportDir)

	if err := generator.Generate(reportData); err != nil {
		return fmt.Errorf("generate report: %w", err)
	}

	log.Printf("Report generated: %s/index.html", reportDir)
	return nil
}

// generatePartialReport creates a report when all variants failed.
func (o *Orbit) generatePartialReport() error {
	log.Println("Generating partial report (all variants failed)...")

	metadata := o.variantManager.GetMetadata()
	if metadata == nil {
		return fmt.Errorf("no variant metadata available")
	}

	variantList := o.variantManager.GetVariantsSnapshot()

	// Build report data with failure info
	reportVariants := make([]report.VariantReportData, 0, len(variantList))
	for _, v := range variantList {
		reportVariants = append(reportVariants, report.VariantReportData{
			ID:     v.ID,
			Branch: v.Branch,
			Status: string(v.Status),
			Error:  v.Error,
			Agent:  v.Agent,
			Metrics: report.VariantMetrics{
				Cost:     v.Cost,
				Duration: v.Duration.Round(time.Second).String(),
				NumTurns: v.NumTurns,
			},
		})
	}

	reportData := &report.ReportData{
		SpecName:       o.config.BranchName,
		GeneratedAt:    time.Now(),
		Variants:       reportVariants,
		Comparison:     nil, // No comparison for all-failed case
		BaseCommit:     metadata.BaseCommit,
		OriginalBranch: metadata.OriginalBranch,
		VariantCommits: o.variantManager.GetVariantCommits(),
	}

	// Create report in comparison-report directory under spec
	reportDir := filepath.Join(o.config.SpecDir, "comparison-report")
	generator := report.NewGenerator(reportDir)

	if err := generator.Generate(reportData); err != nil {
		return fmt.Errorf("generate partial report: %w", err)
	}

	log.Printf("Partial report generated: %s/index.html", reportDir)
	return fmt.Errorf("all variants failed")
}

// readSpecContext reads key spec files to provide context for comparison.
func (o *Orbit) readSpecContext() string {
	var parts []string

	// Key spec files to include
	specFiles := []struct {
		name  string
		label string
	}{
		{"requirements.md", "Requirements"},
		{"design.md", "Design"},
		{"tasks.md", "Tasks"},
	}

	for _, sf := range specFiles {
		path := filepath.Join(o.config.SpecDir, sf.name)
		content, err := os.ReadFile(path)
		if err != nil {
			continue // Skip files that don't exist
		}

		// Truncate if too long
		s := string(content)
		if len(s) > 3000 {
			idx := strings.LastIndex(s[:3000], "\n")
			if idx > 2500 {
				s = s[:idx] + "\n... (truncated)"
			} else {
				s = s[:3000] + "... (truncated)"
			}
		}

		parts = append(parts, fmt.Sprintf("### %s\n\n%s", sf.label, s))
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, "\n\n")
}

// getAgentConfig returns the agent configuration for a given agent name.
// It first checks the AgentConfigs map (from config file), then falls back to
// the default AgentConfig if the agent matches the configured default agent,
// or returns a default config with AutoApprove enabled.
func (o *Orbit) getAgentConfig(agentName string) agents.AgentConfig {
	// Check per-agent configs from config file
	if o.config.AgentConfigs != nil {
		if cfg, ok := o.config.AgentConfigs[agentName]; ok {
			return cfg
		}
	}

	// If this is the default agent, use its config
	if agentName == o.config.Agent {
		return o.config.AgentConfig
	}

	// Return default config with AutoApprove enabled for non-interactive operation
	return agents.AgentConfig{
		AutoApprove: true,
	}
}
