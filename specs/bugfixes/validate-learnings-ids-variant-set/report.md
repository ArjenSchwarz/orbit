# Bugfix Report: validate-learnings-ids-variant-set

**Date:** 2026-03-29
**Status:** Fixed
**Transit:** T-625

## Description of the Issue

Comparison learnings and cross-variant improvements used range-based validation (`1 ≤ id ≤ maxVariantID`) instead of checking against the actual compared variant set. When variants fail and are excluded (e.g., variant 2 fails in a 3-variant run leaving {1, 3}), the AI could reference the excluded variant and it would pass validation.

**Reproduction steps:**
1. Run a 3-variant comparison where variant 2 fails
2. Only variants {1, 3} are compared
3. AI output includes `{ "variant_id": 2, ... }` in learnings
4. Current parser accepts this because 2 ≤ maxVariantID(3)

**Impact:** Reports could contain guidance attributed to a non-compared/non-existent variant. Consolidation prompts could be influenced by invalid source-variant references.

## Investigation Summary

- **Symptoms examined:** `validateLearnings()` uses `numVariants` (maxVariantID) as upper bound; `CrossVariantImprovement.SourceVariantID` has no validation at all
- **Code inspected:** `internal/comparison/compare.go` — `CompareUnified`, `runComparison`, `parseAndValidate`, `validateLearnings`
- **Hypotheses tested:** Confirmed that the range check allows any ID between 1 and maxVariantID, including IDs of failed/excluded variants

## Discovered Root Cause

**Defect type:** Missing validation / Incorrect validation logic

**Why it occurred:** The original T-507 fix changed recommendation validation from count-based to maxVariantID-based, but learnings validation was updated the same way (range-based) rather than using set-based validation. Cross-variant improvements were never validated at all.

**Contributing factors:** The parameter was named `numVariants` (suggesting a count) but actually held `maxVariantID`, making the range-based check seem correct at a glance.

## Resolution for the Issue

**Changes made:**
- `internal/comparison/compare.go` — Changed `validateLearnings` to accept `map[int]bool` (valid ID set) instead of `int` (maxVariantID)
- `internal/comparison/compare.go` — Changed `parseAndValidate` and `runComparison` to accept and propagate `map[int]bool`
- `internal/comparison/compare.go` — Changed recommendation validation from range-based to set-based
- `internal/comparison/compare.go` — Added `validateCrossVariantImprovements` function for set-based validation of improvement source variant IDs
- `internal/comparison/compare.go` — `CompareUnified` now builds the valid ID set from `input.Variants`

**Approach rationale:** Set-based validation is the correct approach because the compared variant set can have gaps. A map lookup is O(1) and more semantically accurate than a range check.

**Alternatives considered:**
- Keep range-based and add exclusion list — More complex and error-prone than a simple set membership check

## Regression Test

**Test file:** `internal/comparison/compare_test.go`
**Test names:**
- `TestValidateLearnings/non-contiguous:_gap_variant_rejected` — Verifies variant 2 is rejected when only {1, 3} are compared
- `TestValidateCrossVariantImprovements/non-contiguous:_gap_variant_rejected` — Same for cross-variant improvements
- `TestParseAndValidate_NonContiguousVariantIDs/variant_2_invalid_not_in_set_{1,3}` — Verifies recommendation 2 is rejected
- `TestParseAndValidate_FiltersCrossVariantImprovements` — End-to-end test for improvement filtering

**Run command:** `go test ./internal/comparison/ -run "NonContiguous|CrossVariant" -v`

## Affected Files

| File | Change |
|------|--------|
| `internal/comparison/compare.go` | Set-based validation for learnings, improvements, and recommendation |
| `internal/comparison/compare_test.go` | Updated all tests to use `map[int]bool`; added regression tests |

## Verification

**Automated:**
- [x] Regression test passes
- [x] Full test suite passes
- [x] Linters pass (pre-existing issues in unrelated file only)

## Prevention

**Recommendations to avoid similar bugs:**
- When validating IDs against a collection, prefer set membership over range checks
- Name parameters precisely (`validIDs` vs `numVariants`) to make incorrect usage obvious
- Validate all ID fields in a response, not just the primary one (recommendation was fixed but learnings/improvements were missed)
