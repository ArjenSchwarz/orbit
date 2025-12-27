// Package transcript provides JSONL parsing and Markdown rendering for Claude session transcripts.
package transcript

import (
	"encoding/json"
)

// Entry represents a single line in the Claude session JSONL.
type Entry struct {
	Type      string   `json:"type"`
	Message   *Message `json:"message,omitempty"`
	Timestamp string   `json:"timestamp,omitempty"`
	SessionID string   `json:"sessionId,omitempty"`
}

// Message represents the message content within an entry.
type Message struct {
	Role    string        `json:"role"`
	Content []ContentItem `json:"content"`
}

// UnmarshalJSON handles polymorphic content field (string or array).
// User messages have content as a plain string, assistant messages have an array.
func (m *Message) UnmarshalJSON(data []byte) error {
	// First, try to unmarshal with Content as []ContentItem
	type messageArray struct {
		Role    string        `json:"role"`
		Content []ContentItem `json:"content"`
	}
	var arrMsg messageArray
	if err := json.Unmarshal(data, &arrMsg); err == nil && len(arrMsg.Content) > 0 {
		m.Role = arrMsg.Role
		m.Content = arrMsg.Content
		return nil
	}

	// Fall back to content as string (user messages)
	type messageString struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	var strMsg messageString
	if err := json.Unmarshal(data, &strMsg); err == nil && strMsg.Content != "" {
		m.Role = strMsg.Role
		m.Content = []ContentItem{{Type: "text", Text: strMsg.Content}}
		return nil
	}

	// Handle empty content case
	m.Role = arrMsg.Role
	m.Content = nil
	return nil
}

// ContentItem represents a content block in a message.
type ContentItem struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Thinking string `json:"thinking,omitempty"`
	Name     string `json:"name,omitempty"`
	Input    any    `json:"input,omitempty"`
	Content  string `json:"content,omitempty"`
	IsError  bool   `json:"is_error,omitempty"`
}

// UnmarshalJSON handles polymorphic content field (string or array).
// Tool results can have content as either a string or an array of content blocks.
func (c *ContentItem) UnmarshalJSON(data []byte) error {
	// Use an alias to avoid infinite recursion
	type contentItemAlias struct {
		Type     string `json:"type"`
		Text     string `json:"text,omitempty"`
		Thinking string `json:"thinking,omitempty"`
		Name     string `json:"name,omitempty"`
		Input    any    `json:"input,omitempty"`
		Content  any    `json:"content,omitempty"`
		IsError  bool   `json:"is_error,omitempty"`
	}

	var alias contentItemAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}

	c.Type = alias.Type
	c.Text = alias.Text
	c.Thinking = alias.Thinking
	c.Name = alias.Name
	c.Input = alias.Input
	c.IsError = alias.IsError

	// Handle content field which can be string or array
	switch v := alias.Content.(type) {
	case string:
		c.Content = v
	case []any:
		// Convert array to JSON string for display
		if len(v) > 0 {
			contentBytes, err := json.Marshal(v)
			if err == nil {
				c.Content = string(contentBytes)
			}
		}
	}

	return nil
}

// RenderOptions configures Markdown rendering.
type RenderOptions struct {
	Title     string // Document title (e.g., "Session Transcript" or "Phase 1 Session Transcript")
	SessionID string // Session ID to display in header
}
