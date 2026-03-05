# Bugfix Report: Usage Limit Reset Without Timezone

**Date:** 2026-03-05
**Status:** Fixed

## Description of the Issue

When Claude CLI outputs a usage-limit reset message without a timezone (e.g., `"You've hit your limit · resets 3am"`), Orbit fails to parse the reset time and classifies the error as Unknown instead of RateLimitWait. This causes Orbit to exit immediately rather than waiting for the usage limit to reset.

**Reproduction steps:**
1. Hit the Claude Code 5-hour usage limit
2. Claude CLI outputs a message like `"You've hit your limit · resets 3am"` (no timezone in parentheses)
3. Orbit exits with an unknown error instead of waiting for the reset

**Impact:** Users who hit the usage limit with a no-timezone message lose their in-progress run. The orchestration stops when it should wait and resume automatically.

## Investigation Summary

The issue is in `parseUsageLimitReset()` in `internal/agents/claudecode/errors.go`.

- **Symptoms examined:** Usage limit messages without timezone cause Orbit to exit
- **Code inspected:** `parseUsageLimitReset()`, `Classify()`, regex pattern at line 83
- **Hypotheses tested:** Single root cause confirmed -- the regex requires `\(([^)]+)\)` (timezone in parentheses) as mandatory

## Discovered Root Cause

The regex pattern in `parseUsageLimitReset()` requires a timezone group `\(([^)]+)\)` at the end. When Claude CLI omits the timezone, the regex does not match, and the function returns 0.

**Defect type:** Missing input variant handling

**Why it occurred:** The function was written to handle only the observed format `"resets <time> (<timezone>)"`. The format without a timezone was not anticipated.

**Contributing factors:** Claude CLI's output format is not formally documented, so different versions or configurations may produce different formats.

## Resolution for the Issue

**Changes made:**
- `internal/agents/claudecode/errors.go:83` - Made the timezone group optional in the regex by wrapping `\(([^)]+)\)` with `(?:...)?`
- `internal/agents/claudecode/errors.go:86-93` - Adjusted match extraction to handle both 5 groups (with timezone) and 4 groups (without), defaulting to `time.Local` when no timezone is present

**Approach rationale:** Defaulting to `time.Local` is the correct choice because Claude CLI most likely displays the reset time in the user's local timezone when it omits the timezone identifier. Using UTC would cause incorrect wait durations for most users.

**Alternatives considered:**
- Default to UTC when no timezone present - Rejected because the displayed time is almost certainly in the user's local timezone
- Treat no-timezone messages as retryable with a fixed backoff - Rejected because we can still parse the actual reset time and wait precisely

## Regression Test

**Test file:** `internal/agents/claudecode/errors_test.go`
**Test names:** `TestClassifier_Classify_UsageLimitWithoutTimezone`, `TestParseUsageLimitReset` (no-timezone subcases)

**What it verifies:** Messages with a reset time but no timezone are correctly classified as RateLimitWait with a positive RetryAfter duration.

**Run command:** `go test ./internal/agents/claudecode/ -run "TestClassifier_Classify_UsageLimitWithoutTimezone|TestParseUsageLimitReset"`

## Affected Files

| File | Change |
|------|--------|
| `internal/agents/claudecode/errors.go` | Make timezone optional in regex, default to local time |
| `internal/agents/claudecode/errors_test.go` | Add regression tests for no-timezone format |

## Verification

**Automated:**
- [x] Regression test passes
- [x] Full test suite passes
- [x] Linters/validators pass

## Prevention

**Recommendations to avoid similar bugs:**
- When parsing external CLI output, consider optional fields and degrade gracefully
- Test with both complete and partial message formats

## Related

- Transit ticket: T-203
