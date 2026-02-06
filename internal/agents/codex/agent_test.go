package codex

import (
	"context"
	"slices"
	"testing"

	"github.com/arjenschwarz/orbit/internal/agents"
)

func TestNew(t *testing.T) {
	cfg := agents.AgentConfig{
		CLIPath:     "/custom/path/codex",
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
	if name := agent.Name(); name != "codex" {
		t.Errorf("Name() = %q, want %q", name, "codex")
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
			expected: "codex",
		},
		{
			name:     "custom CLI path",
			cliPath:  "/usr/local/bin/codex",
			expected: "/usr/local/bin/codex",
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

	// Should contain .codex/sessions
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
	})

	// Should start with "exec"
	if len(args) < 1 || args[0] != "exec" {
		t.Errorf("Expected args to start with 'exec', got %v", args)
	}

	// Check that the prompt is included
	if !slices.Contains(args, "Test prompt") {
		t.Errorf("Expected 'Test prompt' in args, got %v", args)
	}

	// Should NOT contain --dangerously-bypass-approvals-and-sandbox when AutoApprove is false
	if slices.Contains(args, "--dangerously-bypass-approvals-and-sandbox") {
		t.Errorf("Should not have --dangerously-bypass-approvals-and-sandbox without AutoApprove, got %v", args)
	}

	// codex exec never uses --resume or --last (no session resumption support)
	if slices.Contains(args, "--resume") || slices.Contains(args, "--last") {
		t.Errorf("Should not have --resume or --last flag, got %v", args)
	}
}

func TestAgent_ResumeStartsFreshSession(t *testing.T) {
	// codex exec does not support session resumption, so Resume()
	// should produce the same args as Run() (no --last flag).
	agent := New(agents.AgentConfig{
		AutoApprove: false,
	}).(*Agent)

	args := agent.buildArgs(agents.RunOptions{
		Prompt:    "Test prompt",
		SessionID: "existing-session-456",
	})

	if len(args) < 1 || args[0] != "exec" {
		t.Errorf("Expected args to start with 'exec', got %v", args)
	}

	// Must NOT contain --last since codex exec doesn't support it
	if slices.Contains(args, "--last") {
		t.Errorf("Should not have --last flag (codex exec has no resume), got %v", args)
	}

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
	})

	if !slices.Contains(args, "--dangerously-bypass-approvals-and-sandbox") {
		t.Errorf("Expected --dangerously-bypass-approvals-and-sandbox in args, got %v", args)
	}
}

func TestAgent_BuildArgs_WithExtraArgs(t *testing.T) {
	agent := New(agents.AgentConfig{
		ExtraArgs: []string{"--sandbox", "workspace-write"},
	}).(*Agent)

	args := agent.buildArgs(agents.RunOptions{
		Prompt:    "Test prompt",
		SessionID: "test-session",
	})

	if !slices.Contains(args, "--sandbox") {
		t.Errorf("Expected --sandbox in args, got %v", args)
	}
	if !slices.Contains(args, "workspace-write") {
		t.Errorf("Expected 'workspace-write' in args, got %v", args)
	}
}

func TestAgent_BuildArgs_WithOptsExtraArgs(t *testing.T) {
	agent := New(agents.AgentConfig{}).(*Agent)

	args := agent.buildArgs(agents.RunOptions{
		Prompt:    "Test prompt",
		SessionID: "test-session",
		ExtraArgs: []string{"--custom-flag"},
	})

	if !slices.Contains(args, "--custom-flag") {
		t.Errorf("Expected --custom-flag in args, got %v", args)
	}
}

func TestAgent_DefaultPrompt(t *testing.T) {
	agent := New(agents.AgentConfig{}).(*Agent)

	args := agent.buildArgs(agents.RunOptions{
		Prompt:    "", // Empty prompt should use default
		SessionID: "test-session",
	})

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
			options:  map[string]string{"model": "o3"},
			wantFlag: true,
			wantVal:  "o3",
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
			})

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
		Options: map[string]string{"model": "o3"},
	}).(*Agent)

	args := agent.buildArgs(agents.RunOptions{
		Prompt:    "Test prompt",
		SessionID: "test-session",
	})

	modelIdx := slices.Index(args, "--model")
	promptIdx := slices.Index(args, "Test prompt")

	if modelIdx == -1 {
		t.Fatal("Expected --model flag in args")
	}
	if promptIdx == -1 {
		t.Fatal("Expected prompt in args")
	}
	if modelIdx >= promptIdx {
		t.Errorf("--model should come before prompt: model at %d, prompt at %d", modelIdx, promptIdx)
	}
}

func TestAgent_RegisteredInInit(t *testing.T) {
	// Verify the agent is registered in the registry
	agent, err := agents.Get("codex", agents.AgentConfig{})
	if err != nil {
		t.Fatalf("agents.Get(\"codex\") error = %v", err)
	}
	if agent == nil {
		t.Fatal("agents.Get(\"codex\") returned nil")
	}
	if agent.Name() != "codex" {
		t.Errorf("agent.Name() = %q, want %q", agent.Name(), "codex")
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

	// Version may return error if codex CLI is not installed
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
	}{
		{"new session", "new-id"},
		{"any session", "existing-id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := agent.buildArgs(agents.RunOptions{
				Prompt:    "Test prompt",
				SessionID: tt.sessionID,
			})

			// First arg should always be "exec"
			if len(args) < 1 || args[0] != "exec" {
				t.Errorf("Expected first arg to be 'exec', got %v", args)
			}

			// --dangerously-bypass-approvals-and-sandbox should come before prompt
			fullAutoPos := -1
			promptPos := -1
			for i, arg := range args {
				if arg == "--dangerously-bypass-approvals-and-sandbox" {
					fullAutoPos = i
				}
				if arg == "Test prompt" {
					promptPos = i
				}
			}

			if fullAutoPos != -1 && promptPos != -1 && fullAutoPos >= promptPos {
				t.Errorf("--dangerously-bypass-approvals-and-sandbox should come before prompt: --dangerously-bypass-approvals-and-sandbox at %d, prompt at %d", fullAutoPos, promptPos)
			}
		})
	}
}
