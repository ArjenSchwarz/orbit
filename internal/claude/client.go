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

// RunPhase executes a Claude session to run the next phase and commit.
func (c *Client) RunPhase() (*SessionResult, error) {
	// Prompt instructs Claude to run the phase workflow
	prompt := "Run /next-task --phase and when complete run /commit"

	args := []string{"-p", prompt, "--output-format", "json"}
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
func (c *Client) RunCustomPrompt(prompt string) (*SessionResult, error) {
	args := []string{"-p", prompt, "--output-format", "json"}
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
