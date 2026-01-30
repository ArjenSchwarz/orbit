package logs

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// createTestDB creates a temporary SQLite database with the Kiro schema
// and returns a *DB configured to use it.
//
//nolint:unused // Used by db_test.go, discover_test.go, session_test.go (tasks 9, 10, 11)
func createTestDB(t *testing.T) *DB {
	t.Helper()

	tmpFile := filepath.Join(t.TempDir(), "test.db")

	// Create and initialize the database
	conn, err := sql.Open("sqlite", tmpFile)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

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

	// Close the connection so tests can open it fresh
	_ = conn.Close()

	// Return a DB pointing to the test file
	return NewTestDB(tmpFile)
}

// insertSession adds a test session to the database.
//
//nolint:unused // Used by discover_test.go, session_test.go (tasks 10, 11)
func insertSession(t *testing.T, db *DB, dir, id, jsonValue string, ts time.Time) {
	t.Helper()

	conn, err := sql.Open("sqlite", db.path)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	msec := ts.UnixMilli()
	_, err = conn.Exec(
		"INSERT INTO conversations_v2 VALUES (?, ?, ?, ?, ?)",
		dir, id, jsonValue, msec, msec,
	)
	require.NoError(t, err)
}

// insertSessionWithTimes adds a test session with separate created/updated times.
//
//nolint:unused // Used by discover_test.go (task 10)
func insertSessionWithTimes(t *testing.T, db *DB, dir, id, jsonValue string, created, updated time.Time) {
	t.Helper()

	conn, err := sql.Open("sqlite", db.path)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	_, err = conn.Exec(
		"INSERT INTO conversations_v2 VALUES (?, ?, ?, ?, ?)",
		dir, id, jsonValue, created.UnixMilli(), updated.UnixMilli(),
	)
	require.NoError(t, err)
}

// createTestDBWithoutSchema creates a database without the conversations_v2 table
// for testing schema validation errors.
//
//nolint:unused // Used by db_test.go (task 9)
func createTestDBWithoutSchema(t *testing.T) *DB {
	t.Helper()

	tmpFile := filepath.Join(t.TempDir(), "empty.db")

	conn, err := sql.Open("sqlite", tmpFile)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	// Create a different table to ensure the DB file exists
	_, err = conn.Exec("CREATE TABLE other_table (id INTEGER)")
	require.NoError(t, err)

	_ = conn.Close()

	return NewTestDB(tmpFile)
}
