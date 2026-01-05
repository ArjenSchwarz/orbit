// Package registry provides run registration and discovery for Orbit.
package registry

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRunStatus_MarshalJSON(t *testing.T) {
	tests := []struct {
		status RunStatus
		want   string
	}{
		{StatusRunning, `"running"`},
		{StatusCompleted, `"completed"`},
		{StatusFailed, `"failed"`},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			got, err := json.Marshal(tt.status)
			if err != nil {
				t.Fatalf("Marshal(%q) error: %v", tt.status, err)
			}
			if string(got) != tt.want {
				t.Errorf("Marshal(%q) = %s, want %s", tt.status, got, tt.want)
			}
		})
	}
}

func TestRunStatus_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		input string
		want  RunStatus
	}{
		{`"running"`, StatusRunning},
		{`"completed"`, StatusCompleted},
		{`"failed"`, StatusFailed},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var got RunStatus
			if err := json.Unmarshal([]byte(tt.input), &got); err != nil {
				t.Fatalf("Unmarshal(%s) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("Unmarshal(%s) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPhaseStatus_MarshalJSON(t *testing.T) {
	tests := []struct {
		status PhaseStatus
		want   string
	}{
		{PhaseStatusPending, `"pending"`},
		{PhaseStatusRunning, `"running"`},
		{PhaseStatusCompleted, `"completed"`},
		{PhaseStatusFailed, `"failed"`},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			got, err := json.Marshal(tt.status)
			if err != nil {
				t.Fatalf("Marshal(%q) error: %v", tt.status, err)
			}
			if string(got) != tt.want {
				t.Errorf("Marshal(%q) = %s, want %s", tt.status, got, tt.want)
			}
		})
	}
}

func TestPhaseStatus_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		input string
		want  PhaseStatus
	}{
		{`"pending"`, PhaseStatusPending},
		{`"running"`, PhaseStatusRunning},
		{`"completed"`, PhaseStatusCompleted},
		{`"failed"`, PhaseStatusFailed},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var got PhaseStatus
			if err := json.Unmarshal([]byte(tt.input), &got); err != nil {
				t.Fatalf("Unmarshal(%s) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("Unmarshal(%s) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPhase_JSON(t *testing.T) {
	phase := Phase{
		Number:   1,
		Status:   PhaseStatusCompleted,
		RunCount: 2,
	}

	data, err := json.Marshal(phase)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var got Phase
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if got.Number != phase.Number {
		t.Errorf("Number = %d, want %d", got.Number, phase.Number)
	}
	if got.Status != phase.Status {
		t.Errorf("Status = %q, want %q", got.Status, phase.Status)
	}
	if got.RunCount != phase.RunCount {
		t.Errorf("RunCount = %d, want %d", got.RunCount, phase.RunCount)
	}
}

func TestRunEntry_JSON(t *testing.T) {
	startedAt := time.Date(2025, 1, 5, 10, 30, 0, 0, time.UTC)
	finishedAt := time.Date(2025, 1, 5, 12, 45, 0, 0, time.UTC)
	pid := 12345

	entry := RunEntry{
		ID:            "550e8400-e29b-41d4-a716-446655440000",
		SchemaVersion: 1,
		Name:          "feature-auth",
		Repository:    "ArjenSchwarz/orbit",
		LogDir:        "/Users/arjen/projects/orbit/specs/feature-auth/.orbit",
		Status:        StatusCompleted,
		StartedAt:     startedAt,
		FinishedAt:    &finishedAt,
		Branch:        "feature/auth",
		PID:           &pid,
		Phases: []Phase{
			{Number: 1, Status: PhaseStatusCompleted, RunCount: 1},
			{Number: 2, Status: PhaseStatusCompleted, RunCount: 2},
		},
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var got RunEntry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	// Verify all fields
	if got.ID != entry.ID {
		t.Errorf("ID = %q, want %q", got.ID, entry.ID)
	}
	if got.SchemaVersion != entry.SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", got.SchemaVersion, entry.SchemaVersion)
	}
	if got.Name != entry.Name {
		t.Errorf("Name = %q, want %q", got.Name, entry.Name)
	}
	if got.Repository != entry.Repository {
		t.Errorf("Repository = %q, want %q", got.Repository, entry.Repository)
	}
	if got.LogDir != entry.LogDir {
		t.Errorf("LogDir = %q, want %q", got.LogDir, entry.LogDir)
	}
	if got.Status != entry.Status {
		t.Errorf("Status = %q, want %q", got.Status, entry.Status)
	}
	if !got.StartedAt.Equal(entry.StartedAt) {
		t.Errorf("StartedAt = %v, want %v", got.StartedAt, entry.StartedAt)
	}
	if got.FinishedAt == nil {
		t.Error("FinishedAt is nil, want non-nil")
	} else if !got.FinishedAt.Equal(*entry.FinishedAt) {
		t.Errorf("FinishedAt = %v, want %v", *got.FinishedAt, *entry.FinishedAt)
	}
	if got.Branch != entry.Branch {
		t.Errorf("Branch = %q, want %q", got.Branch, entry.Branch)
	}
	if got.PID == nil {
		t.Error("PID is nil, want non-nil")
	} else if *got.PID != *entry.PID {
		t.Errorf("PID = %d, want %d", *got.PID, *entry.PID)
	}
	if len(got.Phases) != len(entry.Phases) {
		t.Errorf("len(Phases) = %d, want %d", len(got.Phases), len(entry.Phases))
	}
}

func TestRunEntry_OmitEmpty(t *testing.T) {
	// Entry with optional fields unset
	entry := RunEntry{
		ID:            "550e8400-e29b-41d4-a716-446655440000",
		SchemaVersion: 1,
		Name:          "test-run",
		Repository:    "owner/repo",
		LogDir:        "/path/to/logs",
		Status:        StatusRunning,
		StartedAt:     time.Now(),
		Branch:        "main",
		// FinishedAt, PID, and Phases are nil/empty
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	jsonStr := string(data)

	// These fields should be omitted when empty/nil
	if contains(jsonStr, `"finished_at"`) {
		t.Error("finished_at should be omitted when nil")
	}
	if contains(jsonStr, `"pid"`) {
		t.Error("pid should be omitted when nil")
	}
	if contains(jsonStr, `"phases"`) {
		t.Error("phases should be omitted when empty")
	}
}

func TestRunEntry_SchemaVersionDefault(t *testing.T) {
	// When creating a new entry, SchemaVersion should be set to 1
	entry := NewRunEntry()
	if entry.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", entry.SchemaVersion)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
