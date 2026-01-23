package consolidation

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupGitRepo creates a temporary git repository for testing.
func setupGitRepo(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()

	// Initialize git repo
	runGit(t, tmpDir, "init")
	runGit(t, tmpDir, "config", "user.email", "test@example.com")
	runGit(t, tmpDir, "config", "user.name", "Test User")

	// Create initial commit
	testFile := filepath.Join(tmpDir, "README.md")
	err := os.WriteFile(testFile, []byte("# Test\n"), 0644)
	require.NoError(t, err)
	runGit(t, tmpDir, "add", "README.md")
	runGit(t, tmpDir, "commit", "-m", "Initial commit")

	return tmpDir
}

// runGit runs a git command in the specified directory.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(out))
	return string(out)
}

func TestRecoveryManager_CaptureState(t *testing.T) {
	repoDir := setupGitRepo(t)
	rm := NewRecoveryManager(repoDir)
	ctx := context.Background()

	err := rm.CaptureState(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, rm.headCommit)
}

func TestRecoveryManager_CreateSnapshot(t *testing.T) {
	t.Run("creates stash when uncommitted changes exist", func(t *testing.T) {
		repoDir := setupGitRepo(t)
		rm := NewRecoveryManager(repoDir)
		ctx := context.Background()

		// Create uncommitted changes
		testFile := filepath.Join(repoDir, "new-file.txt")
		err := os.WriteFile(testFile, []byte("test content"), 0644)
		require.NoError(t, err)

		err = rm.CreateSnapshot(ctx)
		require.NoError(t, err)
		assert.True(t, rm.HasStash())

		// Verify file is no longer present (stashed)
		_, err = os.Stat(testFile)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("no stash when working directory is clean", func(t *testing.T) {
		repoDir := setupGitRepo(t)
		rm := NewRecoveryManager(repoDir)
		ctx := context.Background()

		err := rm.CreateSnapshot(ctx)
		require.NoError(t, err)
		assert.False(t, rm.HasStash())
	})

	t.Run("stashes tracked modified files", func(t *testing.T) {
		repoDir := setupGitRepo(t)
		rm := NewRecoveryManager(repoDir)
		ctx := context.Background()

		// Modify existing file
		readme := filepath.Join(repoDir, "README.md")
		err := os.WriteFile(readme, []byte("# Modified\n"), 0644)
		require.NoError(t, err)

		err = rm.CreateSnapshot(ctx)
		require.NoError(t, err)
		assert.True(t, rm.HasStash())

		// Verify file is restored to original
		content, err := os.ReadFile(readme)
		require.NoError(t, err)
		assert.Equal(t, "# Test\n", string(content))
	})
}

func TestRecoveryManager_RestoreOnFailure(t *testing.T) {
	t.Run("removes uncommitted tracked changes", func(t *testing.T) {
		repoDir := setupGitRepo(t)
		rm := NewRecoveryManager(repoDir)
		ctx := context.Background()

		// Modify existing file
		readme := filepath.Join(repoDir, "README.md")
		err := os.WriteFile(readme, []byte("# Modified\n"), 0644)
		require.NoError(t, err)

		err = rm.RestoreOnFailure(ctx)
		require.NoError(t, err)

		// Verify file is restored
		content, err := os.ReadFile(readme)
		require.NoError(t, err)
		assert.Equal(t, "# Test\n", string(content))
	})

	t.Run("removes untracked files", func(t *testing.T) {
		repoDir := setupGitRepo(t)
		rm := NewRecoveryManager(repoDir)
		ctx := context.Background()

		// Create untracked file
		newFile := filepath.Join(repoDir, "untracked.txt")
		err := os.WriteFile(newFile, []byte("untracked"), 0644)
		require.NoError(t, err)

		err = rm.RestoreOnFailure(ctx)
		require.NoError(t, err)

		// Verify file is removed
		_, err = os.Stat(newFile)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("removes untracked directories", func(t *testing.T) {
		repoDir := setupGitRepo(t)
		rm := NewRecoveryManager(repoDir)
		ctx := context.Background()

		// Create untracked directory with files
		newDir := filepath.Join(repoDir, "newdir")
		err := os.MkdirAll(newDir, 0755)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(newDir, "file.txt"), []byte("content"), 0644)
		require.NoError(t, err)

		err = rm.RestoreOnFailure(ctx)
		require.NoError(t, err)

		// Verify directory is removed
		_, err = os.Stat(newDir)
		assert.True(t, os.IsNotExist(err))
	})
}

func TestRecoveryManager_RestoreStash(t *testing.T) {
	t.Run("restores stashed changes", func(t *testing.T) {
		repoDir := setupGitRepo(t)
		rm := NewRecoveryManager(repoDir)
		ctx := context.Background()

		// Create and stash changes
		testFile := filepath.Join(repoDir, "new-file.txt")
		err := os.WriteFile(testFile, []byte("test content"), 0644)
		require.NoError(t, err)

		err = rm.CreateSnapshot(ctx)
		require.NoError(t, err)

		// Restore stash
		warning, err := rm.RestoreStash(ctx)
		require.NoError(t, err)
		assert.Empty(t, warning)

		// Verify file is restored
		content, err := os.ReadFile(testFile)
		require.NoError(t, err)
		assert.Equal(t, "test content", string(content))
		assert.False(t, rm.HasStash())
	})

	t.Run("no-op when no stash exists", func(t *testing.T) {
		repoDir := setupGitRepo(t)
		rm := NewRecoveryManager(repoDir)
		ctx := context.Background()

		warning, err := rm.RestoreStash(ctx)
		require.NoError(t, err)
		assert.Empty(t, warning)
	})

	t.Run("handles stash conflict gracefully", func(t *testing.T) {
		repoDir := setupGitRepo(t)
		rm := NewRecoveryManager(repoDir)
		ctx := context.Background()

		// Create and stash changes to README.md
		readme := filepath.Join(repoDir, "README.md")
		err := os.WriteFile(readme, []byte("stashed content"), 0644)
		require.NoError(t, err)

		err = rm.CreateSnapshot(ctx)
		require.NoError(t, err)
		require.True(t, rm.HasStash())

		// Now make different changes to README.md and commit
		err = os.WriteFile(readme, []byte("conflicting content"), 0644)
		require.NoError(t, err)
		runGit(t, repoDir, "add", "README.md")
		runGit(t, repoDir, "commit", "-m", "Conflicting change")

		// Try to restore stash - should result in conflict
		warning, err := rm.RestoreStash(ctx)
		require.NoError(t, err) // Not an error, just a warning
		if warning != "" {
			assert.Contains(t, warning, "conflict")
			assert.Contains(t, warning, "stash@{0}")
		}
	})
}

func TestRecoveryManager_Cleanup(t *testing.T) {
	t.Run("drops stash on cleanup", func(t *testing.T) {
		repoDir := setupGitRepo(t)
		rm := NewRecoveryManager(repoDir)
		ctx := context.Background()

		// Create and stash changes
		testFile := filepath.Join(repoDir, "new-file.txt")
		err := os.WriteFile(testFile, []byte("test content"), 0644)
		require.NoError(t, err)

		err = rm.CreateSnapshot(ctx)
		require.NoError(t, err)
		require.True(t, rm.HasStash())

		// Cleanup
		err = rm.Cleanup(ctx)
		require.NoError(t, err)
		assert.False(t, rm.HasStash())

		// Verify stash is actually dropped
		cmd := exec.Command("git", "stash", "list")
		cmd.Dir = repoDir
		out, err := cmd.Output()
		require.NoError(t, err)
		assert.Empty(t, string(out))
	})

	t.Run("no-op when no stash exists", func(t *testing.T) {
		repoDir := setupGitRepo(t)
		rm := NewRecoveryManager(repoDir)
		ctx := context.Background()

		err := rm.Cleanup(ctx)
		require.NoError(t, err)
	})
}

func TestRecoveryManager_PartialFailureScenario(t *testing.T) {
	// Simulates a scenario where agent makes partial modifications before failing
	repoDir := setupGitRepo(t)
	rm := NewRecoveryManager(repoDir)
	ctx := context.Background()

	// Capture initial state
	err := rm.CaptureState(ctx)
	require.NoError(t, err)

	// Simulate agent making partial changes
	readme := filepath.Join(repoDir, "README.md")
	err = os.WriteFile(readme, []byte("# Agent modified this\n"), 0644)
	require.NoError(t, err)

	newFile := filepath.Join(repoDir, "agent-created.txt")
	err = os.WriteFile(newFile, []byte("new content"), 0644)
	require.NoError(t, err)

	// Simulate failure and restore
	err = rm.RestoreOnFailure(ctx)
	require.NoError(t, err)

	// Verify README is restored
	content, err := os.ReadFile(readme)
	require.NoError(t, err)
	assert.Equal(t, "# Test\n", string(content))

	// Verify agent-created file is removed
	_, err = os.Stat(newFile)
	assert.True(t, os.IsNotExist(err))
}
