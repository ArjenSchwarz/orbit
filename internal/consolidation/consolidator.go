// Package consolidation provides the consolidation engine for merging improvements
// from multiple implementation variants into a single chosen variant.
package consolidation

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
	"github.com/arjenschwarz/orbit/internal/display"
	"github.com/arjenschwarz/orbit/internal/variants"
	"github.com/google/uuid"
)

// ErrNoImprovements indicates the comparison report has no cross-variant improvements.
var ErrNoImprovements = errors.New("no cross-variant improvements found in comparison report. Nothing to consolidate")

// Consolidator orchestrates the consolidation workflow.
type Consolidator struct {
	config   Config
	manager  *variants.Manager
	recovery *RecoveryManager
	spinner  *display.Spinner
	logger   *Logger
}

// NewConsolidator creates a consolidator for a spec.
// Implements: [2.3], [2.4], [2.5], [2.6]
func NewConsolidator(cfg Config, mgr *variants.Manager) (*Consolidator, error) {
	if cfg.SpecDir == "" {
		return nil, errors.New("spec directory is required")
	}
	if cfg.VariantID < 1 {
		return nil, errors.New("variant ID must be positive")
	}
	if cfg.Agent == nil {
		return nil, errors.New("agent is required")
	}
	if mgr == nil {
		return nil, errors.New("variant manager is required")
	}

	orbitDir := filepath.Join(cfg.SpecDir, ".orbit")
	worktreePath := getWorktreePath(mgr, cfg.VariantID)
	if worktreePath == "" {
		return nil, fmt.Errorf("variant %d not found", cfg.VariantID)
	}

	return &Consolidator{
		config:   cfg,
		manager:  mgr,
		recovery: NewRecoveryManager(worktreePath),
		spinner:  display.NewSpinner(),
		logger:   NewLogger(orbitDir),
	}, nil
}

// NewConsolidatorForRollback creates a consolidator configured only for rollback operations.
// Unlike NewConsolidator, this does not require an agent since rollback only needs git operations.
func NewConsolidatorForRollback(cfg Config, mgr *variants.Manager) (*Consolidator, error) {
	if cfg.SpecDir == "" {
		return nil, errors.New("spec directory is required")
	}
	if cfg.VariantID < 1 {
		return nil, errors.New("variant ID must be positive")
	}
	if mgr == nil {
		return nil, errors.New("variant manager is required")
	}

	orbitDir := filepath.Join(cfg.SpecDir, ".orbit")
	worktreePath := getWorktreePath(mgr, cfg.VariantID)
	if worktreePath == "" {
		return nil, fmt.Errorf("variant %d not found", cfg.VariantID)
	}

	return &Consolidator{
		config:   cfg,
		manager:  mgr,
		recovery: NewRecoveryManager(worktreePath),
		logger:   NewLogger(orbitDir),
		// No spinner or agent needed for rollback
	}, nil
}

// getWorktreePath returns the worktree path for a variant ID, or empty if not found.
func getWorktreePath(mgr *variants.Manager, variantID int) string {
	v := mgr.GetVariant(variantID)
	if v == nil {
		return ""
	}
	return v.WorktreePath
}

// validateVariant checks that the specified variant exists.
// Lists available variants if the variant is not found.
// Implements: [5.1]
func (c *Consolidator) validateVariant() error {
	v := c.manager.GetVariant(c.config.VariantID)
	if v != nil {
		return nil
	}

	// Variant not found - list available variants
	variants := c.manager.GetVariantsSnapshot()
	if len(variants) == 0 {
		return fmt.Errorf("variant %d not found: no variants exist for this spec", c.config.VariantID)
	}

	var ids []string
	for _, v := range variants {
		ids = append(ids, fmt.Sprintf("%d", v.ID))
	}
	return fmt.Errorf("variant %d not found. Available variants: %s", c.config.VariantID, strings.Join(ids, ", "))
}

// validateReport checks that a Markdown comparison report exists.
// Implements: [5.2]
func (c *Consolidator) validateReport() (string, error) {
	reportPath := filepath.Join(c.config.SpecDir, "comparison-report", "report.md")
	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		return "", fmt.Errorf("comparison-report.md not found at %s. Run 'orbit compare %s' first to generate it",
			reportPath, c.config.SpecName)
	} else if err != nil {
		return "", fmt.Errorf("failed to check report: %w", err)
	}
	return reportPath, nil
}

// checkStaleness compares report metadata against current variant HEADs.
// Returns a warning message if any variant's current commit differs from
// the commit SHA recorded when the comparison report was generated.
// Implements: [2.9]
func (c *Consolidator) checkStaleness(ctx context.Context) (warning string, err error) {
	reportPath := filepath.Join(c.config.SpecDir, "comparison-report", "report.md")

	// Parse YAML frontmatter from comparison-report.md to get variant_commits map
	reportCommits, err := parseReportVariantCommits(reportPath)
	if err != nil {
		// If we can't parse, return no warning (best effort)
		return "", nil
	}

	if len(reportCommits) == 0 {
		// No commit info in report - can't check staleness
		return "", nil
	}

	// Get current variant commits
	currentCommits := c.manager.GetVariantCommits()

	// Compare current HEAD with recorded commit SHA
	var staleVariants []int
	for id, recordedCommit := range reportCommits {
		currentCommit, ok := currentCommits[id]
		if !ok {
			continue // Variant might not exist anymore
		}
		if currentCommit != recordedCommit {
			staleVariants = append(staleVariants, id)
		}
	}

	if len(staleVariants) == 0 {
		return "", nil
	}

	if len(staleVariants) == 1 {
		return fmt.Sprintf("Warning: Comparison report may be stale. Variant %d has new commits since report generation.", staleVariants[0]), nil
	}
	var ids []string
	for _, id := range staleVariants {
		ids = append(ids, fmt.Sprintf("%d", id))
	}
	return fmt.Sprintf("Warning: Comparison report may be stale. Variants %s have new commits since report generation.", strings.Join(ids, ", ")), nil
}

// parseReportVariantCommits extracts variant commit SHAs from report YAML frontmatter.
func parseReportVariantCommits(reportPath string) (map[int]string, error) {
	file, err := os.Open(reportPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	commits := make(map[int]string)
	scanner := bufio.NewScanner(file)

	// Check for YAML frontmatter (starts with ---)
	if !scanner.Scan() || scanner.Text() != "---" {
		return commits, nil // No frontmatter
	}

	inVariantCommits := false
	for scanner.Scan() {
		line := scanner.Text()

		// End of frontmatter
		if line == "---" {
			break
		}

		// Start of variant_commits section
		if strings.HasPrefix(line, "variant_commits:") {
			inVariantCommits = true
			continue
		}

		// Parse variant commits (format: "  1: abc123" or "  2: def456")
		if inVariantCommits && strings.HasPrefix(line, "  ") {
			line = strings.TrimPrefix(line, "  ")
			// Check if it's a new top-level key (no longer in variant_commits)
			if !strings.HasPrefix(line, " ") && strings.Contains(line, ":") && !strings.HasPrefix(line, "-") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					var id int
					if _, err := fmt.Sscanf(parts[0], "%d", &id); err == nil {
						commits[id] = strings.TrimSpace(parts[1])
					} else {
						// It's a new top-level key, exit variant_commits
						inVariantCommits = false
					}
				}
			}
		} else if inVariantCommits && !strings.HasPrefix(line, " ") {
			// End of variant_commits section
			inVariantCommits = false
		}
	}

	return commits, scanner.Err()
}

// checkEmptyImprovements validates the comparison report has actionable content.
// Returns ErrNoImprovements if the CrossVariantImprovements section is empty or missing.
func (c *Consolidator) checkEmptyImprovements(ctx context.Context) error {
	reportPath := filepath.Join(c.config.SpecDir, "comparison-report", "report.md")

	content, err := os.ReadFile(reportPath)
	if err != nil {
		return fmt.Errorf("failed to read comparison report: %w", err)
	}

	contentStr := string(content)

	// Look for the "Improvements from Other Variants" section
	// This section contains the cross-variant improvements
	// The heading is h1 (# ) in the go-output generated report
	sectionHeader := "# Improvements from Other Variants"
	idx := strings.Index(contentStr, sectionHeader)
	if idx == -1 {
		return ErrNoImprovements
	}

	// Get content after the header
	afterHeader := contentStr[idx+len(sectionHeader):]

	// Find the next section (# heading at start of line)
	nextSection := strings.Index(afterHeader[1:], "\n# ")
	if nextSection != -1 {
		afterHeader = afterHeader[:nextSection+1]
	}

	// Check for actual improvements (### From Variant N headings)
	if !strings.Contains(afterHeader, "### From Variant") {
		return ErrNoImprovements
	}

	return nil
}

// checkCleanState validates the worktree has no uncommitted changes.
// Returns an error if dirty and --allow-dirty is not set.
// Implements: [2.7]
func (c *Consolidator) checkCleanState(ctx context.Context) error {
	if c.config.AllowDirty {
		return nil
	}

	worktreePath := c.recovery.worktreePath
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = worktreePath
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to check git status: %w", err)
	}

	if len(strings.TrimSpace(string(out))) > 0 {
		return fmt.Errorf("worktree has uncommitted changes. Commit or stash changes, or use --allow-dirty to proceed anyway")
	}

	return nil
}

// Run executes the consolidation workflow in a single agent session.
// The agent autonomously analyzes, implements, commits, and reports.
// Implements: [3.1]-[3.3], [4.1]-[4.6], [5.1]-[5.8], [7.1]
func (c *Consolidator) Run(ctx context.Context) (*ConsolidationResult, error) {
	// Set up graceful shutdown handling
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	// Channel to coordinate shutdown
	shutdownDone := make(chan struct{})
	go func() {
		select {
		case <-sigChan:
			c.stopSpinner()
			fmt.Fprintln(os.Stderr, "\nInterrupted. Restoring state...")
			if c.recovery != nil {
				if err := c.recovery.RestoreOnFailure(context.Background()); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to restore worktree: %v\n", err)
				}
				if c.recovery.HasStash() {
					if warning, err := c.recovery.RestoreStash(context.Background()); err != nil {
						fmt.Fprintf(os.Stderr, "Warning: failed to restore stash: %v\n", err)
					} else if warning != "" {
						fmt.Fprintln(os.Stderr, warning)
					}
				}
			}
			cancel()
			close(shutdownDone)
		case <-ctx.Done():
			close(shutdownDone)
		}
	}()

	// Validation phase
	c.updateSpinnerMessage("Validating prerequisites...")

	if err := c.validateVariant(); err != nil {
		c.stopSpinner()
		return nil, err
	}

	reportPath, err := c.validateReport()
	if err != nil {
		c.stopSpinner()
		return nil, err
	}

	if err := c.checkCleanState(ctx); err != nil {
		c.stopSpinner()
		return nil, err
	}

	if err := c.checkEmptyImprovements(ctx); err != nil {
		c.stopSpinner()
		return nil, err
	}

	// Check staleness and warn if needed
	if warning, err := c.checkStaleness(ctx); err == nil && warning != "" {
		c.stopSpinner()
		fmt.Fprintln(os.Stderr, warning)
		if c.spinner != nil {
			c.spinner.Start(0)
		}
	}

	// Capture state before agent runs (for all runs)
	if err := c.recovery.CaptureState(ctx); err != nil {
		c.stopSpinner()
		return nil, fmt.Errorf("failed to capture state: %w", err)
	}

	// Create snapshot if --allow-dirty
	if c.config.AllowDirty {
		if err := c.recovery.CreateSnapshot(ctx); err != nil {
			c.stopSpinner()
			return nil, fmt.Errorf("failed to create snapshot: %w", err)
		}
	}

	// Build prompt for consolidation
	worktreePaths := c.getWorktreePaths()
	promptBuilder := NewPromptBuilder(c.config.SpecName, c.config.VariantID, reportPath, worktreePaths, c.config.CustomPrompt)
	prompt := promptBuilder.Build()

	// Run agent
	c.updateSpinnerMessage("Running consolidation agent...")

	result, runErr := c.runWithRetry(ctx, prompt)

	// Handle agent failure
	if runErr != nil {
		c.stopSpinner()

		// Restore on failure if no commit was made
		if !hasCommitInOutput(result) {
			if restoreErr := c.recovery.RestoreOnFailure(ctx); restoreErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to restore worktree: %v\n", restoreErr)
			}
			if c.recovery.HasStash() {
				if warning, err := c.recovery.RestoreStash(ctx); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to restore stash: %v\n", err)
				} else if warning != "" {
					fmt.Fprintln(os.Stderr, warning)
				}
			}
		}
		return nil, runErr
	}

	// Parse agent output for commit SHA and improvement counts
	commitSHA := parseCommitSHA(result.Output)
	report := result.Output

	// Check for SessionExporter interface and call ExportSession for agents like Kiro
	if exporter, ok := c.config.Agent.(agents.SessionExporter); ok {
		sessionFile := filepath.Join(c.config.SpecDir, ".orbit", fmt.Sprintf("consolidation-session-%s.json", result.SessionID))
		if err := exporter.ExportSession(ctx, sessionFile); err != nil {
			// Log but don't fail - session export is best-effort
			fmt.Fprintf(os.Stderr, "Warning: failed to export session: %v\n", err)
		}
	}

	// Run tests
	consolidationResult := &ConsolidationResult{
		CommitSHA:   commitSHA,
		AgentReport: report,
	}

	c.updateSpinnerMessage("Running tests...")

	testsPassed, testErr := c.runTests(ctx)
	consolidationResult.TestsPassed = testsPassed
	if testErr != nil {
		consolidationResult.Errors = append(consolidationResult.Errors, fmt.Sprintf("tests failed: %v", testErr))
	}

	// Run post-command
	if c.config.PostCommand != "" {
		c.updateSpinnerMessage("Running post-command...")
		postPassed, postErr := c.runPostCommand(ctx)
		consolidationResult.PostCommandPassed = postPassed
		if postErr != nil {
			consolidationResult.Errors = append(consolidationResult.Errors, fmt.Sprintf("post-command failed: %v", postErr))
		}
	} else {
		consolidationResult.PostCommandPassed = true
	}

	c.stopSpinner()

	// Log consolidation entry
	if err := c.logConsolidation(consolidationResult, report); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to log consolidation: %v\n", err)
	}

	// Cleanup recovery artifacts on success
	if err := c.recovery.Cleanup(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to cleanup recovery artifacts: %v\n", err)
	}

	return consolidationResult, nil
}

// runWithRetry runs the agent with exponential backoff for retryable errors.
// Uses proper exponential backoff with timing: 1s, 2s, 4s, 8s, 16s.
// Implements: [5.8], [5.9]
func (c *Consolidator) runWithRetry(ctx context.Context, prompt string) (*agents.RunResult, error) {
	const maxRetries = 5
	backoffDurations := []time.Duration{
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
	}

	worktreePath := c.recovery.worktreePath

	// Generate a unique session ID for this consolidation run
	sessionID := uuid.NewString()

	opts := agents.RunOptions{
		Prompt:      prompt,
		SessionID:   sessionID,
		WorkDir:     worktreePath,
		AutoApprove: true,
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		result, err := c.config.Agent.Run(ctx, opts)
		if err == nil && (result == nil || result.Error == nil) {
			return result, nil
		}

		// Classify error for retry decision
		classifier, ok := c.config.Agent.(agents.ErrorClassifier)
		if !ok {
			// Agent doesn't support classification - return the error
			if err != nil {
				return result, err
			}
			if result != nil && result.Error != nil {
				return result, result.Error
			}
			return result, nil
		}

		exitCode := 0
		stderr := ""
		stdout := ""
		var errMsgs []string

		if result != nil {
			exitCode = result.ExitCode
			stderr = result.Stderr
			stdout = result.Output
			errMsgs = result.Errors
		}

		classifiedErr := classifier.Classify(exitCode, stderr, stdout, errMsgs)
		if classifiedErr == nil || classifiedErr.Class != agents.ErrorClassRetryable {
			// Not retryable - return the error
			if err != nil {
				return result, err
			}
			if result != nil && result.Error != nil {
				return result, result.Error
			}
			return result, fmt.Errorf("agent execution failed")
		}

		lastErr = classifiedErr

		// Don't retry on last attempt
		if attempt >= maxRetries {
			break
		}

		// Apply backoff
		backoff := backoffDurations[attempt]
		if classifiedErr.RetryAfter > 0 {
			backoff = classifiedErr.RetryAfter
		}

		c.updateSpinnerMessage(fmt.Sprintf("Retrying in %v (attempt %d/%d)...", backoff, attempt+1, maxRetries))

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// Rollback reverts the most recent consolidation commit.
// 1. First checks consolidation-log.json for stored commit SHA
// 2. Falls back to searching recent commits (git log -n 20) for message pattern
// 3. Validates commit exists and message matches pattern before reverting
// Implements: [5.7]
func (c *Consolidator) Rollback(ctx context.Context) error {
	worktreePath := c.recovery.worktreePath

	// Try to get commit SHA from log first
	commitSHA, err := c.logger.GetLatestCommitSHA()
	if err != nil {
		// Fall back to searching recent commits
		commitSHA, err = c.findConsolidationCommit(ctx, worktreePath)
		if err != nil {
			return fmt.Errorf("no consolidation commit found to rollback: %w", err)
		}
	}

	// Validate commit exists and message matches pattern
	if err := c.validateCommitForRollback(ctx, worktreePath, commitSHA); err != nil {
		return fmt.Errorf("cannot rollback commit %s: %w", truncateSHA(commitSHA), err)
	}

	// Revert the commit
	cmd := exec.CommandContext(ctx, "git", "revert", "--no-edit", commitSHA)
	cmd.Dir = worktreePath
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to revert commit: %s", stderr.String())
	}

	fmt.Printf("Successfully rolled back consolidation commit %s\n", truncateSHA(commitSHA))
	return nil
}

// findConsolidationCommit searches recent commits for the consolidation message pattern.
func (c *Consolidator) findConsolidationCommit(ctx context.Context, worktreePath string) (string, error) {
	// Search recent commits for the message pattern
	cmd := exec.CommandContext(ctx, "git", "log", "-n", "20", "--oneline", "--format=%H %s")
	cmd.Dir = worktreePath
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to read git log: %w", err)
	}

	// Pattern: feat(consolidate): Apply improvements from variants X, Y to variant Z for spec-name
	pattern := regexp.MustCompile(`^feat\(consolidate\): Apply improvements from variants .+ to variant \d+ for .+$`)

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		sha := parts[0]
		msg := parts[1]
		if pattern.MatchString(msg) {
			return sha, nil
		}
	}

	return "", errors.New("no consolidation commit found in recent history")
}

// validateCommitForRollback verifies a commit exists and has the expected message pattern.
func (c *Consolidator) validateCommitForRollback(ctx context.Context, worktreePath, commitSHA string) error {
	// Get commit message
	cmd := exec.CommandContext(ctx, "git", "log", "-1", "--format=%s", commitSHA)
	cmd.Dir = worktreePath
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("commit not found")
	}

	msg := strings.TrimSpace(string(out))
	pattern := regexp.MustCompile(`^feat\(consolidate\): Apply improvements from variants .+ to variant \d+ for .+$`)
	if !pattern.MatchString(msg) {
		return fmt.Errorf("commit message does not match consolidation pattern")
	}

	return nil
}

// getWorktreePaths returns a map of variant ID to worktree path.
func (c *Consolidator) getWorktreePaths() map[int]string {
	paths := make(map[int]string)
	variants := c.manager.GetVariantsSnapshot()
	for _, v := range variants {
		paths[v.ID] = v.WorktreePath
	}
	return paths
}

// runTests runs the project's test suite.
func (c *Consolidator) runTests(ctx context.Context) (bool, error) {
	worktreePath := c.recovery.worktreePath

	// Check if Makefile exists with test target
	makefilePath := filepath.Join(worktreePath, "Makefile")
	if _, err := os.Stat(makefilePath); err == nil {
		// Use make test
		cmd := exec.CommandContext(ctx, "make", "test")
		cmd.Dir = worktreePath
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return false, fmt.Errorf("make test failed: %s", stderr.String())
		}
		return true, nil
	}

	// Check for go.mod
	goModPath := filepath.Join(worktreePath, "go.mod")
	if _, err := os.Stat(goModPath); err == nil {
		cmd := exec.CommandContext(ctx, "go", "test", "./...")
		cmd.Dir = worktreePath
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return false, fmt.Errorf("go test failed: %s", stderr.String())
		}
		return true, nil
	}

	// No test runner found - consider it passed
	return true, nil
}

// runPostCommand executes the configured post-command through the agent.
func (c *Consolidator) runPostCommand(ctx context.Context) (bool, error) {
	if c.config.PostCommand == "" {
		return true, nil
	}

	worktreePath := c.recovery.worktreePath

	opts := agents.RunOptions{
		Prompt:      c.config.PostCommand,
		SessionID:   uuid.NewString(),
		WorkDir:     worktreePath,
		AutoApprove: true,
	}

	result, err := c.config.Agent.Run(ctx, opts)
	if err != nil {
		return false, fmt.Errorf("post-command failed: %w", err)
	}
	if result != nil && result.Error != nil {
		return false, fmt.Errorf("post-command failed: %w", result.Error)
	}
	if result != nil && result.ExitCode != 0 {
		return false, fmt.Errorf("post-command failed with exit code %d: %s", result.ExitCode, result.Stderr)
	}
	return true, nil
}

// logConsolidation records the consolidation in the log file.
func (c *Consolidator) logConsolidation(result *ConsolidationResult, report string) error {
	// Save the report to a timestamped file
	reportFile, err := c.logger.SaveReport(report)
	if err != nil {
		reportFile = "" // Continue without report file
	}

	// Parse improvement counts from report
	applied, skipped := parseImprovementCounts(report)

	entry := LogEntry{
		ChosenVariantID:       c.config.VariantID,
		CommitSHA:             result.CommitSHA,
		Agent:                 c.config.Agent.Name(),
		ReportFile:            reportFile,
		ImprovementsAttempted: applied + skipped,
		ImprovementsApplied:   applied,
		ImprovementsSkipped:   skipped,
		TestsPassed:           result.TestsPassed,
		PostCommandPassed:     result.PostCommandPassed,
		Errors:                result.Errors,
	}

	return c.logger.Append(entry)
}

// stopSpinner safely stops the spinner if it exists.
func (c *Consolidator) stopSpinner() {
	if c.spinner != nil {
		c.spinner.Stop()
	}
}

// updateSpinnerMessage updates or starts the spinner with a new message.
func (c *Consolidator) updateSpinnerMessage(msg string) {
	if c.spinner == nil {
		// No spinner - just print the message
		fmt.Fprintf(os.Stderr, "%s\n", msg)
		return
	}
	c.spinner.Pause()
	fmt.Fprintf(os.Stderr, "\r%s", msg)
	c.spinner.Resume()
}

// parseCommitSHA extracts the commit SHA from agent output.
func parseCommitSHA(output string) string {
	// Look for commit SHA in the report format: ### Commit\n{sha}
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "### Commit" && i+1 < len(lines) {
			sha := strings.TrimSpace(lines[i+1])
			// Remove backticks if present
			sha = strings.Trim(sha, "`")
			// Extract just the SHA if there's more text
			parts := strings.Fields(sha)
			if len(parts) > 0 {
				return parts[0]
			}
		}
	}

	// Fallback: look for any 40-char hex string that looks like a SHA
	shaPattern := regexp.MustCompile(`\b[0-9a-f]{40}\b`)
	matches := shaPattern.FindAllString(output, -1)
	if len(matches) > 0 {
		return matches[len(matches)-1] // Return the last one (most recent)
	}

	// Also try shorter SHAs (7+ chars)
	shortShaPattern := regexp.MustCompile(`\b[0-9a-f]{7,39}\b`)
	shortMatches := shortShaPattern.FindAllString(output, -1)
	if len(shortMatches) > 0 {
		return shortMatches[len(shortMatches)-1]
	}

	return ""
}

// parseImprovementCounts parses the number of applied and skipped improvements from the report.
func parseImprovementCounts(report string) (applied, skipped int) {
	// Count rows in Applied table
	inApplied := false
	inSkipped := false
	lines := strings.Split(report, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "### Applied") {
			inApplied = true
			inSkipped = false
			continue
		}
		if strings.HasPrefix(line, "### Skipped") {
			inApplied = false
			inSkipped = true
			continue
		}
		if strings.HasPrefix(line, "### ") {
			inApplied = false
			inSkipped = false
			continue
		}

		// Count table rows (start with |, but not header separator)
		if strings.HasPrefix(line, "|") && !strings.Contains(line, "---") {
			// Skip header row
			if strings.Contains(line, "Source") || strings.Contains(line, "Files") || strings.Contains(line, "Description") || strings.Contains(line, "Reason") {
				continue
			}
			if inApplied {
				applied++
			}
			if inSkipped {
				skipped++
			}
		}
	}

	return applied, skipped
}

// hasCommitInOutput checks if the agent output contains evidence of a commit being made.
func hasCommitInOutput(result *agents.RunResult) bool {
	if result == nil {
		return false
	}
	// Check for commit SHA in output
	return parseCommitSHA(result.Output) != ""
}

// truncateSHA returns the first 8 characters of a SHA, or the full SHA if shorter.
func truncateSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}
