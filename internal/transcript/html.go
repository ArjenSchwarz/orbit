package transcript

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	stdhtml "html"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

//go:embed transcript.css
var transcriptCSS string

// TranscriptCSS returns the CSS for transcript rendering.
// This can be used by external packages (like the web server) to include
// the same styles without duplication.
func TranscriptCSS() string {
	return transcriptCSS
}

// standaloneCSS contains additional styles only needed for standalone HTML documents.
const standaloneCSS = `
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

.session-id,
.session-cost {
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
`

// mdConverter is the shared goldmark markdown converter instance.
var mdConverter = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(
		gmhtml.WithUnsafe(),
	),
)

// markdownToHTML converts markdown text to HTML.
// Returns the HTML string without wrapper tags.
func markdownToHTML(markdown string) string {
	var buf bytes.Buffer
	if err := mdConverter.Convert([]byte(markdown), &buf); err != nil {
		// On error, fall back to escaped text
		return stdhtml.EscapeString(markdown)
	}
	return buf.String()
}

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
	sb.WriteString(fmt.Sprintf("    <title>%s</title>\n", stdhtml.EscapeString(title)))
	sb.WriteString("    <style>\n")
	sb.WriteString(transcriptCSS)
	sb.WriteString(standaloneCSS)
	sb.WriteString("    </style>\n")
	sb.WriteString("</head>\n")
	sb.WriteString("<body>\n")

	// Header
	sb.WriteString("    <header>\n")
	sb.WriteString(fmt.Sprintf("        <h1>%s</h1>\n", stdhtml.EscapeString(title)))
	if opts.SessionID != "" {
		sb.WriteString(fmt.Sprintf("        <p class=\"session-id\">Session ID: <code>%s</code></p>\n",
			stdhtml.EscapeString(opts.SessionID)))
	}
	if opts.TotalCost != nil && *opts.TotalCost >= 0.005 {
		unit := opts.CostUnit
		if unit == "" {
			unit = "credits"
		}
		sb.WriteString(fmt.Sprintf("        <p class=\"session-cost\">Cost: %.2f %s</p>\n",
			*opts.TotalCost, stdhtml.EscapeString(unit)))
	}
	sb.WriteString("    </header>\n")

	// Navigation at top (if provided)
	if opts.Navigation != nil {
		sb.WriteString(renderNavigationHTML(opts.Navigation))
	}

	// Main content
	sb.WriteString("    <main>\n")

	// Render entries using shared function
	renderEntriesToBuilder(&sb, entries, opts)

	sb.WriteString("    </main>\n")

	// Navigation at bottom (if provided)
	if opts.Navigation != nil {
		sb.WriteString(renderNavigationHTML(opts.Navigation))
	}

	// Inline script for locale-aware timestamp formatting in standalone HTML.
	// The web interface (apsis serve) has its own formatLocalDates in layout.html,
	// but standalone documents need this self-contained IIFE.
	sb.WriteString(`<script>
(function() {
    var fmt = new Intl.DateTimeFormat(undefined, {
        day: 'numeric', month: 'short', year: 'numeric',
        hour: 'numeric', minute: '2-digit'
    });
    document.querySelectorAll('time[datetime]').forEach(function(el) {
        var d = new Date(el.getAttribute('datetime'));
        if (!isNaN(d)) el.textContent = fmt.format(d);
    });
})();
</script>
`)

	sb.WriteString("</body>\n")
	sb.WriteString("</html>\n")

	return sb.String()
}

// RenderHTMLFragment renders just the content without document wrapper.
// Returns HTML that can be embedded in an existing page template.
// Includes navigation at top/bottom when Navigation is set.
// Does NOT include <!DOCTYPE>, <html>, <head>, or <body> tags.
func RenderHTMLFragment(entries []Entry, opts RenderOptions) string {
	var sb strings.Builder

	// Navigation at top (if provided)
	if opts.Navigation != nil {
		sb.WriteString(renderNavigationHTML(opts.Navigation))
	}

	// Render entries using shared function
	renderEntriesToBuilder(&sb, entries, opts)

	// Navigation at bottom (if provided)
	if opts.Navigation != nil {
		sb.WriteString(renderNavigationHTML(opts.Navigation))
	}

	return sb.String()
}

// renderEntriesToBuilder writes entry HTML to the builder.
// Extracted to share between RenderHTML and RenderHTMLFragment.
func renderEntriesToBuilder(sb *strings.Builder, entries []Entry, opts RenderOptions) {
	// Pre-process entries to group consecutive Read calls
	groups := preprocessEntries(entries)

	// Extract project directory from entries (use cwd from first entry that has it)
	projectDir := opts.ProjectDir
	if projectDir == "" {
		for i := range entries {
			if entries[i].Cwd != "" {
				projectDir = entries[i].Cwd
				break
			}
		}
	}

	// Build skill description map from meta entries
	skillDescriptions := BuildSkillDescriptionMap(entries)

	// Initialize tool metadata map at render level (shared across entries)
	// This is critical because tool_use appears in assistant entries but
	// tool_result appears in user entries - the map must persist across both.
	toolMeta := make(map[string]toolMetadata)

	for _, group := range groups {
		switch group.Type {
		case "user":
			for i := range group.Entries {
				sb.WriteString(formatUserMessageHTML(&group.Entries[i], toolMeta, skillDescriptions))
			}
		case "assistant":
			for i := range group.Entries {
				sb.WriteString(formatAssistantMessageHTML(&group.Entries[i], toolMeta, skillDescriptions))
			}
		case "read_group":
			sb.WriteString(formatReadGroupHTML(group.Reads, projectDir))
		case "edit_group":
			sb.WriteString(formatEditGroupHTML(group.Edits, projectDir))
		}
	}
}

// renderNavigationHTML generates the navigation bar HTML with prev/next/back links.
func renderNavigationHTML(nav *NavigationContext) string {
	if nav == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("    <nav class=\"transcript-nav\">\n")

	// Previous link
	if nav.PrevURL != "" {
		sb.WriteString(fmt.Sprintf("        <a href=\"%s\" class=\"nav-prev\">%s</a>\n",
			stdhtml.EscapeString(nav.PrevURL), stdhtml.EscapeString(nav.PrevText)))
	} else {
		sb.WriteString("        <span class=\"nav-spacer\"></span>\n")
	}

	// Back link (center)
	if nav.BackURL != "" {
		sb.WriteString(fmt.Sprintf("        <a href=\"%s\" class=\"nav-back\">%s</a>\n",
			stdhtml.EscapeString(nav.BackURL), stdhtml.EscapeString(nav.BackText)))
	}

	// Next link
	if nav.NextURL != "" {
		sb.WriteString(fmt.Sprintf("        <a href=\"%s\" class=\"nav-next\">%s</a>\n",
			stdhtml.EscapeString(nav.NextURL), stdhtml.EscapeString(nav.NextText)))
	} else {
		sb.WriteString("        <span class=\"nav-spacer\"></span>\n")
	}

	sb.WriteString("    </nav>\n")
	return sb.String()
}

// writeMessageMetaHTML appends the metadata span to a message header if metadata is available.
func writeMessageMetaHTML(sb *strings.Builder, timestamp, model string) {
	if meta := FormatMessageMetaHTML(timestamp, model); meta != "" {
		sb.WriteString("                ")
		sb.WriteString(meta)
		sb.WriteString("\n")
	}
}

// formatReadGroupHTML formats a group of consecutive Read tool calls as HTML.
func formatReadGroupHTML(reads []readItem, projectDir string) string {
	if len(reads) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("        <section class=\"message assistant\">\n")
	sb.WriteString("            <div class=\"message-header\">\n")
	sb.WriteString("                <span class=\"icon\">🤖</span>\n")
	sb.WriteString("                <span>Assistant</span>\n")
	writeMessageMetaHTML(&sb, reads[0].Timestamp, "")
	sb.WriteString("            </div>\n")
	sb.WriteString("            <div class=\"message-content\">\n")

	for _, read := range reads {
		icon := "✅"
		if read.IsError {
			icon = "❌"
		}

		displayPath := stripProjectDir(read.FilePath, projectDir)
		sb.WriteString("                <details class=\"tool-collapsible read-item\">\n")
		sb.WriteString(fmt.Sprintf("                    <summary><span class=\"icon\">%s</span> 🔧 Read: <code>%s</code></summary>\n",
			icon, stdhtml.EscapeString(displayPath)))
		sb.WriteString("                    <div class=\"tool-content\">\n")

		if read.Content != "" {
			sb.WriteString("                        <pre><code>")
			sb.WriteString(stdhtml.EscapeString(read.Content))
			sb.WriteString("</code></pre>\n")
		}

		sb.WriteString("                    </div>\n")
		sb.WriteString("                </details>\n")
	}

	sb.WriteString("            </div>\n")
	sb.WriteString("        </section>\n")

	return sb.String()
}

// formatEditGroupHTML formats a group of consecutive Edit tool calls as HTML.
func formatEditGroupHTML(edits []editItem, projectDir string) string {
	if len(edits) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("        <section class=\"message assistant\">\n")
	sb.WriteString("            <div class=\"message-header\">\n")
	sb.WriteString("                <span class=\"icon\">🤖</span>\n")
	sb.WriteString("                <span>Assistant</span>\n")
	writeMessageMetaHTML(&sb, edits[0].Timestamp, "")
	sb.WriteString("            </div>\n")
	sb.WriteString("            <div class=\"message-content\">\n")

	for _, edit := range edits {
		icon := "✅"
		if edit.IsError {
			icon = "❌"
		}

		displayPath := stripProjectDir(edit.FilePath, projectDir)
		sb.WriteString("                <details class=\"tool-collapsible read-item\">\n")
		sb.WriteString(fmt.Sprintf("                    <summary><span class=\"icon\">%s</span> 🔧 Edit: <code>%s</code></summary>\n",
			icon, stdhtml.EscapeString(displayPath)))
		sb.WriteString("                    <div class=\"tool-content\">\n")

		if len(edit.Patch) > 0 {
			sb.WriteString("                        <div class=\"patch-content\">\n")
			for _, hunk := range edit.Patch {
				for _, line := range hunk.Lines {
					lineClass := "context"
					if len(line) > 0 {
						switch line[0] {
						case '+':
							lineClass = "addition"
						case '-':
							lineClass = "deletion"
						}
					}
					sb.WriteString(fmt.Sprintf("                            <span class=\"patch-line %s\">%s</span>\n",
						lineClass, stdhtml.EscapeString(line)))
				}
			}
			sb.WriteString("                        </div>\n")
		}

		sb.WriteString("                    </div>\n")
		sb.WriteString("                </details>\n")
	}

	sb.WriteString("            </div>\n")
	sb.WriteString("        </section>\n")

	return sb.String()
}

// formatUserMessageHTML formats a user message as HTML.
func formatUserMessageHTML(entry *Entry, toolMeta map[string]toolMetadata, skillDescriptions map[string]string) string {
	if entry.Message == nil || len(entry.Message.Content) == 0 {
		return ""
	}

	// Skip skill/command description meta entries - they're rendered with the Skill/command block
	if entry.IsMeta {
		return ""
	}

	// Check if this is a slash command entry (e.g., /catchup)
	if isSlashCommandEntry(entry) {
		return formatSlashCommandHTML(entry, skillDescriptions)
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
		writeMessageMetaHTML(&sb, entry.Timestamp, "")
		sb.WriteString("            </div>\n")
		sb.WriteString("            <div class=\"message-content\">\n")
	}

	for _, item := range entry.Message.Content {
		switch item.Type {
		case "text":
			if item.Text != "" {
				sb.WriteString("                <p>")
				sb.WriteString(stdhtml.EscapeString(item.Text))
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

// formatSlashCommandHTML formats a slash command entry as HTML.
func formatSlashCommandHTML(entry *Entry, skillDescriptions map[string]string) string {
	if entry.Message == nil {
		return ""
	}

	// Extract command name from the entry
	var commandName string
	for _, item := range entry.Message.Content {
		if item.Type == "text" {
			commandName = parseSlashCommand(item.Text)
			if commandName != "" {
				break
			}
		}
	}

	if commandName == "" {
		return ""
	}

	// Look up description by entry UUID
	var description string
	if entry.UUID != "" && skillDescriptions != nil {
		description = skillDescriptions[entry.UUID]
	}

	var sb strings.Builder
	sb.WriteString("        <section class=\"message user\">\n")
	sb.WriteString("            <div class=\"message-header\">\n")
	sb.WriteString("                <span class=\"icon\">👤</span>\n")
	sb.WriteString("                <span>User</span>\n")
	writeMessageMetaHTML(&sb, entry.Timestamp, "")
	sb.WriteString("            </div>\n")
	sb.WriteString("            <div class=\"message-content\">\n")

	if description != "" {
		// Render as collapsible with description
		sb.WriteString("                <details class=\"tool-collapsible\">\n")
		sb.WriteString(fmt.Sprintf("                    <summary><span class=\"icon\">⚡</span> /%s</summary>\n",
			stdhtml.EscapeString(commandName)))
		sb.WriteString("                    <div class=\"tool-content\">\n")
		sb.WriteString("                        <div class=\"markdown-content\">\n")
		sb.WriteString(markdownToHTML(description))
		sb.WriteString("                        </div>\n")
		sb.WriteString("                    </div>\n")
		sb.WriteString("                </details>\n")
	} else {
		// Simple format without description
		sb.WriteString(fmt.Sprintf("                <div class=\"tool-use\"><span class=\"icon\">⚡</span> /%s</div>\n",
			stdhtml.EscapeString(commandName)))
	}

	sb.WriteString("            </div>\n")
	sb.WriteString("        </section>\n")

	return sb.String()
}

// formatAssistantMessageHTML formats an assistant message as HTML.
func formatAssistantMessageHTML(entry *Entry, toolMeta map[string]toolMetadata, skillDescriptions map[string]string) string {
	if entry.Message == nil {
		return ""
	}

	// First pass: check if there will be any content to render in the section
	// (non-Task/Skill and subagent Task tool_use items don't render in the section - they render with their results)
	hasContent := false
	for _, item := range entry.Message.Content {
		switch item.Type {
		case "thinking":
			if item.Thinking != "" {
				hasContent = true
			}
		case "text":
			if strings.TrimSpace(item.Text) != "" {
				hasContent = true
			}
		case "tool_use":
			// Only Skill and non-subagent Task render in the assistant section
			if item.Name == "Skill" {
				hasContent = true
			} else if item.Name == "Task" && !isSubagent(item.Input) {
				hasContent = true
			}
		case "tool_result":
			hasContent = true
		}
		if hasContent {
			break
		}
	}

	// Always store tool metadata even if we don't render the section
	// This covers other tools and subagent Tasks (which defer rendering)
	for _, item := range entry.Message.Content {
		if item.Type == "tool_use" {
			// Skip Skill and non-subagent Task (they render immediately in formatToolUseHTML)
			if item.Name == "Skill" {
				continue
			}
			if item.Name == "Task" && !isSubagent(item.Input) {
				continue
			}
			summary := getToolSummary(item.Name, item.Input)
			description := getToolDescription(item.Input)
			if summary == "" {
				summary = "Tool: " + item.Name
			}
			subagent := item.Name == "Task" && isSubagent(item.Input)
			if item.ID != "" {
				toolMeta[item.ID] = toolMetadata{
					Name:        item.Name,
					Summary:     summary,
					Description: description,
					Input:       item.Input,
					Prompt:      getSubagentPrompt(item.Input),
					IsSubagent:  subagent,
				}
			}
		}
	}

	if !hasContent {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("        <section class=\"message assistant\">\n")
	sb.WriteString("            <div class=\"message-header\">\n")
	sb.WriteString("                <span class=\"icon\">🤖</span>\n")
	sb.WriteString("                <span>Assistant</span>\n")
	writeMessageMetaHTML(&sb, entry.Timestamp, entry.Model)
	sb.WriteString("            </div>\n")
	sb.WriteString("            <div class=\"message-content\">\n")

	for _, item := range entry.Message.Content {
		switch item.Type {
		case "thinking":
			if item.Thinking != "" {
				sb.WriteString("                <div class=\"thinking-block\">\n")
				sb.WriteString("                    <div class=\"thinking-header\">💭 Thinking</div>\n")
				sb.WriteString("                    <div class=\"thinking-content markdown-content\">\n")
				sb.WriteString(markdownToHTML(item.Thinking))
				sb.WriteString("                    </div>\n")
				sb.WriteString("                </div>\n")
			}

		case "text":
			if item.Text != "" {
				sb.WriteString("                <div class=\"markdown-content\">\n")
				sb.WriteString(markdownToHTML(item.Text))
				sb.WriteString("                </div>\n")
			}

		case "tool_use":
			sb.WriteString(formatToolUseHTML(&item, toolMeta, skillDescriptions))

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

// formatToolUseHTML formats a tool_use content item as HTML.
// For Skill: renders collapsible block with description (if available) or simple non-collapsible block.
// For non-subagent Task: renders collapsed details block with JSON.
// For subagent Task and other tools: stores metadata and defers rendering to formatToolResultHTML.
func formatToolUseHTML(item *ContentItem, toolMeta map[string]toolMetadata, skillDescriptions map[string]string) string {
	// Get summary text and description
	summary := getToolSummary(item.Name, item.Input)
	description := getToolDescription(item.Input)

	if summary == "" {
		switch item.Name {
		case "Task":
			summary = "Task"
		case "Skill":
			summary = "Skill"
		default:
			summary = "Tool: " + item.Name
		}
	}

	// Check if this is a subagent Task
	subagent := item.Name == "Task" && isSubagent(item.Input)

	// Look up skill description if this is a Skill tool
	var skillDesc string
	if item.Name == "Skill" && item.ID != "" && skillDescriptions != nil {
		skillDesc = skillDescriptions[item.ID]
	}

	// Store metadata for result matching (if ID is present)
	if item.ID != "" {
		toolMeta[item.ID] = toolMetadata{
			Name:             item.Name,
			Summary:          summary,
			Description:      description,
			Input:            item.Input,
			Prompt:           getSubagentPrompt(item.Input),
			IsSubagent:       subagent,
			SkillDescription: skillDesc,
		}
	}

	// For subagent Task, defer rendering to formatToolResultHTML
	if subagent {
		return ""
	}

	// For Skill, render with description if available, otherwise simple block
	if item.Name == "Skill" {
		if skillDesc != "" {
			var sb strings.Builder
			sb.WriteString("                <details class=\"tool-collapsible\">\n")
			sb.WriteString(fmt.Sprintf("                    <summary><span class=\"icon\">🔧</span> %s</summary>\n",
				stdhtml.EscapeString(summary)))
			sb.WriteString("                    <div class=\"tool-content\">\n")
			sb.WriteString("                        <div class=\"markdown-content\">\n")
			sb.WriteString(markdownToHTML(skillDesc))
			sb.WriteString("                        </div>\n")
			sb.WriteString("                    </div>\n")
			sb.WriteString("                </details>\n")
			return sb.String()
		}
		// No description, render simple non-collapsible block
		return fmt.Sprintf("                <div class=\"tool-use\"><span class=\"icon\">🔧</span> %s</div>\n",
			stdhtml.EscapeString(summary))
	}

	// For non-subagent Task, render collapsed block now
	if item.Name == "Task" {
		var sb strings.Builder
		var inputJSON []byte
		if item.Input != nil {
			inputJSON, _ = json.MarshalIndent(item.Input, "", "  ")
		}

		sb.WriteString("                <details class=\"tool-collapsible\">\n")
		sb.WriteString(fmt.Sprintf("                    <summary><span class=\"icon\">🔧</span> %s</summary>\n",
			stdhtml.EscapeString(summary)))
		sb.WriteString("                    <div class=\"tool-content\">\n")
		if len(inputJSON) > 0 {
			sb.WriteString("                        <pre><code>")
			sb.WriteString(stdhtml.EscapeString(string(inputJSON)))
			sb.WriteString("</code></pre>\n")
		}
		sb.WriteString("                    </div>\n")
		sb.WriteString("                </details>\n")
		return sb.String()
	}

	// For other tools, don't render here - will be rendered with tool_result
	return ""
}

// formatToolResultHTML formats a tool_result content item as HTML.
// For subagent Task: renders combined Prompt/Result with 🤖 emoji in assistant section.
// For Skill: skips rendering (already shown in tool_use, result is just "Launching skill: X").
// For non-subagent Task: renders just the result in a collapsed block.
// For other tools: renders the combined tool call + result in a single details block.
func formatToolResultHTML(item *ContentItem, toolMeta map[string]toolMetadata) string {
	var sb strings.Builder

	content := item.Content

	// Determine icon
	icon := "✅"
	if item.IsError {
		icon = "❌"
	}

	// Look up tool metadata
	meta, found := toolMeta[item.ToolUseID]

	// For subagent Task, render combined Prompt/Result with robot emoji in assistant section
	if found && meta.IsSubagent {
		errorClass := ""
		if item.IsError {
			errorClass = " error"
		}

		// Wrap in assistant section since tool calls come from assistant
		sb.WriteString("        <section class=\"message assistant\">\n")
		sb.WriteString("            <div class=\"message-header\">\n")
		sb.WriteString("                <span class=\"icon\">🤖</span>\n")
		sb.WriteString("                <span>Assistant</span>\n")
		sb.WriteString("            </div>\n")
		sb.WriteString("            <div class=\"message-content\">\n")

		sb.WriteString(fmt.Sprintf("                <details class=\"tool-collapsible%s\">\n", errorClass))
		sb.WriteString(fmt.Sprintf("                    <summary><span class=\"icon\">%s</span> 🤖🔧 %s</summary>\n",
			icon, stdhtml.EscapeString(meta.Summary)))
		sb.WriteString("                    <div class=\"tool-content\">\n")

		// Render prompt
		if meta.Prompt != "" {
			sb.WriteString("                        <div class=\"tool-input-section\">\n")
			sb.WriteString("                            <strong>Prompt:</strong>\n")
			sb.WriteString("                            <pre><code>")
			sb.WriteString(stdhtml.EscapeString(meta.Prompt))
			sb.WriteString("</code></pre>\n")
			sb.WriteString("                        </div>\n")
		}

		// Render result - extract text from JSON array and render as markdown
		resultText := extractSubagentResultText(content)
		sb.WriteString("                        <div class=\"tool-result-section\">\n")
		sb.WriteString("                            <strong>Result:</strong>\n")
		sb.WriteString("                            <div class=\"markdown-content\">\n")
		sb.WriteString(markdownToHTML(resultText))
		sb.WriteString("                            </div>\n")
		sb.WriteString("                        </div>\n")

		sb.WriteString("                    </div>\n")
		sb.WriteString("                </details>\n")

		sb.WriteString("            </div>\n")
		sb.WriteString("        </section>\n")
		return sb.String()
	}

	// For Skill, skip rendering the result (already shown in tool_use, result is just "Launching skill: X")
	if found && meta.Name == "Skill" {
		return ""
	}

	// For non-subagent Task, render just the result
	if found && meta.Name == "Task" {
		errorClass := ""
		if item.IsError {
			errorClass = " error"
		}
		sb.WriteString(fmt.Sprintf("                <details class=\"tool-collapsible%s\">\n", errorClass))
		sb.WriteString(fmt.Sprintf("                    <summary><span class=\"icon\">%s</span> %s</summary>\n",
			icon, stdhtml.EscapeString(meta.Summary)))
		sb.WriteString("                    <div class=\"tool-content\">\n")
		sb.WriteString("                        <pre><code>")
		sb.WriteString(stdhtml.EscapeString(content))
		sb.WriteString("</code></pre>\n")
		sb.WriteString("                    </div>\n")
		sb.WriteString("                </details>\n")
		return sb.String()
	}

	// For other tools, render combined tool call + result in its own assistant section
	if found {
		errorClass := ""
		if item.IsError {
			errorClass = " error"
		}

		// Build summary: tool name + description
		summaryText := meta.Name
		if meta.Description != "" {
			summaryText += ": " + meta.Description
		}

		// Determine if this tool should be expanded by default
		openAttr := ""
		if meta.Name == "TodoWrite" {
			openAttr = " open"
		}

		// Wrap in assistant section since tool calls come from assistant
		sb.WriteString("        <section class=\"message assistant\">\n")
		sb.WriteString("            <div class=\"message-header\">\n")
		sb.WriteString("                <span class=\"icon\">🤖</span>\n")
		sb.WriteString("                <span>Assistant</span>\n")
		sb.WriteString("            </div>\n")
		sb.WriteString("            <div class=\"message-content\">\n")

		sb.WriteString(fmt.Sprintf("                <details class=\"tool-collapsible%s\"%s>\n", errorClass, openAttr))
		sb.WriteString(fmt.Sprintf("                    <summary><span class=\"icon\">%s</span> 🔧 %s</summary>\n",
			icon, stdhtml.EscapeString(summaryText)))
		sb.WriteString("                    <div class=\"tool-content\">\n")

		// Render tool-specific input
		sb.WriteString(formatToolInputHTML(meta.Name, meta.Input))

		// Render result (skip for TodoWrite as it contains no useful information)
		if meta.Name != "TodoWrite" {
			sb.WriteString("                        <div class=\"tool-result-section\">\n")
			sb.WriteString("                            <strong>Result:</strong>\n")
			sb.WriteString("                            <pre><code>")
			sb.WriteString(stdhtml.EscapeString(content))
			sb.WriteString("</code></pre>\n")
			sb.WriteString("                        </div>\n")
		}

		sb.WriteString("                    </div>\n")
		sb.WriteString("                </details>\n")

		sb.WriteString("            </div>\n")
		sb.WriteString("        </section>\n")
	} else {
		// Unmatched result: render standalone
		errorClass := ""
		if item.IsError {
			errorClass = " error"
		}
		summaryText := "Tool Result"
		if item.IsError {
			summaryText = "Tool Error"
		}
		sb.WriteString(fmt.Sprintf("                <details class=\"tool-collapsible%s\">\n", errorClass))
		sb.WriteString(fmt.Sprintf("                    <summary><span class=\"icon\">%s</span> %s</summary>\n",
			icon, summaryText))
		sb.WriteString("                    <div class=\"tool-content\">\n")
		sb.WriteString("                        <pre><code>")
		sb.WriteString(stdhtml.EscapeString(content))
		sb.WriteString("</code></pre>\n")
		sb.WriteString("                    </div>\n")
		sb.WriteString("                </details>\n")
	}

	return sb.String()
}

// formatToolInputHTML formats tool-specific input for HTML display.
func formatToolInputHTML(name string, input any) string {
	inputMap, ok := input.(map[string]any)
	if !ok {
		return ""
	}

	var sb strings.Builder

	switch name {
	case "Bash":
		if cmd, ok := inputMap["command"].(string); ok {
			sb.WriteString("                        <div class=\"tool-input-section\">\n")
			sb.WriteString("                            <strong>Command:</strong>\n")
			sb.WriteString("                            <pre><code>")
			sb.WriteString(stdhtml.EscapeString(cmd))
			sb.WriteString("</code></pre>\n")
			sb.WriteString("                        </div>\n")
		}
	case "Write":
		if path, ok := inputMap["file_path"].(string); ok {
			sb.WriteString("                        <div class=\"tool-input-section\">\n")
			sb.WriteString(fmt.Sprintf("                            <strong>File:</strong> <code>%s</code>\n",
				stdhtml.EscapeString(path)))
			sb.WriteString("                        </div>\n")
		}
	case "Edit":
		if path, ok := inputMap["file_path"].(string); ok {
			sb.WriteString("                        <div class=\"tool-input-section\">\n")
			sb.WriteString(fmt.Sprintf("                            <strong>File:</strong> <code>%s</code>\n",
				stdhtml.EscapeString(path)))
			sb.WriteString("                        </div>\n")
		}
	case "Glob":
		if pattern, ok := inputMap["pattern"].(string); ok {
			sb.WriteString("                        <div class=\"tool-input-section\">\n")
			sb.WriteString(fmt.Sprintf("                            <strong>Pattern:</strong> <code>%s</code>\n",
				stdhtml.EscapeString(pattern)))
			sb.WriteString("                        </div>\n")
		}
	case "Grep":
		if pattern, ok := inputMap["pattern"].(string); ok {
			sb.WriteString("                        <div class=\"tool-input-section\">\n")
			sb.WriteString(fmt.Sprintf("                            <strong>Pattern:</strong> <code>%s</code>\n",
				stdhtml.EscapeString(pattern)))
			sb.WriteString("                        </div>\n")
		}
	case "TodoWrite":
		if todos, ok := inputMap["todos"].([]any); ok {
			sb.WriteString("                        <div class=\"tool-input-section\">\n")
			sb.WriteString("                            <ul class=\"todo-list\">\n")
			for _, todo := range todos {
				if todoMap, ok := todo.(map[string]any); ok {
					content, _ := todoMap["content"].(string)
					status, _ := todoMap["status"].(string)
					checkbox := "[ ]"
					switch status {
					case "in_progress":
						checkbox = "[-]"
					case "completed":
						checkbox = "[x]"
					}
					sb.WriteString(fmt.Sprintf("                                <li>%s %s</li>\n",
						checkbox, stdhtml.EscapeString(content)))
				}
			}
			sb.WriteString("                            </ul>\n")
			sb.WriteString("                        </div>\n")
		}
	default:
		// For unknown tools, show JSON input
		if input != nil {
			inputJSON, err := json.MarshalIndent(input, "", "  ")
			if err == nil {
				sb.WriteString("                        <div class=\"tool-input-section\">\n")
				sb.WriteString("                            <strong>Input:</strong>\n")
				sb.WriteString("                            <pre><code>")
				sb.WriteString(stdhtml.EscapeString(string(inputJSON)))
				sb.WriteString("</code></pre>\n")
				sb.WriteString("                        </div>\n")
			}
		}
	}

	return sb.String()
}
