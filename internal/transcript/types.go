// Package transcript provides JSONL parsing and Markdown rendering for Claude session transcripts.
package transcript

import (
	"encoding/json"
)

// Format represents the detected log format.
type Format int

const (
	FormatUnknown Format = iota
	FormatClaude
	FormatCodex
	FormatKiro
	FormatCopilot
	FormatKiroIDE
	FormatCodexExec // Codex CLI `codex exec --json` event stream
)

// Entry represents a single line in the Claude session JSONL.
type Entry struct {
	Type            string         `json:"type"`
	Message         *Message       `json:"message,omitempty"`
	Timestamp       string         `json:"timestamp,omitempty"`
	Model           string         `json:"-"` // Not serialized; populated by parsers
	Provider        string         `json:"-"` // Recorded provider (Codex rollouts, Kiro IDE); not part of the source wire format
	RecordID        string         `json:"-"` // Identifier of the single source record this entry is built from (Decision 32)
	Origin          string         `json:"-"` // Claude Code entrypoint, decoded by UnmarshalJSON; feeds ParseResultMetadata.Origin
	Turn            int            `json:"-"` // 1-based Codex exec turn; 0 = no turn
	SessionID       string         `json:"sessionId,omitempty"`
	Cwd             string         `json:"cwd,omitempty"`             // Working directory for this entry
	IsMeta          bool           `json:"isMeta,omitempty"`          // Meta entries are internal Claude markers
	UUID            string         `json:"uuid,omitempty"`            // Unique identifier for this entry
	ParentUUID      string         `json:"parentUuid,omitempty"`      // Links to parent entry's UUID
	SourceToolUseID string         `json:"sourceToolUseID,omitempty"` // Links meta entry to originating tool_use
	ToolUseResult   *ToolUseResult `json:"toolUseResult,omitempty"`   // Result metadata for tool calls
}

// ToolUseResult contains metadata about a tool execution result.
type ToolUseResult struct {
	FilePath        string      `json:"filePath,omitempty"`
	StructuredPatch []PatchHunk `json:"structuredPatch,omitempty"`
}

// UnmarshalJSON handles polymorphic toolUseResult field (string or object).
// Some tool results have toolUseResult as a string, others as an object.
func (e *Entry) UnmarshalJSON(data []byte) error {
	// Use an alias to avoid infinite recursion
	type entryAlias struct {
		Type            string          `json:"type"`
		Message         *Message        `json:"message,omitempty"`
		Timestamp       string          `json:"timestamp,omitempty"`
		SessionID       string          `json:"sessionId,omitempty"`
		Cwd             string          `json:"cwd,omitempty"`
		IsMeta          bool            `json:"isMeta,omitempty"`
		UUID            string          `json:"uuid,omitempty"`
		ParentUUID      string          `json:"parentUuid,omitempty"`
		SourceToolUseID string          `json:"sourceToolUseID,omitempty"`
		ToolUseResult   json.RawMessage `json:"toolUseResult,omitempty"`
		Entrypoint      string          `json:"entrypoint,omitempty"`
	}

	var alias entryAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}

	e.Type = alias.Type
	e.Message = alias.Message
	e.Timestamp = alias.Timestamp
	e.SessionID = alias.SessionID
	e.Cwd = alias.Cwd
	e.IsMeta = alias.IsMeta
	e.UUID = alias.UUID
	e.ParentUUID = alias.ParentUUID
	e.SourceToolUseID = alias.SourceToolUseID
	e.Origin = alias.Entrypoint

	// Handle toolUseResult which can be string or object
	if len(alias.ToolUseResult) > 0 {
		// Try to unmarshal as object first
		var result ToolUseResult
		if err := json.Unmarshal(alias.ToolUseResult, &result); err == nil {
			e.ToolUseResult = &result
		}
		// If it's a string or fails to parse as object, leave ToolUseResult as nil
	}

	return nil
}

// PatchHunk represents a single hunk in a unified diff.
type PatchHunk struct {
	OldStart int      `json:"oldStart"`
	OldLines int      `json:"oldLines"`
	NewStart int      `json:"newStart"`
	NewLines int      `json:"newLines"`
	Lines    []string `json:"lines"`
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
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	Thinking  string `json:"thinking,omitempty"`
	Name      string `json:"name,omitempty"`
	Input     any    `json:"input,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
	ID        string `json:"id,omitempty"`          // tool_use ID for linking to tool_result
	ToolUseID string `json:"tool_use_id,omitempty"` // links tool_result to tool_use

	// Fields below are set by the Codex exec parser and are not part of any
	// source wire format, so Claude unmarshalling is unaffected. Type "error"
	// uses Text for its message.
	ItemID     string          `json:"-"` // source item identifier
	Status     string          `json:"-"` // source item status as recorded
	Incomplete bool            `json:"-"` // the item started but never completed
	ExitCode   *int            `json:"-"` // command exit code; nil when unrecorded
	Plan       []PlanItem      `json:"-"` // Type "plan"
	EventType  string          `json:"-"` // Type "event": the source event or item type
	Payload    json.RawMessage `json:"-"` // Type "event" and "error": the source JSON, compact
	Usage      *Usage          `json:"-"` // Type "usage"
}

// PlanItem is one step of a plan, such as a Codex exec to-do list.
type PlanItem struct {
	Text      string
	Completed bool
}

// UsageBasisThreadTotal labels Codex 0.166.0 `turn.completed` usage: reported
// thread totals at the turn's completion, not per-turn deltas.
const UsageBasisThreadTotal = "thread_total_at_turn_completion"

// Usage is token usage as a source reports it.
type Usage struct {
	Basis    string         // what the counters measure, such as UsageBasisThreadTotal
	Counters []UsageCounter // source key order; unknown counters kept
}

// UsageCounter is one usage counter. Value is the raw JSON number, so
// precision and unknown value kinds survive.
type UsageCounter struct {
	Name  string
	Value json.RawMessage
}

// UnmarshalJSON handles polymorphic content field (string or array).
// Tool results can have content as either a string or an array of content blocks.
func (c *ContentItem) UnmarshalJSON(data []byte) error {
	// Use an alias to avoid infinite recursion
	type contentItemAlias struct {
		Type      string `json:"type"`
		Text      string `json:"text,omitempty"`
		Thinking  string `json:"thinking,omitempty"`
		Name      string `json:"name,omitempty"`
		Input     any    `json:"input,omitempty"`
		Content   any    `json:"content,omitempty"`
		IsError   bool   `json:"is_error,omitempty"`
		ID        string `json:"id,omitempty"`
		ToolUseID string `json:"tool_use_id,omitempty"`
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
	c.ID = alias.ID
	c.ToolUseID = alias.ToolUseID

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

// NavigationContext provides previous/next phase links for transcript navigation.
type NavigationContext struct {
	PrevURL  string // URL for previous phase (empty if no previous)
	PrevText string // Display text for previous link (e.g., "Phase 1")
	NextURL  string // URL for next phase (empty if no next)
	NextText string // Display text for next link (e.g., "Phase 3")
	BackURL  string // URL to return to run detail page
	BackText string // Display text for back link (e.g., "Back to Run")
}

// RenderOptions configures Markdown rendering.
type RenderOptions struct {
	Title      string             // Document title (e.g., "Session Transcript" or "Phase 1 Session Transcript")
	SessionID  string             // Session ID to display in header
	ThreadID   string             // Codex exec thread ID to display in header
	ProjectDir string             // Project directory to strip from file paths (e.g., "/Users/foo/project")
	Navigation *NavigationContext // Optional navigation context for prev/next/back links
	TotalCost  *float64           // nil = don't display, pointer = display if > 0.005
	CostUnit   string             // e.g., "credits" (default if empty)
}
