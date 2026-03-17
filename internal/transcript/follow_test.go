package transcript

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// syncBuffer is a thread-safe buffer for concurrent read/write in tests.
type syncBuffer struct {
	mu  sync.RWMutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.buf.String()
}

func (b *syncBuffer) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.buf.Len()
}

func TestHashLine(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  [16]byte
	}{
		{
			name:  "empty input",
			input: []byte{},
			want:  [16]byte{0xe3, 0xb0, 0xc4, 0x42, 0x98, 0xfc, 0x1c, 0x14, 0x9a, 0xfb, 0xf4, 0xc8, 0x99, 0x6f, 0xb9, 0x24},
		},
		{
			name:  "simple JSON",
			input: []byte(`{"type":"user"}`),
			want:  [16]byte{0x7e, 0xe0, 0x87, 0xec, 0xed, 0x9d, 0xa5, 0x88, 0xb7, 0x88, 0xe9, 0xf7, 0x5d, 0x93, 0x24, 0x53},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hashLine(tt.input)
			if got != tt.want {
				t.Errorf("hashLine() = %x, want %x", got, tt.want)
			}
		})
	}
}

func TestHashLine_Deterministic(t *testing.T) {
	input := []byte(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Hello"}]}}`)

	hash1 := hashLine(input)
	hash2 := hashLine(input)

	if hash1 != hash2 {
		t.Errorf("hashLine() not deterministic: %x != %x", hash1, hash2)
	}
}

func TestHashLine_DifferentInputs(t *testing.T) {
	input1 := []byte(`{"type":"user","message":"hello"}`)
	input2 := []byte(`{"type":"user","message":"world"}`)

	hash1 := hashLine(input1)
	hash2 := hashLine(input2)

	if hash1 == hash2 {
		t.Error("hashLine() produced same hash for different inputs")
	}
}

func TestLineWithHash(t *testing.T) {
	raw := []byte(`{"type":"user"}`)
	hash := hashLine(raw)

	lwh := lineWithHash{
		raw:  raw,
		hash: hash,
	}

	if !bytes.Equal(lwh.raw, raw) {
		t.Error("lineWithHash raw field not set correctly")
	}
	if lwh.hash != hash {
		t.Error("lineWithHash hash field not set correctly")
	}
}

func TestGetFileInfo(t *testing.T) {
	// Create temp file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.jsonl")
	content := []byte(`{"type":"user"}` + "\n")
	if err := os.WriteFile(tmpFile, content, 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	// Get file info
	mtime, inode, size, err := getFileInfo(tmpFile)
	if err != nil {
		t.Fatalf("getFileInfo() error: %v", err)
	}

	// Verify mtime is recent (within last minute)
	if time.Since(mtime) > time.Minute {
		t.Errorf("mtime %v is too old", mtime)
	}

	// Verify size matches content
	if size != int64(len(content)) {
		t.Errorf("size = %d, want %d", size, len(content))
	}

	// Inode should be non-zero on Unix, zero on Windows
	// We don't enforce a specific value, just that the function works
	_ = inode
}

func TestGetFileInfo_MissingFile(t *testing.T) {
	_, _, _, err := getFileInfo("/nonexistent/path/to/file.jsonl")
	if err == nil {
		t.Error("getFileInfo() should return error for missing file")
	}
}

func TestGetFileInfo_MtimeChanges(t *testing.T) {
	// Create temp file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.jsonl")
	if err := os.WriteFile(tmpFile, []byte(`{"type":"user"}`+"\n"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	mtime1, _, size1, err := getFileInfo(tmpFile)
	if err != nil {
		t.Fatalf("getFileInfo() error: %v", err)
	}

	// Wait a bit and modify the file
	time.Sleep(10 * time.Millisecond)

	// Append to file
	f, err := os.OpenFile(tmpFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open file for append: %v", err)
	}
	if _, err := f.WriteString(`{"type":"assistant"}` + "\n"); err != nil {
		_ = f.Close()
		t.Fatalf("failed to write to file: %v", err)
	}
	_ = f.Close()

	mtime2, _, size2, err := getFileInfo(tmpFile)
	if err != nil {
		t.Fatalf("getFileInfo() error: %v", err)
	}

	// Size should increase
	if size2 <= size1 {
		t.Errorf("size did not increase: %d <= %d", size2, size1)
	}

	// Mtime should be >= original (may be equal due to filesystem granularity)
	if mtime2.Before(mtime1) {
		t.Errorf("mtime went backwards: %v < %v", mtime2, mtime1)
	}
}

func TestReadAndHashLines_Normal(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.jsonl")

	content := `{"type":"user","message":{"role":"user","content":"hello"}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"world"}]}}
`
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	var warnBuf bytes.Buffer
	lines, err := readAndHashLines(tmpFile, &warnBuf)
	if err != nil {
		t.Fatalf("readAndHashLines() error: %v", err)
	}

	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	// Verify hashes are computed
	if lines[0].hash == [16]byte{} {
		t.Error("first line hash is empty")
	}
	if lines[1].hash == [16]byte{} {
		t.Error("second line hash is empty")
	}

	// Verify hashes are different
	if lines[0].hash == lines[1].hash {
		t.Error("different lines have same hash")
	}

	// Verify raw bytes can be parsed
	for i, line := range lines {
		var entry Entry
		if err := entry.UnmarshalJSON(line.raw); err != nil {
			t.Errorf("line %d raw bytes failed to parse: %v", i, err)
		}
	}

	// No warnings expected
	if warnBuf.Len() > 0 {
		t.Errorf("unexpected warnings: %s", warnBuf.String())
	}
}

func TestReadAndHashLines_IncompleteEOF(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.jsonl")

	// Simulate incomplete JSON at EOF (partial write)
	content := `{"type":"user","message":{"role":"user","content":"hello"}}
{"type":"assistant","message":`
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	var warnBuf bytes.Buffer
	lines, err := readAndHashLines(tmpFile, &warnBuf)
	if err != nil {
		t.Fatalf("readAndHashLines() error: %v", err)
	}

	// Should have 1 valid line, incomplete line silently skipped
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	// No warning for incomplete EOF (expected during active writing)
	if warnBuf.Len() > 0 {
		t.Errorf("unexpected warnings for incomplete EOF: %s", warnBuf.String())
	}
}

func TestReadAndHashLines_CorruptMidFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.jsonl")

	// Corrupt line in the middle, followed by valid line
	content := `{"type":"user","message":{"role":"user","content":"hello"}}
this is not valid JSON at all
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"world"}]}}
`
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	var warnBuf bytes.Buffer
	lines, err := readAndHashLines(tmpFile, &warnBuf)
	if err != nil {
		t.Fatalf("readAndHashLines() error: %v", err)
	}

	// Should have 2 valid lines, corrupt line skipped
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	// Should have warning for corrupt mid-file line
	assert.Contains(t, warnBuf.String(), "line 2", "expected warning about line 2, got: %s", warnBuf.String())
}

// Regression tests for T-462: consecutive malformed lines must each produce a warning.
func TestReadAndHashLines_ConsecutiveMalformedLines(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.jsonl")

	// Three consecutive corrupt lines (2, 3, 4) followed by a valid line
	content := `{"type":"user","message":{"role":"user","content":"hello"}}
not valid json line two
also broken line three
still broken line four
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"world"}]}}
`
	require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0644))

	var warnBuf bytes.Buffer
	lines, err := readAndHashLines(tmpFile, &warnBuf)
	require.NoError(t, err)
	require.Len(t, lines, 2, "expected 2 valid lines (lines 1 and 5)")

	// Must warn about EACH corrupt mid-file line, not just the last one
	warnings := warnBuf.String()
	assert.Contains(t, warnings, "line 2", "missing warning for line 2")
	assert.Contains(t, warnings, "line 3", "missing warning for line 3")
	assert.Contains(t, warnings, "line 4", "missing warning for line 4")
}

func TestReadAndHashLines_ConsecutiveMalformedAtEnd(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.jsonl")

	// Two corrupt lines at the end; line 2 is definitely mid-file (line 3 follows it)
	// so it must produce a warning even though line 3 is also bad.
	content := `{"type":"user","message":{"role":"user","content":"hello"}}
corrupt mid-file line
{"incomplete": true`
	require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0644))

	var warnBuf bytes.Buffer
	lines, err := readAndHashLines(tmpFile, &warnBuf)
	require.NoError(t, err)
	require.Len(t, lines, 1, "only line 1 is valid")

	// Line 2 is corrupt mid-file (line 3 exists after it) — must warn
	// Line 3 is the final line — may be incomplete, no warning required
	warnings := warnBuf.String()
	assert.Contains(t, warnings, "line 2", "missing warning for corrupt mid-file line 2")
}

func TestReadAndHashLines_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.jsonl")

	if err := os.WriteFile(tmpFile, []byte{}, 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	var warnBuf bytes.Buffer
	lines, err := readAndHashLines(tmpFile, &warnBuf)
	if err != nil {
		t.Fatalf("readAndHashLines() error: %v", err)
	}

	if len(lines) != 0 {
		t.Fatalf("expected 0 lines, got %d", len(lines))
	}
}

func TestReadAndHashLines_MissingFile(t *testing.T) {
	var warnBuf bytes.Buffer
	_, err := readAndHashLines("/nonexistent/path/to/file.jsonl", &warnBuf)
	if err == nil {
		t.Error("readAndHashLines() should return error for missing file")
	}
}

func TestReadAndHashLines_WithCRLF(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.jsonl")

	// Windows-style line endings
	content := "{\"type\":\"user\"}\r\n{\"type\":\"assistant\"}\r\n"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	var warnBuf bytes.Buffer
	lines, err := readAndHashLines(tmpFile, &warnBuf)
	if err != nil {
		t.Fatalf("readAndHashLines() error: %v", err)
	}

	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	// Verify raw bytes don't contain \r
	for i, line := range lines {
		if bytes.Contains(line.raw, []byte{'\r'}) {
			t.Errorf("line %d raw bytes contain \\r", i)
		}
	}
}

func TestReadAndHashLines_SkipsEmptyLines(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.jsonl")

	content := `{"type":"user"}

{"type":"assistant"}

`
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	var warnBuf bytes.Buffer
	lines, err := readAndHashLines(tmpFile, &warnBuf)
	if err != nil {
		t.Fatalf("readAndHashLines() error: %v", err)
	}

	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (empty lines skipped), got %d", len(lines))
	}
}

func TestReadAndHashLines_NilWarnWriter(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.jsonl")

	// Corrupt line in the middle to trigger warning
	content := `{"type":"user"}
not valid json
{"type":"assistant"}
`
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	// Should not panic with nil warnWriter
	lines, err := readAndHashLines(tmpFile, io.Discard)
	if err != nil {
		t.Fatalf("readAndHashLines() error: %v", err)
	}

	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
}

// --- Follower tests ---

func TestNewFollower(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.jsonl")

	// File doesn't exist - should fail
	_, err := NewFollower(tmpFile, io.Discard, RenderOptions{})
	if err == nil {
		t.Error("NewFollower should fail for non-existent file")
	}

	// Create file
	if err := os.WriteFile(tmpFile, []byte(`{"type":"user"}`+"\n"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	// Now it should succeed
	f, err := NewFollower(tmpFile, io.Discard, RenderOptions{})
	if err != nil {
		t.Fatalf("NewFollower() error: %v", err)
	}

	if f.filePath != tmpFile {
		t.Errorf("filePath = %q, want %q", f.filePath, tmpFile)
	}
	if f.seenHashes == nil {
		t.Error("seenHashes not initialized")
	}
}

func TestFollower_Poll_MtimeChange(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.jsonl")

	// Create file
	if err := os.WriteFile(tmpFile, []byte(`{"type":"user"}`+"\n"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	f, err := NewFollower(tmpFile, io.Discard, RenderOptions{})
	if err != nil {
		t.Fatalf("NewFollower() error: %v", err)
	}

	// First poll should detect change (from zero state)
	changed, err := f.poll()
	if err != nil {
		t.Fatalf("poll() error: %v", err)
	}
	if !changed {
		t.Error("first poll should detect change from zero state")
	}

	// Second poll with no changes
	changed, err = f.poll()
	if err != nil {
		t.Fatalf("poll() error: %v", err)
	}
	if changed {
		t.Error("second poll should not detect change")
	}

	// Append to file
	fh, err := os.OpenFile(tmpFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	if _, err := fh.WriteString(`{"type":"assistant"}` + "\n"); err != nil {
		_ = fh.Close()
		t.Fatalf("failed to write: %v", err)
	}
	_ = fh.Close()

	// Third poll should detect change
	changed, err = f.poll()
	if err != nil {
		t.Fatalf("poll() error: %v", err)
	}
	if !changed {
		t.Error("poll should detect change after append")
	}
}

func TestFollower_Poll_Truncation(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.jsonl")

	// Create file with multiple entries
	if err := os.WriteFile(tmpFile, []byte(`{"type":"user"}`+"\n"+`{"type":"assistant"}`+"\n"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	var buf bytes.Buffer
	f, err := NewFollower(tmpFile, &buf, RenderOptions{})
	if err != nil {
		t.Fatalf("NewFollower() error: %v", err)
	}

	// Initialize state
	_, _ = f.poll()
	f.seenHashes[[16]byte{1}] = struct{}{} // Simulate having seen entries
	f.initialRenderDone = true

	// Truncate the file
	if err := os.WriteFile(tmpFile, []byte(`{"type":"user"}`+"\n"), 0644); err != nil {
		t.Fatalf("failed to truncate file: %v", err)
	}

	// Poll should detect truncation and clear state
	changed, err := f.poll()
	if err != nil {
		t.Fatalf("poll() error: %v", err)
	}
	if !changed {
		t.Error("poll should detect truncation")
	}
	if len(f.seenHashes) != 0 {
		t.Error("seenHashes should be cleared on truncation")
	}
	if f.initialRenderDone {
		t.Error("initialRenderDone should be reset on truncation")
	}
}

func TestFollower_Poll_FileDeleted(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.jsonl")

	// Create file
	if err := os.WriteFile(tmpFile, []byte(`{"type":"user"}`+"\n"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	f, err := NewFollower(tmpFile, io.Discard, RenderOptions{})
	if err != nil {
		t.Fatalf("NewFollower() error: %v", err)
	}

	// Initialize state
	_, _ = f.poll()

	// Delete file
	if err := os.Remove(tmpFile); err != nil {
		t.Fatalf("failed to remove file: %v", err)
	}

	// Poll should return error
	_, err = f.poll()
	if err == nil {
		t.Error("poll should return error when file is deleted")
	}
}

func TestFollower_ProcessFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.jsonl")

	// Create file with user entry
	if err := os.WriteFile(tmpFile, []byte(`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"Hello"}]}}`+"\n"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	var buf bytes.Buffer
	f, err := NewFollower(tmpFile, &buf, RenderOptions{})
	if err != nil {
		t.Fatalf("NewFollower() error: %v", err)
	}
	f.warn = io.Discard // Suppress warnings

	// Process file
	if err := f.processFile(); err != nil {
		t.Fatalf("processFile() error: %v", err)
	}

	// Should have output
	output := buf.String()
	assert.Contains(t, output, "Session Transcript", "output should contain header")
	assert.Contains(t, output, "Hello", "output should contain entry content")
	if !f.initialRenderDone {
		t.Error("initialRenderDone should be true after first render")
	}
	if len(f.seenHashes) != 1 {
		t.Errorf("seenHashes should have 1 entry, got %d", len(f.seenHashes))
	}
}

func TestFollower_ProcessFile_Incremental(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.jsonl")

	// Create file with user entry
	if err := os.WriteFile(tmpFile, []byte(`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"First"}]}}`+"\n"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	var buf bytes.Buffer
	f, err := NewFollower(tmpFile, &buf, RenderOptions{})
	if err != nil {
		t.Fatalf("NewFollower() error: %v", err)
	}
	f.warn = io.Discard

	// Initial render
	if err := f.processFile(); err != nil {
		t.Fatalf("processFile() error: %v", err)
	}

	initialOutput := buf.String()
	assert.Contains(t, initialOutput, "Session Transcript", "initial output should contain header")

	// Append new entry
	fh, err := os.OpenFile(tmpFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	if _, err := fh.WriteString(`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"Second"}]}}` + "\n"); err != nil {
		_ = fh.Close()
		t.Fatalf("failed to write: %v", err)
	}
	_ = fh.Close()

	// Process again
	buf.Reset()
	if err := f.processFile(); err != nil {
		t.Fatalf("processFile() error: %v", err)
	}

	incrementalOutput := buf.String()
	// Incremental output should NOT contain header again
	assert.NotContains(t, incrementalOutput, "Session Transcript", "incremental output should not contain header")
	assert.Contains(t, incrementalOutput, "Second", "incremental output should contain new entry")
	if len(f.seenHashes) != 2 {
		t.Errorf("seenHashes should have 2 entries, got %d", len(f.seenHashes))
	}
}

func TestFollower_AddSeenHash_Cap(t *testing.T) {
	f := &Follower{
		seenHashes:        make(map[[16]byte]struct{}),
		initialRenderDone: true,
	}

	// Fill up to just below cap
	for i := range maxSeenHashes - 1 {
		h := [16]byte{byte(i >> 24), byte(i >> 16), byte(i >> 8), byte(i)}
		f.addSeenHash(h)
	}

	if len(f.seenHashes) != maxSeenHashes-1 {
		t.Errorf("expected %d hashes, got %d", maxSeenHashes-1, len(f.seenHashes))
	}
	if !f.initialRenderDone {
		t.Error("initialRenderDone should still be true")
	}

	// Add one more to reach cap, then one more to trigger reset
	f.addSeenHash([16]byte{0xff, 0xff, 0xff, 0xfe})
	if len(f.seenHashes) != maxSeenHashes {
		t.Errorf("expected %d hashes at cap, got %d", maxSeenHashes, len(f.seenHashes))
	}

	// Next add should trigger reset
	f.addSeenHash([16]byte{0xff, 0xff, 0xff, 0xff})
	if len(f.seenHashes) != 1 {
		t.Errorf("after reset, expected 1 hash, got %d", len(f.seenHashes))
	}
	if f.initialRenderDone {
		t.Error("initialRenderDone should be reset after cap exceeded")
	}
}

func TestFollower_Run_WithCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.jsonl")

	// Create file
	if err := os.WriteFile(tmpFile, []byte(`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"Hello"}]}}`+"\n"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	var buf syncBuffer
	f, err := NewFollower(tmpFile, &buf, RenderOptions{})
	if err != nil {
		t.Fatalf("NewFollower() error: %v", err)
	}
	f.warn = io.Discard

	// Start follower with cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- f.Run(ctx)
	}()

	// Wait for initial render
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), "Hello") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	assert.Contains(t, buf.String(), "Hello", "expected initial content to be rendered")

	// Cancel and wait for clean shutdown
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run() returned error on cancellation: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return after cancellation")
	}
}

func TestFollower_Run_DetectsChanges(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.jsonl")

	// Create file
	if err := os.WriteFile(tmpFile, []byte(`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"First"}]}}`+"\n"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	var buf syncBuffer
	f, err := NewFollower(tmpFile, &buf, RenderOptions{})
	if err != nil {
		t.Fatalf("NewFollower() error: %v", err)
	}
	f.warn = io.Discard

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- f.Run(ctx)
	}()

	// Wait for initial render
	waitForOutput(t, &buf, "First", 1500*time.Millisecond)

	// Append new entry
	fh, err := os.OpenFile(tmpFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	if _, err := fh.WriteString(`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"Second"}]}}` + "\n"); err != nil {
		_ = fh.Close()
		t.Fatalf("failed to write: %v", err)
	}
	_ = fh.Close()

	// Wait for new content
	if !waitForOutput(t, &buf, "Second", 1500*time.Millisecond) {
		t.Error("expected new entry to be rendered")
	}

	cancel()
	<-done
}

// waitForOutput polls the buffer until expected content appears or timeout.
func waitForOutput(t *testing.T, buf *syncBuffer, expected string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), expected) {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// --- End-to-End Integration Tests ---

// TestFollowerIntegration_BasicFollowWithAppend tests that new entries are rendered
// as they are appended to the file (requirements 4.1, 4.2).
func TestFollowerIntegration_BasicFollowWithAppend(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "session.jsonl")

	// Create file with initial entry
	initialContent := `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"Initial message"}]}}` + "\n"
	if err := os.WriteFile(tmpFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	var buf syncBuffer
	f, err := NewFollower(tmpFile, &buf, RenderOptions{Title: "Session Transcript"})
	if err != nil {
		t.Fatalf("NewFollower() error: %v", err)
	}
	f.warn = io.Discard

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- f.Run(ctx)
	}()

	// Wait for initial content to be rendered
	if !waitForOutput(t, &buf, "Initial message", 2*time.Second) {
		t.Fatal("initial content not rendered within timeout")
	}

	// Verify header is present in initial render (markdown header format: "# Session Transcript")
	assert.Contains(t, buf.String(), "# Session Transcript", "expected header in initial render")

	initialOutputLen := buf.Len()

	// Append new entry
	fh, err := os.OpenFile(tmpFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open file for append: %v", err)
	}
	_, _ = fh.WriteString(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Appended response"}]}}` + "\n")
	_ = fh.Close()

	// Wait for appended content
	if !waitForOutput(t, &buf, "Appended response", 2*time.Second) {
		t.Error("appended content not rendered within timeout")
	}

	// Verify incremental output doesn't contain header again
	incrementalOutput := buf.String()[initialOutputLen:]
	assert.NotContains(t, incrementalOutput, "# Session Transcript", "incremental output should not contain header")

	cancel()
	<-done
}

// TestFollowerIntegration_FileTruncation tests that truncated files cause
// a full re-render (requirement 3.5).
func TestFollowerIntegration_FileTruncation(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "session.jsonl")

	// Create file with initial content (longer to ensure clear truncation detection)
	initialContent := `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"First entry with lots of content to ensure size difference"}]}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Second entry with even more content to make the file larger"}]}}
`
	if err := os.WriteFile(tmpFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	var buf syncBuffer
	f, err := NewFollower(tmpFile, &buf, RenderOptions{Title: "Session Transcript"})
	if err != nil {
		t.Fatalf("NewFollower() error: %v", err)
	}
	f.warn = io.Discard

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- f.Run(ctx)
	}()

	// Wait for initial render
	if !waitForOutput(t, &buf, "Second entry", 2*time.Second) {
		t.Fatal("initial content not rendered within timeout")
	}

	// Wait for at least one poll cycle to complete so lastSize is set
	time.Sleep(600 * time.Millisecond)

	// Count headers before truncation (markdown format: "# Session Transcript")
	initialHeaderCount := strings.Count(buf.String(), "# Session Transcript")
	if initialHeaderCount != 1 {
		t.Errorf("expected 1 header in initial render, got %d", initialHeaderCount)
	}

	// Truncate and write new content (simulates agent restart with new session)
	// The new content is SHORTER than the original to trigger truncation detection
	newContent := `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"Short"}]}}` + "\n"
	if err := os.WriteFile(tmpFile, []byte(newContent), 0644); err != nil {
		t.Fatalf("failed to truncate file: %v", err)
	}

	// Wait for new content after truncation (give more time for poll cycle)
	if !waitForOutput(t, &buf, "Short", 2*time.Second) {
		t.Error("content after truncation not rendered within timeout")
	}

	// Should have a new header after truncation (re-render)
	finalHeaderCount := strings.Count(buf.String(), "# Session Transcript")
	if finalHeaderCount != 2 {
		t.Errorf("expected 2 headers after truncation re-render, got %d", finalHeaderCount)
	}

	cancel()
	<-done
}

// TestFollowerIntegration_FileReplacement tests that file replacement (inode change)
// causes a full re-render (requirements 3.6, 3.7).
func TestFollowerIntegration_FileReplacement(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "session.jsonl")

	// Create initial file
	initialContent := `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"Original file"}]}}` + "\n"
	if err := os.WriteFile(tmpFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	var buf syncBuffer
	f, err := NewFollower(tmpFile, &buf, RenderOptions{Title: "Session Transcript"})
	if err != nil {
		t.Fatalf("NewFollower() error: %v", err)
	}
	f.warn = io.Discard

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- f.Run(ctx)
	}()

	// Wait for initial render
	if !waitForOutput(t, &buf, "Original file", 2*time.Second) {
		t.Fatal("initial content not rendered within timeout")
	}

	// Wait for at least one poll cycle to complete so lastInode is set
	time.Sleep(600 * time.Millisecond)

	// Replace file atomically (new inode)
	// This simulates how agents might crash and restart, creating a new file
	newFile := filepath.Join(tmpDir, "session.jsonl.new")
	newContent := `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"Replaced file"}]}}` + "\n"
	if err := os.WriteFile(newFile, []byte(newContent), 0644); err != nil {
		t.Fatalf("failed to create new file: %v", err)
	}

	// Atomic replace: remove old, rename new
	if err := os.Remove(tmpFile); err != nil {
		t.Fatalf("failed to remove old file: %v", err)
	}
	if err := os.Rename(newFile, tmpFile); err != nil {
		t.Fatalf("failed to rename new file: %v", err)
	}

	// Wait for new content after replacement (give more time for poll cycle)
	if !waitForOutput(t, &buf, "Replaced file", 2*time.Second) {
		t.Error("content after file replacement not rendered within timeout")
	}

	// Should have re-rendered with header (markdown format)
	headerCount := strings.Count(buf.String(), "# Session Transcript")
	if headerCount < 2 {
		t.Errorf("expected at least 2 headers after file replacement, got %d", headerCount)
	}

	cancel()
	<-done
}

// TestFollowerIntegration_IncompleteJSONHandling tests that incomplete JSON lines
// at EOF are skipped and processed on the next poll (requirements 7.5, 7.6).
func TestFollowerIntegration_IncompleteJSONHandling(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "session.jsonl")

	// Create file with complete entry
	initialContent := `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"Complete entry"}]}}` + "\n"
	if err := os.WriteFile(tmpFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	var buf syncBuffer
	f, err := NewFollower(tmpFile, &buf, RenderOptions{Title: "Session Transcript"})
	if err != nil {
		t.Fatalf("NewFollower() error: %v", err)
	}
	f.warn = io.Discard

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- f.Run(ctx)
	}()

	// Wait for initial render
	if !waitForOutput(t, &buf, "Complete entry", 2*time.Second) {
		t.Fatal("initial content not rendered within timeout")
	}

	// Append incomplete JSON (simulates mid-write capture)
	fh, err := os.OpenFile(tmpFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open file for append: %v", err)
	}
	// Write partial JSON
	_, _ = fh.WriteString(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Partial`)
	_ = fh.Close()

	// Wait a bit to ensure poll cycle happens
	time.Sleep(600 * time.Millisecond)

	// Partial entry should NOT appear yet
	assert.NotContains(t, buf.String(), "Partial", "partial JSON should not be rendered")

	// Complete the JSON
	fh, err = os.OpenFile(tmpFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open file for completion: %v", err)
	}
	_, _ = fh.WriteString(` entry"}]}}` + "\n")
	_ = fh.Close()

	// Now it should appear
	if !waitForOutput(t, &buf, "Partial entry", 2*time.Second) {
		t.Error("completed entry not rendered within timeout")
	}

	cancel()
	<-done
}
