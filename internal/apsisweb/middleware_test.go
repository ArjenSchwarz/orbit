package apsisweb

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateSource(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name       string
		source     string
		wantStatus int
	}{
		{"valid claude", "claude", http.StatusOK},
		{"valid codex", "codex", http.StatusOK},
		{"valid copilot", "copilot", http.StatusOK},
		{"valid kiro-cli", "kiro-cli", http.StatusOK},
		{"valid kiro-ide", "kiro-ide", http.StatusOK},
		{"invalid source", "unknown", http.StatusNotFound},
		// Note: empty source produces a 301 redirect from Go's ServeMux
		// (trailing slash cleanup), not a middleware concern.
		{"partial match", "claud", http.StatusNotFound},
		{"case sensitive", "Claude", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.Handle("GET /sessions/{source}/test", ValidateSource("source")(okHandler))

			req := httptest.NewRequest("GET", "/sessions/"+tt.source+"/test", nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("ValidateSource(%q): got status %d, want %d", tt.source, rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestSanitizeSessionID(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name       string
		id         string
		wantStatus int
	}{
		{"clean id", "abc123", http.StatusOK},
		{"uuid-like id", "550e8400-e29b-41d4-a716-446655440000", http.StatusOK},
		{"path traversal dotdot", "..%2Fetc%2Fpasswd", http.StatusNotFound},
		{"contains slash", "foo/bar", http.StatusNotFound},
		{"contains backslash", "foo\\bar", http.StatusNotFound},
		{"dotdot in middle", "foo..bar", http.StatusNotFound},
		{"url-encoded dotdot", "%2e%2e", http.StatusNotFound},
		{"url-encoded slash", "foo%2fbar", http.StatusNotFound},
		{"url-encoded backslash", "foo%5cbar", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.Handle("GET /sessions/{id}", SanitizeSessionID("id")(okHandler))

			req := httptest.NewRequest("GET", "/sessions/"+tt.id, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("SanitizeSessionID(%q): got status %d, want %d", tt.id, rec.Code, tt.wantStatus)
			}
		})
	}
}
