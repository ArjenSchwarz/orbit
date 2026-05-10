# Finalize Show and Verify (T-1197)

## Overview

The `orbit finalize` command currently announces that it will rebase a variant's branch but tells the user nothing about which agent produced that variant or whether the variant matches the one consolidation was just run on. This change adds two display/verification steps to `cmd/orbit/finalize.go`: it shows the agent (and model, when known) for the variant being finalized, and — when a consolidation log exists — warns if the requested `--variant` does not match the most recent `chosen_variant_id` recorded in `consolidation-log.json`. The warning is informational; the user can still proceed via the existing `y/N` confirmation.

## Requirements

- The system MUST display the agent used for the target variant in the finalize preamble (the `This will:` block printed before the confirmation prompt).
- The system MUST include the variant's agent alias, the underlying agent type, and the model when those values are recorded on the variant; absent individual fields MUST be omitted from the rendered string so no empty parens, "model: ", or trailing punctuation appear.
- The system MUST render `Agent: unknown` when none of `Agent`, `AgentType`, or `Model` are populated on the variant.
- The system MUST read `specs/<spec>/.orbit/consolidation-log.json` (when it exists) and compare the most recent entry's `chosen_variant_id` against the `--variant` flag value.
- The system MUST print a warning before the confirmation prompt when the most recent consolidation entry's `chosen_variant_id` differs from `--variant`. The warning MUST name both variant IDs and the consolidation timestamp formatted as RFC3339 (the format already used in the JSON log).
- The system MUST print the mismatch warning regardless of whether `--force` is set, so the warning is visible in logs/CI output even when the confirmation prompt is skipped.
- The system MUST allow finalize to proceed after the warning via the existing `y/N` prompt (or `--force`); no new flag is introduced.
- The system MUST treat a missing or unreadable consolidation log as "no verification possible" and continue silently without warning.
- The system MUST NOT change the existing finalize success/failure behaviour, exit codes, or rebase logic.

## Implementation Approach

**Key files to modify:**

1. `cmd/orbit/finalize.go` — insert new logic between the `if targetVariant == nil { ... }` guard and the `// Show what will happen` block. Add (a) an agent-info line printed in the preamble using `targetVariant.Agent` / `targetVariant.AgentType` / `targetVariant.Model`, and (b) a consolidation-log lookup that prints a warning when the latest entry's `ChosenVariantID` differs from `*variantID`. The warning lookup MUST run before the `if !*force { ... }` block so it executes in `--force` runs too.
2. `cmd/orbit/finalize_test.go` (new) — table-driven tests covering: agent-info rendering with all/partial/no agent fields populated, mismatch warning, matching entry produces no warning, missing log file produces no warning, corrupt log file produces no warning, warning prints under `--force`.

**Approach:**

- Reuse the existing `consolidation.NewLogger(orbitDir)` constructor and call `Read()` to obtain the full log; the latest entry is the last element of `log.Entries`. This mirrors `cmd/orbit/consolidate.go:240`, which constructs the logger as `consolidation.NewLogger(filepath.Join(specDir, ".orbit"))`.
- Format the agent info as a single line in the preamble. Suggested rendering when all fields are present: `Agent: <alias> (<type>, model: <model>)`. Build the parenthetical conditionally so any of `<type>` or `<model>` being empty produces a clean string with no empty parens, dangling commas, or `model: ` with no value. When all three fields are empty, print `Agent: unknown`.
- Format the mismatch warning to standard output (not stderr) so it appears inline with the rest of the preamble, prefixed with `Warning:` to match the existing `Error:` formatting style used later in the file. Include the prior `chosen_variant_id`, the requested `--variant`, and the prior consolidation `timestamp` formatted as RFC3339.
- Treat any error from `Read()` (file missing, JSON parse failure, empty `Entries` slice) as "no verification" and skip silently. Do not surface log read errors to the user — consolidation is optional and absence of a log is normal.
- For tests, construct a temp repo + spec dir in `t.TempDir()` (the established pattern in `cmd/orbit/subdir_test.go`) and capture stdout using the inline `os.Pipe` / `os.Stdout = w` pattern used in `cmd/orbit/status_test.go:508-517`. Write fixture `consolidation-log.json` files directly into the temp `.orbit` directory.

**Dependencies:**

- `internal/consolidation` — `Logger`, `LogEntry`, `ConsolidationLog`, `NewLogger`, `Read` (all already exported and stable).
- `internal/variants` — `Variant.Agent`, `Variant.AgentType`, `Variant.Model` (already populated during runs by `Manager.UpdateAgentInfo`, see `manager.go:365`).

**Out of Scope:**

- Adding agent/model info to other commands (`status`, `compare`, `cleanup`).
- Persisting any new fields to `variants.json` or `consolidation-log.json`.
- Refusing to finalize on mismatch, prompting an extra confirmation, or adding a `--force-mismatch` flag (user opted for warn-only behaviour).
- Verifying anything beyond the most recent consolidation entry (older entries are not consulted).
- Auto-detecting the consolidated variant ID when `--variant` is omitted.

## Risks and Assumptions

- **Risk:** A user runs consolidation, then re-runs consolidation against a different variant, then finalizes the original variant — the warning will fire even though both consolidations are legitimate. **Mitigation:** Acceptable; the warning is informational and the user can still proceed. The message names the timestamp so the user can see which consolidation is being referenced.
- **Risk:** `consolidation-log.json` exists but is locked by a concurrent consolidation. **Mitigation:** `Logger.Read()` does not acquire the flock (only `Append` does), so a concurrent `Append` will not block the read; a partially-written file would fail JSON parse and be silently skipped per the missing-log handling.
- **Assumption:** `targetVariant.Agent` and `AgentType` are populated for any variant produced by a recent run; older variants from before `UpdateAgentInfo` was added may have empty fields, which is handled by the conditional rendering above.
- **Assumption:** The most recent entry in the consolidation log corresponds to the consolidation the user intends to verify against. Multiple consolidation entries are possible (rollback then re-consolidate), so "most recent" is the only correct choice — checking earlier entries would produce false negatives.
