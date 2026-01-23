# Decision Log: Variant Consolidation

## Decision 1: Generate Both HTML and Markdown Reports Automatically

**Date**: 2025-01-23
**Status**: accepted

### Context

The comparison report is currently generated only in HTML format. AI coding agents have difficulty parsing HTML, making it harder to use the comparison report as context for further automation (like consolidation).

### Decision

Generate both HTML and Markdown reports automatically whenever a comparison is run, without requiring a flag.

### Rationale

Generating both formats by default ensures maximum compatibility: humans get the nicely formatted HTML, while agents get the easily parseable Markdown. The overhead of generating an additional file is negligible.

### Alternatives Considered

- **Flag-controlled format (`--format`)**: User specifies which format(s) to generate - Rejected because it adds friction to the workflow and users would need to remember to request Markdown
- **Markdown only by default**: Replace HTML with Markdown - Rejected because HTML provides better visual presentation for human review

### Consequences

**Positive:**
- AI agents can easily consume comparison reports
- No workflow changes required for existing users
- Enables the consolidate command to use Markdown as context

**Negative:**
- Slightly more disk space used (negligible)
- Two files to manage instead of one

---

## Decision 2: Stop Consolidation on Merge Conflicts

**Date**: 2025-01-23
**Status**: accepted

### Context

When the AI agent attempts to apply improvements from non-chosen variants, some changes may conflict with the chosen variant's implementation.

### Decision

If applying an improvement would cause a merge conflict or error, stop consolidation immediately and report what was applied vs. what failed.

### Rationale

Partial, silent merges can leave the codebase in an inconsistent state. Stopping immediately gives the user a clear point to investigate and decide how to proceed. The consolidation log will show exactly what succeeded and what didn't.

### Alternatives Considered

- **Apply what works, skip conflicts**: Continue consolidation, skipping conflicting changes - Rejected because this can lead to incomplete improvements being applied without user awareness
- **Interactive resolution**: Prompt user for each conflict - Rejected because this adds complexity and breaks the flow; conflicts are better handled manually with full context

### Consequences

**Positive:**
- User always knows the exact state of consolidation
- No hidden partial merges
- Clear recovery point

**Negative:**
- User must manually handle conflicts
- May need multiple consolidation attempts

---

## Decision 3: Consolidate Works on Any Variant State

**Date**: 2025-01-23
**Status**: accepted

### Context

Variants can exist in different states: still in worktrees (not finalized) or already finalized (merged to branch). The consolidate command needs to handle both.

### Decision

Allow consolidation on variants in any state, whether they're still in worktrees or have been finalized.

### Rationale

Users may want to consolidate improvements before committing to a final variant (to evaluate the combined result) or after finalizing (to enhance the chosen implementation). Requiring finalization first would limit the workflow unnecessarily.

### Alternatives Considered

- **Require finalized variant**: Must run `orbit finalize` before consolidate - Rejected because it forces a workflow order that may not suit all users

### Consequences

**Positive:**
- Flexible workflow
- Can evaluate consolidated result before finalizing

**Negative:**
- More complex implementation (must handle both worktree and branch targets)

---

## Decision 4: Keep Worktrees After Consolidation

**Date**: 2025-01-23
**Status**: accepted

### Context

After consolidation completes, the non-chosen variant worktrees still exist. Should they be automatically cleaned up?

### Decision

Keep worktrees intact after consolidation. Users run `orbit cleanup` manually if desired.

### Rationale

Users may want to reference the original variant implementations after consolidation, especially if they need to investigate issues or extract additional code. Automatic cleanup is irreversible and aggressive.

### Alternatives Considered

- **Prompt to cleanup**: Ask user after consolidation if they want to remove worktrees - Rejected because it interrupts the workflow and cleanup is a separate concern
- **Auto-cleanup**: Automatically remove non-chosen variant worktrees - Rejected because it's too aggressive and may destroy useful reference material

### Consequences

**Positive:**
- Original variants remain available for reference
- Consistent with existing Orbit behavior
- User maintains control

**Negative:**
- Disk space not reclaimed until manual cleanup

---

## Decision 5: Use Default Agent for Consolidation

**Date**: 2025-01-23
**Status**: accepted

### Context

The consolidate command needs to invoke an AI agent for analysis and code changes. Which agent configuration should it use?

### Decision

Use the default agent (as configured in `.orbit.yaml` or command-line), the same approach used for comparison.

### Rationale

Using the default agent keeps configuration simple and consistent with how comparison works. There's no need to track which agent generated the comparison report—the default agent is capable of interpreting and applying the improvements regardless of which agent identified them.

### Alternatives Considered

- **Track comparison agent**: Store which agent generated the comparison and reuse it - Rejected because it adds metadata complexity without clear benefit
- **`--agent` flag only**: Require explicit agent selection - Rejected because it adds friction; default is sufficient

### Consequences

**Positive:**
- Simple configuration model
- Consistent with existing orbit behavior
- No additional metadata tracking needed

**Negative:**
- If user changes default agent between compare and consolidate, behavior may differ (acceptable trade-off)

---

## Decision 6: Leverage Existing Comparison Report for Context

**Date**: 2025-01-23
**Status**: accepted

### Context

The consolidation agent needs to understand what improvements to apply. Should it re-analyze the variants or use the existing comparison report?

### Decision

Use the existing Markdown comparison report as the primary context for consolidation, particularly the "cross-variant improvements" section.

### Rationale

The comparison report already contains a thorough analysis of variant differences and identified improvements. Re-analyzing would waste tokens and potentially produce inconsistent recommendations. The Markdown format makes this easy to include in the agent prompt.

### Alternatives Considered

- **Fresh analysis**: Agent analyzes diffs independently - Rejected because it duplicates work already done in comparison
- **Both combined**: Include comparison report plus fresh diff analysis - Rejected because it adds complexity without clear benefit

### Consequences

**Positive:**
- Efficient token usage
- Consistent with comparison findings
- Faster consolidation

**Negative:**
- Depends on comparison report quality
- Cannot discover new improvements not in the report

---

## Decision 7: Two-Phase Confirmation (Plan then Apply)

**Date**: 2025-01-23
**Status**: accepted

### Context

Consolidation modifies the codebase. How much user confirmation should be required?

### Decision

Show a consolidation plan (what will be changed) and require explicit user confirmation before applying any changes.

### Rationale

Users should understand what will happen before code is modified. The plan phase allows review and cancellation without side effects. This is consistent with other destructive operations in Orbit (like finalize).

### Alternatives Considered

- **Dry-run by default**: Show what would change, `--apply` flag to actually apply - Rejected because it adds flag complexity; interactive confirmation is clearer
- **Apply directly with `--force`**: Non-interactive by default - Rejected because it's too dangerous for a code-modifying operation

### Consequences

**Positive:**
- User always sees what will happen
- Safe by default
- Can abort without side effects

**Negative:**
- Requires interactive terminal (no fully automated consolidation)

---

## Decision 8: Single Commit for Applied Improvements

**Date**: 2025-01-23
**Status**: accepted

### Context

When the agent applies improvements from non-chosen variants, the changes need to be tracked in git. Should changes be committed incrementally per improvement or as a single commit?

### Decision

Commit all applied improvements as a single commit with a descriptive message referencing the consolidation.

### Rationale

A single commit makes it easy to review all changes at once and enables simple rollback via `git revert` or `git reset`. Incremental commits would make rollback more complex and create noise in the git history.

### Alternatives Considered

- **Commit per improvement**: Each improvement gets its own commit - Rejected because it complicates rollback and creates fragmented history
- **No commit (leave unstaged)**: Let user commit manually - Rejected because it doesn't provide a clean rollback point

### Consequences

**Positive:**
- Easy to review consolidated changes
- Simple rollback via git revert
- Clean git history

**Negative:**
- Cannot selectively revert individual improvements from the commit

---

## Decision 9: Run Tests After Applying Changes

**Date**: 2025-01-23
**Status**: accepted

### Context

After the agent applies improvements, how do we validate that the consolidated code works correctly?

### Decision

After applying improvements and committing, the agent shall run the project's test suite to validate changes. Test failures are reported but the commit is left in place for user review.

### Rationale

Running tests provides immediate feedback on whether the consolidation broke anything. Leaving the commit in place (rather than auto-reverting on test failure) allows the user to inspect what happened and potentially fix issues manually.

### Alternatives Considered

- **No validation**: Trust the agent applied changes correctly - Rejected because silent breakage is worse than knowing about failures
- **Auto-revert on test failure**: Automatically undo the commit if tests fail - Rejected because this loses the agent's work; user may want to fix rather than discard

### Consequences

**Positive:**
- Immediate feedback on consolidation quality
- User can review and fix failed cases
- Provides validation record in consolidation log

**Negative:**
- Adds time to consolidation process
- Requires project to have a runnable test suite

---

## Decision 10: Agent Discovers Code from Variant Worktrees

**Date**: 2025-01-23
**Status**: accepted

### Context

The comparison report's "cross-variant improvements" section describes improvements in prose but doesn't include actual code. How does the consolidation agent know what code to apply?

### Decision

Provide the agent with paths to all variant worktrees. The agent examines the source variant's actual code to understand what the improvement entails and how to apply it.

### Rationale

Passing the comparison report plus worktree locations gives the agent everything it needs: the report describes *what* to improve, and the worktrees show *how* the non-chosen variants implemented it. This approach has been validated manually and works well.

### Alternatives Considered

- **Include code in comparison report**: Extend the report format to embed code snippets - Rejected because it complicates the comparison phase and bloats the report
- **Fresh diff analysis**: Have consolidation agent re-analyze diffs - Rejected because it duplicates comparison work and may produce different findings

### Consequences

**Positive:**
- Agent has full context to understand improvements
- Comparison report format doesn't need changes
- Proven to work in manual testing

**Negative:**
- Requires variant worktrees to still exist (not yet cleaned up)

---

## Decision 11: Require Markdown Report, Offer to Generate

**Date**: 2025-01-23
**Status**: accepted

### Context

The consolidation command depends on the Markdown comparison report for context. What happens if only an HTML report exists (from runs before this feature)?

### Decision

If no Markdown comparison report exists, fail with a clear error and offer to run `orbit compare` to generate it.

### Rationale

Rather than silently falling back to HTML parsing or failing cryptically, provide a clear path forward. Offering to run compare is user-friendly and gets them to the working state with one confirmation.

### Alternatives Considered

- **Parse HTML instead**: Fall back to HTML if Markdown missing - Rejected because HTML parsing is fragile and adds complexity
- **Fail with error only**: Just tell user to run compare manually - Rejected because offering to run it is more helpful

### Consequences

**Positive:**
- Clear error message with actionable next step
- Single confirmation to resolve the issue
- No fragile HTML parsing needed

**Negative:**
- Existing HTML-only reports require re-running compare

---

## Decision 12: Agent Implements Rather Than Copies

**Date**: 2025-01-23
**Status**: accepted

### Context

When applying an improvement from a non-chosen variant, should the agent copy code directly or adapt it to fit the chosen variant's style and structure?

### Decision

The agent should understand the improvement's intent and implement it in a way that fits the chosen variant, rather than blindly copying code.

### Rationale

Different variants may have different code styles, naming conventions, or architectural approaches. Directly copying code could introduce inconsistencies. The agent should understand *what* the improvement achieves and implement it idiomatically within the chosen variant's codebase.

### Alternatives Considered

- **Direct copy**: Copy code verbatim from source variant - Rejected because it may not integrate well with chosen variant's patterns
- **Provide both options**: Let user choose copy vs. implement - Rejected because it adds complexity; adaptive implementation is generally better

### Consequences

**Positive:**
- Consistent code style in consolidated result
- Agent can adapt improvements to work with chosen variant's architecture
- More intelligent merging

**Negative:**
- Agent may interpret improvement differently than intended
- Harder to verify "correctness" of application

---

## Decision 13: Run Post-Run Command After Consolidation

**Date**: 2025-01-23
**Status**: accepted

### Context

Orbit supports a configurable post-run command (e.g., for linting, formatting, or additional validation). Should this run after consolidation?

### Decision

After applying improvements and committing, run both the project's test suite and the configured post-run command (if any).

### Rationale

Consolidation is similar to a phase run—it modifies code. The same validation that applies to normal phase completion should apply to consolidation. This ensures consistent code quality and catches issues the test suite might miss.

### Alternatives Considered

- **Tests only**: Only run tests, not post-run command - Rejected because it creates inconsistency with normal orbit runs
- **Skip all validation**: Trust the agent - Rejected because validation is valuable

### Consequences

**Positive:**
- Consistent with normal orbit run behavior
- Post-run command can catch formatting/linting issues
- Single validation approach across all code modifications

**Negative:**
- Adds time to consolidation process

---

## Decision 14: Transaction Model for Interrupt Safety

**Date**: 2025-01-23
**Status**: accepted

### Context

If consolidation is interrupted (Ctrl+C) or fails while the agent is mid-file-write, the codebase could be left in an inconsistent state with partial changes.

### Decision

Before applying any changes, create a git stash or temporary branch as a recovery snapshot. On interrupt or failure during application, automatically restore to this snapshot.

### Rationale

A transaction model ensures atomicity: either all changes are applied successfully, or none are. This prevents partial states that are difficult to diagnose and recover from manually.

### Alternatives Considered

- **Leave partial changes**: Let user manually clean up - Rejected because it creates poor UX and potential for lost work
- **Rely on single commit rollback**: Only provide rollback after commit completes - Rejected because failures before commit would leave uncommitted partial changes

### Consequences

**Positive:**
- Atomic operations - no partial states
- Automatic recovery on failure
- User doesn't need to manually clean up

**Negative:**
- Additional git operations (stash/branch creation)
- Slightly more complex implementation

---

## Decision 15: Require Clean Git State

**Date**: 2025-01-23
**Status**: accepted

### Context

If the target worktree has uncommitted changes when consolidation runs, those changes could conflict with or be lost during the consolidation process.

### Decision

Require a clean git state (no uncommitted changes) by default. Provide an `--allow-dirty` flag for users who want to proceed anyway.

### Rationale

Clean state ensures predictable behavior and makes rollback straightforward. The flag provides an escape hatch for advanced users who understand the risks.

### Alternatives Considered

- **Allow dirty state by default**: Proceed regardless of uncommitted changes - Rejected because it risks losing user's work
- **Auto-stash dirty changes**: Stash uncommitted changes before consolidation - Rejected because it adds complexity and may not match user intent

### Consequences

**Positive:**
- Predictable behavior
- Protects user's uncommitted work
- Clear rollback path

**Negative:**
- User must commit or stash changes before consolidating

---

## Decision 16: Plan-Only Mode for Scripting

**Date**: 2025-01-23
**Status**: accepted

### Context

Users may want to see what consolidation would do without being prompted for confirmation, useful for scripting, cost estimation, or review before running interactively.

### Decision

Provide a `--plan-only` flag that runs analysis and displays the consolidation plan, then exits without prompting for confirmation or applying changes.

### Rationale

This supports non-interactive use cases and allows users to review the plan, share it with teammates, or estimate effort before committing to the consolidation.

### Alternatives Considered

- **Interactive only**: Always require confirmation prompt - Rejected because it limits automation and scripting use cases
- **Separate analyze command**: `orbit consolidate-analyze` subcommand - Rejected because it fragments the CLI; a flag is simpler

### Consequences

**Positive:**
- Supports scripting and automation
- Allows plan review before running
- Consistent with `--dry-run` patterns in other tools

**Negative:**
- Another flag to document

---

## Decision 17: Staleness Detection via Commit SHAs

**Date**: 2025-01-23
**Status**: accepted

### Context

If variants have been modified after the comparison report was generated, the report's recommendations may be based on outdated code.

### Decision

Store commit SHAs for each variant in the comparison report metadata. When consolidating, compare current variant HEADs against stored SHAs and warn if they differ.

### Rationale

This prevents applying improvements based on stale analysis. A warning (rather than error) allows users to proceed if they understand the risk, while surfacing potential issues.

### Alternatives Considered

- **Ignore staleness**: Trust the comparison report regardless - Rejected because it could lead to incorrect improvements being applied
- **Hard error on staleness**: Refuse to consolidate if stale - Rejected because it's too restrictive; user may have made unrelated changes

### Consequences

**Positive:**
- Users aware when working with potentially stale data
- Prevents subtle bugs from outdated recommendations

**Negative:**
- Requires adding metadata to comparison reports
- May require re-running compare more often

---

## Decision 18: Standardized Commit Message Format

**Date**: 2025-01-23
**Status**: accepted

### Context

The consolidation commit needs a consistent format for: human readability, machine parsing (for `--rollback` validation), and git history clarity.

### Decision

Use the format: `feat(consolidate): Apply improvements from variants X, Y to variant Z for <spec>`

### Rationale

This follows conventional commit format, identifies the operation type, lists source and target variants, and includes the spec name. The `--rollback` command can validate commits by matching this pattern.

### Alternatives Considered

- **Freeform message**: Let agent choose message - Rejected because inconsistent messages make rollback validation difficult
- **Generic message**: "Consolidation commit" - Rejected because it lacks context about what was consolidated

### Consequences

**Positive:**
- Consistent git history
- Machine-parseable for rollback validation
- Clear context in commit log

**Negative:**
- Less flexibility in commit message

---

## Decision 19: Structured Plan Output Format

**Date**: 2025-01-23
**Status**: accepted

### Context

The consolidation plan shown to users before confirmation needs a consistent format for readability and potential machine parsing.

### Decision

Present the plan as a structured Markdown table showing: improvement description, source variant, priority, and files to be modified.

### Rationale

Markdown tables are readable in terminals, can be piped to files, and are parseable if needed. This matches the Markdown report format used elsewhere in Orbit.

### Alternatives Considered

- **Plain text**: Simple list format - Rejected because tables are more scannable for multiple improvements
- **JSON output**: Machine-readable but not human-friendly - Rejected as primary format; could be added later with `--format json`

### Consequences

**Positive:**
- Scannable format for quick review
- Consistent with Markdown theme of the feature
- Can be saved/shared easily

**Negative:**
- Tables can be hard to render in narrow terminals

---

## Decision 20: Use go-output v2 for Multi-Format Report Generation

**Date**: 2025-01-23
**Status**: accepted

### Context

The comparison report needs to be generated in both HTML and Markdown formats simultaneously. Building separate rendering logic for each format would duplicate code and risk inconsistencies.

### Decision

Use the go-output v2 library (at ../go-output/v2) to build a single structured document and render it to multiple formats (HTML, Markdown) from the same data source.

### Rationale

go-output v2 is specifically designed for this use case. It provides:
- Document-builder pattern with immutable output
- Concurrent multi-format rendering
- Consistent content across all formats
- Key order preservation in tables
- Thread-safe operations

Building on this library avoids duplicating rendering logic and ensures HTML and Markdown reports contain identical content.

### Alternatives Considered

- **Separate renderers**: Build independent HTML and Markdown rendering functions - Rejected because it duplicates code and risks drift between formats
- **Template-based generation**: Use text/template for both formats - Rejected because template maintenance is harder and go-output provides better abstraction

### Consequences

**Positive:**
- Single source of truth for report content
- Guaranteed consistency between formats
- Leverages existing battle-tested library
- Future formats (PDF, etc.) can be added easily

**Negative:**
- Dependency on external library
- Learning curve for go-output API

---

## Decision 21: Single-Session Autonomous Agent Design

**Date**: 2025-01-23
**Status**: accepted (supersedes aspects of Decisions 7, 12)

### Context

The original design had a two-phase approach: (1) agent analyzes and produces a plan, (2) user confirms, (3) agent applies. This created complexity around plan parsing, state preservation between sessions, and doubled agent costs.

### Decision

Use a single agent session that autonomously analyzes, decides, implements, and commits. The agent produces a report afterward. If the user doesn't like the result, they simply rollback the commit.

### Rationale

1. **Simpler implementation**: No plan parsing, no state management between phases
2. **Lower cost**: One agent session instead of two
3. **The commit is the undo**: Single commit makes rollback trivial via `git revert` or `--rollback`
4. **Proven approach**: Manual testing showed agents can successfully handle this autonomously when given proper context

### Alternatives Considered

- **Two-phase with plan approval**: User reviews plan before application - Rejected because it doubles agent cost, requires plan parsing, and the single-commit rollback provides equivalent safety
- **Dry-run mode**: Show what would change without applying - Rejected because it requires agent to "simulate" changes without committing, which is unreliable

### Consequences

**Positive:**
- Simpler code with fewer components
- Lower token costs (single session)
- Faster execution (no confirmation pause)
- Clean rollback via single commit

**Negative:**
- User doesn't see plan before execution (mitigated by easy rollback)
- Can't show per-improvement progress (agent runs as black box)
- Agent has more autonomy (must trust its decisions)

---

## Decision 22: Peer Review Enhancements - Edge Cases and Concurrency

**Date**: 2025-01-23
**Status**: accepted

### Context

Peer review by Gemini and Kiro identified several gaps in the design around edge cases, concurrency, and type completeness. These needed to be addressed before implementation.

### Decision

Enhance the design with:
1. **Staleness check implementation**: Add `checkStaleness()` method that compares report metadata against current variant HEADs
2. **Empty improvements handling**: Add `checkEmptyImprovements()` for early exit when no improvements to apply
3. **File locking**: Add flock-style locking to ConsolidationLogger for concurrent run safety
4. **Stash conflict handling**: If `git stash pop` causes conflicts, leave stash in place and warn user
5. **SessionExporter support**: Check for and call `ExportSession()` for agents like Kiro
6. **Prompt guardrails**: Add binary file exclusion, idempotency checks, and renamed file handling to prompt
7. **Type alignment**: Add improvement counts to LogEntry struct to match JSON schema

### Rationale

These enhancements close gaps that would otherwise manifest as bugs or unexpected behavior in production. The peer review process (Gemini + Kiro) provided valuable external validation and identified issues that internal review missed.

### Alternatives Considered

- **Address only critical issues**: Fix type mismatches and validation gaps only - Rejected because the high-priority items (concurrency, stash conflicts) could cause data loss
- **Defer to implementation**: Let implementation handle these details - Rejected because design should be clear enough to implement without guesswork

### Consequences

**Positive:**
- More robust handling of edge cases
- Concurrent runs won't corrupt log file
- Clear behavior when stash restore has conflicts
- Prompt prevents agent from making problematic changes (binary files, duplicates)

**Negative:**
- Additional complexity in RecoveryManager and Logger
- More test cases required

---
