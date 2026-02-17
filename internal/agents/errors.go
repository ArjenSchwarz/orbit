// Package agents provides the agent abstraction layer for multi-agent support.
package agents

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ErrorClass categorizes errors for orchestrator retry logic.
// This is distinct from internal/errors.ErrorType which provides specific categories.
type ErrorClass int

const (
	// ErrorClassUnknown represents an unclassified error.
	ErrorClassUnknown ErrorClass = iota
	// ErrorClassRetryable represents errors that can be retried (rate limits, transient network issues).
	ErrorClassRetryable
	// ErrorClassFatal represents errors that should not be retried (auth failures, invalid config).
	ErrorClassFatal
	// ErrorClassSessionInvalid represents session-related errors (session expired or not found).
	ErrorClassSessionInvalid
	// ErrorClassRateLimitWait represents usage limits that require waiting until a specific time.
	// This is different from ErrorClassRetryable - the retry counter should reset after waiting.
	ErrorClassRateLimitWait
)

// String returns a human-readable name for the error class.
func (ec ErrorClass) String() string {
	switch ec {
	case ErrorClassRetryable:
		return "retryable"
	case ErrorClassFatal:
		return "fatal"
	case ErrorClassSessionInvalid:
		return "session-invalid"
	case ErrorClassRateLimitWait:
		return "rate-limit-wait"
	default:
		return "unknown"
	}
}

// IsRetryable returns true if the error class indicates the operation can be retried.
func (ec ErrorClass) IsRetryable() bool {
	return ec == ErrorClassRetryable || ec == ErrorClassRateLimitWait
}

// IsRateLimitWait returns true if this is a rate limit that requires waiting until a specific time.
func (ec ErrorClass) IsRateLimitWait() bool {
	return ec == ErrorClassRateLimitWait
}

// ClassifiedError wraps an error with classification metadata.
type ClassifiedError struct {
	Original   error
	Class      ErrorClass
	RetryAfter time.Duration
	Message    string
	Agent      string
}

// Error returns the error message.
func (e *ClassifiedError) Error() string {
	return e.Message
}

// Unwrap returns the original error for use with errors.Is and errors.As.
func (e *ClassifiedError) Unwrap() error {
	return e.Original
}

// ErrorClassifier is implemented by each agent to classify errors.
type ErrorClassifier interface {
	Classify(exitCode int, stderr, stdout string, errMsgs []string) *ClassifiedError
}

// DefaultRateLimitRetryAfter is the default retry delay for rate limit errors
// when no specific retry-after value is provided.
const DefaultRateLimitRetryAfter = 60 * time.Second

// commonSessionInvalidPatterns are session-invalid patterns shared across all agents.
var commonSessionInvalidPatterns = []string{
	"session not found",
	"invalid session",
	"session expired",
}

// MatchesSessionInvalid checks if the lowercased error text matches any common
// session-invalid pattern or any of the provided extra patterns.
func MatchesSessionInvalid(combinedLower string, extraPatterns ...string) bool {
	for _, p := range commonSessionInvalidPatterns {
		if strings.Contains(combinedLower, p) {
			return true
		}
	}
	for _, p := range extraPatterns {
		if strings.Contains(combinedLower, p) {
			return true
		}
	}
	return false
}

// NewSessionInvalidError creates a ClassifiedError for session-invalid conditions.
func NewSessionInvalidError(agentName string) *ClassifiedError {
	return &ClassifiedError{
		Original: errors.New("session invalid"),
		Class:    ErrorClassSessionInvalid,
		Message:  "Session not found or expired",
		Agent:    agentName,
	}
}

// ParseRetryAfter extracts retry-after duration from an error message.
// It looks for patterns like "retry after 30s", "wait: 60 seconds", etc.
// Returns DefaultRateLimitRetryAfter if no pattern matches.
func ParseRetryAfter(msg string) time.Duration {
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

	return DefaultRateLimitRetryAfter
}
