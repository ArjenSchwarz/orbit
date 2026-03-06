# Bugfix Report: Kiro IDE ResolvePath TOCTOU and Missing Validation

**Date:** 2026-03-05
**Status:** Fixed
**Ticket:** T-314

## Description of the Issue

`ResolvePath(SourceKiroIDE)` calls `resolveKiroIDE` (which validates the best `.chat` path with `IsPathWithinDir`), then immediately closes the file and re-derives the path via `findKiroIDEPath` without any `IsPathWithinDir` check. This creates a TOCTOU (Time-of-Check-Time-of-Use) window where a new `.chat` symlink could be added or a different file could win selection, and follow mode would return a different/unvalidated path.

**Reproduction steps:**
1. Place a symlinked `.chat` file in a Kiro IDE workspace directory that points outside the workspace
2. Call `ResolvePath(SourceKiroIDE, sessionID)` where the symlinked file matches the session
3. The path is returned without validation, bypassing the `IsPathWithinDir` check

**Impact:** Security issue -- symlinked `.chat` files pointing outside the workspace directory could be returned by `ResolvePath`, potentially allowing path traversal in follow mode.

## Investigation Summary

- **Symptoms examined:** `ResolvePath` for KiroIDE performs two independent filesystem scans: one via `resolveKiroIDE` (validated) and one via `findKiroIDEPath` (unvalidated)
- **Code inspected:** `internal/sessions/resolver.go` -- `resolveKiroIDE`, `findKiroIDEPath`, `ResolvePath`
- **Hypotheses tested:** Confirmed `findKiroIDEPath` is only called from `ResolvePath` and has no `IsPathWithinDir` check

## Discovered Root Cause

`findKiroIDEPath` duplicates the scanning logic from `resolveKiroIDE` but omits the `IsPathWithinDir` validation. The `ResolvePath` method wastefully calls `resolveKiroIDE` (which opens a file), closes the file, then calls `findKiroIDEPath` to re-derive the path -- creating both a TOCTOU window and returning an unvalidated path.

**Defect type:** Missing validation / TOCTOU race condition

**Why it occurred:** `ResolvePath` was added after `resolveKiroIDE` for follow mode support. Since `ResolvedSession` doesn't expose the file path, the code worked around this by re-deriving the path via a separate function, but the validation was not carried over.

**Contributing factors:** Code duplication between `resolveKiroIDE` and `findKiroIDEPath` made it easy to miss the validation in the copy.

## Resolution for the Issue

**Changes made:**
- `internal/sessions/resolver.go` -- Added `IsPathWithinDir` check to `findKiroIDEPath`; simplified `ResolvePath` KiroIDE case to call `findKiroIDEPath` directly instead of the wasteful resolve-close-rederive pattern; refactored `resolveKiroIDE` to use `findKiroIDEPath` internally to eliminate code duplication

**Approach rationale:** Adding validation to `findKiroIDEPath` and using it as the single source of truth for path finding eliminates both the TOCTOU window and the code duplication. The `ResolvePath` case no longer opens a file just to close it.

**Alternatives considered:**
- Adding a `FilePath` field to `SessionMetadata` -- Would work but adds a field that's only useful for one caller (`ResolvePath`), and most sessions don't need it
- Re-running `IsPathWithinDir` on the `findKiroIDEPath` result -- Fixes the missing validation but leaves the TOCTOU window and code duplication intact

## Regression Test

**Test file:** `internal/sessions/resolver_test.go`
**Test name:** `TestFindKiroIDEPath_RejectsSymlinkOutsideWorkspace`

**What it verifies:** A symlinked `.chat` file pointing outside the workspace directory is rejected by `findKiroIDEPath` with a "session not found" error.

**Run command:** `go test ./internal/sessions/ -run TestFindKiroIDEPath`

## Affected Files

| File | Change |
|------|--------|
| `internal/sessions/resolver.go` | Add `IsPathWithinDir` to `findKiroIDEPath`, simplify `ResolvePath` KiroIDE case, refactor `resolveKiroIDE` to use `findKiroIDEPath` |
| `internal/sessions/resolver_test.go` | Add regression tests for symlink rejection, regular file acceptance, and session-not-found |

## Verification

**Automated:**
- [x] Regression test passes
- [x] Full test suite passes
- [x] Linters/validators pass

## Prevention

**Recommendations to avoid similar bugs:**
- When adding a path-returning variant of an existing validated function, ensure validation is applied in both paths
- Avoid duplicating filesystem scanning logic -- extract shared helpers and apply validation in the shared code
- Review all callers when adding security checks to ensure the check is consistently applied
