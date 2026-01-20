// Package agents provides the agent abstraction layer for multi-agent support.
package agents

import "time"

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
	default:
		return "unknown"
	}
}

// IsRetryable returns true if the error class indicates the operation can be retried.
func (ec ErrorClass) IsRetryable() bool {
	return ec == ErrorClassRetryable
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
