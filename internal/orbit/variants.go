package orbit

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
	"github.com/arjenschwarz/orbit/internal/cost"
	"github.com/arjenschwarz/orbit/internal/debug"
	"github.com/arjenschwarz/orbit/internal/logs"
	"github.com/arjenschwarz/orbit/internal/registry"
	"github.com/arjenschwarz/orbit/internal/rune"
	"github.com/arjenschwarz/orbit/internal/variants"
	"github.com/google/uuid"
)

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

// executeVariantShellCommand runs a shell command in a variant's worktree.
// It delegates to runShellCore with variant-specific parameters: the variant's
// worktree as working directory, the variant's agent name, and ORBIT_VARIANT env var.
func (o *Orbit) executeVariantShellCommand(
	ctx context.Context,
	v *variants.Variant,
	command, logName string,
	logManager *logs.Manager,
) (*ShellCommandResult, error) {
	// Build the variant tasks file path to get phase count
	tasksFile := o.config.TasksFile
	if o.config.RepoRoot != "" {
		absTasksFile := tasksFile
		if !filepath.IsAbs(tasksFile) {
			absTasksFile = filepath.Join(o.config.RepoRoot, tasksFile)
		}
		relPath, err := filepath.Rel(o.config.RepoRoot, absTasksFile)
		if err == nil {
			tasksFile = filepath.Join(v.WorktreePath, relPath)
		}
	}
	phaseCount := 0
	if summaries, err := rune.NewClient(tasksFile).GetPhaseSummaries(); err == nil {
		phaseCount = len(summaries)
	}

	return o.runShellCore(command, logName, shellExecParams{
		ctx:        ctx,
		workDir:    v.WorktreePath,
		phaseCount: phaseCount,
		agentName:  v.Agent,
		variantID:  v.ID,
		logManager: logManager,
	})
}

// runVariantPrePrompt executes pre-prompt for a variant and returns the session ID.
// The session started by pre-prompt should be continued by phase 1.
func (o *Orbit) runVariantPrePrompt(
	ctx context.Context,
	v *variants.Variant,
	agent agents.Agent,
	logManager *logs.Manager,
	timeout time.Duration,
) (string, error) {
	log.Printf("Variant %d: running pre-prompt...", v.ID)

	// Check for existing pre-prompt state (crash recovery)
	if logManager != nil {
		sessionID, status := logManager.GetPrePromptState()
		if status == logs.PrePromptStatusCompleted {
			log.Printf("Variant %d: pre-prompt already completed, using session: %s", v.ID, sessionID)
			return sessionID, nil
		}
		if status == logs.PrePromptStatusStarted {
			o.debug.Log("Variant %d: pre-prompt was interrupted, will resume session: %s", v.ID, sessionID)
		}
	}

	// Start pre-prompt tracking
	var sessionID string
	var isResume bool
	if logManager != nil {
		var err error
		sessionID, isResume, err = logManager.StartPrePrompt(o.config.ContinueSession)
		if err != nil {
			return "", fmt.Errorf("start pre-prompt: %w", err)
		}
		o.debug.Log("Variant %d: pre-prompt session %s (resume=%v)", v.ID, sessionID, isResume)
	} else {
		sessionID = uuid.NewString()
		isResume = false
	}

	// Execute pre-prompt in variant's worktree
	opts := agents.RunOptions{
		Prompt:    o.config.PrePrompt,
		WorkDir:   v.WorktreePath, // Variant worktree
		SessionID: sessionID,
		Timeout:   timeout,
	}

	var result *agents.RunResult
	var err error
	if isResume {
		result, err = agent.Resume(ctx, sessionID, opts)
	} else {
		result, err = agent.Run(ctx, opts)
	}

	// Handle resume failure - start fresh session
	if err != nil && isResume && isSessionInvalidError(result) {
		o.debug.Log("Variant %d: pre-prompt session resume failed, starting fresh session", v.ID)
		log.Printf("Variant %d: pre-prompt session resume failed, starting fresh session", v.ID)
		sessionID = uuid.NewString()
		opts.SessionID = sessionID
		result, err = agent.Run(ctx, opts)
	}

	if err != nil {
		return "", err
	}

	// Check if agent reported an error in its output
	if result != nil && result.IsError {
		return "", fmt.Errorf("agent reported error")
	}

	// Mark complete
	if logManager != nil {
		finalSessionID := sessionID
		if result != nil && result.SessionID != "" {
			finalSessionID = result.SessionID
		}
		if err := logManager.CompletePrePrompt(finalSessionID); err != nil {
			log.Printf("Warning: variant %d failed to complete pre-prompt: %v", v.ID, err)
		}
	}

	// Return the session ID for phase 1 continuation
	if result != nil && result.SessionID != "" {
		sessionID = result.SessionID
	}

	log.Printf("Variant %d: pre-prompt completed", v.ID)
	return sessionID, nil
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

	// Store tasks file path in metadata so status can find it
	if tasksFileRel := o.tasksFileRel(); tasksFileRel != "" {
		if err := o.variantManager.SetTasksFile(tasksFileRel); err != nil {
			o.debug.Log("Warning: failed to store tasks file path in metadata: %v", err)
		}
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

	// Log variant creation to parent logger (Req 1.6, 3.10)
	for _, v := range variantList {
		o.debug.LogStructured("info", "Variant created", map[string]any{
			"variant_id":    v.ID,
			"branch":        v.Branch,
			"worktree_path": v.WorktreePath,
			"agent":         v.Agent,
		})
	}

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
		// Log parallel execution start to parent logger (Req 1.6)
		o.debug.LogStructured("info", "Parallel execution started", map[string]any{
			"variant_count": len(variantList),
			"max_parallel":  o.config.MaxParallel,
		})
		o.runVariantsParallel(ctx, variantList)
		log.Println("All variant goroutines completed")
	} else {
		// Log sequential execution start
		o.debug.LogStructured("info", "Sequential execution started", map[string]any{
			"variant_count": len(variantList),
		})
		o.runVariantsSequential(ctx, variantList)
	}

	// Count successes
	successCount := o.variantManager.CountByStatus(variants.StatusCompleted)
	failedCount := o.variantManager.CountByStatus(variants.StatusFailed)
	canceledCount := o.variantManager.CountByStatus(variants.StatusCanceled)
	pendingCount := o.variantManager.CountByStatus(variants.StatusPending)

	log.Printf("Variant execution complete: %d succeeded, %d failed, %d canceled, %d pending",
		successCount, failedCount, canceledCount, pendingCount)

	// Log all variants completed to parent logger (Req 1.6)
	o.debug.LogStructured("info", "All variants completed", map[string]any{
		"succeeded": successCount,
		"failed":    failedCount,
		"canceled":  canceledCount,
		"pending":   pendingCount,
	})

	// Generate report based on outcomes
	if successCount == 0 {
		log.Println("All variants failed; generating partial report")
		return o.generateReport(ctx, true)
	}

	if successCount == 1 {
		log.Println("Only one variant succeeded; skipping comparison")
		if o.config.AutoConsolidate {
			log.Println("Skipping auto-consolidation: comparison requires 2+ successful variants")
		}
		return o.generateReport(ctx, false)
	}

	// Compare multiple successful variants
	if err := o.runComparison(ctx); err != nil {
		log.Printf("Comparison failed: %v", err)
		// Still try to generate a report without comparison
	}

	// Generate report first - auto-consolidation reads the report file
	if err := o.generateReport(ctx, false); err != nil {
		log.Printf("Report generation failed: %v", err)
		// Return early if report generation fails - consolidation depends on the report
		return err
	}

	// Run auto-consolidation if enabled and comparison succeeded
	// Must run after generateReport() since consolidation reads comparison-report/report.md
	if o.config.AutoConsolidate && o.comparisonResult != nil {
		if err := o.runAutoConsolidate(ctx); err != nil {
			log.Printf("Auto-consolidation failed: %v", err)
			// Non-fatal - report already generated successfully
		}
	}

	return nil
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
			// Leave pending variants in their current state so they can be
			// picked up on a subsequent "continue" run.
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
				// Leave pending variants in their current state so they can be
				// picked up on a subsequent "continue" run.
				return
			default:
			}

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			// Check again after acquiring semaphore
			select {
			case <-ctx.Done():
				// Leave pending variants in their current state so they can be
				// picked up on a subsequent "continue" run.
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

	// Create variant-specific logger (Req 1.4, 1.5)
	variantLogger, err := debug.NewLogger(debug.LoggerConfig{
		StderrEnabled: o.config.Debug,
		FileEnabled:   o.config.CentralizedLog,
		RunID:         o.config.RunID,
		VariantNum:    v.ID,
		Prefix:        fmt.Sprintf("variant-%d", v.ID),
	})
	if err != nil {
		// Log to parent logger but continue - centralized logging is best-effort
		o.debug.Log("Warning: failed to create variant %d logger: %v", v.ID, err)
	}
	// Ensure we close the variant logger when done
	defer func() {
		if variantLogger != nil {
			variantLogger.Close()
		}
	}()

	// Log variant start to variant's own log
	if variantLogger != nil {
		variantLogger.LogStartup(debug.StartupConfig{
			OrbitVersion:     o.config.Version,
			Agent:            v.Agent,
			TasksFile:        o.config.TasksFile,
			WorkingDirectory: v.WorktreePath,
			BranchName:       v.Branch,
		})
	}

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

	variantAgent, err := o.config.AgentResolver.GetAgent(agentType, variantAgentConfig)
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
	// Initialize from existing metrics to support accumulation when continuing a run
	totalCost := v.Cost
	totalTurns := v.NumTurns
	previousDuration := v.Duration

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

	// === Step 1: Agent Pre-Command (shell) ===
	if variantAgentConfig.PreCommand != "" {
		if o.config.DryRun {
			log.Printf("[DRY RUN] Variant %d would execute pre-command: %s", v.ID, variantAgentConfig.PreCommand)
			log.Printf("[DRY RUN] Working directory: %s", v.WorktreePath)
		} else {
			result, err := o.executeVariantShellCommand(ctx, v, variantAgentConfig.PreCommand, "pre-command", variantLogManager)
			if err != nil {
				variantErr := fmt.Errorf("pre-command failed (exit code %d): %w", result.ExitCode, err)
				if updateErr := o.variantManager.UpdateStatus(v.ID, variants.StatusFailed, variantErr); updateErr != nil {
					log.Printf("Warning: failed to update variant %d status: %v", v.ID, updateErr)
				}
				o.updateVariantRegistryStatus(v.ID, registry.StatusFailed)
				if variantLogManager != nil {
					_ = variantLogManager.Fail(variantErr)
				}
				if variantLogger != nil {
					variantLogger.LogShutdown("failed")
				}
				return variantErr
			}
			log.Printf("Variant %d: pre-command finished", v.ID)
		}
	}

	// === Step 2: Pre-Prompt (AI) ===
	var prePromptSessionID string
	if o.config.PrePrompt != "" {
		if o.config.DryRun {
			log.Printf("[DRY RUN] Variant %d would execute pre-prompt", v.ID)
		} else {
			sessionID, err := o.runVariantPrePrompt(ctx, v, variantAgent, variantLogManager, variantAgentConfig.Timeout)
			if err != nil {
				variantErr := fmt.Errorf("pre-prompt failed: %w", err)
				if updateErr := o.variantManager.UpdateStatus(v.ID, variants.StatusFailed, variantErr); updateErr != nil {
					log.Printf("Warning: failed to update variant %d status: %v", v.ID, updateErr)
				}
				o.updateVariantRegistryStatus(v.ID, registry.StatusFailed)
				if variantLogManager != nil {
					_ = variantLogManager.Fail(variantErr)
				}
				if variantLogger != nil {
					variantLogger.LogShutdown("failed")
				}
				return variantErr
			}
			prePromptSessionID = sessionID
		}
	}

	// Track whether we've used the pre-prompt session for phase 1
	prePromptSessionUsed := false

	// === Step 3: Phase Loop ===
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
				log.Printf("Variant %d: finished phase %s (cost=%.2f)", v.ID, lastLoggedPhase, phaseCost)
				phaseCost = 0 // Reset for new phase
			}
			log.Printf("Variant %d: starting phase %s (%d tasks)", v.ID, currentPhase, totalTasks)
			lastLoggedPhase = currentPhase
		}

		phaseStartTime := time.Now()

		// Determine if this phase should continue the pre-prompt session
		var continueSessionID string
		if phaseNum == 1 && prePromptSessionID != "" && !prePromptSessionUsed {
			continueSessionID = prePromptSessionID
			prePromptSessionUsed = true
		}

		// Start phase in log manager to track session ID for status display
		// This populates CurrentPhase in summary.json so orbit status can show live activity
		if variantLogManager != nil {
			sessionID, isResumeFromManager, err := variantLogManager.StartPhase(phaseNum, o.config.ContinueSession, continueSessionID)
			if err != nil {
				o.debug.Log("Variant %d: failed to start phase in log manager: %v", v.ID, err)
			} else if continueSessionID == "" && isResumeFromManager {
				// Only set continueSessionID if we're resuming an existing session (continue interrupted run)
				continueSessionID = sessionID
			}
		}

		// Run the phase with retry
		phaseResult, err := o.runVariantPhaseWithRetry(ctx, v, variantAgent, variantPrompt, continueSessionID, variantAgentConfig.Timeout)
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
			// Log failure to variant's log
			if variantLogger != nil {
				variantLogger.LogShutdown("failed")
			}
			return variantErr
		}

		// Save successful session and complete phase in log manager
		if variantLogManager != nil && phaseResult != nil {
			if saveErr := variantLogManager.SaveSession(phaseNum, phaseResult, phaseStartTime); saveErr != nil {
				log.Printf("Warning: variant %d failed to save session log: %v", v.ID, saveErr)
			}
			// Clear CurrentPhase now that this phase is done
			if completeErr := variantLogManager.CompletePhase(); completeErr != nil {
				o.debug.Log("Variant %d: failed to complete phase in log manager: %v", v.ID, completeErr)
			}
		}

		// Accumulate metrics
		if phaseResult != nil {
			phaseCost += getCostValue(phaseResult)
			totalCost += getCostValue(phaseResult)
			totalTurns += phaseResult.NumTurns
		}
	}

	// Log final phase completion
	if lastLoggedPhase != "" && phaseCost > 0 {
		log.Printf("Variant %d: finished phase %s (cost=%.2f)", v.ID, lastLoggedPhase, phaseCost)
	}

	// === Step 4: Post-Prompt (AI) ===
	if o.config.PostPrompt != "" {
		log.Printf("Variant %d: running post-prompt...", v.ID)
		postStartTime := time.Now()
		postResult, err := o.runVariantPostCompletion(ctx, v, variantAgent, variantLogManager, variantAgentConfig.Timeout)
		if err != nil {
			// Save failed post-prompt session for debugging
			if variantLogManager != nil && postResult != nil {
				_ = variantLogManager.SavePostCompletionSession(postResult, postStartTime)
			}
			variantErr := fmt.Errorf("post-prompt: %w", err)
			if updateErr := o.variantManager.UpdateStatus(v.ID, variants.StatusFailed, variantErr); updateErr != nil {
				log.Printf("Warning: failed to update variant %d status: %v", v.ID, updateErr)
			}
			o.updateVariantRegistryStatus(v.ID, registry.StatusFailed)
			if variantLogManager != nil {
				_ = variantLogManager.Fail(variantErr)
			}
			// Log failure to variant's log
			if variantLogger != nil {
				variantLogger.LogShutdown("failed")
			}
			return variantErr
		}
		// Save successful post-prompt session
		if variantLogManager != nil && postResult != nil {
			if saveErr := variantLogManager.SavePostCompletionSession(postResult, postStartTime); saveErr != nil {
				log.Printf("Warning: variant %d failed to save post-prompt log: %v", v.ID, saveErr)
			}
		}
		if postResult != nil {
			totalCost += getCostValue(postResult)
			totalTurns += postResult.NumTurns
		}
		log.Printf("Variant %d: post-prompt finished", v.ID)
	}

	// === Step 5: Agent Post-Command (shell) ===
	if variantAgentConfig.PostCommand != "" {
		if o.config.DryRun {
			log.Printf("[DRY RUN] Variant %d would execute post-command: %s", v.ID, variantAgentConfig.PostCommand)
			log.Printf("[DRY RUN] Working directory: %s", v.WorktreePath)
		} else {
			result, err := o.executeVariantShellCommand(ctx, v, variantAgentConfig.PostCommand, "post-command", variantLogManager)
			if err != nil {
				// Post-command failure is warning, not fatal
				log.Printf("Warning: variant %d post-command failed (exit code %d): %v", v.ID, result.ExitCode, err)
			} else {
				log.Printf("Variant %d: post-command finished", v.ID)
			}
		}
	}

	// Mark variant as completed
	// Add previous duration to support accumulation when continuing a run
	duration := previousDuration + time.Since(startTime)
	if err := o.variantManager.UpdateStatus(v.ID, variants.StatusCompleted, nil); err != nil {
		log.Printf("Warning: failed to update variant %d status: %v", v.ID, err)
	}

	// Infer cost unit and build totals from agent type
	costUnit := cost.InferUnitFromAgent(agentType)
	costTotals := cost.TotalsFromValue(cost.Totals{}, totalCost, costUnit)

	if err := o.variantManager.UpdateMetrics(v.ID, totalCost, costUnit, costTotals, duration, totalTurns); err != nil {
		log.Printf("Warning: failed to update variant %d metrics: %v", v.ID, err)
	}
	o.updateVariantRegistryStatus(v.ID, registry.StatusCompleted)

	// Mark variant log as complete
	if variantLogManager != nil {
		if completeErr := variantLogManager.Complete(); completeErr != nil {
			log.Printf("Warning: variant %d failed to complete log: %v", v.ID, completeErr)
		}
	}

	log.Printf("Variant %d completed: cost=%.2f, duration=%s, turns=%d",
		v.ID, totalCost, duration.Round(time.Second), totalTurns)

	// Log shutdown to variant's own log
	if variantLogger != nil {
		variantLogger.LogShutdown("completed")
	}

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
// If continueSessionID is non-empty, the first attempt will resume that session (for pre-prompt continuation).
func (o *Orbit) runVariantPhaseWithRetry(ctx context.Context, v *variants.Variant, agent agents.Agent, prompt string, continueSessionID string, timeout time.Duration) (*agents.RunResult, error) {
	return agents.RunWithRetry(ctx, agents.RetryConfig{
		MaxRetries: maxRetries,
		Sleep:      o.sleepFunc(),
		Execute: func(ctx context.Context, attempt int) (*agents.RunResult, error) {
			// Determine session ID and whether to resume
			var sessionID string
			var isResume bool
			if continueSessionID != "" && attempt == 0 {
				sessionID = continueSessionID
				isResume = true
				o.debug.Log("Variant %d: phase 1 continuing pre-prompt session %s", v.ID, sessionID)
			} else {
				sessionID = uuid.NewString()
				isResume = false
			}

			opts := agents.RunOptions{
				Prompt:    prompt,
				SessionID: sessionID,
				WorkDir:   v.WorktreePath,
				Timeout:   timeout,
			}

			var result *agents.RunResult
			var err error
			if isResume {
				result, err = agent.Resume(ctx, sessionID, opts)
				if err != nil && isSessionInvalidError(result) {
					o.debug.Log("Variant %d: session resume failed, starting fresh session", v.ID)
					log.Printf("Variant %d: session resume failed, starting fresh session", v.ID)
					opts.SessionID = uuid.NewString()
					result, err = agent.Run(ctx, opts)
				}
			} else {
				result, err = agent.Run(ctx, opts)
			}
			return result, err
		},
		Classify: classifyFromAgent(agent.Name()),
		OnRetry: func(attempt, maxRetries int, classified *agents.ClassifiedError, backoff time.Duration) {
			if classified.Class.IsRateLimitWait() {
				log.Printf("Variant %d: usage limit reached, waiting %s until reset...", v.ID, backoff)
			} else if classified.RetryAfter > 0 {
				log.Printf("Variant %d: retryable error, waiting %s (attempt %d/%d)",
					v.ID, backoff, attempt, maxRetries)
			} else {
				log.Printf("Variant %d: error, waiting %s (attempt %d/%d)",
					v.ID, backoff, attempt, maxRetries)
			}
		},
	})
}

// runVariantPostCompletion executes the post-completion command for a variant.
//
// Coordinates the variant log manager so post-prompt mirrors the single-run
// lifecycle (StartPostCompletion → Resume/Run → ReconcilePostCompletionSessionID
// → CompletePostCompletion). Without this coordination, variant post-prompt
// loses phase context across resumed runs and silently bypasses the
// post-completion lifecycle (T-715).
func (o *Orbit) runVariantPostCompletion(
	ctx context.Context,
	v *variants.Variant,
	agent agents.Agent,
	logManager *logs.Manager,
	timeout time.Duration,
) (*agents.RunResult, error) {
	return agents.RunWithRetry(ctx, agents.RetryConfig{
		MaxRetries: maxRetries,
		Sleep:      o.sleepFunc(),
		Execute: func(ctx context.Context, _ int) (*agents.RunResult, error) {
			var sessionID string
			var isResume bool
			if logManager != nil {
				var err error
				sessionID, isResume, err = logManager.StartPostCompletion(o.config.ContinueSession)
				if err != nil {
					o.debug.Log("Variant %d: failed to start post-completion in log manager: %v", v.ID, err)
					sessionID = uuid.NewString()
					isResume = false
				} else {
					o.debug.Log("Variant %d: post-completion session %s (resume=%v)", v.ID, sessionID, isResume)
				}
			} else {
				sessionID = uuid.NewString()
				isResume = false
			}

			opts := agents.RunOptions{
				Prompt:    o.config.PostPrompt,
				SessionID: sessionID,
				WorkDir:   v.WorktreePath,
				Timeout:   timeout,
			}

			var result *agents.RunResult
			var err error
			if isResume {
				result, err = agent.Resume(ctx, sessionID, opts)
				// Fall back to a fresh session when the agent reports the
				// resumed session is gone — otherwise the retry loop would
				// keep hitting the same dead session.
				if err != nil && isSessionInvalidError(result) {
					o.debug.Log("Variant %d: post-completion resume failed, starting fresh session", v.ID)
					log.Printf("Variant %d: post-completion session resume failed, starting fresh session", v.ID)
					sessionID = uuid.NewString()
					if logManager != nil {
						if setErr := logManager.SetPostCompletionSessionID(sessionID); setErr != nil {
							o.debug.Log("Variant %d: failed to update post-completion session id: %v", v.ID, setErr)
						}
					}
					opts.SessionID = sessionID
					result, err = agent.Run(ctx, opts)
				}
			} else {
				result, err = agent.Run(ctx, opts)
			}

			// Reconcile the agent-returned session id even when the call
			// failed: a retry needs StartPostCompletion to surface the most
			// recent id, otherwise we resume into an already-dead session.
			// Guard against empty SessionID so we don't overwrite the stored
			// id with "" when the agent omits it.
			if logManager != nil && result != nil && result.SessionID != "" && result.SessionID != sessionID {
				o.debug.Log("Variant %d: post-completion session id changed: expected=%s got=%s",
					v.ID, sessionID, result.SessionID)
				logManager.ReconcilePostCompletionSessionID(result.SessionID)
			}

			// Only clear in-progress state on a clean success — a retryable
			// failure must keep the entry so the next attempt can resume it.
			if err == nil && result != nil && !result.IsError && logManager != nil {
				if completeErr := logManager.CompletePostCompletion(); completeErr != nil {
					o.debug.Log("Variant %d: failed to complete post-completion in log manager: %v", v.ID, completeErr)
				}
			}

			return result, err
		},
		Classify: classifyFromAgent(agent.Name()),
		OnRetry: func(attempt, maxRetries int, classified *agents.ClassifiedError, backoff time.Duration) {
			if classified.Class.IsRateLimitWait() {
				log.Printf("Variant %d post-completion: usage limit reached, waiting %s until reset...", v.ID, backoff)
			} else if classified.RetryAfter > 0 {
				log.Printf("Variant %d post-completion: retryable error, waiting %s (attempt %d/%d)",
					v.ID, backoff, attempt, maxRetries)
			} else {
				log.Printf("Variant %d post-completion: error, waiting %s (attempt %d/%d)",
					v.ID, backoff, attempt, maxRetries)
			}
		},
	})
}

// tasksFileRel returns the tasks file path relative to the repo root.
// Returns empty string if the relative path cannot be computed.
func (o *Orbit) tasksFileRel() string {
	if o.config.RepoRoot == "" {
		return o.config.TasksFile
	}
	absTasksFile := o.config.TasksFile
	if !filepath.IsAbs(absTasksFile) {
		absTasksFile = filepath.Join(o.config.RepoRoot, absTasksFile)
	}
	relPath, err := filepath.Rel(o.config.RepoRoot, absTasksFile)
	if err != nil {
		return ""
	}
	return relPath
}

