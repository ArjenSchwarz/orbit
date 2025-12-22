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

func TestRunPhase_BuildsCorrectArgs_NewSession(t *testing.T) {
	// Test that RunPhase builds correct arguments when starting a new session (resume=false)
	// Expected: --session-id <uuid> -p <prompt> --output-format json
	config := Config{
		SkipPermissions: false,
		WorkingDir:      "/tmp/test",
		Prompt:          "Test prompt",
	}

	client := NewClient(config)
	args := client.buildRunPhaseArgs("test-session-id-123", false)

	// Check that --session-id comes first with the session ID
	if len(args) < 2 || args[0] != "--session-id" || args[1] != "test-session-id-123" {
		t.Errorf("Expected args to start with [--session-id test-session-id-123], got %v", args[:min(len(args), 4)])
	}

	// Check that -p and prompt are included
	foundPrompt := false
	for i, arg := range args {
		if arg == "-p" && i+1 < len(args) && args[i+1] == "Test prompt" {
			foundPrompt = true
			break
		}
	}
	if !foundPrompt {
		t.Errorf("Expected -p 'Test prompt' in args, got %v", args)
	}

	// Check --output-format json is present
	foundOutputFormat := false
	for i, arg := range args {
		if arg == "--output-format" && i+1 < len(args) && args[i+1] == "json" {
			foundOutputFormat = true
			break
		}
	}
	if !foundOutputFormat {
		t.Errorf("Expected --output-format json in args, got %v", args)
	}

	// Should NOT contain --resume
	for _, arg := range args {
		if arg == "--resume" {
			t.Errorf("New session should not have --resume flag, got %v", args)
		}
	}
}

func TestRunPhase_BuildsCorrectArgs_ResumeSession(t *testing.T) {
	// Test that RunPhase builds correct arguments when resuming a session (resume=true)
	// Expected: --resume <uuid> -p <prompt> --output-format json
	config := Config{
		SkipPermissions: false,
		WorkingDir:      "/tmp/test",
		Prompt:          "Test prompt",
	}

	client := NewClient(config)
	args := client.buildRunPhaseArgs("existing-session-456", true)

	// Check that --resume comes first with the session ID
	if len(args) < 2 || args[0] != "--resume" || args[1] != "existing-session-456" {
		t.Errorf("Expected args to start with [--resume existing-session-456], got %v", args[:min(len(args), 4)])
	}

	// Check that -p and prompt are included
	foundPrompt := false
	for i, arg := range args {
		if arg == "-p" && i+1 < len(args) && args[i+1] == "Test prompt" {
			foundPrompt = true
			break
		}
	}
	if !foundPrompt {
		t.Errorf("Expected -p 'Test prompt' in args, got %v", args)
	}

	// Should NOT contain --session-id
	for _, arg := range args {
		if arg == "--session-id" {
			t.Errorf("Resume session should not have --session-id flag, got %v", args)
		}
	}
}

func TestRunPhase_BuildsCorrectArgs_WithSkipPermissions(t *testing.T) {
	// Test that --dangerously-skip-permissions is added when configured
	config := Config{
		SkipPermissions: true,
		WorkingDir:      "/tmp/test",
		Prompt:          "Test prompt",
	}

	client := NewClient(config)
	args := client.buildRunPhaseArgs("test-session", false)

	foundSkipPerms := false
	for _, arg := range args {
		if arg == "--dangerously-skip-permissions" {
			foundSkipPerms = true
			break
		}
	}
	if !foundSkipPerms {
		t.Errorf("Expected --dangerously-skip-permissions in args, got %v", args)
	}
}

func TestRunPhase_BuildsCorrectArgs_DefaultPrompt(t *testing.T) {
	// Test that default prompt is used when none is configured
	config := Config{
		SkipPermissions: false,
		WorkingDir:      "/tmp/test",
		Prompt:          "", // Empty prompt should use default
	}

	client := NewClient(config)
	args := client.buildRunPhaseArgs("test-session", false)

	defaultPrompt := "Run /next-task --phase and when complete run /commit"
	foundDefaultPrompt := false
	for i, arg := range args {
		if arg == "-p" && i+1 < len(args) && args[i+1] == defaultPrompt {
			foundDefaultPrompt = true
			break
		}
	}
	if !foundDefaultPrompt {
		t.Errorf("Expected default prompt in args, got %v", args)
	}
}

func TestRunPhase_ArgOrder(t *testing.T) {
	// Test that session flag comes before prompt and output format
	config := Config{
		SkipPermissions: true,
		Prompt:          "Test prompt",
	}

	client := NewClient(config)

	tests := []struct {
		name      string
		sessionID string
		resume    bool
		firstFlag string
	}{
		{"new session", "new-id", false, "--session-id"},
		{"resume session", "existing-id", true, "--resume"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := client.buildRunPhaseArgs(tt.sessionID, tt.resume)

			if len(args) < 1 || args[0] != tt.firstFlag {
				t.Errorf("Expected first arg to be %s, got %v", tt.firstFlag, args[0])
			}

			// Find positions of key flags
			var sessionFlagPos, promptPos, outputFormatPos, skipPermsPos = -1, -1, -1, -1
			for i, arg := range args {
				switch arg {
				case tt.firstFlag:
					if sessionFlagPos == -1 {
						sessionFlagPos = i
					}
				case "-p":
					promptPos = i
				case "--output-format":
					outputFormatPos = i
				case "--dangerously-skip-permissions":
					skipPermsPos = i
				}
			}

			// Session flag should come before prompt
			if sessionFlagPos >= promptPos {
				t.Errorf("Session flag should come before -p: session at %d, prompt at %d", sessionFlagPos, promptPos)
			}

			// Prompt should come before output format
			if promptPos >= outputFormatPos {
				t.Errorf("-p should come before --output-format: prompt at %d, output-format at %d", promptPos, outputFormatPos)
			}

			// Skip permissions should be last (if present)
			if skipPermsPos != -1 && skipPermsPos < outputFormatPos {
				t.Errorf("--dangerously-skip-permissions should come after --output-format")
			}
		})
	}
}
