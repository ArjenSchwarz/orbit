package kiro

import (
	"context"
	"slices"
	"testing"

	"github.com/arjenschwarz/orbit/internal/agents"
)

func TestNew(t *testing.T) {
	cfg := agents.AgentConfig{
		CLIPath:     "/custom/path/kiro-cli",
		AutoApprove: true,
	}

	agent := New(cfg)
	if agent == nil {
		t.Fatal("New() returned nil")
	}

	// Check that it implements the Agent interface
	var _ agents.Agent = agent //nolint:staticcheck // explicit interface check
}

func TestAgent_Name(t *testing.T) {
	agent := New(agents.AgentConfig{})
	if name := agent.Name(); name != "kiro" {
		t.Errorf("Name() = %q, want %q", name, "kiro")
	}
}

func TestAgent_CLICommand(t *testing.T) {
	tests := []struct {
		name     string
		cliPath  string
		expected string
	}{
		{
			name:     "default CLI path",
			cliPath:  "",
			expected: "kiro-cli",
		},
		{
			name:     "custom CLI path",
			cliPath:  "/usr/local/bin/kiro-cli",
			expected: "/usr/local/bin/kiro-cli",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := New(agents.AgentConfig{CLIPath: tt.cliPath})
			if cmd := agent.CLICommand(); cmd != tt.expected {
				t.Errorf("CLICommand() = %q, want %q", cmd, tt.expected)
			}
		})
	}
}

func TestAgent_DefaultSessionDir(t *testing.T) {
	agent := New(agents.AgentConfig{})
	dir := agent.DefaultSessionDir()

	// Kiro does not store logs automatically per Decision 7
	// DefaultSessionDir should return empty string
	if dir != "" {
		t.Errorf("DefaultSessionDir() = %q, want empty string (Kiro doesn't store logs automatically)", dir)
	}
}

func TestAgent_ImplementsSessionExporter(t *testing.T) {
	agent := New(agents.AgentConfig{})

	// Kiro implements SessionExporter interface
	exporter, ok := agent.(agents.SessionExporter)
	if !ok {
		t.Fatal("Kiro agent should implement SessionExporter interface")
	}
	if exporter == nil {
		t.Fatal("SessionExporter is nil")
	}
}

func TestAgent_BuildArgs_NewSession(t *testing.T) {
	agent := New(agents.AgentConfig{
		AutoApprove: false,
	}).(*Agent)

	args := agent.buildArgs(agents.RunOptions{
		Prompt:    "Test prompt",
		SessionID: "test-session-id-123",
	}, false)

	// Should start with "chat"
	if len(args) < 1 || args[0] != "chat" {
		t.Errorf("Expected args to start with 'chat', got %v", args)
	}

	// Should have --no-interactive
	if !slices.Contains(args, "--no-interactive") {
		t.Errorf("Expected --no-interactive in args, got %v", args)
	}

	// Check that the prompt is included
	if !slices.Contains(args, "Test prompt") {
		t.Errorf("Expected 'Test prompt' in args, got %v", args)
	}

	// Should NOT contain --trust-all-tools when AutoApprove is false
	if slices.Contains(args, "--trust-all-tools") {
		t.Errorf("Should not have --trust-all-tools without AutoApprove, got %v", args)
	}

	// Should NOT contain --resume
	if slices.Contains(args, "--resume") {
		t.Errorf("New session should not have --resume flag, got %v", args)
	}
}

func TestAgent_BuildArgs_ResumeSession(t *testing.T) {
	agent := New(agents.AgentConfig{
		AutoApprove: false,
	}).(*Agent)

	args := agent.buildArgs(agents.RunOptions{
		Prompt:    "Test prompt",
		SessionID: "existing-session-456",
	}, true)

	// Should start with "chat"
	if len(args) < 1 || args[0] != "chat" {
		t.Errorf("Expected args to start with 'chat', got %v", args)
	}

	// Should have --no-interactive
	if !slices.Contains(args, "--no-interactive") {
		t.Errorf("Expected --no-interactive in args, got %v", args)
	}

	// Check that --resume is present
	if !slices.Contains(args, "--resume") {
		t.Errorf("Resume should have --resume flag, got %v", args)
	}

	// Check that the prompt is included
	if !slices.Contains(args, "Test prompt") {
		t.Errorf("Expected 'Test prompt' in args, got %v", args)
	}
}

func TestAgent_BuildArgs_WithAutoApprove(t *testing.T) {
	agent := New(agents.AgentConfig{
		AutoApprove: true,
	}).(*Agent)

	args := agent.buildArgs(agents.RunOptions{
		Prompt:    "Test prompt",
		SessionID: "test-session",
	}, false)

	if !slices.Contains(args, "--trust-all-tools") {
		t.Errorf("Expected --trust-all-tools in args, got %v", args)
	}
}

func TestAgent_BuildArgs_WithExtraArgs(t *testing.T) {
	agent := New(agents.AgentConfig{
		ExtraArgs: []string{"--verbose", "--model", "auto"},
	}).(*Agent)

	args := agent.buildArgs(agents.RunOptions{
		Prompt:    "Test prompt",
		SessionID: "test-session",
	}, false)

	if !slices.Contains(args, "--verbose") {
		t.Errorf("Expected --verbose in args, got %v", args)
	}
	if !slices.Contains(args, "--model") {
		t.Errorf("Expected --model in args, got %v", args)
	}
}

func TestAgent_BuildArgs_WithOptsExtraArgs(t *testing.T) {
	agent := New(agents.AgentConfig{}).(*Agent)

	args := agent.buildArgs(agents.RunOptions{
		Prompt:    "Test prompt",
		SessionID: "test-session",
		ExtraArgs: []string{"--custom-flag"},
	}, false)

	if !slices.Contains(args, "--custom-flag") {
		t.Errorf("Expected --custom-flag in args, got %v", args)
	}
}

func TestAgent_DefaultPrompt(t *testing.T) {
	agent := New(agents.AgentConfig{}).(*Agent)

	args := agent.buildArgs(agents.RunOptions{
		Prompt:    "", // Empty prompt should use default
		SessionID: "test-session",
	}, false)

	if !slices.Contains(args, defaultPrompt) {
		t.Errorf("Expected default prompt %q in args, got %v", defaultPrompt, args)
	}
}

func TestAgent_RegisteredInInit(t *testing.T) {
	// Verify the agent is registered in the registry
	agent, err := agents.Get("kiro", agents.AgentConfig{})
	if err != nil {
		t.Fatalf("agents.Get(\"kiro\") error = %v", err)
	}
	if agent == nil {
		t.Fatal("agents.Get(\"kiro\") returned nil")
	}
	if agent.Name() != "kiro" {
		t.Errorf("agent.Name() = %q, want %q", agent.Name(), "kiro")
	}
}

func TestAgent_DiscoverSessions(t *testing.T) {
	agent := New(agents.AgentConfig{})

	// Kiro doesn't have automatic session storage
	// DiscoverSessions should return nil, nil
	sessions, err := agent.DiscoverSessions(context.Background(), "/any/path")
	if err != nil {
		t.Errorf("DiscoverSessions() error = %v", err)
	}
	if sessions != nil {
		t.Errorf("DiscoverSessions() should return nil (Kiro doesn't store sessions automatically), got %v", sessions)
	}
}

func TestAgent_Version(t *testing.T) {
	agent := New(agents.AgentConfig{})

	// Version may return error if kiro-cli is not installed
	// We just verify it doesn't panic
	_, _ = agent.Version()
}

func TestAgent_ArgOrder(t *testing.T) {
	agent := New(agents.AgentConfig{
		AutoApprove: true,
	}).(*Agent)

	tests := []struct {
		name      string
		sessionID string
		resume    bool
	}{
		{"new session", "new-id", false},
		{"resume session", "existing-id", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := agent.buildArgs(agents.RunOptions{
				Prompt:    "Test prompt",
				SessionID: tt.sessionID,
			}, tt.resume)

			// First arg should always be "chat"
			if len(args) < 1 || args[0] != "chat" {
				t.Errorf("Expected first arg to be 'chat', got %v", args)
			}

			// --no-interactive should come early
			noInteractivePos := -1
			trustToolsPos := -1
			promptPos := -1
			for i, arg := range args {
				if arg == "--no-interactive" {
					noInteractivePos = i
				}
				if arg == "--trust-all-tools" {
					trustToolsPos = i
				}
				if arg == "Test prompt" {
					promptPos = i
				}
			}

			if noInteractivePos == -1 {
				t.Error("Expected --no-interactive in args")
			}

			if trustToolsPos != -1 && promptPos != -1 && trustToolsPos >= promptPos {
				t.Errorf("--trust-all-tools should come before prompt: --trust-all-tools at %d, prompt at %d", trustToolsPos, promptPos)
			}
		})
	}
}

func TestAgent_ExportSession_BuildsCorrectArgs(t *testing.T) {
	agent := New(agents.AgentConfig{}).(*Agent)

	// Test the export session args building
	args := agent.buildExportArgs("test-output.json")

	// Should have chat command
	if len(args) < 1 || args[0] != "chat" {
		t.Errorf("Expected args to start with 'chat', got %v", args)
	}

	// Should have --no-interactive
	if !slices.Contains(args, "--no-interactive") {
		t.Errorf("Expected --no-interactive in args, got %v", args)
	}

	// Should have --resume
	if !slices.Contains(args, "--resume") {
		t.Errorf("Expected --resume in args, got %v", args)
	}

	// Should NOT have --trust-all-tools when AutoApprove is false
	if slices.Contains(args, "--trust-all-tools") {
		t.Errorf("Should not have --trust-all-tools without AutoApprove, got %v", args)
	}

	// Should have the save command with quoted filename
	foundSaveCmd := false
	for _, arg := range args {
		if arg == `/chat save "test-output.json"` {
			foundSaveCmd = true
			break
		}
	}
	if !foundSaveCmd {
		t.Errorf(`Expected '/chat save "test-output.json"' in args, got %v`, args)
	}
}

func TestAgent_ExportSession_WithAutoApprove(t *testing.T) {
	agent := New(agents.AgentConfig{
		AutoApprove: true,
	}).(*Agent)

	args := agent.buildExportArgs("test-output.json")

	// Should have --trust-all-tools when AutoApprove is true
	if !slices.Contains(args, "--trust-all-tools") {
		t.Errorf("Expected --trust-all-tools in args with AutoApprove, got %v", args)
	}

	// Should still have all required args
	if !slices.Contains(args, "--no-interactive") {
		t.Errorf("Expected --no-interactive in args, got %v", args)
	}
	if !slices.Contains(args, "--resume") {
		t.Errorf("Expected --resume in args, got %v", args)
	}
}
