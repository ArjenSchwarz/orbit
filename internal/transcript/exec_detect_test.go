package transcript

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const bom = "\xef\xbb\xbf"

// kiroKeys holds every key the substring-only Kiro heuristics look for.
const kiroKeys = `"conversation_id":"c","history":[],"executionId":"e","chat":[],"metadata":{}`

func TestProbeExecStream(t *testing.T) {
	oversized := `{"type":"x","pad":"` + strings.Repeat("y", 10*1024*1024) + `"}` + "\n"
	tests := map[string]struct {
		input    string
		outcome  ExecProbeOutcome
		threadID string
	}{
		"thread.started":            {execThreadLine, ExecProbeExec, "thread-1"},
		"leading BOM":               {bom + execThreadLine, ExecProbeExec, "thread-1"},
		"leading blank lines":       {"\n  \n\r\n" + execThreadLine, ExecProbeExec, "thread-1"},
		"malformed lines first":     {"{not json\n[1,2]\n" + execThreadLine, ExecProbeExec, "thread-1"},
		"skipped oversized line":    {oversized + execThreadLine, ExecProbeExec, "thread-1"},
		"missing thread_id":         {`{"type":"thread.started"}` + "\n", ExecProbeWithoutThread, ""},
		"empty thread_id":           {`{"type":"thread.started","thread_id":""}` + "\n", ExecProbeWithoutThread, ""},
		"non-string thread_id":      {`{"type":"thread.started","thread_id":7}` + "\n" + execThreadLine, ExecProbeWithoutThread, ""},
		"turn first":                {`{"type":"turn.started"}` + "\n" + execThreadLine, ExecProbeWithoutThread, ""},
		"item first":                {`{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"x"}}` + "\n", ExecProbeWithoutThread, ""},
		"error before thread":       {`{"type":"error","message":"unreachable"}` + "\n", ExecProbeWithoutThread, ""},
		"other source first":        {`{"type":"user","message":{"role":"user","content":"hi"}}` + "\n" + execThreadLine, ExecProbeOther, ""},
		"object without type":       {`{"thread_id":"thread-1"}` + "\n", ExecProbeOther, ""},
		"empty input":               {"", ExecProbeOther, ""},
		"only malformed and blanks": {"{\n\n}\n", ExecProbeOther, ""},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := ProbeExecStream(strings.NewReader(tc.input))
			require.NoError(t, err)
			assert.Equal(t, ExecProbe{Outcome: tc.outcome, ThreadID: tc.threadID}, got)
		})
	}

	t.Run("reader error", func(t *testing.T) {
		_, err := ProbeExecStream(failingReader{strings.NewReader("\n\n")})
		require.Error(t, err)
	})
}

func TestDetectFormat_ExecStreams(t *testing.T) {
	longMalformedPrefix := `{` + kiroKeys + `,"pad":"` + strings.Repeat("z", 70*1024) + "\n" + execThreadLine
	splitFirstObject := `{"type":"thread.started","thread_id":"thread-1","extra":{` + kiroKeys +
		`,"pad":"` + strings.Repeat("z", 70*1024) + `"}}` + "\n" + `{"type":"turn.started"}` + "\n"

	tests := map[string]struct {
		input string
		want  Format
	}{
		"thread.started": {execThreadLine, FormatCodexExec},
		"leading BOM":    {bom + execThreadLine, FormatCodexExec},
		"malformed prefix beyond 64 KiB with Kiro keys": {longMalformedPrefix, FormatCodexExec},
		"first exec object split beyond 64 KiB":         {splitFirstObject, FormatCodexExec},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// The chunk alone matches a Kiro heuristic for the long inputs.
			got, replay, err := DetectFormat(strings.NewReader(tc.input))
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, strings.TrimPrefix(tc.input, bom), string(replay), "every consumed byte is returned for replay")

			parsed, err := Parse(strings.NewReader(tc.input))
			require.NoError(t, err)
			forced, err := ParseCodexExec(strings.NewReader(tc.input))
			require.NoError(t, err)
			assert.Equal(t, forced, parsed, "parsing after an extended probe equals a forced parse")
		})
	}
}

func TestDetectFormat_ExecProbeKeepsLegacyResults(t *testing.T) {
	tests := map[string]string{
		"exec without thread":   `{"type":"error","message":"unreachable"}` + "\n",
		"turn before thread":    `{"type":"turn.started"}` + "\n" + `{"type":"user","message":{"role":"user","content":"hi"}}` + "\n",
		"claude then exec":      `{"type":"user","message":{"role":"user","content":"hi"}}` + "\n" + execThreadLine,
		"kiro heuristic prefix": `{` + kiroKeys + `,"pad":"` + strings.Repeat("z", 70*1024) + "\n",
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			assertLegacyDetection(t, []byte(input))
		})
	}
}

func TestDetectFormat_ExistingTestdataUnchanged(t *testing.T) {
	err := filepath.WalkDir("testdata", func(path string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		if d.IsDir() {
			if d.Name() == "codex_exec" {
				return filepath.SkipDir
			}
			return nil
		}
		if ext := filepath.Ext(path); ext != ".jsonl" && ext != ".json" && ext != ".chat" {
			return nil
		}
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		t.Run(path, func(t *testing.T) { assertLegacyDetection(t, data) })
		return nil
	})
	require.NoError(t, err)
}

func TestDetectFormat_LargeKiroFiles(t *testing.T) {
	var cli bytes.Buffer
	cli.WriteString(`{"conversation_id":"conv-1","history":[`)
	for i := range 400 {
		if i > 0 {
			cli.WriteString(",")
		}
		cli.WriteString(`{"user":{"content":{"Prompt":{"prompt":"` + strings.Repeat("p", 200) + `"}}}}`)
	}
	cli.WriteString(`]}`)

	var ide bytes.Buffer
	ide.WriteString("{\n  \"executionId\": \"exec-1\",\n  \"metadata\": {\"modelId\": \"example-model\"},\n  \"chat\": [\n")
	for i := range 400 {
		if i > 0 {
			ide.WriteString(",\n")
		}
		ide.WriteString(`    {"role": "human", "content": "` + strings.Repeat("q", 200) + `"}`)
	}
	ide.WriteString("\n  ]\n}\n")

	for name, tc := range map[string]struct {
		data []byte
		want Format
	}{
		"kiro cli single line": {cli.Bytes(), FormatKiro},
		"kiro ide multi line":  {ide.Bytes(), FormatKiroIDE},
	} {
		t.Run(name, func(t *testing.T) {
			require.Greater(t, len(tc.data), 64*1024)
			assertLegacyDetection(t, tc.data)
			got, replay, err := DetectFormat(bytes.NewReader(tc.data))
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.data, replay, "every consumed byte is returned for replay")
		})
	}
}

// legacyDetectFormat is detection as it was before exec streams: a 64 KiB
// chunk, any Kiro recognition, then the first format-defining JSONL line.
func legacyDetectFormat(data []byte) (Format, error) {
	if len(data) == 0 {
		return FormatUnknown, errors.New("empty file")
	}
	chunk := stripBOM(data[:min(len(data), 65536)])
	if format, _ := detectKiroFormat(chunk); format != FormatUnknown {
		return format, nil
	}
	format, _, err := detectJSONLFormat(chunk)
	return format, err
}

// assertLegacyDetection checks that DetectFormat gives the legacy result and
// that Parse reads every byte of the input.
func assertLegacyDetection(t *testing.T, data []byte) {
	t.Helper()
	want, wantErr := legacyDetectFormat(data)
	got, replay, err := DetectFormat(bytes.NewReader(data))
	if wantErr != nil {
		require.Error(t, err)
	} else {
		require.NoError(t, err)
	}
	assert.Equal(t, want, got)
	if len(data) > 0 {
		rest := stripBOM(data)
		assert.True(t, bytes.HasPrefix(rest, replay), "replay bytes are a prefix of the input")
	}

	if wantErr == nil {
		parsed, err := Parse(bytes.NewReader(data))
		forced, forcedErr := ParseJSONLWithFormat(bytes.NewReader(stripBOM(data)), want)
		if forcedErr != nil {
			require.Error(t, err)
			return
		}
		require.NoError(t, err)
		assert.Equal(t, forced, parsed)
	}
}
