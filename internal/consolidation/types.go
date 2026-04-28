// Package consolidation provides types and utilities for consolidating improvements
// from multiple implementation variants into a single chosen variant.
package consolidation

import (
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
)

// DefaultTimeout is the maximum duration for a single consolidation agent
// invocation. This bounds both the consolidation prompt (runWithRetry) and the
// post-prompt (runPostPrompt) so a stalled API connection or hung session can
// never run indefinitely.
const DefaultTimeout = 30 * time.Minute

// Config holds consolidation configuration.
type Config struct {
	SpecName     string
	SpecDir      string
	VariantID    int
	Agent        agents.Agent
	AllowDirty   bool
	PostPrompt   string        // AI prompt after consolidation (renamed from PostCommand)
	CustomPrompt string        // User-provided instructions via --prompt
	Timeout      time.Duration // Per-invocation agent timeout (0 = DefaultTimeout)
}

// ConsolidationResult contains the outcome of a consolidation run.
type ConsolidationResult struct {
	CommitSHA        string
	AgentReport      string // Raw report from agent (displayed to user)
	TestsPassed      bool
	PostPromptPassed bool // Whether post-prompt completed successfully
	Errors           []string
}

// ConsolidationReport is parsed from agent output for logging purposes.
// The agent produces this as part of its output; we parse it for the log.
type ConsolidationReport struct {
	Applied []AppliedImprovement
	Skipped []SkippedImprovement
}

// AppliedImprovement describes an improvement that was implemented.
type AppliedImprovement struct {
	SourceVariantID int
	Description     string
	FilesModified   []string
}

// SkippedImprovement describes an improvement that was not applied.
type SkippedImprovement struct {
	SourceVariantID int
	Description     string
	Reason          string
}
