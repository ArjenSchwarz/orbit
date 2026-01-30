package logs

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// DBPath returns the OS-specific path to the Kiro SQLite database.
//
// Path conventions by OS:
//   - macOS: ~/Library/Application Support/kiro-cli/data.sqlite3
//   - Linux: ~/.local/share/kiro-cli/data.sqlite3
//   - Windows: %APPDATA%\kiro-cli\data.sqlite3
//
// Returns ErrDatabaseNotFound if the database file does not exist.
func DBPath() (string, error) {
	var base string

	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get home directory: %w", err)
		}
		base = filepath.Join(home, "Library", "Application Support", "kiro-cli")
	case "linux":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get home directory: %w", err)
		}
		base = filepath.Join(home, ".local", "share", "kiro-cli")
	case "windows":
		// Windows uses %APPDATA% (Roaming AppData) for application data
		configDir, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("get config directory: %w", err)
		}
		base = filepath.Join(configDir, "kiro-cli")
	default:
		return "", fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	dbPath := filepath.Join(base, "data.sqlite3")

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return "", fmt.Errorf("%w: expected at %s", ErrDatabaseNotFound, dbPath)
	}

	return dbPath, nil
}

// normalizePath returns the normalized path and optionally the symlink-resolved path.
// The normalized path uses filepath.Abs followed by filepath.Clean.
// If the symlink-resolved path differs from normalized, it is returned as the second value.
// If symlink resolution fails or produces the same path, resolved is empty.
func normalizePath(dir string) (normalized string, resolved string, err error) {
	normalized, err = filepath.Abs(dir)
	if err != nil {
		return "", "", fmt.Errorf("get absolute path: %w", err)
	}
	normalized = filepath.Clean(normalized)

	resolved, err = filepath.EvalSymlinks(normalized)
	if err != nil {
		// Symlink resolution failed (broken link, permission denied, etc.)
		// Fall back to normalized path only
		return normalized, "", nil
	}

	if resolved == normalized {
		return normalized, "", nil // No difference, don't query twice
	}

	return normalized, resolved, nil
}
