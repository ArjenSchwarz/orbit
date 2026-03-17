# Bugfix Report: compare-wrong-variant

**Date:** 2025-03-17
**Status:** Fixed
**Ticket:** T-507

## Description of the Issue

When a 3-variant run has variant 2 fail, comparison runs between variants 1 and 3. If variant 3 wins, Orbit reports it as variant 2. With `--auto-consolidate`, this causes consolidation to run on the failed variant 2 instead of the winning variant 3.

**Reproduction steps:**
1. Run `orbit run --variants 3` where variant 2 fails
2. Comparison runs between variants 1 and 3
3. AI recommends variant 3 — validation rejects it ("must be between 1 and 2")
4. On retry, AI outputs 2 (thinking "second in the list"), validation accepts it
5. Auto-consolidation targets variant 2 (the failed one)

**Impact:** Auto-consolidation runs on a failed variant, producing incorrect results or errors. Manual `orbit finalize` could also pick the wrong variant.

## Investigation Summary

- **Symptoms examined:** Comparison recommending wrong variant ID when non-contiguous variant IDs are compared
- **Code inspected:** `internal/comparison/compare.go`, `internal/orbit/comparison.go`, `internal/comparison/diff.go`, `internal/comparison/prompt.go`
- **Hypotheses tested:** Index/ID mapping mismatch between filtered variants and validation range

## Discovered Root Cause

`CompareUnified()` passes `len(input.Variants)` (the count of filtered/successful variants) to `runComparison()` as `numVariants`. This value is used by `parseAndValidate()` to validate that the recommendation falls within `1..numVariants`.

When variants 1 and 3 are compared (variant 2 filtered out), `len(input.Variants)` is 2, so validation accepts only recommendations 1 or 2. The AI sees variant IDs 1 and 3 in the prompt but cannot output 3 — it gets rejected. On retry, the AI may output 2 (interpreting it as "the second variant"), which passes validation but refers to the failed variant.

**Defect type:** Logic error — using collection length instead of maximum element ID

**Why it occurred:** The code assumed variant IDs are always contiguous starting from 1, matching their count. This is true when all variants succeed, but breaks when any variant fails and is filtered out.

**Contributing factors:** The `validateLearnings()` function has the same parameter and would also reject learnings from high-numbered variants, though this is less critical.

## Resolution for the Issue

**Changes made:**
- `internal/comparison/compare.go:93` — Compute `maxVariantID` from input variants and pass it to `runComparison()` instead of `len(input.Variants)`

**Approach rationale:** Minimal single-point fix that ensures the validation range covers all variant IDs that appear in the prompt. The max ID correctly represents the upper bound of valid recommendations.

**Alternatives considered:**
- Remapping variant IDs to contiguous 1..N in the prompt — rejected because it would require inverse mapping on the result, adding complexity and more potential for bugs
- Passing a set of valid IDs instead of a max — rejected as over-engineering; the current range check is sufficient since recommending a filtered-out variant is handled by the consolidation layer

## Regression Test

**Test file:** `internal/comparison/compare_test.go`
**Test names:** `TestCompareUnified_NonContiguousVariantIDs`, `TestParseAndValidate_NonContiguousVariantIDs`, `TestValidateLearnings_NonContiguousVariantIDs`

**What it verifies:** When only variants 1 and 3 are compared (variant 2 filtered), a recommendation of 3 is accepted and returned correctly.

**Run command:** `go test ./internal/comparison/ -run "NonContiguousVariantIDs" -v`

## Affected Files

| File | Change |
|------|--------|
| `internal/comparison/compare.go` | Use max variant ID instead of variant count for validation |
| `internal/comparison/compare_test.go` | Add 3 regression tests for non-contiguous variant IDs |

## Verification

**Automated:**
- [x] Regression test passes
- [x] Full test suite passes (30 packages)
- [x] Build succeeds

## Prevention

**Recommendations to avoid similar bugs:**
- When validating IDs from a filtered collection, always derive bounds from the actual IDs, not the collection length
- Integration tests should cover the "variant failure" scenario end-to-end

## Related

- T-507: Compare can give the wrong variant
