package copilot

import (
	"errors"
	"strings"

	"github.com/arjenschwarz/orbit/internal/agents"
)

func init() {
	agents.RegisterClassifier("copilot", NewClassifier)
}

// Classifier implements agents.ErrorClassifier for Copilot.
type Classifier struct{}

// NewClassifier creates a new Copilot error classifier.
func NewClassifier() agents.ErrorClassifier {
	return &Classifier{}
}

// Classify examines error output and classifies it by type.
func (c *Classifier) Classify(exitCode int, stderr, stdout string, errMsgs []string) *agents.ClassifiedError {
	// Combine all error sources for pattern matching
	combined := strings.ToLower(stderr + stdout + strings.Join(errMsgs, " "))

	// Check for rate limiting
	if agents.MatchesRateLimit(combined, "throttled") {
		return agents.NewRateLimitError("copilot", combined)
	}

	// Check for authentication errors (fatal)
	// Copilot has a custom message pointing users to 'gh auth login'.
	if agents.MatchesAuthError(combined, "not logged in", "authentication required",
		"access denied", "login required", "gh auth login") {
		return &agents.ClassifiedError{
			Original: errors.New("authentication failed"),
			Class:    agents.ErrorClassFatal,
			Message:  "Authentication error - please run 'gh auth login'",
			Agent:    "copilot",
		}
	}

	// Check for session-related errors
	if agents.MatchesSessionInvalid(combined, "no session to continue", "no previous session") {
		return agents.NewSessionInvalidError("copilot")
	}

	// Check for connection errors (retryable)
	if agents.MatchesConnectionError(combined, "econnrefused", "enotfound") {
		return agents.NewConnectionError("copilot")
	}

	// Check for API overload (retryable)
	if agents.MatchesOverload(combined, "temporarily unavailable") {
		return agents.NewOverloadError("copilot")
	}

	return agents.NewUnknownError("copilot", errMsgs, stderr, stdout)
}
