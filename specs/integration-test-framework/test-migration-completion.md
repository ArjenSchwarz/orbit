# Test Migration Completion Report

**Date**: 2026-02-03
**Status**: Pending
**Related Feature**: integration-test-framework

## Summary

The integration test framework implementation (variant 1) successfully introduced `testutil.TestAgent` and migrated many tests. However, 4 tests remain skipped because they require real-time delays. Completing the migration requires changes to production code.

## Current State

### Migrated Tests (using testutil.TestAgent)
- `TestRunPostPromptWithRetry_Success`
- `TestRunPostPromptWithRetry_NonRetryableError`
- `TestRunPrePrompt_Success`
- `TestRunPrePrompt_SkipsWhenEmpty`
- `TestRunPrePrompt_FailureAbortsRun`
- `TestRunPrePrompt_ResumesCompletedSession`
- `TestRunAgentPreCommand_*` (all variants)
- `TestRunAgentPostCommand_*` (all variants)
- `TestDryRun_PrintsHooksWithoutExecuting`
- `TestExecutionOrder_SkipsUnconfigured`
- And others...

### Skipped Tests (still using mockClaudeClient)

| Test | Reason for Skip | Required Delay |
|------|-----------------|----------------|
| `TestRunPostPromptWithRetry_RetryableError_EventualSuccess` | Uses real `time.Sleep` for backoff | ~3s |
| `TestRunPostPromptWithRetry_MaxRetriesExceeded` | Uses real `time.Sleep` for backoff | ~31s |
| `TestRunPhaseWithRetry_RateLimitError` | Uses real `time.Sleep` for backoff | ~60s |
| `TestRunPhaseWithRetry_OverloadedError` | Uses real `time.Sleep` for backoff | ~30s |

### Root Cause

The skipped tests use `mockClaudeClient` which implements the legacy `claudeRunner` interface:

```go
// internal/orbit/orbit.go:74-80
type claudeRunner interface {
    RunPhase(sessionID string, resume bool) (*agents.RunResult, error)
    RunCustomPrompt(prompt string) (*agents.RunResult, error)
    RunCustomPromptWithSession(prompt, sessionID string, resume bool) (*agents.RunResult, error)
}
```

The `runPhase()` method (line 820) calls `o.claudeClient.RunPhase()` rather than using the `agent` interface:

```go
// internal/orbit/orbit.go:820
result, err := o.claudeClient.RunPhase(sessionID, isResume)
```

## Required Changes

### 1. Update runPhase() to Use Agent Interface

**File**: `internal/orbit/orbit.go`

Replace the `claudeClient.RunPhase()` call with agent interface calls:

```go
// Before (current):
result, err := o.claudeClient.RunPhase(sessionID, isResume)

// After (proposed):
var result *agents.RunResult
var err error
if isResume {
    result, err = o.agent.Resume(o.shutdownCtx, sessionID, agents.RunOptions{
        Prompt:     o.config.Command,
        WorkingDir: o.config.WorkingDir,
    })
} else {
    result, err = o.agent.Run(o.shutdownCtx, agents.RunOptions{
        Prompt:     o.config.Command,
        WorkingDir: o.config.WorkingDir,
        SessionID:  sessionID,
    })
}
```

### 2. Update Session Invalid Fallback

The session invalid retry logic (lines 829-842) also uses `claudeClient`:

```go
// Before:
result, err = o.claudeClient.RunPhase(sessionID, false)

// After:
result, err = o.agent.Run(o.shutdownCtx, agents.RunOptions{
    Prompt:     o.config.Command,
    WorkingDir: o.config.WorkingDir,
    SessionID:  sessionID,
})
```

### 3. Remove Legacy claudeRunner Interface

After migration, the following can be removed from `orbit.go`:
- `claudeRunner` interface (lines 74-80)
- `claudeClient` field from `Orbit` struct (line 180)
- `rawClaudeClient` field from `Orbit` struct (line 193)
- Claude client initialization in `New()` (lines 221-226)

### 4. Update Test Files

**File**: `internal/orbit/orbit_test.go`

Remove `mockClaudeClient` type and update tests:

```go
// Before:
mock := &mockClaudeClient{
    runPhaseFunc: func(sessionID string, resume bool) (*agents.RunResult, error) {
        // ...
    },
}
o := &Orbit{
    claudeClient: mock,
    // ...
}

// After:
clock := testutil.NewFakeClock(time.Now())
scenario := testutil.NewScenario().
    RetryableError("connection timeout").
    RetryableError("connection timeout").
    Success("test-session", 0.05).
    Build()
agent := testutil.NewTestAgent(t, "test-agent", scenario, testutil.WithClock(clock))
o := &Orbit{
    agent: agent,
    config: Config{Clock: clock},
    // ...
}
```

### 5. Enable Skipped Tests

After the above changes, remove `t.Skip()` calls from:
- `TestRunPostPromptWithRetry_RetryableError_EventualSuccess`
- `TestRunPostPromptWithRetry_MaxRetriesExceeded`
- `TestRunPhaseWithRetry_RateLimitError`
- `TestRunPhaseWithRetry_OverloadedError`

## Impact Analysis

### Files to Modify

| File | Changes |
|------|---------|
| `internal/orbit/orbit.go` | Replace claudeClient calls with agent interface, remove legacy types |
| `internal/orbit/orbit_test.go` | Migrate 4 skipped tests, remove mockClaudeClient |
| `internal/claude/client.go` | May become unused (evaluate for removal) |

### Risk Assessment

| Risk | Mitigation |
|------|------------|
| Breaking production code paths | Ensure all agent implementations support the same semantics as claudeClient |
| Session ID handling differences | Verify SessionID is properly passed through RunOptions |
| Error classification changes | Agent interface already returns classified errors via RunResult.ErrorClass |

### Testing Strategy

1. Run existing passing tests to verify no regressions
2. Enable and verify the 4 previously-skipped tests
3. Run full integration tests with each agent type (claude-code, codex, kiro, copilot, opencode)
4. Verify variant mode still works correctly

## Estimated Effort

| Task | Effort |
|------|--------|
| Update runPhase() to use agent interface | 2-3 hours |
| Update runPostPromptWithRetry() to use agent interface | 1-2 hours |
| Migrate skipped tests to testutil.TestAgent | 2-3 hours |
| Remove legacy claudeRunner code | 1 hour |
| Integration testing | 2-3 hours |
| **Total** | **8-12 hours** |

## Dependencies

- No external dependencies
- Requires understanding of the agents.Agent interface
- Should be done in a feature branch with PR review

## Recommendation

This migration should be prioritized for a future sprint because:

1. **Test coverage**: The 4 skipped tests cover critical retry and error handling paths
2. **Code simplification**: Removing the legacy claudeRunner interface reduces complexity
3. **Consistency**: All agent invocations should go through the same interface
4. **FakeClock support**: Enables fast, deterministic testing without real delays

The current state is acceptable for MVP as the functionality is tested via other code paths, but completing the migration would improve maintainability and test confidence.
