package agents

import (
	"context"
	"fmt"
	"time"
)

// RetryConfig configures the shared retry executor.
// Each caller provides callbacks for execution, error classification,
// and retry notification. This unifies the retry-with-backoff pattern
// used across single-run, variant, and consolidation modes.
type RetryConfig struct {
	// MaxRetries is the maximum number of attempts (must be >= 1).
	MaxRetries int

	// Sleep pauses for the given duration. Inject Clock.Sleep for testability.
	Sleep func(time.Duration)

	// Execute runs the operation. The attempt parameter is 0-based.
	// It resets to 0 after a rate-limit wait.
	Execute func(ctx context.Context, attempt int) (*RunResult, error)

	// Classify determines whether the result is a success (return nil),
	// a retryable error, or a fatal error. Called after every Execute.
	Classify func(result *RunResult, err error) *ClassifiedError

	// OnRetry is called before sleeping when a retry will be attempted.
	// Use it for logging and UI updates (spinner, progress messages).
	// May be nil.
	OnRetry func(attempt, maxRetries int, classified *ClassifiedError, backoff time.Duration)

	// AfterWait is called after sleeping, before the next attempt.
	// Use it for UI cleanup (e.g., stopping a spinner). May be nil.
	AfterWait func()
}

// RunWithRetry executes an operation with retry logic for transient errors.
//
// The retry loop:
//  1. Check context cancellation
//  2. Call Execute
//  3. Call Classify -- nil means success
//  4. If not retryable, return the classified error
//  5. Calculate backoff (rate-limit wait, RetryAfter, or exponential)
//  6. Call OnRetry (if set)
//  7. Sleep
//  8. Call AfterWait (if set)
//  9. Loop
//
// On rate-limit waits (ErrorClassRateLimitWait), the attempt counter resets
// to 0 so the caller gets a fresh set of retries after the limit lifts.
func RunWithRetry(ctx context.Context, cfg RetryConfig) (*RunResult, error) {
	var lastErr error
	var lastResult *RunResult

	for attempt := 0; attempt < cfg.MaxRetries; attempt++ {
		if ctx.Err() != nil {
			return lastResult, ctx.Err()
		}

		result, err := cfg.Execute(ctx, attempt)

		classified := cfg.Classify(result, err)
		if classified == nil {
			return result, nil
		}

		lastResult = result
		lastErr = classified

		if !classified.Class.IsRetryable() {
			return result, classified
		}

		// On the last attempt, don't bother sleeping — just exit the loop.
		// Exception: rate-limit waits always sleep since they reset the counter.
		isLastAttempt := attempt == cfg.MaxRetries-1
		if isLastAttempt && !classified.Class.IsRateLimitWait() {
			break
		}

		// Calculate wait duration.
		backoff := calcBackoff(attempt, classified)

		if cfg.OnRetry != nil {
			cfg.OnRetry(attempt+1, cfg.MaxRetries, classified, backoff)
		}

		// Rate-limit wait resets the attempt counter.
		// Set to -1 because the loop increment will make it 0.
		if classified.Class.IsRateLimitWait() {
			attempt = -1
		}

		cfg.Sleep(backoff)

		if cfg.AfterWait != nil {
			cfg.AfterWait()
		}
	}

	return lastResult, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// calcBackoff determines the wait duration for a retry attempt.
// Priority: rate-limit wait duration > explicit RetryAfter > exponential backoff.
func calcBackoff(attempt int, classified *ClassifiedError) time.Duration {
	if classified.Class.IsRateLimitWait() {
		return classified.RetryAfter
	}
	if classified.RetryAfter > 0 {
		return classified.RetryAfter
	}
	return BackoffDuration(attempt)
}
