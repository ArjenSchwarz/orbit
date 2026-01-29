// Package debug provides debug logging utilities for Orbit.
package debug

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileWriter handles thread-safe JSON Lines file output.
// Satisfies requirements 1.1-1.7, 9.1-9.4, 10.1-10.2
type FileWriter struct {
	file            *os.File
	mu              sync.Mutex
	path            string
	lastWarningTime time.Time
	warningInterval time.Duration
	closed          bool
}

// NewFileWriter creates a writer for the given run.
// Creates ~/.orbit/logs/ if needed (Req 1.2).
// Generates filename: {timestamp}-{runID}.jsonl (Req 1.3)
// Returns error if runID is empty.
// Returns (nil, nil) on directory/file creation failure - not an error, just disabled.
func NewFileWriter(runID string) (*FileWriter, error) {
	if runID == "" {
		return nil, fmt.Errorf("runID is required for centralized logging")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("Warning: failed to get home directory, centralized logging disabled: %v", err)
		return nil, nil
	}

	logDir := filepath.Join(homeDir, ".orbit", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("Warning: failed to create log directory, centralized logging disabled: %v", err)
		return nil, nil
	}

	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("%s-%s.jsonl", timestamp, runID)
	path := filepath.Join(logDir, filename)

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		log.Printf("Warning: failed to create log file, centralized logging disabled: %v", err)
		return nil, nil
	}

	return &FileWriter{
		file:            file,
		path:            path,
		warningInterval: 10 * time.Second,
	}, nil
}

// NewVariantFileWriter creates a writer for a variant run.
// Generates filename: {timestamp}-{runID}-variant-{N}.jsonl (Req 1.5)
// Returns error if runID is empty or variantNum < 1.
// Returns (nil, nil) on directory/file creation failure.
func NewVariantFileWriter(runID string, variantNum int) (*FileWriter, error) {
	if runID == "" {
		return nil, fmt.Errorf("runID is required for centralized logging")
	}
	if variantNum < 1 {
		return nil, fmt.Errorf("variantNum must be >= 1")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("Warning: failed to get home directory, centralized logging disabled: %v", err)
		return nil, nil
	}

	logDir := filepath.Join(homeDir, ".orbit", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("Warning: failed to create log directory, centralized logging disabled: %v", err)
		return nil, nil
	}

	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("%s-%s-variant-%d.jsonl", timestamp, runID, variantNum)
	path := filepath.Join(logDir, filename)

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		log.Printf("Warning: failed to create variant log file, centralized logging disabled: %v", err)
		return nil, nil
	}

	return &FileWriter{
		file:            file,
		path:            path,
		warningInterval: 10 * time.Second,
	}, nil
}

// Write serializes and writes an entry with mutex protection.
// Flushes after each write (Req 10.2).
// Returns nil on failure (Req 9.1), emits rate-limited warning (Req 9.2).
// Safe to call on nil receiver.
func (w *FileWriter) Write(entry any) error {
	if w == nil {
		return nil
	}

	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}

	data, err := json.Marshal(entry)
	if err != nil {
		warning := w.checkWarningLocked("failed to marshal log entry: %v", err)
		w.mu.Unlock()
		if warning != "" {
			log.Print(warning)
		}
		return nil
	}

	if _, err := w.file.Write(append(data, '\n')); err != nil {
		warning := w.checkWarningLocked("failed to write log entry: %v", err)
		w.mu.Unlock()
		if warning != "" {
			log.Print(warning)
		}
		return nil
	}

	if err := w.file.Sync(); err != nil {
		warning := w.checkWarningLocked("failed to flush log entry: %v", err)
		w.mu.Unlock()
		if warning != "" {
			log.Print(warning)
		}
		return nil
	}

	w.mu.Unlock()
	return nil
}

// checkWarningLocked determines if a warning should be emitted.
// Returns the warning message if rate limit allows, empty string otherwise.
// MUST be called with w.mu held. Does NOT emit the warning (caller does after unlock).
func (w *FileWriter) checkWarningLocked(format string, args ...any) string {
	now := time.Now()
	if now.Sub(w.lastWarningTime) >= w.warningInterval {
		w.lastWarningTime = now
		return fmt.Sprintf("Warning: "+format, args...)
	}
	return ""
}

// Path returns the absolute path to the log file.
// Returns empty string if writer is nil.
func (w *FileWriter) Path() string {
	if w == nil {
		return ""
	}
	return w.path
}

// Close flushes and closes the file.
// Safe to call on nil receiver.
func (w *FileWriter) Close() error {
	if w == nil {
		return nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}

	w.closed = true

	if w.file == nil {
		return nil
	}

	// Sync before close for durability
	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("failed to sync file before close: %w", err)
	}

	if err := w.file.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}

	return nil
}
