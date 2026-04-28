package web

import (
	"net/http"
	"net/url"
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

		// Wrap response to add CSP for HTML content
		wrapper := &cspResponseWriter{ResponseWriter: w}
		next.ServeHTTP(wrapper, r)
	})
}

// cspResponseWriter wraps http.ResponseWriter to add CSP header for HTML content.
type cspResponseWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

func (w *cspResponseWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.wroteHeader = true
		// Add CSP for HTML content before writing header
		contentType := w.Header().Get("Content-Type")
		if strings.HasPrefix(contentType, "text/html") {
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'")
		}
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *cspResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
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
// Checks both raw and URL-decoded paths for ".." to prevent directory traversal.
func PathSanitizer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if containsDotDot(r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		if decoded, err := url.PathUnescape(r.URL.Path); err == nil && containsDotDot(decoded) {
			http.NotFound(w, r)
			return
		}
		if r.URL.RawPath != "" {
			if containsDotDot(r.URL.RawPath) {
				http.NotFound(w, r)
				return
			}
			if decoded, err := url.PathUnescape(r.URL.RawPath); err == nil && containsDotDot(decoded) {
				http.NotFound(w, r)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func containsDotDot(path string) bool {
	return path != "" && strings.Contains(path, "..")
}

// IsPathWithinDir checks if path is within dir after resolving symlinks.
// Returns false if the path resolves to a location outside the allowed directory.
func IsPathWithinDir(path, dir string) bool {
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

	// The file is outside the directory only when the first path segment
	// of rel is the literal parent-directory marker "..". Compare it as a
	// discrete segment so legitimate names that merely begin with ".."
	// (e.g. "..cache/file.txt", "..session.jsonl") are still treated as
	// in-tree.
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}
