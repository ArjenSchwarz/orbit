# Bugfix Report: Copilot Premium Requests Singular Form Not Parsed

**Date:** 2026-02-11
**Status:** Fixed

## Description of the Issue

Copilot variant runs in multi-variant mode showed 0 premium requests in comparison reports and summary.json, despite Copilot CLI printing usage data to stderr.

**Reproduction steps:**
1. Run `orbit run --variants 3 --variant-agents claude-code,codex,copilot` on any spec
2. Wait for all variants to complete
3. Check the Copilot variant's `summary.json` — `cost_usd: 0`, `cost_totals: {}`
4. Check the comparison report — Copilot variant shows no cost data

**Impact:** All Copilot runs with exactly 1 premium request (the most common case) had their costs silently dropped. Runs with fractional or >1 premium requests (plural form) were unaffected.

## Investigation Summary

- **Symptoms examined:** Copilot variant-3 in `specs/apsis-latest/.orbit/logs/variant-3/summary.json` shows `cost_usd: 0` and empty `cost_totals`, yet `phase-1-session.txt` stderr clearly contains `Total usage est:        1 Premium request`
- **Code inspected:** Full data flow: `copilot/usage.go` (ParseUsage) → `copilot/agent.go` (execute) → `orbit/orbit.go` (getCostValue, runVariant) → `variants/manager.go` (UpdateMetrics) → `report/markdown.go` (generateMarkdownReport)
- **Hypotheses tested:**
  - `getCostValue` not checking PremiumRequests — already fixed in commit 88d9e13
  - Cost not flowing from variant to report — report code is correct, reads from `v.Cost`/`v.CostTotals`
  - Regex not matching Copilot output — **confirmed as root cause**

## Discovered Root Cause

The `premiumRequestsRe` regex in `internal/agents/copilot/usage.go:32` required "premium requests" (plural) but Copilot CLI outputs "Premium request" (singular) when the count is exactly 1.

**Defect type:** Regex pattern mismatch

**Why it occurred:** All test cases in `usage_test.go` used the plural form "Premium requests", and the property-based test generates formatted strings with `%.2f Premium requests` — always plural. The singular form was never tested.

**Contributing factors:** The previous bugfix (commit 88d9e13) fixed the downstream `getCostValue` function but didn't investigate why the upstream `ParseUsage` was returning nil for real Copilot output. The session `.txt` files contained the evidence (`Cost: -` on line 8) but weren't checked during the previous fix.

## Resolution for the Issue

**Changes made:**
- `internal/agents/copilot/usage.go:32` — Changed `premium\s+requests` to `premium\s+requests?` to match both singular and plural forms

**Approach rationale:** Making the trailing 's' optional with `?` is the minimal fix that handles both "request" and "requests" without changing any other behaviour.

**Alternatives considered:**
- Matching "request" only (without optional 's') — would break for the plural form which is already tested and working

## Regression Test

**Test file:** `internal/agents/copilot/usage_test.go`
**Test name:** `TestParseUsage/singular_premium_request`

**What it verifies:** That `ParseUsage` correctly extracts premium requests from real Copilot CLI output containing "1 Premium request" (singular), along with other metrics from the same output block.

**Run command:** `go test ./internal/agents/copilot/ -run "TestParseUsage/singular_premium_request" -v`

## Affected Files

| File | Change |
|------|--------|
| `internal/agents/copilot/usage.go` | Make trailing 's' optional in premium requests regex |
| `internal/agents/copilot/usage_test.go` | Add regression test with real singular Copilot output |

## Verification

**Automated:**
- [x] Regression test passes
- [x] Full test suite passes
- [x] Linters/validators pass

**Manual verification:**
- Confirmed that `specs/apsis-latest/.orbit/logs/variant-3/phase-1-session.txt` and `post-completion-session.txt` both contain "1 Premium request" (singular) which was not being parsed

## Prevention

**Recommendations to avoid similar bugs:**
- Test with real CLI output samples, not just synthetic patterns — the existing tests all used "Premium requests" (plural) because they were written without observing actual single-request output
- Property-based tests should vary the number format to include exactly 1 (which triggers singular form) alongside fractional values
- When a "cost = 0" bug is reported, check the parsing layer first (does `ParseUsage` return data?) before investigating downstream consumers

## Related

- Commit 88d9e13: Previous fix attempt that addressed `getCostValue` not checking `PremiumRequests` — correct but didn't fix the upstream parsing issue
- PR #61: [bug]: Fix Copilot premium requests missing in multi-variant run metrics
