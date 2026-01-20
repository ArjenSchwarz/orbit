package comparison

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/arjenschwarz/orbit/internal/variants"
)

// DiffGatherer collects diffs from variants using a GitClient.
type DiffGatherer struct {
	git variants.GitClient
}

// NewDiffGatherer creates a new DiffGatherer with the provided git client.
func NewDiffGatherer(git variants.GitClient) *DiffGatherer {
	return &DiffGatherer{git: git}
}

// GatherDiffs retrieves unified diffs from the base commit for each variant.
// It returns VariantData for each variant, including its diff and metrics.
func (d *DiffGatherer) GatherDiffs(ctx context.Context, baseCommit string, variantList []*variants.Variant) ([]VariantData, error) {
	result := make([]VariantData, 0, len(variantList))

	for _, v := range variantList {
		// Skip non-completed variants
		if v.Status != variants.StatusCompleted {
			continue
		}

		diff, err := d.git.GetDiff(ctx, v.WorktreePath, baseCommit)
		if err != nil {
			return nil, err
		}

		result = append(result, VariantData{
			ID:    v.ID,
			Diff:  diff,
			Agent: v.Agent,
			Metrics: VariantMetrics{
				Cost:     v.Cost,
				Duration: v.Duration,
				NumTurns: v.NumTurns,
			},
		})
	}

	return result, nil
}

// GatherSummaries retrieves summary information for variants when diffs are too large.
// This includes commit messages, diff stats, and changelog content.
func (d *DiffGatherer) GatherSummaries(ctx context.Context, baseCommit string, variantList []*variants.Variant) ([]VariantData, error) {
	result := make([]VariantData, 0, len(variantList))

	for _, v := range variantList {
		// Skip non-completed variants
		if v.Status != variants.StatusCompleted {
			continue
		}

		// Get commit messages
		commits, err := d.git.GetCommitLog(ctx, v.WorktreePath, baseCommit)
		if err != nil {
			return nil, err
		}

		// Get diff stats
		diffStat, err := d.git.GetDiffStat(ctx, v.WorktreePath, baseCommit)
		if err != nil {
			return nil, err
		}

		// Try to read changelog if it exists
		changelog := d.readChangelog(v.WorktreePath)

		result = append(result, VariantData{
			ID:             v.ID,
			Agent:          v.Agent,
			CommitMessages: commits,
			DiffStat:       diffStat,
			Changelog:      changelog,
			Metrics: VariantMetrics{
				Cost:     v.Cost,
				Duration: v.Duration,
				NumTurns: v.NumTurns,
			},
		})
	}

	return result, nil
}

// readChangelog attempts to read a changelog file from the worktree.
// Returns empty string if no changelog is found.
func (d *DiffGatherer) readChangelog(worktreePath string) string {
	// Common changelog filenames
	candidates := []string{
		"CHANGELOG.md",
		"CHANGELOG",
		"changelog.md",
		"HISTORY.md",
		"CHANGES.md",
	}

	for _, name := range candidates {
		path := filepath.Join(worktreePath, name)
		content, err := os.ReadFile(path)
		if err == nil {
			// Return first ~2000 chars to keep context manageable
			s := string(content)
			if len(s) > 2000 {
				// Find a good break point (end of line) near 2000 chars
				idx := strings.LastIndex(s[:2000], "\n")
				if idx > 1500 {
					s = s[:idx] + "\n... (truncated)"
				} else {
					s = s[:2000] + "... (truncated)"
				}
			}
			return s
		}
	}
	return ""
}
