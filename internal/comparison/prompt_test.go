package comparison

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildComparisonPrompt_OutputPathInstruction(t *testing.T) {
	t.Run("includes instruction when OutputPath is set", func(t *testing.T) {
		input := ComparisonInput{
			SpecName:   "test-spec",
			Variants:   []VariantData{{ID: 1}, {ID: 2}},
			OutputPath: "/tmp/specs/my-feature/.orbit/comparison.json",
		}

		prompt := buildComparisonPrompt(input)

		assert.Contains(t, prompt, "ADDITIONALLY", "prompt should contain ADDITIONALLY instruction when OutputPath is set")
		assert.Contains(t, prompt, "/tmp/specs/my-feature/.orbit/comparison.json", "prompt should contain the exact OutputPath")
		assert.Contains(t, prompt, "Write the JSON result to the file", "prompt should instruct agent to write JSON to file")
	})

	t.Run("no instruction when OutputPath is empty", func(t *testing.T) {
		input := ComparisonInput{
			SpecName: "test-spec",
			Variants: []VariantData{{ID: 1}, {ID: 2}},
		}

		prompt := buildComparisonPrompt(input)

		assert.NotContains(t, prompt, "ADDITIONALLY", "prompt should not contain ADDITIONALLY instruction when OutputPath is empty")
	})
}

func TestBuildComparisonPrompt_IncludesLearningsInstructions(t *testing.T) {
	input := ComparisonInput{
		SpecName: "test-spec",
		Variants: []VariantData{{ID: 1}},
	}

	prompt := buildComparisonPrompt(input)

	// Check for learnings section header
	assert.Contains(t, prompt, "### Developer Learnings", "Prompt should contain '### Developer Learnings' section")

	// Check for categories
	requiredCategories := []string{
		"code-pattern",
		"architecture",
		"testing",
		"error-handling",
	}
	for _, cat := range requiredCategories {
		assert.Contains(t, prompt, cat, "Prompt should mention category '%s'", cat)
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
		assert.Contains(t, prompt, guideline, "Prompt should contain guideline: '%s'", guideline)
	}

	// Check for JSON schema update
	assert.Contains(t, prompt, "\"learnings\": [", "JSON schema should include 'learnings' array")
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
		assert.Contains(t, prompt, ex, "Prompt should include good example: '%s'", ex)
	}

	// Check for bad examples to exclude
	badExamples := []string{
		"well-formatted",
		"descriptive names",
	}
	for _, ex := range badExamples {
		assert.Contains(t, prompt, ex, "Prompt should include bad example to exclude: '%s'", ex)
	}
}

func TestBuildComparisonPrompt_IncludesLearningsLimit(t *testing.T) {
	input := ComparisonInput{
		SpecName: "test-spec",
		Variants: []VariantData{{ID: 1}},
	}

	prompt := buildComparisonPrompt(input)

	// Should mention limits for learnings
	assert.Contains(t, prompt, "3-5 per variant", "Prompt should mention learnings limit of 3-5 per variant")
	assert.Contains(t, prompt, "maximum 5", "Prompt should mention maximum 5 learnings")
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
		assert.Contains(t, prompt, field, "JSON schema should include field: %s", field)
	}
}
