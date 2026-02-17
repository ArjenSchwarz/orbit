package kiro

import (
	"strings"

	"github.com/arjenschwarz/orbit/internal/agents"
)

func init() {
	agents.RegisterClassifier("kiro", NewClassifier)
}

// Classifier implements agents.ErrorClassifier for Kiro.
type Classifier struct{}

// NewClassifier creates a new Kiro error classifier.
func NewClassifier() agents.ErrorClassifier {
	return &Classifier{}
}

// Classify examines error output and classifies it by type.
func (c *Classifier) Classify(exitCode int, stderr, stdout string, errMsgs []string) *agents.ClassifiedError {
	// Combine all error sources for pattern matching
	combined := strings.ToLower(stderr + stdout + strings.Join(errMsgs, " "))

	// Check for rate limiting
	if agents.MatchesRateLimit(combined, "throttl") {
		return agents.NewRateLimitError("kiro", combined)
	}

	// Check for authentication errors (fatal)
	if agents.MatchesAuthError(combined, "credentials", "not authenticated", "authentication", "access denied") {
		return agents.NewAuthError("kiro")
	}

	// Check for session-related errors
	if agents.MatchesSessionInvalid(combined, "no active session", "no session") {
		return agents.NewSessionInvalidError("kiro")
	}

	// Check for connection errors (retryable)
	if agents.MatchesConnectionError(combined, "econnrefused", "enotfound") {
		return agents.NewConnectionError("kiro")
	}

	// Check for API overload (retryable)
	if agents.MatchesOverload(combined, "temporarily unavailable") {
		return agents.NewOverloadError("kiro")
	}

	return agents.NewUnknownError("kiro", errMsgs, stderr, stdout)
}

