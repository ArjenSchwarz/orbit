// Package main provides the apsis CLI tool for converting Claude Code session transcripts
// from JSONL format to readable Markdown.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents/claudecode"
	"github.com/arjenschwarz/orbit/internal/agents/kiro/logs"
	"github.com/arjenschwarz/orbit/internal/transcript"
)

// version is set at build time via -ldflags
var version = "dev"

// Config holds CLI configuration.
type Config struct {
	List    bool   // -l, --list
	Output  string // -o, --output
	Project string // -p, --project
	Format  string // -f, --format
	Agent   string // -a, --agent (force agent format)
	Follow  bool   // -F, --follow
	Version bool   // -v, --version
	Help    bool   // -h, --help
	Input   string // positional argument (session ID or file path)
}

// SessionInfo contains metadata about a session file.
type SessionInfo struct {
	ID        string
	CreatedAt time.Time
	Size      int64
	Source    string // "claude" or "codex"
}

func main() {
	cfg := parseFlags()
	exitCode, err := run(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if exitCode != 0 {
		os.Exit(exitCode)
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
	flag.StringVar(&cfg.Format, "f", "md", "Output format: md, markdown, html, json (default: md)")
	flag.StringVar(&cfg.Format, "format", "md", "Output format: md, markdown, html, json (default: md)")
	flag.StringVar(&cfg.Agent, "a", "", "Force agent format (claude-code, codex, kiro, copilot)")
	flag.StringVar(&cfg.Agent, "agent", "", "Force agent format (claude-code, codex, kiro, copilot)")
	flag.BoolVar(&cfg.Follow, "F", false, "Follow mode: monitor file for new entries")
	flag.BoolVar(&cfg.Follow, "follow", false, "Follow mode: monitor file for new entries")
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
	fmt.Fprintf(os.Stderr, `apsis - Convert AI agent session transcripts to Markdown or HTML

Usage:
  apsis [options] [session-id | file-path]
  cat session.jsonl | apsis

Options:
  -l, --list              List available sessions for the project
  -o, --output <file>     Write output to file (default: stdout)
  -p, --project <path>    Project directory (default: current directory)
  -f, --format <format>   Output format: md, markdown, html, json (default: md)
                          json outputs the raw session data as pretty-printed JSON
  -a, --agent <name>      Force agent format: claude-code, codex, kiro, copilot
                          (default: auto-detect from content)
  -F, --follow            Follow mode: continuously monitor file for new entries
                          (like tail -f, markdown output to stdout only)
  -v, --version           Show version
  -h, --help              Show this help

Examples:
  apsis 550e8400-e29b-41d4-a716-446655440000     Convert session by ID
  apsis -p /path/to/project session-id           Convert session from different project
  apsis /path/to/session.jsonl                   Convert from file path
  cat session.jsonl | apsis                      Convert from stdin
  apsis -o transcript.md session-id              Save to file
  apsis -f html -o transcript.html session-id    Save as HTML
  apsis -f json session-id                       Output raw JSON (pretty-printed)
  apsis -a kiro session.json                     Force Kiro format parsing
  apsis --list                                   List sessions for current project
  apsis --list -p /path/to/project               List sessions for different project
  apsis -F session-id                            Follow session in real-time
  apsis --follow /path/to/session.jsonl          Follow file for new entries
`)
}

func run(cfg *Config) (int, error) {
	// Handle --version
	if cfg.Version {
		fmt.Printf("apsis version %s\n", version)
		return 0, nil
	}

	// Handle --help
	if cfg.Help {
		printUsage()
		return 0, nil
	}

	// Resolve project path
	projectPath := cfg.Project
	if projectPath == "" {
		var err error
		projectPath, err = os.Getwd()
		if err != nil {
			return 0, fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	// Make project path absolute
	absProjectPath, err := filepath.Abs(projectPath)
	if err != nil {
		return 0, fmt.Errorf("failed to resolve project path: %w", err)
	}

	// Handle --list
	if cfg.List {
		// Validate that no positional argument was provided with --list
		if cfg.Input != "" {
			return 0, fmt.Errorf("cannot specify both --list and a positional argument")
		}
		return 0, listSessions(absProjectPath)
	}

	// Validate follow mode early (requirement 5.2-5.6)
	if err := validateFollowMode(cfg); err != nil {
		return 0, err
	}

	// Check for input source (follow mode doesn't support stdin)
	if cfg.Input == "" && !isInputFromPipe() {
		// TTY with no args - show help and exit with error
		printUsage()
		return 0, fmt.Errorf("no input specified")
	}

	// Handle follow mode
	if cfg.Follow {
		filePath, err := resolveFollowInput(cfg.Input, absProjectPath)
		if err != nil {
			return 0, err
		}
		opts := transcript.RenderOptions{
			Title: "Session Transcript",
		}
		exitCode := runFollow(filePath, opts)
		return exitCode, nil
	}

	// Non-follow mode: resolve input source
	input, sessionID, err := resolveInput(cfg.Input, absProjectPath)
	if err != nil {
		return 0, err
	}
	defer func() { _ = input.Close() }()

	// Determine output destination
	var output io.Writer = os.Stdout
	if cfg.Output != "" {
		f, err := os.Create(cfg.Output)
		if err != nil {
			return 0, fmt.Errorf("failed to create output file: %w", err)
		}
		defer func() { _ = f.Close() }()
		output = f
	}

	return 0, convert(input, output, sessionID, cfg.Format, cfg.Agent)
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
// It checks Claude location first, then Codex, then Kiro.
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

	// Treat as session ID - check Claude location first, then Codex, then Kiro
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get home directory: %w", err)
	}

	// Try Claude location first
	claudeProjectPath := claudecode.BuildProjectPath(projectPath)
	claudeSessionFile := filepath.Join(homeDir, ".claude", "projects", claudeProjectPath, arg+".jsonl")
	if f, err := os.Open(claudeSessionFile); err == nil {
		return f, arg, nil
	}

	// Try Codex location second
	codexPath, err := findCodexSession(homeDir, arg)
	if err != nil {
		return nil, "", fmt.Errorf("failed to search Codex sessions: %w", err)
	}
	if codexPath != "" {
		f, err := os.Open(codexPath)
		if err != nil {
			return nil, "", fmt.Errorf("failed to open Codex session file: %w", err)
		}
		return f, arg, nil
	}

	// Try Kiro location third
	reader, err := resolveKiroSession(arg, projectPath)
	if err == nil {
		return io.NopCloser(reader), arg, nil
	}
	// Only continue searching if session not found or database not available
	if !errors.Is(err, logs.ErrSessionNotFound) && !errors.Is(err, logs.ErrDatabaseNotFound) {
		return nil, "", fmt.Errorf("kiro lookup: %w", err)
	}

	// Not found in any location
	return nil, "", fmt.Errorf("session not found: %s", arg)
}

// listSessions lists all sessions for a project from both Claude and Codex sources.
func listSessions(projectPath string) error {
	sessions, err := listAllSessions(projectPath)
	if err != nil {
		return err
	}

	if len(sessions) == 0 {
		fmt.Println("no sessions found")
		return nil
	}

	// Print sessions with source indicator
	for _, s := range sessions {
		fmt.Printf("[%s]\t%s\t%s\t%s\n", s.Source, s.ID, s.CreatedAt.Format(time.RFC3339), formatSize(s.Size))
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

// uuidPattern matches standard UUID format: 8-4-4-4-12 hex digits (case-insensitive)
var uuidPattern = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// findCodexSession searches ~/.codex/sessions/ for a session by UUID.
// Returns the path to the session file if found, empty string if not found.
// The homeDir parameter allows testing with a mock home directory.
func findCodexSession(homeDir, sessionID string) (string, error) {
	// Validate sessionID is a proper UUID (36 chars with hyphens)
	if len(sessionID) != 36 || !uuidPattern.MatchString(sessionID) {
		return "", nil // Not a valid UUID, skip Codex search
	}

	// Normalize sessionID for case-insensitive matching
	normalizedID := strings.ToLower(sessionID)

	codexDir := filepath.Join(homeDir, ".codex", "sessions")

	// Check directory exists (resolve symlinks first)
	realDir, err := filepath.EvalSymlinks(codexDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // No error, just not found
		}
		return "", err
	}

	var foundPath string
	err = walkDirFollowSymlinks(realDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}

		// Extract UUID from filename and match exactly (case-insensitive)
		filename := filepath.Base(path)
		if match := uuidPattern.FindString(filename); strings.ToLower(match) == normalizedID {
			foundPath = path
			return filepath.SkipAll
		}
		return nil
	})

	return foundPath, err
}

// walkDirFollowSymlinks walks a directory tree, following symlinks with cycle detection.
// Unlike filepath.WalkDir, this resolves symlinks to directories while preventing
// infinite loops from circular symlinks.
func walkDirFollowSymlinks(root string, fn fs.WalkDirFunc) error {
	visited := make(map[string]bool)
	return walkDirFollowSymlinksInternal(root, fn, visited)
}

func walkDirFollowSymlinksInternal(root string, fn fs.WalkDirFunc, visited map[string]bool) error {
	// Resolve to absolute path for cycle detection
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	// Check for cycles
	if visited[absRoot] {
		return nil // Already visited, skip to prevent infinite recursion
	}
	visited[absRoot] = true

	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fn(path, d, err)
		}

		// If it's a symlink, resolve it
		if d.Type()&fs.ModeSymlink != 0 {
			realPath, err := filepath.EvalSymlinks(path)
			if err != nil {
				// Broken symlink - call fn with error and continue
				return fn(path, d, err)
			}

			info, err := os.Stat(realPath)
			if err != nil {
				return fn(path, d, err)
			}

			// If symlink points to directory, walk it (with cycle detection)
			if info.IsDir() {
				absReal, _ := filepath.Abs(realPath)
				if visited[absReal] {
					return nil // Cycle detected, skip
				}
				return walkDirFollowSymlinksInternal(realPath, fn, visited)
			}

			// If symlink points to file, call fn with resolved info
			return fn(realPath, fs.FileInfoToDirEntry(info), nil)
		}

		return fn(path, d, err)
	})
}

// getCodexSessionTimestamp extracts the timestamp from a Codex session file.
// It reads the first line looking for a session_meta event with a timestamp field.
// If the timestamp cannot be extracted or parsed, it falls back to the file modification time.
func getCodexSessionTimestamp(path string) (time.Time, error) {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	if scanner.Scan() {
		var entry struct {
			Timestamp string `json:"timestamp"`
			Type      string `json:"type"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err == nil {
			if entry.Type == "session_meta" && entry.Timestamp != "" {
				if ts, err := time.Parse(time.RFC3339, entry.Timestamp); err == nil {
					return ts, nil
				}
			}
		}
	}

	// Fallback to file modification time
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

// listCodexSessions returns all Codex sessions from ~/.codex/sessions/.
// The homeDir parameter allows testing with a mock home directory.
func listCodexSessions(homeDir string) ([]SessionInfo, error) {
	codexDir := filepath.Join(homeDir, ".codex", "sessions")

	// Check if directory exists
	realDir, err := filepath.EvalSymlinks(codexDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No error, just empty list
		}
		return nil, err
	}

	var sessions []SessionInfo
	err = walkDirFollowSymlinks(realDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}

		// Get file info for size
		info, err := d.Info()
		if err != nil {
			return nil // Skip files we can't stat
		}

		// Skip empty files
		if info.Size() == 0 {
			return nil
		}

		// Extract session ID from filename (the UUID part)
		filename := filepath.Base(path)
		sessionID := strings.TrimSuffix(filename, ".jsonl")

		// If filename contains a UUID, extract just the UUID as the ID
		if match := uuidPattern.FindString(filename); match != "" {
			sessionID = match
		}

		// Get timestamp from session_meta or file mtime
		createdAt, err := getCodexSessionTimestamp(path)
		if err != nil {
			createdAt = info.ModTime()
		}

		sessions = append(sessions, SessionInfo{
			ID:        sessionID,
			CreatedAt: createdAt,
			Size:      info.Size(),
			Source:    "codex",
		})

		return nil
	})

	if err != nil {
		return nil, err
	}

	return sessions, nil
}

// listClaudeSessions returns all Claude sessions for a project.
func listClaudeSessions(projectPath string) ([]SessionInfo, error) {
	claudeProjectPath := claudecode.BuildProjectPath(projectPath)
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	projectDir := filepath.Join(homeDir, ".claude", "projects", claudeProjectPath)

	// Check if directory exists
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		return nil, nil // No error, just empty list
	}

	// Read .jsonl files
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read project directory: %w", err)
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
			Source:    "claude",
		})
	}

	return sessions, nil
}

// listKiroSessions returns all Kiro sessions for the current working directory.
// Returns nil with no error if the Kiro database is not found (Kiro not installed).
func listKiroSessions(cwd string) ([]SessionInfo, error) {
	sessions, err := logs.DiscoverForDirectory(context.Background(), cwd)
	if err != nil {
		if errors.Is(err, logs.ErrDatabaseNotFound) {
			return nil, nil // Kiro not available, not an error
		}
		return nil, fmt.Errorf("discover kiro sessions: %w", err)
	}

	result := make([]SessionInfo, len(sessions))
	for i, s := range sessions {
		result[i] = SessionInfo{
			ID:        s.ConversationID,
			CreatedAt: s.UpdatedAt, // Use updated_at for sorting consistency
			Size:      s.Size,
			Source:    "kiro-cli",
		}
	}

	return result, nil
}

// resolveKiroSession attempts to find a Kiro session by ID in the given directory.
// Returns an io.Reader for the session JSON, or ErrSessionNotFound/ErrDatabaseNotFound.
func resolveKiroSession(sessionID, cwd string) (io.Reader, error) {
	return logs.GetSession(context.Background(), sessionID, cwd)
}

// listAllSessions returns sessions from Claude, Codex, and Kiro locations, merged and sorted.
func listAllSessions(projectPath string) ([]SessionInfo, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	// Get Claude sessions
	claudeSessions, err := listClaudeSessions(projectPath)
	if err != nil {
		// Log warning but continue with other sessions
		fmt.Fprintf(os.Stderr, "Warning: could not list Claude sessions: %v\n", err)
	}

	// Get Codex sessions
	codexSessions, err := listCodexSessions(homeDir)
	if err != nil {
		// Log warning but continue with other sessions
		fmt.Fprintf(os.Stderr, "Warning: could not list Codex sessions: %v\n", err)
	}

	// Get Kiro sessions
	kiroSessions, err := listKiroSessions(projectPath)
	if err != nil {
		// Log warning but continue with other sessions
		fmt.Fprintf(os.Stderr, "Warning: could not list Kiro sessions: %v\n", err)
	}

	// Merge sessions
	allSessions := make([]SessionInfo, 0, len(claudeSessions)+len(codexSessions)+len(kiroSessions))
	allSessions = append(allSessions, claudeSessions...)
	allSessions = append(allSessions, codexSessions...)
	allSessions = append(allSessions, kiroSessions...)

	// Sort by timestamp (oldest first) with Claude first for ties
	sortSessionsByTimestamp(allSessions)

	return allSessions, nil
}

// sortSessionsByTimestamp sorts sessions by creation time (oldest first).
// When timestamps are equal, Claude sessions come before Codex sessions.
func sortSessionsByTimestamp(sessions []SessionInfo) {
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].CreatedAt.Equal(sessions[j].CreatedAt) {
			// Claude comes before Codex for tie-breaking
			return sessions[i].Source == "claude" && sessions[j].Source != "claude"
		}
		return sessions[i].CreatedAt.Before(sessions[j].CreatedAt)
	})
}

// runFollow executes follow mode with signal handling.
// Uses signal.NotifyContext for SIGINT handling (requirement 6.1-6.4).
// Returns the exit code: 0 for clean exit, 130 for SIGINT (128 + 2).
func runFollow(filePath string, opts transcript.RenderOptions) int {
	// Set up signal handling with context
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT)
	defer stop()

	// Create follower
	follower, err := transcript.NewFollower(filePath, os.Stdout, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// Run follower
	if err := follower.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// Check if we were cancelled by SIGINT
	if ctx.Err() != nil {
		// Exit code 130 = 128 + SIGINT (2), per Unix convention
		return 130
	}

	return 0
}

// resolveFollowInput resolves input to a file path (not io.Reader).
// Returns error for stdin input (requirement 2.3, 2.4).
func resolveFollowInput(input string, projectPath string) (string, error) {
	// Check for stdin input
	if input == "" {
		return "", fmt.Errorf("cannot follow stdin input. Please provide a file path or session ID")
	}

	// If it's a file path, return directly
	if isFilePath(input) {
		return input, nil
	}

	// Treat as session ID - resolve to file path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	// Try Claude location first
	claudeProjectPath := claudecode.BuildProjectPath(projectPath)
	claudeSessionFile := filepath.Join(homeDir, ".claude", "projects", claudeProjectPath, input+".jsonl")
	if _, err := os.Stat(claudeSessionFile); err == nil {
		return claudeSessionFile, nil
	}

	// Try Codex location second
	codexPath, err := findCodexSession(homeDir, input)
	if err != nil {
		return "", fmt.Errorf("failed to search Codex sessions: %w", err)
	}
	if codexPath != "" {
		return codexPath, nil
	}

	// Not found in either location
	return "", fmt.Errorf("session not found: %s", input)
}

// validateFollowMode checks for incompatible flag combinations.
// Returns error if -F is used with -o or -f html.
func validateFollowMode(cfg *Config) error {
	if !cfg.Follow {
		return nil
	}

	// Check for -o/--output conflict (requirement 5.2, 5.3)
	if cfg.Output != "" {
		return fmt.Errorf("cannot use --output with --follow. Follow mode only supports stdout")
	}

	// Check for HTML format conflict (requirement 5.5, 5.6)
	if strings.ToLower(cfg.Format) == "html" {
		return fmt.Errorf("HTML output is not supported in follow mode. Use markdown format instead")
	}

	// Check for JSON format conflict
	if strings.ToLower(cfg.Format) == "json" {
		return fmt.Errorf("JSON output is not supported in follow mode. Use markdown format instead")
	}

	return nil
}

// agentToFormat converts an agent name to a transcript.Format.
func agentToFormat(agent string) transcript.Format {
	switch agent {
	case "claude-code":
		return transcript.FormatClaude
	case "codex":
		return transcript.FormatCodex
	case "kiro":
		return transcript.FormatKiro
	case "copilot":
		return transcript.FormatCopilot
	default:
		return transcript.FormatUnknown
	}
}

// convert reads a transcript file from input and writes formatted output (Markdown, HTML, or JSON).
// If agent is specified, it forces the use of that agent's parser instead of auto-detection.
func convert(input io.Reader, output io.Writer, sessionID string, format string, agent string) error {
	// Handle JSON format separately (outputs raw data, not parsed entries)
	if strings.ToLower(format) == "json" {
		return convertToJSON(input, output, agent)
	}

	var result *transcript.ParseResult
	var err error

	if agent != "" {
		// Force specific agent format
		result, err = transcript.ParseJSONLWithFormat(input, agentToFormat(agent))
	} else {
		// Auto-detect format (handles all formats: Claude, Codex, Kiro, Copilot)
		result, err = transcript.Parse(input)
	}
	if err != nil {
		return fmt.Errorf("failed to parse transcript: %w", err)
	}

	// Write warnings to stderr with line numbers
	for _, w := range result.Warnings {
		fmt.Fprintf(os.Stderr, "Warning: line %d: %s\n", w.Line, w.Message)
	}

	// Report warning summary if any warnings occurred
	if len(result.Warnings) > 0 {
		fmt.Fprintf(os.Stderr, "Parsed with %d warning(s)\n", len(result.Warnings))
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

	var rendered string
	switch strings.ToLower(format) {
	case "html":
		rendered = transcript.RenderHTML(result.Entries, opts)
	case "md", "markdown", "":
		rendered = transcript.RenderMarkdown(result.Entries, opts)
	default:
		return fmt.Errorf("unsupported format: %s (use md, markdown, html, or json)", format)
	}

	_, err = output.Write([]byte(rendered))
	if err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	return nil
}

// convertToJSON outputs the raw session data as pretty-printed JSON.
// For JSONL formats (Claude, Codex, Copilot), it outputs an array of entries.
// For Kiro (JSON), it outputs the session object directly.
func convertToJSON(input io.Reader, output io.Writer, agent string) error {
	// Read all input
	data, err := io.ReadAll(input)
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	if len(data) == 0 {
		fmt.Fprintln(os.Stderr, "Session contains no data")
		return nil
	}

	// Determine format
	var detectedFormat transcript.Format
	if agent != "" {
		detectedFormat = agentToFormat(agent)
	} else {
		detectedFormat, _, err = transcript.DetectFormat(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("failed to detect format: %w", err)
		}
	}

	var result any

	if detectedFormat == transcript.FormatKiro {
		// Kiro is already JSON - unmarshal to preserve structure
		if err := json.Unmarshal(data, &result); err != nil {
			return fmt.Errorf("failed to parse Kiro JSON: %w", err)
		}
	} else {
		// JSONL formats - parse each line and collect into array
		var entries []json.RawMessage
		scanner := bufio.NewScanner(bytes.NewReader(data))
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 10*1024*1024) // 10MB max line

		for scanner.Scan() {
			line := scanner.Bytes()
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			// Validate it's valid JSON
			var check json.RawMessage
			if err := json.Unmarshal(line, &check); err != nil {
				// Skip invalid lines but warn
				fmt.Fprintf(os.Stderr, "Warning: skipping invalid JSON line\n")
				continue
			}
			entries = append(entries, json.RawMessage(line))
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("failed to scan input: %w", err)
		}
		result = entries
	}

	// Pretty-print the JSON
	prettyJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format JSON: %w", err)
	}

	_, err = output.Write(prettyJSON)
	if err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	// Add trailing newline
	_, _ = output.Write([]byte("\n"))

	return nil
}
