# Smolspec: Shared Retry Executor

**Transit ticket**: T-94
**Date**: 2026-02-17

## Problem

Four retry executor functions exist with nearly identical retry logic but divergent implementations:

| Function | File | Returns | Spinner | Session Mgmt | Rate Limit |
|----------|------|---------|---------|-------------|------------|
| `runPhaseWithRetry` | single.go | `error` | Yes | Log manager | Yes (reset) |
| `runPostPromptWithRetry` | single.go | `error` | Yes | Log manager | No |
| `runVariantPhaseWithRetry` | variants.go | `(*RunResult, error)` | No | Stateless | Yes (reset) |
| `runVariantPostCompletion` | variants.go | `(*RunResult, error)` | No | Stateless | Yes (reset) |

The core retry loop (classify error, decide retry vs fatal, calculate backoff, wait, retry) is duplicated across all four. This creates maintenance risk: bug fixes to retry logic may not reach all copies. The missing rate limit handling in `runPostPromptWithRetry` is an example of this drift.

## Root Cause of Divergence

Two execution models evolved separately:

1. **Single-run mode** (main thread): needs spinner updates, log manager session tracking, registry status updates
2. **Variant mode** (goroutines): needs context cancellation, stateless sessions, no spinner

The retry logic itself is identical but the surrounding concerns (UI, session management, return types) differ.

## Proposed Approach

Extract a `RetryExecutor` that encapsulates the retry loop and delegates context-specific behavior via callbacks or an interface.

### Option A: Callback-based (recommended)

```go
type RetryConfig struct {
    MaxRetries int
    Classifier func(err error, result *agents.RunResult) *agents.ClassifiedError
    Execute    func(ctx context.Context, attempt int) (*agents.RunResult, error)
    OnRetry    func(attempt int, classified *agents.ClassifiedError, waitDuration time.Duration)
    OnSuccess  func(result *agents.RunResult)
}

func (rc *RetryConfig) Run(ctx context.Context) (*agents.RunResult, error) {
    // Single implementation of: classify -> decide -> backoff -> wait -> retry
}
```

Each call site provides its own `Execute` (run agent), `OnRetry` (update spinner, log), and `OnSuccess` (save session, update registry) callbacks. The retry decision logic lives in one place.

### Option B: Interface-based

```go
type RetryableOperation interface {
    Execute(ctx context.Context, attempt int) (*agents.RunResult, error)
    OnRetry(attempt int, classified *agents.ClassifiedError, waitDuration time.Duration)
    OnSuccess(result *agents.RunResult)
}
```

More structured but heavier — each call site needs a struct implementing the interface.

### Recommendation

**Option A** (callbacks) is simpler and more Go-idiomatic for this case. The operations are stateless enough that closures work well, and it avoids creating 4 new struct types.

## Scope

### In scope
- Extract shared retry loop into `internal/orbit/retry.go`
- Unify backoff calculation (move `BackoffDuration` from `internal/errors` into the retry executor or `internal/orbit`)
- Fix `runPostPromptWithRetry` missing rate limit handling
- Ensure context cancellation works correctly for variant mode
- Standardize return type to `(*agents.RunResult, error)` — single-mode callers can ignore the result

### Out of scope
- Changing error classification logic (T-91/T-95 scope)
- Changing agent execution internals (T-97 scope)
- Removing `internal/errors` package (T-96 scope, depends on moving `BackoffDuration`)

## Dependencies

- Should be done **after** T-96 (move BackoffDuration) since this will consume it
- Independent of T-97 (agent execution patterns)

## Risks

- The retry loop is a critical path — bugs here affect all orchestration
- The four copies have subtle differences in logging that callers may depend on
- Return type unification needs care: single-mode callers currently return `error` only
