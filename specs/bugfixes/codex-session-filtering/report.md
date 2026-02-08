# Bugfix Report: Codex Session Filtering by Project Path

**Date:** 2026-02-08
**Status:** Fixed

## Description of the Issue

When running `apsis -l` (list sessions), Codex sessions were not filtered by the current working directory. All Codex sessions from `~/.codex/sessions/` were returned regardless of which project directory the user was in.

**Reproduction steps:**
1. Run `apsis -l` in project directory `/project-a`
2. Observe that sessions from all projects (not just `/project-a`) appear in the list with the `[codex]` tag

**Expected behavior:** Only Codex sessions created in the current project directory should be listed, matching the behavior of Claude Code, Copilot, and Kiro session listing.

**Impact:** Users with multiple projects using Codex would see sessions from unrelated projects, making it hard to find the right session.

## Investigation Summary

Compared filtering logic across all agent types in `listAllSessions()`:

| Agent | Filtered by project path | How |
|-------|-------------------------|-----|
| Claude Code | Yes | Project path encoded in directory structure |
| Copilot | Yes | `workspace.yaml` metadata with `git_root`/`cwd` |
| Kiro CLI | Yes | SQLite-based directory discovery |
| Kiro IDE | Yes | Workspace directory lookup |
| **Codex** | **No** | **No filtering logic** |

Codex sessions store the working directory in the `session_meta` entry's `cwd` field, but this field was never read during session listing.

## Discovered Root Cause

**Defect type:** Missing Feature / Incomplete Implementation

`listCodexSessions(homeDir string)` at `cmd/apsis/main.go:505` accepted only `homeDir` and had no `projectPath` parameter. It walked all `.jsonl` files in `~/.codex/sessions/` without any filtering.

The call site in `listAllSessions()` at line 1024 passed only `homeDir`:
```go
codexSessions, err := listCodexSessions(homeDir)
```

While every other agent's list function received and used `projectPath` for filtering.

**Why it occurred:** Codex sessions are stored in a flat directory structure (`~/.codex/sessions/YYYY/MM/DD/session-{uuid}.jsonl`) unlike Claude Code which encodes the project path into the directory name. The initial implementation didn't read the `cwd` field from the session metadata for filtering.

## Resolution for the Issue

**Changes made:**
- `cmd/apsis/main.go` - Added `getCodexSessionCwd(path string) string` function that reads the `cwd` field from the `session_meta` payload in the first line of a Codex session file
- `cmd/apsis/main.go` - Changed `listCodexSessions` signature from `(homeDir string)` to `(homeDir, projectPath string)` and added filtering logic that skips sessions whose `cwd` doesn't match `projectPath`
- `cmd/apsis/main.go` - Updated the call in `listAllSessions` to pass `projectPath`

**Filtering behavior:**
- Sessions with `cwd` matching `projectPath` are included
- Sessions with `cwd` set to a different path are excluded
- Sessions without `cwd` (legacy files) are excluded when `projectPath` is set
- When `projectPath` is empty, no filtering is applied

## Regression Test

**Test file:** `cmd/apsis/main_test.go`

**New test:** `TestListCodexSessions_FiltersByProjectPath`
- Creates three Codex session files: one for `/project-a`, one for `/project-b`, one with no `cwd`
- Filters for `/project-a` and verifies only the matching session is returned

**Existing tests updated:**
- `TestListCodexSessions_Basic` - Updated to use new signature and added `cwd` to test data
- `TestListCodexSessions_NonExistentDirectory` - Updated to use new signature
- `TestListCodexSessions_IgnoresEmptyFiles_Negative` - Added `cwd` to valid session data
- `TestUnifiedSessionListing_MergeClaudeAndCodex` - Added `cwd` to codex session data
- `TestUnifiedSessionListing_OnlyCodexAvailable` - Added `cwd` to codex session data
- `TestIntegration_ListWithOnlyCodexSessions` - Added `cwd` to codex session data
- `TestIntegration_SessionOutputFormat` - Added `cwd` to codex session data
- `TestUnifiedSessionListing_SessionSortOrder` - Added `cwd` to codex session data

**Run command:** `go test ./cmd/apsis/... -run TestListCodexSessions`

## Affected Files

| File | Change |
|------|--------|
| `cmd/apsis/main.go` | Added `getCodexSessionCwd` function; updated `listCodexSessions` signature and filtering logic; updated `listAllSessions` call |
| `cmd/apsis/main_test.go` | Added regression test; updated existing tests to use new signature and include `cwd` in test data |

## Verification

**Automated:**
- [x] Regression test passes
- [x] Full test suite passes (`make test`)
- [x] Linters/validators pass (`make lint`)

## Prevention

**Recommendations:**
- When adding session listing for new agents, always implement project path filtering from the start
- The Codex session format already contained the `cwd` field needed for filtering; future implementations should check what metadata is available before deciding filtering isn't possible
