# Smolspec: Shared Agent Execution Pattern

**Transit ticket**: T-97
**Date**: 2026-02-17

## Problem

All 5 agent `Run()` methods follow the same execution pattern (~300 lines duplicated):

1. Build CLI args (agent-specific flags + prompt)
2. Create `exec.CommandContext` with workdir, env, stdin=nil
3. Capture stdout + stderr into buffers
4. Time the execution
5. Extract exit code from `*exec.ExitError`
6. Populate `RunResult` with session ID, duration, output, exit code

The shared scaffolding (steps 2-6) is copy-pasted across all five agents. Agent-specific logic is concentrated in step 1 (arg building) and post-execution processing (cost extraction, output parsing).

## Agent-Specific Variations

| Agent | Output Format | Cost Source | Resume Style | Post-processing |
|-------|--------------|-------------|-------------|-----------------|
| Claude Code | JSON struct | JSON field | `--resume <id>` | Parse JSON, extract turns/errors |
| Codex | Raw text | None | Not supported | None |
| Kiro | Raw text | SQLite query | `--resume` flag | Query DB for credits |
| Copilot | Raw text | stdout parse | `--continue` flag | `ParseUsage()` on output |
| OpenCode | JSON bytes | None yet | `--session <id>` or `--continue` | Validate JSON, detect errors |

## Proposed Approach

### Option A: Shared executor function (recommended)

```go
// internal/agents/executor.go

type ExecuteConfig struct {
    CLIPath   string
    Args      []string
    WorkDir   string
    Env       map[string]string
    SessionID string
}

type ExecuteResult struct {
    Stdout   []byte
    Stderr   []byte
    ExitCode int
    Duration time.Duration
    Err      error
}

func Execute(ctx context.Context, cfg ExecuteConfig) *ExecuteResult {
    // Steps 2-5: create command, capture output, time, extract exit code
}
```

Each agent calls `Execute()` and then does its own post-processing to build `RunResult`. This extracts the mechanical part without constraining agent-specific behavior.

### Option B: Template method via embedding

```go
type BaseAgent struct {
    config AgentConfig
}

func (b *BaseAgent) execute(ctx context.Context, args []string, opts RunOptions) *ExecuteResult { ... }
```

Agents embed `BaseAgent` and call `b.execute()`. More coupled than Option A.

### Recommendation

**Option A** (shared function) is cleaner. It doesn't impose inheritance, keeps agents independent, and the function is trivially testable. Each agent's `Run()` becomes: build args → `Execute()` → post-process result.

## Scope

### In scope
- Extract shared command execution into `internal/agents/executor.go`
- Refactor all 5 agents to use the shared executor
- Standardize stdin=nil, env merging, workdir handling
- Standardize exit code extraction

### Out of scope
- Changing how agents build CLI args (agent-specific, stay in each package)
- Changing output parsing or cost extraction (agent-specific)
- Changing error classification (T-91/T-95 scope)
- Changing retry logic (T-94 scope)

## Dependencies

- Independent of T-94 (retry executor)
- Independent of T-91/T-95 (error classifiers)
- Can be done in any order relative to other chores

## Risks

- Low risk — the extraction is mechanical and each agent's test suite validates behavior
- Need to preserve the exact `exec.CommandContext` behavior (env merging, stdin handling)
- Claude Code's JSON parsing and Kiro's post-execution SQLite query remain in their respective packages
