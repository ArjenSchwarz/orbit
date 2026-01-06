package transcript

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// codexParser maintains state while parsing Codex JSONL.
type codexParser struct {
	sessionID     string
	entries       []Entry
	warnings      []ParseWarning
	functionCalls map[string]*pendingCall // call_id -> pending function_call
	currentEntry  *Entry                  // Current entry being built
	lineNum       int
}

// pendingCall tracks a function_call waiting for its output.
type pendingCall struct {
	callID    string
	entryIdx  int // Index in entries slice where this call lives
	contentIdx int // Index in Content slice where tool_use is
}

// ParseCodexJSONL parses Codex format JSONL and normalizes to []Entry.
func ParseCodexJSONL(r io.Reader) (*ParseResult, error) {
	p := &codexParser{
		entries:       []Entry{},
		warnings:      []ParseWarning{},
		functionCalls: make(map[string]*pendingCall),
	}

	scanner := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)   // 64KB initial
	scanner.Buffer(buf, 10*1024*1024) // 10MB max

	for scanner.Scan() {
		p.lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		if err := p.parseEntry(line); err != nil {
			p.warnings = append(p.warnings, ParseWarning{
				Line:    p.lineNum,
				Message: err.Error(),
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error: %w", err)
	}

	// Finalize any pending entry
	p.finalizeCurrentEntry()

	if len(p.entries) == 0 {
		return nil, fmt.Errorf("no valid entries found in file")
	}

	return &ParseResult{
		Entries:  p.entries,
		Warnings: p.warnings,
	}, nil
}

// parseEntry processes a single JSONL line.
func (p *codexParser) parseEntry(line []byte) error {
	var entry CodexEntry
	if err := json.Unmarshal(line, &entry); err != nil {
		return fmt.Errorf("failed to parse JSON: %v", err)
	}

	// Validate required fields
	if entry.Type == "" {
		return fmt.Errorf("missing required field: type")
	}

	switch entry.Type {
	case "session_meta":
		return p.processSessionMeta(&entry)
	case "response_item":
		if len(entry.Payload) == 0 {
			return fmt.Errorf("missing required field: payload")
		}
		return p.processResponseItem(&entry)
	case "event_msg":
		if len(entry.Payload) == 0 {
			return fmt.Errorf("missing required field: payload")
		}
		return p.processEventMsg(&entry)
	case "turn_context":
		// Skip - internal context tracking
		return nil
	default:
		return fmt.Errorf("unrecognized event type: %s", entry.Type)
	}
}

// processSessionMeta extracts session metadata.
func (p *codexParser) processSessionMeta(entry *CodexEntry) error {
	var meta CodexSessionMeta
	if err := json.Unmarshal(entry.Payload, &meta); err != nil {
		return fmt.Errorf("failed to parse session_meta payload: %v", err)
	}
	p.sessionID = meta.ID
	return nil
}

// processResponseItem handles response_item events.
func (p *codexParser) processResponseItem(entry *CodexEntry) error {
	var item CodexResponseItem
	if err := json.Unmarshal(entry.Payload, &item); err != nil {
		return fmt.Errorf("failed to parse response_item payload: %v", err)
	}

	switch item.Type {
	case "message":
		return p.convertMessage(&item, entry.Timestamp)
	case "function_call":
		return p.convertFunctionCall(&item, entry.Timestamp)
	case "function_call_output":
		return p.convertFunctionCallOutput(&item, entry.Timestamp)
	case "reasoning":
		return p.convertReasoning(&item, entry.Timestamp)
	case "ghost_snapshot":
		// Skip - git tracking metadata
		return nil
	default:
		return fmt.Errorf("unrecognized response_item type: %s", item.Type)
	}
}

// processEventMsg handles event_msg events.
func (p *codexParser) processEventMsg(entry *CodexEntry) error {
	var msg CodexEventMsg
	if err := json.Unmarshal(entry.Payload, &msg); err != nil {
		return fmt.Errorf("failed to parse event_msg payload: %v", err)
	}

	switch msg.Type {
	case "agent_reasoning":
		return p.convertAgentReasoning(&msg, entry.Timestamp)
	case "agent_message":
		return p.convertAgentMessage(&msg, entry.Timestamp)
	case "token_count", "user_message":
		// Skip - metadata events
		return nil
	default:
		return fmt.Errorf("unrecognized event_msg type: %s", msg.Type)
	}
}

// convertMessage converts a Codex message to an Entry.
func (p *codexParser) convertMessage(item *CodexResponseItem, timestamp string) error {
	entryType := item.Role
	if entryType != "user" && entryType != "assistant" {
		return fmt.Errorf("unrecognized message role: %s", item.Role)
	}

	// User message always starts a new entry
	if entryType == "user" {
		p.finalizeCurrentEntry()
		p.currentEntry = &Entry{
			Type:      "user",
			Timestamp: timestamp,
			SessionID: p.sessionID,
			Message: &Message{
				Role:    "user",
				Content: []ContentItem{},
			},
		}
	} else {
		// Assistant message - consolidate with current if it's already assistant
		p.ensureAssistantEntry(timestamp)
	}

	// Convert content items
	for _, content := range item.Content {
		contentItem := p.convertContentItem(&content)
		if contentItem != nil {
			p.currentEntry.Message.Content = append(p.currentEntry.Message.Content, *contentItem)
		}
	}

	return nil
}

// convertContentItem converts a Codex content item to a ContentItem.
func (p *codexParser) convertContentItem(content *CodexContent) *ContentItem {
	switch content.Type {
	case "input_text", "output_text":
		return &ContentItem{
			Type: "text",
			Text: content.Text,
		}
	default:
		// Unknown content type - render as text with raw JSON
		data, _ := json.Marshal(content)
		return &ContentItem{
			Type: "text",
			Text: string(data),
		}
	}
}

// convertFunctionCall converts a function_call to tool_use ContentItem.
func (p *codexParser) convertFunctionCall(item *CodexResponseItem, timestamp string) error {
	p.ensureAssistantEntry(timestamp)

	// Parse arguments
	var input any = item.Arguments
	var parsedInput map[string]any
	if err := json.Unmarshal([]byte(item.Arguments), &parsedInput); err == nil {
		input = parsedInput
	}

	contentItem := ContentItem{
		Type:  "tool_use",
		ID:    item.CallID,
		Name:  item.Name,
		Input: input,
	}

	p.currentEntry.Message.Content = append(p.currentEntry.Message.Content, contentItem)

	// Track for linking with output
	p.functionCalls[item.CallID] = &pendingCall{
		callID:     item.CallID,
		entryIdx:   len(p.entries), // Will be current entry when finalized
		contentIdx: len(p.currentEntry.Message.Content) - 1,
	}

	return nil
}

// convertFunctionCallOutput converts a function_call_output to tool_result ContentItem.
func (p *codexParser) convertFunctionCallOutput(item *CodexResponseItem, timestamp string) error {
	contentItem := ContentItem{
		Type:      "tool_result",
		ToolUseID: item.CallID,
		Content:   item.Output,
	}

	// Check if we have a matching function_call
	pending, found := p.functionCalls[item.CallID]
	if !found {
		// Orphaned output - warn but still add it
		p.warnings = append(p.warnings, ParseWarning{
			Line:    p.lineNum,
			Message: fmt.Sprintf("no matching function_call for call_id: %s", item.CallID),
		})
		p.ensureAssistantEntry(timestamp)
		p.currentEntry.Message.Content = append(p.currentEntry.Message.Content, contentItem)
		return nil
	}

	// Add to current entry (function_call should be in current entry)
	p.ensureAssistantEntry(timestamp)
	p.currentEntry.Message.Content = append(p.currentEntry.Message.Content, contentItem)

	// Don't delete from map - supports multiple outputs per call
	_ = pending

	return nil
}

// convertReasoning converts a reasoning response to thinking ContentItem.
func (p *codexParser) convertReasoning(item *CodexResponseItem, timestamp string) error {
	p.ensureAssistantEntry(timestamp)

	// Concatenate summary texts
	var texts []string
	for _, summary := range item.Summary {
		if summary.Text != "" {
			texts = append(texts, summary.Text)
		}
	}

	if len(texts) > 0 {
		contentItem := ContentItem{
			Type:     "thinking",
			Thinking: strings.Join(texts, "\n"),
		}
		p.currentEntry.Message.Content = append(p.currentEntry.Message.Content, contentItem)
	}

	return nil
}

// convertAgentReasoning converts agent_reasoning event to thinking ContentItem.
func (p *codexParser) convertAgentReasoning(msg *CodexEventMsg, timestamp string) error {
	p.ensureAssistantEntry(timestamp)

	if msg.Text != "" {
		contentItem := ContentItem{
			Type:     "thinking",
			Thinking: msg.Text,
		}
		p.currentEntry.Message.Content = append(p.currentEntry.Message.Content, contentItem)
	}

	return nil
}

// convertAgentMessage converts agent_message event to text ContentItem.
func (p *codexParser) convertAgentMessage(msg *CodexEventMsg, timestamp string) error {
	p.ensureAssistantEntry(timestamp)

	if msg.Message != "" {
		contentItem := ContentItem{
			Type: "text",
			Text: msg.Message,
		}
		p.currentEntry.Message.Content = append(p.currentEntry.Message.Content, contentItem)
	}

	return nil
}

// ensureAssistantEntry ensures there's a current assistant entry to add content to.
func (p *codexParser) ensureAssistantEntry(timestamp string) {
	if p.currentEntry == nil || p.currentEntry.Type == "user" {
		p.finalizeCurrentEntry()
		p.currentEntry = &Entry{
			Type:      "assistant",
			Timestamp: timestamp,
			SessionID: p.sessionID,
			Message: &Message{
				Role:    "assistant",
				Content: []ContentItem{},
			},
		}
	}
}

// finalizeCurrentEntry adds the current entry to entries if it has content.
func (p *codexParser) finalizeCurrentEntry() {
	if p.currentEntry != nil && p.currentEntry.Message != nil && len(p.currentEntry.Message.Content) > 0 {
		p.entries = append(p.entries, *p.currentEntry)
	}
	p.currentEntry = nil
}
