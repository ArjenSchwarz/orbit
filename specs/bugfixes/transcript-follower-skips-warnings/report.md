# Bugfix Report: transcript-follower-skips-warnings

**Date:** 2026-03-17
**Status:** Fixed

## Description of the Issue

`readAndHashLines` in `internal/transcript/follow.go` silently drops warnings for malformed mid-file lines when multiple consecutive lines fail to parse. Only the last malformed line in a consecutive run is tracked, and if it happens to be the final line, no warning is emitted at all.

**Reproduction steps:**
1. Create a JSONL transcript with 3+ consecutive malformed lines in the middle, followed by a valid line
2. Call `readAndHashLines` on the file
3. Observe only 1 warning emitted (for the last malformed line) instead of 3

**Impact:** Corrupted transcript debugging is harder because corrupt mid-file lines are silently ignored without any warnings to aid diagnosis.

## Investigation Summary

- **Symptoms examined:** Warning output from `readAndHashLines` with consecutive malformed lines
- **Code inspected:** `internal/transcript/follow.go` lines 234-292, `readAndHashLines` function
- **Hypotheses tested:** The `pendingBadLine` single-slot tracking overwrites prior bad lines without warning

## Discovered Root Cause

The `pendingBadLine`/`pendingLineNum` pattern uses a single slot to defer warnings. When a line fails to parse, it overwrites the slot without checking if a previous bad line was already stored. This means only the *last* consecutive bad line is ever warned about, and only if a valid line follows it.

**Defect type:** Logic error — single-slot overwrite loses prior malformed line state

**Why it occurred:** The original design assumed malformed lines would be isolated (not consecutive), so a single pending slot sufficed. The deferral pattern (warn only when the next valid line confirms it's mid-file, not EOF) is correct in concept but doesn't handle the consecutive case.

**Contributing factors:** No test coverage for consecutive malformed lines.

## Resolution for the Issue

**Changes made:**
- `internal/transcript/follow.go:264-269` — When a new bad line is encountered and `pendingBadLine` is already set, emit the warning for the pending line before overwriting (it's provably mid-file since another line follows it)

**Approach rationale:** Minimal change that preserves the existing deferral pattern. The last bad line at EOF is still silently skipped (may be incomplete), but all prior consecutive bad lines get their warnings.

**Alternatives considered:**
- Collect all bad lines and warn in batch at end — more complex, changes warning timing
- Always warn immediately including final line — would produce false warnings for legitimately incomplete EOF lines

## Regression Test

**Test file:** `internal/transcript/follow_test.go`
**Test names:** `TestReadAndHashLines_ConsecutiveMalformedLines`, `TestReadAndHashLines_ConsecutiveMalformedAtEnd`

**What it verifies:** Each corrupt mid-file line in a consecutive run produces a warning; final-line-only bad lines remain silently skipped.

**Run command:** `go test ./internal/transcript/ -run "TestReadAndHashLines_Consecutive"`

## Affected Files

| File | Change |
|------|--------|
| `internal/transcript/follow.go` | Emit warning for prior pending bad line before overwriting |
| `internal/transcript/follow_test.go` | Add regression tests for consecutive malformed lines |

## Verification

**Automated:**
- [x] Regression test passes
- [x] Full test suite passes
- [x] Linters/validators pass

## Prevention

**Recommendations to avoid similar bugs:**
- When using single-slot deferral patterns, always handle the "overwrite" case
- Add test cases for consecutive/repeated edge cases, not just single-occurrence cases

## Related

- Transit T-462
