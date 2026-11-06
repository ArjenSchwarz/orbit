package transcript

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMetadata_Formats(t *testing.T) {
	for _, tc := range []struct {
		fixture string
		format  Format
	}{
		{"claude.jsonl", FormatClaude},
		{"codex.jsonl", FormatCodex},
		{"copilot.jsonl", FormatCopilot},
		{"kiro.json", FormatKiro},
		{"kiro_ide.json", FormatKiroIDE},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", "integration", tc.fixture))
			require.NoError(t, err)
			result, err := Parse(strings.NewReader(string(data)))
			require.NoError(t, err)
			assert.Equal(t, tc.format, result.Format)
			result, err = ParseJSONLWithFormat(strings.NewReader(string(data)), tc.format)
			require.NoError(t, err)
			assert.Equal(t, tc.format, result.Format)
			if tc.format == FormatClaude || tc.format == FormatCodex {
				result, err = ParseJSONL(strings.NewReader(string(data)))
				require.NoError(t, err)
				assert.Equal(t, tc.format, result.Format)
			}
		})
	}
	result, err := Parse(strings.NewReader(`{"type":"unknown","timestamp":"2026-10-01T00:00:00Z"}`))
	require.NoError(t, err)
	assert.Equal(t, FormatUnknown, result.Format)
	assert.Empty(t, result.Entries)
}

func TestParseMetadata_Claude(t *testing.T) {
	input := `{"type":"queue-operation","timestamp":"invalid","cwd":"/work/first"}
{"type":"user","timestamp":"2026-10-01T01:00:00.123Z","cwd":"/work/later","message":{"role":"user","content":"Hello"}}
{"type":"assistant","timestamp":"2026-10-01T02:00:00Z","message":{"role":"assistant","model":"unextracted-model","content":"Hi"}}`
	result, err := Parse(strings.NewReader(input))
	require.NoError(t, err)
	require.NotNil(t, result.Metadata)
	assert.Equal(t, "2026-10-01T01:00:00.123Z", result.Metadata.StartedAt)
	assert.Equal(t, "/work/first", result.Metadata.Project)
	assert.Empty(t, result.Metadata.FirstModel)
	assert.Empty(t, result.Metadata.FirstProvider)
	assert.Nil(t, result.Metadata.TotalCost)
	for _, entry := range result.Entries {
		assert.Empty(t, entry.Model)
		assert.Empty(t, entry.Provider)
	}
}

func TestParseMetadata_CodexHeaderAndUserOnly(t *testing.T) {
	header := `{"timestamp":"2026-10-01T01:00:00Z","type":"session_meta","payload":{"id":"example-session","timestamp":"2026-09-01T01:00:00Z","cwd":"/work/example","model":"example-model","model_provider":"example-provider"}}`
	for _, mode := range []string{"direct", "auto", "forced"} {
		t.Run(mode, func(t *testing.T) {
			parse := func(input string, opts ...ParseOptions) (*ParseResult, error) {
				switch mode {
				case "auto":
					return Parse(strings.NewReader(input), opts...)
				case "forced":
					return ParseJSONLWithFormat(strings.NewReader(input), FormatCodex, opts...)
				default:
					return ParseCodexJSONL(strings.NewReader(input), opts...)
				}
			}
			_, err := parse(header)
			require.EqualError(t, err, "no valid entries found in file")
			result, err := parse(header, ParseOptions{AllowEmptyCodex: true})
			require.NoError(t, err)
			assert.NotNil(t, result.Entries)
			assert.Empty(t, result.Entries)
			assert.Equal(t, FormatCodex, result.Format)
			require.NotNil(t, result.Metadata)
			assert.Equal(t, "2026-10-01T01:00:00Z", result.Metadata.StartedAt)
			assert.Equal(t, "/work/example", result.Metadata.Project)
			assert.Equal(t, "example-model", result.Metadata.FirstModel)
			assert.Equal(t, "example-provider", result.Metadata.FirstProvider)
			assert.Nil(t, result.Metadata.TotalCost)

			result, err = parse(header + "\n" + `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Hello"}]}}`)
			require.NoError(t, err)
			require.Len(t, result.Entries, 1)
			assert.Equal(t, "user", result.Entries[0].Type)
			assert.Empty(t, result.Entries[0].Provider)
			assert.Equal(t, "example-provider", result.Metadata.FirstProvider)
			assert.Contains(t, RenderMarkdown(result.Entries, RenderOptions{}), "Hello")
			assert.NotContains(t, RenderMarkdown(result.Entries, RenderOptions{}), "example-provider")
		})
	}
	result, err := ParseCodexJSONL(strings.NewReader(`{"type":"session_meta","payload":{"timestamp":"2026-09-01T01:00:00Z"}}`), ParseOptions{AllowEmptyCodex: true})
	require.NoError(t, err)
	require.NotNil(t, result.Metadata)
	assert.Empty(t, result.Metadata.StartedAt, "payload timestamp is not an outer header timestamp")
}

func TestParseMetadata_CodexChanges(t *testing.T) {
	for _, tc := range []struct {
		name      string
		model     string
		provider  string
		preserve  bool
		wantCount int
	}{
		{"identical default", "first-model", "first-provider", false, 1},
		{"identical preserved", "first-model", "first-provider", true, 1},
		{"model default", "second-model", "", false, 1},
		{"model preserved", "second-model", "", true, 2},
		{"provider default", "first-model", "second-provider", false, 2},
		{"provider preserved", "first-model", "second-provider", true, 2},
		{"both default", "second-model", "second-provider", false, 2},
		{"both preserved", "second-model", "second-provider", true, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			firstProvider := "first-provider"
			if tc.provider == "" {
				firstProvider = ""
			}
			header := func(model, provider, timestamp string) string {
				return fmt.Sprintf(`{"timestamp":%q,"type":"session_meta","payload":{"id":"example-session","cwd":"/work/example","model":%q,"model_provider":%q}}`, timestamp, model, provider)
			}
			input := strings.Join([]string{
				header("first-model", firstProvider, "2026-10-01T00:00:00Z"),
				`{"timestamp":"2026-10-01T00:00:01Z","type":"response_item","payload":{"type":"function_call","name":"shell_command","arguments":"{\"command\":\"echo example\"}","call_id":"example-call"}}`,
				header(tc.model, tc.provider, "2026-10-01T01:00:00Z"),
				`{"timestamp":"2026-10-01T00:00:02Z","type":"response_item","payload":{"type":"function_call_output","call_id":"example-call","output":"example output"}}`,
				`{"timestamp":"2026-10-01T00:00:03Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"After"}]}}`,
			}, "\n")
			result, err := ParseCodexJSONL(strings.NewReader(input), ParseOptions{PreserveModelChanges: tc.preserve})
			require.NoError(t, err)
			require.Len(t, result.Entries, tc.wantCount)
			require.NotNil(t, result.Metadata)
			assert.Equal(t, "2026-10-01T00:00:00Z", result.Metadata.StartedAt)
			assert.Equal(t, "first-model", result.Metadata.FirstModel)
			assert.Equal(t, firstProvider, result.Metadata.FirstProvider)
			first := result.Entries[0]
			assert.Equal(t, "first-model", first.Model)
			assert.Equal(t, firstProvider, first.Provider)
			assert.Equal(t, "2026-10-01T00:00:01Z", first.Timestamp)
			assert.Empty(t, result.Warnings)
			// A function call output joins its call's entry even after a transition (Q82).
			require.GreaterOrEqual(t, len(first.Message.Content), 2)
			assert.Equal(t, "example-call", first.Message.Content[0].ID)
			assert.Equal(t, "example-call", first.Message.Content[1].ToolUseID)
			assert.Equal(t, "example output", first.Message.Content[1].Content)
			last := result.Entries[len(result.Entries)-1]
			after := last.Message.Content[len(last.Message.Content)-1]
			assert.Equal(t, "After", after.Text)
			if tc.wantCount == 2 {
				require.Len(t, last.Message.Content, 1)
				assert.Equal(t, tc.model, last.Model)
				assert.Equal(t, tc.provider, last.Provider)
				assert.Equal(t, "2026-10-01T00:00:03Z", last.Timestamp)
			}
			assert.Contains(t, RenderMarkdown(result.Entries, RenderOptions{}), "example output")
			assert.Contains(t, RenderHTMLFragment(result.Entries, RenderOptions{}), "example output")
		})
	}
}

func TestParseMetadata_CodexNaturalBoundariesAndRawShape(t *testing.T) {
	input := `{"type":"session_meta","payload":{"model":"first-model","model_provider":"first-provider"}}
{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"First"}]}}
{"type":"session_meta","payload":{"model":"second-model","model_provider":"second-provider"}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Next"}]}}
{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Second"}]}}`
	result, err := ParseCodexJSONL(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Entries, 3)
	assert.Equal(t, "first-model", result.Metadata.FirstModel)
	assert.Equal(t, "first-provider", result.Metadata.FirstProvider)
	assert.Equal(t, "second-model", result.Entries[2].Model)
	assert.Equal(t, "second-provider", result.Entries[2].Provider)
	data, err := json.Marshal(result.Entries[2])
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Second"}]}}`, string(data))
}

func TestParseMetadata_CodexFirstNonemptySummaries(t *testing.T) {
	input := `{"type":"session_meta","payload":{"model":"","model_provider":""}}
{"type":"session_meta","payload":{"model":"first-model","model_provider":"first-provider"}}
{"type":"session_meta","payload":{"model":"later-model","model_provider":"later-provider"}}`
	result, err := ParseCodexJSONL(strings.NewReader(input), ParseOptions{AllowEmptyCodex: true})
	require.NoError(t, err)
	require.NotNil(t, result.Metadata)
	assert.Equal(t, "first-model", result.Metadata.FirstModel)
	assert.Equal(t, "first-provider", result.Metadata.FirstProvider)
}

func TestParseMetadata_ModelChangeOptionsForwarded(t *testing.T) {
	input := `{"type":"session_meta","payload":{"model":"first-model"}}
{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"First"}]}}
{"type":"session_meta","payload":{"model":"second-model"}}
{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Second"}]}}`
	for _, forced := range []bool{false, true} {
		t.Run(fmt.Sprint(forced), func(t *testing.T) {
			opts := ParseOptions{AllowEmptyCodex: true, PreserveModelChanges: true}
			var result *ParseResult
			var err error
			if forced {
				result, err = ParseJSONLWithFormat(strings.NewReader(input), FormatCodex, opts)
			} else {
				result, err = Parse(strings.NewReader(input), opts)
			}
			require.NoError(t, err)
			require.Len(t, result.Entries, 2)
			assert.Equal(t, "first-model", result.Entries[0].Model)
			assert.Equal(t, "second-model", result.Entries[1].Model)
			assert.Equal(t, "first-model", result.Metadata.FirstModel)
		})
	}
}

// testdata/codex/turn_context.jsonl is synthetic. Its session_meta and turn_context
// payloads follow the pinned rust-v0.166.0 protocol.rs SessionMeta (model_provider,
// no model) and TurnContextItem (model) definitions; see testdata/codex/PROVENANCE.md.
func TestParseMetadata_CodexTurnContextModels(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "codex", "turn_context.jsonl"))
	require.NoError(t, err)
	input := string(data)

	t.Run("summary in both modes", func(t *testing.T) {
		for _, opts := range []ParseOptions{{}, {PreserveModelChanges: true}} {
			result, err := ParseCodexJSONL(strings.NewReader(input), opts)
			require.NoError(t, err)
			require.NotNil(t, result.Metadata)
			assert.Equal(t, "example-model-a", result.Metadata.FirstModel)
			assert.Equal(t, "example-provider", result.Metadata.FirstProvider)
			assert.Equal(t, "2026-10-01T09:00:00.000Z", result.Metadata.StartedAt)
			assert.Equal(t, "/work/example-repo", result.Metadata.Project)
			assert.Empty(t, result.Warnings)
		}
	})

	t.Run("preserved model changes", func(t *testing.T) {
		result, err := ParseCodexJSONL(strings.NewReader(input), ParseOptions{PreserveModelChanges: true})
		require.NoError(t, err)
		require.Len(t, result.Entries, 3)
		assert.Equal(t, "user", result.Entries[0].Type)

		first := result.Entries[1]
		assert.Equal(t, "example-model-a", first.Model)
		assert.Equal(t, "example-provider", first.Provider)
		assert.Equal(t, "2026-10-01T09:00:02.000Z", first.Timestamp)
		require.Len(t, first.Message.Content, 3)
		assert.Equal(t, "thinking", first.Message.Content[0].Type)
		assert.Equal(t, "call_1", first.Message.Content[1].ID)
		assert.Equal(t, "call_1", first.Message.Content[2].ToolUseID, "the output stays with its call (Q82)")

		second := result.Entries[2]
		assert.Equal(t, "example-model-b", second.Model)
		assert.Equal(t, "example-provider", second.Provider)
		assert.Equal(t, "2026-10-01T09:00:06.000Z", second.Timestamp)
		require.Len(t, second.Message.Content, 1)
		assert.Equal(t, "Tests pass.", second.Message.Content[0].Text)
	})

	t.Run("default output ignores turn_context", func(t *testing.T) {
		result, err := ParseCodexJSONL(strings.NewReader(input))
		require.NoError(t, err)
		require.Len(t, result.Entries, 2)
		assert.Empty(t, result.Entries[1].Model)
		assert.Equal(t, "example-provider", result.Entries[1].Provider)
		require.Len(t, result.Entries[1].Message.Content, 4)

		var kept []string
		for line := range strings.SplitSeq(input, "\n") {
			if !strings.Contains(line, `"type":"turn_context"`) {
				kept = append(kept, line)
			}
		}
		legacy, err := ParseCodexJSONL(strings.NewReader(strings.Join(kept, "\n")))
		require.NoError(t, err)
		assert.Equal(t, legacy.Entries, result.Entries)
		for _, render := range []func([]Entry, RenderOptions) string{RenderMarkdown, RenderHTML, RenderHTMLFragment} {
			assert.Equal(t, render(legacy.Entries, RenderOptions{}), render(result.Entries, RenderOptions{}))
		}
	})
}

func TestParseMetadata_CodexModelSources(t *testing.T) {
	meta := func(model string) string {
		return fmt.Sprintf(`{"timestamp":"2026-10-01T00:00:00Z","type":"session_meta","payload":{"id":"example-session","model":%q,"model_provider":"example-provider"}}`, model)
	}
	turnContext := func(model string) string {
		return fmt.Sprintf(`{"timestamp":"2026-10-01T00:00:01Z","type":"turn_context","payload":{"cwd":"/work/example","model":%q}}`, model)
	}
	assistant := `{"timestamp":"2026-10-01T00:00:02Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hello"}]}}`
	for name, tc := range map[string]struct {
		lines          []string
		wantFirst      string
		wantPreserved  string
		wantDefaultRun string
	}{
		"older session_meta model": {
			lines:          []string{meta("legacy-model"), assistant},
			wantFirst:      "legacy-model",
			wantPreserved:  "legacy-model",
			wantDefaultRun: "legacy-model",
		},
		"session_meta before turn_context": {
			lines:          []string{meta("legacy-model"), turnContext("turn-model"), assistant},
			wantFirst:      "legacy-model",
			wantPreserved:  "turn-model",
			wantDefaultRun: "legacy-model",
		},
		"empty turn_context keeps active model": {
			lines:          []string{meta(""), turnContext("turn-model"), turnContext(""), assistant},
			wantFirst:      "turn-model",
			wantPreserved:  "turn-model",
			wantDefaultRun: "",
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := strings.Join(tc.lines, "\n")
			result, err := ParseCodexJSONL(strings.NewReader(input))
			require.NoError(t, err)
			assert.Equal(t, tc.wantFirst, result.Metadata.FirstModel)
			require.Len(t, result.Entries, 1)
			assert.Equal(t, tc.wantDefaultRun, result.Entries[0].Model)

			result, err = ParseCodexJSONL(strings.NewReader(input), ParseOptions{PreserveModelChanges: true})
			require.NoError(t, err)
			assert.Equal(t, tc.wantFirst, result.Metadata.FirstModel)
			require.Len(t, result.Entries, 1)
			assert.Equal(t, tc.wantPreserved, result.Entries[0].Model)
		})
	}

	result, err := ParseCodexJSONL(strings.NewReader(meta("")+"\n"+turnContext("header-only-model")), ParseOptions{AllowEmptyCodex: true})
	require.NoError(t, err)
	assert.Empty(t, result.Entries)
	assert.Equal(t, "header-only-model", result.Metadata.FirstModel)
}

func TestParseMetadata_CodexStartedAtSkipsInvalidTimestamps(t *testing.T) {
	for name, tc := range map[string]struct {
		timestamps []string
		want       string
	}{
		"unparseable then valid": {[]string{"not-a-time", "2026-10-01T02:00:00Z"}, "2026-10-01T02:00:00Z"},
		"missing then nano":      {[]string{"", "2026-10-01T02:00:00.123456789Z"}, "2026-10-01T02:00:00.123456789Z"},
		"valid then later":       {[]string{"2026-10-01T01:00:00Z", "2026-10-01T02:00:00Z"}, "2026-10-01T01:00:00Z"},
		"none valid":             {[]string{"not-a-time"}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			var lines []string
			for _, ts := range tc.timestamps {
				lines = append(lines, fmt.Sprintf(`{"timestamp":%q,"type":"session_meta","payload":{"id":"example-session","timestamp":"2026-09-01T00:00:00Z"}}`, ts))
			}
			result, err := ParseCodexJSONL(strings.NewReader(strings.Join(lines, "\n")), ParseOptions{AllowEmptyCodex: true})
			require.NoError(t, err)
			assert.Equal(t, tc.want, result.Metadata.StartedAt)
		})
	}
}

func TestParseMetadata_CopilotStartedAt(t *testing.T) {
	prompt := `{"type":"user.message","timestamp":"2026-10-01T00:00:05Z","data":{"content":"Hello"}}`
	for name, tc := range map[string]struct {
		lines []string
		want  string
	}{
		"session.start startTime": {
			lines: []string{`{"type":"session.start","timestamp":"2026-10-01T00:00:01Z","data":{"startTime":"2026-10-01T00:00:00.500Z"}}`, prompt},
			want:  "2026-10-01T00:00:00.500Z",
		},
		"first parseable event timestamp without startTime": {
			lines: []string{`{"type":"session.start","timestamp":"invalid","data":{}}`, `{"type":"session.model_change","timestamp":"2026-10-01T00:00:02Z","data":{"model":"example-model"}}`, prompt},
			want:  "2026-10-01T00:00:02Z",
		},
		"unparseable startTime": {
			lines: []string{`{"type":"session.start","timestamp":"2026-10-01T00:00:01Z","data":{"startTime":"invalid"}}`, prompt},
			want:  "2026-10-01T00:00:01Z",
		},
		"prompt is the only timestamped event": {
			lines: []string{`{"type":"session.start","data":{}}`, prompt},
			want:  "2026-10-01T00:00:05Z",
		},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := ParseCopilot(strings.NewReader(strings.Join(tc.lines, "\n")))
			require.NoError(t, err)
			require.NotNil(t, result.Metadata)
			assert.Equal(t, tc.want, result.Metadata.StartedAt)
		})
	}
}

func TestParseMetadata_Copilot(t *testing.T) {
	input := `{"type":"session.start","timestamp":"2026-10-01T00:00:00Z"}
{"type":"session.model_change","data":{"model":"first-model"}}
{"type":"user.message","timestamp":"2026-10-01T00:00:01Z","data":{"content":"Hello"}}
{"type":"assistant.turn_start","timestamp":"2026-10-01T00:00:02Z"}
{"type":"assistant.message","data":{"content":"First"}}
{"type":"assistant.turn_end"}
{"type":"session.model_change","data":{"model":"second-model"}}
{"type":"assistant.turn_start","timestamp":"2026-10-01T00:00:03Z"}
{"type":"assistant.message","data":{"content":"Second"}}
{"type":"assistant.turn_end"}`
	result, err := ParseCopilot(strings.NewReader(input))
	require.NoError(t, err)
	assert.Equal(t, FormatCopilot, result.Format)
	require.NotNil(t, result.Metadata)
	assert.Equal(t, "2026-10-01T00:00:00Z", result.Metadata.StartedAt, "session.start, not the first prompt (Decision 30)")
	assert.Equal(t, "first-model", result.Metadata.FirstModel)
	assert.Empty(t, result.Metadata.FirstProvider)
	assert.Empty(t, result.Metadata.Project)
	require.Len(t, result.Entries, 3)
	assert.Equal(t, "first-model", result.Entries[1].Model)
	assert.Equal(t, "second-model", result.Entries[2].Model)
	result, err = ParseCopilot(strings.NewReader(`{"type":"session.model_change","data":{"model":"header-model"}}`))
	require.NoError(t, err)
	assert.Empty(t, result.Entries)
	require.NotNil(t, result.Metadata)
	assert.Equal(t, "header-model", result.Metadata.FirstModel)
	assert.Empty(t, result.Metadata.StartedAt)
}

func TestParseMetadata_Kiro(t *testing.T) {
	input := `{"conversation_id":"example-session","history":[
{"user":{"timestamp":"2026-10-01T00:00:00Z","content":{"Prompt":{"prompt":"Hello"}}},"request_metadata":{"model_id":"first-model"}},
{"assistant":{"Response":{"content":"Hi"}},"request_metadata":{"model_id":"second-model","request_start_timestamp_ms":1790812801000}}
]}`
	result, err := ParseKiro(strings.NewReader(input))
	require.NoError(t, err)
	assert.Equal(t, FormatKiro, result.Format)
	require.NotNil(t, result.Metadata)
	assert.Empty(t, result.Metadata.StartedAt, "the resolver supplies the database created_at (Decision 30)")
	assert.Equal(t, "2026-10-01T00:00:00Z", result.Entries[0].Timestamp)
	assert.Equal(t, "first-model", result.Metadata.FirstModel)
	assert.Empty(t, result.Metadata.Project)
	assert.Empty(t, result.Metadata.FirstProvider)
	assert.Nil(t, result.Metadata.TotalCost)
	require.Len(t, result.Entries, 2)
	assert.Equal(t, "second-model", result.Entries[1].Model)
}

func TestParseMetadata_KiroIDEAndWarningUnits(t *testing.T) {
	input := `{"executionId":"example-execution","metadata":{"startTime":1790812800000,"modelId":"example-model","modelProvider":"example-provider"},"chat":[{"role":"unknown","content":"Skipped"},{"role":"bot","content":"Hello"}]}`
	result, err := ParseKiroIDE(strings.NewReader(input))
	require.NoError(t, err)
	assert.Equal(t, FormatKiroIDE, result.Format)
	require.NotNil(t, result.Metadata)
	assert.Equal(t, "2026-10-01T00:00:00Z", result.Metadata.StartedAt)
	assert.Equal(t, "example-model", result.Metadata.FirstModel)
	assert.Equal(t, "example-provider", result.Metadata.FirstProvider)
	assert.Nil(t, result.Metadata.TotalCost)
	assert.Equal(t, "example-provider", result.Entries[0].Provider)
	require.Len(t, result.Warnings, 1)
	assert.Equal(t, "chat-message", result.Warnings[0].Unit)
	assert.Equal(t, 1, result.Warnings[0].Line)
	assert.Equal(t, `unknown role "unknown" in chat message`, result.Warnings[0].Message)

	costPath := filepath.Join(t.TempDir(), "execution.json")
	require.NoError(t, os.WriteFile(costPath, []byte(`{"actions":[{"actionType":"model"},{"actionType":"future-action"}]}`), 0o600))
	result, err = ParseKiroIDEWithCostPath(strings.NewReader(input), costPath)
	require.NoError(t, err)
	require.Len(t, result.Warnings, 1)
	assert.Equal(t, "execution-action", result.Warnings[0].Unit)
	assert.Equal(t, 2, result.Warnings[0].Line)
	assert.Equal(t, `unknown action type "future-action", skipped`, result.Warnings[0].Message)
	assert.Equal(t, "example-model", result.Metadata.FirstModel)
}

func TestParseMetadata_PhysicalWarningUnits(t *testing.T) {
	for _, format := range []Format{FormatClaude, FormatCodex, FormatCopilot} {
		t.Run(fmt.Sprint(format), func(t *testing.T) {
			input := "malformed\n"
			if format == FormatCodex {
				input += `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Hello"}]}}`
			}
			result, err := ParseJSONLWithFormat(strings.NewReader(input), format)
			require.NoError(t, err)
			require.Len(t, result.Warnings, 1)
			assert.Equal(t, 1, result.Warnings[0].Line)
			assert.Empty(t, result.Warnings[0].Unit)
		})
	}
}
