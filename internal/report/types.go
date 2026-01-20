// Package report generates HTML comparison reports for multi-variant runs.
package report

import (
	"time"

	"github.com/arjenschwarz/orbit/internal/comparison"
)

// ReportData holds all data for report generation.
type ReportData struct {
	SpecName       string
	GeneratedAt    time.Time
	Variants       []VariantReportData
	Comparison     *comparison.Result
	BaseCommit     string
	OriginalBranch string
}

// VariantReportData holds per-variant data for report rendering.
type VariantReportData struct {
	ID       int
	Branch   string
	Status   string
	Error    string
	Diff     string
	DiffFile string // Relative path to separate diff file if diff is large
	Metrics  VariantMetrics
	Agent    string // Agent used for this variant [Req 10.6]
}

// VariantMetrics holds execution metrics for report display.
type VariantMetrics struct {
	Cost     float64
	Duration string // Pre-formatted duration string
	NumTurns int
}
