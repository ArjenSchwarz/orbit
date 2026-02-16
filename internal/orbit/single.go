package orbit

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
	"github.com/arjenschwarz/orbit/internal/display"
	orberrors "github.com/arjenschwarz/orbit/internal/errors"
	"github.com/arjenschwarz/orbit/internal/logs"
	"github.com/arjenschwarz/orbit/internal/registry"
	"github.com/google/uuid"
)

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
		"no conversation found",
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

// runPrePrompt executes the pre-prompt and stores the session ID for phase 1.
// The session started by pre-prompt is continued by phase 1.
// Failure aborts the entire run.
func (o *Orbit) runPrePrompt() error {
	if o.config.PrePrompt == "" {
		return nil // No pre-prompt configured
	}

	if o.config.DryRun {
		log.Printf("[DRY RUN] Would execute pre-prompt")
		return nil
	}

	// Check pre-prompt state for resumption (crash recovery)
	if o.logManager != nil {
		sessionID, status := o.logManager.GetPrePromptState()
		switch status {
		case logs.PrePromptStatusCompleted:
			// Already completed in previous run - use stored session
			o.prePromptSessionID = sessionID
			o.debug.Log("Pre-prompt already completed, using session: %s", sessionID)
			log.Println("Pre-prompt already completed, using existing session")
			return nil
		case logs.PrePromptStatusStarted:
			// Started but crashed - will attempt resume below
			o.debug.Log("Pre-prompt was interrupted, will resume session: %s", sessionID)
			// case PrePromptStatusNotStarted: fall through to start fresh
		}
	}

	o.debug.Log("Running pre-prompt...")
	log.Println("Running pre-prompt...")

	// Start spinner for pre-prompt
	if o.spinner != nil {
		o.spinner.StartPrePrompt()
	}

	// Get or generate session ID
	var sessionID string
	var isResume bool
	if o.logManager != nil {
		var err error
		sessionID, isResume, err = o.logManager.StartPrePrompt(o.config.ContinueSession)
		if err != nil {
			if o.spinner != nil {
				o.spinner.Stop()
			}
			o.debug.Log("Failed to start pre-prompt in log manager: %v", err)
			return fmt.Errorf("failed to start pre-prompt: %w", err)
		}
		o.debug.LogSession(sessionID, isResume, "pre-prompt obtained from log manager")
	} else {
		sessionID = uuid.NewString()
		isResume = false
		o.debug.LogSession(sessionID, isResume, "generated new (no log manager)")
	}

	// Execute pre-prompt using the agent
	opts := agents.RunOptions{
		Prompt:    o.config.PrePrompt,
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

	// Stop spinner after agent returns
	if o.spinner != nil {
		o.spinner.Stop()
	}

	// Handle resume failure - start fresh session
	if err != nil && isResume && isSessionInvalidError(result) {
		o.debug.Log("Pre-prompt session resume failed, detected invalid session error")
		log.Printf("Warning: Pre-prompt session resume failed, starting fresh session")
		sessionID = uuid.NewString()
		o.debug.LogSession(sessionID, false, "pre-prompt retrying with fresh session")

		// Restart spinner
		if o.spinner != nil {
			o.spinner.StartPrePrompt()
		}

		opts.SessionID = sessionID
		result, err = o.agent.Run(o.shutdownCtx, opts)

		if o.spinner != nil {
			o.spinner.Stop()
		}
	}

	if err != nil {
		o.debug.Log("Pre-prompt execution failed: %v", err)
		// Pre-prompt failure is fatal
		return fmt.Errorf("pre-prompt failed: %w", err)
	}

	// Check if agent reported an error in its output
	if result != nil && result.IsError {
		o.debug.Log("Agent reported error in pre-prompt output (IsError=true)")
		return fmt.Errorf("pre-prompt failed: agent reported error")
	}

	// Store session ID for phase 1
	if result != nil {
		o.prePromptSessionID = result.SessionID
		o.debug.Log("Pre-prompt completed, session %s will continue in phase 1", result.SessionID)
	}

	// Mark complete in log manager
	if o.logManager != nil {
		finalSessionID := sessionID
		if result != nil && result.SessionID != "" {
			finalSessionID = result.SessionID
		}
		if err := o.logManager.CompletePrePrompt(finalSessionID); err != nil {
			log.Printf("Warning: failed to complete pre-prompt: %v", err)
			o.debug.Log("Failed to complete pre-prompt in log manager: %v", err)
		}
	}

	log.Println("Pre-prompt finished")
	return nil
}

// runPostPrompt executes the post-completion command.
func (o *Orbit) runPostPrompt() error {
	startTime := time.Now()
	o.debug.Log("runPostPrompt starting")

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
		Prompt:    o.config.PostPrompt,
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

	postCostStr := formatCost(result)
	o.debug.Log("Post-completion completed successfully: cost=%s duration=%s turns=%d",
		postCostStr, getSessionDuration(result), result.NumTurns)

	if o.config.Verbose {
		log.Printf("Post-completion: cost=%s, duration=%s, turns=%d",
			postCostStr, getSessionDuration(result), result.NumTurns)
	}

	return nil
}

// runPostPromptWithRetry executes the post-prompt with retry logic for transient errors.
func (o *Orbit) runPostPromptWithRetry() error {
	var lastErr error

	for attempt := range maxRetries {
		err := o.runPostPrompt()
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

		o.config.Clock.Sleep(waitTime)

		// Stop spinner before next attempt (runPostPrompt will start it again)
		if o.spinner != nil {
			o.spinner.Stop()
		}
	}

	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// runPhaseWithRetry executes a phase with retry logic for transient errors.
func (o *Orbit) runPhaseWithRetry(phase int) error {
	var lastErr error
	o.currentPhaseRunCount = 0

	o.debug.Log("Starting phase %d with up to %d retries", phase, maxRetries)

	phaseStart := time.Now()

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

			// Log phase completion with duration and transcript path (Req 3.3, 4.2)
			phaseDuration := time.Since(phaseStart)
			logFields := map[string]any{
				"phase":    phase,
				"status":   "completed",
				"duration": phaseDuration.String(),
			}
			if o.logManager != nil {
				logFields["transcript_path"] = o.logManager.SessionDir()
			}
			o.debug.LogStructured("info", "Phase completed", logFields)

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

		// Log error with chain for structured output (Req 3.8)
		o.debug.LogErrorWithChain("Phase execution failed", err, map[string]any{
			"phase":         phase,
			"attempt":       attempt + 1,
			"error_class":   classified.Class.String(),
			"retryable":     classified.Class.IsRetryable(),
			"is_rate_limit": classified.Class.IsRateLimitWait(),
		})

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
		if classified.Class.IsRateLimitWait() {
			// Usage limit wait - this is a special case where we wait until a specific time
			// and then reset the attempt counter since the limit has been lifted
			waitTime = classified.RetryAfter
			log.Printf("Usage limit reached. Waiting %s until reset...", waitTime)
			o.debug.Log("Rate limit wait: resetting attempt counter after wait")
			// Reset attempt counter after this wait - the rate limit will be lifted
			// We use -1 because the loop will increment it to 0
			attempt = -1
		} else if classified.RetryAfter > 0 {
			waitTime = classified.RetryAfter
			log.Printf("Retryable error (attempt %d/%d). Waiting %s before retry...", attempt+1, maxRetries, waitTime)
		} else {
			waitTime = orberrors.BackoffDuration(attempt)
			log.Printf("Error (attempt %d/%d). Waiting %s before retry...", attempt+1, maxRetries, waitTime)
		}

		o.debug.LogRetry(attempt+1, maxRetries, classified.Class.String(), waitTime.String())

		// Log retry with structured fields (Req 3.6)
		o.debug.LogStructured("info", "Retry attempt", map[string]any{
			"phase":            phase,
			"attempt":          attempt + 1,
			"max_attempts":     maxRetries,
			"error_class":      classified.Class.String(),
			"backoff_duration": waitTime.String(),
		})

		// Resume spinner with wait countdown during retry wait
		if o.spinner != nil {
			o.spinner.UpdateWait(waitTime)
			o.spinner.Resume()
		}

		o.config.Clock.Sleep(waitTime)

		// Stop spinner before next phase attempt (runPhase will start it again)
		if o.spinner != nil {
			o.spinner.Stop()
		}
	}

	o.debug.Log("Phase %d failed after %d attempts", phase, maxRetries)
	// Update phase status to failed after max retries (req 3.6)
	o.updatePhaseStatus(phase, registry.PhaseStatusFailed, o.currentPhaseRunCount)

	// Log phase failure with duration and transcript path (Req 3.3, 4.2)
	phaseDuration := time.Since(phaseStart)
	logFields := map[string]any{
		"phase":    phase,
		"status":   "failed",
		"duration": phaseDuration.String(),
	}
	if o.logManager != nil {
		logFields["transcript_path"] = o.logManager.SessionDir()
	}
	o.debug.LogStructured("error", "Phase failed", logFields)

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

	// Determine if phase 1 should use pre-prompt session
	var overrideSessionID string
	if phase == 1 && o.prePromptSessionID != "" {
		overrideSessionID = o.prePromptSessionID
		o.debug.Log("Phase 1 will continue pre-prompt session: %s", overrideSessionID)
	}

	// Get session ID and determine if resuming (req 3.1-3.3)
	var sessionID string
	var isResume bool
	if o.logManager != nil {
		var err error
		sessionID, isResume, err = o.logManager.StartPhase(phase, o.config.ContinueSession, overrideSessionID)
		if err != nil {
			o.debug.Log("Failed to start phase in log manager: %v", err)
			return o.fail(fmt.Errorf("failed to start phase: %w", err))
		}
		o.debug.LogSession(sessionID, isResume, "obtained from log manager")
	} else {
		// No log manager - use override or generate new
		if overrideSessionID != "" {
			sessionID = overrideSessionID
			isResume = true
		} else {
			sessionID = uuid.NewString()
			isResume = false
		}
		o.debug.LogSession(sessionID, isResume, "generated (no log manager)")
	}

	o.debug.Log("Executing agent %s for phase %d...", o.agent.Name(), phase)

	// Log agent invocation (Req 3.4)
	o.debug.LogStructured("info", "Agent invocation", map[string]any{
		"agent":       o.config.Agent,
		"phase":       phase,
		"session_id":  sessionID,
		"is_resume":   isResume,
		"working_dir": o.config.WorkingDir,
	})

	// Execute using the configured agent (not hardcoded to Claude)
	opts := agents.RunOptions{
		Prompt:    o.config.Command,
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
	o.debug.Log("Agent execution completed: err=%v", err)

	// Stop spinner after agent returns
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
		opts.SessionID = sessionID
		result, err = o.agent.Run(o.shutdownCtx, opts)
		o.debug.Log("Fresh session execution completed: err=%v", err)
	}

	if err != nil {
		o.debug.Log("Phase execution failed: %v", err)

		// Save the failed session for debugging
		if o.logManager != nil && result != nil {
			o.debug.Log("Saving failed session for debugging")
			_ = o.logManager.SaveSession(phase, result, startTime)
		}

		// Classify using agent-specific classifier.
		// Guard against nil result: when the agent returns (nil, error), use the
		// error message for classification instead of dereferencing nil fields.
		var stderr, output string
		var errMsgs []string
		if result != nil {
			stderr = result.Stderr
			output = result.Output
			errMsgs = result.Errors
		} else {
			stderr = err.Error()
			errMsgs = []string{err.Error()}
		}
		o.debug.Log("Classifying error from stderr=%d bytes, output=%d bytes, errors=%v",
			len(stderr), len(output), errMsgs)
		classified := o.errorClassifier.Classify(1, stderr, output, errMsgs)
		o.debug.LogError(classified.Class.String(), classified.Message, classified.Class.IsRetryable())
		return classified
	}

	// Reconcile session ID if agent returned a different one (req 2.5, 2.6)
	if o.logManager != nil && result.SessionID != sessionID {
		o.debug.Log("Session ID changed: expected=%s got=%s", sessionID, result.SessionID)
		o.logManager.ReconcileSessionID(result.SessionID)
	}

	// Check if agent reported an error in its output
	if result.IsError {
		o.debug.Log("Agent reported error in output (IsError=true)")
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

	// Log agent completion (Req 3.5, 4.1)
	agentLogFields := map[string]any{
		"phase":      phase,
		"exit_code":  0,
		"duration":   result.Duration.String(),
		"session_id": result.SessionID,
		"num_turns":  result.NumTurns,
		"cost_usd":   getCostValue(result),
	}
	if result.Cost != nil && result.Cost.Credits > 0 {
		agentLogFields["credits"] = result.Cost.Credits
	}
	if o.logManager != nil {
		agentLogFields["session_log_path"] = o.logManager.SessionDir()
	}
	o.debug.LogStructured("info", "Agent completed", agentLogFields)

	costStr := formatCost(result)
	o.debug.Log("Phase %d completed successfully: cost=%s duration=%s turns=%d",
		phase, costStr, getSessionDuration(result), result.NumTurns)

	if o.config.Verbose {
		log.Printf("Phase %d: cost=%s, duration=%s, turns=%d",
			phase, costStr, getSessionDuration(result), result.NumTurns)
	}

	return nil
}

// runAgentPreCommand executes the agent's pre-command shell script.
// This runs before the first phase and before the pre-prompt (if configured).
// Failure aborts the entire run.
func (o *Orbit) runAgentPreCommand() error {
	if o.config.AgentPreCommand == "" {
		return nil // No pre-command configured
	}

	if o.config.DryRun {
		log.Printf("[DRY RUN] Would execute pre-command: %s", o.config.AgentPreCommand)
		log.Printf("[DRY RUN] Working directory: %s", o.config.WorkingDir)
		return nil
	}

	o.debug.Log("Running agent pre-command: %s", o.config.AgentPreCommand)
	log.Println("Running agent pre-command...")

	result, err := o.executeShellCommand(o.config.AgentPreCommand, "pre-command")
	if err != nil {
		o.debug.Log("Agent pre-command failed: exit_code=%d, err=%v", result.ExitCode, err)
		return fmt.Errorf("pre-command failed (exit code %d): %w", result.ExitCode, err)
	}

	o.debug.Log("Agent pre-command completed successfully")
	log.Println("Agent pre-command finished")
	return nil
}

// runAgentPostCommand executes the agent's post-command shell script.
// This runs after all phases complete and after the post-prompt (if configured).
// Failure logs a warning but does not fail the run.
func (o *Orbit) runAgentPostCommand() error {
	if o.config.AgentPostCommand == "" {
		return nil // No post-command configured
	}

	if o.config.DryRun {
		log.Printf("[DRY RUN] Would execute post-command: %s", o.config.AgentPostCommand)
		log.Printf("[DRY RUN] Working directory: %s", o.config.WorkingDir)
		return nil
	}

	o.debug.Log("Running agent post-command: %s", o.config.AgentPostCommand)
	log.Println("Running agent post-command...")

	result, err := o.executeShellCommand(o.config.AgentPostCommand, "post-command")
	if err != nil {
		// Post-command failure is warning, not fatal
		o.debug.Log("Agent post-command failed: exit_code=%d, err=%v", result.ExitCode, err)
		log.Printf("Warning: post-command failed (exit code %d): %v", result.ExitCode, err)
		return nil // Don't fail the run
	}

	o.debug.Log("Agent post-command completed successfully")
	log.Println("Agent post-command finished")
	return nil
}

// runSingle executes the single-run orchestration loop (existing behavior).
func (o *Orbit) runSingle() error {
	// === Step 1: Agent Pre-Command (shell) ===
	if err := o.runAgentPreCommand(); err != nil {
		return o.fail(err)
	}

	// === Step 2: Pre-Prompt (AI) ===
	if err := o.runPrePrompt(); err != nil {
		return o.fail(err)
	}

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
			if o.config.PostPrompt != "" {
				log.Printf("[DRY RUN] Post-command: %s", o.config.PostPrompt)
			} else {
				log.Printf("[DRY RUN] Post-command: (disabled)")
			}
			return nil
		}

		// Log phase start
		log.Printf("Starting phase %d: %s (%d tasks)", phaseNum, nextPhase.PhaseName, len(nextPhase.Tasks))
		o.debug.LogStructured("info", "Phase started", map[string]any{
			"phase":      phaseNum,
			"phase_name": nextPhase.PhaseName,
			"task_count": len(nextPhase.Tasks),
		})

		// Run the phase
		if err := o.runPhaseWithRetry(phaseNum); err != nil {
			return o.fail(err)
		}

		log.Printf("Completed phase %d: %s", phaseNum, nextPhase.PhaseName)
	}
}

// complete handles successful orchestration completion, including post-prompt and post-command execution.
func (o *Orbit) complete() error {
	// === Step 4: Post-Prompt (AI) - renamed from post-command ===
	if o.config.PostPrompt != "" {
		log.Println("Running post-prompt...")
		if err := o.runPostPromptWithRetry(); err != nil {
			log.Printf("Orchestration succeeded but post-prompt failed: %v", err)
			return o.fail(err)
		}
		log.Println("Post-prompt finished")
	}

	// === Step 5: Agent Post-Command (shell) ===
	// Post-command failure logs warning but doesn't fail the run
	if err := o.runAgentPostCommand(); err != nil {
		// Error already logged in runAgentPostCommand
		_ = err // Explicitly ignore - post-command failure is warning only
	}

	// Write shutdown entry to centralized log
	o.debug.LogShutdown("completed")

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

