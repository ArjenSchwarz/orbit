package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/arjenschwarz/orbit/internal/apsisweb"
	"github.com/arjenschwarz/orbit/internal/sessions"
)

func TestCodexExecIdentifier_Opening(t *testing.T) {
	home := fakeHome(t)
	path, repo := execRepo(t, execFixture(t, "all_items.jsonl"))
	id := sessions.ExecIdentifier(execThreadID, path)
	roots := []string{repo}
	t.Chdir(t.TempDir())

	for name, cfg := range map[string]*Config{
		"source in project scope":      {Source: "codex-exec", Input: id},
		"source across projects":       {AllProjects: true, Source: "codex-exec", Input: id},
		"bare identifier tried last":   {Input: id},
		"bare identifier all projects": {AllProjects: true, Input: id},
	} {
		t.Run(name, func(t *testing.T) {
			cfg.Format, cfg.CodexExecDirs = "shared-json", roots
			stdout, _, err := runCLI(t, cfg, "")
			require.NoError(t, err)
			doc := decodeSession(t, stdout)
			assert.Equal(t, "codex-exec", doc.Session.Source)
			assert.Equal(t, id, doc.Session.ID)
			assert.Equal(t, execThreadID, doc.Session.ThreadID)
			assert.Equal(t, repo, doc.Session.Project)
			assert.Empty(t, doc.Session.CreatedAt, "a modification time is never created_at")
		})
	}

	t.Run("markdown header", func(t *testing.T) {
		stdout, _, err := runCLI(t, &Config{Source: "codex-exec", Input: id, Format: "md", CodexExecDirs: roots}, "")
		require.NoError(t, err)
		assert.Contains(t, stdout, "**Session ID:** `"+id+"`")
		assert.Contains(t, stdout, "**Thread ID:** `"+execThreadID+"`")
	})

	t.Run("without roots the identifier is not found", func(t *testing.T) {
		_, _, err := runCLI(t, &Config{Input: id, Format: "md", CodexExecDirs: []string{""}}, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "session not found")
	})

	t.Run("suppressed streams still open by identifier", func(t *testing.T) {
		writeFile(t, filepath.Join(home, ".codex", "sessions", "2026", "10", "01", "rollout-"+execThreadID+".jsonl"),
			`{"timestamp":"2026-10-01T09:00:00Z","type":"session_meta","payload":{"id":"`+execThreadID+`","cwd":"/work/other"}}`+"\n")
		stdout, _, err := runCLIStdout(t, &Config{List: true, AllProjects: true, Format: "md", CodexExecDirs: roots})
		require.NoError(t, err)
		assert.NotContains(t, stdout, "codex-exec")
		out, _, err := runCLI(t, &Config{Source: "codex-exec", Input: id, Format: "shared-json", CodexExecDirs: roots}, "")
		require.NoError(t, err)
		assert.Equal(t, id, decodeSession(t, out).Session.ID)
	})
}

func TestCodexExecFollowAndLatest(t *testing.T) {
	fakeHome(t)
	path, repo := execRepo(t, execFixture(t, "all_items.jsonl"))
	modified := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(path, modified, modified))
	id := sessions.ExecIdentifier(execThreadID, path)
	roots := []string{repo}
	t.Chdir(t.TempDir())

	for name, cfg := range map[string]*Config{
		"source identifier": {Source: "codex-exec", Input: id},
		"bare identifier":   {Input: id},
		"latest":            {Input: "latest", AllProjects: true},
	} {
		t.Run("follow rejects "+name, func(t *testing.T) {
			cfg.Follow, cfg.Format, cfg.CodexExecDirs = true, "md", roots
			_, err := run(cfg)
			require.Error(t, err)
			assert.Equal(t, "follow mode does not support Codex exec streams", err.Error())
		})
	}

	t.Run("latest marks the filesystem time", func(t *testing.T) {
		_, stderr, err := runCLI(t, &Config{Input: "latest", AllProjects: true, Format: "md", CodexExecDirs: roots}, "")
		require.NoError(t, err)
		assert.Contains(t, stderr, "Using codex-exec session "+id+" from "+modified.Local().Format(time.RFC3339)+" (file modified)\n")
	})
}

// headerOnlyCodexHome writes a Codex rollout recorded in /work/codex-repo
// that holds only a header and a turn_context.
func headerOnlyCodexHome(t *testing.T) {
	t.Helper()
	home := fakeHome(t)
	writeFile(t, filepath.Join(home, ".codex", "sessions", "2026", "10", "01", "rollout-"+codexID+".jsonl"),
		`{"timestamp":"2026-10-01T09:00:00Z","type":"session_meta","payload":{"id":"`+codexID+`","cwd":"/work/codex-repo","model_provider":"example-provider"}}
{"timestamp":"2026-10-01T09:00:01Z","type":"turn_context","payload":{"model":"example-model"}}
`)
}

// The shared parse helper must forward the shared-JSON Codex flags for every
// resolved input, while other formats and the web keep their output.
func TestParseResolvedRegression_HeaderOnlyCodex(t *testing.T) {
	headerOnlyCodexHome(t)
	for name, cfg := range map[string]Config{
		"source identifier": {Source: "codex", Input: codexID, AllProjects: true},
		"latest":            {Input: "latest", AllProjects: true},
	} {
		t.Run(name, func(t *testing.T) {
			shared := cfg
			shared.Format = "shared-json"
			stdout, _, err := runCLI(t, &shared, "")
			require.NoError(t, err)
			doc := decodeSession(t, stdout)
			assert.Equal(t, "example-model", doc.Session.Model)
			assert.Contains(t, stdout, `"entries": []`)

			for _, format := range []string{"md", "html"} {
				legacy := cfg
				legacy.Format = format
				_, _, err = runCLI(t, &legacy, "")
				assert.EqualError(t, err, "failed to parse transcript: no valid entries found in file")
			}
		})
	}

	server, err := apsisweb.New(apsisweb.Config{AllProjects: true})
	require.NoError(t, err)
	web := httptest.NewServer(server.Handler())
	t.Cleanup(web.Close)
	resp, err := web.Client().Get(web.URL + "/sessions/codex/" + codexID)
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, 500, resp.StatusCode, "the web keeps its parse error")
}

func TestParseResolvedRegression_ModelChanges(t *testing.T) {
	home := fakeHome(t)
	data, err := os.ReadFile("../../internal/transcript/testdata/codex/turn_context.jsonl")
	require.NoError(t, err)
	id := sessionIDFromFixture(t, string(data))
	writeFile(t, filepath.Join(home, ".codex", "sessions", "2026", "10", "01", "rollout-"+id+".jsonl"), string(data))

	for name, cfg := range map[string]Config{
		"source identifier": {Source: "codex", Input: id, AllProjects: true},
		"latest":            {Input: "latest", AllProjects: true},
	} {
		t.Run(name, func(t *testing.T) {
			shared := cfg
			shared.Format = "shared-json"
			stdout, _, err := runCLI(t, &shared, "")
			require.NoError(t, err)
			var models []string
			for _, e := range decodeSession(t, stdout).Entries {
				if e.Role == "assistant" {
					models = append(models, e.Model)
				}
			}
			assert.Equal(t, []string{"example-model-a", "example-model-b"}, models)

			legacy := cfg
			legacy.Format = "md"
			markdown, _, err := runCLI(t, &legacy, "")
			require.NoError(t, err)
			assert.Equal(t, 1, strings.Count(markdown, "## 🤖 Assistant"), "default output keeps legacy grouping")
			assert.NotContains(t, markdown, "example-model")
		})
	}

	server, err := apsisweb.New(apsisweb.Config{AllProjects: true})
	require.NoError(t, err)
	web := httptest.NewServer(server.Handler())
	t.Cleanup(web.Close)
	body := get(t, web.URL+"/sessions/codex/"+id)
	assert.NotContains(t, body, "example-model", "the web keeps legacy header text")
}

// sessionIDFromFixture returns the session_meta id a rollout fixture records.
func sessionIDFromFixture(t *testing.T, rollout string) string {
	t.Helper()
	const key = `"id":"`
	i := strings.Index(rollout, key)
	require.GreaterOrEqual(t, i, 0)
	rest := rollout[i+len(key):]
	return rest[:strings.IndexByte(rest, '"')]
}

func TestExplicitAgentPrecedesResolvedSource(t *testing.T) {
	headerOnlyCodexHome(t)
	stdout, _, err := runCLI(t, &Config{Source: "codex", Input: codexID, AllProjects: true, Format: "md", Agent: "codex-exec"}, "")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Event: session_meta", "the forced exec parser reads the rollout lines as generic events")
}

// TestCodexExec_RoundTrip covers 5.8 for exec streams: duplicate and resumed
// streams, a rollout in another project, overlapping roots, an escaping
// symlink and a directory cycle; every listed row opens from an unrelated
// directory through the CLI and the web.
func TestCodexExec_RoundTrip(t *testing.T) {
	home := fakeHome(t)
	path, repo := execRepo(t, execFixture(t, "all_items.jsonl"))
	copyPath := writeFile(t, filepath.Join(repo, "archive", "copy.jsonl"), execFixture(t, "all_items.jsonl"))
	resumed := writeFile(t, filepath.Join(repo, "logs", "resumed.jsonl"), execFixture(t, "turns.jsonl"))
	suppressed := writeFile(t, filepath.Join(repo, "logs", "suppressed.jsonl"), execFixture(t, "failures.jsonl"))
	writeFile(t, filepath.Join(home, ".codex", "sessions", "2026", "10", "01", "rollout-0199aaaa-0000-7000-8000-000000000003.jsonl"),
		`{"timestamp":"2026-10-01T09:00:00Z","type":"session_meta","payload":{"id":"0199aaaa-0000-7000-8000-000000000003","cwd":"/work/other-project"}}`+"\n")
	outside := writeFile(t, filepath.Join(filepath.Dir(repo), "outside", "escape.jsonl"), execFixture(t, "all_items.jsonl"))
	require.NoError(t, os.Symlink(outside, filepath.Join(repo, "logs", "escape.jsonl")))
	require.NoError(t, os.Symlink(repo, filepath.Join(repo, "logs", "cycle")))
	roots := []string{repo, filepath.Join(repo, "logs")}
	t.Chdir(t.TempDir())

	stdout, _, err := runCLIStdout(t, &Config{List: true, AllProjects: true, Format: "shared-json", CodexExecDirs: roots})
	require.NoError(t, err)
	listing := decodeListing(t, stdout)
	var cliRows []string
	for _, row := range listing.Sessions {
		if row.Source != "codex-exec" {
			continue
		}
		cliRows = append(cliRows, row.ID)
		out, _, err := runCLI(t, &Config{AllProjects: true, Source: row.Source, Input: row.ID, Format: "shared-json", CodexExecDirs: roots}, "")
		require.NoError(t, err, row.ID)
		assert.Equal(t, row.ID, decodeSession(t, out).Session.ID)
	}
	want := []string{
		sessions.ExecIdentifier(execThreadID, path),
		sessions.ExecIdentifier(execThreadID, copyPath),
		sessions.ExecIdentifier("0199aaaa-0000-7000-8000-000000000002", resumed),
	}
	slices.Sort(want)
	slices.Sort(cliRows)
	assert.Equal(t, want, cliRows, "copies list separately; the suppressed stream, escaping symlink and cycle do not list")

	out, _, err := runCLI(t, &Config{AllProjects: true, Source: "codex-exec", Format: "shared-json", CodexExecDirs: roots,
		Input: sessions.ExecIdentifier("0199aaaa-0000-7000-8000-000000000003", suppressed)}, "")
	require.NoError(t, err, "the suppressed stream still opens by identifier")
	assert.Equal(t, "codex-exec", decodeSession(t, out).Session.Source)

	server, err := apsisweb.New(apsisweb.Config{AllProjects: true, CodexExecRoots: roots})
	require.NoError(t, err)
	web := httptest.NewServer(server.Handler())
	t.Cleanup(web.Close)
	var webRows []string
	for _, m := range sessionLink.FindAllStringSubmatch(get(t, web.URL+"/"), -1) {
		if m[1] != "codex-exec" {
			continue
		}
		webRows = append(webRows, m[2])
		assert.Contains(t, get(t, web.URL+"/sessions/codex-exec/"+m[2]), m[2])
	}
	slices.Sort(webRows)
	assert.Equal(t, cliRows, webRows)
}
