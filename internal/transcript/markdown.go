package transcript

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"unicode/utf8"
)

const (
	// MaxToolInputRunes is the maximum number of runes for tool input truncation.
	MaxToolInputRunes = 2000
	// MaxToolResultRunes is the maximum number of runes for tool result truncation.
	MaxToolResultRunes = 3000
	// CollapseThresholdRunes is the threshold for collapsing non-Task/Skill tools.
	CollapseThresholdRunes = 500
)

// toolMetadata stores information about a tool_use for result matching.
type toolMetadata struct {
	Name    string // Tool name (e.g., "Task", "Read")
	Summary string // Summary text for collapsed display
}

// RenderMarkdown converts parsed entries to Markdown format.
func RenderMarkdown(entries []Entry, opts RenderOptions) string {
	var sb strings.Builder

	// Write header
	title := opts.Title
	if title == "" {
		title = "Session Transcript"
	}
	sb.WriteString(fmt.Sprintf("# %s\n\n", title))

	if opts.SessionID != "" {
		sb.WriteString(fmt.Sprintf("**Session ID:** `%s`\n\n", opts.SessionID))
	}

	sb.WriteString("---\n\n")

	// Initialize tool metadata map at render level (shared across entries)
	// This is critical because tool_use appears in assistant entries but
	// tool_result appears in user entries - the map must persist across both.
	toolMeta := make(map[string]toolMetadata)

	// Render each entry
	for _, entry := range entries {
		switch entry.Type {
		case "user":
			sb.WriteString(formatUserMessage(&entry, toolMeta))
		case "assistant":
			sb.WriteString(formatAssistantMessage(&entry, toolMeta))
		}
		// Unknown entry types are skipped silently per requirement 4.7
	}

	return sb.String()
}

// formatUserMessage formats a user message as Markdown.
func formatUserMessage(entry *Entry, toolMeta map[string]toolMetadata) string {
	if entry.Message == nil || len(entry.Message.Content) == 0 {
		return ""
	}

	// Check if there's any content to render (text or tool_result)
	hasContent := false
	for _, item := range entry.Message.Content {
		if item.Text != "" || item.Type == "tool_result" {
			hasContent = true
			break
		}
	}

	if !hasContent {
		return ""
	}

	var sb strings.Builder
	hasText := false

	// Check if there's text content to determine header
	for _, item := range entry.Message.Content {
		if item.Text != "" {
			hasText = true
			break
		}
	}

	if hasText {
		sb.WriteString("## 👤 User\n\n")
	}

	for _, item := range entry.Message.Content {
		switch item.Type {
		case "text":
			if item.Text != "" {
				sb.WriteString(item.Text)
				sb.WriteString("\n\n")
			}
		case "tool_result":
			sb.WriteString(formatToolResult(&item, toolMeta))
		}
	}

	if hasText {
		sb.WriteString("---\n\n")
	}

	return sb.String()
}

// formatAssistantMessage formats an assistant message as Markdown.
func formatAssistantMessage(entry *Entry, toolMeta map[string]toolMetadata) string {
	if entry.Message == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## 🤖 Assistant\n\n")

	for _, item := range entry.Message.Content {
		switch item.Type {
		case "thinking":
			if item.Thinking != "" {
				sb.WriteString("<details>\n<summary>💭 Thinking</summary>\n\n")
				sb.WriteString(item.Thinking)
				sb.WriteString("\n\n</details>\n\n")
			}

		case "text":
			if item.Text != "" {
				sb.WriteString(item.Text)
				sb.WriteString("\n\n")
			}

		case "tool_use":
			sb.WriteString(formatToolUse(&item, toolMeta))

		case "tool_result":
			// tool_result in assistant entries (legacy handling)
			sb.WriteString(formatToolResult(&item, toolMeta))

		// Unknown content types are skipped silently per requirement 4.8
		}
	}

	sb.WriteString("---\n\n")
	return sb.String()
}

// formatToolUse formats a tool_use content item, potentially wrapping it in <details>.
func formatToolUse(item *ContentItem, toolMeta map[string]toolMetadata) string {
	var sb strings.Builder

	// Serialize input with json.Marshal (compact) for threshold measurement
	var compactJSON []byte
	var inputJSON []byte
	var runeLen int

	if item.Input != nil {
		var err error
		compactJSON, err = json.Marshal(item.Input)
		if err == nil {
			runeLen = utf8.RuneCountInString(string(compactJSON))
		}
		// Use indented JSON for display
		inputJSON, _ = json.MarshalIndent(item.Input, "", "  ")
	}

	// Determine if we should collapse
	collapse := shouldCollapse(item.Name, runeLen)

	// Get summary text
	summary := getToolSummary(item.Name, item.Input)
	if summary == "" {
		// Fallback summaries
		switch item.Name {
		case "Task":
			summary = "Task"
		case "Skill":
			summary = "Skill"
		default:
			summary = "Tool: " + item.Name
		}
	}

	// Store metadata for result matching (if ID is present)
	if item.ID != "" {
		toolMeta[item.ID] = toolMetadata{
			Name:    item.Name,
			Summary: summary,
		}
	}

	if collapse {
		// Collapsed format with <details>
		sb.WriteString("<details>\n")
		sb.WriteString(fmt.Sprintf("<summary>🔧 %s</summary>\n\n", escapeSummary(summary)))
		if len(inputJSON) > 0 {
			sb.WriteString("```json\n")
			inputStr := truncateString(string(inputJSON), MaxToolInputRunes)
			sb.WriteString(inputStr)
			sb.WriteString("\n```\n\n")
		}
		sb.WriteString("</details>\n\n")
	} else {
		// Uncollapsed format with heading
		sb.WriteString(fmt.Sprintf("### 🔧 Tool: `%s`\n\n", item.Name))
		if len(inputJSON) > 0 {
			sb.WriteString("```json\n")
			inputStr := truncateString(string(inputJSON), MaxToolInputRunes)
			sb.WriteString(inputStr)
			sb.WriteString("\n```\n\n")
		}
	}

	return sb.String()
}

// formatToolResult formats a tool_result content item, potentially wrapping it in <details>.
func formatToolResult(item *ContentItem, toolMeta map[string]toolMetadata) string {
	var sb strings.Builder

	content := item.Content
	runeLen := utf8.RuneCountInString(content)

	// Determine icon
	icon := "✅"
	if item.IsError {
		icon = "❌"
	}

	// Look up tool metadata for summary inheritance
	var summary string
	var collapse bool

	if meta, found := toolMeta[item.ToolUseID]; found {
		// Inherit collapse behavior from tool_use
		summary = meta.Summary
		collapse = shouldCollapse(meta.Name, runeLen)
		// For Task/Skill, always collapse
		if meta.Name == "Task" || meta.Name == "Skill" {
			collapse = true
		}
	} else {
		// Unmatched result: apply threshold-based collapsing
		collapse = runeLen > CollapseThresholdRunes
		if item.IsError {
			summary = "Tool Error"
		} else {
			summary = "Tool Result"
		}
	}

	// Zero-length content should not collapse
	if runeLen == 0 {
		collapse = false
	}

	truncatedContent := truncateString(content, MaxToolResultRunes)

	if collapse {
		// Collapsed format with <details>
		sb.WriteString("<details>\n")
		sb.WriteString(fmt.Sprintf("<summary>%s %s</summary>\n\n", icon, escapeSummary(summary)))
		sb.WriteString("```\n")
		sb.WriteString(truncatedContent)
		sb.WriteString("\n```\n\n")
		sb.WriteString("</details>\n\n")
	} else {
		// Uncollapsed format with heading
		if item.IsError {
			sb.WriteString("#### ❌ Tool Error\n\n")
		} else {
			sb.WriteString("#### ✅ Tool Result\n\n")
		}
		sb.WriteString("```\n")
		sb.WriteString(truncatedContent)
		sb.WriteString("\n```\n\n")
	}

	return sb.String()
}

// truncateString truncates a string to maxRunes, preserving UTF-8 boundaries.
// This is an improvement over byte-based truncation which could produce invalid UTF-8.
func truncateString(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}

	var b strings.Builder
	n := 0
	for _, r := range s {
		if n >= maxRunes {
			break
		}
		b.WriteRune(r)
		n++
	}
	return b.String() + "\n... (truncated)"
}

// getToolSummary extracts a readable summary for Task and Skill tools.
// Returns empty string for other tools or if extraction fails.
// Uses defensive type assertions with comma-ok idiom.
func getToolSummary(name string, input any) string {
	switch name {
	case "Task":
		inputMap, ok := input.(map[string]any)
		if !ok {
			return ""
		}
		subType, _ := inputMap["subagent_type"].(string)
		desc, _ := inputMap["description"].(string)
		// Handle partial fields gracefully (no trailing colon)
		if subType != "" && desc != "" {
			return subType + ": " + desc
		}
		if subType != "" {
			return subType
		}
		return ""
	case "Skill":
		inputMap, ok := input.(map[string]any)
		if !ok {
			return ""
		}
		skill, _ := inputMap["skill"].(string)
		if skill != "" {
			return "Skill: " + skill
		}
		return ""
	}
	return ""
}

// shouldCollapse determines if a tool_use or tool_result should be wrapped
// in a <details> element based on tool name and content rune count.
func shouldCollapse(name string, runeCount int) bool {
	if name == "Task" || name == "Skill" {
		return true
	}
	return runeCount > CollapseThresholdRunes
}

// escapeSummary escapes summary text for safe inclusion in HTML/Markdown.
// Applies html.EscapeString to prevent XSS and structural corruption.
func escapeSummary(s string) string {
	return html.EscapeString(s)
}
