# Bugfix Report: codex-session-meta-position

**Date:** 2026-03-29
**Status:** Fixed
**Ticket:** T-644

## Description of the Issue

Codex sessions were invisible to `apsis --list` and session resolution when the `session_meta` JSONL entry was not the first line in the file. Both `getCodexSessionCwd()` and `getCodexSessionTimestamp()` only read the first line, so any file where `session_meta` appeared later was silently skipped.

**Reproduction steps:**
1. Create a Codex session file where line 1 is `response_item` and line 2+ is `session_meta`
2. Run session listing for that project
3. Session is missing from the list

**Impact:** Valid Codex sessions could be invisible to all session discovery — listing, resolution, and `latest` lookups.

## Investigation Summary

- **Symptoms examined:** Sessions missing from `apsis --list` output
- **Code inspected:** `internal/sessions/lister.go` — `getCodexSessionCwd()`, `getCodexSessionTimestamp()`, `listCodex()`
- **Hypotheses tested:** Confirmed both helper functions use `if scanner.Scan()` (single read) instead of a loop

## Discovered Root Cause

**Defect type:** Logic error — single-line read instead of bounded scan

Both `getCodexSessionCwd()` and `getCodexSessionTimestamp()` called `scanner.Scan()` exactly once and checked if that single line was `session_meta`. The JSONL format does not guarantee ordering of entry types.

**Why it occurred:** The original implementation assumed `session_meta` would always be the first entry, which is typical but not guaranteed by the Codex JSONL format.

**Contributing factors:** The transcript parser (`codex_parser.go`) correctly handles `session_meta` at any position, creating an inconsistency with the session discovery code.

## Resolution for the Issue

**Changes made:**
- `internal/sessions/lister.go` — `getCodexSessionTimestamp()`: Changed from single `if scanner.Scan()` to `for` loop scanning up to 50 lines
- `internal/sessions/lister.go` — `getCodexSessionCwd()`: Same change — loop up to 50 lines
- Added `codexMetaScanLimit` constant (50) to bound the scan

**Approach rationale:** A bounded scan is simple, safe, and consistent with the existing code style. 50 lines is generous — `session_meta` should appear very early if present — while still protecting against scanning huge files.

**Alternatives considered:**
- Unbounded scan of entire file — rejected because JSONL files can be large and we only need metadata
- Reading only first N bytes — rejected because line boundaries matter for JSONL parsing

## Regression Test

**Test file:** `internal/sessions/lister_test.go`
**Test names:** `TestCodexSessionMetaNotFirstLine`, `TestGetCodexSessionCwdScansMultipleLines`, `TestGetCodexSessionTimestampScansMultipleLines`

**What they verify:** Sessions are discovered and metadata is correctly extracted when `session_meta` appears after other JSONL entries.

**Run command:** `go test ./internal/sessions/ -run "TestCodexSessionMetaNotFirstLine|TestGetCodexSessionCwdScans|TestGetCodexSessionTimestampScans" -v`

## Affected Files

| File | Change |
|------|--------|
| `internal/sessions/lister.go` | Scan up to 50 lines for `session_meta` instead of only reading line 1 |
| `internal/sessions/lister_test.go` | Added 3 regression tests and a helper for creating JSONL with leading non-meta lines |

## Verification

**Automated:**
- [x] Regression tests pass
- [x] Full test suite passes (all 29 packages)

## Prevention

**Recommendations to avoid similar bugs:**
- When parsing structured files, avoid assuming entry ordering unless the format specification guarantees it
- The existing `codex_parser.go` already handled this correctly — reusing patterns from the full parser would have prevented this

## Related

- T-644: Codex sessions can be skipped when session_meta is not first JSONL line
