package sessions

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

const execStream = `{"type":"thread.started","thread_id":"0199aaaa-0000-7000-8000-000000000001"}
{"type":"turn.started"}
{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"done"}}
`

var execIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,256}$`)

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestExecIdentifier(t *testing.T) {
	path := "/work/example-repo/logs/run.jsonl"
	digest := sha256Hex(path)
	tests := map[string]struct {
		threadID, want string
	}{
		"uuid thread":        {"0199aaaa-0000-7000-8000-000000000001", "0199aaaa-0000-7000-8000-000000000001-" + digest},
		"unsafe bytes":       {"a/b c.d:e", "a_b_c_d_e-" + digest},
		"multibyte":          {"é", "__-" + digest},
		"truncated to 191 B": {strings.Repeat("x", 300), strings.Repeat("x", 191) + "-" + digest},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := ExecIdentifier(tc.threadID, path)
			assert.Equal(t, tc.want, got)
			assert.Regexp(t, execIDPattern, got)
		})
	}
}

func TestPropertyExecIdentifier(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		threadID := rapid.StringN(1, 400, -1).Draw(rt, "thread")
		path := "/" + rapid.StringN(1, 200, -1).Draw(rt, "path")
		got := ExecIdentifier(threadID, path)
		if !execIDPattern.MatchString(got) {
			rt.Fatalf("identifier %q does not match %s", got, execIDPattern)
		}
		if again := ExecIdentifier(threadID, path); again != got {
			rt.Fatalf("not deterministic: %q then %q", got, again)
		}
		if !strings.HasSuffix(got, "-"+sha256Hex(path)) {
			rt.Fatalf("identifier %q does not end with the path digest", got)
		}
	})
}

func TestIdentifyExecStream_StableAcrossAppends(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(execStream), 0o600))

	before, err := IdentifyExecStream(path)
	require.NoError(t, err)
	assert.Equal(t, "0199aaaa-0000-7000-8000-000000000001", before.ThreadID)
	canonical, err := CanonicalPath(path)
	require.NoError(t, err)
	assert.Equal(t, canonical, before.Path)
	assert.Equal(t, ExecIdentifier(before.ThreadID, canonical), before.ID)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = f.WriteString(`{"type":"turn.started"}` + "\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	after, err := IdentifyExecStream(path)
	require.NoError(t, err)
	assert.Equal(t, before.ID, after.ID)
}

func TestIdentifyExecStream_WithoutThread(t *testing.T) {
	path := filepath.Join(t.TempDir(), "failed.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(`{"type":"error","message":"unreachable"}`+"\n"), 0o600))
	got, err := IdentifyExecStream(path)
	require.NoError(t, err)
	assert.Empty(t, got.ID)
	assert.Empty(t, got.ThreadID)
}

// caseInsensitive reports whether dir's volume ignores name case.
func caseInsensitive(t *testing.T, dir string) bool {
	t.Helper()
	probe := filepath.Join(dir, "CaseProbe")
	require.NoError(t, os.WriteFile(probe, nil, 0o600))
	_, err := os.Stat(filepath.Join(dir, "caseprobe"))
	return err == nil
}

func TestCanonicalPath(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	target := filepath.Join(root, "Logs", "Run.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
	require.NoError(t, os.WriteFile(target, []byte(execStream), 0o600))

	t.Run("symlinks resolve", func(t *testing.T) {
		link := filepath.Join(root, "link.jsonl")
		require.NoError(t, os.Symlink(target, link))
		got, err := CanonicalPath(link)
		require.NoError(t, err)
		assert.Equal(t, target, got)
	})

	t.Run("relative paths become absolute", func(t *testing.T) {
		t.Chdir(root)
		got, err := CanonicalPath(filepath.Join("Logs", "Run.jsonl"))
		require.NoError(t, err)
		assert.Equal(t, target, got)
	})

	t.Run("case aliases take the on-disk spelling", func(t *testing.T) {
		if !caseInsensitive(t, root) {
			t.Skip("temporary volume is case-sensitive")
		}
		got, err := CanonicalPath(filepath.Join(root, "logs", "run.JSONL"))
		require.NoError(t, err)
		assert.Equal(t, target, got)
	})
}

func TestRepositoryRoot(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	repo := filepath.Join(root, "repo")
	worktree := filepath.Join(repo, ".worktrees", "feature")
	notGit := filepath.Join(repo, "vendor", "lib")
	outside := filepath.Join(root, "outside", "logs")
	for _, dir := range []string{filepath.Join(repo, ".git"), filepath.Join(repo, "logs", "deep"), filepath.Join(worktree, "logs"), notGit, outside} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: "+filepath.Join(repo, ".git", "worktrees", "feature")+"\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(notGit, ".git"), []byte("not a pointer\n"), 0o600))

	tests := map[string]struct {
		dir, want string
	}{
		"repository directory":            {repo, repo},
		"nested directory":                {filepath.Join(repo, "logs", "deep"), repo},
		"worktree inside its parent repo": {filepath.Join(worktree, "logs"), worktree},
		".git file without gitdir:":       {notGit, repo},
		"outside any repository":          {outside, ""},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, RepositoryRoot(tc.dir))
		})
	}
}
