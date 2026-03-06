package orbit

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
	"github.com/arjenschwarz/orbit/internal/display"
	"github.com/arjenschwarz/orbit/internal/logs"
	"github.com/arjenschwarz/orbit/internal/registry"
	"github.com/google/uuid"
)

// sleepFunc returns the sleep function from the configured clock.
// Falls back to time.Sleep if no clock is configured (e.g., in tests that
// construct Orbit directly without going through New).
func (o *Orbit) sleepFunc() func(time.Duration) {
	if o.config.Clock != nil {
		return o.config.Clock.Sleep
	}
	return time.Sleep
}

// classifyReturned is a Classify callback for RunWithRetry when the Execute
// function already returns a *ClassifiedError (e.g., runPhase, runPostPrompt).
// Returns nil on success, passes through ClassifiedError, and wraps unknown
// errors as fatal.
func classifyReturned(_ *agents.RunResult, err error) *agents.ClassifiedError {
	if err == nil {
		return nil
	}
	var classified *agents.ClassifiedError
	if ok := errors.As(err, &classified); ok {
		return classified
	}
	// Unknown error type — not retryable
	return &agents.ClassifiedError{
		Original: err,
		Class:    agents.ErrorClassFatal,
		Message:  err.Error(),
	}
}

// classifyFromAgent returns a Classify callback that uses the agent's registered
// error classifier. Used by variant and consolidation modes where Execute returns
// raw agent results instead of pre-classified errors.
func classifyFromAgent(agentName string) func(*agents.RunResult, error) *agents.ClassifiedError {
	classifier := agents.GetClassifier(agentName)
	return func(result *agents.RunResult, err error) *agents.ClassifiedError {
		// Success: no error and result is not an error
		if err == nil && (result == nil || !result.IsError) {
			return nil
		}

		// Build inputs for classifier, guarding against nil result
		var stderr, output string
		var errMsgs []string
		if result != nil {
			stderr = result.Stderr
			output = result.Output
			errMsgs = result.Errors
		}
		if err != nil {
			if stderr == "" {
				stderr = err.Error()
			}
			if len(errMsgs) == 0 {
				errMsgs = []string{err.Error()}
			}
		} else if result != nil && result.IsError {
			if len(errMsgs) == 0 {
				errMsgs = []string{"agent reported error"}
			}
		}
		exitCode := 1 // default when result is nil (no exit code available)
		if result != nil {
			exitCode = result.ExitCode
		}
		return classifier.Classify(exitCode, stderr, output, errMsgs)
	}
}

// isSessionInvalidError checks if the result contains a session-related error.
// This is used to detect when a session resume has failed and a fresh session
// should be started instead.
func isSessionInvalidError(result *agents.RunResult) bool {
	if result == nil {
		return false
	}

	combinedLower := strings.ToLower(result.Stderr + result.Output)
	return agents.MatchesSessionInvalid(combinedLower, "no such session", "no conversation found")
}

// classifyRunError classifies an agent run error using the Orbit's error classifier.
// It extracts stderr, output, and error messages from the result (or the error itself
// if the result is nil), and passes the actual exit code to the classifier.
// When result is nil, exit code defaults to 1 since no real exit code is available.
func (o *Orbit) classifyRunError(result *agents.RunResult, err error) *agents.ClassifiedError {
	var stderr, output string
	var errMsgs []string
	exitCode := 1 // default when result is nil (no exit code available)

	if result != nil {
		stderr = result.Stderr
		output = result.Output
		errMsgs = result.Errors
		exitCode = result.ExitCode
	}
	if err != nil {
		if stderr == "" {
			stderr = err.Error()
		}
		if len(errMsgs) == 0 {
			errMsgs = []string{err.Error()}
		}
	} else if result != nil && result.IsError {
		if len(errMsgs) == 0 {
			errMsgs = []string{"agent reported error"}
		}
	}

	o.debug.Log("Classifying error: exitCode=%d, stderr=%d bytes, output=%d bytes, errors=%v",
		exitCode, len(stderr), len(output), errMsgs)
	return o.errorClassifier.Classify(exitCode, stderr, output, errMsgs)
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
		classified := o.classifyRunError(result, err)
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
		classified := o.classifyRunError(result, nil)
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
	_, err := agents.RunWithRetry(o.shutdownCtx, agents.RetryConfig{
		MaxRetries: maxRetries,
		Sleep:      o.sleepFunc(),
		Execute: func(_ context.Context, _ int) (*agents.RunResult, error) {
			return nil, o.runPostPrompt()
		},
		Classify: classifyReturned,
		OnRetry: func(attempt, maxRetries int, classified *agents.ClassifiedError, backoff time.Duration) {
			if o.spinner != nil {
				o.spinner.Pause()
			}

			if classified.Class.IsRateLimitWait() {
				log.Printf("Usage limit reached. Waiting %s until reset...", backoff)
			} else if classified.RetryAfter > 0 {
				log.Printf("Retryable error (attempt %d/%d). Waiting %s before retry...", attempt, maxRetries, backoff)
			} else {
				log.Printf("Error (attempt %d/%d). Waiting %s before retry...", attempt, maxRetries, backoff)
			}

			if o.spinner != nil {
				o.spinner.UpdateWait(backoff)
				o.spinner.Resume()
			}
		},
		AfterWait: func() {
			if o.spinner != nil {
				o.spinner.Stop()
			}
		},
	})
	return err
}

// runPhaseWithRetry executes a phase with retry logic for transient errors.
func (o *Orbit) runPhaseWithRetry(phase int) error {
	o.currentPhaseRunCount = 0
	o.debug.Log("Starting phase %d with up to %d retries", phase, maxRetries)
	phaseStart := time.Now()

	_, err := agents.RunWithRetry(o.shutdownCtx, agents.RetryConfig{
		MaxRetries: maxRetries,
		Sleep:      o.sleepFunc(),
		Execute: func(_ context.Context, attempt int) (*agents.RunResult, error) {
			o.currentPhaseRunCount++
			o.debug.Log("Phase %d attempt %d/%d", phase, attempt+1, maxRetries)
			o.updatePhaseStatus(phase, registry.PhaseStatusRunning, o.currentPhaseRunCount)
			return nil, o.runPhase(phase)
		},
		Classify: func(_ *agents.RunResult, err error) *agents.ClassifiedError {
			if err == nil {
				// Phase succeeded
				o.debug.Log("Phase %d completed successfully", phase)
				o.updatePhaseStatus(phase, registry.PhaseStatusCompleted, o.currentPhaseRunCount)
				o.logPhaseOutcome(phase, "completed", phaseStart)
				return nil
			}
			o.debug.Log("Phase %d failed: %v", phase, err)
			classified := classifyReturned(nil, err)
			o.debug.LogError(classified.Class.String(), classified.Message, classified.Class.IsRetryable())
			o.debug.LogErrorWithChain("Phase execution failed", err, map[string]any{
				"phase":         phase,
				"error_class":   classified.Class.String(),
				"retryable":     classified.Class.IsRetryable(),
				"is_rate_limit": classified.Class.IsRateLimitWait(),
			})
			return classified
		},
		OnRetry: func(attempt, maxRetries int, classified *agents.ClassifiedError, backoff time.Duration) {
			if o.spinner != nil {
				o.spinner.Pause()
			}

			if classified.Class.IsRateLimitWait() {
				log.Printf("Usage limit reached. Waiting %s until reset...", backoff)
				o.debug.Log("Rate limit wait: resetting attempt counter after wait")
			} else if classified.RetryAfter > 0 {
				log.Printf("Retryable error (attempt %d/%d). Waiting %s before retry...", attempt, maxRetries, backoff)
			} else {
				log.Printf("Error (attempt %d/%d). Waiting %s before retry...", attempt, maxRetries, backoff)
			}

			o.debug.LogRetry(attempt, maxRetries, classified.Class.String(), backoff.String())
			o.debug.LogStructured("info", "Retry attempt", map[string]any{
				"phase":            phase,
				"attempt":          attempt,
				"max_attempts":     maxRetries,
				"error_class":      classified.Class.String(),
				"backoff_duration": backoff.String(),
			})

			if o.spinner != nil {
				o.spinner.UpdateWait(backoff)
				o.spinner.Resume()
			}
		},
		AfterWait: func() {
			if o.spinner != nil {
				o.spinner.Stop()
			}
		},
	})

	if err != nil {
		o.debug.Log("Phase %d failed after %d attempts", phase, maxRetries)
		o.updatePhaseStatus(phase, registry.PhaseStatusFailed, o.currentPhaseRunCount)
		o.logPhaseOutcome(phase, "failed", phaseStart)
	}

	return err
}

// logPhaseOutcome logs phase completion or failure with duration and transcript path.
func (o *Orbit) logPhaseOutcome(phase int, status string, phaseStart time.Time) {
	phaseDuration := time.Since(phaseStart)
	logFields := map[string]any{
		"phase":    phase,
		"status":   status,
		"duration": phaseDuration.String(),
	}
	if o.logManager != nil {
		logFields["transcript_path"] = o.logManager.SessionDir()
	}
	level := "info"
	if status == "failed" {
		level = "error"
	}
	o.debug.LogStructured(level, "Phase "+status, logFields)
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

		classified := o.classifyRunError(result, err)
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
		classified := o.classifyRunError(result, nil)
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

