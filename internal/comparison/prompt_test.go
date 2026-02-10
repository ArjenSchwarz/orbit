package comparison

import (
	"strings"
	"testing"
)

func TestBuildComparisonPrompt_OutputPathInstruction(t *testing.T) {
	t.Run("includes instruction when OutputPath is set", func(t *testing.T) {
		input := ComparisonInput{
			SpecName:   "test-spec",
			Variants:   []VariantData{{ID: 1}, {ID: 2}},
			OutputPath: "/tmp/specs/my-feature/.orbit/comparison.json",
		}

		prompt := buildComparisonPrompt(input)

		if !strings.Contains(prompt, "ADDITIONALLY") {
			t.Error("prompt should contain ADDITIONALLY instruction when OutputPath is set")
		}
		if !strings.Contains(prompt, "/tmp/specs/my-feature/.orbit/comparison.json") {
			t.Error("prompt should contain the exact OutputPath")
		}
		if !strings.Contains(prompt, "Write the JSON result to the file") {
			t.Error("prompt should instruct agent to write JSON to file")
		}
	})

	t.Run("no instruction when OutputPath is empty", func(t *testing.T) {
		input := ComparisonInput{
			SpecName: "test-spec",
			Variants: []VariantData{{ID: 1}, {ID: 2}},
		}

		prompt := buildComparisonPrompt(input)

		if strings.Contains(prompt, "ADDITIONALLY") {
			t.Error("prompt should not contain ADDITIONALLY instruction when OutputPath is empty")
		}
	})
}

func TestBuildComparisonPrompt_IncludesLearningsInstructions(t *testing.T) {
	input := ComparisonInput{
		SpecName: "test-spec",
		Variants: []VariantData{{ID: 1}},
	}

	prompt := buildComparisonPrompt(input)

	// Check for learnings section header
	if !strings.Contains(prompt, "### Developer Learnings") {
		t.Error("Prompt should contain '### Developer Learnings' section")
	}

	// Check for categories
	requiredCategories := []string{
		"code-pattern",
		"architecture",
		"testing",
		"error-handling",
	}
	for _, cat := range requiredCategories {
		if !strings.Contains(prompt, cat) {
			t.Errorf("Prompt should mention category '%s'", cat)
		}
	}

	// Check for quality guidelines
	guidelines := []string{
		"techniques that are transferable",
		"Exclude trivial observations",
		"Uses comments",
		"Has tests",
		"specific file references",
	}
	for _, guideline := range guidelines {
		if !strings.Contains(prompt, guideline) {
			t.Errorf("Prompt should contain guideline: '%s'", guideline)
		}
	}

	// Check for JSON schema update
	if !strings.Contains(prompt, "\"learnings\": [") {
		t.Error("JSON schema should include 'learnings' array")
	}
}

func TestBuildComparisonPrompt_IncludesLearningExamples(t *testing.T) {
	input := ComparisonInput{
		SpecName: "test-spec",
		Variants: []VariantData{{ID: 1}},
	}

	prompt := buildComparisonPrompt(input)

	// Check for good examples
	goodExamples := []string{
		"table-driven tests",
		"functional options pattern",
		"sentinel errors",
	}
	for _, ex := range goodExamples {
		if !strings.Contains(prompt, ex) {
			t.Errorf("Prompt should include good example: '%s'", ex)
		}
	}

	// Check for bad examples to exclude
	badExamples := []string{
		"well-formatted",
		"descriptive names",
	}
	for _, ex := range badExamples {
		if !strings.Contains(prompt, ex) {
			t.Errorf("Prompt should include bad example to exclude: '%s'", ex)
		}
	}
}

func TestBuildComparisonPrompt_IncludesLearningsLimit(t *testing.T) {
	input := ComparisonInput{
		SpecName: "test-spec",
		Variants: []VariantData{{ID: 1}},
	}

	prompt := buildComparisonPrompt(input)

	// Should mention limits for learnings
	if !strings.Contains(prompt, "3-5 per variant") {
		t.Error("Prompt should mention learnings limit of 3-5 per variant")
	}
	if !strings.Contains(prompt, "maximum 5") {
		t.Error("Prompt should mention maximum 5 learnings")
	}
}

func TestBuildComparisonPrompt_JSONSchemaIncludesLearningFields(t *testing.T) {
	input := ComparisonInput{
		SpecName: "test-spec",
		Variants: []VariantData{{ID: 1}},
	}

	prompt := buildComparisonPrompt(input)

	// Verify JSON schema has all learning fields
	requiredFields := []string{
		"\"variant_id\"",
		"\"category\"",
		"\"title\"",
		"\"description\"",
		"\"rationale\"",
		"\"file_references\"",
	}
	for _, field := range requiredFields {
		if !strings.Contains(prompt, field) {
			t.Errorf("JSON schema should include field: %s", field)
		}
	}
}
