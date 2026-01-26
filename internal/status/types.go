// Package status provides variant status data gathering for the orbit status command.
package status

import (
	"github.com/arjenschwarz/orbit/internal/rune"
	"github.com/arjenschwarz/orbit/internal/variants"
)

// VariantInfo contains all gathered information for a single variant.
type VariantInfo struct {
	// From variants.json
	ID           int
	Branch       string
	WorktreePath string
	Status       variants.VariantStatus
	AgentType    string
	Error        string

	// Git information (nil if gathering failed)
	GitInfo *GitInfo

	// Last action from transcript
	LastAction *LastActionResult

	// Task progress (nil if gathering failed)
	TaskProgress *TaskProgress
}

// GitInfo contains git-related status for a variant.
type GitInfo struct {
	Commits    []variants.Commit // Most recent commits (up to 3)
	IsDirty    bool              // Has uncommitted changes
	DirtyState string            // "clean" or "dirty"
}

// LastActionResult represents the result of attempting to get the last action.
// Uses explicit state to distinguish between different outcomes.
type LastActionResult struct {
	State   LastActionState
	Summary string // Only set when State == LastActionFound
}

// LastActionState represents the outcome of last action retrieval.
type LastActionState int

const (
	// LastActionFound means a displayable action was found
	LastActionFound LastActionState = iota
	// LastActionWaiting means no session ID yet or transcript doesn't exist
	LastActionWaiting
	// LastActionUnavailable means there was an error reading/parsing
	LastActionUnavailable
	// LastActionNotSupported means the agent type doesn't support transcript access
	LastActionNotSupported
)

// TaskProgress contains phase-by-phase task completion status.
type TaskProgress struct {
	Phases []PhaseProgress
}

// PhaseProgress contains task counts for a single phase.
type PhaseProgress struct {
	Name      string
	Completed int
	Total     int
	IsActive  bool // Currently in progress
}

// FromRunePhaseSummary converts a slice of rune.PhaseSummary to PhaseProgress.
func FromRunePhaseSummary(summaries []rune.PhaseSummary) []PhaseProgress {
	phases := make([]PhaseProgress, len(summaries))
	for i, s := range summaries {
		phases[i] = PhaseProgress{
			Name:      s.Name,
			Completed: s.Completed,
			Total:     s.Total,
			IsActive:  s.Status == rune.PhaseStatusInProgress,
		}
	}
	return phases
}
