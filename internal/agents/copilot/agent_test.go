package copilot

import (
	"context"
	"slices"
	"testing"

	"github.com/arjenschwarz/orbit/internal/agents"
)

func TestNew(t *testing.T) {
	cfg := agents.AgentConfig{
		CLIPath:     "/custom/path/copilot",
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
	if name := agent.Name(); name != "copilot" {
		t.Errorf("Name() = %q, want %q", name, "copilot")
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
			expected: "copilot",
		},
		{
			name:     "custom CLI path",
			cliPath:  "/usr/local/bin/copilot",
			expected: "/usr/local/bin/copilot",
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

	// Should contain .copilot/session-state
	if dir == "" {
		t.Error("DefaultSessionDir() returned empty string")
	}
	t.Logf("DefaultSessionDir() = %q", dir)
}

func TestAgent_BuildArgs_NewSession(t *testing.T) {
	agent := New(agents.AgentConfig{
		AutoApprove: false,
	}).(*Agent)

	args := agent.buildArgs(agents.RunOptions{
		Prompt:    "Test prompt",
		SessionID: "test-session-id-123",
	}, false)

	// Should have -p flag with prompt
	pIdx := slices.Index(args, "-p")
	if pIdx == -1 {
		t.Errorf("Expected -p flag in args, got %v", args)
	} else if pIdx+1 >= len(args) || args[pIdx+1] != "Test prompt" {
		t.Errorf("Expected prompt after -p flag, got %v", args)
	}

	// Should NOT contain --yolo when AutoApprove is false
	if slices.Contains(args, "--yolo") {
		t.Errorf("Should not have --yolo without AutoApprove, got %v", args)
	}

	// Should NOT contain --continue
	if slices.Contains(args, "--continue") {
		t.Errorf("New session should not have --continue flag, got %v", args)
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

	// Check that --continue is present for resume
	// Note: sessionID is ignored per Known Limitation - Copilot only supports "most recent" resume
	if !slices.Contains(args, "--continue") {
		t.Errorf("Resume should have --continue flag, got %v", args)
	}

	// Should have -p flag with prompt
	pIdx := slices.Index(args, "-p")
	if pIdx == -1 {
		t.Errorf("Expected -p flag in args, got %v", args)
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

	if !slices.Contains(args, "--yolo") {
		t.Errorf("Expected --yolo in args, got %v", args)
	}
}

func TestAgent_BuildArgs_WithExtraArgs(t *testing.T) {
	agent := New(agents.AgentConfig{
		ExtraArgs: []string{"--verbose", "--model", "gpt-4"},
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

func TestAgent_BuildArgs_WithModel(t *testing.T) {
	tests := []struct {
		name     string
		options  map[string]string
		wantFlag bool
		wantVal  string
	}{
		{
			name:     "no model configured",
			options:  nil,
			wantFlag: false,
		},
		{
			name:     "empty options map",
			options:  map[string]string{},
			wantFlag: false,
		},
		{
			name:     "empty model value",
			options:  map[string]string{"model": ""},
			wantFlag: false,
		},
		{
			name:     "model configured",
			options:  map[string]string{"model": "gpt-4"},
			wantFlag: true,
			wantVal:  "gpt-4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := New(agents.AgentConfig{
				Options: tt.options,
			}).(*Agent)

			args := agent.buildArgs(agents.RunOptions{
				Prompt:    "Test prompt",
				SessionID: "test-session",
			}, false)

			modelIdx := slices.Index(args, "--model")
			if tt.wantFlag {
				if modelIdx == -1 {
					t.Errorf("Expected --model flag in args, got %v", args)
				} else if modelIdx+1 >= len(args) || args[modelIdx+1] != tt.wantVal {
					t.Errorf("Expected --model %s in args, got %v", tt.wantVal, args)
				}
			} else {
				if modelIdx != -1 {
					t.Errorf("Did not expect --model flag in args, got %v", args)
				}
			}
		})
	}
}

func TestAgent_BuildArgs_ModelBeforePrompt(t *testing.T) {
	agent := New(agents.AgentConfig{
		Options: map[string]string{"model": "gpt-4"},
	}).(*Agent)

	args := agent.buildArgs(agents.RunOptions{
		Prompt:    "Test prompt",
		SessionID: "test-session",
	}, false)

	modelIdx := slices.Index(args, "--model")
	pIdx := slices.Index(args, "-p")

	if modelIdx == -1 {
		t.Fatal("Expected --model flag in args")
	}
	if pIdx == -1 {
		t.Fatal("Expected -p flag in args")
	}
	if modelIdx >= pIdx {
		t.Errorf("--model should come before -p: model at %d, -p at %d", modelIdx, pIdx)
	}
}

func TestAgent_RegisteredInInit(t *testing.T) {
	// Verify the agent is registered in the registry
	agent, err := agents.Get("copilot", agents.AgentConfig{})
	if err != nil {
		t.Fatalf("agents.Get(\"copilot\") error = %v", err)
	}
	if agent == nil {
		t.Fatal("agents.Get(\"copilot\") returned nil")
	}
	if agent.Name() != "copilot" {
		t.Errorf("agent.Name() = %q, want %q", agent.Name(), "copilot")
	}
}

func TestAgent_DiscoverSessions(t *testing.T) {
	agent := New(agents.AgentConfig{})

	// Should not error on non-existent directory
	sessions, err := agent.DiscoverSessions(context.Background(), "/nonexistent/path")
	if err != nil {
		t.Errorf("DiscoverSessions() error = %v", err)
	}
	// Empty result is acceptable for non-existent directory
	_ = sessions
}

func TestAgent_Version(t *testing.T) {
	agent := New(agents.AgentConfig{})

	// Version may return error if copilot CLI is not installed
	// We just verify it doesn't panic
	_, _ = agent.Version()
}

func TestAgent_Resume_IgnoresSessionID(t *testing.T) {
	// Document the Known Limitation: Copilot CLI only supports resuming the most recent session.
	// The sessionID parameter is accepted for interface compatibility but ignored.
	agent := New(agents.AgentConfig{}).(*Agent)

	args1 := agent.buildArgs(agents.RunOptions{
		Prompt:    "Test prompt",
		SessionID: "session-1",
	}, true)

	args2 := agent.buildArgs(agents.RunOptions{
		Prompt:    "Test prompt",
		SessionID: "session-2",
	}, true)

	// Both should produce identical args since sessionID is ignored
	if len(args1) != len(args2) {
		t.Errorf("Different sessionIDs should produce same args for resume, got %v vs %v", args1, args2)
	}

	for i := range args1 {
		if args1[i] != args2[i] {
			t.Errorf("Arg mismatch at position %d: %q vs %q", i, args1[i], args2[i])
		}
	}
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

			// -p and prompt should be together
			pIdx := slices.Index(args, "-p")
			if pIdx == -1 || pIdx+1 >= len(args) {
				t.Error("Expected -p flag with prompt")
			}

			// --yolo should come before -p
			yoloPos := slices.Index(args, "--yolo")
			if yoloPos != -1 && pIdx != -1 && yoloPos >= pIdx {
				t.Errorf("--yolo should come before -p: --yolo at %d, -p at %d", yoloPos, pIdx)
			}
		})
	}
}
