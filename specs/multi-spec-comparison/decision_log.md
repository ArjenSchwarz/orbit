# Decision Log: Multi-Spec Comparison

## Decision 1: Backwards Compatibility Mode

**Date**: 2026-01-11
**Status**: accepted

### Context

Orbit currently runs a single implementation per spec directly in the working directory. Adding variant support could change the default behavior and break existing workflows.

### Decision

Running `orbit run` without `--variants` SHALL behave exactly as today - single-run mode with no worktrees, direct execution in the current directory.

### Rationale

Existing users and scripts depend on current behavior. Breaking changes would require migration effort and could cause unexpected failures. The variant feature is additive - users opt in when they want it.

### Alternatives Considered

- **Allow config file default**: Let `.orbit.yaml` set default variant count - Rejected because it could surprise users who expect consistent behavior across projects
- **Always use worktrees**: Even for single runs - Rejected because it adds overhead and complexity for simple use cases

### Consequences

**Positive:**
- Zero migration effort for existing users
- Predictable behavior across all Orbit versions
- Simple mental model: no flags = same as before

**Negative:**
- Cannot set organization-wide defaults for variant count
- Config file has less influence over execution mode

---

## Decision 2: Worktree Reuse Strategy

**Date**: 2026-01-11
**Status**: accepted

### Context

When running variants multiple times for the same spec, worktrees may already exist. The system needs a strategy for handling existing worktrees.

### Decision

WHERE worktrees exist from a previous run AND base commit matches current HEAD, the system SHALL reuse existing worktrees. WHERE base commit differs, the system SHALL fail with an error suggesting cleanup.

### Rationale

Reusing compatible worktrees saves time and preserves work-in-progress if a run was interrupted. Failing on incompatible worktrees prevents subtle bugs from stale code mixing with new implementations.

### Alternatives Considered

- **Always fail with error**: Require manual cleanup - Rejected because it adds friction for legitimate resume scenarios
- **Always clean and recreate**: Remove old worktrees automatically - Rejected because it could destroy valuable work-in-progress

### Consequences

**Positive:**
- Fast resumption of interrupted runs
- Preserves partial progress
- Clear failure mode when state is inconsistent

**Negative:**
- More complex state checking logic
- Users must run `orbit cleanup` when base commit changes

---

## Decision 3: Local-Only Cleanup on Finalize

**Date**: 2026-01-11
**Status**: accepted

### Context

The `orbit finalize` command adopts a variant and cleans up others. It could also clean up remote branches if they were pushed.

### Decision

The finalize command SHALL only delete local worktrees and branches. Remote branch cleanup is the user's responsibility.

### Rationale

Remote operations are destructive and harder to undo. Different teams have different Git workflows (PRs, protected branches, etc.). Keeping Orbit's scope local reduces risk and complexity.

### Alternatives Considered

- **Include remote deletion**: Also delete remote branches - Rejected because it requires force push permissions and could conflict with team workflows
- **Optional remote cleanup**: Add `--include-remote` flag - Rejected as scope creep; can be added later if needed

### Consequences

**Positive:**
- No risk of accidental remote deletion
- Works regardless of user's remote permissions
- Simpler implementation

**Negative:**
- Users must manually clean up remote branches
- Could leave orphan remote branches if forgotten

---

## Decision 4: Default Parallel Limit of 3

**Date**: 2026-01-11
**Status**: accepted

### Context

Parallel execution can overwhelm the API with concurrent requests. A sensible default limit prevents accidental resource exhaustion.

### Decision

The system SHALL support a maximum of 3 parallel variants by default, configurable via `--max-parallel N`.

### Rationale

Three variants provides meaningful exploration while keeping API load reasonable. It's enough to compare different approaches without excessive cost. Users who want more can explicitly opt in.

### Alternatives Considered

- **No hard limit**: User decides, rate limiting handles pressure - Rejected because rate limits cause delays; prevention is better
- **Default max of 5**: Higher default - Rejected because typical use cases don't need more than 3

### Consequences

**Positive:**
- Reasonable default prevents accidental overload
- Still configurable for power users
- Aligns with typical comparison use cases (2-3 approaches)

**Negative:**
- Users wanting more must explicitly configure
- May need adjustment as API limits evolve

---

## Decision 5: Claude for Comparison (Configurable)

**Date**: 2026-01-11
**Status**: accepted

### Context

The comparison step needs an AI model to analyze implementations and make recommendations. This could use the same agent as implementation or a dedicated model.

### Decision

The system SHALL use Claude for comparison analysis by default, with the model configurable via `.orbit.yaml`.

### Rationale

Claude is already integrated with Orbit and provides consistent analysis quality. Making it configurable allows future flexibility without coupling comparison to implementation choice.

### Alternatives Considered

- **Same as implementation**: Use whatever agent ran the implementation - Rejected because it ties comparison quality to implementation agent choice
- **Hardcoded Claude**: No configuration option - Rejected because it limits flexibility for users who may want different models

### Consequences

**Positive:**
- Consistent comparison quality regardless of implementation agent
- Configurable for different use cases
- Leverages existing Claude integration

**Negative:**
- Requires Claude access even if using different implementation agent
- Additional API costs for comparison step

---

## Decision 6: Partial Report on Total Failure

**Date**: 2026-01-11
**Status**: accepted

### Context

If all variants fail, the system needs to decide whether to generate any output or simply fail.

### Decision

WHERE all variants fail, the system SHALL generate a partial report showing failure information for each variant.

### Rationale

Failure information is valuable for debugging. A report showing what went wrong for each variant helps users understand patterns and fix issues. Silently failing discards useful diagnostic information.

### Alternatives Considered

- **Exit with error, keep worktrees**: Preserve failed state only - Rejected because it provides less structured information than a report
- **Exit with error, clean up**: Remove worktrees on total failure - Rejected because it destroys debugging information

### Consequences

**Positive:**
- Structured failure information in consistent format
- Easy to compare failure modes across variants
- Preserves worktrees for manual debugging

**Negative:**
- Report generation code must handle empty/failed data
- Users may initially be confused by a "report" with no successful implementations

---

## Decision 7: Include Cost/Duration Metrics in Report

**Date**: 2026-01-11
**Status**: accepted

### Context

Orbit already tracks cost, duration, and API turns per session. This data could be included in the comparison report.

### Decision

The report SHALL display cost, duration, and API turn metrics for each variant.

### Rationale

Cost efficiency is a valid criterion for choosing between implementations. Knowing that one variant took 3x longer or cost 2x more is useful context for decision-making.

### Alternatives Considered

- **Omit metrics, focus on code**: Keep report focused on implementation quality - Rejected because it hides valuable decision-making data

### Consequences

**Positive:**
- Complete picture of each variant's resource usage
- Supports cost-conscious decision making
- Leverages existing metric collection

**Negative:**
- Report becomes slightly more complex
- May influence decisions toward "cheaper" vs "better" implementations

---

## Decision 8: Independent Rate Limit Handling Per Variant

**Date**: 2026-01-11
**Status**: accepted

### Context

The initial design proposed coordinating rate limits across parallel variants by pausing all variants when one hits a rate limit. Reviewers pointed out that you cannot pause running Claude subprocesses mid-execution.

### Decision

Each variant SHALL handle rate limits independently using existing Orbit retry logic. No cross-variant coordination.

### Rationale

1. Running subprocesses cannot be paused gracefully without corrupting state
2. With subscriptions, rate limit behavior is unclear and less predictable
3. Existing retry logic already handles rate limits per-session
4. Simpler implementation with proven behavior

### Alternatives Considered

- **Blocking semaphore before API calls**: Gate new phase starts, let running complete - Rejected as unnecessary complexity given subscription model
- **Shared state with pause/resume**: Coordinate across variants - Rejected as technically infeasible

### Consequences

**Positive:**
- Simple, predictable behavior
- Leverages existing proven retry logic
- No complex coordination mechanisms needed

**Negative:**
- Parallel variants may hit rate limits independently, extending total time
- No optimization for shared rate limit pool

---

## Decision 9: Require Unchanged Original Branch for Finalize

**Date**: 2026-01-11
**Status**: accepted

### Context

The finalize command rebases variant changes onto the original branch. If the original branch has diverged while variants were running, rebase could fail or produce unexpected results.

### Decision

Finalize SHALL verify the original branch has not diverged from the base commit. If it has, fail with an error.

### Rationale

1. All variants start from the same commit (by design)
2. If the user stayed on the same feature branch, rebase should be clean
3. Divergence indicates concurrent work that needs manual resolution
4. Failing fast is safer than attempting a potentially problematic rebase

### Alternatives Considered

- **Handle divergence with rebase**: Attempt rebase anyway, pause on conflicts - Rejected because it adds complexity and may produce confusing states
- **Cherry-pick instead of rebase**: Apply individual commits - Considered but rebase is cleaner for linear history

### Consequences

**Positive:**
- Simple, predictable behavior
- No risk of silent merge problems
- Clear error message guides user to resolution

**Negative:**
- Users who work on the original branch during variant runs must handle manually
- Requires cleanup + fresh variant run if branch diverged

---

## Decision 10: Defer Screenshot Feature

**Date**: 2026-01-11
**Status**: accepted

### Context

The initial design included optional screenshot capture and comparison. Reviewers questioned whether this adds sufficient value for the added complexity.

### Decision

Screenshot capture and comparison is deferred to a future release.

### Rationale

1. Core value is in code comparison, not UI comparison
2. Screenshot tooling varies widely across projects
3. Reduces initial scope and complexity
4. Can be added later without breaking changes

### Alternatives Considered

- **Keep as optional**: Already scoped as optional - Rejected because even optional features require testing and documentation
- **Require screenshot command**: Make it mandatory for UI projects - Rejected as too restrictive

### Consequences

**Positive:**
- Reduced implementation scope
- Faster time to initial release
- Simpler requirements document

**Negative:**
- UI-focused projects may find feature less useful initially
- Will require future work to add

---

## Decision 11: Defer Cost Estimation

**Date**: 2026-01-11
**Status**: accepted

### Context

Running N variants multiplies API costs by N. Reviewers suggested adding cost estimation and budget limits to prevent accidental overspend.

### Decision

Cost estimation and budget limits are deferred to a future release.

### Rationale

1. Users understand that N variants = N x cost
2. Accurate cost estimation is difficult due to variable LLM execution
3. Initial release focuses on core functionality
4. Can be added based on user feedback

### Alternatives Considered

- **Pre-run estimate with confirmation**: Show estimate, require confirm - Rejected as adds friction for power users
- **Max-cost flag**: Stop when budget exceeded - Rejected as scope creep

### Consequences

**Positive:**
- Simpler user experience
- Faster initial release
- Users retain full control

**Negative:**
- No guardrails against accidental expensive runs
- Users must self-manage API costs

---

## Decision 12: Store Metadata in Spec Directory

**Date**: 2026-01-11
**Status**: accepted

### Context

Variant metadata (worktree paths, branch names, status) needs persistent storage. Options were spec-local or centralized in home directory.

### Decision

Store variant metadata in `specs/{spec-name}/.orbit/variants.json`.

### Rationale

1. Keeps all spec-related data together
2. Consistent with existing log storage pattern
3. Easy to find and inspect
4. Naturally cleaned up when spec directory is removed

### Alternatives Considered

- **~/.orbit/variants/{spec}.json**: Central home directory location - Rejected because it separates metadata from the spec it relates to

### Consequences

**Positive:**
- All spec data in one place
- Easy to inspect and debug
- Works with existing .orbit directory pattern

**Negative:**
- Requires spec directory structure
- Less discoverable without knowing spec name

---

## Decision 13: Add Status Command

**Date**: 2026-01-11
**Status**: accepted

### Context

Users need visibility into which variants exist, their current state, and related metadata. Without this, they must use raw git commands.

### Decision

Add `orbit status <spec-name>` command to display variant status table.

### Rationale

1. Provides discoverability for variant state
2. Shows all relevant info in one place (branches, paths, status)
3. More user-friendly than raw git worktree list
4. Consistent with common CLI patterns (git status, docker status)

### Alternatives Considered

- **Rely on git commands**: Let users use git worktree list - Rejected because it doesn't show Orbit-specific metadata

### Consequences

**Positive:**
- Easy variant state inspection
- Shows Orbit-specific metadata (base commit, original branch)
- Professional CLI experience

**Negative:**
- Additional command to implement and test
- Another subcommand to document

---

## Decision 14: Comparison Uses Git Diffs from Base

**Date**: 2026-01-11
**Status**: accepted

### Context

The comparison phase needs to send code to Claude for analysis. Options were sending unified diffs, full file contents, or configurable.

### Decision

Send unified git diffs from base commit showing what each variant changed.

### Rationale

1. Diffs are more token-efficient than full files
2. Shows exactly what changed, which is what we're comparing
3. Easier to fit multiple variants in context window
4. Consistent format across all file types

### Alternatives Considered

- **Full changed files**: Send complete file contents - Rejected because it wastes tokens on unchanged content
- **Configurable**: Allow --full-files flag - Rejected as unnecessary complexity; diffs are strictly better for comparison

### Consequences

**Positive:**
- Token-efficient comparison
- Focused on changes (the actual comparison target)
- Scales better to larger changes

**Negative:**
- May miss some context that full files would provide
- Requires understanding diff format

---

## Decision 15: Worktrees in .orbit Directory

**Date**: 2026-01-11
**Status**: accepted

### Context

The initial design placed worktrees as sibling directories of the repository (e.g., `../orbit-impl-1-spec/`). During design review, concerns were raised about polluting the parent directory with multiple worktree folders.

### Decision

Worktrees SHALL be created inside the spec's `.orbit` directory at `specs/{spec}/.orbit/worktrees/{prefix}-{N}-{spec}/`.

### Rationale

1. Keeps all variant-related files contained within the spec directory
2. No pollution of sibling directories in the parent folder
3. Easier cleanup - removing `.orbit/worktrees/` removes all variant state
4. Consistent with existing `.orbit/` usage for logs and metadata
5. Git worktrees support nested paths within the repository

### Alternatives Considered

- **Sibling directories**: `../orbit-impl-1-spec/` - Rejected because it pollutes the parent directory with multiple folders
- **Home directory**: `~/.orbit/worktrees/` - Rejected because it separates worktrees from their related spec

### Consequences

**Positive:**
- Clean parent directory
- All spec-related state in one location
- Simpler mental model for users

**Negative:**
- Slightly deeper nesting in path names
- Worktrees are within the repository (though ignored by git)

---

## Decision 16: Automatic .gitignore Management

**Date**: 2026-01-11
**Status**: accepted

### Context

With worktrees stored inside the repository at `.orbit/worktrees/`, there's a risk that users could accidentally commit these directories if they forget to add them to `.gitignore`.

### Decision

The system SHALL automatically create or update `.orbit/.gitignore` to include `worktrees/` before creating any worktrees.

### Rationale

1. Prevents user error without requiring manual configuration
2. Idempotent operation - safe to run multiple times
3. Non-destructive - appends to existing .gitignore rather than overwriting
4. Localized to .orbit directory - doesn't modify the repository's root .gitignore

### Alternatives Considered

- **Document in README**: Tell users to add .gitignore manually - Rejected because it relies on users reading and following documentation
- **Modify root .gitignore**: Add `.orbit/` to the repository's main .gitignore - Rejected because it modifies files outside the spec directory scope

### Consequences

**Positive:**
- Zero-configuration protection against accidental commits
- Works automatically on first variant run
- Respects existing .gitignore content

**Negative:**
- Creates an additional file in .orbit directory
- Users who want to commit worktrees (edge case) would need to modify the .gitignore

---
