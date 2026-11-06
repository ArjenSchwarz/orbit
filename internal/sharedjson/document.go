// Package sharedjson projects apsis's shared transcript representation into
// versioned, harness-neutral JSON documents described by
// docs/schemas/apsis-shared-json.v1.schema.json.
package sharedjson

import (
	"encoding/json"
	"io"

	"github.com/arjenschwarz/orbit/internal/transcript"
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
	ThreadID  string `json:"thread_id,omitempty"` // Codex exec streams only
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
	Turn      int    `json:"turn,omitempty"` // 1-based Codex exec turn
	Content   []any  `json:"content"`        // one of the *Block types
}

// BlockCommon holds the fields any block may carry.
type BlockCommon struct {
	ItemID     string `json:"item_id,omitempty"`
	Status     string `json:"status,omitempty"`
	Incomplete bool   `json:"incomplete,omitempty"`
}

// TextBlock is a text or thinking block.
type TextBlock struct {
	Type string `json:"type"`
	BlockCommon
	Text string `json:"text"`
}

// ToolUseBlock is a tool call.
type ToolUseBlock struct {
	Type string `json:"type"`
	BlockCommon
	ID    string `json:"id,omitempty"`
	Name  string `json:"name"`
	Input any    `json:"input,omitempty"`
}

// ToolResultBlock is a tool result, linked to its call by ToolUseID.
type ToolResultBlock struct {
	Type string `json:"type"`
	BlockCommon
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error"`
	ExitCode  *int   `json:"exit_code,omitempty"` // emitted for 0; omitted only when unrecorded
}

// PlanBlock is a plan, such as a Codex exec to-do list.
type PlanBlock struct {
	Type string `json:"type"`
	BlockCommon
	Items []PlanItem `json:"items"`
}

// PlanItem is one step of a plan.
type PlanItem struct {
	Text      string `json:"text"`
	Completed bool   `json:"completed"`
}

// ErrorBlock is an error the source recorded.
type ErrorBlock struct {
	Type string `json:"type"`
	BlockCommon
	Message string `json:"message"`
}

// EventBlock keeps an event or item without a specialised translation.
type EventBlock struct {
	Type string `json:"type"`
	BlockCommon
	EventType string          `json:"event_type"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// UsageBlock is token usage as the source reports it.
type UsageBlock struct {
	Type string `json:"type"`
	BlockCommon
	Basis    string   `json:"basis"`
	Counters Counters `json:"counters"`
}

// Counters is a usage counter object written in source key order, with each
// value as recorded.
type Counters []transcript.UsageCounter

// MarshalJSON writes the counters as one object, preserving their order.
func (c Counters) MarshalJSON() ([]byte, error) {
	buf := []byte{'{'}
	for i, counter := range c {
		if i > 0 {
			buf = append(buf, ',')
		}
		name, err := json.Marshal(counter.Name)
		if err != nil {
			return nil, err
		}
		buf = append(buf, name...)
		buf = append(buf, ':')
		buf = append(buf, counter.Value...)
	}
	return append(buf, '}'), nil
}

// OtherBlock carries a parsed content kind the schema has no block type for.
type OtherBlock struct {
	Type string `json:"type"`
	BlockCommon
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
