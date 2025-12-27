package transcript

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"
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

	for _, entry := range entries {
		switch entry.Type {
		case "user":
			sb.WriteString(formatUserMessageHTML(&entry))
		case "assistant":
			sb.WriteString(formatAssistantMessageHTML(&entry))
		}
		// Unknown entry types are skipped silently
	}

	sb.WriteString("    </main>\n")
	sb.WriteString("</body>\n")
	sb.WriteString("</html>\n")

	return sb.String()
}

// formatUserMessageHTML formats a user message as HTML.
func formatUserMessageHTML(entry *Entry) string {
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
	sb.WriteString("        <section class=\"message user\">\n")
	sb.WriteString("            <div class=\"message-header\">\n")
	sb.WriteString("                <span class=\"icon\">👤</span>\n")
	sb.WriteString("                <span>User</span>\n")
	sb.WriteString("            </div>\n")
	sb.WriteString("            <div class=\"message-content\">\n")

	for _, text := range texts {
		sb.WriteString("                <p>")
		sb.WriteString(html.EscapeString(text))
		sb.WriteString("</p>\n")
	}

	sb.WriteString("            </div>\n")
	sb.WriteString("        </section>\n")

	return sb.String()
}

// formatAssistantMessageHTML formats an assistant message as HTML.
func formatAssistantMessageHTML(entry *Entry) string {
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
			sb.WriteString("                <div class=\"tool-use\">\n")
			sb.WriteString("                    <div class=\"tool-use-header\">\n")
			sb.WriteString("                        <span class=\"icon\">🔧</span>\n")
			sb.WriteString(fmt.Sprintf("                        <span>Tool: <code>%s</code></span>\n",
				html.EscapeString(item.Name)))
			sb.WriteString("                    </div>\n")
			if item.Input != nil {
				inputJSON, err := json.MarshalIndent(item.Input, "", "  ")
				if err == nil {
					inputStr := truncateString(string(inputJSON), MaxToolInputRunes)
					sb.WriteString("                    <div class=\"tool-input\">\n")
					sb.WriteString("                        <pre><code>")
					sb.WriteString(html.EscapeString(inputStr))
					sb.WriteString("</code></pre>\n")
					sb.WriteString("                    </div>\n")
				}
			}
			sb.WriteString("                </div>\n")

		case "tool_result":
			content := truncateString(item.Content, MaxToolResultRunes)
			headerClass := "success"
			headerIcon := "✅"
			headerText := "Tool Result"
			if item.IsError {
				headerClass = "error"
				headerIcon = "❌"
				headerText = "Tool Error"
			}
			sb.WriteString("                <div class=\"tool-result\">\n")
			sb.WriteString(fmt.Sprintf("                    <div class=\"tool-result-header %s\">\n", headerClass))
			sb.WriteString(fmt.Sprintf("                        <span>%s %s</span>\n", headerIcon, headerText))
			sb.WriteString("                    </div>\n")
			sb.WriteString("                    <div class=\"tool-result-content\">\n")
			sb.WriteString("                        <pre><code>")
			sb.WriteString(html.EscapeString(content))
			sb.WriteString("</code></pre>\n")
			sb.WriteString("                    </div>\n")
			sb.WriteString("                </div>\n")

		// Unknown content types are skipped silently
		}
	}

	sb.WriteString("            </div>\n")
	sb.WriteString("        </section>\n")

	return sb.String()
}
