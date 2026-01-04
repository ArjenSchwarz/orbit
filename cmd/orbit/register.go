package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/arjenschwarz/orbit/internal/logs"
	"github.com/arjenschwarz/orbit/internal/registry"
)

// phaseSessionPattern matches phase-N-run-M-session.json files (new format).
var phaseSessionPattern = regexp.MustCompile(`^phase-(\d+)-run-(\d+)-session\.json$`)

// legacyPhaseSessionPattern matches phase-N-session.json files (old format without run number).
var legacyPhaseSessionPattern = regexp.MustCompile(`^phase-(\d+)-session\.json$`)

// registerCommand handles the `orbit register` subcommand.
// It registers an existing orbit log directory with the run registry.
func registerCommand(args []string) error {
	fs := flag.NewFlagSet("register", flag.ContinueOnError)
	nameFlag := fs.String("name", "", "Display name for the run")

	if err := fs.Parse(args); err != nil {
		return err
	}

	remaining := fs.Args()
	var path string
	if len(remaining) == 0 {
		// Auto-detect from branch name, similar to run command
		detected, err := detectOrbitDir()
		if err != nil {
			return fmt.Errorf("usage: orbit register [--name NAME] <path>\n\nAuto-detection failed: %w", err)
		}
		path = detected
	} else {
		path = remaining[0]
	}

	// Resolve the path (handle "." specially)
	logDir, err := resolvePath(path)
	if err != nil {
		return err
	}

	// Validate it's a valid orbit log directory
	if !isValidOrbitLogDir(logDir) {
		return fmt.Errorf("no valid orbit logs found in %s", path)
	}

	// Get registry
	regDir, err := getRegistryDir()
	if err != nil {
		return fmt.Errorf("failed to get registry directory: %w", err)
	}

	reg, err := registry.New(regDir)
	if err != nil {
		return fmt.Errorf("failed to create registry: %w", err)
	}

	// Check if an entry already exists for this log directory
	existing, err := reg.FindByLogDir(logDir)
	if err != nil {
		return fmt.Errorf("failed to check existing entries: %w", err)
	}

	// Derive metadata
	status := deriveStatus(logDir)
	phases := derivePhases(logDir)
	startedAt := deriveStartedAt(logDir)
	branch := deriveBranch(logDir)
	repository := deriveRepository(logDir)
	name := deriveName(logDir, *nameFlag)

	var entry *registry.RunEntry
	if existing != nil {
		// Update existing entry, preserving ID
		entry = existing
		entry.Name = name
		entry.Status = status
		entry.Phases = phases
		entry.Branch = branch
		entry.Repository = repository
	} else {
		// Create new entry
		entry = registry.NewRunEntry()
		entry.Name = name
		entry.Repository = repository
		entry.LogDir = logDir
		entry.Status = status
		entry.StartedAt = startedAt
		entry.Branch = branch
		entry.Phases = phases
		// PID is nil for manual registrations (already nil by default)
	}

	if err := reg.Register(entry); err != nil {
		return fmt.Errorf("failed to register run: %w", err)
	}

	action := "Registered"
	if existing != nil {
		action = "Updated"
	}
	fmt.Printf("%s run: %s (%s)\n", action, entry.Name, entry.ID)

	return nil
}

// resolvePath resolves the input path to an absolute log directory path.
// When path is ".", it looks for ".orbit/" in the current directory.
func resolvePath(path string) (string, error) {
	if path == "." {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get current directory: %w", err)
		}

		orbitDir := filepath.Join(cwd, ".orbit")
		info, err := os.Stat(orbitDir)
		if err != nil || !info.IsDir() {
			return "", fmt.Errorf("no .orbit directory found in current directory")
		}
		return orbitDir, nil
	}

	// Convert to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	return absPath, nil
}

// isValidOrbitLogDir checks if a directory contains valid orbit logs.
// A directory is valid if it contains at least one phase session file
// (either new format phase-N-run-M-session.json or legacy phase-N-session.json).
func isValidOrbitLogDir(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if phaseSessionPattern.MatchString(name) || legacyPhaseSessionPattern.MatchString(name) {
			return true
		}
	}

	return false
}

// deriveStatus derives the run status from the log directory.
// Uses summary.json if present, otherwise defaults to "completed".
func deriveStatus(logDir string) registry.RunStatus {
	summary, err := loadSummary(logDir)
	if err != nil {
		// No summary or corrupt - assume completed (historical run)
		return registry.StatusCompleted
	}

	switch summary.Status {
	case "failed":
		return registry.StatusFailed
	case "success":
		return registry.StatusCompleted
	default:
		// For any other status (including "running" for crashed runs),
		// assume completed for historical registration
		return registry.StatusCompleted
	}
}

// derivePhases derives the phases array from session files in the log directory.
func derivePhases(logDir string) []registry.Phase {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return nil
	}

	// Map phase number to max run count seen
	phaseRuns := make(map[int]int)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		// Try new format first: phase-N-run-M-session.json
		matches := phaseSessionPattern.FindStringSubmatch(name)
		if matches != nil {
			var phaseNum, runNum int
			_, _ = fmt.Sscanf(matches[1], "%d", &phaseNum)
			_, _ = fmt.Sscanf(matches[2], "%d", &runNum)

			if runNum > phaseRuns[phaseNum] {
				phaseRuns[phaseNum] = runNum
			}
			continue
		}

		// Try legacy format: phase-N-session.json
		legacyMatches := legacyPhaseSessionPattern.FindStringSubmatch(name)
		if legacyMatches != nil {
			var phaseNum int
			_, _ = fmt.Sscanf(legacyMatches[1], "%d", &phaseNum)

			// Legacy format has implicit run count of 1
			if phaseRuns[phaseNum] == 0 {
				phaseRuns[phaseNum] = 1
			}
		}
	}

	if len(phaseRuns) == 0 {
		return nil
	}

	// Sort phase numbers
	phaseNums := make([]int, 0, len(phaseRuns))
	for num := range phaseRuns {
		phaseNums = append(phaseNums, num)
	}
	sort.Ints(phaseNums)

	// Build phases array
	phases := make([]registry.Phase, 0, len(phaseNums))
	for _, num := range phaseNums {
		phases = append(phases, registry.Phase{
			Number:   num,
			Status:   registry.PhaseStatusCompleted, // Historical phases are completed
			RunCount: phaseRuns[num],
		})
	}

	return phases
}

// deriveStartedAt derives the start time from summary.json or file modification time.
func deriveStartedAt(logDir string) time.Time {
	summary, err := loadSummary(logDir)
	if err == nil && !summary.StartedAt.IsZero() {
		return summary.StartedAt
	}

	// Fall back to earliest file modification time
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return time.Now()
	}

	var earliest time.Time
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !phaseSessionPattern.MatchString(name) && !legacyPhaseSessionPattern.MatchString(name) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if earliest.IsZero() || info.ModTime().Before(earliest) {
			earliest = info.ModTime()
		}
	}

	if earliest.IsZero() {
		return time.Now()
	}
	return earliest
}

// deriveBranch attempts to derive the branch name from summary.json.
func deriveBranch(logDir string) string {
	summary, err := loadSummary(logDir)
	if err == nil && summary.BranchName != "" {
		return summary.BranchName
	}

	// Try to infer from directory name or parent structure
	// For logs in specs/feature-name/.orbit, use "feature-name"
	parent := filepath.Dir(logDir)
	if filepath.Base(logDir) == ".orbit" {
		return filepath.Base(parent)
	}

	return filepath.Base(logDir)
}

// deriveRepository gets the repository identifier.
func deriveRepository(logDir string) string {
	// Navigate up to find the git root
	dir := logDir
	for i := 0; i < 5; i++ { // Limit depth
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return registry.GetRepository(dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Fall back to directory-based repository name
	return registry.GetRepository(filepath.Dir(logDir))
}

// deriveName determines the display name for the run.
func deriveName(logDir string, customName string) string {
	if customName != "" {
		return customName
	}

	// Use parent directory name if log directory is .orbit
	if filepath.Base(logDir) == ".orbit" {
		parent := filepath.Dir(logDir)
		return filepath.Base(parent)
	}

	return filepath.Base(logDir)
}

// loadSummary loads and parses the summary.json file from a log directory.
func loadSummary(logDir string) (*logs.Summary, error) {
	path := filepath.Join(logDir, "summary.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var summary logs.Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, err
	}

	return &summary, nil
}

// getRegistryDir returns the path to the registry directory.
func getRegistryDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".orbit", "runs"), nil
}

// detectOrbitDir attempts to find an .orbit directory based on the branch name.
// This mirrors the logic in detectTasksFile() from run.go.
func detectOrbitDir() (string, error) {
	branchName, err := getGitBranch()
	if err != nil {
		return "", fmt.Errorf("not in a git repository: %w", err)
	}

	// Strip prefix before first slash (e.g., "specs/my-feature" -> "my-feature")
	name := branchName
	if _, after, found := strings.Cut(branchName, "/"); found {
		name = after
	}

	// Try various paths for .orbit directories
	candidates := []string{
		filepath.Join("specs", name, ".orbit"),
		filepath.Join("specs", branchName, ".orbit"),
	}

	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("could not find .orbit directory for branch '%s'\nTried: %s", branchName, strings.Join(candidates, ", "))
}
