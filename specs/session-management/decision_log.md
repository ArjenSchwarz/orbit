# Decision Log: Session Management

## Decision 1: Default to Flat Log Directory Structure

**Date**: 2025-12-22
**Status**: accepted

### Context

Orbit currently creates timestamped subdirectories (e.g., `.orbit/2025-12-22-143052-branch-name/`) for each run. This makes it difficult to find logs and prevents session continuation across runs since each run creates a new directory.

### Decision

Default to storing logs directly in the `.orbit/` directory with run-numbered filenames for history preservation.

### Rationale

A flat directory structure simplifies log discovery and enables session continuation by maintaining a persistent `summary.json` that tracks in-progress phases across runs.

### Alternatives Considered

- **Keep timestamped subdirs as default**: Preserves current behavior - Rejected because it prevents session continuation
- **Single files with no history**: Simpler but loses run history - Rejected because users need to see multiple attempts

### Consequences

**Positive:**
- Easier to find and manage logs
- Enables session continuation
- Single summary.json tracks all state

**Negative:**
- Run-numbered files may accumulate (phase-1-run-N)
- Existing scripts expecting subdirectories will need updating

---

## Decision 2: No Backward Compatibility for Old Log Format

**Date**: 2025-12-22
**Status**: accepted

### Context

Existing Orbit installations may have timestamped log directories from previous runs. We need to decide whether to support reading these old directories.

### Decision

Only support the new flat `.orbit/` directory structure. Old timestamped directories are not read or migrated.

### Rationale

Maintaining backward compatibility adds complexity for limited benefit. The old logs are still present and readable manually; they just won't be parsed by the new system.

### Alternatives Considered

- **Read both formats**: Support reading old timestamped dirs - Rejected to reduce complexity
- **Migration tool**: Provide a script to migrate old logs - Rejected as not worth the effort

### Consequences

**Positive:**
- Simpler implementation
- Cleaner codebase

**Negative:**
- Users cannot see old run history in new Orbit versions
- No automated migration path

---

## Decision 3: Auto-Recovery on Session Resume Failure

**Date**: 2025-12-22
**Status**: accepted

### Context

When resuming a Claude session, the session may no longer exist (expired, invalid UUID, etc.). We need to decide how to handle this failure case.

### Decision

Automatically start a fresh session with a new UUID when session resumption fails, and log a warning.

### Rationale

Stopping orchestration and requiring manual intervention creates friction. Since the goal is automation, auto-recovery maintains the "hands-off" experience while still informing the user of the fallback.

### Alternatives Considered

- **Fail with error**: Stop and let user decide - Rejected because it breaks automation
- **Prompt user**: Ask whether to start fresh or abort - Rejected because Orbit runs unattended

### Consequences

**Positive:**
- Orchestration continues without manual intervention
- User is informed via warning log

**Negative:**
- Context from previous session is lost
- User may not notice the warning if not watching logs

---

## Decision 4: Support Both CLI Flags and YAML Configuration

**Date**: 2025-12-22
**Status**: accepted

### Context

New options (`date-subdirs`, `continue-session`) need a configuration mechanism. Orbit already supports both CLI flags and `.orbit.yaml` configuration files.

### Decision

Support both CLI flags and YAML configuration for new options, following the existing priority order (CLI > env > project config > home config > defaults).

### Rationale

Consistency with existing configuration patterns and flexibility for users who prefer either approach.

### Alternatives Considered

- **CLI only**: Simpler implementation - Rejected because it's inconsistent with existing options

### Consequences

**Positive:**
- Consistent with existing configuration patterns
- Users can set defaults in config files

**Negative:**
- Additional code to wire up config values

---

## Decision 5: Log Cleanup Out of Scope

**Date**: 2025-12-22
**Status**: accepted

### Context

With flat directory storage and run-numbered files, log files will accumulate over time (e.g., `phase-1-run-1-session.json`, `phase-1-run-2-session.json`, etc.). The review identified this as a potential issue.

### Decision

Log file cleanup is out of scope for this feature. Users must manually delete old files if needed.

### Rationale

Adding a cleanup mechanism (e.g., `--max-runs` or `orbit clean`) increases scope and complexity. This can be addressed in a future feature if users report it as a problem.

### Alternatives Considered

- **--max-runs flag**: Automatically delete old run files - Rejected to limit scope
- **orbit clean command**: Dedicated cleanup command - Rejected to limit scope

### Consequences

**Positive:**
- Simpler implementation
- Focused feature scope

**Negative:**
- Disk usage may grow over time
- Users need to manually manage old files

---

## Decision 6: Concurrent Access Protection Out of Scope

**Date**: 2025-12-22
**Status**: accepted

### Context

If two Orbit processes run simultaneously on the same tasks file, they could both read/write `summary.json` and corrupt it. The review identified this as a potential issue.

### Decision

Concurrent Orbit invocations are explicitly unsupported. No file locking will be implemented.

### Rationale

Orbit is designed for single-user, unattended operation. The complexity of adding file locking is not justified by the use case. Documenting this limitation is sufficient.

### Alternatives Considered

- **File locking**: Add flock or similar mechanism - Rejected as over-engineering for the use case
- **Lock file**: Create `.orbit.lock` while running - Rejected as adding complexity

### Consequences

**Positive:**
- Simpler implementation
- No locking overhead

**Negative:**
- Running multiple Orbit processes may corrupt state
- Users must be aware of this limitation

---

## Decision 7: Post-Completion Commands Support Session Continuation

**Date**: 2025-12-22
**Status**: accepted

### Context

The design review asked whether post-completion commands (`--post-command`) should support session continuation like regular phases.

### Decision

Post-completion commands will support session continuation with the same logic as phases. A `post_completion` state object in `summary.json` tracks in-progress post-completion commands.

### Rationale

Consistency with phase behavior. If post-completion is interrupted, it should be resumable just like phases to avoid re-running potentially expensive review operations.

### Alternatives Considered

- **No session tracking**: Simpler implementation - Rejected for inconsistency with phase behavior

### Consequences

**Positive:**
- Consistent behavior across all Claude invocations
- Post-completion can be resumed after interruption

**Negative:**
- Additional state to track
- More complexity in log manager

---

## Decision 8: Warn on Branch Mismatch, Don't Block

**Date**: 2025-12-22
**Status**: accepted

### Context

If a user runs Orbit on branch A, interrupts, switches to branch B, and resumes, the session continuation may reference work from a different branch. Should we validate and block this?

### Decision

Log a warning when branch changes between runs but allow continuation. Store `branch_name` in `summary.json` and compare on load.

### Rationale

Blocking would be annoying for valid use cases (e.g., user created a new branch for the same feature). A warning informs the user of potential issues while allowing them to proceed.

### Alternatives Considered

- **No validation**: Remove branchName field - Rejected because user should be informed
- **Fail on mismatch**: Refuse to continue - Rejected as too restrictive

### Consequences

**Positive:**
- Users are informed of potential issues
- Flexibility to continue even with branch changes

**Negative:**
- Session continuation may have unexpected results if branches diverged significantly

---

## Decision 9: Session ID Mismatch is Reconciliation, Not Failure

**Date**: 2025-12-22
**Status**: accepted

### Context

Requirement [3.7](b) mentioned detecting resume failure by session ID mismatch. However, Claude may legitimately return a different ID than what was passed via `--session-id`.

### Decision

If Claude returns a different session ID than passed, treat this as normal reconciliation (update stored ID), not as a resume failure.

### Rationale

Claude's behavior with pre-generated session IDs is not fully documented. If Claude chooses to use a different ID, the session was still created successfully. Only treat explicit errors ("session not found", "invalid session") as resume failures.

### Alternatives Considered

- **Treat as failure**: Retry with new session - Rejected as potentially wasteful if session actually worked

### Consequences

**Positive:**
- More robust handling of Claude's session ID behavior
- Avoids unnecessary retries

**Negative:**
- If Claude ignores our ID and generates a new one, continuation may not work as expected on next resume

---
