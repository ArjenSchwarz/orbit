// Package main provides the CLI entry point for Orbit.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/arjenschwarz/orbit/internal/orbit"
)

var version = "dev"

func main() {
	// CLI flags
	tasksFile := flag.String("tasks-file", "", "Path to rune tasks file (auto-detects from branch if not specified)")
	logDir := flag.String("log-dir", "", "Base directory for session logs (default: .orbit next to tasks file)")
	skipPermissions := flag.Bool("skip-permissions", true, "Run Claude with --dangerously-skip-permissions")
	verbose := flag.Bool("verbose", false, "Enable verbose output")
	dryRun := flag.Bool("dry-run", false, "Show what would be executed without running")
	showVersion := flag.Bool("version", false, "Show version and exit")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Orbit - Claude Code Task Orchestrator\n\n")
		fmt.Fprintf(os.Stderr, "Usage: orbit [options]\n\n")
		fmt.Fprintf(os.Stderr, "Orbit orchestrates Claude Code sessions to implement spec phases sequentially.\n")
		fmt.Fprintf(os.Stderr, "It handles session lifecycle, error recovery, and log management.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  orbit                                  # Auto-detect tasks from current branch\n")
		fmt.Fprintf(os.Stderr, "  orbit --tasks-file specs/my-feature/tasks.md\n")
		fmt.Fprintf(os.Stderr, "  orbit --verbose --log-dir ./logs\n")
		fmt.Fprintf(os.Stderr, "  orbit --dry-run                        # Preview without executing\n")
	}

	flag.Parse()

	if *showVersion {
		fmt.Printf("orbit version %s\n", version)
		os.Exit(0)
	}

	// Get working directory
	workingDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get working directory: %v", err)
	}

	// Get branch name
	branchName, err := getGitBranch()
	if err != nil {
		log.Fatalf("Failed to get git branch: %v", err)
	}

	// Auto-detect tasks file if not specified
	if *tasksFile == "" {
		detected, err := detectTasksFile(branchName)
		if err != nil {
			log.Fatalf("Failed to auto-detect tasks file: %v\nUse --tasks-file to specify manually", err)
		}
		*tasksFile = detected
		if *verbose {
			log.Printf("Auto-detected tasks file: %s", *tasksFile)
		}
	}

	// Validate tasks file exists
	if _, err := os.Stat(*tasksFile); os.IsNotExist(err) {
		log.Fatalf("Tasks file not found: %s", *tasksFile)
	}

	// Set default log directory to .orbit next to tasks file
	actualLogDir := *logDir
	if actualLogDir == "" {
		actualLogDir = filepath.Join(filepath.Dir(*tasksFile), ".orbit")
	}

	// Create and run orchestrator
	config := orbit.Config{
		TasksFile:       *tasksFile,
		LogDir:          actualLogDir,
		BranchName:      branchName,
		SkipPermissions: *skipPermissions,
		Verbose:         *verbose,
		DryRun:          *dryRun,
		WorkingDir:      workingDir,
	}

	o, err := orbit.New(config)
	if err != nil {
		log.Fatalf("Failed to initialize Orbit: %v", err)
	}

	if err := o.Run(); err != nil {
		log.Fatalf("Orchestration failed: %v", err)
	}

	log.Println("Orbit completed successfully")
}

// getGitBranch returns the current git branch name.
func getGitBranch() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not in a git repository or git not available: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// detectTasksFile attempts to find a tasks file based on the branch name.
func detectTasksFile(branchName string) (string, error) {
	// Strip prefix before first slash (e.g., "feature/my-feature" -> "my-feature")
	name := branchName
	if _, after, found := strings.Cut(branchName, "/"); found {
		name = after
	}

	// Try various paths
	candidates := []string{
		filepath.Join("specs", name, "tasks.md"),
		filepath.Join("specs", name, "TASKS.md"),
		filepath.Join("specs", branchName, "tasks.md"),
		filepath.Join("specs", branchName, "TASKS.md"),
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("could not find tasks file for branch '%s'\nTried: %s", branchName, strings.Join(candidates, ", "))
}
