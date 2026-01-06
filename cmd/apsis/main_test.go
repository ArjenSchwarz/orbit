package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arjenschwarz/orbit/internal/claude"
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
			result := claude.BuildProjectPath(tt.path)
			if result != tt.expected {
				t.Errorf("claude.BuildProjectPath(%q) = %q, want %q", tt.path, result, tt.expected)
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
	err := convert(strings.NewReader(input), &output, "test-session-id", "md")
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
	err := convert(strings.NewReader(""), &output, "empty-session", "md")

	// Empty files now return an error during format detection
	if err == nil {
		t.Fatal("expected error for empty file")
	}
	if !strings.Contains(err.Error(), "empty file") {
		t.Errorf("expected error containing 'empty file', got: %v", err)
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

func TestConvert_HTMLFormat(t *testing.T) {
	input := `{"type":"user","timestamp":"2025-12-23T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"Hello, Claude!"}]}}
{"type":"assistant","timestamp":"2025-12-23T10:00:01Z","message":{"role":"assistant","content":[{"type":"text","text":"Hello! How can I help?"}]}}`

	var output bytes.Buffer
	err := convert(strings.NewReader(input), &output, "test-session-id", "html")
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}

	result := output.String()

	// Check HTML structure
	if !strings.Contains(result, "<!DOCTYPE html>") {
		t.Error("output should contain DOCTYPE")
	}
	if !strings.Contains(result, "<title>Session Transcript</title>") {
		t.Error("output should contain title")
	}
	if !strings.Contains(result, `<code>test-session-id</code>`) {
		t.Error("output should contain session ID")
	}

	// Check user message
	if !strings.Contains(result, `class="message user"`) {
		t.Error("output should contain user message class")
	}
	if !strings.Contains(result, "Hello, Claude!") {
		t.Error("output should contain user message text")
	}

	// Check assistant message
	if !strings.Contains(result, `class="message assistant"`) {
		t.Error("output should contain assistant message class")
	}
	if !strings.Contains(result, "Hello! How can I help?") {
		t.Error("output should contain assistant message text")
	}
}

func TestConvert_UnsupportedFormat(t *testing.T) {
	input := `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"test"}]}}`

	var output bytes.Buffer
	err := convert(strings.NewReader(input), &output, "test-session", "xml")

	if err == nil {
		t.Fatal("expected error for unsupported format")
	}

	expectedMsg := "unsupported format: xml"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("expected error message to contain %q, got: %v", expectedMsg, err)
	}
}

func TestRun_ListWithPositionalArg(t *testing.T) {
	// Test that providing both --list and a positional argument returns an error
	cfg := &Config{
		List:  true,
		Input: "some-session-id",
	}

	err := run(cfg)
	if err == nil {
		t.Fatal("expected error when both --list and positional argument provided")
	}

	expectedMsg := "cannot specify both --list and a positional argument"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("expected error message to contain %q, got: %v", expectedMsg, err)
	}
}

// --- Codex Session Discovery Tests ---

func TestFindCodexSession_UUIDMatching(t *testing.T) {
	// Create a temporary directory structure mimicking ~/.codex/sessions/
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, ".codex", "sessions", "2026", "01", "05")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a session file with UUID embedded in filename
	sessionUUID := "019b892c-3a14-7773-bd76-6465a8a0b634"
	filename := "rollout-2026-01-05T00-22-15-" + sessionUUID + ".jsonl"
	sessionFile := filepath.Join(sessionsDir, filename)
	content := `{"timestamp":"2026-01-05T00:22:15.725Z","type":"session_meta","payload":{"id":"019b892c-3a14-7773-bd76-6465a8a0b634"}}`
	if err := os.WriteFile(sessionFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Test finding session by UUID
	foundPath, err := findCodexSession(tmpDir, sessionUUID)
	if err != nil {
		t.Fatalf("findCodexSession failed: %v", err)
	}
	if foundPath == "" {
		t.Fatal("expected to find session but got empty path")
	}
	// Compare filenames since paths may differ due to symlink resolution (e.g., /var -> /private/var on macOS)
	if filepath.Base(foundPath) != filepath.Base(sessionFile) {
		t.Errorf("expected filename %q, got %q", filepath.Base(sessionFile), filepath.Base(foundPath))
	}
}

func TestFindCodexSession_CaseInsensitiveUUID(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, ".codex", "sessions", "2026", "01", "05")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a session file with lowercase UUID
	sessionUUID := "019b892c-3a14-7773-bd76-6465a8a0b634"
	filename := "session-" + sessionUUID + ".jsonl"
	sessionFile := filepath.Join(sessionsDir, filename)
	if err := os.WriteFile(sessionFile, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	// Search with uppercase UUID (should still match)
	foundPath, err := findCodexSession(tmpDir, strings.ToUpper(sessionUUID))
	if err != nil {
		t.Fatalf("findCodexSession failed: %v", err)
	}
	if foundPath == "" {
		t.Fatal("expected to find session with case-insensitive match")
	}
}

func TestFindCodexSession_DirectoryTraversal(t *testing.T) {
	tmpDir := t.TempDir()

	// Create nested date-based directory structure
	dates := []string{
		"2026/01/03",
		"2026/01/04",
		"2026/01/05",
	}
	for _, date := range dates {
		dir := filepath.Join(tmpDir, ".codex", "sessions", date)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Put session in the middle directory
	sessionUUID := "abcd1234-5678-90ab-cdef-1234567890ab"
	expectedFilename := "session-" + sessionUUID + ".jsonl"
	sessionFile := filepath.Join(tmpDir, ".codex", "sessions", "2026/01/04", expectedFilename)
	if err := os.WriteFile(sessionFile, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	foundPath, err := findCodexSession(tmpDir, sessionUUID)
	if err != nil {
		t.Fatalf("findCodexSession failed: %v", err)
	}
	// Compare filenames since paths may differ due to symlink resolution
	if filepath.Base(foundPath) != expectedFilename {
		t.Errorf("expected filename %q, got %q", expectedFilename, filepath.Base(foundPath))
	}
}

func TestFindCodexSession_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, ".codex", "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatal(err)
	}

	sessionUUID := "abcd1234-5678-90ab-cdef-1234567890ab"
	foundPath, err := findCodexSession(tmpDir, sessionUUID)
	if err != nil {
		t.Fatalf("findCodexSession should not error on empty directory: %v", err)
	}
	if foundPath != "" {
		t.Errorf("expected empty path for non-existent session, got %q", foundPath)
	}
}

func TestFindCodexSession_NonExistentDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	// Don't create .codex/sessions directory

	sessionUUID := "abcd1234-5678-90ab-cdef-1234567890ab"
	foundPath, err := findCodexSession(tmpDir, sessionUUID)
	if err != nil {
		t.Fatalf("findCodexSession should not error when .codex/sessions doesn't exist: %v", err)
	}
	if foundPath != "" {
		t.Errorf("expected empty path when directory doesn't exist, got %q", foundPath)
	}
}

func TestFindCodexSession_InvalidUUID(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, ".codex", "sessions", "2026", "01", "05")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a session file
	if err := os.WriteFile(filepath.Join(sessionsDir, "session.jsonl"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		sessionID string
	}{
		{"too short", "abc123"},
		{"no hyphens", "019b892c3a147773bd766465a8a0b634"},
		{"wrong length", "019b892c-3a14-7773-bd76-6465a8a0b634-extra"},
		{"non-hex characters", "019b892c-3a14-7773-bd76-gggggggggggg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			foundPath, err := findCodexSession(tmpDir, tt.sessionID)
			if err != nil {
				t.Fatalf("findCodexSession should not error for invalid UUID: %v", err)
			}
			if foundPath != "" {
				t.Errorf("expected empty path for invalid UUID %q, got %q", tt.sessionID, foundPath)
			}
		})
	}
}

func TestFindCodexSession_SymlinkFollowing(t *testing.T) {
	tmpDir := t.TempDir()

	// Create actual session directory elsewhere
	actualDir := filepath.Join(tmpDir, "actual-sessions", "2026", "01", "05")
	if err := os.MkdirAll(actualDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create session file in actual location
	sessionUUID := "019b892c-3a14-7773-bd76-6465a8a0b634"
	sessionFile := filepath.Join(actualDir, "session-"+sessionUUID+".jsonl")
	if err := os.WriteFile(sessionFile, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create .codex directory
	codexDir := filepath.Join(tmpDir, ".codex")
	if err := os.MkdirAll(codexDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create symlink from .codex/sessions to actual-sessions
	symlinkPath := filepath.Join(codexDir, "sessions")
	actualSessionsDir := filepath.Join(tmpDir, "actual-sessions")
	if err := os.Symlink(actualSessionsDir, symlinkPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	foundPath, err := findCodexSession(tmpDir, sessionUUID)
	if err != nil {
		t.Fatalf("findCodexSession failed: %v", err)
	}
	if foundPath == "" {
		t.Fatal("expected to find session through symlink")
	}
}

func TestFindCodexSession_CycleDetection(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, ".codex", "sessions", "2026", "01", "05")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a circular symlink: sessions/2026/01/05/loop -> sessions/2026
	loopTarget := filepath.Join(tmpDir, ".codex", "sessions", "2026")
	loopLink := filepath.Join(sessionsDir, "loop")
	if err := os.Symlink(loopTarget, loopLink); err != nil {
		t.Fatalf("failed to create circular symlink: %v", err)
	}

	// Create a session file to find
	sessionUUID := "019b892c-3a14-7773-bd76-6465a8a0b634"
	sessionFile := filepath.Join(sessionsDir, "session-"+sessionUUID+".jsonl")
	if err := os.WriteFile(sessionFile, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	// Should find the session without hanging in infinite loop
	foundPath, err := findCodexSession(tmpDir, sessionUUID)
	if err != nil {
		t.Fatalf("findCodexSession should handle circular symlinks: %v", err)
	}
	if foundPath == "" {
		t.Fatal("expected to find session despite circular symlink")
	}
}

func TestWalkDirFollowSymlinks_Basic(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directory structure
	if err := os.MkdirAll(filepath.Join(tmpDir, "a", "b"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create files
	files := []string{
		filepath.Join(tmpDir, "a", "file1.txt"),
		filepath.Join(tmpDir, "a", "b", "file2.txt"),
	}
	for _, f := range files {
		if err := os.WriteFile(f, []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Walk and collect files
	var found []string
	err := walkDirFollowSymlinks(tmpDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walkDirFollowSymlinks failed: %v", err)
	}

	if len(found) != 2 {
		t.Errorf("expected 2 files, found %d: %v", len(found), found)
	}
}

func TestWalkDirFollowSymlinks_SymlinkToFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create actual file
	actualFile := filepath.Join(tmpDir, "actual.txt")
	if err := os.WriteFile(actualFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create subdirectory with symlink to file
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	linkFile := filepath.Join(subDir, "link.txt")
	if err := os.Symlink(actualFile, linkFile); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	// Walk and verify symlink to file is visited
	var found []string
	err := walkDirFollowSymlinks(tmpDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && strings.HasSuffix(path, ".txt") {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walkDirFollowSymlinks failed: %v", err)
	}

	// Should find both the actual file and the resolved symlink target
	if len(found) < 1 {
		t.Errorf("expected at least 1 file, found %d: %v", len(found), found)
	}
}

// --- Codex Session Timestamp Tests ---

func TestGetCodexSessionTimestamp_FromSessionMeta(t *testing.T) {
	tmpDir := t.TempDir()
	sessionFile := filepath.Join(tmpDir, "session.jsonl")

	// Create a Codex session file with session_meta containing timestamp
	content := `{"timestamp":"2026-01-05T00:22:15.725Z","type":"session_meta","payload":{"id":"019b892c-3a14-7773-bd76-6465a8a0b634"}}
{"timestamp":"2026-01-05T00:22:16.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Hello"}]}}`
	if err := os.WriteFile(sessionFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	timestamp, err := getCodexSessionTimestamp(sessionFile)
	if err != nil {
		t.Fatalf("getCodexSessionTimestamp failed: %v", err)
	}

	expected := time.Date(2026, 1, 5, 0, 22, 15, 725000000, time.UTC)
	if !timestamp.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, timestamp)
	}
}

func TestGetCodexSessionTimestamp_FallbackToMtime(t *testing.T) {
	tmpDir := t.TempDir()
	sessionFile := filepath.Join(tmpDir, "session.jsonl")

	// Create a file that's not a valid Codex session (no session_meta)
	content := `{"timestamp":"2026-01-05T00:22:15.725Z","type":"response_item","payload":{"type":"message","role":"user"}}`
	if err := os.WriteFile(sessionFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Get file modification time for comparison
	info, err := os.Stat(sessionFile)
	if err != nil {
		t.Fatal(err)
	}
	expectedMtime := info.ModTime()

	timestamp, err := getCodexSessionTimestamp(sessionFile)
	if err != nil {
		t.Fatalf("getCodexSessionTimestamp failed: %v", err)
	}

	// Should fall back to file modification time
	if !timestamp.Equal(expectedMtime) {
		t.Errorf("expected mtime %v, got %v", expectedMtime, timestamp)
	}
}

func TestGetCodexSessionTimestamp_InvalidTimestamp(t *testing.T) {
	tmpDir := t.TempDir()
	sessionFile := filepath.Join(tmpDir, "session.jsonl")

	// Create a session_meta with invalid timestamp format
	content := `{"timestamp":"not-a-valid-timestamp","type":"session_meta","payload":{"id":"test"}}`
	if err := os.WriteFile(sessionFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Get file modification time for comparison
	info, err := os.Stat(sessionFile)
	if err != nil {
		t.Fatal(err)
	}
	expectedMtime := info.ModTime()

	timestamp, err := getCodexSessionTimestamp(sessionFile)
	if err != nil {
		t.Fatalf("getCodexSessionTimestamp should fall back to mtime: %v", err)
	}

	// Should fall back to file modification time
	if !timestamp.Equal(expectedMtime) {
		t.Errorf("expected mtime %v, got %v", expectedMtime, timestamp)
	}
}

func TestGetCodexSessionTimestamp_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	sessionFile := filepath.Join(tmpDir, "session.jsonl")

	// Create an empty file
	if err := os.WriteFile(sessionFile, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	// Get file modification time for comparison
	info, err := os.Stat(sessionFile)
	if err != nil {
		t.Fatal(err)
	}
	expectedMtime := info.ModTime()

	timestamp, err := getCodexSessionTimestamp(sessionFile)
	if err != nil {
		t.Fatalf("getCodexSessionTimestamp should fall back to mtime: %v", err)
	}

	// Should fall back to file modification time
	if !timestamp.Equal(expectedMtime) {
		t.Errorf("expected mtime %v, got %v", expectedMtime, timestamp)
	}
}

// --- Unified Session Listing Tests ---

func TestListCodexSessions_Basic(t *testing.T) {
	tmpDir := t.TempDir()

	// Create Codex sessions directory structure
	sessionsDir := filepath.Join(tmpDir, ".codex", "sessions", "2026", "01", "05")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create two Codex session files with different timestamps
	session1 := `{"timestamp":"2026-01-05T10:00:00Z","type":"session_meta","payload":{"id":"019b892c-3a14-7773-bd76-6465a8a0b634"}}`
	session2 := `{"timestamp":"2026-01-05T09:00:00Z","type":"session_meta","payload":{"id":"019b892c-3a14-7773-bd76-6465a8a0b635"}}`

	sessionFile1 := filepath.Join(sessionsDir, "session-019b892c-3a14-7773-bd76-6465a8a0b634.jsonl")
	sessionFile2 := filepath.Join(sessionsDir, "session-019b892c-3a14-7773-bd76-6465a8a0b635.jsonl")

	if err := os.WriteFile(sessionFile1, []byte(session1), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sessionFile2, []byte(session2), 0644); err != nil {
		t.Fatal(err)
	}

	sessions, err := listCodexSessions(tmpDir)
	if err != nil {
		t.Fatalf("listCodexSessions failed: %v", err)
	}

	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	// Verify source is set correctly
	for _, s := range sessions {
		if s.Source != "codex" {
			t.Errorf("expected source 'codex', got %q", s.Source)
		}
	}
}

func TestListCodexSessions_NonExistentDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	// Don't create .codex/sessions

	sessions, err := listCodexSessions(tmpDir)
	if err != nil {
		t.Fatalf("listCodexSessions should not error when directory doesn't exist: %v", err)
	}

	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions when directory doesn't exist, got %d", len(sessions))
	}
}

func TestUnifiedSessionListing_SortByTimestamp(t *testing.T) {
	// Create session infos with different timestamps
	sessions := []SessionInfo{
		{ID: "session-1", CreatedAt: time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC), Source: "claude"},
		{ID: "session-2", CreatedAt: time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC), Source: "codex"},
		{ID: "session-3", CreatedAt: time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC), Source: "claude"},
		{ID: "session-4", CreatedAt: time.Date(2026, 1, 5, 6, 0, 0, 0, time.UTC), Source: "codex"},
	}

	// Sort by timestamp (oldest first), Claude first for ties
	sortSessionsByTimestamp(sessions)

	// Expected order: session-4 (6:00), session-2 (8:00), session-1 (10:00), session-3 (12:00)
	expectedOrder := []string{"session-4", "session-2", "session-1", "session-3"}
	for i, expected := range expectedOrder {
		if sessions[i].ID != expected {
			t.Errorf("position %d: expected %s, got %s", i, expected, sessions[i].ID)
		}
	}
}

func TestUnifiedSessionListing_ClaudeFirstTieBreaking(t *testing.T) {
	// Create session infos with identical timestamps but different sources
	sameTime := time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC)
	sessions := []SessionInfo{
		{ID: "codex-session", CreatedAt: sameTime, Source: "codex"},
		{ID: "claude-session", CreatedAt: sameTime, Source: "claude"},
	}

	// Sort by timestamp with Claude first for ties
	sortSessionsByTimestamp(sessions)

	// Claude session should come first
	if sessions[0].ID != "claude-session" {
		t.Errorf("expected claude-session first, got %s", sessions[0].ID)
	}
	if sessions[1].ID != "codex-session" {
		t.Errorf("expected codex-session second, got %s", sessions[1].ID)
	}
}

func TestUnifiedSessionListing_MergeClaudeAndCodex(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")

	// Create Claude sessions directory
	claudeProjectDir := filepath.Join(homeDir, ".claude", "projects", "-test-project")
	if err := os.MkdirAll(claudeProjectDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create Codex sessions directory
	codexSessionsDir := filepath.Join(homeDir, ".codex", "sessions", "2026", "01", "05")
	if err := os.MkdirAll(codexSessionsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create Claude session
	claudeSession := `{"type":"user","timestamp":"2026-01-05T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"hello"}]}}`
	if err := os.WriteFile(filepath.Join(claudeProjectDir, "claude-session.jsonl"), []byte(claudeSession), 0644); err != nil {
		t.Fatal(err)
	}

	// Create Codex session
	codexSession := `{"timestamp":"2026-01-05T09:00:00Z","type":"session_meta","payload":{"id":"019b892c-3a14-7773-bd76-6465a8a0b634"}}`
	if err := os.WriteFile(filepath.Join(codexSessionsDir, "session-019b892c-3a14-7773-bd76-6465a8a0b634.jsonl"), []byte(codexSession), 0644); err != nil {
		t.Fatal(err)
	}

	// Override home directory for testing
	origHomeDir := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", origHomeDir) }()

	// Test listAllSessions which merges both sources
	sessions, err := listAllSessions("/test/project")
	if err != nil {
		t.Fatalf("listAllSessions failed: %v", err)
	}

	// Should have sessions from both sources
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions (1 Claude + 1 Codex), got %d", len(sessions))
	}

	// Verify we have both sources
	sources := make(map[string]bool)
	for _, s := range sessions {
		sources[s.Source] = true
	}

	if !sources["claude"] {
		t.Error("expected to find a claude session")
	}
	if !sources["codex"] {
		t.Error("expected to find a codex session")
	}
}

func TestUnifiedSessionListing_OnlyClaudeAvailable(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")

	// Create only Claude sessions directory (no Codex)
	claudeProjectDir := filepath.Join(homeDir, ".claude", "projects", "-test-project")
	if err := os.MkdirAll(claudeProjectDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create Claude session
	claudeSession := `{"type":"user","timestamp":"2026-01-05T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"hello"}]}}`
	if err := os.WriteFile(filepath.Join(claudeProjectDir, "claude-session.jsonl"), []byte(claudeSession), 0644); err != nil {
		t.Fatal(err)
	}

	// Override home directory for testing
	origHomeDir := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", origHomeDir) }()

	// Should still work with only Claude sessions
	sessions, err := listAllSessions("/test/project")
	if err != nil {
		t.Fatalf("listAllSessions failed: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Source != "claude" {
		t.Errorf("expected source 'claude', got %q", sessions[0].Source)
	}
}

func TestUnifiedSessionListing_OnlyCodexAvailable(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")

	// Create only Codex sessions directory (no Claude project)
	codexSessionsDir := filepath.Join(homeDir, ".codex", "sessions", "2026", "01", "05")
	if err := os.MkdirAll(codexSessionsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create Codex session
	codexSession := `{"timestamp":"2026-01-05T09:00:00Z","type":"session_meta","payload":{"id":"019b892c-3a14-7773-bd76-6465a8a0b634"}}`
	if err := os.WriteFile(filepath.Join(codexSessionsDir, "session-019b892c-3a14-7773-bd76-6465a8a0b634.jsonl"), []byte(codexSession), 0644); err != nil {
		t.Fatal(err)
	}

	// Override home directory for testing
	origHomeDir := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", origHomeDir) }()

	// Should still work with only Codex sessions
	sessions, err := listAllSessions("/test/project")
	if err != nil {
		t.Fatalf("listAllSessions failed: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Source != "codex" {
		t.Errorf("expected source 'codex', got %q", sessions[0].Source)
	}
}

// --- resolveInput Tests for Dual-Location Checking ---

func TestResolveInput_ClaudeSessionByID(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")

	// Create Claude sessions directory
	claudeProjectDir := filepath.Join(homeDir, ".claude", "projects", "-test-project")
	if err := os.MkdirAll(claudeProjectDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create Claude session file
	sessionID := "test-claude-session"
	sessionFile := filepath.Join(claudeProjectDir, sessionID+".jsonl")
	content := `{"type":"user","timestamp":"2026-01-05T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"hello"}]}}`
	if err := os.WriteFile(sessionFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Override home directory for testing
	origHomeDir := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", origHomeDir) }()

	reader, resolvedID, err := resolveInput(sessionID, "/test/project")
	if err != nil {
		t.Fatalf("resolveInput failed: %v", err)
	}
	defer func() { _ = reader.Close() }()

	if resolvedID != sessionID {
		t.Errorf("expected session ID %q, got %q", sessionID, resolvedID)
	}
}

func TestResolveInput_CodexSessionByUUID(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")

	// Create Codex sessions directory
	codexSessionsDir := filepath.Join(homeDir, ".codex", "sessions", "2026", "01", "05")
	if err := os.MkdirAll(codexSessionsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create Codex session file
	sessionUUID := "019b892c-3a14-7773-bd76-6465a8a0b634"
	sessionFile := filepath.Join(codexSessionsDir, "session-"+sessionUUID+".jsonl")
	content := `{"timestamp":"2026-01-05T09:00:00Z","type":"session_meta","payload":{"id":"019b892c-3a14-7773-bd76-6465a8a0b634"}}`
	if err := os.WriteFile(sessionFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Override home directory for testing
	origHomeDir := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", origHomeDir) }()

	reader, resolvedID, err := resolveInput(sessionUUID, "/test/project")
	if err != nil {
		t.Fatalf("resolveInput failed: %v", err)
	}
	defer func() { _ = reader.Close() }()

	if resolvedID != sessionUUID {
		t.Errorf("expected session ID %q, got %q", sessionUUID, resolvedID)
	}
}

func TestResolveInput_ClaudeFirst_WhenBothExist(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")

	// Create both Claude and Codex sessions with same ID (UUID format)
	sessionID := "019b892c-3a14-7773-bd76-6465a8a0b634"

	// Create Claude session
	claudeProjectDir := filepath.Join(homeDir, ".claude", "projects", "-test-project")
	if err := os.MkdirAll(claudeProjectDir, 0755); err != nil {
		t.Fatal(err)
	}
	claudeSessionFile := filepath.Join(claudeProjectDir, sessionID+".jsonl")
	claudeContent := `{"type":"user","timestamp":"2026-01-05T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"claude session"}]}}`
	if err := os.WriteFile(claudeSessionFile, []byte(claudeContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create Codex session
	codexSessionsDir := filepath.Join(homeDir, ".codex", "sessions", "2026", "01", "05")
	if err := os.MkdirAll(codexSessionsDir, 0755); err != nil {
		t.Fatal(err)
	}
	codexSessionFile := filepath.Join(codexSessionsDir, "session-"+sessionID+".jsonl")
	codexContent := `{"timestamp":"2026-01-05T09:00:00Z","type":"session_meta","payload":{"id":"019b892c-3a14-7773-bd76-6465a8a0b634"}}`
	if err := os.WriteFile(codexSessionFile, []byte(codexContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Override home directory for testing
	origHomeDir := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", origHomeDir) }()

	reader, _, err := resolveInput(sessionID, "/test/project")
	if err != nil {
		t.Fatalf("resolveInput failed: %v", err)
	}
	defer func() { _ = reader.Close() }()

	// Read content to verify it's the Claude session
	buf := make([]byte, 200)
	n, _ := reader.Read(buf)
	content := string(buf[:n])

	if !strings.Contains(content, "claude session") {
		t.Errorf("expected Claude session content, got: %s", content)
	}
}

func TestResolveInput_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")

	// Create empty directories (no sessions)
	if err := os.MkdirAll(filepath.Join(homeDir, ".claude", "projects", "-test-project"), 0755); err != nil {
		t.Fatal(err)
	}

	// Override home directory for testing
	origHomeDir := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", origHomeDir) }()

	_, _, err := resolveInput("nonexistent-session", "/test/project")
	if err == nil {
		t.Fatal("expected error for non-existent session")
	}
	if !strings.Contains(err.Error(), "session not found") {
		t.Errorf("expected 'session not found' error, got: %v", err)
	}
}
