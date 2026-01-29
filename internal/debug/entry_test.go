package debug

import (
	"encoding/json"
	"testing"
	"time"
)

func TestLogEntryJSON(t *testing.T) {
	ts := time.Date(2025, 1, 28, 12, 5, 30, 0, time.UTC)

	tests := []struct {
		name     string
		entry    LogEntry
		wantKeys []string
		wantJSON map[string]any
	}{
		{
			name: "all fields present",
			entry: LogEntry{
				Timestamp: ts,
				Level:     "info",
				Component: "orchestrator",
				Message:   "Phase started",
				Fields: map[string]any{
					"phase":      1,
					"task_count": 5,
				},
			},
			wantKeys: []string{"timestamp", "level", "component", "message", "fields"},
			wantJSON: map[string]any{
				"timestamp": "2025-01-28T12:05:30Z",
				"level":     "info",
				"component": "orchestrator",
				"message":   "Phase started",
			},
		},
		{
			name: "fields omitempty when nil",
			entry: LogEntry{
				Timestamp: ts,
				Level:     "debug",
				Component: "agent",
				Message:   "Command executed",
				Fields:    nil,
			},
			wantKeys: []string{"timestamp", "level", "component", "message"},
		},
		{
			name: "fields omitempty when empty",
			entry: LogEntry{
				Timestamp: ts,
				Level:     "warn",
				Component: "retry",
				Message:   "Retry attempt",
				Fields:    map[string]any{},
			},
			// Empty map should still be omitted
			wantKeys: []string{"timestamp", "level", "component", "message"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.entry)
			if err != nil {
				t.Fatalf("failed to marshal LogEntry: %v", err)
			}

			var result map[string]any
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatalf("failed to unmarshal JSON: %v", err)
			}

			// Verify expected keys are present
			for _, key := range tt.wantKeys {
				if _, ok := result[key]; !ok {
					t.Errorf("expected key %q not found in JSON output", key)
				}
			}

			// Verify no unexpected keys
			if len(result) != len(tt.wantKeys) {
				t.Errorf("unexpected number of keys: got %d, want %d", len(result), len(tt.wantKeys))
			}

			// Verify specific values if provided
			for key, want := range tt.wantJSON {
				got, ok := result[key]
				if !ok {
					continue // Already checked above
				}
				if got != want {
					t.Errorf("key %q: got %v, want %v", key, got, want)
				}
			}
		})
	}
}

func TestStartupEntryJSON(t *testing.T) {
	ts := time.Date(2025, 1, 28, 12, 5, 30, 0, time.UTC)

	entry := StartupEntry{
		Timestamp:        ts,
		Level:            "info",
		Component:        "orchestrator",
		Message:          "Orchestration started",
		SchemaVersion:    1,
		OrbitVersion:     "0.1.0",
		Agent:            "claude-code",
		TasksFile:        "/Users/user/project/specs/feature/tasks.md",
		WorkingDirectory: "/Users/user/project",
		BranchName:       "feature/my-feature",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("failed to marshal StartupEntry: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	// Verify required fields
	requiredFields := []string{
		"timestamp", "level", "component", "message",
		"schema_version", "orbit_version", "agent",
		"tasks_file", "working_directory", "branch_name",
	}

	for _, field := range requiredFields {
		if _, ok := result[field]; !ok {
			t.Errorf("required field %q not found in StartupEntry JSON", field)
		}
	}

	// Verify schema_version is 1
	if result["schema_version"] != float64(1) {
		t.Errorf("schema_version: got %v, want 1", result["schema_version"])
	}

	// Verify no embedded fields issues (no nested structures)
	if len(result) != len(requiredFields) {
		t.Errorf("StartupEntry has unexpected fields: got %d fields, want %d", len(result), len(requiredFields))
	}

	// Verify timestamp format is ISO 8601
	if result["timestamp"] != "2025-01-28T12:05:30Z" {
		t.Errorf("timestamp format: got %v, want 2025-01-28T12:05:30Z", result["timestamp"])
	}
}

func TestShutdownEntryJSON(t *testing.T) {
	ts := time.Date(2025, 1, 28, 12, 15, 45, 0, time.UTC)

	entry := ShutdownEntry{
		Timestamp:     ts,
		Level:         "info",
		Component:     "orchestrator",
		Message:       "Orchestration completed",
		TotalDuration: "10m15s",
		FinalStatus:   "completed",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("failed to marshal ShutdownEntry: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	// Verify required fields
	requiredFields := []string{
		"timestamp", "level", "component", "message",
		"total_duration", "final_status",
	}

	for _, field := range requiredFields {
		if _, ok := result[field]; !ok {
			t.Errorf("required field %q not found in ShutdownEntry JSON", field)
		}
	}

	// Verify no embedded fields issues
	if len(result) != len(requiredFields) {
		t.Errorf("ShutdownEntry has unexpected fields: got %d fields, want %d", len(result), len(requiredFields))
	}

	// Verify final_status value
	if result["final_status"] != "completed" {
		t.Errorf("final_status: got %v, want completed", result["final_status"])
	}

	// Verify total_duration value
	if result["total_duration"] != "10m15s" {
		t.Errorf("total_duration: got %v, want 10m15s", result["total_duration"])
	}
}

func TestShutdownEntryFinalStatusValues(t *testing.T) {
	ts := time.Now()

	statuses := []string{"completed", "failed", "interrupted"}

	for _, status := range statuses {
		t.Run(status, func(t *testing.T) {
			entry := ShutdownEntry{
				Timestamp:     ts,
				Level:         "info",
				Component:     "orchestrator",
				Message:       "Orchestration completed",
				TotalDuration: "5m",
				FinalStatus:   status,
			}

			data, err := json.Marshal(entry)
			if err != nil {
				t.Fatalf("failed to marshal ShutdownEntry with status %q: %v", status, err)
			}

			var result map[string]any
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatalf("failed to unmarshal JSON: %v", err)
			}

			if result["final_status"] != status {
				t.Errorf("final_status: got %v, want %v", result["final_status"], status)
			}
		})
	}
}

func TestLogEntryFieldsOmitEmpty(t *testing.T) {
	ts := time.Now()

	// Entry with no fields should not have "fields" key
	entry := LogEntry{
		Timestamp: ts,
		Level:     "info",
		Component: "test",
		Message:   "test message",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Check that "fields" is not present in the JSON
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if _, ok := result["fields"]; ok {
		t.Error("fields key should be omitted when nil")
	}
}
