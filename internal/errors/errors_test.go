package errors

import (
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	tests := map[string]struct {
		exitCode int
		stderr   string
		stdout   string
		wantType ErrorType
	}{
		"rate limit in stderr": {
			exitCode: 1,
			stderr:   "error: rate limit exceeded",
			wantType: ErrRateLimit,
		},
		"429 status code": {
			exitCode: 1,
			stdout:   "API returned 429 Too Many Requests",
			wantType: ErrRateLimit,
		},
		"too many requests": {
			exitCode: 1,
			stderr:   "too many requests, please slow down",
			wantType: ErrRateLimit,
		},
		"connection error": {
			exitCode: 1,
			stderr:   "connection refused",
			wantType: ErrConnection,
		},
		"network error": {
			exitCode: 1,
			stderr:   "network unreachable",
			wantType: ErrConnection,
		},
		"timeout error": {
			exitCode: 1,
			stderr:   "request timeout",
			wantType: ErrConnection,
		},
		"dns error": {
			exitCode: 1,
			stderr:   "dns lookup failed",
			wantType: ErrConnection,
		},
		"api overloaded": {
			exitCode: 1,
			stderr:   "API is overloaded",
			wantType: ErrOverloaded,
		},
		"503 service unavailable": {
			exitCode: 1,
			stdout:   "503 service unavailable",
			wantType: ErrOverloaded,
		},
		"unknown error": {
			exitCode: 1,
			stderr:   "some unexpected error",
			wantType: ErrUnknown,
		},
		"empty output uses exit code": {
			exitCode: 42,
			stderr:   "",
			stdout:   "",
			wantType: ErrUnknown,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := Classify(tc.exitCode, tc.stderr, tc.stdout)
			if got.Type != tc.wantType {
				t.Errorf("got type %v, want %v", got.Type, tc.wantType)
			}
		})
	}
}

func TestClassifiedError_Error(t *testing.T) {
	err := &ClassifiedError{
		Type:    ErrRateLimit,
		Message: "API rate limit exceeded",
	}
	got := err.Error()
	want := "rate_limit error: API rate limit exceeded"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestErrorType_String(t *testing.T) {
	tests := map[string]struct {
		errType ErrorType
		want    string
	}{
		"connection": {ErrConnection, "connection"},
		"rate_limit": {ErrRateLimit, "rate_limit"},
		"overloaded": {ErrOverloaded, "overloaded"},
		"unknown":    {ErrUnknown, "unknown"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := tc.errType.String(); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestErrorType_IsRetryable(t *testing.T) {
	tests := map[string]struct {
		errType ErrorType
		want    bool
	}{
		"connection is retryable": {ErrConnection, true},
		"rate_limit is retryable": {ErrRateLimit, true},
		"overloaded is retryable": {ErrOverloaded, true},
		"unknown is not retryable": {ErrUnknown, false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := tc.errType.IsRetryable(); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBackoffDuration(t *testing.T) {
	tests := map[string]struct {
		attempt int
		want    time.Duration
	}{
		"attempt 0": {0, 1 * time.Second},
		"attempt 1": {1, 2 * time.Second},
		"attempt 2": {2, 4 * time.Second},
		"attempt 3": {3, 8 * time.Second},
		"attempt 4": {4, 16 * time.Second},
		"attempt 5 capped": {5, 16 * time.Second},
		"attempt 10 capped": {10, 16 * time.Second},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := BackoffDuration(tc.attempt); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseRetryAfter(t *testing.T) {
	tests := map[string]struct {
		msg  string
		want time.Duration
	}{
		"retry after seconds": {
			msg:  "rate limited, retry after 30 seconds",
			want: 30 * time.Second,
		},
		"retry-after colon": {
			msg:  "retry-after: 45s",
			want: 45 * time.Second,
		},
		"wait seconds": {
			msg:  "please wait 120 seconds",
			want: 120 * time.Second,
		},
		"no match defaults to 60s": {
			msg:  "rate limited, try again later",
			want: 60 * time.Second,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := parseRetryAfter(tc.msg)
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
