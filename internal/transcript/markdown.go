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
	Name             string // Tool name (e.g., "Task", "Read")
	Summary          string // Summary text for collapsed display
	Description      string // Description field from input (for Bash, etc.)
	Input            any    // Full input for rendering
	Prompt           string // Prompt text for subagent Task calls
	IsSubagent       bool   // True if Task has subagent_type
	SkillDescription string // Full skill description from meta entry (for Skill tools)
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
	skillDescriptions := buildSkillDescriptionMap(entries)

	// Initialize tool metadata map at render level (shared across entries)
	// This is critical because tool_use appears in assistant entries but
	// tool_result appears in user entries - the map must persist across both.
	toolMeta := make(map[string]toolMetadata)

	// Render each group
	for _, group := range groups {
		switch group.Type {
		case "user":
			for i := range group.Entries {
				sb.WriteString(formatUserMessage(&group.Entries[i], toolMeta, skillDescriptions))
			}
		case "assistant":
			for i := range group.Entries {
				sb.WriteString(formatAssistantMessage(&group.Entries[i], toolMeta, skillDescriptions))
			}
		case "read_group":
			sb.WriteString(formatReadGroup(group.Reads, projectDir))
		case "edit_group":
			sb.WriteString(formatEditGroup(group.Edits, projectDir))
		}
	}

	return sb.String()
}

// formatReadGroup formats a group of consecutive Read tool calls as a single block.
func formatReadGroup(reads []readItem, projectDir string) string {
	if len(reads) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## 🤖 Assistant\n\n")

	for _, read := range reads {
		icon := "✅"
		if read.IsError {
			icon = "❌"
		}

		displayPath := stripProjectDir(read.FilePath, projectDir)
		sb.WriteString("<details>\n")
		sb.WriteString(fmt.Sprintf("<summary>%s 🔧 Read: <code>%s</code></summary>\n\n",
			icon, html.EscapeString(displayPath)))

		if read.Content != "" {
			sb.WriteString("```\n")
			sb.WriteString(read.Content)
			sb.WriteString("\n```\n\n")
		}

		sb.WriteString("</details>\n\n")
	}

	sb.WriteString("---\n\n")
	return sb.String()
}

// formatEditGroup formats a group of consecutive Edit tool calls as a single block.
func formatEditGroup(edits []editItem, projectDir string) string {
	if len(edits) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## 🤖 Assistant\n\n")

	for _, edit := range edits {
		icon := "✅"
		if edit.IsError {
			icon = "❌"
		}

		displayPath := stripProjectDir(edit.FilePath, projectDir)
		sb.WriteString("<details>\n")
		sb.WriteString(fmt.Sprintf("<summary>%s 🔧 Edit: <code>%s</code></summary>\n\n",
			icon, html.EscapeString(displayPath)))

		if len(edit.Patch) > 0 {
			sb.WriteString("```patch\n")
			for _, hunk := range edit.Patch {
				for _, line := range hunk.Lines {
					sb.WriteString(line)
					sb.WriteString("\n")
				}
			}
			sb.WriteString("```\n\n")
		}

		sb.WriteString("</details>\n\n")
	}

	sb.WriteString("---\n\n")
	return sb.String()
}

// formatUserMessage formats a user message as Markdown.
func formatUserMessage(entry *Entry, toolMeta map[string]toolMetadata, skillDescriptions map[string]string) string {
	if entry.Message == nil || len(entry.Message.Content) == 0 {
		return ""
	}

	// Skip skill/command description meta entries - they're rendered with the Skill/command block
	if entry.IsMeta {
		return ""
	}

	// Check if this is a slash command entry (e.g., /catchup)
	if isSlashCommandEntry(entry) {
		return formatSlashCommand(entry, skillDescriptions)
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

// formatSlashCommand formats a slash command entry as Markdown.
func formatSlashCommand(entry *Entry, skillDescriptions map[string]string) string {
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
	sb.WriteString("## 👤 User\n\n")

	if description != "" {
		// Render as collapsible with description
		sb.WriteString("<details>\n")
		sb.WriteString(fmt.Sprintf("<summary>⚡ /%s</summary>\n\n", commandName))
		sb.WriteString(description)
		sb.WriteString("\n\n</details>\n\n")
	} else {
		// Simple format without description
		sb.WriteString(fmt.Sprintf("⚡ `/%s`\n\n", commandName))
	}

	sb.WriteString("---\n\n")

	return sb.String()
}

// formatAssistantMessage formats an assistant message as Markdown.
func formatAssistantMessage(entry *Entry, toolMeta map[string]toolMetadata, skillDescriptions map[string]string) string {
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
			sb.WriteString(formatToolUse(&item, toolMeta, skillDescriptions))

		case "tool_result":
			// tool_result in assistant entries (legacy handling)
			sb.WriteString(formatToolResult(&item, toolMeta))

		// Unknown content types are skipped silently per requirement 4.8
		}
	}

	sb.WriteString("---\n\n")
	return sb.String()
}

// formatToolUse formats a tool_use content item.
// For Skill: renders collapsible block with skill description (if available) or simple line.
// For non-subagent Task: renders collapsed details block with JSON.
// For subagent Task and other tools: stores metadata and defers rendering to formatToolResult.
func formatToolUse(item *ContentItem, toolMeta map[string]toolMetadata, skillDescriptions map[string]string) string {
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

	// For subagent Task, defer rendering to formatToolResult
	if subagent {
		return ""
	}

	// For Skill, render with description if available
	if item.Name == "Skill" {
		if skillDesc != "" {
			var sb strings.Builder
			sb.WriteString("<details>\n")
			sb.WriteString(fmt.Sprintf("<summary>🔧 %s</summary>\n\n", escapeSummary(summary)))
			sb.WriteString(skillDesc)
			sb.WriteString("\n\n</details>\n\n")
			return sb.String()
		}
		// No description, render simple line
		return fmt.Sprintf("🔧 %s\n\n", escapeSummary(summary))
	}

	// For non-subagent Task, render collapsed block now
	if item.Name == "Task" {
		var sb strings.Builder
		var inputJSON []byte
		if item.Input != nil {
			inputJSON, _ = json.MarshalIndent(item.Input, "", "  ")
		}

		sb.WriteString("<details>\n")
		sb.WriteString(fmt.Sprintf("<summary>🔧 %s</summary>\n\n", escapeSummary(summary)))
		if len(inputJSON) > 0 {
			sb.WriteString("```json\n")
			sb.WriteString(string(inputJSON))
			sb.WriteString("\n```\n\n")
		}
		sb.WriteString("</details>\n\n")
		return sb.String()
	}

	// For other tools, don't render here - will be rendered with tool_result
	return ""
}

// formatToolResult formats a tool_result content item.
// For subagent Task: renders combined Prompt/Result with 🤖 emoji.
// For Skill: skips rendering (already shown in tool_use, result is just "Launching skill: X").
// For non-subagent Task: renders just the result in a collapsed block.
// For other tools: renders the combined tool call + result in a single details block.
func formatToolResult(item *ContentItem, toolMeta map[string]toolMetadata) string {
	var sb strings.Builder

	content := item.Content

	// Determine icon
	icon := "✅"
	if item.IsError {
		icon = "❌"
	}

	// Look up tool metadata
	meta, found := toolMeta[item.ToolUseID]

	// For subagent Task, render combined Prompt/Result with robot emoji
	if found && meta.IsSubagent {
		sb.WriteString("<details>\n")
		sb.WriteString(fmt.Sprintf("<summary>%s 🤖🔧 %s</summary>\n\n", icon, escapeSummary(meta.Summary)))

		// Render prompt
		if meta.Prompt != "" {
			sb.WriteString("**Prompt:**\n")
			sb.WriteString(meta.Prompt)
			sb.WriteString("\n\n")
		}

		// Render result - extract text from JSON array and render as markdown
		resultText := extractSubagentResultText(content)
		sb.WriteString("**Result:**\n\n")
		sb.WriteString(resultText)
		sb.WriteString("\n\n")
		sb.WriteString("</details>\n\n")
		return sb.String()
	}

	// For Skill, skip rendering the result (already shown in tool_use, result is just "Launching skill: X")
	if found && meta.Name == "Skill" {
		return ""
	}

	// For non-subagent Task, render just the result
	if found && meta.Name == "Task" {
		sb.WriteString("<details>\n")
		sb.WriteString(fmt.Sprintf("<summary>%s %s</summary>\n\n", icon, escapeSummary(meta.Summary)))
		sb.WriteString("```\n")
		sb.WriteString(content)
		sb.WriteString("\n```\n\n")
		sb.WriteString("</details>\n\n")
		return sb.String()
	}

	// For other tools, render combined tool call + result
	if found {
		// Build summary: icon + tool name + description
		summaryText := fmt.Sprintf("🔧 %s", meta.Name)
		if meta.Description != "" {
			summaryText += ": " + meta.Description
		}

		// Determine if this tool should be expanded by default
		openAttr := ""
		if meta.Name == "TodoWrite" {
			openAttr = " open"
		}

		sb.WriteString(fmt.Sprintf("<details%s>\n", openAttr))
		sb.WriteString(fmt.Sprintf("<summary>%s %s</summary>\n\n", icon, escapeSummary(summaryText)))

		// Render tool-specific input
		sb.WriteString(formatToolInput(meta.Name, meta.Input))

		// Render result
		sb.WriteString("**Result:**\n```\n")
		sb.WriteString(content)
		sb.WriteString("\n```\n\n")
		sb.WriteString("</details>\n\n")
	} else {
		// Unmatched result: render standalone
		sb.WriteString("<details>\n")
		if item.IsError {
			sb.WriteString("<summary>❌ Tool Error</summary>\n\n")
		} else {
			sb.WriteString("<summary>✅ Tool Result</summary>\n\n")
		}
		sb.WriteString("```\n")
		sb.WriteString(content)
		sb.WriteString("\n```\n\n")
		sb.WriteString("</details>\n\n")
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

// getToolDescription extracts the description field from tool input.
func getToolDescription(input any) string {
	inputMap, ok := input.(map[string]any)
	if !ok {
		return ""
	}
	desc, _ := inputMap["description"].(string)
	return desc
}

// isSubagent checks if a Task tool input has a subagent_type field.
func isSubagent(input any) bool {
	inputMap, ok := input.(map[string]any)
	if !ok {
		return false
	}
	subType, ok := inputMap["subagent_type"].(string)
	return ok && subType != ""
}

// getSubagentPrompt extracts the prompt field from a Task tool input.
func getSubagentPrompt(input any) string {
	inputMap, ok := input.(map[string]any)
	if !ok {
		return ""
	}
	prompt, _ := inputMap["prompt"].(string)
	return prompt
}

// extractSubagentResultText parses subagent result content and extracts text.
// Subagent results come as JSON array: [{"text":"..."},{"text":"..."}]
// This function extracts and concatenates all text fields.
func extractSubagentResultText(content string) string {
	// Try to parse as JSON array
	var items []map[string]any
	if err := json.Unmarshal([]byte(content), &items); err != nil {
		// Not valid JSON array, return as-is
		return content
	}

	// Extract text fields from each item
	var texts []string
	for _, item := range items {
		if text, ok := item["text"].(string); ok && text != "" {
			texts = append(texts, text)
		}
	}

	if len(texts) == 0 {
		// No text fields found, return original
		return content
	}

	return strings.Join(texts, "\n\n")
}

// formatToolInput formats tool-specific input for display in the combined block.
func formatToolInput(name string, input any) string {
	inputMap, ok := input.(map[string]any)
	if !ok {
		return ""
	}

	var sb strings.Builder

	switch name {
	case "Bash":
		if cmd, ok := inputMap["command"].(string); ok {
			sb.WriteString("**Command:**\n```bash\n")
			sb.WriteString(cmd)
			sb.WriteString("\n```\n\n")
		}
	case "Write":
		if path, ok := inputMap["file_path"].(string); ok {
			sb.WriteString(fmt.Sprintf("**File:** `%s`\n\n", path))
		}
	case "Edit":
		if path, ok := inputMap["file_path"].(string); ok {
			sb.WriteString(fmt.Sprintf("**File:** `%s`\n\n", path))
		}
	case "Glob":
		if pattern, ok := inputMap["pattern"].(string); ok {
			sb.WriteString(fmt.Sprintf("**Pattern:** `%s`\n\n", pattern))
		}
	case "Grep":
		if pattern, ok := inputMap["pattern"].(string); ok {
			sb.WriteString(fmt.Sprintf("**Pattern:** `%s`\n\n", pattern))
		}
	case "TodoWrite":
		if todos, ok := inputMap["todos"].([]any); ok {
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
					sb.WriteString(fmt.Sprintf("- %s %s\n", checkbox, content))
				}
			}
			sb.WriteString("\n")
		}
	default:
		// For unknown tools, show JSON input
		if input != nil {
			inputJSON, err := json.MarshalIndent(input, "", "  ")
			if err == nil {
				sb.WriteString("**Input:**\n```json\n")
				sb.WriteString(string(inputJSON))
				sb.WriteString("\n```\n\n")
			}
		}
	}

	return sb.String()
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
