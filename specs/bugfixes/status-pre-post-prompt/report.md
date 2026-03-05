# Bugfix Report: Status Pre/Post-Prompt Session IDs

**Date:** 2026-03-05
**Status:** Fixed

## Description of the Issue

`orbit status` reports "Waiting for activity..." even when a live agent session exists, specifically during pre-prompt or post-prompt execution phases.

**Reproduction steps:**
1. Configure a spec with `pre-prompt` or `post-prompt` in `.orbit.yaml`
2. Run `orbit run --variants N` so that variants use pre/post-prompt
3. While pre-prompt or post-prompt is actively running, invoke `orbit status <spec>`
4. Observe "Waiting for activity..." instead of the actual last action from the live session

**Impact:** Users monitoring long-running variant executions see misleading status during pre-prompt and post-prompt phases. The issue is cosmetic but erodes trust in the status command's accuracy.

## Investigation Summary

- **Symptoms examined:** `orbit status` shows "Waiting for activity..." when agent session is visibly running
- **Code inspected:** `internal/status/gatherer.go` — `GetLiveTranscriptPath()` and `gatherKiroLastAction()`
- **Hypotheses tested:** Single root cause confirmed — session ID lookup only checks `CurrentPhase`

## Discovered Root Cause

Both `GetLiveTranscriptPath` (Claude Code path) and `gatherKiroLastAction` (Kiro path) resolve the active session ID using only `summary.CurrentPhase.SessionID`. During pre-prompt and post-prompt execution, `CurrentPhase` is `nil` because no numbered phase is running. The session ID is stored in `summary.PrePrompt.SessionID` (when `Status == "started"`) or `summary.PostCompletion.SessionID` respectively, but neither is consulted.

**Defect type:** Missing fallback logic

**Why it occurred:** The status gatherer was written during the initial enhanced-status implementation, before pre-prompt and post-prompt were added to Orbit. The session ID lookup was never updated to account for these additional execution stages.

**Contributing factors:** `CurrentPhase`, `PrePrompt`, and `PostCompletion` are independent fields on `logs.Summary` — there is no unified "active session" accessor, making it easy to miss one.

## Resolution for the Issue

**Changes made:**
- `internal/status/gatherer.go` — Added `getActiveSessionID()` helper that checks all three session sources in priority order: CurrentPhase > PrePrompt (started) > PostCompletion
- `internal/status/gatherer.go` — Updated `GetLiveTranscriptPath()` to use `getActiveSessionID()` instead of inline CurrentPhase check
- `internal/status/gatherer.go` — Updated `gatherKiroLastAction()` to use `getActiveSessionID()` instead of inline CurrentPhase check

**Approach rationale:** Extracting a shared helper eliminates the duplication between Claude and Kiro code paths and ensures any future session-source additions are handled in one place.

**Alternatives considered:**
- Inline the fallback logic in both functions — rejected because it duplicates the priority logic and is easy to get out of sync

## Regression Test

**Test file:** `internal/status/gatherer_test.go`
**Test names:** `TestGetActiveSessionID`, `TestGetLiveTranscriptPath/returns_path_when_pre-prompt_is_active`, `TestGetLiveTranscriptPath/returns_path_when_post-prompt_is_active`, `TestGetLiveTranscriptPath/prefers_current_phase_over_pre-prompt`, `TestGetLiveTranscriptPath/ignores_completed_pre-prompt_when_no_current_phase`

**What it verifies:** The active session ID is correctly resolved from pre-prompt, post-prompt, and current-phase sources with proper priority ordering.

**Run command:** `go test ./internal/status/ -run "TestGetActiveSessionID|TestGetLiveTranscriptPath"`

## Affected Files

| File | Change |
|------|--------|
| `internal/status/gatherer.go` | Add `getActiveSessionID()`, use it in `GetLiveTranscriptPath` and `gatherKiroLastAction` |
| `internal/status/gatherer_test.go` | Add regression tests for pre-prompt, post-prompt, and priority ordering |

## Verification

**Automated:**
- [x] Regression test passes
- [x] Full test suite passes
- [x] Linters/validators pass

## Prevention

**Recommendations to avoid similar bugs:**
- When adding new execution stages to the orchestration loop, audit the status gatherer for session ID resolution
- Consider adding a `GetActiveSessionID()` method directly on `logs.Summary` to make it the canonical way to find the current session

## Related

- Transit ticket: T-259
