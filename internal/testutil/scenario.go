package testutil

import (
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
)

// ScenarioBuilder constructs scenarios with a fluent API.
type ScenarioBuilder struct {
	responses []CallResponse
	current   *CallResponse // Points to the last added response for chaining modifiers
}

// NewScenario creates a new scenario builder.
func NewScenario() *ScenarioBuilder {
	return &ScenarioBuilder{}
}

// Success adds a successful execution response to the scenario.
func (b *ScenarioBuilder) Success(sessionID string, cost float64) *ScenarioBuilder {
	resp := CallResponse{
		Result: &agents.RunResult{
			SessionID: sessionID,
			ExitCode:  0,
			IsError:   false,
			Cost: &agents.CostMetrics{
				CostUSD: cost,
			},
		},
	}
	b.responses = append(b.responses, resp)
	b.current = &b.responses[len(b.responses)-1]
	return b
}

// RetryableError adds a retryable error response to the scenario.
func (b *ScenarioBuilder) RetryableError(message string) *ScenarioBuilder {
	resp := CallResponse{
		Result: &agents.RunResult{
			ExitCode: 1,
			IsError:  true,
			Stderr:   message,
			Errors:   []string{message},
		},
		ErrorClass: agents.ErrorClassRetryable,
	}
	b.responses = append(b.responses, resp)
	b.current = &b.responses[len(b.responses)-1]
	return b
}

// FatalError adds a fatal (non-retryable) error response to the scenario.
func (b *ScenarioBuilder) FatalError(message string) *ScenarioBuilder {
	resp := CallResponse{
		Result: &agents.RunResult{
			ExitCode: 1,
			IsError:  true,
			Stderr:   message,
			Errors:   []string{message},
		},
		ErrorClass: agents.ErrorClassFatal,
	}
	b.responses = append(b.responses, resp)
	b.current = &b.responses[len(b.responses)-1]
	return b
}

// SessionInvalid adds a session invalid error response to the scenario.
func (b *ScenarioBuilder) SessionInvalid() *ScenarioBuilder {
	resp := CallResponse{
		Result: &agents.RunResult{
			ExitCode: 1,
			IsError:  true,
			Stderr:   "session not found or expired",
			Errors:   []string{"session not found or expired"},
		},
		ErrorClass: agents.ErrorClassSessionInvalid,
	}
	b.responses = append(b.responses, resp)
	b.current = &b.responses[len(b.responses)-1]
	return b
}

// RateLimitWait adds a rate limit wait error response to the scenario.
func (b *ScenarioBuilder) RateLimitWait(waitDuration time.Duration) *ScenarioBuilder {
	resp := CallResponse{
		Result: &agents.RunResult{
			ExitCode: 1,
			IsError:  true,
			Stderr:   "You've hit your limit",
			Errors:   []string{"You've hit your limit"},
		},
		ErrorClass: agents.ErrorClassRateLimitWait,
		Delay:      waitDuration, // Use Delay to simulate the wait time
	}
	b.responses = append(b.responses, resp)
	b.current = &b.responses[len(b.responses)-1]
	return b
}

// WithDelay sets the delay on the last added response.
// This simulates execution time for the agent call.
func (b *ScenarioBuilder) WithDelay(d time.Duration) *ScenarioBuilder {
	if b.current != nil {
		b.current.Delay = d
	}
	return b
}

// WithOutput sets the output and stderr on the last added response.
func (b *ScenarioBuilder) WithOutput(output, stderr string) *ScenarioBuilder {
	if b.current != nil {
		b.current.Output = output
		b.current.Stderr = stderr
	}
	return b
}

// WithCost sets detailed cost metrics on the last added response.
func (b *ScenarioBuilder) WithCost(metrics *agents.CostMetrics) *ScenarioBuilder {
	if b.current != nil && b.current.Result != nil {
		b.current.Result.Cost = metrics
	}
	return b
}

// Repeat duplicates the last response n times.
// This is useful for scenarios with multiple identical responses.
func (b *ScenarioBuilder) Repeat(n int) *ScenarioBuilder {
	if len(b.responses) == 0 || n <= 0 {
		return b
	}
	last := b.responses[len(b.responses)-1]
	for i := 0; i < n-1; i++ { // n-1 because the first one is already added
		// Make a copy to avoid sharing pointers
		copied := CallResponse{
			Delay:      last.Delay,
			ErrorClass: last.ErrorClass,
			Output:     last.Output,
			Stderr:     last.Stderr,
			CustomFunc: last.CustomFunc,
		}
		if last.Result != nil {
			copiedResult := *last.Result
			if last.Result.Cost != nil {
				copiedCost := *last.Result.Cost
				copiedResult.Cost = &copiedCost
			}
			copied.Result = &copiedResult
		}
		b.responses = append(b.responses, copied)
	}
	// Update current to point to the last response
	b.current = &b.responses[len(b.responses)-1]
	return b
}

// Custom sets a custom function for dynamic behavior on the last added response.
// Use sparingly - this is an escape hatch for truly dynamic edge cases.
func (b *ScenarioBuilder) Custom(fn func(*AgentCall) *CallResponse) *ScenarioBuilder {
	resp := CallResponse{
		CustomFunc: fn,
	}
	b.responses = append(b.responses, resp)
	b.current = &b.responses[len(b.responses)-1]
	return b
}

// Build returns an immutable Scenario.
// After calling Build(), the ScenarioBuilder should not be used.
func (b *ScenarioBuilder) Build() *Scenario {
	// Make a copy of responses to ensure immutability
	responses := make([]CallResponse, len(b.responses))
	copy(responses, b.responses)
	return &Scenario{responses: responses}
}
