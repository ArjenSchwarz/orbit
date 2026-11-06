package sessions

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/arjenschwarz/orbit/internal/transcript"
)

func resolveExec(t *testing.T, project, id string, roots ...string) (*ResolvedSession, error) {
	t.Helper()
	resolver, err := NewResolver(project, WithCodexExecRoots(roots))
	require.NoError(t, err)
	return resolver.Resolve(SourceCodexExec, id)
}

func TestResolveCodexExec(t *testing.T) {
	tree := newExecTree(t)
	repo := filepath.Dir(tree.mkdir("repo", ".git"))
	stream := tree.write(streamWithThread(threadA), "repo", "logs", "run.jsonl")
	roots := []string{tree.path("repo")}
	id := ExecIdentifier(threadA, stream)
	info, err := os.Stat(stream)
	require.NoError(t, err)
	t.Chdir(tree.mkdir("unrelated"))

	for name, project := range map[string]string{"all projects": "", "another project": tree.path("unrelated")} {
		t.Run(name, func(t *testing.T) {
			resolved, err := resolveExec(t, project, id, roots...)
			require.NoError(t, err)
			t.Cleanup(func() { _ = resolved.Reader.Close() })
			body, err := io.ReadAll(resolved.Reader)
			require.NoError(t, err)
			assert.Equal(t, streamWithThread(threadA), string(body))
			assert.Equal(t, SessionMetadata{
				Source: SourceCodexExec, ID: id, CreatedAt: info.ModTime(), TimeFromFilesystem: true,
				Size: info.Size(), Project: repo, Path: stream, ThreadID: threadA,
			}, resolved.Metadata)

			path, err := mustResolver(t, project, roots).ResolvePath(SourceCodexExec, id)
			require.NoError(t, err)
			assert.Equal(t, stream, path)
		})
	}

	t.Run("suppressed streams still open", func(t *testing.T) {
		tree.rollout("rollout-2026-11-01T00-00-00-"+threadA+".jsonl", threadA, repo)
		rows, _ := listExec(t, "", roots...)
		assert.Empty(t, rows)
		resolved, err := resolveExec(t, "", id, roots...)
		require.NoError(t, err)
		_ = resolved.Reader.Close()
	})

	notFound := map[string]struct {
		id    string
		roots []string
	}{
		"no roots":               {id: id},
		"wrong thread prefix":    {id: strings.Replace(id, "0199aaaa", "0199bbbb", 1), roots: roots},
		"unknown digest":         {id: threadA + "-" + strings.Repeat("0", 64), roots: roots},
		"no digest":              {id: threadA, roots: roots},
		"outside the roots":      {id: id, roots: []string{tree.mkdir("elsewhere")}},
		"path traversal attempt": {id: "../" + id, roots: roots},
	}
	for name, tc := range notFound {
		t.Run(name, func(t *testing.T) {
			_, err := resolveExec(t, "", tc.id, tc.roots...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "session not found")
		})
	}

	t.Run("stale identifier after a move", func(t *testing.T) {
		moved := tree.path("repo", "logs", "moved.jsonl")
		require.NoError(t, os.Rename(stream, moved))
		t.Cleanup(func() { _ = os.Rename(moved, stream) })
		_, err := resolveExec(t, "", id, roots...)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "session not found")
	})
}

func TestResolveCodexExec_ConflictingIdentifierIsNotOpened(t *testing.T) {
	tree := newExecTree(t)
	original := pathDigest
	pathDigest = func(string) string { return strings.Repeat("a", 64) }
	t.Cleanup(func() { pathDigest = original })
	first := tree.write(streamWithThread(threadA), "one", "run.jsonl")
	tree.write(streamWithThread(threadA), "two", "run.jsonl")

	_, err := resolveExec(t, "", ExecIdentifier(threadA, first), tree.path("one"), tree.path("two"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")
}

func mustResolver(t *testing.T, project string, roots []string) *Resolver {
	t.Helper()
	resolver, err := NewResolver(project, WithCodexExecRoots(roots))
	require.NoError(t, err)
	return resolver
}

const headerOnlyRollout = `{"timestamp":"2026-10-01T09:00:00Z","type":"session_meta","payload":{"id":"0199cccc-0000-7000-8000-000000000003","cwd":"/work/codex-repo","model_provider":"example-provider"}}
{"timestamp":"2026-10-01T09:00:01Z","type":"turn_context","payload":{"model":"example-model"}}
`

func TestParseResolved(t *testing.T) {
	open := func(t *testing.T, content string, metadata SessionMetadata) *ResolvedSession {
		t.Helper()
		return &ResolvedSession{Reader: io.NopCloser(strings.NewReader(content)), Metadata: metadata}
	}

	t.Run("codex flags are forwarded", func(t *testing.T) {
		_, err := ParseResolved(open(t, headerOnlyRollout, SessionMetadata{Source: SourceCodex}))
		require.Error(t, err, "default options keep the legacy error")

		res, err := ParseResolved(open(t, headerOnlyRollout, SessionMetadata{Source: SourceCodex}),
			transcript.ParseOptions{AllowEmptyCodex: true, PreserveModelChanges: true})
		require.NoError(t, err)
		assert.Empty(t, res.Entries)
		assert.Equal(t, "example-model", res.Metadata.FirstModel)
	})

	t.Run("codex-exec forces the exec parser", func(t *testing.T) {
		res, err := ParseResolved(open(t, `{"type":"error","message":"unreachable"}`+"\n", SessionMetadata{Source: SourceCodexExec}))
		require.NoError(t, err)
		assert.Equal(t, transcript.FormatCodexExec, res.Format)
		require.Len(t, res.Entries, 1)
		assert.Equal(t, "error", res.Entries[0].Message.Content[0].Type)
	})

	t.Run("kiro-ide overlays the resolved cost path", func(t *testing.T) {
		dir := t.TempDir()
		costPath := filepath.Join(dir, "execution.json")
		require.NoError(t, os.WriteFile(costPath, []byte(`{"executionId":"e1","usageSummary":[{"unit":"credit","unitPlural":"credits","usage":0.5}],"actions":[{"actionId":"action-1","actionType":"say","output":{"message":"Done"}}]}`), 0o600))
		chat := `{"executionId":"e1","chat":[{"role":"human","content":"Hi"}],"metadata":{"modelId":"m","startTime":1767225600000}}`

		res, err := ParseResolved(open(t, chat, SessionMetadata{Source: SourceKiroIDE, CostPath: costPath}),
			transcript.ParseOptions{AllowEmptyCodex: true})
		require.NoError(t, err)
		assert.Equal(t, transcript.FormatKiroIDE, res.Format)
		require.NotNil(t, res.Metadata.TotalCost)
		assert.InDelta(t, 0.5, *res.Metadata.TotalCost, 1e-9)
		assert.Equal(t, "action-1", res.Entries[len(res.Entries)-1].RecordID)
	})

	t.Run("other sources auto-detect", func(t *testing.T) {
		res, err := ParseResolved(open(t, `{"type":"user","message":{"role":"user","content":"Hello"}}`+"\n", SessionMetadata{Source: SourceClaude}))
		require.NoError(t, err)
		assert.Equal(t, transcript.FormatClaude, res.Format)
	})
}
