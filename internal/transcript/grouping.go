package transcript

import (
	"regexp"
	"strings"
)

// slashCommandPattern matches slash command format in user entries.
// Example: <command-message>catchup</command-message>\n<command-name>/catchup</command-name>
var slashCommandPattern = regexp.MustCompile(`<command-message>.*?</command-message>\s*<command-name>/([^<]+)</command-name>`)

// parseSlashCommand extracts the command name from a user message if it matches
// the slash command pattern. Returns empty string if not a slash command.
func parseSlashCommand(text string) string {
	matches := slashCommandPattern.FindStringSubmatch(text)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// isSlashCommandEntry checks if a user entry is purely a slash command invocation.
func isSlashCommandEntry(entry *Entry) bool {
	if entry.Type != "user" || entry.Message == nil {
		return false
	}

	// Check string content (older format)
	if len(entry.Message.Content) == 0 {
		return false
	}

	// Check if there's a single text item that matches the slash command pattern
	for _, item := range entry.Message.Content {
		if item.Type == "text" && parseSlashCommand(item.Text) != "" {
			return true
		}
	}
	return false
}

// buildSkillDescriptionMap scans entries for meta entries containing skill/command descriptions
// and returns a map from sourceToolUseID (for Skill tools) or parentUUID (for slash commands)
// to the description text.
func buildSkillDescriptionMap(entries []Entry) map[string]string {
	result := make(map[string]string)

	// First pass: identify slash command entry UUIDs
	slashCommandUUIDs := make(map[string]bool)
	for i := range entries {
		entry := &entries[i]
		if isSlashCommandEntry(entry) && entry.UUID != "" {
			slashCommandUUIDs[entry.UUID] = true
		}
	}

	// Second pass: collect descriptions from meta entries
	for i := range entries {
		entry := &entries[i]
		if !entry.IsMeta || entry.Message == nil {
			continue
		}

		// Extract text content as description
		var descText string
		for _, item := range entry.Message.Content {
			if item.Type == "text" && item.Text != "" {
				descText = item.Text
				break
			}
		}
		if descText == "" {
			continue
		}

		// Link via sourceToolUseID (Skill tools)
		if entry.SourceToolUseID != "" {
			result[entry.SourceToolUseID] = descText
		}

		// Link via parentUUID (slash commands) - meta entry's parent is the slash command
		if entry.ParentUUID != "" && slashCommandUUIDs[entry.ParentUUID] {
			result[entry.ParentUUID] = descText
		}
	}

	return result
}

// stripProjectDir removes the project directory prefix from a file path.
// If the path doesn't start with the project directory, it's returned unchanged.
func stripProjectDir(filePath, projectDir string) string {
	if projectDir == "" {
		return filePath
	}
	// Ensure projectDir ends with / for proper prefix matching
	prefix := projectDir
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	if strings.HasPrefix(filePath, prefix) {
		return strings.TrimPrefix(filePath, prefix)
	}
	return filePath
}

// renderGroup represents a group of related entries for rendering.
// This enables grouping consecutive Read/Edit tool calls into a single block.
type renderGroup struct {
	Type    string       // "user", "assistant", "read_group", or "edit_group"
	Entries []Entry      // Original entries (for user/assistant)
	Reads   []readItem   // Grouped Read calls (for read_group)
	Edits   []editItem   // Grouped Edit calls (for edit_group)
}

// readItem represents a single Read tool call with its result.
type readItem struct {
	FilePath string // The file path being read
	Content  string // The file contents (from tool_result)
	IsError  bool   // Whether the read failed
	ToolID   string // Tool use ID for tracking
}

// editItem represents a single Edit tool call with its result.
type editItem struct {
	FilePath string      // The file path being edited
	Patch    []PatchHunk // The structured patch from tool result
	IsError  bool        // Whether the edit failed
	ToolID   string      // Tool use ID for tracking
}

// toolResultInfo stores tool_result information for matching.
type toolResultInfo struct {
	Content       string
	IsError       bool
	ToolUseResult *ToolUseResult // For Edit tool, contains structuredPatch
}

// preprocessEntries groups consecutive Read/Edit tool calls and matches them with results.
// Returns render groups that can be efficiently rendered.
func preprocessEntries(entries []Entry) []renderGroup {
	if len(entries) == 0 {
		return nil
	}

	// First pass: build map of tool_use_id -> tool_result
	// Also capture ToolUseResult from entries for Edit tool
	resultMap := make(map[string]toolResultInfo)
	for i := range entries {
		entry := &entries[i]
		if entry.Message == nil {
			continue
		}
		for _, item := range entry.Message.Content {
			if item.Type == "tool_result" && item.ToolUseID != "" {
				resultMap[item.ToolUseID] = toolResultInfo{
					Content:       item.Content,
					IsError:       item.IsError,
					ToolUseResult: entry.ToolUseResult,
				}
			}
		}
	}

	// Second pass: group entries
	var groups []renderGroup
	var currentReadGroup []readItem
	var currentEditGroup []editItem
	usedResultIDs := make(map[string]bool)

	flushReadGroup := func() {
		if len(currentReadGroup) > 0 {
			groups = append(groups, renderGroup{
				Type:  "read_group",
				Reads: currentReadGroup,
			})
			currentReadGroup = nil
		}
	}

	flushEditGroup := func() {
		if len(currentEditGroup) > 0 {
			groups = append(groups, renderGroup{
				Type:  "edit_group",
				Edits: currentEditGroup,
			})
			currentEditGroup = nil
		}
	}

	flushAllGroups := func() {
		flushReadGroup()
		flushEditGroup()
	}

	for i := range entries {
		entry := &entries[i]

		// Check if this is a user entry with only tool_results that were already used
		// This must be checked FIRST, before flushing groups, to keep consecutive
		// Read/Edit tool calls grouped together
		if entry.Type == "user" && isUsedToolResultOnlyEntry(entry, usedResultIDs) {
			// Skip this entry - its results were already rendered with the group
			continue
		}

		// Check if this is an assistant entry with only Read tool_use(s)
		if entry.Type == "assistant" && isReadOnlyEntry(entry) {
			flushEditGroup() // Flush edit group when switching to read
			readItems := extractReadItems(entry, resultMap, usedResultIDs)
			if len(readItems) > 0 {
				currentReadGroup = append(currentReadGroup, readItems...)
				continue
			}
		}

		// Check if this is an assistant entry with only Edit tool_use(s)
		if entry.Type == "assistant" && isEditOnlyEntry(entry) {
			flushReadGroup() // Flush read group when switching to edit
			editItems := extractEditItems(entry, resultMap, usedResultIDs)
			if len(editItems) > 0 {
				currentEditGroup = append(currentEditGroup, editItems...)
				continue
			}
		}

		// Not a Read/Edit-only entry, flush any pending groups
		flushAllGroups()

		// Add as regular entry
		groups = append(groups, renderGroup{
			Type:    entry.Type,
			Entries: []Entry{*entry},
		})
	}

	// Flush any remaining groups
	flushAllGroups()

	return groups
}

// isReadOnlyEntry checks if an assistant entry contains only Read tool_use(s).
func isReadOnlyEntry(entry *Entry) bool {
	if entry.Message == nil || len(entry.Message.Content) == 0 {
		return false
	}

	for _, item := range entry.Message.Content {
		switch item.Type {
		case "tool_use":
			if item.Name != "Read" {
				return false
			}
		case "thinking":
			// Allow thinking blocks alongside Read
			continue
		case "text":
			// If there's meaningful text, don't group
			if item.Text != "" {
				return false
			}
		default:
			return false
		}
	}

	// Must have at least one Read tool_use
	for _, item := range entry.Message.Content {
		if item.Type == "tool_use" && item.Name == "Read" {
			return true
		}
	}
	return false
}

// extractReadItems extracts Read tool information from an entry.
func extractReadItems(entry *Entry, resultMap map[string]toolResultInfo, usedIDs map[string]bool) []readItem {
	var items []readItem

	if entry.Message == nil {
		return items
	}

	for _, item := range entry.Message.Content {
		if item.Type == "tool_use" && item.Name == "Read" {
			filePath := extractFilePath(item.Input)
			readItem := readItem{
				FilePath: filePath,
				ToolID:   item.ID,
			}

			// Look up the result
			if result, found := resultMap[item.ID]; found {
				readItem.Content = result.Content
				readItem.IsError = result.IsError
				usedIDs[item.ID] = true
			}

			items = append(items, readItem)
		}
	}

	return items
}

// extractFilePath extracts the file_path from a Read/Edit tool's input.
func extractFilePath(input any) string {
	inputMap, ok := input.(map[string]any)
	if !ok {
		return ""
	}
	filePath, _ := inputMap["file_path"].(string)
	return filePath
}

// isEditOnlyEntry checks if an assistant entry contains only Edit tool_use(s).
func isEditOnlyEntry(entry *Entry) bool {
	if entry.Message == nil || len(entry.Message.Content) == 0 {
		return false
	}

	for _, item := range entry.Message.Content {
		switch item.Type {
		case "tool_use":
			if item.Name != "Edit" {
				return false
			}
		case "thinking":
			// Allow thinking blocks alongside Edit
			continue
		case "text":
			// If there's meaningful text, don't group
			if item.Text != "" {
				return false
			}
		default:
			return false
		}
	}

	// Must have at least one Edit tool_use
	for _, item := range entry.Message.Content {
		if item.Type == "tool_use" && item.Name == "Edit" {
			return true
		}
	}
	return false
}

// extractEditItems extracts Edit tool information from an entry.
func extractEditItems(entry *Entry, resultMap map[string]toolResultInfo, usedIDs map[string]bool) []editItem {
	var items []editItem

	if entry.Message == nil {
		return items
	}

	for _, item := range entry.Message.Content {
		if item.Type == "tool_use" && item.Name == "Edit" {
			filePath := extractFilePath(item.Input)
			edit := editItem{
				FilePath: filePath,
				ToolID:   item.ID,
			}

			// Look up the result
			if result, found := resultMap[item.ID]; found {
				edit.IsError = result.IsError
				if result.ToolUseResult != nil {
					edit.Patch = result.ToolUseResult.StructuredPatch
				}
				usedIDs[item.ID] = true
			}

			items = append(items, edit)
		}
	}

	return items
}

// isUsedToolResultOnlyEntry checks if a user entry contains only tool_results
// that have already been used (rendered with their Read/Edit groups).
func isUsedToolResultOnlyEntry(entry *Entry, usedIDs map[string]bool) bool {
	if entry.Message == nil || len(entry.Message.Content) == 0 {
		return false
	}

	hasToolResult := false
	for _, item := range entry.Message.Content {
		switch item.Type {
		case "tool_result":
			if !usedIDs[item.ToolUseID] {
				// This result hasn't been used, can't skip
				return false
			}
			hasToolResult = true
		case "text":
			if item.Text != "" {
				// Has text content, don't skip
				return false
			}
		default:
			// Other content types, don't skip
			return false
		}
	}

	return hasToolResult
}
