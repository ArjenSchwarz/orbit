// Package kiro provides the Kiro agent implementation.
package kiro

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
)

const defaultPrompt = "Run /next-task --phase and when complete run /commit"

func init() {
	agents.Register("kiro", New)
}

// Agent implements the agents.Agent interface for Kiro.
type Agent struct {
	config  agents.AgentConfig
	cliPath string
}

// Compile-time interface checks.
var (
	_ agents.Agent           = (*Agent)(nil)
	_ agents.SessionExporter = (*Agent)(nil)
)

// New creates a new Kiro agent.
func New(cfg agents.AgentConfig) agents.Agent {
	cliPath := cfg.CLIPath
	if cliPath == "" {
		cliPath = "kiro-cli"
	}
	return &Agent{
		config:  cfg,
		cliPath: cliPath,
	}
}

// Name returns the agent identifier.
func (a *Agent) Name() string { return "kiro" }

// CLICommand returns the CLI command to execute.
func (a *Agent) CLICommand() string { return a.cliPath }

// IsInstalled returns true if the Kiro CLI is available.
func (a *Agent) IsInstalled() bool {
	_, err := exec.LookPath(a.cliPath)
	return err == nil
}

// Version returns the installed Kiro CLI version.
func (a *Agent) Version() (string, error) {
	cmd := exec.Command(a.cliPath, "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(output)), nil
}

// DefaultSessionDir returns the default session storage directory.
// Kiro does not store sessions automatically per Decision 7.
func (a *Agent) DefaultSessionDir() string {
	return ""
}

// DiscoverSessions lists sessions for a given project directory.
// Kiro does not store sessions automatically, so this always returns nil.
func (a *Agent) DiscoverSessions(ctx context.Context, projectDir string) ([]agents.SessionInfo, error) {
	// Kiro doesn't have automatic session storage per Decision 7
	return nil, nil
}

// Run executes a prompt in a new session.
func (a *Agent) Run(ctx context.Context, opts agents.RunOptions) (*agents.RunResult, error) {
	return a.execute(ctx, opts, false)
}

// Resume continues an existing session.
// Note: Kiro uses --resume to continue the current session.
func (a *Agent) Resume(ctx context.Context, sessionID string, opts agents.RunOptions) (*agents.RunResult, error) {
	opts.SessionID = sessionID
	return a.execute(ctx, opts, true)
}

// ExportSession implements agents.SessionExporter.
// This runs a follow-up command to save the session since Kiro doesn't store logs automatically.
// Called by the orchestrator after each phase completes.
func (a *Agent) ExportSession(ctx context.Context, filename string) error {
	args := a.buildExportArgs(filename)

	cmd := exec.CommandContext(ctx, a.cliPath, args...)
	cmd.Stdin = nil // Explicitly close stdin so Kiro doesn't wait for input

	return cmd.Run()
}

// buildArgs constructs the command-line arguments for a Kiro session.
func (a *Agent) buildArgs(opts agents.RunOptions, resume bool) []string {
	prompt := opts.Prompt
	if prompt == "" {
		prompt = defaultPrompt
	}

	// Kiro uses: kiro-cli chat --no-interactive "<prompt>"
	// With --trust-all-tools for automatic approval
	// With --resume to continue previous session
	args := []string{"chat", "--no-interactive"}

	if a.config.AutoApprove {
		args = append(args, "--trust-all-tools")
	}

	if resume {
		args = append(args, "--resume")
	}

	// Add model flag if configured
	if model, ok := a.config.Options["model"]; ok && model != "" {
		args = append(args, "--model", model)
	}

	// Add config-level extra args
	args = append(args, a.config.ExtraArgs...)

	// Add per-invocation extra args
	args = append(args, opts.ExtraArgs...)

	// Prompt comes last
	args = append(args, prompt)

	return args
}

// buildExportArgs constructs the command-line arguments for exporting a session.
func (a *Agent) buildExportArgs(filename string) []string {
	// Export uses: kiro-cli chat --no-interactive "/chat save <filename>" --resume
	// With --trust-all-tools for automatic approval when AutoApprove is enabled
	// Quote the filename to handle paths with spaces or special characters
	args := []string{"chat", "--no-interactive"}

	if a.config.AutoApprove {
		args = append(args, "--trust-all-tools")
	}

	args = append(args, "/chat save \""+filename+"\"", "--resume")
	return args
}

// execute runs the Kiro CLI with the given options.
func (a *Agent) execute(ctx context.Context, opts agents.RunOptions, resume bool) (*agents.RunResult, error) {
	args := a.buildArgs(opts, resume)

	cmd := exec.CommandContext(ctx, a.cliPath, args...)
	if opts.WorkDir != "" {
		cmd.Dir = opts.WorkDir
	}

	// Set environment variables
	if len(opts.Env) > 0 {
		cmd.Env = appendEnv(opts.Env)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = nil // Explicitly close stdin so Kiro doesn't wait for input

	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	result := &agents.RunResult{
		SessionID: opts.SessionID,
		Duration:  duration,
		Output:    stdout.String(),
		Stderr:    stderr.String(),
	}

	// Get exit code
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
	}

	if err != nil {
		result.Error = err
	}

	return result, err
}

// appendEnv appends environment variables to the current environment.
func appendEnv(env map[string]string) []string {
	result := os.Environ() // Start with existing environment
	for k, v := range env {
		result = append(result, k+"="+v)
	}
	return result
}
