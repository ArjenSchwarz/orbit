// Package claudecode provides the Claude Code agent implementation.
package claudecode

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
)

// Compile-time interface check.
var _ agents.Agent = (*Agent)(nil)

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
// When projectDir is non-empty, only sessions for that project's hashed folder
// are returned. When empty, sessions from all projects are returned.
func (a *Agent) DiscoverSessions(ctx context.Context, projectDir string) ([]agents.SessionInfo, error) {
	sessionDir := a.DefaultSessionDir()
	if sessionDir == "" {
		return nil, nil
	}

	return discoverSessionsIn(ctx, sessionDir, projectDir)
}

// discoverSessionsIn scans sessionDir for Claude session files. When projectDir
// is non-empty, only the matching hashed project folder is scanned; otherwise
// all project folders are scanned. Extracted for testability.
func discoverSessionsIn(_ context.Context, sessionDir, projectDir string) ([]agents.SessionInfo, error) {
	if projectDir != "" {
		// Only scan the single matching project hash folder.
		hashName := BuildProjectPath(projectDir)
		return readProjectSessions(filepath.Join(sessionDir, hashName), hashName)
	}

	// No filter — scan all project directories.
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
		found, err := readProjectSessions(filepath.Join(sessionDir, entry.Name()), entry.Name())
		if err != nil {
			// Skip unreadable project directories and continue scanning others.
			// Log the error so the behaviour is diagnosable (e.g. permission denied).
			log.Printf("[claude-code] skipping project directory %s: %v", entry.Name(), err)
			continue
		}
		sessions = append(sessions, found...)
	}

	return sessions, nil
}

// readProjectSessions reads all .jsonl session files from a single project directory.
func readProjectSessions(projectPath, projectName string) ([]agents.SessionInfo, error) {
	files, err := os.ReadDir(projectPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var sessions []agents.SessionInfo
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
			Project:   projectName,
		})
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

	// Session handling: --resume for continuing, --session-id for new sessions.
	// Omit --session-id when empty to let Claude generate its own ID.
	if resume {
		args = append(args, "--resume", opts.SessionID)
	} else if opts.SessionID != "" {
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
		RawJSON:   execResult.Stdout,
		Stderr:    string(execResult.Stderr),
	}

	// Parse JSON output if available
	if len(execResult.Stdout) > 0 {
		var parsed claudeResult
		if jsonErr := json.Unmarshal(execResult.Stdout, &parsed); jsonErr == nil {
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

	if execResult.Err != nil {
		result.Error = execResult.Err
	}

	return result, execResult.Err
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
