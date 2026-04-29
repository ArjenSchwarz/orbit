package opencode

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
		"empty":                {input: nil, want: false},
		"empty string":         {input: []byte(""), want: false},
		"whitespace only":      {input: []byte("   "), want: false},
		"valid object":         {input: []byte(`{"key": "value"}`), want: true},
		"valid array":          {input: []byte(`[1, 2, 3]`), want: true},
		"valid string":         {input: []byte(`"hello"`), want: true},
		"valid number":         {input: []byte(`123`), want: true},
		"plaintext":            {input: []byte("some error text"), want: false},
		"stack trace":          {input: []byte("Error: something went wrong\n  at func()"), want: false},
		"json with whitespace": {input: []byte(`  {"key": "value"}  `), want: true},
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

// TestDiscoverSessions_CreatedAtFallbackToModTime verifies that when no msg_
// files exist in a session directory, CreatedAt falls back to the directory's
// modTime rather than remaining as the zero time.
// Regression test for T-273.
func TestDiscoverSessions_CreatedAtFallbackToModTime(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	// Create a session directory with no msg_ files.
	sessionDir := filepath.Join(tmp, "ses_no_messages")
	require.NoError(t, os.Mkdir(sessionDir, 0o755))

	// Write a non-message file so the directory isn't empty.
	require.NoError(t, os.WriteFile(filepath.Join(sessionDir, "metadata.json"), []byte(`{}`), 0o644))

	sessions, err := discoverSessionsIn(t.Context(), tmp, "")
	require.NoError(t, err)
	require.Len(t, sessions, 1, "expected exactly one session")

	session := sessions[0]
	assert.Equal(t, "ses_no_messages", session.ID)
	assert.False(t, session.CreatedAt.IsZero(),
		"CreatedAt must not be zero when no msg_ files exist; should fall back to directory modTime")
}

// TestDiscoverSessions_ParseCreatedTimeFallbackOnBadTimestamp verifies that
// when a msg_ file has an unparseable timestamp, parseCreatedTime returns
// the modTime fallback. This exercises the fallback parameter in parseCreatedTime,
// not the createdAt.IsZero() guard in discoverSessionsIn.
// Regression test for T-273.
func TestDiscoverSessions_ParseCreatedTimeFallbackOnBadTimestamp(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	sessionDir := filepath.Join(tmp, "ses_bad_timestamps")
	require.NoError(t, os.Mkdir(sessionDir, 0o755))

	// Write a msg_ file with a created field that cannot be parsed into a time.
	badMsg := `{"id":"msg_1","sessionID":"ses_bad_timestamps","role":"user","time":{"created":"not-a-timestamp"}}`
	require.NoError(t, os.WriteFile(filepath.Join(sessionDir, "msg_001.json"), []byte(badMsg), 0o644))

	sessions, err := discoverSessionsIn(t.Context(), tmp, "")
	require.NoError(t, err)
	require.Len(t, sessions, 1)

	session := sessions[0]
	assert.False(t, session.CreatedAt.IsZero(),
		"CreatedAt must not be zero when msg_ timestamp parsing fails; should fall back to directory modTime")
}

// TestDiscoverSessions_CreatedAtFallbackWhenAllMsgFilesUnreadable verifies
// that when all msg_ files fail to unmarshal, the createdAt.IsZero() guard
// in discoverSessionsIn falls back to directory modTime.
// Regression test for T-273.
func TestDiscoverSessions_CreatedAtFallbackWhenAllMsgFilesUnreadable(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	sessionDir := filepath.Join(tmp, "ses_invalid_json")
	require.NoError(t, os.Mkdir(sessionDir, 0o755))

	// Write a msg_ file with invalid JSON so json.Unmarshal fails and
	// parseCreatedTime is never called — createdAt stays zero.
	require.NoError(t, os.WriteFile(
		filepath.Join(sessionDir, "msg_001.json"),
		[]byte(`not valid json at all`),
		0o644,
	))

	sessions, err := discoverSessionsIn(t.Context(), tmp, "")
	require.NoError(t, err)
	require.Len(t, sessions, 1)

	session := sessions[0]
	assert.False(t, session.CreatedAt.IsZero(),
		"CreatedAt must not be zero when all msg_ files fail to unmarshal; should fall back to directory modTime")
}

// TestDiscoverSessions_FiltersbyProjectDir verifies that when a projectDir is
// provided, only sessions belonging to that project are returned.
// Regression test for T-740.
func TestDiscoverSessions_FiltersByProjectDir(t *testing.T) {
	t.Parallel()

	// Build a fake storage tree:
	// tmp/storage/message/ses_A/msg_001.json  (belongs to projectA)
	// tmp/storage/message/ses_B/msg_001.json  (belongs to projectB)
	// tmp/storage/project/proj1.json          (worktree: /fake/projectA)
	// tmp/storage/session/proj1/ses_A.json
	// tmp/storage/session/proj2/ses_B.json
	tmp := t.TempDir()
	storageDir := filepath.Join(tmp, "storage")
	messageDir := filepath.Join(storageDir, "message")
	projectDir := filepath.Join(storageDir, "project")
	sessionDir := filepath.Join(storageDir, "session")

	// Create message directories.
	for _, ses := range []string{"ses_A", "ses_B"} {
		dir := filepath.Join(messageDir, ses)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		msg := `{"id":"msg_1","sessionID":"` + ses + `","role":"user","time":{"created":"2026-01-01T00:00:00Z"}}`
		require.NoError(t, os.WriteFile(filepath.Join(dir, "msg_001.json"), []byte(msg), 0o644))
	}

	// Create project files.
	require.NoError(t, os.MkdirAll(projectDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(projectDir, "proj1.json"),
		[]byte(`{"id":"proj1","worktree":"/fake/projectA"}`), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(projectDir, "proj2.json"),
		[]byte(`{"id":"proj2","worktree":"/fake/projectB"}`), 0o644))

	// Create session index directories.
	require.NoError(t, os.MkdirAll(filepath.Join(sessionDir, "proj1"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(sessionDir, "proj2"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sessionDir, "proj1", "ses_A.json"), []byte(`{}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sessionDir, "proj2", "ses_B.json"), []byte(`{}`), 0o644))

	// Without filter: returns both sessions.
	all, err := discoverSessionsIn(t.Context(), messageDir, "")
	require.NoError(t, err)
	assert.Len(t, all, 2)

	// With projectA filter: returns only ses_A.
	filtered, err := discoverSessionsIn(t.Context(), messageDir, "/fake/projectA")
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	assert.Equal(t, "ses_A", filtered[0].ID)

	// Trailing slash on caller side must still match.
	trailingSlash, err := discoverSessionsIn(t.Context(), messageDir, "/fake/projectA/")
	require.NoError(t, err)
	require.Len(t, trailingSlash, 1)
	assert.Equal(t, "ses_A", trailingSlash[0].ID)

	// With unknown project: returns nil.
	none, err := discoverSessionsIn(t.Context(), messageDir, "/fake/unknown")
	require.NoError(t, err)
	assert.Empty(t, none)
}

// TestParseCreatedTime_ZeroUnixReturnsFallback verifies that a created time
// of unix epoch 0 returns the fallback rather than time.Time{} (Go zero).
// Regression test for T-273.
func TestParseCreatedTime_ZeroUnixReturnsFallback(t *testing.T) {
	t.Parallel()

	fallback := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	tests := map[string]struct {
		raw json.RawMessage
	}{
		"numeric zero":    {raw: json.RawMessage(`0`)},
		"negative number": {raw: json.RawMessage(`-1`)},
		"string zero":     {raw: json.RawMessage(`"0"`)},
		"negative float":  {raw: json.RawMessage(`-100.5`)},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := parseCreatedTime(tc.raw, fallback)
			assert.Equal(t, fallback, got,
				"parseCreatedTime should return fallback for non-positive unix value")
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
			if tt.wantMsgPart != "" {
				assert.Contains(t, errMsg, tt.wantMsgPart)
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
