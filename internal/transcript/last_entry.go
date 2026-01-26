package transcript

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

const (
	initialChunkSize    = 64 * 1024        // 64KB initial read
	maxChunkSize        = 4 * 1024 * 1024  // 4MB max before fallback
	maxFullFileFallback = 32 * 1024 * 1024 // 32MB - read full file if smaller
	chunkGrowFactor     = 2                // Double each iteration
)

// parameterPriority defines the order for extracting key_input from tool parameters.
// Per requirement 3.6: file_path, path, command, pattern, query, url, prompt
var parameterPriority = []string{"file_path", "path", "command", "pattern", "query", "url", "prompt"}

// GetLastDisplayableEntry reads the transcript file from the end and returns
// the most recent displayable entry (assistant message with tool_use or text).
// Uses an expanding search window to handle large entries.
// Returns nil, nil if no displayable entry exists (file empty or only system messages).
// Returns nil, error for actual read/parse errors.
func GetLastDisplayableEntry(filePath string) (*Entry, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	chunkSize := int64(initialChunkSize)

	for {
		// Re-stat file each iteration (file may be growing as agent writes)
		stat, err := f.Stat()
		if err != nil {
			return nil, err
		}
		fileSize := stat.Size()
		if fileSize == 0 {
			return nil, nil // Empty file
		}

		// Check if we've exceeded max chunk and need to fallback to full file
		if chunkSize > int64(maxChunkSize) {
			if fileSize <= int64(maxFullFileFallback) {
				chunkSize = fileSize // Read entire file as last resort
			} else {
				// File is too large to read entirely, give up
				return nil, nil
			}
		}

		if chunkSize > fileSize {
			chunkSize = fileSize
		}

		offset := fileSize - chunkSize
		if offset < 0 {
			offset = 0
		}

		buf := make([]byte, chunkSize)
		n, err := f.ReadAt(buf, offset)
		if err != nil && err != io.EOF {
			return nil, err
		}
		buf = buf[:n]

		// Find complete JSON lines by looking for newlines
		// If we're not at the start, skip partial first line
		startIdx := 0
		if offset > 0 {
			idx := bytes.IndexByte(buf, '\n')
			if idx >= 0 {
				startIdx = idx + 1
			} else {
				// No newline found - entire chunk is a partial line
				// Grow window and retry without trying to parse
				chunkSize *= chunkGrowFactor
				continue
			}
		}

		// Split into lines and process from end
		lines := bytes.Split(buf[startIdx:], []byte("\n"))

		for i := len(lines) - 1; i >= 0; i-- {
			line := bytes.TrimSpace(lines[i])
			if len(line) == 0 {
				continue
			}

			var entry Entry
			if err := json.Unmarshal(line, &entry); err != nil {
				// Skip malformed lines (may be incomplete at chunk boundary
				// or actively being written)
				continue
			}

			if isDisplayableEntry(&entry) {
				return &entry, nil
			}
		}

		// If we've read the entire file and found nothing, return nil
		if offset == 0 {
			return nil, nil
		}

		// Expand search window
		chunkSize *= chunkGrowFactor
	}
}

// isDisplayableEntry checks if an entry should be considered for "last action" display.
// Returns true for assistant messages with tool_use or text content.
// Excludes meta entries and thinking content.
func isDisplayableEntry(e *Entry) bool {
	if e.IsMeta {
		return false
	}
	if e.Message == nil || e.Message.Role != "assistant" {
		return false
	}
	for _, c := range e.Message.Content {
		if c.Type == "tool_use" || c.Type == "text" {
			return true
		}
	}
	return false
}

// FormatToolUse formats a tool_use content item for display.
// Returns format: "{ToolName}: {key_input}" with truncation to 60 chars.
func FormatToolUse(name string, input any) string {
	keyInput := extractKeyInput(input)
	if keyInput == "" {
		return name
	}
	// Truncate to 60 characters per requirement 3.6
	if len(keyInput) > 60 {
		keyInput = keyInput[:57] + "..."
	}
	return fmt.Sprintf("%s: %s", name, keyInput)
}

// extractKeyInput extracts the most relevant parameter value from tool input.
// Tries parameters in priority order, then falls back to first string parameter.
func extractKeyInput(input any) string {
	inputMap, ok := input.(map[string]any)
	if !ok {
		return ""
	}

	// Try parameters in priority order
	for _, key := range parameterPriority {
		if val, exists := inputMap[key]; exists {
			if str, ok := val.(string); ok && str != "" {
				return str
			}
		}
	}

	// Fall back to first parameter value
	for _, val := range inputMap {
		if str, ok := val.(string); ok && str != "" {
			return str
		}
	}

	return ""
}

// FormatLastAction formats an entry as a last action summary.
// Per requirement 3.5: prioritizes tool_use over text when both present.
func FormatLastAction(entry *Entry) string {
	if entry == nil || entry.Message == nil {
		return ""
	}

	// First pass: look for tool_use (higher priority per req 3.5)
	for _, c := range entry.Message.Content {
		if c.Type == "tool_use" {
			return FormatToolUse(c.Name, c.Input)
		}
	}

	// Second pass: fall back to text
	for _, c := range entry.Message.Content {
		if c.Type == "text" && c.Text != "" {
			text := c.Text
			// Truncate to 80 chars per requirement 3.7
			if len(text) > 80 {
				return text[:77] + "..."
			}
			return text
		}
	}

	return ""
}
