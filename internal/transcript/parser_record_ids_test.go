package transcript

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordIDs lists each entry's RecordID in order.
func recordIDs(entries []Entry) []string {
	ids := make([]string, len(entries))
	for i, entry := range entries {
		ids[i] = entry.RecordID
	}
	return ids
}

func TestParseOrigin_Claude(t *testing.T) {
	input := `{"type":"queue-operation","entrypoint":""}
{"type":"user","uuid":"user-1","entrypoint":"cli","message":{"role":"user","content":"Hello"}}
{"type":"assistant","uuid":"assistant-1","parentUuid":"user-1","entrypoint":"sdk-cli","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"ls"}}]}}
{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"ok"}]}}`
	result, err := Parse(strings.NewReader(input))
	require.NoError(t, err)
	require.NotNil(t, result.Metadata)
	assert.Equal(t, "cli", result.Metadata.Origin, "the first nonempty entrypoint; a later value is not reported")
	assert.Equal(t, []string{"user-1", "assistant-1", ""}, recordIDs(result.Entries))
	assert.Equal(t, "user-1", result.Entries[0].UUID)
	assert.Equal(t, "user-1", result.Entries[1].ParentUUID)
	assert.Equal(t, "toolu_1", result.Entries[1].Message.Content[0].ID)
	assert.Equal(t, "toolu_1", result.Entries[2].Message.Content[0].ToolUseID)

	result, err = Parse(strings.NewReader(`{"type":"user","uuid":"user-1","message":{"role":"user","content":"Hello"}}`))
	require.NoError(t, err)
	assert.Empty(t, result.Metadata.Origin)
	assert.Equal(t, []string{"user-1"}, recordIDs(result.Entries))
}

// The originator and message id shapes follow the pinned rust-v0.166.0
// protocol.rs SessionMeta.originator and models.rs ResponseItem::Message id.
func TestParseOrigin_Codex(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "codex", "turn_context.jsonl"))
	require.NoError(t, err)
	for _, opts := range []ParseOptions{{}, {AllowEmptyCodex: true, PreserveModelChanges: true}} {
		result, err := ParseCodexJSONL(strings.NewReader(string(data)), opts)
		require.NoError(t, err)
		assert.Equal(t, "codex_exec", result.Metadata.Origin)
		require.NotEmpty(t, result.Entries)
		assert.Equal(t, "user", result.Entries[0].Type)
		assert.Equal(t, "msg_user_1", result.Entries[0].RecordID)
		for _, entry := range result.Entries[1:] {
			assert.Empty(t, entry.RecordID, "assistant entries consolidate several response items")
		}
	}

	input := `{"type":"session_meta","payload":{"id":"example-session","originator":""}}
{"type":"session_meta","payload":{"id":"example-session","originator":"codex_cli_rs"}}
{"type":"session_meta","payload":{"id":"example-session","originator":"codex_exec"}}`
	result, err := ParseCodexJSONL(strings.NewReader(input), ParseOptions{AllowEmptyCodex: true})
	require.NoError(t, err)
	assert.Empty(t, result.Entries)
	assert.Equal(t, "codex_cli_rs", result.Metadata.Origin)

	input = `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"No id"}]}}
{"type":"response_item","payload":{"type":"message","id":"msg_assistant","role":"assistant","content":[{"type":"output_text","text":"Single item"}]}}
{"type":"response_item","payload":{"type":"message","id":"msg_user_2","role":"user","content":[{"type":"input_text","text":"With id"}]}}`
	result, err = ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)
	assert.Empty(t, result.Metadata.Origin)
	assert.Equal(t, []string{"", "", "msg_user_2"}, recordIDs(result.Entries))
}

func TestParseRecordIDs_Copilot(t *testing.T) {
	input := `{"type":"session.start","id":"evt-1","timestamp":"2026-10-01T00:00:00Z","data":{"producer":"example-producer","startTime":"2026-10-01T00:00:00Z"}}
{"type":"user.message","id":"evt-2","data":{"content":"Hello"}}
{"type":"assistant.turn_start","id":"evt-3"}
{"type":"assistant.message","id":"evt-4","data":{"content":"Running","toolRequests":[{"toolCallId":"call-1","name":"bash","arguments":{"command":"ls"}}]}}
{"type":"tool.execution_complete","id":"evt-5","data":{"toolCallId":"call-1","result":{"content":"ok"}}}
{"type":"assistant.turn_end","id":"evt-6"}`
	result, err := ParseCopilot(strings.NewReader(input))
	require.NoError(t, err)
	assert.Empty(t, result.Metadata.Origin, "producer is not an origin (Q96)")
	require.Len(t, result.Entries, 3)
	assert.Equal(t, []string{"evt-2", "", ""}, recordIDs(result.Entries))
	assert.Equal(t, "call-1", result.Entries[1].Message.Content[1].ID)
	assert.Equal(t, "call-1", result.Entries[2].Message.Content[0].ToolUseID)
}

func TestParseRecordIDs_Kiro(t *testing.T) {
	input := `{"conversation_id":"example-session","history":[
{"user":{"content":{"Prompt":{"prompt":"Hello"}}}},
{"assistant":{"ToolUse":{"message_id":"message-1","content":"Running","tool_uses":[{"id":"tool-1","name":"bash","args":{}}]}}},
{"user":{"content":{"ToolUseResults":{"tool_use_results":[{"tool_use_id":"tool-1","content":[{"Text":"ok"}],"status":"Success"}]}}}},
{"assistant":{"Response":{"message_id":"message-2","content":"Done"}}},
{"assistant":{"TextResponse":{"message_id":"message-3","content":"Also done"}}}
]}`
	result, err := ParseKiro(strings.NewReader(input))
	require.NoError(t, err)
	assert.Empty(t, result.Metadata.Origin)
	assert.Equal(t, []string{"", "message-1", "", "message-2", "message-3"}, recordIDs(result.Entries))
	assert.Equal(t, "tool-1", result.Entries[1].Message.Content[1].ID)
	assert.Equal(t, "tool-1", result.Entries[2].Message.Content[0].ToolUseID)
}

func TestParseRecordIDs_KiroIDE(t *testing.T) {
	chat := `{"executionId":"example-execution","metadata":{"modelId":"example-model"},"chat":[{"role":"human","content":"Hello"},{"role":"bot","content":"Hi"}]}`
	result, err := ParseKiroIDE(strings.NewReader(chat))
	require.NoError(t, err)
	assert.Empty(t, result.Metadata.Origin)
	assert.Equal(t, []string{"", ""}, recordIDs(result.Entries), "chat-file entries record no identifier")

	costPath := filepath.Join(t.TempDir(), "execution.json")
	actions := `{"actions":[
{"actionId":"action-1","actionType":"say","output":{"message":"Starting"}},
{"actionId":"action-2","actionType":"runCommand","actionState":"Success","input":{"command":"ls"},"output":{"output":"ok","exitCode":0}},
{"actionId":"action-3","actionType":"taskStatus","taskId":"task-1","taskStatus":"done"},
{"actionId":"action-4","actionType":"userInput","output":{"questions":[{"question":"Continue?"}]}}
]}`
	require.NoError(t, os.WriteFile(costPath, []byte(actions), 0o600))
	result, err = ParseKiroIDEWithCostPath(strings.NewReader(chat), costPath)
	require.NoError(t, err)
	assert.Empty(t, result.Metadata.Origin)
	assert.Equal(t, []string{"", "action-1", "action-2", "action-2", "action-3", "action-4"}, recordIDs(result.Entries))
	assert.Equal(t, "action-2", result.Entries[2].Message.Content[0].ID)
	assert.Equal(t, "action-2", result.Entries[3].Message.Content[0].ToolUseID)
}

// Renderers and the raw entry shape ignore the new fields (Decision 31).
func TestParseRecordIDs_OutputUnchanged(t *testing.T) {
	for _, fixture := range []string{"claude.jsonl", "codex.jsonl", "copilot.jsonl", "kiro.json", "kiro_ide.json"} {
		t.Run(fixture, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", "integration", fixture))
			require.NoError(t, err)
			result, err := Parse(strings.NewReader(string(data)))
			require.NoError(t, err)
			marked := append([]Entry(nil), result.Entries...)
			cleared := append([]Entry(nil), result.Entries...)
			for i := range marked {
				marked[i].RecordID = "record-id"
				marked[i].Origin = "example-origin"
				cleared[i].RecordID = ""
				cleared[i].Origin = ""
			}
			for _, render := range []func([]Entry, RenderOptions) string{RenderMarkdown, RenderHTML, RenderHTMLFragment} {
				assert.Equal(t, render(cleared, RenderOptions{}), render(marked, RenderOptions{}))
			}
			for i := range marked {
				got, err := json.Marshal(marked[i])
				require.NoError(t, err)
				want, err := json.Marshal(cleared[i])
				require.NoError(t, err)
				assert.Equal(t, string(want), string(got))
			}
		})
	}
}
