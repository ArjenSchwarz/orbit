# Bugfix Report: Comparison Session Hangs Indefinitely

**Date:** 2026-02-09
**Status:** Fixed

## Description of the Issue

When running `orbit compare` (or the comparison phase of `orbit run --variants`), the Claude CLI process hangs indefinitely after completing the comparison analysis. The session logs show the comparison JSON output has been produced, but the CLI never exits, blocking orbit from proceeding to report generation.

**Reproduction steps:**
1. Run `orbit run --variants 3` with a spec that has variant worktrees
2. Wait for all variants to complete
3. Observe comparison starts (`Running comparison of successful variants...`)
4. The comparison JSON is produced in the session log (visible via `apsis`)
5. The Claude CLI process never exits; orbit hangs

**Impact:** High severity. Comparison is the final step of variant runs. A hang here blocks report generation and auto-consolidation, wasting all the work done by the variants.

## Investigation Summary

Examined a live instance of the bug: `orbit compare` for the `apsis-serve` spec with 3 completed variants.

- **Symptoms examined:** Claude CLI process (PID 43926) running for 38+ minutes. Session JSONL file (`80ab497b`) stopped being written to at 15:45. Last assistant message has `stop_reason=None` (streaming incomplete).
- **Code inspected:** `internal/comparison/adapter.go`, `internal/comparison/compare.go`, `internal/agents/claudecode/agent.go`, `cmd/orbit/compare.go`, `internal/orbit/orbit.go`
- **Hypotheses tested:**
  - Empty `--session-id` causing CLI issues: Contributing factor but not root cause
  - Claude CLI in interactive mode: Ruled out (`-p` flag is correctly parsed)
  - API streaming hang: Confirmed. The API response never completed (`stop_reason=None`)

## Discovered Root Cause

Three defects contributed to this bug:

**Defect 1: No timeout on comparison context**

`cmd/orbit/compare.go` uses `context.Background()` with no timeout when invoking the Claude CLI for comparison. If the API connection stalls (as observed), the `cmd.Run()` call blocks indefinitely.

**Defect type:** Missing timeout / resource leak

**Defect 2: Unrestricted tool access during comparison**

The comparison prompt is invoked with `--dangerously-skip-permissions` but no `--tools` restriction. Claude uses tools extensively during comparison — spawning subagents, running `make test`, `make lint`, reading files across all variant worktrees — even though all necessary data is included inline in the prompt. This makes comparison sessions:
- Much longer (5+ minutes of tool use before producing output)
- Much more expensive (observed: 96K input tokens per API call)
- Prone to API stalls due to extended session duration

**Defect type:** Missing constraint / over-permissive execution

**Defect 3: Empty `--session-id` passed to Claude CLI**

`AgentAdapter.RunCustomPrompt()` creates `RunOptions` without a `SessionID`, causing `buildArgs()` to pass `--session-id ""` to the CLI. While this doesn't directly cause the hang (Claude generates its own ID), it's incorrect usage.

**Defect type:** Missing input validation

**Why it occurred:** Comparison was designed to use the same agent execution path as phase runs, but phase runs have external timeout/retry mechanisms. Comparison was added later without equivalent safeguards.

**Contributing factors:** The API streaming connection stalled after Claude produced 22K chars of JSON output but before sending the `end_turn` stop reason. This is likely a transient API issue, but the lack of timeout made it fatal.

## Resolution for the Issue

**Changes made:**
- `internal/comparison/compare.go:40` - Added `DefaultTimeout` constant (10 minutes)
- `internal/comparison/adapter.go:43-48` - Added `WithExtraArgs()` method to pass CLI flags
- `cmd/orbit/compare.go:124-133` - Applied 10-minute timeout context and `--tools ""` for `orbit compare`
- `internal/orbit/orbit.go:2381-2388` - Applied same timeout and tool restriction for `orbit run` comparison
- `internal/agents/claudecode/agent.go:156-160` - Skip `--session-id` flag when ID is empty

**Approach rationale:** The timeout ensures comparison cannot hang forever. Disabling tools via `--tools ""` ensures Claude only analyzes the inline data without reading files or running commands, making comparison faster, cheaper, and deterministic.

**Alternatives considered:**
- Adding `--max-budget-usd` instead of timeout: Doesn't protect against API hangs
- Using `RunOptions.Timeout` field: Exists but unused by any agent; context timeout is the standard Go mechanism

## Regression Test

**Test file:** `internal/comparison/adapter_test.go`
**Test name:** `TestAgentAdapter_WithExtraArgs`, `TestAgentAdapter_WithExtraArgs_DoesNotMutateOriginal`

**What it verifies:** `WithExtraArgs()` correctly passes extra CLI arguments to the agent, and the original adapter is not mutated.

**Test file:** `internal/agents/claudecode/agent_test.go`
**Test name:** `TestAgent_BuildArgs_EmptySessionID`

**What it verifies:** When `SessionID` is empty, the `--session-id` flag is omitted entirely.

**Run command:** `go test ./internal/comparison/ ./internal/agents/claudecode/ -v`

## Affected Files

| File | Change |
|------|--------|
| `internal/comparison/compare.go` | Added `DefaultTimeout` constant (10 min) |
| `internal/comparison/adapter.go` | Added `WithExtraArgs()` method and `extraArgs` field |
| `internal/comparison/adapter_test.go` | Added regression tests for `WithExtraArgs` |
| `cmd/orbit/compare.go` | Applied timeout context and tool restriction |
| `internal/orbit/orbit.go` | Applied timeout context and tool restriction |
| `internal/agents/claudecode/agent.go` | Skip `--session-id` when empty |
| `internal/agents/claudecode/agent_test.go` | Added test for empty session ID |

## Verification

**Automated:**
- [x] Regression test passes
- [x] Full test suite passes
- [x] Linters/validators pass

**Manual verification:**
- Confirmed via `ps` that the hanging Claude process (PID 43926) has `--session-id ""` in its args
- Confirmed the session JSONL shows `stop_reason=None` on the last message
- Confirmed no writes to the session file for 35+ minutes

## Prevention

**Recommendations to avoid similar bugs:**
- All external process invocations should have a timeout context. Never use `context.Background()` without wrapping it in `context.WithTimeout()`.
- When invoking Claude for structured output (JSON), disable tools via `--tools ""` to prevent unbounded execution.
- Consider adding a `--json-schema` flag to comparison prompts for structured output validation by Claude CLI itself.
