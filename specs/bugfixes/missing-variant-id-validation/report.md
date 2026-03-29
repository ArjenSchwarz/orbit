# Bugfix Report: missing-variant-id-validation

**Date:** 2025-03-29
**Ticket:** T-595
**Status:** Fixed

## Description of the Issue

The comparison validator accepts recommendations for variant IDs that don't actually exist in the set of compared variants. When variant IDs are non-contiguous (e.g., variants {1, 3} after variant 2 failed), the validator only checks that the recommendation falls within the range [1, maxVariantID] rather than verifying membership in the actual set.

**Reproduction steps:**
1. Run a 3-variant comparison where variant 2 fails, leaving variants {1, 3}
2. AI returns a recommendation for variant 2
3. Validator accepts it because 2 is in range [1, 3]

**Impact:** Medium — could lead to consolidation or finalization targeting a non-existent variant, causing downstream errors or incorrect results.

## Investigation Summary

- **Symptoms examined:** `parseAndValidate()` uses a range check (`1 ≤ recommendation ≤ maxVariantID`) instead of set membership
- **Code inspected:** `internal/comparison/compare.go` (parseAndValidate, validateLearnings, CompareUnified), `internal/comparison/types.go`
- **Hypotheses tested:** The original code was designed for contiguous IDs but a later commit added non-contiguous support using maxVariantID, which only partially solved the problem

## Discovered Root Cause

Both `parseAndValidate()` and `validateLearnings()` validate variant IDs using a simple range check (`id >= 1 && id <= numVariants`) where `numVariants` is the maximum variant ID. This allows any ID in the range to pass, even if that specific variant doesn't exist.

**Defect type:** Missing validation — set membership check needed instead of range check

**Why it occurred:** The original implementation assumed contiguous variant IDs (1, 2, 3, ...). When non-contiguous ID support was added, the validation was changed from `len(variants)` to `maxVariantID` but the fundamental check remained a range check rather than a set membership check.

**Contributing factors:** The existing test `TestParseAndValidate_NonContiguousVariantIDs` explicitly asserts the incorrect behavior ("variant 2 valid with max ID 3"), masking the bug.

## Resolution for the Issue

**Changes made:**
- `internal/comparison/compare.go` — Changed `parseAndValidate()` and `validateLearnings()` to accept a `map[int]bool` of valid variant IDs instead of an `int` max. Updated recommendation check to use set membership. Updated `runComparison()` and `CompareUnified()` to build and pass the set.

**Approach rationale:** Using a set (`map[int]bool`) is the most direct and idiomatic Go approach for membership testing. It has O(1) lookup and makes the intent clear.

**Alternatives considered:**
- Sorted slice with binary search — more complex for negligible performance difference with small N

## Regression Test

**Test file:** `internal/comparison/compare_test.go`
**Test names:**
- `TestParseAndValidate_RejectsNonExistentVariantRecommendation`
- `TestCompareUnified_RejectsNonExistentVariantRecommendation`
- `TestValidateLearnings_RejectsNonExistentVariantID`

**What they verify:** Recommendations and learnings referencing variant IDs that exist within the numerical range but are not present in the actual variant set are rejected.

**Run command:** `go test ./internal/comparison/ -run "RejectsNonExistent" -v`

## Affected Files

| File | Change |
|------|--------|
| `internal/comparison/compare.go` | Changed validation from range check to set membership |
| `internal/comparison/compare_test.go` | Added regression tests, updated existing tests for new signature |

## Verification

**Automated:**
- [x] Regression test passes
- [x] Full test suite passes
- [x] Linters/validators pass (pre-existing issues only)

## Prevention

**Recommendations to avoid similar bugs:**
- When validating IDs against a collection, prefer set membership over range checks
- Tests for non-contiguous scenarios should include gap IDs as negative cases, not just boundary IDs
