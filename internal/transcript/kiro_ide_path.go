package transcript

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// executionSavesDir is SHA-256("KIRO::EXECUTION::SAVES")[:32].
// This is the subdirectory within a workspace directory that contains execution detail files.
const executionSavesDir = "414d1636299d2b9e4ce7e17fb11f63e9"

// kiroIDESubdir is the relative path from the user config directory to Kiro IDE storage.
const kiroIDESubdir = "Kiro/User/globalStorage/kiro.kiroagent"

// ErrKiroIDENotFound indicates the Kiro IDE storage directory does not exist.
var ErrKiroIDENotFound = errors.New("kiro ide storage not found")

// sha256Hex32 returns the first 32 hex characters of the SHA-256 hash of input.
func sha256Hex32(input string) string {
	h := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", h)[:32]
}

// KiroIDEBasePath returns the platform-specific base directory for Kiro IDE session storage.
// Uses os.UserConfigDir() and appends the Kiro IDE-specific subdirectory.
// Returns ("", ErrKiroIDENotFound) if the directory does not exist.
func KiroIDEBasePath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", ErrKiroIDENotFound
	}
	base := filepath.Join(configDir, kiroIDESubdir)
	if _, err := os.Stat(base); err != nil {
		return "", ErrKiroIDENotFound
	}
	return base, nil
}

// KiroIDEWorkspaceDir returns the workspace directory for a given project path.
// Normalizes the path (filepath.Abs + filepath.Clean), computes SHA-256[:32],
// and joins it with the base path.
// Returns ("", ErrKiroIDENotFound) if the directory does not exist.
func KiroIDEWorkspaceDir(projectPath string) (string, error) {
	base, err := KiroIDEBasePath()
	if err != nil {
		return "", err
	}
	return kiroIDEWorkspaceDirWithBase(base, projectPath)
}

// kiroIDEWorkspaceDirWithBase is the internal implementation that accepts a base directory.
// Exported tests use this to avoid depending on the real filesystem layout.
func kiroIDEWorkspaceDirWithBase(base, projectPath string) (string, error) {
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return "", ErrKiroIDENotFound
	}
	absPath = filepath.Clean(absPath)

	dir := filepath.Join(base, sha256Hex32(absPath))
	if _, err := os.Stat(dir); err != nil {
		return "", ErrKiroIDENotFound
	}
	return dir, nil
}

// KiroIDEExecutionDetailPath returns the deterministic path to an execution detail file.
// Path: {workspaceDir}/414d1636299d2b9e4ce7e17fb11f63e9/{sha256_32(executionId)}
func KiroIDEExecutionDetailPath(workspaceDir, executionID string) string {
	return filepath.Join(workspaceDir, executionSavesDir, sha256Hex32(executionID))
}
