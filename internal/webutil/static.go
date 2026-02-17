// Package webutil provides shared utilities for web servers in Orbit and Apsis.
package webutil

import (
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"strings"
)

// staticHandler serves static files with correct content types and cache headers.
type staticHandler struct {
	fs http.Handler
}

// NewStaticHandler creates a handler that serves static files from the given filesystem.
// The filesystem should already be rooted at the static directory (e.g., via fs.Sub).
func NewStaticHandler(fsys fs.FS) http.Handler {
	return &staticHandler{
		fs: http.FileServer(http.FS(fsys)),
	}
}

func (h *staticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Set content type based on file extension
	ext := path.Ext(r.URL.Path)
	switch ext {
	case ".js":
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	}

	// Set cache headers for static assets
	w.Header().Set("Cache-Control", "public, max-age=86400") // 1 day

	h.fs.ServeHTTP(w, r)
}

// StripPrefix removes a prefix from the request URL path before passing to the handler.
// If the path doesn't have the prefix, it returns 404.
func StripPrefix(prefix string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, prefix)
		if len(p) < len(r.URL.Path) {
			r2 := new(http.Request)
			*r2 = *r
			r2.URL = new(url.URL)
			*r2.URL = *r.URL
			r2.URL.Path = p
			h.ServeHTTP(w, r2)
		} else {
			http.NotFound(w, r)
		}
	})
}
