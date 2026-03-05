# Bugfix Report: Orchestrator Hardcodes exitCode=1 in Classify Calls

**Date:** 2026-03-05
**Status:** Fixed

## Description of the Issue

Six call sites in `single.go` and `variants.go` pass a hardcoded `1` to `classifier.Classify()` instead of the actual `result.ExitCode`. This means no error classifier can use exit codes for classification, even though `RunResult.ExitCode` is populated by the shared executor.

**Reproduction steps:**
1. An agent returns a `RunResult` with a non-1 exit code (e.g., 137 for SIGKILL, 2 for misuse)
2. The orchestrator classifies the error
3. The classifier receives exit code `1` regardless of the actual exit code

**Impact:** Low immediate impact since no current classifier uses exit codes. However, it blocks future classifiers from leveraging exit codes for more precise classification (e.g., distinguishing SIGKILL from auth errors).

## Investigation Summary

Searched all `Classify(` calls in `internal/orbit/` to identify hardcoded exit code values.

- **Symptoms examined:** All 6 call sites passing literal `1` as exit code
- **Code inspected:** `internal/orbit/single.go` (4 direct calls + `classifyFromAgent` helper), `internal/orbit/variants.go` (2 indirect calls via `classifyFromAgent`)
- **Hypotheses tested:** Confirmed `RunResult.ExitCode` is populated by the shared executor in `internal/agents/executor.go`

## Discovered Root Cause

When the error classification code was written, it used a hardcoded exit code of `1` as a placeholder. The `classifyFromAgent` helper function (used by variant mode) and the 4 direct `o.errorClassifier.Classify()` calls in `runPhase` and `runPostPrompt` all pass `1` instead of `result.ExitCode`.

**Defect type:** Hardcoded value instead of using available data

**Why it occurred:** The exit code parameter was likely added to the `Classify` interface before the shared executor was extracting real exit codes. The hardcoded `1` was a reasonable default that worked because no classifier examined exit codes.

**Contributing factors:** The duplicated classification logic across 4 inline call sites made the pattern harder to spot and fix consistently.

## Resolution for the Issue

**Changes made:**
- `internal/orbit/single.go` -- `classifyFromAgent`: Use `result.ExitCode` when result is non-nil, default to `1` when nil
- `internal/orbit/single.go` -- Added `classifyRunError` method to consolidate the duplicated classification pattern from the 4 direct call sites. This method extracts fields from the result (or error if result is nil) and passes the real exit code to the classifier
- `internal/orbit/single.go` -- Replaced 4 inline classification blocks in `runPhase` and `runPostPrompt` with calls to `classifyRunError`

**Approach rationale:** Extracting the duplicated classification logic into `classifyRunError` fixes all 4 direct call sites at once and reduces code duplication. The `classifyFromAgent` helper was fixed inline since it has a different structure (closure-based, used by variant mode).

**Alternatives considered:**
- Fix only the hardcoded `1` values without extracting a method -- simpler but leaves the duplication that caused the bug to exist in 4 places
- Pass `result` to the classifier interface directly -- would require changing the `ErrorClassifier` interface and all implementations, disproportionate for this fix

## Regression Test

**Test file:** `internal/orbit/orbit_test.go`
**Test names:** `TestClassifyFromAgent_PassesExitCode`, `TestDirectClassify_PassesExitCode`

**What they verify:**
- `classifyFromAgent` forwards `result.ExitCode` to the classifier (tested with exit codes 0, 2, 42, 137)
- `classifyRunError` forwards `result.ExitCode` to the classifier
- Both default to exit code `1` when result is nil (no exit code available)
- Uses `exitCodeCapturingClassifier` mock that records the exit code passed to `Classify`

**Run command:** `go test ./internal/orbit -run "TestClassifyFromAgent_PassesExitCode|TestDirectClassify_PassesExitCode" -v`

## Affected Files

| File | Change |
|------|--------|
| `internal/orbit/single.go` | Fixed `classifyFromAgent` to use `result.ExitCode`; added `classifyRunError` method; replaced 4 inline classification blocks |
| `internal/orbit/orbit_test.go` | Added regression tests with exit code capturing classifier |

## Verification

**Automated:**
- [x] Regression test passes
- [x] Full test suite passes
- [x] Linters/validators pass

**Manual verification:**
- Confirmed tests fail with the old hardcoded `1` and pass with the fix

## Prevention

**Recommendations to avoid similar bugs:**
- Prefer extracting shared logic into methods rather than duplicating inline code -- the 4 duplicated classification blocks made it easy to introduce and perpetuate this bug
- When adding parameters to interfaces (like `exitCode` to `Classify`), ensure all call sites pass meaningful values rather than placeholders

## Related

- T-126: Transit ticket tracking this bug
- T-78: Previous bugfix for nil RunResult handling in the same classification code paths
