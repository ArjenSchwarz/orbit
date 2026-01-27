package opencode

import (
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
)

const (
	// defaultOverloadRetryAfter is the default retry delay when the API is overloaded.
	defaultOverloadRetryAfter = 30 * time.Second

	// defaultRateLimitRetryAfter is the default retry delay for rate limit errors
	// when no specific retry-after value is provided.
	defaultRateLimitRetryAfter = 60 * time.Second
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

	// Check for fatal model errors
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
	if strings.Contains(combined, "authenticationerror") ||
		strings.Contains(combined, "authentication failed") ||
		strings.Contains(combined, "api key") ||
		strings.Contains(combined, "invalid token") ||
		strings.Contains(combined, "unauthorized") {
		return &agents.ClassifiedError{
			Original: errors.New("authentication failed"),
			Class:    agents.ErrorClassFatal,
			Message:  "Authentication error",
			Agent:    "opencode",
		}
	}

	// Check for rate limiting (retryable)
	if strings.Contains(combined, "rate limit") ||
		strings.Contains(combined, "rate_limit") ||
		strings.Contains(combined, "429") ||
		strings.Contains(combined, "too many requests") {
		return &agents.ClassifiedError{
			Original:   errors.New("rate limited"),
			Class:      agents.ErrorClassRetryable,
			RetryAfter: parseRetryAfter(combined),
			Message:    "API rate limit exceeded",
			Agent:      "opencode",
		}
	}

	// Check for session-related errors
	if strings.Contains(combined, "session not found") ||
		strings.Contains(combined, "invalid session") ||
		strings.Contains(combined, "session expired") ||
		strings.Contains(combined, "no session") {
		return &agents.ClassifiedError{
			Original: errors.New("session invalid"),
			Class:    agents.ErrorClassSessionInvalid,
			Message:  "Session not found or expired",
			Agent:    "opencode",
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
			Agent:    "opencode",
		}
	}

	// Check for API overload (retryable)
	if strings.Contains(combined, "overloaded") ||
		strings.Contains(combined, "503") ||
		strings.Contains(combined, "service unavailable") {
		return &agents.ClassifiedError{
			Original:   errors.New("api overloaded"),
			Class:      agents.ErrorClassRetryable,
			RetryAfter: defaultOverloadRetryAfter,
			Message:    "API is overloaded",
			Agent:      "opencode",
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
		Agent:    "opencode",
	}
}

// classifyPlaintext parses error patterns from non-JSON output.
func (c *Classifier) classifyPlaintext(combined string) *agents.ClassifiedError {
	// Check for model errors
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
	if strings.Contains(combined, "authenticationerror") ||
		strings.Contains(combined, "unauthorized") ||
		strings.Contains(combined, "api key") {
		return &agents.ClassifiedError{
			Original: errors.New("authentication failed"),
			Class:    agents.ErrorClassFatal,
			Message:  "Authentication error",
			Agent:    "opencode",
		}
	}

	// Check for rate limiting
	if strings.Contains(combined, "rate limit") ||
		strings.Contains(combined, "429") ||
		strings.Contains(combined, "too many requests") {
		return &agents.ClassifiedError{
			Original:   errors.New("rate limited"),
			Class:      agents.ErrorClassRetryable,
			RetryAfter: parseRetryAfter(combined),
			Message:    "API rate limit exceeded",
			Agent:      "opencode",
		}
	}

	// Check for connection errors
	if strings.Contains(combined, "connection") ||
		strings.Contains(combined, "network") ||
		strings.Contains(combined, "timeout") {
		return &agents.ClassifiedError{
			Original: errors.New("connection failed"),
			Class:    agents.ErrorClassRetryable,
			Message:  "Network connection error",
			Agent:    "opencode",
		}
	}

	// Check for overload errors
	if strings.Contains(combined, "overloaded") ||
		strings.Contains(combined, "503") ||
		strings.Contains(combined, "service unavailable") {
		return &agents.ClassifiedError{
			Original:   errors.New("api overloaded"),
			Class:      agents.ErrorClassRetryable,
			RetryAfter: defaultOverloadRetryAfter,
			Message:    "API is overloaded",
			Agent:      "opencode",
		}
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
	return defaultRateLimitRetryAfter
}
