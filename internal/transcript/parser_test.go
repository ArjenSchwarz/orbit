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

func TestParseJSONL_FiltersMetaEntries(t *testing.T) {
	// Meta entries (isMeta: true) should be filtered out
	jsonl := `{"type":"user","isMeta":true,"uuid":"meta-1","timestamp":"2025-12-23T10:30:00+11:00","message":{"role":"user","content":"Caveat: The messages below were generated by the user while running local commands."}}
{"type":"user","uuid":"real-1","parentUuid":"other","timestamp":"2025-12-23T10:30:01+11:00","message":{"role":"user","content":"Hello, this is a real message"}}`

	result, err := ParseJSONL(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings, got %d: %v", len(result.Warnings), result.Warnings)
	}

	// Should only have 1 entry (the non-meta one)
	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry (meta filtered), got %d", len(result.Entries))
	}

	if result.Entries[0].Message.Content[0].Text != "Hello, this is a real message" {
		t.Errorf("expected real message, got %q", result.Entries[0].Message.Content[0].Text)
	}
}

func TestParseJSONL_FiltersCommandNameEntriesAfterMeta(t *testing.T) {
	// <command-name> entries should only be filtered when they follow a meta entry
	jsonl := `{"type":"user","isMeta":true,"uuid":"meta-1","timestamp":"2025-12-23T10:30:00+11:00","message":{"role":"user","content":"Caveat: The messages below were generated by the user while running local commands."}}
{"type":"user","uuid":"cmd-1","parentUuid":"meta-1","timestamp":"2025-12-23T10:30:00+11:00","message":{"role":"user","content":"<command-name>/clear</command-name>\n            <command-message>clear</command-message>\n            <command-args></command-args>"}}
{"type":"user","uuid":"real-1","parentUuid":"other","timestamp":"2025-12-23T10:30:01+11:00","message":{"role":"user","content":"Hello, this is a real message"}}`

	result, err := ParseJSONL(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	// Should only have 1 entry (meta and command filtered)
	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry (command filtered), got %d", len(result.Entries))
	}

	if result.Entries[0].Message.Content[0].Text != "Hello, this is a real message" {
		t.Errorf("expected real message, got %q", result.Entries[0].Message.Content[0].Text)
	}
}

func TestParseJSONL_KeepsCommandNameEntriesWithoutMeta(t *testing.T) {
	// <command-name> entries that don't follow a meta entry should NOT be filtered
	jsonl := `{"type":"user","uuid":"cmd-1","parentUuid":"other","timestamp":"2025-12-23T10:30:00+11:00","message":{"role":"user","content":"<command-name>/some-command</command-name>"}}
{"type":"user","uuid":"real-1","parentUuid":"cmd-1","timestamp":"2025-12-23T10:30:01+11:00","message":{"role":"user","content":"Hello, this is a real message"}}`

	result, err := ParseJSONL(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	// Should have both entries (command-name NOT filtered because no meta parent)
	if len(result.Entries) != 2 {
		t.Fatalf("expected 2 entries (command not filtered), got %d", len(result.Entries))
	}

	if !strings.Contains(result.Entries[0].Message.Content[0].Text, "<command-name>") {
		t.Errorf("expected command entry to be kept, got %q", result.Entries[0].Message.Content[0].Text)
	}
}

func TestParseJSONL_FiltersLocalCommandStdoutAfterCommand(t *testing.T) {
	// <local-command-stdout> entries should be filtered when they follow a filtered command entry
	jsonl := `{"type":"user","isMeta":true,"uuid":"meta-1","timestamp":"2025-12-23T10:30:00+11:00","message":{"role":"user","content":"Caveat: The messages below were generated by the user while running local commands."}}
{"type":"user","uuid":"cmd-1","parentUuid":"meta-1","timestamp":"2025-12-23T10:30:00+11:00","message":{"role":"user","content":"<command-name>/plan</command-name>"}}
{"type":"user","uuid":"stdout-1","parentUuid":"cmd-1","timestamp":"2025-12-23T10:30:00+11:00","message":{"role":"user","content":"<local-command-stdout>No plan found</local-command-stdout>"}}
{"type":"user","uuid":"real-1","parentUuid":"stdout-1","timestamp":"2025-12-23T10:30:01+11:00","message":{"role":"user","content":"Hello, this is a real message"}}`

	result, err := ParseJSONL(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	// Should only have 1 entry (meta, command, and stdout filtered)
	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry (local command filtered), got %d", len(result.Entries))
	}

	if result.Entries[0].Message.Content[0].Text != "Hello, this is a real message" {
		t.Errorf("expected real message, got %q", result.Entries[0].Message.Content[0].Text)
	}
}

func TestParseJSONL_KeepsLocalCommandStdoutWithoutFilteredParent(t *testing.T) {
	// <local-command-stdout> entries that don't follow a filtered entry should NOT be filtered
	jsonl := `{"type":"user","uuid":"stdout-1","parentUuid":"other","timestamp":"2025-12-23T10:30:00+11:00","message":{"role":"user","content":"<local-command-stdout>Some output</local-command-stdout>"}}
{"type":"user","uuid":"real-1","parentUuid":"stdout-1","timestamp":"2025-12-23T10:30:01+11:00","message":{"role":"user","content":"Hello, this is a real message"}}`

	result, err := ParseJSONL(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	// Should have both entries (stdout NOT filtered because no filtered parent)
	if len(result.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result.Entries))
	}
}

func TestParseJSONL_FiltersCompleteLocalCommandSequence(t *testing.T) {
	// Simulate a real session start with complete local command sequence
	jsonl := `{"type":"user","isMeta":true,"uuid":"meta-1","timestamp":"2025-12-23T10:30:00+11:00","message":{"role":"user","content":"Caveat: The messages below were generated by the user while running local commands."}}
{"type":"user","uuid":"cmd-1","parentUuid":"meta-1","timestamp":"2025-12-23T10:30:00+11:00","message":{"role":"user","content":"<command-name>/clear</command-name>\n            <command-message>clear</command-message>\n            <command-args></command-args>"}}
{"type":"user","uuid":"stdout-1","parentUuid":"cmd-1","timestamp":"2025-12-23T10:30:00+11:00","message":{"role":"user","content":"<local-command-stdout></local-command-stdout>"}}
{"type":"user","isMeta":true,"uuid":"meta-2","timestamp":"2025-12-23T10:30:00+11:00","message":{"role":"user","content":"Caveat: The messages below were generated by the user while running local commands."}}
{"type":"user","uuid":"cmd-2","parentUuid":"meta-2","timestamp":"2025-12-23T10:30:00+11:00","message":{"role":"user","content":"<command-name>/plan</command-name>"}}
{"type":"user","uuid":"stdout-2","parentUuid":"cmd-2","timestamp":"2025-12-23T10:30:00+11:00","message":{"role":"user","content":"<local-command-stdout>No plan found for current session</local-command-stdout>"}}
{"type":"user","uuid":"real-1","parentUuid":"stdout-2","timestamp":"2025-12-23T10:30:01+11:00","message":{"role":"user","content":"This is the actual user message"}}`

	result, err := ParseJSONL(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	// Should only have 1 entry (all local command entries filtered)
	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry (all local commands filtered), got %d", len(result.Entries))
	}

	if result.Entries[0].Message.Content[0].Text != "This is the actual user message" {
		t.Errorf("expected actual user message, got %q", result.Entries[0].Message.Content[0].Text)
	}
}

func TestShouldFilterLocalCommand(t *testing.T) {
	tests := map[string]struct {
		entry        Entry
		filteredUUIDs map[string]bool
		expected     bool
		expectUUIDAdded string // UUID expected to be added to filteredUUIDs
	}{
		"meta entry": {
			entry:        Entry{Type: "user", IsMeta: true, UUID: "meta-1"},
			filteredUUIDs: make(map[string]bool),
			expected:     true,
			expectUUIDAdded: "meta-1",
		},
		"normal entry": {
			entry: Entry{
				Type: "user",
				UUID: "normal-1",
				Message: &Message{
					Content: []ContentItem{{Type: "text", Text: "Hello world"}},
				},
			},
			filteredUUIDs: make(map[string]bool),
			expected:     false,
		},
		"command-name with filtered parent": {
			entry: Entry{
				Type:       "user",
				UUID:       "cmd-1",
				ParentUUID: "meta-1",
				Message: &Message{
					Content: []ContentItem{{Type: "text", Text: "<command-name>/clear</command-name>"}},
				},
			},
			filteredUUIDs: map[string]bool{"meta-1": true},
			expected:     true,
			expectUUIDAdded: "cmd-1",
		},
		"command-name without filtered parent": {
			entry: Entry{
				Type:       "user",
				UUID:       "cmd-1",
				ParentUUID: "other",
				Message: &Message{
					Content: []ContentItem{{Type: "text", Text: "<command-name>/clear</command-name>"}},
				},
			},
			filteredUUIDs: make(map[string]bool),
			expected:     false,
		},
		"local-command-stdout with filtered parent": {
			entry: Entry{
				Type:       "user",
				UUID:       "stdout-1",
				ParentUUID: "cmd-1",
				Message: &Message{
					Content: []ContentItem{{Type: "text", Text: "<local-command-stdout>output</local-command-stdout>"}},
				},
			},
			filteredUUIDs: map[string]bool{"cmd-1": true},
			expected:     true,
		},
		"local-command-stdout without filtered parent": {
			entry: Entry{
				Type:       "user",
				UUID:       "stdout-1",
				ParentUUID: "other",
				Message: &Message{
					Content: []ContentItem{{Type: "text", Text: "<local-command-stdout>output</local-command-stdout>"}},
				},
			},
			filteredUUIDs: make(map[string]bool),
			expected:     false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := shouldFilterLocalCommand(&tc.entry, tc.filteredUUIDs)
			if got != tc.expected {
				t.Errorf("shouldFilterLocalCommand() = %v, want %v", got, tc.expected)
			}
			if tc.expectUUIDAdded != "" {
				if !tc.filteredUUIDs[tc.expectUUIDAdded] {
					t.Errorf("expected UUID %q to be added to filteredUUIDs", tc.expectUUIDAdded)
				}
			}
		})
	}
}

func TestHasCommandNameContent(t *testing.T) {
	tests := map[string]struct {
		entry    Entry
		expected bool
	}{
		"nil message": {
			entry:    Entry{Type: "user"},
			expected: false,
		},
		"empty content": {
			entry:    Entry{Type: "user", Message: &Message{Content: []ContentItem{}}},
			expected: false,
		},
		"normal text": {
			entry: Entry{
				Type:    "user",
				Message: &Message{Content: []ContentItem{{Type: "text", Text: "Hello world"}}},
			},
			expected: false,
		},
		"command-name tag": {
			entry: Entry{
				Type:    "user",
				Message: &Message{Content: []ContentItem{{Type: "text", Text: "<command-name>/clear</command-name>"}}},
			},
			expected: true,
		},
		"tool_result type ignored": {
			entry: Entry{
				Type:    "user",
				Message: &Message{Content: []ContentItem{{Type: "tool_result", Content: "<command-name>test</command-name>"}}},
			},
			expected: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := hasCommandNameContent(&tc.entry)
			if got != tc.expected {
				t.Errorf("hasCommandNameContent() = %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestHasLocalCommandStdoutContent(t *testing.T) {
	tests := map[string]struct {
		entry    Entry
		expected bool
	}{
		"nil message": {
			entry:    Entry{Type: "user"},
			expected: false,
		},
		"normal text": {
			entry: Entry{
				Type:    "user",
				Message: &Message{Content: []ContentItem{{Type: "text", Text: "Hello world"}}},
			},
			expected: false,
		},
		"local-command-stdout tag": {
			entry: Entry{
				Type:    "user",
				Message: &Message{Content: []ContentItem{{Type: "text", Text: "<local-command-stdout>output</local-command-stdout>"}}},
			},
			expected: true,
		},
		"empty local-command-stdout": {
			entry: Entry{
				Type:    "user",
				Message: &Message{Content: []ContentItem{{Type: "text", Text: "<local-command-stdout></local-command-stdout>"}}},
			},
			expected: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := hasLocalCommandStdoutContent(&tc.entry)
			if got != tc.expected {
				t.Errorf("hasLocalCommandStdoutContent() = %v, want %v", got, tc.expected)
			}
		})
	}
}
