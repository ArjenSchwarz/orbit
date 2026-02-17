package kiro

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
	"github.com/arjenschwarz/orbit/internal/agents/kiro/logs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite" // SQLite driver for tests
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
			options:  map[string]string{"model": "kiro-model"},
			wantFlag: true,
			wantVal:  "kiro-model",
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
		Options: map[string]string{"model": "kiro-model"},
	}).(*Agent)

	args := agent.buildArgs(agents.RunOptions{
		Prompt:    "Test prompt",
		SessionID: "test-session",
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

func TestAgent_DiscoverSessions_NoDB(t *testing.T) {
	agent := New(agents.AgentConfig{})

	// When Kiro DB doesn't exist (most test environments),
	// DiscoverSessions should return nil, nil (not an error)
	sessions, err := agent.DiscoverSessions(context.Background(), "/any/path")
	if err != nil {
		t.Errorf("DiscoverSessions() error = %v, expected nil for missing DB", err)
	}
	// Sessions may be nil (no DB) or empty (DB exists but no sessions for path)
	// Either is acceptable behavior
	_ = sessions
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

// Integration tests for DiscoverSessions with SQLite database

// createTestDB creates a temporary SQLite database with the Kiro schema for testing.
func createTestDB(t *testing.T) *logs.DB {
	t.Helper()

	tmpFile := filepath.Join(t.TempDir(), "test.db")

	conn, err := sql.Open("sqlite", tmpFile)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	_, err = conn.Exec(`
		CREATE TABLE conversations_v2 (
			key TEXT NOT NULL,
			conversation_id TEXT NOT NULL,
			value TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			PRIMARY KEY (key, conversation_id)
		)
	`)
	require.NoError(t, err)
	_ = conn.Close()

	return logs.NewTestDB(tmpFile)
}

// insertSession adds a test session to the database.
func insertSession(t *testing.T, db *logs.DB, dir, id, jsonValue string, created, updated time.Time) {
	t.Helper()

	// Open a connection to the test DB directly
	conn, err := sql.Open("sqlite", db.Path())
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	_, err = conn.Exec(
		"INSERT INTO conversations_v2 VALUES (?, ?, ?, ?, ?)",
		dir, id, jsonValue, created.UnixMilli(), updated.UnixMilli(),
	)
	require.NoError(t, err)
}

func TestAgent_DiscoverSessions_WithSQLite(t *testing.T) {
	// Create a test directory that we'll use as the project directory
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "test-project")
	err := os.Mkdir(projectDir, 0o755)
	require.NoError(t, err)

	// Create a test database with sessions
	db := createTestDB(t)

	// Resolve symlinks to match how the DB stores paths (e.g., /private/tmp on macOS)
	resolvedProjectDir, err := filepath.EvalSymlinks(projectDir)
	require.NoError(t, err)

	now := time.Now()
	insertSession(t, db, resolvedProjectDir, "session-1", `{"conversation_id":"session-1","history":[]}`, now.Add(-time.Hour), now)
	insertSession(t, db, resolvedProjectDir, "session-2", `{"conversation_id":"session-2","history":[]}`, now.Add(-2*time.Hour), now.Add(-30*time.Minute))
	insertSession(t, db, "/other/project", "session-3", `{"conversation_id":"session-3","history":[]}`, now, now)

	// Create an agent that uses the test database
	// We need to use the DB directly since the agent uses logs.DiscoverForDirectory
	sessions, err := db.DiscoverForDirectory(context.Background(), projectDir)
	require.NoError(t, err)
	require.Len(t, sessions, 2)

	// Verify sessions are converted to SessionInfo format correctly
	agent := New(agents.AgentConfig{}).(*Agent)

	// Convert log sessions to agent sessions (simulating what DiscoverSessions does)
	result := make([]agents.SessionInfo, len(sessions))
	for i, s := range sessions {
		result[i] = agents.SessionInfo{
			ID:        s.ConversationID,
			Agent:     agent.Name(),
			Path:      "",
			CreatedAt: s.CreatedAt,
			Size:      s.Size,
			Project:   s.Directory,
		}
	}

	// Verify the conversion
	assert.Len(t, result, 2)
	assert.Equal(t, "kiro", result[0].Agent)
	assert.Equal(t, "", result[0].Path) // Sessions are in SQLite, not filesystem
	assert.NotEmpty(t, result[0].ID)
	assert.NotZero(t, result[0].Size)

	// Sessions should be ordered by updated_at DESC
	assert.Equal(t, "session-1", result[0].ID)
	assert.Equal(t, "session-2", result[1].ID)
}

func TestAgent_DiscoverSessions_EmptyResult(t *testing.T) {
	// Create a test database without sessions for the target directory
	db := createTestDB(t)

	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "empty-project")
	err := os.Mkdir(projectDir, 0o755)
	require.NoError(t, err)

	// Insert sessions for a different directory
	now := time.Now()
	insertSession(t, db, "/other/project", "session-1", `{}`, now, now)

	sessions, err := db.DiscoverForDirectory(context.Background(), projectDir)
	require.NoError(t, err)
	assert.Empty(t, sessions) // Empty slice, not nil
}

func TestAgent_DiscoverSessions_SessionInfoFields(t *testing.T) {
	// Verify all SessionInfo fields are populated correctly
	db := createTestDB(t)

	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "project")
	err := os.Mkdir(projectDir, 0o755)
	require.NoError(t, err)

	resolvedProjectDir, err := filepath.EvalSymlinks(projectDir)
	require.NoError(t, err)

	created := time.Date(2025, 1, 10, 10, 0, 0, 0, time.UTC)
	updated := time.Date(2025, 1, 15, 14, 30, 0, 0, time.UTC)
	jsonValue := `{"conversation_id":"test-123","history":[{"role":"user","content":"hello"}]}`
	insertSession(t, db, resolvedProjectDir, "test-123", jsonValue, created, updated)

	sessions, err := db.DiscoverForDirectory(context.Background(), projectDir)
	require.NoError(t, err)
	require.Len(t, sessions, 1)

	s := sessions[0]

	// Convert to SessionInfo format as the agent does
	info := agents.SessionInfo{
		ID:        s.ConversationID,
		Agent:     "kiro",
		Path:      "",
		CreatedAt: s.CreatedAt,
		Size:      s.Size,
		Project:   s.Directory,
	}

	assert.Equal(t, "test-123", info.ID)
	assert.Equal(t, "kiro", info.Agent)
	assert.Equal(t, "", info.Path) // SQLite sessions have no filesystem path
	assert.Equal(t, created.UnixMilli(), info.CreatedAt.UnixMilli())
	assert.Equal(t, int64(len(jsonValue)), info.Size)
	assert.Equal(t, resolvedProjectDir, info.Project)
}

