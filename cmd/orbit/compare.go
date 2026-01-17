package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/arjenschwarz/orbit/internal/claude"
	"github.com/arjenschwarz/orbit/internal/comparison"
	"github.com/arjenschwarz/orbit/internal/report"
	"github.com/arjenschwarz/orbit/internal/variants"
)

// compareCommand executes the orbit compare subcommand.
// It regenerates the comparison report for existing variant worktrees.
func compareCommand(args []string) error {
	fs := flag.NewFlagSet("compare", flag.ExitOnError)

	compareCmd := fs.String("compare-command", "", "Custom comparison command (not yet supported)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: orbit compare <spec-name> [options]\n\n")
		fmt.Fprintf(os.Stderr, "Regenerate the comparison report for existing variants.\n")
		fmt.Fprintf(os.Stderr, "Useful after making manual modifications to variant worktrees.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  orbit compare my-feature\n")
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Get spec name from args or auto-detect from branch
	specName := fs.Arg(0)
	if specName == "" {
		branch, err := getGitBranchForStatus()
		if err != nil {
			return fmt.Errorf("failed to get git branch: %w\nProvide spec name as argument", err)
		}
		specName = extractSpecName(branch)
	}

	// Find and load variants.json
	specDir := filepath.Join("specs", specName)
	metadataPath := filepath.Join(specDir, ".orbit", "variants.json")

	if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
		return fmt.Errorf("no variant run found for spec: %s", specName)
	}

	// Get repo root
	repoRoot, err := getRepoRoot()
	if err != nil {
		return fmt.Errorf("failed to get repository root: %w", err)
	}

	// Load metadata using a Manager
	git := variants.NewGit(repoRoot)
	cfg := variants.DefaultConfig()
	mgr, err := variants.NewManager(cfg, specName, specDir, repoRoot, git)
	if err != nil {
		return fmt.Errorf("failed to create manager: %w", err)
	}

	if err := mgr.Load(); err != nil {
		return fmt.Errorf("failed to load variants: %w", err)
	}

	metadata := mgr.GetMetadata()
	if metadata == nil {
		return fmt.Errorf("no variant run found for spec: %s", specName)
	}

	// Count completed variants
	var completedVariants []*variants.Variant
	for _, v := range metadata.Variants {
		if v.Status == variants.StatusCompleted {
			completedVariants = append(completedVariants, v)
		}
	}

	if len(completedVariants) < 2 {
		return fmt.Errorf("at least 2 completed variants are required for comparison (found %d)", len(completedVariants))
	}

	fmt.Printf("Comparing %d variants for spec: %s\n\n", len(completedVariants), specName)

	// Collect diffs for each variant
	ctx := context.Background()
	variantData := make([]comparison.VariantData, 0, len(completedVariants))

	for _, v := range completedVariants {
		fmt.Printf("  Getting diff for variant %d...\n", v.ID)
		diff, err := git.GetDiff(ctx, v.WorktreePath, metadata.BaseCommit)
		if err != nil {
			return fmt.Errorf("failed to get diff for variant %d: %w", v.ID, err)
		}

		variantData = append(variantData, comparison.VariantData{
			ID:   v.ID,
			Diff: diff,
			Metrics: comparison.VariantMetrics{
				Cost:     v.Cost,
				Duration: v.Duration,
				NumTurns: v.NumTurns,
			},
		})
	}

	// Run comparison
	fmt.Println("\nRunning comparison analysis...")
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	claudeClient := claude.NewClient(claude.Config{
		WorkingDir: workDir,
	})

	comparator := comparison.NewComparator(claudeClient, *compareCmd)
	result, err := comparator.Compare(ctx, specName, variantData)
	if err != nil {
		return fmt.Errorf("comparison failed: %w", err)
	}

	// Generate report
	fmt.Println("Generating report...")
	reportDir := filepath.Join(specDir, "comparison-report")

	reportData := &report.ReportData{
		SpecName:       specName,
		GeneratedAt:    time.Now(),
		Comparison:     result,
		BaseCommit:     metadata.BaseCommit,
		OriginalBranch: metadata.OriginalBranch,
	}

	// Add variant data to report
	for _, v := range completedVariants {
		var diff string
		for _, vd := range variantData {
			if vd.ID == v.ID {
				diff = vd.Diff
				break
			}
		}

		reportData.Variants = append(reportData.Variants, report.VariantReportData{
			ID:     v.ID,
			Branch: v.Branch,
			Status: string(v.Status),
			Error:  v.Error,
			Diff:   diff,
			Metrics: report.VariantMetrics{
				Cost:     v.Cost,
				Duration: formatDuration(v.Duration),
				NumTurns: v.NumTurns,
			},
		})
	}

	gen := report.NewGenerator(reportDir)
	if err := gen.Generate(reportData); err != nil {
		return fmt.Errorf("failed to generate report: %w", err)
	}

	fmt.Printf("\nComparison complete!\n")
	fmt.Printf("Recommendation: Variant %d (%s confidence)\n", result.Recommendation, result.Confidence)
	fmt.Printf("Report: %s/index.html\n", reportDir)

	return nil
}

// formatDuration formats a duration for display.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		secs := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm %ds", mins, secs)
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh %dm", hours, mins)
}
