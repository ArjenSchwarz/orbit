# Decision Log: Centralized Logging

## Decision 1: Log Content Scope

**Date**: 2025-01-28
**Status**: accepted

### Context

Centralized logging needs to capture information useful for debugging. The question is whether to include agent stdout/stderr (which can be large) or focus on Orbit's internal operations.

### Decision

Capture Orbit internals only - phase transitions, retries, errors, configuration loading. Agent output remains in existing session logs.

### Rationale

Agent output is already captured in per-spec session logs (`phase-N-session.json/txt`). Duplicating this data would bloat centralized logs and make them harder to navigate. Orbit internals are the gap in current debugging capability.

### Alternatives Considered

- **Everything including agent output**: Full capture - Rejected because it duplicates existing session logs and creates very large files
- **Orbit internals + agent summary**: Include exit codes and durations - Partially accepted; we do log agent completion with these metrics

### Consequences

**Positive:**
- Smaller, focused log files
- No duplication with existing session logs
- Faster writes during execution

**Negative:**
- Need to cross-reference session logs for full agent output
- May require looking in two places when debugging

---

## Decision 2: Log Retention Strategy

**Date**: 2025-01-28
**Status**: accepted

### Context

Centralized logs will accumulate over time. Options include manual cleanup, automatic rotation by count/age/size, or hybrid approaches.

### Decision

Manual cleanup only. No automatic rotation or deletion.

### Rationale

Users have different retention needs. Some may want to keep months of history, others only days. Manual cleanup gives users full control and avoids accidentally deleting logs that may be needed for debugging intermittent issues.

### Alternatives Considered

- **Auto-rotate by count**: Keep last N runs - Rejected to avoid accidental data loss
- **Auto-rotate by age**: Delete after X days - Rejected because users may need old logs for intermittent bugs
- **Auto-rotate by size**: Delete when over limit - Rejected because size limits are arbitrary

### Consequences

**Positive:**
- No accidental data loss
- Users retain full control
- Simpler implementation

**Negative:**
- Disk usage can grow unbounded
- Users must remember to clean up
- Need to document cleanup procedure

---

## Decision 3: Log Format

**Date**: 2025-01-28
**Status**: accepted

### Context

Log format affects both human readability and machine parseability. Options include plain text, structured JSON, or both.

### Decision

Use JSON Lines format (one JSON object per line).

### Rationale

JSON Lines provides the best balance: human-readable enough for quick inspection, fully machine-parseable for tools like `jq`, grep-friendly for searching, and extensible for adding new fields without breaking parsers.

### Alternatives Considered

- **Plain text with timestamps**: Human-readable - Rejected because it's hard to parse programmatically
- **Both formats**: JSON and text files - Rejected because it doubles storage and complexity

### Consequences

**Positive:**
- Easy to parse with jq, grep, and custom tools
- Extensible without breaking consumers
- One line per entry makes grep effective
- Compatible with log aggregation tools

**Negative:**
- Slightly less readable than plain text
- Requires jq or similar for pretty printing

---

## Decision 4: Relationship to --debug Flag

**Date**: 2025-01-28
**Status**: accepted

### Context

Orbit has an existing `--debug` flag that outputs verbose information to stderr via `debug.Logger`. The centralized logging feature needs to decide how to relate to this existing functionality. A design review raised concerns about duplicating logging infrastructure.

### Decision

Extend the existing `debug.Logger` to support both stderr and file output. The `--debug` flag controls stderr output while centralized logging controls file output. Both can be independently enabled/disabled.

### Rationale

Extending `debug.Logger` leverages all existing debug call sites automatically without code duplication. Users retain flexibility: stderr for interactive debugging, file for post-hoc analysis. The implementation is simpler than maintaining two separate logging systems.

### Alternatives Considered

- **Completely independent systems**: Separate logger for files - Rejected because it duplicates infrastructure and requires adding calls at every debug site
- **Merge functionality**: --debug enables both - Rejected because it removes user control over output destinations

### Consequences

**Positive:**
- All existing debug call sites automatically gain file logging
- No code duplication
- Users can independently control stderr vs file output
- Simpler maintenance

**Negative:**
- Slightly more complex Logger implementation
- Must ensure JSON serialization doesn't affect stderr output format

---

## Decision 5: Log Storage Location

**Date**: 2025-01-28
**Status**: accepted

### Context

Centralized logs need a consistent location accessible regardless of which project is being orchestrated.

### Decision

Store logs in `~/.orbit/logs/` directory.

### Rationale

This location is alongside the existing `~/.orbit/runs/` registry, establishing `~/.orbit/` as Orbit's central data directory. It's user-accessible, follows XDG conventions loosely, and doesn't require elevated permissions.

### Alternatives Considered

- **~/.orbit/debug-logs/**: More specific name - Rejected as unnecessary distinction
- **System temp with symlink**: Better for ephemeral data - Rejected because logs should persist across reboots

### Consequences

**Positive:**
- Consistent with existing ~/.orbit/ structure
- User-accessible without elevated permissions
- Persists across system reboots

**Negative:**
- Takes space in home directory
- May need documentation for users on systems with small home quotas

---

## Decision 6: Log File Naming

**Date**: 2025-01-28
**Status**: accepted

### Context

Log files need unique names that help users identify which run they correspond to.

### Decision

Use `{timestamp}-{run-id}.jsonl` format, e.g., `20250128-120530-abc123def.jsonl`.

### Rationale

Timestamp prefix makes files sort chronologically in directory listings. Run ID suffix enables correlation with the run registry. Combined, they're unique and informative.

### Alternatives Considered

- **Run ID only**: e.g., abc123.jsonl - Rejected because you can't see timing without opening file
- **Timestamp only**: e.g., 20250128-120530.jsonl - Rejected due to collision risk with parallel runs

### Consequences

**Positive:**
- Chronological sorting in file listings
- Easy correlation with run registry
- Unique even for parallel runs

**Negative:**
- Longer filenames
- Timestamp redundant with file metadata (but more reliable)

---

## Decision 7: Variant Log Handling

**Date**: 2025-01-28
**Status**: accepted

### Context

Multi-variant runs execute multiple implementations in parallel. Logs could be combined or separated. A design review identified a gap: where do parent orchestration events (variant creation, parallel execution start) go?

### Decision

Create separate log file per variant using `{timestamp}-{run-id}-variant-{N}.jsonl` naming. Parent orchestration events (variant creation, parallel execution start, all variants completed) go to the main log file `{timestamp}-{run-id}.jsonl`.

### Rationale

Separate files allow independent debugging of each variant without interleaved entries. The main log file captures the orchestration layer that coordinates variants, providing a complete picture of the multi-variant run.

### Alternatives Considered

- **Combined with variant ID in entries**: Single file - Rejected because interleaved entries are hard to follow
- **Parent events in variant-0**: Use first variant's log - Rejected because variant-0 may fail or be deleted, losing orchestration context

### Consequences

**Positive:**
- Clean separation for debugging
- No interleaving confusion
- Parent orchestration events preserved in dedicated file
- Each variant's log is self-contained

**Negative:**
- Multiple files to manage (N+1 for N variants)
- Cross-variant correlation requires opening multiple files

---

## Decision 8: Configuration Flag Naming

**Date**: 2025-01-28
**Status**: accepted

### Context

The initial spec used `--no-centralized-log` (negated boolean) for the CLI flag while using positive booleans for env vars and config files. A design review identified this as inconsistent with Orbit's patterns (e.g., `ORBIT_DEBUG` where true enables).

### Decision

Use positive flag pattern: `--centralized-log=false` to disable, matching Orbit's existing configuration patterns.

### Rationale

Consistency with existing patterns reduces cognitive load. Users already understand `--debug` and `ORBIT_DEBUG=true` enabling features. Using the same pattern for centralized logging follows the principle of least surprise.

### Alternatives Considered

- **Negated flag**: `--no-centralized-log` - Rejected because it's inconsistent with other Orbit flags
- **Config-only option**: No CLI flag - Rejected because users need per-run control

### Consequences

**Positive:**
- Consistent with existing Orbit patterns
- Reduced cognitive load for users
- Same pattern across CLI, env, and config

**Negative:**
- Requires typing `=false` rather than just `--no-X`

---

## Decision 9: Schema Versioning

**Date**: 2025-01-28
**Status**: accepted

### Context

A design review identified that structured log formats need versioning to handle future format changes without breaking parsing tools.

### Decision

Include a `schema_version` field with value `1` in the first log entry (startup entry) of each file.

### Rationale

Placing the version in the first entry allows tools to quickly determine compatibility before processing the entire file. Starting at version 1 follows semantic versioning principles. Future changes can increment this value.

### Alternatives Considered

- **Version in every entry**: Redundant and bloats file size
- **Version in filename**: e.g., `.v1.jsonl` - Rejected because it complicates file matching patterns
- **No versioning**: Rejected because it makes future changes breaking

### Consequences

**Positive:**
- Tools can detect incompatible log formats
- Clear upgrade path for format changes
- Minimal overhead (one field in one entry)

**Negative:**
- Parsers must check first entry for version

---

## Decision 10: Completion Markers

**Date**: 2025-01-28
**Status**: accepted

### Context

A design review identified that users analyzing log files have no way to know if a log is complete or if the run was interrupted/crashed.

### Decision

Log a startup entry as the first entry and a shutdown entry as the final entry when orchestration completes normally. Absence of shutdown entry indicates crash or interruption.

### Rationale

This is a standard pattern in logging systems. Users can quickly check log completeness by looking for the shutdown entry. No additional complexity for the common case (normal completion).

### Alternatives Considered

- **Checksum at end of file**: More robust validation - Rejected as overkill for debug logs
- **Separate completion marker file**: e.g., `.complete` - Rejected because it adds file management complexity
- **No markers**: Rejected because users can't distinguish complete from incomplete logs

### Consequences

**Positive:**
- Easy to detect incomplete logs
- Standard pattern familiar to developers
- No additional files to manage

**Negative:**
- Crashed runs won't have shutdown marker (but that's the point)

---

## Decision 11: Session Cross-References

**Date**: 2025-01-28
**Status**: accepted

### Context

A design review identified that users have no way to navigate from centralized log entries to the full session transcripts stored in `specs/{spec}/.orbit/`.

### Decision

Include `session_log_path` and `transcript_path` fields with absolute paths in relevant log entries (agent completion, phase completion).

### Rationale

Absolute paths work regardless of current working directory. Including paths in the log entries creates a direct link between the centralized overview and detailed transcripts.

### Alternatives Considered

- **Relative paths**: Shorter but require knowing the reference point
- **Symbolic links in log directory**: Create links to transcripts - Rejected because it adds complexity and file management
- **No cross-references**: Rejected because it makes debugging harder

### Consequences

**Positive:**
- Direct navigation from centralized log to transcripts
- Works from any directory
- No additional files or infrastructure

**Negative:**
- Paths may break if user moves transcript files
- Slightly larger log entries

---

## Decision 12: Log Location Discoverability

**Date**: 2025-01-28
**Status**: accepted

### Context

A design review identified that users need to know where logs are written without having to read documentation or guess.

### Decision

Output the log file path to stderr at orchestration start with format: `Logging to {path}`. This message appears even when `--debug` is disabled.

### Rationale

Immediate feedback tells users where to look. Outputting to stderr (not stdout) avoids interfering with any piped output. The message is concise and actionable.

### Alternatives Considered

- **Only in documentation**: Rejected because users shouldn't need to look it up
- **Status command**: `orbit logs --list` - Could be added later but doesn't help during runs
- **Verbose mode only**: Rejected because this is useful for all users

### Consequences

**Positive:**
- Users immediately know where logs are
- No documentation lookup required
- Minimal noise (one line)

**Negative:**
- One extra line of output on every run

---

## Decision 13: Concurrency Safety for Parallel Writes

**Date**: 2025-01-28
**Status**: accepted

### Context

Multi-variant runs execute in parallel, and the parent orchestrator may log events to the main log file concurrently with variant-specific logging. This risks corrupted JSONL output if writes interleave.

### Decision

Serialize concurrent writes to each log file using a mutex or channel-based writer to prevent interleaved or corrupted entries.

### Rationale

JSONL format requires each line to be a complete, valid JSON object. Interleaved writes would corrupt the format and make logs unparseable. The performance cost of synchronization is negligible compared to disk I/O.

### Alternatives Considered

- **No synchronization**: Assume writes are atomic - Rejected because file writes are not guaranteed atomic
- **Separate files per goroutine**: Each writer gets own file - Rejected because it fragments the parent orchestration log

### Consequences

**Positive:**
- Guaranteed valid JSONL format
- Safe parallel variant execution
- No data corruption

**Negative:**
- Slight serialization overhead (negligible)
- Requires mutex or channel coordination

---

## Decision 14: Logger API Refactoring

**Date**: 2025-01-28
**Status**: accepted

### Context

The current `debug.Logger` uses `Log(format string, args ...any)` which produces pre-formatted strings. This is incompatible with dual-format output (structured JSON for files, human-readable for stderr).

### Decision

Refactor the `debug.Logger` API to accept structured data: level, component, message, and optional fields map. All existing helper methods (`LogCmd`, `LogRetry`, etc.) will be updated to use this new signature internally.

### Rationale

Structured input enables the logger to format output appropriately for each destination. JSON serialization requires structured data; pre-formatted strings cannot be reliably decomposed.

### Alternatives Considered

- **Parse format strings**: Extract structured data from formatted output - Rejected because it's fragile and error-prone
- **Separate logging system**: New logger alongside existing - Rejected because it duplicates code and call sites

### Consequences

**Positive:**
- Clean dual-format output
- Type-safe structured logging
- Existing helper methods continue to work

**Negative:**
- Breaking API change requires updating all direct `Log()` calls
- Migration effort for existing code

---

## Decision 15: Run ID Generation

**Date**: 2025-01-28
**Status**: accepted

### Context

Log filenames include a run-id for uniqueness and correlation with the run registry. The generation method needs to guarantee uniqueness.

### Decision

Use v4 UUIDs for run-id generation.

### Rationale

v4 UUIDs are universally unique, well-understood, and already used in Orbit (run registry uses UUIDs). They eliminate collision risk even for concurrent runs across machines.

### Alternatives Considered

- **Timestamp only**: Risk of collision with parallel runs
- **Sequential counter**: Requires persistent state management
- **Hash of run parameters**: Complex and potentially non-unique

### Consequences

**Positive:**
- Guaranteed uniqueness
- Consistent with existing Orbit patterns
- No state management required

**Negative:**
- Longer filenames
- Not human-memorable (but timestamps provide human context)

---

## Decision 16: Warning Rate Limiting

**Date**: 2025-01-28
**Status**: accepted

### Context

If log writes fail repeatedly (e.g., disk full), warning messages could flood stderr and make the terminal unusable.

### Decision

Rate-limit write failure warnings to at most one per 10 seconds.

### Rationale

One warning provides awareness of the issue; continuous warnings provide no additional value and create noise. 10 seconds balances visibility with usability.

### Alternatives Considered

- **No rate limiting**: Warn on every failure - Rejected because it floods stderr
- **First warning only**: Single warning then silence - Rejected because users may miss transient issues
- **Disable logging after N failures**: Stop trying - Rejected because issue may resolve (e.g., temporary disk pressure)

### Consequences

**Positive:**
- Usable terminal during write failures
- Continued visibility of persistent issues
- Recovery attempts continue

**Negative:**
- Users may not see every failure (acceptable trade-off)

---
