package transcript

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"unicode/utf8"
)

// htmlCSS contains the embedded stylesheet for HTML transcripts.
const htmlCSS = `
:root {
    --bg-primary: #ffffff;
    --bg-secondary: #f8f9fa;
    --bg-code: #f4f4f4;
    --text-primary: #212529;
    --text-secondary: #6c757d;
    --border-color: #dee2e6;
    --user-accent: #0d6efd;
    --assistant-accent: #6f42c1;
    --success-color: #198754;
    --error-color: #dc3545;
    --tool-accent: #fd7e14;
}

@media (prefers-color-scheme: dark) {
    :root {
        --bg-primary: #1a1a1a;
        --bg-secondary: #2d2d2d;
        --bg-code: #2d2d2d;
        --text-primary: #e9ecef;
        --text-secondary: #adb5bd;
        --border-color: #495057;
    }
}

* {
    box-sizing: border-box;
}

body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
    line-height: 1.6;
    color: var(--text-primary);
    background-color: var(--bg-primary);
    max-width: 900px;
    margin: 0 auto;
    padding: 2rem;
}

header {
    margin-bottom: 2rem;
    padding-bottom: 1rem;
    border-bottom: 2px solid var(--border-color);
}

header h1 {
    margin: 0 0 0.5rem 0;
    font-size: 1.75rem;
}

.session-id {
    color: var(--text-secondary);
    font-size: 0.9rem;
    margin: 0;
}

.session-id code {
    background-color: var(--bg-code);
    padding: 0.2rem 0.4rem;
    border-radius: 4px;
    font-family: "SF Mono", Monaco, "Cascadia Code", monospace;
}

.message {
    margin-bottom: 1.5rem;
    padding: 1rem;
    border-radius: 8px;
    border-left: 4px solid var(--border-color);
    background-color: var(--bg-secondary);
}

.message.user {
    border-left-color: var(--user-accent);
}

.message.assistant {
    border-left-color: var(--assistant-accent);
}

.message-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.75rem;
    font-weight: 600;
    font-size: 1.1rem;
}

.message-header .icon {
    font-size: 1.2rem;
}

.message-content {
    white-space: pre-wrap;
    word-wrap: break-word;
}

.message-content p {
    margin: 0 0 1rem 0;
}

.message-content p:last-child {
    margin-bottom: 0;
}

details.thinking {
    margin: 0.75rem 0;
    background-color: var(--bg-primary);
    border: 1px solid var(--border-color);
    border-radius: 6px;
    padding: 0.5rem;
}

details.thinking summary {
    cursor: pointer;
    font-weight: 500;
    color: var(--text-secondary);
    padding: 0.25rem;
}

details.thinking .thinking-content {
    margin-top: 0.75rem;
    padding: 0.75rem;
    background-color: var(--bg-code);
    border-radius: 4px;
    white-space: pre-wrap;
    font-size: 0.9rem;
}

.tool-use {
    margin: 1rem 0;
    background-color: var(--bg-primary);
    border: 1px solid var(--border-color);
    border-radius: 6px;
    overflow: hidden;
}

.tool-use-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.5rem 0.75rem;
    background-color: var(--bg-code);
    border-bottom: 1px solid var(--border-color);
    font-weight: 500;
}

.tool-use-header .icon {
    color: var(--tool-accent);
}

.tool-use-header code {
    font-family: "SF Mono", Monaco, "Cascadia Code", monospace;
    background-color: var(--bg-secondary);
    padding: 0.15rem 0.4rem;
    border-radius: 4px;
}

.tool-input {
    padding: 0.75rem;
}

.tool-input pre {
    margin: 0;
    overflow-x: auto;
}

.tool-input code {
    font-family: "SF Mono", Monaco, "Cascadia Code", monospace;
    font-size: 0.85rem;
    line-height: 1.5;
}

.tool-result {
    margin: 0.75rem 0;
    border-radius: 6px;
    overflow: hidden;
    border: 1px solid var(--border-color);
}

.tool-result-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.4rem 0.75rem;
    font-weight: 500;
    font-size: 0.9rem;
}

.tool-result-header.success {
    background-color: rgba(25, 135, 84, 0.1);
    color: var(--success-color);
}

.tool-result-header.error {
    background-color: rgba(220, 53, 69, 0.1);
    color: var(--error-color);
}

.tool-result-content {
    padding: 0.75rem;
    background-color: var(--bg-code);
}

.tool-result-content pre {
    margin: 0;
    overflow-x: auto;
    white-space: pre-wrap;
    word-wrap: break-word;
}

.tool-result-content code {
    font-family: "SF Mono", Monaco, "Cascadia Code", monospace;
    font-size: 0.85rem;
    line-height: 1.5;
}

.truncated {
    color: var(--text-secondary);
    font-style: italic;
}

details.tool-collapsible {
    margin: 1rem 0;
    background-color: var(--bg-primary);
    border: 1px solid var(--border-color);
    border-radius: 6px;
}

details.tool-collapsible summary {
    cursor: pointer;
    padding: 0.5rem 0.75rem;
    background-color: var(--bg-code);
    font-weight: 500;
    display: flex;
    align-items: center;
    gap: 0.5rem;
}

details.tool-collapsible summary .icon {
    color: var(--tool-accent);
}

details.tool-collapsible.error summary .icon {
    color: var(--error-color);
}

details.tool-collapsible .tool-content {
    padding: 0.75rem;
    border-top: 1px solid var(--border-color);
}

details.tool-collapsible .tool-content pre {
    margin: 0;
    overflow-x: auto;
    white-space: pre-wrap;
    word-wrap: break-word;
}

details.tool-collapsible .tool-content code {
    font-family: "SF Mono", Monaco, "Cascadia Code", monospace;
    font-size: 0.85rem;
    line-height: 1.5;
}
`

// RenderHTML converts parsed entries to a styled HTML document.
func RenderHTML(entries []Entry, opts RenderOptions) string {
	var sb strings.Builder

	title := opts.Title
	if title == "" {
		title = "Session Transcript"
	}

	// Write HTML document structure
	sb.WriteString("<!DOCTYPE html>\n")
	sb.WriteString("<html lang=\"en\">\n")
	sb.WriteString("<head>\n")
	sb.WriteString("    <meta charset=\"UTF-8\">\n")
	sb.WriteString("    <meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\">\n")
	sb.WriteString(fmt.Sprintf("    <title>%s</title>\n", html.EscapeString(title)))
	sb.WriteString("    <style>\n")
	sb.WriteString(htmlCSS)
	sb.WriteString("    </style>\n")
	sb.WriteString("</head>\n")
	sb.WriteString("<body>\n")

	// Header
	sb.WriteString("    <header>\n")
	sb.WriteString(fmt.Sprintf("        <h1>%s</h1>\n", html.EscapeString(title)))
	if opts.SessionID != "" {
		sb.WriteString(fmt.Sprintf("        <p class=\"session-id\">Session ID: <code>%s</code></p>\n",
			html.EscapeString(opts.SessionID)))
	}
	sb.WriteString("    </header>\n")

	// Main content
	sb.WriteString("    <main>\n")

	// Initialize tool metadata map at render level (shared across entries)
	// This is critical because tool_use appears in assistant entries but
	// tool_result appears in user entries - the map must persist across both.
	toolMeta := make(map[string]toolMetadata)

	for _, entry := range entries {
		switch entry.Type {
		case "user":
			sb.WriteString(formatUserMessageHTML(&entry, toolMeta))
		case "assistant":
			sb.WriteString(formatAssistantMessageHTML(&entry, toolMeta))
		}
		// Unknown entry types are skipped silently
	}

	sb.WriteString("    </main>\n")
	sb.WriteString("</body>\n")
	sb.WriteString("</html>\n")

	return sb.String()
}

// formatUserMessageHTML formats a user message as HTML.
func formatUserMessageHTML(entry *Entry, toolMeta map[string]toolMetadata) string {
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

	// Check if there's text content to determine header
	hasText := false
	for _, item := range entry.Message.Content {
		if item.Text != "" {
			hasText = true
			break
		}
	}

	if hasText {
		sb.WriteString("        <section class=\"message user\">\n")
		sb.WriteString("            <div class=\"message-header\">\n")
		sb.WriteString("                <span class=\"icon\">👤</span>\n")
		sb.WriteString("                <span>User</span>\n")
		sb.WriteString("            </div>\n")
		sb.WriteString("            <div class=\"message-content\">\n")
	}

	for _, item := range entry.Message.Content {
		switch item.Type {
		case "text":
			if item.Text != "" {
				sb.WriteString("                <p>")
				sb.WriteString(html.EscapeString(item.Text))
				sb.WriteString("</p>\n")
			}
		case "tool_result":
			sb.WriteString(formatToolResultHTML(&item, toolMeta))
		}
	}

	if hasText {
		sb.WriteString("            </div>\n")
		sb.WriteString("        </section>\n")
	}

	return sb.String()
}

// formatAssistantMessageHTML formats an assistant message as HTML.
func formatAssistantMessageHTML(entry *Entry, toolMeta map[string]toolMetadata) string {
	if entry.Message == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("        <section class=\"message assistant\">\n")
	sb.WriteString("            <div class=\"message-header\">\n")
	sb.WriteString("                <span class=\"icon\">🤖</span>\n")
	sb.WriteString("                <span>Assistant</span>\n")
	sb.WriteString("            </div>\n")
	sb.WriteString("            <div class=\"message-content\">\n")

	for _, item := range entry.Message.Content {
		switch item.Type {
		case "thinking":
			if item.Thinking != "" {
				sb.WriteString("                <details class=\"thinking\">\n")
				sb.WriteString("                    <summary>💭 Thinking</summary>\n")
				sb.WriteString("                    <div class=\"thinking-content\">")
				sb.WriteString(html.EscapeString(item.Thinking))
				sb.WriteString("</div>\n")
				sb.WriteString("                </details>\n")
			}

		case "text":
			if item.Text != "" {
				sb.WriteString("                <p>")
				sb.WriteString(html.EscapeString(item.Text))
				sb.WriteString("</p>\n")
			}

		case "tool_use":
			sb.WriteString(formatToolUseHTML(&item, toolMeta))

		case "tool_result":
			// tool_result in assistant entries (legacy handling)
			sb.WriteString(formatToolResultHTML(&item, toolMeta))

		// Unknown content types are skipped silently
		}
	}

	sb.WriteString("            </div>\n")
	sb.WriteString("        </section>\n")

	return sb.String()
}

// formatToolUseHTML formats a tool_use content item as HTML, potentially wrapping it in <details>.
func formatToolUseHTML(item *ContentItem, toolMeta map[string]toolMetadata) string {
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
		sb.WriteString("                <details class=\"tool-collapsible\">\n")
		sb.WriteString(fmt.Sprintf("                    <summary><span class=\"icon\">🔧</span> %s</summary>\n",
			html.EscapeString(summary)))
		sb.WriteString("                    <div class=\"tool-content\">\n")
		if len(inputJSON) > 0 {
			inputStr := truncateString(string(inputJSON), MaxToolInputRunes)
			sb.WriteString("                        <pre><code>")
			sb.WriteString(html.EscapeString(inputStr))
			sb.WriteString("</code></pre>\n")
		}
		sb.WriteString("                    </div>\n")
		sb.WriteString("                </details>\n")
	} else {
		// Uncollapsed format with div.tool-use
		sb.WriteString("                <div class=\"tool-use\">\n")
		sb.WriteString("                    <div class=\"tool-use-header\">\n")
		sb.WriteString("                        <span class=\"icon\">🔧</span>\n")
		sb.WriteString(fmt.Sprintf("                        <span>Tool: <code>%s</code></span>\n",
			html.EscapeString(item.Name)))
		sb.WriteString("                    </div>\n")
		if len(inputJSON) > 0 {
			inputStr := truncateString(string(inputJSON), MaxToolInputRunes)
			sb.WriteString("                    <div class=\"tool-input\">\n")
			sb.WriteString("                        <pre><code>")
			sb.WriteString(html.EscapeString(inputStr))
			sb.WriteString("</code></pre>\n")
			sb.WriteString("                    </div>\n")
		}
		sb.WriteString("                </div>\n")
	}

	return sb.String()
}

// formatToolResultHTML formats a tool_result content item as HTML, potentially wrapping it in <details>.
func formatToolResultHTML(item *ContentItem, toolMeta map[string]toolMetadata) string {
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
		errorClass := ""
		if item.IsError {
			errorClass = " error"
		}
		sb.WriteString(fmt.Sprintf("                <details class=\"tool-collapsible%s\">\n", errorClass))
		sb.WriteString(fmt.Sprintf("                    <summary><span class=\"icon\">%s</span> %s</summary>\n",
			icon, html.EscapeString(summary)))
		sb.WriteString("                    <div class=\"tool-content\">\n")
		sb.WriteString("                        <pre><code>")
		sb.WriteString(html.EscapeString(truncatedContent))
		sb.WriteString("</code></pre>\n")
		sb.WriteString("                    </div>\n")
		sb.WriteString("                </details>\n")
	} else {
		// Uncollapsed format with div.tool-result
		headerClass := "success"
		headerText := "Tool Result"
		if item.IsError {
			headerClass = "error"
			headerText = "Tool Error"
		}
		sb.WriteString("                <div class=\"tool-result\">\n")
		sb.WriteString(fmt.Sprintf("                    <div class=\"tool-result-header %s\">\n", headerClass))
		sb.WriteString(fmt.Sprintf("                        <span>%s %s</span>\n", icon, headerText))
		sb.WriteString("                    </div>\n")
		sb.WriteString("                    <div class=\"tool-result-content\">\n")
		sb.WriteString("                        <pre><code>")
		sb.WriteString(html.EscapeString(truncatedContent))
		sb.WriteString("</code></pre>\n")
		sb.WriteString("                    </div>\n")
		sb.WriteString("                </div>\n")
	}

	return sb.String()
}
