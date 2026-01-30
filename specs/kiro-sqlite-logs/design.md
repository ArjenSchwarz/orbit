# Design: Kiro SQLite Log Parsing

## Overview

This design describes the implementation of direct Kiro session log parsing from Kiro's native SQLite database, replacing the current `/chat save` export workaround. The implementation adds a new `kiro/logs` package for SQLite access, modifies the Kiro agent to use native session discovery, and integrates Kiro sessions into Apsis for listing and conversion.

### Goals

1. Enable direct read access to Kiro sessions from SQLite database
2. Provide session discovery for both Orbit and Apsis
3. Maintain consistency with existing agent patterns
4. Keep the codebase CGO-free by using a pure-Go SQLite driver

### Non-Goals

1. Writing to Kiro's database
2. Backwards compatibility with previously exported session files
3. Supporting non-standard Kiro installations or custom database paths

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              User Commands                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   orbit run --agent kiro           apsis -l              apsis <session-id> │
│         │                              │                        │           │
│         ▼                              ▼                        ▼           │
│   ┌───────────┐                 ┌───────────┐             ┌───────────┐     │
│   │   Orbit   │                 │   Apsis   │             │   Apsis   │     │
│   │Orchestrator│                │  Listing  │             │ Convert   │     │
│   └─────┬─────┘                 └─────┬─────┘             └─────┬─────┘     │
│         │                              │                        │           │
│         ▼                              ▼                        ▼           │
│   ┌───────────┐                 ┌─────────────────────────────────────┐     │
│   │   Kiro    │                 │           listKiroSessions()        │     │
│   │   Agent   │                 │           resolveKiroSession()      │     │
│   └─────┬─────┘                 └─────────────────┬───────────────────┘     │
│         │                                          │                        │
│         │  DiscoverSessions()                      │                        │
│         ▼                                          ▼                        │
│   ┌─────────────────────────────────────────────────────────────────┐       │
│   │                    internal/agents/kiro/logs                     │       │
│   │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐  │       │
│   │  │  DBPath()   │  │DiscoverFor │  │   GetSession()          │  │       │
│   │  │ (OS detect) │  │ Directory()│  │   (returns JSON blob)   │  │       │
│   │  └─────────────┘  └─────────────┘  └─────────────────────────┘  │       │
│   └───────────────────────────────┬─────────────────────────────────┘       │
│                                   │                                         │
│                                   ▼                                         │
│   ┌─────────────────────────────────────────────────────────────────┐       │
│   │                    Kiro SQLite Database                          │       │
│   │  ~/Library/Application Support/kiro-cli/data.sqlite3 (macOS)    │       │
│   │  ~/.local/share/kiro-cli/data.sqlite3 (Linux)                   │       │
│   │  %APPDATA%\kiro-cli\data.sqlite3 (Windows)                      │       │
│   └─────────────────────────────────────────────────────────────────┘       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Data Flow

1. **Session Discovery**: Orbit or Apsis calls `logs.DiscoverForDirectory(dir)` → queries SQLite → returns `[]SessionMetadata`
2. **Session Retrieval**: Call `logs.GetSession(conversationID, dir)` → queries SQLite → returns JSON blob as `io.Reader`
3. **Transcript Parsing**: Pass `io.Reader` to existing `transcript.ParseKiro()` → returns `[]Entry`
4. **Rendering**: Pass `[]Entry` to `transcript.RenderMarkdown()` or `transcript.RenderHTML()`

## Components and Interfaces

### New Package: `internal/agents/kiro/logs`

This package handles all SQLite database interactions for Kiro session logs.

```go
package logs

import (
    "bytes"
    "context"
    "io"
    "time"
)

// SessionMetadata contains information about a discovered session.
type SessionMetadata struct {
    ConversationID string
    Directory      string    // The working directory (key column)
    CreatedAt      time.Time
    UpdatedAt      time.Time
    Size           int64     // Size of JSON blob in bytes
}

// Errors
var (
    ErrDatabaseNotFound = errors.New("kiro database not found")
    ErrSchemaInvalid    = errors.New("kiro database schema invalid")
    ErrSessionNotFound  = errors.New("session not found")
    ErrDatabaseLocked   = errors.New("database locked after timeout")
)

// DBPath returns the OS-specific path to the Kiro SQLite database.
// Returns an error if the path cannot be determined or the file doesn't exist.
func DBPath() (string, error)

// DiscoverForDirectory returns all sessions for the given working directory.
// The directory path is normalized and symlink-resolved before querying.
// Returns an empty slice (not error) if no sessions exist.
//
// This is a convenience function that uses DefaultDB().
func DiscoverForDirectory(ctx context.Context, dir string) ([]SessionMetadata, error) {
    db, err := DefaultDB()
    if err != nil {
        return nil, err
    }
    return db.DiscoverForDirectory(ctx, dir)
}

// GetSession retrieves the conversation JSON for the given session ID and directory.
// Returns the JSON blob as an io.Reader (backed by bytes.Reader - fully in memory).
// Returns ErrSessionNotFound if the session doesn't exist.
//
// This is a convenience function that uses DefaultDB().
func GetSession(ctx context.Context, conversationID, dir string) (io.Reader, error) {
    db, err := DefaultDB()
    if err != nil {
        return nil, err
    }
    return db.GetSession(ctx, conversationID, dir)
}

// DB.DiscoverForDirectory implementation with deduplication
func (d *DB) DiscoverForDirectory(ctx context.Context, dir string) ([]SessionMetadata, error) {
    conn, err := d.openConn(ctx)
    if err != nil {
        return nil, err
    }
    defer conn.Close()  // Connection closed before returning

    normalized, resolved, err := normalizePath(dir)
    if err != nil {
        return nil, fmt.Errorf("normalize path: %w", err)
    }

    // Use map for deduplication by ConversationID
    seen := make(map[string]SessionMetadata)

    // Query with normalized path
    if err := d.querySessions(ctx, conn, normalized, seen); err != nil {
        return nil, err
    }

    // Query with symlink-resolved path if different
    if resolved != "" {
        if err := d.querySessions(ctx, conn, resolved, seen); err != nil {
            return nil, err
        }
    }

    // Convert map to slice, sorted by updated_at DESC
    result := make([]SessionMetadata, 0, len(seen))
    for _, s := range seen {
        result = append(result, s)
    }
    sort.Slice(result, func(i, j int) bool {
        return result[i].UpdatedAt.After(result[j].UpdatedAt)
    })

    return result, nil
}

// querySessions queries for sessions matching dir and adds to seen map.
// Deduplication: keep the session with the most recent UpdatedAt.
func (d *DB) querySessions(ctx context.Context, conn *sql.DB, dir string, seen map[string]SessionMetadata) error {
    rows, err := conn.QueryContext(ctx, `
        SELECT conversation_id, key, created_at, updated_at, length(value)
        FROM conversations_v2
        WHERE key = ?
        ORDER BY updated_at DESC
    `, dir)
    if err != nil {
        return classifyError(err)
    }
    defer rows.Close()

    for rows.Next() {
        var s SessionMetadata
        var createdMS, updatedMS int64
        if err := rows.Scan(&s.ConversationID, &s.Directory, &createdMS, &updatedMS, &s.Size); err != nil {
            return classifyError(err)
        }
        s.CreatedAt = time.UnixMilli(createdMS)
        s.UpdatedAt = time.UnixMilli(updatedMS)

        // Deduplicate by ConversationID - keep the most recently updated
        if existing, ok := seen[s.ConversationID]; !ok || s.UpdatedAt.After(existing.UpdatedAt) {
            seen[s.ConversationID] = s
        }
    }

    return classifyError(rows.Err())
}

// DB.GetSession implementation - reads blob into memory, closes connection
func (d *DB) GetSession(ctx context.Context, conversationID, dir string) (io.Reader, error) {
    conn, err := d.openConn(ctx)
    if err != nil {
        return nil, err
    }
    defer conn.Close()  // Connection closed before returning

    normalized, resolved, err := normalizePath(dir)
    if err != nil {
        return nil, fmt.Errorf("normalize path: %w", err)
    }

    // Try normalized path first
    data, err := d.querySessionValue(ctx, conn, conversationID, normalized)
    if err == nil {
        return bytes.NewReader(data), nil
    }
    if !errors.Is(err, ErrSessionNotFound) {
        return nil, err
    }

    // Try symlink-resolved path if different
    if resolved != "" {
        data, err = d.querySessionValue(ctx, conn, conversationID, resolved)
        if err == nil {
            return bytes.NewReader(data), nil
        }
    }

    return nil, fmt.Errorf("%w: %s in directory %s", ErrSessionNotFound, conversationID, dir)
}

// querySessionValue retrieves the JSON blob for a specific session.
func (d *DB) querySessionValue(ctx context.Context, conn *sql.DB, conversationID, dir string) ([]byte, error) {
    var value []byte
    err := conn.QueryRowContext(ctx, `
        SELECT value FROM conversations_v2
        WHERE conversation_id = ? AND key = ?
    `, conversationID, dir).Scan(&value)

    if err == sql.ErrNoRows {
        return nil, ErrSessionNotFound
    }
    if err != nil {
        return nil, classifyError(err)
    }

    return value, nil
}
```

### Internal Implementation Details

#### Connection Lifecycle Strategy

Each public function (`DiscoverForDirectory`, `GetSession`) opens a connection, performs its work, and closes the connection before returning. This approach:
- Avoids resource leaks from long-lived connections
- Eliminates connection pool complexity
- Is acceptable because operations are infrequent and fast (reading small metadata or JSON blobs)

The `GetSession` function reads the entire JSON blob into memory and returns a `bytes.Reader`. This is appropriate because:
- JSON blobs are typically 100KB-1MB (observed from real data)
- SQLite TEXT columns are not streamable - the entire value must be read
- Memory usage is bounded and predictable
- Connection is cleanly closed before returning

#### Dependency Injection for Testing

All database operations go through a `DB` struct that can be configured with a custom path for testing:

```go
// db.go - Database connection management

// DB provides access to the Kiro SQLite database.
type DB struct {
    path string // Overridable for testing
}

// DefaultDB returns a DB configured for the standard Kiro database location.
func DefaultDB() (*DB, error) {
    path, err := DBPath()
    if err != nil {
        return nil, err
    }
    return &DB{path: path}, nil
}

// NewTestDB creates a DB pointing to a custom path for testing.
func NewTestDB(path string) *DB {
    return &DB{path: path}
}

// openConn opens a connection, configures it, and verifies schema.
// The caller is responsible for closing the returned connection.
func (d *DB) openConn(ctx context.Context) (*sql.DB, error) {
    // Escape path for URI - handles paths with ?, #, or other special chars
    escapedPath := url.PathEscape(d.path)

    // Use URI syntax with read-only mode and busy timeout
    // Setting _busy_timeout via DSN is more reliable than PRAGMA
    dsn := fmt.Sprintf("file:%s?mode=ro&_busy_timeout=5000", escapedPath)

    db, err := sql.Open("sqlite", dsn)
    if err != nil {
        return nil, fmt.Errorf("open kiro database: %w", err)
    }

    // Configure connection pool
    db.SetMaxOpenConns(1)  // SQLite best practice

    // Verify schema
    if err := d.verifySchema(ctx, db); err != nil {
        db.Close()
        return nil, err
    }

    return db, nil
}

// verifySchema checks that the conversations_v2 table exists.
// Also warns if newer schema versions are detected.
func (d *DB) verifySchema(ctx context.Context, db *sql.DB) error {
    var name string
    err := db.QueryRowContext(ctx, `
        SELECT name FROM sqlite_master
        WHERE type='table' AND name='conversations_v2'
    `).Scan(&name)
    if err == sql.ErrNoRows {
        return fmt.Errorf("%w: table 'conversations_v2' not found", ErrSchemaInvalid)
    }
    if err != nil {
        return classifyError(err)
    }

    // Check for newer schema versions (forward compatibility warning)
    var v3Exists bool
    err = db.QueryRowContext(ctx, `
        SELECT 1 FROM sqlite_master
        WHERE type='table' AND name='conversations_v3'
    `).Scan(&v3Exists)
    if err == nil && v3Exists {
        // Log warning but continue - v2 may still work
        // This is logged at the call site, not here
    }

    return nil
}

// classifyError converts SQLite-specific errors to application errors.
// Uses sqlite.Error type with error codes for robust detection.
func classifyError(err error) error {
    if err == nil {
        return nil
    }

    // Use driver-specific error type for reliable detection
    var sqliteErr *sqlite.Error
    if errors.As(err, &sqliteErr) {
        switch sqliteErr.Code() {
        case sqlite.SQLITE_BUSY, sqlite.SQLITE_LOCKED:
            return fmt.Errorf("%w: %v", ErrDatabaseLocked, err)
        case sqlite.SQLITE_READONLY, sqlite.SQLITE_PERM:
            return fmt.Errorf("database access denied: %w", err)
        }
    }

    // Fallback string matching for edge cases or wrapped errors
    errStr := err.Error()
    if strings.Contains(errStr, "database is locked") ||
       strings.Contains(errStr, "SQLITE_BUSY") {
        return fmt.Errorf("%w: %v", ErrDatabaseLocked, err)
    }

    return err
}

// path.go - OS-specific path resolution

// DBPath returns the OS-specific path to the Kiro SQLite database.
//
// Path conventions by OS:
// - macOS: ~/Library/Application Support/kiro-cli/data.sqlite3
//   Uses UserHomeDir() because macOS Application Support is relative to home.
// - Linux: ~/.local/share/kiro-cli/data.sqlite3
//   Uses UserHomeDir() following XDG Base Directory Specification.
// - Windows: %APPDATA%\kiro-cli\data.sqlite3
//   Uses UserConfigDir() because Windows separates config from home.
//   UserConfigDir() returns %APPDATA% (Roaming), not %LOCALAPPDATA%.
//
func DBPath() (string, error) {
    var base string

    switch runtime.GOOS {
    case "darwin":
        home, err := os.UserHomeDir()
        if err != nil {
            return "", err
        }
        base = filepath.Join(home, "Library", "Application Support", "kiro-cli")
    case "linux":
        home, err := os.UserHomeDir()
        if err != nil {
            return "", err
        }
        base = filepath.Join(home, ".local", "share", "kiro-cli")
    case "windows":
        // Windows uses a different convention: config data goes in %APPDATA%
        // os.UserConfigDir() returns %APPDATA% (Roaming AppData)
        configDir, err := os.UserConfigDir()
        if err != nil {
            return "", err
        }
        base = filepath.Join(configDir, "kiro-cli")
    default:
        return "", fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
    }

    dbPath := filepath.Join(base, "data.sqlite3")

    if _, err := os.Stat(dbPath); os.IsNotExist(err) {
        return "", fmt.Errorf("%w: expected at %s", ErrDatabaseNotFound, dbPath)
    }

    return dbPath, nil
}

// normalizePath returns the normalized path and optionally the symlink-resolved path.
// Both paths are returned so the caller can query with both if needed.
//
// Note on race conditions: If Kiro creates a session between the two queries,
// the session will be found on the second query. This is acceptable because:
// - The session is still discovered (just on the fallback query)
// - The directory association uses whichever path matched
// - This race is rare in practice (requires active Kiro session during listing)
func normalizePath(dir string) (normalized string, resolved string, err error) {
    normalized, err = filepath.Abs(dir)
    if err != nil {
        return "", "", err
    }
    normalized = filepath.Clean(normalized)

    resolved, err = filepath.EvalSymlinks(normalized)
    if err != nil {
        // Symlink resolution failed (broken link, permission denied, etc.)
        // Fall back to normalized path only
        return normalized, "", nil
    }

    if resolved == normalized {
        return normalized, "", nil  // No difference, don't query twice
    }

    return normalized, resolved, nil
}
```

### Modified: `internal/agents/kiro/agent.go`

Changes to the Kiro agent to use SQLite-based session discovery:

```go
// Before: Returns empty string
// After: Still returns empty string (sessions are in SQLite, not filesystem)
func (a *Agent) DefaultSessionDir() string {
    return ""
}

// Before: Returns nil
// After: Queries SQLite database
func (a *Agent) DiscoverSessions(ctx context.Context, projectDir string) ([]agents.SessionInfo, error) {
    sessions, err := logs.DiscoverForDirectory(ctx, projectDir)
    if err != nil {
        if errors.Is(err, logs.ErrDatabaseNotFound) {
            // Kiro not installed or never used, not an error
            return nil, nil
        }
        return nil, err
    }

    result := make([]agents.SessionInfo, len(sessions))
    for i, s := range sessions {
        result[i] = agents.SessionInfo{
            ID:        s.ConversationID,
            Agent:     "kiro",
            Path:      "",  // No filesystem path
            CreatedAt: s.CreatedAt,
            Size:      s.Size,
            Project:   s.Directory,
        }
    }

    return result, nil
}

// REMOVED: ExportSession() method and SessionExporter interface implementation
```

### Modified: `cmd/apsis/main.go`

New functions for Kiro session integration:

```go
// listKiroSessions returns all Kiro sessions for the current working directory.
func listKiroSessions(cwd string) ([]SessionInfo, error) {
    sessions, err := logs.DiscoverForDirectory(context.Background(), cwd)
    if err != nil {
        if errors.Is(err, logs.ErrDatabaseNotFound) {
            return nil, nil  // Kiro not available, not an error
        }
        return nil, fmt.Errorf("discover kiro sessions: %w", err)
    }

    result := make([]SessionInfo, len(sessions))
    for i, s := range sessions {
        result[i] = SessionInfo{
            ID:        s.ConversationID,
            CreatedAt: s.UpdatedAt,  // Use updated_at for sorting consistency
            Size:      s.Size,
            Source:    "kiro",
        }
    }

    return result, nil
}

// resolveKiroSession attempts to find a Kiro session by ID in the current directory.
func resolveKiroSession(sessionID, cwd string) (io.Reader, error) {
    return logs.GetSession(context.Background(), sessionID, cwd)
}

// Modified listAllSessions to include Kiro
func listAllSessions(projectPath string) ([]SessionInfo, error) {
    var allSessions []SessionInfo
    var warnings []string

    // Claude sessions
    claudeSessions, err := listClaudeSessions(projectPath)
    if err != nil {
        warnings = append(warnings, fmt.Sprintf("claude: %v", err))
    } else {
        allSessions = append(allSessions, claudeSessions...)
    }

    // Codex sessions
    codexSessions, err := listCodexSessions(homeDir)
    if err != nil {
        warnings = append(warnings, fmt.Sprintf("codex: %v", err))
    } else {
        allSessions = append(allSessions, codexSessions...)
    }

    // Kiro sessions (NEW)
    kiroSessions, err := listKiroSessions(projectPath)
    if err != nil {
        warnings = append(warnings, fmt.Sprintf("kiro: %v", err))
    } else {
        allSessions = append(allSessions, kiroSessions...)
    }

    // ... sort and return
}

// Modified resolveInput to check Kiro
func resolveInput(arg string) (input, error) {
    // ... existing file path logic ...

    // Check Claude location
    claudeFile := filepath.Join(homeDir, ".claude", "projects", claudeProjectPath, arg+".jsonl")
    if fileExists(claudeFile) {
        return input{reader: openFile(claudeFile), sessionID: arg}, nil
    }

    // Check Codex location
    // ... existing codex logic ...

    // Check Kiro database (NEW)
    reader, err := resolveKiroSession(arg, cwd)
    if err == nil {
        return input{reader: reader, sessionID: arg}, nil
    }
    if !errors.Is(err, logs.ErrSessionNotFound) && !errors.Is(err, logs.ErrDatabaseNotFound) {
        return input{}, fmt.Errorf("kiro lookup: %w", err)
    }

    return input{}, fmt.Errorf("session not found: %s", arg)
}
```

### Modified: `internal/orbit/orbit.go`

Remove the `ExportSession` call after Kiro phases:

```go
// The existing code checks for SessionExporter interface.
// Since Kiro no longer implements it, this block naturally skips:
if exporter, ok := o.agent.(agents.SessionExporter); ok {
    // Kiro no longer enters this block
    exportFilename := o.generateSessionExportFilename(phase)
    if err := exporter.ExportSession(o.shutdownCtx, exportFilename); err != nil {
        o.log.Warn("failed to export session", "error", err)
    }
}
```

No changes required to `orbit.go` - the interface check handles this automatically.

## Data Models

### SQLite Schema (Reference - Not Modified)

```sql
CREATE TABLE conversations_v2 (
    key TEXT NOT NULL,              -- Working directory path
    conversation_id TEXT NOT NULL,  -- UUID
    value TEXT NOT NULL,            -- JSON blob
    created_at INTEGER NOT NULL,    -- Unix timestamp (milliseconds)
    updated_at INTEGER NOT NULL,    -- Unix timestamp (milliseconds)
    PRIMARY KEY (key, conversation_id)
);
```

### SessionMetadata

```go
type SessionMetadata struct {
    ConversationID string    // From conversation_id column
    Directory      string    // From key column (working directory)
    CreatedAt      time.Time // From created_at (ms since epoch)
    UpdatedAt      time.Time // From updated_at (ms since epoch)
    Size           int64     // Length of value column in bytes
}
```

### JSON Blob Structure (Reference - Existing)

The `value` column contains JSON matching the existing `KiroSession` type in `internal/transcript/kiro_types.go`:

```go
type KiroSession struct {
    ConversationID string             `json:"conversation_id"`
    History        []KiroHistoryEntry `json:"history"`
    NextMessage    *KiroNextMessage   `json:"next_message"`
}
```

## Error Handling

### Error Types and Recovery

| Error | Cause | Recovery |
|-------|-------|----------|
| `ErrDatabaseNotFound` | Kiro not installed or never used | Return empty results, log warning |
| `ErrSchemaInvalid` | Database exists but schema changed | Return error with expected table name |
| `ErrSessionNotFound` | Session ID not in database for directory | Return error, caller tries other sources |
| `ErrDatabaseLocked` | Kiro actively writing, timeout exceeded | Return error with timeout duration |
| JSON parse error | Corrupt entry in database | Log warning, skip entry, continue |

### Error Handling Strategy

1. **Database not found**: Treat as "Kiro not available" - return empty results without error
2. **Schema mismatch**: Fail fast with clear error indicating expected vs actual
3. **Session not found**: Return specific error so callers can try other sources
4. **Database locked**: Return error after 5-second timeout with actionable message
5. **Corrupt JSON entries**: When listing, skip corrupt entries with warning; when fetching specific session, return error

### Integration with Existing Error Patterns

Apsis already handles per-source errors gracefully:

```go
kiroSessions, err := listKiroSessions(projectPath)
if err != nil {
    warnings = append(warnings, fmt.Sprintf("kiro: %v", err))
} else {
    allSessions = append(allSessions, kiroSessions...)
}
```

This pattern continues - Kiro errors don't prevent listing sessions from other agents.

## Testing Strategy

### Unit Tests

#### `internal/agents/kiro/logs/db_test.go`

```go
func TestDBPath(t *testing.T) {
    // Test OS detection returns expected paths
    // Use build tags or runtime.GOOS checks
}

func TestNormalizePath(t *testing.T) {
    tests := []struct {
        name       string
        input      string
        wantNorm   string
        wantResolved string
    }{
        {"absolute path", "/Users/dev/project", "/Users/dev/project", ""},
        {"relative path", "./project", "<abs>/project", ""},
        {"trailing slash", "/Users/dev/project/", "/Users/dev/project", ""},
        // Symlink tests require actual symlinks in test fixtures
    }
    // ...
}

func TestVerifySchema(t *testing.T) {
    // Create in-memory DB with and without conversations_v2 table
    // Verify correct error returned for missing table
}
```

#### `internal/agents/kiro/logs/discover_test.go`

```go
func TestDiscoverForDirectory(t *testing.T) {
    // Create temp SQLite database with test data
    db := createTestDB(t)
    defer db.Close()

    // Insert test sessions
    insertSession(t, db, "/test/project", "session-1", "{...}", time.Now())
    insertSession(t, db, "/test/project", "session-2", "{...}", time.Now())
    insertSession(t, db, "/other/project", "session-3", "{...}", time.Now())

    // Test discovery filters by directory
    sessions, err := DiscoverForDirectory(ctx, "/test/project")
    require.NoError(t, err)
    assert.Len(t, sessions, 2)

    // Test empty result for unknown directory
    sessions, err = DiscoverForDirectory(ctx, "/unknown")
    require.NoError(t, err)
    assert.Empty(t, sessions)
}

func TestDiscoverForDirectory_SymlinkResolution(t *testing.T) {
    // Create symlink: /tmp/link -> /tmp/real
    // Insert session with key=/tmp/real
    // Verify discovery from /tmp/link finds the session
}

func TestDiscoverForDirectory_Deduplication(t *testing.T) {
    // Insert same session findable via both paths
    // Verify it appears only once in results
}
```

#### `internal/agents/kiro/logs/session_test.go`

```go
func TestGetSession(t *testing.T) {
    db := createTestDB(t)

    testJSON := `{"conversation_id":"abc","history":[]}`
    insertSession(t, db, "/test", "abc", testJSON, time.Now())

    reader, err := GetSession(ctx, "abc", "/test")
    require.NoError(t, err)

    content, _ := io.ReadAll(reader)
    assert.JSONEq(t, testJSON, string(content))
}

func TestGetSession_NotFound(t *testing.T) {
    db := createTestDB(t)

    _, err := GetSession(ctx, "nonexistent", "/test")
    assert.ErrorIs(t, err, ErrSessionNotFound)
}
```

### Integration Tests

#### `internal/agents/kiro/agent_test.go`

```go
func TestKiroAgent_DiscoverSessions(t *testing.T) {
    // Skip if Kiro database not available
    if _, err := logs.DBPath(); err != nil {
        t.Skip("Kiro database not available")
    }

    agent := kiro.New(agents.AgentConfig{})
    sessions, err := agent.DiscoverSessions(ctx, os.Getwd())
    require.NoError(t, err)
    // Sessions may be empty if no Kiro sessions exist for current dir
}
```

#### `cmd/apsis/main_test.go`

```go
func TestListAllSessions_IncludesKiro(t *testing.T) {
    // Test that Kiro sessions appear in combined listing
    // May need test fixtures or skip if no Kiro DB
}

func TestResolveInput_KiroSession(t *testing.T) {
    // Test that Kiro session IDs are resolved from SQLite
}
```

### Test Fixtures

Create test helpers that use the `DB` struct's dependency injection:

```go
// internal/agents/kiro/logs/testutil_test.go

// createTestDB creates a temporary SQLite database with the Kiro schema
// and returns a *DB configured to use it.
func createTestDB(t *testing.T) *DB {
    t.Helper()

    tmpFile := filepath.Join(t.TempDir(), "test.db")

    // Create and initialize the database
    conn, err := sql.Open("sqlite", tmpFile)
    require.NoError(t, err)

    _, err = conn.Exec(`
        CREATE TABLE conversations_v2 (
            key TEXT NOT NULL,
            conversation_id TEXT NOT NULL,
            value TEXT NOT NULL,
            created_at INTEGER NOT NULL,
            updated_at INTEGER NOT NULL,
            PRIMARY KEY (key, conversation_id)
        )
    `)
    require.NoError(t, err)
    conn.Close()

    // Return a DB pointing to the test file
    return NewTestDB(tmpFile)
}

// insertSession adds a test session to the database.
func insertSession(t *testing.T, db *DB, dir, id, json string, ts time.Time) {
    t.Helper()

    conn, err := sql.Open("sqlite", db.path)
    require.NoError(t, err)
    defer conn.Close()

    msec := ts.UnixMilli()
    _, err = conn.Exec(
        "INSERT INTO conversations_v2 VALUES (?, ?, ?, ?, ?)",
        dir, id, json, msec, msec,
    )
    require.NoError(t, err)
}

// createTestDBWithoutSchema creates a database without the conversations_v2 table
// for testing schema validation errors.
func createTestDBWithoutSchema(t *testing.T) *DB {
    t.Helper()

    tmpFile := filepath.Join(t.TempDir(), "empty.db")

    conn, err := sql.Open("sqlite", tmpFile)
    require.NoError(t, err)
    // Create a different table to ensure the DB file exists
    conn.Exec("CREATE TABLE other_table (id INTEGER)")
    conn.Close()

    return NewTestDB(tmpFile)
}
```

#### Updated Test Examples Using Dependency Injection

```go
func TestDiscoverForDirectory(t *testing.T) {
    db := createTestDB(t)  // Returns *DB with test path

    // Insert test sessions
    insertSession(t, db, "/test/project", "session-1", `{"conversation_id":"session-1"}`, time.Now())
    insertSession(t, db, "/test/project", "session-2", `{"conversation_id":"session-2"}`, time.Now())
    insertSession(t, db, "/other/project", "session-3", `{"conversation_id":"session-3"}`, time.Now())

    ctx := context.Background()

    // Test discovery filters by directory - uses db directly, not DefaultDB()
    sessions, err := db.DiscoverForDirectory(ctx, "/test/project")
    require.NoError(t, err)
    assert.Len(t, sessions, 2)

    // Test empty result for unknown directory
    sessions, err = db.DiscoverForDirectory(ctx, "/unknown")
    require.NoError(t, err)
    assert.Empty(t, sessions)
}

func TestGetSession(t *testing.T) {
    db := createTestDB(t)

    testJSON := `{"conversation_id":"abc","history":[]}`
    insertSession(t, db, "/test", "abc", testJSON, time.Now())

    ctx := context.Background()

    reader, err := db.GetSession(ctx, "abc", "/test")
    require.NoError(t, err)

    content, _ := io.ReadAll(reader)
    assert.JSONEq(t, testJSON, string(content))
}

func TestVerifySchema_MissingTable(t *testing.T) {
    db := createTestDBWithoutSchema(t)

    ctx := context.Background()
    _, err := db.DiscoverForDirectory(ctx, "/test")

    assert.ErrorIs(t, err, ErrSchemaInvalid)
    assert.Contains(t, err.Error(), "conversations_v2")
}

func TestClassifyError_DatabaseLocked(t *testing.T) {
    // Test that SQLITE_BUSY errors are classified correctly
    busyErr := errors.New("SQLITE_BUSY: database is locked")
    classified := classifyError(busyErr)

    assert.ErrorIs(t, classified, ErrDatabaseLocked)
    assert.Contains(t, classified.Error(), "5s timeout")
}
```

### Acceptance Criteria Coverage

| Requirement | Test Coverage |
|-------------|---------------|
| 1.1 OS detection | `TestDBPath` with build tags |
| 1.2 Path resolution | `TestDBPath` |
| 1.3 DB not found error | `TestDBPath_NotFound` |
| 1.4 Read-only mode | `TestOpenDB_ReadOnly` (attempt write, expect error) |
| 1.5 Error messages | All error tests verify message content |
| 1.6 Busy timeout | `TestOpenDB_BusyTimeout` (concurrent access) |
| 1.7 Lock timeout error | `TestOpenDB_LockError` |
| 1.8 Schema verification | `TestVerifySchema` |
| 2.1-2.8 Path normalization | `TestNormalizePath`, `TestDiscoverForDirectory_*` |
| 3.1-3.4 Session retrieval | `TestGetSession_*` |
| 4.1-4.7 Apsis integration | `TestListAllSessions_*`, `TestResolveInput_*` |
| 5.1-5.4 Orbit integration | `TestKiroAgent_DiscoverSessions` |
| 6.1-6.4 Transcript parsing | Existing tests (no changes needed) |
| 7.1-7.4 Error handling | Various error condition tests |

## SQLite Driver Decision

### Decision: Use `modernc.org/sqlite` (Pure Go)

**Rationale:**
1. Orbit is currently CGO-free - introducing `mattn/go-sqlite3` would add CGO dependency
2. CGO complicates cross-compilation and CI builds
3. Pure Go driver simplifies distribution (single binary)
4. Performance is not critical - we're reading small JSON blobs

**Trade-offs:**
- Slightly larger binary size (~5-10MB increase)
- Marginally slower than CGO version (not significant for this use case)
- Less mature than `mattn/go-sqlite3` but well-tested

**Connection Configuration:**
```go
import (
    "database/sql"
    _ "modernc.org/sqlite"
)

db, err := sql.Open("sqlite", "file:/path/to/db?mode=ro")
db.SetMaxOpenConns(1)
db.ExecContext(ctx, "PRAGMA busy_timeout = 5000")
```

## File Changes Summary

| File | Change Type | Description |
|------|-------------|-------------|
| `internal/agents/kiro/logs/db.go` | New | `DB` struct, `openConn()`, `verifySchema()`, `classifyError()` |
| `internal/agents/kiro/logs/path.go` | New | `DBPath()`, `normalizePath()` |
| `internal/agents/kiro/logs/discover.go` | New | `DiscoverForDirectory()`, `querySessions()`, dedup logic |
| `internal/agents/kiro/logs/session.go` | New | `GetSession()`, `querySessionValue()` |
| `internal/agents/kiro/logs/errors.go` | New | `ErrDatabaseNotFound`, `ErrSchemaInvalid`, `ErrSessionNotFound`, `ErrDatabaseLocked` |
| `internal/agents/kiro/logs/testutil_test.go` | New | `createTestDB()`, `insertSession()`, `createTestDBWithoutSchema()` |
| `internal/agents/kiro/logs/db_test.go` | New | Tests for connection, schema, error classification |
| `internal/agents/kiro/logs/discover_test.go` | New | Tests for discovery, deduplication, symlinks |
| `internal/agents/kiro/logs/session_test.go` | New | Tests for session retrieval |
| `internal/agents/kiro/agent.go` | Modify | Update `DiscoverSessions()`, remove `ExportSession()` |
| `cmd/apsis/main.go` | Modify | Add `listKiroSessions()`, update `resolveInput()` |
| `go.mod` | Modify | Add `modernc.org/sqlite` dependency |

## Traceability Matrix

| Requirement | Design Element |
|-------------|----------------|
| 1.1 OS detection | `logs.DBPath()` with `runtime.GOOS` switch |
| 1.2 Path resolution | `logs.DBPath()` using `os.UserConfigDir()` for Windows (documented) |
| 1.3 DB not found | `ErrDatabaseNotFound` with expected path in message |
| 1.4 Read-only mode | URI parameter `?mode=ro` in `openConn()` |
| 1.5 Error messages | All `Err*` types with descriptive messages |
| 1.6 Busy timeout | `_busy_timeout=5000` DSN parameter in `openConn()` |
| 1.7 Lock error | `classifyError()` uses `sqlite.Error.Code()` → `ErrDatabaseLocked` |
| 1.8 Schema validation | `verifySchema()` queries `sqlite_master` for `conversations_v2` |
| 2.1 Path normalization | `normalizePath()` with `filepath.Abs` + `filepath.Clean` |
| 2.2 Symlink fallback | `normalizePath()` returns resolved path, `DiscoverForDirectory` queries both |
| 2.3 Query filtering | SQL `WHERE key = ?` clause in `querySessions()` |
| 2.4 Session metadata | `SessionMetadata` struct with all fields populated |
| 2.5 Timestamp conversion | `time.UnixMilli(createdMS/updatedMS)` in `querySessions()` |
| 2.6 Sort order | `sort.Slice()` by `UpdatedAt.After()` after dedup |
| 2.7 Empty results | Return empty slice, nil error when `len(seen) == 0` |
| 2.8 Deduplication | `seen` map keyed by `ConversationID`, first match wins |
| 3.1 Value retrieval | `querySessionValue()` with `SELECT value WHERE ...` |
| 3.2 JSON parsing | Return `bytes.NewReader(data)` to existing `ParseKiro()` |
| 3.3 Not found error | `ErrSessionNotFound` with session ID and directory in message |
| 3.4 Malformed JSON | `ParseKiro()` handles, error propagated to caller |
| 4.1 Apsis listing | `listKiroSessions()` in `listAllSessions()` |
| 4.2 Source indicator | `Source: "kiro"` in `SessionInfo` |
| 4.3 ID resolution | `resolveKiroSession()` in `resolveInput()` |
| 4.4 Markdown render | Existing `RenderMarkdown()` pipeline |
| 4.5 HTML render | Existing `RenderHTML()` pipeline |
| 4.6 Directory filter | Path normalization + directory-scoped queries |
| 4.7 Corrupt entry recovery | Apsis warning pattern continues (skip, log, continue) |
| 5.1 Agent discovery | `agent.DiscoverSessions()` calls `logs.DiscoverForDirectory()` |
| 5.2 Empty session dir | Returns `""` unchanged |
| 5.3 Remove exporter | Delete `ExportSession()` method |
| 5.4 Skip export call | Interface check naturally skips Kiro |
| 6.1-6.4 Transcript parsing | Unchanged - reuses existing infrastructure |
| 7.1-7.4 Error handling | `classifyError()` maps SQLite errors to app errors |

## Design Decisions Added

The following design decisions address implementation details not covered in requirements:

| Decision | Rationale |
|----------|-----------|
| Connection-per-operation | Each call opens/closes connection; avoids resource leaks, acceptable for low-frequency operations |
| JSON blob fully in memory | SQLite TEXT not streamable; blobs are 100KB-1MB; `bytes.Reader` provides clean `io.Reader` interface |
| `DB` struct for DI | Enables test injection without modifying production path resolution |
| `classifyError()` with error codes | Uses `sqlite.Error.Code()` for robust detection; fallback string matching for edge cases |
| DSN busy timeout | `_busy_timeout=5000` in DSN is more reliable than PRAGMA for connection pool |
| DSN path escaping | `url.PathEscape()` handles paths with `?`, `#`, or URI-special characters |
| Most-recent-wins dedup | When symlink/normalized queries find same session, keep the one with latest `UpdatedAt` |
| Forward schema check | Warn on `conversations_v3` existence without failing; aids future compatibility |
