// Package display provides terminal display functionality for orbit.
package display

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattn/go-isatty"
)

// OSC 8 escape sequence format for terminal hyperlinks:
// ESC]8;;<uri>ESC\<text>ESC]8;;ESC\
const (
	osc8Start = "\x1b]8;;"
	osc8Sep   = "\x1b\\"
	osc8End   = "\x1b]8;;\x1b\\"
)

// FormatOSC8Link creates an OSC 8 terminal hyperlink.
// Returns plain text if stderr is not a TTY.
func FormatOSC8Link(uri, text string) string {
	return formatOSC8LinkWithTTY(uri, text, IsTTY(os.Stderr))
}

// formatOSC8LinkWithTTY is the internal implementation that accepts a TTY flag.
// This allows testing without depending on actual TTY state.
func formatOSC8LinkWithTTY(uri, text string, isTTY bool) string {
	if !isTTY {
		return text
	}
	return osc8Start + uri + osc8Sep + text + osc8End
}

// FormatFileLink creates a file:// URI from an absolute path.
// Properly encodes special characters using net/url.
func FormatFileLink(absPath string) string {
	// Split path into components and encode each part
	parts := strings.Split(absPath, string(filepath.Separator))
	encodedParts := make([]string, len(parts))

	for i, part := range parts {
		encodedParts[i] = url.PathEscape(part)
	}

	// Join with forward slashes for URI format
	encodedPath := strings.Join(encodedParts, "/")

	return "file://" + encodedPath
}

// PrintIndexLinks outputs the index file links to stderr.
// Does nothing if sessionDir is empty.
func PrintIndexLinks(sessionDir string) {
	printIndexLinksTo(os.Stderr, sessionDir, IsTTY(os.Stderr))
}

// printIndexLinksTo is the internal implementation that writes to a specific writer.
// This allows testing without writing to actual stderr.
func printIndexLinksTo(w io.Writer, sessionDir string, isTTY bool) {
	if sessionDir == "" {
		return
	}

	mdPath := filepath.Join(sessionDir, "index.md")
	htmlPath := filepath.Join(sessionDir, "index.html")

	mdURI := FormatFileLink(mdPath)
	htmlURI := FormatFileLink(htmlPath)

	mdLink := formatOSC8LinkWithTTY(mdURI, mdPath, isTTY)
	htmlLink := formatOSC8LinkWithTTY(htmlURI, htmlPath, isTTY)

	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Session Logs:")
	_, _ = fmt.Fprintf(w, "  Markdown: %s\n", mdLink)
	_, _ = fmt.Fprintf(w, "  HTML:     %s\n", htmlLink)
}

// IsTTY checks if a file descriptor is a terminal.
func IsTTY(f *os.File) bool {
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}
