package logs

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDBPath_ReturnsExpectedPath(t *testing.T) {
	// DBPath checks if the file exists, so we can only verify the error message
	// which contains the expected path. This test documents the expected path format.
	_, err := DBPath()

	// If Kiro is installed, we get the path; otherwise we get ErrDatabaseNotFound
	if err != nil {
		assert.ErrorIs(t, err, ErrDatabaseNotFound)
		// The error message should contain the expected path
		errMsg := err.Error()

		switch runtime.GOOS {
		case "darwin":
			assert.Contains(t, errMsg, "Library/Application Support/kiro-cli/data.sqlite3")
		case "linux":
			assert.Contains(t, errMsg, ".local/share/kiro-cli/data.sqlite3")
		case "windows":
			assert.Contains(t, errMsg, "kiro-cli")
			assert.Contains(t, errMsg, "data.sqlite3")
		}
	}
}

func TestDBPath_UnsupportedOS(t *testing.T) {
	// We can't easily test this without mocking runtime.GOOS,
	// but we can document the expected behavior: it returns an error
	// for unsupported operating systems.
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		_, err := DBPath()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported operating system")
	}
}

func TestNormalizePath_AbsolutePath(t *testing.T) {
	// Use a path that exists to ensure EvalSymlinks works
	tmpDir := t.TempDir()

	normalized, _, err := normalizePath(tmpDir)
	require.NoError(t, err)

	// Should be cleaned and absolute
	assert.True(t, filepath.IsAbs(normalized))
	assert.Equal(t, filepath.Clean(normalized), normalized)
}

func TestNormalizePath_RelativePath(t *testing.T) {
	// Create a temp directory and use relative path to it
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	require.NoError(t, err)

	// Change to temp dir's parent
	err = os.Chdir(filepath.Dir(tmpDir))
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	relPath := "./" + filepath.Base(tmpDir)
	normalized, resolved, err := normalizePath(relPath)
	require.NoError(t, err)

	assert.True(t, filepath.IsAbs(normalized), "normalized path should be absolute")
	// resolved is either empty or different from normalized (if parent path has symlinks)
	_ = resolved
}

func TestNormalizePath_TrailingSlash(t *testing.T) {
	tmpDir := t.TempDir()
	pathWithSlash := tmpDir + string(os.PathSeparator)

	normalized, _, err := normalizePath(pathWithSlash)
	require.NoError(t, err)

	// filepath.Clean removes trailing slash
	assert.False(t, normalized[len(normalized)-1] == os.PathSeparator)
}

func TestNormalizePath_Symlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Symlink tests unreliable on Windows")
	}

	// Create real directory and symlink
	tmpDir := t.TempDir()
	realDir := filepath.Join(tmpDir, "real")
	linkDir := filepath.Join(tmpDir, "link")

	err := os.Mkdir(realDir, 0o755)
	require.NoError(t, err)

	err = os.Symlink(realDir, linkDir)
	require.NoError(t, err)

	// Normalize the symlink path
	normalized, resolved, err := normalizePath(linkDir)
	require.NoError(t, err)

	// normalized should be the symlink path (cleaned)
	assert.Contains(t, normalized, "link")

	// resolved should be the real path
	assert.NotEmpty(t, resolved)
	assert.Contains(t, resolved, "real")
}

func TestNormalizePath_SymlinkSameAsNormalized(t *testing.T) {
	// When a path is not a symlink, resolved should be empty
	tmpDir := t.TempDir()

	normalized, resolved, err := normalizePath(tmpDir)
	require.NoError(t, err)

	// For a non-symlink, resolved should be empty (EvalSymlinks returns same path)
	// The implementation returns "" when normalized == resolved
	if resolved != "" {
		// If resolved is non-empty, it should differ from normalized
		// (this would mean tmpDir itself contains symlinks in parent path)
		assert.NotEqual(t, normalized, resolved)
	}
}

func TestNormalizePath_BrokenSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Symlink tests unreliable on Windows")
	}

	tmpDir := t.TempDir()
	brokenLink := filepath.Join(tmpDir, "broken")

	// Create symlink to non-existent target
	err := os.Symlink(filepath.Join(tmpDir, "nonexistent"), brokenLink)
	require.NoError(t, err)

	// normalizePath should handle broken symlink gracefully
	normalized, resolved, err := normalizePath(brokenLink)
	require.NoError(t, err)

	// Should return the normalized path even if symlink resolution fails
	assert.NotEmpty(t, normalized)
	assert.Empty(t, resolved) // Resolution failed, so empty
}
