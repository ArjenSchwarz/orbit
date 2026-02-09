# Comparison Execution Notes

## Architecture

Comparison runs through two code paths:
1. `cmd/orbit/compare.go` — standalone `orbit compare` subcommand
2. `internal/orbit/orbit.go:runComparison()` — called during `orbit run --variants`

Both create a `comparison.AgentAdapter` wrapping the Claude Code agent, then call `comparator.CompareUnified()` or `comparator.CompareWithSummaries()`.

## Key Design Decisions

### Tools disabled during comparison (2026-02-09)

Comparison prompts include all data inline (diffs, stats, changelogs, spec context). Claude should only analyze this data and produce JSON output. Tools are disabled via `--tools ""` to prevent:
- Subagent spawning (expensive, slow)
- File reading (unnecessary, data is inline)
- Command execution (running tests/lint is not the comparison's job)

### Timeout on comparison context (2026-02-09)

A 10-minute timeout (`comparison.DefaultTimeout`) is applied to prevent indefinite hangs from API stalls. The timeout is applied via `context.WithTimeout()` on the context passed to `NewAgentAdapter`.

### Session ID handling (2026-02-09)

The adapter doesn't set a `SessionID` in `RunOptions`. The Claude agent now correctly omits `--session-id` when the value is empty, letting Claude generate its own.

## Gotchas

- The `RunOptions.Timeout` field exists but is unused by all agents. Use context timeouts instead.
- `exec.CommandContext` kills the process when context is cancelled, but doesn't clean up gracefully. For Claude CLI this is acceptable since `-p` mode sessions can be safely interrupted.
