package comparison

import (
	"encoding/json"
	"testing"
)

func TestVariantLearning_JSONRoundTrip(t *testing.T) {
	learning := VariantLearning{
		VariantID:      1,
		Category:       LearningCategoryCodePattern,
		Title:          "Table-driven tests",
		Description:    "Uses map[string]struct for test cases",
		Rationale:      "Ensures unique test names and clear failure messages",
		FileReferences: []string{"internal/foo/foo_test.go:42", "internal/bar/bar_test.go"},
	}

	data, err := json.Marshal(learning)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded VariantLearning
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.VariantID != learning.VariantID {
		t.Errorf("VariantID: got %d, want %d", decoded.VariantID, learning.VariantID)
	}
	if decoded.Category != learning.Category {
		t.Errorf("Category: got %q, want %q", decoded.Category, learning.Category)
	}
	if decoded.Title != learning.Title {
		t.Errorf("Title: got %q, want %q", decoded.Title, learning.Title)
	}
	if decoded.Description != learning.Description {
		t.Errorf("Description: got %q, want %q", decoded.Description, learning.Description)
	}
	if decoded.Rationale != learning.Rationale {
		t.Errorf("Rationale: got %q, want %q", decoded.Rationale, learning.Rationale)
	}
	if len(decoded.FileReferences) != len(learning.FileReferences) {
		t.Errorf("FileReferences length: got %d, want %d", len(decoded.FileReferences), len(learning.FileReferences))
	}
	for i, ref := range decoded.FileReferences {
		if ref != learning.FileReferences[i] {
			t.Errorf("FileReferences[%d]: got %q, want %q", i, ref, learning.FileReferences[i])
		}
	}
}

func TestVariantLearning_JSONFieldNames(t *testing.T) {
	learning := VariantLearning{
		VariantID:      2,
		Category:       LearningCategoryArchitecture,
		Title:          "Test title",
		Description:    "Test description",
		Rationale:      "Test rationale",
		FileReferences: []string{"file.go:10"},
	}

	data, err := json.Marshal(learning)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Verify JSON field names match the expected format
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal to map: %v", err)
	}

	expectedFields := []string{"variant_id", "category", "title", "description", "rationale", "file_references"}
	for _, field := range expectedFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("expected field %q not found in JSON output", field)
		}
	}
}

func TestResult_LearningsOmitEmpty(t *testing.T) {
	result := Result{
		Recommendation: 1,
		Confidence:     "high",
		Summary:        "Test summary",
		// Learnings is intentionally nil
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Verify learnings field is omitted when nil
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal to map: %v", err)
	}

	if _, ok := raw["learnings"]; ok {
		t.Error("learnings field should be omitted when nil")
	}
}

func TestResult_LearningsIncludedWhenPresent(t *testing.T) {
	result := Result{
		Recommendation: 1,
		Confidence:     "high",
		Summary:        "Test summary",
		Learnings: []VariantLearning{
			{
				VariantID:      1,
				Category:       LearningCategoryTesting,
				Title:          "Test learning",
				Rationale:      "Test rationale",
				FileReferences: []string{"test.go"},
			},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded Result
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(decoded.Learnings) != 1 {
		t.Errorf("expected 1 learning, got %d", len(decoded.Learnings))
	}
}

func TestLearningCategory_AllValues(t *testing.T) {
	// Verify all category constants have expected string values
	tests := []struct {
		category LearningCategory
		expected string
	}{
		{LearningCategoryCodePattern, "code-pattern"},
		{LearningCategoryArchitecture, "architecture"},
		{LearningCategoryTesting, "testing"},
		{LearningCategoryErrorHandling, "error-handling"},
	}

	for _, tt := range tests {
		t.Run(string(tt.category), func(t *testing.T) {
			if string(tt.category) != tt.expected {
				t.Errorf("got %q, want %q", tt.category, tt.expected)
			}
		})
	}
}
