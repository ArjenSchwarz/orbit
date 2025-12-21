package claude

import (
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	config := Config{
		SkipPermissions: true,
		WorkingDir:      "/tmp/test",
	}

	client := NewClient(config)

	if client == nil {
		t.Fatal("NewClient returned nil")
	}

	if client.config.SkipPermissions != true {
		t.Error("SkipPermissions should be true")
	}
	if client.config.WorkingDir != "/tmp/test" {
		t.Errorf("WorkingDir = %q, want %q", client.config.WorkingDir, "/tmp/test")
	}
}

func TestNewClient_DefaultConfig(t *testing.T) {
	config := Config{}
	client := NewClient(config)

	if client == nil {
		t.Fatal("NewClient returned nil")
	}

	if client.config.SkipPermissions != false {
		t.Error("SkipPermissions should default to false")
	}
	if client.config.WorkingDir != "" {
		t.Error("WorkingDir should default to empty")
	}
}

func TestResult_Struct(t *testing.T) {
	result := Result{
		Type:         "result",
		Subtype:      "success",
		TotalCostUSD: 0.25,
		IsError:      false,
		DurationMS:   45000,
		DurationAPI:  40000,
		NumTurns:     5,
		Result:       "Task completed successfully",
		SessionID:    "session-abc123",
	}

	if result.Type != "result" {
		t.Errorf("Type = %q, want %q", result.Type, "result")
	}
	if result.TotalCostUSD != 0.25 {
		t.Errorf("TotalCostUSD = %f, want 0.25", result.TotalCostUSD)
	}
	if result.IsError {
		t.Error("IsError should be false")
	}
	if result.DurationMS != 45000 {
		t.Errorf("DurationMS = %d, want 45000", result.DurationMS)
	}
	if result.NumTurns != 5 {
		t.Errorf("NumTurns = %d, want 5", result.NumTurns)
	}
}

func TestSessionResult_Struct(t *testing.T) {
	result := SessionResult{
		SessionID: "test-session",
		Cost:      0.15,
		Duration:  30 * time.Second,
		NumTurns:  3,
		Output:    "Test output",
		IsError:   false,
		RawJSON:   []byte(`{"test": true}`),
		Stderr:    "warning message",
	}

	if result.SessionID != "test-session" {
		t.Errorf("SessionID = %q, want %q", result.SessionID, "test-session")
	}
	if result.Cost != 0.15 {
		t.Errorf("Cost = %f, want 0.15", result.Cost)
	}
	if result.Duration != 30*time.Second {
		t.Errorf("Duration = %v, want 30s", result.Duration)
	}
	if result.NumTurns != 3 {
		t.Errorf("NumTurns = %d, want 3", result.NumTurns)
	}
	if result.Output != "Test output" {
		t.Errorf("Output = %q, want %q", result.Output, "Test output")
	}
	if result.IsError {
		t.Error("IsError should be false")
	}
	if string(result.RawJSON) != `{"test": true}` {
		t.Errorf("RawJSON = %q, want %q", result.RawJSON, `{"test": true}`)
	}
	if result.Stderr != "warning message" {
		t.Errorf("Stderr = %q, want %q", result.Stderr, "warning message")
	}
}

func TestConfig_Struct(t *testing.T) {
	config := Config{
		SkipPermissions: true,
		WorkingDir:      "/path/to/project",
		Prompt:          "Custom prompt for testing",
	}

	if !config.SkipPermissions {
		t.Error("SkipPermissions should be true")
	}
	if config.WorkingDir != "/path/to/project" {
		t.Errorf("WorkingDir = %q, want %q", config.WorkingDir, "/path/to/project")
	}
	if config.Prompt != "Custom prompt for testing" {
		t.Errorf("Prompt = %q, want %q", config.Prompt, "Custom prompt for testing")
	}
}

func TestNewClient_WithPrompt(t *testing.T) {
	customPrompt := "Run /custom-command and when complete run /commit"
	config := Config{
		SkipPermissions: true,
		WorkingDir:      "/tmp/test",
		Prompt:          customPrompt,
	}

	client := NewClient(config)

	if client == nil {
		t.Fatal("NewClient returned nil")
	}

	if client.config.Prompt != customPrompt {
		t.Errorf("Prompt = %q, want %q", client.config.Prompt, customPrompt)
	}
}

func TestNewClient_EmptyPrompt(t *testing.T) {
	config := Config{
		SkipPermissions: false,
		WorkingDir:      "/tmp/test",
		Prompt:          "",
	}

	client := NewClient(config)

	if client == nil {
		t.Fatal("NewClient returned nil")
	}

	if client.config.Prompt != "" {
		t.Errorf("Prompt should be empty, got %q", client.config.Prompt)
	}
}
