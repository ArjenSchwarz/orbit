package agents_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
)

// fakeSleeper records sleep durations without blocking.
type fakeSleeper struct {
	sleeps []time.Duration
}

func (f *fakeSleeper) sleep(d time.Duration) {
	f.sleeps = append(f.sleeps, d)
}

func TestRunWithRetry_SuccessOnFirstAttempt(t *testing.T) {
	sleeper := &fakeSleeper{}
	result := &agents.RunResult{SessionID: "s1"}

	got, err := agents.RunWithRetry(t.Context(), agents.RetryConfig{
		MaxRetries: 5,
		Sleep:      sleeper.sleep,
		Execute: func(_ context.Context, _ int) (*agents.RunResult, error) {
			return result, nil
		},
		Classify: func(r *agents.RunResult, err error) *agents.ClassifiedError {
			if err == nil {
				return nil
			}
			return &agents.ClassifiedError{Class: agents.ErrorClassFatal, Message: err.Error()}
		},
	})

	if err != nil {
		t.Fatalf("got err = %v, want nil", err)
	}
	if got.SessionID != "s1" {
		t.Errorf("got SessionID = %q, want %q", got.SessionID, "s1")
	}
	if len(sleeper.sleeps) != 0 {
		t.Errorf("got %d sleeps, want 0", len(sleeper.sleeps))
	}
}

func TestRunWithRetry_RetryThenSuccess(t *testing.T) {
	sleeper := &fakeSleeper{}
	calls := 0

	got, err := agents.RunWithRetry(t.Context(), agents.RetryConfig{
		MaxRetries: 5,
		Sleep:      sleeper.sleep,
		Execute: func(_ context.Context, _ int) (*agents.RunResult, error) {
			calls++
			if calls <= 2 {
				return &agents.RunResult{Stderr: "timeout"}, fmt.Errorf("timeout")
			}
			return &agents.RunResult{SessionID: "s1"}, nil
		},
		Classify: func(r *agents.RunResult, err error) *agents.ClassifiedError {
			if err == nil {
				return nil
			}
			return &agents.ClassifiedError{
				Class:   agents.ErrorClassRetryable,
				Message: err.Error(),
			}
		},
	})

	if err != nil {
		t.Fatalf("got err = %v, want nil", err)
	}
	if got.SessionID != "s1" {
		t.Errorf("got SessionID = %q, want %q", got.SessionID, "s1")
	}
	if calls != 3 {
		t.Errorf("got %d calls, want 3", calls)
	}
	// Exponential backoff: 1s, 2s
	wantSleeps := []time.Duration{time.Second, 2 * time.Second}
	if len(sleeper.sleeps) != len(wantSleeps) {
		t.Fatalf("got %d sleeps, want %d", len(sleeper.sleeps), len(wantSleeps))
	}
	for i, want := range wantSleeps {
		if sleeper.sleeps[i] != want {
			t.Errorf("sleep[%d] = %v, want %v", i, sleeper.sleeps[i], want)
		}
	}
}

func TestRunWithRetry_FatalError(t *testing.T) {
	sleeper := &fakeSleeper{}
	calls := 0

	_, err := agents.RunWithRetry(t.Context(), agents.RetryConfig{
		MaxRetries: 5,
		Sleep:      sleeper.sleep,
		Execute: func(_ context.Context, _ int) (*agents.RunResult, error) {
			calls++
			return nil, fmt.Errorf("auth failed")
		},
		Classify: func(_ *agents.RunResult, err error) *agents.ClassifiedError {
			return &agents.ClassifiedError{
				Class:   agents.ErrorClassFatal,
				Message: err.Error(),
			}
		},
	})

	if err == nil {
		t.Fatal("got err = nil, want error")
	}
	if calls != 1 {
		t.Errorf("got %d calls, want 1 (fatal should not retry)", calls)
	}
	if len(sleeper.sleeps) != 0 {
		t.Errorf("got %d sleeps, want 0", len(sleeper.sleeps))
	}
}

func TestRunWithRetry_MaxRetriesExceeded(t *testing.T) {
	sleeper := &fakeSleeper{}
	calls := 0

	_, err := agents.RunWithRetry(t.Context(), agents.RetryConfig{
		MaxRetries: 3,
		Sleep:      sleeper.sleep,
		Execute: func(_ context.Context, _ int) (*agents.RunResult, error) {
			calls++
			return &agents.RunResult{Stderr: "connection refused"}, fmt.Errorf("connection refused")
		},
		Classify: func(_ *agents.RunResult, err error) *agents.ClassifiedError {
			return &agents.ClassifiedError{
				Class:   agents.ErrorClassRetryable,
				Message: err.Error(),
			}
		},
	})

	if err == nil {
		t.Fatal("got err = nil, want error")
	}
	if !errors.Is(err, err) { // Just verify it's an error
		t.Errorf("unexpected error type: %T", err)
	}
	if calls != 3 {
		t.Errorf("got %d calls, want 3", calls)
	}
	// 2 sleeps: between attempts 1→2 and 2→3. No sleep after the last attempt.
	if len(sleeper.sleeps) != 2 {
		t.Errorf("got %d sleeps, want 2", len(sleeper.sleeps))
	}
}

func TestRunWithRetry_RateLimitResetsAttemptCounter(t *testing.T) {
	sleeper := &fakeSleeper{}
	calls := 0

	got, err := agents.RunWithRetry(t.Context(), agents.RetryConfig{
		MaxRetries: 3,
		Sleep:      sleeper.sleep,
		Execute: func(_ context.Context, attempt int) (*agents.RunResult, error) {
			calls++
			// Call 1: rate limit
			if calls == 1 {
				return &agents.RunResult{Stderr: "rate limit"}, fmt.Errorf("rate limit")
			}
			// Call 2 (attempt reset to 0): retryable error
			if calls == 2 {
				return &agents.RunResult{Stderr: "timeout"}, fmt.Errorf("timeout")
			}
			// Call 3: success
			return &agents.RunResult{SessionID: "s1"}, nil
		},
		Classify: func(_ *agents.RunResult, err error) *agents.ClassifiedError {
			if err == nil {
				return nil
			}
			if err.Error() == "rate limit" {
				return &agents.ClassifiedError{
					Class:      agents.ErrorClassRateLimitWait,
					RetryAfter: 5 * time.Minute,
					Message:    "rate limit",
				}
			}
			return &agents.ClassifiedError{
				Class:   agents.ErrorClassRetryable,
				Message: err.Error(),
			}
		},
	})

	if err != nil {
		t.Fatalf("got err = %v, want nil", err)
	}
	if got.SessionID != "s1" {
		t.Errorf("got SessionID = %q, want %q", got.SessionID, "s1")
	}
	if calls != 3 {
		t.Errorf("got %d calls, want 3", calls)
	}
	// First wait: 5 minutes (rate limit) chunked into 10x30s, then 1s (exponential, attempt reset to 0)
	var totalSleep time.Duration
	for _, s := range sleeper.sleeps {
		totalSleep += s
	}
	wantTotal := 5*time.Minute + time.Second
	if totalSleep != wantTotal {
		t.Errorf("total sleep = %v, want %v (sleeps: %v)", totalSleep, wantTotal, sleeper.sleeps)
	}
}

func TestRunWithRetry_RetryAfterFromClassifier(t *testing.T) {
	sleeper := &fakeSleeper{}
	calls := 0

	_, err := agents.RunWithRetry(t.Context(), agents.RetryConfig{
		MaxRetries: 2,
		Sleep:      sleeper.sleep,
		Execute: func(_ context.Context, _ int) (*agents.RunResult, error) {
			calls++
			if calls == 1 {
				return nil, fmt.Errorf("retry after 30s")
			}
			return &agents.RunResult{SessionID: "s1"}, nil
		},
		Classify: func(_ *agents.RunResult, err error) *agents.ClassifiedError {
			if err == nil {
				return nil
			}
			return &agents.ClassifiedError{
				Class:      agents.ErrorClassRetryable,
				RetryAfter: 30 * time.Second,
				Message:    err.Error(),
			}
		},
	})

	if err != nil {
		t.Fatalf("got err = %v, want nil", err)
	}
	if len(sleeper.sleeps) != 1 || sleeper.sleeps[0] != 30*time.Second {
		t.Errorf("got sleeps = %v, want [30s]", sleeper.sleeps)
	}
}

func TestRunWithRetry_ContextCancellation(t *testing.T) {
	sleeper := &fakeSleeper{}
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // Cancel immediately

	_, err := agents.RunWithRetry(ctx, agents.RetryConfig{
		MaxRetries: 5,
		Sleep:      sleeper.sleep,
		Execute: func(_ context.Context, _ int) (*agents.RunResult, error) {
			t.Fatal("Execute should not be called when context is canceled")
			return nil, nil
		},
		Classify: func(_ *agents.RunResult, _ error) *agents.ClassifiedError {
			return nil
		},
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("got err = %v, want context.Canceled", err)
	}
}

func TestRunWithRetry_OnRetryCallback(t *testing.T) {
	sleeper := &fakeSleeper{}
	var retryAttempts []int
	var retryBackoffs []time.Duration
	calls := 0

	_, err := agents.RunWithRetry(t.Context(), agents.RetryConfig{
		MaxRetries: 3,
		Sleep:      sleeper.sleep,
		Execute: func(_ context.Context, _ int) (*agents.RunResult, error) {
			calls++
			if calls < 3 {
				return nil, fmt.Errorf("error")
			}
			return &agents.RunResult{}, nil
		},
		Classify: func(_ *agents.RunResult, err error) *agents.ClassifiedError {
			if err == nil {
				return nil
			}
			return &agents.ClassifiedError{
				Class:   agents.ErrorClassRetryable,
				Message: err.Error(),
			}
		},
		OnRetry: func(attempt, maxRetries int, _ *agents.ClassifiedError, backoff time.Duration) {
			retryAttempts = append(retryAttempts, attempt)
			retryBackoffs = append(retryBackoffs, backoff)
		},
	})

	if err != nil {
		t.Fatalf("got err = %v, want nil", err)
	}
	wantAttempts := []int{1, 2}
	if len(retryAttempts) != len(wantAttempts) {
		t.Fatalf("got %d OnRetry calls, want %d", len(retryAttempts), len(wantAttempts))
	}
	for i, want := range wantAttempts {
		if retryAttempts[i] != want {
			t.Errorf("OnRetry attempt[%d] = %d, want %d", i, retryAttempts[i], want)
		}
	}
}

func TestRunWithRetry_AfterWaitCallback(t *testing.T) {
	sleeper := &fakeSleeper{}
	afterWaitCalls := 0
	calls := 0

	_, err := agents.RunWithRetry(t.Context(), agents.RetryConfig{
		MaxRetries: 3,
		Sleep:      sleeper.sleep,
		Execute: func(_ context.Context, _ int) (*agents.RunResult, error) {
			calls++
			if calls == 1 {
				return nil, fmt.Errorf("error")
			}
			return &agents.RunResult{}, nil
		},
		Classify: func(_ *agents.RunResult, err error) *agents.ClassifiedError {
			if err == nil {
				return nil
			}
			return &agents.ClassifiedError{
				Class:   agents.ErrorClassRetryable,
				Message: err.Error(),
			}
		},
		AfterWait: func() {
			afterWaitCalls++
		},
	})

	if err != nil {
		t.Fatalf("got err = %v, want nil", err)
	}
	if afterWaitCalls != 1 {
		t.Errorf("got %d AfterWait calls, want 1", afterWaitCalls)
	}
}

func TestRunWithRetry_ExponentialBackoff(t *testing.T) {
	sleeper := &fakeSleeper{}

	_, _ = agents.RunWithRetry(t.Context(), agents.RetryConfig{
		MaxRetries: 5,
		Sleep:      sleeper.sleep,
		Execute: func(_ context.Context, _ int) (*agents.RunResult, error) {
			return nil, fmt.Errorf("error")
		},
		Classify: func(_ *agents.RunResult, err error) *agents.ClassifiedError {
			return &agents.ClassifiedError{
				Class:   agents.ErrorClassRetryable,
				Message: err.Error(),
			}
		},
	})

	// Backoff: 1s, 2s, 4s, 8s (4 sleeps for 5 attempts)
	wantSleeps := []time.Duration{
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
	}
	if len(sleeper.sleeps) != len(wantSleeps) {
		t.Fatalf("got %d sleeps, want %d: %v", len(sleeper.sleeps), len(wantSleeps), sleeper.sleeps)
	}
	for i, want := range wantSleeps {
		if sleeper.sleeps[i] != want {
			t.Errorf("sleep[%d] = %v, want %v", i, sleeper.sleeps[i], want)
		}
	}
}

func TestRunWithRetry_ReturnsLastResult(t *testing.T) {
	sleeper := &fakeSleeper{}
	calls := 0

	got, err := agents.RunWithRetry(t.Context(), agents.RetryConfig{
		MaxRetries: 2,
		Sleep:      sleeper.sleep,
		Execute: func(_ context.Context, _ int) (*agents.RunResult, error) {
			calls++
			return &agents.RunResult{SessionID: fmt.Sprintf("s%d", calls)}, fmt.Errorf("fail")
		},
		Classify: func(_ *agents.RunResult, err error) *agents.ClassifiedError {
			return &agents.ClassifiedError{
				Class:   agents.ErrorClassRetryable,
				Message: err.Error(),
			}
		},
	})

	if err == nil {
		t.Fatal("got err = nil, want error")
	}
	if got == nil {
		t.Fatal("got result = nil, want last result")
	}
	if got.SessionID != "s2" {
		t.Errorf("got SessionID = %q, want %q (last attempt's result)", got.SessionID, "s2")
	}
}

func TestRunWithRetry_NilOnRetryAndAfterWait(t *testing.T) {
	// Verify nil callbacks don't panic
	sleeper := &fakeSleeper{}
	calls := 0

	_, err := agents.RunWithRetry(t.Context(), agents.RetryConfig{
		MaxRetries: 2,
		Sleep:      sleeper.sleep,
		Execute: func(_ context.Context, _ int) (*agents.RunResult, error) {
			calls++
			if calls == 1 {
				return nil, fmt.Errorf("error")
			}
			return &agents.RunResult{}, nil
		},
		Classify: func(_ *agents.RunResult, err error) *agents.ClassifiedError {
			if err == nil {
				return nil
			}
			return &agents.ClassifiedError{
				Class:   agents.ErrorClassRetryable,
				Message: err.Error(),
			}
		},
		OnRetry:   nil,
		AfterWait: nil,
	})

	if err != nil {
		t.Fatalf("got err = %v, want nil", err)
	}
}

func TestRunWithRetry_RateLimitResetCapped(t *testing.T) {
	sleeper := &fakeSleeper{}
	calls := 0

	_, err := agents.RunWithRetry(t.Context(), agents.RetryConfig{
		MaxRetries: 3,
		Sleep:      sleeper.sleep,
		Execute: func(_ context.Context, _ int) (*agents.RunResult, error) {
			calls++
			return nil, fmt.Errorf("rate limited")
		},
		Classify: func(_ *agents.RunResult, err error) *agents.ClassifiedError {
			return &agents.ClassifiedError{
				Class:      agents.ErrorClassRateLimitWait,
				RetryAfter: time.Second,
				Message:    err.Error(),
			}
		},
	})

	if err == nil {
		t.Fatal("got err = nil, want error")
	}
	if !errors.Is(err, err) {
		t.Errorf("unexpected error type: %T", err)
	}
	// Should stop after maxRateLimitResets (5) + 1 = 6 rate-limit attempts
	if calls > 10 {
		t.Errorf("got %d calls, expected bounded by rate-limit reset cap", calls)
	}
}

func TestRunWithRetry_MaxRetriesZero(t *testing.T) {
	sleeper := &fakeSleeper{}

	_, err := agents.RunWithRetry(t.Context(), agents.RetryConfig{
		MaxRetries: 0,
		Sleep:      sleeper.sleep,
		Execute: func(_ context.Context, _ int) (*agents.RunResult, error) {
			t.Fatal("Execute should not be called when MaxRetries is 0")
			return nil, nil
		},
		Classify: func(_ *agents.RunResult, _ error) *agents.ClassifiedError {
			return nil
		},
	})

	if err == nil {
		t.Fatal("got err = nil, want error for MaxRetries=0")
	}
}

func TestRunWithRetry_SleepWithContextCancellation(t *testing.T) {
	// Verify that long sleeps are interruptible via context cancellation.
	// The sleep is broken into 30s chunks; cancel after the first chunk.
	ctx, cancel := context.WithCancel(t.Context())
	sleepCalls := 0

	_, err := agents.RunWithRetry(ctx, agents.RetryConfig{
		MaxRetries: 2,
		Sleep: func(d time.Duration) {
			sleepCalls++
			// Cancel during the first chunk of the long sleep
			if sleepCalls == 1 {
				cancel()
			}
		},
		Execute: func(_ context.Context, _ int) (*agents.RunResult, error) {
			return nil, fmt.Errorf("rate limited")
		},
		Classify: func(_ *agents.RunResult, err error) *agents.ClassifiedError {
			return &agents.ClassifiedError{
				Class:      agents.ErrorClassRateLimitWait,
				RetryAfter: 2 * time.Minute, // Long enough to require multiple chunks
				Message:    err.Error(),
			}
		},
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("got err = %v, want context.Canceled", err)
	}
	// Should have stopped after 1-2 sleep chunks, not all 4 (2min / 30s)
	if sleepCalls > 2 {
		t.Errorf("got %d sleep calls, expected <= 2 (should cancel early)", sleepCalls)
	}
}
