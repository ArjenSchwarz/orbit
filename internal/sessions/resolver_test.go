package sessions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/arjenschwarz/orbit/internal/agents/claudecode"
)

// newTestResolver creates a Resolver with custom homeDir for testing.
func newTestResolver(projectPath, homeDir string) *Resolver {
	return &Resolver{projectPath: projectPath, homeDir: homeDir}
}

func TestResolveClaudeSession(t *testing.T) {
	homeDir := t.TempDir()
	projectPath := t.TempDir()

	sessionID := "test-session-123"
	createdAt := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)

	// Create mock session file
	claudeProjectPath := claudecode.BuildProjectPath(projectPath)
	dir := filepath.Join(homeDir, ".claude", "projects", claudeProjectPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}

	entry := map[string]any{
		"type":      "system",
		"timestamp": createdAt.Format(time.RFC3339),
		"message":   "test content",
	}
	data, _ := json.Marshal(entry)
	content := append(data, '\n')
	filePath := filepath.Join(dir, sessionID+".jsonl")
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	resolver := newTestResolver(projectPath, homeDir)
	resolved, err := resolver.Resolve(SourceClaude, sessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = resolved.Reader.Close() }()

	// Verify metadata
	if resolved.Metadata.Source != SourceClaude {
		t.Errorf("source = %q, want %q", resolved.Metadata.Source, SourceClaude)
	}
	if resolved.Metadata.ID != sessionID {
		t.Errorf("id = %q, want %q", resolved.Metadata.ID, sessionID)
	}
	if resolved.Metadata.Size != int64(len(content)) {
		t.Errorf("size = %d, want %d", resolved.Metadata.Size, len(content))
	}
	if resolved.Metadata.CostPath != "" {
		t.Errorf("costPath should be empty for Claude, got %q", resolved.Metadata.CostPath)
	}
	if resolved.Metadata.CreatedAt.IsZero() {
		t.Error("createdAt should be populated for file-backed sessions")
	}

	// Verify reader content
	readData, err := io.ReadAll(resolved.Reader)
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}
	if string(readData) != string(content) {
		t.Errorf("content mismatch")
	}
}

// TestResolveClaudeSessionEmptyProjectPath verifies that when projectPath is
// empty, the resolver searches all project subdirectories under ~/.claude/projects/
// to find the session (matching the lister's all-project behaviour).
func TestResolveClaudeSessionEmptyProjectPath(t *testing.T) {
	homeDir := t.TempDir()

	sessionID := "session-in-subproject"

	// Create a session file in an arbitrary project subdirectory.
	projectDir := filepath.Join(homeDir, ".claude", "projects", "some-hashed-project-dir")
	require.NoError(t, os.MkdirAll(projectDir, 0755))

	entry := map[string]any{"type": "system", "timestamp": "2025-01-15T10:00:00Z", "message": "test"}
	data, _ := json.Marshal(entry)
	content := append(data, '\n')
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, sessionID+".jsonl"), content, 0644))

	// Resolver with empty projectPath should find the session.
	resolver := newTestResolver("", homeDir)

	resolved, err := resolver.Resolve(SourceClaude, sessionID)
	require.NoError(t, err)
	defer func() { _ = resolved.Reader.Close() }()

	assert.Equal(t, SourceClaude, resolved.Metadata.Source)
	assert.Equal(t, sessionID, resolved.Metadata.ID)

	// ResolvePath should also work.
	path, err := resolver.ResolvePath(SourceClaude, sessionID)
	require.NoError(t, err)
	assert.Contains(t, path, sessionID+".jsonl")
}

// TestResolveClaudeSessionEmptyProjectPathNotFound verifies that when projectPath
// is empty and the session doesn't exist in any subdirectory, an error is returned.
func TestResolveClaudeSessionEmptyProjectPathNotFound(t *testing.T) {
	homeDir := t.TempDir()

	// Create the projects root with one subdirectory but no matching session.
	projectDir := filepath.Join(homeDir, ".claude", "projects", "some-project")
	require.NoError(t, os.MkdirAll(projectDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "other-session.jsonl"), []byte("{}"), 0644))

	resolver := newTestResolver("", homeDir)
	_, err := resolver.Resolve(SourceClaude, "nonexistent-session")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")
}

func TestResolveUnknownSource(t *testing.T) {
	resolver := newTestResolver(t.TempDir(), t.TempDir())
	_, err := resolver.Resolve("unknown-agent", "some-id")
	if err == nil {
		t.Fatal("expected error for unknown source")
	}
}

func TestResolveNonExistentSession(t *testing.T) {
	homeDir := t.TempDir()
	projectPath := t.TempDir()

	// Create the Claude projects directory but no session files
	claudeProjectPath := claudecode.BuildProjectPath(projectPath)
	dir := filepath.Join(homeDir, ".claude", "projects", claudeProjectPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}

	resolver := newTestResolver(projectPath, homeDir)
	_, err := resolver.Resolve(SourceClaude, "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent session")
	}
}

func TestResolvePathTraversal(t *testing.T) {
	homeDir := t.TempDir()
	projectPath := t.TempDir()

	// Create a file outside the Claude project dir
	outsideDir := filepath.Join(homeDir, "outside")
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outsideDir, "secret.jsonl"), []byte("secret"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	resolver := newTestResolver(projectPath, homeDir)
	// Attempt path traversal - this should fail because the path won't be within the base dir
	_, err := resolver.Resolve(SourceClaude, "../../outside/secret")
	if err == nil {
		t.Fatal("expected error for path traversal attempt")
	}
}

func TestResolveCodexSession(t *testing.T) {
	homeDir := t.TempDir()
	projectPath := t.TempDir()

	sessionID := "12345678-1234-1234-1234-123456789abc"
	createdAt := time.Date(2025, 1, 15, 14, 30, 0, 0, time.UTC)

	// Create Codex session file
	codexDir := filepath.Join(homeDir, ".codex", "sessions",
		createdAt.Format("2006"), createdAt.Format("01"), createdAt.Format("02"))
	if err := os.MkdirAll(codexDir, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}

	meta := map[string]any{
		"type":      "session_meta",
		"timestamp": createdAt.Format(time.RFC3339),
		"payload": map[string]any{
			"id":  sessionID,
			"cwd": projectPath,
		},
	}
	data, _ := json.Marshal(meta)
	content := append(data, '\n')
	filePath := filepath.Join(codexDir, fmt.Sprintf("session-%s.jsonl", sessionID))
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	resolver := newTestResolver(projectPath, homeDir)
	resolved, err := resolver.Resolve(SourceCodex, sessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = resolved.Reader.Close() }()

	if resolved.Metadata.Source != SourceCodex {
		t.Errorf("source = %q, want %q", resolved.Metadata.Source, SourceCodex)
	}
	if resolved.Metadata.ID != sessionID {
		t.Errorf("id = %q, want %q", resolved.Metadata.ID, sessionID)
	}
	if resolved.Metadata.Size == 0 {
		t.Error("size should be > 0")
	}
}

func TestResolveCodexInvalidUUID(t *testing.T) {
	resolver := newTestResolver(t.TempDir(), t.TempDir())
	_, err := resolver.Resolve(SourceCodex, "not-a-uuid")
	if err == nil {
		t.Fatal("expected error for invalid Codex UUID")
	}
}

// TestResolveCodexNonUUID verifies that Codex sessions whose filenames
// don't contain a UUID can still be resolved by the filename-based ID that
// listCodex returns (basename without .jsonl). Matching is case-insensitive
// to stay consistent with UUID normalisation.
func TestResolveCodexNonUUID(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		filename  string
		sessionID string
	}{
		"plain":  {"events.jsonl", "events"},
		"prefix": {"session-local.jsonl", "session-local"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			homeDir := t.TempDir()
			projectPath := t.TempDir()

			createdAt := time.Date(2025, 3, 10, 9, 0, 0, 0, time.UTC)
			codexDir := filepath.Join(homeDir, ".codex", "sessions",
				createdAt.Format("2006"), createdAt.Format("01"), createdAt.Format("02"))
			if err := os.MkdirAll(codexDir, 0755); err != nil {
				t.Fatalf("failed to create dir: %v", err)
			}

			meta := map[string]any{
				"type":      "session_meta",
				"timestamp": createdAt.Format(time.RFC3339),
				"payload": map[string]any{
					"id":  tc.sessionID,
					"cwd": projectPath,
				},
			}
			data, err := json.Marshal(meta)
			if err != nil {
				t.Fatalf("failed to marshal meta: %v", err)
			}
			filePath := filepath.Join(codexDir, tc.filename)
			if err := os.WriteFile(filePath, append(data, '\n'), 0644); err != nil {
				t.Fatalf("failed to write file: %v", err)
			}

			resolver := newTestResolver(projectPath, homeDir)
			resolved, err := resolver.Resolve(SourceCodex, tc.sessionID)
			if err != nil {
				t.Fatalf("expected non-UUID ID %q to resolve, got error: %v", tc.sessionID, err)
			}
			defer func() { _ = resolved.Reader.Close() }()

			if resolved.Metadata.Source != SourceCodex {
				t.Errorf("source = %q, want %q", resolved.Metadata.Source, SourceCodex)
			}
			if resolved.Metadata.ID != tc.sessionID {
				t.Errorf("id = %q, want %q", resolved.Metadata.ID, tc.sessionID)
			}
		})
	}
}

// TestResolveCodexNonUUIDPathTraversal ensures filename-based resolution does
// not allow path traversal (e.g., "../../etc/passwd" as session ID).
func TestResolveCodexNonUUIDPathTraversal(t *testing.T) {
	t.Parallel()
	homeDir := t.TempDir()
	resolver := newTestResolver(t.TempDir(), homeDir)
	_, err := resolver.Resolve(SourceCodex, "../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal attempt in non-UUID ID")
	}
}

func TestResolveCopilotSession(t *testing.T) {
	homeDir := t.TempDir()
	projectPath := t.TempDir()

	sessionID := "12345678-1234-1234-1234-123456789abc"
	sessionDir := filepath.Join(homeDir, ".copilot", "session-state", sessionID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("failed to create session dir: %v", err)
	}

	eventsData := `{"type":"event","data":"test"}` + "\n"
	if err := os.WriteFile(filepath.Join(sessionDir, "events.jsonl"), []byte(eventsData), 0644); err != nil {
		t.Fatalf("failed to write events file: %v", err)
	}

	yamlContent := fmt.Sprintf("id: %s\ncwd: %s\ncreated_at: 2025-01-15T14:30:00Z\n",
		sessionID, projectPath)
	if err := os.WriteFile(filepath.Join(sessionDir, "workspace.yaml"), []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write workspace file: %v", err)
	}

	resolver := newTestResolver(projectPath, homeDir)
	resolved, err := resolver.Resolve(SourceCopilot, sessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = resolved.Reader.Close() }()

	if resolved.Metadata.Source != SourceCopilot {
		t.Errorf("source = %q, want %q", resolved.Metadata.Source, SourceCopilot)
	}
	if resolved.Metadata.ID != sessionID {
		t.Errorf("id = %q, want %q", resolved.Metadata.ID, sessionID)
	}
}

func TestResolveInvalidSource(t *testing.T) {
	resolver := newTestResolver(t.TempDir(), t.TempDir())
	_, err := resolver.Resolve("invalid-source", "test-session")
	if err == nil {
		t.Fatal("expected error for invalid source")
	}
	assert.Contains(t, err.Error(), "unknown source", "error should mention unknown source, got: %v", err)
}

func TestResolveMetadataPopulation(t *testing.T) {
	homeDir := t.TempDir()
	projectPath := t.TempDir()

	sessionID := "test-session"
	createdAt := time.Date(2025, 1, 15, 14, 30, 0, 0, time.UTC)

	claudeProjectPath := claudecode.BuildProjectPath(projectPath)
	dir := filepath.Join(homeDir, ".claude", "projects", claudeProjectPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}

	entry := map[string]any{
		"type":      "system",
		"timestamp": createdAt.Format(time.RFC3339),
		"message":   "test",
	}
	data, _ := json.Marshal(entry)
	content := append(data, '\n')
	if err := os.WriteFile(filepath.Join(dir, sessionID+".jsonl"), content, 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	expectedSize := int64(len(content))

	resolver := newTestResolver(projectPath, homeDir)
	resolved, err := resolver.Resolve(SourceClaude, sessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = resolved.Reader.Close() }()

	if resolved.Metadata.Size != expectedSize {
		t.Errorf("size = %d, want %d", resolved.Metadata.Size, expectedSize)
	}
	if resolved.Metadata.CostPath != "" {
		t.Errorf("costPath should be empty for Claude, got %q", resolved.Metadata.CostPath)
	}

	// Verify reader works
	readData, err := io.ReadAll(resolved.Reader)
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}
	if len(readData) == 0 {
		t.Error("session data should not be empty")
	}
}

// TestResolvePathCodexSymlinkEscape verifies that ResolvePath for Codex rejects
// sessions found via symlinks that resolve outside ~/.codex/sessions/.
func TestResolvePathCodexSymlinkEscape(t *testing.T) {
	homeDir := t.TempDir()
	projectPath := t.TempDir()

	sessionID := "12345678-1234-1234-1234-123456789abc"

	// Create a session file outside the expected Codex base directory.
	outsideDir := filepath.Join(homeDir, "outside")
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatalf("create outside dir: %v", err)
	}
	outsideFile := filepath.Join(outsideDir, fmt.Sprintf("session-%s.jsonl", sessionID))
	meta := map[string]any{
		"type":      "session_meta",
		"timestamp": "2025-01-15T14:30:00Z",
		"payload": map[string]any{
			"id":  sessionID,
			"cwd": projectPath,
		},
	}
	data, _ := json.Marshal(meta)
	if err := os.WriteFile(outsideFile, append(data, '\n'), 0644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	// Create the Codex sessions directory and place a symlink inside it
	// that points to the outside directory.
	codexDir := filepath.Join(homeDir, ".codex", "sessions", "2025", "01", "15")
	if err := os.MkdirAll(codexDir, 0755); err != nil {
		t.Fatalf("create codex dir: %v", err)
	}
	symlinkPath := filepath.Join(codexDir, fmt.Sprintf("session-%s.jsonl", sessionID))
	if err := os.Symlink(outsideFile, symlinkPath); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	resolver := newTestResolver(projectPath, homeDir)
	_, err := resolver.ResolvePath(SourceCodex, sessionID)
	if err == nil {
		t.Fatal("expected error for Codex session via symlink escaping base dir, got nil")
	}
	assert.Contains(t, err.Error(), "session not found")
}

// TestResolvePathCopilotSymlinkEscape verifies that ResolvePath for Copilot rejects
// sessions where the events.jsonl file is a symlink resolving outside the
// expected base directory. The session directory is real but contains a symlinked
// events.jsonl pointing to an outside file.
func TestResolvePathCopilotSymlinkEscape(t *testing.T) {
	homeDir := t.TempDir()
	projectPath := t.TempDir()

	sessionID := "12345678-1234-1234-1234-123456789abc"

	// Create a real events file outside the expected Copilot base.
	outsideDir := filepath.Join(homeDir, "outside")
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatalf("create outside dir: %v", err)
	}
	outsideFile := filepath.Join(outsideDir, "events.jsonl")
	if err := os.WriteFile(outsideFile, []byte(`{"type":"event"}`+"\n"), 0644); err != nil {
		t.Fatalf("write events file: %v", err)
	}

	// Create the real session directory inside Copilot session-state, but
	// symlink events.jsonl to the outside file.
	copilotSessionDir := filepath.Join(homeDir, ".copilot", "session-state", sessionID)
	if err := os.MkdirAll(copilotSessionDir, 0755); err != nil {
		t.Fatalf("create copilot session dir: %v", err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(copilotSessionDir, "events.jsonl")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	resolver := newTestResolver(projectPath, homeDir)
	_, err := resolver.ResolvePath(SourceCopilot, sessionID)
	if err == nil {
		t.Fatal("expected error for Copilot session via symlink escaping base dir, got nil")
	}
	assert.Contains(t, err.Error(), "session not found")
}

// TestResolvePathKiroIDESymlinkEscape verifies that ResolvePath for Kiro IDE rejects
// .chat files that are symlinks resolving outside the workspace directory.
func TestResolvePathKiroIDESymlinkEscape(t *testing.T) {
	homeDir := t.TempDir()

	// We need to set up a fake Kiro IDE workspace directory.
	// Since KiroIDEWorkspaceDir uses a real base path we can't easily control,
	// we test via findKiroIDEPath directly which is the underlying function.

	workspaceDir := filepath.Join(homeDir, "workspace")
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		t.Fatalf("create workspace dir: %v", err)
	}

	sessionID := "test-execution-id"

	// Create the real .chat file outside the workspace directory.
	outsideDir := filepath.Join(homeDir, "outside")
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatalf("create outside dir: %v", err)
	}
	chatContent := map[string]any{
		"executionId": sessionID,
		"chat":        []any{map[string]string{"role": "human", "content": "hello"}},
	}
	data, _ := json.Marshal(chatContent)
	outsideFile := filepath.Join(outsideDir, "session.chat")
	if err := os.WriteFile(outsideFile, data, 0644); err != nil {
		t.Fatalf("write outside chat file: %v", err)
	}

	// Symlink from inside workspace to outside.
	if err := os.Symlink(outsideFile, filepath.Join(workspaceDir, "session.chat")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	resolver := newTestResolver(t.TempDir(), homeDir)
	_, err := resolver.findKiroIDEPath(workspaceDir, sessionID)
	if err == nil {
		t.Fatal("expected error for Kiro IDE session via symlink escaping workspace dir, got nil")
	}
	assert.Contains(t, err.Error(), "session not found")
}

// TestResolvePathCodexValid verifies that ResolvePath works for a normal Codex session.
func TestResolvePathCodexValid(t *testing.T) {
	homeDir := t.TempDir()
	projectPath := t.TempDir()

	sessionID := "12345678-1234-1234-1234-123456789abc"
	codexDir := filepath.Join(homeDir, ".codex", "sessions", "2025", "01", "15")
	if err := os.MkdirAll(codexDir, 0755); err != nil {
		t.Fatalf("create dir: %v", err)
	}

	meta := map[string]any{
		"type":      "session_meta",
		"timestamp": "2025-01-15T14:30:00Z",
		"payload":   map[string]any{"id": sessionID, "cwd": projectPath},
	}
	data, _ := json.Marshal(meta)
	filePath := filepath.Join(codexDir, fmt.Sprintf("session-%s.jsonl", sessionID))
	if err := os.WriteFile(filePath, append(data, '\n'), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	resolver := newTestResolver(projectPath, homeDir)
	path, err := resolver.ResolvePath(SourceCodex, sessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path == "" {
		t.Fatal("expected non-empty path")
	}
}

// TestResolvePathCopilotValid verifies that ResolvePath works for a normal Copilot session.
func TestResolvePathCopilotValid(t *testing.T) {
	homeDir := t.TempDir()
	projectPath := t.TempDir()

	sessionID := "12345678-1234-1234-1234-123456789abc"
	sessionDir := filepath.Join(homeDir, ".copilot", "session-state", sessionID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("create dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "events.jsonl"), []byte(`{"type":"event"}`+"\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	resolver := newTestResolver(projectPath, homeDir)
	path, err := resolver.ResolvePath(SourceCopilot, sessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path == "" {
		t.Fatal("expected non-empty path")
	}
}

func TestNewResolver(t *testing.T) {
	resolver, err := NewResolver("/test/project")
	if err != nil {
		t.Fatalf("NewResolver() returned error: %v", err)
	}
	if resolver == nil {
		t.Fatal("NewResolver() returned nil")
	}
	if resolver.projectPath != "/test/project" {
		t.Errorf("projectPath = %q, want %q", resolver.projectPath, "/test/project")
	}
	if resolver.homeDir == "" {
		t.Error("homeDir should not be empty")
	}
}

// writeKiroIDEChatFile creates a .chat file with the given executionID and message count.
func writeKiroIDEChatFile(t *testing.T, dir, filename, executionID string, msgCount int) string {
	t.Helper()
	msgs := make([]json.RawMessage, msgCount)
	for i := range msgs {
		msgs[i] = json.RawMessage(`{"role":"human","content":"msg"}`)
	}
	header := struct {
		ExecutionID string            `json:"executionId"`
		Chat        []json.RawMessage `json:"chat"`
	}{
		ExecutionID: executionID,
		Chat:        msgs,
	}
	data, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("failed to marshal chat header: %v", err)
	}
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write chat file: %v", err)
	}
	return path
}

func TestFindKiroIDEPath_RejectsSymlinkOutsideWorkspace(t *testing.T) {
	// Set up a workspace directory and an outside directory
	workspaceDir := t.TempDir()
	outsideDir := t.TempDir()

	sessionID := "exec-123"

	// Create a real .chat file outside the workspace
	writeKiroIDEChatFile(t, outsideDir, "outside.chat", sessionID, 3)

	// Create a symlink inside the workspace pointing to the outside file
	symlinkPath := filepath.Join(workspaceDir, "symlinked.chat")
	if err := os.Symlink(filepath.Join(outsideDir, "outside.chat"), symlinkPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	resolver := newTestResolver("/fake/project", t.TempDir())
	_, err := resolver.findKiroIDEPath(workspaceDir, sessionID)
	if err == nil {
		t.Fatal("expected error for symlinked .chat file pointing outside workspace, but got nil")
	}
	assert.Contains(t, err.Error(), "session not found",
		"error should indicate session not found for path traversal via symlink")
}

func TestFindKiroIDEPath_SymlinkDoesNotShadowLegitimateFile(t *testing.T) {
	// A symlink with more messages should not prevent a legitimate file
	// with fewer messages from being returned.
	workspaceDir := t.TempDir()
	outsideDir := t.TempDir()

	sessionID := "exec-789"

	// Create a legitimate file with 2 messages
	writeKiroIDEChatFile(t, workspaceDir, "legitimate.chat", sessionID, 2)

	// Create a symlink to an outside file with more messages (would win selection without validation)
	writeKiroIDEChatFile(t, outsideDir, "outside.chat", sessionID, 5)
	symlinkPath := filepath.Join(workspaceDir, "symlinked.chat")
	if err := os.Symlink(filepath.Join(outsideDir, "outside.chat"), symlinkPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	resolver := newTestResolver("/fake/project", t.TempDir())
	path, err := resolver.findKiroIDEPath(workspaceDir, sessionID)
	if err != nil {
		t.Fatalf("expected legitimate file to be found, got error: %v", err)
	}
	expected := filepath.Join(workspaceDir, "legitimate.chat")
	if path != expected {
		t.Errorf("path = %q, want %q (should return legitimate file, not symlink)", path, expected)
	}
}

func TestFindKiroIDEPath_AcceptsRegularFile(t *testing.T) {
	workspaceDir := t.TempDir()
	sessionID := "exec-456"

	writeKiroIDEChatFile(t, workspaceDir, "session.chat", sessionID, 2)

	resolver := newTestResolver("/fake/project", t.TempDir())
	path, err := resolver.findKiroIDEPath(workspaceDir, sessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Join(workspaceDir, "session.chat")
	if path != expected {
		t.Errorf("path = %q, want %q", path, expected)
	}
}

func TestFindKiroIDEPath_SessionNotFound(t *testing.T) {
	workspaceDir := t.TempDir()

	// Create a .chat file with a different execution ID
	writeKiroIDEChatFile(t, workspaceDir, "other.chat", "different-id", 1)

	resolver := newTestResolver("/fake/project", t.TempDir())
	_, err := resolver.findKiroIDEPath(workspaceDir, "nonexistent-session")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
	assert.Contains(t, err.Error(), "session not found")
}

func TestKiroIDECreatedAt_UsesStartTime(t *testing.T) {
	// Regression test for T-555: resolveKiroIDE should use metadata.startTime
	// for CreatedAt instead of file modTime.
	startTimeMs := int64(1741572000000) // 2025-03-10 06:00:00 UTC
	chatJSON := mustMarshal(t, map[string]any{
		"executionId": "test-exec",
		"chat":        []any{map[string]string{"role": "human", "content": "hello"}},
		"metadata":    map[string]any{"startTime": startTimeMs},
	})

	rs := bytes.NewReader(chatJSON)
	modTime := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC) // deliberately different

	got := kiroIDECreatedAt(rs, modTime)

	expected := time.UnixMilli(startTimeMs)
	require.Equal(t, expected, got, "CreatedAt should use metadata.startTime, not modTime")
}

func TestKiroIDECreatedAt_FallsBackToModTime(t *testing.T) {
	// When metadata is absent, CreatedAt should fall back to modTime.
	chatJSON := mustMarshal(t, map[string]any{
		"executionId": "test-exec",
		"chat":        []any{map[string]string{"role": "human", "content": "hello"}},
	})

	rs := bytes.NewReader(chatJSON)
	modTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	got := kiroIDECreatedAt(rs, modTime)

	require.Equal(t, modTime, got, "CreatedAt should fall back to modTime when startTime is absent")
}

func TestKiroIDECreatedAt_FallsBackWhenStartTimeZero(t *testing.T) {
	// When startTime is 0, CreatedAt should fall back to modTime.
	chatJSON := mustMarshal(t, map[string]any{
		"executionId": "test-exec",
		"chat":        []any{map[string]string{"role": "human", "content": "hello"}},
		"metadata":    map[string]any{"startTime": 0},
	})

	rs := bytes.NewReader(chatJSON)
	modTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	got := kiroIDECreatedAt(rs, modTime)

	require.Equal(t, modTime, got, "CreatedAt should fall back to modTime when startTime is 0")
}

func TestKiroIDECreatedAt_SeeksBackToStart(t *testing.T) {
	// After parsing, the reader must be seeked back to position 0
	// so the caller can still read the full file content.
	chatJSON := mustMarshal(t, map[string]any{
		"executionId": "test-exec",
		"chat":        []any{map[string]string{"role": "human", "content": "hello"}},
		"metadata":    map[string]any{"startTime": int64(1741572000000)},
	})

	rs := bytes.NewReader(chatJSON)
	modTime := time.Now()

	_ = kiroIDECreatedAt(rs, modTime)

	// Reader should be back at position 0
	pos, err := rs.Seek(0, io.SeekCurrent)
	require.NoError(t, err)
	require.Equal(t, int64(0), pos, "reader should be seeked back to start after kiroIDECreatedAt")
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}

// TestResolveFileCreatedAt_UsesTranscriptTimestamp verifies that the resolver
// derives CreatedAt from source-specific transcript metadata (matching the
// lister) rather than file modification time. Regression test for T-994.
func TestResolveFileCreatedAt_UsesTranscriptTimestamp(t *testing.T) {
	t.Run("claude", func(t *testing.T) {
		homeDir := t.TempDir()
		projectPath := t.TempDir()

		sessionID := "ts-test-claude"
		transcriptTime := time.Date(2025, 3, 10, 9, 0, 0, 0, time.UTC)

		claudeProjectPath := claudecode.BuildProjectPath(projectPath)
		dir := filepath.Join(homeDir, ".claude", "projects", claudeProjectPath)
		require.NoError(t, os.MkdirAll(dir, 0755))

		entry := map[string]any{
			"type":      "system",
			"timestamp": transcriptTime.Format(time.RFC3339),
			"message":   "init",
		}
		data, _ := json.Marshal(entry)
		filePath := filepath.Join(dir, sessionID+".jsonl")
		require.NoError(t, os.WriteFile(filePath, append(data, '\n'), 0644))

		// Set file mtime to a different time to prove we don't use it.
		differentTime := time.Date(2099, 12, 31, 23, 59, 0, 0, time.UTC)
		require.NoError(t, os.Chtimes(filePath, differentTime, differentTime))

		resolver := newTestResolver(projectPath, homeDir)
		resolved, err := resolver.Resolve(SourceClaude, sessionID)
		require.NoError(t, err)
		defer func() { _ = resolved.Reader.Close() }()

		assert.Equal(t, transcriptTime, resolved.Metadata.CreatedAt,
			"CreatedAt should come from transcript first entry, not file mtime")
	})

	t.Run("codex", func(t *testing.T) {
		homeDir := t.TempDir()
		projectPath := t.TempDir()

		sessionID := "12345678-aaaa-bbbb-cccc-123456789abc"
		metaTime := time.Date(2025, 2, 20, 15, 0, 0, 0, time.UTC)

		codexDir := filepath.Join(homeDir, ".codex", "sessions", "2025", "02", "20")
		require.NoError(t, os.MkdirAll(codexDir, 0755))

		meta := map[string]any{
			"type":      "session_meta",
			"timestamp": metaTime.Format(time.RFC3339),
			"payload":   map[string]any{"id": sessionID, "cwd": projectPath},
		}
		data, _ := json.Marshal(meta)
		filePath := filepath.Join(codexDir, fmt.Sprintf("session-%s.jsonl", sessionID))
		require.NoError(t, os.WriteFile(filePath, append(data, '\n'), 0644))

		differentTime := time.Date(2099, 12, 31, 23, 59, 0, 0, time.UTC)
		require.NoError(t, os.Chtimes(filePath, differentTime, differentTime))

		resolver := newTestResolver(projectPath, homeDir)
		resolved, err := resolver.Resolve(SourceCodex, sessionID)
		require.NoError(t, err)
		defer func() { _ = resolved.Reader.Close() }()

		assert.Equal(t, metaTime, resolved.Metadata.CreatedAt,
			"CreatedAt should come from session_meta timestamp, not file mtime")
	})

	t.Run("copilot", func(t *testing.T) {
		homeDir := t.TempDir()
		projectPath := t.TempDir()

		sessionID := "12345678-dddd-eeee-ffff-123456789abc"
		wsTime := time.Date(2025, 4, 5, 8, 30, 0, 0, time.UTC)

		sessionDir := filepath.Join(homeDir, ".copilot", "session-state", sessionID)
		require.NoError(t, os.MkdirAll(sessionDir, 0755))

		eventsPath := filepath.Join(sessionDir, "events.jsonl")
		require.NoError(t, os.WriteFile(eventsPath, []byte(`{"type":"event"}`+"\n"), 0644))

		yamlContent := fmt.Sprintf("id: %s\ncwd: %s\ncreated_at: %s\n",
			sessionID, projectPath, wsTime.Format(time.RFC3339))
		require.NoError(t, os.WriteFile(filepath.Join(sessionDir, "workspace.yaml"), []byte(yamlContent), 0644))

		differentTime := time.Date(2099, 12, 31, 23, 59, 0, 0, time.UTC)
		require.NoError(t, os.Chtimes(eventsPath, differentTime, differentTime))

		resolver := newTestResolver(projectPath, homeDir)
		resolved, err := resolver.Resolve(SourceCopilot, sessionID)
		require.NoError(t, err)
		defer func() { _ = resolved.Reader.Close() }()

		assert.Equal(t, wsTime, resolved.Metadata.CreatedAt,
			"CreatedAt should come from workspace.yaml created_at, not file mtime")
	})
}
