package claudecode

import (
	"testing"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
)

func TestClassifier_Classify_RateLimit(t *testing.T) {
	c := NewClassifier()

	tests := []struct {
		name   string
		stderr string
		stdout string
	}{
		{"rate limit in stderr", "Error: rate limit exceeded", ""},
		{"rate_limit in stderr", "rate_limit error occurred", ""},
		{"429 in stdout", "", "HTTP 429 Too Many Requests"},
		{"too many requests", "too many requests", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.Classify(1, tt.stderr, tt.stdout, nil)
			if err.Class != agents.ErrorClassRetryable {
				t.Errorf("Class = %v, want %v", err.Class, agents.ErrorClassRetryable)
			}
			if err.Agent != "claude-code" {
				t.Errorf("Agent = %q, want %q", err.Agent, "claude-code")
			}
		})
	}
}

func TestClassifier_Classify_Auth(t *testing.T) {
	c := NewClassifier()

	tests := []struct {
		name   string
		stderr string
	}{
		{"not authenticated", "Error: not authenticated"},
		{"api key", "Invalid api key provided"},
		{"authentication", "authentication failed"},
		{"unauthorized", "401 unauthorized"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.Classify(1, tt.stderr, "", nil)
			if err.Class != agents.ErrorClassFatal {
				t.Errorf("Class = %v, want %v", err.Class, agents.ErrorClassFatal)
			}
		})
	}
}

func TestClassifier_Classify_SessionInvalid(t *testing.T) {
	c := NewClassifier()

	tests := []struct {
		name   string
		stderr string
	}{
		{"session not found", "session not found"},
		{"invalid session", "Error: invalid session id"},
		{"session expired", "session expired"},
		{"no such session", "no such session exists"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.Classify(1, tt.stderr, "", nil)
			if err.Class != agents.ErrorClassSessionInvalid {
				t.Errorf("Class = %v, want %v", err.Class, agents.ErrorClassSessionInvalid)
			}
		})
	}
}

func TestClassifier_Classify_Connection(t *testing.T) {
	c := NewClassifier()

	tests := []struct {
		name   string
		stderr string
	}{
		{"connection", "connection refused"},
		{"network", "network error"},
		{"timeout", "request timeout"},
		{"dns", "dns lookup failed"},
		{"unreachable", "host unreachable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.Classify(1, tt.stderr, "", nil)
			if err.Class != agents.ErrorClassRetryable {
				t.Errorf("Class = %v, want %v", err.Class, agents.ErrorClassRetryable)
			}
		})
	}
}

func TestClassifier_Classify_Overloaded(t *testing.T) {
	c := NewClassifier()

	tests := []struct {
		name   string
		stderr string
	}{
		{"overloaded", "API is overloaded"},
		{"503", "HTTP 503 error"},
		{"service unavailable", "service unavailable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.Classify(1, tt.stderr, "", nil)
			if err.Class != agents.ErrorClassRetryable {
				t.Errorf("Class = %v, want %v", err.Class, agents.ErrorClassRetryable)
			}
			if err.RetryAfter != 30*time.Second {
				t.Errorf("RetryAfter = %v, want %v", err.RetryAfter, 30*time.Second)
			}
		})
	}
}

func TestClassifier_Classify_Unknown(t *testing.T) {
	c := NewClassifier()

	err := c.Classify(1, "some random error", "", nil)
	if err.Class != agents.ErrorClassUnknown {
		t.Errorf("Class = %v, want %v", err.Class, agents.ErrorClassUnknown)
	}
}

func TestClassifier_Classify_ErrMsgs(t *testing.T) {
	c := NewClassifier()

	// Error messages from JSON output should be checked
	err := c.Classify(1, "", "", []string{"rate limit exceeded"})
	if err.Class != agents.ErrorClassRetryable {
		t.Errorf("Class = %v, want %v", err.Class, agents.ErrorClassRetryable)
	}
}

func TestClassifier_ImplementsInterface(t *testing.T) {
	// This test verifies that Classifier implements ErrorClassifier
	classifier := NewClassifier()
	if classifier == nil {
		t.Error("NewClassifier() returned nil")
	}
}

func TestClassifier_Classify_UsageLimit(t *testing.T) {
	c := NewClassifier()

	tests := []struct {
		name   string
		stderr string
		stdout string
	}{
		{"hit your limit", "You've hit your limit · resets 3am (Australia/Melbourne)", ""},
		{"hit your session limit", "You've hit your session limit · resets 4pm (Australia/Melbourne)", ""},
		{"hit your limit in stdout", "", "You've hit your limit · resets 5pm (America/New_York)"},
		{"lowercase hit limit", "you've hit your limit resets 12am (UTC)", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.Classify(1, tt.stderr, tt.stdout, nil)
			if err.Class != agents.ErrorClassRateLimitWait {
				t.Errorf("Class = %v, want %v", err.Class, agents.ErrorClassRateLimitWait)
			}
			if err.Agent != "claude-code" {
				t.Errorf("Agent = %q, want %q", err.Agent, "claude-code")
			}
			if err.RetryAfter <= 0 {
				t.Errorf("RetryAfter = %v, want > 0", err.RetryAfter)
			}
		})
	}
}

func TestClassifier_Classify_UsageLimitNoResetTime(t *testing.T) {
	c := NewClassifier()

	// When usage limit message is detected but has no time at all,
	// it should fall through to unknown error (not crash)
	err := c.Classify(1, "you've hit your limit", "", nil)
	// Should not be RateLimitWait because no reset time present
	if err.Class == agents.ErrorClassRateLimitWait {
		t.Errorf("Class = %v, should not be RateLimitWait when no reset time present", err.Class)
	}
	// Should be Unknown since no other pattern matches
	if err.Class != agents.ErrorClassUnknown {
		t.Errorf("Class = %v, want %v", err.Class, agents.ErrorClassUnknown)
	}
}

func TestClassifier_Classify_UsageLimitWithoutTimezone(t *testing.T) {
	c := NewClassifier()

	// T-203: When usage limit has a reset time but no timezone, it should still
	// be classified as RateLimitWait (defaulting to local time).
	tests := []struct {
		name   string
		stderr string
		stdout string
	}{
		{"no timezone simple", "You've hit your limit · resets 3am", ""},
		{"no timezone pm", "You've hit your limit · resets 5pm", ""},
		{"no timezone with minutes", "You've hit your limit · resets 3:30am", ""},
		{"no timezone in stdout", "", "You've hit your limit · resets 12pm"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.Classify(1, tt.stderr, tt.stdout, nil)
			if err.Class != agents.ErrorClassRateLimitWait {
				t.Errorf("Class = %v, want %v", err.Class, agents.ErrorClassRateLimitWait)
			}
			if err.Agent != "claude-code" {
				t.Errorf("Agent = %q, want %q", err.Agent, "claude-code")
			}
			if err.RetryAfter <= 0 {
				t.Errorf("RetryAfter = %v, want > 0", err.RetryAfter)
			}
		})
	}
}

func TestParseUsageLimitReset(t *testing.T) {
	tests := []struct {
		name        string
		msg         string
		expectValid bool
	}{
		{"simple am time", "resets 3am (UTC)", true},
		{"simple pm time", "resets 5pm (UTC)", true},
		{"time with minutes", "resets 3:30am (UTC)", true},
		{"time with space", "resets 3 am (UTC)", true},
		{"timezone with slash", "resets 3am (Australia/Melbourne)", true},
		{"timezone abbreviation", "resets 3am (EST)", true},
		{"full message", "You've hit your limit · resets 3am (Australia/Melbourne)", true},
		{"no match", "some random error", false},
		{"no timezone simple", "resets 3am", true},
		{"no timezone pm", "resets 5pm", true},
		{"no timezone with minutes", "resets 3:30pm", true},
		{"no timezone with space", "resets 3 am", true},
		{"no timezone full message", "You've hit your limit · resets 3am", true},
		{"missing time", "resets (UTC)", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseUsageLimitReset(tt.msg)
			if tt.expectValid && got <= 0 {
				t.Errorf("parseUsageLimitReset(%q) = %v, want > 0", tt.msg, got)
			}
			if !tt.expectValid && got > 0 {
				t.Errorf("parseUsageLimitReset(%q) = %v, want 0", tt.msg, got)
			}
		})
	}
}

func TestParseTimezoneAbbrev(t *testing.T) {
	tests := []struct {
		abbrev   string
		expected bool
	}{
		{"EST", true},
		{"PST", true},
		{"UTC", true},
		{"AEST", true},
		{"JST", true},
		{"INVALID", false},
		{"XYZ", false},
	}

	for _, tt := range tests {
		t.Run(tt.abbrev, func(t *testing.T) {
			loc := parseTimezoneAbbrev(tt.abbrev)
			if tt.expected && loc == nil {
				t.Errorf("parseTimezoneAbbrev(%q) = nil, want non-nil", tt.abbrev)
			}
			if !tt.expected && loc != nil {
				t.Errorf("parseTimezoneAbbrev(%q) = %v, want nil", tt.abbrev, loc)
			}
		})
	}
}

// TestParseTimezoneAbbrev_GMTFixedOffset verifies that "GMT" resolves to a
// fixed UTC+0 location that does not observe DST.  Europe/London switches to
// BST (UTC+1) in summer, so mapping "gmt" → "Europe/London" produces a wrong
// offset for half the year.  Regression test for T-516.
func TestParseTimezoneAbbrev_GMTFixedOffset(t *testing.T) {
	loc := parseTimezoneAbbrev("GMT")
	if loc == nil {
		t.Fatal("parseTimezoneAbbrev(\"GMT\") returned nil")
	}

	// Pick a date in July when Europe/London observes BST (UTC+1).
	summer := time.Date(2025, time.July, 1, 12, 0, 0, 0, loc)
	_, offset := summer.Zone()
	if offset != 0 {
		t.Errorf("GMT in summer: got UTC offset %d, want 0 (location resolved to a zone with DST)", offset)
	}

	// Also verify winter stays at 0 — sanity check.
	winter := time.Date(2025, time.January, 1, 12, 0, 0, 0, loc)
	_, offset = winter.Zone()
	if offset != 0 {
		t.Errorf("GMT in winter: got UTC offset %d, want 0", offset)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"seconds", 30 * time.Second, "30 seconds"},
		{"one minute", 1 * time.Minute, "1 minute"},
		{"minutes", 5 * time.Minute, "5 minutes"},
		{"one hour", 1 * time.Hour, "1 hour"},
		{"hours", 3 * time.Hour, "3 hours"},
		{"hours and minutes", 2*time.Hour + 30*time.Minute, "2 hours 30 minutes"},
		{"one hour one minute", 1*time.Hour + 1*time.Minute, "1 hour 1 minute"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.duration)
			if got != tt.expected {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, got, tt.expected)
			}
		})
	}
}
