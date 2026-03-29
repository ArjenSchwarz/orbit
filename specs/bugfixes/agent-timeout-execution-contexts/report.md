# Bugfix Report: agent-timeout-execution-contexts

**Date:** 2026-03-29
**Status:** Fixed

## Description of the Issue

The `agents.AgentConfig.Timeout` configuration from `.orbit.yaml` is parsed correctly but never applied to the execution contexts used during agent runs. This means configured per-agent timeouts are silently ignored, allowing phases to hang indefinitely.

**Reproduction steps:**
1. Configure agent timeout in `.orbit.yaml`: `agents: { claude-code: { timeout: "30m" } }`
2. Run `orbit run` with a phase that hangs or runs very long
3. Observe the agent runs indefinitely despite the configured timeout

**Impact:** Medium — configured timeouts are silently ignored, preventing automated recovery from hanging agent processes.

## Investigation Summary

Systematic code inspection of the timeout data flow from config parsing to execution.

- **Symptoms examined:** `AgentConfig.Timeout` parsed correctly, `RunOptions.Timeout` field exists, but timeout never applied
- **Code inspected:** `config.go`, `agent.go`, `registry.go`, `executor.go`, `single.go`, `variants.go`, all 5 agent implementations
- **Hypotheses tested:** Confirmed that timeout is never populated in `RunOptions` and never applied in `Execute()`

## Discovered Root Cause

Three-level disconnect in the timeout pipeline:

1. **`RunOptions.Timeout` never populated**: `single.go` and `variants.go` construct `RunOptions{}` without setting the `Timeout` field from `AgentConfig.Timeout`
2. **`ExecuteConfig` lacks `Timeout` field**: The shared `Execute()` function has no way to receive a timeout value
3. **Agents don't forward timeout**: All 5 agent implementations pass `opts.WorkDir` and `opts.Env` to `ExecuteConfig` but never pass timeout information

**Defect type:** Missing feature wiring — all components exist but are never connected

**Why it occurred:** The `Timeout` field was added to both `AgentConfig` and `RunOptions` structs, and config parsing was implemented, but the runtime wiring was never completed.

**Contributing factors:** No integration test verified end-to-end timeout enforcement.

## Resolution for the Issue

**Changes made:**
- `internal/agents/executor.go` — Add `Timeout` field to `ExecuteConfig`; apply via `context.WithTimeout` in `Execute()`
- `internal/agents/claudecode/agent.go` — Forward `opts.Timeout` to `ExecuteConfig.Timeout`
- `internal/agents/codex/agent.go` — Forward `opts.Timeout` to `ExecuteConfig.Timeout`
- `internal/agents/copilot/agent.go` — Forward `opts.Timeout` to `ExecuteConfig.Timeout`
- `internal/agents/kiro/agent.go` — Forward `opts.Timeout` to `ExecuteConfig.Timeout`
- `internal/agents/opencode/agent.go` — Forward `opts.Timeout` to `ExecuteConfig.Timeout`
- `internal/orbit/single.go` — Populate `Timeout` from `o.config.AgentConfig.Timeout` in all 3 RunOptions sites
- `internal/orbit/variants.go` — Thread timeout through variant orchestration functions

**Approach rationale:** Applied timeout in `Execute()` (centralized) rather than in each agent's execute method, so all agents benefit automatically. Each agent forwards the timeout through `ExecuteConfig`.

**Alternatives considered:**
- Apply timeout in `RunWithRetry` — rejected because timeout should be per-execution, not per-retry-loop
- Apply timeout only in agent methods — rejected because it duplicates logic across 5 agents

## Regression Test

**Test file:** `internal/agents/executor_test.go`
**Test names:** `TestExecute_TimeoutKillsLongRunningProcess`, `TestExecute_ZeroTimeoutDoesNotLimit`, `TestExecute_TimeoutShorterThanParentContext`

**Test file:** `internal/orbit/orbit_test.go`
**Test names:** `TestRunPhase_PopulatesTimeoutFromAgentConfig`, `TestRunPrePrompt_PopulatesTimeoutFromAgentConfig`, `TestRunPostPrompt_PopulatesTimeoutFromAgentConfig`

**What it verifies:** Timeout is applied to execution contexts (executor tests) and populated from AgentConfig (orchestration tests)

**Run command:** `make test`

## Affected Files

| File | Change |
|------|--------|
| `internal/agents/executor.go` | Add `Timeout` to `ExecuteConfig`; apply in `Execute()` |
| `internal/agents/claudecode/agent.go` | Forward `opts.Timeout` |
| `internal/agents/codex/agent.go` | Forward `opts.Timeout` |
| `internal/agents/copilot/agent.go` | Forward `opts.Timeout` |
| `internal/agents/kiro/agent.go` | Forward `opts.Timeout` |
| `internal/agents/opencode/agent.go` | Forward `opts.Timeout` |
| `internal/orbit/single.go` | Populate `RunOptions.Timeout` from `AgentConfig` |
| `internal/orbit/variants.go` | Thread timeout to variant RunOptions |

## Verification

**Automated:**
- [x] Regression test passes
- [x] Full test suite passes
- [x] Linters/validators pass

## Prevention

**Recommendations to avoid similar bugs:**
- Add integration tests when adding config fields that affect runtime behavior
- Consider a linting rule or code review checklist for "unused struct fields"

## Related

- Transit ticket: T-584
