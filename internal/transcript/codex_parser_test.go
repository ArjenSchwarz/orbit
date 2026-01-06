package transcript

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func TestParseCodexJSONL_ValidFile(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "codex_valid.jsonl"))
	require.NoError(t, err)

	result, err := ParseCodexJSONL(bytes.NewReader(data))
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should have multiple entries
	assert.Greater(t, len(result.Entries), 0, "should have parsed entries")

	// Check session ID is populated
	for _, entry := range result.Entries {
		assert.Equal(t, "019b892c-3a14-7773-bd76-6465a8a0b634", entry.SessionID)
	}
}

func TestParseCodexJSONL_MessageConversion(t *testing.T) {
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test-session"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Hello world"}]}}
{"timestamp":"2026-01-04T13:22:17.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hi there!"}]}}`

	result, err := ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Entries, 2)

	// Check user message
	userEntry := result.Entries[0]
	assert.Equal(t, "user", userEntry.Type)
	assert.Equal(t, "2026-01-04T13:22:16.000Z", userEntry.Timestamp)
	assert.Equal(t, "test-session", userEntry.SessionID)
	require.NotNil(t, userEntry.Message)
	require.Len(t, userEntry.Message.Content, 1)
	assert.Equal(t, "text", userEntry.Message.Content[0].Type)
	assert.Equal(t, "Hello world", userEntry.Message.Content[0].Text)

	// Check assistant message
	assistantEntry := result.Entries[1]
	assert.Equal(t, "assistant", assistantEntry.Type)
	assert.Equal(t, "2026-01-04T13:22:17.000Z", assistantEntry.Timestamp)
	require.NotNil(t, assistantEntry.Message)
	require.Len(t, assistantEntry.Message.Content, 1)
	assert.Equal(t, "text", assistantEntry.Message.Content[0].Type)
	assert.Equal(t, "Hi there!", assistantEntry.Message.Content[0].Text)
}

func TestParseCodexJSONL_FunctionCallLinking(t *testing.T) {
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test-session"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"response_item","payload":{"type":"function_call","name":"shell_command","arguments":"{\"command\":\"ls\"}","call_id":"call_123"}}
{"timestamp":"2026-01-04T13:22:17.000Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_123","output":"file1.txt\nfile2.txt"}}`

	result, err := ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Entries, 1)

	// Entry should have both tool_use and tool_result
	entry := result.Entries[0]
	assert.Equal(t, "assistant", entry.Type)
	require.NotNil(t, entry.Message)
	require.Len(t, entry.Message.Content, 2)

	// Check tool_use
	toolUse := entry.Message.Content[0]
	assert.Equal(t, "tool_use", toolUse.Type)
	assert.Equal(t, "call_123", toolUse.ID)
	assert.Equal(t, "shell_command", toolUse.Name)

	// Check tool_result
	toolResult := entry.Message.Content[1]
	assert.Equal(t, "tool_result", toolResult.Type)
	assert.Equal(t, "call_123", toolResult.ToolUseID)
	assert.Equal(t, "file1.txt\nfile2.txt", toolResult.Content)
}

func TestParseCodexJSONL_ReasoningExtraction(t *testing.T) {
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test-session"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"response_item","payload":{"type":"reasoning","summary":[{"type":"summary_text","text":"First thought"},{"type":"summary_text","text":"Second thought"}],"encrypted_content":"should_be_ignored"}}`

	result, err := ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Entries, 1)

	entry := result.Entries[0]
	assert.Equal(t, "assistant", entry.Type)
	require.NotNil(t, entry.Message)
	require.Len(t, entry.Message.Content, 1)

	// Check thinking block
	thinking := entry.Message.Content[0]
	assert.Equal(t, "thinking", thinking.Type)
	assert.Equal(t, "First thought\nSecond thought", thinking.Thinking)
}

func TestParseCodexJSONL_AgentReasoningExtraction(t *testing.T) {
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test-session"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"event_msg","payload":{"type":"agent_reasoning","text":"**Analyzing the request**"}}`

	result, err := ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Entries, 1)

	entry := result.Entries[0]
	assert.Equal(t, "assistant", entry.Type)
	require.NotNil(t, entry.Message)
	require.Len(t, entry.Message.Content, 1)

	thinking := entry.Message.Content[0]
	assert.Equal(t, "thinking", thinking.Type)
	assert.Equal(t, "**Analyzing the request**", thinking.Thinking)
}

func TestParseCodexJSONL_AgentMessageExtraction(t *testing.T) {
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test-session"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"event_msg","payload":{"type":"agent_message","message":"I found 2 files."}}`

	result, err := ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Entries, 1)

	entry := result.Entries[0]
	assert.Equal(t, "assistant", entry.Type)
	require.NotNil(t, entry.Message)
	require.Len(t, entry.Message.Content, 1)

	text := entry.Message.Content[0]
	assert.Equal(t, "text", text.Type)
	assert.Equal(t, "I found 2 files.", text.Text)
}

func TestParseCodexJSONL_EventFiltering(t *testing.T) {
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test-session"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"turn_context","payload":{"context":"internal"}}
{"timestamp":"2026-01-04T13:22:17.000Z","type":"event_msg","payload":{"type":"token_count","input_tokens":100}}
{"timestamp":"2026-01-04T13:22:18.000Z","type":"event_msg","payload":{"type":"user_message","message":"Hello"}}
{"timestamp":"2026-01-04T13:22:19.000Z","type":"response_item","payload":{"type":"ghost_snapshot","data":"git_hash"}}
{"timestamp":"2026-01-04T13:22:20.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Real message"}]}}`

	result, err := ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)

	// Only the real user message should be included
	require.Len(t, result.Entries, 1)
	assert.Equal(t, "user", result.Entries[0].Type)
	assert.Equal(t, "Real message", result.Entries[0].Message.Content[0].Text)
}

func TestParseCodexJSONL_OrphanedOutputs(t *testing.T) {
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test-session"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"response_item","payload":{"type":"function_call_output","call_id":"nonexistent_call","output":"orphaned output"}}`

	result, err := ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Entries, 1)

	// Orphaned output should still be rendered
	entry := result.Entries[0]
	assert.Equal(t, "assistant", entry.Type)
	require.NotNil(t, entry.Message)
	require.Len(t, entry.Message.Content, 1)

	toolResult := entry.Message.Content[0]
	assert.Equal(t, "tool_result", toolResult.Type)
	assert.Equal(t, "nonexistent_call", toolResult.ToolUseID)
	assert.Equal(t, "orphaned output", toolResult.Content)

	// Should have a warning about the orphaned output
	require.Len(t, result.Warnings, 1)
	assert.Contains(t, result.Warnings[0].Message, "no matching function_call for call_id")
}

func TestParseCodexJSONL_MultiOutputToolCalls(t *testing.T) {
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test-session"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"response_item","payload":{"type":"function_call","name":"streaming_tool","arguments":"{}","call_id":"call_multi"}}
{"timestamp":"2026-01-04T13:22:17.000Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_multi","output":"chunk 1"}}
{"timestamp":"2026-01-04T13:22:18.000Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_multi","output":"chunk 2"}}
{"timestamp":"2026-01-04T13:22:19.000Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_multi","output":"chunk 3"}}`

	result, err := ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Entries, 1)

	entry := result.Entries[0]
	require.NotNil(t, entry.Message)

	// Should have 1 tool_use + 3 tool_results
	require.Len(t, entry.Message.Content, 4)

	assert.Equal(t, "tool_use", entry.Message.Content[0].Type)
	assert.Equal(t, "tool_result", entry.Message.Content[1].Type)
	assert.Equal(t, "chunk 1", entry.Message.Content[1].Content)
	assert.Equal(t, "tool_result", entry.Message.Content[2].Type)
	assert.Equal(t, "chunk 2", entry.Message.Content[2].Content)
	assert.Equal(t, "tool_result", entry.Message.Content[3].Type)
	assert.Equal(t, "chunk 3", entry.Message.Content[3].Content)

	// All tool_results should reference the same call_id
	for i := 1; i <= 3; i++ {
		assert.Equal(t, "call_multi", entry.Message.Content[i].ToolUseID)
	}
}

func TestParseCodexJSONL_EntryConsolidation(t *testing.T) {
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test-session"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"User message"}]}}
{"timestamp":"2026-01-04T13:22:17.000Z","type":"event_msg","payload":{"type":"agent_reasoning","text":"Thinking..."}}
{"timestamp":"2026-01-04T13:22:18.000Z","type":"response_item","payload":{"type":"function_call","name":"tool","arguments":"{}","call_id":"call_1"}}
{"timestamp":"2026-01-04T13:22:19.000Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_1","output":"result"}}
{"timestamp":"2026-01-04T13:22:20.000Z","type":"event_msg","payload":{"type":"agent_message","message":"Done!"}}
{"timestamp":"2026-01-04T13:22:21.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Final response"}]}}`

	result, err := ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)

	// Should consolidate consecutive assistant events
	// 1 user entry + 1 assistant entry (consolidated from multiple events)
	require.Len(t, result.Entries, 2)

	// First entry is user
	assert.Equal(t, "user", result.Entries[0].Type)

	// Second entry is assistant with all consolidated content
	assistantEntry := result.Entries[1]
	assert.Equal(t, "assistant", assistantEntry.Type)
	require.NotNil(t, assistantEntry.Message)

	// Should have: thinking, tool_use, tool_result, text (agent_message), text (final message)
	require.Len(t, assistantEntry.Message.Content, 5)
	assert.Equal(t, "thinking", assistantEntry.Message.Content[0].Type)
	assert.Equal(t, "tool_use", assistantEntry.Message.Content[1].Type)
	assert.Equal(t, "tool_result", assistantEntry.Message.Content[2].Type)
	assert.Equal(t, "text", assistantEntry.Message.Content[3].Type)
	assert.Equal(t, "text", assistantEntry.Message.Content[4].Type)
}

func TestParseCodexJSONL_MalformedLines(t *testing.T) {
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test-session"}}
not valid json
{"timestamp":"2026-01-04T13:22:16.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Valid message"}]}}`

	result, err := ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Entries, 1)
	assert.Equal(t, "Valid message", result.Entries[0].Message.Content[0].Text)

	// Should have a warning for the malformed line
	require.Len(t, result.Warnings, 1)
	assert.Equal(t, 2, result.Warnings[0].Line)
	assert.Contains(t, result.Warnings[0].Message, "failed to parse JSON")
}

func TestParseCodexJSONL_UnrecognizedEventType(t *testing.T) {
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test-session"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"unknown_type","payload":{}}
{"timestamp":"2026-01-04T13:22:17.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Valid"}]}}`

	result, err := ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Entries, 1)

	// Should have a warning for the unknown type
	require.Len(t, result.Warnings, 1)
	assert.Contains(t, result.Warnings[0].Message, "unrecognized event type")
}

func TestParseCodexJSONL_FunctionCallArgumentsParsing(t *testing.T) {
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test-session"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"response_item","payload":{"type":"function_call","name":"shell_command","arguments":"{\"command\":\"ls -la\",\"cwd\":\"/tmp\"}","call_id":"call_1"}}`

	result, err := ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Entries, 1)

	toolUse := result.Entries[0].Message.Content[0]
	assert.Equal(t, "tool_use", toolUse.Type)
	assert.Equal(t, "shell_command", toolUse.Name)

	// Input should be the parsed JSON
	inputMap, ok := toolUse.Input.(map[string]any)
	require.True(t, ok, "Input should be a map")
	assert.Equal(t, "ls -la", inputMap["command"])
	assert.Equal(t, "/tmp", inputMap["cwd"])
}

func TestParseCodexJSONL_InvalidArguments(t *testing.T) {
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test-session"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"response_item","payload":{"type":"function_call","name":"tool","arguments":"not valid json","call_id":"call_1"}}`

	result, err := ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Entries, 1)

	toolUse := result.Entries[0].Message.Content[0]
	assert.Equal(t, "tool_use", toolUse.Type)

	// Invalid JSON arguments should be stored as raw string
	assert.Equal(t, "not valid json", toolUse.Input)
}

func TestParseCodexJSONL_EmptyFile(t *testing.T) {
	result, err := ParseCodexJSONL(strings.NewReader(""))
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no valid entries found")
}

func TestParseCodexJSONL_NoSessionMeta(t *testing.T) {
	input := `{"timestamp":"2026-01-04T13:22:16.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Hello"}]}}`

	result, err := ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Entries, 1)

	// SessionID should be empty when no session_meta is present
	assert.Equal(t, "", result.Entries[0].SessionID)
}

func TestParseCodexJSONL_EdgeCasesFile(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "codex_edge_cases.jsonl"))
	require.NoError(t, err)

	result, err := ParseCodexJSONL(bytes.NewReader(data))
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should have parsed valid entries despite edge cases
	assert.Greater(t, len(result.Entries), 0)

	// Should have warnings for malformed/unknown content
	assert.Greater(t, len(result.Warnings), 0)
}

func TestParseCodexJSONL_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		warnMessage string
	}{
		{
			name: "missing type field",
			input: `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test"}}
{"timestamp":"2026-01-04T13:22:16.000Z","payload":{}}
{"timestamp":"2026-01-04T13:22:17.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Valid"}]}}`,
			warnMessage: "missing required field: type",
		},
		{
			name: "missing payload",
			input: `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"response_item"}
{"timestamp":"2026-01-04T13:22:17.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Valid"}]}}`,
			warnMessage: "missing required field: payload",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseCodexJSONL(strings.NewReader(tt.input))
			require.NoError(t, err)

			// Should have warning about missing field
			found := false
			for _, w := range result.Warnings {
				if strings.Contains(w.Message, tt.warnMessage) {
					found = true
					break
				}
			}
			assert.True(t, found, "expected warning containing %q", tt.warnMessage)
		})
	}
}

func TestParseCodexJSONL_UnrecognizedContentType(t *testing.T) {
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test-session"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"unknown_content_type","data":"some data"}]}}`

	result, err := ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Entries, 1)

	// Unknown content type should be rendered as text with raw JSON
	content := result.Entries[0].Message.Content[0]
	assert.Equal(t, "text", content.Type)
	assert.Contains(t, content.Text, "unknown_content_type")
}

// Property-based tests using rapid

func TestPropertyFormatDetectionIdempotent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a valid Codex JSONL content
		sessionID := rapid.StringMatching(`[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}`).Draw(t, "sessionID")
		timestamp := rapid.StringMatching(`2026-01-0[1-9]T[0-2][0-9]:[0-5][0-9]:[0-5][0-9]\.[0-9]{3}Z`).Draw(t, "timestamp")

		content := fmt.Sprintf(`{"timestamp":"%s","type":"session_meta","payload":{"id":"%s"}}`, timestamp, sessionID)

		// Format detection should return the same result when called multiple times
		format1, line1, err1 := DetectFormat(strings.NewReader(content))
		format2, line2, err2 := DetectFormat(strings.NewReader(content))

		if err1 != nil || err2 != nil {
			// Both should have same error state
			if (err1 != nil) != (err2 != nil) {
				t.Fatalf("inconsistent error states: %v vs %v", err1, err2)
			}
			return
		}

		if format1 != format2 {
			t.Fatalf("format detection not idempotent: %v != %v", format1, format2)
		}
		if string(line1) != string(line2) {
			t.Fatalf("first line not consistent: %q != %q", string(line1), string(line2))
		}
	})
}

func TestPropertyTextPreservedInNormalization(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate text that doesn't contain special characters that would break JSON
		text := rapid.StringMatching(`[a-zA-Z0-9 .,!?]{1,100}`).Draw(t, "text")
		timestamp := "2026-01-04T13:22:15.725Z"

		input := fmt.Sprintf(`{"timestamp":"%s","type":"session_meta","payload":{"id":"test"}}
{"timestamp":"%s","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"%s"}]}}`,
			timestamp, timestamp, text)

		result, err := ParseCodexJSONL(strings.NewReader(input))
		if err != nil {
			return // Skip invalid inputs
		}

		if len(result.Entries) == 0 {
			return
		}

		// Find the text in the normalized entries
		found := false
		for _, entry := range result.Entries {
			if entry.Message == nil {
				continue
			}
			for _, item := range entry.Message.Content {
				if item.Type == "text" && item.Text == text {
					found = true
					break
				}
			}
		}

		if !found {
			t.Fatalf("original text %q not preserved in normalized entries", text)
		}
	})
}

func TestPropertyToolCallLinkingCorrect(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a call ID
		callID := rapid.StringMatching(`call_[a-z0-9]{8}`).Draw(t, "callID")
		output := rapid.StringMatching(`[a-zA-Z0-9 ]{1,50}`).Draw(t, "output")
		timestamp := "2026-01-04T13:22:15.725Z"

		input := fmt.Sprintf(`{"timestamp":"%s","type":"session_meta","payload":{"id":"test"}}
{"timestamp":"%s","type":"response_item","payload":{"type":"function_call","name":"test_tool","arguments":"{}","call_id":"%s"}}
{"timestamp":"%s","type":"response_item","payload":{"type":"function_call_output","call_id":"%s","output":"%s"}}`,
			timestamp, timestamp, callID, timestamp, callID, output)

		result, err := ParseCodexJSONL(strings.NewReader(input))
		if err != nil {
			return
		}

		// Find the tool_result and verify linking
		for _, entry := range result.Entries {
			if entry.Message == nil {
				continue
			}
			for _, item := range entry.Message.Content {
				if item.Type == "tool_result" {
					if item.ToolUseID != callID {
						t.Fatalf("tool linking incorrect: ToolUseID %q != callID %q", item.ToolUseID, callID)
					}
				}
			}
		}
	})
}

// --- Phase 5: Error Handling and Negative Tests for Codex ---

func TestParseCodexJSONL_OrphanedOutputWarning_Negative(t *testing.T) {
	// Test orphaned function_call_output generates warning (req 4.7)
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test-session"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"response_item","payload":{"type":"function_call_output","call_id":"orphan_call_123","output":"orphaned output data"}}`

	result, err := ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)

	// Orphaned output should still be rendered
	require.Len(t, result.Entries, 1)
	entry := result.Entries[0]
	assert.Equal(t, "assistant", entry.Type)
	require.NotNil(t, entry.Message)
	require.Len(t, entry.Message.Content, 1)

	toolResult := entry.Message.Content[0]
	assert.Equal(t, "tool_result", toolResult.Type)
	assert.Equal(t, "orphan_call_123", toolResult.ToolUseID)
	assert.Equal(t, "orphaned output data", toolResult.Content)

	// Should have warning about orphaned output
	require.Len(t, result.Warnings, 1)
	assert.Contains(t, result.Warnings[0].Message, "no matching function_call for call_id")
	assert.Contains(t, result.Warnings[0].Message, "orphan_call_123")
	// Warning should include line number
	assert.Equal(t, 2, result.Warnings[0].Line)
}

func TestParseCodexJSONL_AllLinesMalformedError_Negative(t *testing.T) {
	// Test all lines malformed after session_meta returns error (req 9.5)
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test"}}
{invalid json line 1}
{invalid json line 2}
{invalid json line 3}`

	_, err := ParseCodexJSONL(strings.NewReader(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no valid entries found")
}

func TestParseCodexJSONL_MalformedMiddleLineWarning_Negative(t *testing.T) {
	// Test malformed line generates warning with line number (req 9.4)
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Valid before"}]}}
{not valid JSON in line 3}
{"timestamp":"2026-01-04T13:22:18.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Valid after"}]}}`

	result, err := ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)

	// Should have parsed valid entries
	require.Len(t, result.Entries, 2)

	// Should have warning for malformed line
	require.Len(t, result.Warnings, 1)
	assert.Equal(t, 3, result.Warnings[0].Line)
	assert.Contains(t, result.Warnings[0].Message, "failed to parse JSON")
}

func TestParseCodexJSONL_MissingTypeFieldWarning_Negative(t *testing.T) {
	// Test missing type field generates warning (req 9.2)
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test"}}
{"timestamp":"2026-01-04T13:22:16.000Z","payload":{"something":"data"}}
{"timestamp":"2026-01-04T13:22:17.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Valid"}]}}`

	result, err := ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)

	// Should have warning about missing type field
	require.Len(t, result.Warnings, 1)
	assert.Equal(t, 2, result.Warnings[0].Line)
	assert.Contains(t, result.Warnings[0].Message, "missing required field: type")
}

func TestParseCodexJSONL_MissingPayloadWarning_Negative(t *testing.T) {
	// Test missing payload field generates warning (req 9.2)
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"response_item"}
{"timestamp":"2026-01-04T13:22:17.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Valid"}]}}`

	result, err := ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)

	// Should have warning about missing payload
	require.Len(t, result.Warnings, 1)
	assert.Equal(t, 2, result.Warnings[0].Line)
	assert.Contains(t, result.Warnings[0].Message, "missing required field: payload")
}

func TestParseCodexJSONL_UnrecognizedEventTypeWarning_Negative(t *testing.T) {
	// Test unrecognized event type generates warning (req 4.11)
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"future_unknown_event","payload":{"data":"some"}}
{"timestamp":"2026-01-04T13:22:17.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Valid"}]}}`

	result, err := ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)

	// Should have parsed valid entry
	require.Len(t, result.Entries, 1)

	// Should have warning about unrecognized event type
	require.Len(t, result.Warnings, 1)
	assert.Equal(t, 2, result.Warnings[0].Line)
	assert.Contains(t, result.Warnings[0].Message, "unrecognized event type")
	assert.Contains(t, result.Warnings[0].Message, "future_unknown_event")
}

func TestParseCodexJSONL_UnrecognizedResponseItemTypeWarning_Negative(t *testing.T) {
	// Test unrecognized response_item type generates warning
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"response_item","payload":{"type":"future_response_type","data":"unknown"}}
{"timestamp":"2026-01-04T13:22:17.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Valid"}]}}`

	result, err := ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)

	// Should have warning about unrecognized response_item type
	require.Len(t, result.Warnings, 1)
	assert.Contains(t, result.Warnings[0].Message, "unrecognized response_item type")
}

func TestParseCodexJSONL_UnrecognizedEventMsgTypeWarning_Negative(t *testing.T) {
	// Test unrecognized event_msg type generates warning
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"event_msg","payload":{"type":"future_event_type","data":"unknown"}}
{"timestamp":"2026-01-04T13:22:17.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Valid"}]}}`

	result, err := ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)

	// Should have warning about unrecognized event_msg type
	require.Len(t, result.Warnings, 1)
	assert.Contains(t, result.Warnings[0].Message, "unrecognized event_msg type")
}

func TestParseCodexJSONL_WarningLineNumberFormat_Negative(t *testing.T) {
	// Verify warning message format includes line number correctly (req 9.4)
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test"}}
{"valid":"but_unknown","type":"weird_type"}
{"timestamp":"2026-01-04T13:22:17.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Valid"}]}}`

	result, err := ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)

	require.Len(t, result.Warnings, 1)
	// Line numbers should be accurate (1-indexed)
	assert.Equal(t, 2, result.Warnings[0].Line)
}

// --- Tool Name Display Tests (Phase 5, Task 24) ---

func TestParseCodexJSONL_ToolNameDisplay_ShellCommand(t *testing.T) {
	// Test shell_command displays as shell_command (not mapped to Bash) (req 7.1)
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"response_item","payload":{"type":"function_call","name":"shell_command","arguments":"{\"command\":\"ls -la\"}","call_id":"call_shell"}}`

	result, err := ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Entries, 1)

	toolUse := result.Entries[0].Message.Content[0]
	assert.Equal(t, "tool_use", toolUse.Type)
	// Tool name should be preserved exactly as shell_command (not mapped to Bash)
	assert.Equal(t, "shell_command", toolUse.Name)
	assert.Equal(t, "call_shell", toolUse.ID)
}

func TestParseCodexJSONL_ToolNameDisplay_PreservesOriginalNames(t *testing.T) {
	// Test various Codex tool names are preserved exactly (req 7.1)
	tests := []struct {
		name             string
		codexToolName    string
		expectedToolName string
	}{
		{"shell_command", "shell_command", "shell_command"},
		{"container_exec", "container_exec", "container_exec"},
		{"read_file", "read_file", "read_file"},
		{"write_to_file", "write_to_file", "write_to_file"},
		{"edit_file", "edit_file", "edit_file"},
		{"list_directory", "list_directory", "list_directory"},
		{"search_codebase", "search_codebase", "search_codebase"},
		{"python", "python", "python"},
		{"custom_mcp_tool", "custom_mcp_tool", "custom_mcp_tool"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := fmt.Sprintf(`{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"response_item","payload":{"type":"function_call","name":"%s","arguments":"{}","call_id":"call_1"}}`,
				tt.codexToolName)

			result, err := ParseCodexJSONL(strings.NewReader(input))
			require.NoError(t, err)
			require.Len(t, result.Entries, 1)

			toolUse := result.Entries[0].Message.Content[0]
			assert.Equal(t, tt.expectedToolName, toolUse.Name)
		})
	}
}

func TestParseCodexJSONL_ArgumentsJSONParsing(t *testing.T) {
	// Test arguments JSON parsing to extract command field (req 7.2)
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"response_item","payload":{"type":"function_call","name":"shell_command","arguments":"{\"command\":\"git status\",\"cwd\":\"/tmp\"}","call_id":"call_1"}}`

	result, err := ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Entries, 1)

	toolUse := result.Entries[0].Message.Content[0]
	assert.Equal(t, "tool_use", toolUse.Type)

	// Input should be parsed JSON with command field accessible
	inputMap, ok := toolUse.Input.(map[string]any)
	require.True(t, ok, "Input should be a map")
	assert.Equal(t, "git status", inputMap["command"])
	assert.Equal(t, "/tmp", inputMap["cwd"])
}

func TestParseCodexJSONL_ArgumentsJSONParsing_ComplexStructure(t *testing.T) {
	// Test arguments with nested JSON structure (req 7.2)
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"response_item","payload":{"type":"function_call","name":"read_file","arguments":"{\"file_path\":\"/path/to/file.txt\",\"encoding\":\"utf-8\",\"options\":{\"limit\":100}}","call_id":"call_1"}}`

	result, err := ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Entries, 1)

	toolUse := result.Entries[0].Message.Content[0]
	inputMap, ok := toolUse.Input.(map[string]any)
	require.True(t, ok)

	assert.Equal(t, "/path/to/file.txt", inputMap["file_path"])
	assert.Equal(t, "utf-8", inputMap["encoding"])

	// Nested options should also be parsed
	options, ok := inputMap["options"].(map[string]any)
	require.True(t, ok, "options should be a map")
	assert.Equal(t, float64(100), options["limit"]) // JSON numbers become float64
}

func TestParseCodexJSONL_RawArgumentsFallback(t *testing.T) {
	// Test raw arguments fallback for invalid JSON (req 7.3)
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"response_item","payload":{"type":"function_call","name":"shell_command","arguments":"not valid json at all","call_id":"call_1"}}`

	result, err := ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Entries, 1)

	toolUse := result.Entries[0].Message.Content[0]
	assert.Equal(t, "tool_use", toolUse.Type)
	assert.Equal(t, "shell_command", toolUse.Name)

	// Invalid JSON arguments should be stored as raw string
	rawArgs, ok := toolUse.Input.(string)
	require.True(t, ok, "Input should be a string for invalid JSON")
	assert.Equal(t, "not valid json at all", rawArgs)
}

func TestParseCodexJSONL_EmptyArguments(t *testing.T) {
	// Test empty arguments string
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"response_item","payload":{"type":"function_call","name":"shell_command","arguments":"","call_id":"call_1"}}`

	result, err := ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Entries, 1)

	toolUse := result.Entries[0].Message.Content[0]
	assert.Equal(t, "tool_use", toolUse.Type)
	// Empty string is valid (though not valid JSON), stored as-is
	assert.Equal(t, "", toolUse.Input)
}

func TestParseCodexJSONL_EmptyJSONObjectArguments(t *testing.T) {
	// Test empty JSON object arguments "{}"
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"test"}}
{"timestamp":"2026-01-04T13:22:16.000Z","type":"response_item","payload":{"type":"function_call","name":"some_tool","arguments":"{}","call_id":"call_1"}}`

	result, err := ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Entries, 1)

	toolUse := result.Entries[0].Message.Content[0]
	assert.Equal(t, "tool_use", toolUse.Type)

	// Empty JSON object should be parsed
	inputMap, ok := toolUse.Input.(map[string]any)
	require.True(t, ok, "Input should be a map")
	assert.Empty(t, inputMap)
}
