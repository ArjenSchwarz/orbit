package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every existing warning creation site sets a code (Q80) without changing its
// message, position or unit.
func TestParseWarningCodes(t *testing.T) {
	type want struct {
		line    int
		code    string
		message string
		unit    string
	}
	for name, tc := range map[string]struct {
		parse func() (*ParseResult, error)
		want  want
	}{
		"claude malformed line": {
			parse: func() (*ParseResult, error) { return parseClaudeJSONL(strings.NewReader("{not json")) },
			want:  want{1, WarningMalformedLine, "failed to parse JSON: invalid character 'n' looking for beginning of object key string", ""},
		},
		"claude oversized line": {
			parse: func() (*ParseResult, error) {
				return parseClaudeJSONL(strings.NewReader(`{"type":"user"}` + "\n" + strings.Repeat("x", 10*1024*1024+1)))
			},
			want: want{2, WarningOversizedLine, "line exceeds 10MB buffer limit", ""},
		},
		"codex malformed line": {
			parse: codexWarningInput("{not json"),
			want:  want{1, WarningMalformedLine, "failed to parse JSON: invalid character 'n' looking for beginning of object key string", ""},
		},
		"codex missing type": {
			parse: codexWarningInput(`{"payload":{}}`),
			want:  want{1, WarningInvalidRecord, "missing required field: type", ""},
		},
		"codex response_item missing payload": {
			parse: codexWarningInput(`{"type":"response_item"}`),
			want:  want{1, WarningInvalidRecord, "missing required field: payload", ""},
		},
		"codex event_msg missing payload": {
			parse: codexWarningInput(`{"type":"event_msg"}`),
			want:  want{1, WarningInvalidRecord, "missing required field: payload", ""},
		},
		"codex unknown record type": {
			parse: codexWarningInput(`{"type":"future_record","payload":{}}`),
			want:  want{1, WarningUnrecognizedRecord, "unrecognized event type: future_record", ""},
		},
		"codex undecodable session_meta": {
			parse: codexWarningInput(`{"type":"session_meta","payload":"text"}`),
			want:  want{1, WarningInvalidRecord, "failed to parse session_meta payload: json: cannot unmarshal string into Go value of type transcript.CodexSessionMeta", ""},
		},
		"codex undecodable response_item": {
			parse: codexWarningInput(`{"type":"response_item","payload":"text"}`),
			want:  want{1, WarningInvalidRecord, "failed to parse response_item payload: json: cannot unmarshal string into Go value of type transcript.CodexResponseItem", ""},
		},
		"codex unknown response_item": {
			parse: codexWarningInput(`{"type":"response_item","payload":{"type":"future_item"}}`),
			want:  want{1, WarningUnrecognizedRecord, "unrecognized response_item type: future_item", ""},
		},
		"codex undecodable event_msg": {
			parse: codexWarningInput(`{"type":"event_msg","payload":"text"}`),
			want:  want{1, WarningInvalidRecord, "failed to parse event_msg payload: json: cannot unmarshal string into Go value of type transcript.CodexEventMsg", ""},
		},
		"codex unknown event_msg": {
			parse: codexWarningInput(`{"type":"event_msg","payload":{"type":"future_event"}}`),
			want:  want{1, WarningUnrecognizedRecord, "unrecognized event_msg type: future_event", ""},
		},
		"codex unknown message role": {
			parse: codexWarningInput(`{"type":"response_item","payload":{"type":"message","role":"system","content":[]}}`),
			want:  want{1, WarningUnrecognizedRecord, "unrecognized message role: system", ""},
		},
		"codex output without call": {
			parse: codexWarningInput(`{"type":"response_item","payload":{"type":"function_call_output","call_id":"missing-call","output":"orphan"}}`),
			want:  want{1, WarningUnmatchedToolResult, "no matching function_call for call_id: missing-call", ""},
		},
		"copilot malformed line": {
			parse: func() (*ParseResult, error) { return ParseCopilot(strings.NewReader("{not json")) },
			want:  want{1, WarningMalformedLine, "failed to parse JSON: invalid character 'n' looking for beginning of object key string", ""},
		},
		"kiro ide missing role": {
			parse: func() (*ParseResult, error) {
				return ParseKiroIDE(strings.NewReader(`{"executionId":"example","metadata":{},"chat":[{"content":"Hello"}]}`))
			},
			want: want{1, WarningInvalidRecord, "message 0: missing role, skipped", "chat-message"},
		},
		"kiro ide unknown role": {
			parse: func() (*ParseResult, error) {
				return ParseKiroIDE(strings.NewReader(`{"executionId":"example","metadata":{},"chat":[{"role":"future","content":"Hello"}]}`))
			},
			want: want{1, WarningUnrecognizedRecord, `unknown role "future" in chat message`, "chat-message"},
		},
		"kiro ide unknown action": {
			parse: func() (*ParseResult, error) {
				costPath := filepath.Join(t.TempDir(), "execution.json")
				if err := os.WriteFile(costPath, []byte(`{"actions":[{"actionType":"say","output":{"message":"Hi"}},{"actionType":"future-action"}]}`), 0o600); err != nil {
					return nil, err
				}
				return ParseKiroIDEWithCostPath(strings.NewReader(`{"executionId":"example","metadata":{},"chat":[]}`), costPath)
			},
			want: want{2, WarningUnrecognizedRecord, `unknown action type "future-action", skipped`, "execution-action"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := tc.parse()
			require.NoError(t, err)
			require.Len(t, result.Warnings, 1)
			got := result.Warnings[0]
			assert.Equal(t, tc.want.line, got.Line)
			assert.Equal(t, tc.want.code, got.Code)
			assert.Equal(t, tc.want.message, got.Message)
			assert.Equal(t, tc.want.unit, got.Unit)
		})
	}
}

func codexWarningInput(line string) func() (*ParseResult, error) {
	return func() (*ParseResult, error) {
		return ParseCodexJSONL(strings.NewReader(line), ParseOptions{AllowEmptyCodex: true})
	}
}
