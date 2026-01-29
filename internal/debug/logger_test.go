package debug

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func uniqueRunID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func TestNewLogger(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		cfg      LoggerConfig
		wantErr  bool
		wantFile bool
		wantPath bool
	}{
		"stderr only": {
			cfg: LoggerConfig{
				StderrEnabled: true,
				FileEnabled:   false,
				Prefix:        "test",
			},
			wantErr:  false,
			wantFile: false,
			wantPath: false,
		},
		"file only": {
			cfg: LoggerConfig{
				StderrEnabled: false,
				FileEnabled:   true,
				RunID:         uniqueRunID("test-run-logger"),
				Prefix:        "test",
			},
			wantErr:  false,
			wantFile: true,
			wantPath: true,
		},
		"both enabled": {
			cfg: LoggerConfig{
				StderrEnabled: true,
				FileEnabled:   true,
				RunID:         uniqueRunID("test-run-both"),
				Prefix:        "test",
			},
			wantErr:  false,
			wantFile: true,
			wantPath: true,
		},
		"neither enabled": {
			cfg: LoggerConfig{
				StderrEnabled: false,
				FileEnabled:   false,
				Prefix:        "test",
			},
			wantErr:  false,
			wantFile: false,
			wantPath: false,
		},
		"file enabled but empty runID": {
			cfg: LoggerConfig{
				StderrEnabled: false,
				FileEnabled:   true,
				RunID:         "",
				Prefix:        "test",
			},
			wantErr:  false, // Empty RunID with FileEnabled just skips file creation
			wantFile: false,
			wantPath: false,
		},
		"variant mode": {
			cfg: LoggerConfig{
				StderrEnabled: false,
				FileEnabled:   true,
				RunID:         uniqueRunID("test-run-variant"),
				VariantNum:    1,
				Prefix:        "variant",
			},
			wantErr:  false,
			wantFile: true,
			wantPath: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			l, err := NewLogger(tc.cfg)
			if (err != nil) != tc.wantErr {
				t.Fatalf("NewLogger() error = %v, wantErr %v", err, tc.wantErr)
			}

			if err != nil {
				return
			}

			defer func() {
				l.Close()
				if l.fileWriter != nil {
					_ = os.Remove(l.fileWriter.Path())
				}
			}()

			if (l.fileWriter != nil) != tc.wantFile {
				t.Errorf("fileWriter present = %v, want %v", l.fileWriter != nil, tc.wantFile)
			}

			if (l.Path() != "") != tc.wantPath {
				t.Errorf("Path() non-empty = %v, want %v", l.Path() != "", tc.wantPath)
			}
		})
	}
}

func TestLoggerEnabled(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		logger *Logger
		want   bool
	}{
		"nil logger": {
			logger: nil,
			want:   false,
		},
		"stderr enabled": {
			logger: &Logger{stderrEnabled: true},
			want:   true,
		},
		"legacy enabled": {
			logger: &Logger{enabled: true},
			want:   true,
		},
		"both enabled": {
			logger: &Logger{stderrEnabled: true, enabled: true},
			want:   true,
		},
		"none enabled": {
			logger: &Logger{},
			want:   false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := tc.logger.Enabled()
			if got != tc.want {
				t.Errorf("Enabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLoggerLogStructured(t *testing.T) {
	t.Parallel()

	l, err := NewLogger(LoggerConfig{
		FileEnabled: true,
		RunID:       uniqueRunID("structured-test"),
		Prefix:      "test",
	})
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer func() {
		l.Close()
		_ = os.Remove(l.Path())
	}()

	fields := map[string]any{
		"key1": "value1",
		"key2": 42,
	}
	l.LogStructured("info", "test message", fields)

	// Read and verify
	data, err := os.ReadFile(l.Path())
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	var entry LogEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("failed to parse log entry: %v", err)
	}

	if entry.Level != "info" {
		t.Errorf("Level = %q, want %q", entry.Level, "info")
	}
	if entry.Message != "test message" {
		t.Errorf("Message = %q, want %q", entry.Message, "test message")
	}
	if entry.Component != "test" {
		t.Errorf("Component = %q, want %q", entry.Component, "test")
	}
	if entry.Fields["key1"] != "value1" {
		t.Errorf("Fields[key1] = %v, want %v", entry.Fields["key1"], "value1")
	}
}

func TestLoggerLogErrorWithChain(t *testing.T) {
	t.Parallel()

	l, err := NewLogger(LoggerConfig{
		FileEnabled: true,
		RunID:       uniqueRunID("error-chain-test"),
		Prefix:      "test",
	})
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer func() {
		l.Close()
		_ = os.Remove(l.Path())
	}()

	innerErr := errors.New("inner error")
	wrappedErr := fmt.Errorf("outer error: %w", innerErr)

	l.LogErrorWithChain("test error", wrappedErr, nil)

	// Read and verify
	data, err := os.ReadFile(l.Path())
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	var entry LogEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("failed to parse log entry: %v", err)
	}

	if entry.Level != "error" {
		t.Errorf("Level = %q, want %q", entry.Level, "error")
	}

	errorChain, ok := entry.Fields["error_chain"].([]any)
	if !ok {
		t.Fatalf("error_chain not present or wrong type")
	}
	if len(errorChain) != 2 {
		t.Errorf("error_chain length = %d, want 2", len(errorChain))
	}
}

func TestExtractErrorChain(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		err  error
		want []string
	}{
		"nil error": {
			err:  nil,
			want: nil,
		},
		"single error": {
			err:  errors.New("single"),
			want: []string{"single"},
		},
		"wrapped error": {
			err:  fmt.Errorf("outer: %w", errors.New("inner")),
			want: []string{"outer: inner", "inner"},
		},
		"double wrapped": {
			err:  fmt.Errorf("a: %w", fmt.Errorf("b: %w", errors.New("c"))),
			want: []string{"a: b: c", "b: c", "c"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := extractErrorChain(tc.err)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("extractErrorChain() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLoggerLogStartup(t *testing.T) {
	t.Parallel()

	l, err := NewLogger(LoggerConfig{
		FileEnabled: true,
		RunID:       uniqueRunID("startup-test"),
		Prefix:      "test",
	})
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer func() {
		l.Close()
		_ = os.Remove(l.Path())
	}()

	cfg := StartupConfig{
		OrbitVersion:     "0.1.0",
		Agent:            "claude-code",
		TasksFile:        "/path/to/tasks.md",
		WorkingDirectory: "/path/to/project",
		BranchName:       "main",
	}
	l.LogStartup(cfg)

	// Read and verify
	data, err := os.ReadFile(l.Path())
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	var entry StartupEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("failed to parse startup entry: %v", err)
	}

	if entry.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", entry.SchemaVersion)
	}
	if entry.OrbitVersion != "0.1.0" {
		t.Errorf("OrbitVersion = %q, want %q", entry.OrbitVersion, "0.1.0")
	}
	if entry.Agent != "claude-code" {
		t.Errorf("Agent = %q, want %q", entry.Agent, "claude-code")
	}
}

func TestLoggerLogShutdown(t *testing.T) {
	t.Parallel()

	l, err := NewLogger(LoggerConfig{
		FileEnabled: true,
		RunID:       uniqueRunID("shutdown-test"),
		Prefix:      "test",
	})
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	path := l.Path()
	defer func() { _ = os.Remove(path) }()

	l.LogShutdown("completed")

	// Second call should be no-op
	l.LogShutdown("failed")

	l.Close()

	// Read and verify only one shutdown entry
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line (single shutdown entry), got %d", len(lines))
	}

	var entry ShutdownEntry
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("failed to parse shutdown entry: %v", err)
	}

	if entry.FinalStatus != "completed" {
		t.Errorf("FinalStatus = %q, want %q", entry.FinalStatus, "completed")
	}
}

func TestLoggerClose(t *testing.T) {
	t.Parallel()

	t.Run("writes shutdown on close if not done", func(t *testing.T) {
		t.Parallel()

		l, err := NewLogger(LoggerConfig{
			FileEnabled: true,
			RunID:       uniqueRunID("close-test"),
			Prefix:      "test",
		})
		if err != nil {
			t.Fatalf("NewLogger() error = %v", err)
		}
		path := l.Path()
		defer func() { _ = os.Remove(path) }()

		l.Close()

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read log file: %v", err)
		}

		var entry ShutdownEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			t.Fatalf("failed to parse shutdown entry: %v", err)
		}

		if entry.FinalStatus != "interrupted" {
			t.Errorf("FinalStatus = %q, want %q", entry.FinalStatus, "interrupted")
		}
	})

	t.Run("nil receiver is safe", func(t *testing.T) {
		t.Parallel()

		var l *Logger
		l.Close() // Should not panic
	})
}

func TestLoggerPath(t *testing.T) {
	t.Parallel()

	t.Run("returns path for file-enabled logger", func(t *testing.T) {
		t.Parallel()

		l, err := NewLogger(LoggerConfig{
			FileEnabled: true,
			RunID:       uniqueRunID("path-test"),
			Prefix:      "test",
		})
		if err != nil {
			t.Fatalf("NewLogger() error = %v", err)
		}
		defer func() {
			l.Close()
			_ = os.Remove(l.Path())
		}()

		got := l.Path()
		if got == "" {
			t.Error("Path() returned empty string for file-enabled logger")
		}
	})

	t.Run("returns empty for file-disabled logger", func(t *testing.T) {
		t.Parallel()

		l, err := NewLogger(LoggerConfig{
			FileEnabled: false,
			Prefix:      "test",
		})
		if err != nil {
			t.Fatalf("NewLogger() error = %v", err)
		}
		defer l.Close()

		got := l.Path()
		if got != "" {
			t.Errorf("Path() = %q, want empty string for file-disabled logger", got)
		}
	})

	t.Run("nil receiver returns empty", func(t *testing.T) {
		t.Parallel()

		var l *Logger
		got := l.Path()
		if got != "" {
			t.Errorf("Path() = %q, want empty string for nil logger", got)
		}
	})
}

func TestLoggerMethodsNilSafe(t *testing.T) {
	t.Parallel()

	var l *Logger

	// None of these should panic
	l.Log("test")
	l.LogCmd("test", []string{"arg"}, "/path")
	l.LogCmdResult(0, "stdout", "stderr", time.Second)
	l.LogJSON(true, nil)
	l.LogSession("id", false, "action")
	l.LogRetry(1, 3, "error", "1s")
	l.LogConfig("key", "value")
	l.LogError("type", "message", true)
	l.LogStructured("info", "message", nil)
	l.LogErrorWithChain("message", errors.New("error"), nil)
	l.LogStartup(StartupConfig{})
	l.LogShutdown("completed")
	l.Close()
	_ = l.Path()
	_ = l.Enabled()
}

func TestLoggerDualOutput(t *testing.T) {
	t.Parallel()

	l, err := NewLogger(LoggerConfig{
		StderrEnabled: false, // Disable stderr for cleaner test
		FileEnabled:   true,
		RunID:         uniqueRunID("dual-output-test"),
		Prefix:        "agent",
	})
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer func() {
		l.Close()
		_ = os.Remove(l.Path())
	}()

	// Test LogCmd
	l.LogCmd("claude", []string{"--version"}, "/home/user")

	// Test LogRetry
	l.LogRetry(1, 3, "connection", "2s")

	// Test LogConfig
	l.LogConfig("agent", "claude-code")

	// Test LogError
	l.LogError("fatal", "something went wrong", false)

	// Read and verify
	data, err := os.ReadFile(l.Path())
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 log entries, got %d", len(lines))
	}

	// Verify each line is valid JSON
	for i, line := range lines {
		var entry LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Errorf("line %d is not valid JSON: %v", i, err)
		}
	}

	// Verify LogCmd entry
	var cmdEntry LogEntry
	if err := json.Unmarshal([]byte(lines[0]), &cmdEntry); err != nil {
		t.Fatalf("failed to parse LogCmd entry: %v", err)
	}
	if cmdEntry.Message != "Command execution" {
		t.Errorf("LogCmd message = %q, want %q", cmdEntry.Message, "Command execution")
	}
	if cmdEntry.Fields["command"] != "claude --version" {
		t.Errorf("LogCmd command = %v, want %v", cmdEntry.Fields["command"], "claude --version")
	}

	// Verify LogRetry entry
	var retryEntry LogEntry
	if err := json.Unmarshal([]byte(lines[1]), &retryEntry); err != nil {
		t.Fatalf("failed to parse LogRetry entry: %v", err)
	}
	if retryEntry.Component != "retry" {
		t.Errorf("LogRetry component = %q, want %q", retryEntry.Component, "retry")
	}
	if retryEntry.Level != "info" {
		t.Errorf("LogRetry level = %q, want %q", retryEntry.Level, "info")
	}
}

func TestLoggerBackwardCompatibility(t *testing.T) {
	t.Parallel()

	// Test that New() still works and creates a functional logger
	l := New(true, "test")
	if l == nil {
		t.Fatal("New() returned nil")
	}

	if !l.Enabled() {
		t.Error("New(true, ...) should create enabled logger")
	}

	// Test that legacy enabled field still works
	l2 := &Logger{enabled: true, prefix: "legacy"}
	if !l2.Enabled() {
		t.Error("Logger with legacy enabled=true should be enabled")
	}
}
