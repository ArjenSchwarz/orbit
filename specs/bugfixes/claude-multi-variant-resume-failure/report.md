# Bugfix Report: Variant Session Resume Failure Without Pre-Prompt

**Date:** 2026-02-11
**Status:** Fixed

## Description of the Issue

When running multi-variant orchestration without a pre-prompt configured, each variant logs a "session resume failed" warning message before falling back to a fresh session. While the orchestration continues successfully, the warning is misleading and indicates the code is attempting to resume a non-existent session.

**Reproduction steps:**
1. Run `orbit run --variants 2` without a pre-prompt configured
2. Observe log messages: `Variant 1: session resume failed, starting fresh session`
3. The same message appears for each variant on each phase

**Impact:** Low severity. Orchestration completes successfully due to the fallback mechanism, but the warning messages are confusing and indicate incorrect session handling logic.

## Investigation Summary

Used systematic debugging with Fagan Inspection methodology to trace the session handling logic.

- **Symptoms examined:** Warning message appears for all variants even without pre-prompt
- **Code inspected:** `internal/orbit/orbit.go` lines 2007-2026 (variant session handling) and 769-816 (non-variant session handling)
- **Hypotheses tested:**
  - Is a session being created incorrectly? No, the log manager correctly generates new session IDs
  - Why is Resume() being called? Because `continueSessionID` is incorrectly set

## Discovered Root Cause

**Defect: Ignored `isResume` return value from `StartPhase`**

Location: `internal/orbit/orbit.go:2016-2022`

The variant session handling code calls `variantLogManager.StartPhase()` but ignores the second return value (`isResume`):

```go
sessionID, _, err := variantLogManager.StartPhase(phaseNum, o.config.ContinueSession, continueSessionID)
if err != nil {
    o.debug.Log("Variant %d: failed to start phase in log manager: %v", v.ID, err)
} else if continueSessionID == "" {
    // If we didn't have a pre-prompt session to continue, use the generated session ID
    continueSessionID = sessionID  // <-- BUG: sets continueSessionID for NEW sessions
}
```

When `continueSessionID` is empty (no pre-prompt), the code sets it to the log manager's generated session ID. This causes `runVariantPhaseWithRetry` to interpret a non-empty `continueSessionID` as a signal to resume an existing session:

```go
if continueSessionID != "" && attempt == 0 {
    sessionID = continueSessionID
    isResume = true  // <-- Wrongly set to true for new sessions
    ...
}
```

**Comparison with correct implementation:**

The non-variant code at lines 769-790 correctly uses the `isResume` return value:
```go
sessionID, isResume, err = o.logManager.StartPhase(phase, o.config.ContinueSession, overrideSessionID)
```

**Why it occurred:** The variant code was added later and attempted to coordinate session IDs between the log manager and phase execution, but conflated "session ID for tracking" with "session ID to resume."

**Five Whys:**
1. Why does the variant show "session resume failed"? → Because it calls `agent.Resume()` on a non-existent session
2. Why does it call `Resume()` instead of `Run()`? → Because `isResume` is set to `true`
3. Why is `isResume` set to `true`? → Because `continueSessionID != ""`
4. Why is `continueSessionID` non-empty when there's no pre-prompt? → Because lines 2019-2021 set it from the log manager's new session ID
5. Why does the code set `continueSessionID` from a new session? → **Root cause:** The `isResume` return value from `StartPhase` was ignored

## Resolution for the Issue

**Changes made:**

Modified `internal/orbit/orbit.go:2015-2022` to use the `isResume` return value:

```go
sessionID, isResumeFromManager, err := variantLogManager.StartPhase(phaseNum, o.config.ContinueSession, continueSessionID)
if err != nil {
    o.debug.Log("Variant %d: failed to start phase in log manager: %v", v.ID, err)
} else if continueSessionID == "" && isResumeFromManager {
    // Only set continueSessionID if we're resuming an existing session (continue interrupted run)
    continueSessionID = sessionID
}
```

**Approach rationale:** The `StartPhase` return value correctly indicates whether this is a resume scenario (continuing an interrupted run) or a new session. The fix uses this value to only set `continueSessionID` when appropriate.

**Alternatives considered:**
- Removing the session ID coordination entirely: Would break the log manager's session tracking
- Adding a separate `isResume` parameter: Unnecessary since `StartPhase` already returns this information

## Regression Test

**Existing test coverage:**

`internal/logs/manager_test.go:TestStartPhase_NewSession` already verifies that `StartPhase` returns `isResume=false` for new sessions:

```go
sessionID, isResume, err := m.StartPhase(1, true)
// ...
if isResume {
    t.Error("should not be a resume for new session")
}
```

The fix relies on this already-tested behavior being correctly consumed by the orbit code.

**Testing gap:** A full end-to-end integration test that verifies `Run()` is called instead of `Resume()` during variant execution would require significant infrastructure (variant manager, mock git, worktrees). The existing test framework (`testutil.TestAgent.Recorder`) can track method calls, but wiring it through variant execution is complex.

**Run command:** `go test ./internal/logs/ -run TestStartPhase -v`

## Affected Files

| File | Change |
|------|--------|
| `internal/orbit/orbit.go` | Use `isResumeFromManager` return value from `StartPhase` to conditionally set `continueSessionID` |

## Verification

**Automated:**
- [x] All tests pass (`make test`)
- [x] Linters pass (`make lint`)

**Manual verification:**
- The warning message should no longer appear for variant runs without pre-prompt
- Pre-prompt continuation should still work correctly for phase 1
- Continue interrupted run (`--continue-session`) should still resume correctly

## Prevention

**Recommendations to avoid similar bugs:**
- When adding variant-specific code paths, compare with the non-variant implementation to ensure consistent handling
- Avoid ignoring return values (`_`) unless there's a clear reason; consider adding a comment if intentional
- Session handling patterns should be centralized or clearly documented to prevent drift between code paths
