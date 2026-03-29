# Bugfix Report: kiro-ide-chat-starttime-createdat

**Date:** 2025-03-29
**Status:** Fixed
**Transit:** T-555

## Description of the Issue

`resolveKiroIDE` in `internal/sessions/resolver.go` used the `.chat` file's filesystem modification time (`info.ModTime()`) as the `CreatedAt` timestamp on `SessionMetadata`. Meanwhile, `listKiroIDE` in the same package already parsed the JSON metadata to use `metadata.startTime` (milliseconds since epoch). This caused timestamps to disagree between listing sessions and viewing them.

**Reproduction steps:**
1. List Kiro IDE sessions via `apsis` — observe `CreatedAt` derived from `metadata.startTime`
2. Resolve/view the same session — observe `CreatedAt` derived from file modTime
3. Timestamps differ because file modTime reflects last write, not session start

**Impact:** Low severity — cosmetic inconsistency in displayed timestamps between list and detail views.

## Investigation Summary

- **Symptoms examined:** Timestamp mismatch between `listKiroIDE` and `resolveKiroIDE`
- **Code inspected:** `resolver.go` (resolveKiroIDE), `lister.go` (listKiroIDE, kiroIDEChatHeader types), `kiro_ide_types.go` (KiroIDEMetadata)
- **Hypotheses tested:** Single root cause confirmed — resolver simply didn't parse the chat metadata

## Discovered Root Cause

`resolveKiroIDE` set `CreatedAt: info.ModTime()` without consulting the `.chat` file's JSON metadata, while the lister already had the correct logic using `metadata.startTime` with a modTime fallback.

**Defect type:** Missing feature parity — the resolver was never updated to use the startTime metadata that the lister already handled.

**Why it occurred:** The resolver was written using the same pattern as other file-backed resolvers (`openFileSession`), which only use `os.Stat`. The Kiro IDE format is unique in embedding a `startTime` in JSON metadata.

## Resolution for the Issue

**Changes made:**
- `internal/sessions/resolver.go` — Added `kiroIDECreatedAt(rs io.ReadSeeker, modTime time.Time) time.Time` helper that parses the chat header's `metadata.startTime`, falling back to modTime. Updated `resolveKiroIDE` to call it. Reuses the existing `kiroIDEChatHeader` type from `lister.go`.

**Approach rationale:** Mirrors the exact logic from `listKiroIDE` (lines 397-402 of lister.go) — use `time.UnixMilli(startTime)` when startTime > 0, otherwise fall back to modTime. The reader is seeked back to position 0 so downstream consumers can still read the full file.

**Alternatives considered:**
- Reading file into memory and returning a `bytes.Reader` — unnecessary complexity; seeking is sufficient.
- Extracting a shared helper used by both lister and resolver — the lister operates on a struct field, not a reader, so the contexts are different enough that sharing would be forced.

## Regression Test

**Test file:** `internal/sessions/resolver_test.go`
**Test names:**
- `TestKiroIDECreatedAt_UsesStartTime`
- `TestKiroIDECreatedAt_FallsBackToModTime`
- `TestKiroIDECreatedAt_FallsBackWhenStartTimeZero`
- `TestKiroIDECreatedAt_SeeksBackToStart`

**What they verify:** startTime is used when present, modTime fallback when absent or zero, and reader position is reset.

**Run command:** `go test ./internal/sessions/ -run TestKiroIDECreatedAt -v`

## Affected Files

| File | Change |
|------|--------|
| `internal/sessions/resolver.go` | Added `kiroIDECreatedAt` helper; updated `resolveKiroIDE` to use it |
| `internal/sessions/resolver_test.go` | Added 4 regression tests |

## Verification

**Automated:**
- [x] Regression tests pass
- [x] Full test suite passes
- [x] Linters pass (no new issues)

## Prevention

**Recommendations to avoid similar bugs:**
- When a format embeds timestamp metadata, all code paths that surface timestamps should use the same source
- Consider adding a shared `createdAt` method to a session type so lister and resolver can't diverge
