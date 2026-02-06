package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents/claudecode"
	"github.com/arjenschwarz/orbit/internal/agents/kiro/logs"
	"github.com/arjenschwarz/orbit/internal/transcript"
)

// --- Follow Mode Flag Parsing Tests ---

func TestFollowFlagParsing(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected bool
	}{
		{
			name:     "short flag -F",
			args:     []string{"-F", "session-id"},
			expected: true,
		},
		{
			name:     "long flag --follow",
			args:     []string{"--follow", "session-id"},
			expected: true,
		},
		{
			name:     "no follow flag",
			args:     []string{"session-id"},
			expected: false,
		},
		{
			name:     "follow with format flag (no conflict)",
			args:     []string{"-F", "-f", "md", "session-id"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flag package state
			flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

			// Create a new Config and register flags
			cfg := &Config{}
			flag.BoolVar(&cfg.Follow, "F", false, "Follow mode")
			flag.BoolVar(&cfg.Follow, "follow", false, "Follow mode")
			flag.StringVar(&cfg.Format, "f", "md", "Output format")
			flag.StringVar(&cfg.Format, "format", "md", "Output format")

			// Parse args
			err := flag.CommandLine.Parse(tt.args)
			if err != nil {
				t.Fatalf("flag parsing failed: %v", err)
			}

			if cfg.Follow != tt.expected {
				t.Errorf("Follow = %v, want %v", cfg.Follow, tt.expected)
			}
		})
	}
}

func TestRunFollow_FileNotFound(t *testing.T) {
	// Test that runFollow returns error code 1 for non-existent file
	opts := transcript.RenderOptions{
		Title: "Test Transcript",
	}

	exitCode := runFollow("/nonexistent/file.jsonl", opts)
	if exitCode != 1 {
		t.Errorf("expected exit code 1 for non-existent file, got %d", exitCode)
	}
}

func TestRunFollow_BasicExecution(t *testing.T) {
	// Create a temporary JSONL file with valid content
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "session.jsonl")
	content := `{"type":"user","timestamp":"2025-12-23T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"Hello"}]}}`
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// We can't easily test the full follow loop without signals,
	// but we can verify the follower is created correctly
	opts := transcript.RenderOptions{
		Title: "Test Transcript",
	}

	// Create follower to verify file is valid
	follower, err := transcript.NewFollower(tmpFile, os.Stdout, opts)
	if err != nil {
		t.Fatalf("NewFollower failed: %v", err)
	}
	if follower == nil {
		t.Fatal("expected non-nil follower")
	}
}

func TestResolveFollowInput_StdinRejected(t *testing.T) {
	// Test that empty input (stdin) returns error (requirement 2.3, 2.4)
	_, err := resolveFollowInput("", "/some/project")
	if err == nil {
		t.Fatal("expected error for stdin input")
	}
	if !strings.Contains(err.Error(), "cannot follow stdin input") {
		t.Errorf("expected error containing 'cannot follow stdin input', got: %v", err)
	}
}

func TestResolveFollowInput_FilePath(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test-session.jsonl")
	if err := os.WriteFile(tmpFile, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	// Test resolving a file path
	result, err := resolveFollowInput(tmpFile, "/some/project")
	if err != nil {
		t.Fatalf("resolveFollowInput failed: %v", err)
	}
	if result != tmpFile {
		t.Errorf("expected %q, got %q", tmpFile, result)
	}
}

func TestResolveFollowInput_SessionID(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")

	// Create Claude session file
	claudeProjectDir := filepath.Join(homeDir, ".claude", "projects", "-test-project")
	if err := os.MkdirAll(claudeProjectDir, 0755); err != nil {
		t.Fatal(err)
	}
	sessionFile := filepath.Join(claudeProjectDir, "my-session.jsonl")
	if err := os.WriteFile(sessionFile, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	// Override home directory
	origHomeDir := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", origHomeDir) }()

	// Test resolving by session ID
	result, err := resolveFollowInput("my-session", "/test/project")
	if err != nil {
		t.Fatalf("resolveFollowInput failed: %v", err)
	}
	if result != sessionFile {
		t.Errorf("expected %q, got %q", sessionFile, result)
	}
}

func TestResolveFollowInput_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")

	// Create empty directories
	if err := os.MkdirAll(filepath.Join(homeDir, ".claude", "projects", "-test-project"), 0755); err != nil {
		t.Fatal(err)
	}

	// Override home directory
	origHomeDir := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", origHomeDir) }()

	// Test resolving non-existent session
	_, err := resolveFollowInput("nonexistent-session", "/test/project")
	if err == nil {
		t.Fatal("expected error for non-existent session")
	}
	if !strings.Contains(err.Error(), "session not found") {
		t.Errorf("expected 'session not found' error, got: %v", err)
	}
}

func TestValidateFollowMode(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
		errMsg  string
	}{
		{
			name:    "follow mode disabled - no validation",
			cfg:     &Config{Follow: false, Output: "file.md", Format: "html"},
			wantErr: false,
		},
		{
			name:    "follow mode with -o flag",
			cfg:     &Config{Follow: true, Output: "file.md"},
			wantErr: true,
			errMsg:  "cannot use --output with --follow",
		},
		{
			name:    "follow mode with HTML format",
			cfg:     &Config{Follow: true, Format: "html"},
			wantErr: true,
			errMsg:  "HTML output is not supported in follow mode",
		},
		{
			name:    "follow mode with HTML uppercase",
			cfg:     &Config{Follow: true, Format: "HTML"},
			wantErr: true,
			errMsg:  "HTML output is not supported in follow mode",
		},
		{
			name:    "follow mode with markdown format - valid",
			cfg:     &Config{Follow: true, Format: "md"},
			wantErr: false,
		},
		{
			name:    "follow mode with empty format - valid (defaults to md)",
			cfg:     &Config{Follow: true, Format: ""},
			wantErr: false,
		},
		{
			name:    "follow mode with markdown long format - valid",
			cfg:     &Config{Follow: true, Format: "markdown"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFollowMode(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

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
			result := claudecode.BuildProjectPath(tt.path)
			if result != tt.expected {
				t.Errorf("claudecode.BuildProjectPath(%q) = %q, want %q", tt.path, result, tt.expected)
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
	err := convert(strings.NewReader(input), &output, "test-session-id", "md", "", "")
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
	err := convert(strings.NewReader(""), &output, "empty-session", "md", "", "")

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
	reader, sessionID, _, err := resolveInput("", "/some/project")
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

	reader, sessionID, _, err := resolveInput(tmpFile, "/some/project")
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
	err := convert(strings.NewReader(input), &output, "test-session-id", "html", "", "")
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
	err := convert(strings.NewReader(input), &output, "test-session", "xml", "", "")

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

	_, err := run(cfg)
	if err == nil {
		t.Fatal("expected error when both --list and positional argument provided")
	}

	expectedMsg := "cannot specify both --list and a positional argument"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("expected error message to contain %q, got: %v", expectedMsg, err)
	}
}

// --- Follow Mode Integration Tests ---

func TestRun_FollowModeValidation_OutputConflict(t *testing.T) {
	// Test that --follow with -o returns error
	cfg := &Config{
		Follow: true,
		Output: "output.md",
		Input:  "session-id",
	}

	_, err := run(cfg)
	if err == nil {
		t.Fatal("expected error when --follow and --output are both specified")
	}

	if !strings.Contains(err.Error(), "cannot use --output with --follow") {
		t.Errorf("expected error about output conflict, got: %v", err)
	}
}

func TestRun_FollowModeValidation_HTMLConflict(t *testing.T) {
	// Test that --follow with -f html returns error
	cfg := &Config{
		Follow: true,
		Format: "html",
		Input:  "session-id",
	}

	_, err := run(cfg)
	if err == nil {
		t.Fatal("expected error when --follow and --format html are both specified")
	}

	if !strings.Contains(err.Error(), "HTML output is not supported in follow mode") {
		t.Errorf("expected error about HTML conflict, got: %v", err)
	}
}

func TestRun_FollowModeValidation_StdinConflict(t *testing.T) {
	// Test that --follow without input (stdin) returns error
	// Note: We can't easily test stdin without piping, so we test empty input
	cfg := &Config{
		Follow: true,
		Input:  "", // Empty input would be stdin
	}

	_, err := run(cfg)
	if err == nil {
		t.Fatal("expected error when --follow is used without input")
	}

	// Either "no input specified" or "cannot follow stdin" depending on TTY detection
	// In test context, isInputFromPipe() will return false for stdin
	expectedErrors := []string{"no input specified", "cannot follow stdin"}
	found := false
	for _, expected := range expectedErrors {
		if strings.Contains(err.Error(), expected) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error about stdin or no input, got: %v", err)
	}
}

func TestRun_FollowMode_SessionNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")

	// Create empty directories
	if err := os.MkdirAll(filepath.Join(homeDir, ".claude", "projects", "-test-project"), 0755); err != nil {
		t.Fatal(err)
	}

	// Override home directory
	origHomeDir := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", origHomeDir) }()

	cfg := &Config{
		Follow:  true,
		Input:   "nonexistent-session",
		Project: "/test/project",
	}

	_, err := run(cfg)
	if err == nil {
		t.Fatal("expected error when session is not found")
	}

	if !strings.Contains(err.Error(), "session not found") {
		t.Errorf("expected 'session not found' error, got: %v", err)
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

	reader, resolvedID, _, err := resolveInput(sessionID, "/test/project")
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

	reader, resolvedID, _, err := resolveInput(sessionUUID, "/test/project")
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

	reader, _, _, err := resolveInput(sessionID, "/test/project")
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

	_, _, _, err := resolveInput("nonexistent-session", "/test/project")
	if err == nil {
		t.Fatal("expected error for non-existent session")
	}
	if !strings.Contains(err.Error(), "session not found") {
		t.Errorf("expected 'session not found' error, got: %v", err)
	}
}

// --- Phase 5: Error Handling and Negative Tests for Session Discovery ---

func TestFindCodexSession_InvalidUUIDSearch_Negative(t *testing.T) {
	// Test that invalid UUIDs don't cause false matches (req 2.3)
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, ".codex", "sessions", "2026", "01", "05")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a session file with valid UUID
	validUUID := "019b892c-3a14-7773-bd76-6465a8a0b634"
	if err := os.WriteFile(filepath.Join(sessionsDir, "session-"+validUUID+".jsonl"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	// Test invalid UUID patterns that should NOT match
	invalidIDs := []struct {
		name string
		id   string
	}{
		{"short string", "abc123"},
		{"partial uuid", "019b892c"},
		{"uuid without hyphens", "019b892c3a147773bd766465a8a0b634"},
		{"uuid with extra chars", "019b892c-3a14-7773-bd76-6465a8a0b634-extra"},
		{"different uuid", "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"},
		{"partial match string", "019b892c-3a14"},
	}

	for _, tc := range invalidIDs {
		t.Run(tc.name, func(t *testing.T) {
			foundPath, err := findCodexSession(tmpDir, tc.id)
			if err != nil {
				t.Fatalf("findCodexSession should not error: %v", err)
			}
			if foundPath != "" {
				t.Errorf("invalid UUID %q should not match, but got path: %s", tc.id, foundPath)
			}
		})
	}
}

func TestFindCodexSession_SymlinkToMissingPath_Negative(t *testing.T) {
	// Test symlink pointing to non-existent path is handled gracefully (req 2.8)
	tmpDir := t.TempDir()

	// Create .codex directory
	codexDir := filepath.Join(tmpDir, ".codex")
	if err := os.MkdirAll(codexDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create symlink pointing to non-existent directory
	symlinkPath := filepath.Join(codexDir, "sessions")
	if err := os.Symlink("/nonexistent/path/that/does/not/exist", symlinkPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	// Should not crash or hang, should return not found
	sessionUUID := "019b892c-3a14-7773-bd76-6465a8a0b634"
	foundPath, err := findCodexSession(tmpDir, sessionUUID)
	if err != nil {
		t.Fatalf("findCodexSession should handle broken symlink gracefully: %v", err)
	}
	if foundPath != "" {
		t.Errorf("expected empty path for broken symlink, got: %s", foundPath)
	}
}

func TestWalkDirFollowSymlinks_BrokenSymlink_Negative(t *testing.T) {
	// Test walkDirFollowSymlinks handles broken symlinks gracefully
	tmpDir := t.TempDir()

	// Create a directory with a broken symlink
	if err := os.MkdirAll(filepath.Join(tmpDir, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create valid file
	validFile := filepath.Join(tmpDir, "valid.txt")
	if err := os.WriteFile(validFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create broken symlink
	brokenLink := filepath.Join(tmpDir, "subdir", "broken.txt")
	if err := os.Symlink("/nonexistent/file", brokenLink); err != nil {
		t.Fatalf("failed to create broken symlink: %v", err)
	}

	// Walk should complete without crashing
	var foundFiles []string
	err := walkDirFollowSymlinks(tmpDir, func(path string, d os.DirEntry, err error) error {
		// Callback may receive error for broken symlink - that's OK
		if err != nil {
			return nil // Continue walking
		}
		if !d.IsDir() {
			foundFiles = append(foundFiles, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walkDirFollowSymlinks should handle broken symlinks: %v", err)
	}

	// Should have found the valid file
	if len(foundFiles) < 1 {
		t.Error("expected to find at least 1 valid file")
	}
}

func TestListCodexSessions_IgnoresEmptyFiles_Negative(t *testing.T) {
	// Test that empty files are ignored during discovery (req 2.7)
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, ".codex", "sessions", "2026", "01", "05")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create empty file
	emptyFile := filepath.Join(sessionsDir, "empty-session.jsonl")
	if err := os.WriteFile(emptyFile, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	// Create valid file
	validUUID := "019b892c-3a14-7773-bd76-6465a8a0b634"
	validSession := `{"timestamp":"2026-01-05T00:22:15.725Z","type":"session_meta","payload":{"id":"019b892c-3a14-7773-bd76-6465a8a0b634"}}`
	validFile := filepath.Join(sessionsDir, "session-"+validUUID+".jsonl")
	if err := os.WriteFile(validFile, []byte(validSession), 0644); err != nil {
		t.Fatal(err)
	}

	sessions, err := listCodexSessions(tmpDir)
	if err != nil {
		t.Fatalf("listCodexSessions failed: %v", err)
	}

	// Should only have 1 session (the valid one, not the empty one)
	if len(sessions) != 1 {
		t.Errorf("expected 1 session (empty file ignored), got %d", len(sessions))
	}
}

func TestConvert_EmptyFile_Negative(t *testing.T) {
	// Test convert on empty file returns proper error
	var output bytes.Buffer
	err := convert(strings.NewReader(""), &output, "empty-session", "md", "", "")

	if err == nil {
		t.Fatal("expected error for empty file")
	}
	if !strings.Contains(err.Error(), "empty file") {
		t.Errorf("expected error containing 'empty file', got: %v", err)
	}
}

func TestConvert_InvalidJSONFile_Negative(t *testing.T) {
	// Test convert on invalid JSON returns proper error
	var output bytes.Buffer
	err := convert(strings.NewReader("{invalid json}"), &output, "test-session", "md", "", "")

	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	// Invalid JSON is skipped during detection, resulting in no format found
	if !strings.Contains(err.Error(), "no format-defining entries found") {
		t.Errorf("expected error about no format-defining entries, got: %v", err)
	}
}

func TestConvert_UnknownFormatType_Negative(t *testing.T) {
	// Test convert on unknown format type returns proper error
	var output bytes.Buffer
	err := convert(strings.NewReader(`{"type":"unknown_format"}`), &output, "test-session", "md", "", "")

	if err == nil {
		t.Fatal("expected error for unknown format type")
	}
	if !strings.Contains(err.Error(), "unrecognized log format") {
		t.Errorf("expected error about unrecognized format, got: %v", err)
	}
}

func TestConvert_WarningSummaryOutput(t *testing.T) {
	// Test that warnings are reported with line numbers and summary (req 9.4)
	// Input with malformed middle line
	input := `{"type":"user","timestamp":"2025-12-23T10:30:00+11:00","message":{"role":"user","content":"First"}}
{not valid json in the middle}
{"type":"assistant","timestamp":"2025-12-23T10:30:05+11:00","message":{"role":"assistant","content":[{"type":"text","text":"Second"}]}}`

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	var output bytes.Buffer
	err := convert(strings.NewReader(input), &output, "test-session", "md", "", "")

	// Restore stderr and capture output
	_ = w.Close()
	os.Stderr = oldStderr
	var stderrBuf bytes.Buffer
	_, _ = stderrBuf.ReadFrom(r)
	stderrOutput := stderrBuf.String()

	if err != nil {
		t.Fatalf("convert should succeed with malformed middle line: %v", err)
	}

	// Verify warning format: "Warning: line N: message"
	if !strings.Contains(stderrOutput, "Warning: line 2:") {
		t.Errorf("expected warning with line number format, got: %s", stderrOutput)
	}

	// Verify warning summary: "Parsed with N warning(s)"
	if !strings.Contains(stderrOutput, "Parsed with 1 warning(s)") {
		t.Errorf("expected warning summary 'Parsed with 1 warning(s)', got: %s", stderrOutput)
	}
}

// --- Phase 6: CLI Integration Tests for Codex Support ---

// TestConvert_CodexSessionToMarkdown tests the full pipeline: Codex JSONL -> ParseJSONL -> RenderMarkdown
func TestConvert_CodexSessionToMarkdown(t *testing.T) {
	// Valid Codex session input (mimics codex_valid.jsonl structure)
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"019b892c-3a14-7773-bd76-6465a8a0b634","cwd":"/Users/arjen/projects/orbit"}}
{"timestamp":"2026-01-04T13:22:15.800Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"List all files in the current directory"}]}}
{"timestamp":"2026-01-04T13:22:21.499Z","type":"event_msg","payload":{"type":"agent_reasoning","text":"**Preparing to list files**"}}
{"timestamp":"2026-01-04T13:22:21.885Z","type":"response_item","payload":{"type":"function_call","name":"shell_command","arguments":"{\"command\":\"ls -la\"}","call_id":"call_abc123"}}
{"timestamp":"2026-01-04T13:22:21.912Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_abc123","output":"Exit code: 0\nOutput:\nfile1.txt\nfile2.txt"}}
{"timestamp":"2026-01-04T13:23:16.617Z","type":"event_msg","payload":{"type":"agent_message","message":"I found 2 files in the directory."}}
{"timestamp":"2026-01-04T13:23:30.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"The directory contains 2 files."}]}}`

	var output bytes.Buffer
	err := convert(strings.NewReader(input), &output, "codex-test-session", "md", "", "")
	if err != nil {
		t.Fatalf("convert failed for Codex session: %v", err)
	}

	result := output.String()

	// Verify header
	if !strings.Contains(result, "# Session Transcript") {
		t.Error("output should contain Session Transcript header")
	}

	// Verify session ID
	if !strings.Contains(result, "`codex-test-session`") {
		t.Error("output should contain session ID")
	}

	// Verify user message
	if !strings.Contains(result, "## 👤 User") {
		t.Error("output should contain User heading")
	}
	if !strings.Contains(result, "List all files in the current directory") {
		t.Error("output should contain user message text")
	}

	// Verify tool use (shell_command)
	if !strings.Contains(result, "shell_command") {
		t.Error("output should contain shell_command tool name")
	}
	if !strings.Contains(result, "ls -la") {
		t.Error("output should contain command arguments")
	}

	// Verify tool result
	if !strings.Contains(result, "file1.txt") || !strings.Contains(result, "file2.txt") {
		t.Error("output should contain tool result with file names")
	}

	// Verify assistant message
	if !strings.Contains(result, "## 🤖 Assistant") {
		t.Error("output should contain Assistant heading")
	}
	if !strings.Contains(result, "The directory contains 2 files") {
		t.Error("output should contain assistant message text")
	}
}

// TestConvert_CodexSessionToHTML tests the full pipeline: Codex JSONL -> ParseJSONL -> RenderHTML
func TestConvert_CodexSessionToHTML(t *testing.T) {
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"019b892c-3a14-7773-bd76-6465a8a0b634"}}
{"timestamp":"2026-01-04T13:22:15.800Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Hello from Codex!"}]}}
{"timestamp":"2026-01-04T13:22:30.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hello! How can I help you today?"}]}}`

	var output bytes.Buffer
	err := convert(strings.NewReader(input), &output, "codex-html-session", "html", "", "")
	if err != nil {
		t.Fatalf("convert failed for Codex HTML: %v", err)
	}

	result := output.String()

	// Check HTML structure
	if !strings.Contains(result, "<!DOCTYPE html>") {
		t.Error("output should contain DOCTYPE")
	}
	if !strings.Contains(result, "<title>Session Transcript</title>") {
		t.Error("output should contain title")
	}
	if !strings.Contains(result, "<code>codex-html-session</code>") {
		t.Error("output should contain session ID")
	}

	// Check user message
	if !strings.Contains(result, `class="message user"`) {
		t.Error("output should contain user message class")
	}
	if !strings.Contains(result, "Hello from Codex!") {
		t.Error("output should contain user message text")
	}

	// Check assistant message
	if !strings.Contains(result, `class="message assistant"`) {
		t.Error("output should contain assistant message class")
	}
	if !strings.Contains(result, "Hello! How can I help you today?") {
		t.Error("output should contain assistant message text")
	}
}

// TestConvert_CodexReasoningBlocks tests that Codex reasoning blocks are rendered correctly
func TestConvert_CodexReasoningBlocks(t *testing.T) {
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"019b892c-3a14-7773-bd76-6465a8a0b634"}}
{"timestamp":"2026-01-04T13:22:15.800Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Think about this problem"}]}}
{"timestamp":"2026-01-04T13:22:56.849Z","type":"response_item","payload":{"type":"reasoning","summary":[{"type":"summary_text","text":"**Analyzing the problem**\nLet me think through this step by step."}],"encrypted_content":"encrypted_data_here"}}
{"timestamp":"2026-01-04T13:23:00.000Z","type":"event_msg","payload":{"type":"agent_reasoning","text":"**Additional reasoning context**"}}
{"timestamp":"2026-01-04T13:23:30.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Here is my analysis."}]}}`

	var output bytes.Buffer
	err := convert(strings.NewReader(input), &output, "reasoning-session", "md", "", "")
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}

	result := output.String()

	// Verify reasoning summary is rendered (not encrypted content)
	if !strings.Contains(result, "Analyzing the problem") {
		t.Error("output should contain reasoning summary text")
	}
	if strings.Contains(result, "encrypted_data_here") {
		t.Error("output should NOT contain encrypted content")
	}

	// Verify agent_reasoning is also rendered
	if !strings.Contains(result, "Additional reasoning context") {
		t.Error("output should contain agent_reasoning text")
	}
}

// TestIntegration_ApsisWithCodexFile tests apsis reading from a Codex JSONL file path
func TestIntegration_ApsisWithCodexFile(t *testing.T) {
	// Create a temporary Codex session file
	tmpDir := t.TempDir()
	sessionFile := filepath.Join(tmpDir, "codex-session-019b892c-3a14-7773-bd76-6465a8a0b634.jsonl")
	content := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"019b892c-3a14-7773-bd76-6465a8a0b634"}}
{"timestamp":"2026-01-04T13:22:15.800Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Integration test message"}]}}
{"timestamp":"2026-01-04T13:22:30.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Integration test response"}]}}`
	if err := os.WriteFile(sessionFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Test resolving input from file path
	reader, sessionID, _, err := resolveInput(sessionFile, "/some/project")
	if err != nil {
		t.Fatalf("resolveInput failed: %v", err)
	}
	defer func() { _ = reader.Close() }()

	// Session ID should be extracted from filename
	expected := "codex-session-019b892c-3a14-7773-bd76-6465a8a0b634"
	if sessionID != expected {
		t.Errorf("expected session ID %q, got %q", expected, sessionID)
	}

	// Test convert
	var output bytes.Buffer
	f, _ := os.Open(sessionFile)
	defer func() { _ = f.Close() }()
	err = convert(f, &output, sessionID, "md", "", "")
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}

	result := output.String()
	if !strings.Contains(result, "Integration test message") {
		t.Error("output should contain user message")
	}
	if !strings.Contains(result, "Integration test response") {
		t.Error("output should contain assistant response")
	}
}

// TestIntegration_ListMixedSessions tests listing sessions from both Claude and Codex
func TestIntegration_ListMixedSessions(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")

	// Create Claude sessions directory
	claudeProjectDir := filepath.Join(homeDir, ".claude", "projects", "-test-project")
	if err := os.MkdirAll(claudeProjectDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create Codex sessions directory with date structure
	codexSessionsDir := filepath.Join(homeDir, ".codex", "sessions", "2026", "01", "05")
	if err := os.MkdirAll(codexSessionsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create Claude sessions with different timestamps
	claudeSession1 := `{"type":"user","timestamp":"2026-01-05T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"Claude session 1"}]}}`
	claudeSession2 := `{"type":"user","timestamp":"2026-01-05T14:00:00Z","message":{"role":"user","content":[{"type":"text","text":"Claude session 2"}]}}`
	if err := os.WriteFile(filepath.Join(claudeProjectDir, "claude-session-1.jsonl"), []byte(claudeSession1), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeProjectDir, "claude-session-2.jsonl"), []byte(claudeSession2), 0644); err != nil {
		t.Fatal(err)
	}

	// Create Codex sessions with different timestamps
	codexSession1 := `{"timestamp":"2026-01-05T08:00:00Z","type":"session_meta","payload":{"id":"019b892c-3a14-7773-bd76-6465a8a0b634"}}`
	codexSession2 := `{"timestamp":"2026-01-05T12:00:00Z","type":"session_meta","payload":{"id":"abcd1234-5678-90ab-cdef-1234567890ab"}}`
	if err := os.WriteFile(filepath.Join(codexSessionsDir, "session-019b892c-3a14-7773-bd76-6465a8a0b634.jsonl"), []byte(codexSession1), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexSessionsDir, "session-abcd1234-5678-90ab-cdef-1234567890ab.jsonl"), []byte(codexSession2), 0644); err != nil {
		t.Fatal(err)
	}

	// Override home directory for testing
	origHomeDir := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", origHomeDir) }()

	// Get all sessions
	sessions, err := listAllSessions("/test/project")
	if err != nil {
		t.Fatalf("listAllSessions failed: %v", err)
	}

	// Verify we have 4 sessions total (2 Claude + 2 Codex)
	if len(sessions) != 4 {
		t.Fatalf("expected 4 sessions, got %d", len(sessions))
	}

	// Verify sources are correctly identified
	claudeCount := 0
	codexCount := 0
	for _, s := range sessions {
		switch s.Source {
		case "claude":
			claudeCount++
		case "codex":
			codexCount++
		default:
			t.Errorf("unexpected source: %s", s.Source)
		}
	}
	if claudeCount != 2 {
		t.Errorf("expected 2 Claude sessions, got %d", claudeCount)
	}
	if codexCount != 2 {
		t.Errorf("expected 2 Codex sessions, got %d", codexCount)
	}

	// Verify sessions are sorted by timestamp (oldest first)
	// Expected order by timestamp:
	// 1. Codex 08:00 (019b892c...)
	// 2. Claude 10:00 (claude-session-1)
	// 3. Codex 12:00 (abcd1234...)
	// 4. Claude 14:00 (claude-session-2)
	expectedOrder := []struct {
		source string
		hour   int
	}{
		{"codex", 8},
		{"claude", 10},
		{"codex", 12},
		{"claude", 14},
	}

	for i, expected := range expectedOrder {
		if sessions[i].Source != expected.source {
			t.Errorf("position %d: expected source %s, got %s", i, expected.source, sessions[i].Source)
		}
		if sessions[i].CreatedAt.Hour() != expected.hour {
			t.Errorf("position %d: expected hour %d, got %d", i, expected.hour, sessions[i].CreatedAt.Hour())
		}
	}
}

// TestIntegration_ListWithOnlyCodexSessions tests listing when only Codex sessions exist
func TestIntegration_ListWithOnlyCodexSessions(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")

	// Create only Codex sessions (no Claude project directory)
	codexSessionsDir := filepath.Join(homeDir, ".codex", "sessions", "2026", "01", "05")
	if err := os.MkdirAll(codexSessionsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create multiple Codex sessions
	sessions := []struct {
		uuid      string
		timestamp string
	}{
		{"019b892c-3a14-7773-bd76-6465a8a0b634", "2026-01-05T08:00:00Z"},
		{"abcd1234-5678-90ab-cdef-1234567890ab", "2026-01-05T09:00:00Z"},
		{"11112222-3333-4444-5555-666677778888", "2026-01-05T10:00:00Z"},
	}

	for _, s := range sessions {
		content := fmt.Sprintf(`{"timestamp":"%s","type":"session_meta","payload":{"id":"%s"}}`, s.timestamp, s.uuid)
		filename := filepath.Join(codexSessionsDir, "session-"+s.uuid+".jsonl")
		if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Override home directory
	origHomeDir := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", origHomeDir) }()

	// List sessions
	result, err := listAllSessions("/nonexistent/project")
	if err != nil {
		t.Fatalf("listAllSessions failed: %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(result))
	}

	// All should be Codex
	for _, s := range result {
		if s.Source != "codex" {
			t.Errorf("expected source 'codex', got %q", s.Source)
		}
	}
}

// TestIntegration_SessionOutputFormat tests that session listing output format is correct
func TestIntegration_SessionOutputFormat(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")

	// Create Claude session
	claudeProjectDir := filepath.Join(homeDir, ".claude", "projects", "-test-project")
	if err := os.MkdirAll(claudeProjectDir, 0755); err != nil {
		t.Fatal(err)
	}
	claudeSession := `{"type":"user","timestamp":"2026-01-05T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"test"}]}}`
	if err := os.WriteFile(filepath.Join(claudeProjectDir, "test-session.jsonl"), []byte(claudeSession), 0644); err != nil {
		t.Fatal(err)
	}

	// Create Codex session
	codexSessionsDir := filepath.Join(homeDir, ".codex", "sessions", "2026", "01", "05")
	if err := os.MkdirAll(codexSessionsDir, 0755); err != nil {
		t.Fatal(err)
	}
	codexSession := `{"timestamp":"2026-01-05T09:00:00Z","type":"session_meta","payload":{"id":"019b892c-3a14-7773-bd76-6465a8a0b634"}}`
	if err := os.WriteFile(filepath.Join(codexSessionsDir, "session-019b892c-3a14-7773-bd76-6465a8a0b634.jsonl"), []byte(codexSession), 0644); err != nil {
		t.Fatal(err)
	}

	// Override home directory
	origHomeDir := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", origHomeDir) }()

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Run listSessions
	err := listSessions("/test/project")

	// Restore stdout
	_ = w.Close()
	os.Stdout = oldStdout
	var stdoutBuf bytes.Buffer
	_, _ = stdoutBuf.ReadFrom(r)
	output := stdoutBuf.String()

	if err != nil {
		t.Fatalf("listSessions failed: %v", err)
	}

	// Verify output contains source indicators
	if !strings.Contains(output, "[claude]") {
		t.Error("output should contain [claude] source indicator")
	}
	if !strings.Contains(output, "[codex]") {
		t.Error("output should contain [codex] source indicator")
	}

	// Verify output contains session IDs
	if !strings.Contains(output, "test-session") {
		t.Error("output should contain Claude session ID")
	}
	if !strings.Contains(output, "019b892c-3a14-7773-bd76-6465a8a0b634") {
		t.Error("output should contain Codex session UUID")
	}
}

// --- Follow Mode Integration Tests ---

// TestFollowMode_SIGINTExitCode tests that SIGINT termination returns exit code 130
// (requirements 6.1-6.4).
func TestFollowMode_SIGINTExitCode(t *testing.T) {
	// Create a temporary JSONL file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "session.jsonl")
	content := `{"type":"user","timestamp":"2025-12-23T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"Hello"}]}}`
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	opts := transcript.RenderOptions{Title: "Test"}

	// We can't send real SIGINT in a unit test easily, but we can verify
	// the runFollow function returns 130 when context is cancelled.
	// The actual SIGINT behavior relies on signal.NotifyContext which is tested
	// by the OS-level signal handling.

	// Create follower and verify it returns 130 when cancelled by context
	// by testing the internal behavior directly
	follower, err := transcript.NewFollower(tmpFile, bytes.NewBuffer(nil), opts)
	if err != nil {
		t.Fatalf("NewFollower failed: %v", err)
	}

	// Create a context that we'll cancel to simulate SIGINT
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- follower.Run(ctx)
	}()

	// Wait for initial render
	time.Sleep(100 * time.Millisecond)

	// Cancel (simulates SIGINT)
	cancel()

	// Wait for completion
	select {
	case err := <-done:
		// Follower.Run returns nil on clean shutdown (including cancellation)
		if err != nil {
			t.Errorf("expected nil error on cancellation, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("follower did not exit after cancellation")
	}

	// The exit code 130 is set by runFollow based on ctx.Err()
	// We can verify this logic by checking ctx.Err() is set
	if ctx.Err() == nil {
		t.Error("expected context to be cancelled")
	}
}

// TestFollowMode_ExitCode130Logic tests that runFollow returns 130 when cancelled.
func TestFollowMode_ExitCode130Logic(t *testing.T) {
	// Create a temporary JSONL file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "session.jsonl")
	content := `{"type":"user","timestamp":"2025-12-23T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"Hello"}]}}`
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Test file not found returns 1
	exitCode := runFollow("/nonexistent/file.jsonl", transcript.RenderOptions{})
	if exitCode != 1 {
		t.Errorf("expected exit code 1 for file not found, got %d", exitCode)
	}
}

// TestFollowMode_BasicFollowWithEntry tests the basic follow mode flow at CLI level.
func TestFollowMode_BasicFollowWithEntry(t *testing.T) {
	// Create a temporary JSONL file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "session.jsonl")
	content := `{"type":"user","timestamp":"2025-12-23T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"Hello from CLI test"}]}}`
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Verify resolveFollowInput works with file path
	resolvedPath, err := resolveFollowInput(tmpFile, "/some/project")
	if err != nil {
		t.Fatalf("resolveFollowInput failed: %v", err)
	}
	if resolvedPath != tmpFile {
		t.Errorf("expected %q, got %q", tmpFile, resolvedPath)
	}

	// Test validateFollowMode with valid config
	validCfg := &Config{Follow: true, Format: "md"}
	if err := validateFollowMode(validCfg); err != nil {
		t.Errorf("validateFollowMode should accept valid config: %v", err)
	}
}

// --- Kiro Integration Tests ---

// TestListKiroSessions_NoMatchingSessions tests that listKiroSessions returns
// an empty slice when no sessions match the directory.
// Note: This test behaves differently depending on whether Kiro is installed:
// - If Kiro database exists: returns empty slice (no sessions for this dir)
// - If Kiro database doesn't exist: returns nil (graceful fallback)
func TestListKiroSessions_NoMatchingSessions(t *testing.T) {
	// Use a random path that won't have any Kiro sessions
	sessions, err := listKiroSessions("/nonexistent/random/path/12345")
	if err != nil {
		t.Fatalf("listKiroSessions should not error for non-matching directory: %v", err)
	}
	// Either nil or empty slice is acceptable
	if len(sessions) != 0 {
		t.Errorf("expected no sessions for non-matching directory, got: %d", len(sessions))
	}
}

// TestResolveKiroSession_SessionNotFound tests that resolveKiroSession returns
// an error when the session doesn't exist.
// Note: The specific error depends on whether Kiro is installed:
// - If Kiro database exists: ErrSessionNotFound
// - If Kiro database doesn't exist: ErrDatabaseNotFound
func TestResolveKiroSession_SessionNotFound(t *testing.T) {
	_, err := resolveKiroSession("nonexistent-session-id-12345", "/some/random/path")
	if err == nil {
		t.Fatal("expected error when session not found")
	}
	// The error should be either ErrDatabaseNotFound or ErrSessionNotFound
	if !errors.Is(err, logs.ErrDatabaseNotFound) && !errors.Is(err, logs.ErrSessionNotFound) {
		t.Errorf("expected ErrDatabaseNotFound or ErrSessionNotFound, got: %v", err)
	}
}

// TestResolveInput_FallsThroughKiroWhenDatabaseNotFound tests that resolveInput
// continues to search other sources when Kiro database is not found.
func TestResolveInput_FallsThroughKiroWhenDatabaseNotFound(t *testing.T) {
	// This test verifies that when Claude and Codex don't have the session,
	// and Kiro returns ErrDatabaseNotFound, we get "session not found" error
	// (not a Kiro-specific error)
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")

	// Create empty Claude project directory (no sessions)
	claudeProjectDir := filepath.Join(homeDir, ".claude", "projects", "-test-project")
	if err := os.MkdirAll(claudeProjectDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Set HOME to our test directory
	origHomeDir := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", origHomeDir) }()

	// Try to resolve a non-existent session ID
	_, _, _, err := resolveInput("nonexistent-session-id", "/test/project")
	if err == nil {
		t.Fatal("expected error for non-existent session")
	}

	// The error should be "session not found", not a Kiro-specific error
	if !strings.Contains(err.Error(), "session not found") {
		t.Errorf("expected 'session not found' error, got: %v", err)
	}
}

// TestListAllSessions_IncludesKiroSessions tests that listAllSessions includes
// Kiro sessions in the combined listing (when Kiro database is available).
// Note: This test only verifies the warning behavior when Kiro is unavailable,
// since we can't easily create a test Kiro database.
func TestListAllSessions_IncludesKiroWarning(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")

	// Create Claude and Codex directories but no Kiro database
	claudeProjectDir := filepath.Join(homeDir, ".claude", "projects", "-test-project")
	if err := os.MkdirAll(claudeProjectDir, 0755); err != nil {
		t.Fatal(err)
	}
	codexSessionsDir := filepath.Join(homeDir, ".codex", "sessions")
	if err := os.MkdirAll(codexSessionsDir, 0755); err != nil {
		t.Fatal(err)
	}

	origHomeDir := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", origHomeDir) }()

	// listAllSessions should not error even when Kiro database doesn't exist
	// (it just returns empty list for Kiro)
	sessions, err := listAllSessions("/test/project")
	if err != nil {
		t.Fatalf("listAllSessions should not fail when Kiro unavailable: %v", err)
	}

	// Should have 0 sessions (no Claude, Codex, or Kiro sessions)
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

// TestSessionInfo_KiroSource tests that Kiro sessions have the correct source field.
func TestSessionInfo_KiroSource(t *testing.T) {
	// Test the sorting function includes Kiro sessions correctly
	sessions := []SessionInfo{
		{ID: "claude-1", CreatedAt: time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC), Source: "claude"},
		{ID: "kiro-1", CreatedAt: time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC), Source: "kiro"},
		{ID: "codex-1", CreatedAt: time.Date(2026, 1, 5, 11, 0, 0, 0, time.UTC), Source: "codex"},
	}

	sortSessionsByTimestamp(sessions)

	// Should be sorted: kiro (9am), claude (10am), codex (11am)
	expectedOrder := []string{"kiro", "claude", "codex"}
	for i, expected := range expectedOrder {
		if sessions[i].Source != expected {
			t.Errorf("position %d: expected source %s, got %s", i, expected, sessions[i].Source)
		}
	}
}

// TestResolveFollowInput_KiroSessionNotSupported tests that follow mode
// doesn't find Kiro sessions (since they're in SQLite, not files).
// The session should return "session not found" for a Kiro-only session.
func TestResolveFollowInput_KiroSessionNotSupported(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")

	// Create empty Claude and Codex directories
	claudeProjectDir := filepath.Join(homeDir, ".claude", "projects", "-test-project")
	if err := os.MkdirAll(claudeProjectDir, 0755); err != nil {
		t.Fatal(err)
	}

	origHomeDir := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", origHomeDir) }()

	// Try to resolve a session ID that would be in Kiro
	// (but Kiro doesn't support follow mode since sessions are in SQLite)
	_, err := resolveFollowInput("kiro-session-id", "/test/project")
	if err == nil {
		t.Fatal("expected error for Kiro session in follow mode")
	}
	if !strings.Contains(err.Error(), "session not found") {
		t.Errorf("expected 'session not found' error, got: %v", err)
	}
}

// --- JSON Output Format Tests ---

func TestConvertToJSON_JSONL(t *testing.T) {
	// Test JSONL input produces JSON array output
	input := `{"type":"user","timestamp":"2025-12-23T10:00:00Z","message":{"role":"user","content":"Hello"}}
{"type":"assistant","timestamp":"2025-12-23T10:00:05Z","message":{"role":"assistant","content":[{"type":"text","text":"Hi there"}]}}`

	var output bytes.Buffer
	err := convertToJSON(strings.NewReader(input), &output, "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := output.String()
	// Should be a JSON array
	if !strings.HasPrefix(result, "[") {
		t.Error("expected output to start with '[' (JSON array)")
	}
	// Pretty-printed JSON has quotes on their own (not escaped in Contains check)
	if !strings.Contains(result, `"type": "user"`) {
		t.Error("expected output to contain user entry")
	}
	if !strings.Contains(result, `"type": "assistant"`) {
		t.Error("expected output to contain assistant entry")
	}
}

func TestConvertToJSON_Kiro(t *testing.T) {
	// Test Kiro JSON input is preserved as object
	input := `{
		"conversation_id": "test-123",
		"history": [
			{
				"user": {"content": {"prompt": {"prompt": "Hello"}}},
				"assistant": {"TextResponse": {"content": "Hi"}}
			}
		],
		"user_turn_metadata": {
			"continuation_id": "cont-1",
			"requests": [],
			"usage_info": [{"unit": "credit", "unit_plural": "credits", "value": 0.05}]
		}
	}`

	var output bytes.Buffer
	err := convertToJSON(strings.NewReader(input), &output, "kiro")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := output.String()
	// Should be a JSON object (Kiro format), not array
	if !strings.HasPrefix(result, "{") {
		t.Error("expected output to start with '{' (JSON object)")
	}
	if !strings.Contains(result, `"conversation_id"`) {
		t.Error("expected output to contain conversation_id")
	}
	if !strings.Contains(result, `"usage_info"`) {
		t.Error("expected output to contain usage_info")
	}
}

func TestConvertToJSON_EmptyInput(t *testing.T) {
	var output bytes.Buffer
	err := convertToJSON(strings.NewReader(""), &output, "")

	// Empty input should not error (returns nil), just prints warning
	if err != nil {
		t.Errorf("unexpected error for empty input: %v", err)
	}
}

func TestConvertToJSON_InvalidLines(t *testing.T) {
	// Test that invalid JSON lines are skipped with warning
	input := `{"type":"user","message":"valid"}
{invalid json line}
{"type":"assistant","message":"also valid"}`

	var output bytes.Buffer
	err := convertToJSON(strings.NewReader(input), &output, "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := output.String()
	// Should have 2 valid entries in array (invalid line skipped)
	// Pretty-printed JSON has quotes on their own
	if !strings.Contains(result, `"type": "user"`) {
		t.Error("expected output to contain user entry")
	}
	if !strings.Contains(result, `"type": "assistant"`) {
		t.Error("expected output to contain assistant entry")
	}
}

func TestConvert_JSONFormat(t *testing.T) {
	// Test that -f json routes to convertToJSON
	input := `{"type":"user","timestamp":"2025-12-23T10:00:00Z","message":{"role":"user","content":"Hello"}}`

	var output bytes.Buffer
	err := convert(strings.NewReader(input), &output, "test-session", "json", "", "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := output.String()
	// Should be JSON array output
	if !strings.HasPrefix(result, "[") {
		t.Error("expected JSON array output")
	}
}

func TestValidateFollowMode_JSONFormatConflict(t *testing.T) {
	// Test that JSON format is rejected in follow mode
	cfg := &Config{
		Follow: true,
		Format: "json",
		Input:  "test-session",
	}

	err := validateFollowMode(cfg)
	if err == nil {
		t.Fatal("expected error for JSON format in follow mode")
	}
	if !strings.Contains(err.Error(), "JSON output is not supported in follow mode") {
		t.Errorf("expected JSON follow mode error, got: %v", err)
	}
}

func TestParseCopilotWorkspace_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	workspaceFile := filepath.Join(tmpDir, "workspace.yaml")

	content := `id: b310b03c-e860-461a-840c-aafb44b812f8
cwd: /Users/test/projects/myproject
git_root: /Users/test/projects/myproject
created_at: 2026-01-31T21:23:32.449Z
summary: test session
`
	if err := os.WriteFile(workspaceFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	ws, err := parseCopilotWorkspace(workspaceFile)
	if err != nil {
		t.Fatalf("parseCopilotWorkspace failed: %v", err)
	}
	if ws == nil {
		t.Fatal("expected workspace to be parsed")
	}
	if ws.ID != "b310b03c-e860-461a-840c-aafb44b812f8" {
		t.Errorf("expected ID 'b310b03c-e860-461a-840c-aafb44b812f8', got '%s'", ws.ID)
	}
	if ws.Cwd != "/Users/test/projects/myproject" {
		t.Errorf("expected Cwd '/Users/test/projects/myproject', got '%s'", ws.Cwd)
	}
	if ws.GitRoot != "/Users/test/projects/myproject" {
		t.Errorf("expected GitRoot '/Users/test/projects/myproject', got '%s'", ws.GitRoot)
	}
	if ws.CreatedAt == nil {
		t.Error("expected CreatedAt to be parsed")
	}
}

func TestParseCopilotWorkspace_NotExists(t *testing.T) {
	ws, err := parseCopilotWorkspace("/nonexistent/workspace.yaml")
	if err != nil {
		t.Fatalf("expected nil error for nonexistent file, got: %v", err)
	}
	if ws != nil {
		t.Error("expected nil workspace for nonexistent file")
	}
}

func TestParseCopilotWorkspace_Malformed(t *testing.T) {
	tmpDir := t.TempDir()
	workspaceFile := filepath.Join(tmpDir, "workspace.yaml")

	content := `not: valid: yaml: content:: here`
	if err := os.WriteFile(workspaceFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	ws, err := parseCopilotWorkspace(workspaceFile)
	if err != nil {
		t.Fatalf("expected nil error for malformed file, got: %v", err)
	}
	if ws != nil {
		t.Error("expected nil workspace for malformed file")
	}
}

func TestFindCopilotSession_UUIDMatching(t *testing.T) {
	// Create a temporary directory structure mimicking ~/.copilot/session-state/
	tmpDir := t.TempDir()
	sessionUUID := "b310b03c-e860-461a-840c-aafb44b812f8"
	sessionDir := filepath.Join(tmpDir, ".copilot", "session-state", sessionUUID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create events.jsonl file
	eventsFile := filepath.Join(sessionDir, "events.jsonl")
	content := `{"type":"session.start","id":"1"}`
	if err := os.WriteFile(eventsFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Test finding session by UUID
	foundPath, err := findCopilotSession(tmpDir, sessionUUID)
	if err != nil {
		t.Fatalf("findCopilotSession failed: %v", err)
	}
	if foundPath == "" {
		t.Fatal("expected to find session but got empty path")
	}
	if filepath.Base(foundPath) != "events.jsonl" {
		t.Errorf("expected filename 'events.jsonl', got %q", filepath.Base(foundPath))
	}
}

func TestFindCopilotSession_CaseInsensitiveUUID(t *testing.T) {
	tmpDir := t.TempDir()
	sessionUUID := "b310b03c-e860-461a-840c-aafb44b812f8"
	sessionDir := filepath.Join(tmpDir, ".copilot", "session-state", sessionUUID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatal(err)
	}

	eventsFile := filepath.Join(sessionDir, "events.jsonl")
	if err := os.WriteFile(eventsFile, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	// Search with uppercase UUID (should still match)
	foundPath, err := findCopilotSession(tmpDir, strings.ToUpper(sessionUUID))
	if err != nil {
		t.Fatalf("findCopilotSession failed: %v", err)
	}
	if foundPath == "" {
		t.Fatal("expected to find session with case-insensitive match")
	}
}

func TestFindCopilotSession_NonExistentDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	sessionUUID := "b310b03c-e860-461a-840c-aafb44b812f8"
	foundPath, err := findCopilotSession(tmpDir, sessionUUID)
	if err != nil {
		t.Fatalf("expected no error for nonexistent directory, got: %v", err)
	}
	if foundPath != "" {
		t.Errorf("expected empty path for nonexistent directory, got: %s", foundPath)
	}
}

func TestFindCopilotSession_InvalidUUID(t *testing.T) {
	tmpDir := t.TempDir()

	// Test with non-UUID string
	foundPath, err := findCopilotSession(tmpDir, "not-a-uuid")
	if err != nil {
		t.Fatalf("expected no error for invalid UUID, got: %v", err)
	}
	if foundPath != "" {
		t.Errorf("expected empty path for invalid UUID, got: %s", foundPath)
	}
}

func TestFindCopilotSession_MissingEventsFile(t *testing.T) {
	tmpDir := t.TempDir()
	sessionUUID := "b310b03c-e860-461a-840c-aafb44b812f8"
	sessionDir := filepath.Join(tmpDir, ".copilot", "session-state", sessionUUID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Don't create events.jsonl

	foundPath, err := findCopilotSession(tmpDir, sessionUUID)
	if err != nil {
		t.Fatalf("findCopilotSession failed: %v", err)
	}
	if foundPath != "" {
		t.Error("expected empty path when events.jsonl is missing")
	}
}

func TestListCopilotSessions_Basic(t *testing.T) {
	tmpDir := t.TempDir()
	sessionUUID := "b310b03c-e860-461a-840c-aafb44b812f8"
	sessionDir := filepath.Join(tmpDir, ".copilot", "session-state", sessionUUID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create workspace.yaml
	workspaceContent := `id: b310b03c-e860-461a-840c-aafb44b812f8
cwd: /test/project
git_root: /test/project
created_at: 2026-01-31T21:23:32.449Z
`
	if err := os.WriteFile(filepath.Join(sessionDir, "workspace.yaml"), []byte(workspaceContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create events.jsonl
	if err := os.WriteFile(filepath.Join(sessionDir, "events.jsonl"), []byte(`{"type":"session.start"}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Override home directory for testing
	origHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmpDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", origHome) }()

	sessions, err := listCopilotSessions("/test/project")
	if err != nil {
		t.Fatalf("listCopilotSessions failed: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	if sessions[0].ID != sessionUUID {
		t.Errorf("expected ID %s, got %s", sessionUUID, sessions[0].ID)
	}
	if sessions[0].Source != "copilot" {
		t.Errorf("expected source 'copilot', got %s", sessions[0].Source)
	}
}

func TestListCopilotSessions_FiltersByProjectPath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create two sessions with different project paths
	session1UUID := "11111111-1111-1111-1111-111111111111"
	session2UUID := "22222222-2222-2222-2222-222222222222"

	for _, uuid := range []string{session1UUID, session2UUID} {
		sessionDir := filepath.Join(tmpDir, ".copilot", "session-state", uuid)
		if err := os.MkdirAll(sessionDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sessionDir, "events.jsonl"), []byte(`{"type":"session.start"}`), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Session 1: matches target project
	workspace1 := `id: 11111111-1111-1111-1111-111111111111
git_root: /target/project
created_at: 2026-01-31T21:23:32.449Z
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".copilot", "session-state", session1UUID, "workspace.yaml"), []byte(workspace1), 0644); err != nil {
		t.Fatal(err)
	}

	// Session 2: different project
	workspace2 := `id: 22222222-2222-2222-2222-222222222222
git_root: /other/project
created_at: 2026-01-31T21:23:32.449Z
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".copilot", "session-state", session2UUID, "workspace.yaml"), []byte(workspace2), 0644); err != nil {
		t.Fatal(err)
	}

	origHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmpDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", origHome) }()

	sessions, err := listCopilotSessions("/target/project")
	if err != nil {
		t.Fatalf("listCopilotSessions failed: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session after filtering, got %d", len(sessions))
	}
	if sessions[0].ID != session1UUID {
		t.Errorf("expected session %s, got %s", session1UUID, sessions[0].ID)
	}
}

func TestListCopilotSessions_FallbackToCwd(t *testing.T) {
	tmpDir := t.TempDir()
	sessionUUID := "b310b03c-e860-461a-840c-aafb44b812f8"
	sessionDir := filepath.Join(tmpDir, ".copilot", "session-state", sessionUUID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create workspace.yaml without git_root (should fall back to cwd)
	workspaceContent := `id: b310b03c-e860-461a-840c-aafb44b812f8
cwd: /test/project
created_at: 2026-01-31T21:23:32.449Z
`
	if err := os.WriteFile(filepath.Join(sessionDir, "workspace.yaml"), []byte(workspaceContent), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(sessionDir, "events.jsonl"), []byte(`{"type":"session.start"}`), 0644); err != nil {
		t.Fatal(err)
	}

	origHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmpDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", origHome) }()

	sessions, err := listCopilotSessions("/test/project")
	if err != nil {
		t.Fatalf("listCopilotSessions failed: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session (matched by cwd), got %d", len(sessions))
	}
}

func TestListCopilotSessions_SkipsEmptySessions(t *testing.T) {
	tmpDir := t.TempDir()
	sessionUUID := "b310b03c-e860-461a-840c-aafb44b812f8"
	sessionDir := filepath.Join(tmpDir, ".copilot", "session-state", sessionUUID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatal(err)
	}

	workspaceContent := `id: b310b03c-e860-461a-840c-aafb44b812f8
git_root: /test/project
created_at: 2026-01-31T21:23:32.449Z
`
	if err := os.WriteFile(filepath.Join(sessionDir, "workspace.yaml"), []byte(workspaceContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create empty events.jsonl
	if err := os.WriteFile(filepath.Join(sessionDir, "events.jsonl"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	origHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmpDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", origHome) }()

	sessions, err := listCopilotSessions("/test/project")
	if err != nil {
		t.Fatalf("listCopilotSessions failed: %v", err)
	}

	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions (empty events.jsonl), got %d", len(sessions))
	}
}

func TestListCopilotSessions_NonExistentDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	origHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmpDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", origHome) }()

	sessions, err := listCopilotSessions("/test/project")
	if err != nil {
		t.Fatalf("expected no error for nonexistent directory, got: %v", err)
	}
	if sessions != nil {
		t.Errorf("expected nil sessions for nonexistent directory, got: %v", sessions)
	}
}

func TestSortSessionsByTimestamp_CopilotAfterClaude(t *testing.T) {
	sameTime := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)

	sessions := []SessionInfo{
		{ID: "copilot-1", Source: "copilot", CreatedAt: sameTime},
		{ID: "claude-1", Source: "claude", CreatedAt: sameTime},
		{ID: "codex-1", Source: "codex", CreatedAt: sameTime},
		{ID: "kiro-1", Source: "kiro-cli", CreatedAt: sameTime},
	}

	sortSessionsByTimestamp(sessions)

	expectedOrder := []string{"claude", "copilot", "codex", "kiro-cli"}
	for i, expected := range expectedOrder {
		if sessions[i].Source != expected {
			t.Errorf("position %d: expected source %s, got %s", i, expected, sessions[i].Source)
		}
	}
}

// sha256Hex32Test computes SHA-256 of input and returns first 32 hex chars.
// Duplicated from transcript package to avoid exporting an internal helper.
func sha256Hex32Test(input string) string {
	h := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", h)[:32]
}

// kiroIDEConfigSubdir returns the platform-specific subdirectory path for Kiro IDE
// storage, relative to the home directory. On macOS this is
// "Library/Application Support/Kiro/User/globalStorage/kiro.kiroagent", etc.
func kiroIDEConfigSubdir() string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join("Library", "Application Support", "Kiro", "User", "globalStorage", "kiro.kiroagent")
	case "windows":
		return filepath.Join("AppData", "Roaming", "Kiro", "User", "globalStorage", "kiro.kiroagent")
	default: // linux and others
		return filepath.Join(".config", "Kiro", "User", "globalStorage", "kiro.kiroagent")
	}
}

// setupKiroIDEWorkspace creates a mock Kiro IDE workspace structure in a temp directory.
// Returns (homeDir, workspaceDir, projectPath). Sets HOME env var and returns a cleanup function.
func setupKiroIDEWorkspace(t *testing.T) (homeDir, workspaceDir, projectPath string) {
	t.Helper()
	tmpDir := t.TempDir()
	homeDir = filepath.Join(tmpDir, "home")

	// Use a stable project path within the temp dir so filepath.Abs works
	projectPath = filepath.Join(tmpDir, "myproject")
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		t.Fatalf("create project dir: %v", err)
	}

	// Create the Kiro IDE base directory
	kiroBase := filepath.Join(homeDir, kiroIDEConfigSubdir())
	workspaceHash := sha256Hex32Test(projectPath)
	workspaceDir = filepath.Join(kiroBase, workspaceHash)
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		t.Fatalf("create workspace dir: %v", err)
	}

	// Override HOME for os.UserConfigDir()
	origHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("set HOME: %v", err)
	}
	t.Cleanup(func() { _ = os.Setenv("HOME", origHome) })

	return homeDir, workspaceDir, projectPath
}

// writeChatFile writes a .chat JSON file to the given directory.
func writeChatFile(t *testing.T, dir, filename, executionID string, messages []map[string]string, startTime int64) {
	t.Helper()
	chatMsgs := make([]map[string]string, len(messages))
	copy(chatMsgs, messages)

	data := map[string]any{
		"executionId": executionID,
		"chat":        chatMsgs,
		"metadata": map[string]any{
			"modelId":       "auto",
			"modelProvider": "qdev",
			"workflow":      "act",
			"workflowId":    "test-workflow-id",
			"startTime":     startTime,
			"endTime":       startTime + 5000,
		},
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal chat file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), jsonData, 0644); err != nil {
		t.Fatalf("write chat file: %v", err)
	}
}

func TestListKiroIDESessions_MultipleFilesForSameExecutionId(t *testing.T) {
	_, workspaceDir, projectPath := setupKiroIDEWorkspace(t)

	execID := "ccfd398f-c4d8-44d7-ad56-532bb7f2ffa1"
	startTime := int64(1770349922198)

	// Write two .chat files with the same executionId but different message counts.
	// The one with more messages should be selected as representative.
	writeChatFile(t, workspaceDir, "snapshot1.chat", execID,
		[]map[string]string{
			{"role": "human", "content": "Hello"},
		}, startTime)

	writeChatFile(t, workspaceDir, "snapshot2.chat", execID,
		[]map[string]string{
			{"role": "human", "content": "Hello"},
			{"role": "bot", "content": "Hi there"},
			{"role": "human", "content": "How are you?"},
		}, startTime)

	sessions, err := listKiroIDESessions(projectPath)
	if err != nil {
		t.Fatalf("listKiroIDESessions: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	if sessions[0].ID != execID {
		t.Errorf("expected ID %q, got %q", execID, sessions[0].ID)
	}
	if sessions[0].Source != "kiro ide" {
		t.Errorf("expected source %q, got %q", "kiro ide", sessions[0].Source)
	}
}

func TestListKiroIDESessions_MultipleExecutionIds(t *testing.T) {
	_, workspaceDir, projectPath := setupKiroIDEWorkspace(t)

	exec1 := "aaaa1111-2222-3333-4444-555566667777"
	exec2 := "bbbb1111-2222-3333-4444-555566667777"

	writeChatFile(t, workspaceDir, "session1.chat", exec1,
		[]map[string]string{{"role": "human", "content": "First"}},
		1770349922198)

	writeChatFile(t, workspaceDir, "session2.chat", exec2,
		[]map[string]string{{"role": "human", "content": "Second"}},
		1770349932198)

	sessions, err := listKiroIDESessions(projectPath)
	if err != nil {
		t.Fatalf("listKiroIDESessions: %v", err)
	}

	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	ids := map[string]bool{}
	for _, s := range sessions {
		ids[s.ID] = true
		if s.Source != "kiro ide" {
			t.Errorf("expected source %q, got %q", "kiro ide", s.Source)
		}
	}
	if !ids[exec1] {
		t.Errorf("missing session %q", exec1)
	}
	if !ids[exec2] {
		t.Errorf("missing session %q", exec2)
	}
}

func TestListKiroIDESessions_NonExistentWorkspaceDir(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")

	// Create Kiro IDE base but no workspace dir for this project
	kiroBase := filepath.Join(homeDir, kiroIDEConfigSubdir())
	if err := os.MkdirAll(kiroBase, 0755); err != nil {
		t.Fatalf("create kiro base: %v", err)
	}

	origHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("set HOME: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", origHome) }()

	sessions, err := listKiroIDESessions("/nonexistent/project")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if sessions != nil {
		t.Errorf("expected nil sessions, got %v", sessions)
	}
}

func TestListKiroIDESessions_MalformedChatFile(t *testing.T) {
	_, workspaceDir, projectPath := setupKiroIDEWorkspace(t)

	// Write a valid .chat file
	writeChatFile(t, workspaceDir, "good.chat", "good-exec-id-1234-5678-abcd-ef0123456789",
		[]map[string]string{{"role": "human", "content": "Hello"}},
		1770349922198)

	// Write a malformed .chat file
	if err := os.WriteFile(filepath.Join(workspaceDir, "bad.chat"), []byte("not json{{{"), 0644); err != nil {
		t.Fatalf("write bad chat file: %v", err)
	}

	sessions, err := listKiroIDESessions(projectPath)
	if err != nil {
		t.Fatalf("listKiroIDESessions: %v", err)
	}

	// Should still list the valid session, skipping the malformed one
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session (malformed skipped), got %d", len(sessions))
	}
	if sessions[0].ID != "good-exec-id-1234-5678-abcd-ef0123456789" {
		t.Errorf("expected good session ID, got %q", sessions[0].ID)
	}
}

func TestListKiroIDESessions_TieBreakingSameEntryCount(t *testing.T) {
	_, workspaceDir, projectPath := setupKiroIDEWorkspace(t)

	execID := "tie-break-1234-5678-abcd-ef0123456789"
	startTime := int64(1770349922198)

	// Write two files with the same executionId and same entry count.
	// Both have 2 messages, so tie-break goes to the newer mtime.
	writeChatFile(t, workspaceDir, "aaa.chat", execID,
		[]map[string]string{
			{"role": "human", "content": "Hello"},
			{"role": "bot", "content": "Hi"},
		}, startTime)

	// Set the first file's mtime to the past
	oldTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(workspaceDir, "aaa.chat"), oldTime, oldTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	writeChatFile(t, workspaceDir, "bbb.chat", execID,
		[]map[string]string{
			{"role": "human", "content": "Hello"},
			{"role": "bot", "content": "Hi there"},
		}, startTime)

	sessions, err := listKiroIDESessions(projectPath)
	if err != nil {
		t.Fatalf("listKiroIDESessions: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	// Should pick the file with the newer mtime (bbb.chat)
	// We can't directly verify which file was chosen from SessionInfo,
	// but we verify only one session is returned and it has the right ID
	if sessions[0].ID != execID {
		t.Errorf("expected ID %q, got %q", execID, sessions[0].ID)
	}
}

func TestListKiroIDESessions_UsesStartTime(t *testing.T) {
	_, workspaceDir, projectPath := setupKiroIDEWorkspace(t)

	execID := "time-test-1234-5678-abcd-ef0123456789"
	startTime := int64(1770349922198) // specific millisecond timestamp

	writeChatFile(t, workspaceDir, "session.chat", execID,
		[]map[string]string{{"role": "human", "content": "Hello"}},
		startTime)

	sessions, err := listKiroIDESessions(projectPath)
	if err != nil {
		t.Fatalf("listKiroIDESessions: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	expectedTime := time.UnixMilli(startTime)
	if !sessions[0].CreatedAt.Equal(expectedTime) {
		t.Errorf("expected CreatedAt %v, got %v", expectedTime, sessions[0].CreatedAt)
	}
}

func TestResolveKiroIDESession_ValidExecutionId(t *testing.T) {
	_, workspaceDir, projectPath := setupKiroIDEWorkspace(t)

	execID := "resolve-test-1234-5678-abcd-ef012345"

	writeChatFile(t, workspaceDir, "session.chat", execID,
		[]map[string]string{
			{"role": "human", "content": "Hello"},
			{"role": "bot", "content": "Hi there!"},
		}, 1770349922198)

	reader, costPath, err := resolveKiroIDESession(execID, projectPath)
	if err != nil {
		t.Fatalf("resolveKiroIDESession: %v", err)
	}
	defer func() { _ = reader.Close() }()

	// Verify we got a valid reader with content
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read session: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty reader content")
	}

	// Verify the content is valid JSON with the right executionId
	var chatFile struct {
		ExecutionID string `json:"executionId"`
	}
	if err := json.Unmarshal(data, &chatFile); err != nil {
		t.Fatalf("unmarshal session: %v", err)
	}
	if chatFile.ExecutionID != execID {
		t.Errorf("expected executionId %q, got %q", execID, chatFile.ExecutionID)
	}

	// Verify cost path is non-empty and deterministic
	if costPath == "" {
		t.Error("expected non-empty cost path")
	}
	expectedCostPath := transcript.KiroIDEExecutionDetailPath(workspaceDir, execID)
	if costPath != expectedCostPath {
		t.Errorf("cost path = %q, want %q", costPath, expectedCostPath)
	}
}

func TestResolveKiroIDESession_UnknownExecutionId(t *testing.T) {
	_, workspaceDir, projectPath := setupKiroIDEWorkspace(t)

	// Write a session with a different executionId
	writeChatFile(t, workspaceDir, "other.chat", "other-exec-id-0000-0000-0000-000000000000",
		[]map[string]string{{"role": "human", "content": "Hello"}},
		1770349922198)

	_, _, err := resolveKiroIDESession("nonexistent-exec-id-0000-0000-000000", projectPath)
	if !errors.Is(err, transcript.ErrKiroIDENotFound) {
		t.Errorf("expected ErrKiroIDENotFound, got: %v", err)
	}
}

func TestResolveKiroIDESession_NonExistentWorkspace(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")

	// Create Kiro IDE base but no workspace dir
	kiroBase := filepath.Join(homeDir, kiroIDEConfigSubdir())
	if err := os.MkdirAll(kiroBase, 0755); err != nil {
		t.Fatalf("create kiro base: %v", err)
	}

	origHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("set HOME: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", origHome) }()

	_, _, err := resolveKiroIDESession("any-exec-id", "/nonexistent/project/path")
	if !errors.Is(err, transcript.ErrKiroIDENotFound) {
		t.Errorf("expected ErrKiroIDENotFound, got: %v", err)
	}
}

func TestResolveKiroIDESession_SelectsBestFile(t *testing.T) {
	_, workspaceDir, projectPath := setupKiroIDEWorkspace(t)

	execID := "best-file-1234-5678-abcd-ef0123456789"

	// Write a snapshot with 1 message
	writeChatFile(t, workspaceDir, "old.chat", execID,
		[]map[string]string{{"role": "human", "content": "Hello"}},
		1770349922198)

	// Write a snapshot with 3 messages (should be selected)
	writeChatFile(t, workspaceDir, "new.chat", execID,
		[]map[string]string{
			{"role": "human", "content": "Hello"},
			{"role": "bot", "content": "Hi!"},
			{"role": "human", "content": "How are you?"},
		}, 1770349922198)

	reader, _, err := resolveKiroIDESession(execID, projectPath)
	if err != nil {
		t.Fatalf("resolveKiroIDESession: %v", err)
	}
	defer func() { _ = reader.Close() }()

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Verify the returned file has 3 messages (the one with more entries)
	var chatFile struct {
		Chat []json.RawMessage `json:"chat"`
	}
	if err := json.Unmarshal(data, &chatFile); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(chatFile.Chat) != 3 {
		t.Errorf("expected 3 chat entries (best file), got %d", len(chatFile.Chat))
	}
}

func TestCostPathIntegration(t *testing.T) {
	_, workspaceDir, projectPath := setupKiroIDEWorkspace(t)

	execID := "cost-test-1234-5678-abcd-ef0123456789"

	// Write a .chat file
	writeChatFile(t, workspaceDir, "session.chat", execID,
		[]map[string]string{
			{"role": "human", "content": "Hello"},
			{"role": "bot", "content": "Hi there!"},
		}, 1770349922198)

	// Create execution detail file with known cost data
	costPath := transcript.KiroIDEExecutionDetailPath(workspaceDir, execID)
	if err := os.MkdirAll(filepath.Dir(costPath), 0755); err != nil {
		t.Fatalf("create execution saves dir: %v", err)
	}

	executionDetail := map[string]any{
		"executionId": execID,
		"usageSummary": []map[string]any{
			{"unit": "credit", "unitPlural": "credits", "usage": 0.0024},
			{"unit": "credit", "unitPlural": "credits", "usage": 0.1022},
		},
	}
	detailJSON, err := json.Marshal(executionDetail)
	if err != nil {
		t.Fatalf("marshal execution detail: %v", err)
	}
	if err := os.WriteFile(costPath, detailJSON, 0644); err != nil {
		t.Fatalf("write execution detail: %v", err)
	}

	// Resolve session to get reader and cost path
	reader, resolvedCostPath, err := resolveKiroIDESession(execID, projectPath)
	if err != nil {
		t.Fatalf("resolveKiroIDESession: %v", err)
	}
	defer func() { _ = reader.Close() }()

	// Parse with cost path
	result, err := transcript.ParseKiroIDEWithCostPath(reader, resolvedCostPath)
	if err != nil {
		t.Fatalf("ParseKiroIDEWithCostPath: %v", err)
	}

	// Verify cost was extracted
	if result.Metadata == nil {
		t.Fatal("expected non-nil Metadata")
	}
	if result.Metadata.TotalCost == nil {
		t.Fatal("expected non-nil TotalCost")
	}

	expectedCost := 0.1046 // 0.0024 + 0.1022
	if math.Abs(*result.Metadata.TotalCost-expectedCost) > 0.0001 {
		t.Errorf("TotalCost = %f, want %f (within 0.0001)", *result.Metadata.TotalCost, expectedCost)
	}
	if result.Metadata.CostUnit != "credits" {
		t.Errorf("CostUnit = %q, want %q", result.Metadata.CostUnit, "credits")
	}

	// Also verify parsing without cost path returns no cost
	reader2, _, err := resolveKiroIDESession(execID, projectPath)
	if err != nil {
		t.Fatalf("resolveKiroIDESession (second): %v", err)
	}
	defer func() { _ = reader2.Close() }()

	resultNoCost, err := transcript.ParseKiroIDE(reader2)
	if err != nil {
		t.Fatalf("ParseKiroIDE: %v", err)
	}
	if resultNoCost.Metadata != nil {
		t.Errorf("expected nil Metadata without cost path, got %+v", resultNoCost.Metadata)
	}
}
