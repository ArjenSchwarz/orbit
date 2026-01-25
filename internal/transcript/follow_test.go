package transcript

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
	if !strings.Contains(warnBuf.String(), "line 2") {
		t.Errorf("expected warning about line 2, got: %s", warnBuf.String())
	}
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
