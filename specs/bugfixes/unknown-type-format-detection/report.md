# Bugfix Report: Unknown Type Format Detection Failure

**Date:** 2026-02-09
**Status:** Fixed

## Description of the Issue

Apsis failed with "unrecognized log format: type field value 'file-history-snapshot'" when parsing Claude Code session files that contained new/unknown entry types. The `file-history-snapshot` type was introduced in newer Claude Code versions but was not in the parser's list of recognized types.

**Reproduction steps:**
1. Run `apsis a9582b89-552a-436d-8b0e-dc1a221f0840` (a session file containing only `file-history-snapshot` entries)
2. Observe error: `failed to detect format: unrecognized log format: type field value 'file-history-snapshot'`

**Impact:** Any Claude Code session file starting with (or containing only) unknown entry types would fail to parse, even if it contained valid conversation entries after the unknown ones.

## Investigation Summary

- **Symptoms examined:** Error message pointed to `detectJSONLFormat` returning error on first unknown type
- **Code inspected:** `internal/transcript/parser.go` — `detectJSONLFormat`, `ParseJSONL`, `Parse`
- **Hypotheses tested:** The format detection treated unknown types as fatal errors rather than skipping them, unlike the actual parsers (e.g., `parseClaudeJSONL`) which already skip unknown types gracefully

## Discovered Root Cause

**Defect type:** Overly strict validation during format detection

**Why it occurred:** Format detection was designed to fail fast on any unrecognized type field value, rather than skipping unknown entries and continuing to look for recognized ones. This made it fragile against new entry types added by Claude Code.

**Contributing factors:** Three code paths had the same issue:
1. `detectJSONLFormat` (line 177): returned error immediately on unknown type
2. `ParseJSONL` (line 358): returned error on unknown non-infrastructure type
3. `Parse` (line 277): propagated detection failure as hard error with no fallback

## Resolution for the Issue

**Changes made:**
- `internal/transcript/parser.go:53` - Added `file-history-snapshot` to `infrastructureTypes`
- `internal/transcript/parser.go:177` - Changed `detectJSONLFormat` to `continue` past unknown types instead of returning error
- `internal/transcript/parser.go:351-352` - Changed `ParseJSONL` to skip unknown types instead of failing
- `internal/transcript/parser.go:182` - Return chunk data from `detectJSONLFormat` even on error (for fallback)
- `internal/transcript/parser.go:273-284` - Added fallback in `Parse()`: when detection fails but chunk data exists, fall back to Claude parser

**Approach rationale:** The Claude parser (`parseClaudeJSONL`) already handles unknown types gracefully by filtering to only `user` and `assistant` entries. By falling back to it when detection fails, files with unknown types produce 0 entries instead of hard errors.

**Alternatives considered:**
- Only adding `file-history-snapshot` to the known types list - Would not prevent the same issue recurring with future new types
- Making `detectJSONLFormat` default to Claude format on unknown types - Would incorrectly classify non-Claude files

## Regression Test

**Test file:** `internal/transcript/parser_test.go`
**Test names:** `TestDetectFormat_SkipsUnknownTypesToFindFormat`, `TestDetectFormat_OnlyUnknownTypes`, `TestParse_FileHistorySnapshotOnly`, `TestParseJSONL_SkipsUnknownTypesBeforeFormat`

**What it verifies:**
- Unknown types are skipped during format detection (not treated as errors)
- Files with unknown types followed by known types detect format correctly
- Files with only unknown types return 0 entries (not an error)
- The specific `file-history-snapshot` scenario works end-to-end

**Test data:** `internal/transcript/testdata/file_history_snapshot.jsonl`

**Run command:** `go test ./internal/transcript/ -run "TestDetectFormat_SkipsUnknown|TestDetectFormat_OnlyUnknown|TestParse_FileHistory|TestParseJSONL_SkipsUnknown" -v`

## Affected Files

| File | Change |
|------|--------|
| `internal/transcript/parser.go` | Skip unknown types in detection, add fallback in Parse() |
| `internal/transcript/parser_test.go` | Add regression tests, update existing tests for new skip behavior |
| `internal/transcript/testdata/file_history_snapshot.jsonl` | Test data for regression test |
| `cmd/apsis/main_test.go` | Update tests expecting errors for unknown/invalid content |

## Verification

**Automated:**
- [x] Regression test passes
- [x] Full test suite passes
- [x] Linters/validators pass

**Manual verification:**
- `apsis a9582b89-552a-436d-8b0e-dc1a221f0840` now outputs "Session contains no entries" (exit 0) instead of failing

## Prevention

**Recommendations to avoid similar bugs:**
- Format detection should be resilient to new entry types — skip what you don't recognize
- Parsing and detection should have matching strictness levels (parsers were already lenient, detection was not)
- When Claude Code adds new entry types, apsis should degrade gracefully rather than fail
