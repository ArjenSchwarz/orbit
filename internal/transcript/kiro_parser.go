package transcript

import (
	"encoding/json"
	"fmt"
	"io"
)

// ParseKiro parses a Kiro session JSON file and returns the result.
func ParseKiro(r io.Reader) (*ParseResult, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read Kiro session: %w", err)
	}

	var session KiroSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to parse Kiro session JSON: %w", err)
	}

	entries, warnings := convertKiroToEntries(&session)

	return &ParseResult{
		Entries:  entries,
		Warnings: warnings,
	}, nil
}

// convertKiroToEntries converts a Kiro session to the common Entry format.
// It processes history entries and generates the appropriate user/assistant Entry sequence.
func convertKiroToEntries(session *KiroSession) ([]Entry, []ParseWarning) {
	var entries []Entry
	var warnings []ParseWarning

	for _, historyEntry := range session.History {
		// Process user message first
		if historyEntry.User != nil {
			userEntries := convertKiroUserMessage(historyEntry.User)
			entries = append(entries, userEntries...)
		}

		// Process assistant message
		if historyEntry.Assistant != nil {
			assistantEntries := convertKiroAssistantMessage(historyEntry.Assistant)
			entries = append(entries, assistantEntries...)
		}
	}

	return entries, warnings
}

// convertKiroUserMessage converts a Kiro user message to Entry format.
func convertKiroUserMessage(userMsg *KiroUserMessage) []Entry {
	var entries []Entry

	if userMsg.Content.Prompt != nil {
		// Regular user prompt
		entries = append(entries, Entry{
			Type: "user",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{
						Type: "text",
						Text: userMsg.Content.Prompt.Prompt,
					},
				},
			},
		})
	}

	if userMsg.Content.ToolUseResults != nil {
		// Tool results - generate a user entry with tool_result content items
		var resultItems []ContentItem
		for _, result := range userMsg.Content.ToolUseResults.ToolUseResults {
			var text string
			for _, content := range result.Content {
				if content.Text != "" {
					text += content.Text
				}
			}
			resultItems = append(resultItems, ContentItem{
				Type:      "tool_result",
				ToolUseID: result.ToolUseID,
				Content:   text,
				IsError:   result.Status != "Success",
			})
		}
		if len(resultItems) > 0 {
			entries = append(entries, Entry{
				Type: "user",
				Message: &Message{
					Role:    "user",
					Content: resultItems,
				},
			})
		}
	}

	return entries
}

// convertKiroAssistantMessage converts a Kiro assistant message to Entry format.
func convertKiroAssistantMessage(assistantMsg *KiroAssistantMessage) []Entry {
	var entries []Entry

	if assistantMsg.ToolUse != nil {
		entry := convertKiroToolUse(assistantMsg.ToolUse)
		entries = append(entries, entry)
	}

	if assistantMsg.TextResponse != nil {
		entries = append(entries, Entry{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type: "text",
						Text: assistantMsg.TextResponse.Content,
					},
				},
			},
		})
	}

	return entries
}

// convertKiroToolUse converts a Kiro ToolUse message to Entry format.
func convertKiroToolUse(toolUse *KiroToolUse) Entry {
	var contentItems []ContentItem

	// Add assistant text content first (if any)
	if toolUse.Content != "" {
		contentItems = append(contentItems, ContentItem{
			Type: "text",
			Text: toolUse.Content,
		})
	}

	// Add tool uses
	for _, tool := range toolUse.ToolUses {
		contentItems = append(contentItems, ContentItem{
			Type:  "tool_use",
			ID:    tool.ID,
			Name:  tool.Name,
			Input: tool.Args,
		})
	}

	return Entry{
		Type: "assistant",
		Message: &Message{
			Role:    "assistant",
			Content: contentItems,
		},
	}
}
