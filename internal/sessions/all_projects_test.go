package sessions

import (
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/arjenschwarz/orbit/internal/transcript"
)

// kiroCLITestDB creates the Kiro CLI database where logs.DBPath finds it under home.
func kiroCLITestDB(t *testing.T, home string) *sql.DB {
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

// kiroIDEBaseUnder returns the Kiro IDE storage directory under the current
// (isolated) HOME, creating it.
func kiroIDEBaseUnder(t *testing.T) string {
	t.Helper()
	configDir, err := os.UserConfigDir()
	require.NoError(t, err)
	base := filepath.Join(configDir, "Kiro", "User", "globalStorage", "kiro.kiroagent")
	require.NoError(t, os.MkdirAll(base, 0o755))
	return base
}

// kiroIDEChat writes a .chat file for execution id with n chat messages.
func kiroIDEChat(t *testing.T, path, id string, n int, modTime time.Time) {
	t.Helper()
	chat := `[`
	for i := range n {
		if i > 0 {
			chat += `,`
		}
		chat += `{"role":"human","content":"Hello"}`
	}
	writeLines(t, path, `{"executionId":"`+id+`","chat":`+chat+`]}`)
	require.NoError(t, os.Chtimes(path, modTime, modTime))
}

// warningCodes returns the codes of the warnings from source.
func warningCodes(warnings []ListWarning, source string) []string {
	var codes []string
	for _, w := range warnings {
		if w.Source == source {
			codes = append(codes, w.Code)
		}
	}
	return codes
}

func codexMetaLine(id, cwd string) string {
	return `{"timestamp":"2026-10-01T09:00:00Z","type":"session_meta","payload":{"id":"` + id + `","cwd":"` + cwd + `"}}`
}

func TestAllProjects_ClaudeDuplicateKeepsTheResolverCandidate(t *testing.T) {
	home := isolateHome(t)
	for dir, cwd := range map[string]string{"-work-a": "/work/a", "-work-b": "/work/b"} {
		writeLines(t, filepath.Join(home, ".claude", "projects", dir, "dup.jsonl"),
			`{"type":"user","timestamp":"2026-10-01T08:00:00Z","cwd":"`+cwd+`","message":{"role":"user","content":"Hello"}}`)
	}

	list, warnings, err := newTestLister(home).ListAll("")
	require.NoError(t, err)
	rows := onlySource(list, SourceClaude)
	require.Len(t, rows, 1)
	assert.Equal(t, "/work/a", rows[0].Project, "the first project directory in sorted order wins")
	assert.Equal(t, []string{WarningDuplicateSession}, warningCodes(warnings, SourceClaude))

	resolved, err := newTestResolver("", home).Resolve(SourceClaude, "dup")
	require.NoError(t, err)
	t.Cleanup(func() { _ = resolved.Reader.Close() })
	assert.Equal(t, filepath.Join(home, ".claude", "projects", "-work-a", "dup.jsonl"), resolved.Metadata.Path)
}

func TestAllProjects_CodexDuplicateKeepsTheFirstWalkMatch(t *testing.T) {
	home := isolateHome(t)
	const id = "0199aaaa-0000-7000-8000-000000000001"
	first, err := filepath.EvalSymlinks(writeLines(t, codexRolloutPath(home, "01", id), codexMetaLine(id, "/work/one")))
	require.NoError(t, err)
	writeLines(t, codexRolloutPath(home, "02", id), codexMetaLine(id, "/work/two"))

	list, warnings, err := newTestLister(home).ListAll("")
	require.NoError(t, err)
	rows := onlySource(list, SourceCodex)
	require.Len(t, rows, 1)
	assert.Equal(t, "/work/one", rows[0].Project)
	assert.Equal(t, []string{WarningDuplicateSession}, warningCodes(warnings, SourceCodex))

	resolved, err := newTestResolver("", home).Resolve(SourceCodex, id)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resolved.Reader.Close() })
	assert.Equal(t, first, resolved.Metadata.Path, "listing and opening select the same candidate")
}

func TestAllProjects_InvalidSelectedCodexCandidateIsNotReplaced(t *testing.T) {
	const id = "0199aaaa-0000-7000-8000-000000000001"
	tests := map[string]struct {
		first    string
		wantCode string
	}{
		"no session_meta in the head": {
			first:    `{"type":"response_item","payload":{"type":"message","role":"user","content":[]}}`,
			wantCode: transcript.WarningInvalidRecord,
		},
		"recorded id differs from the filename": {
			first:    codexMetaLine("0199ffff-0000-7000-8000-00000000000f", "/work/one"),
			wantCode: transcript.WarningInvalidRecord,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			home := isolateHome(t)
			writeLines(t, codexRolloutPath(home, "01", id), tc.first)
			writeLines(t, codexRolloutPath(home, "02", id), codexMetaLine(id, "/work/two"))

			list, warnings, err := newTestLister(home).ListAll("")
			require.NoError(t, err)
			assert.Empty(t, onlySource(list, SourceCodex), "a later valid duplicate is not promoted (Decision 22)")
			assert.Equal(t, []string{tc.wantCode, WarningDuplicateSession}, warningCodes(warnings, SourceCodex))
		})
	}

	t.Run("unreadable", func(t *testing.T) {
		if runtime.GOOS == "windows" || os.Getuid() == 0 {
			t.Skip("file permissions do not block reads here")
		}
		home := isolateHome(t)
		path := writeLines(t, codexRolloutPath(home, "01", id), codexMetaLine(id, "/work/one"))
		require.NoError(t, os.Chmod(path, 0))
		t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

		list, warnings, err := newTestLister(home).ListAll("")
		require.NoError(t, err)
		assert.Empty(t, onlySource(list, SourceCodex))
		assert.Equal(t, []string{WarningPathUnreadable}, warningCodes(warnings, SourceCodex))
	})
}

// TestProjectScope_CodexRowsKeepPerFileFiltering covers Q62: a later duplicate
// whose cwd is the project keeps its legacy project row even though the global
// winner lives in another project.
func TestProjectScope_CodexRowsKeepPerFileFiltering(t *testing.T) {
	home := isolateHome(t)
	project := t.TempDir()
	const id = "0199aaaa-0000-7000-8000-000000000001"
	writeLines(t, codexRolloutPath(home, "01", id), codexMetaLine(id, "/work/other"))
	writeLines(t, codexRolloutPath(home, "02", id), codexMetaLine(id, project))

	list, warnings, err := newTestLister(home).ListAll(project)
	require.NoError(t, err)
	rows := onlySource(list, SourceCodex)
	require.Len(t, rows, 1)
	assert.Equal(t, project, rows[0].Project)
	assert.Empty(t, warningCodes(warnings, SourceCodex), "project scope reports no duplicates")
}

func TestAllProjects_KiroIDEUsesTheBestCandidateAcrossWorkspaces(t *testing.T) {
	modTime := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	tests := map[string]struct {
		chats         map[string]int // workspace -> chat messages
		mtimes        map[string]time.Time
		wantWorkspace string
	}{
		"most chat messages": {
			chats:         map[string]int{"aaa": 2, "bbb": 3},
			mtimes:        map[string]time.Time{"aaa": modTime.Add(time.Hour), "bbb": modTime},
			wantWorkspace: "bbb",
		},
		"newest modification time": {
			chats:         map[string]int{"aaa": 2, "bbb": 2},
			mtimes:        map[string]time.Time{"aaa": modTime, "bbb": modTime.Add(time.Hour)},
			wantWorkspace: "bbb",
		},
		"lexically smallest path": {
			chats:         map[string]int{"aaa": 2, "bbb": 2},
			mtimes:        map[string]time.Time{"aaa": modTime, "bbb": modTime},
			wantWorkspace: "aaa",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			home := isolateHome(t)
			base := kiroIDEBaseUnder(t)
			for ws, n := range tc.chats {
				kiroIDEChat(t, filepath.Join(base, ws, "session.chat"), "dup", n, tc.mtimes[ws])
			}
			winner := filepath.Join(base, tc.wantWorkspace)

			list, warnings, err := newTestLister(home).ListAll("")
			require.NoError(t, err)
			rows := onlySource(list, SourceKiroIDE)
			require.Len(t, rows, 1)
			assert.Equal(t, []string{WarningDuplicateSession}, warningCodes(warnings, SourceKiroIDE))

			resolved, err := newTestResolver("", home).Resolve(SourceKiroIDE, "dup")
			require.NoError(t, err)
			t.Cleanup(func() { _ = resolved.Reader.Close() })
			assert.Equal(t, filepath.Join(winner, "session.chat"), resolved.Metadata.Path)
			assert.Equal(t, transcript.KiroIDEExecutionDetailPath(winner, "dup"), resolved.Metadata.CostPath,
				"the cost path stays paired with the selected workspace")
			assert.True(t, tc.mtimes[tc.wantWorkspace].Equal(rows[0].CreatedAt), "the listed row is the opened candidate")
		})
	}
}

func TestAllProjects_KiroCLIOpensTheListedRow(t *testing.T) {
	home := isolateHome(t)
	db := kiroCLITestDB(t, home)
	created := time.Date(2026, 10, 1, 6, 0, 0, 0, time.UTC)
	for _, key := range []string{"/z-key", "/a-key"} {
		_, err := db.Exec("INSERT INTO conversations_v2 VALUES (?, ?, ?, ?, ?)", key, "conversation",
			`{"conversation_id":"conversation","key":"`+key+`"}`, created.UnixMilli(), created.Add(time.Hour).UnixMilli())
		require.NoError(t, err)
	}

	list, _, err := newTestLister(home).ListAll("")
	require.NoError(t, err)
	rows := onlySource(list, SourceKiroCLI)
	require.Len(t, rows, 1)
	assert.Equal(t, "/a-key", rows[0].Project)

	// Opening must not turn the empty project into the working directory.
	resolved, err := newTestResolver("", home).Resolve(SourceKiroCLI, "conversation")
	require.NoError(t, err)
	t.Cleanup(func() { _ = resolved.Reader.Close() })
	assert.Equal(t, "/a-key", resolved.Metadata.Project)
	assert.True(t, created.Equal(resolved.Metadata.DatabaseCreatedAt))
	body, err := io.ReadAll(resolved.Reader)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"key":"/a-key"`)
}

// TestAllProjects_EveryListedRowOpens lists every existing source across
// projects and opens each row with an all-projects resolver from an
// unrelated working directory.
func TestAllProjects_EveryListedRowOpens(t *testing.T) {
	home := isolateHome(t)
	t.Chdir(t.TempDir())

	writeLines(t, filepath.Join(home, ".claude", "projects", "-work-claude", "claude-session.jsonl"),
		`{"type":"user","timestamp":"2026-10-01T08:00:00Z","cwd":"/work/claude","message":{"role":"user","content":"Hello"}}`)
	const codexID = "0199cccc-0000-7000-8000-000000000003"
	writeLines(t, codexRolloutPath(home, "01", codexID), codexMetaLine(codexID, "/work/codex"))
	const copilotID = "12345678-aaaa-bbbb-cccc-123456789abc"
	writeLines(t, filepath.Join(home, ".copilot", "session-state", copilotID, "events.jsonl"), `{"type":"session.start"}`)
	db := kiroCLITestDB(t, home)
	_, err := db.Exec("INSERT INTO conversations_v2 VALUES (?, ?, ?, ?, ?)", "/work/kiro", "kiro-conversation", `{}`, 1, 2)
	require.NoError(t, err)
	kiroIDEChat(t, filepath.Join(kiroIDEBaseUnder(t), "aaa", "session.chat"), "kiro-ide-execution", 1, time.Now())

	lister := newTestLister(home)
	list, warnings, err := lister.ListAll("")
	require.NoError(t, err)
	assert.Empty(t, warnings)
	sources := map[string]bool{}
	resolver := newTestResolver("", home)
	for _, row := range list {
		sources[row.Source] = true
		resolved, err := resolver.Resolve(row.Source, row.ID)
		if assert.NoError(t, err, "%s/%s", row.Source, row.ID) {
			assert.Equal(t, row.Source, resolved.Metadata.Source)
			assert.Equal(t, row.ID, resolved.Metadata.ID)
			_ = resolved.Reader.Close()
		}
	}
	assert.Len(t, sources, len(lister.Sources()), "every source is listed: %v", sources)
}
