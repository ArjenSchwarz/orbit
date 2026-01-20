package variants

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// GitClient defines the interface for git operations used by the variant manager.
// Long-running operations accept context.Context for cancellation support.
type GitClient interface {
	// GetCurrentBranch returns the current branch name.
	GetCurrentBranch() (string, error)

	// GetHeadCommit returns the current HEAD commit SHA.
	GetHeadCommit() (string, error)

	// CreateBranch creates a new branch from HEAD.
	CreateBranch(name string) error

	// CreateWorktree creates a worktree for a branch at the specified path.
	CreateWorktree(ctx context.Context, path, branch string) error

	// RemoveWorktree removes a worktree at the specified path.
	RemoveWorktree(ctx context.Context, path string) error

	// DeleteBranch deletes a local branch.
	DeleteBranch(name string) error

	// GetDiff returns a unified diff from baseCommit for a worktree.
	GetDiff(ctx context.Context, worktreePath, baseCommit string) (string, error)

	// Rebase rebases source branch onto target branch.
	Rebase(ctx context.Context, sourceBranch, targetBranch string) error

	// BranchHasDiverged checks if branch has new commits since baseCommit.
	BranchHasDiverged(branch, baseCommit string) (bool, error)

	// HasUncommittedChanges returns true if the working directory has uncommitted changes.
	HasUncommittedChanges() (bool, error)

	// GetCommitLog returns commit messages from baseCommit to HEAD in a worktree.
	GetCommitLog(ctx context.Context, worktreePath, baseCommit string) ([]string, error)

	// GetDiffStat returns a summary of changes (files changed, insertions, deletions) from baseCommit.
	GetDiffStat(ctx context.Context, worktreePath, baseCommit string) (string, error)
}

// Git implements GitClient with real git command execution.
type Git struct {
	repoRoot string
}

// NewGit creates a new Git client rooted at the specified repository path.
func NewGit(repoRoot string) *Git {
	return &Git{repoRoot: repoRoot}
}

// GetCurrentBranch returns the current branch name.
func (g *Git) GetCurrentBranch() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = g.repoRoot
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("get current branch: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// GetHeadCommit returns the current HEAD commit SHA.
func (g *Git) GetHeadCommit() (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = g.repoRoot
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("get head commit: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// CreateBranch creates a new branch from HEAD.
func (g *Git) CreateBranch(name string) error {
	cmd := exec.Command("git", "branch", name)
	cmd.Dir = g.repoRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("create branch %s: %s", name, stderr.String())
	}
	return nil
}

// CreateWorktree creates a worktree for a branch at the specified path.
func (g *Git) CreateWorktree(ctx context.Context, path, branch string) error {
	cmd := exec.CommandContext(ctx, "git", "worktree", "add", path, branch)
	cmd.Dir = g.repoRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("create worktree cancelled: %w", ctx.Err())
		}
		return fmt.Errorf("create worktree at %s for branch %s: %s", path, branch, stderr.String())
	}
	return nil
}

// RemoveWorktree removes a worktree at the specified path.
func (g *Git) RemoveWorktree(ctx context.Context, path string) error {
	// Use --force to remove even if there are untracked files
	cmd := exec.CommandContext(ctx, "git", "worktree", "remove", "--force", path)
	cmd.Dir = g.repoRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("remove worktree cancelled: %w", ctx.Err())
		}
		return fmt.Errorf("remove worktree at %s: %s", path, stderr.String())
	}
	return nil
}

// DeleteBranch deletes a local branch.
func (g *Git) DeleteBranch(name string) error {
	// Use -D to force delete even if not fully merged
	cmd := exec.Command("git", "branch", "-D", name)
	cmd.Dir = g.repoRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("delete branch %s: %s", name, stderr.String())
	}
	return nil
}

// GetDiff returns a unified diff from baseCommit for a worktree.
func (g *Git) GetDiff(ctx context.Context, worktreePath, baseCommit string) (string, error) {
	// Run git diff in the worktree directory to get changes from base commit
	cmd := exec.CommandContext(ctx, "git", "diff", baseCommit+"..HEAD")
	cmd.Dir = worktreePath
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("get diff cancelled: %w", ctx.Err())
		}
		var stderr bytes.Buffer
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr.Write(exitErr.Stderr)
		}
		return "", fmt.Errorf("get diff from %s: %s", baseCommit, stderr.String())
	}
	return string(out), nil
}

// Rebase rebases source branch onto target branch.
// It uses a fast-forward merge approach: checks out the target branch, then merges the source.
// This keeps the repository on the target branch after completion.
// NOTE: The caller should verify that target branch has not diverged before calling this,
// as the fast-forward merge will fail if there are conflicting changes.
func (g *Git) Rebase(ctx context.Context, sourceBranch, targetBranch string) error {
	// First checkout the target branch (where we want to end up)
	checkoutCmd := exec.CommandContext(ctx, "git", "checkout", targetBranch)
	checkoutCmd.Dir = g.repoRoot
	var stderr bytes.Buffer
	checkoutCmd.Stderr = &stderr
	if err := checkoutCmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("checkout cancelled: %w", ctx.Err())
		}
		return fmt.Errorf("checkout %s: %s", targetBranch, stderr.String())
	}

	// Merge the source branch into target (fast-forward since base hasn't diverged)
	// Using --ff-only ensures we fail cleanly if a fast-forward isn't possible
	mergeCmd := exec.CommandContext(ctx, "git", "merge", "--ff-only", sourceBranch)
	mergeCmd.Dir = g.repoRoot
	stderr.Reset()
	mergeCmd.Stderr = &stderr
	if err := mergeCmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("merge cancelled: %w", ctx.Err())
		}
		return fmt.Errorf("merge %s into %s: %s", sourceBranch, targetBranch, stderr.String())
	}

	return nil
}

// BranchHasDiverged checks if branch has new commits since baseCommit.
// Returns true if the branch has moved forward from baseCommit, meaning
// a fast-forward merge would not be possible without first rebasing.
// This is used to detect if the original branch has been modified while
// variant implementations were in progress.
func (g *Git) BranchHasDiverged(branch, baseCommit string) (bool, error) {
	// Get the commit that branch points to
	cmd := exec.Command("git", "rev-parse", branch)
	cmd.Dir = g.repoRoot
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("get branch commit: %w", err)
	}
	branchCommit := strings.TrimSpace(string(out))

	// Check if baseCommit is an ancestor of branchCommit
	// If they're the same, the branch hasn't diverged
	if branchCommit == baseCommit {
		return false, nil
	}

	// Check if baseCommit is an ancestor of branchCommit
	// If it is, the branch has diverged (has new commits)
	cmd = exec.Command("git", "merge-base", "--is-ancestor", baseCommit, branchCommit)
	cmd.Dir = g.repoRoot
	err = cmd.Run()
	if err != nil {
		// Exit code 1 means baseCommit is NOT an ancestor
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// This means the branch has diverged in a way that baseCommit is not in its history
			return true, nil
		}
		return false, fmt.Errorf("check ancestor: %w", err)
	}

	// baseCommit IS an ancestor and commits differ, so branch has new commits
	return true, nil
}

// HasUncommittedChanges returns true if the working directory has uncommitted changes.
func (g *Git) HasUncommittedChanges() (bool, error) {
	// Check for both staged and unstaged changes
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = g.repoRoot
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("check uncommitted changes: %w", err)
	}
	// If there's any output, there are uncommitted changes
	return len(strings.TrimSpace(string(out))) > 0, nil
}

// GetCommitLog returns commit messages from baseCommit to HEAD in a worktree.
// Each entry contains the short hash and subject line.
func (g *Git) GetCommitLog(ctx context.Context, worktreePath, baseCommit string) ([]string, error) {
	// Get commit messages with format: short hash - subject
	cmd := exec.CommandContext(ctx, "git", "log", "--oneline", "--no-decorate", baseCommit+"..HEAD")
	cmd.Dir = worktreePath
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("get commit log cancelled: %w", ctx.Err())
		}
		var stderr bytes.Buffer
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr.Write(exitErr.Stderr)
		}
		return nil, fmt.Errorf("get commit log from %s: %s", baseCommit, stderr.String())
	}

	// Split into lines, filtering empty
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return []string{}, nil
	}
	return lines, nil
}

// GetDiffStat returns a summary of changes (files changed, insertions, deletions) from baseCommit.
func (g *Git) GetDiffStat(ctx context.Context, worktreePath, baseCommit string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--stat", baseCommit+"..HEAD")
	cmd.Dir = worktreePath
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("get diff stat cancelled: %w", ctx.Err())
		}
		var stderr bytes.Buffer
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr.Write(exitErr.Stderr)
		}
		return "", fmt.Errorf("get diff stat from %s: %s", baseCommit, stderr.String())
	}
	return string(out), nil
}
