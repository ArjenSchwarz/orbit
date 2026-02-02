package testutil

import (
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
	"pgregory.net/rapid"
)

// RunResultGen generates valid RunResult values.
// Invariants:
//   - SessionID is non-empty when IsError is false
//   - ExitCode is 0 for success, non-zero for errors
func RunResultGen() *rapid.Generator[*agents.RunResult] {
	return rapid.Custom(func(t *rapid.T) *agents.RunResult {
		isError := rapid.Bool().Draw(t, "isError")

		sessionID := ""
		exitCode := 0
		if !isError {
			// Generate UUID-like session ID for successful runs
			sessionID = rapid.StringMatching(`[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}`).Draw(t, "sessionID")
		} else {
			// Non-zero exit code for errors
			exitCode = rapid.IntRange(1, 255).Draw(t, "exitCode")
		}

		return &agents.RunResult{
			SessionID: sessionID,
			ExitCode:  exitCode,
			IsError:   isError,
			Duration:  time.Duration(rapid.Int64Range(0, int64(time.Hour)).Draw(t, "duration")),
			NumTurns:  rapid.IntRange(0, 100).Draw(t, "numTurns"),
			Cost:      CostMetricsGen().Draw(t, "cost"),
		}
	})
}

// CostMetricsGen generates valid CostMetrics values.
// Invariant: CostUSD is non-negative
func CostMetricsGen() *rapid.Generator[*agents.CostMetrics] {
	return rapid.Custom(func(t *rapid.T) *agents.CostMetrics {
		return &agents.CostMetrics{
			InputTokens:  rapid.IntMin(0).Draw(t, "inputTokens"),
			OutputTokens: rapid.IntMin(0).Draw(t, "outputTokens"),
			CostUSD:      rapid.Float64Range(0, 100).Draw(t, "costUSD"),
		}
	})
}

// ErrorClassGen generates valid error classifications.
func ErrorClassGen() *rapid.Generator[agents.ErrorClass] {
	return rapid.SampledFrom([]agents.ErrorClass{
		agents.ErrorClassRetryable,
		agents.ErrorClassFatal,
		agents.ErrorClassSessionInvalid,
		agents.ErrorClassRateLimitWait,
	})
}

// RandomScenarioGen generates a valid scenario of given length.
// The generated scenario contains a mix of successes and errors,
// with approximately 70% successes and 30% various error types.
func RandomScenarioGen(length int) *rapid.Generator[*Scenario] {
	return rapid.Custom(func(t *rapid.T) *Scenario {
		builder := NewScenario()
		for range length {
			// Generate mostly successes with occasional errors
			if rapid.Float64Range(0, 1).Draw(t, "successProb") > 0.3 {
				sessionID := rapid.StringMatching(`session-[a-z0-9]{8}`).Draw(t, "sessionID")
				cost := rapid.Float64Range(0, 1).Draw(t, "cost")
				builder.Success(sessionID, cost)
			} else {
				errClass := ErrorClassGen().Draw(t, "errClass")
				switch errClass {
				case agents.ErrorClassRetryable:
					builder.RetryableError("random retryable error")
				case agents.ErrorClassFatal:
					builder.FatalError("random fatal error")
				case agents.ErrorClassSessionInvalid:
					builder.SessionInvalid()
				case agents.ErrorClassRateLimitWait:
					waitSeconds := rapid.IntRange(1, 300).Draw(t, "wait")
					builder.RateLimitWait(time.Duration(waitSeconds) * time.Second)
				}
			}
		}
		return builder.Build()
	})
}
