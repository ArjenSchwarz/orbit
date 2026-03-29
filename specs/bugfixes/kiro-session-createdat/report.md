# Bugfix Report: kiro-session-createdat

**Date:** 2026-03-29
**Status:** Fixed
**Ticket:** T-550

## Description of the Issue

Kiro CLI sessions listed by `apsis --list` and the sessions lister showed incorrect creation timestamps. The `CreatedAt` field was populated with the session's last update time instead of its actual creation time, causing sessions to be ordered by last modification rather than creation — inconsistent with all other agent sources.

**Reproduction steps:**
1. Have multiple Kiro CLI sessions with different created/updated timestamps
2. Run `apsis --list` or use the sessions lister
3. Observe that Kiro CLI sessions are ordered by update time, not creation time

**Impact:** Medium — Kiro CLI session ordering was inconsistent with Claude, Codex, Copilot, and Kiro IDE sources. Users selecting "latest" sessions could get the most recently *updated* session rather than the most recently *created* one.

## Investigation Summary

- **Symptoms examined:** `SessionInfo.CreatedAt` populated with wrong timestamp for Kiro CLI sessions
- **Code inspected:** `internal/sessions/lister.go` (listKiro), `internal/agents/kiro/logs/discover.go` (SessionMetadata struct)
- **Hypotheses tested:** Compared listKiro mapping against listClaude, listCodex, listCopilot, and listKiroIDE — all others correctly use the creation timestamp

## Discovered Root Cause

In `listKiro()`, the field mapping from `logs.SessionMetadata` to `SessionInfo` used `s.UpdatedAt` instead of `s.CreatedAt`.

**Defect type:** Copy-paste / field selection error

**Why it occurred:** The `SessionMetadata` struct has both `CreatedAt` and `UpdatedAt` fields. When writing the mapping loop, `UpdatedAt` was used instead of `CreatedAt` — likely because `DiscoverForDirectory()` sorts results by `UpdatedAt DESC`, making it the more prominent field in that context.

**Contributing factors:** No test coverage existed for the `listKiro` function, so the incorrect mapping was never caught.

## Resolution for the Issue

**Changes made:**
- `internal/sessions/lister.go:315` — Changed `CreatedAt: s.UpdatedAt` to `CreatedAt: s.CreatedAt`
- `internal/sessions/lister.go:23-29` — Added `kiroDiscoverFunc` type and injectable field to `Lister` for testability
- `internal/sessions/lister.go:302-306` — `listKiro` uses injectable discover function (defaults to `logs.DiscoverForDirectory`)

**Approach rationale:** One-line fix for the field mapping, plus minimal dependency injection to enable unit testing without requiring a real SQLite database.

**Alternatives considered:**
- Test via real SQLite DB setup — rejected because `createTestDB` is package-internal to `kiro/logs` and the overhead is unnecessary for testing a simple field mapping

## Regression Test

**Test file:** `internal/sessions/lister_test.go`
**Test name:** `TestListKiroUsesCreatedAtNotUpdatedAt`

**What it verifies:** When a Kiro session has different CreatedAt and UpdatedAt values, the resulting `SessionInfo.CreatedAt` matches the source's `CreatedAt` (not `UpdatedAt`).

**Run command:** `go test ./internal/sessions/ -run TestListKiroUsesCreatedAtNotUpdatedAt -v`

## Affected Files

| File | Change |
|------|--------|
| `internal/sessions/lister.go` | Fixed `CreatedAt` mapping; added injectable discover function |
| `internal/sessions/lister_test.go` | Added regression test with mock Kiro discover |

## Verification

**Automated:**
- [x] Regression test passes
- [x] Full test suite passes (29 packages)
- [x] Linters pass (pre-existing issues in unrelated files only)

## Prevention

**Recommendations to avoid similar bugs:**
- Ensure all `list*` functions in the sessions lister have dedicated unit tests
- Consider adding a linter rule or code review checklist item for field mapping correctness

## Related

- T-550: Fix Kiro session CreatedAt in sessions lister
