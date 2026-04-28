package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/arjenschwarz/orbit/internal/agents"
	_ "github.com/arjenschwarz/orbit/internal/agents/claudecode" // Register claude-code agent
	_ "github.com/arjenschwarz/orbit/internal/agents/codex"      // Register codex agent
	_ "github.com/arjenschwarz/orbit/internal/agents/copilot"    // Register copilot agent
	_ "github.com/arjenschwarz/orbit/internal/agents/kiro"       // Register kiro agent
	_ "github.com/arjenschwarz/orbit/internal/agents/opencode"   // Register opencode agent
	"github.com/arjenschwarz/orbit/internal/config"
	"github.com/arjenschwarz/orbit/internal/consolidation"
	"github.com/arjenschwarz/orbit/internal/variants"
)

// consolidateCommand executes the orbit consolidate subcommand.
// It uses an AI agent to analyze improvements from non-chosen variants
// and apply them to the chosen variant.
// Implements: [2.1], [2.2], [2.7], [2.8]
func consolidateCommand(args []string) error {
	fs := flag.NewFlagSet("consolidate", flag.ExitOnError)

	variantID := fs.Int("variant", 0, "Target variant ID (required for consolidation, not needed for --rollback)")
	allowDirty := fs.Bool("allow-dirty", false, "Allow uncommitted changes in the target worktree")
	customPrompt := fs.String("prompt", "", "Custom instructions to influence consolidation decisions")
	rollback := fs.Bool("rollback", false, "Revert the most recent consolidation commit")
	force := fs.Bool("force", false, "Skip confirmation prompt (useful for CI/CD pipelines)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: orbit consolidate <spec-name> --variant N [options]\n\n")
		fmt.Fprintf(os.Stderr, "Consolidate improvements from non-chosen variants into the chosen variant.\n")
		fmt.Fprintf(os.Stderr, "Uses an AI agent to analyze the comparison report and apply improvements.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  orbit consolidate my-feature --variant 1\n")
		fmt.Fprintf(os.Stderr, "  orbit consolidate my-feature --variant 1 --prompt \"Focus on error handling\"\n")
		fmt.Fprintf(os.Stderr, "  orbit consolidate my-feature --rollback    # Revert last consolidation\n")
		fmt.Fprintf(os.Stderr, "  orbit consolidate --variant 1              # Auto-detect spec from branch\n")
	}

	// Reorder args so flags come before positional args (Go's flag package requires this)
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return err
	}

	// Validate flags: --variant is required unless --rollback is used
	if !*rollback && *variantID <= 0 {
		return fmt.Errorf("--variant is required and must be a positive integer (unless using --rollback)")
	}

	// Get spec name from args or auto-detect from branch [Req 2.2]
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

	// Handle rollback mode [Req 5.7]
	if *rollback {
		return handleRollback(specName, specDir, mgr, *variantID, *force)
	}

	// Resolve the agent to use [Req 2.6]
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	appConfig := config.Load(workDir)

	// Require config file for agent resolution
	if err := appConfig.RequireConfigFile(); err != nil {
		return err
	}

	// Resolve and validate all agent aliases
	if err := appConfig.ResolveAliases(); err != nil {
		return err
	}

	// Resolve agent alias: use default agent
	aliasName := resolveAgent("", appConfig)
	resolved, err := appConfig.GetResolvedAgent(aliasName)
	if err != nil {
		return err
	}

	// Get agent factory by type
	agentCfg := buildAgentConfig(resolved)
	agent, err := agents.Get(resolved.Type, agentCfg)
	if err != nil {
		return fmt.Errorf("unknown agent type %q for alias %q\n\nValid agent types: %v", resolved.Type, aliasName, agents.List())
	}
	if !agent.IsInstalled() {
		return fmt.Errorf("agent %q CLI (%s) not found\nInstall it from: %s", aliasName, agent.CLICommand(), getAgentInstallURL(resolved.Type))
	}

	// Create consolidator configuration
	consolidatorCfg := consolidation.Config{
		SpecName:     specName,
		SpecDir:      specDir,
		VariantID:    *variantID,
		Agent:        agent,
		AllowDirty:   *allowDirty, // [Req 2.7]
		PostPrompt:   appConfig.PostPrompt,
		CustomPrompt: *customPrompt, // [Req 2.8]
	}

	consolidator, err := consolidation.NewConsolidator(consolidatorCfg, mgr)
	if err != nil {
		// Check if the error is about variant not found to provide better message
		if strings.Contains(err.Error(), "not found") {
			return fmt.Errorf("variant %d not found. Use 'orbit status %s' to see available variants", *variantID, specName)
		}
		return fmt.Errorf("failed to create consolidator: %w", err)
	}

	// Show what we're about to do
	fmt.Printf("Consolidate spec: %s\n", specName)
	fmt.Printf("Target variant:   %d\n", *variantID)
	fmt.Printf("Agent:            %s\n", aliasName)
	if *customPrompt != "" {
		fmt.Printf("Custom prompt:    %s\n", truncateString(*customPrompt, 50))
	}
	fmt.Println()

	// Confirm unless --force is set or in CI/automation (checking for TTY)
	if !*force && !isAutomatedEnvironment() {
		fmt.Print("Proceed with consolidation? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		answer, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("Consolidation cancelled")
			return nil
		}
	}

	// Run consolidation
	ctx := context.Background()
	result, err := consolidator.Run(ctx)
	if err != nil {
		// Check for specific error types to provide helpful messages
		if err == consolidation.ErrNoImprovements {
			fmt.Println(err.Error())
			return nil // Not an error - just nothing to do
		}
		return fmt.Errorf("consolidation failed: %w", err)
	}

	// Display results
	fmt.Println()
	fmt.Println("=== Consolidation Complete ===")
	fmt.Println()

	if result.CommitSHA != "" {
		fmt.Printf("Commit: %s\n", result.CommitSHA)
	}

	if result.TestsPassed {
		fmt.Println("Tests:  PASSED")
	} else {
		fmt.Println("Tests:  FAILED")
	}

	if result.PostPromptPassed {
		fmt.Println("Post-prompt: PASSED")
	} else if len(result.Errors) > 0 {
		fmt.Println("Post-prompt: FAILED")
	}

	if len(result.Errors) > 0 {
		fmt.Println()
		fmt.Println("Errors:")
		for _, e := range result.Errors {
			fmt.Printf("  - %s\n", e)
		}
	}

	fmt.Println()
	fmt.Printf("To undo: orbit consolidate %s --rollback\n", specName)

	// Display the agent's report
	if result.AgentReport != "" {
		fmt.Println()
		fmt.Println("=== Agent Report ===")
		fmt.Println()
		fmt.Println(result.AgentReport)
	}

	return nil
}

// handleRollback reverts the most recent consolidation commit.
// If variantID is 0, attempts to infer it from the consolidation log.
// Implements: [5.7]
func handleRollback(specName, specDir string, mgr *variants.Manager, variantID int, force bool) error {
	// If variantID is not specified, try to infer from consolidation log
	if variantID == 0 {
		logger := consolidation.NewLogger(filepath.Join(specDir, ".orbit"))
		log, err := logger.Read()
		if err != nil {
			return fmt.Errorf("--variant is required: failed to read consolidation log: %w", err)
		}
		if len(log.Entries) == 0 {
			return fmt.Errorf("--variant is required: no previous consolidation found")
		}
		// Use variant ID from the most recent consolidation
		variantID = log.Entries[len(log.Entries)-1].ChosenVariantID
		fmt.Printf("Using variant %d from consolidation log\n", variantID)
	}

	// Verify the variant exists
	allVariants := mgr.GetVariantsSnapshot()
	if len(allVariants) == 0 {
		return fmt.Errorf("no variants found for spec: %s", specName)
	}

	var foundVariant bool
	for _, v := range allVariants {
		if v.ID == variantID {
			foundVariant = true
			break
		}
	}
	if !foundVariant {
		var ids []string
		for _, v := range allVariants {
			ids = append(ids, fmt.Sprintf("%d", v.ID))
		}
		return fmt.Errorf("variant %d not found; available variants: %s", variantID, strings.Join(ids, ", "))
	}

	// Confirm unless --force is set
	if !force {
		fmt.Printf("This will revert the most recent consolidation commit for variant %d.\n\n", variantID)
		fmt.Print("Proceed with rollback? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		answer, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("Rollback cancelled")
			return nil
		}
	}

	// Create consolidator for rollback
	cfg := consolidation.Config{
		SpecName:  specName,
		SpecDir:   specDir,
		VariantID: variantID,
		Agent:     nil, // Not needed for rollback
	}

	consolidator, err := consolidation.NewConsolidatorForRollback(cfg, mgr)
	if err != nil {
		return fmt.Errorf("failed to create consolidator for rollback: %w", err)
	}

	ctx := context.Background()
	if err := consolidator.Rollback(ctx); err != nil {
		return fmt.Errorf("rollback failed: %w", err)
	}

	fmt.Println("Rollback complete. The consolidation commit has been reverted.")
	return nil
}

// truncateString truncates a string to maxLen characters, adding "..." if truncated.
func truncateString(s string, maxLen int) string {
	if maxLen <= 3 {
		// For very small maxLen, just return what fits without ellipsis
		if maxLen <= 0 {
			return ""
		}
		if len(s) <= maxLen {
			return s
		}
		return s[:maxLen]
	}
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// isAutomatedEnvironment checks if we're running in a non-interactive environment.
func isAutomatedEnvironment() bool {
	// Check common CI environment variables
	ciVars := []string{
		"CI",
		"GITHUB_ACTIONS",
		"GITLAB_CI",
		"JENKINS_URL",
		"BUILDKITE",
		"CIRCLECI",
		"TRAVIS",
	}
	for _, v := range ciVars {
		if os.Getenv(v) != "" {
			return true
		}
	}
	return false
}
