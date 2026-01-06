package transcript

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodexEntry_Unmarshal(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected CodexEntry
	}{
		{
			name:  "session_meta entry",
			input: `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"abc123","cwd":"/home/user"}}`,
			expected: CodexEntry{
				Timestamp: "2026-01-04T13:22:15.725Z",
				Type:      "session_meta",
			},
		},
		{
			name:  "response_item entry",
			input: `{"timestamp":"2026-01-04T13:22:16.000Z","type":"response_item","payload":{"type":"message","role":"user"}}`,
			expected: CodexEntry{
				Timestamp: "2026-01-04T13:22:16.000Z",
				Type:      "response_item",
			},
		},
		{
			name:  "event_msg entry",
			input: `{"timestamp":"2026-01-04T13:22:17.000Z","type":"event_msg","payload":{"type":"agent_reasoning","text":"thinking"}}`,
			expected: CodexEntry{
				Timestamp: "2026-01-04T13:22:17.000Z",
				Type:      "event_msg",
			},
		},
		{
			name:  "turn_context entry",
			input: `{"timestamp":"2026-01-04T13:22:18.000Z","type":"turn_context","payload":{}}`,
			expected: CodexEntry{
				Timestamp: "2026-01-04T13:22:18.000Z",
				Type:      "turn_context",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var entry CodexEntry
			err := json.Unmarshal([]byte(tt.input), &entry)
			require.NoError(t, err)
			assert.Equal(t, tt.expected.Timestamp, entry.Timestamp)
			assert.Equal(t, tt.expected.Type, entry.Type)
			assert.NotEmpty(t, entry.Payload)
		})
	}
}

func TestCodexEntry_MissingFields(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectType    string
		expectPayload bool
	}{
		{
			name:          "missing timestamp",
			input:         `{"type":"session_meta","payload":{}}`,
			expectType:    "session_meta",
			expectPayload: true,
		},
		{
			name:          "missing type",
			input:         `{"timestamp":"2026-01-04T13:22:15.725Z","payload":{}}`,
			expectType:    "",
			expectPayload: true,
		},
		{
			name:          "missing payload",
			input:         `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta"}`,
			expectType:    "session_meta",
			expectPayload: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var entry CodexEntry
			err := json.Unmarshal([]byte(tt.input), &entry)
			require.NoError(t, err)
			assert.Equal(t, tt.expectType, entry.Type)
			if tt.expectPayload {
				assert.NotEmpty(t, entry.Payload)
			} else {
				assert.Empty(t, entry.Payload)
			}
		})
	}
}

func TestCodexEntry_ExtraFields(t *testing.T) {
	input := `{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{},"extra_field":"ignored","another":123}`
	var entry CodexEntry
	err := json.Unmarshal([]byte(input), &entry)
	require.NoError(t, err)
	assert.Equal(t, "session_meta", entry.Type)
	assert.Equal(t, "2026-01-04T13:22:15.725Z", entry.Timestamp)
}

func TestCodexResponseItem_Unmarshal(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected CodexResponseItem
	}{
		{
			name:  "user message",
			input: `{"type":"message","role":"user","content":[{"type":"input_text","text":"Hello"}]}`,
			expected: CodexResponseItem{
				Type: "message",
				Role: "user",
				Content: []CodexContent{
					{Type: "input_text", Text: "Hello"},
				},
			},
		},
		{
			name:  "assistant message",
			input: `{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hi there"}]}`,
			expected: CodexResponseItem{
				Type: "message",
				Role: "assistant",
				Content: []CodexContent{
					{Type: "output_text", Text: "Hi there"},
				},
			},
		},
		{
			name:  "function_call",
			input: `{"type":"function_call","name":"shell_command","arguments":"{\"command\":\"ls\"}","call_id":"call_abc123"}`,
			expected: CodexResponseItem{
				Type:      "function_call",
				Name:      "shell_command",
				Arguments: `{"command":"ls"}`,
				CallID:    "call_abc123",
			},
		},
		{
			name:  "function_call_output",
			input: `{"type":"function_call_output","call_id":"call_abc123","output":"file1.txt\nfile2.txt"}`,
			expected: CodexResponseItem{
				Type:   "function_call_output",
				CallID: "call_abc123",
				Output: "file1.txt\nfile2.txt",
			},
		},
		{
			name:  "reasoning with summary",
			input: `{"type":"reasoning","summary":[{"type":"summary_text","text":"Analyzing the request"}]}`,
			expected: CodexResponseItem{
				Type: "reasoning",
				Summary: []CodexSummary{
					{Type: "summary_text", Text: "Analyzing the request"},
				},
			},
		},
		{
			name:  "ghost_snapshot",
			input: `{"type":"ghost_snapshot"}`,
			expected: CodexResponseItem{
				Type: "ghost_snapshot",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var item CodexResponseItem
			err := json.Unmarshal([]byte(tt.input), &item)
			require.NoError(t, err)
			assert.Equal(t, tt.expected.Type, item.Type)
			assert.Equal(t, tt.expected.Role, item.Role)
			assert.Equal(t, tt.expected.Name, item.Name)
			assert.Equal(t, tt.expected.Arguments, item.Arguments)
			assert.Equal(t, tt.expected.CallID, item.CallID)
			assert.Equal(t, tt.expected.Output, item.Output)
			assert.Equal(t, tt.expected.Content, item.Content)
			assert.Equal(t, tt.expected.Summary, item.Summary)
		})
	}
}

func TestCodexResponseItem_MissingFields(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		expectType string
	}{
		{
			name:       "missing type",
			input:      `{"role":"user"}`,
			expectType: "",
		},
		{
			name:       "empty content array",
			input:      `{"type":"message","role":"user","content":[]}`,
			expectType: "message",
		},
		{
			name:       "null content",
			input:      `{"type":"message","role":"user","content":null}`,
			expectType: "message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var item CodexResponseItem
			err := json.Unmarshal([]byte(tt.input), &item)
			require.NoError(t, err)
			assert.Equal(t, tt.expectType, item.Type)
		})
	}
}

func TestCodexResponseItem_ExtraFields(t *testing.T) {
	input := `{"type":"message","role":"user","content":[],"extra":"ignored","id":"some_id"}`
	var item CodexResponseItem
	err := json.Unmarshal([]byte(input), &item)
	require.NoError(t, err)
	assert.Equal(t, "message", item.Type)
	assert.Equal(t, "user", item.Role)
}

func TestCodexContent_Unmarshal(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected CodexContent
	}{
		{
			name:     "input_text",
			input:    `{"type":"input_text","text":"User message"}`,
			expected: CodexContent{Type: "input_text", Text: "User message"},
		},
		{
			name:     "output_text",
			input:    `{"type":"output_text","text":"Assistant response"}`,
			expected: CodexContent{Type: "output_text", Text: "Assistant response"},
		},
		{
			name:     "empty text",
			input:    `{"type":"input_text","text":""}`,
			expected: CodexContent{Type: "input_text", Text: ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var content CodexContent
			err := json.Unmarshal([]byte(tt.input), &content)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, content)
		})
	}
}

func TestCodexContent_MissingFields(t *testing.T) {
	input := `{"type":"input_text"}`
	var content CodexContent
	err := json.Unmarshal([]byte(input), &content)
	require.NoError(t, err)
	assert.Equal(t, "input_text", content.Type)
	assert.Equal(t, "", content.Text)
}

func TestCodexContent_ExtraFields(t *testing.T) {
	input := `{"type":"input_text","text":"Hello","annotations":[],"extra":"ignored"}`
	var content CodexContent
	err := json.Unmarshal([]byte(input), &content)
	require.NoError(t, err)
	assert.Equal(t, "input_text", content.Type)
	assert.Equal(t, "Hello", content.Text)
}

func TestCodexSummary_Unmarshal(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected CodexSummary
	}{
		{
			name:     "summary_text",
			input:    `{"type":"summary_text","text":"**Analyzing the problem**"}`,
			expected: CodexSummary{Type: "summary_text", Text: "**Analyzing the problem**"},
		},
		{
			name:     "empty text",
			input:    `{"type":"summary_text","text":""}`,
			expected: CodexSummary{Type: "summary_text", Text: ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var summary CodexSummary
			err := json.Unmarshal([]byte(tt.input), &summary)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, summary)
		})
	}
}

func TestCodexSummary_MissingFields(t *testing.T) {
	input := `{"type":"summary_text"}`
	var summary CodexSummary
	err := json.Unmarshal([]byte(input), &summary)
	require.NoError(t, err)
	assert.Equal(t, "summary_text", summary.Type)
	assert.Equal(t, "", summary.Text)
}

func TestCodexSummary_ExtraFields(t *testing.T) {
	input := `{"type":"summary_text","text":"Summary","extra":"ignored"}`
	var summary CodexSummary
	err := json.Unmarshal([]byte(input), &summary)
	require.NoError(t, err)
	assert.Equal(t, "summary_text", summary.Type)
	assert.Equal(t, "Summary", summary.Text)
}

func TestCodexEventMsg_Unmarshal(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected CodexEventMsg
	}{
		{
			name:     "agent_reasoning",
			input:    `{"type":"agent_reasoning","text":"**Preparing to execute**"}`,
			expected: CodexEventMsg{Type: "agent_reasoning", Text: "**Preparing to execute**"},
		},
		{
			name:     "agent_message",
			input:    `{"type":"agent_message","message":"I found 2 files."}`,
			expected: CodexEventMsg{Type: "agent_message", Message: "I found 2 files."},
		},
		{
			name:     "user_message",
			input:    `{"type":"user_message","message":"List files"}`,
			expected: CodexEventMsg{Type: "user_message", Message: "List files"},
		},
		{
			name:     "token_count",
			input:    `{"type":"token_count"}`,
			expected: CodexEventMsg{Type: "token_count"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var msg CodexEventMsg
			err := json.Unmarshal([]byte(tt.input), &msg)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, msg)
		})
	}
}

func TestCodexEventMsg_MissingFields(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		expectType string
	}{
		{
			name:       "missing type",
			input:      `{"text":"some text"}`,
			expectType: "",
		},
		{
			name:       "type only",
			input:      `{"type":"agent_reasoning"}`,
			expectType: "agent_reasoning",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var msg CodexEventMsg
			err := json.Unmarshal([]byte(tt.input), &msg)
			require.NoError(t, err)
			assert.Equal(t, tt.expectType, msg.Type)
		})
	}
}

func TestCodexEventMsg_ExtraFields(t *testing.T) {
	input := `{"type":"agent_message","message":"Hello","extra":"ignored","count":42}`
	var msg CodexEventMsg
	err := json.Unmarshal([]byte(input), &msg)
	require.NoError(t, err)
	assert.Equal(t, "agent_message", msg.Type)
	assert.Equal(t, "Hello", msg.Message)
}

func TestCodexSessionMeta_Unmarshal(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected CodexSessionMeta
	}{
		{
			name:  "full session meta",
			input: `{"id":"019b892c-3a14-7773-bd76-6465a8a0b634","timestamp":"2026-01-04T13:22:15.725Z","cwd":"/Users/arjen/projects/orbit"}`,
			expected: CodexSessionMeta{
				ID:        "019b892c-3a14-7773-bd76-6465a8a0b634",
				Timestamp: "2026-01-04T13:22:15.725Z",
				Cwd:       "/Users/arjen/projects/orbit",
			},
		},
		{
			name:  "minimal session meta",
			input: `{"id":"abc123"}`,
			expected: CodexSessionMeta{
				ID: "abc123",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var meta CodexSessionMeta
			err := json.Unmarshal([]byte(tt.input), &meta)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, meta)
		})
	}
}

func TestCodexSessionMeta_MissingFields(t *testing.T) {
	input := `{"timestamp":"2026-01-04T13:22:15.725Z"}`
	var meta CodexSessionMeta
	err := json.Unmarshal([]byte(input), &meta)
	require.NoError(t, err)
	assert.Equal(t, "", meta.ID)
	assert.Equal(t, "2026-01-04T13:22:15.725Z", meta.Timestamp)
}

func TestCodexSessionMeta_ExtraFields(t *testing.T) {
	input := `{"id":"abc123","timestamp":"2026-01-04T13:22:15.725Z","cwd":"/home","model":"gpt-4","version":"1.0"}`
	var meta CodexSessionMeta
	err := json.Unmarshal([]byte(input), &meta)
	require.NoError(t, err)
	assert.Equal(t, "abc123", meta.ID)
	assert.Equal(t, "2026-01-04T13:22:15.725Z", meta.Timestamp)
	assert.Equal(t, "/home", meta.Cwd)
}
