// Package transcript provides JSONL parsing and Markdown rendering for Claude session transcripts.
package transcript

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

// RenderOptions configures Markdown rendering.
type RenderOptions struct {
	Title     string // Document title (e.g., "Session Transcript" or "Phase 1 Session Transcript")
	SessionID string // Session ID to display in header
}
