package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/arjenschwarz/orbit/internal/agents"
	_ "github.com/arjenschwarz/orbit/internal/agents/claudecode" // Register claude-code agent
	_ "github.com/arjenschwarz/orbit/internal/agents/codex"      // Register codex agent
	_ "github.com/arjenschwarz/orbit/internal/agents/copilot"    // Register copilot agent
	_ "github.com/arjenschwarz/orbit/internal/agents/kiro"       // Register kiro agent
	_ "github.com/arjenschwarz/orbit/internal/agents/opencode"   // Register opencode agent
	"github.com/arjenschwarz/orbit/internal/config"
	"github.com/arjenschwarz/orbit/internal/orbit"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// runCommand executes the orbit run subcommand.
// It orchestrates Claude Code sessions to implement spec phases sequentially.
func runCommand(args []string) error {
	// Stage 1: Check for deprecated --post-command flag before flag parsing.
	// This allows us to provide a clear error message instead of "unknown flag".
	for _, arg := range args {
		if arg == "--post-command" || strings.HasPrefix(arg, "--post-command=") {
			return fmt.Errorf("flag --post-command is deprecated\n\n"+
				"  Rename to: --post-prompt\n\n"+
				"Update your command and retry")
		}
	}

	fs := flag.NewFlagSet("run", flag.ExitOnError)

	tasksFile := fs.String("tasks-file", "", "Path to rune tasks file (auto-detects from branch if not specified)")
	logDir := fs.String("log-dir", "", "Base directory for session logs (default: .orbit next to tasks file)")
	skipPermissions := fs.Bool("skip-permissions", true, "Run Claude with --dangerously-skip-permissions")
	verbose := fs.Bool("verbose", false, "Enable verbose output")
	debug := fs.Bool("debug", false, "Enable debug logging (detailed CLI execution info)")
	noCentralizedLog := fs.Bool("no-centralized-log", false, "Disable centralized debug logging (use ORBIT_CENTRALIZED_LOG=true to re-enable)")
	dryRun := fs.Bool("dry-run", false, "Show what would be executed without running")
	showVersion := fs.Bool("version", false, "Show version and exit")
	commandFlag := fs.String("command", "", "Custom prompt for Claude phases")
	prePromptFlag := fs.String("pre-prompt", "", "AI prompt before phases start")
	noPrePrompt := fs.Bool("no-pre-prompt", false, "Disable pre-prompt")
	postPromptFlag := fs.String("post-prompt", "", "AI prompt after all tasks complete")
	noPostPrompt := fs.Bool("no-post-prompt", false, "Skip post-completion AI prompt")
	dateSubdirs := fs.Bool("date-subdirs", false, "Use timestamped subdirectories for logs")
	noContinueSession := fs.Bool("no-continue-session", false, "Start fresh sessions instead of resuming")

	// Agent selection
	agentFlag := fs.String("agent", "", "Agent to use (claude-code, codex, kiro, copilot, opencode)")

	// Variant flags for multi-spec comparison
	fs.Int("variants", 0, "Number of implementation variants to run (0 = single-run mode)")
	parallel := fs.Bool("parallel", false, "Run variants in parallel")
	maxParallel := fs.Int("max-parallel", config.DefaultMaxParallel, "Maximum parallel variants")
	fs.String("branch-prefix", "orbit-impl", "Branch naming prefix for variants")
	fs.String("guidance-file", "", "YAML file with per-variant guidance")
	fs.String("compare-command", "", "Custom comparison command")
	variantAgentsFlag := fs.String("variant-agents", "", "Comma-separated agent list for variants (cycles if fewer agents than variants)")

	// Auto-consolidation flags
	autoConsolidate := fs.Bool("auto-consolidate", false, "Run consolidation on recommended variant after comparison")
	noAutoConsolidate := fs.Bool("no-auto-consolidate", false, "Disable auto-consolidation when enabled via config")
	allowDirty := fs.Bool("allow-dirty", false, "Allow consolidation even if worktree has uncommitted changes")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: orbit run [options]\n\n")
		fmt.Fprintf(os.Stderr, "Orchestrate Claude Code sessions to implement spec phases sequentially.\n")
		fmt.Fprintf(os.Stderr, "Handles session lifecycle, error recovery, and log management.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  orbit run                                  # Auto-detect tasks from current branch\n")
		fmt.Fprintf(os.Stderr, "  orbit run --tasks-file specs/my-feature/tasks.md\n")
		fmt.Fprintf(os.Stderr, "  orbit run --verbose --log-dir ./logs\n")
		fmt.Fprintf(os.Stderr, "  orbit run --dry-run                        # Preview without executing\n")
		fmt.Fprintf(os.Stderr, "  orbit run --variants 3                     # Run 3 implementation variants\n")
		fmt.Fprintf(os.Stderr, "  orbit run --variants 2 --parallel          # Run 2 variants in parallel\n")
		fmt.Fprintf(os.Stderr, "  orbit run --variants 3 --guidance-file guidance.yaml\n")
		fmt.Fprintf(os.Stderr, "  orbit run --variants 2 --variant-agents claude-code,codex  # Compare agents\n")
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *showVersion {
		fmt.Printf("orbit version %s\n", version)
		return nil
	}

	// Get working directory
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Stage 2: Check for deprecated configuration (env vars and config files).
	// This must happen before loading configuration to fail fast.
	if err := config.CheckDeprecation(workingDir); err != nil {
		return err
	}

	// Get branch name
	branchName, err := getGitBranch()
	if err != nil {
		return fmt.Errorf("failed to get git branch: %w", err)
	}

	// Auto-detect tasks file if not specified
	if *tasksFile == "" {
		detected, err := detectTasksFile(branchName)
		if err != nil {
			return fmt.Errorf("failed to auto-detect tasks file: %w\nUse --tasks-file to specify manually", err)
		}
		*tasksFile = detected
		if *verbose {
			log.Printf("Auto-detected tasks file: %s", *tasksFile)
		}
	}

	// Validate tasks file exists
	if _, err := os.Stat(*tasksFile); os.IsNotExist(err) {
		return fmt.Errorf("tasks file not found: %s", *tasksFile)
	}

	// Set default log directory to .orbit next to tasks file
	actualLogDir := *logDir
	if actualLogDir == "" {
		actualLogDir = filepath.Join(filepath.Dir(*tasksFile), ".orbit")
	}

	// Load configuration (Viper handles merging of home/project configs and env vars)
	cfg := config.Load(workingDir)

	// Require config file for agent resolution
	if err := cfg.RequireConfigFile(); err != nil {
		return err
	}

	// Resolve and validate all agent aliases
	if err := cfg.ResolveAliases(); err != nil {
		return err
	}

	// Resolve agent alias: CLI flag > config file > default (claude-code)
	aliasName := resolveAgent(*agentFlag, cfg)
	resolved, err := cfg.GetResolvedAgent(aliasName)
	if err != nil {
		return err
	}

	// Get agent factory by type
	agentCfg := buildAgentConfig(resolved)
	agent, err := agents.Get(resolved.Type, agentCfg)
	if err != nil {
		return fmt.Errorf("unknown agent type %q for alias %q\n\nValid agent types: %v", resolved.Type, aliasName, agents.List())
	}
	if !agent.IsInstalled() {
		return fmt.Errorf("agent %q CLI (%s) not found\nInstall it from: %s", aliasName, agent.CLICommand(), getAgentInstallURL(resolved.Type))
	}

	// Apply CLI flag overrides
	command, prePrompt, postPrompt := resolvePrompts(cfg, *commandFlag, *prePromptFlag, *noPrePrompt, *postPromptFlag, *noPostPrompt)

	// Extract agent-level shell commands from the resolved agent config
	agentPreCommand := resolved.Config.PreCommand
	agentPostCommand := resolved.Config.PostCommand

	// Resolve date-subdirs: CLI flag can enable (overrides config)
	dateSubdirsValue := cfg.DateSubdirs
	if *dateSubdirs {
		dateSubdirsValue = true
	}

	// Resolve continue-session: CLI flag can disable (overrides config)
	continueSessionValue := cfg.ContinueSession
	if *noContinueSession {
		continueSessionValue = false
	}

	// Resolve debug: CLI flag can enable (overrides config)
	debugValue := cfg.Debug
	if *debug {
		debugValue = true
	}

	// Resolve centralized-log: CLI flag can disable (overrides config)
	// --no-centralized-log explicitly disables, similar to --no-continue-session
	centralizedLogValue := cfg.CentralizedLog
	if *noCentralizedLog {
		centralizedLogValue = false
	}

	// Resolve parallel: CLI flag can enable (overrides config)
	parallelValue := cfg.Parallel
	if *parallel {
		parallelValue = true
	}

	// Resolve and validate max-parallel. Validation operates on the resolved
	// value (not the raw CLI flag) so a negative `.orbit.yaml` value cannot
	// bypass the check just because the flag holds its default.
	maxParallelExplicit := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "max-parallel" {
			maxParallelExplicit = true
		}
	})
	maxParallelValue, err := resolveMaxParallel(cfg.MaxParallel, *maxParallel, maxParallelExplicit)
	if err != nil {
		return err
	}

	// Resolve auto-consolidate: --auto-consolidate enables, --no-auto-consolidate disables
	autoConsolidateValue := cfg.AutoConsolidate
	if *autoConsolidate {
		autoConsolidateValue = true
	}
	if *noAutoConsolidate {
		autoConsolidateValue = false
	}

	vf := resolveVariantFlags(fs, cfg)
	variantCountValue := vf.VariantCount
	branchPrefixValue := vf.BranchPrefix
	compareCommandValue := vf.CompareCommand
	guidanceFileValue := vf.GuidanceFile

	// Parse guidance file if provided
	var guidance []string
	if guidanceFileValue != "" {
		var err error
		guidance, err = parseGuidanceFile(guidanceFileValue, variantCountValue)
		if err != nil {
			return fmt.Errorf("failed to parse guidance file: %w", err)
		}
	}

	// Parse variant agents if provided
	var variantAgents []string
	if *variantAgentsFlag != "" {
		variantAgents = strings.Split(*variantAgentsFlag, ",")
		// Trim whitespace from each agent name
		for i := range variantAgents {
			variantAgents[i] = strings.TrimSpace(variantAgents[i])
		}
	}

	// Validate variant configuration. Max-parallel is validated inside
	// resolveMaxParallel so config-sourced values are also checked.
	if variantCountValue < 0 {
		return fmt.Errorf("--variants must be non-negative")
	}

	// Validate auto-consolidate requires variants
	if autoConsolidateValue && variantCountValue == 0 {
		return fmt.Errorf("--auto-consolidate requires --variants to be specified")
	}

	// Derive SpecDir and RepoRoot for variant mode
	var specDir, repoRoot string
	if variantCountValue > 0 {
		// SpecDir is the directory containing the tasks file
		specDir = filepath.Dir(*tasksFile)

		// RepoRoot is the git repository root
		cmd := exec.Command("git", "rev-parse", "--show-toplevel")
		output, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("failed to get repository root: %w", err)
		}
		repoRoot = strings.TrimSpace(string(output))
	}

	// Generate unique run ID for this orchestration
	runID := uuid.NewString()

	// Create and run orchestrator
	orbitCfg := orbit.Config{
		TasksFile:        *tasksFile,
		LogDir:           actualLogDir,
		BranchName:       branchName,
		SkipPermissions:  *skipPermissions,
		Verbose:          *verbose,
		Debug:            debugValue,
		CentralizedLog:   centralizedLogValue,
		RunID:            runID,
		Version:          version,
		DryRun:           *dryRun,
		WorkingDir:       workingDir,
		Command:          command,
		PrePrompt:        prePrompt,
		PostPrompt:       postPrompt,
		AgentPreCommand:  agentPreCommand,
		AgentPostCommand: agentPostCommand,
		CommandTimeout:   cfg.CommandTimeout,
		DateSubdirs:      dateSubdirsValue,
		ContinueSession:  continueSessionValue,
		Agent:            aliasName,
		AgentConfig:      agentCfg,
		AgentConfigs:     cfg.GetAllAgentConfigs(),
		VariantCount:     variantCountValue,
		Parallel:         parallelValue,
		MaxParallel:      maxParallelValue,
		BranchPrefix:     branchPrefixValue,
		Guidance:         guidance,
		CompareCommand:   compareCommandValue,
		GlobalGuidance:   cfg.GlobalGuidance,
		SpecDir:  specDir,
		RepoRoot: repoRoot,
		VariantAgents:          variantAgents,
		AutoConsolidate:        autoConsolidateValue,
		AllowDirty:             *allowDirty,
		PostConsolidateCommand: cfg.PostConsolidateCommand,
	}

	o, err := orbit.New(orbitCfg)
	if err != nil {
		return fmt.Errorf("failed to initialize Orbit: %w", err)
	}
	defer o.Close()

	if err := o.Run(); err != nil {
		return err
	}

	log.Println("Orbit completed successfully")
	return nil
}

// getGitBranch returns the current git branch name.
func getGitBranch() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not in a git repository or git not available: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// resolveMaxParallel applies CLI/config precedence to --max-parallel and
// validates the resolved value. The flag must be detected as explicitly set
// via fs.Visit so an explicit --max-parallel=3 still wins over a non-default
// config value.
//
// variantFlagValues holds the resolved values for variant-related flags.
type variantFlagValues struct {
	VariantCount   int
	BranchPrefix   string
	CompareCommand string
	GuidanceFile   string
}

// resolveVariantFlags resolves variant-related flags using explicit-set detection.
// fs.Visit only visits flags the user actually passed, letting us distinguish
// "user passed the default value" from "flag was never set".
func resolveVariantFlags(fs *flag.FlagSet, cfg *config.Config) variantFlagValues {
	variantCountExplicit, branchPrefixExplicit, compareCommandExplicit, guidanceFileExplicit := false, false, false, false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "variants":
			variantCountExplicit = true
		case "branch-prefix":
			branchPrefixExplicit = true
		case "compare-command":
			compareCommandExplicit = true
		case "guidance-file":
			guidanceFileExplicit = true
		}
	})

	v := variantFlagValues{
		VariantCount:   cfg.VariantCount,
		BranchPrefix:   cfg.BranchPrefix,
		CompareCommand: cfg.CompareCommand,
		GuidanceFile:   cfg.GuidanceFile,
	}
	if variantCountExplicit {
		val, _ := fs.Lookup("variants").Value.(flag.Getter)
		v.VariantCount = val.Get().(int)
	}
	if branchPrefixExplicit {
		v.BranchPrefix = fs.Lookup("branch-prefix").Value.String()
	}
	if compareCommandExplicit {
		v.CompareCommand = fs.Lookup("compare-command").Value.String()
	}
	if guidanceFileExplicit {
		v.GuidanceFile = fs.Lookup("guidance-file").Value.String()
	}
	return v
}

// Precedence: explicit flag > config > flag default. A config value of 0 is
// treated as "unset" and falls through to the flag's default (which flagValue
// already holds when flagExplicit is false). An explicit --max-parallel=0 is
// left at 0 so validation rejects it. Validation runs on the *resolved* value
// so a negative `.orbit.yaml` value cannot bypass the check just because the
// flag held its built-in default.
func resolveMaxParallel(configValue, flagValue int, flagExplicit bool) (int, error) {
	var resolved int
	switch {
	case flagExplicit:
		resolved = flagValue
	case configValue == 0:
		resolved = flagValue // flag holds its default when not explicitly set
	default:
		resolved = configValue
	}
	if resolved < 1 {
		return 0, fmt.Errorf("--max-parallel must be at least 1 (got %d)", resolved)
	}
	return resolved, nil
}

// resolvePrompts applies CLI flag overrides to config values.
// Priority: CLI flags > config (which includes env vars > project > home > defaults).
func resolvePrompts(cfg *config.Config, commandFlag, prePromptFlag string, noPrePrompt bool, postPromptFlag string, noPostPrompt bool) (command, prePrompt, postPrompt string) {
	// Resolve effective command (priority: flag > config/env > default)
	command = cfg.Command // Already has default from Viper
	if commandFlag != "" {
		command = commandFlag
	}

	// Resolve effective pre-prompt (priority: flag > config/env > default)
	// Pre-prompt has no default, so empty means disabled unless explicitly set
	prePrompt = cfg.PrePrompt
	if cfg.IsPrePromptDisabled() {
		prePrompt = "" // Config explicitly disabled
	}
	if prePromptFlag != "" {
		prePrompt = prePromptFlag
	}
	if noPrePrompt {
		prePrompt = "" // Flag disables
	}

	// Resolve effective post-prompt (priority: flag > config/env > default)
	postPrompt = cfg.PostPrompt
	if cfg.IsPostPromptDisabled() {
		postPrompt = "" // Config explicitly disabled
	}
	if postPromptFlag != "" {
		postPrompt = postPromptFlag
	}
	if noPostPrompt {
		postPrompt = "" // Flag disables
	}

	return command, prePrompt, postPrompt
}

// resolveAgent determines which agent to use based on priority:
// CLI flag > config file > default (claude-code)
func resolveAgent(flagValue string, cfg *config.Config) string {
	if flagValue != "" {
		return flagValue
	}
	if cfg.Agent != "" {
		return cfg.Agent
	}
	return "claude-code"
}

// buildAgentConfig creates agents.AgentConfig from a resolved alias.
// It converts the alias configuration including model (stored in Options).
func buildAgentConfig(resolved config.ResolvedAgent) agents.AgentConfig {
	return config.GetResolvedAgentConfig(resolved)
}

// getAgentInstallURL returns the installation URL for a given agent.
func getAgentInstallURL(agentName string) string {
	urls := map[string]string{
		"claude-code": "https://docs.anthropic.com/en/docs/agents-and-tools/claude-code/overview",
		"codex":       "https://github.com/openai/codex",
		"kiro":        "https://kiro.dev/docs/cli",
		"copilot":     "https://docs.github.com/en/copilot/using-github-copilot/using-github-copilot-in-the-command-line",
	}
	if url, ok := urls[agentName]; ok {
		return url
	}
	return "unknown agent - check documentation"
}

// detectTasksFile attempts to find a tasks file based on the branch name.
func detectTasksFile(branchName string) (string, error) {
	// Strip prefix before first slash (e.g., "feature/my-feature" -> "my-feature")
	name := branchName
	if _, after, found := strings.Cut(branchName, "/"); found {
		name = after
	}

	// Try various paths
	candidates := []string{
		filepath.Join("specs", name, "tasks.md"),
		filepath.Join("specs", name, "TASKS.md"),
		filepath.Join("specs", branchName, "tasks.md"),
		filepath.Join("specs", branchName, "TASKS.md"),
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("could not find tasks file for branch '%s'\nTried: %s", branchName, strings.Join(candidates, ", "))
}

// guidanceFileSchema represents the YAML schema for per-variant guidance.
type guidanceFileSchema struct {
	Variants []struct {
		ID       int    `yaml:"id"`
		Guidance string `yaml:"guidance"`
	} `yaml:"variants"`
	GlobalGuidance string `yaml:"global_guidance"`
}

// parseGuidanceFile parses a YAML guidance file and returns per-variant guidance strings.
// The returned slice is indexed by variant ID-1 (0-indexed).
func parseGuidanceFile(filePath string, variantCount int) ([]string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read guidance file: %w", err)
	}

	var schema guidanceFileSchema
	if err := yaml.Unmarshal(data, &schema); err != nil {
		return nil, fmt.Errorf("parse guidance YAML: %w", err)
	}

	// Validate variant count matches if variants are specified
	if len(schema.Variants) > 0 && variantCount > 0 && len(schema.Variants) != variantCount {
		return nil, fmt.Errorf("guidance file has %d variants but --variants=%d", len(schema.Variants), variantCount)
	}

	// Build guidance slice indexed by variant ID
	// First pass: collect variant-specific guidance
	guidance := make([]string, variantCount)
	for _, v := range schema.Variants {
		if v.ID < 1 || v.ID > variantCount {
			return nil, fmt.Errorf("guidance file variant ID %d is out of range (1-%d)", v.ID, variantCount)
		}
		guidance[v.ID-1] = v.Guidance
	}

	// Second pass: apply global guidance to all variants
	if schema.GlobalGuidance != "" {
		for i := range guidance {
			if guidance[i] != "" {
				// Variant has specific guidance, append global
				guidance[i] = guidance[i] + "\n\n" + schema.GlobalGuidance
			} else {
				// No variant-specific guidance, use only global
				guidance[i] = schema.GlobalGuidance
			}
		}
	}

	return guidance, nil
}
