package logs

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"time"
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
	record, err := d.GetSessionRecord(ctx, conversationID, dir)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(record.Value), nil
}

// SessionRecord is a stored conversation with the database fields that
// describe it.
type SessionRecord struct {
	Value     []byte    // Conversation JSON
	Key       string    // The key (working directory) that matched
	CreatedAt time.Time // Recorded creation time
}

// GetSessionRecord retrieves the conversation for the given session ID and
// directory, reporting which key matched: the normalized path is tried first,
// then the symlink-resolved path if different.
//
// This is a convenience function that uses DefaultDB().
func GetSessionRecord(ctx context.Context, conversationID, dir string) (*SessionRecord, error) {
	db, err := DefaultDB()
	if err != nil {
		return nil, err
	}
	return db.GetSessionRecord(ctx, conversationID, dir)
}

// GetSessionRecord retrieves the conversation for the given session ID and
// directory, reporting which key matched: the normalized path is tried first,
// then the symlink-resolved path if different.
func (d *DB) GetSessionRecord(ctx context.Context, conversationID, dir string) (*SessionRecord, error) {
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
	record, err := querySessionRecord(ctx, conn, conversationID, normalized)
	if err == nil {
		return record, nil
	}
	if !errors.Is(err, ErrSessionNotFound) {
		return nil, err
	}

	// Try symlink-resolved path if different
	if resolved != "" {
		record, err = querySessionRecord(ctx, conn, conversationID, resolved)
		if err == nil {
			return record, nil
		}
	}

	return nil, fmt.Errorf("%w: %s in directory %s", ErrSessionNotFound, conversationID, dir)
}

// GetSessionByID retrieves a conversation by ID from any directory.
//
// This is a convenience function that uses DefaultDB().
func GetSessionByID(ctx context.Context, conversationID string) (*SessionRecord, error) {
	db, err := DefaultDB()
	if err != nil {
		return nil, err
	}
	return db.GetSessionByID(ctx, conversationID)
}

// GetSessionByID retrieves a conversation by ID from any directory. When the
// conversation is stored under several keys, it returns the most recently
// updated row and then the lowest key, the row DiscoverAll lists.
func (d *DB) GetSessionByID(ctx context.Context, conversationID string) (*SessionRecord, error) {
	conn, err := d.openConn(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()

	var createdMS int64
	record := &SessionRecord{}
	err = conn.QueryRowContext(ctx, `
		SELECT key, value, created_at FROM conversations_v2
		WHERE conversation_id = ?
		ORDER BY updated_at DESC, key ASC
		LIMIT 1
	`, conversationID).Scan(&record.Key, &record.Value, &createdMS)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: %s", ErrSessionNotFound, conversationID)
	}
	if err != nil {
		return nil, classifyError(err)
	}

	record.CreatedAt = time.UnixMilli(createdMS)
	return record, nil
}

// querySessionRecord retrieves the stored record for a specific session and key.
func querySessionRecord(ctx context.Context, conn *sql.DB, conversationID, dir string) (*SessionRecord, error) {
	var createdMS int64
	record := &SessionRecord{Key: dir}
	err := conn.QueryRowContext(ctx, `
		SELECT value, created_at FROM conversations_v2
		WHERE conversation_id = ? AND key = ?
	`, conversationID, dir).Scan(&record.Value, &createdMS)

	if err == sql.ErrNoRows {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, classifyError(err)
	}

	record.CreatedAt = time.UnixMilli(createdMS)
	return record, nil
}
