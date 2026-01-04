package web

import (
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
)

// uuidV4Pattern matches valid UUID v4 format (lowercase only).
// Format: 8-4-4-4-12 hex chars, with version 4 and variant 8, 9, a, or b.
var uuidV4Pattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// SecurityHeaders adds security headers to all responses.
// Headers added:
// - X-Content-Type-Options: nosniff
// - X-Frame-Options: DENY
// - Referrer-Policy: no-referrer
// - Content-Security-Policy (for HTML responses only)
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add security headers before calling next handler
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")

		// Use a response wrapper to add CSP for HTML content
		wrapper := &responseWrapper{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(wrapper, r)

		// Add CSP for HTML content
		contentType := w.Header().Get("Content-Type")
		if strings.HasPrefix(contentType, "text/html") {
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'")
		}
	})
}

// responseWrapper captures the response for inspection.
type responseWrapper struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWrapper) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// ValidateUUID validates that a path parameter is a valid UUID v4.
// Returns 404 for invalid UUIDs to prevent path enumeration.
func ValidateUUID(paramName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.PathValue(paramName)
			if id == "" || !uuidV4Pattern.MatchString(id) {
				http.NotFound(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// PathSanitizer rejects requests with path traversal attempts.
// Returns 404 for paths containing ".." to prevent directory traversal.
func PathSanitizer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if the path contains ".."
		if strings.Contains(r.URL.Path, "..") {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isPathWithinDir checks if path is within dir after resolving symlinks.
// Returns false if the path resolves to a location outside the allowed directory.
func isPathWithinDir(path, dir string) bool {
	// Resolve symlinks for both paths
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}

	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false
	}

	// Get relative path from resolved dir to resolved path
	rel, err := filepath.Rel(resolvedDir, resolved)
	if err != nil {
		return false
	}

	// If relative path starts with "..", the file is outside the directory
	return !strings.HasPrefix(rel, "..")
}
