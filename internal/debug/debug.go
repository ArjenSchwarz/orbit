// Package debug provides debug logging utilities for Orbit.
package debug

import (
	"fmt"
	"log"
	"strings"
	"time"
)

// Logger provides conditional debug logging.
type Logger struct {
	enabled bool
	prefix  string
}

// New creates a new debug logger.
func New(enabled bool, prefix string) *Logger {
	return &Logger{
		enabled: enabled,
		prefix:  prefix,
	}
}

// Enabled returns whether debug logging is enabled.
func (l *Logger) Enabled() bool {
	if l == nil {
		return false
	}
	return l.enabled
}

// Log logs a debug message if debug mode is enabled.
func (l *Logger) Log(format string, args ...any) {
	if l == nil || !l.enabled {
		return
	}
	msg := fmt.Sprintf(format, args...)
	if l.prefix != "" {
		log.Printf("[DEBUG:%s] %s", l.prefix, msg)
	} else {
		log.Printf("[DEBUG] %s", msg)
	}
}

// LogCmd logs command execution details.
func (l *Logger) LogCmd(name string, args []string, workingDir string) {
	if l == nil || !l.enabled {
		return
	}
	cmd := name + " " + strings.Join(args, " ")
	l.Log("Executing: %s", cmd)
	if workingDir != "" {
		l.Log("Working dir: %s", workingDir)
	}
}

// LogCmdResult logs the result of a command execution.
func (l *Logger) LogCmdResult(exitCode int, stdout, stderr string, duration time.Duration) {
	if l == nil || !l.enabled {
		return
	}
	l.Log("Exit code: %d (duration: %s)", exitCode, duration)
	if len(stdout) > 0 {
		preview := truncate(stdout, 500)
		l.Log("Stdout (%d bytes): %s", len(stdout), preview)
	} else {
		l.Log("Stdout: (empty)")
	}
	if len(stderr) > 0 {
		preview := truncate(stderr, 500)
		l.Log("Stderr (%d bytes): %s", len(stderr), preview)
	}
}

// LogJSON logs JSON parsing results.
func (l *Logger) LogJSON(success bool, parseErr error) {
	if l == nil || !l.enabled {
		return
	}
	if success {
		l.Log("JSON parsed successfully")
	} else {
		l.Log("JSON parse failed: %v", parseErr)
	}
}

// LogSession logs session-related information.
func (l *Logger) LogSession(sessionID string, isResume bool, action string) {
	if l == nil || !l.enabled {
		return
	}
	mode := "new"
	if isResume {
		mode = "resume"
	}
	l.Log("Session %s: id=%s mode=%s", action, sessionID, mode)
}

// LogRetry logs retry-related information.
func (l *Logger) LogRetry(attempt, maxAttempts int, errType, waitDuration string) {
	if l == nil || !l.enabled {
		return
	}
	l.Log("Retry %d/%d: error_type=%s wait=%s", attempt, maxAttempts, errType, waitDuration)
}

// LogConfig logs configuration values.
func (l *Logger) LogConfig(key string, value any) {
	if l == nil || !l.enabled {
		return
	}
	l.Log("Config %s = %v", key, value)
}

// LogError logs error classification details.
func (l *Logger) LogError(errType string, message string, retryable bool) {
	if l == nil || !l.enabled {
		return
	}
	l.Log("Error classified: type=%s retryable=%v message=%s", errType, retryable, truncate(message, 200))
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
