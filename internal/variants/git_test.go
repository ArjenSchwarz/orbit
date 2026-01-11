package variants

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupTestRepo creates a temporary git repository for testing.
// Returns the path to the repo and a cleanup function.
func setupTestRepo(t *testing.T) (string, func()) {
	t.Helper()

	dir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	cleanup := func() {
		_ = os.RemoveAll(dir)
	}

	// Initialize git repo
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")

	// Create initial commit
	testFile := filepath.Join(dir, "README.md")
	if err := os.WriteFile(testFile, []byte("# Test\n"), 0644); err != nil {
		cleanup()
		t.Fatalf("failed to create test file: %v", err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "Initial commit")

	return dir, cleanup
}

// runGit runs a git command in the specified directory.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestGetCurrentBranch(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	git := NewGit(dir)

	// Default branch should be main or master
	branch, err := git.GetCurrentBranch()
	if err != nil {
		t.Fatalf("GetCurrentBranch failed: %v", err)
	}

	// Git uses 'master' or 'main' depending on config
	if branch != "master" && branch != "main" {
		t.Errorf("expected 'master' or 'main', got %q", branch)
	}

	// Create and checkout a new branch
	runGit(t, dir, "checkout", "-b", "feature/test")
	branch, err = git.GetCurrentBranch()
	if err != nil {
		t.Fatalf("GetCurrentBranch after checkout failed: %v", err)
	}
	if branch != "feature/test" {
		t.Errorf("expected 'feature/test', got %q", branch)
	}
}

func TestGetHeadCommit(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	git := NewGit(dir)

	commit, err := git.GetHeadCommit()
	if err != nil {
		t.Fatalf("GetHeadCommit failed: %v", err)
	}

	// Commit SHA should be 40 characters
	if len(commit) != 40 {
		t.Errorf("expected 40-char SHA, got %d chars: %q", len(commit), commit)
	}

	// Verify it matches git rev-parse output
	expected := runGit(t, dir, "rev-parse", "HEAD")
	if commit != expected {
		t.Errorf("expected %q, got %q", expected, commit)
	}
}

func TestCreateBranch(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	git := NewGit(dir)

	err := git.CreateBranch("test-branch")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	// Verify branch exists
	branches := runGit(t, dir, "branch", "--list", "test-branch")
	if !strings.Contains(branches, "test-branch") {
		t.Error("branch was not created")
	}
}

func TestCreateBranch_AlreadyExists(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	git := NewGit(dir)

	// Create branch first
	runGit(t, dir, "branch", "existing-branch")

	// Attempt to create same branch should fail
	err := git.CreateBranch("existing-branch")
	if err == nil {
		t.Error("expected error when creating existing branch")
	}
}

func TestCreateWorktree(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	git := NewGit(dir)

	// Create a branch for the worktree
	branchName := "worktree-branch"
	err := git.CreateBranch(branchName)
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	// Create worktree
	worktreePath := filepath.Join(dir, ".orbit", "worktrees", "test-wt")
	ctx := context.Background()
	err = git.CreateWorktree(ctx, worktreePath, branchName)
	if err != nil {
		t.Fatalf("CreateWorktree failed: %v", err)
	}

	// Verify worktree exists
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		t.Error("worktree directory was not created")
	}

	// Verify worktree is listed
	worktrees := runGit(t, dir, "worktree", "list")
	if !strings.Contains(worktrees, worktreePath) {
		t.Error("worktree not listed")
	}
}

func TestRemoveWorktree(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	git := NewGit(dir)

	// Setup worktree
	branchName := "wt-remove-branch"
	runGit(t, dir, "branch", branchName)
	worktreePath := filepath.Join(dir, ".orbit", "worktrees", "remove-wt")
	runGit(t, dir, "worktree", "add", worktreePath, branchName)

	// Remove worktree
	ctx := context.Background()
	err := git.RemoveWorktree(ctx, worktreePath)
	if err != nil {
		t.Fatalf("RemoveWorktree failed: %v", err)
	}

	// Verify worktree is gone
	worktrees := runGit(t, dir, "worktree", "list")
	if strings.Contains(worktrees, worktreePath) {
		t.Error("worktree still listed after removal")
	}
}

func TestDeleteBranch(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	git := NewGit(dir)

	// Create and then delete a branch
	runGit(t, dir, "branch", "delete-me")

	err := git.DeleteBranch("delete-me")
	if err != nil {
		t.Fatalf("DeleteBranch failed: %v", err)
	}

	// Verify branch is gone
	branches := runGit(t, dir, "branch", "--list")
	if strings.Contains(branches, "delete-me") {
		t.Error("branch still exists after deletion")
	}
}

func TestGetDiff(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	git := NewGit(dir)

	// Get base commit
	baseCommit, _ := git.GetHeadCommit()

	// Create a branch and worktree
	branchName := "diff-branch"
	runGit(t, dir, "branch", branchName)
	worktreePath := filepath.Join(dir, ".orbit", "worktrees", "diff-wt")
	runGit(t, dir, "worktree", "add", worktreePath, branchName)

	// Make a change in the worktree
	testFile := filepath.Join(worktreePath, "new-file.txt")
	if err := os.WriteFile(testFile, []byte("new content\n"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	runGit(t, worktreePath, "add", ".")
	runGit(t, worktreePath, "commit", "-m", "Add new file")

	// Get diff
	ctx := context.Background()
	diff, err := git.GetDiff(ctx, worktreePath, baseCommit)
	if err != nil {
		t.Fatalf("GetDiff failed: %v", err)
	}

	// Verify diff contains expected content
	if !strings.Contains(diff, "new-file.txt") {
		t.Error("diff does not contain expected file")
	}
	if !strings.Contains(diff, "new content") {
		t.Error("diff does not contain expected content")
	}
}

func TestGetDiff_NoChanges(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	git := NewGit(dir)

	// Get current commit
	commit, _ := git.GetHeadCommit()

	// Diff from HEAD to HEAD should be empty
	ctx := context.Background()
	diff, err := git.GetDiff(ctx, dir, commit)
	if err != nil {
		t.Fatalf("GetDiff failed: %v", err)
	}

	if diff != "" {
		t.Errorf("expected empty diff, got %q", diff)
	}
}

func TestBranchHasDiverged(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	git := NewGit(dir)

	// Get base commit
	baseCommit, _ := git.GetHeadCommit()
	baseBranch, _ := git.GetCurrentBranch()

	// Check that base branch hasn't diverged from itself
	diverged, err := git.BranchHasDiverged(baseBranch, baseCommit)
	if err != nil {
		t.Fatalf("BranchHasDiverged failed: %v", err)
	}
	if diverged {
		t.Error("branch should not have diverged from its own commit")
	}

	// Add a new commit
	testFile := filepath.Join(dir, "diverge.txt")
	if err := os.WriteFile(testFile, []byte("new content\n"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "Add diverge file")

	// Now the branch should have diverged
	diverged, err = git.BranchHasDiverged(baseBranch, baseCommit)
	if err != nil {
		t.Fatalf("BranchHasDiverged failed: %v", err)
	}
	if !diverged {
		t.Error("branch should have diverged after new commit")
	}
}

func TestHasUncommittedChanges(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	git := NewGit(dir)

	// Clean repo should have no changes
	hasChanges, err := git.HasUncommittedChanges()
	if err != nil {
		t.Fatalf("HasUncommittedChanges failed: %v", err)
	}
	if hasChanges {
		t.Error("expected no uncommitted changes in clean repo")
	}

	// Create an untracked file
	testFile := filepath.Join(dir, "untracked.txt")
	if err := os.WriteFile(testFile, []byte("content\n"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	hasChanges, err = git.HasUncommittedChanges()
	if err != nil {
		t.Fatalf("HasUncommittedChanges failed: %v", err)
	}
	if !hasChanges {
		t.Error("expected uncommitted changes with untracked file")
	}

	// Stage the file
	runGit(t, dir, "add", ".")

	hasChanges, err = git.HasUncommittedChanges()
	if err != nil {
		t.Fatalf("HasUncommittedChanges failed: %v", err)
	}
	if !hasChanges {
		t.Error("expected uncommitted changes with staged file")
	}

	// Commit the file
	runGit(t, dir, "commit", "-m", "Add file")

	hasChanges, err = git.HasUncommittedChanges()
	if err != nil {
		t.Fatalf("HasUncommittedChanges failed: %v", err)
	}
	if hasChanges {
		t.Error("expected no uncommitted changes after commit")
	}
}

func TestHasUncommittedChanges_ModifiedFile(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	git := NewGit(dir)

	// Modify an existing file
	testFile := filepath.Join(dir, "README.md")
	if err := os.WriteFile(testFile, []byte("# Modified\n"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	hasChanges, err := git.HasUncommittedChanges()
	if err != nil {
		t.Fatalf("HasUncommittedChanges failed: %v", err)
	}
	if !hasChanges {
		t.Error("expected uncommitted changes with modified file")
	}
}

func TestRebase(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	git := NewGit(dir)
	baseBranch, _ := git.GetCurrentBranch()

	// Create a feature branch with a commit (ahead of base, not diverged)
	runGit(t, dir, "checkout", "-b", "feature")
	featureFile := filepath.Join(dir, "feature.txt")
	if err := os.WriteFile(featureFile, []byte("feature content\n"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "Add feature")

	// Go back to base branch (don't add any commits - this keeps feature ahead, not diverged)
	runGit(t, dir, "checkout", baseBranch)

	// "Rebase" feature onto base (really a fast-forward merge of feature into base)
	ctx := context.Background()
	err := git.Rebase(ctx, "feature", baseBranch)
	if err != nil {
		t.Fatalf("Rebase failed: %v", err)
	}

	// Verify we're on base branch (target branch) after rebase
	currentBranch, _ := git.GetCurrentBranch()
	if currentBranch != baseBranch {
		t.Errorf("expected to be on %q branch, got %q", baseBranch, currentBranch)
	}

	// Verify feature.txt exists (merged from feature branch)
	if _, err := os.Stat(featureFile); os.IsNotExist(err) {
		t.Error("feature.txt should exist after rebase")
	}
}

func TestRebase_FailsWhenDiverged(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	git := NewGit(dir)
	baseBranch, _ := git.GetCurrentBranch()

	// Create a feature branch with a commit
	runGit(t, dir, "checkout", "-b", "feature")
	featureFile := filepath.Join(dir, "feature.txt")
	if err := os.WriteFile(featureFile, []byte("feature content\n"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "Add feature")

	// Go back to base branch and add a conflicting commit (causes divergence)
	runGit(t, dir, "checkout", baseBranch)
	baseFile := filepath.Join(dir, "base.txt")
	if err := os.WriteFile(baseFile, []byte("base content\n"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "Add base file")

	// Rebase should fail because branches have diverged (--ff-only will reject)
	ctx := context.Background()
	err := git.Rebase(ctx, "feature", baseBranch)
	if err == nil {
		t.Fatal("Rebase should have failed when branches have diverged")
	}

	// Verify error message mentions merge failure
	if !strings.Contains(err.Error(), "merge") {
		t.Errorf("error should mention merge, got: %v", err)
	}
}
