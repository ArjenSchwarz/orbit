// Package rune provides a client wrapper for the rune CLI task management tool.
package rune

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Status represents a task's completion status.
type Status int

const (
	StatusPending    Status = 0
	StatusInProgress Status = 1
	StatusCompleted  Status = 2
)

// Task represents a task from rune's JSON output.
type Task struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   Status `json:"status"`
	Details  string `json:"details,omitempty"`
	Parent   string `json:"parent,omitempty"`
	Phase    string `json:"phase,omitempty"`
	Subtasks []Task `json:"subtasks,omitempty"`
}

// NextPhaseResult represents the output of rune next --phase.
type NextPhaseResult struct {
	PhaseName             string   `json:"phase_name"`
	Tasks                 []Task   `json:"tasks"`
	FrontMatterReferences []string `json:"front_matter_references,omitempty"`
	AllComplete           bool     `json:"all_complete,omitempty"`
}

// Client wraps the rune CLI for programmatic access.
type Client struct {
	tasksFile string
}

// NewClient creates a new rune client.
func NewClient(tasksFile string) *Client {
	return &Client{
		tasksFile: tasksFile,
	}
}

// TasksFile returns the configured tasks file path.
func (c *Client) TasksFile() string {
	return c.tasksFile
}

// ListPending returns all pending (incomplete) tasks.
func (c *Client) ListPending() ([]Task, error) {
	args := []string{"list", c.tasksFile, "--filter", "pending", "--format", "json"}
	cmd := exec.Command("rune", args...)

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("rune list failed: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("rune list failed: %w", err)
	}

	// Handle empty output (no pending tasks)
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" || trimmed == "[]" || trimmed == "null" {
		return []Task{}, nil
	}

	var tasks []Task
	if err := json.Unmarshal(output, &tasks); err != nil {
		return nil, fmt.Errorf("failed to parse rune output: %w", err)
	}

	return tasks, nil
}

// GetNextPhase returns the next incomplete phase with all its tasks.
func (c *Client) GetNextPhase() (*NextPhaseResult, error) {
	args := []string{"next", c.tasksFile, "--phase", "--format", "json"}
	cmd := exec.Command("rune", args...)

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			// Check if this is "all tasks complete" message
			if strings.Contains(strings.ToLower(stderr), "complete") ||
				strings.Contains(strings.ToLower(stderr), "no tasks") {
				return &NextPhaseResult{AllComplete: true}, nil
			}
			return nil, fmt.Errorf("rune next failed: %s", stderr)
		}
		return nil, fmt.Errorf("rune next failed: %w", err)
	}

	// Handle empty output
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" || trimmed == "null" {
		return &NextPhaseResult{AllComplete: true}, nil
	}

	var result NextPhaseResult
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse rune output: %w", err)
	}

	return &result, nil
}

// HasPendingTasks returns true if there are any incomplete tasks.
func (c *Client) HasPendingTasks() (bool, error) {
	tasks, err := c.ListPending()
	if err != nil {
		return false, err
	}
	return len(tasks) > 0, nil
}

// GetCurrentPhaseNumber extracts the phase number from pending tasks.
// Returns 0 if no phase information is available.
func (c *Client) GetCurrentPhaseNumber() (int, error) {
	result, err := c.GetNextPhase()
	if err != nil {
		return 0, err
	}

	if result.AllComplete || len(result.Tasks) == 0 {
		return 0, nil
	}

	// Extract phase number from phase name (e.g., "Phase 1: Setup" -> 1)
	var phaseNum int
	_, err = fmt.Sscanf(result.PhaseName, "Phase %d", &phaseNum)
	if err != nil {
		// Try alternative format or default to 1
		phaseNum = 1
	}

	return phaseNum, nil
}
