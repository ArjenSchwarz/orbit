package opencode

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
			if err.Agent != "opencode" {
				t.Errorf("Agent = %q, want %q", err.Agent, "opencode")
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
		{"authentication failed", "AuthenticationError: authentication failed"},
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

func TestClassifier_Classify_ModelNotFound(t *testing.T) {
	c := NewClassifier()

	tests := []struct {
		name   string
		stderr string
		stdout string
	}{
		{"ProviderModelNotFoundError", "ProviderModelNotFoundError: model not found", ""},
		{"model not found", "Error: model not found", ""},
		{"invalid model", "invalid model specified", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.Classify(1, tt.stderr, tt.stdout, nil)
			if err.Class != agents.ErrorClassFatal {
				t.Errorf("Class = %v, want %v", err.Class, agents.ErrorClassFatal)
			}
		})
	}
}

func TestClassifier_Classify_RealOpenCodeModelError(t *testing.T) {
	// Test with actual OpenCode error output for invalid model
	c := NewClassifier()

	// This is the actual output from: opencode run "test" --model github-copilot/claude-opus-4.8 --format json
	stdout := `1076 |     const info = provider.models[modelID]
1077 |     if (!info) {
1078 |       const availableModels = Object.keys(provider.models)
1079 |       const matches = fuzzysort.go(modelID, availableModels, { limit: 3, threshold: -10000 })
1080 |       const suggestions = matches.map((m) => m.target)
1081 |       throw new ModelNotFoundError({ providerID, modelID, suggestions })
                   ^
ProviderModelNotFoundError: ProviderModelNotFoundError
 data: {
  providerID: "github-copilot",
  modelID: "claude-opus-4.8",
  suggestions: [],
},

      at getModel (src/provider/provider.ts:1081:13)`

	stderr := "INFO  2026-01-27T23:52:47 +39ms service=models.dev file={} refreshing"

	// Exit code 0 but plaintext output (not JSON)
	err := c.Classify(0, stderr, stdout, nil)

	if err.Class != agents.ErrorClassFatal {
		t.Errorf("Class = %v, want %v (fatal for model not found)", err.Class, agents.ErrorClassFatal)
	}
	if err.Agent != "opencode" {
		t.Errorf("Agent = %q, want %q", err.Agent, "opencode")
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

	// Valid JSON output should result in unknown error type
	err := c.Classify(1, "some random error", `{"success": true}`, nil)
	if err.Class != agents.ErrorClassUnknown {
		t.Errorf("Class = %v, want %v", err.Class, agents.ErrorClassUnknown)
	}
}

func TestClassifier_Classify_ErrMsgs(t *testing.T) {
	c := NewClassifier()

	// Error messages from errMsgs should be checked
	err := c.Classify(1, "", "", []string{"rate limited"})
	if err.Class != agents.ErrorClassRetryable {
		t.Errorf("Class = %v, want %v", err.Class, agents.ErrorClassRetryable)
	}
}

func TestClassifier_Classify_PlaintextOutput_DetectsError(t *testing.T) {
	c := NewClassifier()

	// OpenCode may exit with code 0 but return plaintext error
	// instead of JSON. This should be detected as an error.
	tests := []struct {
		name       string
		stdout     string
		wantClass  agents.ErrorClass
		wantUnkown bool
	}{
		{
			name:       "stack trace error",
			stdout:     "Error: something went wrong\n  at someFunction()",
			wantClass:  agents.ErrorClassUnknown,
			wantUnkown: true,
		},
		{
			name:       "model not found in plaintext",
			stdout:     "ProviderModelNotFoundError: anthropic/invalid-model",
			wantClass:  agents.ErrorClassFatal,
			wantUnkown: false,
		},
		{
			name:       "auth error in plaintext",
			stdout:     "AuthenticationError: invalid api key",
			wantClass:  agents.ErrorClassFatal,
			wantUnkown: false,
		},
		{
			name:       "rate limit in plaintext",
			stdout:     "Error: rate limit exceeded, retry after 60 seconds",
			wantClass:  agents.ErrorClassRetryable,
			wantUnkown: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.Classify(0, "", tt.stdout, nil) // exit code 0 but plaintext output
			if err.Class != tt.wantClass {
				t.Errorf("Class = %v, want %v", err.Class, tt.wantClass)
			}
		})
	}
}

func TestClassifier_Classify_ValidJSON_NoError(t *testing.T) {
	c := NewClassifier()

	// Valid JSON output with no error patterns should return unknown
	err := c.Classify(0, "", `{"status": "ok", "result": "success"}`, nil)
	if err.Class != agents.ErrorClassUnknown {
		t.Errorf("Class = %v, want %v", err.Class, agents.ErrorClassUnknown)
	}
}

func TestClassifier_ImplementsInterface(t *testing.T) {
	var _ = NewClassifier()
}

func TestIsValidJSONOutput(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty string", "", false},
		{"whitespace only", "   ", false},
		{"valid object", `{"key": "value"}`, true},
		{"valid array", `[1, 2, 3]`, true},
		{"plaintext error", "Error: something went wrong", false},
		{"stack trace", "Error: failed\n  at func()", false},
		{"json with whitespace", `  {"key": "value"}  `, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidJSONOutput(tt.input)
			if got != tt.want {
				t.Errorf("isValidJSONOutput(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
