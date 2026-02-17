// Package codex provides the Codex agent implementation.
package codex

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/arjenschwarz/orbit/internal/agents"
)

// Compile-time interface check.
var _ agents.Agent = (*Agent)(nil)

const defaultPrompt = "Run /next-task --phase and when complete run /commit"

func init() {
	agents.Register("codex", New)
}

// Agent implements the agents.Agent interface for Codex.
type Agent struct {
	config  agents.AgentConfig
	cliPath string
}

// New creates a new Codex agent.
func New(cfg agents.AgentConfig) agents.Agent {
	cliPath := cfg.CLIPath
	if cliPath == "" {
		cliPath = "codex"
	}
	return &Agent{
		config:  cfg,
		cliPath: cliPath,
	}
}

// Name returns the agent identifier.
func (a *Agent) Name() string { return "codex" }

// CLICommand returns the CLI command to execute.
func (a *Agent) CLICommand() string { return a.cliPath }

// IsInstalled returns true if the Codex CLI is available.
func (a *Agent) IsInstalled() bool {
	_, err := exec.LookPath(a.cliPath)
	return err == nil
}

// Version returns the installed Codex CLI version.
func (a *Agent) Version() (string, error) {
	cmd := exec.Command(a.cliPath, "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(output)), nil
}

// DefaultSessionDir returns the default session storage directory.
func (a *Agent) DefaultSessionDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex", "sessions")
}

// DiscoverSessions lists sessions for a given project directory.
func (a *Agent) DiscoverSessions(ctx context.Context, projectDir string) ([]agents.SessionInfo, error) {
	sessionDir := a.DefaultSessionDir()
	if sessionDir == "" {
		return nil, nil
	}

	// Sessions may be stored in ~/.codex/sessions/
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var sessions []agents.SessionInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		sessions = append(sessions, agents.SessionInfo{
			ID:        entry.Name(),
			Agent:     "codex",
			Path:      filepath.Join(sessionDir, entry.Name()),
			CreatedAt: info.ModTime(),
			Size:      info.Size(),
		})
	}

	return sessions, nil
}

// Run executes a prompt in a new session.
func (a *Agent) Run(ctx context.Context, opts agents.RunOptions) (*agents.RunResult, error) {
	return a.execute(ctx, opts, false)
}

// Resume continues an existing session.
// Codex exec does not support session resumption, so this starts a fresh session.
func (a *Agent) Resume(ctx context.Context, sessionID string, opts agents.RunOptions) (*agents.RunResult, error) {
	return a.execute(ctx, opts, false)
}

// buildArgs constructs the command-line arguments for a Codex session.
func (a *Agent) buildArgs(opts agents.RunOptions) []string {
	prompt := opts.Prompt
	if prompt == "" {
		prompt = defaultPrompt
	}

	// Codex uses: codex exec "<prompt>"
	// With --dangerously-bypass-approvals-and-sandbox for non-interactive operation
	// Note: codex exec does not support session resumption (no --last flag)
	args := []string{"exec"}

	if a.config.AutoApprove {
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
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

// execute runs the Codex CLI with the given options.
func (a *Agent) execute(ctx context.Context, opts agents.RunOptions, _ bool) (*agents.RunResult, error) {
	args := a.buildArgs(opts)

	execResult := agents.Execute(ctx, agents.ExecuteConfig{
		CLIPath: a.cliPath,
		Args:    args,
		WorkDir: opts.WorkDir,
		Env:     opts.Env,
	})

	result := &agents.RunResult{
		SessionID: opts.SessionID,
		Duration:  execResult.Duration,
		ExitCode:  execResult.ExitCode,
		Output:    string(execResult.Stdout),
		Stderr:    execResult.Stderr,
	}

	if execResult.Err != nil {
		result.Error = execResult.Err
	}

	return result, execResult.Err
}
