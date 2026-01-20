// Package rune provides a client wrapper for the rune CLI task management tool.
package rune

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/arjenschwarz/orbit/internal/debug"
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
	debug     *debug.Logger
}

// NewClient creates a new rune client.
func NewClient(tasksFile string) *Client {
	return &Client{
		tasksFile: tasksFile,
		debug:     debug.New(false, "rune"),
	}
}

// SetDebug enables or disables debug logging.
func (c *Client) SetDebug(enabled bool) {
	c.debug = debug.New(enabled, "rune")
}

// TasksFile returns the configured tasks file path.
func (c *Client) TasksFile() string {
	return c.tasksFile
}

// ListPending returns all pending (incomplete) tasks.
func (c *Client) ListPending() ([]Task, error) {
	args := []string{"list", c.tasksFile, "--filter", "pending", "--format", "json"}

	c.debug.LogCmd("rune", args, "")

	startTime := time.Now()
	cmd := exec.Command("rune", args...)
	output, err := cmd.Output()
	duration := time.Since(startTime)

	if err != nil {
		exitCode := -1
		stderr := ""
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			stderr = string(exitErr.Stderr)
		}
		c.debug.LogCmdResult(exitCode, string(output), stderr, duration)
		c.debug.Log("ListPending failed: %v", err)
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("rune list failed: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("rune list failed: %w", err)
	}

	c.debug.LogCmdResult(0, string(output), "", duration)

	// Handle empty output (no pending tasks)
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" || trimmed == "[]" || trimmed == "null" {
		c.debug.Log("ListPending: no pending tasks (empty output)")
		return []Task{}, nil
	}

	var result ListResult
	if err := json.Unmarshal(output, &result); err != nil {
		c.debug.LogJSON(false, err)
		return nil, fmt.Errorf("failed to parse rune output: %w", err)
	}

	c.debug.LogJSON(true, nil)
	c.debug.Log("ListPending: found %d pending tasks", len(result.Tasks))

	return result.Tasks, nil
}

// GetNextPhase returns the next incomplete phase with all its tasks.
func (c *Client) GetNextPhase() (*NextPhaseResult, error) {
	args := []string{"next", c.tasksFile, "--phase", "--format", "json"}

	c.debug.LogCmd("rune", args, "")

	startTime := time.Now()
	cmd := exec.Command("rune", args...)
	output, err := cmd.Output()
	duration := time.Since(startTime)

	if err != nil {
		exitCode := -1
		stderr := ""
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			stderr = string(exitErr.Stderr)
		}
		c.debug.LogCmdResult(exitCode, string(output), stderr, duration)

		if exitErr, ok := err.(*exec.ExitError); ok {
			stderrStr := string(exitErr.Stderr)
			// Check if this is "all tasks complete" message
			if strings.Contains(strings.ToLower(stderrStr), "complete") ||
				strings.Contains(strings.ToLower(stderrStr), "no tasks") {
				c.debug.Log("GetNextPhase: all tasks complete (detected from stderr)")
				return &NextPhaseResult{AllComplete: true}, nil
			}
			c.debug.Log("GetNextPhase failed: %s", stderrStr)
			return nil, fmt.Errorf("rune next failed: %s", stderrStr)
		}
		c.debug.Log("GetNextPhase failed: %v", err)
		return nil, fmt.Errorf("rune next failed: %w", err)
	}

	c.debug.LogCmdResult(0, string(output), "", duration)

	// Handle empty output
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" || trimmed == "null" {
		c.debug.Log("GetNextPhase: all tasks complete (empty output)")
		return &NextPhaseResult{AllComplete: true}, nil
	}

	var result NextPhaseResult
	if err := json.Unmarshal(output, &result); err != nil {
		c.debug.LogJSON(false, err)
		return nil, fmt.Errorf("failed to parse rune output: %w", err)
	}

	c.debug.LogJSON(true, nil)
	c.debug.Log("GetNextPhase: phase=%s tasks=%d", result.PhaseName, len(result.Tasks))

	// Check if all tasks in the phase are completed
	// rune might return a phase even when all its tasks are done
	if len(result.Tasks) == 0 {
		c.debug.Log("GetNextPhase: no tasks in phase, marking all complete")
		result.AllComplete = true
	} else {
		allDone := true
		for _, task := range result.Tasks {
			if task.Status != "completed" && task.Status != "done" {
				allDone = false
				break
			}
		}
		if allDone {
			c.debug.Log("GetNextPhase: all %d tasks in phase are completed", len(result.Tasks))
			result.AllComplete = true
		}
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

	c.debug.LogCmd("rune", args, "")

	startTime := time.Now()
	cmd := exec.Command("rune", args...)
	output, err := cmd.Output()
	duration := time.Since(startTime)

	if err != nil {
		exitCode := -1
		stderr := ""
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			stderr = string(exitErr.Stderr)
		}
		c.debug.LogCmdResult(exitCode, string(output), stderr, duration)
		c.debug.Log("ListAll failed: %v", err)

		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("rune list failed: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("rune list failed: %w", err)
	}

	c.debug.LogCmdResult(0, string(output), "", duration)

	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" || trimmed == "[]" || trimmed == "null" {
		c.debug.Log("ListAll: no tasks (empty output)")
		return []Task{}, nil
	}

	var result ListResult
	if err := json.Unmarshal(output, &result); err != nil {
		c.debug.LogJSON(false, err)
		return nil, fmt.Errorf("failed to parse rune output: %w", err)
	}

	c.debug.LogJSON(true, nil)
	c.debug.Log("ListAll: found %d tasks", len(result.Tasks))

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
