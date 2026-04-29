package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/arjenschwarz/orbit/internal/variants"
)

// setupRepoWithVariants creates a git repo with a variants.json in specs/<specName>/.orbit/
// and returns the repo root path. The caller should chdir into a subdirectory to test
// subdirectory invocation.
func setupRepoWithVariants(t *testing.T, specName string) string {
	t.Helper()
	tmpDir := t.TempDir()

	// Initialize git repo
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = tmpDir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %s failed: %s", args[0], out)
	}

	// Create initial commit
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# Test"), 0644))
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "commit", "-m", "init")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	// Get HEAD
	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = tmpDir
	out, err := cmd.Output()
	require.NoError(t, err)
	baseCommit := strings.TrimSpace(string(out))

	// Create spec/.orbit/ with variants.json
	specOrbitDir := filepath.Join(tmpDir, "specs", specName, ".orbit")
	require.NoError(t, os.MkdirAll(specOrbitDir, 0755))

	worktreePath := filepath.Join(specOrbitDir, "worktrees", "orbit-impl-1-"+specName)
	require.NoError(t, os.MkdirAll(worktreePath, 0755))

	metadata := variants.VariantsMetadata{
		RunID:          "test-run",
		BaseCommit:     baseCommit,
		OriginalBranch: "main",
		StartedAt:      time.Now(),
		Variants: []*variants.Variant{
			{
				ID:           1,
				Branch:       "orbit-impl-1-" + specName,
				WorktreePath: worktreePath,
				Status:       variants.StatusCompleted,
			},
			{
				ID:           2,
				Branch:       "orbit-impl-2-" + specName,
				WorktreePath: filepath.Join(specOrbitDir, "worktrees", "orbit-impl-2-"+specName),
				Status:       variants.StatusCompleted,
			},
		},
	}

	data, err := json.MarshalIndent(metadata, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(specOrbitDir, "variants.json"), data, 0644))

	return tmpDir
}

// TestStatusCommand_FromSubdirectory verifies that `orbit status` works when
// invoked from a subdirectory within the repo (T-976).
func TestStatusCommand_FromSubdirectory(t *testing.T) {
	specName := "subdir-test"
	repoRoot := setupRepoWithVariants(t, specName)

	// Create a subdirectory and chdir into it
	subDir := filepath.Join(repoRoot, "src", "pkg")
	require.NoError(t, os.MkdirAll(subDir, 0755))

	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(originalWd) })
	require.NoError(t, os.Chdir(subDir))

	// Should succeed (previously would fail with "variants.json not found")
	err = statusCommand([]string{"--format", "json", specName})
	assert.NoError(t, err)
}

// TestCleanupCommand_FromSubdirectory verifies that `orbit cleanup` finds
// variants.json when invoked from a subdirectory (T-976).
func TestCleanupCommand_FromSubdirectory(t *testing.T) {
	specName := "subdir-cleanup"
	repoRoot := setupRepoWithVariants(t, specName)

	subDir := filepath.Join(repoRoot, "internal", "foo")
	require.NoError(t, os.MkdirAll(subDir, 0755))

	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(originalWd) })
	require.NoError(t, os.Chdir(subDir))

	// Dry-run should succeed without error
	err = cleanupCommand([]string{"--dry-run", specName})
	assert.NoError(t, err)
}

// TestFinalizeCommand_FromSubdirectory verifies that `orbit finalize` finds
// variants.json when invoked from a subdirectory (T-976).
func TestFinalizeCommand_FromSubdirectory(t *testing.T) {
	specName := "subdir-finalize"
	repoRoot := setupRepoWithVariants(t, specName)

	subDir := filepath.Join(repoRoot, "cmd", "app")
	require.NoError(t, os.MkdirAll(subDir, 0755))

	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(originalWd) })
	require.NoError(t, os.Chdir(subDir))

	// Should find variants.json and fail on variant validation (not "no variant run found")
	err = finalizeCommand([]string{"--variant", "99", specName})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "variant 99 not found")
	assert.NotContains(t, err.Error(), "no variant run found")
}

// TestConsolidateCommand_FromSubdirectory verifies that `orbit consolidate` finds
// variants.json when invoked from a subdirectory (T-976).
func TestConsolidateCommand_FromSubdirectory(t *testing.T) {
	specName := "subdir-consolidate"
	repoRoot := setupRepoWithVariants(t, specName)

	// Create .orbit.yaml in repo root so config resolution works
	require.NoError(t, os.WriteFile(
		filepath.Join(repoRoot, ".orbit.yaml"),
		[]byte("agent: claude-code\nagents:\n  claude-code:\n    type: claude-code\n"),
		0644,
	))

	subDir := filepath.Join(repoRoot, "docs")
	require.NoError(t, os.MkdirAll(subDir, 0755))

	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(originalWd) })
	require.NoError(t, os.Chdir(subDir))

	// Should find variants.json (will fail later on agent check, not on "no variant run found")
	err = consolidateCommand([]string{"--variant", "1", specName})
	require.Error(t, err)
	// Should NOT be a "no variant run found" error
	assert.NotContains(t, err.Error(), "no variant run found")
}

// TestCompareCommand_FromSubdirectory verifies that `orbit compare` finds
// variants.json when invoked from a subdirectory (T-976).
func TestCompareCommand_FromSubdirectory(t *testing.T) {
	specName := "subdir-compare"
	repoRoot := setupRepoWithVariants(t, specName)

	// Create .orbit.yaml in repo root so config resolution works
	require.NoError(t, os.WriteFile(
		filepath.Join(repoRoot, ".orbit.yaml"),
		[]byte("agent: claude-code\nagents:\n  claude-code:\n    type: claude-code\n"),
		0644,
	))

	subDir := filepath.Join(repoRoot, "internal", "bar")
	require.NoError(t, os.MkdirAll(subDir, 0755))

	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(originalWd) })
	require.NoError(t, os.Chdir(subDir))

	// Should find variants.json (will fail on "at least 2 completed variants" or agent,
	// not on "no variant run found")
	err = runCompare(t.Context(), []string{specName})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "no variant run found")
}
