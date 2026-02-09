package sessions

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

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
