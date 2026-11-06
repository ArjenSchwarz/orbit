package apsisweb

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/arjenschwarz/orbit/internal/agents/claudecode"
)

// allProjectsHome writes a Claude session for project, recorded in
// /work/claude-repo, and a Copilot session without workspace metadata under a
// new HOME.
func allProjectsHome(t *testing.T, project string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	write := func(path, content string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	write(filepath.Join(home, ".claude", "projects", claudecode.BuildProjectPath(project), "claude-session.jsonl"),
		`{"type":"user","timestamp":"2026-10-01T08:00:00Z","cwd":"/work/claude-repo","message":{"role":"user","content":"Hello"}}`+"\n")
	write(filepath.Join(home, ".copilot", "session-state", "12345678-aaaa-bbbb-cccc-123456789abc", "events.jsonl"),
		`{"type":"session.start"}`+"\n")
}

func TestAllProjectsListingShowsEachRowsProject(t *testing.T) {
	project := t.TempDir()
	allProjectsHome(t, project)
	s, err := New(Config{Bind: "127.0.0.1", AllProjects: true})
	require.NoError(t, err)

	for _, path := range []string{"/", "/api/sessions"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			s.router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			require.Equal(t, http.StatusOK, rec.Code)
			body := rec.Body.String()
			assert.Contains(t, body, `href="/sessions/copilot/12345678-aaaa-bbbb-cccc-123456789abc"`,
				"sessions outside the working directory's project are listed")
			assert.Contains(t, body, `<span class="session-project" title="/work/claude-repo">/work/claude-repo</span>`)
			assert.Contains(t, body, `<span class="session-project">-</span>`, "an unrecorded project shows as -")
		})
	}
}

func TestProjectScopeListingHasNoProjectColumn(t *testing.T) {
	project := t.TempDir()
	allProjectsHome(t, project)
	s, err := New(Config{Bind: "127.0.0.1", ProjectPath: project})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `href="/sessions/claude/claude-session"`)
	assert.NotContains(t, body, "copilot/12345678", "project scope keeps filtering by project")
	assert.NotContains(t, body, "session-project", "project scope pages are unchanged")
}
