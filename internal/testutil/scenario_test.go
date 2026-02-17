package testutil

import (
	"testing"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
)

func TestScenarioBuilder_Immutability(t *testing.T) {
	builder := NewScenario().
		Success("session-1", 0.05).
		Success("session-2", 0.03)

	scenario := builder.Build()

	// Verify scenario has correct count
	if scenario.Len() != 2 {
		t.Fatalf("expected 2 responses, got %d", scenario.Len())
	}

	// Modify the builder after Build()
	builder.Success("session-3", 0.01)

	// Verify scenario is unchanged (immutable)
	if scenario.Len() != 2 {
		t.Fatalf("expected scenario to remain at 2 responses, got %d", scenario.Len())
	}
}

func TestScenarioBuilder_Chaining(t *testing.T) {
	// Test that all methods return the same builder for chaining
	builder := NewScenario()

	result := builder.Success("session-1", 0.05)
	if result != builder {
		t.Fatal("Success should return the same builder")
	}

	result = builder.WithDelay(time.Second)
	if result != builder {
		t.Fatal("WithDelay should return the same builder")
	}

	result = builder.WithOutput("output", "stderr")
	if result != builder {
		t.Fatal("WithOutput should return the same builder")
	}

	result = builder.Repeat(3)
	if result != builder {
		t.Fatal("Repeat should return the same builder")
	}
}

func TestScenarioBuilder_Success(t *testing.T) {
	scenario := NewScenario().
		Success("session-abc", 0.05).
		Build()

	if scenario.Len() != 1 {
		t.Fatalf("expected 1 response, got %d", scenario.Len())
	}

	resp := scenario.Responses()[0]
	if resp.Result == nil {
		t.Fatal("expected Result to be non-nil")
	}
	if resp.Result.SessionID != "session-abc" {
		t.Fatalf("expected session ID 'session-abc', got %q", resp.Result.SessionID)
	}
	if resp.Result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", resp.Result.ExitCode)
	}
	if resp.Result.IsError {
		t.Fatal("expected IsError to be false")
	}
	if resp.Result.Cost == nil || resp.Result.Cost.CostUSD != 0.05 {
		t.Fatalf("expected cost 0.05, got %v", resp.Result.Cost)
	}
}

func TestScenarioBuilder_RetryableError(t *testing.T) {
	scenario := NewScenario().
		RetryableError("connection timeout").
		Build()

	resp := scenario.Responses()[0]
	if resp.Result == nil {
		t.Fatal("expected Result to be non-nil")
	}
	if resp.Result.ExitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}
	if !resp.Result.IsError {
		t.Fatal("expected IsError to be true")
	}
	if resp.ErrorClass != agents.ErrorClassRetryable {
		t.Fatalf("expected ErrorClassRetryable, got %v", resp.ErrorClass)
	}
	if resp.Result.Stderr != "connection timeout" {
		t.Fatalf("expected stderr 'connection timeout', got %q", resp.Result.Stderr)
	}
}

func TestScenarioBuilder_FatalError(t *testing.T) {
	scenario := NewScenario().
		FatalError("authentication failed").
		Build()

	resp := scenario.Responses()[0]
	if resp.ErrorClass != agents.ErrorClassFatal {
		t.Fatalf("expected ErrorClassFatal, got %v", resp.ErrorClass)
	}
}

func TestScenarioBuilder_SessionInvalid(t *testing.T) {
	scenario := NewScenario().
		SessionInvalid().
		Build()

	resp := scenario.Responses()[0]
	if resp.ErrorClass != agents.ErrorClassSessionInvalid {
		t.Fatalf("expected ErrorClassSessionInvalid, got %v", resp.ErrorClass)
	}
}

func TestScenarioBuilder_RateLimitWait(t *testing.T) {
	scenario := NewScenario().
		RateLimitWait(5 * time.Minute).
		Build()

	resp := scenario.Responses()[0]
	if resp.ErrorClass != agents.ErrorClassRateLimitWait {
		t.Fatalf("expected ErrorClassRateLimitWait, got %v", resp.ErrorClass)
	}
	if resp.Delay != 5*time.Minute {
		t.Fatalf("expected delay of 5 minutes, got %v", resp.Delay)
	}
}

func TestScenarioBuilder_WithDelay(t *testing.T) {
	scenario := NewScenario().
		Success("session-1", 0.05).
		WithDelay(100 * time.Millisecond).
		Build()

	resp := scenario.Responses()[0]
	if resp.Delay != 100*time.Millisecond {
		t.Fatalf("expected delay of 100ms, got %v", resp.Delay)
	}
}

func TestScenarioBuilder_WithOutput(t *testing.T) {
	scenario := NewScenario().
		Success("session-1", 0.05).
		WithOutput("hello stdout", "hello stderr").
		Build()

	resp := scenario.Responses()[0]
	if resp.Output != "hello stdout" {
		t.Fatalf("expected output 'hello stdout', got %q", resp.Output)
	}
	if resp.Stderr != "hello stderr" {
		t.Fatalf("expected stderr 'hello stderr', got %q", resp.Stderr)
	}
}

func TestScenarioBuilder_WithCost(t *testing.T) {
	metrics := &agents.CostMetrics{
		InputTokens:  1000,
		OutputTokens: 500,
		CostUSD:      0.10,
	}
	scenario := NewScenario().
		Success("session-1", 0.05).
		WithCost(metrics).
		Build()

	resp := scenario.Responses()[0]
	if resp.Result.Cost.InputTokens != 1000 {
		t.Fatalf("expected 1000 input tokens, got %d", resp.Result.Cost.InputTokens)
	}
	if resp.Result.Cost.OutputTokens != 500 {
		t.Fatalf("expected 500 output tokens, got %d", resp.Result.Cost.OutputTokens)
	}
	if resp.Result.Cost.CostUSD != 0.10 {
		t.Fatalf("expected cost 0.10, got %f", resp.Result.Cost.CostUSD)
	}
}

func TestScenarioBuilder_Repeat(t *testing.T) {
	scenario := NewScenario().
		Success("session-1", 0.05).
		Repeat(5).
		Build()

	if scenario.Len() != 5 {
		t.Fatalf("expected 5 responses, got %d", scenario.Len())
	}

	// Verify all responses are independent copies
	responses := scenario.Responses()
	for i, resp := range responses {
		if resp.Result.SessionID != "session-1" {
			t.Fatalf("response %d: expected session ID 'session-1', got %q", i, resp.Result.SessionID)
		}
	}

	// Verify they are actually copies (modifying one doesn't affect others)
	responses[0].Result.SessionID = "modified"
	responses2 := scenario.Responses()
	if responses2[1].Result.SessionID == "modified" {
		t.Fatal("expected responses to be independent copies")
	}
}

func TestScenarioBuilder_RepeatZero(t *testing.T) {
	scenario := NewScenario().
		Success("session-1", 0.05).
		Repeat(0). // Should be a no-op
		Build()

	if scenario.Len() != 1 {
		t.Fatalf("expected 1 response with Repeat(0), got %d", scenario.Len())
	}
}

func TestScenarioBuilder_Custom(t *testing.T) {
	callCount := 0
	scenario := NewScenario().
		Custom(func(call *AgentCall) *CallResponse {
			callCount++
			return &CallResponse{
				Result: &agents.RunResult{
					SessionID: "dynamic-session",
					ExitCode:  0,
				},
			}
		}).
		Build()

	resp := scenario.Responses()[0]
	if resp.CustomFunc == nil {
		t.Fatal("expected CustomFunc to be set")
	}

	// Invoke the custom function
	result := resp.CustomFunc(&AgentCall{Index: 0, Method: "Run"})
	if result.Result.SessionID != "dynamic-session" {
		t.Fatalf("expected session ID 'dynamic-session', got %q", result.Result.SessionID)
	}
	if callCount != 1 {
		t.Fatalf("expected custom function to be called once, got %d", callCount)
	}
}

func TestScenarioBuilder_ModifiersApplyToLastOnly(t *testing.T) {
	scenario := NewScenario().
		Success("session-1", 0.05).
		Success("session-2", 0.03).
		WithDelay(100 * time.Millisecond). // Should only apply to session-2
		Build()

	responses := scenario.Responses()
	if responses[0].Delay != 0 {
		t.Fatalf("expected first response to have no delay, got %v", responses[0].Delay)
	}
	if responses[1].Delay != 100*time.Millisecond {
		t.Fatalf("expected second response to have 100ms delay, got %v", responses[1].Delay)
	}
}

func TestScenarioBuilder_ComplexSequence(t *testing.T) {
	// Test a realistic scenario with mixed responses
	scenario := NewScenario().
		RetryableError("connection timeout").
		RetryableError("connection timeout").
		Success("session-1", 0.05).WithDelay(50*time.Millisecond).
		Success("session-1", 0.03).
		Build()

	if scenario.Len() != 4 {
		t.Fatalf("expected 4 responses, got %d", scenario.Len())
	}

	responses := scenario.Responses()

	// First two should be retryable errors
	if responses[0].ErrorClass != agents.ErrorClassRetryable {
		t.Fatal("expected first response to be retryable error")
	}
	if responses[1].ErrorClass != agents.ErrorClassRetryable {
		t.Fatal("expected second response to be retryable error")
	}

	// Third should be success with delay
	if responses[2].Result.SessionID != "session-1" {
		t.Fatal("expected third response to have session ID")
	}
	if responses[2].Delay != 50*time.Millisecond {
		t.Fatal("expected third response to have delay")
	}

	// Fourth should be success without delay
	if responses[3].Delay != 0 {
		t.Fatal("expected fourth response to have no delay")
	}
}
