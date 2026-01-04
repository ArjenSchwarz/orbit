# Requirements: Orbit UX Improvements

## Introduction

This feature enhances the user experience of orbit runs with two improvements:

1. **Completion Links**: Upon successful completion (or failure), display clickable terminal hyperlinks to the generated index files (index.md and index.html), enabling users to quickly access run logs.

2. **Progress Indicator**: Show an animated spinner with status information during phase execution, providing visual feedback that work is in progress alongside Claude's streaming output.

These improvements aim to make orbit runs more informative and user-friendly without requiring additional configuration.

---

## Requirements

### 1. Completion Index Links

**User Story:** As an orbit user, I want to see clickable links to the log index files when a run completes, so that I can quickly open and review the session logs without manually navigating to the log directory.

**Acceptance Criteria:**

1. <a name="1.1"></a>WHEN an orbit run completes successfully, the system SHALL display the absolute file paths to both index.md and index.html
2. <a name="1.2"></a>WHEN an orbit run fails, the system SHALL display the index file links to help users review what happened
3. <a name="1.3"></a>The system SHALL format the links as OSC 8 terminal hyperlinks (escape sequence format `\e]8;;file://path\e\\text\e]8;;\e\\`)
4. <a name="1.4"></a>The system SHALL use the `file://` URI scheme for local file paths to enable browser/editor opening
5. <a name="1.5"></a>The system SHALL display both the Markdown index link and the HTML index link on separate lines
6. <a name="1.6"></a>The system SHALL label each link clearly (e.g., "Markdown:" and "HTML:") so users understand what each link opens
7. <a name="1.7"></a>IF the log manager is nil (e.g., during dry-run mode), THEN the system SHALL NOT attempt to display index links

### 2. Progress Indicator During Execution

**User Story:** As an orbit user, I want to see a visual indicator that a phase is running, so that I know the system is actively working even during long-running Claude sessions.

**Acceptance Criteria:**

1. <a name="2.1"></a>WHEN a phase begins execution, the system SHALL display an animated spinner indicator
2. <a name="2.2"></a>The spinner SHALL show the current phase number (e.g., "Phase 3")
3. <a name="2.3"></a>The spinner SHALL show elapsed time since the phase started, updated at regular intervals
4. <a name="2.4"></a>The spinner animation SHALL update at least every 100 milliseconds to appear smooth
5. <a name="2.5"></a>The spinner SHALL run concurrently with the Claude process (note: Claude's output is captured to buffers, not streamed to the terminal, so no output interference occurs)
6. <a name="2.6"></a>WHEN a phase completes, the system SHALL stop the spinner and clear the status line
7. <a name="2.7"></a>WHEN waiting for retry (rate limit, connection error), the spinner SHALL indicate the wait state and countdown
8. <a name="2.8"></a>IF running in dry-run mode, THEN the system SHALL NOT display a spinner
9. <a name="2.9"></a>WHEN the process receives SIGINT or SIGTERM, the system SHALL stop the spinner and restore terminal state before exiting

### 3. Terminal Compatibility

**User Story:** As an orbit user running in various terminal environments, I want the UX improvements to work gracefully across different terminals, so that I don't experience broken output.

**Acceptance Criteria:**

1. <a name="3.1"></a>The OSC 8 hyperlinks SHALL be rendered correctly in terminals that support them (iTerm2, Windows Terminal, modern VTE-based terminals)
2. <a name="3.2"></a>In terminals that do not support OSC 8, the link text SHALL still be visible as plain text (graceful degradation)
3. <a name="3.3"></a>The spinner SHALL use ASCII characters that render correctly across all terminals
4. <a name="3.4"></a>The system SHALL write spinner output to stderr to avoid interfering with stdout redirection
5. <a name="3.5"></a>IF stdout/stderr is not a TTY (e.g., piped to a file), THEN the spinner SHALL be disabled automatically

### 4. Integration with Existing Output

**User Story:** As an orbit user, I want the new progress display to integrate cleanly with the existing phase overview table and log messages, so that the output remains organized and readable.

**Acceptance Criteria:**

1. <a name="4.1"></a>The spinner status line SHALL be displayed below the phase overview table
2. <a name="4.2"></a>The spinner SHALL NOT duplicate information already shown by existing log messages
3. <a name="4.3"></a>The completion links SHALL be displayed after the "All tasks complete!" message
4. <a name="4.4"></a>The existing verbose mode output SHALL continue to function alongside the spinner
5. <a name="4.5"></a>The spinner SHALL clear its line before any new log messages are printed to prevent visual artifacts

### 5. Demo Command

**User Story:** As an orbit user, I want to preview how the spinner and progress display will look, so that I can verify it works in my terminal and experiment with visual settings.

**Acceptance Criteria:**

1. <a name="5.1"></a>The system SHALL provide an `orbit demo` subcommand that displays the spinner
2. <a name="5.2"></a>The demo SHALL show a simulated phase overview table with sample data
3. <a name="5.3"></a>The demo SHALL run a spinner with simulated phase progression (e.g., "Phase 1", then "Phase 2")
4. <a name="5.4"></a>The demo SHALL simulate a retry wait countdown at least once
5. <a name="5.5"></a>The demo SHALL continue running until the user presses Ctrl+C
6. <a name="5.6"></a>WHEN the user presses Ctrl+C, the demo SHALL display sample completion links
7. <a name="5.7"></a>The demo SHALL NOT require any configuration files or git repository
