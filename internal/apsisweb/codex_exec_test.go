package apsisweb

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/arjenschwarz/orbit/internal/agents/claudecode"
	"github.com/arjenschwarz/orbit/internal/sessions"
)

const execStreamWithWarning = `{"type":"thread.started","thread_id":"0199aaaa-0000-7000-8000-000000000001"}
{"type":"turn.started"}
{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"Starting."}}
{"type":"item.completed","item":
`

// execWeb writes an exec stream into a repository under a new HOME and
// returns the stream path and the repository root.
func execWeb(t *testing.T) (stream, repo string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	t.Setenv("HOME", filepath.Join(root, "home"))
	repo = filepath.Join(root, "repo")
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".git"), 0o755))
	stream = filepath.Join(repo, "logs", "run.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(stream), 0o755))
	require.NoError(t, os.WriteFile(stream, []byte(execStreamWithWarning), 0o600))
	modified := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(stream, modified, modified))
	return stream, repo
}

func fetch(t *testing.T, srv *Server, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	body, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	return rec.Code, string(body)
}

func TestCodexExecPillOnlyWithRoots(t *testing.T) {
	_, repo := execWeb(t)
	for name, tc := range map[string]struct {
		roots []string
		want  bool
	}{
		"no roots":   {},
		"with roots": {roots: []string{repo}, want: true},
	} {
		t.Run(name, func(t *testing.T) {
			srv, err := New(Config{AllProjects: true, CodexExecRoots: tc.roots})
			require.NoError(t, err)
			_, page := fetch(t, srv, "/")
			assert.Equal(t, tc.want, strings.Contains(page, `data-source="codex-exec"`))
		})
	}
}

func TestCodexExecListingMarksFileTime(t *testing.T) {
	stream, repo := execWeb(t)
	srv, err := New(Config{ProjectPath: repo, CodexExecRoots: []string{repo}})
	require.NoError(t, err)
	_, page := fetch(t, srv, "/api/sessions")
	id := sessions.ExecIdentifier("0199aaaa-0000-7000-8000-000000000001", stream)
	assert.Contains(t, page, `href="/sessions/codex-exec/`+id+`"`)
	assert.Contains(t, page, `<span class="session-file-time">file modified</span>`)
}

func TestWarningBannerFollowsEachRefresh(t *testing.T) {
	execWeb(t)
	missing := filepath.Join(t.TempDir(), "missing")
	srv, err := New(Config{AllProjects: true, CodexExecRoots: []string{missing}})
	require.NoError(t, err)

	_, page := fetch(t, srv, "/")
	assert.Equal(t, 1, strings.Count(page, `class="warning-banner"`), "the page includes the fragment's banner once")
	_, fragment := fetch(t, srv, "/api/sessions")
	assert.Contains(t, fragment, `class="warning-banner"`)
	assert.Contains(t, fragment, "Could not load codex-exec sessions")
}

func TestCodexExecTranscriptView(t *testing.T) {
	stream, repo := execWeb(t)
	srv, err := New(Config{ProjectPath: repo, CodexExecRoots: []string{repo}})
	require.NoError(t, err)
	id := sessions.ExecIdentifier("0199aaaa-0000-7000-8000-000000000001", stream)

	code, page := fetch(t, srv, "/sessions/codex-exec/"+id)
	require.Equal(t, http.StatusOK, code, page)
	assert.Contains(t, page, `class="warning-banner"`)
	assert.Contains(t, page, "line 4: malformed JSON event")
	assert.Contains(t, page, "Thread ID 0199aaaa-0000-7000-8000-000000000001")
	assert.Contains(t, page, `datetime="2026-10-01T09:00:00Z"`)
	assert.Contains(t, page, "file modified")
	assert.Contains(t, page, "Starting.")
}

func TestOtherTranscriptViewsShowNoParseWarnings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := t.TempDir()
	dir := filepath.Join(home, ".claude", "projects", claudecode.BuildProjectPath(project))
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "claude-session.jsonl"),
		[]byte(`{"type":"user","timestamp":"2026-10-01T08:00:00Z","message":{"role":"user","content":"Hello"}}`+"\nnot json\n"), 0o600))
	srv, err := New(Config{ProjectPath: project})
	require.NoError(t, err)

	code, page := fetch(t, srv, "/sessions/claude/claude-session")
	require.Equal(t, http.StatusOK, code)
	assert.NotContains(t, page, "warning-banner")
	assert.NotContains(t, page, "Thread ID")
	assert.NotContains(t, page, "file modified")
}

func TestCodexExecBadgeStyles(t *testing.T) {
	css, err := os.ReadFile(filepath.Join("static", "style.css"))
	require.NoError(t, err)
	assert.Equal(t, 2, strings.Count(string(css), ".source-codex-exec {"), "light and dark badge rules")
	assert.Equal(t, "3", CSSVersion, "the stylesheet changed, so its cache-busting version rises")
}
