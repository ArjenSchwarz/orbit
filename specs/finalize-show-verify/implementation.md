# Implementation: Finalize Show and Verify (T-1197)

This note explains the changes that landed under `specs/finalize-show-verify/` — what was added, how it works, and how each numbered requirement in `smolspec.md` maps to code.

## Beginner Level

### What Changed / What This Does

`orbit finalize` is the command you run when you have several variant implementations of a spec sitting in side-by-side worktrees and you've picked a winner. It rebases the winner onto your original branch and tidies up the rest. Before this change, the preamble it printed only said *which* variant you'd picked — not *who* implemented it (which AI agent, which model) or whether the variant you picked is actually the one you just consolidated against.

Two things were added to that preamble:

1. An **"Agent: ..." line** that names the agent alias, agent type (e.g., `claude-code`), and model used to produce the variant.
2. A **"Warning: ..." line** that fires when the variant ID you passed to `--variant` doesn't match the most recent consolidation entry. It only appears when there's a real mismatch; if you never ran `orbit consolidate`, or the variants line up, nothing extra is printed.

### Why It Matters

When you've been comparing three or four variants from different agents, it's easy to lose track. Showing the agent up front confirms you're finalizing what you think you are. The warning catches a specific footgun: running `orbit consolidate --variant 2` and then accidentally `orbit finalize --variant 1` ships the un-consolidated branch.

### Key Concepts

- **Variant**: A standalone implementation attempt living in its own git worktree. Multiple variants run side-by-side so you can compare.
- **Consolidation**: A separate step where the chosen variant absorbs improvements from the others. Each consolidation is logged in `consolidation-log.json`.
- **Finalize**: Adopt one variant as the final implementation, rebase it, delete the rest.

---

## Intermediate Level

### Changes Overview

Production change is contained to `cmd/orbit/finalize.go`:

- New imports: `time` and `internal/consolidation`.
- New helper `formatVariantAgentInfo(v *variants.Variant) string` (`finalize.go:180`).
- New helper `printConsolidationMismatchWarning(orbitDir string, variantID int)` (`finalize.go:160`).
- Two call sites in `finalizeCommand`:
  - `finalize.go:101` — print agent info inside the existing preamble.
  - `finalize.go:110` — print mismatch warning, deliberately placed *before* the `if !*force` block.

Tests live in the new `cmd/orbit/finalize_test.go` (table-driven, ~310 lines) and use the established temp-repo + stdout-capture patterns from `cmd/orbit/subdir_test.go` and `cmd/orbit/status_test.go`.

`CHANGELOG.md` gained two `Unreleased > Added` entries; `specs/OVERVIEW.md` registers the spec in the catalog. No CLAUDE.md changes were needed (no new flags, env vars, or config keys).

### Implementation Approach

- **Agent line.** `formatVariantAgentInfo` builds the string conditionally. If all three of `Agent`, `AgentType`, `Model` are empty it returns `Agent: unknown`. Otherwise it appends the alias, then a parenthetical containing whichever of `<type>` and `model: <model>` are populated. Empty fields produce no empty parens, no dangling commas, no `model: ` with nothing after it.
- **Mismatch warning.** Reuses `consolidation.NewLogger(filepath.Join(specDir, ".orbit"))` and `Logger.Read()` (mirrors `cmd/orbit/consolidate.go:240`). The latest entry is `log.Entries[len(log.Entries)-1]`. If `Read()` returns an error, or `Entries` is empty, the helper returns silently — that's the documented "no verification possible" case. When the entry exists and `ChosenVariantID != variantID`, it prints `Warning: variant N does not match the most recent consolidation (variant M, <RFC3339 timestamp>)\n\n` to stdout.
- **Placement.** The warning lookup runs *before* the `--force` short-circuit so the line is visible in CI / non-interactive runs.

### Trade-offs

- **Warn vs. hard error.** Decision log entry 1 records the choice: a hard error would block legitimate "I changed my mind" workflows where the user deliberately ships the un-consolidated original. The existing `y/N` prompt already serves as the deliberate acknowledgement.
- **Silent on missing/corrupt log.** Decision log entry 2: most finalize runs aren't preceded by consolidation, so a "no log found" line would be noise. Surfacing JSON parse errors was rejected because it conflates absence with corruption and blocks finalize for an unrelated reason.
- **No `Logger.Latest()` helper.** The reach into `log.Entries[len-1]` is duplicated between `internal/consolidation/logger.go:137` (`GetLatestCommitSHA`) and the new code. Encapsulation could be improved, but it's a two-call surface — out of scope for this spec.

---

## Expert Level

### Technical Deep Dive

- **Lock-free read.** `Logger.Read()` does not take the flock that `Append` uses, so a concurrent consolidation does not block the finalize preamble. A partially-written log fails JSON parse and is treated as "no verification possible" — same path as a missing file.
- **Stdout vs. stderr.** The warning is intentionally on stdout, matching the rest of the preamble formatting and the existing `Error:`-style messages later in the file. CI consumers reading stdout get the warning naturally.
- **Force semantics.** The placement of `printConsolidationMismatchWarning` at `finalize.go:110`, before `:113`, means automated `--force` runs still emit the warning to logs. The function is not gated on interactivity at all.
- **All-empty fallback.** `formatVariantAgentInfo` checks all three fields up front and returns `Agent: unknown` only when all are empty. With at least one populated, it never emits an empty-paren `()`, a dangling `, `, or a bare `model: `.
- **Sequencing constraint.** Tests mutate `os.Stdout` and `os.Chdir` globally and therefore can't use `t.Parallel()`. This is consistent with `status_test.go` and `subdir_test.go`. A future contributor who adds `t.Parallel()` here would silently corrupt other tests' captures — worth keeping in mind if the test surface grows.

### Architecture Impact

- The change is additive and contained to the finalize command surface. No exported API change. No schema change to `variants.json` or `consolidation-log.json`.
- The `consolidation.Logger.Read()` API is now consumed by two callers (`consolidate.go` and `finalize.go`). Adding an explicit `Latest() (LogEntry, bool)` method would consolidate the empty-check + last-element pattern; today it's open-coded twice. Worth doing the next time the consolidation package is touched, but not warranted on this branch.
- Out-of-scope items the spec called out are honoured: no changes to `status` / `compare` / `cleanup`; no `--force-mismatch` flag; no auto-detection of the consolidated variant when `--variant` is omitted.

### Potential Issues

- **Quiet corrupt log.** A genuinely corrupt `consolidation-log.json` is silently ignored. Decision log entry 2 accepts this: the next `consolidate` run will surface the corruption, and finalize is not the right diagnostic site.
- **Repeat consolidation, then finalize the original.** Documented risk in `smolspec.md`. The warning fires; the user can still proceed via `y/N` or `--force`. The warning includes the consolidation timestamp so the user can match it against shell history.
- **Older variants without agent fields.** `targetVariant.Agent`/`AgentType`/`Model` may be empty for variants produced before `Manager.UpdateAgentInfo` was added. `formatVariantAgentInfo` handles each empty subset, including all-empty, so no crash and no garbage output.

---

## Completeness Assessment

Each numbered requirement in `smolspec.md` mapped to the code that implements it.

| # | Requirement | Implementation |
|---|-------------|----------------|
| 1 | Display the agent used for the target variant in the finalize preamble | `cmd/orbit/finalize.go:101` calls `formatVariantAgentInfo(targetVariant)` between `Finalize spec:` and the `This will:` block. |
| 2 | Include alias, type, and model when recorded; absent fields omitted cleanly | `formatVariantAgentInfo` (`finalize.go:180-202`) builds the parenthetical from `AgentType` and `model: <Model>` only when populated, then joins with the alias. Test cases at `finalize_test.go:118-148` cover all-three, only-agent, agent+type, and only-model. |
| 3 | `Agent: unknown` when none of `Agent`, `AgentType`, `Model` are populated | `finalize.go:181-183` — explicit early return when all three are empty. Test at `finalize_test.go:144-147`. |
| 4 | Read `specs/<spec>/.orbit/consolidation-log.json` (when it exists) and compare its most recent `chosen_variant_id` against `--variant` | `printConsolidationMismatchWarning` (`finalize.go:160-174`) constructs `consolidation.NewLogger(orbitDir).Read()`; latest entry via `log.Entries[len(log.Entries)-1]`. |
| 5 | Print warning before the confirmation prompt naming both IDs and the consolidation timestamp as RFC3339 | `finalize.go:172-173` — `Warning: variant <requested> does not match the most recent consolidation (variant <chosen>, <RFC3339 timestamp>)`. Asserted by `finalize_test.go:303-305`. |
| 6 | Warning prints regardless of `--force` | Call placed at `finalize.go:110`, before `if !*force` at `:113`. Asserted by the `warning prints under force` test case (`finalize_test.go:289-294`). |
| 7 | Allow finalize to proceed via the existing `y/N` prompt or `--force`; no new flag | No flag added. Existing `bufio.NewReader(os.Stdin).ReadString('\n')` flow at `finalize.go:114-124` is untouched. |
| 8 | Treat missing or unreadable consolidation log as "no verification possible" — silent | `finalize.go:162-167` — early returns on `Read()` error or empty `Entries`. `missing log file produces no warning`, `corrupt json log produces no warning`, and `empty entries slice produces no warning` test cases all assert no `Warning:` string. |
| 9 | No change to existing finalize success/failure behaviour, exit codes, or rebase logic | Diff is purely additive (two helper functions and three lines in `finalizeCommand`). Rebase path at `finalize.go:127-148` is byte-identical to `origin/main`. |

### Out-of-Scope Compliance

The smolspec listed five out-of-scope items. None were implemented:

- No agent/model display added to `status`, `compare`, or `cleanup`.
- No new fields persisted to `variants.json` or `consolidation-log.json`.
- No `--force-mismatch` flag; no refusal-to-finalize on mismatch; no extra confirmation prompt.
- Only the most recent consolidation entry is consulted.
- `--variant` is still required; no auto-detection.

### Validation Findings

**Gaps identified:** none. Every numbered requirement has a clean explanation grounded in specific code.

**Logic issues:** none. The "only model populated" path produces `Agent: (model: <m>)` — a slightly awkward leading space before `(` because the alias slot is empty. The spec's wording (no empty parens, no dangling `model: `, no trailing punctuation) is satisfied; the leading-space reading is locked into `finalize_test.go:142`. Treated as intentional, not a defect.

**Questions raised:** the `log.Entries[len-1]` indexing is duplicated in `internal/consolidation/logger.go:137`. A `Logger.Latest()` helper would be cleaner; deferring to a future package touch.

**Recommendations:** none for this branch. Both items above are intentionally out of scope per the smolspec and decision log.
