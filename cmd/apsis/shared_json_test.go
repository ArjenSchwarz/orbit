package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/arjenschwarz/orbit/internal/agents/claudecode"
	"github.com/arjenschwarz/orbit/internal/sharedjson"
)

// fakeHome points HOME, and the XDG directories derived from it, at a new
// temporary directory.
func fakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	return home
}

func writeFile(t *testing.T, path, content string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// runCLI runs apsis with cfg, writing output to a temporary file and capturing stderr.
func runCLI(t *testing.T, cfg *Config, stdin string) (stdout, stderr string, err error) {
	t.Helper()
	cfg.Output = filepath.Join(t.TempDir(), "output")

	if stdin != "" {
		r, w, pipeErr := os.Pipe()
		require.NoError(t, pipeErr)
		_, writeErr := w.WriteString(stdin)
		require.NoError(t, writeErr)
		require.NoError(t, w.Close())
		oldStdin := os.Stdin
		os.Stdin = r
		t.Cleanup(func() { os.Stdin = oldStdin; _ = r.Close() })
	}

	r, w, pipeErr := os.Pipe()
	require.NoError(t, pipeErr)
	oldStderr := os.Stderr
	os.Stderr = w
	captured := make(chan string)
	go func() {
		data, _ := io.ReadAll(r)
		captured <- string(data)
	}()

	exitCode, err := run(cfg)

	_ = w.Close()
	os.Stderr = oldStderr
	stderr = <-captured
	assert.Zero(t, exitCode)

	data, _ := os.ReadFile(cfg.Output)
	return string(data), stderr, err
}

func decodeSession(t *testing.T, stdout string) sharedjson.SessionDocument {
	t.Helper()
	var doc sharedjson.SessionDocument
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc), stdout)
	assert.Equal(t, sharedjson.SchemaVersion, doc.SchemaVersion)
	assert.Equal(t, "session", doc.Kind)
	return doc
}

// kiroCLIDatabase creates the Kiro CLI database where logs.DBPath looks for it under home.
func kiroCLIDatabase(t *testing.T, home string) *sql.DB {
	t.Helper()
	var path string
	switch runtime.GOOS {
	case "darwin":
		path = filepath.Join(home, "Library", "Application Support", "kiro-cli", "data.sqlite3")
	case "linux":
		path = filepath.Join(home, ".local", "share", "kiro-cli", "data.sqlite3")
	default:
		t.Skipf("no Kiro CLI database location for %s under a fake HOME", runtime.GOOS)
	}
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`CREATE TABLE conversations_v2 (
		key TEXT NOT NULL, conversation_id TEXT NOT NULL, value TEXT NOT NULL,
		created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
		PRIMARY KEY (key, conversation_id))`)
	require.NoError(t, err)
	return db
}

// kiroIDEWorkspace returns the Kiro IDE workspace directory for project under the fake HOME.
func kiroIDEWorkspace(t *testing.T, project string) string {
	t.Helper()
	configDir, err := os.UserConfigDir()
	require.NoError(t, err)
	sum := sha256.Sum256([]byte(project))
	return filepath.Join(configDir, "Kiro", "User", "globalStorage", "kiro.kiroagent", hex.EncodeToString(sum[:16]))
}

const (
	codexID   = "0199cccc-0000-7000-8000-000000000003"
	copilotID = "12345678-aaaa-bbbb-cccc-123456789abc"
)

// sessionsForEverySource writes one synthetic session per existing source for
// project, whose Kiro CLI key is realProject.
func sessionsForEverySource(t *testing.T, home, project, realProject string) {
	t.Helper()
	writeFile(t, filepath.Join(home, ".claude", "projects", claudecode.BuildProjectPath(project), "claude-session.jsonl"),
		`{"type":"user","uuid":"claude-user-1","entrypoint":"cli","timestamp":"2026-10-01T08:00:00Z","cwd":"/work/claude-repo","message":{"role":"user","content":"Hello"}}
{"type":"assistant","uuid":"claude-assistant-1","entrypoint":"cli","timestamp":"2026-10-01T08:00:01Z","message":{"role":"assistant","content":[{"type":"text","text":"Hi"}]}}
`)
	writeFile(t, filepath.Join(home, ".codex", "sessions", "2026", "10", "01", "rollout-2026-10-01T09-00-00-"+codexID+".jsonl"),
		`{"timestamp":"2026-10-01T09:00:00Z","type":"session_meta","payload":{"id":"`+codexID+`","cwd":"/work/codex-repo","originator":"codex_cli_rs","model_provider":"example-provider"}}
{"timestamp":"2026-10-01T09:00:01Z","type":"turn_context","payload":{"cwd":"/work/codex-repo","model":"example-model"}}
{"timestamp":"2026-10-01T09:00:02Z","type":"response_item","payload":{"type":"message","id":"msg_user_1","role":"user","content":[{"type":"input_text","text":"Hello"}]}}
{"timestamp":"2026-10-01T09:00:03Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hi"}]}}
`)
	copilotDir := filepath.Join(home, ".copilot", "session-state", copilotID)
	writeFile(t, filepath.Join(copilotDir, "events.jsonl"),
		`{"type":"session.start","id":"evt-1","timestamp":"2026-10-01T07:00:05Z","data":{"startTime":"2026-10-01T07:00:00Z"}}
{"type":"user.message","id":"evt-2","timestamp":"2026-10-01T07:01:00Z","data":{"content":"Hello"}}
`)
	writeFile(t, filepath.Join(copilotDir, "workspace.yaml"), "id: "+copilotID+"\ncwd: /work/copilot-repo/sub\ngit_root: /work/copilot-repo\ncreated_at: 2026-09-30T00:00:00Z\n")

	db := kiroCLIDatabase(t, home)
	created := time.Date(2026, 10, 1, 6, 0, 0, 0, time.UTC)
	_, err := db.Exec("INSERT INTO conversations_v2 VALUES (?, ?, ?, ?, ?)", realProject, "kiro-conversation",
		`{"conversation_id":"kiro-conversation","history":[{"user":{"timestamp":"2026-10-01T06:30:00Z","content":{"Prompt":{"prompt":"Hello"}}}}]}`,
		created.UnixMilli(), created.Add(time.Hour).UnixMilli())
	require.NoError(t, err)

	writeFile(t, filepath.Join(kiroIDEWorkspace(t, project), "session.chat"),
		`{"executionId":"kiro-ide-execution","metadata":{"modelId":"example-model","modelProvider":"example-provider","startTime":1790812800000},"chat":[{"role":"human","content":"Hello"},{"role":"bot","content":"Hi"}]}`)
}

func TestSharedJSON_IdentifierLookups(t *testing.T) {
	home := fakeHome(t)
	realProject, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	project := filepath.Join(t.TempDir(), "project-link")
	require.NoError(t, os.Symlink(realProject, project))
	sessionsForEverySource(t, home, project, realProject)

	for id, want := range map[string]sharedjson.Metadata{
		"claude-session": {Source: "claude", ID: "claude-session", CreatedAt: "2026-10-01T08:00:00Z", Project: "/work/claude-repo", Origin: "cli"},
		codexID:          {Source: "codex", ID: codexID, CreatedAt: "2026-10-01T09:00:00Z", Project: "/work/codex-repo", Model: "example-model", Provider: "example-provider", Origin: "codex_cli_rs"},
		// session.start startTime wins over workspace.yaml created_at (Decision 30).
		copilotID: {Source: "copilot", ID: copilotID, CreatedAt: "2026-10-01T07:00:00Z", Project: "/work/copilot-repo"},
		// The database created_at and matched key, not the first message time (Decision 30).
		"kiro-conversation":  {Source: "kiro-cli", ID: "kiro-conversation", CreatedAt: "2026-10-01T06:00:00Z", Project: realProject},
		"kiro-ide-execution": {Source: "kiro-ide", ID: "kiro-ide-execution", CreatedAt: "2026-10-01T00:00:00Z", Model: "example-model", Provider: "example-provider"},
	} {
		t.Run(want.Source, func(t *testing.T) {
			stdout, _, err := runCLI(t, &Config{Input: id, Format: "shared-json", Project: project}, "")
			require.NoError(t, err)
			assert.Equal(t, want, decodeSession(t, stdout).Session)
		})
	}

	t.Run("copilot file by path has no project", func(t *testing.T) {
		path := filepath.Join(home, ".copilot", "session-state", copilotID, "events.jsonl")
		stdout, _, err := runCLI(t, &Config{Input: path, Format: "shared-json", Project: project}, "")
		require.NoError(t, err)
		assert.Equal(t, sharedjson.Metadata{Source: "copilot", ID: "events", CreatedAt: "2026-10-01T07:00:00Z"}, decodeSession(t, stdout).Session)
	})
}

func TestSharedJSON_StdinAndLatest(t *testing.T) {
	session := `{"type":"user","uuid":"user-1","timestamp":"2026-10-01T08:00:00Z","message":{"role":"user","content":"Hello"}}
`
	t.Run("stdin", func(t *testing.T) {
		fakeHome(t)
		stdout, _, err := runCLI(t, &Config{Format: "shared-json"}, session)
		require.NoError(t, err)
		doc := decodeSession(t, stdout)
		assert.Equal(t, sharedjson.Metadata{Source: "claude", CreatedAt: "2026-10-01T08:00:00Z"}, doc.Session)
		require.Len(t, doc.Entries, 1)
		assert.Equal(t, "user-1", doc.Entries[0].ID)
	})

	t.Run("latest", func(t *testing.T) {
		home := fakeHome(t)
		project := t.TempDir()
		writeFile(t, filepath.Join(home, ".claude", "projects", claudecode.BuildProjectPath(project), "latest-session.jsonl"), session)
		stdout, stderr, err := runCLI(t, &Config{Input: "latest", Format: "shared-json", Project: project}, "")
		require.NoError(t, err)
		assert.Contains(t, stderr, "Using claude session latest-session")
		assert.Equal(t, sharedjson.Metadata{Source: "claude", ID: "latest-session", CreatedAt: "2026-10-01T08:00:00Z"}, decodeSession(t, stdout).Session)
	})
}

func TestSharedJSON_FilesystemTimeIsNeverCreatedAt(t *testing.T) {
	home := fakeHome(t)
	project := t.TempDir()
	path := writeFile(t, filepath.Join(home, ".claude", "projects", claudecode.BuildProjectPath(project), "no-time.jsonl"),
		`{"type":"user","message":{"role":"user","content":"Hello"}}
`)
	modTime := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(path, modTime, modTime))

	stdout, _, err := runCLI(t, &Config{Input: "no-time", Format: "shared-json", Project: project}, "")
	require.NoError(t, err)
	assert.Equal(t, sharedjson.Metadata{Source: "claude", ID: "no-time"}, decodeSession(t, stdout).Session)
}

func TestSharedJSON_EmptySessions(t *testing.T) {
	home := fakeHome(t)
	unknownOnly := writeFile(t, filepath.Join(t.TempDir(), "unknown.jsonl"), `{"type":"future-record"}`+"\n")
	stdout, stderr, err := runCLI(t, &Config{Input: unknownOnly, Format: "shared-json"}, "")
	require.NoError(t, err)
	assert.Contains(t, stdout, `"entries": []`)
	assert.Equal(t, sharedjson.Metadata{ID: "unknown"}, decodeSession(t, stdout).Session, "the Claude fallback claims no source")
	assert.NotContains(t, stderr, "Session contains no entries")

	for _, format := range []string{"md", "html"} {
		stdout, stderr, err = runCLI(t, &Config{Input: unknownOnly, Format: format}, "")
		require.NoError(t, err)
		assert.Empty(t, stdout)
		assert.Contains(t, stderr, "Session contains no entries")
	}

	header := `{"timestamp":"2026-10-01T09:00:00Z","type":"session_meta","payload":{"id":"` + codexID + `","cwd":"/work/codex-repo","originator":"codex_exec","model_provider":"example-provider"}}
{"timestamp":"2026-10-01T09:00:01Z","type":"turn_context","payload":{"model":"example-model"}}
`
	want := sharedjson.Metadata{Source: "codex", CreatedAt: "2026-10-01T09:00:00Z", Project: "/work/codex-repo", Model: "example-model", Provider: "example-provider", Origin: "codex_exec"}
	headerFile := writeFile(t, filepath.Join(home, ".codex", "sessions", "2026", "10", "01", "rollout-"+codexID+".jsonl"), header)
	for name, tc := range map[string]struct {
		input, stdin, id string
	}{
		"file":       {input: headerFile, id: "rollout-" + codexID},
		"stdin":      {stdin: header},
		"identifier": {input: codexID, id: codexID},
	} {
		t.Run("header-only codex "+name, func(t *testing.T) {
			stdout, _, err := runCLI(t, &Config{Input: tc.input, Format: "shared-json", Project: t.TempDir()}, tc.stdin)
			require.NoError(t, err)
			doc := decodeSession(t, stdout)
			expected := want
			expected.ID = tc.id
			assert.Equal(t, expected, doc.Session)
			assert.Empty(t, doc.Entries)
			assert.Contains(t, stdout, `"entries": []`)

			_, _, err = runCLI(t, &Config{Input: tc.input, Format: "md", Project: t.TempDir()}, tc.stdin)
			assert.EqualError(t, err, "failed to parse transcript: no valid entries found in file", "the legacy error is kept")
		})
	}
}

func TestSharedJSON_ModelChangesSplitOnlyInSharedJSON(t *testing.T) {
	input := "../../internal/transcript/testdata/codex/turn_context.jsonl"
	stdout, _, err := runCLI(t, &Config{Input: input, Format: "shared-json"}, "")
	require.NoError(t, err)
	var models []string
	for _, e := range decodeSession(t, stdout).Entries {
		if e.Role == "assistant" {
			models = append(models, e.Model)
		}
	}
	assert.Equal(t, []string{"example-model-a", "example-model-b"}, models)

	markdown, _, err := runCLI(t, &Config{Input: input, Format: "md"}, "")
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(markdown, "## 🤖 Assistant"), "default output keeps legacy grouping")
	assert.Contains(t, markdown, "(example-provider)")
	assert.NotContains(t, markdown, "example-model")
}

func TestSharedJSON_WarningsStayOnStderr(t *testing.T) {
	input := writeFile(t, filepath.Join(t.TempDir(), "warn.jsonl"), `{"type":"user","message":{"role":"user","content":"Hello"}}
not json
`)
	stdout, stderr, err := runCLI(t, &Config{Input: input, Format: "shared-json"}, "")
	require.NoError(t, err)
	assert.Contains(t, stderr, "Warning: line 2: failed to parse JSON")
	assert.Contains(t, stderr, "Parsed with 1 warning(s)")
	doc := decodeSession(t, stdout)
	require.Len(t, doc.Warnings, 1)
	assert.Equal(t, sharedjson.Warning{Code: "malformed_line", Message: doc.Warnings[0].Message, Line: 2}, doc.Warnings[0])
}

func TestSharedJSON_UnsupportedCombinations(t *testing.T) {
	err := validateFollowMode(&Config{Follow: true, Format: "shared-json"})
	assert.EqualError(t, err, "follow mode does not support -f shared-json")

	_, err = run(&Config{Input: "any-session", Follow: true, Format: "shared-json"})
	assert.EqualError(t, err, "follow mode does not support -f shared-json")

	err = convert(strings.NewReader(`{"type":"user","message":{"role":"user","content":"Hello"}}`), io.Discard, inputContext{}, "xml", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "shared-json")
}

// Raw -f json passes source records through for every existing source.
func TestRawJSON_UnchangedForEverySource(t *testing.T) {
	for _, fixture := range []string{"claude.jsonl", "codex.jsonl", "copilot.jsonl", "kiro.json", "kiro_ide.json"} {
		t.Run(fixture, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", "internal", "transcript", "testdata", "integration", fixture))
			require.NoError(t, err)
			var output strings.Builder
			require.NoError(t, convert(strings.NewReader(string(data)), &output, inputContext{}, "json", ""))
			assert.NotContains(t, output.String(), "schema_version")

			if strings.HasSuffix(fixture, ".json") {
				assert.JSONEq(t, string(data), output.String())
				return
			}
			var records []json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(output.String()), &records))
			var lines []string
			for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
				if strings.TrimSpace(line) != "" {
					lines = append(lines, line)
				}
			}
			require.Len(t, records, len(lines))
			for i, line := range lines {
				assert.JSONEq(t, line, string(records[i]))
			}
		})
	}
}
