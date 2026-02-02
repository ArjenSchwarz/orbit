# Bugfix Report: Status Display and Auto-Consolidate Race Condition

**Date:** 2026-02-02
**Status:** Fixed

## Description of the Issue

Two related bugs were identified in the Orbit orchestrator:

### Bug 1: Claude Status Display Not Showing Activity
When running `orbit status` during a variant run, the Claude status kept showing "Waiting for activity..." even though the agent was actively working.

**Reproduction steps:**
1. Run `orbit run --variants 2` to start a multi-variant run
2. In another terminal, run `orbit status <spec-name>`
3. Observe that the status shows "Waiting for activity..." for running variants despite active Claude sessions

**Impact:** Users could not monitor variant progress during long-running implementations.

### Bug 2: Auto-Consolidate Race Condition
The auto-consolidate feature tried to read the comparison report before it was written to disk, causing consolidation to fail with "comparison-report.md not found".

**Reproduction steps:**
1. Run `orbit run --variants 2 --auto-consolidate`
2. Wait for comparison to complete
3. Auto-consolidation fails because the report file doesn't exist yet

**Impact:** Auto-consolidation feature was broken for all users.

## Investigation Summary

Using the Fagan Inspection methodology, the following was examined:

- **Symptoms examined:** Status gatherer returning `LastActionWaiting`, consolidator returning file-not-found error
- **Code inspected:** `internal/status/gatherer.go`, `internal/orbit/orbit.go`, `internal/logs/manager.go`, `internal/consolidation/consolidator.go`
- **Hypotheses tested:**
  - Path construction issues (ruled out - paths were correct)
  - Log manager integration issues (confirmed)
  - Execution order issues (confirmed)

## Discovered Root Cause

### Bug 1: Missing Log Manager Integration
**Defect type:** Data Flow Issue / Missing Integration

The status gatherer at `internal/status/gatherer.go:256-258` reads `summary.CurrentPhase.SessionID` to find the live transcript path. However, the variant execution code in `runVariantPhaseWithRetry` generates session IDs directly via `uuid.NewString()` and never calls `variantLogManager.StartPhase()` to record them in `summary.json`.

The log manager's `StartPhase()` method is what populates `CurrentPhase` in the summary, but it was never called for variant phases. The variant code only called `SaveSession()` AFTER the phase completes, which doesn't populate `CurrentPhase`.

**Why it occurred:** The variant execution code was written to be independent of the main orchestration loop, but didn't fully integrate with the log manager's phase tracking mechanism.

### Bug 2: Wrong Order of Operations
**Defect type:** Logic Error / Timing Issue

In `runWithVariants()`, the execution order was:
1. `runComparison()` - stores result in `o.comparisonResult` (in memory)
2. `runAutoConsolidate()` - tries to read `comparison-report/report.md` from disk
3. `generateReport()` - writes the report file to disk

The consolidator at `internal/consolidation/consolidator.go:131` reads: `reportPath := filepath.Join(c.config.SpecDir, "comparison-report", "report.md")`

The report file didn't exist when `runAutoConsolidate` was called because `generateReport()` hadn't run yet.

**Why it occurred:** The auto-consolidate feature was added to the existing flow without considering that it depends on the report file existing on disk.

## Resolution for the Issue

**Changes made:**
- `internal/orbit/orbit.go:1953-1969` - Added `variantLogManager.StartPhase()` call before running each phase to populate `CurrentPhase` with the session ID
- `internal/orbit/orbit.go:1997-2002` - Added `variantLogManager.CompletePhase()` call after successful phase completion to clear `CurrentPhase`
- `internal/orbit/orbit.go:1588-1612` - Reordered operations so `generateReport()` runs before `runAutoConsolidate()`

**Approach rationale:**
- For Bug 1: Using the existing log manager API ensures consistency with the non-variant execution path and leverages well-tested code
- For Bug 2: Reordering the operations is the simplest fix and makes the dependency explicit in the code flow

**Alternatives considered:**
- Bug 1: Have the consolidator read session IDs from a different source - Rejected because it would duplicate logic and diverge from the existing design
- Bug 2: Have the consolidator use in-memory comparison result instead of reading from disk - Rejected because the consolidator legitimately needs the formatted report with the "Improvements from Other Variants" section

## Regression Test

**Test files:**
- `internal/status/gatherer_test.go` - Existing test `TestGetLiveTranscriptPath` at line 335 verifies `CurrentPhase` is read correctly
- `internal/logs/manager_test.go` - Existing tests `TestStartPhase_NewSession` and `TestCompletePhase_ClearsCurrentPhase` verify the log manager behavior
- `internal/orbit/integration_test.go` - Added `TestAutoConsolidate_RequiresReportGeneration` at line 1865 documenting the execution order requirement

**What it verifies:**
- `TestGetLiveTranscriptPath` verifies that when `CurrentPhase.SessionID` is set in summary.json, the correct transcript path is returned
- The integration test documents that `generateReport()` must run before `runAutoConsolidate()`

**Run command:** `go test ./internal/status/... ./internal/logs/... ./internal/orbit/... -short`

## Affected Files

| File | Change |
|------|--------|
| `internal/orbit/orbit.go` | Added StartPhase/CompletePhase calls for variant phases; reordered auto-consolidate to run after report generation |
| `internal/orbit/integration_test.go` | Added regression test documenting execution order requirement |

## Verification

**Automated:**
- [x] Regression test passes
- [x] Full test suite passes (`go test ./... -short`)
- [x] Linters/validators pass (`make lint`)

**Manual verification:**
- Build successful (`make build`)
- Code compiles without errors

## Prevention

**Recommendations to avoid similar bugs:**
- When adding new execution phases to variant runs, ensure proper integration with the log manager's tracking APIs (StartPhase/CompletePhase)
- When adding features that read files generated by other steps, explicitly document and verify the execution order dependency
- Consider adding integration tests that verify the full orchestration flow for variant runs with auto-consolidation enabled

## Related

- The status display feature was added in the enhanced-status spec
- Auto-consolidation was recently added as a new feature
- PR review (Issue 1 in `specs/enhanced-status/review-overview-1.md`) previously identified a related path issue that was fixed, but this integration issue was missed
