package orbit

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/arjenschwarz/orbit/internal/comparison"
	"github.com/arjenschwarz/orbit/internal/consolidation"
	"github.com/arjenschwarz/orbit/internal/cost"
	"github.com/arjenschwarz/orbit/internal/report"
	"github.com/arjenschwarz/orbit/internal/variants"
)

// runComparison performs the comparison of successful variants.
func (o *Orbit) runComparison(ctx context.Context) error {
	log.Println("Running comparison of successful variants...")

	metadata := o.variantManager.GetMetadata()
	if metadata == nil {
		return fmt.Errorf("no variant metadata available")
	}

	// Gather summaries from successful variants (not full diffs - they use too much context)
	gitClient := variants.NewGit(o.config.RepoRoot)
	diffGatherer := comparison.NewDiffGatherer(gitClient)
	variantList := o.variantManager.GetVariantsSnapshot()

	variantData, err := diffGatherer.GatherSummaries(ctx, metadata.BaseCommit, variantList)
	if err != nil {
		return fmt.Errorf("gather summaries: %w", err)
	}

	if len(variantData) < 2 {
		log.Println("Not enough successful variants for comparison (need at least 2)")
		return nil
	}

	// Read spec context for additional context
	specContext := o.readSpecContext()

	// Create comparator with timeout to prevent indefinite hangs from stalled API connections.
	comparisonCtx, cancel := context.WithTimeout(o.shutdownCtx, comparison.DefaultTimeout)
	defer cancel()

	adapter := comparison.NewAgentAdapter(o.agent, comparisonCtx, o.config.WorkingDir)
	comparator := comparison.NewComparator(adapter, o.config.CompareCommand)

	comparisonJSONPath := filepath.Join(o.config.SpecDir, ".orbit", "comparison.json")
	comparisonInput := comparison.ComparisonInput{
		SpecName:    o.config.BranchName,
		SpecContext: specContext,
		Variants:    variantData,
		IncludeDiff: false,
		OutputPath:  comparisonJSONPath,
	}
	result, err := comparator.CompareUnified(ctx, comparisonInput)
	if err != nil {
		return fmt.Errorf("compare variants: %w", err)
	}

	// Check if agent wrote the JSON file as instructed
	if _, statErr := os.Stat(comparisonJSONPath); os.IsNotExist(statErr) {
		log.Printf("Warning: agent did not write comparison JSON to %s", comparisonJSONPath)
	}

	log.Printf("Comparison complete: recommends variant %d (confidence: %s)",
		result.Recommendation, result.Confidence)
	log.Printf("Summary: %s", result.Summary)

	// Store comparison result for report generation
	o.comparisonResult = result

	return nil
}

// runAutoConsolidate runs consolidation on the recommended variant after comparison.
// It applies improvements from non-chosen variants to the recommended variant.
// Failures are non-fatal and logged as warnings.
func (o *Orbit) runAutoConsolidate(ctx context.Context) error {
	if o.comparisonResult == nil || o.comparisonResult.Recommendation == 0 {
		log.Println("Skipping auto-consolidation: no recommendation from comparison")
		return nil
	}

	variantID := o.comparisonResult.Recommendation
	log.Printf("Running auto-consolidation on recommended variant %d...", variantID)

	// Get the variant to check its worktree state
	variant := o.variantManager.GetVariant(variantID)
	if variant == nil {
		return fmt.Errorf("recommended variant %d not found", variantID)
	}

	// Check for uncommitted changes unless --allow-dirty is set
	if !o.config.AllowDirty {
		cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
		cmd.Dir = variant.WorktreePath
		out, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("failed to check git status: %w", err)
		}
		if len(strings.TrimSpace(string(out))) > 0 {
			log.Println("Skipping auto-consolidation: worktree has uncommitted changes (use --allow-dirty to override)")
			return nil
		}
	}

	// Create the agent using default agent config (not variant-specific).
	// Resolve type from config to handle alias-based agent names.
	agentType := o.config.AgentConfig.Type
	if agentType == "" {
		agentType = o.config.Agent
	}
	agent, err := o.config.AgentResolver.GetAgent(agentType, o.config.AgentConfig)
	if err != nil {
		return fmt.Errorf("failed to create agent for consolidation: %w", err)
	}

	// Create consolidator configuration
	consolidatorCfg := consolidation.Config{
		SpecName:   o.config.BranchName,
		SpecDir:    o.config.SpecDir,
		VariantID:  variantID,
		Agent:      agent,
		AllowDirty: o.config.AllowDirty,
	}

	consolidator, err := consolidation.NewConsolidator(consolidatorCfg, o.variantManager)
	if err != nil {
		return fmt.Errorf("failed to create consolidator: %w", err)
	}

	// Run consolidation
	result, err := consolidator.Run(ctx)
	if err != nil {
		if errors.Is(err, consolidation.ErrNoImprovements) {
			log.Println("Auto-consolidation: no cross-variant improvements found")
			// Run post-consolidate-command even when there are no improvements
			if cmdErr := o.runPostConsolidateCommand(ctx, variant.WorktreePath); cmdErr != nil {
				log.Printf("Warning: post-consolidate-command failed: %v", cmdErr)
			}
			return nil
		}
		return fmt.Errorf("consolidation failed: %w", err)
	}

	// Log success
	if result.CommitSHA != "" {
		log.Printf("Auto-consolidation commit: %s", result.CommitSHA)
	}
	if !result.TestsPassed {
		log.Println("Warning: auto-consolidation tests failed")
	}

	// Run post-consolidate-command after successful consolidation
	if cmdErr := o.runPostConsolidateCommand(ctx, variant.WorktreePath); cmdErr != nil {
		log.Printf("Warning: post-consolidate-command failed: %v", cmdErr)
	}

	return nil
}

// runPostConsolidateCommand executes the post-consolidate-command in the given worktree.
func (o *Orbit) runPostConsolidateCommand(ctx context.Context, worktreePath string) error {
	if o.config.PostConsolidateCommand == "" {
		return nil
	}

	if o.config.DryRun {
		log.Printf("[DRY RUN] Would execute post-consolidate-command: %s", o.config.PostConsolidateCommand)
		log.Printf("[DRY RUN] Working directory: %s", worktreePath)
		return nil
	}

	log.Printf("Running post-consolidate-command: %s", o.config.PostConsolidateCommand)

	// Create context with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, o.config.CommandTimeout)
	defer cancel()

	// Execute using /bin/sh -c
	cmd := exec.CommandContext(cmdCtx, "/bin/sh", "-c", o.config.PostConsolidateCommand)
	cmd.Dir = worktreePath

	// Set up environment
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("ORBIT_AGENT=%s", o.agent.Name()),
	)

	// Capture output
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Log output on failure
		if stdout.Len() > 0 {
			log.Printf("post-consolidate-command stdout:\n%s", stdout.String())
		}
		if stderr.Len() > 0 {
			log.Printf("post-consolidate-command stderr:\n%s", stderr.String())
		}

		if cmdCtx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("command timed out after %v", o.config.CommandTimeout)
		}
		return err
	}

	log.Println("Post-consolidate-command completed successfully")
	return nil
}

// generateReport creates the HTML comparison report.
func (o *Orbit) generateReport() error {
	log.Println("Generating comparison report...")

	metadata := o.variantManager.GetMetadata()
	if metadata == nil {
		return fmt.Errorf("no variant metadata available")
	}

	// Gather diffs for report
	gitClient := variants.NewGit(o.config.RepoRoot)
	diffGatherer := comparison.NewDiffGatherer(gitClient)
	variantList := o.variantManager.GetVariantsSnapshot()

	variantData, err := diffGatherer.GatherDiffs(context.Background(), metadata.BaseCommit, variantList)
	if err != nil {
		log.Printf("Warning: could not gather diffs for report: %v", err)
	}

	// Build variant data map for lookup
	variantDiffs := make(map[int]string)
	for _, vd := range variantData {
		variantDiffs[vd.ID] = vd.Diff
	}

	// Build report data
	reportVariants := make([]report.VariantReportData, 0, len(variantList))
	for _, v := range variantList {
		// Build cost totals from variant data
		var costTotals cost.Totals
		if v.CostTotals.USD > 0 || v.CostTotals.Credits > 0 || v.CostTotals.PremiumRequests > 0 {
			costTotals = v.CostTotals
		} else {
			// Construct totals from single cost value
			switch v.CostUnit {
			case cost.UnitCredits:
				costTotals.Credits = v.Cost
			case cost.UnitPremiumRequests:
				costTotals.PremiumRequests = v.Cost
			default:
				// Default to USD if unknown or explicitly USD
				costTotals.USD = v.Cost
			}
		}

		reportVariants = append(reportVariants, report.VariantReportData{
			ID:     v.ID,
			Branch: v.Branch,
			Status: string(v.Status),
			Error:  v.Error,
			Diff:   variantDiffs[v.ID],
			Agent:  v.Agent,
			Metrics: report.VariantMetrics{
				Cost:         &costTotals,
				Duration:     v.Duration.Round(time.Second).String(),
				NumTurns:     v.NumTurns,
				LinesAdded:   v.LinesAdded,
				LinesRemoved: v.LinesRemoved,
			},
		})
	}

	reportData := &report.ReportData{
		SpecName:       o.config.BranchName,
		GeneratedAt:    time.Now(),
		Variants:       reportVariants,
		Comparison:     o.comparisonResult,
		BaseCommit:     metadata.BaseCommit,
		OriginalBranch: metadata.OriginalBranch,
		VariantCommits: o.variantManager.GetVariantCommits(),
	}

	// Create report in comparison-report directory under spec
	reportDir := filepath.Join(o.config.SpecDir, "comparison-report")
	generator := report.NewGenerator(reportDir)

	if err := generator.Generate(reportData); err != nil {
		return fmt.Errorf("generate report: %w", err)
	}

	log.Printf("Report generated: %s/index.html", reportDir)
	return nil
}

// generatePartialReport creates a report when all variants failed.
func (o *Orbit) generatePartialReport() error {
	log.Println("Generating partial report (all variants failed)...")

	metadata := o.variantManager.GetMetadata()
	if metadata == nil {
		return fmt.Errorf("no variant metadata available")
	}

	variantList := o.variantManager.GetVariantsSnapshot()

	// Build report data with failure info
	reportVariants := make([]report.VariantReportData, 0, len(variantList))
	for _, v := range variantList {
		// Build cost totals from variant data
		var costTotals cost.Totals
		if v.CostTotals.USD > 0 || v.CostTotals.Credits > 0 || v.CostTotals.PremiumRequests > 0 {
			costTotals = v.CostTotals
		} else {
			// Construct totals from single cost value
			switch v.CostUnit {
			case cost.UnitCredits:
				costTotals.Credits = v.Cost
			case cost.UnitPremiumRequests:
				costTotals.PremiumRequests = v.Cost
			default:
				// Default to USD if unknown or explicitly USD
				costTotals.USD = v.Cost
			}
		}

		reportVariants = append(reportVariants, report.VariantReportData{
			ID:     v.ID,
			Branch: v.Branch,
			Status: string(v.Status),
			Error:  v.Error,
			Agent:  v.Agent,
			Metrics: report.VariantMetrics{
				Cost:         &costTotals,
				Duration:     v.Duration.Round(time.Second).String(),
				NumTurns:     v.NumTurns,
				LinesAdded:   v.LinesAdded,
				LinesRemoved: v.LinesRemoved,
			},
		})
	}

	reportData := &report.ReportData{
		SpecName:       o.config.BranchName,
		GeneratedAt:    time.Now(),
		Variants:       reportVariants,
		Comparison:     nil, // No comparison for all-failed case
		BaseCommit:     metadata.BaseCommit,
		OriginalBranch: metadata.OriginalBranch,
		VariantCommits: o.variantManager.GetVariantCommits(),
	}

	// Create report in comparison-report directory under spec
	reportDir := filepath.Join(o.config.SpecDir, "comparison-report")
	generator := report.NewGenerator(reportDir)

	if err := generator.Generate(reportData); err != nil {
		return fmt.Errorf("generate partial report: %w", err)
	}

	log.Printf("Partial report generated: %s/index.html", reportDir)
	return fmt.Errorf("all variants failed")
}

// readSpecContext reads key spec files to provide context for comparison.
func (o *Orbit) readSpecContext() string {
	var parts []string

	// Key spec files to include
	specFiles := []struct {
		name  string
		label string
	}{
		{"requirements.md", "Requirements"},
		{"design.md", "Design"},
		{"tasks.md", "Tasks"},
	}

	for _, sf := range specFiles {
		path := filepath.Join(o.config.SpecDir, sf.name)
		content, err := os.ReadFile(path)
		if err != nil {
			continue // Skip files that don't exist
		}

		// Truncate if too long
		s := string(content)
		if len(s) > 3000 {
			idx := strings.LastIndex(s[:3000], "\n")
			if idx > 2500 {
				s = s[:idx] + "\n... (truncated)"
			} else {
				s = s[:3000] + "... (truncated)"
			}
		}

		parts = append(parts, fmt.Sprintf("### %s\n\n%s", sf.label, s))
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, "\n\n")
}

