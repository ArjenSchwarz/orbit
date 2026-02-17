package opencode

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/arjenschwarz/orbit/internal/agents"
)

func init() {
	agents.RegisterClassifier("opencode", NewClassifier)
}

// Classifier implements agents.ErrorClassifier for OpenCode.
type Classifier struct{}

// NewClassifier creates a new OpenCode error classifier.
func NewClassifier() agents.ErrorClassifier {
	return &Classifier{}
}

// Classify examines error output and classifies it by type.
// OpenCode may exit with code 0 even on errors. We detect errors by:
// 1. Checking if stdout contains valid JSON (errors produce plaintext stack traces)
// 2. Parsing error patterns from the plaintext output
func (c *Classifier) Classify(exitCode int, stderr, stdout string, errMsgs []string) *agents.ClassifiedError {
	// Combine all error sources for pattern matching
	combined := strings.ToLower(stderr + stdout + strings.Join(errMsgs, " "))

	// Check if output is not valid JSON (indicates error output)
	if !isValidJSONOutput(stdout) && strings.TrimSpace(stdout) != "" {
		// Parse error patterns from plaintext output
		return c.classifyPlaintext(combined)
	}

	// Check for fatal model errors (opencode-specific)
	if strings.Contains(combined, "providermodelnotfounderror") ||
		strings.Contains(combined, "model not found") ||
		strings.Contains(combined, "invalid model") {
		return &agents.ClassifiedError{
			Original: errors.New("model not found"),
			Class:    agents.ErrorClassFatal,
			Message:  "Invalid or unsupported model",
			Agent:    "opencode",
		}
	}

	// Check for authentication errors (fatal)
	if agents.MatchesAuthError(combined, "authenticationerror", "authentication failed") {
		return agents.NewAuthError("opencode")
	}

	// Check for rate limiting (retryable)
	if agents.MatchesRateLimit(combined) {
		return agents.NewRateLimitError("opencode", combined)
	}

	// Check for session-related errors
	if agents.MatchesSessionInvalid(combined, "no session") {
		return agents.NewSessionInvalidError("opencode")
	}

	// Check for connection errors (retryable)
	if agents.MatchesConnectionError(combined) {
		return agents.NewConnectionError("opencode")
	}

	// Check for API overload (retryable)
	if agents.MatchesOverload(combined) {
		return agents.NewOverloadError("opencode")
	}

	return agents.NewUnknownError("opencode", errMsgs, stderr, stdout)
}

// classifyPlaintext parses error patterns from non-JSON output.
func (c *Classifier) classifyPlaintext(combined string) *agents.ClassifiedError {
	// Check for model errors (opencode-specific)
	if strings.Contains(combined, "providermodelnotfounderror") ||
		strings.Contains(combined, "model not found") {
		return &agents.ClassifiedError{
			Original: errors.New("model not found"),
			Class:    agents.ErrorClassFatal,
			Message:  "Invalid or unsupported model",
			Agent:    "opencode",
		}
	}

	// Check for authentication errors
	if agents.MatchesAuthError(combined, "authenticationerror") {
		return agents.NewAuthError("opencode")
	}

	// Check for rate limiting
	if agents.MatchesRateLimit(combined) {
		return agents.NewRateLimitError("opencode", combined)
	}

	// Check for connection errors
	if agents.MatchesConnectionError(combined) {
		return agents.NewConnectionError("opencode")
	}

	// Check for overload errors
	if agents.MatchesOverload(combined) {
		return agents.NewOverloadError("opencode")
	}

	// Default: return as unknown with output included
	return &agents.ClassifiedError{
		Original: errors.New("non-JSON output indicates error"),
		Class:    agents.ErrorClassUnknown,
		Message:  "OpenCode returned non-JSON output indicating an error",
		Agent:    "opencode",
	}
}

// isValidJSONOutput checks if the string is valid JSON.
func isValidJSONOutput(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	var js json.RawMessage
	return json.Unmarshal([]byte(s), &js) == nil
}

