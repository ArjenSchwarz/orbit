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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.class.IsRetryable(); got != tt.expected {
				t.Errorf("ErrorClass.IsRetryable() = %v, want %v", got, tt.expected)
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
