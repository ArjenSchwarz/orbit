# Bugfix Report: GMT Usage-Limit Reset Uses DST Offset

**Date:** 2026-03-29
**Status:** Fixed

## Description of the Issue

The usage-limit reset time parser in the Claude Code agent maps the "GMT" timezone abbreviation to `Europe/London`, which observes British Summer Time (BST = UTC+1) during summer months. GMT is a fixed offset (UTC+0, no DST), so during BST season the calculated wait duration is off by 1 hour.

**Reproduction steps:**
1. Claude Code emits a usage-limit message with "GMT" timezone, e.g. `resets 3am (GMT)`
2. During summer (late March – late October), the parser loads `Europe/London` which applies BST (UTC+1)
3. The reset time is computed 1 hour later than intended

**Impact:** Wait durations for usage-limit resets can be 1 hour too long or too short during BST season, delaying or prematurely resuming orchestration.

## Investigation Summary

- **Symptoms examined:** `parseTimezoneAbbrev("GMT")` returns `Europe/London` location
- **Code inspected:** `internal/agents/claudecode/errors.go` — `parseTimezoneAbbrev` and `parseUsageLimitReset`
- **Hypotheses tested:** Confirmed that `Europe/London` applies UTC+1 offset in July via `time.Date(...).Zone()`

## Discovered Root Cause

The `abbrevMap` in `parseTimezoneAbbrev` maps `"gmt"` to `"Europe/London"`.

**Defect type:** Incorrect mapping / logic error

**Why it occurred:** GMT and BST share the same geographic zone (UK), so `Europe/London` was used for both. However, GMT is defined as a fixed UTC+0 offset, while BST is the daylight-saving variant. The map conflated the two.

**Contributing factors:** The IANA database uses `Europe/London` for both GMT and BST, making it a natural but incorrect choice for a "fixed offset" abbreviation like GMT.

## Resolution for the Issue

**Changes made:**
- `internal/agents/claudecode/errors.go:167` — Changed `"gmt": "Europe/London"` to `"gmt": "UTC"`

**Approach rationale:** GMT is defined as UTC+0 with no DST. Mapping to `"UTC"` ensures a fixed zero offset year-round. The `"bst"` entry correctly stays as `"Europe/London"` since BST is the daylight-saving variant.

**Alternatives considered:**
- `Etc/GMT` — Equivalent to UTC but less readable; no benefit
- Fixed offset via `time.FixedZone("GMT", 0)` — Would require code restructuring for no gain

## Regression Test

**Test file:** `internal/agents/claudecode/errors_test.go`
**Test name:** `TestParseTimezoneAbbrev_GMTFixedOffset`

**What it verifies:** That `parseTimezoneAbbrev("GMT")` returns a location with zero UTC offset in both summer (July) and winter (January), proving DST is not applied.

**Run command:** `go test ./internal/agents/claudecode/ -run TestParseTimezoneAbbrev_GMTFixedOffset -v`

## Affected Files

| File | Change |
|------|--------|
| `internal/agents/claudecode/errors.go` | Map `"gmt"` to `"UTC"` instead of `"Europe/London"` |
| `internal/agents/claudecode/errors_test.go` | Add regression test for GMT fixed offset |

## Verification

- [x] Regression test passes
- [x] Full test suite passes
- [x] Linters/validators pass

## Prevention

**Recommendations to avoid similar bugs:**
- When mapping timezone abbreviations, distinguish between fixed-offset abbreviations (GMT, UTC) and location-based abbreviations (EST, BST) that naturally pair with a geographic IANA zone
- Add offset-verification tests for any timezone abbreviation that is expected to be fixed-offset

## Related

- Transit ticket: T-516
