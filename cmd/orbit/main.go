// Package main provides the CLI entry point for Orbit.
package main

import (
	"fmt"
	"log"
	"os"
	"strings"
)

var (
	version   = "dev"
	buildTime = "unknown"
	gitCommit = "unknown"
)

// knownSubcommands lists all valid subcommands.
var knownSubcommands = map[string]bool{
	"run":         true,
	"serve":       true,
	"register":    true,
	"demo":        true,
	"status":      true,
	"cleanup":     true,
	"finalize":    true,
	"compare":     true,
	"consolidate": true,
	"init":        true,
}

func main() {
	// Handle top-level --help and --version before subcommand parsing
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--help", "-h", "help":
			printUsage()
			return
		case "--version", "-v":
			fmt.Printf("orbit %s (commit: %s, built: %s)\n", version, gitCommit, buildTime)
			return
		}
	}

	// Parse subcommand from os.Args
	cmd, cmdArgs := parseSubcommand(os.Args[1:])

	var err error
	switch cmd {
	case "run":
		err = runCommand(cmdArgs)
	case "serve":
		err = serveCommand(cmdArgs)
	case "register":
		err = registerCommand(cmdArgs)
	case "demo":
		err = demoCommand(cmdArgs)
	case "status":
		err = statusCommand(cmdArgs)
	case "cleanup":
		err = cleanupCommand(cmdArgs)
	case "finalize":
		err = finalizeCommand(cmdArgs)
	case "compare":
		err = compareCommand(cmdArgs)
	case "consolidate":
		err = consolidateCommand(cmdArgs)
	case "init":
		err = initCommand(cmdArgs)
	default:
		// This shouldn't happen since parseSubcommand defaults to "run"
		err = runCommand(cmdArgs)
	}

	if err != nil {
		log.Fatalf("Error: %v", err)
	}
}

// parseSubcommand determines which subcommand to run based on arguments.
// Returns the subcommand name and the remaining arguments.
//
// Routing logic:
//   - If no arguments: default to "run" with empty args
//   - If first arg starts with "-": default to "run" with all args (backward compat)
//   - If first arg is a known subcommand: use it and pass remaining args
//   - Otherwise: default to "run" with all args (backward compat)
func parseSubcommand(args []string) (string, []string) {
	if len(args) == 0 {
		return "run", []string{}
	}

	first := args[0]

	// If first argument starts with a flag, default to run
	if strings.HasPrefix(first, "-") {
		return "run", args
	}

	// If first argument is a known subcommand, use it
	if isKnownSubcommand(first) {
		return first, args[1:]
	}

	// Unknown first argument, default to run for backward compatibility
	return "run", args
}

// isKnownSubcommand returns true if the argument is a valid subcommand.
func isKnownSubcommand(arg string) bool {
	return knownSubcommands[arg]
}

// reorderArgs moves flags before positional arguments.
// Go's flag package stops parsing at the first non-flag argument,
// so we need to reorder to support "cmd arg --flag value" syntax.
func reorderArgs(args []string) []string {
	var flags []string
	var positional []string

	i := 0
	for i < len(args) {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			// Check if this flag has a value (not a boolean flag)
			// Heuristic: if next arg exists and doesn't start with -, it's a value
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				// Check if it's a flag=value format
				if !strings.Contains(arg, "=") {
					i++
					flags = append(flags, args[i])
				}
			}
		} else {
			positional = append(positional, arg)
		}
		i++
	}

	return append(flags, positional...)
}

// printUsage displays the top-level help message.
func printUsage() {
	fmt.Fprintf(os.Stderr, `Orbit - AI coding agent orchestrator

Usage: orbit <command> [options]

Commands:
  init         Create a default .orbit.yaml configuration file
  run          Orchestrate agent sessions to implement spec phases (default)
  compare      Regenerate comparison report for existing variants
  consolidate  Merge improvements from non-chosen variants into chosen variant
  status       Show variant status for a spec
  finalize     Adopt a variant and clean up others
  cleanup      Remove all variant worktrees and branches
  serve        Start web interface for viewing runs
  register     Manually register a run in the registry
  demo         Run demo (status, spinner)

Global Options:
  --help, -h       Show this help message
  --version, -v    Show version

Run 'orbit <command> --help' for more information on a command.

Examples:
  orbit run                              # Auto-detect tasks from current branch
  orbit run --tasks-file specs/foo/tasks.md
  orbit run --variants 3 --parallel      # Run 3 implementation variants
  orbit compare my-feature               # Regenerate comparison report
  orbit consolidate my-feature --variant 1  # Consolidate improvements into variant 1
  orbit serve                            # Start web interface on :8080
`)
}
