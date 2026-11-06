package sessions

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/arjenschwarz/orbit/internal/agents/claudecode"
	"github.com/arjenschwarz/orbit/internal/agents/kiro/logs"
)

// writeLines writes lines, each followed by a newline, creating parent directories.
func writeLines(t *testing.T, path string, lines ...string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
	return path
}

// isolateHome points HOME and the XDG and AppData directories at a new
// temporary directory, so Kiro lookups that ignore Lister.homeDir never read
// this machine's data. It returns the new home directory.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("APPDATA", filepath.Join(home, "AppData"))
	return home
}

// repeatLines returns n copies of line.
func repeatLines(line string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = line
	}
	return out
}

func claudeTranscriptPath(homeDir, projectPath, id string) string {
	return filepath.Join(homeDir, ".claude", "projects", claudecode.BuildProjectPath(projectPath), id+".jsonl")
}

func codexRolloutPath(homeDir, day, id string) string {
	return filepath.Join(homeDir, ".codex", "sessions", "2026", "10", day, "rollout-2026-10-"+day+"T09-00-00-"+id+".jsonl")
}

// onlySource returns the listed sessions of one source.
func onlySource(list []SessionInfo, source string) []SessionInfo {
	var out []SessionInfo
	for _, s := range list {
		if s.Source == source {
			out = append(out, s)
		}
	}
	return out
}

func TestListAll_ClaudeRowMetadata(t *testing.T) {
	recorded := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	modTime := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	userLine := `{"type":"user","timestamp":"2026-10-01T08:00:00Z","cwd":"/work/claude-repo","entrypoint":"cli","message":{"role":"user","content":"Hello"}}`
	noTimeLine := `{"type":"user","message":{"role":"user","content":"Hello"}}`

	tests := map[string]struct {
		lines       []string
		wantTime    time.Time
		wantFromFS  bool
		wantProject string
		wantOrigin  string
	}{
		"cwd and entrypoint within the head": {
			lines:       []string{`{"type":"summary","summary":"s"}`, userLine},
			wantTime:    recorded,
			wantProject: "/work/claude-repo",
			wantOrigin:  "cli",
		},
		"first values win": {
			lines: []string{userLine,
				`{"type":"assistant","timestamp":"2026-10-01T08:00:01Z","cwd":"/work/later","entrypoint":"sdk-cli","message":{"role":"assistant","content":"Hi"}}`},
			wantTime:    recorded,
			wantProject: "/work/claude-repo",
			wantOrigin:  "cli",
		},
		"timestamp beyond 50 lines keeps the recorded time while cwd and origin are out of bounds": {
			lines:    append(repeatLines(noTimeLine, 60), userLine),
			wantTime: recorded,
		},
		"cwd beyond the 1 MiB head is out of bounds": {
			lines:    []string{`{"type":"summary","summary":"` + strings.Repeat("x", 1<<20) + `"}`, userLine},
			wantTime: recorded,
		},
		"no timestamp falls back to the flagged modification time": {
			lines:       []string{`{"type":"user","cwd":"/work/claude-repo","entrypoint":"cli","message":{"role":"user","content":"Hello"}}`},
			wantTime:    modTime,
			wantFromFS:  true,
			wantProject: "/work/claude-repo",
			wantOrigin:  "cli",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			homeDir := isolateHome(t)
			projectPath := t.TempDir()
			path := writeLines(t, claudeTranscriptPath(homeDir, projectPath, "claude-session"), tc.lines...)
			require.NoError(t, os.Chtimes(path, modTime, modTime))

			list, _, err := newTestLister(homeDir).ListAll(projectPath)
			require.NoError(t, err)
			rows := onlySource(list, SourceClaude)
			require.Len(t, rows, 1)
			got := rows[0]
			assert.True(t, tc.wantTime.Equal(got.CreatedAt), "CreatedAt = %v, want %v", got.CreatedAt, tc.wantTime)
			assert.Equal(t, tc.wantFromFS, got.TimeFromFilesystem)
			assert.Equal(t, tc.wantProject, got.Project)
			assert.Equal(t, tc.wantOrigin, got.Origin)
			assert.Empty(t, got.Model, "Claude rows carry no model (Q92)")
			assert.Empty(t, got.Provider)
		})
	}
}

func TestListAll_CodexRowMetadata(t *testing.T) {
	const id = "0199aaaa-0000-7000-8000-000000000001"
	recorded := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	modTime := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	meta := `{"timestamp":"2026-10-01T09:00:00Z","type":"session_meta","payload":{"id":"` + id + `","cwd":"/work/codex-repo","originator":"codex_exec","model_provider":"example-provider","model":"header-model"}}`
	turnContext := `{"timestamp":"2026-10-01T09:00:01Z","type":"turn_context","payload":{"cwd":"/work/codex-repo","model":"example-model"}}`
	message := `{"timestamp":"2026-10-01T09:00:02Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Hello"}]}}`

	tests := map[string]struct {
		lines        []string
		wantTime     time.Time
		wantFromFS   bool
		wantProject  string
		wantModel    string
		wantProvider string
		wantOrigin   string
	}{
		"turn_context model wins over the session_meta model": {
			lines:        []string{meta, message, turnContext},
			wantTime:     recorded,
			wantProject:  "/work/codex-repo",
			wantModel:    "example-model",
			wantProvider: "example-provider",
			wantOrigin:   "codex_exec",
		},
		"session_meta model without turn_context": {
			lines:        []string{meta, message},
			wantTime:     recorded,
			wantProject:  "/work/codex-repo",
			wantModel:    "header-model",
			wantProvider: "example-provider",
			wantOrigin:   "codex_exec",
		},
		"turn_context beyond the scan limit is not read": {
			lines:        append(append([]string{meta}, repeatLines(message, codexMetaScanLimit)...), turnContext),
			wantTime:     recorded,
			wantProject:  "/work/codex-repo",
			wantModel:    "header-model",
			wantProvider: "example-provider",
			wantOrigin:   "codex_exec",
		},
		"long session_meta line keeps the recorded time": {
			lines: []string{`{"timestamp":"2026-10-01T09:00:00Z","type":"session_meta","payload":{"id":"` + id +
				`","cwd":"/work/codex-repo","instructions":"` + strings.Repeat("x", 2<<20) + `"}}`},
			wantTime:    recorded,
			wantProject: "/work/codex-repo",
		},
		"session_meta without a parseable timestamp falls back to the flagged modification time": {
			lines: []string{`{"timestamp":"not a time","type":"session_meta","payload":{"id":"` + id +
				`","cwd":"/work/codex-repo"}}`},
			wantTime:    modTime,
			wantFromFS:  true,
			wantProject: "/work/codex-repo",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			homeDir := isolateHome(t)
			path := writeLines(t, codexRolloutPath(homeDir, "01", id), tc.lines...)
			require.NoError(t, os.Chtimes(path, modTime, modTime))

			list, _, err := newTestLister(homeDir).ListAll("")
			require.NoError(t, err)
			rows := onlySource(list, SourceCodex)
			require.Len(t, rows, 1)
			got := rows[0]
			assert.True(t, tc.wantTime.Equal(got.CreatedAt), "CreatedAt = %v, want %v", got.CreatedAt, tc.wantTime)
			assert.Equal(t, tc.wantFromFS, got.TimeFromFilesystem)
			assert.Equal(t, tc.wantProject, got.Project)
			assert.Equal(t, tc.wantModel, got.Model)
			assert.Equal(t, tc.wantProvider, got.Provider)
			assert.Equal(t, tc.wantOrigin, got.Origin)
		})
	}
}

func TestListAll_CopilotRowMetadata(t *testing.T) {
	const id = "12345678-aaaa-bbbb-cccc-123456789abc"
	recorded := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	modTime := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := map[string]struct {
		workspace   string
		wantTime    time.Time
		wantFromFS  bool
		wantProject string
	}{
		"git_root":             {"cwd: /work/repo/sub\ngit_root: /work/repo\ncreated_at: 2026-09-30T00:00:00Z\n", recorded, false, "/work/repo"},
		"cwd without git_root": {"cwd: /work/plain\ncreated_at: 2026-09-30T00:00:00Z\n", recorded, false, "/work/plain"},
		"no created_at":        {"cwd: /work/plain\n", modTime, true, "/work/plain"},
		"no workspace.yaml":    {"", modTime, true, ""},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			homeDir := isolateHome(t)
			dir := filepath.Join(homeDir, ".copilot", "session-state", id)
			events := writeLines(t, filepath.Join(dir, "events.jsonl"), `{"type":"session.start"}`)
			require.NoError(t, os.Chtimes(events, modTime, modTime))
			if tc.workspace != "" {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "workspace.yaml"), []byte(tc.workspace), 0o600))
			}

			list, _, err := newTestLister(homeDir).ListAll("")
			require.NoError(t, err)
			rows := onlySource(list, SourceCopilot)
			require.Len(t, rows, 1)
			assert.True(t, tc.wantTime.Equal(rows[0].CreatedAt))
			assert.Equal(t, tc.wantFromFS, rows[0].TimeFromFilesystem)
			assert.Equal(t, tc.wantProject, rows[0].Project)
		})
	}
}

func TestListKiro_ProjectIsTheDatabaseKey(t *testing.T) {
	created := time.Date(2026, 10, 1, 6, 0, 0, 0, time.UTC)
	lister := &Lister{
		homeDir: t.TempDir(),
		kiroDiscover: func(_ context.Context, _ string) ([]logs.SessionMetadata, error) {
			return []logs.SessionMetadata{{ConversationID: "kiro-1", Directory: "/work/kiro-repo", CreatedAt: created}}, nil
		},
	}

	rows, err := lister.listKiro("/work/kiro-repo")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "/work/kiro-repo", rows[0].Project)
	assert.False(t, rows[0].TimeFromFilesystem)
	assert.Empty(t, rows[0].Origin)
}

func TestListKiroIDE_RowMetadata(t *testing.T) {
	modTime := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	baseDir := t.TempDir()
	workspace := filepath.Join(baseDir, "aaa111")
	writeLines(t, filepath.Join(workspace, "with-meta.chat"),
		`{"executionId":"exec-meta","metadata":{"modelId":"example-model","modelProvider":"example-provider","startTime":1790812800000},"chat":[{"role":"human","content":"Hello"}]}`)
	bare := writeLines(t, filepath.Join(workspace, "bare.chat"),
		`{"executionId":"exec-bare","chat":[{"role":"human","content":"Hello"}]}`)
	require.NoError(t, os.Chtimes(bare, modTime, modTime))

	lister := &Lister{homeDir: t.TempDir(), kiroIDEBasePath: func() (string, error) { return baseDir, nil }}
	rows, _, err := lister.listKiroIDE("")
	require.NoError(t, err)
	byID := map[string]SessionInfo{}
	for _, r := range rows {
		byID[r.ID] = r
	}
	require.Len(t, byID, 2)

	withMeta := byID["exec-meta"]
	assert.True(t, time.UnixMilli(1790812800000).Equal(withMeta.CreatedAt))
	assert.False(t, withMeta.TimeFromFilesystem)
	assert.Equal(t, "example-model", withMeta.Model)
	assert.Equal(t, "example-provider", withMeta.Provider)
	assert.Empty(t, withMeta.Project, "the workspace hash is one-way")

	assert.True(t, modTime.Equal(byID["exec-bare"].CreatedAt))
	assert.True(t, byID["exec-bare"].TimeFromFilesystem)
	assert.Empty(t, byID["exec-bare"].Model)
	assert.Empty(t, byID["exec-bare"].Provider)
}

// equalTimeCodexPair writes two Codex rollouts with the same recorded time
// whose walk order (by day directory) is the reverse of their identifier order.
func equalTimeCodexPair(t *testing.T, homeDir, projectPath string) (walkFirst, walkSecond string) {
	t.Helper()
	walkFirst = "ffffffff-0000-7000-8000-000000000001"
	walkSecond = "00000000-0000-7000-8000-000000000002"
	for day, id := range map[string]string{"01": walkFirst, "02": walkSecond} {
		writeLines(t, codexRolloutPath(homeDir, day, id),
			`{"timestamp":"2026-10-01T09:00:00Z","type":"session_meta","payload":{"id":"`+id+`","cwd":"`+projectPath+`"}}`)
	}
	return walkFirst, walkSecond
}

func TestListAll_EqualTimesKeepLegacyOrderWithoutExtendedOrdering(t *testing.T) {
	homeDir := isolateHome(t)
	projectPath := t.TempDir()
	walkFirst, walkSecond := equalTimeCodexPair(t, homeDir, projectPath)

	ids := func(l *Lister) []string {
		list, _, err := l.ListAll(projectPath)
		require.NoError(t, err)
		var out []string
		for _, s := range onlySource(list, SourceCodex) {
			out = append(out, s.ID)
		}
		return out
	}

	legacy := newTestLister(homeDir)
	assert.Equal(t, []string{walkFirst, walkSecond}, ids(legacy), "no-flag listings keep the existing order, so latest is %s", walkSecond)

	extended := newTestLister(homeDir)
	extended.opts = Options{ExtendedOrdering: true}
	assert.Equal(t, []string{walkSecond, walkFirst}, ids(extended), "extended ordering breaks the tie by identifier")
}

func TestListAll_WarningCodes(t *testing.T) {
	homeDir := isolateHome(t)
	setupCopilotSessionInvalidWorkspace(t, homeDir, "invalid-workspace-session", time.Date(2025, 4, 2, 9, 0, 0, 0, time.UTC))
	lister := &Lister{
		homeDir: homeDir,
		kiroDiscover: func(_ context.Context, _ string) ([]logs.SessionMetadata, error) {
			return nil, errors.New("database exploded")
		},
		kiroIDEBasePath: func() (string, error) { return "", errors.New("unreadable base") },
	}
	codes := func(warnings []ListWarning) map[string][]string {
		out := map[string][]string{}
		for _, w := range warnings {
			out[w.Source] = append(out[w.Source], w.Code)
		}
		return out
	}

	// Project scope discovers Kiro CLI sessions through kiroDiscover.
	_, warnings, err := lister.ListAll(t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, []string{WarningSourceFailed}, codes(warnings)[SourceKiroCLI])

	// All-projects scope scans Kiro IDE workspaces under kiroIDEBasePath and
	// lists the Copilot session whose workspace.yaml does not parse.
	_, warnings, err = lister.ListAll("")
	require.NoError(t, err)
	assert.Equal(t, []string{WarningSourceFailed}, codes(warnings)[SourceKiroIDE])
	assert.Equal(t, []string{WarningPathUnreadable}, codes(warnings)[SourceCopilot])
}

func TestConstructors_ShareOptions(t *testing.T) {
	opts := []Option{WithExtendedOrdering(true)}

	lister, err := NewLister(opts...)
	require.NoError(t, err)
	resolver, err := NewResolver("", opts...)
	require.NoError(t, err)

	assert.Equal(t, Options{ExtendedOrdering: true}, lister.opts)
	assert.Equal(t, lister.opts, resolver.opts)

	lister, err = NewLister()
	require.NoError(t, err)
	assert.Equal(t, Options{}, lister.opts, "no options keep the legacy defaults")
}
