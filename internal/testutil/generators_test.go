package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
	"pgregory.net/rapid"
)

// startTime is a fixed time used for FakeClock in tests.
var startTime = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

func TestRunResultGen_Invariants(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		result := RunResultGen().Draw(rt, "result")

		// Invariant: SessionID is non-empty when IsError is false
		if !result.IsError && result.SessionID == "" {
			rt.Fatalf("successful result must have non-empty SessionID")
		}

		// Invariant: ExitCode is 0 for success
		if !result.IsError && result.ExitCode != 0 {
			rt.Fatalf("successful result must have ExitCode 0, got %d", result.ExitCode)
		}

		// Invariant: ExitCode is non-zero for errors
		if result.IsError && result.ExitCode == 0 {
			rt.Fatalf("error result must have non-zero ExitCode")
		}

		// Invariant: Duration is non-negative
		if result.Duration < 0 {
			rt.Fatalf("Duration must be non-negative, got %v", result.Duration)
		}

		// Invariant: NumTurns is non-negative
		if result.NumTurns < 0 {
			rt.Fatalf("NumTurns must be non-negative, got %d", result.NumTurns)
		}
	})
}

func TestCostMetricsGen_Invariants(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		cost := CostMetricsGen().Draw(rt, "cost")

		// Invariant: CostUSD is non-negative
		if cost.CostUSD < 0 {
			rt.Fatalf("CostUSD must be non-negative, got %f", cost.CostUSD)
		}

		// Invariant: InputTokens is non-negative
		if cost.InputTokens < 0 {
			rt.Fatalf("InputTokens must be non-negative, got %d", cost.InputTokens)
		}

		// Invariant: OutputTokens is non-negative
		if cost.OutputTokens < 0 {
			rt.Fatalf("OutputTokens must be non-negative, got %d", cost.OutputTokens)
		}
	})
}

func TestErrorClassGen_ValidValues(t *testing.T) {
	t.Parallel()

	validClasses := map[agents.ErrorClass]bool{
		agents.ErrorClassRetryable:      true,
		agents.ErrorClassFatal:          true,
		agents.ErrorClassSessionInvalid: true,
		agents.ErrorClassRateLimitWait:  true,
	}

	rapid.Check(t, func(rt *rapid.T) {
		errClass := ErrorClassGen().Draw(rt, "errClass")

		if !validClasses[errClass] {
			rt.Fatalf("ErrorClassGen produced invalid error class: %v", errClass)
		}
	})
}

func TestRandomScenarioGen_Invariants(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		length := rapid.IntRange(1, 20).Draw(rt, "length")
		scenario := RandomScenarioGen(length).Draw(rt, "scenario")

		// Invariant: scenario has exactly the requested length
		if scenario.Len() != length {
			rt.Fatalf("scenario should have %d responses, got %d", length, scenario.Len())
		}

		// Invariant: all responses have valid structure
		for i, resp := range scenario.Responses() {
			if resp.Result == nil && resp.CustomFunc == nil {
				// Success responses have Result set
				// Error responses also have Result set via the builder
				// Only Custom() responses might have nil Result with CustomFunc set
				rt.Fatalf("response %d has nil Result and no CustomFunc", i)
			}
		}
	})
}

// TestProperty_OrchestrationHandlesAnyErrorSequence verifies that the orchestration
// layer can handle any valid sequence of errors without panicking.
// Note: This test uses a mock orchestration check since the full Orbit.Run()
// requires external dependencies. It validates that scenarios are well-formed.
func TestProperty_OrchestrationHandlesAnyErrorSequence(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		length := rapid.IntRange(1, 10).Draw(rt, "length")
		scenario := RandomScenarioGen(length).Draw(rt, "scenario")

		// Use FakeClock to avoid real delays from RateLimitWait responses
		clock := NewFakeClock(startTime)

		// Create a test agent with the generated scenario
		agent := NewTestAgent(t, "mock", scenario, WithClock(clock))

		// Simulate making calls - should not panic
		// Note: TestAgent returns errors when IsError is true, which is expected
		ctx := context.Background()
		for range length {
			result, err := agent.Run(ctx, agents.RunOptions{})
			// Error is expected for error scenarios (RetryableError, FatalError, etc.)
			// Both result and err are always returned (err != nil when IsError is true)
			_ = err
			if result == nil {
				rt.Fatalf("agent.Run returned nil result")
			}
		}

		// Verify all responses were consumed
		agent.AssertAllConsumed(t)
	})
}

// TestProperty_RetryCountBounded verifies that retry scenarios are properly bounded.
// This tests that the TestAgent correctly returns responses in sequence.
func TestProperty_RetryCountBounded(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		// Generate a scenario with N retryable errors followed by success
		retryCount := rapid.IntRange(0, 10).Draw(rt, "retryCount")
		builder := NewScenario()
		for range retryCount {
			builder.RetryableError("transient error")
		}
		builder.Success("session-final", 0.05)
		scenario := builder.Build()

		agent := NewTestAgent(t, "mock", scenario)

		// Make all calls - should not panic
		// Note: TestAgent returns errors for error scenarios, success for success scenarios
		ctx := context.Background()
		for i := range retryCount + 1 {
			result, err := agent.Run(ctx, agents.RunOptions{})
			// First retryCount calls return errors, last call succeeds
			if i < retryCount {
				if err == nil {
					rt.Fatalf("call %d: expected error for retryable scenario", i)
				}
			} else {
				if err != nil {
					rt.Fatalf("call %d: unexpected error: %v", i, err)
				}
			}
			if result == nil {
				rt.Fatalf("call %d: got nil result", i)
			}
		}

		// Verify total call count matches expected
		if agent.Recorder().CallCount() != retryCount+1 {
			rt.Fatalf("expected %d calls, got %d", retryCount+1, agent.Recorder().CallCount())
		}

		// Verify all responses were consumed
		agent.AssertAllConsumed(t)
	})
}
