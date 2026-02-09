package sessions

import (
	"fmt"
	"io"
	"slices"
	"time"
)

// Source constants — canonical identifiers used in URLs, code, and storage.
const (
	SourceClaude  = "claude"
	SourceCodex   = "codex"
	SourceCopilot = "copilot"
	SourceKiroCLI = "kiro-cli"
	SourceKiroIDE = "kiro-ide"
)

// allSources is the ordered list of all known agent sources.
var allSources = []string{SourceClaude, SourceCodex, SourceCopilot, SourceKiroCLI, SourceKiroIDE}

// displayNames maps source constants to their display strings.
var displayNames = map[string]string{
	SourceClaude:  "claude",
	SourceCodex:   "codex",
	SourceCopilot: "copilot",
	SourceKiroCLI: "kiro-cli",
	SourceKiroIDE: "kiro ide",
}

// AllSources returns all known source identifiers.
func AllSources() []string {
	out := make([]string, len(allSources))
	copy(out, allSources)
	return out
}

// DisplayName returns the display string for a source constant.
// Used by apsis -l to preserve existing output format.
// e.g., "kiro-ide" -> "kiro ide", "claude" -> "claude"
func DisplayName(source string) string {
	if name, ok := displayNames[source]; ok {
		return name
	}
	return source
}

// IsValidSource returns true if the source is a known agent type.
func IsValidSource(source string) bool {
	return slices.Contains(allSources, source)
}

// FormatSize formats a file size in human-readable format.
// e.g., 1536 -> "1.5 KB", 0 -> "0 B"
func FormatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// SessionInfo holds metadata about a discovered session.
type SessionInfo struct {
	ID        string
	CreatedAt time.Time
	Size      int64
	Source    string // One of the Source* constants
}

// SessionMetadata is returned by the Resolver alongside the reader.
type SessionMetadata struct {
	Source    string
	ID       string
	CreatedAt time.Time
	Size      int64
	CostPath  string // Non-empty only for Kiro IDE sessions
}

// ResolvedSession is the result of resolving a session by source and ID.
type ResolvedSession struct {
	Reader   io.ReadCloser
	Metadata SessionMetadata
}

// ListWarning represents a non-fatal error during session listing.
type ListWarning struct {
	Source string // Which agent source failed
	Err    error  // The underlying error
}
