package consolidation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPromptBuilder_Build(t *testing.T) {
	t.Run("includes all required sections", func(t *testing.T) {
		worktrees := map[int]string{
			1: "/path/to/variant-1",
			2: "/path/to/variant-2",
			3: "/path/to/variant-3",
		}
		pb := NewPromptBuilder("my-feature", 1, "/path/to/report.md", worktrees, "")

		prompt := pb.Build()

		// Check header
		assert.Contains(t, prompt, "variant 1")
		assert.Contains(t, prompt, `"my-feature"`)

		// Check context section
		assert.Contains(t, prompt, "## Context")
		assert.Contains(t, prompt, "Comparison report: /path/to/report.md")
		assert.Contains(t, prompt, "Chosen variant worktree: /path/to/variant-1")
		assert.Contains(t, prompt, "Other variant worktrees:")
		assert.Contains(t, prompt, "V2: /path/to/variant-2")
		assert.Contains(t, prompt, "V3: /path/to/variant-3")

		// Check instructions section
		assert.Contains(t, prompt, "## Instructions")
		assert.Contains(t, prompt, "Cross-Variant Improvements")

		// Check commit message format
		assert.Contains(t, prompt, "feat(consolidate): Apply improvements from variants X, Y to variant 1 for my-feature")

		// Check conflict resolution policy
		assert.Contains(t, prompt, "## Conflict Resolution Policy")
		assert.Contains(t, prompt, "Prioritize the chosen variant's existing patterns")

		// Check scope constraints
		assert.Contains(t, prompt, "## Scope Constraints")
		assert.Contains(t, prompt, "DO NOT")
		assert.Contains(t, prompt, "binary files")

		// Check edge case handling
		assert.Contains(t, prompt, "## Edge Case Handling")
		assert.Contains(t, prompt, "renamed/moved")
		assert.Contains(t, prompt, "already present")

		// Check report format
		assert.Contains(t, prompt, "## Report Format")
		assert.Contains(t, prompt, "### Applied")
		assert.Contains(t, prompt, "### Skipped")
		assert.Contains(t, prompt, "### Commit")
	})

	t.Run("includes custom prompt when provided", func(t *testing.T) {
		worktrees := map[int]string{
			1: "/path/to/variant-1",
		}
		customPrompt := "Focus on error handling improvements. Ignore performance optimizations."
		pb := NewPromptBuilder("my-feature", 1, "/path/to/report.md", worktrees, customPrompt)

		prompt := pb.Build()

		assert.Contains(t, prompt, "## Custom Instructions")
		assert.Contains(t, prompt, "Focus on error handling improvements")
		assert.Contains(t, prompt, "Ignore performance optimizations")
	})

	t.Run("omits custom instructions section when not provided", func(t *testing.T) {
		worktrees := map[int]string{
			1: "/path/to/variant-1",
		}
		pb := NewPromptBuilder("my-feature", 1, "/path/to/report.md", worktrees, "")

		prompt := pb.Build()

		assert.NotContains(t, prompt, "## Custom Instructions")
	})

	t.Run("handles special characters in spec name", func(t *testing.T) {
		worktrees := map[int]string{
			1: "/path/to/variant-1",
		}
		pb := NewPromptBuilder("feature-with-dashes_and_underscores", 1, "/path/to/report.md", worktrees, "")

		prompt := pb.Build()

		assert.Contains(t, prompt, `"feature-with-dashes_and_underscores"`)
	})

	t.Run("handles single variant (no other worktrees)", func(t *testing.T) {
		worktrees := map[int]string{
			1: "/path/to/variant-1",
		}
		pb := NewPromptBuilder("my-feature", 1, "/path/to/report.md", worktrees, "")

		prompt := pb.Build()

		assert.Contains(t, prompt, "Chosen variant worktree: /path/to/variant-1")
		// Should not have "Other variant worktrees" with content
		lines := strings.Split(prompt, "\n")
		for _, line := range lines {
			if strings.Contains(line, "Other variant worktrees:") {
				// The line should just have empty content after the colon
				assert.Equal(t, "- Other variant worktrees: ", line)
				break
			}
		}
	})

	t.Run("sorts other variant worktrees by ID", func(t *testing.T) {
		worktrees := map[int]string{
			1: "/path/to/variant-1",
			3: "/path/to/variant-3",
			2: "/path/to/variant-2",
			5: "/path/to/variant-5",
		}
		pb := NewPromptBuilder("my-feature", 1, "/path/to/report.md", worktrees, "")

		prompt := pb.Build()

		// Find the line with other worktrees
		idx2 := strings.Index(prompt, "V2:")
		idx3 := strings.Index(prompt, "V3:")
		idx5 := strings.Index(prompt, "V5:")

		// Verify they appear in order
		assert.True(t, idx2 < idx3, "V2 should appear before V3")
		assert.True(t, idx3 < idx5, "V3 should appear before V5")
	})

	t.Run("handles paths with spaces", func(t *testing.T) {
		worktrees := map[int]string{
			1: "/path/with spaces/variant-1",
			2: "/another path/variant-2",
		}
		pb := NewPromptBuilder("my feature", 1, "/path/to/report file.md", worktrees, "")

		prompt := pb.Build()

		assert.Contains(t, prompt, "/path/with spaces/variant-1")
		assert.Contains(t, prompt, "/another path/variant-2")
		assert.Contains(t, prompt, "/path/to/report file.md")
	})
}

func TestPromptBuilder_getOtherWorktrees(t *testing.T) {
	t.Run("excludes chosen variant", func(t *testing.T) {
		worktrees := map[int]string{
			1: "/path/to/variant-1",
			2: "/path/to/variant-2",
			3: "/path/to/variant-3",
		}
		pb := NewPromptBuilder("my-feature", 2, "/path/to/report.md", worktrees, "")

		others := pb.getOtherWorktrees()

		assert.Len(t, others, 2)
		assert.Contains(t, others[0], "V1:")
		assert.Contains(t, others[1], "V3:")
		// Should not contain V2 (the chosen variant)
		for _, other := range others {
			assert.NotContains(t, other, "V2:")
		}
	})

	t.Run("returns empty slice when no other variants", func(t *testing.T) {
		worktrees := map[int]string{
			1: "/path/to/variant-1",
		}
		pb := NewPromptBuilder("my-feature", 1, "/path/to/report.md", worktrees, "")

		others := pb.getOtherWorktrees()

		assert.Empty(t, others)
	})
}
