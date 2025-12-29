package transcript

// renderGroup represents a group of related entries for rendering.
// This enables grouping consecutive Read tool calls into a single block.
type renderGroup struct {
	Type    string       // "user", "assistant", or "read_group"
	Entries []Entry      // Original entries (for user/assistant)
	Reads   []readItem   // Grouped Read calls (for read_group)
}

// readItem represents a single Read tool call with its result.
type readItem struct {
	FilePath string // The file path being read
	Content  string // The file contents (from tool_result)
	IsError  bool   // Whether the read failed
	ToolID   string // Tool use ID for tracking
}

// toolResultInfo stores tool_result information for matching.
type toolResultInfo struct {
	Content string
	IsError bool
}

// preprocessEntries groups consecutive Read tool calls and matches them with results.
// Returns render groups that can be efficiently rendered.
func preprocessEntries(entries []Entry) []renderGroup {
	if len(entries) == 0 {
		return nil
	}

	// First pass: build map of tool_use_id -> tool_result
	resultMap := make(map[string]toolResultInfo)
	for _, entry := range entries {
		if entry.Message == nil {
			continue
		}
		for _, item := range entry.Message.Content {
			if item.Type == "tool_result" && item.ToolUseID != "" {
				resultMap[item.ToolUseID] = toolResultInfo{
					Content: item.Content,
					IsError: item.IsError,
				}
			}
		}
	}

	// Second pass: group entries
	var groups []renderGroup
	var currentReadGroup []readItem
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

	for i := range entries {
		entry := &entries[i]

		// Check if this is an assistant entry with only a Read tool_use
		if entry.Type == "assistant" && isReadOnlyEntry(entry) {
			readItems := extractReadItems(entry, resultMap, usedResultIDs)
			if len(readItems) > 0 {
				currentReadGroup = append(currentReadGroup, readItems...)
				continue
			}
		}

		// Not a Read-only entry, flush any pending read group
		flushReadGroup()

		// Check if this is a user entry with only tool_results that were already used
		if entry.Type == "user" && isUsedToolResultOnlyEntry(entry, usedResultIDs) {
			// Skip this entry - its results were already rendered with the Read group
			continue
		}

		// Add as regular entry
		groups = append(groups, renderGroup{
			Type:    entry.Type,
			Entries: []Entry{*entry},
		})
	}

	// Flush any remaining read group
	flushReadGroup()

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

// extractFilePath extracts the file_path from a Read tool's input.
func extractFilePath(input any) string {
	inputMap, ok := input.(map[string]any)
	if !ok {
		return ""
	}
	filePath, _ := inputMap["file_path"].(string)
	return filePath
}

// isUsedToolResultOnlyEntry checks if a user entry contains only tool_results
// that have already been used (rendered with their Read calls).
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
