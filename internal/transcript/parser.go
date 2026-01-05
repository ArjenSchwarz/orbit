package transcript

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

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
