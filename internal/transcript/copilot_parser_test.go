package transcript

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCopilot_TimestampOnUserMessage(t *testing.T) {
	input := `{"type":"session.start","data":{"sessionId":"s1"},"id":"1","timestamp":"2026-01-17T12:00:00Z"}
{"type":"user.message","data":{"content":"hello"},"id":"2","timestamp":"2026-01-17T12:00:01Z"}
{"type":"assistant.turn_start","data":{"turnId":"0"},"id":"3","timestamp":"2026-01-17T12:00:02Z"}
{"type":"assistant.message","data":{"messageId":"m1","content":"hi"},"id":"4","timestamp":"2026-01-17T12:00:03Z"}
{"type":"assistant.turn_end","data":{"turnId":"0"},"id":"5","timestamp":"2026-01-17T12:00:04Z"}`

	result, err := ParseCopilot(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Entries, 2)

	assert.Equal(t, "user", result.Entries[0].Type)
	assert.Equal(t, "2026-01-17T12:00:01Z", result.Entries[0].Timestamp,
		"user message should get timestamp from event.Timestamp")
}

func TestParseCopilot_TimestampOnAssistantFromTurnStart(t *testing.T) {
	input := `{"type":"session.start","data":{"sessionId":"s1"},"id":"1","timestamp":"2026-01-17T12:00:00Z"}
{"type":"user.message","data":{"content":"hello"},"id":"2","timestamp":"2026-01-17T12:00:01Z"}
{"type":"assistant.turn_start","data":{"turnId":"0"},"id":"3","timestamp":"2026-01-17T12:00:02Z"}
{"type":"assistant.message","data":{"messageId":"m1","content":"hi"},"id":"4","timestamp":"2026-01-17T12:00:03Z"}
{"type":"assistant.turn_end","data":{"turnId":"0"},"id":"5","timestamp":"2026-01-17T12:00:04Z"}`

	result, err := ParseCopilot(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Entries, 2)

	assert.Equal(t, "assistant", result.Entries[1].Type)
	assert.Equal(t, "2026-01-17T12:00:02Z", result.Entries[1].Timestamp,
		"assistant entry should get timestamp from turn_start event")
}

func TestParseCopilot_ModelFromModelChangeEvent(t *testing.T) {
	input := `{"type":"session.start","data":{"sessionId":"s1"},"id":"1","timestamp":"2026-01-17T12:00:00Z"}
{"type":"session.model_change","data":{"model":"gpt-4o"},"id":"2","timestamp":"2026-01-17T12:00:00.500Z"}
{"type":"user.message","data":{"content":"hello"},"id":"3","timestamp":"2026-01-17T12:00:01Z"}
{"type":"assistant.turn_start","data":{"turnId":"0"},"id":"4","timestamp":"2026-01-17T12:00:02Z"}
{"type":"assistant.message","data":{"messageId":"m1","content":"hi"},"id":"5","timestamp":"2026-01-17T12:00:03Z"}
{"type":"assistant.turn_end","data":{"turnId":"0"},"id":"6","timestamp":"2026-01-17T12:00:04Z"}`

	result, err := ParseCopilot(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Entries, 2)

	// User entry should NOT have model
	assert.Empty(t, result.Entries[0].Model, "user entries should not have model")

	// Assistant entry should have model from model_change event
	assert.Equal(t, "assistant", result.Entries[1].Type)
	assert.Equal(t, "gpt-4o", result.Entries[1].Model)
}

func TestParseCopilot_ModelChangeMidSession(t *testing.T) {
	// Model changes mid-session: earlier entries keep old model, later entries get new model
	input := `{"type":"session.start","data":{"sessionId":"s1"},"id":"1","timestamp":"2026-01-17T12:00:00Z"}
{"type":"session.model_change","data":{"model":"gpt-4o"},"id":"2","timestamp":"2026-01-17T12:00:00.500Z"}
{"type":"user.message","data":{"content":"first"},"id":"3","timestamp":"2026-01-17T12:00:01Z"}
{"type":"assistant.turn_start","data":{"turnId":"0"},"id":"4","timestamp":"2026-01-17T12:00:02Z"}
{"type":"assistant.message","data":{"messageId":"m1","content":"reply1"},"id":"5","timestamp":"2026-01-17T12:00:03Z"}
{"type":"assistant.turn_end","data":{"turnId":"0"},"id":"6","timestamp":"2026-01-17T12:00:04Z"}
{"type":"session.model_change","data":{"model":"claude-sonnet"},"id":"7","timestamp":"2026-01-17T12:00:05Z"}
{"type":"user.message","data":{"content":"second"},"id":"8","timestamp":"2026-01-17T12:00:06Z"}
{"type":"assistant.turn_start","data":{"turnId":"1"},"id":"9","timestamp":"2026-01-17T12:00:07Z"}
{"type":"assistant.message","data":{"messageId":"m2","content":"reply2"},"id":"10","timestamp":"2026-01-17T12:00:08Z"}
{"type":"assistant.turn_end","data":{"turnId":"1"},"id":"11","timestamp":"2026-01-17T12:00:09Z"}`

	result, err := ParseCopilot(strings.NewReader(input))
	require.NoError(t, err)

	// Find assistant entries
	var assistants []Entry
	for _, e := range result.Entries {
		if e.Type == "assistant" {
			assistants = append(assistants, e)
		}
	}
	require.Len(t, assistants, 2)

	assert.Equal(t, "gpt-4o", assistants[0].Model, "first assistant should have original model")
	assert.Equal(t, "claude-sonnet", assistants[1].Model, "second assistant should have updated model")
}

func TestParseCopilot_NoModelChangeEvent(t *testing.T) {
	// Without model_change event, model should be empty
	input := `{"type":"session.start","data":{"sessionId":"s1"},"id":"1","timestamp":"2026-01-17T12:00:00Z"}
{"type":"user.message","data":{"content":"hello"},"id":"2","timestamp":"2026-01-17T12:00:01Z"}
{"type":"assistant.turn_start","data":{"turnId":"0"},"id":"3","timestamp":"2026-01-17T12:00:02Z"}
{"type":"assistant.message","data":{"messageId":"m1","content":"hi"},"id":"4","timestamp":"2026-01-17T12:00:03Z"}
{"type":"assistant.turn_end","data":{"turnId":"0"},"id":"5","timestamp":"2026-01-17T12:00:04Z"}`

	result, err := ParseCopilot(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Entries, 2)

	assert.Empty(t, result.Entries[1].Model, "model should be empty without model_change event")
}

func TestParseCopilot_TimestampsOnMultipleTurns(t *testing.T) {
	input := `{"type":"session.start","data":{"sessionId":"s1"},"id":"1","timestamp":"2026-01-17T12:00:00Z"}
{"type":"user.message","data":{"content":"first"},"id":"2","timestamp":"2026-01-17T12:00:01Z"}
{"type":"assistant.turn_start","data":{"turnId":"0"},"id":"3","timestamp":"2026-01-17T12:00:02Z"}
{"type":"assistant.message","data":{"messageId":"m1","content":"reply1"},"id":"4","timestamp":"2026-01-17T12:00:03Z"}
{"type":"assistant.turn_end","data":{"turnId":"0"},"id":"5","timestamp":"2026-01-17T12:00:04Z"}
{"type":"user.message","data":{"content":"second"},"id":"6","timestamp":"2026-01-17T12:00:10Z"}
{"type":"assistant.turn_start","data":{"turnId":"1"},"id":"7","timestamp":"2026-01-17T12:00:11Z"}
{"type":"assistant.message","data":{"messageId":"m2","content":"reply2"},"id":"8","timestamp":"2026-01-17T12:00:12Z"}
{"type":"assistant.turn_end","data":{"turnId":"1"},"id":"9","timestamp":"2026-01-17T12:00:13Z"}`

	result, err := ParseCopilot(strings.NewReader(input))
	require.NoError(t, err)

	// Expect: user, assistant, tool-results(if any), user, assistant
	var users, assistants []Entry
	for _, e := range result.Entries {
		switch e.Type {
		case "user":
			if e.Message != nil && len(e.Message.Content) > 0 && e.Message.Content[0].Type == "text" {
				users = append(users, e)
			}
		case "assistant":
			assistants = append(assistants, e)
		}
	}
	require.Len(t, users, 2)
	require.Len(t, assistants, 2)

	assert.Equal(t, "2026-01-17T12:00:01Z", users[0].Timestamp)
	assert.Equal(t, "2026-01-17T12:00:10Z", users[1].Timestamp)
	assert.Equal(t, "2026-01-17T12:00:02Z", assistants[0].Timestamp, "first turn_start timestamp")
	assert.Equal(t, "2026-01-17T12:00:11Z", assistants[1].Timestamp, "second turn_start timestamp")
}

func TestParseCopilot_ToolResultEntriesGetUserTimestamp(t *testing.T) {
	// Tool result entries (user type with tool_result content) should NOT get a timestamp
	// since they are synthetic entries, not actual user messages
	input := `{"type":"session.start","data":{"sessionId":"s1"},"id":"1","timestamp":"2026-01-17T12:00:00Z"}
{"type":"user.message","data":{"content":"run ls"},"id":"2","timestamp":"2026-01-17T12:00:01Z"}
{"type":"assistant.turn_start","data":{"turnId":"0"},"id":"3","timestamp":"2026-01-17T12:00:02Z"}
{"type":"assistant.message","data":{"messageId":"m1","content":"","toolRequests":[{"toolCallId":"tc1","name":"bash","arguments":{"command":"ls"}}]},"id":"4","timestamp":"2026-01-17T12:00:03Z"}
{"type":"tool.execution_complete","data":{"toolCallId":"tc1","success":true,"result":{"content":"file.txt"}},"id":"5","timestamp":"2026-01-17T12:00:04Z"}
{"type":"assistant.turn_end","data":{"turnId":"0"},"id":"6","timestamp":"2026-01-17T12:00:05Z"}`

	result, err := ParseCopilot(strings.NewReader(input))
	require.NoError(t, err)

	// Entries: user(text), assistant(tool_use), user(tool_result)
	require.Len(t, result.Entries, 3)

	// Real user message gets timestamp
	assert.Equal(t, "user", result.Entries[0].Type)
	assert.Equal(t, "2026-01-17T12:00:01Z", result.Entries[0].Timestamp)

	// Assistant gets turn_start timestamp
	assert.Equal(t, "assistant", result.Entries[1].Type)
	assert.Equal(t, "2026-01-17T12:00:02Z", result.Entries[1].Timestamp)

	// Synthetic tool-result user entry should not have a timestamp
	assert.Equal(t, "user", result.Entries[2].Type)
	assert.Empty(t, result.Entries[2].Timestamp,
		"synthetic tool-result entries should not have a timestamp")
}

func TestParseCopilot_ModelOnlyOnAssistantEntries(t *testing.T) {
	// Model should only appear on assistant entries, never on user entries
	input := `{"type":"session.start","data":{"sessionId":"s1"},"id":"1","timestamp":"2026-01-17T12:00:00Z"}
{"type":"session.model_change","data":{"model":"gpt-4o"},"id":"2","timestamp":"2026-01-17T12:00:00.500Z"}
{"type":"user.message","data":{"content":"hello"},"id":"3","timestamp":"2026-01-17T12:00:01Z"}
{"type":"assistant.turn_start","data":{"turnId":"0"},"id":"4","timestamp":"2026-01-17T12:00:02Z"}
{"type":"assistant.message","data":{"messageId":"m1","content":"","toolRequests":[{"toolCallId":"tc1","name":"bash","arguments":{"command":"ls"}}]},"id":"5","timestamp":"2026-01-17T12:00:03Z"}
{"type":"tool.execution_complete","data":{"toolCallId":"tc1","success":true,"result":{"content":"ok"}},"id":"6","timestamp":"2026-01-17T12:00:04Z"}
{"type":"assistant.turn_end","data":{"turnId":"0"},"id":"7","timestamp":"2026-01-17T12:00:05Z"}`

	result, err := ParseCopilot(strings.NewReader(input))
	require.NoError(t, err)

	for _, entry := range result.Entries {
		if entry.Type == "user" {
			assert.Empty(t, entry.Model, "user entries must not have model set")
		}
		if entry.Type == "assistant" {
			assert.Equal(t, "gpt-4o", entry.Model)
		}
	}
}
