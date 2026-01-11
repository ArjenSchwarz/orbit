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

	// Header
	sb.WriteString(fmt.Sprintf("You are comparing %d implementation variants of the specification \"%s\".\n\n", len(variants), specName))

	// Variant diffs section
	sb.WriteString("## Variant Diffs\n\n")
	for _, v := range variants {
		sb.WriteString(fmt.Sprintf("### Variant %d\n", v.ID))
		sb.WriteString("<diff>\n")
		sb.WriteString(v.Diff)
		if !strings.HasSuffix(v.Diff, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("</diff>\n\n")
	}

	// Metrics table
	sb.WriteString("## Metrics\n\n")
	sb.WriteString("| Variant | Cost | Duration | Turns |\n")
	sb.WriteString("|---------|------|----------|-------|\n")
	for _, v := range variants {
		cost := fmt.Sprintf("$%.4f", v.Metrics.Cost)
		duration := formatDuration(v.Metrics.Duration)
		sb.WriteString(fmt.Sprintf("| %d | %s | %s | %d |\n", v.ID, cost, duration, v.Metrics.NumTurns))
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
