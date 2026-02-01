# Auto-Consolidate

## Overview

This feature adds an `--auto-consolidate` flag to `orbit run --variants` that automatically runs consolidation on the recommended variant after comparison completes. This eliminates the manual step of running `orbit consolidate` separately, streamlining the variant workflow. An optional `post-consolidate-command` hook allows running a shell command on the consolidated branch after consolidation succeeds.

## Requirements

### Core Functionality

- The system MUST provide an `--auto-consolidate` flag for `orbit run`
- The system MUST validate that `--auto-consolidate` requires `--variants` and exit with error `"--auto-consolidate requires --variants to be specified"` if used alone
- The system MUST provide a `--no-auto-consolidate` flag to disable auto-consolidation when enabled via config
- The system MUST support `auto-consolidate: true` in `.orbit.yaml` configuration file
- The system MUST use the comparison result's `Recommendation` field as the consolidation target variant ID
- The system MUST use the default agent (from `--agent` flag or config) for consolidation, not variant-specific agents

### Execution Conditions

- The system MUST skip auto-consolidation if comparison was not run (fewer than 2 successful variants) with log message: `"Skipping auto-consolidation: comparison requires 2+ successful variants"`
- The system MUST skip auto-consolidation if comparison fails or returns no recommendation
- The system MUST check the recommended variant's worktree for uncommitted changes; if dirty and `--allow-dirty` is not set, skip with warning: `"Skipping auto-consolidation: worktree has uncommitted changes (use --allow-dirty to override)"`
- The system MUST pass the `--allow-dirty` flag value through to the consolidator configuration
- The system SHOULD log a warning and continue if consolidation fails (non-fatal to the variant run)
- The system MUST NOT run finalize after auto-consolidation (variants remain available for manual review)

### Post-Consolidate Command

- The system MUST provide a `post-consolidate-command` configuration option in `.orbit.yaml` that specifies a shell command
- The system MUST only execute `post-consolidate-command` when ALL of: auto-consolidate is enabled, the command is configured, and consolidation completed without error (includes both commit created and no-improvements-found cases)
- The system MUST execute `post-consolidate-command` in the consolidated variant's worktree directory
- The system MUST use the existing `command-timeout` configuration for `post-consolidate-command` (default 5m)
- The system SHOULD log a warning and continue if `post-consolidate-command` fails (non-fatal)

## Implementation Approach

**Key files to modify:**

1. `internal/config/config.go` - Add `AutoConsolidate bool` and `PostConsolidateCommand string` fields to Config struct, wire up Viper bindings
2. `cmd/orbit/run.go` - Add `--auto-consolidate`, `--no-auto-consolidate`, and `--allow-dirty` flags, validate `--auto-consolidate` requires `--variants`, pass to orbit.Config
3. `internal/orbit/orbit.go` - Add `AutoConsolidate`, `AllowDirty`, `PostConsolidateCommand` fields to Config struct, implement `runAutoConsolidate()` method called after `runComparison()` in `runWithVariants()`

**Approach:**

Follow the existing pattern for boolean flags like `--parallel`:
- Config field with Viper binding (see `config.go:78-82` for Parallel example)
- CLI flag override with negation flag (see `run.go:199-200` for parallel, `run.go:51` for `--no-post-prompt`)
- Orbit config field (see `orbit.go:103` for Parallel)

For consolidation execution, reuse the logic from `cmd/orbit/consolidate.go`:
- Create `consolidation.Config` with specName, specDir, variantID from `o.comparisonResult.Recommendation`, and AllowDirty from `o.config.AllowDirty`
- Get agent via existing `agents.Get()` using default agent config
- Instantiate `Consolidator` and call `Run()`
- Handle `ErrNoImprovements` gracefully (log info, still run post-consolidate-command)

For post-consolidate-command, follow the `runAgentPostCommand()` pattern in `orbit.go`:
- Execute via `exec.CommandContext` with `/bin/sh -c`
- Set working directory to variant's worktree path
- Use `o.config.CommandTimeout` for timeout
- Log output on failure, continue execution

**Integration point in `runWithVariants()`:**
```go
// After line 1570: if err := o.runComparison(ctx); err != nil { ... }
if o.config.AutoConsolidate && o.comparisonResult != nil {
    if err := o.runAutoConsolidate(ctx); err != nil {
        log.Printf("Auto-consolidation failed: %v", err)
        // Non-fatal, continue to report generation
    }
}
```

**Dependencies:**
- `internal/consolidation` package (Consolidator, Config, ErrNoImprovements)
- `internal/comparison` package (Result.Recommendation field) - already stored in `o.comparisonResult`
- `internal/agents` package for agent resolution
- Existing `o.variantManager` for worktree paths

**Out of Scope:**
- Per-agent post-consolidate-command configuration (global only)
- Automatic finalize after consolidation
- Consolidation of multiple variants (only consolidates the recommended one)
- Custom consolidation prompts during auto-consolidate (uses defaults)

## Risks and Assumptions

- **Risk:** Consolidation takes significant time, extending variant run duration | **Mitigation:** Document in help text that auto-consolidate adds agent execution time; consolidation is already optimized
- **Risk:** Post-consolidate-command fails but consolidation succeeded | **Mitigation:** Log warning, don't fail the run (consolidation commit is preserved)
- **Risk:** Worktree unexpectedly dirty after variant run | **Mitigation:** Check state and skip with warning unless `--allow-dirty` is set
- **Assumption:** The comparison result's `Recommendation` field is always a valid variant ID when comparison succeeds (validated by `parseAndValidate` in compare.go:188-190)
- **Assumption:** Successful variant runs leave worktrees in clean git state (all changes committed by agent)
- **Prerequisite:** Comparison must complete successfully with 2+ variants before auto-consolidation can run
