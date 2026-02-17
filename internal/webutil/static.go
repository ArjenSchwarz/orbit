// Package webutil provides shared utilities for web servers in Orbit and Apsis.
package webutil

import (
	"io/fs"
	"net/http"
	"path"
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

