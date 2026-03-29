# Bugfix Report: kiro-empty-project-path

**Date:** 2026-03-29
**Status:** Fixed
**Ticket:** T-534

## Description of the Issue

`ListAll("")` should return all sessions across all agent sources. However, `listKiro` passed the empty `projectPath` directly to `DiscoverForDirectory`, which called `filepath.Abs("")`. In Go, `filepath.Abs("")` resolves to the current working directory, causing the query to return only Kiro sessions for the CWD instead of all sessions.

**Reproduction steps:**
1. Call `lister.ListAll("")` (empty project path to list all sessions)
2. Kiro sessions exist for multiple project directories in the SQLite database
3. Only sessions matching the current working directory are returned

**Impact:** Moderate — `apsis` session listing and any tooling using `ListAll("")` would show incomplete Kiro CLI sessions. All other agents (Claude, Codex, Copilot) correctly returned all sessions.

## Investigation Summary

- **Symptoms examined:** `listKiro` returns partial results when projectPath is empty
- **Code inspected:** `internal/sessions/lister.go` (all `list*` functions), `internal/agents/kiro/logs/discover.go`, `internal/agents/kiro/logs/path.go`
- **Hypotheses tested:** Compared Kiro's empty-path handling against Claude, Codex, and Copilot — all three have explicit `if projectPath == ""` guards; Kiro did not

## Discovered Root Cause

`listKiro` lacked an empty-path check that all other agent listers had. It passed the empty string directly to `DiscoverForDirectory`, which called `normalizePath("")` → `filepath.Abs("")` → current working directory. The query then filtered to only that directory.

**Defect type:** Missing validation / missing code path

**Why it occurred:** When the Kiro session lister was implemented, the empty-path case (list all sessions) was not considered. The Kiro backend uses a SQLite database with directory-keyed rows, unlike other agents that use filesystem-based discovery where "list all" is naturally handled by directory traversal.

**Contributing factors:** `filepath.Abs("")` silently succeeds (returns CWD) rather than returning an error, masking the issue.

## Resolution for the Issue

**Changes made:**
- `internal/agents/kiro/logs/discover.go` — Added `DiscoverAll` method on `DB` and convenience function that queries all sessions without directory filtering
- `internal/sessions/lister.go:293-307` — Added empty-path check in `listKiro`: calls `logs.DiscoverAll` when path is empty, `logs.DiscoverForDirectory` otherwise

**Approach rationale:** Consistent with how Claude, Codex, and Copilot handle empty paths — return all sessions. A `DiscoverAll` method is the natural SQL equivalent of "don't filter by directory".

**Alternatives considered:**
- Skip Kiro sessions with warning when path is empty — rejected because it would provide a worse user experience and is inconsistent with other agents
- Return error for empty path — rejected because the callers expect empty path to mean "all sessions"

## Regression Test

**Test file:** `internal/agents/kiro/logs/discover_test.go`
**Test names:** `TestDiscoverAll_ReturnsAllSessions`, `TestDiscoverAll_EmptyDatabase`, `TestDiscoverAll_SortsByUpdatedAtDesc`, `TestDiscoverAll_PopulatesMetadata`

**What it verifies:** `DiscoverAll` returns sessions from multiple directories, handles empty databases, sorts by updated_at DESC, and populates all metadata fields.

**Run command:** `go test ./internal/agents/kiro/logs/... -run TestDiscoverAll -v`

## Affected Files

| File | Change |
|------|--------|
| `internal/agents/kiro/logs/discover.go` | Added `DiscoverAll` method and convenience function |
| `internal/sessions/lister.go` | Added empty-path guard in `listKiro` |
| `internal/agents/kiro/logs/discover_test.go` | Added 4 regression tests for `DiscoverAll` |

## Verification

**Automated:**
- [x] Regression tests pass
- [x] Full test suite passes
- [x] Linters pass (no new issues)

## Prevention

**Recommendations to avoid similar bugs:**
- When adding a new agent lister, check the empty-path case explicitly — follow the pattern established by Claude, Codex, and Copilot
- Consider adding a shared integration test that verifies `ListAll("")` returns sessions from all agent sources
- `filepath.Abs("")` returning CWD is a known Go footgun — validate inputs before calling path functions

## Related

- T-146: Similar bug in Claude session listing with empty project path
- T-374: Similar bug in Copilot session listing with empty project path
