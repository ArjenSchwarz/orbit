// Package sharedjson projects apsis's shared transcript representation into
// versioned, harness-neutral JSON documents described by
// docs/schemas/apsis-shared-json.v1.schema.json.
package sharedjson

import (
	"encoding/json"
	"io"
)

// SchemaVersion is the published schema version. Removing a field or changing
// its meaning raises it; additive fields and block types do not.
const SchemaVersion = 1

// SessionDocument is a session: metadata, ordered entries and parse warnings.
type SessionDocument struct {
	SchemaVersion int       `json:"schema_version"`
	Kind          string    `json:"kind"`
	Session       Metadata  `json:"session"`
	Entries       []Entry   `json:"entries"`
	Warnings      []Warning `json:"warnings"`
}

// Metadata describes a session. Unavailable values are omitted.
type Metadata struct {
	Source    string `json:"source,omitempty"`
	ID        string `json:"id,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	Project   string `json:"project,omitempty"`
	Model     string `json:"model,omitempty"`
	Provider  string `json:"provider,omitempty"`
	Origin    string `json:"origin,omitempty"`
	Cost      *Cost  `json:"cost,omitempty"`
}

// Cost is a session's total cost in its unit.
type Cost struct {
	Amount float64 `json:"amount"`
	Unit   string  `json:"unit"`
}

// Entry is one entry in transcript order.
type Entry struct {
	ID        string `json:"id,omitempty"` // source record identifier, not a tool link
	Role      string `json:"role"`
	Timestamp string `json:"timestamp,omitempty"`
	Model     string `json:"model,omitempty"`
	Provider  string `json:"provider,omitempty"`
	Content   []any  `json:"content"` // one of the *Block types
}

// TextBlock is a text or thinking block.
type TextBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ToolUseBlock is a tool call.
type ToolUseBlock struct {
	Type  string `json:"type"`
	ID    string `json:"id,omitempty"`
	Name  string `json:"name"`
	Input any    `json:"input,omitempty"`
}

// ToolResultBlock is a tool result, linked to its call by ToolUseID.
type ToolResultBlock struct {
	Type      string `json:"type"`
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error"`
}

// OtherBlock carries a parsed content kind the schema has no block type for.
type OtherBlock struct {
	Type       string `json:"type"`
	SourceType string `json:"source_type,omitempty"`
	Text       string `json:"text,omitempty"`
}

// Warning is a non-fatal problem with at most one one-based source position.
type Warning struct {
	Code            string `json:"code"`
	Message         string `json:"message"`
	Line            int    `json:"line,omitempty"`
	ChatMessage     int    `json:"chat_message,omitempty"`
	ExecutionAction int    `json:"execution_action,omitempty"`
}

// Write encodes doc with a two-space indent, no HTML escaping and a trailing newline.
func Write(w io.Writer, doc any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(doc)
}
