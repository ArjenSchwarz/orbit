package transcript

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// ParseKiroIDE parses a Kiro IDE .chat JSON file and returns the result without cost data.
func ParseKiroIDE(r io.Reader) (*ParseResult, error) {
	return parseKiroIDE(r, "")
}

// ParseKiroIDEWithCostPath parses a .chat file and extracts cost from the given
// execution detail file path.
func ParseKiroIDEWithCostPath(r io.Reader, executionDetailPath string) (*ParseResult, error) {
	return parseKiroIDE(r, executionDetailPath)
}

func parseKiroIDE(r io.Reader, costPath string) (*ParseResult, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read Kiro IDE chat file: %w", err)
	}

	var chatFile KiroIDEChatFile
	if err := json.Unmarshal(data, &chatFile); err != nil {
		return nil, fmt.Errorf("failed to parse Kiro IDE chat JSON: %w", err)
	}

	entries, warnings := convertKiroIDEToEntries(&chatFile)

	var metadata *ParseResultMetadata
	if costPath != "" {
		totalCost := extractKiroIDECost(costPath)
		if totalCost > 0 {
			metadata = &ParseResultMetadata{
				TotalCost: &totalCost,
				CostUnit:  "credits",
			}
		}
	}

	return &ParseResult{
		Entries:  entries,
		Warnings: warnings,
		Metadata: metadata,
	}, nil
}

// convertKiroIDEToEntries converts a KiroIDEChatFile to the common Entry format.
// Filters system prompts (first human message with <identity> prefix), skips empty
// content messages, and generates warnings for messages with missing or unknown roles.
func convertKiroIDEToEntries(chatFile *KiroIDEChatFile) ([]Entry, []ParseWarning) {
	entries := make([]Entry, 0, len(chatFile.Chat))
	warnings := make([]ParseWarning, 0)

	for i, msg := range chatFile.Chat {
		// Skip messages with empty role
		if msg.Role == "" {
			warnings = append(warnings, ParseWarning{
				Line:    i + 1,
				Message: fmt.Sprintf("message %d: missing role, skipped", i),
			})
			continue
		}

		// Filter first human message if it's a system prompt
		if i == 0 && msg.Role == "human" && strings.HasPrefix(msg.Content, "<identity>") {
			continue
		}

		// Skip empty or whitespace-only content (streaming artifacts)
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}

		switch msg.Role {
		case "human":
			entries = append(entries, Entry{
				Type: "user",
				Message: &Message{
					Role: "user",
					Content: []ContentItem{
						{Type: "text", Text: content},
					},
				},
			})
		case "bot":
			entries = append(entries, Entry{
				Type: "assistant",
				Message: &Message{
					Role: "assistant",
					Content: []ContentItem{
						{Type: "text", Text: content},
					},
				},
			})
		case "tool":
			entries = append(entries, Entry{
				Type: "user",
				Message: &Message{
					Role: "user",
					Content: []ContentItem{
						{Type: "tool_result", Content: content},
					},
				},
			})
		default:
			warnings = append(warnings, ParseWarning{
				Line:    i + 1,
				Message: fmt.Sprintf("unknown role %q in chat message", msg.Role),
			})
		}
	}

	return entries, warnings
}

// extractKiroIDECost reads an execution detail file and sums credit usage.
// Returns 0 if the path is empty, the file doesn't exist, or can't be parsed.
func extractKiroIDECost(path string) float64 {
	if path == "" {
		return 0
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}

	var detail KiroIDEExecutionDetail
	if err := json.Unmarshal(data, &detail); err != nil {
		return 0
	}

	var total float64
	for _, usage := range detail.UsageSummary {
		if usage.Unit == "credit" {
			total += usage.Usage
		}
	}
	return total
}
