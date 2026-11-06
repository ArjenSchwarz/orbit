package sessions

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	threadA = "0199aaaa-0000-7000-8000-00000000000a"
	threadB = "0199aaaa-0000-7000-8000-00000000000b"
)

func streamWithThread(threadID string) string {
	return fmt.Sprintf(`{"type":"thread.started","thread_id":%q}`+"\n"+`{"type":"turn.started"}`+"\n", threadID)
}

// execTree is a canonical temporary directory for discovery tests, with HOME
// pointed at an empty directory inside it.
type execTree struct {
	t    *testing.T
	root string
	home string
}

func newExecTree(t *testing.T) *execTree {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	home := filepath.Join(root, "home")
	require.NoError(t, os.MkdirAll(home, 0o755))
	t.Setenv("HOME", home)
	return &execTree{t: t, root: root, home: home}
}

func (e *execTree) path(parts ...string) string {
	return filepath.Join(append([]string{e.root}, parts...)...)
}

func (e *execTree) write(content string, parts ...string) string {
	e.t.Helper()
	path := e.path(parts...)
	require.NoError(e.t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(e.t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func (e *execTree) mkdir(parts ...string) string {
	e.t.Helper()
	path := e.path(parts...)
	require.NoError(e.t, os.MkdirAll(path, 0o755))
	return path
}

// rollout writes a Codex rollout under HOME whose filename and session_meta
// record id.
func (e *execTree) rollout(name, id, cwd string) string {
	e.t.Helper()
	meta := fmt.Sprintf(`{"timestamp":"2026-11-01T00:00:00Z","type":"session_meta","payload":{"id":%q,"cwd":%q}}`+"\n", id, cwd)
	return e.write(meta, "home", ".codex", "sessions", "2026", "11", "01", name)
}

func listExec(t *testing.T, project string, roots ...string) ([]SessionInfo, []ListWarning) {
	t.Helper()
	lister, err := NewLister(WithCodexExecRoots(roots))
	require.NoError(t, err)
	list, warnings, err := lister.ListAll(project)
	require.NoError(t, err)
	var execRows []SessionInfo
	for _, s := range list {
		if s.Source == SourceCodexExec {
			execRows = append(execRows, s)
		}
	}
	var execWarnings []ListWarning
	for _, w := range warnings {
		if w.Source == SourceCodexExec {
			execWarnings = append(execWarnings, w)
		}
	}
	return execRows, execWarnings
}

func rowIDs(rows []SessionInfo) []string {
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	slices.Sort(ids)
	return ids
}

func execWarningCodes(warnings []ListWarning) []string {
	codes := make([]string, 0, len(warnings))
	for _, w := range warnings {
		codes = append(codes, w.Code)
	}
	slices.Sort(codes)
	return codes
}

func TestListCodexExec_OverlappingRootsAndOrder(t *testing.T) {
	tree := newExecTree(t)
	a := tree.write(streamWithThread(threadA), "streams", "a.jsonl")
	b := tree.write(streamWithThread(threadB), "streams", "nested", "b.jsonl")
	want := []string{ExecIdentifier(threadA, a), ExecIdentifier(threadB, b)}
	slices.Sort(want)

	outer, inner := tree.path("streams"), tree.path("streams", "nested")
	for name, roots := range map[string][]string{
		"outer only":    {outer},
		"outer, inner":  {outer, inner},
		"inner, outer":  {inner, outer},
		"repeated root": {outer, outer + string(filepath.Separator)},
	} {
		t.Run(name, func(t *testing.T) {
			rows, warnings := listExec(t, "", roots...)
			assert.Empty(t, warnings)
			assert.Equal(t, want, rowIDs(rows), "each canonical file is listed once with an unchanged identifier")
		})
	}
}

func TestListCodexExec_SkippedNamesAndExplicitRoots(t *testing.T) {
	tree := newExecTree(t)
	listed := tree.write(streamWithThread(threadA), "root", "logs", "run.jsonl")
	tree.write(streamWithThread(threadB), "root", ".git", "x.jsonl")
	tree.write(streamWithThread(threadB), "root", "node_modules", "pkg", "y.jsonl")
	fixture := tree.write(streamWithThread(threadB), "root", "testdata", "z.jsonl")
	nested := tree.write(streamWithThread(threadB), "root", "node_modules", "inner", "w.jsonl")

	rows, _ := listExec(t, "", tree.path("root"))
	assert.Equal(t, []string{ExecIdentifier(threadA, listed)}, rowIDs(rows), ".git, node_modules and testdata are skipped below a root")

	rows, _ = listExec(t, "", tree.path("root", "testdata"))
	assert.Equal(t, []string{ExecIdentifier(threadB, fixture)}, rowIDs(rows), "a skipped name is scanned when it is the root")

	rows, _ = listExec(t, "", tree.path("root"), tree.path("root", "node_modules", "inner"))
	want := []string{ExecIdentifier(threadA, listed), ExecIdentifier(threadB, nested)}
	slices.Sort(want)
	assert.Equal(t, want, rowIDs(rows), "a root nested inside another root's skipped directory is scanned")
}

func TestListCodexExec_Symlinks(t *testing.T) {
	tree := newExecTree(t)
	target := tree.write(streamWithThread(threadA), "root", "real", "run.jsonl")
	outside := tree.write(streamWithThread(threadB), "elsewhere", "out.jsonl")
	tree.write(streamWithThread(threadB), "elsewhere", "dir", "hidden.jsonl")
	require.NoError(t, os.Symlink(target, tree.path("root", "link.jsonl")))
	require.NoError(t, os.Symlink(outside, tree.path("root", "escape.jsonl")))
	require.NoError(t, os.Symlink(tree.path("elsewhere", "dir"), tree.path("root", "dirlink")))
	require.NoError(t, os.Symlink(tree.path("root"), tree.path("root", "real", "cycle")))
	require.NoError(t, os.Symlink(tree.path("root"), tree.path("rootlink")))

	for name, root := range map[string]string{"root": tree.path("root"), "symlinked root": tree.path("rootlink")} {
		t.Run(name, func(t *testing.T) {
			rows, warnings := listExec(t, "", root)
			assert.Empty(t, warnings)
			assert.Equal(t, []string{ExecIdentifier(threadA, target)}, rowIDs(rows),
				"a file symlink inside the roots joins its target; escaping symlinks, directory symlinks and cycles are not followed")
		})
	}
}

func TestListCodexExec_Qualification(t *testing.T) {
	tree := newExecTree(t)
	valid := tree.write(streamWithThread(threadA), "root", "valid.jsonl")
	tree.write(`{"type":"error","message":"unreachable"}`+"\n", "root", "threadless.jsonl")
	tree.write(`{"type":"user","message":{"role":"user","content":"hi"}}`+"\n", "root", "claude.jsonl")
	tree.write("not json\n", "root", "notes.jsonl")

	rows, warnings := listExec(t, "", tree.path("root"), tree.path("missing"))
	assert.Equal(t, []string{ExecIdentifier(threadA, valid)}, rowIDs(rows))
	assert.Equal(t, []string{WarningMissingThread, WarningRootUnreadable}, execWarningCodes(warnings), "other files are ignored silently")

	row := rows[0]
	assert.Equal(t, threadA, row.ThreadID)
	assert.True(t, row.TimeFromFilesystem)
	info, err := os.Stat(valid)
	require.NoError(t, err)
	assert.Equal(t, info.Size(), row.Size)
	assert.True(t, info.ModTime().Equal(row.CreatedAt))
}

func TestListCodexExec_UnreadablePaths(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permissions do not restrict root")
	}
	tree := newExecTree(t)
	valid := tree.write(streamWithThread(threadA), "root", "valid.jsonl")
	file := tree.write(streamWithThread(threadB), "root", "locked.jsonl")
	dir := tree.mkdir("root", "locked")
	tree.write(streamWithThread(threadB), "root", "locked", "inner.jsonl")
	require.NoError(t, os.Chmod(file, 0))
	require.NoError(t, os.Chmod(dir, 0))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755); _ = os.Chmod(file, 0o600) })

	rows, warnings := listExec(t, "", tree.path("root"))
	assert.Equal(t, []string{ExecIdentifier(threadA, valid)}, rowIDs(rows), "other results remain")
	assert.Equal(t, []string{WarningPathUnreadable, WarningPathUnreadable}, execWarningCodes(warnings))
}

func TestListCodexExec_IdentifierConflict(t *testing.T) {
	tree := newExecTree(t)
	original := pathDigest
	pathDigest = func(string) string { return "same-digest" }
	t.Cleanup(func() { pathDigest = original })

	tree.write(streamWithThread(threadA), "one", "run.jsonl")
	tree.write(streamWithThread(threadA), "two", "run.jsonl")
	other := tree.write(streamWithThread(threadB), "two", "other.jsonl")

	for name, roots := range map[string][]string{
		"one, two": {tree.path("one"), tree.path("two")},
		"two, one": {tree.path("two"), tree.path("one")},
	} {
		t.Run(name, func(t *testing.T) {
			rows, warnings := listExec(t, "", roots...)
			assert.Equal(t, []string{ExecIdentifier(threadB, other)}, rowIDs(rows), "both conflicting paths are omitted")
			assert.Equal(t, []string{WarningIdentifierConflict}, execWarningCodes(warnings))
		})
	}
}

func TestListCodexExec_ProjectScope(t *testing.T) {
	tree := newExecTree(t)
	repo := tree.mkdir("repo", ".git")
	repo = filepath.Dir(repo)
	inRepo := tree.write(streamWithThread(threadA), "repo", "logs", "run.jsonl")
	outside := tree.write(streamWithThread(threadB), "loose", "run.jsonl")
	roots := []string{tree.path("repo"), tree.path("loose")}

	rows, _ := listExec(t, repo, roots...)
	require.Len(t, rows, 1)
	assert.Equal(t, ExecIdentifier(threadA, inRepo), rows[0].ID)
	assert.Equal(t, repo, rows[0].Project)

	rows, _ = listExec(t, tree.mkdir("other"), roots...)
	assert.Empty(t, rows)

	rows, _ = listExec(t, "", roots...)
	want := []string{ExecIdentifier(threadA, inRepo), ExecIdentifier(threadB, outside)}
	slices.Sort(want)
	assert.Equal(t, want, rowIDs(rows), "streams outside repositories list only across projects")
}

func TestListCodexExec_RolloutSuppression(t *testing.T) {
	tree := newExecTree(t)
	repo := filepath.Dir(tree.mkdir("repo", ".git"))
	stream := tree.write(streamWithThread(threadA), "repo", "logs", "run.jsonl")
	roots := []string{tree.path("repo")}

	t.Run("a verified rollout suppresses the stream in every scope", func(t *testing.T) {
		rollout := tree.rollout("rollout-2026-11-01T00-00-00-"+threadA+".jsonl", threadA, tree.mkdir("elsewhere"))
		t.Cleanup(func() { _ = os.Remove(rollout) })
		for _, project := range []string{"", repo} {
			rows, _ := listExec(t, project, roots...)
			assert.Empty(t, rows, "project %q", project)
		}
	})

	t.Run("an invalid first duplicate suppresses nothing", func(t *testing.T) {
		first := tree.write("not a rollout\n", "home", ".codex", "sessions", "2026", "11", "01", "a-rollout-"+threadA+".jsonl")
		second := tree.rollout("b-rollout-"+threadA+".jsonl", threadA, repo)
		t.Cleanup(func() { _ = os.Remove(first); _ = os.Remove(second) })
		rows, _ := listExec(t, "", roots...)
		assert.Equal(t, []string{ExecIdentifier(threadA, stream)}, rowIDs(rows))
	})

	t.Run("an unreadable rollout suppresses nothing", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("permissions do not restrict root")
		}
		rollout := tree.rollout("rollout-2026-11-01T00-00-00-"+threadA+".jsonl", threadA, repo)
		require.NoError(t, os.Chmod(rollout, 0))
		t.Cleanup(func() { _ = os.Remove(rollout) })
		rows, _ := listExec(t, "", roots...)
		assert.Equal(t, []string{ExecIdentifier(threadA, stream)}, rowIDs(rows))
	})
}

func TestLister_SourcesAndOrderingWithRoots(t *testing.T) {
	newExecTree(t)
	without, err := NewLister()
	require.NoError(t, err)
	assert.NotContains(t, without.Sources(), SourceCodexExec)
	assert.False(t, without.opts.ExtendedOrdering)

	with, err := NewLister(WithCodexExecRoots([]string{"/somewhere"}))
	require.NoError(t, err)
	assert.Contains(t, with.Sources(), SourceCodexExec)
	assert.True(t, with.opts.ExtendedOrdering, "configured roots enable extended ordering")

	assert.True(t, IsValidSource(SourceCodexExec))
	assert.Equal(t, "codex-exec", DisplayName(SourceCodexExec))
}

// BenchmarkListAllCodexExec lists a synthetic tree of 10,000 files: 9,000
// streams and 1,000 other JSONL files spread over 100 directories, with a
// skipped node_modules subtree.
func BenchmarkListAllCodexExec(b *testing.B) {
	root, err := filepath.EvalSymlinks(b.TempDir())
	require.NoError(b, err)
	b.Setenv("HOME", filepath.Join(root, "home"))
	for i := range 10_000 {
		dir := filepath.Join(root, "streams", fmt.Sprintf("d%02d", i%100))
		if i%50 == 0 {
			dir = filepath.Join(root, "streams", "node_modules", fmt.Sprintf("m%02d", i%100))
		}
		content := streamWithThread(fmt.Sprintf("0199aaaa-0000-7000-8000-%012d", i))
		if i%10 == 9 {
			content = `{"type":"user","message":{"role":"user","content":"hi"}}` + "\n"
		}
		require.NoError(b, os.MkdirAll(dir, 0o755))
		require.NoError(b, os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%05d.jsonl", i)), []byte(content), 0o600))
	}
	lister, err := NewLister(WithCodexExecRoots([]string{filepath.Join(root, "streams")}))
	require.NoError(b, err)
	b.ResetTimer()
	for b.Loop() {
		if _, _, err := lister.ListAll(""); err != nil {
			b.Fatal(err)
		}
	}
}
