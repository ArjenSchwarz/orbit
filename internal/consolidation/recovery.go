package consolidation

import (
	"context"
	"fmt"
)

// RecoveryManager handles git state management for safe consolidation.
// It captures the worktree state before agent runs and restores it on failure.
type RecoveryManager struct {
	worktreePath string
	git          GitOps
	stashRef     string
	hasStash     bool
	headCommit   string // Captured HEAD commit for restoration
}

// NewRecoveryManager creates a recovery manager for a worktree.
func NewRecoveryManager(worktreePath string, git GitOps) *RecoveryManager {
	return &RecoveryManager{
		worktreePath: worktreePath,
		git:          git,
	}
}

// SetGitOps replaces the GitOps implementation used by this recovery manager.
func (rm *RecoveryManager) SetGitOps(git GitOps) {
	rm.git = git
}

// CaptureState records the current worktree state before agent runs.
// Called for ALL runs (not just --allow-dirty) to enable cleanup on failure.
func (rm *RecoveryManager) CaptureState(ctx context.Context) error {
	// Record current HEAD commit for potential restoration
	commit, err := rm.git.GetHeadCommit(ctx)
	if err != nil {
		return fmt.Errorf("failed to capture HEAD: %w", err)
	}
	rm.headCommit = commit
	return nil
}

// CreateSnapshot stashes uncommitted changes if present.
// Only called when --allow-dirty is used.
func (rm *RecoveryManager) CreateSnapshot(ctx context.Context) error {
	ref, hasStash, err := rm.git.StashPush(ctx, "orbit-consolidation-snapshot")
	if err != nil {
		return err
	}

	rm.stashRef = ref
	rm.hasStash = hasStash
	return nil
}

// RestoreOnFailure restores worktree to pre-session state if agent fails without committing.
// Resets HEAD to the captured commit, then uses git checkout and git clean to remove modifications.
func (rm *RecoveryManager) RestoreOnFailure(ctx context.Context) error {
	// Reset HEAD to captured commit if we have one and HEAD has changed
	if rm.headCommit != "" {
		currentHead, err := rm.git.GetHeadCommit(ctx)
		if err == nil && currentHead != rm.headCommit {
			// HEAD has changed - reset to captured commit
			if err := rm.git.ResetHard(ctx, rm.headCommit); err != nil {
				return err
			}
		}
	}

	// Reset any uncommitted changes
	if err := rm.git.CheckoutAll(ctx); err != nil {
		return err
	}

	// Remove untracked files
	if err := rm.git.CleanUntracked(ctx); err != nil {
		return err
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

	warning, err = rm.git.StashPop(ctx)
	if err != nil {
		return "", err
	}

	if warning == "" {
		rm.hasStash = false
		rm.stashRef = ""
	}

	return warning, nil
}

// Cleanup removes recovery artifacts after successful completion.
func (rm *RecoveryManager) Cleanup(ctx context.Context) error {
	// If we still have a stash (shouldn't happen after successful completion),
	// drop it to avoid leaving orphaned stashes
	if rm.hasStash {
		_ = rm.git.StashDrop(ctx, rm.stashRef) // Best effort
		rm.hasStash = false
	}
	return nil
}

// HasStash returns true if a recovery stash was created.
func (rm *RecoveryManager) HasStash() bool {
	return rm.hasStash
}
