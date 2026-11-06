package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/arjenschwarz/orbit/internal/apsisweb"
	"github.com/arjenschwarz/orbit/internal/sharedjson"
	"github.com/arjenschwarz/orbit/internal/transcript"
)

func TestScopeAndSource_UsageErrors(t *testing.T) {
	fakeHome(t)
	tests := map[string]struct {
		cfg     Config
		wantErr string
	}{
		"all projects with a project":  {Config{List: true, AllProjects: true, Project: t.TempDir()}, "--all-projects"},
		"source without an identifier": {Config{Source: "claude"}, "--source"},
		"unknown source":               {Config{Source: "claude-code", Input: "some-id"}, "unknown source"},
		"source with list":             {Config{Source: "claude", List: true}, "--list"},
		"source with latest":           {Config{Source: "claude", Input: "latest"}, "latest"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, _, err := runCLIStdout(t, &tc.cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}

	t.Run("source with stdin", func(t *testing.T) {
		_, _, err := runCLI(t, &Config{Source: "claude"}, `{"type":"user","message":{"role":"user","content":"Hello"}}`+"\n")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--source")
	})

	t.Run("serve with all projects and a project", func(t *testing.T) {
		err := serveCommand([]string{"--all-projects", "--project", t.TempDir()})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--all-projects")
	})
}

func TestAllProjects_TextListingAddsAProjectColumn(t *testing.T) {
	home := fakeHome(t)
	project, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	sessionsForEverySource(t, home, project, project)
	t.Chdir(t.TempDir())

	stdout, _, err := runCLIStdout(t, &Config{List: true, AllProjects: true})
	require.NoError(t, err)

	projects := map[string]string{}
	for line := range strings.SplitSeq(strings.TrimSpace(stdout), "\n") {
		fields := strings.Split(line, "\t")
		require.Len(t, fields, 5, "all-projects listings add a fifth column: %q", line)
		projects[fields[0]] = fields[4]
	}
	assert.Equal(t, map[string]string{
		"[claude]":   "/work/claude-repo",
		"[codex]":    "/work/codex-repo",
		"[copilot]":  "/work/copilot-repo",
		"[kiro-cli]": project,
		"[kiro ide]": "-",
	}, projects)
}

func TestSource_SelectsAmongIdenticalIdentifiers(t *testing.T) {
	home := fakeHome(t)
	project, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	writeFile(t, filepath.Join(home, ".claude", "projects", claudeProjectPath(project), "shared-id.jsonl"),
		`{"type":"user","timestamp":"2026-10-01T08:00:00Z","message":{"role":"user","content":"Hello"}}`+"\n")
	writeFile(t, filepath.Join(home, ".codex", "sessions", "2026", "10", "01", "shared-id.jsonl"),
		`{"timestamp":"2026-10-01T09:00:00Z","type":"session_meta","payload":{"id":"shared-id","cwd":"`+project+`"}}
{"timestamp":"2026-10-01T09:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Hello"}]}}
`)

	source := func(cfg Config) string {
		t.Helper()
		cfg.Format = "shared-json"
		stdout, _, err := runCLI(t, &cfg, "")
		require.NoError(t, err)
		return decodeSession(t, stdout).Session.Source
	}

	assert.Equal(t, "claude", source(Config{Input: "shared-id", Project: project}), "bare identifiers keep the existing source order")
	assert.Equal(t, "codex", source(Config{Input: "shared-id", Project: project, Source: "codex"}))
	t.Chdir(t.TempDir())
	assert.Equal(t, "codex", source(Config{Input: "shared-id", AllProjects: true, Source: "codex"}))
	assert.Equal(t, "claude", source(Config{Input: "shared-id", AllProjects: true}))
}

func TestAllProjects_LatestUsesExtendedOrdering(t *testing.T) {
	home := fakeHome(t)
	// Equal recorded times in two projects; walk order is the reverse of identifier order.
	const walkFirst = "ffffffff-0000-7000-8000-000000000001"
	const walkSecond = "00000000-0000-7000-8000-000000000002"
	codexRollout(t, home, "01", walkFirst, "/work/one")
	codexRollout(t, home, "02", walkSecond, "/work/two")
	t.Chdir(t.TempDir())

	stdout, stderr, err := runCLI(t, &Config{Input: "latest", AllProjects: true, Format: "shared-json"}, "")
	require.NoError(t, err)
	assert.Contains(t, stderr, "Using codex session "+walkFirst, "equal times break by identifier, so the larger one is latest")
	assert.Equal(t, walkFirst, decodeSession(t, stdout).Session.ID)
}

// sessionLink matches session links in the web listing.
var sessionLink = regexp.MustCompile(`href="/sessions/([^/"]+)/([^"]+)"`)

// TestAllProjects_RoundTripEverySource covers 5.8 for the existing sources:
// list across projects through the CLI and the web, then open every row from
// an unrelated working directory through both.
func TestAllProjects_RoundTripEverySource(t *testing.T) {
	home := fakeHome(t)
	project, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	sessionsForEverySource(t, home, project, project)
	workspace := kiroIDEWorkspace(t, project)
	writeFile(t, transcript.KiroIDEExecutionDetailPath(workspace, "kiro-ide-execution"),
		`{"executionId":"kiro-ide-execution","usageSummary":[{"unit":"credit","unitPlural":"credits","usage":0.5}],"actions":[{"actionId":"action-1","actionType":"say","output":{"message":"Done"}}]}`)
	t.Chdir(t.TempDir())

	stdout, _, err := runCLIStdout(t, &Config{List: true, AllProjects: true, Format: "shared-json"})
	require.NoError(t, err)
	listing := decodeListing(t, stdout)
	assert.Equal(t, sharedjson.Scope{AllProjects: true}, listing.Scope)
	assert.Empty(t, listing.Warnings)

	var cliRows []string
	for _, row := range listing.Sessions {
		cliRows = append(cliRows, row.Source+"/"+row.ID)
		out, _, err := runCLI(t, &Config{AllProjects: true, Source: row.Source, Input: row.ID, Format: "shared-json"}, "")
		require.NoError(t, err, row.Source+"/"+row.ID)
		doc := decodeSession(t, out)
		assert.Equal(t, row.Source, doc.Session.Source)
		assert.Equal(t, row.ID, doc.Session.ID)
		if row.Source == "kiro-ide" {
			assert.Equal(t, &sharedjson.Cost{Amount: 0.5, Unit: "credits"}, doc.Session.Cost, "the execution detail beside the selected chat is read")
			require.NotEmpty(t, doc.Entries)
			assert.Equal(t, "action-1", doc.Entries[len(doc.Entries)-1].ID)
		}
	}
	slices.Sort(cliRows)
	assert.Equal(t, []string{
		"claude/claude-session",
		"codex/" + codexID,
		"copilot/" + copilotID,
		"kiro-cli/kiro-conversation",
		"kiro-ide/kiro-ide-execution",
	}, cliRows)

	server, err := apsisweb.New(apsisweb.Config{AllProjects: true})
	require.NoError(t, err)
	web := httptest.NewServer(server.Handler())
	t.Cleanup(web.Close)

	page := get(t, web.URL+"/")
	var webRows []string
	for _, m := range sessionLink.FindAllStringSubmatch(page, -1) {
		webRows = append(webRows, m[1]+"/"+m[2])
		body := get(t, web.URL+"/sessions/"+m[1]+"/"+m[2])
		assert.Contains(t, body, m[2], "the transcript page names the session")
	}
	slices.Sort(webRows)
	assert.Equal(t, cliRows, webRows, "the web lists the same sessions as the CLI")
	assert.Contains(t, page, "/work/claude-repo", "all-projects web listings show each row's project")
}

func get(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url) //nolint:gosec,noctx // test server URL
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s: %s", url, body)
	return string(body)
}
