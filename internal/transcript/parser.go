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

// copilotTypes are the type values that indicate Copilot format.
// Copilot uses dot-notation type fields like "session.start", "user.message", etc.
var copilotTypes = map[string]bool{
	"session.start":           true,
	"session.info":            true,
	"session.model_change":    true,
	"user.message":            true,
	"assistant.turn_start":    true,
	"assistant.message":       true,
	"assistant.reasoning":     true,
	"assistant.turn_end":      true,
	"tool.execution_start":    true,
	"tool.execution_complete": true,
	"skill.invoked":           true,
	"abort":                   true,
	"function":                true, // Legacy event type
}

// infrastructureTypes are entry types that should be skipped during format detection.
// These are internal Claude infrastructure entries, not part of the conversation.
var infrastructureTypes = map[string]bool{
	"queue-operation": true,
	"progress":        true,
}

// DetectFormat examines file content to determine the log format.
// Returns the detected format, the initial bytes read (with BOM stripped), and any error.
//
// Detection strategy:
// 1. Read a chunk of content (up to 64KB to handle long JSONL lines)
// 2. Try parsing as complete JSON with Kiro markers - if successful, it's Kiro format
// 3. Otherwise, treat as JSONL and detect based on first format-defining line
//
// Note: Cannot use first-byte check alone because both JSON and JSONL start with '{'
func DetectFormat(r io.Reader) (Format, []byte, error) {
	// Read up to 64KB for format detection (handles long JSONL lines)
	chunk := make([]byte, 65536)
	n, err := io.ReadFull(r, chunk)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return FormatUnknown, nil, err
	}
	if n == 0 {
		return FormatUnknown, nil, fmt.Errorf("empty file")
	}
	chunk = chunk[:n]

	// Strip BOM if present
	chunk = stripBOM(chunk)

	// First, try to detect Kiro format (plain JSON with specific structure)
	if format := detectKiroFormat(chunk); format != FormatUnknown {
		return format, chunk, nil
	}

	// Fall back to JSONL detection based on first line
	return detectJSONLFormat(chunk)
}

// detectKiroFormat checks if content is Kiro plain JSON format (CLI or IDE).
// Kiro CLI format is a single JSON object with conversation_id and non-empty history fields.
// Kiro IDE format is a single JSON object with executionId, chat array, and metadata fields.
func detectKiroFormat(data []byte) Format {
	// Kiro sessions can be large; we only need to check the beginning structure.
	// Look for the characteristic fields: conversation_id and history array start.
	var kiroCheck struct {
		ConversationID string `json:"conversation_id"`
		History        []any  `json:"history"`
	}

	// Try to parse enough to detect structure
	if err := json.Unmarshal(data, &kiroCheck); err != nil {
		// If we got a partial read (buffer too small), check if it looks like Kiro
		// by examining the beginning of the content for characteristic fields
		dataStr := string(data)
		if strings.Contains(dataStr, `"conversation_id"`) &&
			strings.Contains(dataStr, `"history"`) &&
			!strings.Contains(dataStr, "\n{") { // JSONL would have newline + brace
			// Likely Kiro but truncated - still return as Kiro since full file would parse
			return FormatKiro
		}
		// Check for truncated Kiro IDE format
		if strings.Contains(dataStr, `"executionId"`) &&
			strings.Contains(dataStr, `"chat"`) &&
			strings.Contains(dataStr, `"metadata"`) {
			return FormatKiroIDE
		}
		return FormatUnknown
	}

	// Must have both fields populated to be valid Kiro CLI format
	if kiroCheck.ConversationID != "" && len(kiroCheck.History) > 0 {
		return FormatKiro
	}

	// Check for Kiro IDE format: executionId + chat + metadata
	var ideCheck struct {
		ExecutionID string `json:"executionId"`
		Chat        []any  `json:"chat"`
		Metadata    any    `json:"metadata"`
	}
	if err := json.Unmarshal(data, &ideCheck); err == nil {
		if ideCheck.ExecutionID != "" && ideCheck.Chat != nil && ideCheck.Metadata != nil {
			return FormatKiroIDE
		}
	}

	return FormatUnknown
}

// detectJSONLFormat detects format from the first format-defining line of JSONL content.
// Skips infrastructure types (like queue-operation) to find the actual conversation format.
// Also skips lines that fail to parse (may be truncated due to chunk size).
func detectJSONLFormat(data []byte) (Format, []byte, error) {
	// Split into lines and find first format-defining line
	lines := bytes.SplitSeq(data, []byte("\n"))

	for line := range lines {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}

		var obj struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(trimmed, &obj); err != nil {
			// Skip lines that don't parse - they may be truncated
			// due to the chunk size limit. Keep looking for a valid line.
			continue
		}

		// Skip infrastructure types
		if infrastructureTypes[obj.Type] {
			continue
		}

		if claudeTypes[obj.Type] {
			return FormatClaude, data, nil
		}
		if codexTypes[obj.Type] {
			return FormatCodex, data, nil
		}
		if copilotTypes[obj.Type] {
			return FormatCopilot, data, nil
		}

		return FormatUnknown, data, fmt.Errorf("unrecognized log format: type field value '%s'", obj.Type)
	}

	return FormatUnknown, nil, fmt.Errorf("no format-defining entries found in file")
}

// readFirstNonEmptyLineFromBufReader reads lines until finding a non-empty line.
// Uses bufio.Reader directly to preserve reader position for subsequent reads.
// Returns io.EOF if no non-empty line is found.
func readFirstNonEmptyLineFromBufReader(r *bufio.Reader) ([]byte, error) {
	for {
		line, err := r.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return nil, err
		}

		line = bytes.TrimSpace(line)
		if len(line) > 0 {
			return line, nil
		}

		// If we hit EOF while looking for a non-empty line, check if we found anything
		if err == io.EOF {
			if len(line) == 0 {
				return nil, io.EOF
			}
			return line, nil
		}
	}
}

// readFirstNonEmptyLine reads lines until finding a non-empty line.
// Returns io.EOF if no non-empty line is found.
// Note: This wraps readFirstNonEmptyLineFromBufReader for compatibility with DetectFormat.
func readFirstNonEmptyLine(r io.Reader) ([]byte, error) {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}
	return readFirstNonEmptyLineFromBufReader(br)
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
	if copilotTypes[obj.Type] {
		return FormatCopilot, nil
	}

	return FormatUnknown, fmt.Errorf("unrecognized log format: type field value '%s'", obj.Type)
}

// ParseResultMetadata contains optional format-specific metadata.
type ParseResultMetadata struct {
	TotalCost *float64 // nil = not available, pointer to value = available
	CostUnit  string   // e.g., "credits"
}

// ParseResult contains the parsed entries and any warnings encountered.
type ParseResult struct {
	Entries  []Entry
	Warnings []ParseWarning
	Metadata *ParseResultMetadata // nil for formats without metadata
}

// ParseWarning represents a non-fatal parsing issue.
type ParseWarning struct {
	Line    int
	Message string
}

// Parse reads a transcript file and automatically detects its format.
// It handles all supported formats: Claude JSONL, Codex JSONL, Kiro CLI JSON, Kiro IDE JSON, and Copilot JSONL.
// Use this function when the format is unknown; use ParseJSONLWithFormat when the format is known.
func Parse(r io.Reader) (*ParseResult, error) {
	// Use DetectFormat to identify the format (handles both JSON and JSONL)
	format, chunk, err := DetectFormat(r)
	if err != nil {
		return nil, fmt.Errorf("failed to detect format: %w", err)
	}

	// Combine the chunk we read for detection with the remaining content
	combined := io.MultiReader(bytes.NewReader(chunk), r)

	return ParseJSONLWithFormat(combined, format)
}

// ParseOptions provides optional configuration for format-specific parsers.
type ParseOptions struct {
	KiroIDECostPath string // execution detail file path for cost extraction
}

// ParseJSONLWithFormat reads JSONL from the provided reader using the specified format.
// This skips format detection and directly uses the given parser.
// Use Parse for auto-detection of all formats, or ParseJSONL for JSONL-only auto-detection.
func ParseJSONLWithFormat(r io.Reader, format Format, opts ...ParseOptions) (*ParseResult, error) {
	switch format {
	case FormatCodex:
		return ParseCodexJSONL(r)
	case FormatClaude:
		return parseClaudeJSONL(r)
	case FormatKiro:
		return ParseKiro(r)
	case FormatCopilot:
		return ParseCopilot(r)
	case FormatKiroIDE:
		if len(opts) > 0 && opts[0].KiroIDECostPath != "" {
			return ParseKiroIDEWithCostPath(r, opts[0].KiroIDECostPath)
		}
		return ParseKiroIDE(r)
	default:
		return nil, fmt.Errorf("unsupported format: %d", format)
	}
}

// ParseJSONL reads JSONL from the provided reader and returns parsed entries.
// Automatically detects format (Claude or Codex) and delegates to appropriate parser.
// Preserves streaming architecture to avoid memory issues with large files.
func ParseJSONL(r io.Reader) (*ParseResult, error) {
	bufReader := bufio.NewReader(r)

	// Collect lines for format detection, skipping infrastructure entries
	var collectedLines [][]byte
	var format Format
	var formatErr error

	for {
		line, err := readFirstNonEmptyLineFromBufReader(bufReader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				if len(collectedLines) == 0 {
					return nil, fmt.Errorf("empty file")
				}
				return nil, fmt.Errorf("no recognizable format entries found")
			}
			return nil, fmt.Errorf("failed to read line: %w", err)
		}

		// Strip BOM if present (only relevant for first line)
		if len(collectedLines) == 0 {
			line = stripBOM(line)
		}

		collectedLines = append(collectedLines, line)

		// Try to detect format from this line
		format, formatErr = detectFormatFromLine(line)
		if formatErr == nil {
			// Found a recognizable format
			break
		}

		// Check if this is an infrastructure type we should skip
		var obj struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(line, &obj) == nil && infrastructureTypes[obj.Type] {
			// Skip infrastructure entries and keep looking
			continue
		}

		// Unknown type that's not infrastructure - fail
		return nil, formatErr
	}

	// Reconstruct collected lines for the parser
	var linesBuf bytes.Buffer
	for _, line := range collectedLines {
		linesBuf.Write(line)
		linesBuf.WriteByte('\n')
	}

	// Combine collected lines with remaining content for streaming parse
	combined := io.MultiReader(&linesBuf, bufReader)

	// Delegate to appropriate parser
	switch format {
	case FormatCodex:
		return ParseCodexJSONL(combined)
	case FormatClaude:
		return parseClaudeJSONL(combined)
	default:
		return nil, fmt.Errorf("unrecognized log format")
	}
}

// parseClaudeJSONL parses Claude Code format JSONL.
// This is the existing Claude parsing logic extracted to a separate function.
func parseClaudeJSONL(r io.Reader) (*ParseResult, error) {
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
		// Skip infrastructure types (queue-operation) and unknown types silently
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
