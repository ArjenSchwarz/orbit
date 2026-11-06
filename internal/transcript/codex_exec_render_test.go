package transcript

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var updateGoldens = flag.Bool("update", false, "rewrite Codex exec golden files")

// styleBlock removes the embedded stylesheet, so standalone HTML goldens do
// not change with unrelated CSS edits.
var styleBlock = regexp.MustCompile(`(?s)<style>.*?</style>`)

func execRenderOptions(res *ParseResult) RenderOptions {
	return RenderOptions{SessionID: "example-session", ThreadID: res.Metadata.ThreadID}
}

func TestCodexExecRender_Goldens(t *testing.T) {
	for _, name := range []string{"all_items", "turns", "failures"} {
		t.Run(name, func(t *testing.T) {
			res := parseExecFixture(t, name+".jsonl")
			opts := execRenderOptions(res)
			outputs := map[string]string{
				name + ".md.golden":            RenderMarkdown(res.Entries, opts),
				name + ".html.golden":          styleBlock.ReplaceAllString(RenderHTML(res.Entries, opts), "<style></style>"),
				name + ".fragment.html.golden": RenderHTMLFragment(res.Entries, opts),
			}
			for file, got := range outputs {
				path := filepath.Join("testdata", "codex_exec", file)
				if *updateGoldens {
					require.NoError(t, os.WriteFile(path, []byte(got), 0o600))
				}
				want, err := os.ReadFile(path)
				require.NoError(t, err)
				assert.Equal(t, string(want), got, file)
			}
		})
	}
}

func TestCodexExecRender_Markdown(t *testing.T) {
	res := parseExecFixture(t, "all_items.jsonl")
	md := RenderMarkdown(res.Entries, execRenderOptions(res))

	for name, want := range map[string]string{
		"session ID":             "**Session ID:** `example-session`",
		"thread ID":              "**Thread ID:** `0199aaaa-0000-7000-8000-000000000001`",
		"turn in assistant meta": "## 🤖 Assistant · turn 1\n",
		"command like Bash":      "**Command:**\n```bash\n/bin/sh -lc 'make test'\n```",
		"exit code 0":            "**Result (exit code 0):**\n```\nok\n",
		"nonzero exit code":      "**Result (exit code 2):**",
		"file change list":       "- update `/work/example-repo/main.go`\n- add `/work/example-repo/notes.md`\n",
		"mcp input":              "\"server\": \"docs\"",
		"web search input":       "\"query\": \"go json number precision\"",
		"plan like TodoWrite":    "- [x] Run tests\n- [ ] Report\n",
		"incomplete plan":        "<summary>⏳ 📋 Plan (incomplete)</summary>",
		"error item":             "❌ **Error:** command output was truncated",
		"generic event":          "<summary>📦 Event: collab_tool_call</summary>",
		"usage line":             "**Token usage (thread totals at turn completion):** input_tokens 1200 · cached_input_tokens 800 · cache_write_input_tokens 0 · output_tokens 90 · reasoning_output_tokens 40 · future_counter 123456789012345678901234567890",
	} {
		assert.Contains(t, md, want, name)
	}

	declined := blockContaining(t, md, "<details", "rm -rf build")
	assert.Contains(t, declined, "**Result:**", "an unrecorded exit code shows no number")
	assert.NotContains(t, declined, "exit code")
	assert.Contains(t, declined, "❌")
}

func TestCodexExecRender_HTML(t *testing.T) {
	res := parseExecFixture(t, "all_items.jsonl")
	opts := execRenderOptions(res)
	for name, out := range map[string]string{
		"standalone": RenderHTML(res.Entries, opts),
		"fragment":   RenderHTMLFragment(res.Entries, opts),
	} {
		t.Run(name, func(t *testing.T) {
			for label, want := range map[string]string{
				"turn in assistant meta": "<span>turn 1</span>",
				"command like Bash":      "<strong>Command:</strong>\n                            <pre><code>/bin/sh -lc &#39;make test&#39;</code></pre>",
				"exit code 0":            "<strong>Result (exit code 0):</strong>",
				"file change list":       "<li>update <code>/work/example-repo/main.go</code></li>",
				"plan like TodoWrite":    "<li>[x] Run tests</li>",
				"incomplete plan":        "<span class=\"icon\">⏳</span> 📋 Plan (incomplete)",
				"error item":             "<strong>Error:</strong> command output was truncated",
				"generic event":          "<span class=\"icon\">📦</span> Event: collab_tool_call",
				"escaped payload":        "Review &lt;script&gt;alert(1)&lt;/script&gt; output",
				"usage":                  "<div class=\"usage\">Token usage (thread totals at turn completion): input_tokens 1200",
			} {
				assert.Contains(t, out, want, label)
			}
			assert.NotContains(t, out, "<script>alert(1)", "event payloads are escaped, never raw HTML")
		})
	}

	standalone := RenderHTML(res.Entries, opts)
	assert.Contains(t, standalone, "Session ID: <code>example-session</code>")
	assert.Contains(t, standalone, "Thread ID: <code>0199aaaa-0000-7000-8000-000000000001</code>")
}

func TestCodexExecRender_EntryWithOnlyNewKinds(t *testing.T) {
	entries := []Entry{{
		Type: "assistant",
		Turn: 3,
		Message: &Message{Role: "assistant", Content: []ContentItem{
			{Type: "usage", Usage: &Usage{Basis: UsageBasisThreadTotal, Counters: []UsageCounter{{Name: "input_tokens", Value: []byte("5")}}}},
		}},
	}}
	for name, out := range map[string]string{
		"markdown":   RenderMarkdown(entries, RenderOptions{}),
		"standalone": RenderHTML(entries, RenderOptions{}),
		"fragment":   RenderHTMLFragment(entries, RenderOptions{}),
	} {
		assert.Contains(t, out, "input_tokens 5", name)
		assert.Contains(t, out, "turn 3", name)
	}
}

func TestCodexExecRender_ReusedItemIDsAcrossTurns(t *testing.T) {
	res := parseExecFixture(t, "turns.jsonl")
	opts := execRenderOptions(res)
	for name, tc := range map[string]struct {
		out, sep string
	}{
		"markdown":   {RenderMarkdown(res.Entries, opts), "<details"},
		"standalone": {RenderHTML(res.Entries, opts), "<details"},
		"fragment":   {RenderHTMLFragment(res.Entries, opts), "<details"},
	} {
		t.Run(name, func(t *testing.T) {
			first := blockContaining(t, tc.out, tc.sep, "go test ./...")
			assert.Contains(t, first, "PASS")
			second := blockContaining(t, tc.out, tc.sep, "make build")
			assert.Contains(t, second, "compiling")
			assert.NotContains(t, second, "PASS")
			assert.Contains(t, second, "⏳")
			assert.Contains(t, second, "(incomplete)")
		})
	}
}

// blockContaining returns the part of out between two separators that
// contains marker.
func blockContaining(t *testing.T, out, sep, marker string) string {
	t.Helper()
	for part := range strings.SplitSeq(out, sep) {
		if strings.Contains(part, marker) {
			return part
		}
	}
	t.Fatalf("no block contains %q", marker)
	return ""
}
