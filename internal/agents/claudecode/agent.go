// Package claudecode provides the Claude Code agent implementation.
package claudecode

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
	agents.Register("claude-code", New)
}

// Agent implements the agents.Agent interface for Claude Code.
type Agent struct {
	config  agents.AgentConfig
	cliPath string
}

// New creates a new Claude Code agent.
func New(cfg agents.AgentConfig) agents.Agent {
	cliPath := cfg.CLIPath
	if cliPath == "" {
		cliPath = "claude"
	}
	return &Agent{
		config:  cfg,
		cliPath: cliPath,
	}
}

// Name returns the agent identifier.
func (a *Agent) Name() string { return "claude-code" }

// CLICommand returns the CLI command to execute.
func (a *Agent) CLICommand() string { return a.cliPath }

// IsInstalled returns true if the Claude CLI is available.
func (a *Agent) IsInstalled() bool {
	_, err := exec.LookPath(a.cliPath)
	return err == nil
}

// Version returns the installed Claude CLI version.
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
	return filepath.Join(home, ".claude", "projects")
}

// BuildProjectPath converts a project path to Claude's projects directory format.
// Example: /Users/foo/project -> -Users-foo-project
// The leading dash is preserved to match Claude's directory structure.
func BuildProjectPath(projectPath string) string {
	// Replace path separators with dashes (leading separator becomes leading dash)
	p := strings.ReplaceAll(projectPath, "/", "-")
	p = strings.ReplaceAll(p, "\\", "-")
	// Replace dots with dashes to match Claude's encoding
	p = strings.ReplaceAll(p, ".", "-")
	return p
}

// DiscoverSessions lists sessions for a given project directory.
func (a *Agent) DiscoverSessions(ctx context.Context, projectDir string) ([]agents.SessionInfo, error) {
	sessionDir := a.DefaultSessionDir()
	if sessionDir == "" {
		return nil, nil
	}

	// Sessions are stored in ~/.claude/projects/<project-hash>/*.jsonl
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

		projectPath := filepath.Join(sessionDir, entry.Name())
		files, err := os.ReadDir(projectPath)
		if err != nil {
			continue
		}

		for _, file := range files {
			if filepath.Ext(file.Name()) != ".jsonl" {
				continue
			}

			info, err := file.Info()
			if err != nil {
				continue
			}

			sessions = append(sessions, agents.SessionInfo{
				ID:        file.Name(),
				Agent:     "claude-code",
				Path:      filepath.Join(projectPath, file.Name()),
				CreatedAt: info.ModTime(),
				Size:      info.Size(),
				Project:   entry.Name(),
			})
		}
	}

	return sessions, nil
}

// Run executes a prompt in a new session.
func (a *Agent) Run(ctx context.Context, opts agents.RunOptions) (*agents.RunResult, error) {
	return a.execute(ctx, opts, false)
}

// Resume continues an existing session.
func (a *Agent) Resume(ctx context.Context, sessionID string, opts agents.RunOptions) (*agents.RunResult, error) {
	opts.SessionID = sessionID
	return a.execute(ctx, opts, true)
}

// buildArgs constructs the command-line arguments for a Claude session.
func (a *Agent) buildArgs(opts agents.RunOptions, resume bool) []string {
	prompt := opts.Prompt
	if prompt == "" {
		prompt = defaultPrompt
	}

	var args []string

	// Session handling: --resume for continuing, --session-id for new sessions
	if resume {
		args = append(args, "--resume", opts.SessionID)
	} else {
		args = append(args, "--session-id", opts.SessionID)
	}

	args = append(args, "-p", prompt, "--output-format", "json")

	if a.config.AutoApprove {
		args = append(args, "--dangerously-skip-permissions")
	}

	// Add model flag if configured
	if model, ok := a.config.Options["model"]; ok && model != "" {
		args = append(args, "--model", model)
	}

	// Add config-level extra args
	args = append(args, a.config.ExtraArgs...)

	// Add per-invocation extra args
	args = append(args, opts.ExtraArgs...)

	return args
}

// execute runs the Claude CLI with the given options.
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
	cmd.Stdin = nil // Explicitly close stdin so Claude doesn't wait for input

	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	result := &agents.RunResult{
		SessionID: opts.SessionID,
		Duration:  duration,
		RawJSON:   stdout.Bytes(),
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

	// Parse JSON output if available
	if len(stdout.Bytes()) > 0 {
		var parsed claudeResult
		if jsonErr := json.Unmarshal(stdout.Bytes(), &parsed); jsonErr == nil {
			result.SessionID = parsed.SessionID
			result.Output = parsed.Result
			result.NumTurns = parsed.NumTurns
			result.IsError = parsed.IsError
			result.Errors = parsed.Errors
			result.Cost = &agents.CostMetrics{
				CostUSD: parsed.TotalCostUSD,
			}
			// Use duration from API response if available
			if parsed.DurationMS > 0 {
				result.Duration = time.Duration(parsed.DurationMS) * time.Millisecond
			}

			if parsed.IsError {
				result.Error = &agents.ClassifiedError{
					Class:   agents.ErrorClassUnknown,
					Message: "claude reported error",
					Agent:   "claude-code",
				}
			}
		}
	}

	if err != nil {
		result.Error = err
	}

	return result, err
}

// claudeResult represents the JSON output from claude -p --output-format json.
type claudeResult struct {
	Type         string   `json:"type"`
	Subtype      string   `json:"subtype"`
	TotalCostUSD float64  `json:"total_cost_usd"`
	IsError      bool     `json:"is_error"`
	DurationMS   int64    `json:"duration_ms"`
	DurationAPI  int64    `json:"duration_api_ms"`
	NumTurns     int      `json:"num_turns"`
	Result       string   `json:"result"`
	SessionID    string   `json:"session_id"`
	Errors       []string `json:"errors"`
}
