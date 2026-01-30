package logs

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
)

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

// GetSession retrieves the conversation JSON for the given session ID and directory.
// The JSON blob is read entirely into memory and returned as a bytes.Reader.
// Tries normalized path first, then symlink-resolved path if different.
func (d *DB) GetSession(ctx context.Context, conversationID, dir string) (io.Reader, error) {
	conn, err := d.openConn(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()

	normalized, resolved, err := normalizePath(dir)
	if err != nil {
		return nil, fmt.Errorf("normalize path: %w", err)
	}

	// Try normalized path first
	data, err := querySessionValue(ctx, conn, conversationID, normalized)
	if err == nil {
		return bytes.NewReader(data), nil
	}
	if !errors.Is(err, ErrSessionNotFound) {
		return nil, err
	}

	// Try symlink-resolved path if different
	if resolved != "" {
		data, err = querySessionValue(ctx, conn, conversationID, resolved)
		if err == nil {
			return bytes.NewReader(data), nil
		}
	}

	return nil, fmt.Errorf("%w: %s in directory %s", ErrSessionNotFound, conversationID, dir)
}

// querySessionValue retrieves the JSON blob for a specific session.
func querySessionValue(ctx context.Context, conn *sql.DB, conversationID, dir string) ([]byte, error) {
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
