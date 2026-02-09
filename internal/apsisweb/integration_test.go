package apsisweb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents/claudecode"
)

// TestIntegrationEndToEnd starts a real server with mock Claude session data
// and verifies session listing, transcript rendering, 404 handling, and
// security headers through actual HTTP requests.
func TestIntegrationEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Set up fake home and project directories
	homeDir := t.TempDir()
	projectDir := filepath.Join(homeDir, "project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	// Redirect HOME so Lister/Resolver find our mock data
	t.Setenv("HOME", homeDir)

	// Create a mock Claude session file
	sessionID := "test-integration-session"
	claudeProjectPath := claudecode.BuildProjectPath(projectDir)
	claudeDir := filepath.Join(homeDir, ".claude", "projects", claudeProjectPath)
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("failed to create claude dir: %v", err)
	}

	sessionContent := createMockSession(t, time.Date(2025, 6, 15, 14, 30, 0, 0, time.UTC))
	sessionFile := filepath.Join(claudeDir, sessionID+".jsonl")
	if err := os.WriteFile(sessionFile, sessionContent, 0644); err != nil {
		t.Fatalf("failed to write session: %v", err)
	}

	// Start server on ephemeral port
	port := findAvailablePort(t)
	srv, err := New(Config{
		Port:        port,
		Bind:        "127.0.0.1",
		ProjectPath: projectDir,
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	errChan := make(chan error, 1)
	go func() {
		errChan <- srv.Start()
	}()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitForServer(t, fmt.Sprintf("127.0.0.1:%d", port))

	t.Run("session list contains mock session", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/")
		if err != nil {
			t.Fatalf("GET / failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		body := readBody(t, resp)
		if !strings.Contains(body, sessionID) {
			t.Error("session list should contain mock session ID")
		}
		if !strings.Contains(body, "claude") {
			t.Error("session list should show claude source")
		}
	})

	t.Run("transcript renders successfully", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/sessions/claude/" + sessionID)
		if err != nil {
			t.Fatalf("GET /sessions/claude/%s failed: %v", sessionID, err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		body := readBody(t, resp)
		if !strings.Contains(body, sessionID) {
			t.Error("transcript page should contain session ID")
		}
		if !strings.Contains(body, "Hello from integration test") {
			t.Error("transcript page should contain message content")
		}
	})

	t.Run("nonexistent session returns 404", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/sessions/claude/no-such-session")
		if err != nil {
			t.Fatalf("GET nonexistent session failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("invalid source returns 404", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/sessions/invalid-agent/some-id")
		if err != nil {
			t.Fatalf("GET invalid source failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("security headers present", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/")
		if err != nil {
			t.Fatalf("GET / failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		checks := map[string]string{
			"X-Content-Type-Options": "nosniff",
			"X-Frame-Options":       "DENY",
			"Referrer-Policy":       "no-referrer",
		}
		for header, want := range checks {
			if got := resp.Header.Get(header); got != want {
				t.Errorf("%s = %q, want %q", header, got, want)
			}
		}
	})

	t.Run("HTMX fragment endpoint returns partial HTML", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/sessions")
		if err != nil {
			t.Fatalf("GET /api/sessions failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		body := readBody(t, resp)
		if strings.Contains(body, "<!DOCTYPE html>") {
			t.Error("fragment should not contain full HTML layout")
		}
		if !strings.Contains(body, sessionID) {
			t.Error("fragment should contain session ID")
		}
	})

	// Clean shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Errorf("shutdown error: %v", err)
	}

	select {
	case err := <-errChan:
		if err != nil && err != http.ErrServerClosed {
			t.Errorf("server error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("server did not stop in time")
	}
}

// createMockSession creates a minimal valid Claude JSONL session.
func createMockSession(t *testing.T, ts time.Time) []byte {
	t.Helper()

	lines := []map[string]any{
		{
			"type":      "user",
			"timestamp": ts.Format(time.RFC3339),
			"message": map[string]any{
				"role": "user",
				"content": []map[string]any{
					{"type": "text", "text": "Hello from integration test"},
				},
			},
		},
		{
			"type":      "assistant",
			"timestamp": ts.Add(time.Second).Format(time.RFC3339),
			"message": map[string]any{
				"role": "assistant",
				"content": []map[string]any{
					{"type": "text", "text": "Hello! I'm responding to the integration test."},
				},
			},
		},
	}

	var buf []byte
	for _, line := range lines {
		data, err := json.Marshal(line)
		if err != nil {
			t.Fatalf("failed to marshal: %v", err)
		}
		buf = append(buf, data...)
		buf = append(buf, '\n')
	}
	return buf
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	return string(data)
}
