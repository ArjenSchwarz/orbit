# Agent Execution Pattern

## Shared Executor (`internal/agents/executor.go`)

All 5 agents delegate CLI command execution to `agents.Execute()`. This function handles the mechanical parts that were previously duplicated across all agents:

1. Create `exec.CommandContext` with the CLI path and args
2. Set working directory (if provided)
3. Merge environment variables with `os.Environ()` (only when env map is non-empty)
4. Capture stdout/stderr into buffers, set stdin=nil
5. Time the execution (wall clock)
6. Extract exit code from `*exec.ExitError` (or -1 for non-exit errors like command-not-found)

## Agent Responsibilities

Each agent still owns:
- **Arg building** (`buildArgs()`) — agent-specific CLI flags, prompt placement, resume handling
- **Post-processing** — parsing output, extracting costs, validating JSON

## Per-Agent Post-Processing

| Agent | Post-processing |
|-------|----------------|
| Claude Code | Parse JSON output for session ID, cost, turns, errors; override duration from API response |
| Codex | None (raw text output) |
| Kiro | Query SQLite for session credits via `extractSessionCredits()` |
| Copilot | `ParseUsage()` on combined stdout/stderr for premium requests, tokens, durations |
| OpenCode | Validate JSON output, detect errors from empty/invalid JSON |

## Kiro WorkDir Resolution

Kiro resolves the working directory independently of the executor for its post-processing. When `opts.WorkDir` is empty, it calls `os.Getwd()` to determine the directory for session credit lookup. The executor receives `opts.WorkDir` as-is (empty means inherit).

## Environment Handling

When `cfg.Env` is nil or empty, the command inherits the default environment (Go's `exec.Cmd` default behavior). When env vars are provided, the current environment is explicitly copied via `os.Environ()` and the additional vars are appended. This means provided vars override existing ones with the same name (last value wins in the env slice).
