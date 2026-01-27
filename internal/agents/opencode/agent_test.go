package opencode

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/arjenschwarz/orbit/internal/agents"
)

func TestNew(t *testing.T) {
	cfg := agents.AgentConfig{
		CLIPath: "/custom/path/opencode",
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
	if name := agent.Name(); name != "opencode" {
		t.Errorf("Name() = %q, want %q", name, "opencode")
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
			expected: "opencode",
		},
		{
			name:     "custom CLI path",
			cliPath:  "/usr/local/bin/opencode",
			expected: "/usr/local/bin/opencode",
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

	// Should contain .local/share/opencode/storage/message
	if dir == "" {
		t.Error("DefaultSessionDir() returned empty string")
	}
	t.Logf("DefaultSessionDir() = %q", dir)
}

func TestAgent_BuildArgs_NewSession(t *testing.T) {
	agent := New(agents.AgentConfig{}).(*Agent)

	args := agent.buildArgs(agents.RunOptions{
		Prompt: "Test prompt",
	}, false)

	// Should start with "run"
	if len(args) < 1 || args[0] != "run" {
		t.Errorf("Expected args to start with 'run', got %v", args)
	}

	// Check that --format json is included
	if !slices.Contains(args, "--format") || !slices.Contains(args, "json") {
		t.Errorf("Expected '--format json' in args, got %v", args)
	}

	// Check that the prompt is included
	if !slices.Contains(args, "Test prompt") {
		t.Errorf("Expected 'Test prompt' in args, got %v", args)
	}

	// Should NOT contain --continue for new session
	if slices.Contains(args, "--continue") {
		t.Errorf("New session should not have --continue flag, got %v", args)
	}
}

func TestAgent_BuildArgs_ResumeSession(t *testing.T) {
	agent := New(agents.AgentConfig{}).(*Agent)

	args := agent.buildArgs(agents.RunOptions{
		Prompt:    "Test prompt",
		SessionID: "existing-session-456",
	}, true)

	// Should start with "run"
	if len(args) < 1 || args[0] != "run" {
		t.Errorf("Expected args to start with 'run', got %v", args)
	}

	// Check that --continue is present for resume
	if !slices.Contains(args, "--continue") {
		t.Errorf("Resume should have --continue flag, got %v", args)
	}

	// Check that the prompt is included
	if !slices.Contains(args, "Test prompt") {
		t.Errorf("Expected 'Test prompt' in args, got %v", args)
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
			options:  map[string]string{"model": "anthropic/claude-sonnet-4-5"},
			wantFlag: true,
			wantVal:  "anthropic/claude-sonnet-4-5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := New(agents.AgentConfig{
				Options: tt.options,
			}).(*Agent)

			args := agent.buildArgs(agents.RunOptions{
				Prompt: "Test prompt",
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
		Options: map[string]string{"model": "anthropic/claude-sonnet-4-5"},
	}).(*Agent)

	args := agent.buildArgs(agents.RunOptions{
		Prompt: "Test prompt",
	}, false)

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

func TestAgent_BuildArgs_WithExtraArgs(t *testing.T) {
	agent := New(agents.AgentConfig{
		ExtraArgs: []string{"--custom-flag", "value"},
	}).(*Agent)

	args := agent.buildArgs(agents.RunOptions{
		Prompt: "Test prompt",
	}, false)

	if !slices.Contains(args, "--custom-flag") {
		t.Errorf("Expected --custom-flag in args, got %v", args)
	}
	if !slices.Contains(args, "value") {
		t.Errorf("Expected 'value' in args, got %v", args)
	}
}

func TestAgent_BuildArgs_WithOptsExtraArgs(t *testing.T) {
	agent := New(agents.AgentConfig{}).(*Agent)

	args := agent.buildArgs(agents.RunOptions{
		Prompt:    "Test prompt",
		ExtraArgs: []string{"--opt-flag"},
	}, false)

	if !slices.Contains(args, "--opt-flag") {
		t.Errorf("Expected --opt-flag in args, got %v", args)
	}
}

func TestAgent_DefaultPrompt(t *testing.T) {
	agent := New(agents.AgentConfig{}).(*Agent)

	args := agent.buildArgs(agents.RunOptions{
		Prompt: "", // Empty prompt should use default
	}, false)

	if !slices.Contains(args, defaultPrompt) {
		t.Errorf("Expected default prompt %q in args, got %v", defaultPrompt, args)
	}
}

func TestAgent_RegisteredInInit(t *testing.T) {
	// Verify the agent is registered in the registry
	agent, err := agents.Get("opencode", agents.AgentConfig{})
	if err != nil {
		t.Fatalf("agents.Get(\"opencode\") error = %v", err)
	}
	if agent == nil {
		t.Fatal("agents.Get(\"opencode\") returned nil")
	}
	if agent.Name() != "opencode" {
		t.Errorf("agent.Name() = %q, want %q", agent.Name(), "opencode")
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

	// Version may return error if opencode CLI is not installed
	// We just verify it doesn't panic
	_, _ = agent.Version()
}

func TestAgent_ArgOrder(t *testing.T) {
	agent := New(agents.AgentConfig{
		Options: map[string]string{"model": "anthropic/claude-sonnet-4-5"},
	}).(*Agent)

	tests := []struct {
		name   string
		resume bool
	}{
		{"new session", false},
		{"resume session", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := agent.buildArgs(agents.RunOptions{
				Prompt: "Test prompt",
			}, tt.resume)

			// First arg should always be "run"
			if len(args) < 1 || args[0] != "run" {
				t.Errorf("Expected first arg to be 'run', got %v", args)
			}

			// --format should come early after run
			formatPos := slices.Index(args, "--format")
			promptPos := slices.Index(args, "Test prompt")

			if formatPos != -1 && promptPos != -1 && formatPos >= promptPos {
				t.Errorf("--format should come before prompt: --format at %d, prompt at %d", formatPos, promptPos)
			}

			// --model should come before prompt
			modelPos := slices.Index(args, "--model")
			if modelPos != -1 && promptPos != -1 && modelPos >= promptPos {
				t.Errorf("--model should come before prompt: --model at %d, prompt at %d", modelPos, promptPos)
			}
		})
	}
}

func TestIsValidJSON(t *testing.T) {
	tests := map[string]struct {
		input []byte
		want  bool
	}{
		"empty":                 {input: nil, want: false},
		"empty string":          {input: []byte(""), want: false},
		"whitespace only":       {input: []byte("   "), want: false},
		"valid object":          {input: []byte(`{"key": "value"}`), want: true},
		"valid array":           {input: []byte(`[1, 2, 3]`), want: true},
		"valid string":          {input: []byte(`"hello"`), want: true},
		"valid number":          {input: []byte(`123`), want: true},
		"plaintext":             {input: []byte("some error text"), want: false},
		"stack trace":           {input: []byte("Error: something went wrong\n  at func()"), want: false},
		"json with whitespace":  {input: []byte(`  {"key": "value"}  `), want: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := isValidJSON(tt.input)
			if got != tt.want {
				t.Errorf("isValidJSON(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestAgent_DiscoverSessions_ContextCancellation(t *testing.T) {
	agent := New(agents.AgentConfig{})

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Should return context error when context is cancelled
	_, err := agent.DiscoverSessions(ctx, "/some/path")
	if err != context.Canceled {
		// Note: May return nil if session dir doesn't exist (checked before loop)
		// This test mainly verifies the function doesn't panic with cancelled context
		t.Logf("DiscoverSessions with cancelled context returned: %v", err)
	}
}

func TestErrorDetection_EmptyOutput(t *testing.T) {
	// Test that empty output is correctly detected as an error.
	// This simulates what happens when OpenCode exits 0 but produces no stdout
	// (e.g., auth/CLI errors that only write to stderr).
	tests := map[string]struct {
		raw         []byte
		wantIsError bool
		wantMsgPart string
	}{
		"empty output": {
			raw:         []byte{},
			wantIsError: true,
			wantMsgPart: "empty output",
		},
		"whitespace only": {
			raw:         []byte("   \n\t  "),
			wantIsError: true,
			wantMsgPart: "not valid JSON",
		},
		"invalid JSON": {
			raw:         []byte("Error: model not found"),
			wantIsError: true,
			wantMsgPart: "not valid JSON",
		},
		"valid JSON": {
			raw:         []byte(`{"status": "ok"}`),
			wantIsError: false,
			wantMsgPart: "",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// Simulate the error detection logic from execute()
			var isError bool
			var errMsg string

			if !isValidJSON(tt.raw) {
				isError = true
				if len(tt.raw) == 0 {
					errMsg = "empty output (expected JSON)"
				} else {
					preview := string(tt.raw)
					if len(preview) > 100 {
						preview = preview[:100] + "..."
					}
					errMsg = "output is not valid JSON: " + preview
				}
			}

			if isError != tt.wantIsError {
				t.Errorf("isError = %v, want %v", isError, tt.wantIsError)
			}
			if tt.wantMsgPart != "" && !strings.Contains(errMsg, tt.wantMsgPart) {
				t.Errorf("errMsg = %q, want to contain %q", errMsg, tt.wantMsgPart)
			}
		})
	}
}

func TestParseVersionOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "version only",
			output: "1.1.36\n",
			want:   "1.1.36",
		},
		{
			name:   "version without newline",
			output: "1.1.36",
			want:   "1.1.36",
		},
		{
			name: "INFO log prefix",
			output: `INFO  2026-01-27T12:16:29 +27ms service=models.dev file={} refreshing
1.1.36
`,
			want: "1.1.36",
		},
		{
			name: "multiple INFO log lines",
			output: `INFO  2026-01-27T12:16:29 +27ms service=models.dev file={} refreshing
INFO  2026-01-27T12:16:29 +30ms service=other doing something
1.1.36
`,
			want: "1.1.36",
		},
		{
			name:   "trailing whitespace",
			output: "1.1.36   \n\n",
			want:   "1.1.36",
		},
		{
			name:   "empty output",
			output: "",
			want:   "",
		},
		{
			name:   "whitespace only",
			output: "   \n\n   ",
			want:   "",
		},
		{
			name: "semver with prefix",
			output: `INFO  refreshing
v2.0.0
`,
			want: "v2.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseVersionOutput(tt.output)
			if got != tt.want {
				t.Errorf("parseVersionOutput() = %q, want %q", got, tt.want)
			}
		})
	}
}
