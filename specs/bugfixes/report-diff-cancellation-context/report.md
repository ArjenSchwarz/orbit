# Bugfix Report: report-diff-cancellation-context

**Date:** 2026-03-29
**Status:** Fixed

## Description of the Issue

The `generateReport` method in `internal/orbit/comparison.go` uses `context.Background()` when calling `DiffGatherer.GatherDiffs()`, which creates a non-cancellable context. This means diff gathering continues running even after the parent context is cancelled (e.g., via Ctrl+C or shutdown signal).

Additionally, `GatherDiffs` itself does not check `ctx.Err()` between loop iterations, so even with a proper context, cancellation wouldn't take effect until the next `GetDiff` call returns.

**Reproduction steps:**
1. Run `orbit run --variants N` with multiple variants
2. Cancel the run (Ctrl+C) during report generation
3. Observe that diff gathering continues despite cancellation

**Impact:** Medium — causes unnecessary work after cancellation and delays clean shutdown. Could be significant with many variants or large diffs.

## Investigation Summary

- **Symptoms examined:** `context.Background()` used in `generateReport` at line 244
- **Code inspected:** `internal/orbit/comparison.go`, `internal/comparison/diff.go`, `internal/orbit/variants.go`
- **Hypotheses tested:** Compared with `runComparison`, `runAutoConsolidate`, and `runPostConsolidateCommand` which all properly accept and use context

## Discovered Root Cause

`generateReport` does not accept a `context.Context` parameter and uses `context.Background()` instead. All three callers in `variants.go` have access to a proper context from `RunVariants(ctx)` but cannot pass it because the signature doesn't accept one.

**Defect type:** Missing context propagation

**Why it occurred:** The `generateReport` function was likely written before the cancellation pattern was established across other methods, or context propagation was overlooked during the report generation feature.

**Contributing factors:** `GatherDiffs` also lacks a loop-level `ctx.Err()` check, so even with a proper context, cancellation between iterations depends entirely on the downstream `GetDiff` implementation.

## Resolution for the Issue

**Changes made:**
- `internal/orbit/comparison.go:224` — Added `ctx context.Context` parameter to `generateReport`, replaced `context.Background()` with `ctx`
- `internal/orbit/variants.go:272,280,290` — Updated all three callers to pass `ctx`
- `internal/comparison/diff.go:27` — Added `ctx.Err()` check at start of each loop iteration in `GatherDiffs`

**Approach rationale:** Follows the established pattern used by `runComparison(ctx)`, `runAutoConsolidate(ctx)`, and `runPostConsolidateCommand(ctx)`.

**Alternatives considered:**
- Using `o.shutdownCtx` directly inside `generateReport` — rejected because it breaks the explicit context-passing pattern used everywhere else

## Regression Test

**Test file:** `internal/comparison/diff_test.go`
**Test names:** `TestGatherDiffs_RespectsPreCancelledContext`, `TestGatherDiffs_RespectsExpiredDeadline`, `TestGatherDiffs_SucceedsWithActiveContext`

**What it verifies:** GatherDiffs returns an error immediately when given a cancelled context (without calling GetDiff), and works normally with an active context.

**Run command:** `go test ./internal/comparison/ -run "TestGatherDiffs_"`

## Affected Files

| File | Change |
|------|--------|
| `internal/orbit/comparison.go` | Added `ctx` parameter to `generateReport`, pass to `GatherDiffs` |
| `internal/orbit/variants.go` | Updated 3 callers to pass `ctx` |
| `internal/comparison/diff.go` | Added `ctx.Err()` check in `GatherDiffs` loop |
| `internal/comparison/diff_test.go` | New regression tests |

## Verification

**Automated:**
- [x] Regression test passes
- [x] Full test suite passes
- [x] Linters/validators pass

## Prevention

**Recommendations to avoid similar bugs:**
- Lint for `context.Background()` usage in methods that could receive a context from their caller
- Follow the established pattern: all methods doing I/O or calling external commands should accept `context.Context` as first parameter

## Related

- Transit ticket: T-586
