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

// KiroIDEExecutionDetail represents the execution detail file (for cost extraction).
type KiroIDEExecutionDetail struct {
	ExecutionID  string                 `json:"executionId"`
	UsageSummary []KiroIDEUsageSummary  `json:"usageSummary"`
}

// KiroIDEUsageSummary represents a single usage entry in the execution detail file.
type KiroIDEUsageSummary struct {
	Unit       string  `json:"unit"`
	UnitPlural string  `json:"unitPlural"`
	Usage      float64 `json:"usage"` // Note: "usage" not "value" (differs from Kiro CLI)
}
