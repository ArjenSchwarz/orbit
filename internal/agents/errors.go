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

// DefaultRateLimitRetryAfter is the default retry delay for 429/rate-limit errors
// when no Retry-After header or parseable duration is present.
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
	return matchesAny(combinedLower, commonSessionInvalidPatterns, extraPatterns)
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

// DefaultOverloadRetryAfter is the default retry delay for 503/overload errors.
const DefaultOverloadRetryAfter = 30 * time.Second

// commonRateLimitPatterns are rate-limit patterns shared across all agents.
var commonRateLimitPatterns = []string{
	"rate limit",
	"rate_limit",
	"429",
	"too many requests",
}

// MatchesRateLimit checks if the lowercased error text matches any common
// rate-limit pattern or any of the provided extra patterns.
func MatchesRateLimit(combinedLower string, extraPatterns ...string) bool {
	return matchesAny(combinedLower, commonRateLimitPatterns, extraPatterns)
}

// commonAuthErrorPatterns are authentication error patterns shared across 3+ agents.
var commonAuthErrorPatterns = []string{
	"unauthorized",
	"invalid token",
	"api key",
}

// MatchesAuthError checks if the lowercased error text matches any common
// authentication error pattern or any of the provided extra patterns.
func MatchesAuthError(combinedLower string, extraPatterns ...string) bool {
	return matchesAny(combinedLower, commonAuthErrorPatterns, extraPatterns)
}

// commonConnectionPatterns are connection/network error patterns shared across all agents.
// Includes "dns" and "unreachable" beyond the original per-agent sets — these are
// legitimate connection errors that were simply missing from some classifiers.
var commonConnectionPatterns = []string{
	"connection",
	"network",
	"timeout",
	"dns",
	"unreachable",
}

// MatchesConnectionError checks if the lowercased error text matches any common
// connection error pattern or any of the provided extra patterns.
func MatchesConnectionError(combinedLower string, extraPatterns ...string) bool {
	return matchesAny(combinedLower, commonConnectionPatterns, extraPatterns)
}

// commonOverloadPatterns are API overload patterns shared across all agents.
var commonOverloadPatterns = []string{
	"overloaded",
	"503",
	"service unavailable",
}

// MatchesOverload checks if the lowercased error text matches any common
// overload pattern or any of the provided extra patterns.
func MatchesOverload(combinedLower string, extraPatterns ...string) bool {
	return matchesAny(combinedLower, commonOverloadPatterns, extraPatterns)
}

// matchesAny checks if combinedLower contains any pattern from the common list
// or the extra list.
func matchesAny(combinedLower string, common, extra []string) bool {
	for _, p := range common {
		if strings.Contains(combinedLower, p) {
			return true
		}
	}
	for _, p := range extra {
		if strings.Contains(combinedLower, p) {
			return true
		}
	}
	return false
}

// NewRateLimitError creates a ClassifiedError for rate-limit conditions.
func NewRateLimitError(agentName, combinedLower string) *ClassifiedError {
	return &ClassifiedError{
		Original:   errors.New("rate limited"),
		Class:      ErrorClassRetryable,
		RetryAfter: ParseRetryAfter(combinedLower),
		Message:    "API rate limit exceeded",
		Agent:      agentName,
	}
}

// NewAuthError creates a ClassifiedError for authentication failures.
func NewAuthError(agentName string) *ClassifiedError {
	return &ClassifiedError{
		Original: errors.New("authentication failed"),
		Class:    ErrorClassFatal,
		Message:  "Authentication error",
		Agent:    agentName,
	}
}

// NewConnectionError creates a ClassifiedError for network/connection failures.
func NewConnectionError(agentName string) *ClassifiedError {
	return &ClassifiedError{
		Original: errors.New("connection failed"),
		Class:    ErrorClassRetryable,
		Message:  "Network connection error",
		Agent:    agentName,
	}
}

// NewOverloadError creates a ClassifiedError for API overload conditions.
func NewOverloadError(agentName string) *ClassifiedError {
	return &ClassifiedError{
		Original:   errors.New("api overloaded"),
		Class:      ErrorClassRetryable,
		RetryAfter: DefaultOverloadRetryAfter,
		Message:    "API is overloaded",
		Agent:      agentName,
	}
}

// NewUnknownError creates a ClassifiedError for unclassified errors,
// building the message from the available error sources.
func NewUnknownError(agentName string, errMsgs []string, stderr, stdout string) *ClassifiedError {
	msg := BuildUnknownMessage(errMsgs, stderr, stdout)
	return &ClassifiedError{
		Original: errors.New(msg),
		Class:    ErrorClassUnknown,
		Message:  msg,
		Agent:    agentName,
	}
}

// BuildUnknownMessage constructs a fallback error message from available sources.
func BuildUnknownMessage(errMsgs []string, stderr, stdout string) string {
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
	return msg
}

// BackoffDuration returns the recommended backoff duration for a retry attempt.
// Exponential backoff: 1s, 2s, 4s, 8s, 16s (capped).
func BackoffDuration(attempt int) time.Duration {
	base := time.Second
	return min(base*time.Duration(1<<uint(attempt)), 16*time.Second)
}

// retryAfterPatterns are pre-compiled regexes for extracting retry-after durations.
var retryAfterPatterns = []*regexp.Regexp{
	regexp.MustCompile(`retry.?after[:\s]+(\d+)\s*s`),
	regexp.MustCompile(`retry.?after[:\s]+(\d+)`),
	regexp.MustCompile(`wait[:\s]+(\d+)\s*s`),
	regexp.MustCompile(`(\d+)\s*seconds?`),
}

// ParseRetryAfter extracts retry-after duration from an error message.
// It looks for patterns like "retry after 30s", "wait: 60 seconds", etc.
// Returns DefaultRateLimitRetryAfter if no pattern matches.
func ParseRetryAfter(msg string) time.Duration {
	for _, re := range retryAfterPatterns {
		if matches := re.FindStringSubmatch(msg); len(matches) > 1 {
			if seconds, err := strconv.Atoi(matches[1]); err == nil {
				return time.Duration(seconds) * time.Second
			}
		}
	}

	return DefaultRateLimitRetryAfter
}
