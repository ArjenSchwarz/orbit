package transcript

import (
	"strings"
	"testing"
)

func TestFormatKiroJsonOutput(t *testing.T) {
	tests := map[string]struct {
		input *KiroJsonOutput
		want  string
	}{
		"stdout only": {
			input: &KiroJsonOutput{
				ExitStatus: "0",
				Stdout:     "hello world",
				Stderr:     "",
			},
			want: "hello world",
		},
		"stderr only": {
			input: &KiroJsonOutput{
				ExitStatus: "0",
				Stdout:     "",
				Stderr:     "error message",
			},
			want: "stderr: error message",
		},
		"stdout and stderr": {
			input: &KiroJsonOutput{
				ExitStatus: "0",
				Stdout:     "output",
				Stderr:     "warning",
			},
			want: "output\nstderr: warning",
		},
		"non-zero exit status": {
			input: &KiroJsonOutput{
				ExitStatus: "1",
				Stdout:     "",
				Stderr:     "command failed",
			},
			want: "stderr: command failed\n[exit: 1]",
		},
		"full output with non-zero exit": {
			input: &KiroJsonOutput{
				ExitStatus: "127",
				Stdout:     "partial output",
				Stderr:     "command not found",
			},
			want: "partial output\nstderr: command not found\n[exit: 127]",
		},
		"empty output with zero exit": {
			input: &KiroJsonOutput{
				ExitStatus: "0",
				Stdout:     "",
				Stderr:     "",
			},
			want: "",
		},
		"empty exit status": {
			input: &KiroJsonOutput{
				ExitStatus: "",
				Stdout:     "output",
				Stderr:     "",
			},
			want: "output",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := formatKiroJsonOutput(tc.input)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseKiroJsonVariant(t *testing.T) {
	input := `{
		"conversation_id": "test-json-variant",
		"history": [
			{
				"assistant": {
					"ToolUse": {
						"message_id": "msg-1",
						"content": "Running command",
						"tool_uses": [
							{
								"id": "tool-1",
								"name": "Bash",
								"orig_name": "Bash",
								"args": {"command": "echo hello"},
								"orig_args": {"command": "echo hello"}
							}
						]
					}
				}
			},
			{
				"user": {
					"additional_context": "",
					"content": {
						"ToolUseResults": {
							"tool_use_results": [
								{
									"tool_use_id": "tool-1",
									"content": [
										{
											"Json": {
												"exit_status": "0",
												"stdout": "hello",
												"stderr": ""
											}
										}
									],
									"status": "Success"
								}
							]
						}
					},
					"timestamp": null,
					"images": []
				}
			}
		]
	}`

	result, err := ParseKiro(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result.Entries))
	}

	// Check that the tool result contains the formatted Json output
	userEntry := result.Entries[1]
	if userEntry.Message == nil || len(userEntry.Message.Content) == 0 {
		t.Fatal("user entry has no content")
	}

	toolResult := userEntry.Message.Content[0]
	if toolResult.Type != "tool_result" {
		t.Errorf("expected tool_result type, got %s", toolResult.Type)
	}
	if toolResult.Content != "hello" {
		t.Errorf("expected content 'hello', got %q", toolResult.Content)
	}
}

func TestParseKiroJsonVariantWithError(t *testing.T) {
	input := `{
		"conversation_id": "test-json-error",
		"history": [
			{
				"user": {
					"additional_context": "",
					"content": {
						"ToolUseResults": {
							"tool_use_results": [
								{
									"tool_use_id": "tool-1",
									"content": [
										{
											"Json": {
												"exit_status": "1",
												"stdout": "",
												"stderr": "command not found"
											}
										}
									],
									"status": "Success"
								}
							]
						}
					},
					"timestamp": null,
					"images": []
				}
			}
		]
	}`

	result, err := ParseKiro(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.Entries))
	}

	userEntry := result.Entries[0]
	toolResult := userEntry.Message.Content[0]
	expected := "stderr: command not found\n[exit: 1]"
	if toolResult.Content != expected {
		t.Errorf("expected content %q, got %q", expected, toolResult.Content)
	}
}

func TestParseKiroMixedTextAndJson(t *testing.T) {
	input := `{
		"conversation_id": "test-mixed",
		"history": [
			{
				"user": {
					"additional_context": "",
					"content": {
						"ToolUseResults": {
							"tool_use_results": [
								{
									"tool_use_id": "tool-1",
									"content": [
										{"Text": "Prefix text"},
										{
											"Json": {
												"exit_status": "0",
												"stdout": "command output",
												"stderr": ""
											}
										}
									],
									"status": "Success"
								}
							]
						}
					},
					"timestamp": null,
					"images": []
				}
			}
		]
	}`

	result, err := ParseKiro(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	userEntry := result.Entries[0]
	toolResult := userEntry.Message.Content[0]
	expected := "Prefix text\ncommand output"
	if toolResult.Content != expected {
		t.Errorf("expected content %q, got %q", expected, toolResult.Content)
	}
}

func TestParseKiroCostMetadata(t *testing.T) {
	tests := map[string]struct {
		input        string
		wantMetadata bool
		wantCost     float64
		wantUnit     string
	}{
		"session with credits": {
			input: `{
				"conversation_id": "test-cost",
				"history": [],
				"user_turn_metadata": {
					"continuation_id": "cont-1",
					"requests": [],
					"usage_info": [
						{"unit": "credit", "unit_plural": "credits", "value": 0.05},
						{"unit": "credit", "unit_plural": "credits", "value": 0.04}
					]
				}
			}`,
			wantMetadata: true,
			wantCost:     0.09,
			wantUnit:     "credits",
		},
		"session without user_turn_metadata": {
			input: `{
				"conversation_id": "test-no-meta",
				"history": []
			}`,
			wantMetadata: false,
		},
		"session with zero cost": {
			input: `{
				"conversation_id": "test-zero",
				"history": [],
				"user_turn_metadata": {
					"continuation_id": "cont-2",
					"requests": [],
					"usage_info": []
				}
			}`,
			wantMetadata: false,
		},
		"session with non-credit units only": {
			input: `{
				"conversation_id": "test-tokens",
				"history": [],
				"user_turn_metadata": {
					"continuation_id": "cont-3",
					"requests": [],
					"usage_info": [
						{"unit": "token", "unit_plural": "tokens", "value": 1000}
					]
				}
			}`,
			wantMetadata: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result, err := ParseKiro(strings.NewReader(tc.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.wantMetadata {
				if result.Metadata == nil {
					t.Fatal("expected metadata, got nil")
				}
				if result.Metadata.TotalCost == nil {
					t.Fatal("expected TotalCost, got nil")
				}
				if got := *result.Metadata.TotalCost; got < tc.wantCost-0.001 || got > tc.wantCost+0.001 {
					t.Errorf("TotalCost: got %.4f, want %.4f", got, tc.wantCost)
				}
				if result.Metadata.CostUnit != tc.wantUnit {
					t.Errorf("CostUnit: got %q, want %q", result.Metadata.CostUnit, tc.wantUnit)
				}
			} else {
				if result.Metadata != nil {
					t.Errorf("expected nil metadata, got %+v", result.Metadata)
				}
			}
		})
	}
}

func TestParseKiroUsageInfo(t *testing.T) {
	tests := map[string]struct {
		input   string
		want    float64
		wantErr bool
	}{
		"valid session with credits": {
			input: `{
				"conversation_id": "test-123",
				"history": [],
				"user_turn_metadata": {
					"continuation_id": "cont-1",
					"requests": [],
					"usage_info": [
						{"unit": "credit", "unit_plural": "credits", "value": 0.05},
						{"unit": "credit", "unit_plural": "credits", "value": 0.04}
					]
				}
			}`,
			want:    0.09,
			wantErr: false,
		},
		"session without user_turn_metadata": {
			input: `{
				"conversation_id": "test-456",
				"history": []
			}`,
			want:    0,
			wantErr: false,
		},
		"session with empty usage_info": {
			input: `{
				"conversation_id": "test-789",
				"history": [],
				"user_turn_metadata": {
					"continuation_id": "cont-2",
					"requests": [],
					"usage_info": []
				}
			}`,
			want:    0,
			wantErr: false,
		},
		"session with mixed unit types": {
			input: `{
				"conversation_id": "test-mixed",
				"history": [],
				"user_turn_metadata": {
					"continuation_id": "cont-3",
					"requests": [],
					"usage_info": [
						{"unit": "credit", "unit_plural": "credits", "value": 0.10},
						{"unit": "token", "unit_plural": "tokens", "value": 1000},
						{"unit": "credit", "unit_plural": "credits", "value": 0.05}
					]
				}
			}`,
			want:    0.15,
			wantErr: false,
		},
		"invalid JSON": {
			input:   `{invalid json`,
			want:    0,
			wantErr: true,
		},
		"empty input": {
			input:   ``,
			want:    0,
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := ParseKiroUsageInfo(strings.NewReader(tc.input))

			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Use tolerance for float comparison
			if got < tc.want-0.001 || got > tc.want+0.001 {
				t.Errorf("got %.4f, want %.4f", got, tc.want)
			}
		})
	}
}

func TestExtractKiroCredits(t *testing.T) {
	tests := map[string]struct {
		session *KiroSession
		want    float64
	}{
		"nil metadata": {
			session: &KiroSession{
				ConversationID:   "test-1",
				UserTurnMetadata: nil,
			},
			want: 0,
		},
		"empty usage info": {
			session: &KiroSession{
				ConversationID: "test-2",
				UserTurnMetadata: &KiroUserTurnMetadata{
					UsageInfo: []KiroUsageInfo{},
				},
			},
			want: 0,
		},
		"single credit entry": {
			session: &KiroSession{
				ConversationID: "test-3",
				UserTurnMetadata: &KiroUserTurnMetadata{
					UsageInfo: []KiroUsageInfo{
						{Unit: "credit", UnitPlural: "credits", Value: 0.123},
					},
				},
			},
			want: 0.123,
		},
		"multiple credit entries": {
			session: &KiroSession{
				ConversationID: "test-4",
				UserTurnMetadata: &KiroUserTurnMetadata{
					UsageInfo: []KiroUsageInfo{
						{Unit: "credit", UnitPlural: "credits", Value: 0.05},
						{Unit: "credit", UnitPlural: "credits", Value: 0.03},
						{Unit: "credit", UnitPlural: "credits", Value: 0.02},
					},
				},
			},
			want: 0.10,
		},
		"ignores non-credit units": {
			session: &KiroSession{
				ConversationID: "test-5",
				UserTurnMetadata: &KiroUserTurnMetadata{
					UsageInfo: []KiroUsageInfo{
						{Unit: "token", UnitPlural: "tokens", Value: 5000},
						{Unit: "credit", UnitPlural: "credits", Value: 0.08},
						{Unit: "dollar", UnitPlural: "dollars", Value: 1.50},
					},
				},
			},
			want: 0.08,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := extractKiroCredits(tc.session)

			// Use tolerance for float comparison
			if got < tc.want-0.0001 || got > tc.want+0.0001 {
				t.Errorf("got %.4f, want %.4f", got, tc.want)
			}
		})
	}
}

func TestParseKiro_UserTimestampExtraction(t *testing.T) {
	ts := "2026-01-17T12:00:01Z"
	input := `{
		"conversation_id": "test-ts",
		"history": [
			{
				"user": {
					"additional_context": "",
					"content": {"Prompt": {"prompt": "hello"}},
					"timestamp": "` + ts + `",
					"images": []
				},
				"assistant": {
					"TextResponse": {"content": "hi"}
				},
				"request_metadata": {
					"request_id": "r1",
					"request_start_timestamp_ms": 1768694401000,
					"stream_end_timestamp_ms": 1768694402000,
					"model_id": "some-model",
					"message_id": "m1",
					"user_prompt_length": 5,
					"response_size": 2,
					"chat_conversation_type": "chat",
					"tool_use_ids_and_names": [],
					"message_meta_tags": []
				}
			}
		]
	}`

	result, err := ParseKiro(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// First entry is user prompt
	if result.Entries[0].Type != "user" {
		t.Fatalf("expected user entry, got %s", result.Entries[0].Type)
	}
	if result.Entries[0].Timestamp != ts {
		t.Errorf("user timestamp: got %q, want %q", result.Entries[0].Timestamp, ts)
	}
}

func TestParseKiro_AssistantTimestampFromRequestMetadata(t *testing.T) {
	input := `{
		"conversation_id": "test-ts",
		"history": [
			{
				"user": {
					"additional_context": "",
					"content": {"Prompt": {"prompt": "hello"}},
					"timestamp": "2026-01-17T12:00:01Z",
					"images": []
				},
				"assistant": {
					"TextResponse": {"content": "hi"}
				},
				"request_metadata": {
					"request_id": "r1",
					"request_start_timestamp_ms": 1768694401000,
					"stream_end_timestamp_ms": 1768694402000,
					"model_id": "some-model",
					"message_id": "m1",
					"user_prompt_length": 5,
					"response_size": 2,
					"chat_conversation_type": "chat",
					"tool_use_ids_and_names": [],
					"message_meta_tags": []
				}
			}
		]
	}`

	result, err := ParseKiro(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Find assistant entry
	var assistant *Entry
	for i := range result.Entries {
		if result.Entries[i].Type == "assistant" {
			assistant = &result.Entries[i]
			break
		}
	}
	if assistant == nil {
		t.Fatal("no assistant entry found")
	}

	// 1768694401000 ms = 2026-01-18T00:00:01Z
	want := "2026-01-18T00:00:01Z"
	if assistant.Timestamp != want {
		t.Errorf("assistant timestamp: got %q, want %q", assistant.Timestamp, want)
	}
}

func TestParseKiro_AssistantModelFromRequestMetadata(t *testing.T) {
	input := `{
		"conversation_id": "test-model",
		"history": [
			{
				"user": {
					"additional_context": "",
					"content": {"Prompt": {"prompt": "hello"}},
					"timestamp": null,
					"images": []
				},
				"assistant": {
					"TextResponse": {"content": "hi"}
				},
				"request_metadata": {
					"request_id": "r1",
					"request_start_timestamp_ms": 1768694401000,
					"stream_end_timestamp_ms": 1768694402000,
					"model_id": "claude-sonnet-4-20250514",
					"message_id": "m1",
					"user_prompt_length": 5,
					"response_size": 2,
					"chat_conversation_type": "chat",
					"tool_use_ids_and_names": [],
					"message_meta_tags": []
				}
			}
		]
	}`

	result, err := ParseKiro(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var assistant *Entry
	for i := range result.Entries {
		if result.Entries[i].Type == "assistant" {
			assistant = &result.Entries[i]
			break
		}
	}
	if assistant == nil {
		t.Fatal("no assistant entry found")
	}

	if assistant.Model != "claude-sonnet-4-20250514" {
		t.Errorf("assistant model: got %q, want %q", assistant.Model, "claude-sonnet-4-20250514")
	}
}

func TestParseKiro_ZeroTimestampMs(t *testing.T) {
	// Zero RequestStartTimestampMs should produce no timestamp
	input := `{
		"conversation_id": "test-zero-ts",
		"history": [
			{
				"user": {
					"additional_context": "",
					"content": {"Prompt": {"prompt": "hello"}},
					"timestamp": null,
					"images": []
				},
				"assistant": {
					"TextResponse": {"content": "hi"}
				},
				"request_metadata": {
					"request_id": "r1",
					"request_start_timestamp_ms": 0,
					"stream_end_timestamp_ms": 0,
					"model_id": "some-model",
					"message_id": "m1",
					"user_prompt_length": 5,
					"response_size": 2,
					"chat_conversation_type": "chat",
					"tool_use_ids_and_names": [],
					"message_meta_tags": []
				}
			}
		]
	}`

	result, err := ParseKiro(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var assistant *Entry
	for i := range result.Entries {
		if result.Entries[i].Type == "assistant" {
			assistant = &result.Entries[i]
			break
		}
	}
	if assistant == nil {
		t.Fatal("no assistant entry found")
	}

	if assistant.Timestamp != "" {
		t.Errorf("expected empty timestamp for zero ms, got %q", assistant.Timestamp)
	}
}

func TestParseKiro_NegativeTimestampMs(t *testing.T) {
	// Negative RequestStartTimestampMs should produce no timestamp
	input := `{
		"conversation_id": "test-neg-ts",
		"history": [
			{
				"user": {
					"additional_context": "",
					"content": {"Prompt": {"prompt": "hello"}},
					"timestamp": null,
					"images": []
				},
				"assistant": {
					"TextResponse": {"content": "hi"}
				},
				"request_metadata": {
					"request_id": "r1",
					"request_start_timestamp_ms": -1,
					"stream_end_timestamp_ms": 0,
					"model_id": "some-model",
					"message_id": "m1",
					"user_prompt_length": 5,
					"response_size": 2,
					"chat_conversation_type": "chat",
					"tool_use_ids_and_names": [],
					"message_meta_tags": []
				}
			}
		]
	}`

	result, err := ParseKiro(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var assistant *Entry
	for i := range result.Entries {
		if result.Entries[i].Type == "assistant" {
			assistant = &result.Entries[i]
			break
		}
	}
	if assistant == nil {
		t.Fatal("no assistant entry found")
	}

	if assistant.Timestamp != "" {
		t.Errorf("expected empty timestamp for negative ms, got %q", assistant.Timestamp)
	}
}

func TestParseKiro_NoRequestMetadata(t *testing.T) {
	// Without request_metadata, assistant entries should have no timestamp or model
	input := `{
		"conversation_id": "test-no-meta",
		"history": [
			{
				"user": {
					"additional_context": "",
					"content": {"Prompt": {"prompt": "hello"}},
					"timestamp": null,
					"images": []
				},
				"assistant": {
					"TextResponse": {"content": "hi"}
				}
			}
		]
	}`

	result, err := ParseKiro(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var assistant *Entry
	for i := range result.Entries {
		if result.Entries[i].Type == "assistant" {
			assistant = &result.Entries[i]
			break
		}
	}
	if assistant == nil {
		t.Fatal("no assistant entry found")
	}

	if assistant.Timestamp != "" {
		t.Errorf("expected empty timestamp without metadata, got %q", assistant.Timestamp)
	}
	if assistant.Model != "" {
		t.Errorf("expected empty model without metadata, got %q", assistant.Model)
	}
}

func TestParseKiro_NullUserTimestamp(t *testing.T) {
	// null timestamp on user message should produce empty timestamp
	input := `{
		"conversation_id": "test-null-ts",
		"history": [
			{
				"user": {
					"additional_context": "",
					"content": {"Prompt": {"prompt": "hello"}},
					"timestamp": null,
					"images": []
				}
			}
		]
	}`

	result, err := ParseKiro(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Entries[0].Timestamp != "" {
		t.Errorf("expected empty timestamp for null, got %q", result.Entries[0].Timestamp)
	}
}

func TestParseKiro_UserModelNotSet(t *testing.T) {
	// User entries should never have Model set, even when request_metadata has model_id
	input := `{
		"conversation_id": "test-user-model",
		"history": [
			{
				"user": {
					"additional_context": "",
					"content": {"Prompt": {"prompt": "hello"}},
					"timestamp": "2026-01-17T12:00:01Z",
					"images": []
				},
				"assistant": {
					"TextResponse": {"content": "hi"}
				},
				"request_metadata": {
					"request_id": "r1",
					"request_start_timestamp_ms": 1768694401000,
					"stream_end_timestamp_ms": 1768694402000,
					"model_id": "claude-sonnet-4-20250514",
					"message_id": "m1",
					"user_prompt_length": 5,
					"response_size": 2,
					"chat_conversation_type": "chat",
					"tool_use_ids_and_names": [],
					"message_meta_tags": []
				}
			}
		]
	}`

	result, err := ParseKiro(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, entry := range result.Entries {
		if entry.Type == "user" && entry.Model != "" {
			t.Errorf("user entry should not have model, got %q", entry.Model)
		}
	}
}

func TestParseKiro_ToolUseResultsGetUserTimestamp(t *testing.T) {
	// Tool use results (user entries) should also get the user timestamp
	ts := "2026-01-17T12:00:05Z"
	input := `{
		"conversation_id": "test-tool-ts",
		"history": [
			{
				"user": {
					"additional_context": "",
					"content": {
						"ToolUseResults": {
							"tool_use_results": [
								{
									"tool_use_id": "tool-1",
									"content": [{"Text": "output"}],
									"status": "Success"
								}
							]
						}
					},
					"timestamp": "` + ts + `",
					"images": []
				}
			}
		]
	}`

	result, err := ParseKiro(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.Entries))
	}

	if result.Entries[0].Timestamp != ts {
		t.Errorf("tool result entry timestamp: got %q, want %q", result.Entries[0].Timestamp, ts)
	}
}
