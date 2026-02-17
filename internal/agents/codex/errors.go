package codex

import (
	"errors"
	"strings"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
)

func init() {
	agents.RegisterClassifier("codex", NewClassifier)
}

// Classifier implements agents.ErrorClassifier for Codex.
type Classifier struct{}

// NewClassifier creates a new Codex error classifier.
func NewClassifier() agents.ErrorClassifier {
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
			RetryAfter: agents.ParseRetryAfter(combined),
			Message:    "API rate limit exceeded",
			Agent:      "codex",
		}
	}

	// Check for authentication errors (fatal)
	if strings.Contains(combined, "authentication failed") ||
		strings.Contains(combined, "api key") ||
		strings.Contains(combined, "invalid token") ||
		strings.Contains(combined, "unauthorized") {
		return &agents.ClassifiedError{
			Original: errors.New("authentication failed"),
			Class:    agents.ErrorClassFatal,
			Message:  "Authentication error",
			Agent:    "codex",
		}
	}

	// Check for session-related errors
	if agents.MatchesSessionInvalid(combined, "no session") {
		return agents.NewSessionInvalidError("codex")
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
			Agent:    "codex",
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
			Agent:      "codex",
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
		Agent:    "codex",
	}
}

