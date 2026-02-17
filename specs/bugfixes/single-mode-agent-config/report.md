# Bugfix Report: Single Mode Doesn't Use Configured Agent Aliases

**Date:** 2026-02-17
**Status:** Fixed

## Description of the Issue

When running `orbit run` in single (non-variant) mode with an agent alias (e.g., `orbit run -agent copilot-sonnet`), Orbit fails with "unknown agent" because it tries to look up the alias name directly in the agent registry instead of resolving the alias to its underlying agent type first.

**Reproduction steps:**
1. Configure `.orbit.yaml` with agent aliases (e.g., `copilot-sonnet` with `type: copilot`)
2. Run `orbit run -agent copilot-sonnet`
3. Error: `failed to get agent "copilot-sonnet": unknown agent: copilot-sonnet (available: [claude-code codex copilot kiro opencode])`

**Impact:** All users running single-mode with configured agent aliases are affected. The only workaround is using the base agent type directly, which loses all alias-specific configuration (model, extra-args, CLI path, etc.). Multi-variant mode is not affected because it has its own agent resolution logic that correctly resolves aliases to types.

## Investigation Summary

Traced the agent resolution path for both single and variant modes:

- **Variant mode (correct):** `runVariant()` in `variants.go` looks up `variantAgentConfig.Type` and falls back to the alias name only if Type is empty. It passes the resolved type to `AgentResolver.GetAgent()`.
- **Single mode (broken):** `orbit.New()` in `orbit.go` uses `config.Agent` (the alias name) directly in `AgentResolver.GetAgent()`, which fails because the agent registry only contains base types.

The same bug also existed in `comparison.go` for the auto-consolidation agent creation path.

## Discovered Root Cause

In `orbit.New()` (orbit.go line 227), `config.AgentResolver.GetAgent(agentName, agentConfig)` was called with `agentName` set to `config.Agent`, which holds the alias name (e.g., "copilot-sonnet"). The agent registry only registers base types ("claude-code", "codex", "copilot", "kiro", "opencode"), so any alias that differs from the base type fails lookup.

The `run.go` command does resolve the alias correctly -- it calls `cfg.GetResolvedAgent()` to get the type and creates the agent using `agents.Get(resolved.Type, agentCfg)`. However, it stores the alias name (not the type) in `orbit.Config.Agent`, and the agent instance created in `run.go` is never passed to `orbit.New()`. Instead, `orbit.New()` independently creates a new agent using the alias name, which fails.

**Defect type:** Missing type resolution -- the alias-to-type mapping was performed in `run.go` but not propagated to `orbit.New()`.

**Why it occurred:** The `AgentConfig.Type` field was already set correctly by `run.go` via `buildAgentConfig()`, but `orbit.New()` never consulted it. The variant mode code was written later with the correct pattern, but the single-mode code in `orbit.New()` predates the alias system and was never updated.

## Resolution for the Issue

**Changes made:**
- `internal/orbit/orbit.go` -- `New()`: Added type resolution from `agentConfig.Type` before calling `AgentResolver.GetAgent()`, falling back to `agentName` for backward compatibility when Type is empty. Also updated `GetClassifier()` to use the resolved type.
- `internal/orbit/comparison.go` -- `runAutoConsolidate()`: Same type resolution pattern for the consolidation agent creation.

**Approach rationale:** This mirrors the pattern already used in variant mode (`variants.go` lines 553-558), keeping the codebase consistent. The fallback to `agentName` when Type is empty preserves backward compatibility for cases where the alias name equals the base type (e.g., `claude-code` without an explicit Type field).

**Alternatives considered:**
- Passing the pre-created agent from `run.go` through `orbit.Config` -- Rejected because it would require a larger interface change and `orbit.New()` also needs the agent for other purposes (error classifier, log manager setup). The type resolution approach is simpler and matches the variant mode pattern.
- Changing `run.go` to store the resolved type in `Config.Agent` instead of the alias -- Rejected because `Config.Agent` is used for display/logging purposes where the alias name is more informative than the base type.

## Regression Test

**Test file:** `internal/orbit/integration_test.go`

**Tests added:**
1. `TestNew_AgentAliasResolvesType` -- Creates an orbit config with `Agent="copilot-sonnet"` (alias) and `AgentConfig.Type="copilot"` (base type). Verifies that `orbit.New()` successfully creates the orbit instance by resolving the type. Before the fix, this test fails with "test agent copilot-sonnet not registered".
2. `TestNew_AgentAliasWithoutType_FallsBackToName` -- Verifies backward compatibility: when `AgentConfig.Type` is empty, `orbit.New()` falls back to using `Config.Agent` as the type. This ensures existing configurations where the alias name equals the base type continue to work.

**Verification:**
- Both regression tests pass
- Full test suite passes (`make test`)
- Linter passes (`make lint`)
- Build succeeds (`make build`)
