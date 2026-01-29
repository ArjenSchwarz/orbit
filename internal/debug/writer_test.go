package debug

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestNewFileWriter(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		runID         string
		wantErr       bool
		wantNilWriter bool
	}{
		"valid runID creates writer": {
			runID:         "test-run-123",
			wantErr:       false,
			wantNilWriter: false,
		},
		"empty runID returns error": {
			runID:         "",
			wantErr:       true,
			wantNilWriter: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w, err := NewFileWriter(tc.runID)
			if (err != nil) != tc.wantErr {
				t.Fatalf("NewFileWriter(%q) error = %v, wantErr %v", tc.runID, err, tc.wantErr)
			}
			if (w == nil) != tc.wantNilWriter {
				t.Fatalf("NewFileWriter(%q) writer = %v, wantNilWriter %v", tc.runID, w, tc.wantNilWriter)
			}

			if w != nil {
				defer func() {
					_ = w.Close()
					_ = os.Remove(w.Path())
				}()

				// Verify file was created
				if _, err := os.Stat(w.Path()); err != nil {
					t.Errorf("log file not created at %s: %v", w.Path(), err)
				}

				// Verify filename format
				filename := filepath.Base(w.Path())
				if !strings.HasSuffix(filename, ".jsonl") {
					t.Errorf("filename %q does not end with .jsonl", filename)
				}
				if !strings.Contains(filename, tc.runID) {
					t.Errorf("filename %q does not contain runID %q", filename, tc.runID)
				}

				// Verify directory is ~/.orbit/logs/
				homeDir, _ := os.UserHomeDir()
				wantDir := filepath.Join(homeDir, ".orbit", "logs")
				gotDir := filepath.Dir(w.Path())
				if gotDir != wantDir {
					t.Errorf("log directory = %q, want %q", gotDir, wantDir)
				}
			}
		})
	}
}

func TestNewVariantFileWriter(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		runID         string
		variantNum    int
		wantErr       bool
		wantNilWriter bool
	}{
		"valid runID and variantNum creates writer": {
			runID:         "test-run-456",
			variantNum:    1,
			wantErr:       false,
			wantNilWriter: false,
		},
		"empty runID returns error": {
			runID:         "",
			variantNum:    1,
			wantErr:       true,
			wantNilWriter: true,
		},
		"variantNum zero returns error": {
			runID:         "test-run-789",
			variantNum:    0,
			wantErr:       true,
			wantNilWriter: true,
		},
		"variantNum negative returns error": {
			runID:         "test-run-abc",
			variantNum:    -1,
			wantErr:       true,
			wantNilWriter: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w, err := NewVariantFileWriter(tc.runID, tc.variantNum)
			if (err != nil) != tc.wantErr {
				t.Fatalf("NewVariantFileWriter(%q, %d) error = %v, wantErr %v", tc.runID, tc.variantNum, err, tc.wantErr)
			}
			if (w == nil) != tc.wantNilWriter {
				t.Fatalf("NewVariantFileWriter(%q, %d) writer = %v, wantNilWriter %v", tc.runID, tc.variantNum, w, tc.wantNilWriter)
			}

			if w != nil {
				defer func() {
					_ = w.Close()
					_ = os.Remove(w.Path())
				}()

				// Verify filename format includes variant number
				filename := filepath.Base(w.Path())
				if !strings.Contains(filename, "variant-1") {
					t.Errorf("filename %q does not contain 'variant-1'", filename)
				}
			}
		})
	}
}

func TestFileWriterWrite(t *testing.T) {
	t.Parallel()

	w, err := NewFileWriter("write-test-run")
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	defer func() {
		_ = w.Close()
		_ = os.Remove(w.Path())
	}()

	entry := LogEntry{
		Timestamp: time.Date(2025, 1, 28, 12, 0, 0, 0, time.UTC),
		Level:     "info",
		Component: "test",
		Message:   "test message",
		Fields:    map[string]any{"key": "value"},
	}

	if err := w.Write(entry); err != nil {
		t.Errorf("Write() error = %v", err)
	}

	// Read file and verify JSONL format
	data, err := os.ReadFile(w.Path())
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	var got LogEntry
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("failed to parse JSON line: %v", err)
	}

	if diff := cmp.Diff(entry, got); diff != "" {
		t.Errorf("parsed entry mismatch (-want +got):\n%s", diff)
	}
}

func TestFileWriterWriteMultiple(t *testing.T) {
	t.Parallel()

	w, err := NewFileWriter("multi-write-test")
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	defer func() {
		_ = w.Close()
		_ = os.Remove(w.Path())
	}()

	entries := []LogEntry{
		{Timestamp: time.Now(), Level: "info", Component: "test", Message: "first"},
		{Timestamp: time.Now(), Level: "debug", Component: "test", Message: "second"},
		{Timestamp: time.Now(), Level: "error", Component: "test", Message: "third"},
	}

	for _, e := range entries {
		if err := w.Write(e); err != nil {
			t.Errorf("Write() error = %v", err)
		}
	}

	// Verify all entries are valid JSONL
	data, err := os.ReadFile(w.Path())
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}

	for i, line := range lines {
		var entry LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Errorf("line %d is not valid JSON: %v", i, err)
		}
	}
}

func TestFileWriterConcurrentWrites(t *testing.T) {
	t.Parallel()

	w, err := NewFileWriter("concurrent-test")
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	defer func() {
		_ = w.Close()
		_ = os.Remove(w.Path())
	}()

	const numGoroutines = 10
	const writesPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for range numGoroutines {
		go func() {
			defer wg.Done()
			for range writesPerGoroutine {
				entry := LogEntry{
					Timestamp: time.Now(),
					Level:     "info",
					Component: "concurrent",
					Message:   "concurrent write",
				}
				_ = w.Write(entry)
			}
		}()
	}

	wg.Wait()

	// Verify all entries are valid JSONL (no interleaved lines)
	data, err := os.ReadFile(w.Path())
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	want := numGoroutines * writesPerGoroutine
	if len(lines) != want {
		t.Errorf("expected %d lines, got %d", want, len(lines))
	}

	for i, line := range lines {
		var entry LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Errorf("line %d is not valid JSON: %v\nline content: %s", i, err, line)
		}
	}
}

func TestFileWriterPath(t *testing.T) {
	t.Parallel()

	t.Run("returns path for valid writer", func(t *testing.T) {
		t.Parallel()

		w, err := NewFileWriter("path-test")
		if err != nil {
			t.Fatalf("failed to create writer: %v", err)
		}
		defer func() {
			_ = w.Close()
			_ = os.Remove(w.Path())
		}()

		got := w.Path()
		if got == "" {
			t.Error("Path() returned empty string for valid writer")
		}
	})

	t.Run("returns empty for nil writer", func(t *testing.T) {
		t.Parallel()

		var w *FileWriter
		got := w.Path()
		if got != "" {
			t.Errorf("Path() = %q, want empty string for nil writer", got)
		}
	})
}

func TestFileWriterClose(t *testing.T) {
	t.Parallel()

	t.Run("closes file", func(t *testing.T) {
		t.Parallel()

		w, err := NewFileWriter("close-test")
		if err != nil {
			t.Fatalf("failed to create writer: %v", err)
		}
		path := w.Path()
		defer func() { _ = os.Remove(path) }()

		if err := w.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}

		// Write after close should be a no-op
		err = w.Write(LogEntry{Message: "after close"})
		if err != nil {
			t.Errorf("Write() after Close() error = %v", err)
		}
	})

	t.Run("nil receiver returns nil", func(t *testing.T) {
		t.Parallel()

		var w *FileWriter
		if err := w.Close(); err != nil {
			t.Errorf("Close() on nil writer error = %v", err)
		}
	})

	t.Run("double close is safe", func(t *testing.T) {
		t.Parallel()

		w, err := NewFileWriter("double-close-test")
		if err != nil {
			t.Fatalf("failed to create writer: %v", err)
		}
		path := w.Path()
		defer func() { _ = os.Remove(path) }()

		_ = w.Close()
		if err := w.Close(); err != nil {
			t.Errorf("second Close() error = %v", err)
		}
	})
}

func TestFileWriterNilReceiverWrite(t *testing.T) {
	t.Parallel()

	var w *FileWriter
	err := w.Write(LogEntry{Message: "test"})
	if err != nil {
		t.Errorf("Write() on nil writer error = %v", err)
	}
}

func TestFileWriterWarningRateLimit(t *testing.T) {
	t.Parallel()

	// Create a writer with a very short warning interval for testing
	w := &FileWriter{
		warningInterval: 50 * time.Millisecond,
	}

	// First warning should be emitted
	msg1 := w.checkWarningLocked("test error: %v", "first")
	if msg1 == "" {
		t.Error("first warning should be emitted")
	}

	// Immediate second warning should be suppressed
	msg2 := w.checkWarningLocked("test error: %v", "second")
	if msg2 != "" {
		t.Errorf("second immediate warning should be suppressed, got: %s", msg2)
	}

	// Wait for interval to pass
	time.Sleep(60 * time.Millisecond)

	// Warning after interval should be emitted
	msg3 := w.checkWarningLocked("test error: %v", "third")
	if msg3 == "" {
		t.Error("warning after interval should be emitted")
	}
}

func TestFileWriterFilePermissions(t *testing.T) {
	t.Parallel()

	w, err := NewFileWriter("permissions-test")
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	defer func() {
		_ = w.Close()
		_ = os.Remove(w.Path())
	}()

	info, err := os.Stat(w.Path())
	if err != nil {
		t.Fatalf("failed to stat log file: %v", err)
	}

	// Check that file permissions are 0600 (owner read/write only)
	got := info.Mode().Perm()
	want := os.FileMode(0600)
	if got != want {
		t.Errorf("file permissions = %o, want %o", got, want)
	}
}

func TestFileWriterStartupEntry(t *testing.T) {
	t.Parallel()

	runID := fmt.Sprintf("startup-test-%d", time.Now().UnixNano())
	w, err := NewFileWriter(runID)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	defer func() {
		_ = w.Close()
		_ = os.Remove(w.Path())
	}()

	entry := StartupEntry{
		Timestamp:        time.Date(2025, 1, 28, 12, 0, 0, 0, time.UTC),
		Level:            "info",
		Component:        "orchestrator",
		Message:          "Orchestration started",
		SchemaVersion:    1,
		OrbitVersion:     "0.1.0",
		Agent:            "claude-code",
		TasksFile:        "/path/to/tasks.md",
		WorkingDirectory: "/path/to/project",
		BranchName:       "main",
	}

	if err := w.Write(entry); err != nil {
		t.Errorf("Write() error = %v", err)
	}

	data, err := os.ReadFile(w.Path())
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	var got StartupEntry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("failed to parse startup entry: %v", err)
	}

	if got.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", got.SchemaVersion)
	}
}

func TestFileWriterShutdownEntry(t *testing.T) {
	t.Parallel()

	runID := fmt.Sprintf("shutdown-test-%d", time.Now().UnixNano())
	w, err := NewFileWriter(runID)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	defer func() {
		_ = w.Close()
		_ = os.Remove(w.Path())
	}()

	entry := ShutdownEntry{
		Timestamp:     time.Date(2025, 1, 28, 12, 10, 0, 0, time.UTC),
		Level:         "info",
		Component:     "orchestrator",
		Message:       "Orchestration completed",
		TotalDuration: "10m0s",
		FinalStatus:   "completed",
	}

	if err := w.Write(entry); err != nil {
		t.Errorf("Write() error = %v", err)
	}

	data, err := os.ReadFile(w.Path())
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	var got ShutdownEntry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("failed to parse shutdown entry: %v", err)
	}

	if got.FinalStatus != "completed" {
		t.Errorf("FinalStatus = %q, want %q", got.FinalStatus, "completed")
	}
}
