// Package logs provides session log management for Orbit.
package logs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/arjenschwarz/orbit/internal/claude"
)

// SessionEntry records metadata about a completed Claude session.
type SessionEntry struct {
	Phase      int       `json:"phase"`
	SessionID  string    `json:"session_id"`
	DurationMS int64     `json:"duration_ms"`
	CostUSD    float64   `json:"cost_usd"`
	NumTurns   int       `json:"num_turns"`
	StartedAt  time.Time `json:"started_at"`
	EndedAt    time.Time `json:"ended_at"`
	IsError    bool      `json:"is_error,omitempty"`
}

// Summary contains the overall orchestration run summary.
type Summary struct {
	StartedAt       time.Time      `json:"started_at"`
	CompletedAt     *time.Time     `json:"completed_at,omitempty"`
	Status          string         `json:"status"`
	PhasesCompleted int            `json:"phases_completed"`
	TotalCostUSD    float64        `json:"total_cost_usd"`
	TotalDurationMS int64          `json:"total_duration_ms"`
	Sessions        []SessionEntry `json:"sessions"`
	Error           string         `json:"error,omitempty"`
}

// Manager handles log storage and retrieval.
type Manager struct {
	baseDir    string
	sessionDir string
	summary    Summary
}

// NewManager creates a new log manager with a timestamped session directory.
func NewManager(baseDir, branchName string) (*Manager, error) {
	// Create timestamped directory
	timestamp := time.Now().Format("2006-01-02-150405")
	sessionDir := filepath.Join(baseDir, fmt.Sprintf("%s-%s", timestamp, sanitizeName(branchName)))

	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	m := &Manager{
		baseDir:    baseDir,
		sessionDir: sessionDir,
		summary: Summary{
			StartedAt: time.Now(),
			Status:    "running",
			Sessions:  []SessionEntry{},
		},
	}

	// Write initial summary
	if err := m.writeSummary(); err != nil {
		return nil, err
	}

	return m, nil
}

// SessionDir returns the current session directory path.
func (m *Manager) SessionDir() string {
	return m.sessionDir
}

// SaveSession records a completed Claude session.
func (m *Manager) SaveSession(phase int, result *claude.SessionResult, startTime time.Time) error {
	endTime := time.Now()

	entry := SessionEntry{
		Phase:      phase,
		SessionID:  result.SessionID,
		DurationMS: result.Duration.Milliseconds(),
		CostUSD:    result.Cost,
		NumTurns:   result.NumTurns,
		StartedAt:  startTime,
		EndedAt:    endTime,
		IsError:    result.IsError,
	}

	m.summary.Sessions = append(m.summary.Sessions, entry)
	m.summary.PhasesCompleted = phase
	m.summary.TotalCostUSD += result.Cost
	m.summary.TotalDurationMS += result.Duration.Milliseconds()

	// Write session JSON
	jsonPath := filepath.Join(m.sessionDir, fmt.Sprintf("phase-%d-session.json", phase))
	if err := os.WriteFile(jsonPath, result.RawJSON, 0644); err != nil {
		return fmt.Errorf("failed to write session JSON: %w", err)
	}

	// Write human-readable transcript
	txtPath := filepath.Join(m.sessionDir, fmt.Sprintf("phase-%d-session.txt", phase))
	transcript := formatTranscript(phase, result, startTime, endTime)
	if err := os.WriteFile(txtPath, []byte(transcript), 0644); err != nil {
		return fmt.Errorf("failed to write session transcript: %w", err)
	}

	// Update summary
	return m.writeSummary()
}

// Complete marks the orchestration run as complete.
func (m *Manager) Complete() error {
	now := time.Now()
	m.summary.CompletedAt = &now
	m.summary.Status = "success"
	return m.writeSummary()
}

// Fail marks the orchestration run as failed with an error message.
func (m *Manager) Fail(err error) error {
	now := time.Now()
	m.summary.CompletedAt = &now
	m.summary.Status = "failed"
	m.summary.Error = err.Error()
	return m.writeSummary()
}

// writeSummary writes the current summary to disk.
func (m *Manager) writeSummary() error {
	data, err := json.MarshalIndent(m.summary, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal summary: %w", err)
	}

	path := filepath.Join(m.sessionDir, "summary.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write summary: %w", err)
	}

	return nil
}

// formatTranscript creates a human-readable session transcript.
func formatTranscript(phase int, result *claude.SessionResult, start, end time.Time) string {
	return fmt.Sprintf(`Orbit Session Log - Phase %d
========================================

Session ID: %s
Started:    %s
Ended:      %s
Duration:   %s
Cost:       $%.4f
Turns:      %d
Error:      %v

Output:
----------------------------------------
%s

Stderr:
----------------------------------------
%s
`,
		phase,
		result.SessionID,
		start.Format(time.RFC3339),
		end.Format(time.RFC3339),
		result.Duration.String(),
		result.Cost,
		result.NumTurns,
		result.IsError,
		result.Output,
		result.Stderr,
	)
}

// sanitizeName replaces characters that are invalid in filenames.
func sanitizeName(name string) string {
	result := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			result = append(result, c)
		} else if c == '/' {
			result = append(result, '-')
		}
	}
	return string(result)
}
