package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsFilePath(t *testing.T) {
	tests := []struct {
		name     string
		arg      string
		expected bool
	}{
		{
			name:     "path with forward slash",
			arg:      "/path/to/session.jsonl",
			expected: true,
		},
		{
			name:     "path with backslash",
			arg:      "path\\to\\session.jsonl",
			expected: true,
		},
		{
			name:     "file ending in .jsonl",
			arg:      "session.jsonl",
			expected: true,
		},
		{
			name:     "session ID format (UUID-like)",
			arg:      "550e8400-e29b-41d4-a716-446655440000",
			expected: false,
		},
		{
			name:     "simple session ID",
			arg:      "my-session-id",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isFilePath(tt.arg)
			if result != tt.expected {
				t.Errorf("isFilePath(%q) = %v, want %v", tt.arg, result, tt.expected)
			}
		})
	}
}

func TestIsFilePath_ExistingFile(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "existing-session")
	if err := os.WriteFile(tmpFile, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	// Test that existing file is detected as file path
	if !isFilePath(tmpFile) {
		t.Errorf("isFilePath should return true for existing file: %s", tmpFile)
	}
}

func TestBuildClaudePath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "unix path with leading slash",
			path:     "/Users/foo/project",
			expected: "-Users-foo-project",
		},
		{
			name:     "unix path nested",
			path:     "/home/user/dev/my-app",
			expected: "-home-user-dev-my-app",
		},
		{
			name:     "path without leading slash",
			path:     "Users/foo/project",
			expected: "Users-foo-project",
		},
		{
			name:     "windows path with backslash prefix",
			path:     "\\Users\\foo\\project",
			expected: "-Users-foo-project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildClaudePath(tt.path)
			if result != tt.expected {
				t.Errorf("buildClaudePath(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestListSessions(t *testing.T) {
	// Create a temporary directory structure
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	// Claude stores project paths with leading dash: /test/project -> -test-project
	projectsDir := filepath.Join(homeDir, ".claude", "projects", "-test-project")

	if err := os.MkdirAll(projectsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create sample session files
	session1 := `{"type":"user","timestamp":"2025-12-23T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"hello"}]}}`
	session2 := `{"type":"user","timestamp":"2025-12-23T09:00:00Z","message":{"role":"user","content":[{"type":"text","text":"earlier"}]}}`

	if err := os.WriteFile(filepath.Join(projectsDir, "session-1.jsonl"), []byte(session1), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectsDir, "session-2.jsonl"), []byte(session2), 0644); err != nil {
		t.Fatal(err)
	}

	// Override home directory for testing
	origHomeDir := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", origHomeDir) }()

	// Test listSessions - we can't easily capture output, so just verify no error
	err := listSessions("/test/project")
	if err != nil {
		t.Errorf("listSessions failed: %v", err)
	}
}

func TestConvert(t *testing.T) {
	input := `{"type":"user","timestamp":"2025-12-23T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"Hello, Claude!"}]}}
{"type":"assistant","timestamp":"2025-12-23T10:00:01Z","message":{"role":"assistant","content":[{"type":"text","text":"Hello! How can I help?"}]}}`

	var output bytes.Buffer
	err := convert(strings.NewReader(input), &output, "test-session-id")
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}

	result := output.String()

	// Check header
	if !strings.Contains(result, "# Session Transcript") {
		t.Error("output should contain Session Transcript header")
	}

	// Check session ID
	if !strings.Contains(result, "`test-session-id`") {
		t.Error("output should contain session ID")
	}

	// Check user message
	if !strings.Contains(result, "## 👤 User") {
		t.Error("output should contain User heading")
	}
	if !strings.Contains(result, "Hello, Claude!") {
		t.Error("output should contain user message text")
	}

	// Check assistant message
	if !strings.Contains(result, "## 🤖 Assistant") {
		t.Error("output should contain Assistant heading")
	}
	if !strings.Contains(result, "Hello! How can I help?") {
		t.Error("output should contain assistant message text")
	}
}

func TestConvert_EmptyFile(t *testing.T) {
	var output bytes.Buffer
	err := convert(strings.NewReader(""), &output, "empty-session")
	if err != nil {
		t.Fatalf("convert failed on empty file: %v", err)
	}

	// Output should be empty (only message written to stderr)
	if output.Len() > 0 {
		t.Error("output should be empty for empty file")
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{bytes: 500, expected: "500 B"},
		{bytes: 1024, expected: "1.0 KB"},
		{bytes: 1536, expected: "1.5 KB"},
		{bytes: 1048576, expected: "1.0 MB"},
		{bytes: 1572864, expected: "1.5 MB"},
		{bytes: 1073741824, expected: "1.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatSize(tt.bytes)
			if result != tt.expected {
				t.Errorf("formatSize(%d) = %q, want %q", tt.bytes, result, tt.expected)
			}
		})
	}
}

func TestResolveInput_Stdin(t *testing.T) {
	// When arg is empty, should return stdin
	reader, sessionID, err := resolveInput("", "/some/project")
	if err != nil {
		t.Fatalf("resolveInput failed: %v", err)
	}

	if reader != os.Stdin {
		t.Error("expected stdin when arg is empty")
	}

	if sessionID != "" {
		t.Errorf("expected empty session ID, got %q", sessionID)
	}
}

func TestResolveInput_FilePath(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test-session.jsonl")
	content := `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"test"}]}}`
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	reader, sessionID, err := resolveInput(tmpFile, "/some/project")
	if err != nil {
		t.Fatalf("resolveInput failed: %v", err)
	}
	defer func() { _ = reader.Close() }()

	if sessionID != "test-session" {
		t.Errorf("expected session ID 'test-session', got %q", sessionID)
	}
}
