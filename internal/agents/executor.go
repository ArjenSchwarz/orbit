package agents

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"time"
)

// ExecuteConfig holds the parameters for running an agent CLI command.
type ExecuteConfig struct {
	// CLIPath is the path or name of the CLI binary.
	CLIPath string
	// Args are the command-line arguments (not including the binary name).
	Args []string
	// WorkDir sets the working directory for the command. Empty means inherit.
	WorkDir string
	// Env provides additional environment variables to merge with os.Environ().
	// When nil or empty, the command inherits the current environment unchanged.
	Env map[string]string
}

// ExecuteResult holds the raw output from running an agent CLI command.
type ExecuteResult struct {
	// Stdout is the captured standard output.
	Stdout []byte
	// Stderr is the captured standard error.
	Stderr []byte
	// ExitCode is the process exit code: 0 for success, -1 if the exit code
	// could not be determined (e.g. the command was not found).
	ExitCode int
	// Duration is the wall-clock time the command took.
	Duration time.Duration
	// Err is the raw error returned by cmd.Run(), nil on success.
	Err error
}

// Execute runs a CLI command with stdout/stderr capture, timing, and exit code
// extraction. This is the shared execution scaffolding used by all agents.
//
// The caller is responsible for building the args (agent-specific) and
// post-processing the result (parsing output, extracting costs, etc.).
func Execute(ctx context.Context, cfg ExecuteConfig) *ExecuteResult {
	cmd := exec.CommandContext(ctx, cfg.CLIPath, cfg.Args...)

	if cfg.WorkDir != "" {
		cmd.Dir = cfg.WorkDir
	}

	if len(cfg.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range cfg.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = nil // Prevent agents from blocking on interactive input

	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	result := &ExecuteResult{
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
		Duration: duration,
		Err:      err,
	}

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
	}

	return result
}
