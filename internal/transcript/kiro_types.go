package transcript

// KiroSession represents the top-level Kiro session JSON structure.
type KiroSession struct {
	ConversationID   string                `json:"conversation_id"`
	NextMessage      *KiroHistoryEntry     `json:"next_message"`
	History          []KiroHistoryEntry    `json:"history"`
	UserTurnMetadata *KiroUserTurnMetadata `json:"user_turn_metadata,omitempty"`
}

// KiroUserTurnMetadata contains session-level metadata including usage info.
type KiroUserTurnMetadata struct {
	ContinuationID string          `json:"continuation_id"`
	Requests       []any           `json:"requests"`
	UsageInfo      []KiroUsageInfo `json:"usage_info"`
}

// KiroUsageInfo represents usage/cost information for a session.
type KiroUsageInfo struct {
	Unit       string  `json:"unit"`        // e.g., "credit"
	UnitPlural string  `json:"unit_plural"` // e.g., "credits"
	Value      float64 `json:"value"`       // e.g., 0.09024116169154228
}

// KiroHistoryEntry represents a single exchange in the Kiro history.
type KiroHistoryEntry struct {
	User            *KiroUserMessage      `json:"user,omitempty"`
	Assistant       *KiroAssistantMessage `json:"assistant,omitempty"`
	RequestMetadata *KiroRequestMetadata  `json:"request_metadata,omitempty"`
}

// KiroUserMessage represents a user message in Kiro format.
type KiroUserMessage struct {
	AdditionalContext string          `json:"additional_context"`
	EnvContext        *KiroEnvContext `json:"env_context,omitempty"`
	Content           KiroUserContent `json:"content"`
	Timestamp         *string         `json:"timestamp"`
	Images            []KiroImage     `json:"images"`
}

// KiroEnvContext contains environment state information.
type KiroEnvContext struct {
	EnvState *KiroEnvState `json:"env_state,omitempty"`
}

// KiroEnvState contains OS and directory information.
type KiroEnvState struct {
	OperatingSystem         string   `json:"operating_system"`
	CurrentWorkingDirectory string   `json:"current_working_directory"`
	EnvironmentVariables    []string `json:"environment_variables"`
}

// KiroUserContent represents user content which can be either a Prompt or ToolUseResults.
// Uses Go's interface to handle the discriminated union.
type KiroUserContent struct {
	Prompt         *KiroPrompt         `json:"Prompt,omitempty"`
	ToolUseResults *KiroToolUseResults `json:"ToolUseResults,omitempty"`
}

// KiroPrompt contains the user's text prompt.
type KiroPrompt struct {
	Prompt string `json:"prompt"`
}

// KiroToolUseResults contains results from tool executions.
type KiroToolUseResults struct {
	ToolUseResults []KiroToolUseResult `json:"tool_use_results"`
}

// KiroToolUseResult represents a single tool result.
type KiroToolUseResult struct {
	ToolUseID string              `json:"tool_use_id"`
	Content   []KiroResultContent `json:"content"`
	Status    string              `json:"status"` // "Success", "Error", etc.
}

// KiroResultContent represents content in a tool result.
type KiroResultContent struct {
	Text string `json:"Text,omitempty"`
}

// KiroImage represents an image attachment.
type KiroImage struct {
	Source string `json:"source,omitempty"`
}

// KiroAssistantMessage represents an assistant response in Kiro format.
// It can be ToolUse (with tool calls), TextResponse (text only), or Response (also text only).
type KiroAssistantMessage struct {
	ToolUse      *KiroToolUse      `json:"ToolUse,omitempty"`
	TextResponse *KiroTextResponse `json:"TextResponse,omitempty"`
	Response     *KiroTextResponse `json:"Response,omitempty"`
}

// KiroToolUse represents an assistant response that includes tool calls.
type KiroToolUse struct {
	MessageID string         `json:"message_id"`
	Content   string         `json:"content"`
	ToolUses  []KiroToolCall `json:"tool_uses"`
}

// KiroToolCall represents a single tool invocation.
type KiroToolCall struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	OrigName string         `json:"orig_name"`
	Args     map[string]any `json:"args"`
	OrigArgs map[string]any `json:"orig_args"`
}

// KiroTextResponse represents a text-only assistant response.
type KiroTextResponse struct {
	MessageID string `json:"message_id"`
	Content   string `json:"content"`
}

// KiroRequestMetadata contains telemetry and metadata about the request.
type KiroRequestMetadata struct {
	RequestID               string         `json:"request_id"`
	ContextUsagePercentage  *float64       `json:"context_usage_percentage"`
	MessageID               string         `json:"message_id"`
	RequestStartTimestampMs int64          `json:"request_start_timestamp_ms"`
	StreamEndTimestampMs    int64          `json:"stream_end_timestamp_ms"`
	TimeToFirstChunk        *KiroDuration  `json:"time_to_first_chunk,omitempty"`
	TimeBetweenChunks       []KiroDuration `json:"time_between_chunks,omitempty"`
	UserPromptLength        int            `json:"user_prompt_length"`
	ResponseSize            int            `json:"response_size"`
	ChatConversationType    string         `json:"chat_conversation_type"`
	ToolUseIDsAndNames      [][]string     `json:"tool_use_ids_and_names"`
	ModelID                 string         `json:"model_id"`
	MessageMetaTags         []string       `json:"message_meta_tags"`
}

// KiroDuration represents a duration with seconds and nanoseconds.
type KiroDuration struct {
	Secs  int64 `json:"secs"`
	Nanos int64 `json:"nanos"`
}
