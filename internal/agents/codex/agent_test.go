package codex

import (
	"context"
	"os"
	"path/filepath"
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

func TestAgent_DiscoverSessions_NestedDirs(t *testing.T) {
	// Codex sessions are stored in YYYY/MM/DD subdirectories, not flat in the root.
	// Regression test: DiscoverSessions must find sessions in nested directories.
	tmpDir := t.TempDir()

	// Create a session file at YYYY/MM/DD/session.jsonl
	nestedDir := filepath.Join(tmpDir, "2025", "01", "15")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("failed to create nested dir: %v", err)
	}

	sessionFile := filepath.Join(nestedDir, "session-abc123.jsonl")
	if err := os.WriteFile(sessionFile, []byte(`{"type":"session_meta"}`+"\n"), 0o644); err != nil {
		t.Fatalf("failed to write session file: %v", err)
	}

	agent := &Agent{
		config:     agents.AgentConfig{},
		cliPath:    "codex",
		sessionDir: tmpDir,
	}

	sessions, err := agent.DiscoverSessions(context.Background(), "")
	if err != nil {
		t.Fatalf("DiscoverSessions() error = %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session from nested directory, got %d", len(sessions))
	}

	if sessions[0].Path != sessionFile {
		t.Errorf("session.Path = %q, want %q", sessions[0].Path, sessionFile)
	}
	if sessions[0].Agent != "codex" {
		t.Errorf("session.Agent = %q, want %q", sessions[0].Agent, "codex")
	}
}

func TestAgent_DiscoverSessions_MultipleNestedSessions(t *testing.T) {
	// Verify that sessions across multiple date directories are all discovered.
	tmpDir := t.TempDir()

	dates := []struct {
		path string
		name string
	}{
		{"2025/01/15", "session-aaa.jsonl"},
		{"2025/01/16", "session-bbb.jsonl"},
		{"2025/02/01", "session-ccc.jsonl"},
	}

	for _, d := range dates {
		dir := filepath.Join(tmpDir, d.path)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create dir %s: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(dir, d.name), []byte(`{"type":"session_meta"}`+"\n"), 0o644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
	}

	agent := &Agent{
		config:     agents.AgentConfig{},
		cliPath:    "codex",
		sessionDir: tmpDir,
	}

	sessions, err := agent.DiscoverSessions(context.Background(), "")
	if err != nil {
		t.Fatalf("DiscoverSessions() error = %v", err)
	}

	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions from nested directories, got %d", len(sessions))
	}
}

func TestAgent_DiscoverSessions_SkipsNonJSONL(t *testing.T) {
	// Only .jsonl files should be returned, not other file types.
	tmpDir := t.TempDir()

	nestedDir := filepath.Join(tmpDir, "2025", "01", "15")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("failed to create nested dir: %v", err)
	}

	// Create a .jsonl file and a .txt file
	if err := os.WriteFile(filepath.Join(nestedDir, "session-abc.jsonl"), []byte(`{"type":"session_meta"}`+"\n"), 0o644); err != nil {
		t.Fatalf("failed to write session file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "notes.txt"), []byte("not a session"), 0o644); err != nil {
		t.Fatalf("failed to write txt file: %v", err)
	}

	agent := &Agent{
		config:     agents.AgentConfig{},
		cliPath:    "codex",
		sessionDir: tmpDir,
	}

	sessions, err := agent.DiscoverSessions(context.Background(), "")
	if err != nil {
		t.Fatalf("DiscoverSessions() error = %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session (only .jsonl), got %d", len(sessions))
	}
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
