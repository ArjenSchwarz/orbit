// Package registry provides run registration and discovery for Orbit.
package registry

import (
	"time"

	"github.com/google/uuid"
)

// RunStatus represents the state of a run.
type RunStatus string

const (
	StatusRunning   RunStatus = "running"
	StatusCompleted RunStatus = "completed"
	StatusFailed    RunStatus = "failed"
)

// PhaseStatus represents the state of a phase within a run.
type PhaseStatus string

const (
	PhaseStatusPending   PhaseStatus = "pending"
	PhaseStatusRunning   PhaseStatus = "running"
	PhaseStatusCompleted PhaseStatus = "completed"
	PhaseStatusFailed    PhaseStatus = "failed"
)

// Phase represents a single phase within a run.
type Phase struct {
	Number   int         `json:"number"`
	Status   PhaseStatus `json:"status"`
	RunCount int         `json:"run_count"`
}

// RunEntry represents a registered run in the registry.
type RunEntry struct {
	ID            string     `json:"id"`
	SchemaVersion int        `json:"schema_version"`
	Name          string     `json:"name"`
	Repository    string     `json:"repository"`
	LogDir        string     `json:"log_dir"`
	Status        RunStatus  `json:"status"`
	StartedAt     time.Time  `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	Branch        string     `json:"branch"`
	PID           *int       `json:"pid,omitempty"`
	Phases        []Phase    `json:"phases,omitempty"`
}

// NewRunEntry creates a new RunEntry with default values.
// Sets SchemaVersion to 1 and generates a new UUID for ID.
func NewRunEntry() *RunEntry {
	return &RunEntry{
		ID:            uuid.NewString(),
		SchemaVersion: 1,
	}
}
