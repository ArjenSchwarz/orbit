# Requirements: Kiro SQLite Log Parsing

## Introduction

This feature enables direct parsing of Kiro CLI session logs from Kiro's native SQLite database storage, replacing the current workaround that uses `/chat save` to export sessions. Kiro stores all conversation history in an SQLite database at OS-specific locations, with the conversation data matching the existing JSON format that Orbit's transcript parser already understands.

The implementation will provide first-class Kiro session support in both Apsis (for listing and converting transcripts) and Orbit (for session discovery and log management), bringing Kiro to feature parity with Claude Code and Codex in terms of transcript handling.

### SQLite Database Details

**Database Locations:**
- macOS: `~/Library/Application Support/kiro-cli/data.sqlite3`
- Linux: `~/.local/share/kiro-cli/data.sqlite3`
- Windows: `%APPDATA%\kiro-cli\data.sqlite3`

**Schema (conversations_v2 table):**
```sql
CREATE TABLE conversations_v2 (
    key TEXT NOT NULL,              -- Working directory path
    conversation_id TEXT NOT NULL,  -- UUID of the conversation
    value TEXT NOT NULL,            -- JSON blob (conversation data)
    created_at INTEGER NOT NULL,    -- Unix timestamp in milliseconds
    updated_at INTEGER NOT NULL,    -- Unix timestamp in milliseconds
    PRIMARY KEY (key, conversation_id)
);
```

The `value` column contains JSON matching the existing Kiro transcript format with `conversation_id`, `history`, and `next_message` fields.

---

### 1. SQLite Database Access

**User Story:** As a developer using Orbit or Apsis, I want the tools to automatically locate and read Kiro's SQLite database, so that I can access Kiro session logs without manual configuration.

**Acceptance Criteria:**

1. <a name="1.1"></a>The system SHALL detect the operating system (macOS, Linux, Windows) at runtime
2. <a name="1.2"></a>The system SHALL resolve the correct database path based on the detected operating system, using `os.UserConfigDir()` for Windows rather than hardcoded environment variables
3. <a name="1.3"></a>The system SHALL return a clear error message WHEN the database file does not exist at the expected location
4. <a name="1.4"></a>The system SHALL open the database in read-only mode using SQLite URI syntax (`file:/path?mode=ro`)
5. <a name="1.5"></a>The system SHALL handle database access errors gracefully with descriptive error messages
6. <a name="1.6"></a>The system SHALL configure a busy timeout of 5 seconds to handle concurrent database access from Kiro CLI
7. <a name="1.7"></a>The system SHALL return a descriptive error IF the database remains locked after the timeout period
8. <a name="1.8"></a>The system SHALL verify the `conversations_v2` table exists by querying `sqlite_master` before attempting data queries

---

### 2. Session Discovery

**User Story:** As a developer, I want to discover Kiro sessions for a specific project directory, so that I can list and select sessions relevant to my current work.

**Acceptance Criteria:**

1. <a name="2.1"></a>The system SHALL normalize the working directory path using `filepath.Abs` followed by `filepath.Clean` before querying
2. <a name="2.2"></a>The system SHALL first query using the normalized path, then attempt a second query using the symlink-resolved path (via `filepath.EvalSymlinks`) IF the resolved path differs from the normalized path
3. <a name="2.3"></a>The system SHALL query the `conversations_v2` table filtering by the `key` column matching the normalized or resolved path
4. <a name="2.4"></a>The system SHALL return session metadata including conversation_id, created_at, and updated_at timestamps
5. <a name="2.5"></a>The system SHALL convert Unix millisecond timestamps to standard Go time.Time values
6. <a name="2.6"></a>The system SHALL order discovered sessions by updated_at in descending order (most recent first)
7. <a name="2.7"></a>The system SHALL return an empty list with no error WHEN no sessions exist for the specified directory
8. <a name="2.8"></a>The system SHALL deduplicate sessions IF the same session is found via both normalized and symlink-resolved paths

---

### 3. Session Data Retrieval

**User Story:** As a developer, I want to retrieve the full conversation data for a specific Kiro session, so that I can view or convert the transcript.

**Acceptance Criteria:**

1. <a name="3.1"></a>The system SHALL retrieve the `value` column for a given conversation_id and key combination
2. <a name="3.2"></a>The system SHALL parse the JSON blob from the `value` column into the existing Kiro transcript data structures
3. <a name="3.3"></a>The system SHALL return a clear error message WHEN the specified session does not exist
4. <a name="3.4"></a>The system SHALL handle malformed JSON in the `value` column by returning a descriptive parse error

---

### 4. Apsis Integration

**User Story:** As a developer using Apsis, I want to list and convert Kiro sessions just like Claude Code and Codex sessions, so that I have a consistent experience across all supported agents.

**Acceptance Criteria:**

1. <a name="4.1"></a>The `apsis -l` command SHALL include Kiro sessions in the session list when Kiro database is present
2. <a name="4.2"></a>Kiro sessions in the list SHALL display a source indicator identifying them as Kiro sessions
3. <a name="4.3"></a>The `apsis` command SHALL resolve Kiro session IDs by searching the SQLite database, filtering to the current working directory (using the same path normalization as session discovery)
4. <a name="4.4"></a>The `apsis` command SHALL convert Kiro sessions to Markdown using the existing transcript rendering pipeline
5. <a name="4.5"></a>The `apsis` command SHALL convert Kiro sessions to HTML WHEN the `--html` flag is provided
6. <a name="4.6"></a>The system SHALL filter Kiro sessions to only show those matching the current working directory
7. <a name="4.7"></a>WHEN listing sessions, IF a single entry has malformed JSON, the system SHALL log a warning and continue processing remaining entries rather than failing the entire operation

---

### 5. Orbit Agent Integration

**User Story:** As a developer using Orbit, I want the Kiro agent to discover sessions directly from SQLite, so that Orbit can track and manage Kiro sessions without requiring manual exports.

**Acceptance Criteria:**

1. <a name="5.1"></a>The Kiro agent's `DiscoverSessions` method SHALL return sessions from the SQLite database instead of returning nil
2. <a name="5.2"></a>The Kiro agent's `DefaultSessionDir` method SHALL return an empty string (sessions are in SQLite, not filesystem)
3. <a name="5.3"></a>The system SHALL remove the `SessionExporter` interface implementation from the Kiro agent
4. <a name="5.4"></a>The Orbit orchestrator SHALL no longer call `ExportSession` for Kiro agents after phase completion

---

### 6. Transcript Parsing

**User Story:** As a developer, I want SQLite-retrieved Kiro sessions to be parsed using the existing transcript parser, so that all rendering and display features work consistently.

**Acceptance Criteria:**

1. <a name="6.1"></a>The system SHALL convert SQLite-retrieved JSON to the existing `KiroConversation` struct
2. <a name="6.2"></a>The system SHALL use the existing `convertKiroToEntries` function for transcript entry conversion
3. <a name="6.3"></a>The system SHALL produce identical `Entry` output for the same conversation data whether retrieved from SQLite or a JSON file
4. <a name="6.4"></a>The system SHALL support the existing Markdown and HTML rendering pipelines without modification

---

### 7. Error Handling

**User Story:** As a developer, I want clear error messages when Kiro database access fails, so that I can diagnose and resolve issues quickly.

**Acceptance Criteria:**

1. <a name="7.1"></a>The system SHALL return an error indicating the expected database path WHEN the database file is not found
2. <a name="7.2"></a>The system SHALL return an error with SQLite error details WHEN the database cannot be opened
3. <a name="7.3"></a>The system SHALL return an error WHEN a query fails due to schema mismatch (e.g., missing conversations_v2 table)
4. <a name="7.4"></a>The system SHALL log warnings but continue operation WHEN Kiro database is unavailable but other agent databases are accessible
