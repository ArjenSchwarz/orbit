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
	ID         string   `json:"ID"`
	Title      string   `json:"Title"`
	Status     Status   `json:"Status"`
	Details    []string `json:"Details,omitempty"`
	References []string `json:"References,omitempty"`
	Reqs       []string `json:"requirements,omitempty"`
	Children   []Task   `json:"Children,omitempty"`
	ParentID   string   `json:"ParentID,omitempty"`
	Phase      string   `json:"Phase,omitempty"`
}

// ListResult represents the wrapper object from rune list --format json.
type ListResult struct {
	Title string `json:"Title"`
	Tasks []Task `json:"Tasks"`
}

// PhaseTask represents a task from rune next --phase JSON output.
// This uses different field names and status format than Task.
type PhaseTask struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Status  string   `json:"status"`
	Details []string `json:"details,omitempty"`
}

// NextPhaseResult represents the output of rune next --phase.
type NextPhaseResult struct {
	PhaseName             string      `json:"phase_name"`
	Tasks                 []PhaseTask `json:"tasks"`
	FrontMatterReferences []string    `json:"front_matter_references,omitempty"`
	AllComplete           bool        `json:"all_complete,omitempty"`
}

// PhaseStatus represents the overall status of a phase.
type PhaseStatus string

const (
	PhaseStatusPending    PhaseStatus = ""
	PhaseStatusInProgress PhaseStatus = "in progress"
	PhaseStatusCompleted  PhaseStatus = "completed"
)

// PhaseSummary contains statistics about a phase.
type PhaseSummary struct {
	Name      string
	Order     int
	Total     int
	Completed int
	Pending   int
	Status    PhaseStatus
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

	var result ListResult
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse rune output: %w", err)
	}

	return result.Tasks, nil
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

// ListAll returns all tasks regardless of status.
func (c *Client) ListAll() ([]Task, error) {
	args := []string{"list", c.tasksFile, "--format", "json"}
	cmd := exec.Command("rune", args...)

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("rune list failed: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("rune list failed: %w", err)
	}

	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" || trimmed == "[]" || trimmed == "null" {
		return []Task{}, nil
	}

	var result ListResult
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse rune output: %w", err)
	}

	return result.Tasks, nil
}

// GetPhaseSummaries returns a summary of all phases with their task counts and status.
func (c *Client) GetPhaseSummaries() ([]PhaseSummary, error) {
	tasks, err := c.ListAll()
	if err != nil {
		return nil, err
	}

	// Group tasks by phase, maintaining order
	phaseOrder := []string{}
	phaseTasks := make(map[string][]Task)

	for _, task := range tasks {
		phase := task.Phase
		if phase == "" {
			phase = "Uncategorized"
		}
		if _, exists := phaseTasks[phase]; !exists {
			phaseOrder = append(phaseOrder, phase)
		}
		phaseTasks[phase] = append(phaseTasks[phase], task)
	}

	// Build summaries
	summaries := make([]PhaseSummary, 0, len(phaseOrder))
	for i, phaseName := range phaseOrder {
		tasks := phaseTasks[phaseName]
		completed := 0
		inProgress := 0
		for _, t := range tasks {
			switch t.Status {
			case StatusCompleted:
				completed++
			case StatusInProgress:
				inProgress++
			}
		}

		var status PhaseStatus
		if completed == len(tasks) {
			status = PhaseStatusCompleted
		} else if inProgress > 0 || completed > 0 {
			status = PhaseStatusInProgress
		} else {
			status = PhaseStatusPending
		}

		summaries = append(summaries, PhaseSummary{
			Name:      phaseName,
			Order:     i + 1,
			Total:     len(tasks),
			Completed: completed,
			Pending:   len(tasks) - completed,
			Status:    status,
		})
	}

	return summaries, nil
}
