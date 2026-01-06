# Decision Log: Codex Support

## Decision 1: CLI Interface Approach

**Date**: 2026-01-05
**Status**: accepted

### Context

Adding Codex support requires deciding how users will interact with the feature. Options include keeping the CLI unchanged with transparent detection, adding explicit format flags, or creating separate subcommands.

### Decision

Make Codex support fully transparent. The existing CLI commands work unchanged, with automatic format detection.

### Rationale

Transparent operation provides the best user experience. Users don't need to learn new commands or remember to specify formats. The format can be reliably detected from the first line of the JSONL file.

### Alternatives Considered

- **Add --format flag**: Optional flag to force format detection - Rejected because it adds unnecessary complexity when auto-detection is reliable
- **Separate subcommand**: e.g., `apsis codex <session-id>` - Rejected because it fragments the user experience

### Consequences

**Positive:**
- Zero learning curve for existing users
- Consistent command interface
- Simpler documentation

**Negative:**
- Cannot force a specific parser if auto-detection fails
- Slightly more complex internal logic

---

## Decision 2: Session Listing Behavior

**Date**: 2026-01-05
**Status**: accepted

### Context

The `apsis --list` command currently shows only Claude Code sessions. With Codex support, we need to decide whether to show Codex sessions in the same listing.

### Decision

Show both Claude Code and Codex sessions in a unified listing, with source indicators.

### Rationale

A unified view gives users a complete picture of all their sessions. Source indicators allow users to distinguish between formats when needed.

### Alternatives Considered

- **Claude only**: Keep listing unchanged - Rejected because it would make Codex sessions harder to discover
- **Add --source filter**: Filter by source - Rejected for v1, can be added later if needed

### Consequences

**Positive:**
- Complete visibility of all sessions
- Easier session discovery
- Consistent with transparent operation principle

**Negative:**
- Potentially longer lists for users with many sessions
- No filtering option in v1

---

## Decision 3: Error Handling Strategy

**Date**: 2026-01-05
**Status**: accepted

### Context

Codex JSONL files may contain malformed lines or unexpected content. We need to decide how strictly to handle parsing errors.

### Decision

Warn and continue: Log warnings for malformed lines but continue parsing the rest of the file.

### Rationale

This matches the existing Claude Code parser behavior and provides the best user experience. Partial transcripts are more useful than complete failures.

### Alternatives Considered

- **Fail on first error**: Stop parsing on any malformed line - Rejected because it would make partial transcripts inaccessible
- **Strict mode option**: Add --strict flag - Rejected for v1, adds complexity without clear benefit

### Consequences

**Positive:**
- Resilient to minor file corruption
- Partial transcripts still viewable
- Consistent with Claude Code parser behavior

**Negative:**
- Malformed data may go unnoticed if warnings aren't checked
- Could hide systematic parsing issues

---

## Decision 4: Reasoning Block Rendering

**Date**: 2026-01-05
**Status**: accepted

### Context

Codex logs include reasoning entries with both plaintext summaries and encrypted content. We need to decide what to display.

### Decision

Render only the summary text from reasoning blocks. Skip encrypted content entirely.

### Rationale

The summary provides useful context about the model's reasoning. Encrypted content cannot be decrypted and would appear as noise in the output.

### Alternatives Considered

- **Show placeholder**: Display "[encrypted reasoning]" - Rejected because it adds no value
- **Skip entirely**: Don't render reasoning at all - Rejected because summaries contain useful information

### Consequences

**Positive:**
- Clean, readable transcripts
- Useful reasoning context preserved
- No meaningless encrypted data in output

**Negative:**
- Full reasoning details not available (but they're encrypted anyway)

---

## Decision 5: Metadata Event Filtering

**Date**: 2026-01-05
**Status**: accepted

### Context

Codex logs contain metadata events (session_meta, turn_context, token_count, ghost_snapshot) that don't represent conversation content.

### Decision

Skip all metadata events during transcript rendering.

### Rationale

These events are operational metadata that don't contribute to understanding the conversation. Including them would clutter transcripts without adding value.

### Alternatives Considered

- **Show token counts**: Display usage statistics - Rejected for v1, could be added as option later
- **Show session metadata**: Display in header - Rejected for v1, could be added as option later

### Consequences

**Positive:**
- Clean, focused transcripts
- Consistent with Claude Code transcript style
- Smaller output files

**Negative:**
- No token usage or session metadata visibility

---

## Decision 6: Implementation Scope

**Date**: 2026-01-05
**Status**: accepted

### Context

Codex support could be added to both Apsis (transcript conversion) and Orbit (run orchestration, web interface).

### Decision

Implement Codex support in Apsis only for v1. Orbit integration is deferred to future work.

### Rationale

Apsis provides immediate value for viewing Codex transcripts. Orbit integration would require additional work on registry format, web templates, and orchestration logic. Starting with Apsis validates the parsing implementation first.

### Alternatives Considered

- **Full Orbit + Apsis**: Implement everywhere - Rejected because it increases scope and risk
- **Orbit only**: Just web interface - Rejected because Apsis is the primary use case

### Consequences

**Positive:**
- Reduced implementation scope
- Faster delivery of core functionality
- Lower risk

**Negative:**
- Codex runs won't appear in Orbit web interface
- Cannot orchestrate Codex sessions

---

## Decision 7: Session Discovery Priority

**Date**: 2026-01-05
**Status**: accepted

### Context

When resolving a session ID, it could potentially match files in both Claude Code and Codex locations. We need to define priority.

### Decision

Check Claude Code locations first, then Codex. If found in both, use Claude Code.

### Rationale

Claude Code is the original and primary use case. Prioritizing it maintains backward compatibility and predictable behavior for existing users.

### Alternatives Considered

- **Most recent**: Use whichever file is newer - Rejected because it could be surprising and non-deterministic
- **Codex first**: Check Codex before Claude - Rejected because it changes behavior for existing users

### Consequences

**Positive:**
- Backward compatible
- Predictable behavior
- Existing users unaffected

**Negative:**
- Codex sessions with conflicting IDs require full path

---

## Decision 8: Tool Name Display

**Date**: 2026-01-05
**Status**: accepted

### Context

Codex uses different tool names than Claude Code (e.g., `shell_command` vs `Bash`). During requirements review, reviewers questioned whether mapping Codex tool names to Claude equivalents would confuse users familiar with Codex.

### Decision

Display Codex tool names exactly as they appear in the logs, without mapping to Claude equivalents.

### Rationale

Authentic display preserves the Codex CLI experience. Users of Codex sessions expect to see Codex terminology. Mapping to Claude names could cause confusion when comparing transcripts to actual Codex CLI output.

### Alternatives Considered

- **Map to Claude names**: Display `shell_command` as `Bash` - Rejected because it could confuse Codex users and misrepresents the source
- **Show both**: Display as `Bash (shell_command)` - Rejected as unnecessarily verbose

### Consequences

**Positive:**
- Authentic representation of Codex sessions
- No confusion when comparing to CLI output
- Simpler implementation (no mapping table)

**Negative:**
- Transcripts look different from Claude Code transcripts
- Users must learn Codex tool names

---

## Decision 9: Format Detection Robustness

**Date**: 2026-01-05
**Status**: accepted

### Context

During requirements review, critics identified that detecting format from only the first line is brittle. Edge cases include empty files, files with BOM markers, and incomplete final lines.

### Decision

Detect format from first non-empty line. Handle empty files, BOM markers, and incomplete final lines with specific error handling.

### Rationale

More robust detection reduces user frustration from cryptic errors. Specifying exact error messages for each edge case makes behavior predictable and testable.

### Alternatives Considered

- **Scan first N lines**: Try multiple lines before giving up - Rejected as unnecessary complexity; first non-empty line is sufficient
- **File extension detection**: Use filename patterns - Rejected because it's unreliable and both formats use .jsonl

### Consequences

**Positive:**
- Handles common edge cases gracefully
- Clear error messages for each failure mode
- Predictable, testable behavior

**Negative:**
- Slightly more complex detection logic
- Cannot recover from truly malformed files

---
