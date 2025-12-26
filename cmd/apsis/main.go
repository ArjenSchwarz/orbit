// Package main provides the apsis CLI tool for converting Claude Code session transcripts
// from JSONL format to readable Markdown.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/arjenschwarz/orbit/internal/claude"
	"github.com/arjenschwarz/orbit/internal/transcript"
)

// version is set at build time via -ldflags
var version = "dev"

// Config holds CLI configuration.
type Config struct {
	List    bool   // -l, --list
	Output  string // -o, --output
	Project string // -p, --project
	Version bool   // -v, --version
	Help    bool   // -h, --help
	Input   string // positional argument (session ID or file path)
}

// SessionInfo contains metadata about a session file.
type SessionInfo struct {
	ID        string
	CreatedAt time.Time
	Size      int64
}

func main() {
	cfg := parseFlags()
	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func parseFlags() *Config {
	cfg := &Config{}

	flag.BoolVar(&cfg.List, "l", false, "List available sessions")
	flag.BoolVar(&cfg.List, "list", false, "List available sessions")
	flag.StringVar(&cfg.Output, "o", "", "Output file (default: stdout)")
	flag.StringVar(&cfg.Output, "output", "", "Output file (default: stdout)")
	flag.StringVar(&cfg.Project, "p", "", "Project directory (default: current directory)")
	flag.StringVar(&cfg.Project, "project", "", "Project directory (default: current directory)")
	flag.BoolVar(&cfg.Version, "v", false, "Show version")
	flag.BoolVar(&cfg.Version, "version", false, "Show version")
	flag.BoolVar(&cfg.Help, "h", false, "Show help")
	// Note: -help is automatically added by the flag package

	flag.Usage = printUsage
	flag.Parse()

	// Get positional argument
	if flag.NArg() > 0 {
		cfg.Input = flag.Arg(0)
	}

	return cfg
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `apsis - Convert Claude Code session transcripts to Markdown

Usage:
  apsis [options] [session-id | file-path]
  cat session.jsonl | apsis

Options:
  -l, --list              List available sessions for the project
  -o, --output <file>     Write output to file (default: stdout)
  -p, --project <path>    Project directory (default: current directory)
  -v, --version           Show version
  -h, --help              Show this help

Examples:
  apsis 550e8400-e29b-41d4-a716-446655440000     Convert session by ID
  apsis -p /path/to/project session-id           Convert session from different project
  apsis /path/to/session.jsonl                   Convert from file path
  cat session.jsonl | apsis                      Convert from stdin
  apsis -o transcript.md session-id              Save to file
  apsis --list                                   List sessions for current project
  apsis --list -p /path/to/project               List sessions for different project
`)
}

func run(cfg *Config) error {
	// Handle --version
	if cfg.Version {
		fmt.Printf("apsis version %s\n", version)
		return nil
	}

	// Handle --help
	if cfg.Help {
		printUsage()
		return nil
	}

	// Resolve project path
	projectPath := cfg.Project
	if projectPath == "" {
		var err error
		projectPath, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	// Make project path absolute
	absProjectPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("failed to resolve project path: %w", err)
	}

	// Handle --list
	if cfg.List {
		// Validate that no positional argument was provided with --list
		if cfg.Input != "" {
			return fmt.Errorf("cannot specify both --list and a positional argument")
		}
		return listSessions(absProjectPath)
	}

	// Check for input source
	if cfg.Input == "" && !isInputFromPipe() {
		// TTY with no args - show help and exit with error
		printUsage()
		return fmt.Errorf("no input specified")
	}

	// Resolve input source
	input, sessionID, err := resolveInput(cfg.Input, absProjectPath)
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()

	// Determine output destination
	var output io.Writer = os.Stdout
	if cfg.Output != "" {
		f, err := os.Create(cfg.Output)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer func() { _ = f.Close() }()
		output = f
	}

	return convert(input, output, sessionID)
}

// isFilePath returns true if the argument appears to be a file path rather than a session ID.
func isFilePath(arg string) bool {
	// Contains path separator
	if strings.Contains(arg, "/") || strings.Contains(arg, "\\") {
		return true
	}

	// Ends with .jsonl extension
	if strings.HasSuffix(arg, ".jsonl") {
		return true
	}

	// File exists at that path
	if _, err := os.Stat(arg); err == nil {
		return true
	}

	return false
}

// isInputFromPipe returns true if stdin is receiving piped input.
func isInputFromPipe() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) == 0
}

// resolveInput determines the input source and returns a reader and session ID.
func resolveInput(arg string, projectPath string) (io.ReadCloser, string, error) {
	// If no argument, read from stdin
	if arg == "" {
		return os.Stdin, "", nil
	}

	// Check if it's a file path
	if isFilePath(arg) {
		f, err := os.Open(arg)
		if err != nil {
			return nil, "", fmt.Errorf("failed to open file: %w", err)
		}
		// Extract session ID from filename if it's a .jsonl file
		sessionID := ""
		if strings.HasSuffix(arg, ".jsonl") {
			sessionID = strings.TrimSuffix(filepath.Base(arg), ".jsonl")
		}
		return f, sessionID, nil
	}

	// Treat as session ID - look up in Claude projects directory
	claudeProjectPath := claude.BuildProjectPath(projectPath)
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get home directory: %w", err)
	}

	sessionFile := filepath.Join(homeDir, ".claude", "projects", claudeProjectPath, arg+".jsonl")
	f, err := os.Open(sessionFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", fmt.Errorf("session not found: %s\nExpected file: %s", arg, sessionFile)
		}
		return nil, "", fmt.Errorf("failed to open session file: %w", err)
	}

	return f, arg, nil
}

// listSessions lists all sessions for a project.
func listSessions(projectPath string) error {
	claudeProjectPath := claude.BuildProjectPath(projectPath)
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	projectDir := filepath.Join(homeDir, ".claude", "projects", claudeProjectPath)

	// Check if directory exists
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		return fmt.Errorf("project directory not found: %s", projectDir)
	}

	// Read .jsonl files
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return fmt.Errorf("failed to read project directory: %w", err)
	}

	var sessions []SessionInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}

		sessionID := strings.TrimSuffix(entry.Name(), ".jsonl")
		filePath := filepath.Join(projectDir, entry.Name())

		// Get file info for size
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Try to get creation time from first entry
		var createdAt time.Time
		f, err := os.Open(filePath)
		if err == nil {
			createdAt, err = transcript.ParseFirstTimestamp(f)
			_ = f.Close()
			if err != nil {
				// Fall back to modification time
				createdAt = info.ModTime()
			}
		} else {
			createdAt = info.ModTime()
		}

		sessions = append(sessions, SessionInfo{
			ID:        sessionID,
			CreatedAt: createdAt,
			Size:      info.Size(),
		})
	}

	if len(sessions) == 0 {
		fmt.Printf("No sessions found for project: %s\n", projectPath)
		return nil
	}

	// Sort by creation date (oldest first, newest last)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].CreatedAt.Before(sessions[j].CreatedAt)
	})

	// Print sessions
	for _, s := range sessions {
		fmt.Printf("%s\t%s\t%s\n", s.ID, s.CreatedAt.Format(time.RFC3339), formatSize(s.Size))
	}

	return nil
}

// formatSize formats a file size in human-readable format.
func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// convert reads JSONL from input and writes Markdown to output.
func convert(input io.Reader, output io.Writer, sessionID string) error {
	result, err := transcript.ParseJSONL(input)
	if err != nil {
		return fmt.Errorf("failed to parse transcript: %w", err)
	}

	// Write warnings to stderr
	for _, w := range result.Warnings {
		fmt.Fprintf(os.Stderr, "Warning: line %d: %s\n", w.Line, w.Message)
	}

	// Handle empty file
	if len(result.Entries) == 0 {
		fmt.Fprintln(os.Stderr, "Session contains no entries")
		return nil
	}

	opts := transcript.RenderOptions{
		Title:     "Session Transcript",
		SessionID: sessionID,
	}
	markdown := transcript.RenderMarkdown(result.Entries, opts)

	_, err = output.Write([]byte(markdown))
	if err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	return nil
}
