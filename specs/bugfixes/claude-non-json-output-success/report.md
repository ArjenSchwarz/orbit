# Bugfix Report: claude-non-json-output-success

**Date:** 2026-03-29
**Status:** Fixed
**Transit:** T-530

## Description of the Issue

The Claude Code agent's `execute()` method silently ignores JSON unmarshal failures when processing CLI stdout. When the Claude CLI outputs non-JSON text (e.g., truncated output, plain-text error messages), the result's `IsError` stays `false` and `Output` stays empty, causing Orbit to mark the run as successful.

**Reproduction steps:**
1. Claude CLI runs with `--output-format json` but produces non-JSON stdout (truncated, error message, etc.)
2. CLI exits with code 0 (no exec error)
3. `json.Unmarshal` fails, but the error is silently ignored
4. Orbit sees `IsError=false`, `Error=nil` and treats it as a successful run

**Impact:** Medium severity — silently drops errors or malformed sessions, leading to false-positive success reports in orchestration.

## Investigation Summary

- **Symptoms examined:** `result.IsError` stays `false` and `result.Output` stays empty when JSON parsing fails
- **Code inspected:** `internal/agents/claudecode/agent.go` — the `execute()` method, lines 238–262
- **Hypotheses tested:** Confirmed that when `json.Unmarshal` fails, the entire success-path `if` block is skipped with no fallback

## Discovered Root Cause

**Defect type:** Missing error handling

In `agent.go` line 241, the JSON unmarshal error is checked with `if jsonErr == nil` but there is no `else` branch. When parsing fails:
- `result.IsError` retains its zero value (`false`)
- `result.Output` retains its zero value (`""`)
- `result.Error` stays `nil`
- The function returns `(result, nil)` since `execResult.Err` is also `nil` for exit code 0

**Why it occurred:** The original implementation assumed Claude CLI stdout would always be valid JSON when `--output-format json` is used, not accounting for truncated output, CLI errors printed as plain text, or other edge cases.

**Contributing factors:** The `execute()` method was not unit-tested for JSON parsing — existing tests only covered argument building.

## Resolution for the Issue

**Changes made:**
- `internal/agents/claudecode/agent.go` — Extracted `processExecResult()` from `execute()` for testability. Added an `else` branch to the JSON unmarshal check that:
  - Sets `result.IsError = true` and populates `result.Errors` with the parse failure message
  - Runs the raw stdout through the Claude error classifier to detect known patterns (rate limits, auth errors, etc.)
  - Falls back to `ErrorClassRetryable` with the JSON parse error when no known pattern matches
  - Returns the classified error as the second return value (non-nil error)
  - Preserves raw stdout in `result.RawJSON` for debugging

**Approach rationale:** The fix is minimal and surgical — it adds handling for the missing `else` branch without changing any existing behavior for valid JSON output or `parsed.IsError` cases. Reusing the existing error classifier ensures consistency with how other error types are handled.

**Alternatives considered:**
- Treating all JSON parse failures as fatal — rejected because truncated output is likely transient and retryable
- Always returning the parse error even when `execResult.Err` is set — rejected because the exec error is more authoritative

## Regression Test

**Test file:** `internal/agents/claudecode/execute_test.go`
**Test names:**
- `TestProcessExecResult_NonJSONOutput_ShouldError` — plain text stdout → error
- `TestProcessExecResult_TruncatedJSON_ShouldError` — truncated JSON → error
- `TestProcessExecResult_NonJSON_RetryableByDefault` — unknown non-JSON defaults to retryable
- `TestProcessExecResult_NonJSON_WithRateLimitMessage` — rate limit in plain text → classified
- `TestProcessExecResult_NonJSON_AuthError` — auth error in plain text → classified
- `TestProcessExecResult_ExecError_TakesPrecedence` — exec error overrides parse error
- `TestProcessExecResult_ValidJSON` — valid JSON still works (regression guard)
- `TestProcessExecResult_ValidJSON_IsError` — `is_error:true` preserved
- `TestProcessExecResult_EmptyStdout_NoError` — empty stdout still OK

**What it verifies:** Non-JSON stdout is detected and surfaced as an error with proper classification, while all existing behavior is preserved.

**Run command:** `go test ./internal/agents/claudecode/ -run TestProcessExecResult -v`

## Affected Files

| File | Change |
|------|--------|
| `internal/agents/claudecode/agent.go` | Added `fmt` import, extracted `processExecResult()`, added JSON parse error handling |
| `internal/agents/claudecode/execute_test.go` | New file with 9 regression tests |

## Verification

**Automated:**
- [x] Regression tests pass (9/9)
- [x] Full test suite passes (29 packages)
- [x] Linter passes on changed package (0 issues)

## Prevention

**Recommendations to avoid similar bugs:**
- Always handle both branches of conditional parsing — use `else` with error propagation
- Unit test output processing independently from CLI execution (the `processExecResult` extraction pattern)
- Consider adding a project-wide lint rule or review checklist item for "silently ignored errors"

## Related

- Transit ticket: T-530
