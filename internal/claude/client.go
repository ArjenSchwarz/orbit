// Package claude provides a client wrapper for the Claude Code CLI.
package claude

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"time"
)

// Result represents the JSON output from claude -p --output-format json.
type Result struct {
	Type         string  `json:"type"`
	Subtype      string  `json:"subtype"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	IsError      bool    `json:"is_error"`
	DurationMS   int64   `json:"duration_ms"`
	DurationAPI  int64   `json:"duration_api_ms"`
	NumTurns     int     `json:"num_turns"`
	Result       string  `json:"result"`
	SessionID    string  `json:"session_id"`
}

// SessionResult contains the results of a Claude session.
type SessionResult struct {
	SessionID string
	Cost      float64
	Duration  time.Duration
	NumTurns  int
	Output    string
	IsError   bool
	RawJSON   []byte
	Stderr    string
}

// Config holds client configuration.
type Config struct {
	SkipPermissions bool
	WorkingDir      string
	Prompt          string // Prompt for phase execution
}

// Client wraps the Claude Code CLI for programmatic access.
type Client struct {
	config Config
}

// NewClient creates a new Claude client.
func NewClient(config Config) *Client {
	return &Client{
		config: config,
	}
}

// buildRunPhaseArgs constructs the command-line arguments for a Claude session.
// - sessionID: UUID for this session (required)
// - resume: if true, use --resume <id>; if false, use --session-id <id>
func (c *Client) buildRunPhaseArgs(sessionID string, resume bool) []string {
	prompt := c.config.Prompt
	if prompt == "" {
		prompt = "Run /next-task --phase and when complete run /commit"
	}

	args := []string{}

	// Session handling: --resume for continuing, --session-id for new sessions
	if resume {
		args = append(args, "--resume", sessionID)
	} else {
		args = append(args, "--session-id", sessionID)
	}

	args = append(args, "-p", prompt, "--output-format", "json")

	if c.config.SkipPermissions {
		args = append(args, "--dangerously-skip-permissions")
	}

	return args
}

// RunPhase executes a Claude session to run the next phase and commit.
// - sessionID: UUID for this session (required)
// - resume: if true, use --resume <id>; if false, use --session-id <id>
func (c *Client) RunPhase(sessionID string, resume bool) (*SessionResult, error) {
	args := c.buildRunPhaseArgs(sessionID, resume)

	cmd := exec.Command("claude", args...)
	if c.config.WorkingDir != "" {
		cmd.Dir = c.config.WorkingDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	result := &SessionResult{
		RawJSON: stdout.Bytes(),
		Stderr:  stderr.String(),
	}

	// Parse JSON output if available
	if len(stdout.Bytes()) > 0 {
		var parsed Result
		if jsonErr := json.Unmarshal(stdout.Bytes(), &parsed); jsonErr == nil {
			result.SessionID = parsed.SessionID
			result.Cost = parsed.TotalCostUSD
			result.Duration = time.Duration(parsed.DurationMS) * time.Millisecond
			result.NumTurns = parsed.NumTurns
			result.Output = parsed.Result
			result.IsError = parsed.IsError
		}
	}

	if err != nil {
		// Return the result with error information for classification
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.IsError = true
			if result.Stderr == "" {
				result.Stderr = string(exitErr.Stderr)
			}
		}
		return result, err
	}

	return result, nil
}

// RunCustomPrompt executes a Claude session with a custom prompt.
// This is a convenience wrapper that starts a new session without session tracking.
func (c *Client) RunCustomPrompt(prompt string) (*SessionResult, error) {
	return c.RunCustomPromptWithSession(prompt, "", false)
}

// RunCustomPromptWithSession executes a Claude session with a custom prompt and optional session tracking.
// - sessionID: UUID for this session (empty string to let Claude generate one)
// - resume: if true, use --resume <id>; if false, use --session-id <id> (ignored if sessionID is empty)
func (c *Client) RunCustomPromptWithSession(prompt, sessionID string, resume bool) (*SessionResult, error) {
	var args []string

	// Session handling: --resume for continuing, --session-id for new sessions
	if sessionID != "" {
		if resume {
			args = append(args, "--resume", sessionID)
		} else {
			args = append(args, "--session-id", sessionID)
		}
	}

	args = append(args, "-p", prompt, "--output-format", "json")
	if c.config.SkipPermissions {
		args = append(args, "--dangerously-skip-permissions")
	}

	cmd := exec.Command("claude", args...)
	if c.config.WorkingDir != "" {
		cmd.Dir = c.config.WorkingDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	result := &SessionResult{
		RawJSON: stdout.Bytes(),
		Stderr:  stderr.String(),
	}

	// Parse JSON output if available
	if len(stdout.Bytes()) > 0 {
		var parsed Result
		if jsonErr := json.Unmarshal(stdout.Bytes(), &parsed); jsonErr == nil {
			result.SessionID = parsed.SessionID
			result.Cost = parsed.TotalCostUSD
			result.Duration = time.Duration(parsed.DurationMS) * time.Millisecond
			result.NumTurns = parsed.NumTurns
			result.Output = parsed.Result
			result.IsError = parsed.IsError
		}
	}

	if err != nil {
		result.IsError = true
		if exitErr, ok := err.(*exec.ExitError); ok {
			if result.Stderr == "" {
				result.Stderr = string(exitErr.Stderr)
			}
		}
		return result, err
	}

	return result, nil
}
