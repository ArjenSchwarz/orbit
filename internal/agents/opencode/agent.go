// Package opencode provides the OpenCode agent implementation.
package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
)

const defaultPrompt = "Run /next-task --phase and when complete run /commit"

func init() {
	agents.Register("opencode", New)
}

// Agent implements the agents.Agent interface for OpenCode.
type Agent struct {
	config  agents.AgentConfig
	cliPath string
}

// New creates a new OpenCode agent.
func New(cfg agents.AgentConfig) agents.Agent {
	cliPath := cfg.CLIPath
	if cliPath == "" {
		cliPath = "opencode"
	}
	return &Agent{
		config:  cfg,
		cliPath: cliPath,
	}
}

// Name returns the agent identifier.
func (a *Agent) Name() string { return "opencode" }

// CLICommand returns the CLI command to execute.
func (a *Agent) CLICommand() string { return a.cliPath }

// IsInstalled returns true if the OpenCode CLI is available.
func (a *Agent) IsInstalled() bool {
	_, err := exec.LookPath(a.cliPath)
	return err == nil
}

// Version returns the installed OpenCode CLI version.
// OpenCode outputs INFO log lines followed by the version on the last line.
func (a *Agent) Version() (string, error) {
	cmd := exec.Command(a.cliPath, "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return parseVersionOutput(string(output)), nil
}

// parseVersionOutput extracts the version string from OpenCode's --version output.
// OpenCode outputs INFO log lines followed by the version on the last line:
//
//	INFO  2026-01-27T12:16:29 +27ms service=models.dev file={} refreshing
//	1.1.36
func parseVersionOutput(output string) string {
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return strings.TrimSpace(output)
}

// DefaultSessionDir returns the default session storage directory.
func (a *Agent) DefaultSessionDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "opencode", "storage", "message")
}

// sessionMessage represents a message from an OpenCode session file.
type sessionMessage struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionID"`
	Role      string `json:"role"`
	Model     string `json:"model"`
	Time      struct {
		Created time.Time `json:"created"`
	} `json:"time"`
}

// DiscoverSessions lists sessions for a given project directory.
func (a *Agent) DiscoverSessions(ctx context.Context, projectDir string) ([]agents.SessionInfo, error) {
	sessionDir := a.DefaultSessionDir()
	if sessionDir == "" {
		return nil, nil
	}

	// Sessions are stored in ~/.local/share/opencode/storage/message/<sessionID>/
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var sessions []agents.SessionInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		sessionID := entry.Name()
		sessionPath := filepath.Join(sessionDir, sessionID)

		// Read the first message file to get metadata
		msgFiles, err := os.ReadDir(sessionPath)
		if err != nil {
			continue
		}

		var createdAt time.Time
		for _, msgFile := range msgFiles {
			if msgFile.IsDir() || !strings.HasPrefix(msgFile.Name(), "msg_") {
				continue
			}

			msgPath := filepath.Join(sessionPath, msgFile.Name())
			data, err := os.ReadFile(msgPath)
			if err != nil {
				continue
			}

			var msg sessionMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}

			createdAt = msg.Time.Created
			break // Only need the first message for metadata
		}

		info, _ := entry.Info()
		var size int64
		if info != nil {
			size = info.Size()
		}

		sessions = append(sessions, agents.SessionInfo{
			ID:        sessionID,
			Agent:     "opencode",
			Path:      sessionPath,
			CreatedAt: createdAt,
			Size:      size,
		})
	}

	return sessions, nil
}

// Run executes a prompt in a new session.
func (a *Agent) Run(ctx context.Context, opts agents.RunOptions) (*agents.RunResult, error) {
	return a.execute(ctx, opts, false)
}

// Resume continues an existing session.
// OpenCode uses --continue to resume the most recent session.
func (a *Agent) Resume(ctx context.Context, sessionID string, opts agents.RunOptions) (*agents.RunResult, error) {
	opts.SessionID = sessionID
	return a.execute(ctx, opts, true)
}

// buildArgs constructs the command-line arguments for an OpenCode session.
func (a *Agent) buildArgs(opts agents.RunOptions, resume bool) []string {
	prompt := opts.Prompt
	if prompt == "" {
		prompt = defaultPrompt
	}

	// OpenCode uses: opencode run --format json "<prompt>"
	args := []string{"run", "--format", "json"}

	// Resume with --continue flag
	if resume {
		args = append(args, "--continue")
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

// execute runs the OpenCode CLI with the given options.
func (a *Agent) execute(ctx context.Context, opts agents.RunOptions, resume bool) (*agents.RunResult, error) {
	args := a.buildArgs(opts, resume)

	cmd := exec.CommandContext(ctx, a.cliPath, args...)
	if opts.WorkDir != "" {
		cmd.Dir = opts.WorkDir
	}

	// Set environment variables
	if len(opts.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range opts.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = nil // Explicitly close stdin so OpenCode doesn't wait for input

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

	// OpenCode may exit with code 0 even on errors.
	// Detect errors by checking if stdout contains valid JSON.
	if !isValidJSON(stdout.String()) && stdout.Len() > 0 {
		result.IsError = true
		result.Errors = append(result.Errors, "output is not valid JSON")
	}

	if err != nil {
		result.Error = err
	}

	return result, err
}

// isValidJSON checks if the string is valid JSON.
func isValidJSON(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	var js json.RawMessage
	return json.Unmarshal([]byte(s), &js) == nil
}
