package comparison

import (
	"context"
	"testing"
	"time"

	"github.com/arjenschwarz/orbit/internal/variants"
)

// TestGatherDiffs_RespectsPreCancelledContext verifies that GatherDiffs
// returns immediately when given an already-cancelled context, without
// making any GetDiff calls. The standard MockGit ignores context in
// GetDiff, so this test exercises GatherDiffs' own loop-level check.
func TestGatherDiffs_RespectsPreCancelledContext(t *testing.T) {
	mock := variants.NewMockGit()
	mock.Diff = "diff --git a/file.go b/file.go\n"

	gatherer := NewDiffGatherer(mock)

	variantList := []*variants.Variant{
		{ID: 1, Status: variants.StatusCompleted, WorktreePath: "/worktree/v1"},
		{ID: 2, Status: variants.StatusCompleted, WorktreePath: "/worktree/v2"},
		{ID: 3, Status: variants.StatusCompleted, WorktreePath: "/worktree/v3"},
	}

	// Cancel the context before calling GatherDiffs
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := gatherer.GatherDiffs(ctx, "abc123", variantList)
	if err == nil {
		t.Fatal("expected error from GatherDiffs with cancelled context, got nil")
	}
	if err != context.Canceled {
		t.Errorf("expected context.Canceled error, got: %v", err)
	}
	// With a pre-cancelled context, GatherDiffs should not call GetDiff at all
	if len(mock.DiffCalls) > 0 {
		t.Errorf("expected no GetDiff calls with pre-cancelled context, got %d", len(mock.DiffCalls))
	}
}

// TestGatherDiffs_RespectsExpiredDeadline verifies that GatherDiffs
// returns early when the context deadline has already passed.
func TestGatherDiffs_RespectsExpiredDeadline(t *testing.T) {
	mock := variants.NewMockGit()
	mock.Diff = "diff content"

	gatherer := NewDiffGatherer(mock)

	variantList := []*variants.Variant{
		{ID: 1, Status: variants.StatusCompleted, WorktreePath: "/worktree/v1"},
	}

	// Create a context with an already-expired deadline
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	_, err := gatherer.GatherDiffs(ctx, "abc123", variantList)
	if err == nil {
		t.Fatal("expected error from GatherDiffs with expired deadline, got nil")
	}
}

// TestGatherDiffs_SucceedsWithActiveContext is a sanity check that
// GatherDiffs works normally when given a valid (non-cancelled) context.
func TestGatherDiffs_SucceedsWithActiveContext(t *testing.T) {
	mock := variants.NewMockGit()
	mock.Diff = "diff --git a/file.go b/file.go\n+added line"

	gatherer := NewDiffGatherer(mock)

	variantList := []*variants.Variant{
		{ID: 1, Status: variants.StatusCompleted, WorktreePath: "/worktree/v1"},
		{ID: 2, Status: variants.StatusFailed, WorktreePath: "/worktree/v2"},
		{ID: 3, Status: variants.StatusCompleted, WorktreePath: "/worktree/v3"},
	}

	ctx := context.Background()
	result, err := gatherer.GatherDiffs(ctx, "abc123", variantList)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only completed variants (1, 3) should produce diffs
	if len(result) != 2 {
		t.Fatalf("expected 2 results for completed variants, got %d", len(result))
	}
	if result[0].ID != 1 || result[1].ID != 3 {
		t.Errorf("expected variant IDs [1, 3], got [%d, %d]", result[0].ID, result[1].ID)
	}
}
