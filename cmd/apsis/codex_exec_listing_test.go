package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/arjenschwarz/orbit/internal/sessions"
)

func TestCodexExecRootsSettings(t *testing.T) {
	t.Chdir(t.TempDir())
	abs := func(p string) string {
		got, err := filepath.Abs(p)
		require.NoError(t, err)
		return got
	}
	list := strings.Join([]string{"/srv/a", "rel"}, string(os.PathListSeparator))
	empty := ""
	tests := map[string]struct {
		env     *string
		flags   []string
		want    []string
		wantErr string
	}{
		"unset":                         {},
		"empty variable":                {env: &empty},
		"variable list":                 {env: &list, want: []string{"/srv/a", abs("rel")}},
		"flag replaces variable":        {env: &list, flags: []string{"/srv/b"}, want: []string{"/srv/b"}},
		"repeated flags":                {flags: []string{"/srv/b", "rel"}, want: []string{"/srv/b", abs("rel")}},
		"one empty flag means no roots": {env: &list, flags: []string{""}, want: nil},
		"mixed empty and nonempty":      {flags: []string{"", "/srv/b"}, wantErr: "--codex-exec-dir"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if tc.env != nil {
				t.Setenv(codexExecDirsEnv, *tc.env)
			} else {
				t.Setenv(codexExecDirsEnv, "")
				require.NoError(t, os.Unsetenv(codexExecDirsEnv))
			}
			got, err := codexExecRoots(tc.flags)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestServeRejectsMixedCodexExecDirs(t *testing.T) {
	err := serveCommand([]string{"--codex-exec-dir", "", "--codex-exec-dir", "/srv/b"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--codex-exec-dir")
}

func TestCodexExecListing(t *testing.T) {
	fakeHome(t)
	path, repo := execRepo(t, execFixture(t, "all_items.jsonl"))
	modified := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(path, modified, modified))
	id := sessions.ExecIdentifier(execThreadID, path)

	t.Run("text listing marks the filesystem time", func(t *testing.T) {
		stdout, _, err := runCLIStdout(t, &Config{List: true, AllProjects: true, Format: "md", CodexExecDirs: []string{repo}})
		require.NoError(t, err)
		want := "[codex-exec]\t" + id + "\t" + modified.Local().Format(time.RFC3339) + " (file modified)\t"
		assert.Contains(t, stdout, want)
		assert.Contains(t, stdout, "\t"+repo+"\n")
	})

	t.Run("shared JSON listing", func(t *testing.T) {
		stdout, _, err := runCLIStdout(t, &Config{List: true, Project: repo, Format: "shared-json", CodexExecDirs: []string{repo}})
		require.NoError(t, err)
		doc := decodeListing(t, stdout)
		require.Len(t, doc.Sessions, 1)
		row := doc.Sessions[0]
		assert.Equal(t, "codex-exec", row.Source)
		assert.Equal(t, id, row.ID)
		assert.Equal(t, execThreadID, row.ThreadID)
		assert.Equal(t, "2026-10-01T09:00:00Z", row.FileModifiedAt)
		assert.Empty(t, row.CreatedAt)
		assert.Equal(t, repo, row.Project)
	})

	t.Run("environment roots apply", func(t *testing.T) {
		t.Setenv(codexExecDirsEnv, repo)
		stdout, _, err := runCLIStdout(t, &Config{List: true, AllProjects: true, Format: "md"})
		require.NoError(t, err)
		assert.Contains(t, stdout, id)
	})

	t.Run("no roots lists no streams", func(t *testing.T) {
		t.Setenv(codexExecDirsEnv, "")
		stdout, _, err := runCLIStdout(t, &Config{List: true, AllProjects: true, Format: "md"})
		require.NoError(t, err)
		assert.NotContains(t, stdout, "codex-exec")
	})
}
