package logs

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"modernc.org/sqlite"
)

func TestNewTestDB(t *testing.T) {
	db := NewTestDB("/some/path")
	assert.Equal(t, "/some/path", db.path)
}

func TestOpenConn_ValidDB(t *testing.T) {
	db := createTestDB(t)

	conn, err := db.openConn(context.Background())
	require.NoError(t, err)
	require.NotNil(t, conn)
	_ = conn.Close()
}

func TestOpenConn_NonExistentDB(t *testing.T) {
	db := NewTestDB("/nonexistent/path/to/db.sqlite3")

	_, err := db.openConn(context.Background())
	require.Error(t, err)
}

func TestVerifySchema_ValidSchema(t *testing.T) {
	db := createTestDB(t)

	conn, err := db.openConn(context.Background())
	require.NoError(t, err)
	_ = conn.Close()
	// If we got here without error, schema verification passed
}

func TestVerifySchema_MissingTable(t *testing.T) {
	db := createTestDBWithoutSchema(t)

	_, err := db.openConn(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSchemaInvalid)
	assert.Contains(t, err.Error(), "conversations_v2")
}

func TestClassifyError_Nil(t *testing.T) {
	err := classifyError(nil)
	assert.NoError(t, err)
}

func TestClassifyError_GenericError(t *testing.T) {
	origErr := errors.New("some generic error")
	err := classifyError(origErr)
	assert.Equal(t, origErr, err)
}

func TestClassifyError_DatabaseLocked_StringMatch(t *testing.T) {
	// Test string-based fallback for "database is locked" message
	origErr := errors.New("database is locked")
	err := classifyError(origErr)
	assert.ErrorIs(t, err, ErrDatabaseLocked)
}

func TestClassifyError_DatabaseLocked_SQLiteBusy(t *testing.T) {
	// Test string-based fallback for SQLITE_BUSY message
	origErr := errors.New("SQLITE_BUSY: some operation")
	err := classifyError(origErr)
	assert.ErrorIs(t, err, ErrDatabaseLocked)
}

func TestClassifyError_SQLiteError_Busy(t *testing.T) {
	// Create a real sqlite.Error with SQLITE_BUSY code (5)
	// The sqlite.Error type is created by the driver, so we test via string matching
	// when we can't easily construct the error type
	origErr := &sqlite.Error{}
	// Note: We can't easily set the code on sqlite.Error from outside,
	// so this test verifies the fallback path works
	err := classifyError(origErr)
	// Should return the original error (code 0 doesn't match BUSY/LOCKED)
	assert.NotNil(t, err)
}

func TestClassifyError_WrappedError(t *testing.T) {
	// Test that wrapped errors with lock messages are classified
	wrapped := errors.New("operation failed: database is locked")
	err := classifyError(wrapped)
	assert.ErrorIs(t, err, ErrDatabaseLocked)
}

func TestOpenConn_ReadOnlyMode(t *testing.T) {
	// Verify that the connection is opened in read-only mode by attempting a write
	db := createTestDB(t)

	conn, err := db.openConn(context.Background())
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	// Try to insert data - should fail because we're in read-only mode
	_, err = conn.Exec("INSERT INTO conversations_v2 VALUES (?, ?, ?, ?, ?)",
		"/test", "session-1", "{}", 1000, 1000)
	require.Error(t, err)
	// The error should indicate read-only or permission denied
	assert.Contains(t, err.Error(), "readonly")
}

func TestOpenConn_MaxConnections(t *testing.T) {
	// Verify that max connections is set to 1
	db := createTestDB(t)

	conn, err := db.openConn(context.Background())
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	stats := conn.Stats()
	assert.Equal(t, 1, stats.MaxOpenConnections)
}
