package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/arjenschwarz/orbit/internal/consolidation"
	"github.com/arjenschwarz/orbit/internal/variants"
)

// finalizeCommand executes the orbit finalize subcommand.
// It rebases the chosen variant onto the original branch and cleans up other variants.
func finalizeCommand(args []string) error {
	fs := flag.NewFlagSet("finalize", flag.ExitOnError)

	variantID := fs.Int("variant", 0, "Variant to adopt (required)")
	force := fs.Bool("force", false, "Skip confirmation prompt")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: orbit finalize <spec-name> --variant N [options]\n\n")
		fmt.Fprintf(os.Stderr, "Adopt a variant as the final implementation and clean up others.\n")
		fmt.Fprintf(os.Stderr, "This rebases the chosen variant onto the original branch.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  orbit finalize my-feature --variant 1\n")
		fmt.Fprintf(os.Stderr, "  orbit finalize my-feature --variant 2 --force\n")
	}

	// Reorder args so flags come before positional args (Go's flag package requires this)
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return err
	}

	// Validate required variant flag
	if *variantID <= 0 {
		return fmt.Errorf("--variant is required and must be a positive integer")
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
		return fmt.Errorf("no variant run found for spec: %s", specName)
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

	// Validate variant exists
	var targetVariant *variants.Variant
	for _, v := range metadata.Variants {
		if v.ID == *variantID {
			targetVariant = v
			break
		}
	}
	if targetVariant == nil {
		return fmt.Errorf("variant %d not found", *variantID)
	}

	// Show what will happen
	fmt.Printf("Finalize spec: %s\n\n", specName)
	fmt.Printf("%s\n", formatVariantAgentInfo(targetVariant))
	fmt.Printf("This will:\n")
	fmt.Printf("  1. Rebase variant %d (%s) onto %s\n", targetVariant.ID, targetVariant.Branch, metadata.OriginalBranch)
	fmt.Printf("  2. Remove all variant worktrees\n")
	fmt.Printf("  3. Delete all variant branches\n")
	fmt.Println()

	// Verify against the most recent consolidation log entry, if any.
	// Runs before the force-gate so the warning is visible in --force / CI runs.
	printConsolidationMismatchWarning(filepath.Join(specDir, ".orbit"), *variantID)

	// Confirm unless force flag is set
	if !*force {
		fmt.Print("Proceed with finalize? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		answer, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("Finalize cancelled")
			return nil
		}
	}

	// Perform finalize
	ctx := context.Background()
	if err := mgr.Finalize(ctx, *variantID); err != nil {
		// Check for common errors and provide helpful messages
		if strings.Contains(err.Error(), "diverged") {
			fmt.Printf("\nError: %v\n", err)
			fmt.Println("\nThe original branch has changed since the variant run started.")
			fmt.Println("Options:")
			fmt.Println("  1. Manually merge the changes")
			fmt.Println("  2. Run 'orbit cleanup' and start fresh")
			return fmt.Errorf("finalize aborted due to diverged branch")
		}
		if strings.Contains(err.Error(), "conflict") {
			fmt.Printf("\nError: %v\n", err)
			fmt.Println("\nRebase conflicts occurred. Resolve them manually:")
			fmt.Println("  1. cd to repository root")
			fmt.Println("  2. git rebase --continue (after resolving conflicts)")
			fmt.Println("  3. Run 'orbit cleanup' to remove remaining worktrees")
			return fmt.Errorf("finalize paused due to rebase conflicts")
		}
		return fmt.Errorf("finalize failed: %w", err)
	}

	fmt.Printf("\nFinalize complete! Variant %d has been rebased onto %s.\n", *variantID, metadata.OriginalBranch)
	fmt.Println("All variant worktrees and branches have been cleaned up.")

	return nil
}

// printConsolidationMismatchWarning reads consolidation-log.json under orbitDir
// and prints a warning when the most recent entry's ChosenVariantID differs
// from the requested variantID. Missing log, parse failure, or empty entries
// are treated as "no verification possible" and print nothing.
func printConsolidationMismatchWarning(orbitDir string, variantID int) {
	log, err := consolidation.NewLogger(orbitDir).Read()
	if err != nil {
		return
	}
	if len(log.Entries) == 0 {
		return
	}
	latest := log.Entries[len(log.Entries)-1]
	if latest.ChosenVariantID == variantID {
		return
	}
	fmt.Printf("Warning: variant %d does not match the most recent consolidation (variant %d, %s)\n\n",
		variantID, latest.ChosenVariantID, latest.Timestamp.Format(time.RFC3339))
}

// formatVariantAgentInfo renders an "Agent: ..." line for the finalize preamble.
// When all of Agent, AgentType, and Model are empty, returns "Agent: unknown".
// Otherwise, builds "Agent: <alias> (<type>, model: <model>)" with each parenthetical
// piece omitted cleanly when its source field is empty.
func formatVariantAgentInfo(v *variants.Variant) string {
	if v.Agent == "" && v.AgentType == "" && v.Model == "" {
		return "Agent: unknown"
	}

	var parens []string
	if v.AgentType != "" {
		parens = append(parens, v.AgentType)
	}
	if v.Model != "" {
		parens = append(parens, "model: "+v.Model)
	}

	var parts []string
	if v.Agent != "" {
		parts = append(parts, v.Agent)
	}
	if len(parens) > 0 {
		parts = append(parts, "("+strings.Join(parens, ", ")+")")
	}

	return "Agent: " + strings.Join(parts, " ")
}
