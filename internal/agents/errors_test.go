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
		{"retry-after numeric only", "retry-after: 120", 120 * time.Second},
		{"retry after numeric no unit", "retry after 90", 90 * time.Second},
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

func TestMatchesRateLimit(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		extra    []string
		expected bool
	}{
		{"rate limit", "error: rate limit exceeded", nil, true},
		{"rate_limit", "rate_limit_error", nil, true},
		{"429 status", "http 429 response", nil, true},
		{"too many requests", "too many requests", nil, true},
		{"no match", "authentication failed", nil, false},
		{"empty string", "", nil, false},
		{"extra pattern matches", "request throttled", []string{"throttl"}, true},
		{"extra pattern no match", "some error", []string{"throttl"}, false},
		{"common pattern with extra", "rate limit hit", []string{"throttl"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesRateLimit(tt.input, tt.extra...)
			if got != tt.expected {
				t.Errorf("MatchesRateLimit(%q, %v) = %v, want %v", tt.input, tt.extra, got, tt.expected)
			}
		})
	}
}

func TestMatchesAuthError(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		extra    []string
		expected bool
	}{
		{"unauthorized", "401 unauthorized", nil, true},
		{"invalid token", "invalid token provided", nil, true},
		{"api key", "missing api key", nil, true},
		{"no match", "connection timeout", nil, false},
		{"empty string", "", nil, false},
		{"extra pattern matches", "not authenticated", []string{"not authenticated"}, true},
		{"extra pattern no match", "rate limit", []string{"not authenticated"}, false},
		{"common pattern with extra", "unauthorized access", []string{"credentials"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesAuthError(tt.input, tt.extra...)
			if got != tt.expected {
				t.Errorf("MatchesAuthError(%q, %v) = %v, want %v", tt.input, tt.extra, got, tt.expected)
			}
		})
	}
}

func TestMatchesConnectionError(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		extra    []string
		expected bool
	}{
		{"connection", "connection refused", nil, true},
		{"network", "network error", nil, true},
		{"timeout", "request timeout", nil, true},
		{"dns", "dns resolution failed", nil, true},
		{"unreachable", "host unreachable", nil, true},
		{"no match", "authentication failed", nil, false},
		{"empty string", "", nil, false},
		{"extra pattern matches", "econnrefused", []string{"econnrefused"}, true},
		{"extra pattern no match", "rate limit", []string{"econnrefused"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesConnectionError(tt.input, tt.extra...)
			if got != tt.expected {
				t.Errorf("MatchesConnectionError(%q, %v) = %v, want %v", tt.input, tt.extra, got, tt.expected)
			}
		})
	}
}

func TestMatchesOverload(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		extra    []string
		expected bool
	}{
		{"overloaded", "api overloaded", nil, true},
		{"503", "http 503 error", nil, true},
		{"service unavailable", "service unavailable", nil, true},
		{"no match", "authentication failed", nil, false},
		{"empty string", "", nil, false},
		{"extra pattern matches", "temporarily unavailable", []string{"temporarily unavailable"}, true},
		{"extra pattern no match", "rate limit", []string{"temporarily unavailable"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesOverload(tt.input, tt.extra...)
			if got != tt.expected {
				t.Errorf("MatchesOverload(%q, %v) = %v, want %v", tt.input, tt.extra, got, tt.expected)
			}
		})
	}
}

func TestNewRateLimitError(t *testing.T) {
	// RetryAfter is parsed via ParseRetryAfter, so this also validates that path.
	err := NewRateLimitError("test-agent", "retry after 30 seconds")

	if err.Class != ErrorClassRetryable {
		t.Errorf("Class = %v, want %v", err.Class, ErrorClassRetryable)
	}
	if err.Agent != "test-agent" {
		t.Errorf("Agent = %q, want %q", err.Agent, "test-agent")
	}
	if err.Message != "API rate limit exceeded" {
		t.Errorf("Message = %q, want %q", err.Message, "API rate limit exceeded")
	}
	if err.RetryAfter != 30*time.Second {
		t.Errorf("RetryAfter = %v, want %v", err.RetryAfter, 30*time.Second)
	}
	if err.Original == nil {
		t.Error("Original should not be nil")
	}
}

func TestNewRateLimitError_DefaultRetryAfter(t *testing.T) {
	err := NewRateLimitError("test-agent", "rate limit exceeded")

	if err.RetryAfter != DefaultRateLimitRetryAfter {
		t.Errorf("RetryAfter = %v, want %v", err.RetryAfter, DefaultRateLimitRetryAfter)
	}
}

func TestNewAuthError(t *testing.T) {
	err := NewAuthError("test-agent")

	if err.Class != ErrorClassFatal {
		t.Errorf("Class = %v, want %v", err.Class, ErrorClassFatal)
	}
	if err.Agent != "test-agent" {
		t.Errorf("Agent = %q, want %q", err.Agent, "test-agent")
	}
	if err.Message != "Authentication error" {
		t.Errorf("Message = %q, want %q", err.Message, "Authentication error")
	}
	if err.Original == nil {
		t.Error("Original should not be nil")
	}
}

func TestNewConnectionError(t *testing.T) {
	err := NewConnectionError("test-agent")

	if err.Class != ErrorClassRetryable {
		t.Errorf("Class = %v, want %v", err.Class, ErrorClassRetryable)
	}
	if err.Agent != "test-agent" {
		t.Errorf("Agent = %q, want %q", err.Agent, "test-agent")
	}
	if err.Message != "Network connection error" {
		t.Errorf("Message = %q, want %q", err.Message, "Network connection error")
	}
	if err.Original == nil {
		t.Error("Original should not be nil")
	}
}

func TestNewOverloadError(t *testing.T) {
	err := NewOverloadError("test-agent")

	if err.Class != ErrorClassRetryable {
		t.Errorf("Class = %v, want %v", err.Class, ErrorClassRetryable)
	}
	if err.Agent != "test-agent" {
		t.Errorf("Agent = %q, want %q", err.Agent, "test-agent")
	}
	if err.Message != "API is overloaded" {
		t.Errorf("Message = %q, want %q", err.Message, "API is overloaded")
	}
	if err.RetryAfter != DefaultOverloadRetryAfter {
		t.Errorf("RetryAfter = %v, want %v", err.RetryAfter, DefaultOverloadRetryAfter)
	}
	if err.Original == nil {
		t.Error("Original should not be nil")
	}
}

func TestNewUnknownError(t *testing.T) {
	tests := []struct {
		name        string
		errMsgs     []string
		stderr      string
		stdout      string
		expectedMsg string
	}{
		{
			name:        "uses errMsgs first",
			errMsgs:     []string{"error one", "error two"},
			stderr:      "stderr content",
			stdout:      "stdout content",
			expectedMsg: "error one; error two",
		},
		{
			name:        "falls back to stderr",
			errMsgs:     nil,
			stderr:      "stderr content",
			stdout:      "stdout content",
			expectedMsg: "stderr content",
		},
		{
			name:        "falls back to stdout",
			errMsgs:     nil,
			stderr:      "",
			stdout:      "stdout content",
			expectedMsg: "stdout content",
		},
		{
			name:        "defaults to unknown error",
			errMsgs:     nil,
			stderr:      "",
			stdout:      "",
			expectedMsg: "unknown error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewUnknownError("test-agent", tt.errMsgs, tt.stderr, tt.stdout)
			if err.Class != ErrorClassUnknown {
				t.Errorf("Class = %v, want %v", err.Class, ErrorClassUnknown)
			}
			if err.Agent != "test-agent" {
				t.Errorf("Agent = %q, want %q", err.Agent, "test-agent")
			}
			if err.Message != tt.expectedMsg {
				t.Errorf("Message = %q, want %q", err.Message, tt.expectedMsg)
			}
		})
	}
}

func TestBuildUnknownMessage(t *testing.T) {
	tests := []struct {
		name     string
		errMsgs  []string
		stderr   string
		stdout   string
		expected string
	}{
		{"errMsgs joined", []string{"a", "b"}, "x", "y", "a; b"},
		{"stderr fallback", nil, "stderr msg", "stdout msg", "stderr msg"},
		{"stdout fallback", nil, "", "stdout msg", "stdout msg"},
		{"default", nil, "", "", "unknown error"},
		{"empty errMsgs", []string{}, "stderr msg", "", "stderr msg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildUnknownMessage(tt.errMsgs, tt.stderr, tt.stdout)
			if got != tt.expected {
				t.Errorf("BuildUnknownMessage(%v, %q, %q) = %q, want %q", tt.errMsgs, tt.stderr, tt.stdout, got, tt.expected)
			}
		})
	}
}
