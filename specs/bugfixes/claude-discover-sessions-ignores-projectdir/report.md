# Bugfix Report: claude-discover-sessions-ignores-projectdir

**Date:** 2026-03-17
**Status:** Fixed

## Description of the Issue

`Agent.DiscoverSessions` in `internal/agents/claudecode/agent.go` accepts a `projectDir` parameter but ignores it entirely, always walking all subdirectories of `~/.claude/projects/`. This causes `apsis list` and any caller to receive unrelated sessions and degrades performance for users with many projects.

**Reproduction steps:**
1. Have multiple project session directories under `~/.claude/projects/`
2. Call `DiscoverSessions(ctx, "/Users/alice/projectA")`
3. Observe that sessions from *all* projects are returned, not just projectA

**Impact:** Moderate — incorrect session listings and unnecessary I/O for every session discovery call.

## Investigation Summary

- **Symptoms examined:** `DiscoverSessions` returns sessions for every project regardless of the `projectDir` argument.
- **Code inspected:** `internal/agents/claudecode/agent.go` lines 88-137, `BuildProjectPath`, and other agent implementations for comparison.
- **Hypotheses tested:** The `projectDir` parameter was simply never wired into the directory scanning logic.

## Discovered Root Cause

The `DiscoverSessions` method reads all entries from `sessionDir` (i.e., `~/.claude/projects/`) and iterates over every subdirectory without checking whether it matches the requested `projectDir`.

**Defect type:** Missing filter logic (unused parameter)

**Why it occurred:** The method was initially written to list *all* sessions, and the `projectDir` filter was added to the interface signature but never implemented in this agent.

**Contributing factors:** No test existed that verified the filtering behaviour.

## Resolution for the Issue

**Changes made:**
- `internal/agents/claudecode/agent.go` — Extracted scanning logic into `discoverSessionsIn(ctx, sessionDir, projectDir)` and `readProjectSessions(projectPath, projectName)`. When `projectDir` is non-empty, `BuildProjectPath(projectDir)` computes the hashed folder name and only that folder is scanned. When empty, all folders are scanned (preserving existing behaviour).

**Approach rationale:** Minimal change that reuses the existing `BuildProjectPath` function and keeps the `DiscoverSessions` public method signature unchanged.

**Alternatives considered:**
- Adding a `sessionDir` field to `Agent` and overriding in tests — more invasive, not needed since the extracted function is directly testable.

## Regression Test

**Test file:** `internal/agents/claudecode/agent_test.go`
**Test names:**
- `TestDiscoverSessions_FiltersByProjectDir` — verifies only matching project sessions are returned
- `TestDiscoverSessions_NoProjectDir_ReturnsAll` — verifies empty `projectDir` returns all sessions
- `TestDiscoverSessions_ProjectDir_NonexistentProject` — verifies graceful empty result for unknown projects

**What it verifies:** The `projectDir` parameter correctly filters sessions to a single project hash folder when provided, returns all when empty, and handles missing project folders gracefully.

**Run command:** `go test ./internal/agents/claudecode/ -run TestDiscoverSessions -v`

## Affected Files

| File | Change |
|------|--------|
| `internal/agents/claudecode/agent.go` | Extract `discoverSessionsIn` and `readProjectSessions`; filter by `projectDir` when non-empty |
| `internal/agents/claudecode/agent_test.go` | Add three regression tests for `projectDir` filtering |

## Verification

**Automated:**
- [x] Regression tests pass
- [x] Full test suite passes (30 packages)
- [x] No new linter issues

## Prevention

**Recommendations to avoid similar bugs:**
- When adding parameters to interface methods, ensure all implementations use them
- Add tests that exercise filter parameters with both empty and non-empty values

## Related

- Transit ticket: T-396
