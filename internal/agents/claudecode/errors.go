package claudecode

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
)

// Classifier implements agents.ErrorClassifier for Claude Code.
type Classifier struct{}

// NewClassifier creates a new Claude Code error classifier.
func NewClassifier() *Classifier {
	return &Classifier{}
}

// Classify examines error output and classifies it by type.
func (c *Classifier) Classify(exitCode int, stderr, stdout string, errMsgs []string) *agents.ClassifiedError {
	// Combine all error sources for pattern matching
	combined := strings.ToLower(stderr + stdout + strings.Join(errMsgs, " "))

	// Check for rate limiting
	if strings.Contains(combined, "rate limit") ||
		strings.Contains(combined, "rate_limit") ||
		strings.Contains(combined, "429") ||
		strings.Contains(combined, "too many requests") {
		return &agents.ClassifiedError{
			Original:   errors.New("rate limited"),
			Class:      agents.ErrorClassRetryable,
			RetryAfter: parseRetryAfter(combined),
			Message:    "API rate limit exceeded",
			Agent:      "claude-code",
		}
	}

	// Check for authentication errors (fatal)
	if strings.Contains(combined, "not authenticated") ||
		strings.Contains(combined, "api key") ||
		strings.Contains(combined, "authentication") ||
		strings.Contains(combined, "unauthorized") ||
		strings.Contains(combined, "invalid token") {
		return &agents.ClassifiedError{
			Original: errors.New("authentication failed"),
			Class:    agents.ErrorClassFatal,
			Message:  "Authentication error",
			Agent:    "claude-code",
		}
	}

	// Check for session-related errors
	if strings.Contains(combined, "session not found") ||
		strings.Contains(combined, "invalid session") ||
		strings.Contains(combined, "session expired") ||
		strings.Contains(combined, "no such session") {
		return &agents.ClassifiedError{
			Original: errors.New("session invalid"),
			Class:    agents.ErrorClassSessionInvalid,
			Message:  "Session not found or expired",
			Agent:    "claude-code",
		}
	}

	// Check for connection errors (retryable)
	if strings.Contains(combined, "connection") ||
		strings.Contains(combined, "network") ||
		strings.Contains(combined, "timeout") ||
		strings.Contains(combined, "dns") ||
		strings.Contains(combined, "unreachable") {
		return &agents.ClassifiedError{
			Original: errors.New("connection failed"),
			Class:    agents.ErrorClassRetryable,
			Message:  "Network connection error",
			Agent:    "claude-code",
		}
	}

	// Check for API overload (retryable)
	if strings.Contains(combined, "overloaded") ||
		strings.Contains(combined, "503") ||
		strings.Contains(combined, "service unavailable") {
		return &agents.ClassifiedError{
			Original:   errors.New("api overloaded"),
			Class:      agents.ErrorClassRetryable,
			RetryAfter: 30 * time.Second,
			Message:    "API is overloaded",
			Agent:      "claude-code",
		}
	}

	// Unknown error - build message from available sources
	msg := strings.Join(errMsgs, "; ")
	if msg == "" {
		msg = stderr
	}
	if msg == "" {
		msg = stdout
	}
	if msg == "" {
		msg = "unknown error"
	}

	return &agents.ClassifiedError{
		Original: errors.New(msg),
		Class:    agents.ErrorClassUnknown,
		Message:  msg,
		Agent:    "claude-code",
	}
}

// parseRetryAfter extracts retry-after duration from error message.
func parseRetryAfter(msg string) time.Duration {
	patterns := []string{
		`retry.?after[:\s]+(\d+)\s*s`,
		`wait[:\s]+(\d+)\s*s`,
		`(\d+)\s*seconds?`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(msg); len(matches) > 1 {
			if seconds, err := strconv.Atoi(matches[1]); err == nil {
				return time.Duration(seconds) * time.Second
			}
		}
	}

	// Default retry after for rate limits
	return 60 * time.Second
}

// IsSessionInvalidError checks if the result indicates a session-related error.
func IsSessionInvalidError(stderr, stdout string) bool {
	combined := strings.ToLower(stderr + stdout)
	sessionErrors := []string{
		"session not found",
		"invalid session",
		"session expired",
		"no such session",
	}

	for _, msg := range sessionErrors {
		if strings.Contains(combined, msg) {
			return true
		}
	}

	return false
}
