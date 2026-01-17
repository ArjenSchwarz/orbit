package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	output "github.com/ArjenSchwarz/go-output/v2"
	"github.com/arjenschwarz/orbit/internal/variants"
)

// statusCommand executes the orbit status subcommand.
// It displays the status of variant worktrees for a spec.
func statusCommand(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: orbit status <spec-name>\n\n")
		fmt.Fprintf(os.Stderr, "Display the status of variant implementations for a spec.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  orbit status my-feature\n")
		fmt.Fprintf(os.Stderr, "  orbit status                   # Auto-detect from current branch\n")
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
		fmt.Printf("No variant run in progress for spec: %s\n", specName)
		fmt.Printf("Start a variant run with: orbit run --variants N\n")
		return nil
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
		fmt.Printf("No variant run in progress for spec: %s\n", specName)
		return nil
	}

	// Display header
	fmt.Printf("Variant Status: %s\n\n", specName)
	fmt.Printf("Base Commit:     %s\n", metadata.BaseCommit[:12])
	fmt.Printf("Original Branch: %s\n", metadata.OriginalBranch)
	fmt.Printf("Started:         %s\n\n", metadata.StartedAt.Format("2006-01-02 15:04:05"))

	// Build table data
	rows := make([]map[string]any, 0, len(metadata.Variants))
	for _, v := range metadata.Variants {
		rows = append(rows, map[string]any{
			"ID":     v.ID,
			"Branch": v.Branch,
			"Path":   v.WorktreePath,
			"Status": string(v.Status),
		})
	}

	doc := output.New().
		Table("Variants", rows, output.WithKeys("ID", "Branch", "Path", "Status")).
		Build()

	out := output.NewOutput(
		output.WithFormat(output.Table()),
		output.WithWriter(output.NewStdoutWriter()),
	)

	if err := out.Render(context.Background(), doc); err != nil {
		return fmt.Errorf("failed to render table: %w", err)
	}

	return nil
}

// getGitBranchForStatus returns the current git branch name.
func getGitBranchForStatus() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not in a git repository or git not available: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
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
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not in a git repository: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}
