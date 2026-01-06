package transcript

import "encoding/json"

// CodexEntry represents a single line in Codex JSONL.
type CodexEntry struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"` // session_meta, response_item, event_msg, turn_context
	Payload   json.RawMessage `json:"payload"`
}

// CodexResponseItem is the payload for response_item entries.
type CodexResponseItem struct {
	Type      string         `json:"type"`                // message, function_call, function_call_output, reasoning, ghost_snapshot
	Role      string         `json:"role,omitempty"`      // user, assistant (for message type)
	Name      string         `json:"name,omitempty"`      // function name (for function_call)
	Arguments string         `json:"arguments,omitempty"` // JSON string of arguments (for function_call)
	CallID    string         `json:"call_id,omitempty"`   // links function_call and function_call_output
	Output    string         `json:"output,omitempty"`    // function output (for function_call_output)
	Content   []CodexContent `json:"content,omitempty"`   // message content items
	Summary   []CodexSummary `json:"summary,omitempty"`   // reasoning summary items
}

// CodexContent represents content items in Codex messages.
type CodexContent struct {
	Type string `json:"type"` // input_text, output_text
	Text string `json:"text"`
}

// CodexSummary represents reasoning summary items.
type CodexSummary struct {
	Type string `json:"type"` // summary_text
	Text string `json:"text"`
}

// CodexEventMsg is the payload for event_msg entries.
type CodexEventMsg struct {
	Type    string `json:"type"`              // agent_reasoning, agent_message, user_message, token_count
	Text    string `json:"text,omitempty"`    // for agent_reasoning
	Message string `json:"message,omitempty"` // for agent_message
}

// CodexSessionMeta is the payload for session_meta entries.
type CodexSessionMeta struct {
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Cwd       string `json:"cwd"`
}
