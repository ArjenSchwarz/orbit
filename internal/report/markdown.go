package report

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	output "github.com/ArjenSchwarz/go-output/v2"
)

// generateMarkdownReport creates report.md alongside the HTML report using go-output v2.
// The Markdown report contains the same content as the HTML report for use with
// GitHub repository browsing. [Req 1.1]
func (g *Generator) generateMarkdownReport(data *ReportData) error {
	builder := output.New()

	// Title and basic info
	builder.SetMetadata("title", fmt.Sprintf("Comparison Report: %s", data.SpecName))

	// Add recommendation section if comparison exists
	if data.Comparison != nil {
		builder.Section("Recommendation", func(b *output.Builder) {
			b.Text(fmt.Sprintf("**Recommended:** Variant %d (%s confidence)",
				data.Comparison.Recommendation, data.Comparison.Confidence))
			b.Text(data.Comparison.Summary)
		})
	}

	// Run Information section
	builder.Section("Run Information", func(b *output.Builder) {
		infoRecords := []output.Record{
			{"Field": "Base Commit", "Value": fmt.Sprintf("`%s`", data.BaseCommit)},
			{"Field": "Original Branch", "Value": data.OriginalBranch},
			{"Field": "Generated", "Value": data.GeneratedAt.Format("2006-01-02 15:04:05")},
		}
		b.Table("", infoRecords, output.WithKeys("Field", "Value"))
	})

	// Variants Overview table
	builder.Section("Variants Overview", func(b *output.Builder) {
		variantRecords := make([]output.Record, 0, len(data.Variants))
		for _, v := range data.Variants {
			variantName := fmt.Sprintf("Variant %d", v.ID)
			if data.Comparison != nil && data.Comparison.Recommendation == v.ID {
				variantName += " (Recommended)"
			}

			agent := "-"
			if v.Agent != "" {
				agent = v.Agent
			}

			variantRecords = append(variantRecords, output.Record{
				"Variant":  variantName,
				"Branch":   fmt.Sprintf("`%s`", v.Branch),
				"Agent":    agent,
				"Status":   v.Status,
				"Cost":     formatCost(v.Metrics.Cost, v.Metrics.CostUnit),
				"Duration": v.Metrics.Duration,
				"Turns":    v.Metrics.NumTurns,
			})
		}
		b.Table("", variantRecords, output.WithKeys("Variant", "Branch", "Agent", "Status", "Cost", "Duration", "Turns"))
	})

	// Key Observations
	if data.Comparison != nil && len(data.Comparison.Observations) > 0 {
		builder.Section("Key Observations", func(b *output.Builder) {
			var observations strings.Builder
			for _, obs := range data.Comparison.Observations {
				observations.WriteString(fmt.Sprintf("- %s\n", obs))
			}
			b.Raw(output.FormatMarkdown, []byte(observations.String()))
		})
	}

	// Per-File Analysis
	if data.Comparison != nil && len(data.Comparison.FileAnalyses) > 0 {
		builder.Section("Per-File Analysis", func(b *output.Builder) {
			for _, fa := range data.Comparison.FileAnalyses {
				header := fmt.Sprintf("`%s`", fa.Path)
				if fa.Preference > 0 {
					header += fmt.Sprintf(" (Variant %d preferred)", fa.Preference)
				}
				b.Raw(output.FormatMarkdown, []byte(fmt.Sprintf("### %s\n", header)))
				for id, assessment := range fa.Variants {
					b.Raw(output.FormatMarkdown, []byte(fmt.Sprintf("- **Variant %d:** %s\n", id, assessment)))
				}
			}
		})
	}

	// Documentation Assessment
	if data.Comparison != nil && len(data.Comparison.DocumentationAssessment) > 0 {
		builder.Section("Documentation Assessment", func(b *output.Builder) {
			docRecords := make([]output.Record, 0, len(data.Comparison.DocumentationAssessment))
			for _, da := range data.Comparison.DocumentationAssessment {
				docRecords = append(docRecords, output.Record{
					"Variant":        fmt.Sprintf("Variant %d", da.VariantID),
					"Dev Setup":      boolToCheck(da.HasDevSetup),
					"Deployment":     boolToCheck(da.HasDeployment),
					"Requirements":   boolToCheck(da.HasRequirements),
					"Usage Examples": boolToCheck(da.HasUsageExamples),
					"Missing":        strings.Join(da.MissingDocs, ", "),
				})
			}
			b.Table("", docRecords, output.WithKeys("Variant", "Dev Setup", "Deployment", "Requirements", "Usage Examples", "Missing"))
		})
	}

	// Cross-Variant Improvements
	if data.Comparison != nil && len(data.Comparison.CrossVariantImprovements) > 0 {
		builder.Section("Improvements from Other Variants", func(b *output.Builder) {
			b.Text("These improvements from non-chosen variants could enhance the recommended implementation:")
			for _, imp := range data.Comparison.CrossVariantImprovements {
				b.Raw(output.FormatMarkdown, []byte(fmt.Sprintf("### From Variant %d (%s priority)\n", imp.SourceVariantID, imp.Priority)))
				b.Raw(output.FormatMarkdown, []byte(imp.Description+"\n"))
				b.Raw(output.FormatMarkdown, []byte(fmt.Sprintf("**Rationale:** %s\n", imp.Rationale)))
			}
		})
	}

	// Implementation Diffs
	builder.Section("Implementation Diffs", func(b *output.Builder) {
		for _, v := range data.Variants {
			header := fmt.Sprintf("Variant %d", v.ID)
			if v.Agent != "" {
				header += fmt.Sprintf(" (%s)", v.Agent)
			}
			if data.Comparison != nil && data.Comparison.Recommendation == v.ID {
				header += " - Recommended"
			}

			b.Section(header, func(sb *output.Builder) {
				if v.Error != "" {
					sb.Text(fmt.Sprintf("**Error:** %s", v.Error))
				} else if v.DiffFile != "" {
					// Create relative link for markdown [Req 1.2]
					sb.Text(fmt.Sprintf("Diff is large (>%d lines). [View full diff](%s)", LargeDiffThreshold, v.DiffFile))
				} else if v.Diff != "" {
					sb.Raw(output.FormatMarkdown, []byte(fmt.Sprintf("```diff\n%s\n```", v.Diff)))
				} else {
					sb.Text("No changes from base commit.")
				}
			}, output.WithLevel(3))
		}
	})

	// Build document and render to markdown
	doc := builder.Build()

	// Render the document content (without frontmatter, we'll add our own)
	mdFormat := output.Markdown()
	rendered, err := mdFormat.Renderer.Render(context.Background(), doc)
	if err != nil {
		return fmt.Errorf("render markdown: %w", err)
	}

	// Build custom YAML frontmatter with variant_commits for staleness detection [Req 1.7]
	frontMatter := g.buildFrontMatter(data)

	// Combine frontmatter and content
	var finalContent strings.Builder
	finalContent.WriteString(frontMatter)
	finalContent.Write(rendered)

	// Write to file
	mdPath := filepath.Join(g.outputDir, "report.md")
	if err := writeFileAtomic(mdPath, []byte(finalContent.String())); err != nil {
		return fmt.Errorf("write report.md: %w", err)
	}

	return nil
}

// buildFrontMatter generates YAML frontmatter with metadata for staleness detection.
// Implements: [1.7]
func (g *Generator) buildFrontMatter(data *ReportData) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("title: \"Comparison Report: %s\"\n", data.SpecName))
	sb.WriteString(fmt.Sprintf("generated_at: %s\n", data.GeneratedAt.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("base_commit: %s\n", data.BaseCommit))

	// Add variant commits for staleness detection
	if len(data.VariantCommits) > 0 {
		sb.WriteString("variant_commits:\n")
		// Sort keys for deterministic output
		var ids []int
		for id := range data.VariantCommits {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		for _, id := range ids {
			sb.WriteString(fmt.Sprintf("  %d: %s\n", id, data.VariantCommits[id]))
		}
	}
	sb.WriteString("---\n\n")
	return sb.String()
}

// boolToCheck converts a boolean to a check mark or X.
func boolToCheck(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

// writeFileAtomic writes data to a file atomically by writing to a temp file first.
func writeFileAtomic(path string, data []byte) error {
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename to final: %w", err)
	}
	return nil
}
