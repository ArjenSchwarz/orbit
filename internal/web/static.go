package web

import (
	"embed"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"strings"
)

//go:embed static/*
var staticFS embed.FS

// staticHandler serves embedded static files with correct content types.
type staticHandler struct {
	fs http.Handler
}

// newStaticHandler creates a handler for serving embedded static files.
func newStaticHandler() http.Handler {
	// Create a sub-filesystem rooted at "static"
	subFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		// This should never happen with embedded files
		panic("failed to create static sub-filesystem: " + err.Error())
	}

	return &staticHandler{
		fs: http.FileServer(http.FS(subFS)),
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

// stripPrefix removes a prefix from the request URL path for the static handler.
func stripPrefix(prefix string, h http.Handler) http.Handler {
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
