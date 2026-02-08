# Bugfix Report: Copilot Premium Requests Not Shown in Variant Runs

**Date:** 2026-02-08
**Status:** Fixed

## Description of the Issue

In multi-variant runs, Copilot premium requests were not displayed in the comparison report or variant metrics. The cost always showed as 0 or "-" for Copilot variants, even though Copilot correctly parsed and reported premium request usage.

**Reproduction steps:**
1. Run `orbit run --variants 2 --variant-agents copilot,claude-code` on a spec
2. After completion, run `orbit compare <spec>`
3. Observe that the Copilot variant shows no cost while Claude Code shows its cost

**Impact:** Medium - cost data was silently lost for Copilot variants, making it impossible to compare costs across agents in multi-variant runs.

## Investigation Summary

- **Symptoms examined:** Copilot variant cost always 0 in comparison reports; other agents display correctly
- **Code inspected:** Agent cost parsing (`copilot/agent.go`), cost accumulation in variant loop (`orbit.go`), variant metrics storage (`variants/manager.go`), report generation (`compare.go`)
- **Hypotheses tested:** Initially suspected missing code path between summary.json and variants.json. Actual root cause was earlier in the chain - the cost accumulation function itself.

## Discovered Root Cause

The `getCostUSD` function in `internal/orbit/orbit.go:75` was used to accumulate `totalCost` during variant phase execution. This function only checked `CostUSD` and `Credits` fields, completely ignoring `PremiumRequests`.

**Defect type:** Missing case in cost extraction logic

**Why it occurred:** The function was written before Copilot support was added and was named/designed around the assumption that all costs are either USD or credits. When Copilot support was added with `PremiumRequests`, this function was not updated.

**Contributing factors:** The function name `getCostUSD` implied it only handled USD, which may have discouraged updating it when adding non-USD cost types. The `formatCost` function right below it correctly handled all three cost types, showing the inconsistency.

## Resolution for the Issue

**Changes made:**
- `internal/orbit/orbit.go:73-84` - Renamed `getCostUSD` to `getCostValue` and added `PremiumRequests` as a fallback after `Credits`

**Approach rationale:** The fix adds `PremiumRequests` to the existing priority chain (CostUSD > Credits > PremiumRequests). This mirrors the pattern already used in `formatCost` and maintains backward compatibility. The rename from `getCostUSD` to `getCostValue` reflects that the function returns whichever cost unit is populated, not specifically USD.

**Alternatives considered:**
- Accumulate costs per-unit type separately (track USD, credits, premium requests independently) - Would be more precise but unnecessarily complex since agents only report one cost type at a time
- Read back from summary.json after variant completion - The summary.json accumulation is correct, but the simpler fix is to accumulate correctly in the first place

## Regression Test

**Test file:** `internal/orbit/orbit_test.go`
**Test name:** `TestGetCostValue_PremiumRequests`

**What it verifies:** That `getCostValue` correctly returns PremiumRequests when CostUSD and Credits are both zero (the Copilot case), and that the priority order (USD > Credits > PremiumRequests) is maintained.

**Run command:** `go test ./internal/orbit -run TestGetCostValue_PremiumRequests`

## Affected Files

| File | Change |
|------|--------|
| `internal/orbit/orbit.go` | Renamed `getCostUSD` to `getCostValue`, added `PremiumRequests` handling |
| `internal/orbit/orbit_test.go` | Added regression test `TestGetCostValue_PremiumRequests` |

## Verification

**Automated:**
- [x] Regression test passes
- [x] Full test suite passes
- [x] Linters/validators pass

## Prevention

**Recommendations to avoid similar bugs:**
- When adding a new cost unit type, grep for all cost extraction/accumulation functions and update them, not just the display/formatting functions
- Consider replacing the priority-based fallback with an explicit `CostUnit` field check, similar to how `formatCost` works with a switch on `CostUnit`
