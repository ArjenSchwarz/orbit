package transcript

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

const execThreadLine = `{"type":"thread.started","thread_id":"thread-1"}` + "\n"

func parseExecString(t *testing.T, input string) *ParseResult {
	t.Helper()
	res, err := ParseCodexExec(strings.NewReader(input))
	require.NoError(t, err)
	require.NotNil(t, res)
	return res
}

func parseExecFixture(t *testing.T, name string) *ParseResult {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "codex_exec", name))
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })
	res, err := ParseCodexExec(f)
	require.NoError(t, err)
	return res
}

func intPtr(v int) *int { return &v }

// execContent returns the content of the only entry, asserting its turn.
func execContent(t *testing.T, res *ParseResult, turn int) []ContentItem {
	t.Helper()
	require.Len(t, res.Entries, 1)
	e := res.Entries[0]
	assert.Equal(t, turn, e.Turn)
	assert.Equal(t, "assistant", e.Type)
	require.NotNil(t, e.Message)
	assert.Equal(t, "assistant", e.Message.Role)
	return e.Message.Content
}

func TestParseCodexExec_ItemTypes(t *testing.T) {
	tests := map[string]struct {
		item string
		want []ContentItem
	}{
		"agent message": {
			item: `{"id":"item_0","type":"agent_message","text":"Done."}`,
			want: []ContentItem{{Type: "text", ItemID: "item_0", Text: "Done."}},
		},
		"reasoning": {
			item: `{"id":"item_0","type":"reasoning","text":"Thinking it over"}`,
			want: []ContentItem{{Type: "thinking", ItemID: "item_0", Thinking: "Thinking it over"}},
		},
		"command exit 0": {
			item: `{"id":"item_0","type":"command_execution","command":"make test","aggregated_output":"ok\n","exit_code":0,"status":"completed"}`,
			want: []ContentItem{
				{Type: "tool_use", ID: "t1:item_0", ItemID: "item_0", Name: "command_execution", Input: map[string]any{"command": "make test"}},
				{Type: "tool_result", ToolUseID: "t1:item_0", ItemID: "item_0", Content: "ok\n", ExitCode: intPtr(0), Status: "completed"},
			},
		},
		"command nonzero exit": {
			item: `{"id":"item_0","type":"command_execution","command":"make lint","aggregated_output":"2 issues\n","exit_code":2,"status":"completed"}`,
			want: []ContentItem{
				{Type: "tool_use", ID: "t1:item_0", ItemID: "item_0", Name: "command_execution", Input: map[string]any{"command": "make lint"}},
				{Type: "tool_result", ToolUseID: "t1:item_0", ItemID: "item_0", Content: "2 issues\n", ExitCode: intPtr(2), Status: "completed", IsError: true},
			},
		},
		"command failed": {
			item: `{"id":"item_0","type":"command_execution","command":"false","aggregated_output":"","exit_code":1,"status":"failed"}`,
			want: []ContentItem{
				{Type: "tool_use", ID: "t1:item_0", ItemID: "item_0", Name: "command_execution", Input: map[string]any{"command": "false"}},
				{Type: "tool_result", ToolUseID: "t1:item_0", ItemID: "item_0", ExitCode: intPtr(1), Status: "failed", IsError: true},
			},
		},
		"command declined without exit code": {
			item: `{"id":"item_0","type":"command_execution","command":"rm -rf build","aggregated_output":"","exit_code":null,"status":"declined"}`,
			want: []ContentItem{
				{Type: "tool_use", ID: "t1:item_0", ItemID: "item_0", Name: "command_execution", Input: map[string]any{"command": "rm -rf build"}},
				{Type: "tool_result", ToolUseID: "t1:item_0", ItemID: "item_0", Status: "declined", IsError: true},
			},
		},
		"file change": {
			item: `{"id":"item_0","type":"file_change","changes":[{"path":"a.go","kind":"update"},{"path":"b.md","kind":"add"}],"status":"completed"}`,
			want: []ContentItem{
				{Type: "tool_use", ID: "t1:item_0", ItemID: "item_0", Name: "file_change", Input: map[string]any{"changes": []any{
					map[string]any{"path": "a.go", "kind": "update"},
					map[string]any{"path": "b.md", "kind": "add"},
				}}},
				{Type: "tool_result", ToolUseID: "t1:item_0", ItemID: "item_0", Content: "update a.go\nadd b.md", Status: "completed"},
			},
		},
		"file change failed": {
			item: `{"id":"item_0","type":"file_change","changes":[{"path":"a.go","kind":"delete"}],"status":"failed"}`,
			want: []ContentItem{
				{Type: "tool_use", ID: "t1:item_0", ItemID: "item_0", Name: "file_change", Input: map[string]any{"changes": []any{
					map[string]any{"path": "a.go", "kind": "delete"},
				}}},
				{Type: "tool_result", ToolUseID: "t1:item_0", ItemID: "item_0", Content: "delete a.go", Status: "failed", IsError: true},
			},
		},
		"mcp result": {
			item: `{"id":"item_0","type":"mcp_tool_call","server":"docs","tool":"search","arguments":{"query":"q","limit":3},"result":{"content":[{"type":"text","text":"hit"}],"structured_content":null},"error":null,"status":"completed"}`,
			want: []ContentItem{
				{Type: "tool_use", ID: "t1:item_0", ItemID: "item_0", Name: "mcp_tool_call", Input: map[string]any{
					"server": "docs", "tool": "search", "arguments": map[string]any{"query": "q", "limit": json.Number("3")},
				}},
				{Type: "tool_result", ToolUseID: "t1:item_0", ItemID: "item_0", Content: `{"content":[{"type":"text","text":"hit"}],"structured_content":null}`, Status: "completed"},
			},
		},
		"mcp error": {
			item: `{"id":"item_0","type":"mcp_tool_call","server":"docs","tool":"fetch","arguments":{},"result":null,"error":{"message":"server unavailable"},"status":"failed"}`,
			want: []ContentItem{
				{Type: "tool_use", ID: "t1:item_0", ItemID: "item_0", Name: "mcp_tool_call", Input: map[string]any{
					"server": "docs", "tool": "fetch", "arguments": map[string]any{},
				}},
				{Type: "tool_result", ToolUseID: "t1:item_0", ItemID: "item_0", Content: "server unavailable", Status: "failed", IsError: true},
			},
		},
		"web search with two id keys keeps the last": {
			item: `{"id":"item_0","type":"web_search","id":"ws_1","query":"go json","action":{"type":"search","query":"go json"},"results":[{"url":"https://example.com"}]}`,
			want: []ContentItem{
				{Type: "tool_use", ID: "t1:ws_1", ItemID: "ws_1", Name: "web_search", Input: map[string]any{
					"query": "go json", "action": map[string]any{"type": "search", "query": "go json"},
				}},
				{Type: "tool_result", ToolUseID: "t1:ws_1", ItemID: "ws_1", Content: `[{"url":"https://example.com"}]`},
			},
		},
		"todo list": {
			item: `{"id":"item_0","type":"todo_list","items":[{"text":"Run tests","completed":true},{"text":"Report","completed":false}]}`,
			want: []ContentItem{{Type: "plan", ItemID: "item_0", Plan: []PlanItem{{Text: "Run tests", Completed: true}, {Text: "Report"}}}},
		},
		"error item": {
			item: `{"id":"item_0","type":"error","message":"output truncated"}`,
			want: []ContentItem{{Type: "error", ItemID: "item_0", Text: "output truncated"}},
		},
		"collab tool call is a generic event": {
			item: `{"id":"item_0","type":"collab_tool_call","tool":"wait", "receiver_thread_ids":[],"status":"completed"}`,
			want: []ContentItem{{Type: "event", ItemID: "item_0", EventType: "collab_tool_call",
				Payload: json.RawMessage(`{"id":"item_0","type":"collab_tool_call","tool":"wait","receiver_thread_ids":[],"status":"completed"}`)}},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			input := execThreadLine + `{"type":"turn.started"}` + "\n" +
				`{"type":"item.completed","item":` + tc.item + "}\n"
			res := parseExecString(t, input)
			assert.Empty(t, res.Warnings)
			assert.Equal(t, tc.want, execContent(t, res, 1))
		})
	}
}

func TestParseCodexExec_AllItemsFixture(t *testing.T) {
	res := parseExecFixture(t, "all_items.jsonl")

	assert.Equal(t, FormatCodexExec, res.Format)
	require.NotNil(t, res.Metadata)
	assert.Equal(t, "0199aaaa-0000-7000-8000-000000000001", res.Metadata.ThreadID)
	assert.Empty(t, res.Warnings)

	content := execContent(t, res, 1)
	var kinds []string
	for _, c := range content {
		kinds = append(kinds, c.Type)
	}
	assert.Equal(t, []string{
		"thinking",
		"tool_use", "tool_result", // item_1
		"tool_use", "tool_result", // item_2
		"tool_use", "tool_result", // item_3
		"tool_use", "tool_result", // item_4
		"tool_use", "tool_result", // item_5
		"tool_use", "tool_result", // item_6
		"tool_use", "tool_result", // web search
		"plan", "error", "event", "text", "usage",
	}, kinds)

	// The todo list was started and updated but never completed.
	plan := content[15]
	assert.True(t, plan.Incomplete)
	assert.Equal(t, []PlanItem{{Text: "Run tests", Completed: true}, {Text: "Report"}}, plan.Plan)

	// The web search's last payload replaced its started state.
	assert.Equal(t, "go json number precision", content[13].Input.(map[string]any)["query"])
	assert.False(t, content[13].Incomplete)
}

func TestParseCodexExec_TurnsAndLifecycle(t *testing.T) {
	res := parseExecFixture(t, "turns.jsonl")

	assert.Empty(t, res.Warnings, "a repeated thread.started with the same ID is dropped silently")
	assert.Equal(t, "0199aaaa-0000-7000-8000-000000000002", res.Metadata.ThreadID)
	require.Len(t, res.Entries, 2)

	first := res.Entries[0]
	assert.Equal(t, 1, first.Turn)
	ids := []string{}
	for _, c := range first.Message.Content {
		if c.Type == "tool_use" {
			ids = append(ids, c.ID)
		}
	}
	assert.Equal(t, []string{"t1:item_0", "t1:item_1"}, ids, "items keep first-appearance order, not completion order")

	second := res.Entries[1]
	assert.Equal(t, 2, second.Turn, "turn numbering continues after a repeated thread.started")
	require.Len(t, second.Message.Content, 2)
	use, result := second.Message.Content[0], second.Message.Content[1]
	assert.Equal(t, "t2:item_0", use.ID)
	assert.Equal(t, "item_0", use.ItemID)
	assert.True(t, use.Incomplete)
	assert.Equal(t, "t2:item_0", result.ToolUseID)
	assert.True(t, result.Incomplete)
	assert.Equal(t, "compiling\n", result.Content, "the last update is kept")
	assert.Equal(t, "in_progress", result.Status)
	assert.Nil(t, result.ExitCode)

	for _, e := range res.Entries {
		assert.Empty(t, e.Timestamp)
		assert.Empty(t, e.Model)
		assert.Empty(t, e.Provider)
		assert.Empty(t, e.RecordID)
	}
}

func TestParseCodexExec_Failures(t *testing.T) {
	res := parseExecFixture(t, "failures.jsonl")

	assert.Equal(t, "0199aaaa-0000-7000-8000-000000000003", res.Metadata.ThreadID, "a different later thread ID does not replace the first")
	require.Len(t, res.Warnings, 2)

	malformed := res.Warnings[0]
	assert.Equal(t, 4, malformed.Line)
	assert.Equal(t, WarningMalformedLine, malformed.Code)
	assert.Regexp(t, `^malformed JSON event \(offset \d+\)$`, malformed.Message)
	assert.NotContains(t, malformed.Message, "broken")

	mismatch := res.Warnings[1]
	assert.Equal(t, 8, mismatch.Line)
	assert.Equal(t, WarningThreadMismatch, mismatch.Code)
	assert.NotContains(t, mismatch.Message, "00000000000f", "warnings do not copy source values")

	content := execContent(t, res, 1)
	require.Len(t, content, 5)
	assert.Equal(t, ContentItem{Type: "text", ItemID: "item_0", Text: "Starting."}, content[0])
	assert.Equal(t, ContentItem{Type: "event", EventType: "session.compacted",
		Payload: json.RawMessage(`{"type":"session.compacted","reason":"context window"}`)}, content[1])
	assert.Equal(t, ContentItem{Type: "error", Text: "stream disconnected; retrying",
		Payload: json.RawMessage(`{"type":"error","message":"stream disconnected; retrying"}`)}, content[2])
	assert.Equal(t, ContentItem{Type: "error", Text: "model request failed",
		Payload: json.RawMessage(`{"type":"turn.failed","error":{"message":"model request failed"}}`)}, content[3])
	assert.Equal(t, ContentItem{Type: "event", EventType: "thread.started",
		Payload: json.RawMessage(`{"type":"thread.started","thread_id":"0199aaaa-0000-7000-8000-00000000000f"}`)}, content[4])
}

func TestParseCodexExec_Usage(t *testing.T) {
	input := execThreadLine +
		`{"type":"turn.started"}` + "\n" +
		`{"type":"turn.completed","usage":{"output_tokens":10,"input_tokens":100,"future_counter":123456789012345678901234567890,"ratio":0.5}}` + "\n" +
		`{"type":"turn.started"}` + "\n" +
		`{"type":"turn.completed","usage":{"input_tokens":250,"output_tokens":30}}` + "\n"
	res := parseExecString(t, input)
	require.Len(t, res.Entries, 2)

	first := res.Entries[0].Message.Content
	require.Len(t, first, 1)
	assert.Equal(t, "usage", first[0].Type)
	assert.Equal(t, &Usage{
		Basis: UsageBasisThreadTotal,
		Counters: []UsageCounter{
			{Name: "output_tokens", Value: json.RawMessage("10")},
			{Name: "input_tokens", Value: json.RawMessage("100")},
			{Name: "future_counter", Value: json.RawMessage("123456789012345678901234567890")},
			{Name: "ratio", Value: json.RawMessage("0.5")},
		},
	}, first[0].Usage)

	second := res.Entries[1].Message.Content
	require.Len(t, second, 1)
	assert.Equal(t, []UsageCounter{
		{Name: "input_tokens", Value: json.RawMessage("250")},
		{Name: "output_tokens", Value: json.RawMessage("30")},
	}, second[0].Usage.Counters, "each turn keeps its own reported totals; nothing is summed")
}

func TestParseCodexExec_InvalidItems(t *testing.T) {
	input := execThreadLine + `{"type":"turn.started"}` + "\n" +
		`{"type":"item.started","item":{"type":"command_execution","command":"a","aggregated_output":"","status":"in_progress"}}` + "\n" + // line 3: missing id
		`{"type":"item.completed","item":{"type":"command_execution","command":"a","aggregated_output":"x","exit_code":0,"status":"completed"}}` + "\n" + // line 4: missing id
		`{"type":"item.completed","item":{"id":"","type":"agent_message","text":"empty id"}}` + "\n" + // line 5
		`{"type":"item.completed","item":{"id":7,"type":"reasoning","text":"numeric id"}}` + "\n" + // line 6
		`{"type":"item.completed","item":{"id":"item_9","type":"command_execution","command":42,"status":"completed"}}` + "\n" + // line 7: invalid typed field
		`{"type":"item.completed","item":{"id":"item_10","type":"todo_list","items":"none"}}` + "\n" // line 8
	res := parseExecString(t, input)

	content := execContent(t, res, 1)
	require.Len(t, content, 6, "missing IDs key by physical line, so the two command items stay distinct")
	for i, c := range content {
		assert.Equal(t, "event", c.Type, "block %d", i)
		assert.Empty(t, c.ID, "no tool link for block %d", i)
		assert.Empty(t, c.ToolUseID, "no tool link for block %d", i)
	}
	assert.Equal(t, "command_execution", content[0].EventType)
	assert.JSONEq(t, `{"type":"command_execution","command":"a","aggregated_output":"","status":"in_progress"}`, string(content[0].Payload))
	assert.True(t, content[0].Incomplete)
	assert.False(t, content[1].Incomplete)
	assert.Equal(t, "item_9", content[4].ItemID)
	assert.Equal(t, "todo_list", content[5].EventType)

	var lines []int
	for _, w := range res.Warnings {
		assert.Equal(t, WarningItemDecodeFailed, w.Code)
		lines = append(lines, w.Line)
	}
	assert.Equal(t, []int{3, 4, 5, 6, 7, 8}, lines)
}

func TestParseCodexExec_OversizedLine(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(execThreadLine)
	sb.WriteString(`{"type":"turn.started"}` + "\n")
	sb.WriteString(`{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"`)
	sb.WriteString(strings.Repeat("x", 10*1024*1024+1))
	sb.WriteString(`"}}` + "\n")
	sb.WriteString(`{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"after"}}` + "\n")

	res := parseExecString(t, sb.String())
	require.Len(t, res.Warnings, 1)
	assert.Equal(t, ParseWarning{Line: 3, Code: WarningOversizedLine, Message: "line exceeds 10 MiB; skipped"}, res.Warnings[0])
	assert.Equal(t, []ContentItem{{Type: "text", ItemID: "item_1", Text: "after"}}, execContent(t, res, 1))
}

type failingReader struct{ data *strings.Reader }

func (r failingReader) Read(p []byte) (int, error) {
	if r.data.Len() == 0 {
		return 0, errors.New("disk failure")
	}
	return r.data.Read(p)
}

func TestParseCodexExec_ReaderErrorIsFatal(t *testing.T) {
	_, err := ParseCodexExec(failingReader{strings.NewReader(execThreadLine)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk failure")
}

func TestParseCodexExec_WithoutThread(t *testing.T) {
	tests := map[string]struct {
		input string
		want  []ContentItem
		codes []string
	}{
		"failure before thread creation": {
			input: `{"type":"error","message":"model provider is not reachable"}` + "\n",
			want: []ContentItem{{Type: "error", Text: "model provider is not reachable",
				Payload: json.RawMessage(`{"type":"error","message":"model provider is not reachable"}`)}},
		},
		"thread.started without a thread ID": {
			input: `{"type":"thread.started","thread_id":""}` + "\n",
			want:  []ContentItem{{Type: "event", EventType: "thread.started", Payload: json.RawMessage(`{"type":"thread.started","thread_id":""}`)}},
			codes: []string{WarningInvalidRecord},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			res := parseExecString(t, tc.input)
			assert.Empty(t, res.Metadata.ThreadID)
			assert.Equal(t, tc.want, execContent(t, res, 0))
			var codes []string
			for _, w := range res.Warnings {
				codes = append(codes, w.Code)
			}
			assert.Equal(t, tc.codes, codes)
		})
	}

	t.Run("fixture", func(t *testing.T) {
		res := parseExecFixture(t, "pre_thread_failure.jsonl")
		assert.Empty(t, res.Metadata.ThreadID)
		require.Len(t, res.Entries, 1)
		assert.Equal(t, "error", res.Entries[0].Message.Content[0].Type)
	})
}

func TestParseCodexExec_EmptyStream(t *testing.T) {
	for name, input := range map[string]string{"empty": "", "blank lines": "\n\n  \n", "bom only": "\xef\xbb\xbf"} {
		t.Run(name, func(t *testing.T) {
			res := parseExecString(t, input)
			assert.Empty(t, res.Entries)
			assert.NotNil(t, res.Entries)
			assert.Empty(t, res.Warnings)
			assert.Equal(t, FormatCodexExec, res.Format)
		})
	}
}

func TestParseCodexExec_LeadingBOM(t *testing.T) {
	res := parseExecString(t, "\xef\xbb\xbf"+execThreadLine)
	assert.Equal(t, "thread-1", res.Metadata.ThreadID)
	assert.Empty(t, res.Warnings)
}

func TestParseJSONLWithFormat_CodexExec(t *testing.T) {
	res, err := ParseJSONLWithFormat(strings.NewReader(execThreadLine), FormatCodexExec)
	require.NoError(t, err)
	assert.Equal(t, FormatCodexExec, res.Format)
	assert.Equal(t, "thread-1", res.Metadata.ThreadID)
}

// TestPropertyCodexExecLifecycle checks that random lifecycle sequences give
// one record per (turn, id) in first-appearance order with the last payload,
// incomplete exactly when it never completed.
func TestPropertyCodexExecLifecycle(t *testing.T) {
	type record struct {
		id        string
		text      string
		completed bool
	}
	rapid.Check(t, func(rt *rapid.T) {
		var buf bytes.Buffer
		buf.WriteString(execThreadLine)

		turn := 0
		turns := map[int][]*record{}
		index := map[string]*record{}
		ops := rapid.IntRange(0, 40).Draw(rt, "ops")
		for n := range ops {
			if rapid.IntRange(0, 9).Draw(rt, "new turn") == 0 {
				turn++
				buf.WriteString(`{"type":"turn.started"}` + "\n")
				continue
			}
			id := fmt.Sprintf("item_%d", rapid.IntRange(0, 3).Draw(rt, "id"))
			event := rapid.SampledFrom([]string{"item.started", "item.updated", "item.completed"}).Draw(rt, "event")
			text := fmt.Sprintf("v%d", n)
			fmt.Fprintf(&buf, `{"type":%q,"item":{"id":%q,"type":"agent_message","text":%q}}`+"\n", event, id, text)

			key := fmt.Sprintf("%d/%s", turn, id)
			r, ok := index[key]
			if !ok {
				r = &record{id: id}
				index[key] = r
				turns[turn] = append(turns[turn], r)
			}
			r.text = text
			if event == "item.completed" {
				r.completed = true
			}
		}

		res, err := ParseCodexExec(&buf)
		if err != nil {
			rt.Fatalf("parse: %v", err)
		}
		var wantTurns []int
		for tn := 0; tn <= turn; tn++ {
			if len(turns[tn]) > 0 {
				wantTurns = append(wantTurns, tn)
			}
		}
		if len(res.Entries) != len(wantTurns) {
			rt.Fatalf("entries = %d, want %d", len(res.Entries), len(wantTurns))
		}
		for i, tn := range wantTurns {
			e := res.Entries[i]
			if e.Turn != tn {
				rt.Fatalf("entry %d turn = %d, want %d", i, e.Turn, tn)
			}
			want := turns[tn]
			if len(e.Message.Content) != len(want) {
				rt.Fatalf("turn %d blocks = %d, want %d", tn, len(e.Message.Content), len(want))
			}
			for j, r := range want {
				got := e.Message.Content[j]
				if got.ItemID != r.id || got.Text != r.text || got.Incomplete != !r.completed {
					rt.Fatalf("turn %d block %d = %+v, want id %s text %s completed %v", tn, j, got, r.id, r.text, r.completed)
				}
			}
		}
	})
}
