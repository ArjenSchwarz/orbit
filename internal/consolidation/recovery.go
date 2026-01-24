package consolidation

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// RecoveryManager handles git state management for safe consolidation.
// It captures the worktree state before agent runs and restores it on failure.
type RecoveryManager struct {
	worktreePath string
	stashRef     string
	hasStash     bool
	headCommit   string // Captured HEAD commit for restoration
}

// NewRecoveryManager creates a recovery manager for a worktree.
func NewRecoveryManager(worktreePath string) *RecoveryManager {
	return &RecoveryManager{
		worktreePath: worktreePath,
	}
}

// CaptureState records the current worktree state before agent runs.
// Called for ALL runs (not just --allow-dirty) to enable cleanup on failure.
func (rm *RecoveryManager) CaptureState(ctx context.Context) error {
	// Record current HEAD commit for potential restoration
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = rm.worktreePath
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to capture HEAD: %w", err)
	}
	rm.headCommit = strings.TrimSpace(string(out))
	return nil
}

// CreateSnapshot stashes uncommitted changes if present.
// Only called when --allow-dirty is used.
func (rm *RecoveryManager) CreateSnapshot(ctx context.Context) error {
	// Check if there are uncommitted changes
	hasChanges, err := rm.hasUncommittedChanges(ctx)
	if err != nil {
		return fmt.Errorf("failed to check uncommitted changes: %w", err)
	}

	if !hasChanges {
		rm.hasStash = false
		return nil
	}

	// Create a stash with all changes including untracked files
	cmd := exec.CommandContext(ctx, "git", "stash", "push", "--include-untracked", "-m", "orbit-consolidation-snapshot")
	cmd.Dir = rm.worktreePath
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create stash: %s", stderr.String())
	}

	// Get the stash reference
	cmd = exec.CommandContext(ctx, "git", "stash", "list", "-1")
	cmd.Dir = rm.worktreePath
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get stash ref: %w", err)
	}

	stashList := strings.TrimSpace(string(out))
	if stashList != "" && strings.Contains(stashList, "orbit-consolidation-snapshot") {
		rm.stashRef = "stash@{0}"
		rm.hasStash = true
	}

	return nil
}

// RestoreOnFailure restores worktree to pre-session state if agent fails without committing.
// Resets HEAD to the captured commit, then uses git checkout and git clean to remove modifications.
func (rm *RecoveryManager) RestoreOnFailure(ctx context.Context) error {
	// Reset HEAD to captured commit if we have one and HEAD has changed
	if rm.headCommit != "" {
		// Check current HEAD
		cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
		cmd.Dir = rm.worktreePath
		out, err := cmd.Output()
		if err == nil {
			currentHead := strings.TrimSpace(string(out))
			if currentHead != rm.headCommit {
				// HEAD has changed - reset to captured commit
				cmd = exec.CommandContext(ctx, "git", "reset", "--hard", rm.headCommit)
				cmd.Dir = rm.worktreePath
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				if err := cmd.Run(); err != nil {
					return fmt.Errorf("failed to reset HEAD: %s", stderr.String())
				}
			}
		}
	}

	// Reset any uncommitted changes
	cmd := exec.CommandContext(ctx, "git", "checkout", "--", ".")
	cmd.Dir = rm.worktreePath
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to checkout: %s", stderr.String())
	}

	// Remove untracked files
	cmd = exec.CommandContext(ctx, "git", "clean", "-fd")
	cmd.Dir = rm.worktreePath
	stderr.Reset()
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to clean: %s", stderr.String())
	}

	return nil
}

// RestoreStash restores stashed changes (for --allow-dirty interrupt).
// If stash pop causes merge conflicts:
// 1. Leave the stash in place (don't drop it)
// 2. Return a warning message (not an error) about conflicts
// 3. Return nil (the stash is safe)
func (rm *RecoveryManager) RestoreStash(ctx context.Context) (warning string, err error) {
	if !rm.hasStash {
		return "", nil
	}

	// Try to apply the stash
	cmd := exec.CommandContext(ctx, "git", "stash", "pop")
	cmd.Dir = rm.worktreePath
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	if runErr != nil {
		combinedOutput := stdout.String() + stderr.String()
		// Check if it's a conflict error - git stash pop can report conflicts in stdout or stderr
		if strings.Contains(combinedOutput, "CONFLICT") || strings.Contains(combinedOutput, "conflict") {
			// Stash pop with conflicts leaves the stash in place
			// Return a warning but not an error
			return "Stash restore caused conflicts. Your changes are preserved in stash@{0}. Resolve manually with: git stash pop", nil
		}
		// Check for common stash-specific errors that indicate stash was not dropped
		if strings.Contains(combinedOutput, "Merge conflict") || strings.Contains(combinedOutput, "uncommitted changes") {
			return "Stash restore caused conflicts. Your changes are preserved in stash@{0}. Resolve manually with: git stash pop", nil
		}
		return "", fmt.Errorf("failed to restore stash: %s", combinedOutput)
	}

	rm.hasStash = false
	rm.stashRef = ""
	return "", nil
}

// Cleanup removes recovery artifacts after successful completion.
func (rm *RecoveryManager) Cleanup(ctx context.Context) error {
	// If we still have a stash (shouldn't happen after successful completion),
	// drop it to avoid leaving orphaned stashes
	if rm.hasStash {
		cmd := exec.CommandContext(ctx, "git", "stash", "drop", rm.stashRef)
		cmd.Dir = rm.worktreePath
		_ = cmd.Run() // Best effort - don't fail cleanup if stash drop fails
		rm.hasStash = false
	}
	return nil
}

// HasStash returns true if a recovery stash was created.
func (rm *RecoveryManager) HasStash() bool {
	return rm.hasStash
}

// hasUncommittedChanges returns true if the worktree has uncommitted changes.
func (rm *RecoveryManager) hasUncommittedChanges(ctx context.Context) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = rm.worktreePath
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return len(strings.TrimSpace(string(out))) > 0, nil
}
