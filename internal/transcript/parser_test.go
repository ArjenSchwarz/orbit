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

func TestParseJSONL_UserMessageStringContent(t *testing.T) {
	// User messages can have content as a plain string (not array)
	jsonl := `{"type":"user","timestamp":"2025-12-23T10:30:00+11:00","message":{"role":"user","content":"Hello, this is a plain string message"}}`

	result, err := ParseJSONL(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings, got %d: %v", len(result.Warnings), result.Warnings)
	}

	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.Entries))
	}

	entry := result.Entries[0]
	if entry.Type != "user" {
		t.Errorf("expected type 'user', got %q", entry.Type)
	}

	if len(entry.Message.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(entry.Message.Content))
	}

	content := entry.Message.Content[0]
	if content.Type != "text" {
		t.Errorf("expected content type 'text', got %q", content.Type)
	}

	if content.Text != "Hello, this is a plain string message" {
		t.Errorf("expected text 'Hello, this is a plain string message', got %q", content.Text)
	}
}

func TestParseJSONL_ToolResultArrayContent(t *testing.T) {
	// Tool results can have content as an array of content blocks
	jsonl := `{"type":"assistant","timestamp":"2025-12-23T10:30:00+11:00","message":{"role":"assistant","content":[{"type":"tool_result","content":[{"type":"text","text":"Result item 1"},{"type":"text","text":"Result item 2"}],"is_error":false}]}}`

	result, err := ParseJSONL(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings, got %d: %v", len(result.Warnings), result.Warnings)
	}

	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.Entries))
	}

	entry := result.Entries[0]
	if len(entry.Message.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(entry.Message.Content))
	}

	content := entry.Message.Content[0]
	if content.Type != "tool_result" {
		t.Errorf("expected content type 'tool_result', got %q", content.Type)
	}

	// Array content should be serialized to JSON string
	if content.Content == "" {
		t.Error("expected content to be non-empty (array serialized to JSON)")
	}

	// Verify the content contains our expected text
	if !strings.Contains(content.Content, "Result item 1") {
		t.Errorf("expected content to contain 'Result item 1', got %q", content.Content)
	}
}

func TestParseJSONL_MixedContentFormats(t *testing.T) {
	// Mix of string and array content formats in same session
	jsonl := `{"type":"user","timestamp":"2025-12-23T10:30:00+11:00","message":{"role":"user","content":"User message as string"}}
{"type":"assistant","timestamp":"2025-12-23T10:30:05+11:00","message":{"role":"assistant","content":[{"type":"text","text":"Assistant message as array"}]}}
{"type":"user","timestamp":"2025-12-23T10:30:10+11:00","message":{"role":"user","content":"Another string message"}}`

	result, err := ParseJSONL(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings, got %d: %v", len(result.Warnings), result.Warnings)
	}

	if len(result.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(result.Entries))
	}

	// Verify each entry was parsed correctly
	tests := map[string]struct {
		entryType   string
		expectedTxt string
	}{
		"entry 0": {"user", "User message as string"},
		"entry 1": {"assistant", "Assistant message as array"},
		"entry 2": {"user", "Another string message"},
	}

	for name, tc := range tests {
		var idx int
		switch name {
		case "entry 0":
			idx = 0
		case "entry 1":
			idx = 1
		case "entry 2":
			idx = 2
		}

		t.Run(name, func(t *testing.T) {
			entry := result.Entries[idx]
			if entry.Type != tc.entryType {
				t.Errorf("expected type %q, got %q", tc.entryType, entry.Type)
			}

			if len(entry.Message.Content) != 1 {
				t.Fatalf("expected 1 content item, got %d", len(entry.Message.Content))
			}

			if entry.Message.Content[0].Text != tc.expectedTxt {
				t.Errorf("expected text %q, got %q", tc.expectedTxt, entry.Message.Content[0].Text)
			}
		})
	}
}

func TestParseJSONL_IDField(t *testing.T) {
	// tool_use content blocks include an "id" field
	jsonl := `{"type":"assistant","timestamp":"2025-12-23T10:30:00+11:00","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_123abc","name":"Read","input":{"file_path":"/test.txt"}}]}}`

	result, err := ParseJSONL(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings, got %d: %v", len(result.Warnings), result.Warnings)
	}

	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.Entries))
	}

	entry := result.Entries[0]
	if len(entry.Message.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(entry.Message.Content))
	}

	content := entry.Message.Content[0]
	if content.Type != "tool_use" {
		t.Errorf("expected content type 'tool_use', got %q", content.Type)
	}

	if content.ID != "toolu_123abc" {
		t.Errorf("expected ID 'toolu_123abc', got %q", content.ID)
	}

	if content.Name != "Read" {
		t.Errorf("expected Name 'Read', got %q", content.Name)
	}
}

func TestParseJSONL_ToolUseIDField(t *testing.T) {
	// tool_result content blocks include a "tool_use_id" field
	jsonl := `{"type":"user","timestamp":"2025-12-23T10:30:00+11:00","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_123abc","content":"file contents here","is_error":false}]}}`

	result, err := ParseJSONL(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings, got %d: %v", len(result.Warnings), result.Warnings)
	}

	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.Entries))
	}

	entry := result.Entries[0]
	if len(entry.Message.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(entry.Message.Content))
	}

	content := entry.Message.Content[0]
	if content.Type != "tool_result" {
		t.Errorf("expected content type 'tool_result', got %q", content.Type)
	}

	if content.ToolUseID != "toolu_123abc" {
		t.Errorf("expected ToolUseID 'toolu_123abc', got %q", content.ToolUseID)
	}

	if content.Content != "file contents here" {
		t.Errorf("expected Content 'file contents here', got %q", content.Content)
	}
}

func TestParseJSONL_MissingIDFields(t *testing.T) {
	// Old JSONL without id/tool_use_id fields should parse correctly (backward compatibility)
	jsonl := `{"type":"assistant","timestamp":"2025-12-23T10:30:00+11:00","message":{"role":"assistant","content":[{"type":"tool_use","name":"Read","input":{"file_path":"/test.txt"}}]}}
{"type":"user","timestamp":"2025-12-23T10:30:01+11:00","message":{"role":"user","content":[{"type":"tool_result","content":"file contents here","is_error":false}]}}`

	result, err := ParseJSONL(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings, got %d: %v", len(result.Warnings), result.Warnings)
	}

	if len(result.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result.Entries))
	}

	// First entry: tool_use without ID
	toolUse := result.Entries[0].Message.Content[0]
	if toolUse.Type != "tool_use" {
		t.Errorf("expected content type 'tool_use', got %q", toolUse.Type)
	}
	if toolUse.ID != "" {
		t.Errorf("expected empty ID, got %q", toolUse.ID)
	}
	if toolUse.Name != "Read" {
		t.Errorf("expected Name 'Read', got %q", toolUse.Name)
	}

	// Second entry: tool_result without ToolUseID
	toolResult := result.Entries[1].Message.Content[0]
	if toolResult.Type != "tool_result" {
		t.Errorf("expected content type 'tool_result', got %q", toolResult.Type)
	}
	if toolResult.ToolUseID != "" {
		t.Errorf("expected empty ToolUseID, got %q", toolResult.ToolUseID)
	}
	if toolResult.Content != "file contents here" {
		t.Errorf("expected Content 'file contents here', got %q", toolResult.Content)
	}
}
