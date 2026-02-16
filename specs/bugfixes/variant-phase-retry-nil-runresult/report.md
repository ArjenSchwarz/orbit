# Bugfix Report: Variant Phase Retry Panics on Nil RunResult

**Date:** 2026-02-16
**Status:** Fixed

## Description of the Issue

When a variant phase retry encounters a nil `RunResult` from an agent (i.e., the agent returns `(nil, error)`), the error classification code panics with a nil pointer dereference. This crashes the entire variant run instead of classifying the error and either retrying or reporting it.

**Reproduction steps:**
1. Run `orbit run --variants 2` with an agent that can return `(nil, error)` (e.g., agent binary not found, process crash before output)
2. The agent returns nil result with an error on any attempt
3. Orbit panics at `classifier.Classify(1, result.Stderr, result.Output, result.Errors)` due to nil `result`

**Impact:** High severity for variant runs. Any transient failure that produces a nil result (agent binary missing, process killed before output, OS-level errors) would crash the entire multi-variant orchestration instead of failing gracefully.

## Investigation Summary

Examined the error classification code paths in the three retry functions.

- **Symptoms examined:** Nil pointer dereference panic at `result.Stderr` / `result.Output` / `result.Errors` when `result` is nil
- **Code inspected:** `internal/orbit/orbit.go` -- `runVariantPhaseWithRetry`, `runVariantPostCompletion`, and `runPhase`
- **Hypotheses tested:** Confirmed all current agent implementations always return non-nil results, but the `agents.Agent` interface contract does not guarantee this. The code path is reachable when the agent process fails before producing any output.

## Discovered Root Cause

The `classifier.Classify()` calls in `runVariantPhaseWithRetry` (line 2257), `runVariantPostCompletion` (line 2324), and `runPhase` (line 853) accessed `result.Stderr`, `result.Output`, and `result.Errors` without checking whether `result` was nil.

**Defect type:** Missing nil guard -- defensive programming gap

**Why it occurred:** The code was written assuming agents always return a non-nil `RunResult`. All current agent implementations do construct a result struct before returning, so the nil case was never triggered in practice. However, the `agents.Agent` interface does not enforce this contract.

**Contributing factors:** The non-variant `runPhaseWithRetry` avoids this issue by delegating error classification to `runPhase` which returns a `ClassifiedError`. The variant functions inline the classification logic, duplicating the pattern without the same nil guards that exist elsewhere (e.g., `isSessionInvalidError` properly checks for nil).

## Resolution for the Issue

**Changes made:**
- `internal/orbit/orbit.go` -- `runVariantPhaseWithRetry`: Added nil guard before `Classify()`. When `result` is nil, extracts error message from `lastErr` for classification.
- `internal/orbit/orbit.go` -- `runVariantPostCompletion`: Same nil guard pattern.
- `internal/orbit/orbit.go` -- `runPhase`: Same nil guard pattern (defensive fix -- same vulnerability existed but was less likely to trigger).

**Approach rationale:** Extract stderr/output/errors from the result when available, or fall back to the error message when result is nil. This preserves the existing classification behavior for non-nil results while providing the classifier with useful error text for the nil case.

**Alternatives considered:**
- Defaulting nil result to an empty `RunResult{}` before classification -- Rejected because it would lose the error message, giving the classifier nothing to work with.
- Adding a nil-result check that always treats nil as retryable -- Rejected because it bypasses the classifier entirely and would mask agent-specific classification logic.

## Regression Test

**Test file:** `internal/orbit/orbit_test.go`
**Test names:** `TestVariantPhaseRetry_NilRunResult`, `TestVariantPhaseRetry_NilRunResult_AllFail`, `TestVariantPostCompletion_NilRunResult`

**What they verify:**
- `runVariantPhaseWithRetry` does not panic when the agent returns `(nil, error)` and returns a proper `ClassifiedError`
- The error message from the original error is preserved in the classified error
- `runVariantPostCompletion` does not panic when the agent returns `(nil, error)`

**Run command:** `go test ./internal/orbit -run "TestVariant.*NilRunResult|TestVariantPostCompletion_NilRunResult" -v`

## Affected Files

| File | Change |
|------|--------|
| `internal/orbit/orbit.go` | Added nil guards before `classifier.Classify()` in three functions |
| `internal/orbit/orbit_test.go` | Added `nilResultAgent` mock and three regression tests |

## Verification

**Automated:**
- [x] Regression test passes
- [x] Full test suite passes
- [x] Linters/validators pass

## Prevention

**Recommendations to avoid similar bugs:**
- When accessing fields on pointers returned from interface methods, always consider the nil case even if current implementations don't produce it
- The variant retry functions (`runVariantPhaseWithRetry`, `runVariantPostCompletion`) duplicate logic from the non-variant path (`runPhase` + `runPhaseWithRetry`). Consider extracting a shared error classification helper to reduce duplication and prevent divergent nil-safety

## Related

- Transit ticket: T-78
