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
//   - Linux: $XDG_DATA_HOME/kiro-cli/data.sqlite3 (defaults to ~/.local/share/kiro-cli/data.sqlite3)
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
		// Check XDG_DATA_HOME first per XDG Base Directory Specification
		if xdgData := os.Getenv("XDG_DATA_HOME"); xdgData != "" {
			base = filepath.Join(xdgData, "kiro-cli")
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("get home directory: %w", err)
			}
			base = filepath.Join(home, ".local", "share", "kiro-cli")
		}
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
//
// Note on race conditions: If Kiro creates a session between the two queries,
// the session will be found on the second query. This is acceptable because:
//   - The session is still discovered (just on the fallback query)
//   - The directory association uses whichever path matched
//   - This race is rare in practice (requires active Kiro session during listing)
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
