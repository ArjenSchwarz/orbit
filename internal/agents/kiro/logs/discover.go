package logs

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"
)

// SessionMetadata contains information about a discovered session.
type SessionMetadata struct {
	ConversationID string
	Directory      string    // The working directory (key column)
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Size           int64 // Size of JSON blob in bytes
}

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

// DiscoverForDirectory returns all sessions for the given working directory.
// Queries both normalized and symlink-resolved paths, deduplicating by ConversationID.
// Results are sorted by updated_at DESC (most recent first).
func (d *DB) DiscoverForDirectory(ctx context.Context, dir string) ([]SessionMetadata, error) {
	conn, err := d.openConn(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()

	normalized, resolved, err := normalizePath(dir)
	if err != nil {
		return nil, fmt.Errorf("normalize path: %w", err)
	}

	// Use map for deduplication by ConversationID
	seen := make(map[string]SessionMetadata)

	// Query with normalized path
	if err := querySessions(ctx, conn, normalized, seen); err != nil {
		return nil, err
	}

	// Query with symlink-resolved path if different
	if resolved != "" {
		if err := querySessions(ctx, conn, resolved, seen); err != nil {
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
func querySessions(ctx context.Context, conn *sql.DB, dir string, seen map[string]SessionMetadata) error {
	rows, err := conn.QueryContext(ctx, `
		SELECT conversation_id, key, created_at, updated_at, length(value)
		FROM conversations_v2
		WHERE key = ?
		ORDER BY updated_at DESC
	`, dir)
	if err != nil {
		return classifyError(err)
	}
	defer func() { _ = rows.Close() }()

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
