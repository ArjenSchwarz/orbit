# Bugfix Report: consolidation-retry-iserror-success

**Date:** 2026-03-29
**Status:** Fixed
**Ticket:** T-609

## Description of the Issue

The consolidation retry logic in `runWithRetry` incorrectly treated an agent-level `IsError=true` result as a success. When an agent (e.g., OpenCode) returned `IsError=true` with `err=nil` and `result.Error=nil` (e.g., invalid JSON output), the retry classify callback returned nil (success), allowing consolidation to proceed on invalid output.

**Reproduction steps:**
1. Run consolidation with an agent that returns `IsError=true`, `Error=nil`, `ExitCode=0`
2. The `Classify` callback checks `err == nil && (result == nil || result.Error == nil)` — both true
3. Consolidation proceeds as if the agent succeeded

**Impact:** Consolidation phases could be marked as successful when the agent actually reported an error, leading to follow-up steps operating on invalid output.

## Investigation Summary

- **Symptoms examined:** The `Classify` callback in `consolidator.go:runWithRetry` uses `result.Error == nil` for success determination, ignoring `result.IsError`
- **Code inspected:** `internal/consolidation/consolidator.go` (lines 511-515), `internal/orbit/single.go` (`classifyFromAgent` at line 55)
- **Hypotheses tested:** Confirmed that `classifyFromAgent` correctly checks `!result.IsError`, while consolidation's inline classify checks `result.Error == nil`

## Discovered Root Cause

The consolidation retry classify callback checked `result.Error == nil` instead of `!result.IsError` for its success condition. The `Error` field is a Go-level error, while `IsError` is the agent-reported error flag. An agent can report `IsError=true` (indicating it detected a problem) while `Error` remains nil (no Go-level execution failure).

**Defect type:** Incorrect field check — wrong struct field used in success condition

**Why it occurred:** The consolidation classify was written independently from `classifyFromAgent` (used in variant paths) and used a different success condition. The two were never aligned.

**Contributing factors:** `RunResult` has two error-related fields (`Error` and `IsError`) that serve different purposes, making it easy to check the wrong one.

## Resolution for the Issue

**Changes made:**
- `internal/consolidation/consolidator.go:513` — Changed success condition from `result.Error == nil` to `!result.IsError` to match the established `classifyFromAgent` pattern

**Approach rationale:** Aligns with the `classifyFromAgent` behavior in `internal/orbit/single.go:55`, which is the correct pattern used throughout variant mode. The `IsError` field is the canonical agent-reported error flag.

**Alternatives considered:**
- Check both `result.Error == nil && !result.IsError` — overly defensive since `IsError` subsumes the intent; diverges from the established pattern

## Regression Test

**Test file:** `internal/consolidation/consolidator_test.go`
**Test name:** `TestRunWithRetry_IsErrorTreatedAsFailure`

**What it verifies:**
- `IsError=true` with `Error=nil` and `ExitCode=0` is treated as failure (not success)
- `IsError=false` with `Error=nil` is treated as success (no regression)
- `nil` result is treated as success (no regression)

**Run command:** `go test ./internal/consolidation -run TestRunWithRetry_IsErrorTreatedAsFailure -v`

## Affected Files

| File | Change |
|------|--------|
| `internal/consolidation/consolidator.go` | Changed success condition from `result.Error == nil` to `!result.IsError` |
| `internal/consolidation/consolidator_test.go` | Added regression test `TestRunWithRetry_IsErrorTreatedAsFailure` |
| `specs/bugfixes/consolidation-retry-iserror-success/report.md` | This report |

## Verification

**Automated:**
- [x] Regression test passes
- [x] Full test suite passes
- [x] Linters/validators pass

## Prevention

**Recommendations to avoid similar bugs:**
- When writing retry classify callbacks, always use `result.IsError` (not `result.Error`) for success checks — consider adding a helper method like `result.Succeeded()` to encapsulate this
- Align all classify callbacks with the `classifyFromAgent` pattern to avoid drift

## Related

- T-609: Consolidation retry treats agent-level IsError as success
- `internal/orbit/single.go:classifyFromAgent` — the correct pattern this fix aligns with
