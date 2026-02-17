package agents

import (
	"context"
	"fmt"
	"time"
)

// maxRateLimitResets is the maximum number of times the attempt counter can
// be reset due to rate-limit waits before giving up. Prevents infinite loops
// if a rate-limit condition never resolves.
const maxRateLimitResets = 5

// RetryConfig configures the shared retry executor.
// Each caller provides callbacks for execution, error classification,
// and retry notification. This unifies the retry-with-backoff pattern
// used across single-run, variant, and consolidation modes.
type RetryConfig struct {
	// MaxRetries is the maximum number of attempts (must be >= 1).
	MaxRetries int

	// Sleep pauses for the given duration. Inject Clock.Sleep for testability.
	// During rate-limit waits, sleep is performed in 30-second chunks with
	// context cancellation checks between chunks, so shutdown is responsive
	// even during multi-hour waits.
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
	if cfg.MaxRetries < 1 {
		return nil, fmt.Errorf("MaxRetries must be >= 1, got %d", cfg.MaxRetries)
	}

	var lastErr error
	var lastResult *RunResult
	rateLimitResets := 0

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

		// calcBackoff returns RetryAfter for rate-limit waits (ignoring attempt),
		// explicit RetryAfter for retryable errors, or exponential backoff.
		backoff := calcBackoff(attempt, classified)

		if cfg.OnRetry != nil {
			cfg.OnRetry(attempt+1, cfg.MaxRetries, classified, backoff)
		}

		// Rate-limit wait resets the attempt counter so the caller gets
		// a fresh set of retries. Cap resets to prevent infinite loops
		// if the rate-limit condition never resolves.
		if classified.Class.IsRateLimitWait() {
			rateLimitResets++
			if rateLimitResets > maxRateLimitResets {
				return lastResult, fmt.Errorf("rate-limit wait exceeded %d resets: %w", maxRateLimitResets, lastErr)
			}
			attempt = -1
		}

		// Sleep in chunks for long waits so context cancellation is responsive.
		if err := sleepWithContext(ctx, backoff, cfg.Sleep); err != nil {
			return lastResult, err
		}

		if cfg.AfterWait != nil {
			cfg.AfterWait()
		}
	}

	return lastResult, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// sleepChunk is the maximum duration for a single sleep call during
// chunked sleeping. Long waits (e.g., rate-limit resets) are broken into
// chunks of this size with context checks between them.
const sleepChunk = 30 * time.Second

// sleepWithContext sleeps for the given duration, checking for context
// cancellation between chunks. For short durations (<= sleepChunk), this
// is a single sleep call. For longer durations, sleep is broken into chunks
// so that shutdown (Ctrl+C) is responsive even during multi-hour waits.
func sleepWithContext(ctx context.Context, d time.Duration, sleep func(time.Duration)) error {
	for d > 0 {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		chunk := min(d, sleepChunk)
		sleep(chunk)
		d -= chunk
	}
	return nil
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
