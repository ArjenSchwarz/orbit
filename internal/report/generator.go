package report

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LargeDiffThreshold is the number of lines above which a diff is stored
// in a separate file. This keeps the main report manageable.
const LargeDiffThreshold = 500

// Generator creates HTML comparison reports.
type Generator struct {
	outputDir string
}

// NewGenerator creates a Generator that outputs to the specified directory.
func NewGenerator(outputDir string) *Generator {
	return &Generator{
		outputDir: outputDir,
	}
}

// Generate creates the HTML report files.
func (g *Generator) Generate(data *ReportData) error {
	// Create output directory
	if err := os.MkdirAll(g.outputDir, 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	// Process variants to escape content and handle large diffs
	processedData := g.processReportData(data)

	// Generate main report
	if err := g.generateMainReport(processedData); err != nil {
		return fmt.Errorf("generate main report: %w", err)
	}

	return nil
}

// processReportData handles large diffs by extracting them to separate files.
// Note: HTML escaping is handled automatically by html/template.
func (g *Generator) processReportData(data *ReportData) *ReportData {
	// Create a copy, processing variants for large diffs
	processed := &ReportData{
		SpecName:       data.SpecName,
		GeneratedAt:    data.GeneratedAt,
		Comparison:     data.Comparison,
		BaseCommit:     data.BaseCommit,
		OriginalBranch: data.OriginalBranch,
	}

	// Process each variant for large diffs
	processed.Variants = make([]VariantReportData, len(data.Variants))
	for i, v := range data.Variants {
		processed.Variants[i] = g.processVariant(data.SpecName, v)
	}

	return processed
}

// processVariant handles large diffs by extracting them to separate files.
// Note: HTML escaping is handled automatically by html/template.
func (g *Generator) processVariant(specName string, v VariantReportData) VariantReportData {
	processed := VariantReportData{
		ID:      v.ID,
		Branch:  v.Branch,
		Status:  v.Status,
		Error:   v.Error,
		Metrics: v.Metrics,
	}

	// Check if diff is large
	lineCount := countLines(v.Diff)
	if lineCount > LargeDiffThreshold {
		// Generate separate diff file
		diffFile, err := g.generateDiffFile(specName, v.ID, v.Diff)
		if err != nil {
			// Fall back to inline diff on error
			processed.Diff = v.Diff
		} else {
			processed.DiffFile = diffFile
		}
	} else {
		processed.Diff = v.Diff
	}

	return processed
}

// generateMainReport creates index.html.
func (g *Generator) generateMainReport(data *ReportData) error {
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "index.html", data); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}

	indexPath := filepath.Join(g.outputDir, "index.html")
	if err := os.WriteFile(indexPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("write index.html: %w", err)
	}

	return nil
}

// generateDiffFile creates a separate diff file for large diffs.
// Note: HTML escaping is handled automatically by html/template.
func (g *Generator) generateDiffFile(specName string, variantID int, diff string) (string, error) {
	// Create diffs directory
	diffsDir := filepath.Join(g.outputDir, "diffs")
	if err := os.MkdirAll(diffsDir, 0755); err != nil {
		return "", fmt.Errorf("create diffs directory: %w", err)
	}

	// Prepare template data
	data := struct {
		SpecName  string
		VariantID int
		Diff      string
	}{
		SpecName:  specName,
		VariantID: variantID,
		Diff:      diff,
	}

	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "diff.html", data); err != nil {
		return "", fmt.Errorf("execute diff template: %w", err)
	}

	filename := fmt.Sprintf("variant-%d.html", variantID)
	diffPath := filepath.Join(diffsDir, filename)
	if err := os.WriteFile(diffPath, buf.Bytes(), 0644); err != nil {
		return "", fmt.Errorf("write diff file: %w", err)
	}

	// Return relative path from index.html
	return filepath.Join("diffs", filename), nil
}

// countLines returns the number of lines in a string.
func countLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}
