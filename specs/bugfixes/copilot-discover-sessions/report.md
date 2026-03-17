# Bugfix Report: copilot-discover-sessions

**Date:** 2026-03-17
**Status:** Fixed

## Description of the Issue

The Copilot agent's `DiscoverSessions` method scans `~/.copilot/session-state/` for files and skips directories. However, GitHub Copilot stores sessions as per-session directories containing `events.jsonl` and `workspace.yaml`. This causes `DiscoverSessions` to always return an empty list, breaking features that rely on agent session discovery.

**Reproduction steps:**
1. Run a Copilot session that creates `~/.copilot/session-state/<session-id>/events.jsonl`
2. Call `agent.DiscoverSessions(ctx, projectDir)`
3. Observe: returns empty slice because the loop skips all directory entries

**Impact:** Session discovery returns no results for Copilot, breaking session listing and any features depending on agent-level session discovery.

## Investigation Summary

- **Symptoms examined:** `DiscoverSessions` returns empty slices despite valid Copilot sessions existing on disk
- **Code inspected:** `internal/agents/copilot/agent.go` (DiscoverSessions), `internal/sessions/lister.go` (listCopilot)
- **Hypotheses tested:** Compared the two implementations — the `sessions.Lister.listCopilot` method correctly scans directories while the agent method incorrectly skips them

## Discovered Root Cause

Line 91 of `internal/agents/copilot/agent.go` has `if entry.IsDir() { continue }` which skips directory entries — the exact opposite of what's needed since Copilot stores sessions in directories.

**Defect type:** Logic error (inverted condition)

**Why it occurred:** The original implementation was likely modeled after a file-based session format rather than Copilot's actual directory-based storage layout.

**Contributing factors:** The `sessions.Lister.listCopilot` was implemented correctly but the agent's `DiscoverSessions` was not aligned with it. No regression test existed to catch the discrepancy.

## Resolution for the Issue

**Changes made:**
- `internal/agents/copilot/agent.go` - Rewrote `DiscoverSessions` to scan directories, verify `events.jsonl` exists and is non-empty, parse `workspace.yaml` for project filtering, and use path normalization for symlink-safe comparison.

**Approach rationale:** Aligned the agent's `DiscoverSessions` with the proven logic in `sessions.Lister.listCopilot`, adapted for the `agents.SessionInfo` return type.

**Alternatives considered:**
- Exporting `parseCopilotWorkspace`/`normalizePath` from `sessions` package — rejected to avoid circular dependency risk and keep agents self-contained
- Calling `sessions.Lister.listCopilot` from the agent — rejected because agent and session packages have different types and concerns

## Regression Test

**Test file:** `internal/agents/copilot/agent_test.go`
**Test names:** `TestDiscoverSessions_FindsDirectorySessions`, `TestDiscoverSessions_SkipsEmptyEvents`, `TestDiscoverSessions_FiltersProjectDir`, `TestDiscoverSessions_SkipsMissingEvents`, `TestDiscoverSessions_UsesCreatedAtFromWorkspace`

**What they verify:** Directory-based sessions are discovered; empty/missing events.jsonl are skipped; project filtering by workspace.yaml git_root/cwd works; timestamps from workspace.yaml are used.

**Run command:** `go test ./internal/agents/copilot/ -run TestDiscoverSessions_ -v`

## Affected Files

| File | Change |
|------|--------|
| `internal/agents/copilot/agent.go` | Rewrote DiscoverSessions to scan directories with workspace.yaml filtering |
| `internal/agents/copilot/agent_test.go` | Added 5 regression tests for directory-based session discovery |

## Verification

**Automated:**
- [ ] Regression test passes
- [ ] Full test suite passes
- [ ] Linters/validators pass

## Prevention

**Recommendations to avoid similar bugs:**
- Ensure agent-level `DiscoverSessions` and `sessions.Lister` methods share test fixtures or are validated against the same session layout
- Add integration tests that verify round-trip discovery across both code paths

## Related

- T-408: Copilot DiscoverSessions skips directory-based sessions
- `internal/sessions/lister.go:listCopilot` — the reference implementation
