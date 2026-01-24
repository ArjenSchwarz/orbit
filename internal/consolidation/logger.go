package consolidation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/google/uuid"
)

const (
	// SchemaVersion is the current version of the consolidation log format.
	SchemaVersion = "1"

	// logFileName is the name of the consolidation log file.
	logFileName = "consolidation-log.json"

	// lockFileName is the name of the lock file for concurrent access.
	lockFileName = "consolidation-log.lock"
)

// LogEntry represents a single consolidation attempt.
type LogEntry struct {
	SchemaVersion         string    `json:"schema_version"`
	Timestamp             time.Time `json:"timestamp"`
	ChosenVariantID       int       `json:"chosen_variant_id"`
	CommitSHA             string    `json:"commit_sha,omitempty"`
	Agent                 string    `json:"agent"`
	ReportFile            string    `json:"report_file,omitempty"`
	ImprovementsAttempted int       `json:"improvements_attempted"`
	ImprovementsApplied   int       `json:"improvements_applied"`
	ImprovementsSkipped   int       `json:"improvements_skipped"`
	TestsPassed           bool      `json:"tests_passed"`
	PostCommandPassed     bool      `json:"post_command_passed"`
	Errors                []string  `json:"errors,omitempty"`
}

// ConsolidationLog is the root structure for the log file.
type ConsolidationLog struct {
	SchemaVersion string     `json:"schema_version"`
	Entries       []LogEntry `json:"entries"`
}

// Logger manages the consolidation-log.json file.
type Logger struct {
	orbitDir string
	logPath  string
	lockPath string
}

// NewLogger creates a logger for a spec's .orbit directory.
func NewLogger(orbitDir string) *Logger {
	return &Logger{
		orbitDir: orbitDir,
		logPath:  filepath.Join(orbitDir, logFileName),
		lockPath: filepath.Join(orbitDir, lockFileName),
	}
}

// Append adds a new entry to the log with file locking for concurrent safety.
func (l *Logger) Append(entry LogEntry) error {
	// Ensure the directory exists
	if err := os.MkdirAll(l.orbitDir, 0755); err != nil {
		return fmt.Errorf("failed to create orbit directory: %w", err)
	}

	// Acquire exclusive lock using flock on the lock file.
	// We don't delete the lock file to avoid race conditions with concurrent processes.
	lockFile, err := os.OpenFile(l.lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("failed to open lock file: %w", err)
	}
	defer func() {
		_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		_ = lockFile.Close()
	}()

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}

	// Read existing log or create new one
	log, err := l.readLog()
	if err != nil {
		// File doesn't exist or is corrupt - start fresh
		log = &ConsolidationLog{
			SchemaVersion: SchemaVersion,
			Entries:       []LogEntry{},
		}
	}

	// Set schema version on entry
	entry.SchemaVersion = SchemaVersion

	// Append new entry
	log.Entries = append(log.Entries, entry)

	// Write atomically (temp file + rename)
	return l.writeLogAtomic(log)
}

// SaveReport saves the agent's report to a timestamped markdown file.
// Returns the file path for reference in the log entry.
func (l *Logger) SaveReport(report string) (string, error) {
	// Ensure the directory exists
	if err := os.MkdirAll(l.orbitDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create orbit directory: %w", err)
	}

	timestamp := time.Now().Format("2006-01-02T15-04-05")
	filename := fmt.Sprintf("consolidation-report-%s.md", timestamp)
	filePath := filepath.Join(l.orbitDir, filename)

	if err := os.WriteFile(filePath, []byte(report), 0644); err != nil {
		return "", fmt.Errorf("failed to write report: %w", err)
	}

	return filename, nil
}

// GetLatestCommitSHA returns the commit SHA from the most recent log entry.
// Used by Rollback as the primary mechanism to find the consolidation commit.
func (l *Logger) GetLatestCommitSHA() (string, error) {
	log, err := l.readLog()
	if err != nil {
		return "", fmt.Errorf("failed to read consolidation log: %w", err)
	}

	if len(log.Entries) == 0 {
		return "", fmt.Errorf("no consolidation entries found")
	}

	// Return the most recent entry's commit SHA
	latestEntry := log.Entries[len(log.Entries)-1]
	if latestEntry.CommitSHA == "" {
		return "", fmt.Errorf("latest consolidation entry has no commit SHA")
	}

	return latestEntry.CommitSHA, nil
}

// Read returns the full consolidation log file.
// Used to access variant IDs and other metadata from rollback mode,
// enabling the CLI to infer variant ID from the last consolidation entry
// when --variant flag is not provided.
func (l *Logger) Read() (*ConsolidationLog, error) {
	log, err := l.readLog()
	if err != nil {
		return nil, fmt.Errorf("failed to read consolidation log: %w", err)
	}
	return log, nil
}

// readLog reads the existing consolidation log from disk.
func (l *Logger) readLog() (*ConsolidationLog, error) {
	data, err := os.ReadFile(l.logPath)
	if err != nil {
		return nil, err
	}

	var log ConsolidationLog
	if err := json.Unmarshal(data, &log); err != nil {
		return nil, fmt.Errorf("failed to parse consolidation log: %w", err)
	}

	return &log, nil
}

// writeLogAtomic writes the log to disk using a temp file + rename pattern.
func (l *Logger) writeLogAtomic(log *ConsolidationLog) error {
	data, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal log: %w", err)
	}

	// Write to temp file with unique name to avoid conflicts
	tempPath := fmt.Sprintf("%s.%s.tmp", l.logPath, uuid.NewString()[:8])
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Rename temp file to final path (atomic on POSIX)
	if err := os.Rename(tempPath, l.logPath); err != nil {
		_ = os.Remove(tempPath) // Clean up temp file on failure
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}
