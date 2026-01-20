// Package comparison orchestrates the comparison of completed variants.
package comparison

import (
	"time"
)

// Result holds comparison output from Claude analysis.
type Result struct {
	Recommendation int            `json:"recommendation"`
	Confidence     string         `json:"confidence"` // high, medium, low
	Summary        string         `json:"summary"`
	FileAnalyses   []FileAnalysis `json:"file_analyses"`
	Observations   []string       `json:"observations"`

	// Documentation assessment for each variant
	DocumentationAssessment []DocAssessment `json:"documentation_assessment,omitempty"`

	// Improvements that could be adopted from non-chosen variants
	CrossVariantImprovements []CrossVariantImprovement `json:"cross_variant_improvements,omitempty"`
}

// DocAssessment evaluates the documentation quality of a variant.
type DocAssessment struct {
	VariantID       int      `json:"variant_id"`
	HasDevSetup     bool     `json:"has_dev_setup"`      // Instructions for running in development
	HasDeployment   bool     `json:"has_deployment"`     // Deployment instructions
	HasRequirements bool     `json:"has_requirements"`   // Dependencies/requirements documented
	HasUsageExamples bool    `json:"has_usage_examples"` // Examples of how to use the feature
	MissingDocs     []string `json:"missing_docs"`       // List of missing documentation
	Notes           string   `json:"notes,omitempty"`    // Additional observations
}

// CrossVariantImprovement describes an improvement from a non-chosen variant.
type CrossVariantImprovement struct {
	SourceVariantID int    `json:"source_variant_id"` // Which variant has this improvement
	Description     string `json:"description"`       // What the improvement is
	Rationale       string `json:"rationale"`         // Why it would improve the chosen variant
	Priority        string `json:"priority"`          // high, medium, low
}

// FileAnalysis contains per-file comparison details.
type FileAnalysis struct {
	Path       string         `json:"path"`
	Variants   map[int]string `json:"variants"`             // variant ID -> assessment
	Preference int            `json:"preference,omitempty"` // preferred variant ID for this file
}

// VariantData holds data for a single variant's comparison input.
type VariantData struct {
	ID      int
	Diff    string
	Metrics VariantMetrics
	Agent   string // Agent used for this variant [Req 10.6]

	// Summary mode fields (used when diff is too large)
	CommitMessages []string // Commit messages from base to HEAD
	DiffStat       string   // Summary stats (files changed, insertions, deletions)
	Changelog      string   // Changelog content if present
}

// VariantMetrics holds execution metrics for a variant.
type VariantMetrics struct {
	Cost     float64
	Duration time.Duration
	NumTurns int
}
