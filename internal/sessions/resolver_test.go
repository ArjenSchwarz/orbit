package sessions

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

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
