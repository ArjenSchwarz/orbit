package apsisweb

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/arjenschwarz/orbit/internal/webutil"
)

//go:embed static/*
var staticFS embed.FS

// newStaticHandler creates a handler for serving embedded static files.
func newStaticHandler() http.Handler {
	subFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic("failed to create static sub-filesystem: " + err.Error())
	}

	return webutil.NewStaticHandler(subFS)
}
