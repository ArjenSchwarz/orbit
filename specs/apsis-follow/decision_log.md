# Decision Log: Apsis Follow Mode

## Decision 1: Flag Choice for Follow Mode

**Date**: 2025-01-25
**Status**: accepted

### Context

The follow mode needs a command-line flag. The natural choice `-f` is already used by the `--format` flag in apsis. We need an alternative that is intuitive and doesn't conflict with existing flags.

### Decision

Use `-F` (uppercase) as the short flag and `--follow` as the long flag.

### Rationale

This mirrors the convention used by `tail`, where `-f` is follow and `-F` is follow with retry. Since apsis already uses `-f` for format, using `-F` for follow maintains familiarity with Unix conventions while avoiding conflicts.

### Alternatives Considered

- **`-w/--watch`**: Describes the behavior but is less familiar than "follow" terminology
- **`-t/--tail`**: Would imply showing only the end of the file, not the full content plus updates

### Consequences

**Positive:**
- Familiar to users who know `tail -f` / `tail -F`
- No conflict with existing flags
- Lowercase/uppercase distinction is common in Unix tools

**Negative:**
- Users might initially try `-f` and get format behavior instead

---

## Decision 2: Output Update Mode

**Date**: 2025-01-25
**Status**: accepted

### Context

When new entries arrive in follow mode, the system could either append only new entries to the output, or clear the screen and re-render the entire transcript.

### Decision

Use append-only mode where only new entries are rendered and appended to the output.

### Rationale

This matches the behavior of `tail -f` which users expect. It's more efficient (no re-rendering), works well with file output, and doesn't disrupt the user's terminal scroll history.

### Alternatives Considered

- **Clear and rerender**: Would provide better context but is disruptive, doesn't work with file output, and isn't what users expect from "follow" mode
- **User choice via flag**: Adds complexity for minimal benefit

### Consequences

**Positive:**
- Familiar behavior matching `tail -f`
- Efficient - no re-parsing or re-rendering of old content
- Works naturally with file output (append mode)

**Negative:**
- Users lose context of earlier messages as terminal scrolls
- If rendering depends on earlier entries (e.g., tool metadata), may need to track state

---

## Decision 3: HTML Output Support

**Date**: 2025-01-25
**Status**: accepted

### Context

Apsis supports both markdown and HTML output formats. We need to decide whether follow mode should support HTML output.

### Decision

Follow mode supports only markdown output. HTML format is explicitly disallowed with a clear error message.

### Rationale

HTML output would require rewriting the entire file on each update (to maintain valid HTML structure with proper closing tags). This defeats the purpose of streaming/follow mode and would cause issues with browsers trying to render incomplete HTML.

### Alternatives Considered

- **Support both formats**: HTML would need full file rewrite on each update, causing performance issues and potential rendering problems in browsers

### Consequences

**Positive:**
- Simple implementation
- Clear user experience
- Markdown streams naturally to terminal

**Negative:**
- Users who want HTML must wait for session to complete, then convert

---

## Decision 4: Stdin Input Handling

**Date**: 2025-01-25
**Status**: accepted

### Context

Apsis supports reading from stdin (piped input). We need to decide how follow mode should handle stdin.

### Decision

Stdin input with follow mode produces a clear error message and exits.

### Rationale

Stdin is consumed once and then closed - there's nothing to "follow" after the initial read. Attempting to follow stdin would just hang indefinitely with no new data.

### Alternatives Considered

- **Read once, then wait**: Would be confusing - appears to work but never updates

### Consequences

**Positive:**
- Clear feedback to users about what's supported
- No confusing hanging behavior

**Negative:**
- Users who pipe to apsis must use a different workflow for live monitoring

---

## Decision 5: File Monitoring Method

**Date**: 2025-01-25
**Status**: accepted

### Context

There are two main approaches to detect file changes: poll-based (periodically check file stats) and event-based (fsnotify/inotify).

### Decision

Use poll-based monitoring, checking file size/modification time every 500 milliseconds.

### Rationale

Poll-based is simpler to implement, has consistent behavior across platforms, and handles edge cases like atomic writes (temp file → rename) that can break fsnotify watches. The 500ms interval provides good responsiveness while keeping CPU usage minimal.

### Alternatives Considered

- **fsnotify events**: More efficient for long idle periods but has platform-specific quirks, atomic write issues, and adds complexity. The benefits don't justify the complexity for this use case.

### Consequences

**Positive:**
- Simple implementation (~20 lines)
- No platform-specific edge cases
- Handles atomic writes gracefully
- Easy to test

**Negative:**
- Small latency (up to 500ms) between write and display
- CPU wakes even when file is unchanged (minimal impact at 500ms)

---

## Decision 6: Poll Interval Configuration

**Date**: 2025-01-25
**Status**: accepted

### Context

The poll interval affects responsiveness vs resource usage. We could make it configurable or use a fixed value.

### Decision

Use a fixed 500ms poll interval with no configuration option.

### Rationale

500ms is fast enough that users won't notice the delay (~0.2% CPU overhead for stat calls), but slow enough to have negligible resource impact. Adding a configuration flag adds complexity without significant benefit.

### Alternatives Considered

- **Configurable via `--interval` flag**: Adds complexity for edge case users who need different timing
- **100ms interval**: More responsive but higher CPU usage with no perceptible benefit
- **1 second interval**: Lower CPU but noticeably laggy for rapid output

### Consequences

**Positive:**
- Simpler implementation and CLI
- Reasonable default for all use cases

**Negative:**
- Users with specific needs cannot tune the interval

---

## Decision 7: Termination Behavior

**Date**: 2025-01-25
**Status**: accepted

### Context

Follow mode runs indefinitely until stopped. We need to decide how it should be terminated and what exit code to use.

### Decision

Terminate only via Ctrl+C (SIGINT), exiting with status code 130 (128 + signal number 2).

### Rationale

Exit code 130 follows Unix convention for processes terminated by SIGINT. This allows scripts and tooling to distinguish between normal completion, errors, and signal-based termination.

### Alternatives Considered

- **Exit code 0**: Would indicate success, but SIGINT termination is technically an interruption, not successful completion
- **Also exit on file deletion**: Adds complexity and could cause unexpected exits if the file is briefly unavailable

### Consequences

**Positive:**
- Follows Unix conventions
- Scripts can detect signal-based termination
- Simple implementation

**Negative:**
- If the file is deleted, follow mode will error on next read rather than exiting gracefully

---

## Decision 8: Entry Tracking via Rolling Cache

**Date**: 2025-01-25
**Status**: accepted

### Context

To render only new entries, we need to track what has already been output. Options include byte offset tracking, line counting, or caching seen entries.

### Decision

Use a rolling cache of entry identifiers (content hash or unique message ID) to track what has been rendered. On each poll, parse the entire file but only render entries not in the cache.

### Rationale

This approach is simpler and more robust than offset tracking:
- Handles file truncation naturally (cache miss = re-render)
- No need to handle partial lines or multi-line JSON
- Works correctly if file is rewritten (not just appended)
- Avoids complex bookkeeping of byte positions

### Alternatives Considered

- **Byte offset tracking**: Track position in file and only read new bytes. Fails when file is truncated or rewritten. Requires handling partial JSON lines at boundaries.
- **Line counting**: Count lines processed and skip that many. Fails with multi-line JSON entries. Doesn't handle truncation.

### Consequences

**Positive:**
- Simple, robust implementation
- Handles truncation and rewrite scenarios
- No partial line handling needed at read boundaries

**Negative:**
- Re-parses entire file each poll (acceptable for typical session sizes)
- Memory grows with session length (mitigated by storing only identifiers, not content)

---

## Decision 9: No File Output in Follow Mode

**Date**: 2025-01-25
**Status**: accepted

### Context

Follow mode could potentially support writing to an output file (`-o` flag), but this creates complexity around append vs truncate, existing file handling, and atomic writes.

### Decision

Follow mode only outputs to stdout. The `-o/--output` flag conflicts with `--follow` and produces an error.

### Rationale

File output with follow mode creates ambiguous scenarios:
- If output file exists, should it be truncated or appended?
- Append mode would create duplicates if apsis is restarted
- File writes need flushing/syncing for visibility
- Users can achieve the same result with shell redirection: `apsis -F session >> output.md`

### Alternatives Considered

- **Support file output with append mode**: Requires handling existing file content, flushing, and creates confusing duplicate scenarios on restart

### Consequences

**Positive:**
- Simpler implementation
- Clear, predictable behavior
- Users can use shell redirection if needed

**Negative:**
- Less convenient for users who want to log to a file

---

## Decision 10: Incomplete JSON Handling

**Date**: 2025-01-25
**Status**: accepted

### Context

When polling a file being actively written, we may catch a partial JSON line (write in progress). We need to handle this gracefully.

### Decision

Skip incomplete JSON lines at the end of the file and retry on the next poll cycle. Do not treat this as a fatal error.

### Rationale

Partial writes are a normal occurrence when following an active file. The incomplete line will be complete on the next poll (500ms later). Erroring out would make follow mode unusable for its primary purpose.

### Alternatives Considered

- **Buffer incomplete lines**: Adds complexity for minimal benefit since next poll is only 500ms away
- **Error and exit**: Would make follow mode unusable for active sessions

### Consequences

**Positive:**
- Robust handling of active file writes
- Simple implementation (just catch JSON parse error on last line)

**Negative:**
- Slight delay (up to 500ms) for entries caught mid-write

---

## Decision 11: File Change Detection via mtime

**Date**: 2025-01-25
**Status**: accepted

### Context

To avoid unnecessary parsing, we should detect whether the file has changed before re-parsing. Options include checking file size, modification time (mtime), or both.

### Decision

Use file modification time (mtime) to detect changes.

### Rationale

Modification time is updated whenever the file is written, making it reliable for append-only JSONL files. File size could theoretically miss same-length overwrites, though this is unlikely for JSONL.

### Alternatives Considered

- **File size only**: Could miss same-length content changes (unlikely but possible)
- **Both size and mtime**: Redundant; mtime alone is sufficient
- **Content hash**: Too expensive to compute on every poll

### Consequences

**Positive:**
- Single stat call per poll cycle
- Reliable for append-only files

**Negative:**
- Depends on filesystem timestamp granularity (typically 1 second on older filesystems, but 500ms polling is fine since we re-check on next cycle)

---

## Decision 12: Renderer State Accumulation

**Date**: 2025-01-25
**Status**: accepted

### Context

The markdown renderer has cross-entry dependencies (tool metadata maps, read/edit grouping). New entries may reference tools or context defined in earlier entries that were already rendered.

### Decision

Parse and accumulate renderer state from all entries on each poll, but only render entries not in the seen-entry cache.

### Rationale

This ensures the renderer has full context to correctly format new entries that reference earlier content. Since we're parsing the full file anyway (for the cache comparison), accumulating state adds minimal overhead.

### Alternatives Considered

- **Persist state between polls**: Would require serializing renderer state, adding complexity
- **Render without context**: Would produce incorrect output for entries referencing earlier tools

### Consequences

**Positive:**
- Correct rendering of context-dependent entries
- No state persistence needed between polls

**Negative:**
- Full parse on each poll (acceptable for typical session sizes)

---

## Decision 13: Entry Identification via Content Hash

**Date**: 2025-01-25
**Status**: accepted

### Context

To track which entries have been rendered, we need a unique identifier for each entry. The Entry struct has an optional UUID field, but it's not always present.

### Decision

Use SHA-256 hash of the raw JSON line content, truncated to 16 bytes, as the entry identifier for the seen-entry cache.

### Rationale

Content hashing provides a reliable identifier regardless of whether the entry has a UUID. Truncating to 16 bytes (128 bits) provides sufficient collision resistance while minimizing memory usage for long sessions.

### Alternatives Considered

- **Use UUID field**: Not always present, would require fallback anyway
- **Full SHA-256**: 32 bytes per entry; unnecessary for collision resistance at typical session sizes
- **Line number**: Not stable across file modifications

### Consequences

**Positive:**
- Works for all entry types regardless of UUID presence
- Memory-efficient (16 bytes per entry)
- Collision probability negligible for realistic session sizes

**Negative:**
- Requires hashing each line on every poll (minimal overhead)

---

## Decision 14: Inode Tracking for File Replacement

**Date**: 2025-01-25
**Status**: accepted

### Context

When an agent crashes and restarts, it may create a new file at the same path. This results in a different inode but same filename. The follow mode needs to detect this scenario.

### Decision

Track the file's inode alongside mtime. When the inode changes, treat it as a new file: clear the entry cache and re-render from the beginning.

### Rationale

Inode change indicates the file was replaced (not just modified). This commonly happens when agents crash/restart or when files are atomically replaced (write temp → rename). Continuing with stale state would produce incorrect output.

### Alternatives Considered

- **Ignore file replacement**: Would cause confusion when following restarted sessions
- **Track filename only**: Doesn't detect replacement, only deletion

### Consequences

**Positive:**
- Correct behavior when agent restarts mid-session
- Handles atomic file replacement patterns

**Negative:**
- Requires platform-specific code to access inode (syscall.Stat_t.Ino on Unix)
- May need different approach on Windows

---

## Decision 15: Explicit Stdout Flushing

**Date**: 2025-01-25
**Status**: accepted

### Context

When stdout is piped to another process (e.g., `apsis -F session | grep error`), Go's bufio may buffer output, causing delays in visibility.

### Decision

Explicitly flush stdout after rendering each batch of new entries.

### Rationale

Users expect to see output immediately in follow mode. Buffering defeats the purpose of real-time monitoring. Flushing after each batch ensures output is visible even when piped.

### Alternatives Considered

- **Line buffering**: Go doesn't have native line buffering for stdout
- **No explicit flushing**: Would cause unpredictable delays when piped

### Consequences

**Positive:**
- Immediate output visibility regardless of piping
- Predictable behavior

**Negative:**
- Slightly more syscalls (negligible at 500ms poll rate)

---
