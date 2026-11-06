package sharedjson

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/arjenschwarz/orbit/internal/sessions"
	"github.com/arjenschwarz/orbit/internal/transcript"
)

func parseExecFile(t *testing.T, name string) *transcript.ParseResult {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "transcript", "testdata", "codex_exec", name))
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })
	res, err := transcript.ParseCodexExec(f)
	require.NoError(t, err)
	return res
}

func TestSession_CodexExecGolden(t *testing.T) {
	schemas := loadSchemas(t)
	got := encode(t, Session(parseExecFile(t, "all_items.jsonl"), SessionContext{}))

	goldenPath := filepath.Join("testdata", "golden", "codex-exec.json")
	if *update {
		require.NoError(t, os.WriteFile(goldenPath, got, 0o600))
	}
	want, err := os.ReadFile(goldenPath)
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got))
	requireValid(t, schemas, decodeJSON(t, string(got)))
}

func TestSession_CodexExecFields(t *testing.T) {
	out := string(encode(t, Session(parseExecFile(t, "all_items.jsonl"), SessionContext{})))
	doc := decodeJSON(t, out).(map[string]any)

	session := doc["session"].(map[string]any)
	assert.Equal(t, map[string]any{"source": "codex-exec", "thread_id": "0199aaaa-0000-7000-8000-000000000001"}, session,
		"no origin, creation time, model or provider: streams record none")

	entries := doc["entries"].([]any)
	require.Len(t, entries, 1)
	entry := entries[0].(map[string]any)
	assert.Equal(t, json.Number("1"), entry["turn"])
	assert.NotContains(t, entry, "id", "turn entries are built from several events (Decision 32)")
	assert.NotContains(t, entry, "timestamp")

	blocks := entry["content"].([]any)
	byItem := func(itemID, blockType string) map[string]any {
		t.Helper()
		for _, b := range blocks {
			m := b.(map[string]any)
			if m["item_id"] == itemID && m["type"] == blockType {
				return m
			}
		}
		t.Fatalf("no %s block for %s", blockType, itemID)
		return nil
	}

	assert.Equal(t, map[string]any{
		"type": "tool_result", "tool_use_id": "t1:item_1", "item_id": "item_1", "status": "completed",
		"content": "ok\n", "is_error": false, "exit_code": json.Number("0"),
	}, byItem("item_1", "tool_result"), "exit code 0 is emitted")
	assert.NotContains(t, byItem("item_3", "tool_result"), "exit_code", "an unrecorded exit code is omitted")
	assert.Equal(t, map[string]any{
		"type": "tool_use", "id": "t1:item_1", "item_id": "item_1", "name": "command_execution",
		"input": map[string]any{"command": "/bin/sh -lc 'make test'"},
	}, byItem("item_1", "tool_use"))

	assert.Equal(t, map[string]any{
		"type": "plan", "item_id": "item_8", "incomplete": true,
		"items": []any{
			map[string]any{"text": "Run tests", "completed": true},
			map[string]any{"text": "Report", "completed": false},
		},
	}, byItem("item_8", "plan"))
	assert.Equal(t, map[string]any{"type": "error", "item_id": "item_9", "message": "command output was truncated"}, byItem("item_9", "error"))

	event := byItem("item_10", "event")
	assert.Equal(t, "collab_tool_call", event["event_type"])
	assert.Equal(t, "Review <script>alert(1)</script> output", event["payload"].(map[string]any)["prompt"])

	usage := blocks[len(blocks)-1].(map[string]any)
	assert.Equal(t, "usage", usage["type"])
	assert.Equal(t, "thread_total_at_turn_completion", usage["basis"])
	assert.Contains(t, out, `"future_counter": 123456789012345678901234567890`, "counters keep their precision")
	order := []string{`"input_tokens"`, `"cached_input_tokens"`, `"cache_write_input_tokens"`, `"output_tokens"`, `"reasoning_output_tokens"`, `"future_counter"`}
	last := -1
	for _, key := range order {
		i := strings.Index(out, key)
		assert.Greater(t, i, last, "counters keep source key order: %s", key)
		last = i
	}
}

func TestSession_CodexExecFixturesValidate(t *testing.T) {
	schemas := loadSchemas(t)
	codes := publishedWarningCodes(t)
	for _, name := range []string{"all_items.jsonl", "turns.jsonl", "failures.jsonl", "pre_thread_failure.jsonl"} {
		t.Run(name, func(t *testing.T) {
			doc := Session(parseExecFile(t, name), SessionContext{})
			requireValid(t, schemas, decodeJSON(t, string(encode(t, doc))))
			for _, w := range doc.Warnings {
				assert.Contains(t, codes, w.Code)
				assert.Positive(t, w.Line)
			}
		})
	}
}

func TestListing_CodexExecRowValidates(t *testing.T) {
	schemas := loadSchemas(t)
	modified := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	doc := Listing([]sessions.SessionInfo{{
		ID: "0199aaaa-0000-7000-8000-000000000001-3f2a", Source: sessions.SourceCodexExec, ThreadID: "0199aaaa-0000-7000-8000-000000000001",
		CreatedAt: modified, TimeFromFilesystem: true, Size: 5120, Project: "/work/example-repo",
	}}, []sessions.ListWarning{{Source: sessions.SourceCodexExec, Code: sessions.WarningMissingThread, Err: errors.New("stream omitted")}},
		Scope{AllProjects: true})

	out := encode(t, doc)
	requireValid(t, schemas, decodeJSON(t, string(out)))
	row := doc.Sessions[0]
	assert.Equal(t, "0199aaaa-0000-7000-8000-000000000001", row.ThreadID)
	assert.Equal(t, "2026-10-01T09:00:00Z", row.FileModifiedAt)
	assert.Empty(t, row.CreatedAt)
}
