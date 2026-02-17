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
	// First sleep: 5 minutes (rate limit), second: 1s (exponential, attempt reset to 0)
	wantSleeps := []time.Duration{5 * time.Minute, time.Second}
	if len(sleeper.sleeps) != len(wantSleeps) {
		t.Fatalf("got %d sleeps, want %d: %v", len(sleeper.sleeps), len(wantSleeps), sleeper.sleeps)
	}
	for i, want := range wantSleeps {
		if sleeper.sleeps[i] != want {
			t.Errorf("sleep[%d] = %v, want %v", i, sleeper.sleeps[i], want)
		}
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
