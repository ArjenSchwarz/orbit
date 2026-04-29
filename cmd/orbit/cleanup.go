package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/arjenschwarz/orbit/internal/variants"
)

// cleanupCommand executes the orbit cleanup subcommand.
// It removes variant worktrees and branches for a spec.
func cleanupCommand(args []string) error {
	fs := flag.NewFlagSet("cleanup", flag.ExitOnError)

	keepVariant := fs.Int("keep", 0, "Preserve variant N, remove others")
	force := fs.Bool("force", false, "Skip confirmation prompt")
	dryRun := fs.Bool("dry-run", false, "Show what would be deleted without deleting")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: orbit cleanup <spec-name> [options]\n\n")
		fmt.Fprintf(os.Stderr, "Remove variant worktrees and branches for a spec.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  orbit cleanup my-feature             # Remove all variants\n")
		fmt.Fprintf(os.Stderr, "  orbit cleanup my-feature --keep 1    # Keep variant 1, remove others\n")
		fmt.Fprintf(os.Stderr, "  orbit cleanup my-feature --dry-run   # Show what would be deleted\n")
		fmt.Fprintf(os.Stderr, "  orbit cleanup my-feature --force     # Skip confirmation\n")
	}

	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
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

	// Get repo root
	repoRoot, err := getRepoRoot()
	if err != nil {
		return fmt.Errorf("failed to get repository root: %w", err)
	}

	// Find and load variants.json
	specDir := filepath.Join(repoRoot, "specs", specName)
	metadataPath := filepath.Join(specDir, ".orbit", "variants.json")

	if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
		fmt.Printf("No variant run to clean up for spec: %s\n", specName)
		return nil
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
		fmt.Printf("No variant run to clean up for spec: %s\n", specName)
		return nil
	}

	// Show what will be deleted
	fmt.Printf("Cleanup for spec: %s\n\n", specName)
	fmt.Println("The following will be removed:")

	for _, v := range metadata.Variants {
		if *keepVariant > 0 && v.ID == *keepVariant {
			fmt.Printf("  [KEEP] Variant %d: %s\n", v.ID, v.Branch)
		} else {
			fmt.Printf("  - Variant %d: %s\n", v.ID, v.Branch)
			fmt.Printf("    Worktree: %s\n", v.WorktreePath)
		}
	}

	if *keepVariant == 0 {
		fmt.Printf("  - variants.json\n")
	}

	fmt.Println()

	// Dry run mode - just show what would be done
	if *dryRun {
		fmt.Println("(dry-run mode - no changes made)")
		return nil
	}

	// Confirm unless force flag is set
	if !*force {
		fmt.Print("Proceed with cleanup? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		answer, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("Cleanup cancelled")
			return nil
		}
	}

	// Perform cleanup
	ctx := context.Background()
	if err := mgr.Cleanup(ctx, *keepVariant); err != nil {
		return fmt.Errorf("cleanup failed: %w", err)
	}

	if *keepVariant > 0 {
		fmt.Printf("Cleanup complete. Variant %d preserved.\n", *keepVariant)
	} else {
		fmt.Println("Cleanup complete. All variants removed.")
	}

	return nil
}
