package codex

import (
	"strings"

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
	if agents.MatchesRateLimit(combined) {
		return agents.NewRateLimitError("codex", combined)
	}

	// Check for authentication errors (fatal)
	if agents.MatchesAuthError(combined, "authentication failed") {
		return agents.NewAuthError("codex")
	}

	// Check for session-related errors
	if agents.MatchesSessionInvalid(combined, "no session") {
		return agents.NewSessionInvalidError("codex")
	}

	// Check for connection errors (retryable)
	if agents.MatchesConnectionError(combined) {
		return agents.NewConnectionError("codex")
	}

	// Check for API overload (retryable)
	if agents.MatchesOverload(combined) {
		return agents.NewOverloadError("codex")
	}

	return agents.NewUnknownError("codex", errMsgs, stderr, stdout)
}

