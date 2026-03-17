# Bugfix Report: Codex Resolver Non-UUID IDs

**Date:** 2026-03-17
**Status:** Fixed
**Ticket:** T-446

## Description of the Issue

Codex sessions whose filenames don't contain a UUID (e.g., `events.jsonl`, `session-local.jsonl`) are listed by `ListAll`/`listCodex` with filename-based IDs (basename without `.jsonl`), but `Resolver.Resolve` silently fails for those IDs because `findCodexSession` only accepts 36-character UUID strings.

**Reproduction steps:**
1. Place a Codex session file without a UUID in its name under `~/.codex/sessions/` (e.g., `events.jsonl`)
2. Run `apsis list` — the session appears with ID `events`
3. Run `apsis view codex:events` — fails with "session not found"

**Impact:** Any Codex session file without a UUID in the filename cannot be opened by ID after being listed. Affects `apsis` session viewing and `orbit status` session resolution.

## Investigation Summary

- **Symptoms examined:** `listCodex` returns non-UUID IDs; `findCodexSession` returns empty string for those IDs
- **Code inspected:** `internal/sessions/lister.go` (listCodex ID generation), `internal/sessions/resolver.go` (findCodexSession matching)
- **Hypotheses tested:** Format mismatch between lister and resolver confirmed as the sole root cause

## Discovered Root Cause

**Defect type:** Format mismatch between producer and consumer

`listCodex` (lister.go:200–204) generates session IDs with a fallback: if no UUID is found in the filename, it uses the full basename without `.jsonl`. However, `findCodexSession` (resolver.go:297–329) had a hard gate at line 299 requiring exactly 36 characters and UUID format, returning empty for anything else. This made it impossible to resolve any ID that `listCodex` produced via the fallback path.

**Why it occurred:** The resolver was written assuming all Codex filenames contain UUIDs. The lister was later updated to handle non-UUID filenames but the resolver was not updated to match.

## Resolution for the Issue

**Changes made:**
- `internal/sessions/resolver.go` — Rewrote `findCodexSession` to support both UUID and non-UUID IDs:
  - Added path traversal guard (rejects `/`, `\`, `..` in session IDs)
  - UUID IDs: matched against UUID substrings in filenames (existing behaviour)
  - Non-UUID IDs: matched against the full basename without `.jsonl` (case-insensitive)

**Approach rationale:** The filename-based matching mirrors exactly how `listCodex` generates IDs, ensuring the resolver can always find what the lister advertises. Path traversal protection was added since non-UUID IDs could contain arbitrary strings.

**Alternatives considered:**
- Storing a session-ID-to-path mapping in the lister — rejected as unnecessarily complex and requiring state management
- Always extracting UUIDs from file content instead of filenames — rejected because it would require reading every file to list sessions

## Regression Test

**Test file:** `internal/sessions/resolver_test.go`
**Test names:** `TestResolveCodexNonUUID` (table-driven with `plain` and `prefix` subtests), `TestResolveCodexNonUUIDPathTraversal`

**What they verify:**
- A file named `events.jsonl` (no UUID) can be resolved by ID `events`
- A file named `session-local.jsonl` (prefix but no UUID) can be resolved by ID `session-local`
- Path traversal attempts like `../../etc/passwd` are rejected

**Run command:** `go test ./internal/sessions/ -run TestResolveCodexNonUUID -v`

## Affected Files

| File | Change |
|------|--------|
| `internal/sessions/resolver.go` | Relaxed `findCodexSession` to accept non-UUID filename-based IDs |
| `internal/sessions/resolver_test.go` | Added 3 regression tests for non-UUID ID resolution |

## Verification

**Automated:**
- [x] Regression tests pass
- [x] Full test suite passes (29 packages)
- [x] Linter clean on changed files (pre-existing issues in unrelated files)

## Prevention

**Recommendations to avoid similar bugs:**
- When a lister produces IDs, ensure the corresponding resolver accepts the full range of ID formats that the lister can generate
- Consider adding a round-trip integration test that lists sessions and then resolves each one by ID

## Related

- Transit ticket: T-446
