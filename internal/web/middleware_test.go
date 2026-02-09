package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSecurityHeaders(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html></html>"))
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Check required security headers
	tests := []struct {
		header   string
		expected string
	}{
		{"X-Content-Type-Options", "nosniff"},
		{"X-Frame-Options", "DENY"},
		{"Referrer-Policy", "no-referrer"},
	}

	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			got := rec.Header().Get(tt.header)
			if got != tt.expected {
				t.Errorf("expected %s: %s, got: %s", tt.header, tt.expected, got)
			}
		})
	}

	// Check CSP for HTML content
	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Error("expected Content-Security-Policy header for HTML content")
	}
}

func TestSecurityHeadersCSPForHTML(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		expectCSP   bool
	}{
		{"HTML content", "text/html", true},
		{"HTML with charset", "text/html; charset=utf-8", true},
		{"JSON content", "application/json", false},
		{"JavaScript", "application/javascript", false},
		{"CSS", "text/css", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest("GET", "/", nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			csp := rec.Header().Get("Content-Security-Policy")
			if tt.expectCSP && csp == "" {
				t.Error("expected Content-Security-Policy header")
			}
			if !tt.expectCSP && csp != "" {
				t.Errorf("unexpected Content-Security-Policy header: %s", csp)
			}
		})
	}
}

func TestValidateUUID(t *testing.T) {
	tests := []struct {
		name       string
		runID      string
		expectCode int
	}{
		{
			name:       "valid UUID v4",
			runID:      "550e8400-e29b-41d4-a716-446655440000",
			expectCode: http.StatusOK,
		},
		{
			name:       "valid UUID v4 lowercase",
			runID:      "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
			expectCode: http.StatusOK,
		},
		{
			name:       "invalid - too short",
			runID:      "550e8400-e29b-41d4",
			expectCode: http.StatusNotFound,
		},
		{
			name:       "invalid - wrong format",
			runID:      "not-a-uuid",
			expectCode: http.StatusNotFound,
		},
		{
			name:       "invalid - uppercase",
			runID:      "550E8400-E29B-41D4-A716-446655440000",
			expectCode: http.StatusNotFound,
		},
		{
			name:       "invalid - not v4 (version 1)",
			runID:      "550e8400-e29b-11d4-a716-446655440000",
			expectCode: http.StatusNotFound,
		},
		{
			name:       "invalid - wrong variant",
			runID:      "550e8400-e29b-41d4-0716-446655440000",
			expectCode: http.StatusNotFound,
		},
		{
			name:       "invalid - path traversal",
			runID:      "../../../etc/passwd",
			expectCode: http.StatusNotFound,
		},
		{
			name:       "empty",
			runID:      "",
			expectCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := ValidateUUID("id")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest("GET", "/runs/"+tt.runID, nil)
			req.SetPathValue("id", tt.runID)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.expectCode {
				t.Errorf("expected status %d, got %d", tt.expectCode, rec.Code)
			}
		})
	}
}

func TestPathSanitizer(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		expectCode int
	}{
		{
			name:       "clean path",
			path:       "/runs/550e8400-e29b-41d4-a716-446655440000",
			expectCode: http.StatusOK,
		},
		{
			name:       "clean path with transcript",
			path:       "/runs/550e8400-e29b-41d4-a716-446655440000/transcript/1",
			expectCode: http.StatusOK,
		},
		{
			name:       "path traversal - dotdot",
			path:       "/runs/../../../etc/passwd",
			expectCode: http.StatusNotFound,
		},
		{
			name:       "path traversal - encoded",
			path:       "/runs/..%2F..%2Fetc/passwd",
			expectCode: http.StatusNotFound, // URL is decoded before path check, so ".." is detected
		},
		{
			name:       "path traversal - encoded dots",
			path:       "/runs/%2e%2e/%2e%2e/etc/passwd",
			expectCode: http.StatusNotFound,
		},
		{
			name:       "path with dotdot in value",
			path:       "/runs/foo..bar",
			expectCode: http.StatusNotFound, // Contains ".."
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := PathSanitizer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest("GET", tt.path, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.expectCode {
				t.Errorf("expected status %d, got %d", tt.expectCode, rec.Code)
			}
		})
	}
}

func TestIsPathWithinDir(t *testing.T) {
	// Create temp directory structure for testing
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	testFile := filepath.Join(subDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	tests := []struct {
		name     string
		path     string
		dir      string
		expected bool
	}{
		{
			name:     "file within dir",
			path:     testFile,
			dir:      tmpDir,
			expected: true,
		},
		{
			name:     "file within subdir",
			path:     testFile,
			dir:      subDir,
			expected: true,
		},
		{
			name:     "file outside dir",
			path:     testFile,
			dir:      filepath.Join(tmpDir, "other"),
			expected: false,
		},
		{
			name:     "path traversal attempt",
			path:     filepath.Join(subDir, "..", "other"),
			dir:      subDir,
			expected: false,
		},
		{
			name:     "nonexistent file",
			path:     filepath.Join(subDir, "nonexistent.txt"),
			dir:      subDir,
			expected: false, // File doesn't exist, so EvalSymlinks fails
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsPathWithinDir(tt.path, tt.dir)
			if result != tt.expected {
				t.Errorf("IsPathWithinDir(%q, %q) = %v, want %v", tt.path, tt.dir, result, tt.expected)
			}
		})
	}
}

func TestIsPathWithinDirSymlinks(t *testing.T) {
	// Create temp directory structure for testing symlinks
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "subdir")
	outsideDir := filepath.Join(tmpDir, "outside")

	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatalf("failed to create outside dir: %v", err)
	}

	// Create file outside subdir
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0644); err != nil {
		t.Fatalf("failed to create outside file: %v", err)
	}

	// Create symlink inside subdir pointing outside
	symlinkPath := filepath.Join(subDir, "link")
	if err := os.Symlink(outsideFile, symlinkPath); err != nil {
		t.Skipf("symlink creation not supported: %v", err)
	}

	// Symlink should be detected as outside the allowed directory
	if IsPathWithinDir(symlinkPath, subDir) {
		t.Error("symlink pointing outside should return false")
	}

	// Create symlink inside subdir pointing inside
	insideFile := filepath.Join(subDir, "file.txt")
	if err := os.WriteFile(insideFile, []byte("inside"), 0644); err != nil {
		t.Fatalf("failed to create inside file: %v", err)
	}

	internalLink := filepath.Join(subDir, "internal-link")
	if err := os.Symlink(insideFile, internalLink); err != nil {
		t.Skipf("symlink creation not supported: %v", err)
	}

	// Internal symlink should be allowed
	if !IsPathWithinDir(internalLink, subDir) {
		t.Error("symlink pointing inside should return true")
	}
}
