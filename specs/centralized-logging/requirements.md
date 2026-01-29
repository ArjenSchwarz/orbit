# Centralized Logging Requirements

## Introduction

This feature extends Orbit's existing debug logging infrastructure to write structured logs to a central location (`~/.orbit/logs/`). By extending the existing `debug.Logger`, all current debug call sites automatically gain file logging capability. These logs capture Orbit's internal operations (phase transitions, retries, configuration loading, errors) in structured JSON Lines format, enabling effective debugging of issues during and after runs.

Centralized logging is enabled by default but can be disabled via CLI flags, environment variables, or configuration files following Orbit's standard configuration hierarchy.

**Implementation Note:** This feature requires refactoring the `debug.Logger` API to accept structured data rather than pre-formatted strings. The current `Log(format, args)` signature is incompatible with dual-format output (JSON for files, human-readable for stderr).

## Requirements

### 1. Centralized Log Storage

**User Story:** As an Orbit user, I want all debug logs stored in a central location, so that I can find and analyze logs from any run regardless of which project it was executed in.

**Acceptance Criteria:**

1. <a name="1.1"></a>The system SHALL store centralized logs in `~/.orbit/logs/` directory
2. <a name="1.2"></a>The system SHALL create the `~/.orbit/logs/` directory if it does not exist
3. <a name="1.3"></a>The system SHALL name log files using the pattern `{timestamp}-{run-id}.jsonl` where timestamp is `YYYYMMDD-HHMMSS` format and run-id is a v4 UUID
4. <a name="1.4"></a>The system SHALL create a separate log file for each variant when running multi-variant orchestration
5. <a name="1.5"></a>The system SHALL name variant log files using the pattern `{timestamp}-{run-id}-variant-{N}.jsonl`
6. <a name="1.6"></a>WHEN running multi-variant orchestration, the system SHALL log parent orchestration events (variant creation, parallel execution start, all variants completed) to the main log file `{timestamp}-{run-id}.jsonl`
7. <a name="1.7"></a>The system SHALL serialize concurrent writes to each log file to prevent interleaved or corrupted entries

### 2. Structured Log Format

**User Story:** As a developer debugging an Orbit issue, I want logs in a structured format, so that I can parse and query them programmatically.

**Acceptance Criteria:**

1. <a name="2.1"></a>The system SHALL write logs in JSON Lines format (one JSON object per line)
2. <a name="2.2"></a>Each log entry SHALL contain a `timestamp` field in ISO 8601 format
3. <a name="2.3"></a>Each log entry SHALL contain a `level` field with values: `debug`, `info`, `warn`, or `error`
4. <a name="2.4"></a>Each log entry SHALL contain a `message` field with human-readable text
5. <a name="2.5"></a>Each log entry SHALL contain a `component` field identifying the source (valid values: `orchestrator`, `agent`, `config`, `retry`, `variant`, `registry`)
6. <a name="2.6"></a>Each log entry MAY contain additional structured fields relevant to the event (e.g., `phase`, `agent`, `duration`, `error`)
7. <a name="2.7"></a>The first log entry in each file SHALL contain a `schema_version` field with value `1` to support future format changes
8. <a name="2.8"></a>The system SHALL NOT include redundant `run_id` in each entry since the filename contains the run identifier

### 3. Log Content Coverage

**User Story:** As an Orbit user, I want logs to capture all significant internal operations, so that I can trace what happened during a run.

**Acceptance Criteria:**

1. <a name="3.1"></a>The system SHALL log orchestration start with configuration summary (agent, tasks file, working directory)
2. <a name="3.2"></a>The system SHALL log each phase start with phase number and task count
3. <a name="3.3"></a>The system SHALL log each phase completion with duration and success/failure status
4. <a name="3.4"></a>The system SHALL log agent invocation details (command executed, arguments, working directory)
5. <a name="3.5"></a>The system SHALL log agent completion with exit code, duration, and session ID
6. <a name="3.6"></a>The system SHALL log all retry attempts with attempt number, error classification, and backoff duration
7. <a name="3.7"></a>The system SHALL log configuration loading from each source (CLI, env, project config, home config)
8. <a name="3.8"></a>The system SHALL log all errors with full error message and an `error_chain` array field containing each error message from `errors.Unwrap` iteration
9. <a name="3.9"></a>The system SHALL log orchestration completion with total duration and final status
10. <a name="3.10"></a>The system SHALL log variant creation and cleanup operations when running multi-variant orchestration

### 4. Session Cross-Reference

**User Story:** As a developer debugging an issue, I want centralized logs to reference session transcripts, so that I can navigate from a log entry to the full agent output.

**Acceptance Criteria:**

1. <a name="4.1"></a>WHEN logging agent completion, the system SHALL include a `session_log_path` field with the absolute path to the session transcript file
2. <a name="4.2"></a>WHEN logging phase completion, the system SHALL include a `transcript_path` field with the path to the phase transcript if available
3. <a name="4.3"></a>The system SHALL use absolute paths for all file references to enable navigation from any working directory

### 5. Default Enabled Behavior

**User Story:** As an Orbit user, I want centralized logging enabled by default, so that debug information is available when I need it without prior configuration.

**Acceptance Criteria:**

1. <a name="5.1"></a>The system SHALL enable centralized logging by default when no configuration is specified
2. <a name="5.2"></a>The system SHALL begin logging immediately after configuration is resolved to capture the full run lifecycle
3. <a name="5.3"></a>The system SHALL log a startup entry as the first entry in each log file containing: `schema_version`, `orbit_version`, `agent`, `tasks_file`, `working_directory`, and `branch_name`
4. <a name="5.4"></a>The system SHALL log a shutdown entry as the final entry when orchestration completes normally
5. <a name="5.5"></a>WHEN analyzing a log file, absence of a shutdown entry SHALL indicate the run was interrupted or crashed

### 6. Configuration Options

**User Story:** As an Orbit user, I want to disable centralized logging when I don't need it, so that I can avoid unnecessary disk writes.

**Acceptance Criteria:**

1. <a name="6.1"></a>The system SHALL support a `--centralized-log` CLI flag where `--centralized-log=false` disables centralized logging for a single run
2. <a name="6.2"></a>The system SHALL support an `ORBIT_CENTRALIZED_LOG` environment variable where `false` or `0` disables logging
3. <a name="6.3"></a>The system SHALL support a `centralized-log` boolean key in `.orbit.yaml` configuration files
4. <a name="6.4"></a>The system SHALL follow the standard configuration hierarchy: CLI flags > env vars > project config > home config > defaults
5. <a name="6.5"></a>WHEN centralized logging is disabled, the system SHALL NOT create any log files or directories

### 7. Integration with Debug Logger

**User Story:** As an Orbit developer, I want centralized logging to extend the existing debug infrastructure, so that all debug call sites automatically gain file logging without code duplication.

**Acceptance Criteria:**

1. <a name="7.1"></a>The system SHALL extend the existing `debug.Logger` to support file output in addition to stderr
2. <a name="7.2"></a>The `--debug` flag SHALL control stderr output independently of centralized file logging
3. <a name="7.3"></a>WHEN both `--debug` and centralized logging are enabled, the system SHALL write to both stderr and file
4. <a name="7.4"></a>WHEN only centralized logging is enabled (default), the system SHALL write to file without stderr output
5. <a name="7.5"></a>WHEN only `--debug` is enabled and centralized logging is disabled, the system SHALL write to stderr only
6. <a name="7.6"></a>The system SHALL use structured JSON format for file output while maintaining human-readable format for stderr
7. <a name="7.7"></a>The `debug.Logger` API SHALL be refactored to accept structured data (level, component, message, fields) rather than pre-formatted strings

### 8. Log Location Discoverability

**User Story:** As an Orbit user, I want to know where logs are being written, so that I can find them for debugging.

**Acceptance Criteria:**

1. <a name="8.1"></a>WHEN centralized logging is enabled, the system SHALL output the log file path to stderr at orchestration start
2. <a name="8.2"></a>The log path message SHALL be displayed even when `--debug` is disabled
3. <a name="8.3"></a>The log path message SHALL use the format: `Logging to {path}`

### 9. Error Resilience

**User Story:** As an Orbit user, I want logging failures to not interrupt my runs, so that orchestration continues even if log writing fails.

**Acceptance Criteria:**

1. <a name="9.1"></a>WHEN a log write operation fails, the system SHALL continue orchestration without interruption
2. <a name="9.2"></a>WHEN a log write operation fails, the system SHALL emit a warning to stderr, rate-limited to at most one warning per 10 seconds
3. <a name="9.3"></a>The system SHALL attempt to write subsequent log entries even after a write failure
4. <a name="9.4"></a>WHEN the log directory cannot be created, the system SHALL warn and proceed with centralized logging disabled

### 10. Log File Lifecycle

**User Story:** As an Orbit user, I want log files to persist after runs complete, so that I can analyze them later when debugging issues.

**Acceptance Criteria:**

1. <a name="10.1"></a>The system SHALL keep log files after orchestration completes (successful or failed)
2. <a name="10.2"></a>The system SHALL flush log entries to disk after each write to prevent data loss on crash
3. <a name="10.3"></a>The system SHALL NOT automatically delete or rotate log files
4. <a name="10.4"></a>The system SHALL document the log location (`~/.orbit/logs/`) in user documentation with cleanup instructions
