// Package debug provides debug logging utilities for Orbit.
package debug

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// Logger provides conditional debug logging with optional file output.
// Extended from existing implementation to satisfy Req 7.1-7.7
type Logger struct {
	stderrEnabled bool        // Controlled by --debug (Req 7.2)
	fileEnabled   bool        // Controlled by --centralized-log (Req 7.4)
	prefix        string      // Component name (used for both stderr prefix and JSON component)
	fileWriter    *FileWriter // Centralized log writer (may be nil)
	startTime     time.Time   // For shutdown duration calculation
	shutdownDone  bool        // Prevents double shutdown entry
	mu            sync.Mutex  // Protects shutdownDone

	// Legacy field for backward compatibility
	enabled bool // Deprecated: use stderrEnabled
}

// LoggerConfig configures Logger creation.
type LoggerConfig struct {
	StderrEnabled bool   // --debug flag
	FileEnabled   bool   // --centralized-log flag (default: true)
	RunID         string // UUID for filename (required if FileEnabled)
	VariantNum    int    // 0 for main, 1+ for variants
	Prefix        string // Component name for this logger instance
}

// New creates a new debug logger.
// Deprecated: Use NewLogger for new code that needs file output.
func New(enabled bool, prefix string) *Logger {
	return &Logger{
		enabled:       enabled,
		stderrEnabled: enabled,
		prefix:        prefix,
	}
}

// NewLogger creates a logger with configured outputs.
// If FileEnabled but writer creation fails, continues with file logging disabled.
func NewLogger(cfg LoggerConfig) (*Logger, error) {
	l := &Logger{
		stderrEnabled: cfg.StderrEnabled,
		fileEnabled:   cfg.FileEnabled,
		prefix:        cfg.Prefix,
		startTime:     time.Now(),
		// Also set legacy field for backward compatibility
		enabled: cfg.StderrEnabled,
	}

	if cfg.FileEnabled && cfg.RunID != "" {
		var err error
		if cfg.VariantNum > 0 {
			l.fileWriter, err = NewVariantFileWriter(cfg.RunID, cfg.VariantNum)
		} else {
			l.fileWriter, err = NewFileWriter(cfg.RunID)
		}
		if err != nil {
			// Empty RunID is a programming error - propagate it
			return nil, err
		}
		// Note: fileWriter may still be nil if directory/file creation failed,
		// but that's not an error - just means file logging is disabled
	}

	return l, nil
}

// Enabled returns whether stderr logging is enabled.
func (l *Logger) Enabled() bool {
	if l == nil {
		return false
	}
	return l.stderrEnabled || l.enabled
}

// Log logs a debug message. EXISTING SIGNATURE preserved.
// Internally converts to structured entry for file output.
func (l *Logger) Log(format string, args ...any) {
	if l == nil {
		return
	}

	msg := fmt.Sprintf(format, args...)

	// Stderr output (existing behavior, unchanged)
	if l.stderrEnabled || l.enabled {
		l.logToStderr("%s", msg)
	}

	// File output - message only, no structured fields
	// (callers should migrate to LogStructured for richer output)
	if l.fileEnabled && l.fileWriter != nil {
		_ = l.fileWriter.Write(LogEntry{
			Timestamp: time.Now(),
			Level:     "debug",
			Component: l.prefix,
			Message:   msg,
		})
	}
}

// LogCmd logs command execution details. EXISTING SIGNATURE.
func (l *Logger) LogCmd(name string, args []string, workingDir string) {
	if l == nil {
		return
	}

	cmd := name + " " + strings.Join(args, " ")

	// Stderr output (existing behavior, unchanged)
	if l.stderrEnabled || l.enabled {
		l.logToStderr("Executing: %s", cmd)
		if workingDir != "" {
			l.logToStderr("Working dir: %s", workingDir)
		}
	}

	// File output (new structured format)
	if l.fileEnabled && l.fileWriter != nil {
		_ = l.fileWriter.Write(LogEntry{
			Timestamp: time.Now(),
			Level:     "debug",
			Component: l.prefix,
			Message:   "Command execution",
			Fields: map[string]any{
				"command":     cmd,
				"working_dir": workingDir,
			},
		})
	}
}

// LogCmdResult logs the result of a command execution. EXISTING SIGNATURE.
func (l *Logger) LogCmdResult(exitCode int, stdout, stderr string, duration time.Duration) {
	if l == nil {
		return
	}

	// Stderr output (existing behavior, unchanged)
	if l.stderrEnabled || l.enabled {
		l.logToStderr("Exit code: %d (duration: %s)", exitCode, duration)
		if len(stdout) > 0 {
			preview := truncate(stdout, 500)
			l.logToStderr("Stdout (%d bytes): %s", len(stdout), preview)
		} else {
			l.logToStderr("Stdout: (empty)")
		}
		if len(stderr) > 0 {
			preview := truncate(stderr, 500)
			l.logToStderr("Stderr (%d bytes): %s", len(stderr), preview)
		}
	}

	// File output (structured)
	if l.fileEnabled && l.fileWriter != nil {
		_ = l.fileWriter.Write(LogEntry{
			Timestamp: time.Now(),
			Level:     "debug",
			Component: l.prefix,
			Message:   "Command result",
			Fields: map[string]any{
				"exit_code":    exitCode,
				"duration":     duration.String(),
				"stdout_bytes": len(stdout),
				"stderr_bytes": len(stderr),
			},
		})
	}
}

// LogJSON logs JSON parsing results. EXISTING SIGNATURE.
func (l *Logger) LogJSON(success bool, parseErr error) {
	if l == nil {
		return
	}

	// Stderr output (existing behavior, unchanged)
	if l.stderrEnabled || l.enabled {
		if success {
			l.logToStderr("JSON parsed successfully")
		} else {
			l.logToStderr("JSON parse failed: %v", parseErr)
		}
	}

	// File output (structured)
	if l.fileEnabled && l.fileWriter != nil {
		fields := map[string]any{"success": success}
		if parseErr != nil {
			fields["error"] = parseErr.Error()
		}
		_ = l.fileWriter.Write(LogEntry{
			Timestamp: time.Now(),
			Level:     "debug",
			Component: l.prefix,
			Message:   "JSON parsing",
			Fields:    fields,
		})
	}
}

// LogSession logs session-related information. EXISTING SIGNATURE.
func (l *Logger) LogSession(sessionID string, isResume bool, action string) {
	if l == nil {
		return
	}

	mode := "new"
	if isResume {
		mode = "resume"
	}

	// Stderr output (existing behavior, unchanged)
	if l.stderrEnabled || l.enabled {
		l.logToStderr("Session %s: id=%s mode=%s", action, sessionID, mode)
	}

	// File output (structured)
	if l.fileEnabled && l.fileWriter != nil {
		_ = l.fileWriter.Write(LogEntry{
			Timestamp: time.Now(),
			Level:     "debug",
			Component: l.prefix,
			Message:   "Session " + action,
			Fields: map[string]any{
				"session_id": sessionID,
				"mode":       mode,
				"action":     action,
			},
		})
	}
}

// LogRetry logs retry-related information. EXISTING SIGNATURE.
func (l *Logger) LogRetry(attempt, maxAttempts int, errType, waitDuration string) {
	if l == nil {
		return
	}

	// Stderr output (existing behavior)
	if l.stderrEnabled || l.enabled {
		l.logToStderr("Retry %d/%d: error_type=%s wait=%s", attempt, maxAttempts, errType, waitDuration)
	}

	// File output (structured)
	if l.fileEnabled && l.fileWriter != nil {
		_ = l.fileWriter.Write(LogEntry{
			Timestamp: time.Now(),
			Level:     "info",
			Component: "retry",
			Message:   "Retry attempt",
			Fields: map[string]any{
				"attempt":       attempt,
				"max_attempts":  maxAttempts,
				"error_type":    errType,
				"wait_duration": waitDuration,
			},
		})
	}
}

// LogConfig logs configuration values. EXISTING SIGNATURE.
func (l *Logger) LogConfig(key string, value any) {
	if l == nil {
		return
	}

	// Stderr output (existing behavior)
	if l.stderrEnabled || l.enabled {
		l.logToStderr("Config %s = %v", key, value)
	}

	// File output (structured)
	if l.fileEnabled && l.fileWriter != nil {
		_ = l.fileWriter.Write(LogEntry{
			Timestamp: time.Now(),
			Level:     "debug",
			Component: "config",
			Message:   "Configuration loaded",
			Fields: map[string]any{
				"key":   key,
				"value": value,
			},
		})
	}
}

// LogError logs error classification details. EXISTING SIGNATURE.
func (l *Logger) LogError(errType string, message string, retryable bool) {
	if l == nil {
		return
	}

	// Stderr output (existing behavior)
	if l.stderrEnabled || l.enabled {
		l.logToStderr("Error classified: type=%s retryable=%v message=%s", errType, retryable, truncate(message, 200))
	}

	// File output (structured)
	if l.fileEnabled && l.fileWriter != nil {
		_ = l.fileWriter.Write(LogEntry{
			Timestamp: time.Now(),
			Level:     "error",
			Component: l.prefix,
			Message:   "Error classified",
			Fields: map[string]any{
				"error_type": errType,
				"message":    message,
				"retryable":  retryable,
			},
		})
	}
}

// truncate shortens a string to the given max length.
func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// --- NEW METHODS for structured logging ---

// logToStderr is the internal method for stderr output (existing format).
func (l *Logger) logToStderr(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if l.prefix != "" {
		log.Printf("[DEBUG:%s] %s", l.prefix, msg)
	} else {
		log.Printf("[DEBUG] %s", msg)
	}
}

// LogStructured writes a structured log entry with explicit fields.
// Use this for new code that needs fine-grained control over fields.
func (l *Logger) LogStructured(level, message string, fields map[string]any) {
	if l == nil {
		return
	}

	if l.stderrEnabled {
		l.logToStderr("%s", message)
	}

	if l.fileEnabled && l.fileWriter != nil {
		_ = l.fileWriter.Write(LogEntry{
			Timestamp: time.Now(),
			Level:     level,
			Component: l.prefix,
			Message:   message,
			Fields:    fields,
		})
	}
}

// extractErrorChain builds the error_chain array from wrapped errors.
// Satisfies Req 3.8.
func extractErrorChain(err error) []string {
	if err == nil {
		return nil
	}
	var chain []string
	for e := err; e != nil; {
		chain = append(chain, e.Error())
		// Try to unwrap - handles both errors.Unwrap and interface { Unwrap() error }
		if unwrapper, ok := e.(interface{ Unwrap() error }); ok {
			e = unwrapper.Unwrap()
		} else {
			break
		}
	}
	return chain
}

// LogErrorWithChain logs an error with the full wrapped error chain.
// Satisfies Req 3.8.
func (l *Logger) LogErrorWithChain(message string, err error, fields map[string]any) {
	if l == nil {
		return
	}

	if fields == nil {
		fields = make(map[string]any)
	}
	if err != nil {
		fields["error"] = err.Error()
		fields["error_chain"] = extractErrorChain(err)
	}

	if l.stderrEnabled {
		l.logToStderr("Error: %s: %v", message, err)
	}

	if l.fileEnabled && l.fileWriter != nil {
		_ = l.fileWriter.Write(LogEntry{
			Timestamp: time.Now(),
			Level:     "error",
			Component: l.prefix,
			Message:   message,
			Fields:    fields,
		})
	}
}

// LogStartup writes the startup entry. Called once at orchestration start.
// The StartupConfig provides metadata that appears in the first log entry.
func (l *Logger) LogStartup(cfg StartupConfig) {
	if l == nil || l.fileWriter == nil {
		return
	}

	_ = l.fileWriter.Write(StartupEntry{
		Timestamp:        time.Now(),
		Level:            "info",
		Component:        "orchestrator",
		Message:          "Orchestration started",
		SchemaVersion:    1,
		OrbitVersion:     cfg.OrbitVersion,
		Agent:            cfg.Agent,
		TasksFile:        cfg.TasksFile,
		WorkingDirectory: cfg.WorkingDirectory,
		BranchName:       cfg.BranchName,
	})
}

// LogShutdown writes the shutdown entry. Called on normal completion.
// Safe to call multiple times (only writes once).
func (l *Logger) LogShutdown(status string) {
	if l == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.shutdownDone {
		return
	}

	if l.fileWriter != nil {
		_ = l.fileWriter.Write(ShutdownEntry{
			Timestamp:     time.Now(),
			Level:         "info",
			Component:     "orchestrator",
			Message:       "Orchestration completed",
			TotalDuration: time.Since(l.startTime).String(),
			FinalStatus:   status,
		})
		l.shutdownDone = true
	}
}

// Close writes shutdown entry if not already written, closes file writer.
// Hooks into signal handling for graceful shutdown.
func (l *Logger) Close() {
	if l == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Write shutdown entry if not already done
	if !l.shutdownDone && l.fileWriter != nil {
		_ = l.fileWriter.Write(ShutdownEntry{
			Timestamp:     time.Now(),
			Level:         "info",
			Component:     "orchestrator",
			Message:       "Orchestration shutdown",
			TotalDuration: time.Since(l.startTime).String(),
			FinalStatus:   "interrupted",
		})
		l.shutdownDone = true
	}

	if l.fileWriter != nil {
		_ = l.fileWriter.Close()
	}
}

// Path returns the centralized log file path (empty if disabled or nil writer).
func (l *Logger) Path() string {
	if l == nil || l.fileWriter == nil {
		return ""
	}
	return l.fileWriter.Path()
}
