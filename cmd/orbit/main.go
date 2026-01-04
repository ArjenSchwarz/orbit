// Package main provides the CLI entry point for Orbit.
package main

import (
	"log"
	"os"
	"strings"
)

var version = "dev"

// knownSubcommands lists all valid subcommands.
var knownSubcommands = map[string]bool{
	"run":      true,
	"serve":    true,
	"register": true,
	"demo":     true,
}

func main() {
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
		err = RunDemo()
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
