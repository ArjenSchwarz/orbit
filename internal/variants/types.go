// Package variants manages git worktrees and branches for multi-variant spec implementations.
package variants

import (
	"time"
)

// VariantStatus represents the execution state of a variant.
type VariantStatus string

const (
	StatusPending   VariantStatus = "pending"
	StatusRunning   VariantStatus = "running"
	StatusCompleted VariantStatus = "completed"
	StatusFailed    VariantStatus = "failed"
	StatusCanceled  VariantStatus = "canceled"
)

// Variant represents a single implementation variant.
type Variant struct {
	ID           int           `json:"id"`
	Branch       string        `json:"branch"`
	WorktreePath string        `json:"worktree_path"`
	Status       VariantStatus `json:"status"`
	Error        string        `json:"error,omitempty"`
	Guidance     string        `json:"guidance,omitempty"`
	Agent        string        `json:"agent,omitempty"`      // Agent alias name used for this variant
	AgentType    string        `json:"agent_type,omitempty"` // Underlying agent type (e.g., "claude-code")
	Model        string        `json:"model,omitempty"`      // Model used for this variant

	// Metrics populated after completion
	Cost     float64       `json:"cost,omitempty"`
	CostUnit string        `json:"cost_unit,omitempty"` // Cost unit type: "USD", "credits", or "premium_requests"
	Duration time.Duration `json:"duration,omitempty"`
	NumTurns int           `json:"num_turns,omitempty"`
}

// VariantsMetadata is the root structure for variants.json.
type VariantsMetadata struct {
	RunID          string     `json:"run_id"`
	BaseCommit     string     `json:"base_commit"`
	OriginalBranch string     `json:"original_branch"`
	StartedAt      time.Time  `json:"started_at"`
	Variants       []*Variant `json:"variants"`
}

// Config holds variant execution configuration.
type Config struct {
	Count        int
	Parallel     bool
	MaxParallel  int
	BranchPrefix string
	Guidance     []string // Per-variant guidance from file
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Count:        1,
		Parallel:     false,
		MaxParallel:  3,
		BranchPrefix: "orbit-impl",
	}
}
