package comparison

import (
	"testing"
)

func TestGroupLearningsByVariant(t *testing.T) {
	tests := map[string]struct {
		input      []VariantLearning
		wantGroups int
		wantCounts map[int]int
	}{
		"empty input": {
			input:      []VariantLearning{},
			wantGroups: 0,
			wantCounts: map[int]int{},
		},
		"nil input": {
			input:      nil,
			wantGroups: 0,
			wantCounts: map[int]int{},
		},
		"single variant": {
			input: []VariantLearning{
				{VariantID: 1, Title: "Learning 1"},
				{VariantID: 1, Title: "Learning 2"},
			},
			wantGroups: 1,
			wantCounts: map[int]int{1: 2},
		},
		"multiple variants": {
			input: []VariantLearning{
				{VariantID: 1, Title: "V1 Learning 1"},
				{VariantID: 2, Title: "V2 Learning 1"},
				{VariantID: 1, Title: "V1 Learning 2"},
				{VariantID: 3, Title: "V3 Learning 1"},
				{VariantID: 2, Title: "V2 Learning 2"},
			},
			wantGroups: 3,
			wantCounts: map[int]int{1: 2, 2: 2, 3: 1},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := GroupLearningsByVariant(tc.input)
			if len(got) != tc.wantGroups {
				t.Errorf("expected %d groups, got %d", tc.wantGroups, len(got))
			}
			for variantID, wantCount := range tc.wantCounts {
				if gotCount := len(got[variantID]); gotCount != wantCount {
					t.Errorf("variant %d: expected %d learnings, got %d", variantID, wantCount, gotCount)
				}
			}
		})
	}
}

func TestGroupLearningsByVariant_PreservesOrder(t *testing.T) {
	input := []VariantLearning{
		{VariantID: 1, Title: "First"},
		{VariantID: 1, Title: "Second"},
		{VariantID: 1, Title: "Third"},
	}

	got := GroupLearningsByVariant(input)
	learnings := got[1]

	if len(learnings) != 3 {
		t.Fatalf("expected 3 learnings, got %d", len(learnings))
	}
	if learnings[0].Title != "First" {
		t.Errorf("expected first learning 'First', got %q", learnings[0].Title)
	}
	if learnings[1].Title != "Second" {
		t.Errorf("expected second learning 'Second', got %q", learnings[1].Title)
	}
	if learnings[2].Title != "Third" {
		t.Errorf("expected third learning 'Third', got %q", learnings[2].Title)
	}
}

func TestSortedVariantIDs(t *testing.T) {
	tests := map[string]struct {
		input   map[int][]VariantLearning
		wantIDs []int
	}{
		"empty map": {
			input:   map[int][]VariantLearning{},
			wantIDs: []int{},
		},
		"single variant": {
			input: map[int][]VariantLearning{
				3: {{Title: "test"}},
			},
			wantIDs: []int{3},
		},
		"multiple variants unsorted": {
			input: map[int][]VariantLearning{
				3: {{Title: "v3"}},
				1: {{Title: "v1"}},
				2: {{Title: "v2"}},
			},
			wantIDs: []int{1, 2, 3},
		},
		"already sorted": {
			input: map[int][]VariantLearning{
				1: {{Title: "v1"}},
				2: {{Title: "v2"}},
			},
			wantIDs: []int{1, 2},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := SortedVariantIDs(tc.input)
			if len(got) != len(tc.wantIDs) {
				t.Fatalf("expected %d IDs, got %d", len(tc.wantIDs), len(got))
			}
			for i, wantID := range tc.wantIDs {
				if got[i] != wantID {
					t.Errorf("position %d: expected ID %d, got %d", i, wantID, got[i])
				}
			}
		})
	}
}

func TestSortedVariantIDs_NilMap(t *testing.T) {
	got := SortedVariantIDs(nil)
	if len(got) != 0 {
		t.Errorf("expected empty slice for nil map, got %v", got)
	}
}
