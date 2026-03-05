# Bugfix Report: Variant New Run Mixes Base Commits

**Date:** 2026-03-05
**Status:** In Progress

## Description of the Issue

When starting a new variant run with existing metadata (`continueExisting=false`), `Manager.Setup()` cleans up unfinished variants but preserves completed ones. It does not update `metadata.BaseCommit`, and new branches are created at the current HEAD. If HEAD has moved since the original run, this mixes base commits across variants: completed variants use the old base, new variants use the current HEAD. Comparisons later use `metadata.BaseCommit` for all diffs, producing incorrect results for newly-created variants.

**Reproduction steps:**
1. Run `orbit run --variants 3` — all variants complete at commit `abc123`
2. Only variant 1 completes; variants 2 and 3 fail
3. Make a new commit (HEAD is now `def456`)
4. Re-run `orbit run --variants 3`, choose "new run"
5. Variant 1 is preserved (branched from `abc123`), variants 2/3 are recreated at `def456`
6. Comparison uses `abc123` as base for all variants, producing incorrect diffs for variants 2/3

**Impact:** Comparison results and reports are silently incorrect when completed variants are preserved across a HEAD change. This can lead to wrong variant recommendations.

## Investigation Summary

- **Symptoms examined:** `Setup()` flow when `continueExisting=false` with existing metadata containing completed variants
- **Code inspected:** `internal/variants/manager.go` (Setup, CleanupUnfinished), `internal/orbit/comparison.go` (runComparison uses BaseCommit)
- **Hypotheses tested:** Confirmed that `CleanupUnfinished` preserves `m.metadata` when completed variants exist, causing the `m.metadata == nil` check on line 232 to skip fresh metadata creation

## Discovered Root Cause

**Defect type:** Missing validation

**Why it occurred:** `Setup()` captures `headCommit` from current HEAD and uses it for new branches, but when completed variants are preserved, the old metadata (with the old `BaseCommit`) is reused unchanged. There is no check that `headCommit == metadata.BaseCommit`.

**Contributing factors:** The `CleanupUnfinished` method correctly preserves completed variants and their metadata, but `Setup` was not designed to handle the case where HEAD has moved since the preserved metadata was created.

## Resolution for the Issue

*(To be filled after fix is implemented)*

## Regression Test

**Test file:** `internal/variants/manager_test.go`
**Test names:**
- `TestSetup_NewRunErrorsWhenHEADDiffersFromBaseCommit` — verifies Setup errors when HEAD != BaseCommit with preserved completed variants
- `TestSetup_NewRunSucceedsWhenHEADMatchesBaseCommit` — verifies the happy path works
- `TestSetup_NewRunNoCompletedVariantsAllowsDifferentHEAD` — verifies fresh runs (no preserved variants) work regardless of HEAD

**What it verifies:** That Setup rejects mixed base commits when preserving completed variants, but allows normal operation otherwise.

**Run command:** `go test ./internal/variants/ -run "TestSetup_NewRun" -v`

## Affected Files

| File | Change |
|------|--------|
| `internal/variants/manager.go` | Add validation in Setup |
| `internal/variants/manager_test.go` | Add regression tests, fix existing test |

## Verification

**Automated:**
- [ ] Regression test passes
- [ ] Full test suite passes
- [ ] Linters/validators pass

## Prevention

**Recommendations to avoid similar bugs:**
- When preserving state across runs, always validate that shared invariants (like base commit) still hold
- Test the "partial preservation" path explicitly with differing state

## Related

- Transit ticket T-191
