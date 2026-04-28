# Comparison Execution Notes

## Architecture

Comparison runs through two code paths:
1. `cmd/orbit/compare.go` — standalone `orbit compare` subcommand
2. `internal/orbit/orbit.go:runComparison()` — called during `orbit run --variants`

Both create a `comparison.AgentAdapter` wrapping the Claude Code agent, then call `comparator.CompareUnified()`.

## Key Design Decisions

### Tools disabled during comparison (2026-02-09)

Comparison prompts include all data inline (diffs, stats, changelogs, spec context). Claude should only analyze this data and produce JSON output. Tools are disabled via `--tools ""` to prevent:
- Subagent spawning (expensive, slow)
- File reading (unnecessary, data is inline)
- Command execution (running tests/lint is not the comparison's job)

### Timeout on comparison context (2026-02-10, updated 2026-04-28 for T-678)

When `AgentConfig.Timeout` is set (via `.orbit.yaml` `agents.<agent>.timeout`), it is honored for the comparison run too — both via the wrapping `context.WithTimeout()` and via `RunOptions.Timeout` (set on the adapter through `AgentAdapter.WithTimeout`). When no timeout is configured, the hardcoded 30-minute `comparison.DefaultTimeout` is still applied as a safety net to prevent indefinite hangs from API stalls. The adapter no longer stores context — it receives it as a method parameter, following Go best practices. Both call sites (`internal/orbit/comparison.go` and `cmd/orbit/compare.go`) read the timeout the same way so behaviour stays consistent.

### Agent writes comparison JSON to file (2026-02-10)

The comparison prompt instructs the agent to write the JSON result to `specs/<name>/.orbit/comparison.json` before outputting it. This ensures the result is persisted even if the agent session hangs after producing output. The file path is passed via `ComparisonInput.OutputPath`.

### --from-file flag for orbit compare (2026-02-10)

`orbit compare <spec> --from-file <path>` loads a pre-existing comparison JSON file and generates the report without invoking the agent. Useful when:
- The agent wrote `comparison.json` but the session hung before orbit could parse stdout
- You want to re-generate the report from a saved result
- Testing report generation with known data

Uses `comparison.LoadResultFromFile()` which handles raw JSON and markdown-wrapped JSON.

### Auto-fallback to comparison file on failure (2026-02-15)

`CompareUnified()` automatically checks whether the agent wrote `comparison.json` to `OutputPath` when the comparison fails (e.g., timeout, garbled response, JSON validation failure after retries). If the file was created or updated during the comparison run, it is loaded via `LoadResultFromFile()` and used as the result instead of propagating the error.

This handles the common scenario where the agent successfully writes the file to disk but the session then hangs or the streamed response is malformed. The mod-time of the file is recorded before `runComparison()` starts; on failure, the file is only accepted if its mod-time is newer (preventing stale pre-existing files from being used). Both code paths (`orbit run --variants` and `orbit compare`) benefit automatically since they both call `CompareUnified()`.

### Session ID handling (2026-02-09)

The adapter doesn't set a `SessionID` in `RunOptions`. The Claude agent now correctly omits `--session-id` when the value is empty, letting Claude generate its own.

## Gotchas

- The `RunOptions.Timeout` field exists but is unused by all agents. Use context timeouts instead.
- `exec.CommandContext` kills the process when context is cancelled, but doesn't clean up gracefully. For Claude CLI this is acceptable since `-p` mode sessions can be safely interrupted.
- All variant branches must share the same base commit as `metadata.BaseCommit`. `Manager.Setup()` enforces this (T-191): when preserving completed variants on a new run, it errors if HEAD != BaseCommit. Without this check, comparison diffs use the wrong base for newly-created variants.
