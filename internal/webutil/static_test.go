package webutil

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestNewStaticHandler_ContentType(t *testing.T) {
	t.Parallel()

	fs := fstest.MapFS{
		"app.js":    {Data: []byte("console.log('hello')")},
		"style.css": {Data: []byte("body { margin: 0 }")},
		"index.html": {Data: []byte("<html></html>")},
	}

	handler := NewStaticHandler(fs)

	tests := map[string]struct {
		path     string
		wantType string
	}{
		"javascript": {
			path:     "/app.js",
			wantType: "application/javascript; charset=utf-8",
		},
		"css": {
			path:     "/style.css",
			wantType: "text/css; charset=utf-8",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			got := rec.Header().Get("Content-Type")
			if got != tc.wantType {
				t.Errorf("Content-Type = %q, want %q", got, tc.wantType)
			}
		})
	}
}

func TestNewStaticHandler_CacheControl(t *testing.T) {
	t.Parallel()

	fs := fstest.MapFS{
		"app.js": {Data: []byte("console.log('hello')")},
	}

	handler := NewStaticHandler(fs)
	req := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	got := rec.Header().Get("Cache-Control")
	want := "public, max-age=86400"
	if got != want {
		t.Errorf("Cache-Control = %q, want %q", got, want)
	}
}
