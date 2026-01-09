// Package errors provides error classification and handling for Orbit.
package errors

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ErrorType represents the category of an error.
type ErrorType int

const (
	ErrUnknown ErrorType = iota
	ErrConnection
	ErrRateLimit
	ErrOverloaded
)

func (t ErrorType) String() string {
	switch t {
	case ErrConnection:
		return "connection"
	case ErrRateLimit:
		return "rate_limit"
	case ErrOverloaded:
		return "overloaded"
	default:
		return "unknown"
	}
}

// ClassifiedError wraps an error with classification metadata.
type ClassifiedError struct {
	Type       ErrorType
	Original   error
	RetryAfter time.Duration
	Message    string
}

func (e *ClassifiedError) Error() string {
	return fmt.Sprintf("%s error: %s", e.Type, e.Message)
}

func (e *ClassifiedError) Unwrap() error {
	return e.Original
}

// Classify examines error output and classifies it by type.
// The errMsgs parameter contains error messages from Claude's JSON output.
func Classify(exitCode int, stderr, stdout string, errMsgs []string) *ClassifiedError {
	// Combine all error sources for pattern matching
	combined := strings.ToLower(stderr + stdout + strings.Join(errMsgs, " "))

	// Check for rate limiting
	if strings.Contains(combined, "rate limit") ||
		strings.Contains(combined, "rate_limit") ||
		strings.Contains(combined, "429") ||
		strings.Contains(combined, "too many requests") {
		return &ClassifiedError{
			Type:       ErrRateLimit,
			Original:   errors.New("rate limited"),
			RetryAfter: parseRetryAfter(combined),
			Message:    "API rate limit exceeded",
		}
	}

	// Check for connection errors
	if strings.Contains(combined, "connection") ||
		strings.Contains(combined, "network") ||
		strings.Contains(combined, "timeout") ||
		strings.Contains(combined, "dns") ||
		strings.Contains(combined, "unreachable") {
		return &ClassifiedError{
			Type:     ErrConnection,
			Original: errors.New("connection failed"),
			Message:  "Network connection error",
		}
	}

	// Check for API overload
	if strings.Contains(combined, "overloaded") ||
		strings.Contains(combined, "503") ||
		strings.Contains(combined, "service unavailable") {
		return &ClassifiedError{
			Type:       ErrOverloaded,
			Original:   errors.New("api overloaded"),
			RetryAfter: 30 * time.Second,
			Message:    "API is overloaded",
		}
	}

	// Unknown error - prefer error messages from JSON, then stderr, then stdout
	msg := strings.Join(errMsgs, "; ")
	if msg == "" {
		msg = stderr
	}
	if msg == "" {
		msg = stdout
	}
	if msg == "" {
		msg = fmt.Sprintf("exit code %d", exitCode)
	}
	return &ClassifiedError{
		Type:     ErrUnknown,
		Original: errors.New(msg),
		Message:  msg,
	}
}

// parseRetryAfter extracts retry-after duration from error message.
func parseRetryAfter(msg string) time.Duration {
	// Try to find "retry after X seconds" or similar patterns
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

// IsRetryable returns true if the error type can be retried.
func (t ErrorType) IsRetryable() bool {
	return t == ErrConnection || t == ErrRateLimit || t == ErrOverloaded
}

// BackoffDuration returns the recommended backoff duration for a retry attempt.
func BackoffDuration(attempt int) time.Duration {
	// Exponential backoff: 1s, 2s, 4s, 8s, 16s (capped)
	base := time.Second
	duration := min(base*time.Duration(1<<uint(attempt)), 16*time.Second)
	return duration
}
