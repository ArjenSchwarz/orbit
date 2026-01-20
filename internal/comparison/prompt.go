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
  ]
}`

// buildPrompt constructs the comparison prompt for Claude.
func buildPrompt(specName string, variants []VariantData) string {
	var sb strings.Builder

	// Check if any variant has an agent specified
	hasAgents := false
	for _, v := range variants {
		if v.Agent != "" {
			hasAgents = true
			break
		}
	}

	// Header
	sb.WriteString(fmt.Sprintf("You are comparing %d implementation variants of the specification \"%s\".\n\n", len(variants), specName))

	// Variant diffs section
	sb.WriteString("## Variant Diffs\n\n")
	for _, v := range variants {
		if v.Agent != "" {
			sb.WriteString(fmt.Sprintf("### Variant %d (Agent: %s)\n", v.ID, v.Agent))
		} else {
			sb.WriteString(fmt.Sprintf("### Variant %d\n", v.ID))
		}
		sb.WriteString("<diff>\n")
		sb.WriteString(v.Diff)
		if !strings.HasSuffix(v.Diff, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("</diff>\n\n")
	}

	// Metrics table - include Agent column if any variant has an agent
	sb.WriteString("## Metrics\n\n")
	if hasAgents {
		sb.WriteString("| Variant | Agent | Cost | Duration | Turns |\n")
		sb.WriteString("|---------|-------|------|----------|-------|\n")
		for _, v := range variants {
			cost := fmt.Sprintf("$%.4f", v.Metrics.Cost)
			duration := formatDuration(v.Metrics.Duration)
			agent := v.Agent
			if agent == "" {
				agent = "-"
			}
			sb.WriteString(fmt.Sprintf("| %d | %s | %s | %s | %d |\n", v.ID, agent, cost, duration, v.Metrics.NumTurns))
		}
	} else {
		sb.WriteString("| Variant | Cost | Duration | Turns |\n")
		sb.WriteString("|---------|------|----------|-------|\n")
		for _, v := range variants {
			cost := fmt.Sprintf("$%.4f", v.Metrics.Cost)
			duration := formatDuration(v.Metrics.Duration)
			sb.WriteString(fmt.Sprintf("| %d | %s | %s | %d |\n", v.ID, cost, duration, v.Metrics.NumTurns))
		}
	}
	sb.WriteString("\n")

	// Instructions
	sb.WriteString("## Instructions\n\n")
	sb.WriteString("Analyze these implementations and provide:\n")
	sb.WriteString("1. A recommendation (which variant number is best)\n")
	sb.WriteString("2. Confidence level (high/medium/low)\n")
	sb.WriteString("3. Executive summary (2-3 sentences)\n")
	sb.WriteString("4. Per-file analysis noting significant differences\n")
	sb.WriteString("5. Key observations about each approach\n\n")
	sb.WriteString("Output your analysis as JSON matching this schema:\n")
	sb.WriteString("```json\n")
	sb.WriteString(jsonSchema)
	sb.WriteString("\n```\n\n")
	sb.WriteString("IMPORTANT: Output ONLY valid JSON. Do not include any text before or after the JSON.\n")

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

// buildSummaryPrompt constructs a comparison prompt using summaries instead of full diffs.
// This is used when diffs are too large to fit in context.
func buildSummaryPrompt(specName string, variants []VariantData, specContext string) string {
	var sb strings.Builder

	// Check if any variant has an agent specified
	hasAgents := false
	for _, v := range variants {
		if v.Agent != "" {
			hasAgents = true
			break
		}
	}

	// Header
	sb.WriteString(fmt.Sprintf("You are comparing %d implementation variants of the specification \"%s\".\n\n", len(variants), specName))
	sb.WriteString("Note: Full diffs are too large to include. This comparison uses commit messages, change statistics, and changelogs instead.\n\n")

	// Spec context section (if provided)
	if specContext != "" {
		sb.WriteString("## Specification Context\n\n")
		sb.WriteString(specContext)
		sb.WriteString("\n\n")
	}

	// Variant summaries section
	sb.WriteString("## Variant Summaries\n\n")
	for _, v := range variants {
		if v.Agent != "" {
			sb.WriteString(fmt.Sprintf("### Variant %d (Agent: %s)\n\n", v.ID, v.Agent))
		} else {
			sb.WriteString(fmt.Sprintf("### Variant %d\n\n", v.ID))
		}

		// Diff stats
		sb.WriteString("**Change Statistics:**\n```\n")
		sb.WriteString(strings.TrimSpace(v.DiffStat))
		sb.WriteString("\n```\n\n")

		// Commit messages
		sb.WriteString("**Commits:**\n")
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
			sb.WriteString("**Changelog:**\n")
			sb.WriteString("```\n")
			sb.WriteString(v.Changelog)
			if !strings.HasSuffix(v.Changelog, "\n") {
				sb.WriteString("\n")
			}
			sb.WriteString("```\n\n")
		}
	}

	// Metrics table
	sb.WriteString("## Metrics\n\n")
	if hasAgents {
		sb.WriteString("| Variant | Agent | Cost | Duration | Turns |\n")
		sb.WriteString("|---------|-------|------|----------|-------|\n")
		for _, v := range variants {
			cost := fmt.Sprintf("$%.4f", v.Metrics.Cost)
			duration := formatDuration(v.Metrics.Duration)
			agent := v.Agent
			if agent == "" {
				agent = "-"
			}
			sb.WriteString(fmt.Sprintf("| %d | %s | %s | %s | %d |\n", v.ID, agent, cost, duration, v.Metrics.NumTurns))
		}
	} else {
		sb.WriteString("| Variant | Cost | Duration | Turns |\n")
		sb.WriteString("|---------|------|----------|-------|\n")
		for _, v := range variants {
			cost := fmt.Sprintf("$%.4f", v.Metrics.Cost)
			duration := formatDuration(v.Metrics.Duration)
			sb.WriteString(fmt.Sprintf("| %d | %s | %s | %d |\n", v.ID, cost, duration, v.Metrics.NumTurns))
		}
	}
	sb.WriteString("\n")

	// Instructions
	sb.WriteString("## Instructions\n\n")
	sb.WriteString("Based on the commit messages, change statistics, and any changelogs provided, analyze these implementations and provide:\n")
	sb.WriteString("1. A recommendation (which variant number is best)\n")
	sb.WriteString("2. Confidence level (high/medium/low) - note that without full diffs, confidence should typically be 'medium' or 'low'\n")
	sb.WriteString("3. Executive summary (2-3 sentences)\n")
	sb.WriteString("4. Key observations about each implementation approach\n\n")
	sb.WriteString("Consider:\n")
	sb.WriteString("- Commit message quality and clarity\n")
	sb.WriteString("- Scope of changes (files modified, lines changed)\n")
	sb.WriteString("- Implementation approach based on commit history\n")
	sb.WriteString("- Changelog documentation quality (if present)\n\n")
	sb.WriteString("Output your analysis as JSON matching this schema:\n")
	sb.WriteString("```json\n")
	sb.WriteString(jsonSchema)
	sb.WriteString("\n```\n\n")
	sb.WriteString("IMPORTANT: Output ONLY valid JSON. Do not include any text before or after the JSON.\n")

	return sb.String()
}
