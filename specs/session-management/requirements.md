# Session Management Requirements

## Introduction

This feature enhances Orbit's log storage and Claude session handling with three key improvements:

1. **Flat Log Directory**: By default, logs are stored directly in `.orbit/` next to the tasks file instead of timestamped subdirectories. This simplifies log management and enables session continuation across runs.

2. **Session ID Generation**: Orbit generates a UUID before starting each Claude phase and passes it via `--session-id`, allowing the session to be resumed if interrupted.

3. **Session Continuation**: When resuming an unfinished phase, Orbit uses Claude's `--resume` flag with the stored session ID to continue the existing conversation, preserving context and avoiding duplicate work.

---

## Requirements

### 1. Flat Log Directory Storage

**User Story:** As a developer, I want logs stored in a flat `.orbit/` directory by default, so that I can easily find and manage session data without navigating timestamped subdirectories.

**Acceptance Criteria:**

1. <a name="1.1"></a>The system SHALL store all log files directly in the `.orbit/` directory located next to the tasks file by default
2. <a name="1.2"></a>The system SHALL use run-numbered filenames (e.g., `phase-1-run-2-session.json`) WHEN the same phase is executed multiple times
3. <a name="1.3"></a>The system SHALL maintain a single `summary.json` file that persists across multiple runs
4. <a name="1.4"></a>The system SHALL increment a `run_number` field in the summary WHEN a new orchestration run begins
5. <a name="1.5"></a>The system SHALL provide a `--date-subdirs` flag that, WHEN enabled, creates timestamped subdirectories following the existing `{timestamp}-{branch}` format
6. <a name="1.6"></a>The system SHALL support `date_subdirs` configuration in `.orbit.yaml` files

---

### 2. Session ID Generation and Storage

**User Story:** As a developer, I want Orbit to generate and store session IDs before starting each phase, so that interrupted sessions can be identified and resumed.

**Acceptance Criteria:**

1. <a name="2.1"></a>The system SHALL generate a UUID (v4) before starting each new Claude phase
2. <a name="2.2"></a>The system SHALL pass the generated UUID to Claude via the `--session-id` flag
3. <a name="2.3"></a>The system SHALL store the session ID in `summary.json` as part of a `current_phase` object BEFORE executing the Claude command
4. <a name="2.4"></a>The `current_phase` object SHALL contain the phase number, session ID, and start timestamp
5. <a name="2.5"></a>The system SHALL verify that Claude's returned session ID matches the passed session ID after execution
6. <a name="2.6"></a>The system SHALL update `current_phase.session_id` with Claude's returned ID IF it differs from the passed ID
7. <a name="2.7"></a>The system SHALL clear the `current_phase` object WHEN a phase completes successfully
8. <a name="2.8"></a>The system SHALL preserve the session ID in the `sessions` array after phase completion

---

### 3. Session Continuation

**User Story:** As a developer, I want Orbit to automatically continue interrupted Claude sessions, so that I don't lose progress or context when resuming after an interruption.

**Acceptance Criteria:**

1. <a name="3.1"></a>The system SHALL detect an unfinished phase by checking for a `current_phase` object in `summary.json`
2. <a name="3.2"></a>The system SHALL use `claude --resume <session-id>` syntax (session ID as argument to --resume) WHEN continuing an existing phase AND `--continue-session` is enabled
3. <a name="3.3"></a>The system SHALL use `--session-id <uuid>` WHEN starting a new phase
4. <a name="3.4"></a>The system SHALL provide a `--continue-session` flag that defaults to `true`
5. <a name="3.5"></a>The system SHALL start a fresh session with a new UUID WHEN `--continue-session` is set to `false`
6. <a name="3.6"></a>The system SHALL support `continue_session` configuration in `.orbit.yaml` files
7. <a name="3.7"></a>The system SHALL detect resume failure by: (a) non-zero exit code with "session not found" or "invalid session" error, OR (b) returned session ID not matching the stored session ID
8. <a name="3.8"></a>The system SHALL automatically start a fresh session with a new UUID IF session resumption fails
9. <a name="3.9"></a>The system SHALL log a warning message WHEN falling back to a fresh session after a failed resume attempt
10. <a name="3.10"></a>The system SHALL use `--resume` with the existing session ID WHEN retrying after a transient failure (rate limit, network error) within the same phase
11. <a name="3.11"></a>The system SHALL generate a new session ID and use `--session-id` IF a retry fails due to session-invalid errors

---

### 4. Configuration Priority

**User Story:** As a developer, I want configuration options to follow a consistent priority order, so that I can override defaults at different levels as needed.

**Acceptance Criteria:**

1. <a name="4.1"></a>The system SHALL apply configuration in priority order: CLI flags > environment variables > project `.orbit.yaml` > home `~/.orbit.yaml` > defaults
2. <a name="4.2"></a>The system SHALL default `date_subdirs` to `false`
3. <a name="4.3"></a>The system SHALL default `continue_session` to `true`

---

## Non-Functional Requirements

### 5. Reliability

**User Story:** As a developer, I want Orbit to handle edge cases gracefully, so that orchestration continues even when unexpected situations occur.

**Acceptance Criteria:**

1. <a name="5.1"></a>The system SHALL persist `current_phase` to disk before invoking Claude, so that crash recovery is possible
2. <a name="5.2"></a>The system SHALL handle the case WHERE `summary.json` exists but is malformed by starting a fresh run
3. <a name="5.3"></a>The system SHALL handle the case WHERE `current_phase` references a session that no longer exists by starting a fresh session

---

## Constraints and Limitations

1. <a name="C.1"></a>Concurrent Orbit invocations on the same tasks file are NOT supported. Running multiple Orbit processes simultaneously may corrupt `summary.json`.
2. <a name="C.2"></a>Log file cleanup is out of scope for this feature. Run-numbered files will accumulate over time.
