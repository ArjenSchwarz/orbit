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

// StatusOutput represents the complete status output for all variants.
// This enables JSON/Markdown output formats via go-output.
type StatusOutput struct {
	SpecName       string          `json:"spec_name"`
	BaseCommit     string          `json:"base_commit"`
	OriginalBranch string          `json:"original_branch"`
	StartedAt      string          `json:"started_at"`
	ActiveVariants []VariantOutput `json:"active_variants"`
	OtherVariants  []VariantOutput `json:"other_variants"`
}

// VariantOutput represents a single variant in the output.
type VariantOutput struct {
	ID         int            `json:"id"`
	Branch     string         `json:"branch"`
	Status     string         `json:"status"`
	GitState   string         `json:"git_state,omitempty"`
	Commits    []CommitOutput `json:"commits,omitempty"`
	LastAction string         `json:"last_action,omitempty"`
	Tasks      []TaskOutput   `json:"tasks,omitempty"`
	Error      string         `json:"error,omitempty"`
}

// CommitOutput represents a single commit in the output.
type CommitOutput struct {
	Hash    string `json:"hash"`
	Subject string `json:"subject"`
}

// TaskOutput represents a single phase's task progress.
type TaskOutput struct {
	Phase     string `json:"phase"`
	Completed int    `json:"completed"`
	Total     int    `json:"total"`
	IsActive  bool   `json:"is_active"`
}

// BuildStatusOutput creates a StatusOutput from metadata and variant info.
func BuildStatusOutput(specName string, baseCommit, originalBranch string, startedAt string, infos []*VariantInfo) *StatusOutput {
	result := &StatusOutput{
		SpecName:       specName,
		BaseCommit:     truncateHash(baseCommit, 12),
		OriginalBranch: originalBranch,
		StartedAt:      startedAt,
	}

	for _, info := range infos {
		vo := BuildVariantOutput(info)
		if info.Status == variants.StatusRunning || info.Status == variants.StatusFailed {
			result.ActiveVariants = append(result.ActiveVariants, vo)
		} else {
			result.OtherVariants = append(result.OtherVariants, vo)
		}
	}

	return result
}

// BuildVariantOutput creates a VariantOutput from a VariantInfo.
func BuildVariantOutput(info *VariantInfo) VariantOutput {
	vo := VariantOutput{
		ID:     info.ID,
		Branch: info.Branch,
		Status: string(info.Status),
		Error:  info.Error,
	}

	// Git info
	if info.GitInfo != nil {
		vo.GitState = info.GitInfo.DirtyState
		for _, c := range info.GitInfo.Commits {
			vo.Commits = append(vo.Commits, CommitOutput{Hash: c.Hash, Subject: c.Subject})
		}
	}

	// Last action
	if info.LastAction != nil {
		switch info.LastAction.State {
		case LastActionFound:
			vo.LastAction = info.LastAction.Summary
		case LastActionWaiting:
			vo.LastAction = "Waiting for activity..."
		case LastActionUnavailable:
			vo.LastAction = "Transcript unavailable"
		case LastActionNotSupported:
			vo.LastAction = "Last action tracking not available for " + info.AgentType
		}
	}

	// Task progress
	if info.TaskProgress != nil {
		for _, p := range info.TaskProgress.Phases {
			vo.Tasks = append(vo.Tasks, TaskOutput{
				Phase:     p.Name,
				Completed: p.Completed,
				Total:     p.Total,
				IsActive:  p.IsActive,
			})
		}
	}

	return vo
}

// truncateHash truncates a hash to the specified length.
func truncateHash(hash string, length int) string {
	if len(hash) <= length {
		return hash
	}
	return hash[:length]
}
