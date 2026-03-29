// Package opencode provides the OpenCode agent implementation.
package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
)

// Compile-time interface check.
var _ agents.Agent = (*Agent)(nil)

const (
	defaultPrompt = "Run /next-task --phase and when complete run /commit"

	// unixMillisecondThreshold is used to distinguish between unix timestamps
	// in seconds vs milliseconds. Timestamps greater than this are milliseconds.
	unixMillisecondThreshold = 1_000_000_000_000
)

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
		Created json.RawMessage `json:"created"`
	} `json:"time"`
}

// parseCreatedTime parses the created time from various formats that OpenCode may use.
// Supports RFC3339, RFC3339Nano, unix seconds, and unix milliseconds.
func parseCreatedTime(raw json.RawMessage, fallback time.Time) time.Time {
	if len(raw) == 0 {
		return fallback
	}

	// Try to parse as a string (RFC3339/RFC3339Nano or numeric string)
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		// Try RFC3339Nano first (more precise)
		if t, err := time.Parse(time.RFC3339Nano, str); err == nil {
			return t
		}
		// Try RFC3339
		if t, err := time.Parse(time.RFC3339, str); err == nil {
			return t
		}
		// Try as numeric string (unix timestamp)
		if unix, err := strconv.ParseInt(str, 10, 64); err == nil {
			if t := unixToTime(unix); !t.IsZero() {
				return t
			}
			return fallback
		}
	}

	// Try to parse as a number (unix timestamp)
	var num float64
	if err := json.Unmarshal(raw, &num); err == nil {
		if t := unixToTime(int64(num)); !t.IsZero() {
			return t
		}
		return fallback
	}

	return fallback
}

// unixToTime converts a unix timestamp to time.Time.
// Automatically handles seconds vs milliseconds based on magnitude.
func unixToTime(value int64) time.Time {
	switch {
	case value > unixMillisecondThreshold:
		// Milliseconds
		return time.Unix(0, value*int64(time.Millisecond))
	case value > 0:
		// Seconds
		return time.Unix(value, 0)
	default:
		return time.Time{}
	}
}

// DiscoverSessions lists sessions for a given project directory.
func (a *Agent) DiscoverSessions(ctx context.Context, projectDir string) ([]agents.SessionInfo, error) {
	sessionDir := a.DefaultSessionDir()
	if sessionDir == "" {
		return nil, nil
	}

	return discoverSessionsIn(ctx, sessionDir)
}

// discoverSessionsIn scans sessionDir for OpenCode session subdirectories and
// returns session metadata for each. Extracted from DiscoverSessions to allow
// testing with arbitrary directories.
func discoverSessionsIn(ctx context.Context, sessionDir string) ([]agents.SessionInfo, error) {
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
		// Check for context cancellation during directory traversal
		select {
		case <-ctx.Done():
			return sessions, ctx.Err()
		default:
		}

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

		info, _ := entry.Info()
		var size int64
		var modTime time.Time
		if info != nil {
			size = info.Size()
			modTime = info.ModTime()
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

			createdAt = parseCreatedTime(msg.Time.Created, modTime)
			break // Only need the first message for metadata
		}

		// Fall back to directory modTime when no msg_ file provided a usable timestamp.
		if createdAt.IsZero() {
			createdAt = modTime
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

	// Resume with --session <id> for specific sessions or --continue for most recent
	if resume {
		if strings.HasPrefix(opts.SessionID, "ses_") {
			args = append(args, "--session", opts.SessionID)
		} else {
			args = append(args, "--continue")
		}
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

	execResult := agents.Execute(ctx, agents.ExecuteConfig{
		CLIPath: a.cliPath,
		Args:    args,
		WorkDir: opts.WorkDir,
		Env:     opts.Env,
		Timeout: opts.Timeout,
	})

	raw := execResult.Stdout
	result := &agents.RunResult{
		SessionID: opts.SessionID,
		Duration:  execResult.Duration,
		ExitCode:  execResult.ExitCode,
		Output:    string(raw),
		Stderr:    string(execResult.Stderr),
		RawJSON:   raw,
	}

	// OpenCode may exit with code 0 even on errors.
	// In JSON mode, empty stdout or invalid JSON indicates an error.
	if !isValidJSON(raw) {
		result.IsError = true
		if len(raw) == 0 {
			result.Errors = append(result.Errors, "empty output (expected JSON)")
		} else {
			preview := string(raw)
			if len(preview) > 100 {
				preview = preview[:100] + "..."
			}
			result.Errors = append(result.Errors, "output is not valid JSON: "+preview)
		}
	}

	if execResult.Err != nil {
		result.Error = execResult.Err
	}

	return result, execResult.Err
}

// isValidJSON checks if the byte slice is valid JSON using json.Valid().
func isValidJSON(data []byte) bool {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return false
	}
	return json.Valid(data)
}
