# Bugfix Report: ListAll Misses Claude Sessions When Project Path Is Empty

**Date:** 2026-03-05
**Status:** Fixed

## Description of the Issue

When `ListAll("")` is called with an empty project path (meaning "return all sessions, no filtering"), Claude sessions are silently omitted from the results. Other agent sources (Codex, Copilot) have similar but separate handling for the empty-path case.

**Reproduction steps:**
1. Have Claude sessions stored under `~/.claude/projects/` for one or more projects
2. Call `lister.ListAll("")` with an empty project path
3. Observe that zero Claude sessions are returned

**Impact:** Any caller that passes an empty project path to `ListAll` gets incomplete results. This affects session discovery when no project filtering is desired.

## Investigation Summary

- **Symptoms examined:** `ListAll("")` returns no Claude sessions despite sessions existing on disk
- **Code inspected:** `internal/sessions/lister.go` (listClaude, listCodex, listCopilot), `internal/agents/claudecode/agent.go` (BuildProjectPath)
- **Hypotheses tested:** Confirmed that `BuildProjectPath("")` returns `""`, which causes `filepath.Join(homeDir, ".claude", "projects", "")` to resolve to the parent `projects/` directory rather than any project-specific subdirectory

## Discovered Root Cause

`listClaude` always calls `BuildProjectPath(projectPath)` and constructs a single directory path. When `projectPath` is empty, `BuildProjectPath("")` returns `""`, so `projectDir` becomes `~/.claude/projects/` (the root). This directory contains project subdirectories, not `.jsonl` session files, so the loop finds nothing.

**Defect type:** Missing edge case handling

**Why it occurred:** The `listClaude` function was written assuming `projectPath` would always be a valid directory path. The empty-path case (meaning "all projects") was not considered.

**Contributing factors:** Other agent sources (Codex, Copilot) handle the empty case differently — Codex explicitly checks `if projectPath != ""` before filtering, while Copilot's filter logic happens to skip sessions with known paths when `projectPath` is empty (a related but separate issue).

## Resolution for the Issue

**Changes made:**
- `internal/sessions/lister.go:listClaude` - Split into three functions: `listClaude` (entry point that checks for empty path), `listClaudeAllProjects` (iterates all project subdirectories), and `listClaudeDir` (reads `.jsonl` files from a single directory)
- When `projectPath` is empty, `listClaudeAllProjects` iterates all subdirectories under `~/.claude/projects/` and collects sessions from each

**Approach rationale:** This mirrors how Codex handles the empty path case — skip the project filter and return everything. Extracting `listClaudeDir` avoids duplicating the session-reading logic between the filtered and unfiltered paths.

**Alternatives considered:**
- Add a separate `ListAllUnfiltered()` method — rejected because the current API contract already supports empty path meaning "all"
- Change the caller to always provide a project path — rejected because the empty-path case is a valid use case (e.g., `apsis serve` without a project flag)

## Regression Test

**Test file:** `internal/sessions/lister_test.go`
**Test name:** `TestListAllClaudeSessionsEmptyProjectPath`

**What it verifies:** That `ListAll("")` returns Claude sessions from multiple project directories when no project filtering is applied.

**Run command:** `go test ./internal/sessions/ -run TestListAllClaudeSessionsEmptyProjectPath`

## Affected Files

| File | Change |
|------|--------|
| `internal/sessions/lister.go` | Handle empty projectPath in listClaude; extract listClaudeDir and listClaudeAllProjects |
| `internal/sessions/lister_test.go` | Add regression test TestListAllClaudeSessionsEmptyProjectPath |
| `docs/agent-notes/apsis-session-listing.md` | Document empty project path handling |

## Verification

**Automated:**
- [x] Regression test passes
- [x] Full test suite passes
- [x] Linters/validators pass

## Prevention

**Recommendations to avoid similar bugs:**
- When writing list/filter functions, explicitly handle the "no filter" case (empty string) at the top of the function
- Add test coverage for the empty-path case whenever a function takes an optional filter parameter

## Related

- Transit ticket: T-146
