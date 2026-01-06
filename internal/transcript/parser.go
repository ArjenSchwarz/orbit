package transcript

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// utf8BOM is the byte sequence for UTF-8 Byte Order Mark.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// claudeTypes are the type values that indicate Claude Code format.
var claudeTypes = map[string]bool{
	"user":      true,
	"assistant": true,
}

// codexTypes are the type values that indicate Codex format.
var codexTypes = map[string]bool{
	"session_meta":  true,
	"response_item": true,
	"event_msg":     true,
	"turn_context":  true,
}

// DetectFormat examines the first non-empty line to determine the log format.
// Returns the detected format, the first line bytes (with BOM stripped), and any error.
// The first line is returned to allow reuse without re-reading.
func DetectFormat(r io.Reader) (Format, []byte, error) {
	firstLine, err := readFirstNonEmptyLine(r)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return FormatUnknown, nil, fmt.Errorf("empty file")
		}
		return FormatUnknown, nil, err
	}

	// Strip BOM if present
	firstLine = stripBOM(firstLine)

	// Detect format from line content
	format, err := detectFormatFromLine(firstLine)
	if err != nil {
		return FormatUnknown, nil, err
	}

	return format, firstLine, nil
}

// readFirstNonEmptyLine reads lines until finding a non-empty line.
// Returns io.EOF if no non-empty line is found.
func readFirstNonEmptyLine(r io.Reader) ([]byte, error) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) > 0 {
			// Return a copy since scanner reuses the buffer
			result := make([]byte, len(line))
			copy(result, line)
			return result, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

// stripBOM removes UTF-8 BOM prefix if present.
func stripBOM(data []byte) []byte {
	if bytes.HasPrefix(data, utf8BOM) {
		return data[len(utf8BOM):]
	}
	return data
}

// detectFormatFromLine determines the format from a single JSON line.
func detectFormatFromLine(line []byte) (Format, error) {
	var obj struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(line, &obj); err != nil {
		return FormatUnknown, fmt.Errorf("failed to parse first line as JSON: %w", err)
	}

	if claudeTypes[obj.Type] {
		return FormatClaude, nil
	}
	if codexTypes[obj.Type] {
		return FormatCodex, nil
	}

	return FormatUnknown, fmt.Errorf("unrecognized log format: type field value '%s'", obj.Type)
}

// ParseResult contains the parsed entries and any warnings encountered.
type ParseResult struct {
	Entries  []Entry
	Warnings []ParseWarning
}

// ParseWarning represents a non-fatal parsing issue.
type ParseWarning struct {
	Line    int
	Message string
}

// ParseJSONL reads JSONL from the provided reader and returns parsed entries.
// It uses a 64KB initial buffer with a 10MB maximum per line.
// Malformed lines and unknown entry types are skipped with warnings.
func ParseJSONL(r io.Reader) (*ParseResult, error) {
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)   // 64KB initial
	scanner.Buffer(buf, 10*1024*1024) // 10MB max

	result := &ParseResult{
		Entries:  []Entry{},
		Warnings: []ParseWarning{},
	}
	lineNum := 0

	// Track UUIDs of filtered entries for context-aware local command filtering
	filteredUUIDs := make(map[string]bool)

	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			result.Warnings = append(result.Warnings, ParseWarning{
				Line:    lineNum,
				Message: fmt.Sprintf("failed to parse JSON: %v", err),
			})
			continue
		}

		// Only process known entry types (user, assistant)
		// Skip unknown types silently per requirement 4.7
		if entry.Type != "user" && entry.Type != "assistant" {
			continue
		}

		// Skip local command sequences using UUID tracking
		if shouldFilterLocalCommand(&entry, filteredUUIDs) {
			continue
		}

		result.Entries = append(result.Entries, entry)
	}

	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			result.Warnings = append(result.Warnings, ParseWarning{
				Line:    lineNum + 1,
				Message: "line exceeds 10MB buffer limit",
			})
		} else {
			return nil, fmt.Errorf("scanner error: %w", err)
		}
	}

	return result, nil
}

// ParseFirstTimestamp reads only the first entry's timestamp from JSONL.
// Used for efficient session listing without parsing the entire file.
// If the timestamp cannot be parsed, returns the zero time and an error.
func ParseFirstTimestamp(r io.Reader) (time.Time, error) {
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue // Try next line
		}

		if entry.Timestamp == "" {
			continue // Try next line
		}

		// Parse timestamp - Claude uses RFC3339 format
		t, err := time.Parse(time.RFC3339, entry.Timestamp)
		if err != nil {
			// Try RFC3339Nano as fallback
			t, err = time.Parse(time.RFC3339Nano, entry.Timestamp)
			if err != nil {
				return time.Time{}, fmt.Errorf("failed to parse timestamp %q: %w", entry.Timestamp, err)
			}
		}
		return t, nil
	}

	if err := scanner.Err(); err != nil {
		return time.Time{}, fmt.Errorf("scanner error: %w", err)
	}

	return time.Time{}, fmt.Errorf("no timestamp found in file")
}

// shouldFilterLocalCommand determines if an entry should be filtered as part of
// a local command sequence. It uses UUID tracking to filter:
// 1. Meta entries (isMeta: true) - the "Caveat" warning messages (but NOT skill/command descriptions)
// 2. Command entries whose parent is a filtered meta entry
// 3. Local command stdout entries whose parent is a filtered command entry
//
// The filteredUUIDs map is updated in place to track filtered entries.
func shouldFilterLocalCommand(entry *Entry, filteredUUIDs map[string]bool) bool {
	// Filter meta entries (internal Claude markers with "Caveat" warnings)
	// BUT keep meta entries that are skill/command descriptions:
	// - Skill descriptions have sourceToolUseID
	// - Command descriptions have text content that doesn't start with "Caveat:"
	if entry.IsMeta {
		// Keep skill description meta entries (they have sourceToolUseID)
		if entry.SourceToolUseID != "" {
			return false
		}
		// Keep command description meta entries (text content without "Caveat:" marker)
		// Caveat entries are local command warnings that should be filtered
		if hasNonCaveatTextContent(entry) {
			return false
		}
		if entry.UUID != "" {
			filteredUUIDs[entry.UUID] = true
		}
		return true
	}

	// Check if this entry's parent was filtered
	parentFiltered := entry.ParentUUID != "" && filteredUUIDs[entry.ParentUUID]

	// Filter command entries that follow a meta entry
	if parentFiltered && hasCommandNameContent(entry) {
		if entry.UUID != "" {
			filteredUUIDs[entry.UUID] = true
		}
		return true
	}

	// Filter local-command-stdout entries that follow a filtered command entry
	if parentFiltered && hasLocalCommandStdoutContent(entry) {
		return true
	}

	return false
}

// hasCommandNameContent checks if an entry contains <command-name> XML tags.
func hasCommandNameContent(entry *Entry) bool {
	if entry.Message == nil || len(entry.Message.Content) == 0 {
		return false
	}

	for _, item := range entry.Message.Content {
		if item.Type == "text" && strings.Contains(item.Text, "<command-name>") {
			return true
		}
	}
	return false
}

// hasLocalCommandStdoutContent checks if an entry contains <local-command-stdout> XML tags.
func hasLocalCommandStdoutContent(entry *Entry) bool {
	if entry.Message == nil || len(entry.Message.Content) == 0 {
		return false
	}

	for _, item := range entry.Message.Content {
		if item.Type == "text" && strings.Contains(item.Text, "<local-command-stdout>") {
			return true
		}
	}
	return false
}

// hasNonCaveatTextContent checks if an entry has text content that isn't a "Caveat:" warning.
// Used to identify command descriptions (which should be kept) vs local command warnings (which should be filtered).
func hasNonCaveatTextContent(entry *Entry) bool {
	if entry.Message == nil || len(entry.Message.Content) == 0 {
		return false
	}

	for _, item := range entry.Message.Content {
		if item.Type == "text" && item.Text != "" {
			// Filter entries that contain "Caveat:" - these are local command warnings
			if strings.Contains(item.Text, "Caveat:") {
				return false
			}
			return true
		}
	}
	return false
}
