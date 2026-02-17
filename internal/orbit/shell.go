// Package orbit provides the main orchestration loop for running AI coding agent sessions.
package orbit

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/arjenschwarz/orbit/internal/logs"
)

// ShellCommandResult holds the result of a shell command execution.
type ShellCommandResult struct {
	Command     string        // The command that was executed
	ExitCode    int           // Exit code (0 = success)
	Stdout      string        // Standard output
	Stderr      string        // Standard error
	Duration    time.Duration // Execution duration
	StartedAt   time.Time     // When the command started
	CompletedAt time.Time     // When the command completed
}

// shellExecParams captures the varying parameters for shell command execution.
// Both single-run and variant modes provide different values for these fields
// while sharing the same core execution logic via runShellCore.
type shellExecParams struct {
	ctx        context.Context // Parent context (shutdownCtx for single-run, variant ctx for variants)
	workDir    string          // Working directory for the command
	phaseCount int             // Number of phases (from rune client)
	agentName  string          // Agent name for ORBIT_AGENT env var
	variantID  int             // Variant ID for ORBIT_VARIANT env var (0 = not a variant)
	logManager *logs.Manager   // Log manager for saving output (nil = skip logging)
}

// executeShellCommand runs a shell command with timeout and environment setup.
// It executes the command using /bin/sh -c, sets the working directory,
// and adds ORBIT_PHASE_COUNT and ORBIT_AGENT environment variables.
// Returns an error if the command is empty or if running on Windows.
func (o *Orbit) executeShellCommand(command, logName string) (*ShellCommandResult, error) {
	// Get phase count from rune client (not from cached phaseSummaries which may be empty)
	phaseCount := 0
	if summaries, err := o.runeClient.GetPhaseSummaries(); err == nil {
		phaseCount = len(summaries)
	}

	return o.runShellCore(command, logName, shellExecParams{
		ctx:        o.shutdownCtx,
		workDir:    o.config.WorkingDir,
		phaseCount: phaseCount,
		agentName:  o.agent.Name(),
		logManager: o.logManager,
	})
}

// runShellCore is the shared implementation for shell command execution.
// It handles command creation, environment setup, output capture, exit code
// extraction, logging, and context error reporting. Both executeShellCommand
// (single-run) and executeVariantShellCommand (variant mode) delegate here.
func (o *Orbit) runShellCore(command, logName string, params shellExecParams) (*ShellCommandResult, error) {
	if command == "" {
		return nil, fmt.Errorf("command cannot be empty")
	}

	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("shell commands are not supported on Windows (requires /bin/sh)")
	}

	startTime := time.Now()
	result := &ShellCommandResult{
		Command:   command,
		StartedAt: startTime,
	}

	// Create context with timeout, respecting the parent context
	cmdCtx, cancel := context.WithTimeout(params.ctx, o.config.CommandTimeout)
	defer cancel()

	// Build command using /bin/sh -c
	cmd := exec.CommandContext(cmdCtx, "/bin/sh", "-c", command)
	cmd.Dir = params.workDir

	// Set up environment variables
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("ORBIT_PHASE_COUNT=%d", params.phaseCount),
		fmt.Sprintf("ORBIT_AGENT=%s", params.agentName),
	)
	if params.variantID > 0 {
		cmd.Env = append(cmd.Env, fmt.Sprintf("ORBIT_VARIANT=%d", params.variantID))
	}

	// Capture stdout and stderr
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute the command
	err := cmd.Run()
	result.CompletedAt = time.Now()
	result.Duration = result.CompletedAt.Sub(startTime)
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()

	// Get exit code
	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
	} else if err != nil {
		result.ExitCode = -1 // Command didn't start or context was canceled
	}

	// Save log file and record in summary
	if params.logManager != nil {
		o.saveShellCommandLog(result, logName, params.logManager)
		if recordErr := params.logManager.RecordShellCommand(logName, result.Command, result.ExitCode,
			result.StartedAt, result.CompletedAt, result.Duration); recordErr != nil {
			o.debug.Log("Warning: failed to record shell command: %v", recordErr)
		}
	}

	// Check for context errors to provide better error messages
	if cmdCtx.Err() == context.DeadlineExceeded {
		return result, fmt.Errorf("command timed out after %v", o.config.CommandTimeout)
	}
	if cmdCtx.Err() == context.Canceled && params.ctx.Err() != nil {
		return result, fmt.Errorf("command interrupted by shutdown")
	}

	if err != nil {
		return result, err
	}

	return result, nil
}

// saveShellCommandLog writes the command output to a log file.
// The file is saved in the session directory with the format: {logName}-run-N.txt
func (o *Orbit) saveShellCommandLog(result *ShellCommandResult, logName string, logManager *logs.Manager) {
	if logManager == nil {
		return
	}

	filename := fmt.Sprintf("%s-run-%d.txt", logName, logManager.RunNumber())
	path := filepath.Join(logManager.SessionDir(), filename)

	content := fmt.Sprintf(`Orbit Shell Command Log
========================================

Command: %s
Exit Code: %d
Started: %s
Completed: %s
Duration: %s

Stdout:
----------------------------------------
%s

Stderr:
----------------------------------------
%s
`,
		result.Command,
		result.ExitCode,
		result.StartedAt.Format(time.RFC3339),
		result.CompletedAt.Format(time.RFC3339),
		result.Duration.String(),
		result.Stdout,
		result.Stderr,
	)

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		o.debug.Log("Warning: failed to save %s log to %s: %v", logName, path, err)
	}
}
