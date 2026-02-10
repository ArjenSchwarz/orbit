package comparison

import (
	"fmt"
	"strings"
	"time"
)

// jsonSchema is the expected JSON schema for comparison output.
const jsonSchema = `{
  "recommendation": <number: variant ID (1 to N)>,
  "confidence": "<string: 'high', 'medium', or 'low'>",
  "summary": "<string: 2-3 sentence executive summary>",
  "file_analyses": [
    {
      "path": "<string: file path>",
      "variants": {
        "<variant_id>": "<string: assessment of this variant's approach>"
      },
      "preference": <number: preferred variant ID for this file, optional>
    }
  ],
  "observations": [
    "<string: key observation about the implementations>"
  ],
  "documentation_assessment": [
    {
      "variant_id": <number: variant ID>,
      "has_dev_setup": <boolean: has development setup instructions>,
      "has_deployment": <boolean: has deployment instructions>,
      "has_requirements": <boolean: has requirements/dependencies documented>,
      "has_usage_examples": <boolean: has usage examples>,
      "missing_docs": ["<string: missing documentation item>"],
      "notes": "<string: additional observations, optional>"
    }
  ],
  "cross_variant_improvements": [
    {
      "source_variant_id": <number: variant ID that has this improvement>,
      "description": "<string: what the improvement is>",
      "rationale": "<string: why it would improve the chosen variant>",
      "priority": "<string: 'high', 'medium', or 'low'>"
    }
  ],
  "learnings": [
    {
      "variant_id": <number: which variant this learning is from>,
      "category": "<string: 'code-pattern', 'architecture', 'testing', or 'error-handling'>",
      "title": "<string: brief title for the learning (5-10 words)>",
      "description": "<string: what the pattern/technique is>",
      "rationale": "<string: why this matters and how it could be applied elsewhere>",
      "file_references": ["<string: path/to/file.go:123>"]
    }
  ]
}`

// ComparisonInput holds all data needed for comparison.
type ComparisonInput struct {
	SpecName    string
	SpecContext string // Content from requirements.md, design.md, etc.
	Variants    []VariantData
	IncludeDiff bool   // Whether full diffs are included (false if too large)
	OutputPath  string // If set, the agent is instructed to write the JSON to this file path
}

// buildComparisonPrompt constructs a comprehensive comparison prompt.
// Always includes summaries (commits, changelog, stats). Diffs are optional based on size.
func buildComparisonPrompt(input ComparisonInput) string {
	var sb strings.Builder

	// Check if any variant has an agent specified
	hasAgents := false
	for _, v := range input.Variants {
		if v.Agent != "" {
			hasAgents = true
			break
		}
	}

	// Header
	sb.WriteString(fmt.Sprintf("You are comparing %d implementation variants of the specification \"%s\".\n\n", len(input.Variants), input.SpecName))

	if !input.IncludeDiff {
		sb.WriteString("Note: Full diffs are too large to include. Use the commit messages, change statistics, and changelogs to understand each implementation.\n\n")
	}

	// Spec context section (if provided)
	if input.SpecContext != "" {
		sb.WriteString("## Specification Context\n\n")
		sb.WriteString(input.SpecContext)
		sb.WriteString("\n\n")
	}

	// Variant details section
	for _, v := range input.Variants {
		if v.Agent != "" {
			sb.WriteString(fmt.Sprintf("## Variant %d (Agent: %s)\n\n", v.ID, v.Agent))
		} else {
			sb.WriteString(fmt.Sprintf("## Variant %d\n\n", v.ID))
		}

		// Change statistics (always included)
		if v.DiffStat != "" {
			sb.WriteString("### Change Statistics\n```\n")
			sb.WriteString(strings.TrimSpace(v.DiffStat))
			sb.WriteString("\n```\n\n")
		}

		// Commit messages (always included)
		sb.WriteString("### Commits\n")
		if len(v.CommitMessages) > 0 {
			for _, msg := range v.CommitMessages {
				sb.WriteString(fmt.Sprintf("- %s\n", msg))
			}
		} else {
			sb.WriteString("- (no commits)\n")
		}
		sb.WriteString("\n")

		// Changelog if present
		if v.Changelog != "" {
			sb.WriteString("### Changelog\n```\n")
			sb.WriteString(v.Changelog)
			if !strings.HasSuffix(v.Changelog, "\n") {
				sb.WriteString("\n")
			}
			sb.WriteString("```\n\n")
		}

		// Full diff (if included)
		if input.IncludeDiff && v.Diff != "" {
			sb.WriteString("### Full Diff\n<diff>\n")
			sb.WriteString(v.Diff)
			if !strings.HasSuffix(v.Diff, "\n") {
				sb.WriteString("\n")
			}
			sb.WriteString("</diff>\n\n")
		}
	}

	// Metrics table
	sb.WriteString("## Metrics\n\n")
	if hasAgents {
		sb.WriteString("| Variant | Agent | Duration | Turns |\n")
		sb.WriteString("|---------|-------|----------|-------|\n")
		for _, v := range input.Variants {
			duration := formatDuration(v.Metrics.Duration)
			agent := v.Agent
			if agent == "" {
				agent = "-"
			}
			sb.WriteString(fmt.Sprintf("| %d | %s | %s | %d |\n", v.ID, agent, duration, v.Metrics.NumTurns))
		}
	} else {
		sb.WriteString("| Variant | Duration | Turns |\n")
		sb.WriteString("|---------|----------|-------|\n")
		for _, v := range input.Variants {
			duration := formatDuration(v.Metrics.Duration)
			sb.WriteString(fmt.Sprintf("| %d | %s | %d |\n", v.ID, duration, v.Metrics.NumTurns))
		}
	}
	sb.WriteString("\n")

	// Instructions
	sb.WriteString("## Instructions\n\n")
	sb.WriteString("Analyze these implementations and provide a comprehensive comparison.\n\n")

	sb.WriteString("### Required Analysis\n\n")
	sb.WriteString("1. **Recommendation**: Which variant is best overall (variant number)\n")
	sb.WriteString("2. **Confidence**: Your confidence level (high/medium/low)\n")
	sb.WriteString("3. **Summary**: 2-3 sentence executive summary\n")
	sb.WriteString("4. **File Analysis**: Per-file comparison noting significant differences\n")
	sb.WriteString("5. **Observations**: Key observations about each implementation approach\n\n")

	sb.WriteString("### Documentation Assessment\n\n")
	sb.WriteString("For each variant, evaluate whether adequate documentation exists for:\n")
	sb.WriteString("- **Development setup**: How to run the code in a development environment\n")
	sb.WriteString("- **Deployment**: How to deploy to production\n")
	sb.WriteString("- **Requirements**: Dependencies and prerequisites\n")
	sb.WriteString("- **Usage examples**: How to use the implemented feature\n\n")
	sb.WriteString("List any missing documentation that should be added.\n\n")

	sb.WriteString("### Cross-Variant Improvements\n\n")
	sb.WriteString("Identify specific aspects from the NON-chosen variants that would improve the chosen implementation.\n")
	sb.WriteString("For each improvement, specify:\n")
	sb.WriteString("- Which variant it comes from\n")
	sb.WriteString("- What the improvement is\n")
	sb.WriteString("- Why it would benefit the chosen variant\n")
	sb.WriteString("- Priority (high/medium/low)\n\n")

	// Learnings section [Req 2.1-2.5]
	sb.WriteString("### Developer Learnings\n\n")
	sb.WriteString("Identify educational insights from EACH variant that could help the user become a better developer.\n")
	sb.WriteString("Focus on techniques that are transferable to other projects.\n\n")

	sb.WriteString("**Categories:**\n")
	sb.WriteString("- `code-pattern`: Idiomatic code, clever algorithms, elegant solutions\n")
	sb.WriteString("- `architecture`: Structural decisions, module organization, separation of concerns\n")
	sb.WriteString("- `testing`: Test approaches, coverage patterns, mocking techniques\n")
	sb.WriteString("- `error-handling`: Defensive coding, edge case handling, resilience patterns\n\n")

	sb.WriteString("**For each learning:**\n")
	sb.WriteString("- Include specific file references (path/to/file.go:123)\n")
	sb.WriteString("- Explain WHY this pattern matters (the broader principle)\n")
	sb.WriteString("- Focus on techniques the user could apply in future projects\n\n")

	// Quality guidelines [Req 5.1-5.4]
	sb.WriteString("**Exclude trivial observations like:**\n")
	sb.WriteString("- \"Uses comments\" or \"Has tests\"\n")
	sb.WriteString("- Generic observations that apply to any codebase\n")
	sb.WriteString("- Implementation details without educational value\n\n")

	sb.WriteString("**Good learning examples:**\n")
	sb.WriteString("- \"Uses table-driven tests with map[string]struct for unique test case names\"\n")
	sb.WriteString("- \"Implements the functional options pattern for flexible configuration\"\n")
	sb.WriteString("- \"Uses sentinel errors with errors.Is() for type-safe error checking\"\n\n")

	sb.WriteString("**Bad learning examples (too trivial):**\n")
	sb.WriteString("- \"Code is well-formatted\"\n")
	sb.WriteString("- \"Functions have descriptive names\"\n")
	sb.WriteString("- \"Uses if statements for control flow\"\n\n")

	sb.WriteString("**Limits:** Provide the most important learnings only. Aim for 3-5 per variant, maximum 5.\n\n")

	sb.WriteString("### Evaluation Criteria\n\n")
	sb.WriteString("Consider:\n")
	sb.WriteString("- Code quality and maintainability\n")
	sb.WriteString("- Commit message quality and clarity\n")
	sb.WriteString("- Scope of changes (appropriate for the task?)\n")
	sb.WriteString("- Documentation completeness\n")
	sb.WriteString("- Error handling and edge cases\n")
	sb.WriteString("- Testing coverage (if visible)\n\n")

	sb.WriteString("Output your analysis as JSON matching this schema:\n")
	sb.WriteString("```json\n")
	sb.WriteString(jsonSchema)
	sb.WriteString("\n```\n\n")
	sb.WriteString("IMPORTANT: Output ONLY valid JSON. Do not include any text before or after the JSON.\n")

	if input.OutputPath != "" {
		sb.WriteString(fmt.Sprintf("\nADDITIONALLY: Write the JSON result to the file `%s`. "+
			"This is critical — write the file BEFORE outputting the JSON to stdout. "+
			"The file must contain only the valid JSON object, nothing else.\n", input.OutputPath))
	}

	return sb.String()
}

// formatDuration formats a duration for display in the metrics table.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm %ds", minutes, seconds)
}

// estimatePromptTokens provides a rough estimate of the number of tokens in the prompt.
// This is a simple heuristic: ~4 characters per token for English text.
func estimatePromptTokens(prompt string) int {
	return len(prompt) / 4
}

// Legacy functions for backwards compatibility - these now delegate to the new unified prompt builder

// buildPrompt constructs the comparison prompt for Claude (legacy, uses diffs only).
func buildPrompt(specName string, variants []VariantData) string {
	return buildComparisonPrompt(ComparisonInput{
		SpecName:    specName,
		Variants:    variants,
		IncludeDiff: true,
	})
}

// buildSummaryPrompt constructs a comparison prompt using summaries instead of full diffs (legacy).
func buildSummaryPrompt(specName string, variants []VariantData, specContext string) string {
	return buildComparisonPrompt(ComparisonInput{
		SpecName:    specName,
		SpecContext: specContext,
		Variants:    variants,
		IncludeDiff: false,
	})
}
