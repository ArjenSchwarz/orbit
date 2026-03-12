package transcript

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
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

	var metadata *ParseResultMetadata
	var entries []Entry
	var warnings []ParseWarning

	if costPath != "" {
		detail := readKiroIDEExecutionDetail(costPath)
		if detail != nil {
			// Extract cost
			totalCost := sumKiroIDECredits(detail)
			if totalCost > 0 {
				metadata = &ParseResultMetadata{
					TotalCost: &totalCost,
					CostUnit:  "credits",
				}
			}

			// Use actions for richer entries when available
			if len(detail.Actions) > 0 {
				entries, warnings = convertKiroIDEActionsToEntries(detail.Actions, &chatFile)
			}
		}
	}

	// Fall back to chat-based entries when no actions available
	if entries == nil {
		entries, warnings = convertKiroIDEToEntries(&chatFile)
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

	// Extract session-level metadata
	var sessionTimestamp, modelID string
	if chatFile.Metadata != nil {
		if chatFile.Metadata.StartTime > 0 {
			sessionTimestamp = time.UnixMilli(chatFile.Metadata.StartTime).UTC().Format(time.RFC3339)
		}
		modelID = chatFile.Metadata.ModelID
	}

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
		if strings.TrimSpace(msg.Content) == "" {
			continue
		}

		switch msg.Role {
		case "human":
			entries = append(entries, Entry{
				Type: "user",
				Message: &Message{
					Role: "user",
					Content: []ContentItem{
						{Type: "text", Text: msg.Content},
					},
				},
			})
		case "bot":
			entries = append(entries, Entry{
				Type:  "assistant",
				Model: modelID,
				Message: &Message{
					Role: "assistant",
					Content: []ContentItem{
						{Type: "text", Text: msg.Content},
					},
				},
			})
		case "tool":
			entries = append(entries, Entry{
				Type: "user",
				Message: &Message{
					Role: "user",
					Content: []ContentItem{
						{Type: "tool_result", Content: msg.Content},
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

	// Set timestamp on first entry only (session-level start time)
	if len(entries) > 0 && sessionTimestamp != "" {
		entries[0].Timestamp = sessionTimestamp
	}

	return entries, warnings
}

// kiroIDEActionToolNames maps Kiro IDE action types to tool display names.
var kiroIDEActionToolNames = map[string]string{
	"readFiles":  "Read",
	"replace":    "Edit",
	"create":     "Write",
	"append":     "Edit",
	"runCommand": "Bash",
	"search":     "Grep",
}

// kiroIDESkippedActions are internal action types that don't produce entries.
var kiroIDESkippedActions = map[string]bool{
	"model":                true,
	"steering":             true,
	"intentClassification": true,
	"specAgent":            true,
}

// convertKiroIDEActionsToEntries builds entries from execution detail actions,
// supplemented with user messages from the chat file. This produces richer output
// than chat-only parsing because actions contain tool calls with inputs and results.
func convertKiroIDEActionsToEntries(actions []KiroIDEAction, chatFile *KiroIDEChatFile) ([]Entry, []ParseWarning) {
	entries := make([]Entry, 0, len(actions))
	warnings := make([]ParseWarning, 0)

	// Extract session-level metadata
	var sessionTimestamp, modelID string
	if chatFile.Metadata != nil {
		if chatFile.Metadata.StartTime > 0 {
			sessionTimestamp = time.UnixMilli(chatFile.Metadata.StartTime).UTC().Format(time.RFC3339)
		}
		modelID = chatFile.Metadata.ModelID
	}

	// Extract user messages from chat (non-system-prompt human messages)
	userMessages := extractKiroIDEUserMessages(chatFile)

	// Add user messages at the start (these are the user's prompts)
	entries = append(entries, userMessages...)

	for i, action := range actions {
		if kiroIDESkippedActions[action.ActionType] {
			continue
		}

		switch action.ActionType {
		case "say":
			entry := convertKiroIDESayAction(&action)
			if entry != nil {
				entry.Model = modelID
				entries = append(entries, *entry)
			}

		case "taskStatus":
			entry := convertKiroIDETaskStatusAction(&action)
			if entry != nil {
				entry.Model = modelID
				entries = append(entries, *entry)
			}

		case "userInput":
			entry := convertKiroIDEUserInputAction(&action)
			if entry != nil {
				entries = append(entries, *entry)
			}

		case "readFiles", "replace", "create", "append", "runCommand", "search":
			toolUse, toolResult := convertKiroIDEToolAction(&action)
			if toolUse != nil {
				toolUse.Model = modelID
				entries = append(entries, *toolUse)
			}
			if toolResult != nil {
				// tool_result entries are "user" type — no model
				entries = append(entries, *toolResult)
			}

		default:
			warnings = append(warnings, ParseWarning{
				Line:    i + 1,
				Message: fmt.Sprintf("unknown action type %q, skipped", action.ActionType),
			})
		}
	}

	// Set timestamp on first entry only (session-level start time)
	if len(entries) > 0 && sessionTimestamp != "" {
		entries[0].Timestamp = sessionTimestamp
	}

	return entries, warnings
}

// extractKiroIDEUserMessages extracts user prompt messages from the chat file,
// filtering system prompts and empty content.
func extractKiroIDEUserMessages(chatFile *KiroIDEChatFile) []Entry {
	var entries []Entry
	for i, msg := range chatFile.Chat {
		if msg.Role != "human" {
			continue
		}
		// Filter system prompt (first human message with <identity> prefix)
		if i == 0 && strings.HasPrefix(msg.Content, "<identity>") {
			continue
		}
		if strings.TrimSpace(msg.Content) == "" {
			continue
		}
		entries = append(entries, Entry{
			Type: "user",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{Type: "text", Text: msg.Content},
				},
			},
		})
	}
	return entries
}

// convertKiroIDESayAction converts a "say" action to an assistant text entry.
func convertKiroIDESayAction(action *KiroIDEAction) *Entry {
	msg, _ := action.Output["message"].(string)
	if msg == "" {
		return nil
	}
	return &Entry{
		Type: "assistant",
		Message: &Message{
			Role: "assistant",
			Content: []ContentItem{
				{Type: "text", Text: msg},
			},
		},
	}
}

// convertKiroIDETaskStatusAction converts a "taskStatus" action to an assistant text entry.
func convertKiroIDETaskStatusAction(action *KiroIDEAction) *Entry {
	if action.TaskID == "" {
		return nil
	}
	text := fmt.Sprintf("Task %q: %s", action.TaskID, action.TaskStatus)
	return &Entry{
		Type: "assistant",
		Message: &Message{
			Role: "assistant",
			Content: []ContentItem{
				{Type: "text", Text: text},
			},
		},
	}
}

// convertKiroIDEUserInputAction converts a "userInput" action to a user text entry.
func convertKiroIDEUserInputAction(action *KiroIDEAction) *Entry {
	questions, ok := action.Output["questions"].([]any)
	if !ok || len(questions) == 0 {
		return nil
	}

	var parts []string
	for _, q := range questions {
		qMap, ok := q.(map[string]any)
		if !ok {
			continue
		}
		question, _ := qMap["question"].(string)
		if question != "" {
			parts = append(parts, question)
		}
		resp, _ := qMap["response"].(map[string]any)
		if resp != nil {
			if respType, _ := resp["type"].(string); respType != "" {
				parts = append(parts, fmt.Sprintf("[Response: %s]", respType))
			}
		}
	}

	if len(parts) == 0 {
		return nil
	}

	return &Entry{
		Type: "user",
		Message: &Message{
			Role: "user",
			Content: []ContentItem{
				{Type: "text", Text: strings.Join(parts, "\n")},
			},
		},
	}
}

// convertKiroIDEToolAction converts a tool action (readFiles, replace, create,
// append, runCommand, search) to a tool_use entry + tool_result entry pair.
func convertKiroIDEToolAction(action *KiroIDEAction) (*Entry, *Entry) {
	toolName, ok := kiroIDEActionToolNames[action.ActionType]
	if !ok {
		return nil, nil
	}

	// Build tool_use input description
	input := buildKiroIDEToolInput(action)

	toolUseEntry := &Entry{
		Type: "assistant",
		Message: &Message{
			Role: "assistant",
			Content: []ContentItem{
				{
					Type:  "tool_use",
					ID:    action.ActionID,
					Name:  toolName,
					Input: input,
				},
			},
		},
	}

	// Build tool_result
	resultContent, isError := buildKiroIDEToolResult(action)

	toolResultEntry := &Entry{
		Type: "user",
		Message: &Message{
			Role: "user",
			Content: []ContentItem{
				{
					Type:      "tool_result",
					ToolUseID: action.ActionID,
					Content:   resultContent,
					IsError:   isError,
				},
			},
		},
	}

	return toolUseEntry, toolResultEntry
}

// buildKiroIDEToolInput creates the input representation for a tool action.
func buildKiroIDEToolInput(action *KiroIDEAction) any {
	switch action.ActionType {
	case "readFiles":
		files, ok := action.Input["files"].([]any)
		if !ok {
			return map[string]any{"files": action.Input["files"]}
		}
		var paths []string
		for _, f := range files {
			fMap, ok := f.(map[string]any)
			if !ok {
				continue
			}
			if p, ok := fMap["path"].(string); ok {
				paths = append(paths, p)
			}
		}
		return map[string]any{"file_paths": paths}

	case "replace", "append":
		result := map[string]any{}
		if file, ok := action.Input["file"].(string); ok {
			result["file_path"] = file
		}
		return result

	case "create":
		result := map[string]any{}
		if file, ok := action.Input["file"].(string); ok {
			result["file_path"] = file
		}
		return result

	case "runCommand":
		result := map[string]any{}
		if cmd, ok := action.Input["command"].(string); ok {
			result["command"] = cmd
		}
		return result

	case "search":
		result := map[string]any{}
		if query, ok := action.Input["query"].(string); ok {
			result["query"] = query
		}
		if why, ok := action.Input["why"].(string); ok {
			result["reason"] = why
		}
		return result

	default:
		return action.Input
	}
}

// buildKiroIDEToolResult creates the result content string and error flag for a tool action.
func buildKiroIDEToolResult(action *KiroIDEAction) (string, bool) {
	isError := action.ActionState == "Error"

	// Handle error message
	if action.ErrorMessage != "" {
		return action.ErrorMessage, true
	}

	switch action.ActionState {
	case "Rejected":
		return "Action rejected by user", true
	case "Canceled":
		return "Action canceled", true
	case "Running":
		return "Action still running", false
	}

	switch action.ActionType {
	case "runCommand":
		if action.Output != nil {
			var parts []string
			if output, ok := action.Output["output"].(string); ok && output != "" {
				parts = append(parts, output)
			}
			if exitCode, ok := action.Output["exitCode"].(float64); ok {
				if exitCode != 0 {
					parts = append(parts, fmt.Sprintf("Exit code: %d", int(exitCode)))
					isError = true
				}
			}
			if len(parts) > 0 {
				return strings.Join(parts, "\n"), isError
			}
		}

	case "readFiles":
		if files, ok := action.Input["files"].([]any); ok {
			var paths []string
			for _, f := range files {
				if fMap, ok := f.(map[string]any); ok {
					if p, ok := fMap["path"].(string); ok {
						paths = append(paths, p)
					}
				}
			}
			if len(paths) > 0 {
				return fmt.Sprintf("Read %d file(s): %s", len(paths), strings.Join(paths, ", ")), false
			}
		}

	case "replace", "append", "create":
		if file, ok := action.Input["file"].(string); ok {
			return file, false
		}

	case "search":
		if query, ok := action.Input["query"].(string); ok {
			return query, false
		}
	}

	return action.ActionState, isError
}

// readKiroIDEExecutionDetail reads and parses an execution detail file.
// Returns nil if the file doesn't exist or can't be parsed.
func readKiroIDEExecutionDetail(path string) *KiroIDEExecutionDetail {
	if path == "" {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var detail KiroIDEExecutionDetail
	if err := json.Unmarshal(data, &detail); err != nil {
		return nil
	}

	return &detail
}

// sumKiroIDECredits sums credit usage from an execution detail.
func sumKiroIDECredits(detail *KiroIDEExecutionDetail) float64 {
	var total float64
	for _, usage := range detail.UsageSummary {
		if usage.Unit == "credit" {
			total += usage.Usage
		}
	}
	return total
}

// extractKiroIDECost reads an execution detail file and sums credit usage.
// Returns 0 if the path is empty, the file doesn't exist, or can't be parsed.
func extractKiroIDECost(path string) float64 {
	detail := readKiroIDEExecutionDetail(path)
	if detail == nil {
		return 0
	}
	return sumKiroIDECredits(detail)
}
