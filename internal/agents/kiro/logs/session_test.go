package logs

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSession_ReturnsJSON(t *testing.T) {
	db := createTestDB(t)

	testJSON := `{"conversation_id":"test-session","history":[{"role":"user","content":"hello"}]}`
	insertSession(t, db, "/test/project", "test-session", testJSON, time.Now())

	reader, err := db.GetSession(context.Background(), "test-session", "/test/project")
	require.NoError(t, err)
	require.NotNil(t, reader)

	content, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.JSONEq(t, testJSON, string(content))
}

func TestGetSession_NotFound(t *testing.T) {
	db := createTestDB(t)

	// Insert a session in a different directory
	insertSession(t, db, "/other/project", "existing-session", `{}`, time.Now())

	// Try to get a non-existent session
	_, err := db.GetSession(context.Background(), "nonexistent", "/test/project")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSessionNotFound)
	assert.Contains(t, err.Error(), "nonexistent")
	assert.Contains(t, err.Error(), "/test/project")
}

func TestGetSession_WrongDirectory(t *testing.T) {
	db := createTestDB(t)

	// Insert session in one directory
	insertSession(t, db, "/project-a", "session-1", `{}`, time.Now())

	// Try to get it from a different directory
	_, err := db.GetSession(context.Background(), "session-1", "/project-b")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestGetSession_SymlinkResolution(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Symlink tests unreliable on Windows")
	}

	db := createTestDB(t)

	// Create real directory and symlink
	tmpDir := t.TempDir()
	realDir := filepath.Join(tmpDir, "real")
	linkDir := filepath.Join(tmpDir, "link")

	err := os.Mkdir(realDir, 0o755)
	require.NoError(t, err)
	err = os.Symlink(realDir, linkDir)
	require.NoError(t, err)

	// Resolve real path to handle /tmp -> /private/tmp on macOS
	resolvedRealDir, err := filepath.EvalSymlinks(realDir)
	require.NoError(t, err)

	// Insert session under real path
	testJSON := `{"conversation_id":"session-via-real"}`
	insertSession(t, db, resolvedRealDir, "session-via-real", testJSON, time.Now())

	// Retrieve via symlink path
	reader, err := db.GetSession(context.Background(), "session-via-real", linkDir)
	require.NoError(t, err)

	content, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.JSONEq(t, testJSON, string(content))
}

func TestGetSession_PathNormalization(t *testing.T) {
	db := createTestDB(t)

	// Create a real directory
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "project")
	err := os.Mkdir(projectDir, 0o755)
	require.NoError(t, err)

	// Resolve path to handle platform symlinks
	resolvedDir, err := filepath.EvalSymlinks(projectDir)
	require.NoError(t, err)

	testJSON := `{"conversation_id":"session-1"}`
	insertSession(t, db, resolvedDir, "session-1", testJSON, time.Now())

	// Query with trailing slash
	reader, err := db.GetSession(context.Background(), "session-1", projectDir+string(os.PathSeparator))
	require.NoError(t, err)

	content, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.JSONEq(t, testJSON, string(content))
}

func TestGetSession_ReaderIsReusable(t *testing.T) {
	db := createTestDB(t)

	testJSON := `{"test":"data"}`
	insertSession(t, db, "/test", "session-1", testJSON, time.Now())

	reader, err := db.GetSession(context.Background(), "session-1", "/test")
	require.NoError(t, err)

	// Read the content
	content1, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, testJSON, string(content1))

	// bytes.Reader supports Seek, so we can reset and read again
	seeker, ok := reader.(io.Seeker)
	require.True(t, ok, "reader should be a Seeker")

	_, err = seeker.Seek(0, io.SeekStart)
	require.NoError(t, err)

	content2, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, testJSON, string(content2))
}

func TestGetSession_LargeJSON(t *testing.T) {
	db := createTestDB(t)

	// Create a larger JSON payload
	largeContent := `{"conversation_id":"large-session","history":[`
	for i := range 100 {
		if i > 0 {
			largeContent += ","
		}
		largeContent += `{"role":"user","content":"message ` + string(rune('0'+i%10)) + `"}`
	}
	largeContent += `]}`

	insertSession(t, db, "/test", "large-session", largeContent, time.Now())

	reader, err := db.GetSession(context.Background(), "large-session", "/test")
	require.NoError(t, err)

	content, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, largeContent, string(content))
}

func TestGetSession_EmptyJSON(t *testing.T) {
	db := createTestDB(t)

	// Insert session with empty JSON
	insertSession(t, db, "/test", "empty-session", `{}`, time.Now())

	reader, err := db.GetSession(context.Background(), "empty-session", "/test")
	require.NoError(t, err)

	content, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, `{}`, string(content))
}

func TestGetSession_InvalidDB(t *testing.T) {
	db := NewTestDB("/nonexistent/db.sqlite3")

	_, err := db.GetSession(context.Background(), "any-session", "/any/dir")
	require.Error(t, err)
}

func TestGetSession_MissingSchema(t *testing.T) {
	db := createTestDBWithoutSchema(t)

	_, err := db.GetSession(context.Background(), "any-session", "/any/dir")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSchemaInvalid)
}
