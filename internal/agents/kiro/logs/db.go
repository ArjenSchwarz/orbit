package logs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"modernc.org/sqlite"
)

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

// Path returns the database path. Used for testing.
func (d *DB) Path() string {
	return d.path
}

// openConn opens a connection, configures it, and verifies schema.
// The caller is responsible for closing the returned connection.
func (d *DB) openConn(ctx context.Context) (*sql.DB, error) {
	// Escape path for URI - handles paths with ?, #, or other special chars
	// Replace backslashes with forward slashes for Windows compatibility in URI
	escapedPath := url.PathEscape(strings.ReplaceAll(d.path, "\\", "/"))

	// Use URI syntax with read-only mode and busy timeout
	// Setting _busy_timeout via DSN is more reliable than PRAGMA
	dsn := fmt.Sprintf("file:%s?mode=ro&_busy_timeout=5000", escapedPath)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open kiro database: %w", err)
	}

	// Configure connection pool - SQLite best practice
	db.SetMaxOpenConns(1)

	// Ping to ensure connection works
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, classifyError(err)
	}

	// Verify schema
	if err := d.verifySchema(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

// verifySchema checks that the conversations_v2 table exists.
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
		code := sqliteErr.Code()
		switch code {
		case 5, 6: // SQLITE_BUSY=5, SQLITE_LOCKED=6
			return fmt.Errorf("%w: %v", ErrDatabaseLocked, err)
		case 8, 3: // SQLITE_READONLY=8, SQLITE_PERM=3
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
