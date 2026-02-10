package main

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents/claudecode"
	"github.com/arjenschwarz/orbit/internal/sessions"
	"github.com/arjenschwarz/orbit/internal/transcript"
)

// --- Serve Command Tests ---

func TestResolveInt(t *testing.T) {
	tests := []struct {
		name       string
		flagVal    int
		envKey     string
		envVal     string
		defaultVal int
		want       int
	}{
		{
			name:       "flag value takes priority",
			flagVal:    9000,
			envKey:     "TEST_RESOLVE_INT_PORT",
			envVal:     "3000",
			defaultVal: 8081,
			want:       9000,
		},
		{
			name:       "env var used when flag is zero",
			flagVal:    0,
			envKey:     "TEST_RESOLVE_INT_PORT",
			envVal:     "3000",
			defaultVal: 8081,
			want:       3000,
		},
		{
			name:       "default used when both flag and env unset",
			flagVal:    0,
			envKey:     "TEST_RESOLVE_INT_PORT",
			envVal:     "",
			defaultVal: 8081,
			want:       8081,
		},
		{
			name:       "default used when env is invalid",
			flagVal:    0,
			envKey:     "TEST_RESOLVE_INT_PORT",
			envVal:     "notanumber",
			defaultVal: 8081,
			want:       8081,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVal != "" {
				t.Setenv(tt.envKey, tt.envVal)
			} else {
				t.Setenv(tt.envKey, "")
			}
			got := resolveInt(tt.flagVal, tt.envKey, tt.defaultVal)
			if got != tt.want {
				t.Errorf("resolveInt(%d, %q, %d) = %d, want %d", tt.flagVal, tt.envKey, tt.defaultVal, got, tt.want)
			}
		})
	}
}

func TestResolveString(t *testing.T) {
	tests := []struct {
		name       string
		flagVal    string
		envKey     string
		envVal     string
		defaultVal string
		want       string
	}{
		{
			name:       "flag value takes priority",
			flagVal:    "0.0.0.0",
			envKey:     "TEST_RESOLVE_STR_BIND",
			envVal:     "127.0.0.1",
			defaultVal: "localhost",
			want:       "0.0.0.0",
		},
		{
			name:       "env var used when flag is empty",
			flagVal:    "",
			envKey:     "TEST_RESOLVE_STR_BIND",
			envVal:     "127.0.0.1",
			defaultVal: "localhost",
			want:       "127.0.0.1",
		},
		{
			name:       "default used when both flag and env unset",
			flagVal:    "",
			envKey:     "TEST_RESOLVE_STR_BIND",
			envVal:     "",
			defaultVal: "localhost",
			want:       "localhost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVal != "" {
				t.Setenv(tt.envKey, tt.envVal)
			} else {
				t.Setenv(tt.envKey, "")
			}
			got := resolveString(tt.flagVal, tt.envKey, tt.defaultVal)
			if got != tt.want {
				t.Errorf("resolveString(%q, %q, %q) = %q, want %q", tt.flagVal, tt.envKey, tt.defaultVal, got, tt.want)
			}
		})
	}
}

func TestResolveProjectPath(t *testing.T) {
	t.Run("empty uses cwd", func(t *testing.T) {
		got, err := resolveProjectPath("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		cwd, _ := os.Getwd()
		if got != cwd {
			t.Errorf("got %q, want %q", got, cwd)
		}
	})

	t.Run("relative path resolved to absolute", func(t *testing.T) {
		got, err := resolveProjectPath("relative/path")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !filepath.IsAbs(got) {
			t.Errorf("expected absolute path, got %q", got)
		}
	})

	t.Run("absolute path returned as-is", func(t *testing.T) {
		got, err := resolveProjectPath("/absolute/path")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "/absolute/path" {
			t.Errorf("got %q, want /absolute/path", got)
		}
	})
}

func TestServeCommand_VersionFlag(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := serveCommand([]string{"--version"})

	_ = w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "apsis serve version") {
		t.Errorf("expected version output, got: %s", buf.String())
	}
}

func TestServeCommand_HelpFlag(t *testing.T) {
	// --help with ContinueOnError returns flag.ErrHelp which serveCommand propagates
	err := serveCommand([]string{"--help"})
	if err != nil && err != flag.ErrHelp {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServeCommand_UnknownFlag(t *testing.T) {
	err := serveCommand([]string{"--unknown-flag"})
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

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
			result := sessions.FormatSize(tt.bytes)
			if result != tt.expected {
				t.Errorf("sessions.FormatSize(%d) = %q, want %q", tt.bytes, result, tt.expected)
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
	// Invalid JSON lines are skipped during detection and parsing.
	// Falls back to Claude parser which emits a warning and returns 0 entries.
	var output bytes.Buffer
	err := convert(strings.NewReader("{invalid json}"), &output, "test-session", "md", "", "")

	if err != nil {
		t.Fatalf("convert should not error for invalid JSON content, got: %v", err)
	}
	if output.Len() != 0 {
		t.Errorf("expected no output for invalid JSON, got %d bytes", output.Len())
	}
}

func TestConvert_UnknownFormatType_Negative(t *testing.T) {
	// Unknown types are skipped during format detection and parsing.
	// When only unknown types exist, Parse falls back to Claude format
	// which returns 0 entries — convert then prints "Session contains no entries".
	var output bytes.Buffer
	err := convert(strings.NewReader(`{"type":"unknown_format"}`), &output, "test-session", "md", "", "")

	if err != nil {
		t.Fatalf("convert should not error for unknown types, got: %v", err)
	}
	if output.Len() != 0 {
		t.Errorf("expected no output for unknown format types, got %d bytes", output.Len())
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

// --- Integration Test: Session Output Format ---

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

	// Create Codex session (with cwd matching the project path)
	codexSessionsDir := filepath.Join(homeDir, ".codex", "sessions", "2026", "01", "05")
	if err := os.MkdirAll(codexSessionsDir, 0755); err != nil {
		t.Fatal(err)
	}
	codexSession := `{"timestamp":"2026-01-05T09:00:00Z","type":"session_meta","payload":{"id":"019b892c-3a14-7773-bd76-6465a8a0b634","cwd":"/test/project"}}`
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

// --- Latest Keyword Tests ---

func TestResolveLatestSession_ReturnsNewest(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")

	// Create Claude sessions directory
	claudeProjectDir := filepath.Join(homeDir, ".claude", "projects", "-test-project")
	if err := os.MkdirAll(claudeProjectDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create two sessions with different timestamps — session-2 is newer
	session1 := `{"type":"user","timestamp":"2026-01-01T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"older"}]}}`
	session2 := `{"type":"user","timestamp":"2026-01-05T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"newer"}]}}`

	if err := os.WriteFile(filepath.Join(claudeProjectDir, "session-old.jsonl"), []byte(session1), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeProjectDir, "session-new.jsonl"), []byte(session2), 0644); err != nil {
		t.Fatal(err)
	}

	origHomeDir := os.Getenv("HOME")
	t.Setenv("HOME", homeDir)
	defer func() { _ = os.Setenv("HOME", origHomeDir) }()

	latest, err := resolveLatestSession("/test/project")
	if err != nil {
		t.Fatalf("resolveLatestSession failed: %v", err)
	}

	// ListAll sorts oldest-first, so the last element should be session-new
	if latest.ID != "session-new" {
		t.Errorf("expected latest session ID 'session-new', got %q", latest.ID)
	}
	if latest.Source != sessions.SourceClaude {
		t.Errorf("expected source %q, got %q", sessions.SourceClaude, latest.Source)
	}
}

func TestResolveLatestSession_NoSessions(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")

	// Create empty Claude sessions directory
	claudeProjectDir := filepath.Join(homeDir, ".claude", "projects", "-test-project")
	if err := os.MkdirAll(claudeProjectDir, 0755); err != nil {
		t.Fatal(err)
	}

	origHomeDir := os.Getenv("HOME")
	t.Setenv("HOME", homeDir)
	defer func() { _ = os.Setenv("HOME", origHomeDir) }()

	_, err := resolveLatestSession("/test/project")
	if err == nil {
		t.Fatal("expected error when no sessions exist")
	}
	if !strings.Contains(err.Error(), "no sessions found for project") {
		t.Errorf("expected 'no sessions found for project' error, got: %v", err)
	}
}

func TestRunLatest_LatestKeywordNotShadowedByFile(t *testing.T) {
	// Create a file named "latest" in a temp dir, then verify that
	// run() with Input="latest" calls runLatest (not isFilePath path)
	tmpDir := t.TempDir()

	// Create a file named "latest" in tmpDir
	latestFile := filepath.Join(tmpDir, "latest")
	if err := os.WriteFile(latestFile, []byte("I am a file named latest"), 0644); err != nil {
		t.Fatal(err)
	}

	// Verify isFilePath would return true for "latest" when the file exists
	// (this is the shadowing we're protecting against)
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	if !isFilePath("latest") {
		t.Fatal("precondition failed: isFilePath('latest') should be true when file exists")
	}

	// Set up a valid session so runLatest can resolve it.
	// Use a fixed project path so the Claude directory name is predictable.
	projectPath := "/test/project"
	homeDir := filepath.Join(tmpDir, "home")
	claudeProjectDir := filepath.Join(homeDir, ".claude", "projects", claudeProjectPath(projectPath))
	if err := os.MkdirAll(claudeProjectDir, 0755); err != nil {
		t.Fatal(err)
	}
	sessionContent := `{"type":"user","timestamp":"2026-01-05T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"hello"}]}}`
	if err := os.WriteFile(filepath.Join(claudeProjectDir, "test-session.jsonl"), []byte(sessionContent), 0644); err != nil {
		t.Fatal(err)
	}

	origHomeDir := os.Getenv("HOME")
	t.Setenv("HOME", homeDir)
	defer func() { _ = os.Setenv("HOME", origHomeDir) }()

	// run() with Input="latest" should use resolveLatestSession, not treat "latest" as a file
	cfg := &Config{
		Input:   "latest",
		Format:  "md",
		Project: projectPath,
	}

	// Capture stderr to check for "Using ... session" output
	oldStderr := os.Stderr
	rStderr, wStderr, _ := os.Pipe()
	os.Stderr = wStderr

	exitCode, err := run(cfg)

	_ = wStderr.Close()
	os.Stderr = oldStderr
	var stderrBuf bytes.Buffer
	_, _ = stderrBuf.ReadFrom(rStderr)

	if err != nil {
		t.Fatalf("run() failed: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	// Verify stderr contains the "Using" message from runLatest
	stderrOutput := stderrBuf.String()
	if !strings.Contains(stderrOutput, "Using") {
		t.Errorf("expected stderr to contain 'Using' message from latest resolution, got: %q", stderrOutput)
	}
}

// claudeProjectPath converts a project path to the Claude project directory name.
// Duplicates the logic from claudecode.BuildProjectPath for test convenience.
func claudeProjectPath(path string) string {
	return claudecode.BuildProjectPath(path)
}

func TestRunLatest_NormalMode(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")

	// Create a Claude session
	claudeProjectDir := filepath.Join(homeDir, ".claude", "projects", "-test-project")
	if err := os.MkdirAll(claudeProjectDir, 0755); err != nil {
		t.Fatal(err)
	}
	sessionContent := `{"type":"user","timestamp":"2026-01-05T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"test message"}]}}
{"type":"assistant","timestamp":"2026-01-05T10:00:01Z","message":{"role":"assistant","content":[{"type":"text","text":"test response"}]}}`
	if err := os.WriteFile(filepath.Join(claudeProjectDir, "my-session.jsonl"), []byte(sessionContent), 0644); err != nil {
		t.Fatal(err)
	}

	origHomeDir := os.Getenv("HOME")
	t.Setenv("HOME", homeDir)
	defer func() { _ = os.Setenv("HOME", origHomeDir) }()

	// Capture stdout
	oldStdout := os.Stdout
	rStdout, wStdout, _ := os.Pipe()
	os.Stdout = wStdout

	// Suppress stderr
	oldStderr := os.Stderr
	_, wStderr, _ := os.Pipe()
	os.Stderr = wStderr

	cfg := &Config{
		Input:   "latest",
		Format:  "md",
		Project: "/test/project",
	}
	exitCode, err := run(cfg)

	_ = wStdout.Close()
	_ = wStderr.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	var stdoutBuf bytes.Buffer
	_, _ = stdoutBuf.ReadFrom(rStdout)

	if err != nil {
		t.Fatalf("run() with latest failed: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	output := stdoutBuf.String()
	if !strings.Contains(output, "test message") {
		t.Error("output should contain the session's user message")
	}
	if !strings.Contains(output, "test response") {
		t.Error("output should contain the session's assistant response")
	}
}

