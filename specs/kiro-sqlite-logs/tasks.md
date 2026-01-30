---
references:
    - requirements.md
    - design.md
    - decision_log.md
---
# Kiro SQLite Log Parsing

## Setup

- [x] 1. Add modernc.org/sqlite dependency to go.mod <!-- id:3wwt01s -->
  - Run go get modernc.org/sqlite
  - Pure Go SQLite driver
  - No CGO required
  - Stream: 1
  - Owner: agent-stream-1

## Foundation

- [x] 2. Create error types in internal/agents/kiro/logs/errors.go <!-- id:3wwt01a -->
  - Define ErrDatabaseNotFound, ErrSchemaInvalid, ErrSessionNotFound, ErrDatabaseLocked
  - Stream: 1
  - Owner: agent-stream-1
  - Requirements: [1.3](requirements.md#1.3), [1.5](requirements.md#1.5), [3.3](requirements.md#3.3), [7.1](requirements.md#7.1), [7.2](requirements.md#7.2), [7.3](requirements.md#7.3), [7.4](requirements.md#7.4)

- [x] 3. Create path resolution in internal/agents/kiro/logs/path.go <!-- id:3wwt01b -->
  - Implement DBPath() with OS detection (darwin/linux/windows) and normalizePath() for symlink handling
  - Use runtime.GOOS switch
  - os.UserConfigDir() for Windows
  - filepath.EvalSymlinks for symlink resolution
  - Stream: 1
  - Owner: agent-stream-1
  - Requirements: [1.1](requirements.md#1.1), [1.2](requirements.md#1.2), [2.1](requirements.md#2.1), [2.2](requirements.md#2.2)

- [x] 4. Create DB struct and connection management in internal/agents/kiro/logs/db.go <!-- id:3wwt01c -->
  - Implement DB struct with NewTestDB(), DefaultDB(), openConn(), verifySchema(), classifyError()
  - Use url.PathEscape for DSN
  - _busy_timeout=5000 in DSN
  - Use sqlite.Error.Code() for error classification
  - Check sqlite_master for schema validation
  - Blocked-by: 3wwt01a (Create error types in internal/agents/kiro/logs/errors.go), 3wwt01b (Create path resolution in internal/agents/kiro/logs/path.go), 3wwt01s (Add modernc.org/sqlite dependency to go.mod)
  - Stream: 1
  - Requirements: [1.4](requirements.md#1.4), [1.6](requirements.md#1.6), [1.7](requirements.md#1.7), [1.8](requirements.md#1.8)

## Session Operations

- [ ] 5. Create session discovery in internal/agents/kiro/logs/discover.go <!-- id:3wwt01d -->
  - Implement DiscoverForDirectory() and querySessions() with deduplication
  - Query both normalized and resolved paths
  - Deduplicate by ConversationID keeping most recent UpdatedAt
  - Return sorted by updated_at DESC
  - Blocked-by: 3wwt01c (Create DB struct and connection management in internal/agents/kiro/logs/db.go)
  - Stream: 1
  - Requirements: [2.3](requirements.md#2.3), [2.4](requirements.md#2.4), [2.5](requirements.md#2.5), [2.6](requirements.md#2.6), [2.7](requirements.md#2.7), [2.8](requirements.md#2.8)

- [ ] 6. Create session retrieval in internal/agents/kiro/logs/session.go <!-- id:3wwt01e -->
  - Implement GetSession() and querySessionValue() returning bytes.Reader
  - Read entire JSON blob into memory
  - Return bytes.NewReader for io.Reader interface
  - Try normalized path first, then resolved
  - Blocked-by: 3wwt01c (Create DB struct and connection management in internal/agents/kiro/logs/db.go)
  - Stream: 1
  - Requirements: [3.1](requirements.md#3.1), [3.2](requirements.md#3.2), [3.3](requirements.md#3.3), [3.4](requirements.md#3.4)

## Unit Tests

- [x] 7. Create test utilities in internal/agents/kiro/logs/testutil_test.go <!-- id:3wwt01f -->
  - Implement createTestDB(), insertSession(), createTestDBWithoutSchema()
  - Use t.TempDir() for temp database
  - NewTestDB() for dependency injection
  - Blocked-by: 3wwt01s (Add modernc.org/sqlite dependency to go.mod)
  - Stream: 1

- [ ] 8. Write unit tests for path.go <!-- id:3wwt01g -->
  - Test DBPath() OS detection and normalizePath() behavior
  - Blocked-by: 3wwt01b (Create path resolution in internal/agents/kiro/logs/path.go), 3wwt01f (Create test utilities in internal/agents/kiro/logs/testutil_test.go)
  - Stream: 1
  - Requirements: [1.1](requirements.md#1.1), [1.2](requirements.md#1.2), [2.1](requirements.md#2.1), [2.2](requirements.md#2.2)

- [ ] 9. Write unit tests for db.go <!-- id:3wwt01h -->
  - Test openConn(), verifySchema(), classifyError()
  - Test ErrSchemaInvalid for missing table
  - Test ErrDatabaseLocked classification
  - Blocked-by: 3wwt01c (Create DB struct and connection management in internal/agents/kiro/logs/db.go), 3wwt01f (Create test utilities in internal/agents/kiro/logs/testutil_test.go)
  - Stream: 1
  - Requirements: [1.4](requirements.md#1.4), [1.6](requirements.md#1.6), [1.7](requirements.md#1.7), [1.8](requirements.md#1.8)

- [ ] 10. Write unit tests for discover.go <!-- id:3wwt01i -->
  - Test DiscoverForDirectory() filtering, deduplication, and symlink handling
  - Blocked-by: 3wwt01d (Create session discovery in internal/agents/kiro/logs/discover.go), 3wwt01f (Create test utilities in internal/agents/kiro/logs/testutil_test.go)
  - Stream: 1
  - Requirements: [2.1](requirements.md#2.1), [2.2](requirements.md#2.2), [2.3](requirements.md#2.3), [2.6](requirements.md#2.6), [2.7](requirements.md#2.7), [2.8](requirements.md#2.8)

- [ ] 11. Write unit tests for session.go <!-- id:3wwt01j -->
  - Test GetSession() retrieval and ErrSessionNotFound
  - Blocked-by: 3wwt01e (Create session retrieval in internal/agents/kiro/logs/session.go), 3wwt01f (Create test utilities in internal/agents/kiro/logs/testutil_test.go)
  - Stream: 1
  - Requirements: [3.1](requirements.md#3.1), [3.3](requirements.md#3.3), [3.4](requirements.md#3.4)

## Agent Integration

- [ ] 12. Update Kiro agent DiscoverSessions in internal/agents/kiro/agent.go <!-- id:3wwt01k -->
  - Replace nil return with logs.DiscoverForDirectory() call
  - Convert SessionMetadata to agents.SessionInfo
  - Blocked-by: 3wwt01d (Create session discovery in internal/agents/kiro/logs/discover.go)
  - Stream: 2
  - Requirements: [5.1](requirements.md#5.1), [5.2](requirements.md#5.2)

- [x] 13. Remove ExportSession from Kiro agent <!-- id:3wwt01l -->
  - Delete ExportSession() method and SessionExporter interface implementation
  - Orbit interface check will naturally skip Kiro
  - Stream: 2
  - Owner: agent-stream-2
  - Requirements: [5.3](requirements.md#5.3), [5.4](requirements.md#5.4)

- [ ] 14. Write integration tests for Kiro agent <!-- id:3wwt01m -->
  - Test DiscoverSessions() with SQLite
  - Blocked-by: 3wwt01k (Update Kiro agent DiscoverSessions in internal/agents/kiro/agent.go)
  - Stream: 2
  - Requirements: [5.1](requirements.md#5.1)

## Apsis Integration

- [ ] 15. Add listKiroSessions in cmd/apsis/main.go <!-- id:3wwt01n -->
  - Implement function to list Kiro sessions from SQLite for current directory
  - Return Source: kiro in SessionInfo
  - Handle ErrDatabaseNotFound gracefully
  - Blocked-by: 3wwt01d (Create session discovery in internal/agents/kiro/logs/discover.go)
  - Stream: 2
  - Requirements: [4.1](requirements.md#4.1), [4.2](requirements.md#4.2), [4.6](requirements.md#4.6), [4.7](requirements.md#4.7)

- [ ] 16. Update listAllSessions in cmd/apsis/main.go <!-- id:3wwt01o -->
  - Add Kiro sessions to combined listing alongside Claude and Codex
  - Blocked-by: 3wwt01n (Add listKiroSessions in cmd/apsis/main.go)
  - Stream: 2
  - Requirements: [4.1](requirements.md#4.1)

- [ ] 17. Add resolveKiroSession in cmd/apsis/main.go <!-- id:3wwt01p -->
  - Implement function to resolve Kiro session ID from SQLite
  - Blocked-by: 3wwt01e (Create session retrieval in internal/agents/kiro/logs/session.go)
  - Stream: 2
  - Requirements: [4.3](requirements.md#4.3)

- [ ] 18. Update resolveInput in cmd/apsis/main.go <!-- id:3wwt01q -->
  - Add Kiro session lookup after Claude and Codex checks
  - Handle ErrSessionNotFound to continue searching
  - Blocked-by: 3wwt01p (Add resolveKiroSession in cmd/apsis/main.go)
  - Stream: 2
  - Requirements: [4.3](requirements.md#4.3), [4.4](requirements.md#4.4), [4.5](requirements.md#4.5)

- [ ] 19. Write tests for Apsis Kiro integration <!-- id:3wwt01r -->
  - Test listKiroSessions(), resolveKiroSession(), updated resolveInput()
  - Blocked-by: 3wwt01n (Add listKiroSessions in cmd/apsis/main.go), 3wwt01o (Update listAllSessions in cmd/apsis/main.go), 3wwt01p (Add resolveKiroSession in cmd/apsis/main.go), 3wwt01q (Update resolveInput in cmd/apsis/main.go)
  - Stream: 2
  - Requirements: [4.1](requirements.md#4.1), [4.3](requirements.md#4.3)
