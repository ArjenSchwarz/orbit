// Package debug provides debug logging utilities for Orbit.
package debug

import "time"

// LogEntry represents a single structured log entry.
// Satisfies requirements 2.1-2.8
type LogEntry struct {
	Timestamp time.Time      `json:"timestamp"`        // ISO 8601 (Req 2.2)
	Level     string         `json:"level"`            // debug|info|warn|error (Req 2.3)
	Component string         `json:"component"`        // Source identifier (Req 2.5)
	Message   string         `json:"message"`          // Human-readable text (Req 2.4)
	Fields    map[string]any `json:"fields,omitempty"` // Additional structured data (Req 2.6)
}

// StartupEntry is the first entry in a log file.
// Flat struct to control exact JSON output.
// Satisfies requirements 2.7, 5.3
type StartupEntry struct {
	Timestamp        time.Time `json:"timestamp"`
	Level            string    `json:"level"`
	Component        string    `json:"component"`
	Message          string    `json:"message"`
	SchemaVersion    int       `json:"schema_version"` // Always 1 (Req 2.7)
	OrbitVersion     string    `json:"orbit_version"`
	Agent            string    `json:"agent"`
	TasksFile        string    `json:"tasks_file"`
	WorkingDirectory string    `json:"working_directory"`
	BranchName       string    `json:"branch_name"`
}

// ShutdownEntry marks normal completion.
// Flat struct to control exact JSON output.
// Satisfies requirement 5.4
type ShutdownEntry struct {
	Timestamp     time.Time `json:"timestamp"`
	Level         string    `json:"level"`
	Component     string    `json:"component"`
	Message       string    `json:"message"`
	TotalDuration string    `json:"total_duration"`
	FinalStatus   string    `json:"final_status"` // completed|failed|interrupted
}

// StartupConfig provides metadata for the startup log entry.
type StartupConfig struct {
	OrbitVersion     string // Orbit binary version
	Agent            string // Agent name (e.g., "claude-code")
	TasksFile        string // Absolute path to tasks file
	WorkingDirectory string // Absolute path to working directory
	BranchName       string // Current git branch
}
