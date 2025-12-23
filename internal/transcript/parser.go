package transcript

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	buf := make([]byte, 0, 64*1024)       // 64KB initial
	scanner.Buffer(buf, 10*1024*1024)     // 10MB max

	result := &ParseResult{
		Entries:  []Entry{},
		Warnings: []ParseWarning{},
	}
	lineNum := 0

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
