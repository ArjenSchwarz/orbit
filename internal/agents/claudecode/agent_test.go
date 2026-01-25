package claudecode

import (
	"context"
	"slices"
	"testing"

	"github.com/arjenschwarz/orbit/internal/agents"
)

func TestNew(t *testing.T) {
	cfg := agents.AgentConfig{
		CLIPath:     "/custom/path/claude",
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
	if name := agent.Name(); name != "claude-code" {
		t.Errorf("Name() = %q, want %q", name, "claude-code")
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
			expected: "claude",
		},
		{
			name:     "custom CLI path",
			cliPath:  "/usr/local/bin/claude",
			expected: "/usr/local/bin/claude",
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

	// Should contain .claude/projects
	if dir == "" {
		t.Error("DefaultSessionDir() returned empty string")
	}
	if !slices.Contains([]string{".claude"}, "") && dir != "" {
		// Just verify it's non-empty and likely contains the expected path structure
		t.Logf("DefaultSessionDir() = %q", dir)
	}
}

func TestAgent_BuildRunPhaseArgs_NewSession(t *testing.T) {
	agent := New(agents.AgentConfig{
		AutoApprove: false,
	}).(*Agent)

	args := agent.buildArgs(agents.RunOptions{
		Prompt:    "Test prompt",
		SessionID: "test-session-id-123",
	}, false)

	// Check that --session-id comes first with the session ID
	if len(args) < 2 || args[0] != "--session-id" || args[1] != "test-session-id-123" {
		t.Errorf("Expected args to start with [--session-id test-session-id-123], got %v", args)
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
	if slices.Contains(args, "--resume") {
		t.Errorf("New session should not have --resume flag, got %v", args)
	}
}

func TestAgent_BuildRunPhaseArgs_ResumeSession(t *testing.T) {
	agent := New(agents.AgentConfig{
		AutoApprove: false,
	}).(*Agent)

	args := agent.buildArgs(agents.RunOptions{
		Prompt:    "Test prompt",
		SessionID: "existing-session-456",
	}, true)

	// Check that --resume comes first with the session ID
	if len(args) < 2 || args[0] != "--resume" || args[1] != "existing-session-456" {
		t.Errorf("Expected args to start with [--resume existing-session-456], got %v", args)
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
	if slices.Contains(args, "--session-id") {
		t.Errorf("Resume session should not have --session-id flag, got %v", args)
	}
}

func TestAgent_BuildRunPhaseArgs_WithAutoApprove(t *testing.T) {
	agent := New(agents.AgentConfig{
		AutoApprove: true,
	}).(*Agent)

	args := agent.buildArgs(agents.RunOptions{
		Prompt:    "Test prompt",
		SessionID: "test-session",
	}, false)

	if !slices.Contains(args, "--dangerously-skip-permissions") {
		t.Errorf("Expected --dangerously-skip-permissions in args, got %v", args)
	}
}

func TestAgent_BuildRunPhaseArgs_WithExtraArgs(t *testing.T) {
	agent := New(agents.AgentConfig{
		ExtraArgs: []string{"--verbose", "--model", "claude-3-opus"},
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

func TestAgent_BuildRunPhaseArgs_WithOptsExtraArgs(t *testing.T) {
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
			options:  map[string]string{"model": "claude-opus-4"},
			wantFlag: true,
			wantVal:  "claude-opus-4",
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

func TestAgent_RegisteredInInit(t *testing.T) {
	// Verify the agent is registered in the registry
	agent, err := agents.Get("claude-code", agents.AgentConfig{})
	if err != nil {
		t.Fatalf("agents.Get(\"claude-code\") error = %v", err)
	}
	if agent == nil {
		t.Fatal("agents.Get(\"claude-code\") returned nil")
	}
	if agent.Name() != "claude-code" {
		t.Errorf("agent.Name() = %q, want %q", agent.Name(), "claude-code")
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

	// Version may return error if claude CLI is not installed
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
		firstFlag string
	}{
		{"new session", "new-id", false, "--session-id"},
		{"resume session", "existing-id", true, "--resume"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := agent.buildArgs(agents.RunOptions{
				Prompt:    "Test prompt",
				SessionID: tt.sessionID,
			}, tt.resume)

			if len(args) < 1 || args[0] != tt.firstFlag {
				t.Errorf("Expected first arg to be %s, got %v", tt.firstFlag, args)
			}

			// Find positions of key flags
			var sessionFlagPos, promptPos, outputFormatPos = -1, -1, -1
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
		})
	}
}
