# Bugfix Report: copilot-empty-projectpath

**Date:** 2025-07-25
**Status:** Fixed

## Description of the Issue

When `projectPath` is empty, listing Copilot sessions returns zero results instead of returning all sessions. This contrasts with Claude and Codex session listing, which correctly return all sessions when no project filter is specified.

A secondary bug was also found: `Agent.DiscoverSessions()` in the copilot package skipped directories, but Copilot sessions are stored as directories — so it always returned zero results.

**Reproduction steps:**
1. Have Copilot sessions in `~/.copilot/session-state/`
2. Call `Lister.ListAll("")` with an empty projectPath
3. Observe that zero Copilot sessions are returned

**Impact:** Session listing commands (e.g., `apsis` without a project filter) silently omit all Copilot sessions.

## Investigation Summary

- **Symptoms examined:** `listCopilot("")` returns 0 sessions despite valid sessions existing on disk
- **Code inspected:** `internal/sessions/lister.go` (listCopilot), `internal/agents/copilot/agent.go` (DiscoverSessions), compared with listClaude and listCodex patterns
- **Hypotheses tested:** Confirmed the path-matching condition at line 269 always filters out sessions when projectPath is empty

## Discovered Root Cause

**Bug 1 (`listCopilot`):** The filtering condition `matchPath != "" && normalizePath(matchPath) != normalizePath(projectPath)` compares session paths against `normalizePath("")`. Since most sessions have a non-empty `git_root` or `cwd`, this evaluates to `true` for all real sessions, filtering them all out.

**Bug 2 (`DiscoverSessions`):** The condition `if entry.IsDir() { continue }` skips directories, but Copilot sessions are stored as directories containing `workspace.yaml` and `events.jsonl`.

**Defect type:** Logic error — missing guard for empty input

**Why it occurred:** The Copilot listing was added without handling the empty-projectPath case that Claude (via `listClaudeAllProjects`) and Codex (via `if projectPath != ""` guard) already handled.

**Contributing factors:** No test existed for the empty-projectPath case on Copilot, unlike Claude and Codex which had explicit tests.

## Resolution for the Issue

**Changes made:**
- `internal/sessions/lister.go:265-271` — Wrapped the path-matching filter in `if projectPath != ""` so that an empty projectPath returns all sessions (matching Codex pattern)
- `internal/agents/copilot/agent.go:74-110` — Rewrote `DiscoverSessions` to process directories (not skip them), look for `events.jsonl` inside each session directory, and skip empty transcript files

**Approach rationale:** The Codex pattern (`if projectPath != "" { ... filter ... }`) is the simplest and most consistent approach. An empty projectPath means "no filter" — return everything.

**Alternatives considered:**
- Delegating to a `listCopilotAllProjects` helper (Claude pattern) — unnecessary complexity since Copilot sessions are flat (not grouped by project)

## Regression Test

**Test file:** `internal/sessions/lister_test.go`
**Test name:** `TestListAllCopilotSessionsEmptyProjectPath`

**What it verifies:** When `ListAll("")` is called with an empty projectPath, all Copilot sessions from different projects are returned.

**Run command:** `go test ./internal/sessions/ -run TestListAllCopilotSessionsEmptyProjectPath -v`

## Affected Files

| File | Change |
|------|--------|
| `internal/sessions/lister.go` | Guard path-matching filter with `projectPath != ""` |
| `internal/agents/copilot/agent.go` | Fix DiscoverSessions to process directories and read events.jsonl |
| `internal/sessions/lister_test.go` | Add regression test for empty projectPath |

## Verification

**Automated:**
- [x] Regression test passes
- [x] Full test suite passes (30/30 packages)
- [x] Build succeeds

## Prevention

**Recommendations to avoid similar bugs:**
- When adding a new agent's session listing, always add an empty-projectPath test (following the Claude/Codex pattern)
- Consider extracting the "empty means no filter" pattern into a shared helper to enforce consistency
