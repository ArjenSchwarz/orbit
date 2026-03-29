# Bugfix Report: Honor explicit --max-parallel=3 CLI override

**Date:** 2026-03-29
**Status:** Fixed
**Ticket:** T-585

## Description of the Issue

The `--max-parallel` CLI flag was not honored when the user explicitly set it to `3` (the built-in default). The config file value would take precedence, making it impossible to override a non-default config value (e.g., `max-parallel: 8`) back to 3 via the CLI.

**Reproduction steps:**
1. Set `max-parallel: 8` in `.orbit.yaml`
2. Run `orbit run --variants 4 --parallel --max-parallel 3`
3. Observe that max-parallel resolves to 8 (from config) instead of 3 (from CLI)

**Impact:** Users could not override the config's max-parallel value when the desired value happened to match the flag's built-in default of 3.

## Investigation Summary

- **Symptoms examined:** CLI flag `--max-parallel=3` ignored when config has a different value
- **Code inspected:** `cmd/orbit/run.go` lines 207-217 — max-parallel resolution logic
- **Hypotheses tested:** The resolution compared the flag value against the built-in default (3) rather than detecting whether the flag was explicitly set

## Discovered Root Cause

The resolution logic at `cmd/orbit/run.go:211` used `if *maxParallel != 3` to decide whether to override the config value. This meant that when the user explicitly passed `--max-parallel=3`, the condition was false and the config value was kept.

**Defect type:** Logic error — incorrect flag-set detection

**Why it occurred:** The original comment acknowledged the limitation: "Since we can't detect if the flag was explicitly set, we only override if the CLI value differs from the built-in default of 3." However, Go's `flag.FlagSet.Visit` method does support explicit-set detection.

**Contributing factors:** The pattern of comparing against defaults is a common Go anti-pattern when the default value is a valid user input.

## Resolution for the Issue

**Changes made:**
- `cmd/orbit/run.go:207-217` — Replaced value-comparison with `fs.Visit` to detect explicit flag setting

**Approach rationale:** `fs.Visit` iterates only over flags that were explicitly set on the command line, regardless of their value. This is the idiomatic Go approach for distinguishing "user set the default" from "user didn't set the flag."

**Alternatives considered:**
- Using a sentinel value (e.g., -1) as the default — rejected because it changes the help text and requires extra validation

## Regression Test

**Test file:** `cmd/orbit/run_test.go`
**Test names:** `TestMaxParallel_FlagResolution`, `TestMaxParallel_FlagVisitDetection`

**What they verify:**
- `TestMaxParallel_FlagResolution`: Resolution logic correctly applies explicit flag vs config precedence, including the T-585 regression case where flag=3 must override config=8
- `TestMaxParallel_FlagVisitDetection`: `fs.Visit` correctly detects explicit `--max-parallel` flags including when the value equals the default

**Run command:** `go test ./cmd/orbit/ -run TestMaxParallel -v`

## Affected Files

| File | Change |
|------|--------|
| `cmd/orbit/run.go` | Use `fs.Visit` instead of value comparison for max-parallel resolution |
| `cmd/orbit/run_test.go` | Add regression tests for max-parallel flag resolution |

## Verification

**Automated:**
- [x] Regression test passes
- [x] Full test suite passes

## Prevention

**Recommendations to avoid similar bugs:**
- Use `fs.Visit` (or equivalent) to detect explicit flag setting whenever a flag's default is a valid user input
- Avoid comparing flag values against defaults to determine if a flag was set

## Related

- T-585: Honor explicit --max-parallel=3 CLI override
