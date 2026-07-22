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

	// Check for session/usage limits - must check before regular rate limits.
	// Example messages:
	//   "You've hit your limit · resets 3am (Australia/Melbourne)"
	//   "You've hit your session limit · resets 4pm (Australia/Melbourne)"
	//   "You've hit your limit · resets 3am"
	if strings.Contains(combinedLower, "hit your limit") ||
		strings.Contains(combinedLower, "hit your session limit") {
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
	if agents.MatchesRateLimit(combinedLower) {
		return agents.NewRateLimitError("claude-code", combinedLower)
	}

	// Check for authentication errors (fatal)
	if agents.MatchesAuthError(combinedLower, "not authenticated", "authentication") {
		return agents.NewAuthError("claude-code")
	}

	// Check for session-related errors
	if agents.MatchesSessionInvalid(combinedLower, "no such session") {
		return agents.NewSessionInvalidError("claude-code")
	}

	// Check for connection errors (retryable)
	if agents.MatchesConnectionError(combinedLower) {
		return agents.NewConnectionError("claude-code")
	}

	// Check for API overload (retryable)
	if agents.MatchesOverload(combinedLower) {
		return agents.NewOverloadError("claude-code")
	}

	return agents.NewUnknownError("claude-code", errMsgs, stderr, stdout)
}

// parseUsageLimitReset parses the reset time from usage limit messages.
// Example messages:
//   - "You've hit your limit · resets 3am (Australia/Melbourne)"
//   - "You've hit your limit · resets 3am"
//
// When no timezone is specified, defaults to the system's local timezone.
// Returns the duration to wait until the reset time, or 0 if parsing fails.
func parseUsageLimitReset(msg string) time.Duration {
	// Pattern to match: "resets <time>" with optional "(<timezone>)"
	// Time formats: "3am", "3:00am", "12:30pm", "3 am", "3:00 am"
	pattern := `resets?\s+(\d{1,2})(?::(\d{2}))?\s*(am|pm)(?:\s*\(([^)]+)\))?`
	re := regexp.MustCompile("(?i)" + pattern)
	matches := re.FindStringSubmatch(msg)
	if matches == nil {
		return 0
	}

	hourStr := matches[1]
	minuteStr := matches[2]
	ampm := strings.ToLower(matches[3])

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

	// Load the timezone, defaulting to local time when not specified.
	// matches[4] is "" when the optional timezone group didn't match.
	var loc *time.Location
	tzName := matches[4]

	if tzName != "" {
		loc, err = time.LoadLocation(tzName)
		if err != nil {
			// Try common timezone abbreviations
			loc = parseTimezoneAbbrev(tzName)
			if loc == nil {
				return 0
			}
		}
	} else {
		// No timezone specified — assume local time, as Claude CLI most likely
		// displays the reset time in the user's local timezone.
		loc = time.Local
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
		"gmt":  "UTC",
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
