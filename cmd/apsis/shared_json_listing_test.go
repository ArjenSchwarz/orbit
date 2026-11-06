package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/arjenschwarz/orbit/internal/sharedjson"
)

// runCLIStdout runs apsis with cfg and returns what it writes to stdout and stderr.
func runCLIStdout(t *testing.T, cfg *Config) (stdout, stderr string, err error) {
	t.Helper()
	capture := func(target **os.File) func() string {
		r, w, pipeErr := os.Pipe()
		require.NoError(t, pipeErr)
		old := *target
		*target = w
		done := make(chan string)
		go func() {
			data, _ := io.ReadAll(r)
			done <- string(data)
		}()
		return func() string {
			_ = w.Close()
			*target = old
			return <-done
		}
	}
	stopStdout := capture(&os.Stdout)
	stopStderr := capture(&os.Stderr)
	exitCode, err := run(cfg)
	stderr = stopStderr()
	stdout = stopStdout()
	assert.Zero(t, exitCode)
	return stdout, stderr, err
}

func decodeListing(t *testing.T, stdout string) sharedjson.ListingDocument {
	t.Helper()
	var doc sharedjson.ListingDocument
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc), stdout)
	assert.Equal(t, sharedjson.SchemaVersion, doc.SchemaVersion)
	assert.Equal(t, "listing", doc.Kind)
	require.NotNil(t, doc.Sessions, "sessions is [] rather than null")
	require.NotNil(t, doc.Warnings, "warnings is [] rather than null")
	return doc
}

// codexRollout writes a Codex rollout recorded at 2026-10-0<day>T09:00:00Z for project.
func codexRollout(t *testing.T, home, day, id, project string) {
	t.Helper()
	writeFile(t, filepath.Join(home, ".codex", "sessions", "2026", "10", day, "rollout-"+id+".jsonl"),
		`{"timestamp":"2026-10-01T09:00:00Z","type":"session_meta","payload":{"id":"`+id+`","cwd":"`+project+`","originator":"codex_exec","model_provider":"example-provider"}}
{"timestamp":"2026-10-01T09:00:01Z","type":"turn_context","payload":{"model":"example-model"}}
`)
}

func TestSharedJSONListing_ProjectScope(t *testing.T) {
	home := fakeHome(t)
	project, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	sessionsForEverySource(t, home, project, project)
	const projectCodexID = "0199dddd-0000-7000-8000-000000000004"
	codexRollout(t, home, "02", projectCodexID, project)

	stdout, _, err := runCLIStdout(t, &Config{List: true, Format: "shared-json", Project: project})
	require.NoError(t, err)
	doc := decodeListing(t, stdout)
	assert.Equal(t, sharedjson.Scope{Project: project}, doc.Scope)

	rows := map[string]sharedjson.ListingRow{}
	for _, row := range doc.Sessions {
		assert.Positive(t, row.SizeBytes, "%s/%s", row.Source, row.ID)
		row.SizeBytes = 0
		rows[row.Source+"/"+row.ID] = row
	}
	row := func(source, id string, meta sharedjson.Metadata) sharedjson.ListingRow {
		meta.Source, meta.ID = source, id
		return sharedjson.ListingRow{Metadata: meta}
	}
	assert.Equal(t, map[string]sharedjson.ListingRow{
		"claude/claude-session": row("claude", "claude-session", sharedjson.Metadata{
			CreatedAt: "2026-10-01T08:00:00Z", Project: "/work/claude-repo", Origin: "cli"}),
		"codex/" + projectCodexID: row("codex", projectCodexID, sharedjson.Metadata{
			CreatedAt: "2026-10-01T09:00:00Z", Project: project, Model: "example-model", Provider: "example-provider", Origin: "codex_exec"}),
		"kiro-cli/kiro-conversation": row("kiro-cli", "kiro-conversation", sharedjson.Metadata{
			CreatedAt: "2026-10-01T06:00:00Z", Project: project}),
		"kiro-ide/kiro-ide-execution": row("kiro-ide", "kiro-ide-execution", sharedjson.Metadata{
			CreatedAt: time.UnixMilli(1790812800000).UTC().Format(time.RFC3339Nano), Model: "example-model", Provider: "example-provider"}),
	}, rows)
}

func TestSharedJSONListing_EmptyAndFilesystemFallback(t *testing.T) {
	home := fakeHome(t)
	project, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	stdout, _, err := runCLIStdout(t, &Config{List: true, Format: "shared-json", Project: project})
	require.NoError(t, err)
	assert.Empty(t, decodeListing(t, stdout).Sessions, "an empty listing is a document, not \"no sessions found\"")

	path := writeFile(t, filepath.Join(home, ".claude", "projects", claudeProjectPath(project), "no-time.jsonl"),
		`{"type":"user","message":{"role":"user","content":"Hello"}}`+"\n")
	modTime := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(path, modTime, modTime))

	stdout, _, err = runCLIStdout(t, &Config{List: true, Format: "shared-json", Project: project})
	require.NoError(t, err)
	doc := decodeListing(t, stdout)
	require.Len(t, doc.Sessions, 1)
	assert.Empty(t, doc.Sessions[0].CreatedAt, "a modification time is never created_at")
	assert.Equal(t, "2026-10-01T12:00:00Z", doc.Sessions[0].FileModifiedAt)
}

// TestSharedJSONListing_ExtendedOrderWhileLatestKeepsLegacyTieOrder covers
// Q83: shared JSON listings break equal times by source and identifier, while
// apsis latest and the text listing keep the legacy order in project scope.
func TestSharedJSONListing_ExtendedOrderWhileLatestKeepsLegacyTieOrder(t *testing.T) {
	home := fakeHome(t)
	project, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	// Walk order (by day directory) is the reverse of identifier order.
	const walkFirst = "ffffffff-0000-7000-8000-000000000001"
	const walkSecond = "00000000-0000-7000-8000-000000000002"
	codexRollout(t, home, "01", walkFirst, project)
	codexRollout(t, home, "02", walkSecond, project)

	stdout, _, err := runCLIStdout(t, &Config{List: true, Format: "shared-json", Project: project})
	require.NoError(t, err)
	var ids []string
	for _, row := range decodeListing(t, stdout).Sessions {
		ids = append(ids, row.ID)
	}
	assert.Equal(t, []string{walkSecond, walkFirst}, ids, "shared JSON listings use extended ordering")

	text, _, err := runCLIStdout(t, &Config{List: true, Project: project})
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(text), "\n")
	require.Len(t, lines, 2)
	for i, want := range []string{walkFirst, walkSecond} {
		fields := strings.Split(lines[i], "\t")
		require.Len(t, fields, 4, "text listings keep four columns: %q", lines[i])
		assert.Equal(t, "[codex]", fields[0])
		assert.Equal(t, want, fields[1], "text listings keep the legacy order")
	}

	stdout, stderr, err := runCLI(t, &Config{Input: "latest", Format: "shared-json", Project: project}, "")
	require.NoError(t, err)
	assert.Contains(t, stderr, "Using codex session "+walkSecond)
	assert.Equal(t, walkSecond, decodeSession(t, stdout).Session.ID, "latest keeps the legacy tie order whatever the format")
}
