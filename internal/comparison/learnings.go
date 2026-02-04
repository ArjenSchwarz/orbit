package comparison

import "slices"

// GroupLearningsByVariant organizes learnings by their variant ID.
// Returns a map with deterministic iteration order via sorted keys.
func GroupLearningsByVariant(learnings []VariantLearning) map[int][]VariantLearning {
	result := make(map[int][]VariantLearning)
	for _, l := range learnings {
		result[l.VariantID] = append(result[l.VariantID], l)
	}
	return result
}

// SortedVariantIDs returns variant IDs from a learnings map in sorted order.
func SortedVariantIDs(learningsByVariant map[int][]VariantLearning) []int {
	ids := make([]int, 0, len(learningsByVariant))
	for id := range learningsByVariant {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}
