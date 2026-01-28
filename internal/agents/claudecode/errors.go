package claudecode

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
)

func init() {
	agents.RegisterClassifier("claude-code", NewClassifier)
}

// Classifier implements agents.ErrorClassifier for Claude Code.
type Classifier struct{}

// NewClassifier creates a new Claude Code error classifier.
func NewClassifier() agents.ErrorClassifier {
	return &Classifier{}
}

// Classify examines error output and classifies it by type.
func (c *Classifier) Classify(exitCode int, stderr, stdout string, errMsgs []string) *agents.ClassifiedError {
	// Combine all error sources for pattern matching
	combined := stderr + stdout + strings.Join(errMsgs, " ")
	combinedLower := strings.ToLower(combined)

	// Check for usage limit (5-hour limit) - must check before regular rate limit
	// Example message: "You've hit your limit · resets 3am (Australia/Melbourne)"
	if strings.Contains(combinedLower, "hit your limit") ||
		strings.Contains(combinedLower, "you've hit your limit") {
		waitDuration := parseUsageLimitReset(combined)
		if waitDuration > 0 {
			return &agents.ClassifiedError{
				Original:   errors.New("usage limit reached"),
				Class:      agents.ErrorClassRateLimitWait,
				RetryAfter: waitDuration,
				Message:    fmt.Sprintf("Usage limit reached, waiting %s until reset", formatDuration(waitDuration)),
				Agent:      "claude-code",
			}
		}
	}

	// Check for rate limiting
	if strings.Contains(combinedLower, "rate limit") ||
		strings.Contains(combinedLower, "rate_limit") ||
		strings.Contains(combinedLower, "429") ||
		strings.Contains(combinedLower, "too many requests") {
		return &agents.ClassifiedError{
			Original:   errors.New("rate limited"),
			Class:      agents.ErrorClassRetryable,
			RetryAfter: parseRetryAfter(combinedLower),
			Message:    "API rate limit exceeded",
			Agent:      "claude-code",
		}
	}

	// Check for authentication errors (fatal)
	if strings.Contains(combinedLower, "not authenticated") ||
		strings.Contains(combinedLower, "api key") ||
		strings.Contains(combinedLower, "authentication") ||
		strings.Contains(combinedLower, "unauthorized") ||
		strings.Contains(combinedLower, "invalid token") {
		return &agents.ClassifiedError{
			Original: errors.New("authentication failed"),
			Class:    agents.ErrorClassFatal,
			Message:  "Authentication error",
			Agent:    "claude-code",
		}
	}

	// Check for session-related errors
	if strings.Contains(combinedLower, "session not found") ||
		strings.Contains(combinedLower, "invalid session") ||
		strings.Contains(combinedLower, "session expired") ||
		strings.Contains(combinedLower, "no such session") {
		return &agents.ClassifiedError{
			Original: errors.New("session invalid"),
			Class:    agents.ErrorClassSessionInvalid,
			Message:  "Session not found or expired",
			Agent:    "claude-code",
		}
	}

	// Check for connection errors (retryable)
	if strings.Contains(combinedLower, "connection") ||
		strings.Contains(combinedLower, "network") ||
		strings.Contains(combinedLower, "timeout") ||
		strings.Contains(combinedLower, "dns") ||
		strings.Contains(combinedLower, "unreachable") {
		return &agents.ClassifiedError{
			Original: errors.New("connection failed"),
			Class:    agents.ErrorClassRetryable,
			Message:  "Network connection error",
			Agent:    "claude-code",
		}
	}

	// Check for API overload (retryable)
	if strings.Contains(combinedLower, "overloaded") ||
		strings.Contains(combinedLower, "503") ||
		strings.Contains(combinedLower, "service unavailable") {
		return &agents.ClassifiedError{
			Original:   errors.New("api overloaded"),
			Class:      agents.ErrorClassRetryable,
			RetryAfter: 30 * time.Second,
			Message:    "API is overloaded",
			Agent:      "claude-code",
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
		Agent:    "claude-code",
	}
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
	return 60 * time.Second
}

// parseUsageLimitReset parses the reset time from usage limit messages.
// Example message: "You've hit your limit · resets 3am (Australia/Melbourne)"
// Returns the duration to wait until the reset time, or 0 if parsing fails.
func parseUsageLimitReset(msg string) time.Duration {
	// Pattern to match: "resets <time> (<timezone>)"
	// Time formats: "3am", "3:00am", "12:30pm", "3 am", "3:00 am"
	pattern := `resets?\s+(\d{1,2})(?::(\d{2}))?\s*(am|pm)\s*\(([^)]+)\)`
	re := regexp.MustCompile("(?i)" + pattern)
	matches := re.FindStringSubmatch(msg)
	if len(matches) < 5 {
		return 0
	}

	hourStr := matches[1]
	minuteStr := matches[2]
	ampm := strings.ToLower(matches[3])
	tzName := matches[4]

	hour, err := strconv.Atoi(hourStr)
	if err != nil {
		return 0
	}

	minute := 0
	if minuteStr != "" {
		minute, err = strconv.Atoi(minuteStr)
		if err != nil {
			return 0
		}
	}

	// Convert 12-hour to 24-hour format
	if ampm == "pm" && hour != 12 {
		hour += 12
	} else if ampm == "am" && hour == 12 {
		hour = 0
	}

	// Load the timezone
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		// Try common timezone abbreviations
		loc = parseTimezoneAbbrev(tzName)
		if loc == nil {
			return 0
		}
	}

	now := time.Now().In(loc)
	resetTime := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, loc)

	// If reset time is in the past, it's for tomorrow
	if resetTime.Before(now) {
		resetTime = resetTime.Add(24 * time.Hour)
	}

	// Add a small buffer (1 minute) to ensure we're past the reset
	waitDuration := resetTime.Sub(now) + time.Minute

	return waitDuration
}

// parseTimezoneAbbrev attempts to parse common timezone abbreviations.
// Returns nil if the abbreviation is not recognized.
func parseTimezoneAbbrev(abbrev string) *time.Location {
	// Common timezone mappings
	abbrevMap := map[string]string{
		"pst":  "America/Los_Angeles",
		"pdt":  "America/Los_Angeles",
		"mst":  "America/Denver",
		"mdt":  "America/Denver",
		"cst":  "America/Chicago",
		"cdt":  "America/Chicago",
		"est":  "America/New_York",
		"edt":  "America/New_York",
		"gmt":  "Europe/London",
		"bst":  "Europe/London",
		"utc":  "UTC",
		"aest": "Australia/Sydney",
		"aedt": "Australia/Sydney",
		"acst": "Australia/Adelaide",
		"acdt": "Australia/Adelaide",
		"awst": "Australia/Perth",
		"jst":  "Asia/Tokyo",
		"kst":  "Asia/Seoul",
		"cet":  "Europe/Paris",
		"cest": "Europe/Paris",
	}

	if tzName, ok := abbrevMap[strings.ToLower(abbrev)]; ok {
		loc, err := time.LoadLocation(tzName)
		if err == nil {
			return loc
		}
	}
	return nil
}

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		return fmt.Sprintf("%d minute%s", mins, pluralS(mins))
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	if mins == 0 {
		return fmt.Sprintf("%d hour%s", hours, pluralS(hours))
	}
	return fmt.Sprintf("%d hour%s %d minute%s", hours, pluralS(hours), mins, pluralS(mins))
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// IsSessionInvalidError checks if the result indicates a session-related error.
func IsSessionInvalidError(stderr, stdout string) bool {
	combined := strings.ToLower(stderr + stdout)
	sessionErrors := []string{
		"session not found",
		"invalid session",
		"session expired",
		"no such session",
	}

	for _, msg := range sessionErrors {
		if strings.Contains(combined, msg) {
			return true
		}
	}

	return false
}
