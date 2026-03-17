package copilot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

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

// setupCopilotSession creates a Copilot session directory with events.jsonl and workspace.yaml.
func setupCopilotSession(t *testing.T, homeDir, projectPath, sessionID string, createdAt time.Time) {
	t.Helper()
	sessionDir := filepath.Join(homeDir, ".copilot", "session-state", sessionID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("failed to create session dir: %v", err)
	}

	eventsData := `{"type":"event","data":"test"}` + "\n"
	if err := os.WriteFile(filepath.Join(sessionDir, "events.jsonl"), []byte(eventsData), 0644); err != nil {
		t.Fatalf("failed to write events file: %v", err)
	}

	yamlContent := fmt.Sprintf("id: %s\ncwd: %s\ngit_root: %s\ncreated_at: %s\n",
		sessionID, projectPath, projectPath, createdAt.Format(time.RFC3339))
	if err := os.WriteFile(filepath.Join(sessionDir, "workspace.yaml"), []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write workspace file: %v", err)
	}
}

// TestDiscoverSessions_FindsDirectorySessions verifies that DiscoverSessions
// correctly scans per-session directories containing events.jsonl and workspace.yaml,
// rather than skipping directories (the bug described in T-408).
func TestDiscoverSessions_FindsDirectorySessions(t *testing.T) {
	homeDir := t.TempDir()
	projectPath := t.TempDir()

	t1 := time.Date(2025, 1, 15, 14, 30, 0, 0, time.UTC)
	sessionID := "12345678-1234-1234-1234-123456789abc"

	setupCopilotSession(t, homeDir, projectPath, sessionID, t1)

	agent := &Agent{config: agents.AgentConfig{}, cliPath: "copilot", sessionDir: filepath.Join(homeDir, ".copilot", "session-state")}

	sessions, err := agent.DiscoverSessions(context.Background(), projectPath)
	if err != nil {
		t.Fatalf("DiscoverSessions() error = %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	s := sessions[0]
	if s.ID != sessionID {
		t.Errorf("session.ID = %q, want %q", s.ID, sessionID)
	}
	if s.Agent != "copilot" {
		t.Errorf("session.Agent = %q, want %q", s.Agent, "copilot")
	}
	if s.Size == 0 {
		t.Error("session.Size should be > 0")
	}
}

// TestDiscoverSessions_SkipsEmptyEvents verifies sessions with empty events.jsonl are excluded.
func TestDiscoverSessions_SkipsEmptyEvents(t *testing.T) {
	homeDir := t.TempDir()
	projectPath := t.TempDir()
	sessionDir := filepath.Join(homeDir, ".copilot", "session-state", "empty-session")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Empty events.jsonl
	if err := os.WriteFile(filepath.Join(sessionDir, "events.jsonl"), []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	yamlContent := fmt.Sprintf("id: empty-session\ncwd: %s\ngit_root: %s\n", projectPath, projectPath)
	if err := os.WriteFile(filepath.Join(sessionDir, "workspace.yaml"), []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	agent := &Agent{config: agents.AgentConfig{}, cliPath: "copilot", sessionDir: filepath.Join(homeDir, ".copilot", "session-state")}

	sessions, err := agent.DiscoverSessions(context.Background(), projectPath)
	if err != nil {
		t.Fatalf("DiscoverSessions() error = %v", err)
	}

	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions for empty events.jsonl, got %d", len(sessions))
	}
}

// TestDiscoverSessions_FiltersProjectDir verifies that only sessions matching
// the requested projectDir are returned.
func TestDiscoverSessions_FiltersProjectDir(t *testing.T) {
	homeDir := t.TempDir()
	projectA := t.TempDir()
	projectB := t.TempDir()

	t1 := time.Date(2025, 3, 1, 10, 0, 0, 0, time.UTC)
	setupCopilotSession(t, homeDir, projectA, "session-a", t1)
	setupCopilotSession(t, homeDir, projectB, "session-b", t1)

	agent := &Agent{config: agents.AgentConfig{}, cliPath: "copilot", sessionDir: filepath.Join(homeDir, ".copilot", "session-state")}

	sessionsA, err := agent.DiscoverSessions(context.Background(), projectA)
	if err != nil {
		t.Fatalf("DiscoverSessions() error = %v", err)
	}
	if len(sessionsA) != 1 {
		t.Fatalf("expected 1 session for projectA, got %d", len(sessionsA))
	}
	if sessionsA[0].ID != "session-a" {
		t.Errorf("session.ID = %q, want %q", sessionsA[0].ID, "session-a")
	}

	sessionsB, err := agent.DiscoverSessions(context.Background(), projectB)
	if err != nil {
		t.Fatalf("DiscoverSessions() error = %v", err)
	}
	if len(sessionsB) != 1 {
		t.Fatalf("expected 1 session for projectB, got %d", len(sessionsB))
	}
	if sessionsB[0].ID != "session-b" {
		t.Errorf("session.ID = %q, want %q", sessionsB[0].ID, "session-b")
	}
}

// TestDiscoverSessions_SkipsMissingEvents verifies sessions without events.jsonl are skipped.
func TestDiscoverSessions_SkipsMissingEvents(t *testing.T) {
	homeDir := t.TempDir()
	projectPath := t.TempDir()
	sessionDir := filepath.Join(homeDir, ".copilot", "session-state", "no-events")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Only workspace.yaml, no events.jsonl
	yamlContent := fmt.Sprintf("id: no-events\ncwd: %s\ngit_root: %s\n", projectPath, projectPath)
	if err := os.WriteFile(filepath.Join(sessionDir, "workspace.yaml"), []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	agent := &Agent{config: agents.AgentConfig{}, cliPath: "copilot", sessionDir: filepath.Join(homeDir, ".copilot", "session-state")}

	sessions, err := agent.DiscoverSessions(context.Background(), projectPath)
	if err != nil {
		t.Fatalf("DiscoverSessions() error = %v", err)
	}

	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions for missing events.jsonl, got %d", len(sessions))
	}
}

// TestDiscoverSessions_UsesCreatedAtFromWorkspace verifies that the CreatedAt
// field is populated from workspace.yaml when available.
func TestDiscoverSessions_UsesCreatedAtFromWorkspace(t *testing.T) {
	homeDir := t.TempDir()
	projectPath := t.TempDir()

	createdAt := time.Date(2025, 6, 15, 9, 30, 0, 0, time.UTC)
	setupCopilotSession(t, homeDir, projectPath, "ts-session", createdAt)

	agent := &Agent{config: agents.AgentConfig{}, cliPath: "copilot", sessionDir: filepath.Join(homeDir, ".copilot", "session-state")}

	sessions, err := agent.DiscoverSessions(context.Background(), projectPath)
	if err != nil {
		t.Fatalf("DiscoverSessions() error = %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	if !sessions[0].CreatedAt.Equal(createdAt) {
		t.Errorf("CreatedAt = %v, want %v", sessions[0].CreatedAt, createdAt)
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
