# Bugfix Report: Copilot Normalize Paths

**Date:** 2026-03-05
**Status:** Fixed

## Description of the Issue

Copilot sessions are not found when the project path contains symlinks or is otherwise differently normalized. The `listCopilot` function in `internal/sessions/lister.go` compares workspace paths using direct string equality, while the equivalent `listCodex` function uses `normalizePath` (which resolves symlinks and cleans paths).

**Reproduction steps:**
1. Have a project directory accessed via a symlink (e.g., `/var` -> `/private/var` on macOS)
2. Create Copilot sessions with the real path stored in `workspace.yaml`
3. Call `ListAll` with the symlinked path
4. Observe that Copilot sessions are missing from results

**Impact:** Users on macOS (where `/tmp` is a symlink to `/private/tmp`) or any environment with symlinked project paths will silently miss all Copilot sessions.

## Investigation Summary

The bug is a straightforward inconsistency between two similar code paths in the same file.

- **Symptoms examined:** Copilot sessions not returned when project path uses a symlink
- **Code inspected:** `internal/sessions/lister.go` — `listCopilot` (line 226), `listCodex` (line 152), `normalizePath` (line 472)
- **Hypotheses tested:** Direct string comparison fails when paths resolve through symlinks — confirmed

## Discovered Root Cause

On line 226 of `lister.go`, `listCopilot` uses `matchPath != projectPath` for path comparison. This is a plain string comparison that does not account for symlinks or path normalization differences.

**Defect type:** Missing normalization (inconsistent code pattern)

**Why it occurred:** The `normalizePath` function was added when Codex support was implemented, but the Copilot code path was not updated to use it.

**Contributing factors:** No test coverage for symlinked paths in the Copilot listing path. On macOS, `os.TempDir()` returns `/tmp` which is a symlink to `/private/tmp`, making this a common scenario.

## Resolution for the Issue

**Changes made:**
- `internal/sessions/lister.go:226` - Replace `matchPath != projectPath` with `normalizePath(matchPath) != normalizePath(projectPath)`

**Approach rationale:** The `normalizePath` function already exists and is used by `listCodex`. Applying it to `listCopilot` makes both code paths consistent and handles symlinks + path cleaning.

**Alternatives considered:**
- Normalizing paths once at the top of `listCopilot` — adds unnecessary complexity for a single comparison point

## Regression Test

**Test file:** `internal/sessions/lister_test.go`
**Test name:** `TestListCopilotNormalizesPathsForComparison`

**What it verifies:** When a Copilot session's workspace path (stored in workspace.yaml) differs from the query path only because of symlink resolution, the session is still found and returned.

**Run command:** `go test ./internal/sessions/ -run TestListCopilotNormalizesPathsForComparison`

## Affected Files

| File | Change |
|------|--------|
| `internal/sessions/lister.go` | Normalize both paths before comparison in `listCopilot` |
| `internal/sessions/lister_test.go` | Add regression test for symlinked Copilot paths |

## Verification

**Automated:**
- [x] Regression test passes
- [x] Full test suite passes
- [x] Linters/validators pass

**Manual verification:**
- Confirmed test fails before fix and passes after

## Prevention

**Recommendations to avoid similar bugs:**
- When adding path comparison logic, always use `normalizePath` rather than direct string equality
- Consider adding a linter rule or code review checklist item for path comparisons

## Related

- Transit ticket: T-290
