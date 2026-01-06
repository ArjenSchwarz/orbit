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

func TestParseJSONL_EmptyFileFromFile(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "empty.jsonl"))
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer func() { _ = f.Close() }()

	// Empty files now return an error during format detection
	_, err = ParseJSONL(f)
	if err == nil {
		t.Fatal("expected error for empty file")
	}

	if err.Error() != "empty file" {
		t.Errorf("expected error 'empty file', got %q", err.Error())
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

func TestParseJSONL_KeepsSkillDescriptionMetaEntries(t *testing.T) {
	// Meta entries with sourceToolUseID are skill descriptions and should be KEPT
	jsonl := `{"type":"user","isMeta":true,"uuid":"meta-caveat","timestamp":"2025-12-23T10:30:00+11:00","message":{"role":"user","content":[{"type":"text","text":"Caveat: warning message"}]}}
{"type":"user","isMeta":true,"uuid":"meta-skill","sourceToolUseID":"toolu_123","timestamp":"2025-12-23T10:30:01+11:00","message":{"role":"user","content":[{"type":"text","text":"This skill does something useful."}]}}
{"type":"user","uuid":"real-1","timestamp":"2025-12-23T10:30:02+11:00","message":{"role":"user","content":[{"type":"text","text":"Hello"}]}}`

	result, err := ParseJSONL(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	// Should have 2 entries: the skill description meta entry + the real message
	// (the caveat meta entry should be filtered)
	if len(result.Entries) != 2 {
		t.Fatalf("expected 2 entries (skill description + real), got %d", len(result.Entries))
	}

	// First entry should be the skill description
	if !result.Entries[0].IsMeta {
		t.Error("expected first entry to be meta (skill description)")
	}
	if result.Entries[0].SourceToolUseID != "toolu_123" {
		t.Errorf("expected sourceToolUseID 'toolu_123', got %q", result.Entries[0].SourceToolUseID)
	}

	// Second entry should be the real message
	if result.Entries[1].IsMeta {
		t.Error("expected second entry to not be meta")
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
		entry           Entry
		filteredUUIDs   map[string]bool
		expected        bool
		expectUUIDAdded string // UUID expected to be added to filteredUUIDs
	}{
		"meta entry": {
			entry:           Entry{Type: "user", IsMeta: true, UUID: "meta-1"},
			filteredUUIDs:   make(map[string]bool),
			expected:        true,
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
			expected:      false,
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
			filteredUUIDs:   map[string]bool{"meta-1": true},
			expected:        true,
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
			expected:      false,
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
			expected:      true,
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
			expected:      false,
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

// Tests for format detection (Codex support)

func TestDetectFormat_ClaudeUserType(t *testing.T) {
	input := `{"type":"user","timestamp":"2025-12-23T10:30:00+11:00","message":{"role":"user","content":"Hello"}}`
	format, firstLine, err := DetectFormat(strings.NewReader(input))
	if err != nil {
		t.Fatalf("DetectFormat returned error: %v", err)
	}
	if format != FormatClaude {
		t.Errorf("expected FormatClaude, got %v", format)
	}
	if len(firstLine) == 0 {
		t.Error("expected non-empty first line")
	}
}

func TestDetectFormat_ClaudeAssistantType(t *testing.T) {
	input := `{"type":"assistant","timestamp":"2025-12-23T10:30:00+11:00","message":{"role":"assistant","content":[{"type":"text","text":"Hi"}]}}`
	format, _, err := DetectFormat(strings.NewReader(input))
	if err != nil {
		t.Fatalf("DetectFormat returned error: %v", err)
	}
	if format != FormatClaude {
		t.Errorf("expected FormatClaude, got %v", format)
	}
}

func TestDetectFormat_CodexSessionMeta(t *testing.T) {
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"019b892c-3a14-7773-bd76-6465a8a0b634"}}`
	format, _, err := DetectFormat(strings.NewReader(input))
	if err != nil {
		t.Fatalf("DetectFormat returned error: %v", err)
	}
	if format != FormatCodex {
		t.Errorf("expected FormatCodex, got %v", format)
	}
}

func TestDetectFormat_CodexResponseItem(t *testing.T) {
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"response_item","payload":{"type":"message","role":"user","content":[]}}`
	format, _, err := DetectFormat(strings.NewReader(input))
	if err != nil {
		t.Fatalf("DetectFormat returned error: %v", err)
	}
	if format != FormatCodex {
		t.Errorf("expected FormatCodex, got %v", format)
	}
}

func TestDetectFormat_CodexEventMsg(t *testing.T) {
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"event_msg","payload":{"type":"agent_reasoning","text":"thinking"}}`
	format, _, err := DetectFormat(strings.NewReader(input))
	if err != nil {
		t.Fatalf("DetectFormat returned error: %v", err)
	}
	if format != FormatCodex {
		t.Errorf("expected FormatCodex, got %v", format)
	}
}

func TestDetectFormat_CodexTurnContext(t *testing.T) {
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"turn_context","payload":{}}`
	format, _, err := DetectFormat(strings.NewReader(input))
	if err != nil {
		t.Fatalf("DetectFormat returned error: %v", err)
	}
	if format != FormatCodex {
		t.Errorf("expected FormatCodex, got %v", format)
	}
}

func TestDetectFormat_EmptyFile(t *testing.T) {
	input := ""
	_, _, err := DetectFormat(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for empty file")
	}
	if err.Error() != "empty file" {
		t.Errorf("expected error 'empty file', got %q", err.Error())
	}
}

func TestDetectFormat_WhitespaceOnly(t *testing.T) {
	input := "   \n\n   \n"
	_, _, err := DetectFormat(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for whitespace-only file")
	}
	if err.Error() != "empty file" {
		t.Errorf("expected error 'empty file', got %q", err.Error())
	}
}

func TestDetectFormat_InvalidJSON(t *testing.T) {
	input := `{not valid json}`
	_, _, err := DetectFormat(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "failed to parse first line as JSON") {
		t.Errorf("expected error containing 'failed to parse first line as JSON', got %q", err.Error())
	}
}

func TestDetectFormat_UnrecognizedType(t *testing.T) {
	input := `{"type":"unknown_type","timestamp":"2025-12-23T10:30:00+11:00"}`
	_, _, err := DetectFormat(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for unrecognized type")
	}
	if !strings.Contains(err.Error(), "unrecognized log format: type field value 'unknown_type'") {
		t.Errorf("expected error containing 'unrecognized log format: type field value', got %q", err.Error())
	}
}

func TestDetectFormat_MissingTypeField(t *testing.T) {
	input := `{"timestamp":"2025-12-23T10:30:00+11:00","message":"hello"}`
	_, _, err := DetectFormat(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for missing type field")
	}
	if !strings.Contains(err.Error(), "unrecognized log format: type field value ''") {
		t.Errorf("expected error about empty type field, got %q", err.Error())
	}
}

func TestDetectFormat_BOMHandling(t *testing.T) {
	// UTF-8 BOM: EF BB BF
	bom := []byte{0xEF, 0xBB, 0xBF}
	json := `{"type":"user","timestamp":"2025-12-23T10:30:00+11:00","message":{"role":"user","content":"Hello"}}`
	input := append(bom, []byte(json)...)

	format, firstLine, err := DetectFormat(strings.NewReader(string(input)))
	if err != nil {
		t.Fatalf("DetectFormat returned error: %v", err)
	}
	if format != FormatClaude {
		t.Errorf("expected FormatClaude, got %v", format)
	}
	// First line should have BOM stripped
	if strings.HasPrefix(string(firstLine), "\xEF\xBB\xBF") {
		t.Error("BOM was not stripped from first line")
	}
}

func TestDetectFormat_SkipsEmptyLines(t *testing.T) {
	input := "\n\n   \n{\"type\":\"user\",\"timestamp\":\"2025-12-23T10:30:00+11:00\"}\n"
	format, _, err := DetectFormat(strings.NewReader(input))
	if err != nil {
		t.Fatalf("DetectFormat returned error: %v", err)
	}
	if format != FormatClaude {
		t.Errorf("expected FormatClaude, got %v", format)
	}
}

func TestReadFirstNonEmptyLine_ReturnsFirstLine(t *testing.T) {
	input := "first line\nsecond line\n"
	line, err := readFirstNonEmptyLine(strings.NewReader(input))
	if err != nil {
		t.Fatalf("readFirstNonEmptyLine returned error: %v", err)
	}
	if string(line) != "first line" {
		t.Errorf("expected 'first line', got %q", string(line))
	}
}

func TestReadFirstNonEmptyLine_SkipsEmptyLines(t *testing.T) {
	input := "\n\n   \nactual content\n"
	line, err := readFirstNonEmptyLine(strings.NewReader(input))
	if err != nil {
		t.Fatalf("readFirstNonEmptyLine returned error: %v", err)
	}
	if string(line) != "actual content" {
		t.Errorf("expected 'actual content', got %q", string(line))
	}
}

func TestReadFirstNonEmptyLine_EmptyFile(t *testing.T) {
	input := ""
	_, err := readFirstNonEmptyLine(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for empty file")
	}
}

func TestReadFirstNonEmptyLine_WhitespaceOnly(t *testing.T) {
	input := "   \n   \n   "
	_, err := readFirstNonEmptyLine(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for whitespace-only file")
	}
}

// Tests for ParseJSONL format dispatch (Phase 3)

func TestParseJSONL_AutoDetectsClaudeFormat(t *testing.T) {
	// Claude format starts with user or assistant type
	input := `{"type":"user","timestamp":"2025-12-23T10:30:00+11:00","message":{"role":"user","content":"Hello"}}
{"type":"assistant","timestamp":"2025-12-23T10:30:05+11:00","message":{"role":"assistant","content":[{"type":"text","text":"Hi there!"}]}}`

	result, err := ParseJSONL(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	if len(result.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result.Entries))
	}

	// Verify entries were parsed correctly
	if result.Entries[0].Type != "user" {
		t.Errorf("expected first entry type 'user', got %q", result.Entries[0].Type)
	}
	if result.Entries[1].Type != "assistant" {
		t.Errorf("expected second entry type 'assistant', got %q", result.Entries[1].Type)
	}
}

func TestParseJSONL_AutoDetectsCodexFormat(t *testing.T) {
	// Codex format starts with session_meta
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test-session-id","cwd":"/test"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Hello from Codex"}]}}
{"timestamp":"2026-01-04T13:22:17.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hi from assistant"}]}}`

	result, err := ParseJSONL(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	if len(result.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result.Entries))
	}

	// Verify entries were parsed and normalized correctly
	if result.Entries[0].Type != "user" {
		t.Errorf("expected first entry type 'user', got %q", result.Entries[0].Type)
	}
	if result.Entries[1].Type != "assistant" {
		t.Errorf("expected second entry type 'assistant', got %q", result.Entries[1].Type)
	}

	// Verify session ID was extracted
	if result.Entries[0].SessionID != "test-session-id" {
		t.Errorf("expected session ID 'test-session-id', got %q", result.Entries[0].SessionID)
	}
}

func TestParseJSONL_CodexResponseItemFirst(t *testing.T) {
	// Codex format can also start with response_item
	input := `{"timestamp":"2026-01-04T13:22:16.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Hello from Codex"}]}}`

	result, err := ParseJSONL(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.Entries))
	}

	if result.Entries[0].Type != "user" {
		t.Errorf("expected entry type 'user', got %q", result.Entries[0].Type)
	}
}

func TestParseJSONL_CodexEventMsgFirst(t *testing.T) {
	// Codex format can start with event_msg
	input := `{"timestamp":"2026-01-04T13:22:21.499Z","type":"event_msg","payload":{"type":"agent_reasoning","text":"Thinking about the problem..."}}
{"timestamp":"2026-01-04T13:22:22.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Here's my response"}]}}`

	result, err := ParseJSONL(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.Entries))
	}

	// Agent reasoning and output should be consolidated into one assistant entry
	if result.Entries[0].Type != "assistant" {
		t.Errorf("expected entry type 'assistant', got %q", result.Entries[0].Type)
	}
}

func TestParseJSONL_CodexTurnContextFirst(t *testing.T) {
	// Codex format can start with turn_context (which is skipped)
	input := `{"timestamp":"2026-01-04T13:22:15.000Z","type":"turn_context","payload":{"context":"internal"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Hello"}]}}`

	result, err := ParseJSONL(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.Entries))
	}

	if result.Entries[0].Type != "user" {
		t.Errorf("expected entry type 'user', got %q", result.Entries[0].Type)
	}
}

func TestParseJSONL_EmptyFileError(t *testing.T) {
	input := ""
	_, err := ParseJSONL(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for empty file")
	}
	if err.Error() != "empty file" {
		t.Errorf("expected error 'empty file', got %q", err.Error())
	}
}

func TestParseJSONL_WhitespaceOnlyError(t *testing.T) {
	input := "   \n\n   \n"
	_, err := ParseJSONL(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for whitespace-only file")
	}
	if err.Error() != "empty file" {
		t.Errorf("expected error 'empty file', got %q", err.Error())
	}
}

func TestParseJSONL_InvalidJSONError(t *testing.T) {
	input := `{not valid json}`
	_, err := ParseJSONL(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "failed to parse first line as JSON") {
		t.Errorf("expected error containing 'failed to parse first line as JSON', got %q", err.Error())
	}
}

func TestParseJSONL_UnrecognizedFormatError(t *testing.T) {
	input := `{"type":"unknown_format_type","data":"test"}`
	_, err := ParseJSONL(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for unrecognized format")
	}
	if !strings.Contains(err.Error(), "unrecognized log format") {
		t.Errorf("expected error containing 'unrecognized log format', got %q", err.Error())
	}
}

func TestParseJSONL_BOMHandledCorrectly(t *testing.T) {
	// UTF-8 BOM: EF BB BF
	bom := []byte{0xEF, 0xBB, 0xBF}
	json := `{"type":"user","timestamp":"2025-12-23T10:30:00+11:00","message":{"role":"user","content":"Hello"}}`
	input := append(bom, []byte(json)...)

	result, err := ParseJSONL(strings.NewReader(string(input)))
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.Entries))
	}
}

func TestParseJSONL_SkipsEmptyLinesBeforeDetection(t *testing.T) {
	input := "\n\n   \n{\"type\":\"user\",\"timestamp\":\"2025-12-23T10:30:00+11:00\",\"message\":{\"role\":\"user\",\"content\":\"Hello\"}}\n"

	result, err := ParseJSONL(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.Entries))
	}
}

func TestParseJSONL_CodexErrorPropagation(t *testing.T) {
	// Codex format with all lines invalid (except first which determines format)
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test","cwd":"/test"}}
invalid json line 1
invalid json line 2`

	_, err := ParseJSONL(strings.NewReader(input))
	// Should fail because no valid entries found (session_meta is skipped)
	if err == nil {
		t.Fatal("expected error when no valid entries found")
	}
	if !strings.Contains(err.Error(), "no valid entries found") {
		t.Errorf("expected error containing 'no valid entries found', got %q", err.Error())
	}
}

func TestParseJSONL_CodexValidFile(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "codex_valid.jsonl"))
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer func() { _ = f.Close() }()

	result, err := ParseJSONL(f)
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	// Should have user and assistant entries (metadata events filtered)
	if len(result.Entries) == 0 {
		t.Error("expected at least one entry")
	}

	// Verify entry types
	for _, entry := range result.Entries {
		if entry.Type != "user" && entry.Type != "assistant" {
			t.Errorf("unexpected entry type: %s", entry.Type)
		}
	}
}

func TestParseJSONL_CodexEdgeCases(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "codex_edge_cases.jsonl"))
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer func() { _ = f.Close() }()

	result, err := ParseJSONL(f)
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	// Should have warnings for malformed lines and unknown types
	if len(result.Warnings) == 0 {
		t.Error("expected warnings for edge cases")
	}

	// Should still parse valid entries
	if len(result.Entries) == 0 {
		t.Error("expected some valid entries despite edge cases")
	}
}

// Integration tests for Codex to Markdown/HTML rendering (Phase 3 - Task 13)

func TestCodexToMarkdown_FullPipeline(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "codex_valid.jsonl"))
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer func() { _ = f.Close() }()

	// Parse JSONL
	result, err := ParseJSONL(f)
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	// Render to Markdown
	opts := RenderOptions{
		Title:     "Codex Session",
		SessionID: "test-session",
	}
	md := RenderMarkdown(result.Entries, opts)

	// Verify basic structure
	if !strings.Contains(md, "# Codex Session") {
		t.Error("expected title in markdown output")
	}
	if !strings.Contains(md, "**Session ID:** `test-session`") {
		t.Error("expected session ID in markdown output")
	}

	// Verify user message is present
	if !strings.Contains(md, "## 👤 User") {
		t.Error("expected user message header in markdown output")
	}

	// Verify assistant message is present
	if !strings.Contains(md, "## 🤖 Assistant") {
		t.Error("expected assistant message header in markdown output")
	}

	// Verify tool call is present (shell_command)
	if !strings.Contains(md, "shell_command") {
		t.Error("expected shell_command tool in markdown output")
	}

	// Verify thinking block is present (from reasoning)
	if !strings.Contains(md, "💭 Thinking") {
		t.Error("expected thinking block in markdown output")
	}
}

func TestCodexToHTML_FullPipeline(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "codex_valid.jsonl"))
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer func() { _ = f.Close() }()

	// Parse JSONL
	result, err := ParseJSONL(f)
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	// Render to HTML
	opts := RenderOptions{
		Title:     "Codex Session",
		SessionID: "test-session",
	}
	html := RenderHTML(result.Entries, opts)

	// Verify HTML document structure
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("expected HTML doctype")
	}
	if !strings.Contains(html, "<html lang=\"en\">") {
		t.Error("expected html tag")
	}
	if !strings.Contains(html, "<title>Codex Session</title>") {
		t.Error("expected title tag")
	}

	// Verify user message is present
	if !strings.Contains(html, "class=\"message user\"") {
		t.Error("expected user message section in HTML output")
	}

	// Verify assistant message is present
	if !strings.Contains(html, "class=\"message assistant\"") {
		t.Error("expected assistant message section in HTML output")
	}

	// Verify tool call is present
	if !strings.Contains(html, "shell_command") {
		t.Error("expected shell_command tool in HTML output")
	}
}

func TestCodexToHTMLFragment_FullPipeline(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "codex_valid.jsonl"))
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer func() { _ = f.Close() }()

	// Parse JSONL
	result, err := ParseJSONL(f)
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	// Render to HTML fragment
	opts := RenderOptions{
		Title:     "Codex Session",
		SessionID: "test-session",
	}
	fragment := RenderHTMLFragment(result.Entries, opts)

	// Verify NO document structure
	if strings.Contains(fragment, "<!DOCTYPE html>") {
		t.Error("fragment should not contain HTML doctype")
	}
	if strings.Contains(fragment, "<html") {
		t.Error("fragment should not contain html tag")
	}

	// Verify content is present
	if !strings.Contains(fragment, "class=\"message user\"") {
		t.Error("expected user message section in HTML fragment")
	}
	if !strings.Contains(fragment, "class=\"message assistant\"") {
		t.Error("expected assistant message section in HTML fragment")
	}
}

func TestCodexToMarkdown_ToolCalls(t *testing.T) {
	// Test that Codex tool calls are properly rendered in markdown
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test-session","cwd":"/test"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"response_item","payload":{"type":"function_call","name":"shell_command","arguments":"{\"command\":\"ls -la\"}","call_id":"call_123"}}
{"timestamp":"2026-01-04T13:22:16.100Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_123","output":"file1.txt\nfile2.txt"}}`

	result, err := ParseJSONL(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	md := RenderMarkdown(result.Entries, RenderOptions{})

	// Verify tool call name is present
	if !strings.Contains(md, "shell_command") {
		t.Error("expected shell_command in markdown output")
	}

	// Verify tool result is present
	if !strings.Contains(md, "file1.txt") {
		t.Error("expected tool output in markdown")
	}
}

func TestCodexToMarkdown_Reasoning(t *testing.T) {
	// Test that Codex reasoning blocks are rendered as thinking
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test-session","cwd":"/test"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"response_item","payload":{"type":"reasoning","summary":[{"type":"summary_text","text":"Analyzing the problem..."}],"encrypted_content":"encrypted"}}`

	result, err := ParseJSONL(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	md := RenderMarkdown(result.Entries, RenderOptions{})

	// Verify thinking block is present
	if !strings.Contains(md, "💭 Thinking") {
		t.Error("expected thinking block in markdown output")
	}

	// Verify reasoning text is present
	if !strings.Contains(md, "Analyzing the problem") {
		t.Error("expected reasoning text in markdown output")
	}

	// Verify encrypted content is NOT present
	if strings.Contains(md, "encrypted_content") || strings.Contains(md, "encrypted_data") {
		t.Error("encrypted content should not appear in output")
	}
}

func TestCodexToMarkdown_AgentReasoning(t *testing.T) {
	// Test that agent_reasoning events are rendered as thinking
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test-session","cwd":"/test"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"event_msg","payload":{"type":"agent_reasoning","text":"**Planning the approach**"}}`

	result, err := ParseJSONL(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	md := RenderMarkdown(result.Entries, RenderOptions{})

	// Verify thinking block is present
	if !strings.Contains(md, "💭 Thinking") {
		t.Error("expected thinking block in markdown output")
	}

	// Verify agent reasoning text is present
	if !strings.Contains(md, "Planning the approach") {
		t.Error("expected agent reasoning text in markdown output")
	}
}

func TestCodexToMarkdown_AgentMessage(t *testing.T) {
	// Test that agent_message events are rendered as text
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test-session","cwd":"/test"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"event_msg","payload":{"type":"agent_message","message":"I found 2 files in the directory."}}`

	result, err := ParseJSONL(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	md := RenderMarkdown(result.Entries, RenderOptions{})

	// Verify message text is present
	if !strings.Contains(md, "I found 2 files") {
		t.Error("expected agent message text in markdown output")
	}
}

func TestCodexToMarkdown_MultipleToolOutputs(t *testing.T) {
	// Test that multiple outputs for the same function_call are rendered
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test-session","cwd":"/test"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"response_item","payload":{"type":"function_call","name":"shell_command","arguments":"{}","call_id":"call_multi"}}
{"timestamp":"2026-01-04T13:22:16.100Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_multi","output":"First chunk"}}
{"timestamp":"2026-01-04T13:22:16.200Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_multi","output":"Second chunk"}}
{"timestamp":"2026-01-04T13:22:16.300Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_multi","output":"Third chunk"}}`

	result, err := ParseJSONL(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	md := RenderMarkdown(result.Entries, RenderOptions{})

	// Verify all chunks are present
	if !strings.Contains(md, "First chunk") {
		t.Error("expected first chunk in markdown output")
	}
	if !strings.Contains(md, "Second chunk") {
		t.Error("expected second chunk in markdown output")
	}
	if !strings.Contains(md, "Third chunk") {
		t.Error("expected third chunk in markdown output")
	}
}

func TestCodexToHTML_ThinkingBlock(t *testing.T) {
	// Test that thinking blocks are properly styled in HTML
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test-session","cwd":"/test"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"event_msg","payload":{"type":"agent_reasoning","text":"Thinking about this..."}}`

	result, err := ParseJSONL(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	html := RenderHTML(result.Entries, RenderOptions{})

	// Verify thinking block class is present
	if !strings.Contains(html, "thinking-block") {
		t.Error("expected thinking-block class in HTML output")
	}

	// Verify thinking content is present
	if !strings.Contains(html, "Thinking about this") {
		t.Error("expected thinking content in HTML output")
	}
}

// --- Phase 5: Error Handling and Negative Tests ---

func TestParseJSONL_EmptyFileError_Negative(t *testing.T) {
	// Test empty file returns proper error message (req 1.6)
	input := ""
	_, err := ParseJSONL(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for empty file")
	}
	if err.Error() != "empty file" {
		t.Errorf("expected exact error message 'empty file', got %q", err.Error())
	}
}

func TestParseJSONL_WhitespaceOnlyError_Negative(t *testing.T) {
	// Test whitespace-only file returns "empty file" error (req 1.6)
	tests := []struct {
		name  string
		input string
	}{
		{"spaces only", "   "},
		{"newlines only", "\n\n\n"},
		{"mixed whitespace", "   \n\n   \n   "},
		{"tabs and spaces", "\t  \t\n  \t"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseJSONL(strings.NewReader(tt.input))
			if err == nil {
				t.Fatal("expected error for whitespace-only file")
			}
			if err.Error() != "empty file" {
				t.Errorf("expected exact error message 'empty file', got %q", err.Error())
			}
		})
	}
}

func TestParseJSONL_InvalidFirstLineJSONError_Negative(t *testing.T) {
	// Test invalid JSON on first line returns proper error (req 1.4)
	tests := []struct {
		name  string
		input string
	}{
		{"malformed braces", "{not valid json}"},
		{"missing quotes", `{type: user}`},
		{"truncated json", `{"type":"user"`},
		{"random text", "hello world"},
		{"html content", "<html><body></body></html>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseJSONL(strings.NewReader(tt.input))
			if err == nil {
				t.Fatal("expected error for invalid JSON")
			}
			if !strings.Contains(err.Error(), "failed to parse first line as JSON") {
				t.Errorf("expected error containing 'failed to parse first line as JSON', got %q", err.Error())
			}
		})
	}
}

func TestParseJSONL_UnknownFormatTypeError_Negative(t *testing.T) {
	// Test unknown type field value returns proper error (req 1.5)
	tests := []struct {
		name         string
		input        string
		expectedType string
	}{
		{"unknown_type", `{"type":"unknown_type"}`, "unknown_type"},
		{"empty_type", `{"type":""}`, ""},
		{"numeric_type", `{"type":"123"}`, "123"},
		{"system_type", `{"type":"system"}`, "system"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseJSONL(strings.NewReader(tt.input))
			if err == nil {
				t.Fatal("expected error for unrecognized format type")
			}
			expectedMsg := "unrecognized log format: type field value '" + tt.expectedType + "'"
			if !strings.Contains(err.Error(), expectedMsg) {
				t.Errorf("expected error containing %q, got %q", expectedMsg, err.Error())
			}
		})
	}
}

func TestParseJSONL_MissingTypeFieldError_Negative(t *testing.T) {
	// Test missing type field returns proper error (req 1.5)
	input := `{"timestamp":"2025-12-23T10:30:00+11:00","message":"hello"}`
	_, err := ParseJSONL(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for missing type field")
	}
	if !strings.Contains(err.Error(), "unrecognized log format: type field value ''") {
		t.Errorf("expected error about empty type field, got %q", err.Error())
	}
}

func TestParseJSONL_MalformedMiddleLineWarning_Negative(t *testing.T) {
	// Test malformed line in middle generates warning but continues parsing (req 9.1)
	input := `{"type":"user","timestamp":"2025-12-23T10:30:00+11:00","message":{"role":"user","content":"First"}}
{not valid json in the middle}
{"type":"assistant","timestamp":"2025-12-23T10:30:05+11:00","message":{"role":"assistant","content":[{"type":"text","text":"Second"}]}}`

	result, err := ParseJSONL(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseJSONL should not return error for malformed middle line: %v", err)
	}

	// Should have parsed both valid entries
	if len(result.Entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(result.Entries))
	}

	// Should have warning for malformed line
	if len(result.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(result.Warnings), result.Warnings)
	}

	// Warning should include line number (req 9.4)
	if result.Warnings[0].Line != 2 {
		t.Errorf("expected warning on line 2, got line %d", result.Warnings[0].Line)
	}

	// Warning message should describe the issue
	if !strings.Contains(result.Warnings[0].Message, "failed to parse JSON") {
		t.Errorf("expected warning about JSON parsing, got %q", result.Warnings[0].Message)
	}
}

func TestParseJSONL_CodexAllLinesMalformedError_Negative(t *testing.T) {
	// Test all lines malformed returns error (req 9.5)
	// First line must be valid Codex type for format detection
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test"}}
not valid json line 1
also not valid json line 2
still not valid json line 3`

	_, err := ParseJSONL(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error when no valid entries found")
	}
	if !strings.Contains(err.Error(), "no valid entries found") {
		t.Errorf("expected error containing 'no valid entries found', got %q", err.Error())
	}
}

func TestParseJSONL_TruncatedLastLineWarning_Negative(t *testing.T) {
	// Test truncated/incomplete last line is handled gracefully (req 9.6)
	// Note: This test simulates a truncated file by not closing a JSON object
	input := `{"type":"user","timestamp":"2025-12-23T10:30:00+11:00","message":{"role":"user","content":"Valid"}}
{"type":"assistant","timestamp":"2025-12-23T10:30:05+11:00","message":{"role":"assistant","content":[{"type":"text","text":"Trunca`

	result, err := ParseJSONL(strings.NewReader(input))
	// Should not fail, but should warn about truncated line
	if err != nil {
		t.Fatalf("ParseJSONL should handle truncated last line gracefully: %v", err)
	}

	// Should have parsed the valid entry
	if len(result.Entries) < 1 {
		t.Error("expected at least 1 valid entry")
	}

	// Should have warning for truncated line
	if len(result.Warnings) < 1 {
		t.Error("expected warning for truncated/malformed last line")
	}
}

func TestDetectFormat_EmptyFile_Negative(t *testing.T) {
	// Test DetectFormat on empty file
	_, _, err := DetectFormat(strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error for empty file")
	}
	if err.Error() != "empty file" {
		t.Errorf("expected exact error 'empty file', got %q", err.Error())
	}
}

func TestDetectFormat_WhitespaceOnly_Negative(t *testing.T) {
	// Test DetectFormat on whitespace-only file
	_, _, err := DetectFormat(strings.NewReader("   \n\n   \n"))
	if err == nil {
		t.Fatal("expected error for whitespace-only file")
	}
	if err.Error() != "empty file" {
		t.Errorf("expected exact error 'empty file', got %q", err.Error())
	}
}

func TestDetectFormat_InvalidJSON_Negative(t *testing.T) {
	// Test DetectFormat on invalid JSON
	_, _, err := DetectFormat(strings.NewReader("{invalid json}"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "failed to parse first line as JSON") {
		t.Errorf("expected error containing 'failed to parse first line as JSON', got %q", err.Error())
	}
}

func TestDetectFormat_UnknownType_Negative(t *testing.T) {
	// Test DetectFormat on unknown type value
	_, _, err := DetectFormat(strings.NewReader(`{"type":"completely_unknown"}`))
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
	if !strings.Contains(err.Error(), "unrecognized log format: type field value 'completely_unknown'") {
		t.Errorf("expected specific error about unknown type, got %q", err.Error())
	}
}
