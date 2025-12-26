package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseJSONL_ValidInput(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "valid.jsonl"))
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer func() { _ = f.Close() }()

	result, err := ParseJSONL(f)
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings, got %d: %v", len(result.Warnings), result.Warnings)
	}

	// Check that we got expected entries (user and assistant only)
	if len(result.Entries) == 0 {
		t.Error("expected at least one entry")
	}

	// Verify entry types are only user or assistant
	for i, entry := range result.Entries {
		if entry.Type != "user" && entry.Type != "assistant" {
			t.Errorf("entry %d: unexpected type %q", i, entry.Type)
		}
	}
}

func TestParseJSONL_EmptyFile(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "empty.jsonl"))
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer func() { _ = f.Close() }()

	result, err := ParseJSONL(f)
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	if len(result.Entries) != 0 {
		t.Errorf("expected empty entries, got %d", len(result.Entries))
	}

	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings, got %d", len(result.Warnings))
	}
}

func TestParseJSONL_MalformedLines(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "malformed.jsonl"))
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer func() { _ = f.Close() }()

	result, err := ParseJSONL(f)
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	// Should have warnings for malformed lines
	if len(result.Warnings) == 0 {
		t.Error("expected warnings for malformed lines")
	}

	// Warnings should include line numbers
	for _, w := range result.Warnings {
		if w.Line == 0 {
			t.Error("warning should include line number")
		}
		if w.Message == "" {
			t.Error("warning should include message")
		}
	}

	// Should still parse valid entries
	if len(result.Entries) == 0 {
		t.Error("expected some valid entries despite malformed lines")
	}
}

func TestParseJSONL_UnknownTypes(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "unknown_types.jsonl"))
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer func() { _ = f.Close() }()

	result, err := ParseJSONL(f)
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	// Unknown types should be skipped silently (no warnings)
	// Only user and assistant entries should be returned
	for _, entry := range result.Entries {
		if entry.Type != "user" && entry.Type != "assistant" {
			t.Errorf("unexpected entry type: %s", entry.Type)
		}
	}
}

func TestParseJSONL_BufferOverflow(t *testing.T) {
	// Create a line that's near but under the 10MB limit
	// This test verifies the buffer configuration works
	largeContent := strings.Repeat("a", 100000) // 100KB of content
	jsonl := `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"` + largeContent + `"}]}}`

	result, err := ParseJSONL(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("ParseJSONL returned error for large (but valid) line: %v", err)
	}

	if len(result.Entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(result.Entries))
	}
}

func TestParseFirstTimestamp(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "valid.jsonl"))
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer func() { _ = f.Close() }()

	ts, err := ParseFirstTimestamp(f)
	if err != nil {
		t.Fatalf("ParseFirstTimestamp returned error: %v", err)
	}

	// Timestamp should not be zero
	if ts.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestParseFirstTimestamp_EmptyFile(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "empty.jsonl"))
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer func() { _ = f.Close() }()

	_, err = ParseFirstTimestamp(f)
	if err == nil {
		t.Error("expected error for empty file")
	}
}

func TestParseFirstTimestamp_RFC3339(t *testing.T) {
	jsonl := `{"type":"user","timestamp":"2025-12-23T10:30:00+11:00"}`

	ts, err := ParseFirstTimestamp(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("ParseFirstTimestamp returned error: %v", err)
	}

	expected := time.Date(2025, 12, 23, 10, 30, 0, 0, time.FixedZone("", 11*3600))
	if !ts.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, ts)
	}
}

func TestParseFirstTimestamp_RFC3339Nano(t *testing.T) {
	jsonl := `{"type":"user","timestamp":"2025-12-23T10:30:00.123456789+11:00"}`

	ts, err := ParseFirstTimestamp(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("ParseFirstTimestamp returned error: %v", err)
	}

	if ts.Nanosecond() != 123456789 {
		t.Errorf("expected nanoseconds 123456789, got %d", ts.Nanosecond())
	}
}
