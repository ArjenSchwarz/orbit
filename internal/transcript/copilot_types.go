package transcript

// CopilotEvent represents a single event line in the Copilot JSONL format.
type CopilotEvent struct {
	Type      string          `json:"type"`
	Data      CopilotData     `json:"data"`
	ID        string          `json:"id"`
	Timestamp string          `json:"timestamp"`
	ParentID  *string         `json:"parentId"`
}

// CopilotData holds the polymorphic data payload for different event types.
// Different event types use different fields.
type CopilotData struct {
	// session.start fields
	SessionID     string `json:"sessionId,omitempty"`
	Version       int    `json:"version,omitempty"`
	Producer      string `json:"producer,omitempty"`
	CopilotVersion string `json:"copilotVersion,omitempty"`
	StartTime     string `json:"startTime,omitempty"`

	// session.info fields
	InfoType string `json:"infoType,omitempty"`
	Message  string `json:"message,omitempty"`

	// user.message fields
	Content            string               `json:"content,omitempty"`
	TransformedContent string               `json:"transformedContent,omitempty"`
	Attachments        []CopilotAttachment  `json:"attachments,omitempty"`

	// assistant.turn_start / assistant.turn_end fields
	TurnID string `json:"turnId,omitempty"`

	// assistant.message fields
	MessageID    string              `json:"messageId,omitempty"`
	ToolRequests []CopilotToolRequest `json:"toolRequests,omitempty"`

	// assistant.reasoning fields
	ReasoningID string `json:"reasoningId,omitempty"`

	// tool.execution_start fields
	ToolCallID string         `json:"toolCallId,omitempty"`
	ToolName   string         `json:"toolName,omitempty"`
	Arguments  map[string]any `json:"arguments,omitempty"`

	// tool.execution_complete fields
	Success       bool                 `json:"success,omitempty"`
	Result        *CopilotToolResult   `json:"result,omitempty"`
	ToolTelemetry *CopilotToolTelemetry `json:"toolTelemetry,omitempty"`
}

// CopilotAttachment represents a file attachment in a user message.
type CopilotAttachment struct {
	Path string `json:"path,omitempty"`
	Type string `json:"type,omitempty"`
}

// CopilotToolRequest represents a tool call request in an assistant message.
type CopilotToolRequest struct {
	ToolCallID string         `json:"toolCallId"`
	Name       string         `json:"name"`
	Arguments  map[string]any `json:"arguments,omitempty"`
}

// CopilotToolResult contains the result of a tool execution.
type CopilotToolResult struct {
	Content string `json:"content,omitempty"`
}

// CopilotToolTelemetry contains telemetry data for tool executions.
type CopilotToolTelemetry struct {
	Properties map[string]string  `json:"properties,omitempty"`
	Metrics    map[string]float64 `json:"metrics,omitempty"`
}
