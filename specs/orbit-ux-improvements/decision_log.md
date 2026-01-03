# Decision Log: Orbit UX Improvements

## Decision 1: Use OSC 8 Terminal Hyperlinks for Completion Links

**Date**: 2026-01-03
**Status**: accepted

### Context

Upon completion of an orbit run, users need to access the generated log index files. The feature requires displaying links to these files in a way that enables quick access.

### Decision

Use OSC 8 terminal hyperlinks (the escape sequence format for clickable links in modern terminals) with the `file://` URI scheme.

### Rationale

OSC 8 hyperlinks are supported by modern terminals (iTerm2, Windows Terminal, GNOME Terminal, etc.) and allow users to simply click to open files. This provides a better UX than requiring users to manually copy paths and paste them into a file manager or browser.

### Alternatives Considered

- **Plain file paths**: Simple to implement - Rejected because it requires manual copy/paste, which is slower
- **Both formats (hybrid)**: Show clickable + plain - Rejected as unnecessary complexity; OSC 8 degrades gracefully to plain text in unsupported terminals

### Consequences

**Positive:**
- One-click access to log files in supported terminals
- Graceful degradation to plain text in older terminals
- No additional dependencies needed

**Negative:**
- Users in terminals without OSC 8 support won't see the visual distinction of a link
- Testing requires verifying behavior across multiple terminal emulators

---

## Decision 2: Use Animated Spinner for Progress Indication

**Date**: 2026-01-03
**Status**: accepted

### Context

Orbit runs can take significant time (minutes to hours) while Claude processes phases. Users need visual feedback that work is in progress.

### Decision

Implement an animated spinner with status information (phase number, elapsed time) that displays alongside Claude's streaming output.

### Rationale

A spinner is appropriate for tasks of unknown duration. Unlike a progress bar (which implies known progress), a spinner clearly indicates "work in progress" without misleading users about completion percentage. Showing elapsed time helps users gauge how long the phase has been running.

### Alternatives Considered

- **Phase progress bar (X of Y phases)**: Shows high-level progress - Rejected because it doesn't provide feedback during long single phases
- **Simple static status line**: "Running phase 3..." - Rejected because it doesn't provide visual confirmation that the process is still active
- **Spinner only (no status)**: Minimal approach - Rejected because phase number and elapsed time are valuable context

### Consequences

**Positive:**
- Clear visual indication that work is in progress
- Users can see which phase is running and how long it's taken
- Does not mislead about completion percentage

**Negative:**
- Requires careful handling to not interfere with Claude's output
- Animation requires managing terminal state (cursor position, line clearing)

---

## Decision 3: No Configuration Options for UX Enhancements

**Date**: 2026-01-03
**Status**: accepted

### Context

These UX enhancements could potentially be made optional via flags or config settings.

### Decision

The UX enhancements will always be active (when appropriate conditions are met, e.g., TTY detected). No flags like `--quiet` or `--no-progress` will be added.

### Rationale

These enhancements are universally beneficial and don't interfere with functionality. Adding configuration options increases complexity without clear benefit. The system already has appropriate guards (dry-run mode, non-TTY detection) that disable the features when they're not appropriate.

### Alternatives Considered

- **Add --quiet flag**: Disable all extra output - Rejected because it adds configuration burden without clear use case
- **Add --no-progress flag**: Specifically disable spinner - Rejected for same reason

### Consequences

**Positive:**
- Simpler implementation
- No configuration documentation needed
- Consistent user experience

**Negative:**
- Users who don't want these features cannot disable them (mitigated by automatic TTY detection)

---

## Decision 4: Display Both Markdown and HTML Index Links

**Date**: 2026-01-03
**Status**: accepted

### Context

Orbit generates both index.md and index.html files. The completion message needs to decide which links to show.

### Decision

Display links to both index.md and index.html files, each on its own labeled line.

### Rationale

Different users have different preferences. Some prefer Markdown (opens in text editors, GitHub renders it), while others prefer HTML (better formatting in browsers, standalone viewing). Showing both gives users immediate choice without adding configuration.

### Alternatives Considered

- **HTML only**: More user-friendly format - Rejected because some users prefer Markdown for its editability
- **Config option**: Let users choose default - Rejected as over-engineering for two small links

### Consequences

**Positive:**
- Users can choose their preferred format immediately
- No configuration needed
- Both formats are equally accessible

**Negative:**
- Two lines of output instead of one (minimal cost)

---

## Decision 5: Spinner Runs Concurrently Without Output Coordination

**Date**: 2026-01-03
**Status**: accepted

### Context

Initial review raised concerns about how a spinner could run "without interfering" with Claude's streaming output to the terminal. This seemed to require complex coordination between concurrent output streams.

### Decision

The spinner runs in a goroutine concurrently with the Claude subprocess. No special coordination is needed because Claude's output is captured to buffers, not streamed to the terminal.

### Rationale

Examining the existing code in `internal/claude/client.go` shows that `cmd.Stdout` and `cmd.Stderr` are set to `bytes.Buffer` instances. The `cmd.Run()` call is blocking, and all Claude output is captured into these buffers for later parsing. The user sees nothing from Claude during phase execution, so the spinner can update freely without any interference.

### Alternatives Considered

- **Pause spinner during output**: Not needed - there is no visible output to interfere with
- **Complex terminal coordination**: Overkill - the buffered output design means this is unnecessary

### Consequences

**Positive:**
- Simple implementation - just start/stop a goroutine
- No terminal coordination complexity
- Spinner provides continuous feedback during the otherwise silent execution

**Negative:**
- None identified - this is a straightforward case

---

## Decision 6: Use briandowns/spinner Library

**Date**: 2026-01-03
**Status**: accepted

### Context

The spinner implementation requires animation, terminal handling, cursor management, and cross-platform support. This could be implemented from scratch or using an existing library.

### Decision

Use the `github.com/briandowns/spinner` library for spinner functionality.

### Rationale

The briandowns/spinner library is well-established (2k+ GitHub stars), provides 90+ character sets, handles cursor hiding/restoration, writes to configurable io.Writer, and has cross-platform support. Using it reduces implementation effort and leverages tested terminal handling code.

### Alternatives Considered

- **Custom implementation**: Full control, no dependency - Rejected because it requires implementing terminal handling, cursor control, and signal cleanup from scratch
- **go-output progress**: Already a dependency - Rejected because its Progress type is designed for known-total progress bars, not indeterminate spinners

### Consequences

**Positive:**
- Reduced implementation complexity
- Tested cross-platform terminal handling
- Multiple character set options
- Built-in color support if desired later

**Negative:**
- Adds one new external dependency

---

## Decision 7: Use signal.NotifyContext for Graceful Shutdown

**Date**: 2026-01-03
**Status**: accepted

### Context

The design originally proposed calling `os.Exit(1)` directly in a signal handler to stop the spinner on SIGINT/SIGTERM. Review feedback identified that this skips deferred cleanup and prevents index links from being printed on interrupt.

### Decision

Use `signal.NotifyContext` to create a cancellable context. The main loop checks for context cancellation and exits gracefully through the normal error path, which allows cleanup and index link printing.

### Rationale

Context-based signal handling is idiomatic Go and preserves all cleanup behavior including deferred functions, log flushing, and index link printing. This provides a better user experience when interrupting a run.

### Alternatives Considered

- **Direct os.Exit(1)**: Simple but skips cleanup - Rejected because it prevents index links from being shown
- **Channel-based shutdown**: More manual control - Rejected because signal.NotifyContext is simpler

### Consequences

**Positive:**
- Cleanup runs on interrupt (defers, log saves)
- Index links shown even when interrupted
- Idiomatic Go pattern

**Negative:**
- Slightly more complex initialization

---

## Decision 8: Use mattn/go-isatty for TTY Detection

**Date**: 2026-01-03
**Status**: accepted

### Context

The design needs to detect whether stderr is a TTY to decide whether to show the spinner and format hyperlinks.

### Decision

Use the existing `mattn/go-isatty` package (already an indirect dependency) rather than implementing custom TTY detection.

### Rationale

The package handles platform-specific edge cases (Windows, Cygwin, etc.) and is already in the dependency tree as an indirect dependency. Reusing it reduces code and avoids platform-specific bugs.

### Alternatives Considered

- **Custom implementation with os.Stdout.Fd()**: Direct approach - Rejected because it doesn't handle edge cases well on all platforms

### Consequences

**Positive:**
- Reliable cross-platform TTY detection
- No additional dependency (already indirect)
- Less code to maintain

**Negative:**
- None identified

---
