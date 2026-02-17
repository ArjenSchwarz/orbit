package consolidation

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// GitOps defines the git operations used by the consolidation package.
// This interface enables testing without real git repositories and maintains
// consistency with the project's pattern of abstracting git behind interfaces.
type GitOps interface {
	// GetHeadCommit returns the HEAD commit SHA for the worktree.
	GetHeadCommit(ctx context.Context) (string, error)

	// HasUncommittedChanges returns true if the worktree has uncommitted changes.
	HasUncommittedChanges(ctx context.Context) (bool, error)

	// ResetHard resets the worktree to the specified commit, discarding all changes.
	ResetHard(ctx context.Context, commit string) error

	// CheckoutAll resets all tracked file modifications in the worktree.
	CheckoutAll(ctx context.Context) error

	// CleanUntracked removes untracked files and directories from the worktree.
	CleanUntracked(ctx context.Context) error

	// StashPush creates a stash including untracked files with the given message.
	// Returns the stash reference (e.g., "stash@{0}") if changes were stashed.
	StashPush(ctx context.Context, message string) (ref string, hasStash bool, err error)

	// StashPop applies and drops the most recent stash entry.
	// Returns a warning message if conflicts occur (stash is preserved in that case).
	StashPop(ctx context.Context) (warning string, err error)

	// StashDrop drops the specified stash reference.
	StashDrop(ctx context.Context, ref string) error

	// RevertCommit creates a new commit that reverts the specified commit.
	RevertCommit(ctx context.Context, commitSHA string) error

	// LogOneline returns recent commits in "%H %s" format (full SHA + subject).
	// limit specifies the maximum number of commits to return.
	LogOneline(ctx context.Context, limit int) (string, error)

	// GetCommitSubject returns the subject line of a single commit.
	GetCommitSubject(ctx context.Context, commitSHA string) (string, error)
}

// execGitOps implements GitOps using real git commands via exec.Command.
type execGitOps struct {
	worktreePath string
}

// NewExecGitOps creates a GitOps implementation that executes real git commands
// in the specified worktree directory.
func NewExecGitOps(worktreePath string) GitOps {
	return &execGitOps{worktreePath: worktreePath}
}

func (g *execGitOps) GetHeadCommit(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = g.worktreePath
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (g *execGitOps) HasUncommittedChanges(ctx context.Context) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = g.worktreePath
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to check git status: %w", err)
	}
	return len(strings.TrimSpace(string(out))) > 0, nil
}

func (g *execGitOps) ResetHard(ctx context.Context, commit string) error {
	cmd := exec.CommandContext(ctx, "git", "reset", "--hard", commit)
	cmd.Dir = g.worktreePath
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to reset HEAD: %s", stderr.String())
	}
	return nil
}

func (g *execGitOps) CheckoutAll(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "checkout", "--", ".")
	cmd.Dir = g.worktreePath
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to checkout: %s", stderr.String())
	}
	return nil
}

func (g *execGitOps) CleanUntracked(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "clean", "-fd")
	cmd.Dir = g.worktreePath
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to clean: %s", stderr.String())
	}
	return nil
}

func (g *execGitOps) StashPush(ctx context.Context, message string) (string, bool, error) {
	// First check if there are changes to stash
	hasChanges, err := g.HasUncommittedChanges(ctx)
	if err != nil {
		return "", false, fmt.Errorf("failed to check uncommitted changes: %w", err)
	}
	if !hasChanges {
		return "", false, nil
	}

	// Create stash with untracked files
	cmd := exec.CommandContext(ctx, "git", "stash", "push", "--include-untracked", "-m", message)
	cmd.Dir = g.worktreePath
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", false, fmt.Errorf("failed to create stash: %s", stderr.String())
	}

	// Get the stash reference
	cmd = exec.CommandContext(ctx, "git", "stash", "list", "-1")
	cmd.Dir = g.worktreePath
	out, err := cmd.Output()
	if err != nil {
		return "", false, fmt.Errorf("failed to get stash ref: %w", err)
	}

	stashList := strings.TrimSpace(string(out))
	if stashList != "" && strings.Contains(stashList, message) {
		return "stash@{0}", true, nil
	}

	return "", false, nil
}

func (g *execGitOps) StashPop(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "stash", "pop")
	cmd.Dir = g.worktreePath
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	if runErr != nil {
		combinedOutput := stdout.String() + stderr.String()
		if strings.Contains(combinedOutput, "CONFLICT") || strings.Contains(combinedOutput, "conflict") ||
			strings.Contains(combinedOutput, "Merge conflict") || strings.Contains(combinedOutput, "uncommitted changes") {
			return "Stash restore caused conflicts. Your changes are preserved in stash@{0}. Resolve manually with: git stash pop", nil
		}
		return "", fmt.Errorf("failed to restore stash: %s", combinedOutput)
	}

	return "", nil
}

func (g *execGitOps) StashDrop(ctx context.Context, ref string) error {
	cmd := exec.CommandContext(ctx, "git", "stash", "drop", ref)
	cmd.Dir = g.worktreePath
	_ = cmd.Run() // Best effort
	return nil
}

func (g *execGitOps) RevertCommit(ctx context.Context, commitSHA string) error {
	cmd := exec.CommandContext(ctx, "git", "revert", "--no-edit", commitSHA)
	cmd.Dir = g.worktreePath
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to revert commit: %s", stderr.String())
	}
	return nil
}

func (g *execGitOps) LogOneline(ctx context.Context, limit int) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "log", "-n", fmt.Sprintf("%d", limit), "--oneline", "--format=%H %s")
	cmd.Dir = g.worktreePath
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to read git log: %w", err)
	}
	return string(out), nil
}

func (g *execGitOps) GetCommitSubject(ctx context.Context, commitSHA string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "log", "-1", "--format=%s", commitSHA)
	cmd.Dir = g.worktreePath
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("commit not found")
	}
	return strings.TrimSpace(string(out)), nil
}
