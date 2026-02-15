# Bugfix Report: ORBIT_CENTRALIZED_LOG Empty String Enables Logging

**Date:** 2026-02-16
**Status:** Fixed

## Description of the Issue

Setting `ORBIT_CENTRALIZED_LOG=""` (empty string) incorrectly enabled centralized logging instead of disabling it. The project convention documented in CLAUDE.md states: "Empty string environment variables explicitly disable features." All other boolean environment variables follow this convention, but `ORBIT_CENTRALIZED_LOG` used a different parsing pattern.

**Reproduction steps:**
1. Set `ORBIT_CENTRALIZED_LOG=""` in the environment
2. Run orbit
3. Observe that centralized logging is enabled despite the empty string

**Impact:** Low severity. Users attempting to disable centralized logging via empty string would find it still active. The `"false"` and `"0"` values worked correctly, so there was a workaround.

## Investigation Summary

Compared the env var parsing pattern for `ORBIT_CENTRALIZED_LOG` against all other boolean env vars in `config.go`.

- **Symptoms examined:** Empty string value for `ORBIT_CENTRALIZED_LOG` resulted in `CentralizedLog=true`
- **Code inspected:** `internal/config/config.go` lines 323-327, plus all other boolean env var handlers
- **Hypotheses tested:** Confirmed the negated check pattern `!= "false" && != "0"` was the sole cause

## Discovered Root Cause

The `ORBIT_CENTRALIZED_LOG` env var used a negated boolean check (`!= "false" && != "0"`) while all other boolean env vars used a positive check (`== "true" || == "1"`).

**Defect type:** Logic error -- inconsistent boolean parsing pattern

**Why it occurred:** The centralized-log default is `true`, so the original implementation was written to "disable only on explicit false/0". This inverted the check compared to other boolean vars, creating an inconsistency where empty string evaluates to `true` instead of `false`.

**Contributing factors:** The existing test `TestLoad_CentralizedLog_EnvEmptyEnables` explicitly asserted the wrong behaviour, encoding the bug as intended.

## Resolution for the Issue

**Changes made:**
- `internal/config/config.go:324-325` - Changed from `envCentralizedLog != "false" && envCentralizedLog != "0"` to `envCentralizedLog == "true" || envCentralizedLog == "1"`
- `internal/config/config_test.go:1782-1794` - Renamed test from `TestLoad_CentralizedLog_EnvEmptyEnables` to `TestLoad_CentralizedLog_EnvEmptyDisables` and inverted the assertion

**Approach rationale:** Using the same positive-check pattern as all other boolean env vars ensures consistent behaviour across the entire configuration system.

**Alternatives considered:**
- Adding `""` as an additional disable value to the negated check - Rejected because it would still use an inconsistent pattern and could miss other edge cases (e.g., whitespace-only strings)

## Regression Test

**Test file:** `internal/config/config_test.go`
**Test name:** `TestLoad_CentralizedLog_EnvEmptyDisables`

**What it verifies:** Setting `ORBIT_CENTRALIZED_LOG=""` results in `CentralizedLog=false`

**Run command:** `go test ./internal/config/ -run TestLoad_CentralizedLog_EnvEmptyDisables`

## Affected Files

| File | Change |
|------|--------|
| `internal/config/config.go` | Fixed boolean parsing to use positive check pattern |
| `internal/config/config_test.go` | Corrected test to assert empty string disables logging |

## Verification

**Automated:**
- [x] Regression test passes
- [x] Full test suite passes
- [x] Linters/validators pass

## Prevention

**Recommendations to avoid similar bugs:**
- Use the positive check pattern (`== "true" || == "1"`) for all boolean env vars, regardless of the default value
- When a boolean defaults to `true`, the env var should still require an explicit `"true"` or `"1"` to enable it -- `LookupEnv` already distinguishes "not set" (uses default) from "set to empty" (disables)

## Related

- Transit ticket: T-68
