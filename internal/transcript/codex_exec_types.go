package transcript

import "encoding/json"

// Codex exec item payloads, following the pinned Codex 0.166.0
// codex-rs/exec/src/exec_events.rs ThreadItemDetails variants. Pointer and
// slice fields distinguish a missing required field from its zero value;
// a field of the wrong JSON type fails decoding. Either makes the item a
// generic event with an item_decode_failed warning.

type execTextItem struct { // agent_message, reasoning
	Text *string `json:"text"`
}

type execCommandItem struct {
	Command          *string `json:"command"`
	AggregatedOutput string  `json:"aggregated_output"`
	ExitCode         *int    `json:"exit_code"`
	Status           string  `json:"status"`
}

type execFileChangeItem struct {
	Changes []execFileUpdate `json:"changes"`
	Status  string           `json:"status"`
}

type execFileUpdate struct {
	Path *string `json:"path"`
	Kind *string `json:"kind"`
}

type execMCPToolCallItem struct {
	Server    *string         `json:"server"`
	Tool      *string         `json:"tool"`
	Arguments json.RawMessage `json:"arguments"`
	Result    json.RawMessage `json:"result"`
	Error     *execMCPError   `json:"error"`
	Status    string          `json:"status"`
}

type execMCPError struct {
	Message string `json:"message"`
}

type execWebSearchItem struct {
	Query   *string         `json:"query"`
	Action  json.RawMessage `json:"action"`
	Results json.RawMessage `json:"results"`
}

type execTodoListItem struct {
	Items []execTodoItem `json:"items"`
}

type execTodoItem struct {
	Text      *string `json:"text"`
	Completed *bool   `json:"completed"`
}

type execErrorItem struct {
	Message *string `json:"message"`
}
