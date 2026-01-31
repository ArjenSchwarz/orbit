package transcript

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// ParseCopilot parses a Copilot session JSONL file and returns the result.
func ParseCopilot(r io.Reader) (*ParseResult, error) {
	scanner := bufio.NewScanner(r)
	// Increase buffer size to handle long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024) // 1MB max line size

	var events []CopilotEvent
	var warnings []ParseWarning
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var event CopilotEvent
		if err := json.Unmarshal(line, &event); err != nil {
			warnings = append(warnings, ParseWarning{
				Line:    lineNum,
				Message: fmt.Sprintf("failed to parse JSON: %v", err),
			})
			continue
		}

		events = append(events, event)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading Copilot session: %w", err)
	}

	entries := convertCopilotToEntries(events)

	return &ParseResult{
		Entries:  entries,
		Warnings: warnings,
	}, nil
}

// convertCopilotToEntries converts Copilot events to the common Entry format.
func convertCopilotToEntries(events []CopilotEvent) []Entry {
	var entries []Entry

	// Track tool calls and their results for matching
	toolCalls := make(map[string]CopilotToolRequest) // toolCallId -> request
	toolResults := make(map[string]string)           // toolCallId -> result

	// First pass: collect tool calls and results
	for _, event := range events {
		switch event.Type {
		case "assistant.message":
			for _, req := range event.Data.ToolRequests {
				toolCalls[req.ToolCallID] = req
			}
		case "tool.execution_complete":
			if event.Data.Result != nil {
				toolResults[event.Data.ToolCallID] = event.Data.Result.Content
			}
		}
	}

	// Second pass: generate entries
	var currentTurnContent []ContentItem
	var inAssistantTurn bool

	for _, event := range events {
		switch event.Type {
		case "user.message":
			// User message
			entries = append(entries, Entry{
				Type: "user",
				Message: &Message{
					Role: "user",
					Content: []ContentItem{
						{
							Type: "text",
							Text: event.Data.Content,
						},
					},
				},
			})

		case "assistant.turn_start":
			inAssistantTurn = true
			currentTurnContent = nil

		case "assistant.message":
			// Add text content if present
			if event.Data.Content != "" {
				currentTurnContent = append(currentTurnContent, ContentItem{
					Type: "text",
					Text: event.Data.Content,
				})
			}

			// Add tool calls
			for _, req := range event.Data.ToolRequests {
				currentTurnContent = append(currentTurnContent, ContentItem{
					Type:  "tool_use",
					ID:    req.ToolCallID,
					Name:  req.Name,
					Input: req.Arguments,
				})
			}

		case "assistant.turn_end":
			if inAssistantTurn && len(currentTurnContent) > 0 {
				// Generate assistant entry with the collected content
				entry := Entry{
					Type: "assistant",
					Message: &Message{
						Role:    "assistant",
						Content: currentTurnContent,
					},
				}
				entries = append(entries, entry)

				// Generate user entry with tool results for any tools in this turn
				var toolResultItems []ContentItem
				for _, item := range currentTurnContent {
					if item.Type == "tool_use" && item.ID != "" {
						if result, ok := toolResults[item.ID]; ok {
							toolResultItems = append(toolResultItems, ContentItem{
								Type:      "tool_result",
								ToolUseID: item.ID,
								Content:   result,
							})
							delete(toolResults, item.ID) // Mark as used
						}
					}
				}

				if len(toolResultItems) > 0 {
					entries = append(entries, Entry{
						Type: "user",
						Message: &Message{
							Role:    "user",
							Content: toolResultItems,
						},
					})
				}
			}
			inAssistantTurn = false
			currentTurnContent = nil
		}
	}

	return entries
}
