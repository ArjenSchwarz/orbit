package agents

import (
	"errors"
	"testing"
	"time"
)

func TestErrorClass_String(t *testing.T) {
	tests := []struct {
		name     string
		class    ErrorClass
		expected string
	}{
		{
			name:     "unknown error class",
			class:    ErrorClassUnknown,
			expected: "unknown",
		},
		{
			name:     "retryable error class",
			class:    ErrorClassRetryable,
			expected: "retryable",
		},
		{
			name:     "fatal error class",
			class:    ErrorClassFatal,
			expected: "fatal",
		},
		{
			name:     "session invalid error class",
			class:    ErrorClassSessionInvalid,
			expected: "session-invalid",
		},
		{
			name:     "rate limit wait error class",
			class:    ErrorClassRateLimitWait,
			expected: "rate-limit-wait",
		},
		{
			name:     "undefined error class returns unknown",
			class:    ErrorClass(99),
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.class.String(); got != tt.expected {
				t.Errorf("ErrorClass.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestErrorClass_IsRetryable(t *testing.T) {
	tests := []struct {
		name     string
		class    ErrorClass
		expected bool
	}{
		{
			name:     "unknown is not retryable",
			class:    ErrorClassUnknown,
			expected: false,
		},
		{
			name:     "retryable is retryable",
			class:    ErrorClassRetryable,
			expected: true,
		},
		{
			name:     "fatal is not retryable",
			class:    ErrorClassFatal,
			expected: false,
		},
		{
			name:     "session invalid is not retryable",
			class:    ErrorClassSessionInvalid,
			expected: false,
		},
		{
			name:     "rate limit wait is retryable",
			class:    ErrorClassRateLimitWait,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.class.IsRetryable(); got != tt.expected {
				t.Errorf("ErrorClass.IsRetryable() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestErrorClass_IsRateLimitWait(t *testing.T) {
	tests := []struct {
		name     string
		class    ErrorClass
		expected bool
	}{
		{
			name:     "unknown is not rate limit wait",
			class:    ErrorClassUnknown,
			expected: false,
		},
		{
			name:     "retryable is not rate limit wait",
			class:    ErrorClassRetryable,
			expected: false,
		},
		{
			name:     "fatal is not rate limit wait",
			class:    ErrorClassFatal,
			expected: false,
		},
		{
			name:     "session invalid is not rate limit wait",
			class:    ErrorClassSessionInvalid,
			expected: false,
		},
		{
			name:     "rate limit wait is rate limit wait",
			class:    ErrorClassRateLimitWait,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.class.IsRateLimitWait(); got != tt.expected {
				t.Errorf("ErrorClass.IsRateLimitWait() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestClassifiedError_Error(t *testing.T) {
	err := &ClassifiedError{
		Original:   errors.New("original error"),
		Class:      ErrorClassRetryable,
		RetryAfter: 30 * time.Second,
		Message:    "rate limit exceeded",
		Agent:      "claude-code",
	}

	got := err.Error()
	if got != "rate limit exceeded" {
		t.Errorf("ClassifiedError.Error() = %q, want %q", got, "rate limit exceeded")
	}
}

func TestClassifiedError_Unwrap(t *testing.T) {
	originalErr := errors.New("original error")
	err := &ClassifiedError{
		Original:   originalErr,
		Class:      ErrorClassFatal,
		RetryAfter: 0,
		Message:    "authentication failed",
		Agent:      "codex",
	}

	unwrapped := err.Unwrap()
	if unwrapped != originalErr {
		t.Errorf("ClassifiedError.Unwrap() = %v, want %v", unwrapped, originalErr)
	}
}

func TestClassifiedError_ErrorsIs(t *testing.T) {
	originalErr := errors.New("original error")
	err := &ClassifiedError{
		Original: originalErr,
		Class:    ErrorClassRetryable,
		Message:  "test error",
		Agent:    "test-agent",
	}

	// errors.Is should work through Unwrap
	if !errors.Is(err, originalErr) {
		t.Error("errors.Is should find the original error through Unwrap")
	}
}

func TestClassifiedError_Fields(t *testing.T) {
	originalErr := errors.New("connection timeout")
	err := &ClassifiedError{
		Original:   originalErr,
		Class:      ErrorClassRetryable,
		RetryAfter: 60 * time.Second,
		Message:    "network error: connection timeout",
		Agent:      "kiro",
	}

	if err.Class != ErrorClassRetryable {
		t.Errorf("Class = %v, want %v", err.Class, ErrorClassRetryable)
	}
	if err.RetryAfter != 60*time.Second {
		t.Errorf("RetryAfter = %v, want %v", err.RetryAfter, 60*time.Second)
	}
	if err.Message != "network error: connection timeout" {
		t.Errorf("Message = %q, want %q", err.Message, "network error: connection timeout")
	}
	if err.Agent != "kiro" {
		t.Errorf("Agent = %q, want %q", err.Agent, "kiro")
	}
}

func TestErrorClassifier_Interface(t *testing.T) {
	// Test that ErrorClassifier interface is properly defined
	// by creating a mock implementation
	var _ ErrorClassifier = &mockClassifier{}
}

type mockClassifier struct{}

func (m *mockClassifier) Classify(exitCode int, stderr, stdout string, errMsgs []string) *ClassifiedError {
	return &ClassifiedError{
		Class:   ErrorClassUnknown,
		Message: "mock classification",
	}
}

func TestMatchesSessionInvalid(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		extra    []string
		expected bool
	}{
		{"session not found", "error: session not found", nil, true},
		{"invalid session", "invalid session id", nil, true},
		{"session expired", "session expired", nil, true},
		{"no match", "some other error", nil, false},
		{"empty string", "", nil, false},
		{"case sensitive requires lowercase input", "SESSION NOT FOUND", nil, false},
		{"extra pattern matches", "no such session", []string{"no such session"}, true},
		{"extra pattern no match", "some error", []string{"no such session"}, false},
		{"common pattern with extra provided", "session not found", []string{"no such session"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesSessionInvalid(tt.input, tt.extra...)
			if got != tt.expected {
				t.Errorf("MatchesSessionInvalid(%q, %v) = %v, want %v", tt.input, tt.extra, got, tt.expected)
			}
		})
	}
}

func TestNewSessionInvalidError(t *testing.T) {
	err := NewSessionInvalidError("test-agent")

	if err.Class != ErrorClassSessionInvalid {
		t.Errorf("Class = %v, want %v", err.Class, ErrorClassSessionInvalid)
	}
	if err.Agent != "test-agent" {
		t.Errorf("Agent = %q, want %q", err.Agent, "test-agent")
	}
	if err.Message != "Session not found or expired" {
		t.Errorf("Message = %q, want %q", err.Message, "Session not found or expired")
	}
	if err.Original == nil {
		t.Error("Original should not be nil")
	}
}

func TestBackoffDuration(t *testing.T) {
	tests := map[string]struct {
		attempt int
		want    time.Duration
	}{
		"attempt 0":         {0, 1 * time.Second},
		"attempt 1":         {1, 2 * time.Second},
		"attempt 2":         {2, 4 * time.Second},
		"attempt 3":         {3, 8 * time.Second},
		"attempt 4":         {4, 16 * time.Second},
		"attempt 5 capped":  {5, 16 * time.Second},
		"attempt 10 capped": {10, 16 * time.Second},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := BackoffDuration(tc.attempt); got != tc.want {
				t.Errorf("BackoffDuration(%d) = %v, want %v", tc.attempt, got, tc.want)
			}
		})
	}
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name     string
		msg      string
		expected time.Duration
	}{
		{"retry after seconds", "retry after 45 seconds", 45 * time.Second},
		{"retry-after colon", "retry-after: 30s", 30 * time.Second},
		{"wait seconds", "wait: 60 seconds", 60 * time.Second},
		{"default when no match", "rate limit exceeded", DefaultRateLimitRetryAfter},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseRetryAfter(tt.msg)
			if got != tt.expected {
				t.Errorf("ParseRetryAfter(%q) = %v, want %v", tt.msg, got, tt.expected)
			}
		})
	}
}
