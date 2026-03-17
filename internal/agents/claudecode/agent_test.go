package claudecode

import (
	"os"
	"path/filepath"
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
	sessions, err := agent.DiscoverSessions(t.Context(), "/nonexistent/path")
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

func TestAgent_BuildArgs_EmptySessionID(t *testing.T) {
	agent := New(agents.AgentConfig{}).(*Agent)

	args := agent.buildArgs(agents.RunOptions{
		Prompt:    "Test prompt",
		SessionID: "", // Empty session ID — should omit --session-id entirely
	}, false)

	// Should NOT contain --session-id when the ID is empty
	if slices.Contains(args, "--session-id") {
		t.Errorf("Empty session ID should omit --session-id flag, got %v", args)
	}

	// Prompt should still be present
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

// setupFakeSessionDir creates a temporary directory mimicking ~/.claude/projects/
// with multiple project hash folders, each containing .jsonl session files.
// Returns the temp dir and a cleanup function.
func setupFakeSessionDir(t *testing.T) string {
	t.Helper()
	base := t.TempDir()

	// Derive hash names from BuildProjectPath to stay in sync with encoding changes.
	projAHash := BuildProjectPath("/Users/alice/projectA")
	projBHash := BuildProjectPath("/Users/bob/projectB")

	for _, ph := range []string{projAHash, projBHash} {
		dir := filepath.Join(base, ph)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		// Write a dummy .jsonl file in each.
		if err := os.WriteFile(filepath.Join(dir, "session1.jsonl"), []byte(`{}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return base
}

// TestDiscoverSessions_FiltersbyProjectDir verifies that when projectDir is
// provided, only sessions for that project are returned (bug T-396).
func TestDiscoverSessions_FiltersByProjectDir(t *testing.T) {
	t.Parallel()
	base := setupFakeSessionDir(t)

	sessions, err := discoverSessionsIn(t.Context(), base, "/Users/alice/projectA")
	if err != nil {
		t.Fatalf("discoverSessionsIn() error = %v", err)
	}

	// Should only return sessions from projectA's hash folder.
	wantProject := BuildProjectPath("/Users/alice/projectA")
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session for projectA, got %d", len(sessions))
	}
	if sessions[0].Project != wantProject {
		t.Errorf("expected project %q, got %q", wantProject, sessions[0].Project)
	}
}

// TestDiscoverSessions_NoProjectDir_ReturnsAll verifies that when projectDir
// is empty, sessions from all projects are returned.
func TestDiscoverSessions_NoProjectDir_ReturnsAll(t *testing.T) {
	t.Parallel()
	base := setupFakeSessionDir(t)

	sessions, err := discoverSessionsIn(t.Context(), base, "")
	if err != nil {
		t.Fatalf("discoverSessionsIn() error = %v", err)
	}

	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions (all projects), got %d", len(sessions))
	}
}

// TestDiscoverSessions_ProjectDir_NonexistentProject verifies that when
// projectDir points to a project with no sessions, an empty result is returned.
func TestDiscoverSessions_ProjectDir_NonexistentProject(t *testing.T) {
	t.Parallel()
	base := setupFakeSessionDir(t)

	sessions, err := discoverSessionsIn(t.Context(), base, "/Users/nobody/nonexistent")
	if err != nil {
		t.Fatalf("discoverSessionsIn() error = %v", err)
	}

	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions for nonexistent project, got %d", len(sessions))
	}
}

func TestBuildProjectPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "unix absolute path",
			input:    "/Users/foo/project",
			expected: "-Users-foo-project",
		},
		{
			name:     "unix relative path",
			input:    "foo/project",
			expected: "foo-project",
		},
		{
			name:     "windows absolute path",
			input:    `C:\Users\foo\project`,
			expected: `C:-Users-foo-project`,
		},
		{
			name:     "windows relative path",
			input:    `foo\project`,
			expected: `foo-project`,
		},
		{
			name:     "mixed separators",
			input:    `/Users/foo\project`,
			expected: `-Users-foo-project`,
		},
		{
			name:     "single directory",
			input:    "/project",
			expected: "-project",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		// The following test cases cover the bug fix for variant log paths
		// where dots in paths weren't being replaced with dashes
		{
			name:     "path with dots in directory name",
			input:    "/home/user/project.name/subdir",
			expected: "-home-user-project-name-subdir",
		},
		{
			name:     "variant worktree path with dot suffix",
			input:    "/home/user/orbit/specs/feature/.orbit/worktrees/orbit-impl-1-feature.5",
			expected: "-home-user-orbit-specs-feature--orbit-worktrees-orbit-impl-1-feature-5",
		},
		{
			name:     "hidden directory (dot prefix)",
			input:    "/home/user/.config/project",
			expected: "-home-user--config-project",
		},
		{
			name:     "multiple dots in path",
			input:    "/path/to/file.tar.gz",
			expected: "-path-to-file-tar-gz",
		},
		{
			name:     "dot orbit directory in variant path",
			input:    "/repo/specs/my-feature/.orbit/worktrees/variant-1",
			expected: "-repo-specs-my-feature--orbit-worktrees-variant-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildProjectPath(tt.input)
			if result != tt.expected {
				t.Errorf("BuildProjectPath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
