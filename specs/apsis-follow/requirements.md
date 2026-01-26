# Apsis Follow Mode Requirements

## Introduction

This feature adds a follow mode to the apsis CLI tool, similar to `tail -f`. When enabled via the `-F` or `--follow` flag, apsis continuously monitors a JSONL transcript file and outputs new entries as they are appended. This allows users to watch Claude Code, Codex, Kiro, or Copilot sessions in real-time as they progress.

The feature uses poll-based file monitoring (checking every 500ms) for cross-platform compatibility. It uses a rolling cache of recently seen entries to detect and render only new content, avoiding complex offset tracking. Only markdown output to stdout is supported in follow mode.

---

## Requirements

### 1. Follow Mode Flag

**User Story:** As a user, I want to enable follow mode with a command-line flag, so that I can continuously monitor a transcript file for new entries.

**Acceptance Criteria:**

1. <a name="1.1"></a>The system SHALL accept `-F` as a short flag to enable follow mode
2. <a name="1.2"></a>The system SHALL accept `--follow` as a long flag to enable follow mode
3. <a name="1.3"></a>WHEN follow mode is enabled, the system SHALL continuously monitor the input file for changes
4. <a name="1.4"></a>The system SHALL NOT conflict with the existing `-f/--format` flag

### 2. Input Source Support

**User Story:** As a user, I want follow mode to work with both file paths and session IDs, so that I can use whichever input method is most convenient.

**Acceptance Criteria:**

1. <a name="2.1"></a>WHEN a file path is provided with follow mode, the system SHALL monitor that file directly
2. <a name="2.2"></a>WHEN a session ID is provided with follow mode, the system SHALL resolve it to the corresponding file path and monitor that file
3. <a name="2.3"></a>WHEN stdin input is used with follow mode, the system SHALL display an error message explaining that stdin cannot be followed
4. <a name="2.4"></a>The error message for stdin SHALL clearly state: "Cannot follow stdin input. Please provide a file path or session ID."

### 3. File Monitoring

**User Story:** As a user, I want the system to efficiently detect new content in the transcript file, so that I see updates promptly without excessive resource usage.

**Acceptance Criteria:**

1. <a name="3.1"></a>The system SHALL use poll-based monitoring to detect file changes
2. <a name="3.2"></a>The system SHALL check for file changes every 500 milliseconds
3. <a name="3.3"></a>The system SHALL detect changes by comparing file modification time (mtime)
4. <a name="3.4"></a>WHEN no change in mtime is detected, the system SHALL skip parsing for that poll cycle
5. <a name="3.5"></a>WHEN the source file is truncated (size decreases), the system SHALL clear its entry cache and re-render from the beginning
6. <a name="3.6"></a>The system SHALL track the file's inode to detect file replacement (e.g., agent crash/restart creating new file at same path)
7. <a name="3.7"></a>WHEN the file's inode changes, the system SHALL clear its entry cache and re-render from the beginning
8. <a name="3.8"></a>The 500ms interval is chosen to balance responsiveness with minimal CPU overhead (~0.2% for stat calls)

### 4. Incremental Output

**User Story:** As a user, I want to see only new transcript entries as they arrive, so that I can follow the session progress without seeing duplicate content.

**Acceptance Criteria:**

1. <a name="4.1"></a>WHEN follow mode starts, the system SHALL parse and render all existing entries in the file
2. <a name="4.2"></a>WHEN new entries are appended to the file, the system SHALL render only the new entries
3. <a name="4.3"></a>The system SHALL NOT re-render previously output entries
4. <a name="4.4"></a>The system SHALL identify entries by computing a hash of the raw JSON line content (SHA-256 truncated to 16 bytes for memory efficiency)
5. <a name="4.5"></a>The system SHALL maintain a set of seen entry hashes to track what has been rendered
6. <a name="4.6"></a>On each poll cycle, the system SHALL parse the entire file but only render entries whose hash is not in the seen set
7. <a name="4.7"></a>The system SHALL accumulate renderer state (tool metadata, grouping context) across all parsed entries to ensure correct rendering of new entries that reference earlier content
8. <a name="4.8"></a>The system SHALL flush stdout after rendering each batch of new entries to ensure output is visible when piped

### 5. Output and Format Restrictions

**User Story:** As a user, I want clear feedback when I use incompatible options, so that I understand why certain combinations don't work.

**Acceptance Criteria:**

1. <a name="5.1"></a>WHEN follow mode is enabled, the system SHALL only output to stdout
2. <a name="5.2"></a>WHEN follow mode is enabled with `-o/--output` flag, the system SHALL display an error and exit
3. <a name="5.3"></a>The output conflict error message SHALL state: "Cannot use --output with --follow. Follow mode only supports stdout."
4. <a name="5.4"></a>WHEN follow mode is enabled, the system SHALL only support markdown output format
5. <a name="5.5"></a>WHEN follow mode is enabled with HTML format (`-f html`), the system SHALL display an error and exit
6. <a name="5.6"></a>The HTML error message SHALL state: "HTML output is not supported in follow mode. Use markdown format instead."
7. <a name="5.7"></a>WHEN follow mode is enabled without explicit format, the system SHALL default to markdown

### 6. Termination Handling

**User Story:** As a user, I want to stop follow mode gracefully with a standard interrupt signal, so that I can exit cleanly when done monitoring.

**Acceptance Criteria:**

1. <a name="6.1"></a>WHEN the user sends an interrupt signal (Ctrl+C / SIGINT), the system SHALL stop monitoring and exit
2. <a name="6.2"></a>The system SHALL complete any in-progress write to stdout before exiting
3. <a name="6.3"></a>WHEN interrupted via SIGINT, the system SHALL exit with status code 130 (128 + SIGINT signal number 2), following Unix convention
4. <a name="6.4"></a>The system SHALL register a signal handler to ensure clean shutdown

### 7. Error Handling

**User Story:** As a user, I want clear error messages when something goes wrong, so that I can understand and resolve issues.

**Acceptance Criteria:**

1. <a name="7.1"></a>WHEN the monitored file does not exist at startup, the system SHALL display an error and exit
2. <a name="7.2"></a>WHEN the monitored file becomes unreadable during follow mode, the system SHALL display an error and exit
3. <a name="7.3"></a>WHEN the session ID cannot be resolved to a file, the system SHALL display an error and exit
4. <a name="7.4"></a>All error messages SHALL be written to stderr
5. <a name="7.5"></a>WHEN the file contains an incomplete JSON line at the end (partial write caught mid-poll), the system SHALL skip that line and retry on the next poll cycle
6. <a name="7.6"></a>The system SHALL NOT treat incomplete trailing JSON as a fatal error
