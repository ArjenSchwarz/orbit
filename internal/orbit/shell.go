// Package orbit provides the main orchestration loop for running AI coding agent sessions.
package orbit

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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

// executeShellCommand runs a shell command with timeout and environment setup.
// It executes the command using /bin/sh -c, sets the working directory,
// and adds ORBIT_PHASE_COUNT and ORBIT_AGENT environment variables.
// Returns an error if the command is empty.
func (o *Orbit) executeShellCommand(command, logName string) (*ShellCommandResult, error) {
	if command == "" {
		return nil, fmt.Errorf("command cannot be empty")
	}

	startTime := time.Now()
	result := &ShellCommandResult{
		Command:   command,
		StartedAt: startTime,
	}

	// Create context with timeout, respecting shutdown context
	ctx, cancel := context.WithTimeout(o.shutdownCtx, o.config.CommandTimeout)
	defer cancel()

	// Build command using /bin/sh -c
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	cmd.Dir = o.config.WorkingDir

	// Get phase count from rune client (not from cached phaseSummaries which may be empty)
	phaseCount := 0
	if summaries, err := o.runeClient.GetPhaseSummaries(); err == nil {
		phaseCount = len(summaries)
	}

	// Set up environment with ORBIT_PHASE_COUNT and ORBIT_AGENT
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("ORBIT_PHASE_COUNT=%d", phaseCount),
		fmt.Sprintf("ORBIT_AGENT=%s", o.agent.Name()),
	)

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

	// Save log file
	if o.logManager != nil {
		o.saveShellCommandLog(result, logName)
		// Record in summary.json
		if recordErr := o.logManager.RecordShellCommand(logName, result.Command, result.ExitCode,
			result.StartedAt, result.CompletedAt, result.Duration); recordErr != nil {
			o.debug.Log("Warning: failed to record shell command: %v", recordErr)
		}
	}

	// Check for context errors to provide better error messages
	if ctx.Err() == context.DeadlineExceeded {
		return result, fmt.Errorf("command timed out after %v", o.config.CommandTimeout)
	}
	if ctx.Err() == context.Canceled && o.shutdownCtx.Err() != nil {
		return result, fmt.Errorf("command interrupted by shutdown")
	}

	if err != nil {
		return result, err
	}

	return result, nil
}

// saveShellCommandLog writes the command output to a log file.
// The file is saved in the session directory with the format: {logName}-run-N.txt
func (o *Orbit) saveShellCommandLog(result *ShellCommandResult, logName string) {
	if o.logManager == nil {
		return
	}

	filename := fmt.Sprintf("%s-run-%d.txt", logName, o.logManager.RunNumber())
	path := filepath.Join(o.logManager.SessionDir(), filename)

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
