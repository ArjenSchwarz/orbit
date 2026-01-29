package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	output "github.com/ArjenSchwarz/go-output/v2"
	"github.com/arjenschwarz/orbit/internal/status"
	"github.com/arjenschwarz/orbit/internal/variants"
)

// statusCommand executes the orbit status subcommand.
// It displays the status of variant worktrees for a spec.
func statusCommand(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	formatFlag := fs.String("format", "text", "Output format: text or json")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: orbit status [options] <spec-name>\n\n")
		fmt.Fprintf(os.Stderr, "Display the status of variant implementations for a spec.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  orbit status my-feature\n")
		fmt.Fprintf(os.Stderr, "  orbit status                   # Auto-detect from current branch\n")
		fmt.Fprintf(os.Stderr, "  orbit status --format json     # Output as JSON\n")
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
		fmt.Fprintf(os.Stderr, "No variant run in progress for spec: %s\n", specName)
		fmt.Fprintf(os.Stderr, "Start a variant run with: orbit run --variants N\n")
		return fmt.Errorf("variants.json not found")
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
		fmt.Fprintf(os.Stderr, "No variant run in progress for spec: %s\n", specName)
		return fmt.Errorf("variants metadata not found")
	}

	// Gather status information for all variants
	gatherer := status.NewGatherer(git, specName, specDir, metadata.BaseCommit, repoRoot)
	ctx := context.Background()
	infos := gatherer.GatherAllVariants(ctx, metadata.Variants)

	// Build structured output
	startedAt := metadata.StartedAt.Format("2006-01-02 15:04:05")
	statusData := status.BuildStatusOutput(specName, metadata.BaseCommit, metadata.OriginalBranch, startedAt, infos)

	// Render based on format
	return renderStatus(ctx, statusData, *formatFlag)
}

// renderStatus outputs the status data in the requested format.
func renderStatus(ctx context.Context, data *status.StatusOutput, format string) error {
	switch format {
	case "json":
		return renderJSON(data)
	default:
		return renderTerminal(ctx, data)
	}
}

// renderJSON outputs structured JSON.
func renderJSON(data *status.StatusOutput) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

// renderTerminal outputs formatted text for terminal display.
func renderTerminal(ctx context.Context, data *status.StatusOutput) error {
	out := output.NewOutput(
		output.WithFormat(output.Table()),
		output.WithWriter(output.NewStdoutWriter()),
	)

	// Header table
	headerRows := []map[string]any{
		{"Field": "Spec", "Value": data.SpecName},
		{"Field": "Base Commit", "Value": data.BaseCommit},
		{"Field": "Original Branch", "Value": data.OriginalBranch},
		{"Field": "Started", "Value": data.StartedAt},
	}
	headerDoc := output.New().
		Table("Variant Status", headerRows, output.WithKeys("Field", "Value")).
		Build()
	if err := out.Render(ctx, headerDoc); err != nil {
		return err
	}

	// Active variants with details
	for _, v := range data.ActiveVariants {
		fmt.Println()
		if err := renderVariantDetails(ctx, out, &v); err != nil {
			return err
		}
	}

	// Other variants summary
	if len(data.OtherVariants) > 0 {
		fmt.Println()
		sectionTitle := "Other Variants"
		if len(data.ActiveVariants) == 0 {
			sectionTitle = "Variants (No Active)"
		}
		otherRows := make([]map[string]any, 0, len(data.OtherVariants))
		for _, v := range data.OtherVariants {
			otherRows = append(otherRows, map[string]any{
				"Variant": v.ID,
				"Branch":  v.Branch,
				"Status":  v.Status,
			})
		}
		otherDoc := output.New().
			Table(sectionTitle, otherRows, output.WithKeys("Variant", "Branch", "Status")).
			Build()
		if err := out.Render(ctx, otherDoc); err != nil {
			return err
		}
	}

	return nil
}

// renderVariantDetails renders a single active variant's details.
func renderVariantDetails(ctx context.Context, out *output.Output, v *status.VariantOutput) error {
	header := buildVariantHeader(v)
	fmt.Println(header)
	fmt.Println(strings.Repeat("-", len(header)))

	// Worktree path
	if v.Worktree != "" {
		fmt.Printf("Directory: %s\n", v.Worktree)
	}

	// Commits
	fmt.Print("Commits: ")
	if v.GitState == "" && len(v.Commits) == 0 {
		fmt.Println("Git info unavailable")
	} else if len(v.Commits) == 0 {
		fmt.Println("No commits yet")
	} else {
		fmt.Println()
		for _, c := range v.Commits {
			fmt.Printf("  %s %s\n", c.Hash, c.Subject)
		}
	}

	// Last Action
	fmt.Print("Last Action: ")
	if v.LastAction == "" {
		fmt.Println("Waiting for activity...")
	} else {
		fmt.Println(v.LastAction)
	}

	// Tasks table
	if len(v.Tasks) == 0 {
		fmt.Println("Tasks: Task progress unavailable")
	} else {
		taskRows := make([]map[string]any, 0, len(v.Tasks))
		for _, t := range v.Tasks {
			active := ""
			if t.IsActive {
				active = "→"
			}
			taskRows = append(taskRows, map[string]any{
				"":        active,
				"Phase":   t.Phase,
				"Done":    t.Completed,
				"Total":   t.Total,
				"Pending": t.Total - t.Completed,
			})
		}
		taskDoc := output.New().
			Table("Tasks", taskRows, output.WithKeys("", "Phase", "Done", "Total", "Pending")).
			Build()
		if err := out.Render(ctx, taskDoc); err != nil {
			return err
		}
	}
	return nil
}

// buildVariantHeader creates the header string for an active variant section.
func buildVariantHeader(v *status.VariantOutput) string {
	header := fmt.Sprintf("Variant %d: %s [%s", v.ID, v.Branch, v.Status)
	if v.GitState != "" {
		header += fmt.Sprintf(" (%s)", v.GitState)
	}
	header += "]"
	return header
}

// getGitBranchForStatus returns the current git branch name.
func getGitBranchForStatus() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not in a git repository or git not available: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// extractSpecName extracts the spec name from a branch name.
// e.g., "feature/my-feature" -> "my-feature"
func extractSpecName(branch string) string {
	if _, after, found := strings.Cut(branch, "/"); found {
		return after
	}
	return branch
}

// getRepoRoot returns the git repository root directory.
func getRepoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not in a git repository: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
