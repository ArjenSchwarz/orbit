package transcript

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	// MaxToolInputRunes is the maximum number of runes for tool input truncation.
	MaxToolInputRunes = 2000
	// MaxToolResultRunes is the maximum number of runes for tool result truncation.
	MaxToolResultRunes = 3000
)

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

	// Render each entry
	for _, entry := range entries {
		switch entry.Type {
		case "user":
			sb.WriteString(formatUserMessage(&entry))
		case "assistant":
			sb.WriteString(formatAssistantMessage(&entry))
		}
		// Unknown entry types are skipped silently per requirement 4.7
	}

	return sb.String()
}

// formatUserMessage formats a user message as Markdown.
func formatUserMessage(entry *Entry) string {
	if entry.Message == nil || len(entry.Message.Content) == 0 {
		return ""
	}

	// Collect text content first to check if there's anything to output
	var texts []string
	for _, item := range entry.Message.Content {
		if item.Text != "" {
			texts = append(texts, item.Text)
		}
	}

	// Skip if no actual text content
	if len(texts) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## 👤 User\n\n")

	for _, text := range texts {
		sb.WriteString(text)
		sb.WriteString("\n\n")
	}

	sb.WriteString("---\n\n")
	return sb.String()
}

// formatAssistantMessage formats an assistant message as Markdown.
func formatAssistantMessage(entry *Entry) string {
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
			sb.WriteString(fmt.Sprintf("### 🔧 Tool: `%s`\n\n", item.Name))
			if item.Input != nil {
				inputJSON, err := json.MarshalIndent(item.Input, "", "  ")
				if err == nil {
					sb.WriteString("```json\n")
					inputStr := truncateString(string(inputJSON), MaxToolInputRunes)
					sb.WriteString(inputStr)
					sb.WriteString("\n```\n\n")
				}
			}

		case "tool_result":
			content := truncateString(item.Content, MaxToolResultRunes)
			if item.IsError {
				sb.WriteString("#### ❌ Tool Error\n\n")
			} else {
				sb.WriteString("#### ✅ Tool Result\n\n")
			}
			sb.WriteString("```\n")
			sb.WriteString(content)
			sb.WriteString("\n```\n\n")

		// Unknown content types are skipped silently per requirement 4.8
		}
	}

	sb.WriteString("---\n\n")
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
