# Bugfix Report: Status Timezone

**Date:** 2026-02-15
**Status:** Fixed
**Ticket:** T-64

## Description of the Issue

The `orbit status` command displays the "Started" timestamp in UTC instead of the user's local timezone. For example, a run started at 5:52 PM local time (AEDT) would display as `2026-02-14 06:52:03` instead of `2026-02-14 17:52:03`.

**Reproduction steps:**
1. Run `orbit run --variants N` (which stores `StartedAt` as UTC)
2. Run `orbit status <spec>`
3. Observe the "Started" field shows UTC time, not local time

**Impact:** Low severity, cosmetic. Users see incorrect timestamps that don't match their local time, causing confusion about when runs started.

## Investigation Summary

- **Symptoms examined:** Timestamp in status output didn't match local wall clock time
- **Code inspected:** `cmd/orbit/status.go`, `internal/variants/manager.go`, `internal/variants/types.go`, `internal/status/types.go`
- **Hypotheses tested:** The timestamp is stored as UTC in `variants.json` and formatted without timezone conversion

## Discovered Root Cause

`variants/manager.go:237` stores `StartedAt` as `time.Now().UTC()`. When `status.go:89` formats it with `.Format("2006-01-02 15:04:05")`, Go preserves the UTC timezone of the `time.Time` value, displaying the time in UTC rather than converting to local.

**Defect type:** Missing timezone conversion

**Why it occurred:** Storing times in UTC is correct practice for persistence, but the display layer needs to convert back to local time before formatting. The `.Local()` call was missing.

**Contributing factors:** The existing codebase had a correct pattern in `apsis/main.go` (`.Local().Format(...)`) but it wasn't applied here.

## Resolution for the Issue

**Changes made:**
- `cmd/orbit/status.go:89` - Added `.Local()` before `.Format()` to convert UTC to local timezone

**Approach rationale:** Adding `.Local()` is the standard Go idiom for converting a UTC time to the user's local timezone before display. The `apsis` code already uses this pattern.

**Alternatives considered:**
- Store `StartedAt` in local time instead of UTC - Not chosen because UTC is the correct convention for persisted timestamps; the conversion belongs in the display layer

## Regression Test

**Test file:** `cmd/orbit/status_test.go`
**Test name:** `TestStatusCommand_TimestampLocalTimezone`

**What it verifies:** Creates a `variants.json` with a known UTC `StartedAt` time, runs `orbit status --format json`, and verifies the output timestamp matches the local timezone representation.

**Run command:** `go test ./cmd/orbit/ -run TestStatusCommand_TimestampLocalTimezone -v`

## Affected Files

| File | Change |
|------|--------|
| `cmd/orbit/status.go` | Added `.Local()` to timestamp formatting |
| `cmd/orbit/status_test.go` | Added regression test for local timezone display |

## Verification

**Automated:**
- [x] Regression test passes
- [x] Full test suite passes
- [x] Linters/validators pass

## Prevention

**Recommendations to avoid similar bugs:**
- When displaying persisted UTC timestamps, always use `.Local()` before `.Format()`
- Follow the existing pattern in `apsis/main.go` for timestamp display

## Related

- Transit ticket: T-64
