package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/arjenschwarz/orbit/internal/sessions"
)

const execThreadID = "0199aaaa-0000-7000-8000-000000000001"

func execFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "internal", "transcript", "testdata", "codex_exec", name))
	require.NoError(t, err)
	return string(data)
}

// execRepo writes an exec stream inside a new repository and returns the
// stream path and the repository's canonical root.
func execRepo(t *testing.T, content string) (path, repo string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	repo = filepath.Join(root, "example-repo")
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".git"), 0o755))
	return writeFile(t, filepath.Join(repo, "logs", "run.jsonl"), content), repo
}

func TestCodexExecFile_MarkdownAndHTMLHeaders(t *testing.T) {
	path, _ := execRepo(t, execFixture(t, "all_items.jsonl"))
	wantID := sessions.ExecIdentifier(execThreadID, path)

	for name, format := range map[string]string{"markdown": "md", "html": "html"} {
		t.Run(name, func(t *testing.T) {
			stdout, _, err := runCLI(t, &Config{Input: path, Format: format}, "")
			require.NoError(t, err)
			assert.Contains(t, stdout, execThreadID)
			assert.Contains(t, stdout, wantID)
			assert.Contains(t, stdout, "Thread ID")
			assert.NotContains(t, stdout, "`run`", "the filename stem is never the session ID")
			assert.NotContains(t, stdout, "<code>run</code>", "the filename stem is never the session ID")
			assert.Contains(t, stdout, "command_execution")
		})
	}
}

func TestCodexExecFile_ForcedPreThreadFailure(t *testing.T) {
	path, repo := execRepo(t, execFixture(t, "pre_thread_failure.jsonl"))

	stdout, _, err := runCLI(t, &Config{Input: path, Format: "md", Agent: "codex-exec"}, "")
	require.NoError(t, err)
	assert.Contains(t, stdout, "❌ **Error:** model provider is not reachable")
	assert.NotContains(t, stdout, "Session ID")
	assert.NotContains(t, stdout, "Thread ID")

	stdout, _, err = runCLI(t, &Config{Input: path, Format: "shared-json", Agent: "codex-exec"}, "")
	require.NoError(t, err)
	doc := decodeSession(t, stdout)
	assert.Equal(t, "codex-exec", doc.Session.Source)
	assert.Empty(t, doc.Session.ID, "without a thread there is no identifier")
	assert.Empty(t, doc.Session.ThreadID)
	assert.Equal(t, repo, doc.Session.Project)
}

func TestCodexExecFile_SharedJSON(t *testing.T) {
	path, repo := execRepo(t, execFixture(t, "all_items.jsonl"))

	stdout, _, err := runCLI(t, &Config{Input: path, Format: "shared-json"}, "")
	require.NoError(t, err)
	doc := decodeSession(t, stdout)
	assert.Equal(t, "codex-exec", doc.Session.Source)
	assert.Equal(t, sessions.ExecIdentifier(execThreadID, path), doc.Session.ID)
	assert.Equal(t, execThreadID, doc.Session.ThreadID)
	assert.Equal(t, repo, doc.Session.Project)
	assert.Empty(t, doc.Session.CreatedAt)
	require.Len(t, doc.Entries, 1)
	assert.Equal(t, 1, doc.Entries[0].Turn)
}

func TestCodexExecStdin_HasNoFilesystemFacts(t *testing.T) {
	stdout, _, err := runCLI(t, &Config{Format: "shared-json"}, execFixture(t, "all_items.jsonl"))
	require.NoError(t, err)
	doc := decodeSession(t, stdout)
	assert.Equal(t, "codex-exec", doc.Session.Source)
	assert.Equal(t, execThreadID, doc.Session.ThreadID)
	assert.Empty(t, doc.Session.ID)
	assert.Empty(t, doc.Session.Project)
	assert.Empty(t, doc.Session.CreatedAt)

	stdout, _, err = runCLI(t, &Config{Format: "md"}, execFixture(t, "all_items.jsonl"))
	require.NoError(t, err)
	assert.Contains(t, stdout, "**Thread ID:** `"+execThreadID+"`")
	assert.NotContains(t, stdout, "Session ID")
}

func TestCodexExecFile_MalformedLineWarnings(t *testing.T) {
	path, _ := execRepo(t, execFixture(t, "failures.jsonl"))

	stdout, stderr, err := runCLI(t, &Config{Input: path, Format: "shared-json"}, "")
	require.NoError(t, err)
	assert.Contains(t, stderr, "Warning: line 4: malformed JSON event (offset")
	assert.NotContains(t, stderr, "broken")

	doc := decodeSession(t, stdout)
	require.NotEmpty(t, doc.Warnings)
	assert.Equal(t, "malformed_line", doc.Warnings[0].Code)
	assert.Equal(t, 4, doc.Warnings[0].Line)
}

func TestCodexExecFollowIsRejected(t *testing.T) {
	path, _ := execRepo(t, execFixture(t, "all_items.jsonl"))
	tests := map[string]*Config{
		"forced exec format": {Input: path, Follow: true, Format: "md", Agent: "codex-exec"},
		"exec file":          {Input: path, Follow: true, Format: "md"},
	}
	for name, cfg := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := run(cfg)
			require.Error(t, err)
			assert.Equal(t, "follow mode does not support Codex exec streams", err.Error())
		})
	}
}

func TestCodexExecRawJSON_KeepsThreadStartedAfterBOM(t *testing.T) {
	path, _ := execRepo(t, "\xef\xbb\xbf"+execFixture(t, "turns.jsonl"))

	stdout, stderr, err := runCLI(t, &Config{Input: path, Format: "json"}, "")
	require.NoError(t, err)
	assert.NotContains(t, stderr, "skipping invalid JSON line")
	var events []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &events))
	require.NotEmpty(t, events)
	assert.Equal(t, "thread.started", events[0]["type"])
	assert.Len(t, events, 12)
}
