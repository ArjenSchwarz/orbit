# Bugfix Report: OpenCode Session Listing CreatedAt Fallback

**Date:** 2026-03-05
**Status:** Fixed

## Description of the Issue

OpenCode `DiscoverSessions` leaves `SessionInfo.CreatedAt` as the Go zero time (`0001-01-01 00:00:00`) when no `msg_` files are present in a session directory, or when `parseCreatedTime` returns zero due to a zero/negative unix timestamp. This causes sessions to sort incorrectly and display "year 0001" timestamps in the UI.

**Reproduction steps:**
1. Have an OpenCode session directory with no `msg_` files (or only non-message files)
2. Call `DiscoverSessions` (e.g., via `apsis list`)
3. Observe that `CreatedAt` is `0001-01-01T00:00:00Z` instead of the directory's modification time

**Impact:** Sessions with missing or unparseable message timestamps sort to the bottom/top incorrectly and display nonsensical dates.

## Investigation Summary

- **Symptoms examined:** Sessions showing year 0001 timestamps in listings
- **Code inspected:** `internal/agents/opencode/agent.go` — `DiscoverSessions`, `parseCreatedTime`, `unixToTime`
- **Hypotheses tested:** Two separate code paths that can produce zero times

## Discovered Root Cause

Two defects in `internal/agents/opencode/agent.go`:

1. **Missing fallback in `DiscoverSessions`**: When the `msg_` file loop completes without finding a usable message (no `msg_` files, or all fail to read/unmarshal), `createdAt` stays as `time.Time{}` despite `modTime` being available.

2. **`parseCreatedTime` bypasses fallback via `unixToTime`**: When the raw JSON value is a number that parses to `<= 0`, `unixToTime` returns `time.Time{}`. The function returns this zero value directly instead of falling back.

**Defect type:** Missing fallback / defensive programming gap

**Why it occurred:** The `msg_` file loop was the only path that set `createdAt`. The `modTime` fallback was passed to `parseCreatedTime` but never used at the `DiscoverSessions` level. Additionally, `unixToTime` returning zero was not checked before returning from `parseCreatedTime`.

**Contributing factors:** The `parseCreatedTime` function accepts a `fallback` parameter but the `unixToTime` helper returns zero directly, creating an inconsistency in fallback handling.

## Resolution for the Issue

**Changes made:**
- `internal/agents/opencode/agent.go` — `discoverSessionsIn`: After the `msg_` file loop, fall back to `modTime` when `createdAt` is still zero
- `internal/agents/opencode/agent.go` — `parseCreatedTime`: Check if `unixToTime` returns zero and return `fallback` instead

**Approach rationale:** Minimal, targeted fixes at the two points where zero times can escape. The directory `modTime` is always available as a reasonable fallback.

**Alternatives considered:**
- Refactoring `unixToTime` to accept a fallback — rejected because it changes the helper's contract and the caller (`parseCreatedTime`) already has the fallback

## Regression Test

**Test file:** `internal/agents/opencode/agent_test.go`
**Test names:**
- `TestDiscoverSessions_CreatedAtFallbackToModTime` — no msg_ files in session directory
- `TestDiscoverSessions_CreatedAtFallbackWhenParsingFails` — msg_ file with unparseable timestamp
- `TestParseCreatedTime_ZeroUnixReturnsFallback` — numeric 0, negative, and string "0"

**What it verifies:** `CreatedAt` is never zero when the directory exists and has a valid modTime.

**Run command:** `go test ./internal/agents/opencode/ -run "TestDiscoverSessions_CreatedAt|TestParseCreatedTime_ZeroUnix" -v`

## Affected Files

| File | Change |
|------|--------|
| `internal/agents/opencode/agent.go` | Add modTime fallback in `discoverSessionsIn`; fix `parseCreatedTime` to check `unixToTime` result |
| `internal/agents/opencode/agent_test.go` | Add three regression tests for T-273 |

## Verification

**Automated:**
- [x] Regression test passes
- [x] Full test suite passes
- [x] Linters/validators pass

## Prevention

**Recommendations to avoid similar bugs:**
- When a function accepts a fallback parameter, ensure all return paths honor it — helper functions that return zero values can silently bypass fallback logic
- Session discovery functions should always produce a non-zero `CreatedAt` when the filesystem entry exists

## Related

- Transit ticket: T-273
