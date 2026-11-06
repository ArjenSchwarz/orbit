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
	// SourceCodexExec is a Codex CLI `codex exec --json` event stream.
	SourceCodexExec = "codex-exec"
)

// allSources is the ordered list of all known agent sources.
var allSources = []string{SourceClaude, SourceCodex, SourceCopilot, SourceKiroCLI, SourceKiroIDE, SourceCodexExec}

// displayNames maps source constants to their display strings.
var displayNames = map[string]string{
	SourceClaude:    "claude",
	SourceCodex:     "codex",
	SourceCopilot:   "copilot",
	SourceKiroCLI:   "kiro-cli",
	SourceKiroIDE:   "kiro ide",
	SourceCodexExec: "codex-exec",
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
	ID                 string
	CreatedAt          time.Time // Sort time: the recorded creation time, else the file modification time
	Size               int64
	Source             string // One of the Source* constants
	TimeFromFilesystem bool   // CreatedAt is the file modification time, not a recorded time
	Project            string // Recorded project directory, when the listing read yields it
	Model              string // First recorded model, when the listing read yields it
	Provider           string // First recorded model provider, when the listing read yields it
	Origin             string // Claude entrypoint or Codex originator from the bounded head scan
	ThreadID           string // Codex exec thread ID; codex-exec only
}

// SessionMetadata is returned by the Resolver alongside the reader.
type SessionMetadata struct {
	Source             string
	ID                 string
	CreatedAt          time.Time
	TimeFromFilesystem bool      // CreatedAt is the file modification time, not a recorded time
	DatabaseCreatedAt  time.Time // Kiro CLI database created_at; CreatedAt stays unset, as the web view expects
	Size               int64
	CostPath           string // Non-empty only for Kiro IDE sessions
	Project            string // Recorded project outside the transcript, such as Copilot's workspace.yaml git_root
	Path               string // Path of the resolved transcript file
	ThreadID           string // Codex exec thread ID; codex-exec only
}

// ResolvedSession is the result of resolving a session by source and ID.
type ResolvedSession struct {
	Reader   io.ReadCloser
	Metadata SessionMetadata
}

// Listing warning codes, shared with shared JSON listing documents.
const (
	WarningSourceFailed     = "source_failed"     // a source could not be listed
	WarningPathUnreadable   = "path_unreadable"   // a session file, or a file beside it, could not be read or parsed
	WarningDuplicateSession = "duplicate_session" // all-projects scope listed one copy of a session found in several places

	WarningRootUnreadable     = "root_unreadable"     // a configured exec root is missing or cannot be read
	WarningMissingThread      = "missing_thread"      // an exec stream has no valid thread.started first; omitted
	WarningIdentifierConflict = "identifier_conflict" // two exec streams derive one identifier; both omitted
)

// ListWarning represents a non-fatal error during session listing.
type ListWarning struct {
	Source string // Which agent source failed
	Code   string // One of the Warning* codes
	Err    error  // The underlying error
}

// Options are the settings shared by a Lister and a Resolver.
type Options struct {
	ExtendedOrdering bool     // sort by time, then source, then identifier (requirement 4.4)
	CodexExecRoots   []string // directories searched for Codex exec streams; none by default
}

// Option changes Options. NewLister and NewResolver apply the same functions.
type Option func(*Options)

// WithExtendedOrdering enables the deterministic source-then-identifier tie
// order. Without it, listings keep the legacy order.
func WithExtendedOrdering(enabled bool) Option {
	return func(o *Options) { o.ExtendedOrdering = enabled }
}

// WithCodexExecRoots sets the directories searched for Codex exec streams.
// Configured roots also enable extended ordering, since exec rows are ordered
// by file modification time beside recorded times.
func WithCodexExecRoots(roots []string) Option {
	return func(o *Options) { o.CodexExecRoots = slices.Clone(roots) }
}

// applyOptions returns the Options that opts produce.
func applyOptions(opts []Option) Options {
	var o Options
	for _, opt := range opts {
		opt(&o)
	}
	if len(o.CodexExecRoots) > 0 {
		o.ExtendedOrdering = true
	}
	return o
}
