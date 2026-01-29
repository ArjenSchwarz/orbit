---
references:
    - specs/centralized-logging/requirements.md
    - specs/centralized-logging/design.md
    - specs/centralized-logging/decision_log.md
---
# Centralized Logging Implementation

## Types and Data Models

- [x] 1. Create LogEntry types in internal/debug/entry.go <!-- id:5dh82t1 -->
  - Define LogEntry struct with Timestamp
  - Level
  - Component
  - Message
  - Fields. Define StartupEntry struct (flat
  - no embedding) with schema_version and metadata fields. Define ShutdownEntry struct (flat) with total_duration and final_status. Define StartupConfig struct for LogStartup() parameter. Requirements: 2.1
  - 2.2
  - 2.3
  - 2.4
  - 2.5
  - 2.6
  - 2.7
  - 5.3
  - Stream: 1

- [x] 2. Write unit tests for LogEntry JSON serialization <!-- id:5dh82t2 -->
  - Test LogEntry produces valid JSON with all required fields. Test StartupEntry produces correct JSON without embedded fields issues. Test ShutdownEntry produces correct JSON. Test Fields omitempty behavior. Requirements: 2.1
  - 2.2
  - 2.3
  - 2.4
  - 2.5
  - Blocked-by: 5dh82t1 (Create LogEntry types in internal/debug/entry.go)
  - Stream: 1

## FileWriter Implementation

- [ ] 3. Create FileWriter in internal/debug/writer.go <!-- id:5dh82t3 -->
  - Implement FileWriter struct with file
  - mutex
  - path
  - lastWarningTime
  - warningInterval
  - closed fields. Implement NewFileWriter(runID) that creates ~/.orbit/logs/ directory and opens file with 0600 permissions. Return error if runID is empty
  - return nil writer (not error) on directory/file creation failure. Generate filename pattern: {timestamp}-{runID}.jsonl. Requirements: 1.1
  - 1.2
  - 1.3
  - 9.4
  - Blocked-by: 5dh82t1 (Create LogEntry types in internal/debug/entry.go)
  - Stream: 1

- [ ] 4. Implement NewVariantFileWriter <!-- id:5dh82t4 -->
  - Implement NewVariantFileWriter(runID
  - variantNum) for variant-specific log files. Generate filename pattern: {timestamp}-{runID}-variant-{N}.jsonl. Validate variantNum >= 1. Requirements: 1.4
  - 1.5
  - Blocked-by: 5dh82t3 (Create FileWriter in internal/debug/writer.go)
  - Stream: 1

- [ ] 5. Implement thread-safe Write method with rate-limited warnings <!-- id:5dh82t5 -->
  - Implement Write(entry) with mutex protection. Check closed inside mutex to avoid data race. Emit warnings outside mutex to avoid deadlock risk. Implement checkWarningLocked() for rate limiting (10 second interval). Implement Sync() after each write for durability. Requirements: 1.7
  - 9.1
  - 9.2
  - 9.3
  - 10.2
  - Blocked-by: 5dh82t3 (Create FileWriter in internal/debug/writer.go)
  - Stream: 1

- [ ] 6. Implement Path() and Close() methods <!-- id:5dh82t6 -->
  - Implement Path() with nil-safety returning empty string if writer is nil. Implement Close() with nil-safety. Requirements: 10.1
  - Blocked-by: 5dh82t3 (Create FileWriter in internal/debug/writer.go)
  - Stream: 1

- [ ] 7. Write unit tests for FileWriter <!-- id:5dh82t7 -->
  - Test NewFileWriter creates directory and file. Test NewFileWriter returns error on empty runID. Test NewFileWriter returns nil on directory creation failure. Test Write produces valid JSONL. Test Write is thread-safe with concurrent writes (property test). Test warning rate limiting. Test nil receiver safety. Requirements: 1.1
  - 1.2
  - 1.3
  - 1.7
  - 9.1
  - 9.2
  - 9.4
  - Blocked-by: 5dh82t5 (Implement thread-safe Write method with rate-limited warnings), 5dh82t6 (Implement Path() and Close() methods)
  - Stream: 1

## Logger Extension

- [ ] 8. Extend Logger struct in internal/debug/debug.go <!-- id:5dh82t8 -->
  - Add stderrEnabled
  - fileEnabled
  - fileWriter
  - startTime
  - shutdownDone
  - mu fields. Create LoggerConfig struct with StderrEnabled
  - FileEnabled
  - RunID
  - VariantNum
  - Prefix. Implement NewLogger(cfg) that creates FileWriter based on config. Requirements: 7.1
  - 7.2
  - Blocked-by: 5dh82t5 (Implement thread-safe Write method with rate-limited warnings)
  - Stream: 1

- [ ] 9. Implement new structured logging methods <!-- id:5dh82t9 -->
  - Implement LogStructured(level
  - message
  - fields) for new code. Implement LogErrorWithChain(message
  - err
  - fields) with extractErrorChain(). Implement LogStartup(cfg StartupConfig). Implement LogShutdown(status) with double-write prevention. Implement Close() that writes shutdown if not done and closes writer. Requirements: 3.8
  - 5.3
  - 5.4
  - 7.7
  - Blocked-by: 5dh82t8 (Extend Logger struct in internal/debug/debug.go)
  - Stream: 1

- [ ] 10. Update existing Logger methods for dual output <!-- id:5dh82ta -->
  - Update Log(format
  - args) to write to file if enabled (message only
  - no structured fields). Update LogCmd() to write structured JSON to file. Update LogRetry() to write structured JSON to file. Update LogConfig() to write structured JSON to file. Update LogSession() to write structured JSON to file. Update LogError() to write structured JSON to file. Update LogCmdResult() to write structured JSON to file. Preserve existing stderr format unchanged. Requirements: 7.1
  - 7.3
  - 7.4
  - 7.5
  - 7.6
  - Blocked-by: 5dh82t9 (Implement new structured logging methods)
  - Stream: 1

- [ ] 11. Write unit tests for extended Logger <!-- id:5dh82tb -->
  - Test NewLogger creates FileWriter when enabled. Test Logger output modes (stderr only
  - file only
  - both
  - neither). Test LogStructured produces correct JSON. Test LogErrorWithChain produces correct error_chain array. Test LogStartup writes StartupEntry. Test LogShutdown writes ShutdownEntry only once. Test nil receiver safety for all methods. Test backward compatibility of existing method signatures. Requirements: 7.1
  - 7.2
  - 7.3
  - 7.4
  - 7.5
  - 7.6
  - Blocked-by: 5dh82ta (Update existing Logger methods for dual output)
  - Stream: 1

## Configuration

- [ ] 12. Add CentralizedLog to config.Config <!-- id:5dh82tc -->
  - Add CentralizedLog bool field to Config struct. Add default value true in Load(). Add ORBIT_CENTRALIZED_LOG environment variable handling. Add centralized-log YAML key loading. Requirements: 5.1
  - 6.2
  - 6.3
  - 6.4
  - Stream: 2

- [ ] 13. Add --centralized-log CLI flag <!-- id:5dh82td -->
  - Add --centralized-log flag to run.go with default true. Wire flag value to orbit.Config. Requirements: 6.1
  - Blocked-by: 5dh82tc (Add CentralizedLog to config.Config)
  - Stream: 2

- [ ] 14. Add RunID to orbit.Config <!-- id:5dh82te -->
  - Add RunID field to orbit.Config struct. Generate RunID using uuid.NewString() in run.go before orbit.New().
  - Blocked-by: 5dh82tc (Add CentralizedLog to config.Config)
  - Stream: 2

- [ ] 15. Write tests for configuration <!-- id:5dh82tf -->
  - Test default centralized-log is true. Test ORBIT_CENTRALIZED_LOG=false disables logging. Test --centralized-log=false disables logging. Test config hierarchy precedence. Requirements: 5.1
  - 6.1
  - 6.2
  - 6.3
  - 6.4
  - 6.5
  - Blocked-by: 5dh82td (Add --centralized-log CLI flag), 5dh82te (Add RunID to orbit.Config)
  - Stream: 2

## Integration

- [ ] 16. Update orbit.New() to create extended Logger <!-- id:5dh82tg -->
  - Create Logger with RunID from config. Pass CentralizedLog and Debug flags to LoggerConfig. Store logger in Orbit struct. Output log path to stderr if centralized logging enabled. Requirements: 5.2
  - 7.1
  - 8.1
  - 8.2
  - 8.3
  - Blocked-by: 5dh82tb (Write unit tests for extended Logger), 5dh82tf (Write tests for configuration)
  - Stream: 1

- [ ] 17. Add startup and shutdown logging <!-- id:5dh82th -->
  - Call LogStartup() with StartupConfig at orchestration start. Call LogShutdown() with status at orchestration completion. Update signal handler to call logger.Close() on interrupt. Requirements: 3.1
  - 3.9
  - 5.3
  - 5.4
  - Blocked-by: 5dh82tg (Update orbit.New() to create extended Logger)
  - Stream: 1

- [ ] 18. Add phase lifecycle logging <!-- id:5dh82ti -->
  - Log phase start with phase number and task count. Log phase completion with duration
  - status
  - and transcript_path. Requirements: 3.2
  - 3.3
  - 4.2
  - Blocked-by: 5dh82th (Add startup and shutdown logging)
  - Stream: 1

- [ ] 19. Add agent execution logging <!-- id:5dh82tj -->
  - Log agent invocation with command
  - args
  - working directory. Log agent completion with exit code
  - duration
  - session ID
  - session_log_path. Requirements: 3.4
  - 3.5
  - 4.1
  - Blocked-by: 5dh82th (Add startup and shutdown logging)
  - Stream: 1

- [ ] 20. Add retry and error logging <!-- id:5dh82tk -->
  - Log retry attempts with attempt number
  - error classification
  - backoff duration. Log errors with full message and error_chain array. Requirements: 3.6
  - 3.8
  - Blocked-by: 5dh82th (Add startup and shutdown logging)
  - Stream: 1

- [ ] 21. Add configuration loading logging <!-- id:5dh82tl -->
  - Log configuration loading from each source (CLI
  - env
  - project config
  - home config). Requirements: 3.7
  - Blocked-by: 5dh82th (Add startup and shutdown logging)
  - Stream: 1

- [ ] 22. Add variant logging support <!-- id:5dh82tm -->
  - Create variant-specific Logger in variants.Manager.runVariant(). Log variant creation and cleanup to parent logger. Log parallel execution start to parent logger. Requirements: 1.4
  - 1.5
  - 1.6
  - 3.10
  - Blocked-by: 5dh82th (Add startup and shutdown logging)
  - Stream: 1

## Integration Tests

- [ ] 23. Write integration tests <!-- id:5dh82tn -->
  - Test full orchestration produces all expected log entries. Test variant mode creates N+1 log files with correct names. Test log file contains startup and shutdown entries. Test cross-reference paths are absolute. Test SIGTERM triggers shutdown entry. Test disabled logging creates no files. Requirements: 1.4
  - 1.6
  - 3.1
  - 3.9
  - 4.1
  - 4.2
  - 4.3
  - 5.3
  - 5.4
  - 6.5
  - Blocked-by: 5dh82tm (Add variant logging support)
  - Stream: 1
