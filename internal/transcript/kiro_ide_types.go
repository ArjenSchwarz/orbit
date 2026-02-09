package transcript

// KiroIDEChatFile represents the top-level structure of a Kiro IDE .chat file.
// Fields like actionId, context, and validations are present in the source
// but not used for transcript conversion — they are omitted from this struct.
type KiroIDEChatFile struct {
	ExecutionID string            `json:"executionId"`
	Chat        []KiroIDEMessage  `json:"chat"`
	Metadata    *KiroIDEMetadata  `json:"metadata"`
}

// KiroIDEMessage represents a single message in the chat array.
type KiroIDEMessage struct {
	Role    string `json:"role"`    // "human", "bot", or "tool"
	Content string `json:"content"`
}

// KiroIDEMetadata contains session metadata.
type KiroIDEMetadata struct {
	ModelID       string `json:"modelId"`
	ModelProvider string `json:"modelProvider"`
	Workflow      string `json:"workflow"`
	WorkflowID    string `json:"workflowId"`
	StartTime     int64  `json:"startTime"` // milliseconds since epoch
	EndTime       int64  `json:"endTime"`   // milliseconds since epoch
}

// KiroIDEAction represents a single action in the execution detail's actions array.
// Actions encode tool calls (readFiles, replace, create, runCommand, search),
// assistant messages (say), task updates (taskStatus), user interactions (userInput),
// and internal operations (model, steering, intentClassification, specAgent).
type KiroIDEAction struct {
	ActionID     string         `json:"actionId"`
	ActionType   string         `json:"actionType"`
	ActionState  string         `json:"actionState"`
	Input        map[string]any `json:"input,omitempty"`
	Output       map[string]any `json:"output,omitempty"`
	ErrorMessage string         `json:"errorMessage,omitempty"`
	TaskID       string         `json:"taskId,omitempty"`
	TaskStatus   string         `json:"taskStatus,omitempty"`
	TaskListURI  string         `json:"taskListUri,omitempty"`
}

// KiroIDEExecutionDetail represents the execution detail file (for cost and action extraction).
type KiroIDEExecutionDetail struct {
	ExecutionID  string                `json:"executionId"`
	UsageSummary []KiroIDEUsageSummary `json:"usageSummary"`
	Actions      []KiroIDEAction       `json:"actions"`
}

// KiroIDEUsageSummary represents a single usage entry in the execution detail file.
type KiroIDEUsageSummary struct {
	Unit       string  `json:"unit"`
	UnitPlural string  `json:"unitPlural"`
	Usage      float64 `json:"usage"` // Note: "usage" not "value" (differs from Kiro CLI)
}
