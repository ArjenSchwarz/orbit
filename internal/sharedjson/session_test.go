package sharedjson

import (
	"bytes"
	"encoding/json"
	"flag"
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

var update = flag.Bool("update", false, "rewrite golden session documents")

// sharedJSONOptions are the parse options shared-JSON callers use.
var sharedJSONOptions = transcript.ParseOptions{AllowEmptyCodex: true, PreserveModelChanges: true}

func encode(t *testing.T, doc any) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, Write(&buf, doc))
	return buf.Bytes()
}

func parseFile(t *testing.T, path string, opts transcript.ParseOptions) *transcript.ParseResult {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })
	result, err := transcript.Parse(f, opts)
	require.NoError(t, err)
	return result
}

// One golden per existing source. Inputs are synthetic: transcript package
// fixtures, plus local inputs that add origin, record identifiers, cost and
// warnings in each position unit.
func TestSession_Goldens(t *testing.T) {
	schemas := loadSchemas(t)
	codes := publishedWarningCodes(t)
	for name, tc := range map[string]struct {
		input    string
		costPath string
		id       string
	}{
		"claude":           {input: "testdata/input/claude.jsonl", id: "claude"},
		"codex":            {input: "../transcript/testdata/codex/turn_context.jsonl", id: "turn_context"},
		"copilot":          {input: "../transcript/testdata/integration/copilot.jsonl", id: "copilot"},
		"kiro-cli":         {input: "../transcript/testdata/integration/kiro.json", id: "kiro"},
		"kiro-ide-chat":    {input: "testdata/input/kiro_ide.chat", id: "kiro_ide"},
		"kiro-ide-actions": {input: "testdata/input/kiro_ide.chat", costPath: "testdata/input/kiro_ide_execution.json", id: "kiro_ide"},
	} {
		t.Run(name, func(t *testing.T) {
			opts := sharedJSONOptions
			opts.KiroIDECostPath = tc.costPath
			doc := Session(parseFile(t, tc.input, opts), SessionContext{ID: tc.id})
			got := encode(t, doc)

			goldenPath := filepath.Join("testdata", "golden", name+".json")
			if *update {
				require.NoError(t, os.WriteFile(goldenPath, got, 0o600))
			}
			want, err := os.ReadFile(goldenPath)
			require.NoError(t, err)
			assert.Equal(t, string(want), string(got))

			requireValid(t, schemas, decodeJSON(t, string(got)))
			for _, w := range doc.Warnings {
				assert.Contains(t, codes, w.Code, "every projected warning carries a published code")
			}
		})
	}
}

func TestSession_GoldenDetails(t *testing.T) {
	load := func(t *testing.T, name string) SessionDocument {
		t.Helper()
		data, err := os.ReadFile(filepath.Join("testdata", "golden", name+".json"))
		require.NoError(t, err)
		var doc SessionDocument
		require.NoError(t, json.Unmarshal(data, &doc))
		return doc
	}
	ids := func(doc SessionDocument) []string {
		var out []string
		for _, e := range doc.Entries {
			out = append(out, e.ID)
		}
		return out
	}

	t.Run("claude origin, record ids and warning line", func(t *testing.T) {
		doc := load(t, "claude")
		assert.Equal(t, "sdk-cli", doc.Session.Origin)
		assert.Equal(t, "/work/example-repo", doc.Session.Project)
		assert.Equal(t, "2026-10-01T09:00:00.000Z", doc.Session.CreatedAt)
		assert.Len(t, doc.Entries, 4)
		assert.NotContains(t, ids(doc), "")
		require.Len(t, doc.Warnings, 1)
		assert.Equal(t, transcript.WarningMalformedLine, doc.Warnings[0].Code)
		assert.Equal(t, 3, doc.Warnings[0].Line)
		other := doc.Entries[3].Content[0].(map[string]any)
		assert.Equal(t, map[string]any{"type": "other", "source_type": "image"}, other)
	})

	t.Run("codex models, provider and origin", func(t *testing.T) {
		doc := load(t, "codex")
		assert.Equal(t, "example-model-a", doc.Session.Model)
		assert.Equal(t, "example-provider", doc.Session.Provider)
		assert.Equal(t, "codex_exec", doc.Session.Origin)
		var models []string
		for _, e := range doc.Entries {
			models = append(models, e.Model)
		}
		assert.Equal(t, []string{"", "example-model-a", "example-model-b"}, models)
		assert.Equal(t, []string{"msg_user_1", "", ""}, ids(doc), "assistant entries consolidate several items")
	})

	t.Run("copilot ids and no origin", func(t *testing.T) {
		doc := load(t, "copilot")
		assert.Empty(t, doc.Session.Origin)
		assert.Equal(t, "2026-03-10T09:00:00Z", doc.Session.CreatedAt)
		for _, e := range doc.Entries {
			first := e.Content[0].(map[string]any)["type"]
			if e.Role == "assistant" || first == "tool_result" {
				assert.Empty(t, e.ID, "assistant and tool-result entries merge several events")
			} else {
				assert.NotEmpty(t, e.ID, "user.message entries carry the event id")
			}
		}
	})

	t.Run("kiro ide chat and action ids", func(t *testing.T) {
		chat := load(t, "kiro-ide-chat")
		assert.Equal(t, []string{"", ""}, ids(chat))
		assert.Equal(t, "example-provider", chat.Session.Provider)
		assert.Empty(t, chat.Session.Origin)
		require.Len(t, chat.Warnings, 1)
		assert.Equal(t, 3, chat.Warnings[0].ChatMessage)
		assert.Zero(t, chat.Warnings[0].Line)

		actions := load(t, "kiro-ide-actions")
		assert.Equal(t, []string{"", "action-1", "action-2", "action-2"}, ids(actions), "a tool action's two entries share its id")
		assert.Equal(t, &Cost{Amount: 0.25, Unit: "credits"}, actions.Session.Cost)
		require.Len(t, actions.Warnings, 1)
		assert.Equal(t, 3, actions.Warnings[0].ExecutionAction)
		assert.Zero(t, actions.Warnings[0].Line)
	})
}

func TestSession_EmptyCodexKeepsHeaderMetadata(t *testing.T) {
	header := `{"timestamp":"2026-10-01T09:00:00Z","type":"session_meta","payload":{"id":"example","cwd":"/work/example","originator":"codex_cli_rs","model_provider":"example-provider"}}
{"timestamp":"2026-10-01T09:00:01Z","type":"turn_context","payload":{"model":"example-model"}}`
	result, err := transcript.Parse(strings.NewReader(header), sharedJSONOptions)
	require.NoError(t, err)
	doc := Session(result, SessionContext{})
	assert.Equal(t, Metadata{Source: "codex", CreatedAt: "2026-10-01T09:00:00Z", Project: "/work/example", Model: "example-model", Provider: "example-provider", Origin: "codex_cli_rs"}, doc.Session)

	got := string(encode(t, doc))
	assert.Contains(t, got, `"entries": []`)
	assert.Contains(t, got, `"warnings": []`)
	requireValid(t, loadSchemas(t), decodeJSON(t, got))
}

func TestSession_LongToolResultKeptExactly(t *testing.T) {
	long := strings.Repeat("résumé ", 500) // 3,500 runes, multi-byte
	result := &transcript.ParseResult{Entries: []transcript.Entry{{
		Type:    "user",
		Message: &transcript.Message{Role: "user", Content: []transcript.ContentItem{{Type: "tool_result", ToolUseID: "call-1", Content: long}}},
	}}}
	var decoded struct {
		Entries []struct {
			Content []ToolResultBlock `json:"content"`
		} `json:"entries"`
	}
	require.NoError(t, json.Unmarshal(encode(t, Session(result, SessionContext{})), &decoded))
	assert.Equal(t, long, decoded.Entries[0].Content[0].Content)
}

func TestSession_MetadataPrecedence(t *testing.T) {
	recorded := time.Date(2026, 10, 1, 8, 0, 0, 0, time.FixedZone("example", 3600))
	entries := []transcript.Entry{
		{Type: "user", Timestamp: "not-a-time", Message: &transcript.Message{Role: "user"}},
		{Type: "assistant", Timestamp: "2026-10-01T09:30:00Z", Model: "entry-model", Provider: "entry-provider"},
	}
	for name, tc := range map[string]struct {
		format   transcript.Format
		metadata *transcript.ParseResultMetadata
		ctx      SessionContext
		want     Metadata
	}{
		"parsed start record wins": {
			format:   transcript.FormatCopilot,
			metadata: &transcript.ParseResultMetadata{StartedAt: "2026-10-01T07:00:00Z", Project: "/work/parsed"},
			ctx:      SessionContext{Resolved: &sessions.SessionMetadata{Source: "copilot", ID: "resolved-id", CreatedAt: recorded, Project: "/work/resolved"}},
			want:     Metadata{Source: "copilot", ID: "resolved-id", CreatedAt: "2026-10-01T07:00:00Z", Project: "/work/parsed", Model: "entry-model", Provider: "entry-provider"},
		},
		"recorded resolver time fills a missing start": {
			format:   transcript.FormatKiro,
			metadata: &transcript.ParseResultMetadata{StartedAt: "invalid"},
			ctx:      SessionContext{Source: "kiro-cli", ID: "listed-id", Resolved: &sessions.SessionMetadata{Source: "other", ID: "resolved-id", CreatedAt: recorded, Project: "/work/key"}},
			want:     Metadata{Source: "kiro-cli", ID: "listed-id", CreatedAt: "2026-10-01T07:00:00Z", Project: "/work/key", Model: "entry-model", Provider: "entry-provider"},
		},
		"filesystem time never becomes created_at": {
			format: transcript.FormatKiro,
			ctx:    SessionContext{Resolved: &sessions.SessionMetadata{CreatedAt: recorded, TimeFromFilesystem: true}},
			want:   Metadata{Source: "kiro-cli", CreatedAt: "2026-10-01T09:30:00Z", Model: "entry-model", Provider: "entry-provider"},
		},
		"parsed summaries win over entries": {
			format:   transcript.FormatCodex,
			metadata: &transcript.ParseResultMetadata{FirstModel: "first-model", FirstProvider: "first-provider", Origin: "codex_exec"},
			want:     Metadata{Source: "codex", CreatedAt: "2026-10-01T09:30:00Z", Model: "first-model", Provider: "first-provider", Origin: "codex_exec"},
		},
		"claude fallback has no source": {
			format: transcript.FormatUnknown,
			want:   Metadata{CreatedAt: "2026-10-01T09:30:00Z", Model: "entry-model", Provider: "entry-provider"},
		},
		"cost with and without unit": {
			format:   transcript.FormatKiroIDE,
			metadata: &transcript.ParseResultMetadata{TotalCost: ptr(0.5)},
			want:     Metadata{Source: "kiro-ide", CreatedAt: "2026-10-01T09:30:00Z", Model: "entry-model", Provider: "entry-provider", Cost: &Cost{Amount: 0.5, Unit: "credits"}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			result := &transcript.ParseResult{Entries: entries, Metadata: tc.metadata, Format: tc.format}
			assert.Equal(t, tc.want, Session(result, tc.ctx).Session)
		})
	}

	cost := &transcript.ParseResult{Metadata: &transcript.ParseResultMetadata{TotalCost: ptr(2.0), CostUnit: "premium requests"}}
	assert.Equal(t, &Cost{Amount: 2, Unit: "premium requests"}, Session(cost, SessionContext{}).Session.Cost)
}

func TestSession_EntriesAndBlocks(t *testing.T) {
	result := &transcript.ParseResult{
		Entries: []transcript.Entry{
			{Type: "assistant", RecordID: "record-1", Timestamp: "invalid", Message: &transcript.Message{Content: []transcript.ContentItem{
				{Type: "tool_use", ID: "call-1", Name: "Bash", Input: map[string]any{"command": "ls"}},
				{Type: "redacted_thinking", Text: "hidden"},
			}}},
			{Type: "summary"},
		},
		Warnings: []transcript.ParseWarning{{Line: 7, Code: transcript.WarningInvalidRecord, Message: "bad", Unit: "future-unit"}},
	}
	doc := Session(result, SessionContext{})
	assert.Equal(t, []Entry{
		{ID: "record-1", Role: "assistant", Content: []any{
			ToolUseBlock{Type: "tool_use", ID: "call-1", Name: "Bash", Input: map[string]any{"command": "ls"}},
			OtherBlock{Type: "other", SourceType: "redacted_thinking", Text: "hidden"},
		}},
		{Role: "summary", Content: []any{}},
	}, doc.Entries)
	assert.Equal(t, []Warning{{Code: transcript.WarningInvalidRecord, Message: "bad", Line: 7}}, doc.Warnings)
}

func TestWrite_Format(t *testing.T) {
	doc := map[string]any{"text": "<a & b>", "list": []int{1}}
	got := string(encode(t, doc))
	assert.Equal(t, "{\n  \"list\": [\n    1\n  ],\n  \"text\": \"<a & b>\"\n}\n", got)
}

func ptr(f float64) *float64 { return &f }
