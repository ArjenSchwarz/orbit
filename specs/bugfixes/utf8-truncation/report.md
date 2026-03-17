# Bugfix Report: utf8-truncation

**Date:** 2025-07-14
**Status:** Fixed
**Ticket:** T-372

## Description of the Issue

`FormatToolUse` and `FormatLastAction` in `internal/transcript/last_entry.go` truncated strings by byte length using `len()` and byte-index slicing. When the input contained multibyte UTF-8 characters (emoji, CJK, accented characters), the byte slice could cut in the middle of a multi-byte rune, producing invalid UTF-8 sequences (replacement characters `�`) in status output.

**Reproduction steps:**
1. Run an agent session where a tool_use input contains emoji or non-ASCII characters exceeding 60 chars (e.g., a file path with emoji directory names)
2. View status output via `orbit status`
3. Observe replacement characters (�) in the truncated output

**Impact:** Cosmetic — garbled output in terminal status display when paths or text contain Unicode.

## Investigation Summary

- **Symptoms examined:** Replacement characters in truncated status output
- **Code inspected:** `FormatToolUse` (line 163) and `FormatLastAction` (line 215) in `last_entry.go`
- **Hypotheses tested:** Byte-length truncation confirmed as root cause — `len()` returns byte count, not rune count

## Discovered Root Cause

Both `FormatToolUse` and `FormatLastAction` used `len(s)` (byte count) and `s[:N]` (byte-index slicing) to truncate strings. In Go, strings are byte sequences, and multibyte UTF-8 characters (emoji = 4 bytes, CJK = 3 bytes) can be split mid-rune by byte-based slicing.

**Defect type:** Encoding-aware string handling error

**Why it occurred:** The original code treated Go strings as if indexing by character, but Go string indexing is by byte.

**Contributing factors:** ASCII-only test data didn't catch the issue; the bug only manifests with multibyte characters.

## Resolution for the Issue

**Changes made:**
- `internal/transcript/last_entry.go` — Added `truncateRunes()` helper that uses `utf8.RuneCountInString` for length check and `[]rune` conversion for safe slicing. Applied to both `FormatToolUse` (60-rune limit) and `FormatLastAction` (80-rune limit).

**Approach rationale:** Using `[]rune` conversion is the simplest and most correct approach for rune-aware truncation. The strings are short (≤80 runes) so the allocation is negligible.

**Alternatives considered:**
- Manual `utf8.DecodeRuneInString` iteration — more complex, no meaningful perf benefit at these sizes
- `strings.Cut` at rune boundary — more error-prone to implement correctly

## Regression Test

**Test file:** `internal/transcript/last_entry_test.go`
**Test names:**
- `TestFormatToolUse/truncation_preserves_multibyte_emoji_runes`
- `TestFormatToolUse/truncation_preserves_multibyte_CJK_characters`
- `TestFormatLastAction/text_truncation_preserves_multibyte_emoji_runes_at_80_chars`
- `TestFormatToolUse_UTF8Safety`
- `TestFormatLastAction_UTF8Safety`

**What it verifies:** Truncation of strings containing emoji (4-byte runes) and CJK characters (3-byte runes) produces valid UTF-8 and correct rune-count boundaries.

**Run command:** `go test ./internal/transcript/ -run "TestFormatToolUse|TestFormatLastAction" -v`

## Affected Files

| File | Change |
|------|--------|
| `internal/transcript/last_entry.go` | Added `truncateRunes()` helper; switched `FormatToolUse` and `FormatLastAction` from byte to rune truncation |
| `internal/transcript/last_entry_test.go` | Added 5 regression tests for multibyte truncation safety |

## Verification

**Automated:**
- [x] Regression test passes
- [x] Full test suite passes (30 packages)
- [x] Linters pass (pre-existing warnings in unrelated file only)

## Prevention

**Recommendations to avoid similar bugs:**
- Always use `utf8.RuneCountInString()` / `[]rune` when truncating user-visible strings in Go
- Include multibyte characters in truncation test cases
- Consider adding a `go vet` or linter rule for `s[:N]` on strings that may contain non-ASCII

## Related

- Transit ticket: T-372
