# Compare Improvements

Three changes to orbit's variant comparison system: increased timeout, agent-side JSON persistence, and offline report generation from saved files.

## Beginner Level

### What Changed

When orbit compares implementation variants (different AI agents solving the same task), it asks an AI agent to analyze the differences and produce a recommendation. Three things changed:

1. **Longer timeout** (10 min to 30 min): The comparison agent now gets 30 minutes instead of 10 to finish its analysis. The old limit was too short for larger comparisons.

2. **Agent saves its own results**: The comparison prompt now tells the agent to write its JSON analysis to a file (`specs/<name>/.orbit/comparison.json`) before finishing. This is a safety net - if the agent session hangs after producing output, the file is already on disk.

3. **Load results from a file**: A new `--from-file` flag on `orbit compare` lets you point at an existing JSON file and generate the comparison report from it, without running the agent at all.

### Why It Matters

Comparison runs are expensive (they use an AI agent) and can take significant time. If something goes wrong at the end of a comparison - the session hangs, the process gets killed, the API times out - all that work is lost. These changes add resilience: the agent writes its output early, and you can recover from it later.

### Key Concepts

- **Variant**: A separate implementation of the same feature, each potentially by a different AI agent. They live in git worktrees.
- **Comparison**: An AI-driven analysis that reads all variant diffs and recommends which one is best.
- **Agent adapter**: A wrapper that lets orbit call different AI agent CLIs (Claude, Codex, Copilot) through a common interface.

---

## Intermediate Level

### Changes Overview

| File | Change |
|------|--------|
| `internal/comparison/compare.go` | `DefaultTimeout`: 10min to 30min. New `LoadResultFromFile()` function. |
| `internal/comparison/prompt.go` | `ComparisonInput.OutputPath` field. `buildComparisonPrompt()` appends file-write instruction when set. |
| `cmd/orbit/compare.go` | `--from-file` flag. Branching logic: load from file or run agent. Config validation skipped for `--from-file`. |
| `internal/orbit/orbit.go` | Switched from `CompareWithSummaries()` (legacy) to `CompareUnified()` with `OutputPath`. |
| Test files | 8 new tests for `LoadResultFromFile`, 2 new tests for `OutputPath` prompt behavior. |

### Implementation Approach

**Agent-side persistence**: Rather than having orbit write the JSON after parsing agent output, the approach instructs the agent itself to write the file as part of its task. The prompt appends an `ADDITIONALLY` instruction telling the agent to write the JSON to a specific path before outputting it to stdout. This way, even if the agent's session hangs after producing the JSON (a known failure mode), the file is already written.

**File loading**: `LoadResultFromFile()` reuses the existing `extractJSON()` function, which handles both raw JSON and markdown-wrapped code blocks. This is intentional - AI agents sometimes wrap output in ```` ```json ```` blocks even when told not to. After extraction, basic validation checks that required fields (`recommendation`, `confidence`, `summary`) are present.

**Unified API migration**: `orbit.go` was still using the legacy `CompareWithSummaries()` method. This change migrates it to `CompareUnified()`, which accepts `ComparisonInput` and supports the new `OutputPath` field. Both code paths (standalone `orbit compare` and in-run comparison) now use the same method.

### Trade-offs

- **Agent writes the file, not orbit**: This means the agent needs file-write tool access during comparison. Currently tools aren't restricted during comparison, so this works. If tools were locked down in the future, this would need adjustment.
- **Prompt injection of file path**: The output path is embedded directly in the prompt string. This is a trusted internal value (constructed from the spec directory), not user input, so injection risk is negligible.
- **30-minute timeout**: This is generous. Most comparisons finish in 5-15 minutes. The timeout exists as a safety net for hung API connections, not as a resource limit.

---

## Expert Level

### Technical Deep Dive

**Timeout change**: The `DefaultTimeout` constant is used by both code paths via `context.WithTimeout()`. The context is passed to `NewAgentAdapter` at construction time, which means the timeout governs the entire agent session including retries. The 30-minute window accommodates:
- Slow API responses (Claude can take minutes for large prompts)
- JSON validation retry loop (up to 3 attempts with re-prompting)
- Rate limit pauses that may occur mid-session

**`LoadResultFromFile` resilience**: The function chains `extractJSON()` before `json.Unmarshal()`. This is necessary because `extractJSON()` handles brace-matching extraction from surrounding text, which `json.Unmarshal()` alone would reject. The validation is deliberately lighter than `parseAndValidate()` - it doesn't enforce `DisallowUnknownFields()` or validate learnings. This is intentional: when loading from a file, you want maximum tolerance since you're recovering from a failure scenario.

**Prompt instruction ordering**: The file-write instruction is appended after the `IMPORTANT: Output ONLY valid JSON` line. The instruction explicitly says "write the file BEFORE outputting the JSON to stdout" because the agent session's stdout is what orbit parses. If the agent wrote stdout first and then hung before writing the file, we'd be in the same failure mode as before.

### Architecture Impact

The `ComparisonInput.OutputPath` field is a prompt-level concern, not an orchestration concern. This keeps the agent adapter and comparator clean - they don't need to know about file I/O. The output path flows through `buildComparisonPrompt()` and becomes part of the natural language instruction to the agent.

The migration from `CompareWithSummaries()` to `CompareUnified()` in `orbit.go` unifies both code paths on the same API. `CompareWithSummaries` is now only called by legacy callers (if any). A future cleanup could remove it entirely.

### Potential Issues

- **Agent compliance**: The agent is asked (not forced) to write the file. If the agent ignores the instruction (tool unavailable, prompt truncated, model confusion), the file won't exist. The system doesn't verify the file was written - it's a best-effort safety net.
- **File path in worktree mode**: When running variants in worktrees, `o.config.SpecDir` points to the main repo's spec dir, not the worktree's. The comparison JSON will be written to the main repo path, which is correct (comparison runs from the main repo context, not from within a worktree).
- **`--from-file` still gathers variant data**: Even when loading from file, `compareCommand` still gathers variant diffs via `diffGatherer.GatherAll()`. This is needed for report generation (the report includes per-variant diffs and metrics). This means the variant worktrees must still exist when using `--from-file`.
