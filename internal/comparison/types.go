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
}

// VariantMetrics holds execution metrics for a variant.
type VariantMetrics struct {
	Cost     float64
	Duration time.Duration
	NumTurns int
}
