package codex

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
		{"rate limited in stderr", "Error: rate limited", ""},
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
			if err.Agent != "codex" {
				t.Errorf("Agent = %q, want %q", err.Agent, "codex")
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
		{"authentication failed", "Error: authentication failed"},
		{"invalid token", "invalid token provided"},
		{"unauthorized", "401 unauthorized"},
		{"api key", "invalid api key"},
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
		{"no session", "no session to resume"},
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
	err := c.Classify(1, "", "", []string{"rate limited"})
	if err.Class != agents.ErrorClassRetryable {
		t.Errorf("Class = %v, want %v", err.Class, agents.ErrorClassRetryable)
	}
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name     string
		msg      string
		expected time.Duration
	}{
		{"retry after seconds", "retry after 45 seconds", 45 * time.Second},
		{"retry after s", "retry-after: 30s", 30 * time.Second},
		{"wait seconds", "wait: 60 seconds", 60 * time.Second},
		{"default", "rate limit exceeded", 60 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseRetryAfter(tt.msg)
			if got != tt.expected {
				t.Errorf("parseRetryAfter(%q) = %v, want %v", tt.msg, got, tt.expected)
			}
		})
	}
}

func TestClassifier_ImplementsInterface(t *testing.T) {
	var _ = NewClassifier()
}
