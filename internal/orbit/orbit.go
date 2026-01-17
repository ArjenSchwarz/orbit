// Package orbit provides the main orchestration loop for running Claude Code sessions.
package orbit

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	output "github.com/ArjenSchwarz/go-output/v2"
	"github.com/arjenschwarz/orbit/internal/claude"
	"github.com/arjenschwarz/orbit/internal/debug"
	"github.com/arjenschwarz/orbit/internal/display"
	orberrors "github.com/arjenschwarz/orbit/internal/errors"
	"github.com/arjenschwarz/orbit/internal/logs"
	"github.com/arjenschwarz/orbit/internal/registry"
	"github.com/arjenschwarz/orbit/internal/rune"
	"github.com/google/uuid"
)

const (
	maxRetries = 5
)

// claudeRunner is an interface for running Claude sessions.
// This allows for mocking in tests.
type claudeRunner interface {
	RunPhase(sessionID string, resume bool) (*claude.SessionResult, error)
	RunCustomPrompt(prompt string) (*claude.SessionResult, error)
	RunCustomPromptWithSession(prompt, sessionID string, resume bool) (*claude.SessionResult, error)
}

// Config holds the orchestrator configuration.
type Config struct {
	TasksFile       string
	LogDir          string
	BranchName      string
	SkipPermissions bool
	Verbose         bool
	DryRun          bool
	Debug           bool   // Enable debug logging for troubleshooting
	WorkingDir      string
	Command         string // Custom phase command
	PostCommand     string // Post-completion command (empty = disabled)
	DateSubdirs     bool   // If true, use timestamped subdirectories for logs
	ContinueSession bool   // If true, continue existing Claude sessions when resuming
}

// Orbit orchestrates Claude Code sessions to implement spec phases.
type Orbit struct {
	config               Config
	runeClient           *rune.Client
	claudeClient         claudeRunner
	logManager           *logs.Manager
	phaseSummaries       []rune.PhaseSummary
	spinner              *display.Spinner
	shutdownCtx          context.Context
	shutdownCancel       context.CancelFunc
	registry             *registry.Registry // Web interface run registry
	runID                string             // UUID of the current run in registry
	currentPhaseRunCount int                // Track retry count for current phase
	debug                *debug.Logger      // Debug logger
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
		if config.Verbose {
			log.Printf("Logs will be saved to: %s", logManager.SessionDir())
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

	return &Orbit{
		config:         config,
		runeClient:     runeClient,
		claudeClient:   claudeClient,
		logManager:     logManager,
		spinner:        spin,
		shutdownCtx:    ctx,
		shutdownCancel: cancel,
		registry:       reg,
		debug:          dbg,
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

	for attempt := range maxRetries {
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

		// Classify the error
		classified, ok := err.(*orberrors.ClassifiedError)
		if !ok {
			o.debug.Log("Error is not a ClassifiedError, not retrying: %T", err)
			// Unknown error type, don't retry
			return err
		}

		o.debug.LogError(classified.Type.String(), classified.Message, classified.Type.IsRetryable())
		lastErr = err

		if !classified.Type.IsRetryable() {
			o.debug.Log("Error type %s is not retryable, stopping", classified.Type)
			// Non-retryable error
			return err
		}

		// Pause spinner before logging to prevent visual artifacts
		if o.spinner != nil {
			o.spinner.Pause()
		}

		// Determine wait time
		var waitTime time.Duration
		switch classified.Type {
		case orberrors.ErrRateLimit:
			waitTime = classified.RetryAfter
			if waitTime == 0 {
				waitTime = 60 * time.Second
			}
			log.Printf("Rate limited. Waiting %s before retry...", waitTime)

		case orberrors.ErrOverloaded:
			waitTime = 30 * time.Second
			log.Printf("API overloaded. Waiting %s before retry...", waitTime)

		case orberrors.ErrConnection:
			waitTime = orberrors.BackoffDuration(attempt)
			log.Printf("Connection error (attempt %d/%d). Waiting %s before retry...", attempt+1, maxRetries, waitTime)

		default:
			waitTime = orberrors.BackoffDuration(attempt)
		}

		o.debug.LogRetry(attempt+1, maxRetries, classified.Type.String(), waitTime.String())

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

		// Classify and return the error
		o.debug.Log("Classifying error from stderr=%d bytes, output=%d bytes, errors=%v",
			len(result.Stderr), len(result.Output), result.Errors)
		classified := orberrors.Classify(1, result.Stderr, result.Output, result.Errors)
		o.debug.LogError(classified.Type.String(), classified.Message, classified.Type.IsRetryable())
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
		classified := orberrors.Classify(1, result.Stderr, result.Output, result.Errors)
		o.debug.LogError(classified.Type.String(), classified.Message, classified.Type.IsRetryable())
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

	o.debug.Log("Phase %d completed successfully: cost=$%.4f duration=%s turns=%d",
		phase, result.Cost, result.Duration, result.NumTurns)

	if o.config.Verbose {
		log.Printf("Phase %d: cost=$%.4f, duration=%s, turns=%d",
			phase, result.Cost, result.Duration, result.NumTurns)
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
	}

	o.debug.Log("Executing post-completion command...")
	result, err := o.claudeClient.RunCustomPromptWithSession(o.config.PostCommand, sessionID, isResume)
	o.debug.Log("Post-completion execution completed: err=%v", err)

	// Stop spinner after Claude returns
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
		result, err = o.claudeClient.RunCustomPromptWithSession(o.config.PostCommand, sessionID, false)
		o.debug.Log("Fresh post-completion execution completed: err=%v", err)
	}

	if err != nil {
		o.debug.Log("Post-completion execution failed: %v", err)
		// Save the failed session for debugging
		if o.logManager != nil && result != nil {
			o.debug.Log("Saving failed post-completion session for debugging")
			_ = o.logManager.SavePostCompletionSession(result, startTime)
		}
		classified := orberrors.Classify(1, result.Stderr, result.Output, result.Errors)
		o.debug.LogError(classified.Type.String(), classified.Message, classified.Type.IsRetryable())
		return classified
	}

	// Reconcile session ID if Claude returned a different one
	if o.logManager != nil && result.SessionID != sessionID {
		o.debug.Log("Post-completion session ID changed: expected=%s got=%s", sessionID, result.SessionID)
		o.logManager.ReconcilePostCompletionSessionID(result.SessionID)
	}

	// Check if Claude reported an error in its output
	if result.IsError {
		o.debug.Log("Claude reported error in post-completion output (IsError=true)")
		if o.logManager != nil {
			_ = o.logManager.SavePostCompletionSession(result, startTime)
		}
		classified := orberrors.Classify(1, result.Stderr, result.Output, result.Errors)
		o.debug.LogError(classified.Type.String(), classified.Message, classified.Type.IsRetryable())
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
		result.Cost, result.Duration, result.NumTurns)

	if o.config.Verbose {
		log.Printf("Post-completion: cost=$%.4f, duration=%s, turns=%d",
			result.Cost, result.Duration, result.NumTurns)
	}

	return nil
}

// runPostCommandWithRetry executes the post-command with retry logic for transient errors.
func (o *Orbit) runPostCommandWithRetry() error {
	var lastErr error

	for attempt := range maxRetries {
		err := o.runPostCommand()
		if err == nil {
			return nil
		}

		// Classify the error
		classified, ok := err.(*orberrors.ClassifiedError)
		if !ok {
			return err
		}

		lastErr = err

		if !classified.Type.IsRetryable() {
			return err
		}

		// Pause spinner before logging to prevent visual artifacts
		if o.spinner != nil {
			o.spinner.Pause()
		}

		// Determine wait time
		var waitTime time.Duration
		switch classified.Type {
		case orberrors.ErrRateLimit:
			waitTime = classified.RetryAfter
			if waitTime == 0 {
				waitTime = 60 * time.Second
			}
			log.Printf("Rate limited. Waiting %s before retry...", waitTime)

		case orberrors.ErrOverloaded:
			waitTime = 30 * time.Second
			log.Printf("API overloaded. Waiting %s before retry...", waitTime)

		case orberrors.ErrConnection:
			waitTime = orberrors.BackoffDuration(attempt)
			log.Printf("Connection error (attempt %d/%d). Waiting %s before retry...", attempt+1, maxRetries, waitTime)

		default:
			waitTime = orberrors.BackoffDuration(attempt)
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
func isSessionInvalidError(result *claude.SessionResult) bool {
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
