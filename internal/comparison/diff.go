package comparison

import (
	"context"

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
