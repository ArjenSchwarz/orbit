// Package logs provides access to Kiro session logs stored in SQLite.
package logs

import "errors"

// Error types for Kiro database operations.
var (
	// ErrDatabaseNotFound indicates the Kiro SQLite database file does not exist.
	ErrDatabaseNotFound = errors.New("kiro database not found")

	// ErrSchemaInvalid indicates the database exists but has an unexpected schema.
	ErrSchemaInvalid = errors.New("kiro database schema invalid")

	// ErrSessionNotFound indicates the requested session does not exist in the database.
	ErrSessionNotFound = errors.New("session not found")

	// ErrDatabaseLocked indicates the database remained locked after the busy timeout.
	ErrDatabaseLocked = errors.New("database locked after timeout")
)
