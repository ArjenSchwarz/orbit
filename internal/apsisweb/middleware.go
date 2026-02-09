package apsisweb

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/arjenschwarz/orbit/internal/sessions"
)

// ValidateSource validates the {source} path parameter against known agent types.
// Returns 404 for unknown sources.
func ValidateSource(paramName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			source := r.PathValue(paramName)
			if !sessions.IsValidSource(source) {
				http.NotFound(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SanitizeSessionID rejects session IDs containing path traversal characters.
// Checks for "..", "/", and "\" after URL decoding.
// Enforces a maximum length of 256 characters.
// Returns 404 for invalid IDs.
func SanitizeSessionID(paramName string) func(http.Handler) http.Handler {
	const maxSessionIDLength = 256

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.PathValue(paramName)
			if id == "" || len(id) > maxSessionIDLength {
				http.NotFound(w, r)
				return
			}

			// URL-decode before checking for traversal characters
			decoded, err := url.PathUnescape(id)
			if err != nil {
				http.NotFound(w, r)
				return
			}

			if strings.Contains(decoded, "..") ||
				strings.Contains(decoded, "/") ||
				strings.Contains(decoded, "\\") {
				http.NotFound(w, r)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
