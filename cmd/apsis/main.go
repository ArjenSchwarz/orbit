// Package main provides the apsis CLI tool for converting Claude Code session transcripts
// from JSONL format to readable Markdown.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/arjenschwarz/orbit/internal/apsisweb"
	"github.com/arjenschwarz/orbit/internal/sessions"
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

func main() {
	// Detect serve subcommand before flag.Parse() (req. 2.8)
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		if err := serveCommand(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

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

// serveCommand handles the 'apsis serve' subcommand.
func serveCommand(args []string) error {
	fs := flag.NewFlagSet("apsis serve", flag.ContinueOnError)
	port := fs.Int("port", 0, "Port to listen on (default 8081, or APSIS_SERVE_PORT)")
	bind := fs.String("bind", "", "Address to bind to (default localhost, or APSIS_SERVE_BIND)")
	project := fs.String("project", "", "Project directory (default: current directory)")
	showVersion := fs.Bool("version", false, "Show version and exit")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: apsis serve [options]\n\n")
		fmt.Fprintf(os.Stderr, "Start a web server to browse session transcripts.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nEnvironment:\n")
		fmt.Fprintf(os.Stderr, "  APSIS_SERVE_PORT    Port to listen on (default 8081)\n")
		fmt.Fprintf(os.Stderr, "  APSIS_SERVE_BIND    Address to bind to (default localhost)\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  apsis serve                    Start with default settings\n")
		fmt.Fprintf(os.Stderr, "  apsis serve --port 3000        Start on port 3000\n")
		fmt.Fprintf(os.Stderr, "  apsis serve --bind 0.0.0.0     Listen on all interfaces\n")
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *showVersion {
		fmt.Printf("apsis serve version %s\n", version)
		return nil
	}

	// Resolve defaults: CLI flag > env var > default
	resolvedPort := resolveInt(*port, "APSIS_SERVE_PORT", 8081)
	resolvedBind := resolveString(*bind, "APSIS_SERVE_BIND", "localhost")

	// Network binding warning (req. 2.9)
	if resolvedBind == "0.0.0.0" {
		fmt.Fprintln(os.Stderr, "Warning: Server is accessible from the network. Session data may contain sensitive information.")
	}

	// Resolve project path
	projectPath, err := resolveProjectPath(*project)
	if err != nil {
		return fmt.Errorf("resolve project path: %w", err)
	}

	// Create server
	server, err := apsisweb.New(apsisweb.Config{
		Port:        resolvedPort,
		Bind:        resolvedBind,
		ProjectPath: projectPath,
	})
	if err != nil {
		return fmt.Errorf("create server: %w", err)
	}

	// Print URL (req. 2.6)
	fmt.Fprintf(os.Stderr, "Listening on http://%s:%d\n", resolvedBind, resolvedPort)

	// Signal handling (req. 2.7)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start()
	}()

	select {
	case sig := <-sigCh:
		fmt.Fprintf(os.Stderr, "\nReceived %v, shutting down...\n", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(ctx)
	case err := <-errCh:
		return err
	}
}

// resolveInt returns: flag value if non-zero, env var if set and valid, otherwise default.
func resolveInt(flagVal int, envKey string, defaultVal int) int {
	if flagVal != 0 {
		return flagVal
	}
	if envStr := os.Getenv(envKey); envStr != "" {
		if val, err := strconv.Atoi(envStr); err == nil {
			return val
		}
	}
	return defaultVal
}

// resolveString returns: flag value if non-empty, env var if set, otherwise default.
func resolveString(flagVal, envKey, defaultVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if envStr := os.Getenv(envKey); envStr != "" {
		return envStr
	}
	return defaultVal
}

// resolveProjectPath returns the absolute path of the project directory.
func resolveProjectPath(project string) (string, error) {
	if project == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("get working directory: %w", err)
		}
		return cwd, nil
	}
	return filepath.Abs(project)
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
	flag.StringVar(&cfg.Agent, "a", "", "Force agent format (claude-code, codex, kiro, kiro-ide, copilot)")
	flag.StringVar(&cfg.Agent, "agent", "", "Force agent format (claude-code, codex, kiro, kiro-ide, copilot)")
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
  apsis [options] [session-id | file-path | latest]
  apsis serve [options]
  cat session.jsonl | apsis

Subcommands:
  serve                   Start a web server to browse session transcripts
                          Run 'apsis serve --help' for serve-specific options

Options:
  -l, --list              List available sessions for the project
  -o, --output <file>     Write output to file (default: stdout)
  -p, --project <path>    Project directory (default: current directory)
  -f, --format <format>   Output format: md, markdown, html, json (default: md)
                          json outputs the raw session data as pretty-printed JSON
  -a, --agent <name>      Force agent format: claude-code, codex, kiro, kiro-ide, copilot
                          (default: auto-detect from content)
  -F, --follow            Follow mode: continuously monitor file for new entries
                          (like tail -f, markdown output to stdout only)
  -v, --version           Show version
  -h, --help              Show this help

Examples:
  apsis latest                                   Convert the most recent session
  apsis latest -f html -o out.html               Save latest session as HTML
  apsis latest -F                                Follow the most recent session
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
  apsis serve                                    Start web server on localhost:8081
  apsis serve --port 3000 --bind 0.0.0.0         Start on custom port and address
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

	// Handle "latest" keyword before isFilePath() so a file named "latest" doesn't shadow it
	if cfg.Input == "latest" {
		return runLatest(cfg, absProjectPath)
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
	input, sessionID, costPath, err := resolveInput(cfg.Input, absProjectPath)
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

	return 0, convert(input, output, sessionID, cfg.Format, cfg.Agent, costPath)
}

// isFilePath returns true if the argument appears to be a file path rather than a session ID.
func isFilePath(arg string) bool {
	// Contains path separator
	if strings.Contains(arg, "/") || strings.Contains(arg, "\\") {
		return true
	}

	// Ends with known file extensions
	if strings.HasSuffix(arg, ".jsonl") || strings.HasSuffix(arg, ".chat") {
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

// resolveInput determines the input source and returns a reader, session ID, and cost path.
// It checks Claude location first, then Codex, then Copilot, then Kiro CLI, then Kiro IDE.
// The cost path is non-empty only for Kiro IDE sessions (used for credit cost extraction).
func resolveInput(arg string, projectPath string) (io.ReadCloser, string, string, error) {
	// If no argument, read from stdin
	if arg == "" {
		return os.Stdin, "", "", nil
	}

	// Check if it's a file path
	if isFilePath(arg) {
		f, err := os.Open(arg)
		if err != nil {
			return nil, "", "", fmt.Errorf("failed to open file: %w", err)
		}
		// Extract session ID from filename if it's a .jsonl file
		sessionID := ""
		if strings.HasSuffix(arg, ".jsonl") {
			sessionID = strings.TrimSuffix(filepath.Base(arg), ".jsonl")
		}
		// For .chat files, derive cost path from the file's executionId and parent directory
		costPath := ""
		if strings.HasSuffix(arg, ".chat") {
			costPath = deriveChatFileCostPath(arg)
		}
		return f, sessionID, costPath, nil
	}

	// Treat as session ID — try each source via Resolver
	resolver, err := sessions.NewResolver(projectPath)
	if err != nil {
		return nil, "", "", err
	}

	// Try sources in priority order
	sources := []string{
		sessions.SourceClaude,
		sessions.SourceCodex,
		sessions.SourceCopilot,
		sessions.SourceKiroCLI,
		sessions.SourceKiroIDE,
	}

	for _, source := range sources {
		resolved, err := resolver.Resolve(source, arg)
		if err == nil {
			return resolved.Reader, arg, resolved.Metadata.CostPath, nil
		}
	}

	return nil, "", "", fmt.Errorf("session not found: %s", arg)
}

// listSessions lists all sessions for a project.
func listSessions(projectPath string) error {
	lister, err := sessions.NewLister()
	if err != nil {
		return err
	}

	sessionList, warnings, err := lister.ListAll(projectPath)
	if err != nil {
		return err
	}

	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "Warning: could not list %s sessions: %v\n", sessions.DisplayName(w.Source), w.Err)
	}

	if len(sessionList) == 0 {
		fmt.Println("no sessions found")
		return nil
	}

	for _, s := range sessionList {
		fmt.Printf("[%s]\t%s\t%s\t%s\n",
			sessions.DisplayName(s.Source),
			s.ID,
			s.CreatedAt.Local().Format(time.RFC3339),
			sessions.FormatSize(s.Size))
	}

	return nil
}

// resolveLatestSession returns the newest session for the project.
func resolveLatestSession(projectPath string) (*sessions.SessionInfo, error) {
	lister, err := sessions.NewLister()
	if err != nil {
		return nil, err
	}

	sessionList, warnings, err := lister.ListAll(projectPath)
	if err != nil {
		return nil, err
	}

	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "Warning: could not list %s sessions: %v\n", sessions.DisplayName(w.Source), w.Err)
	}

	if len(sessionList) == 0 {
		return nil, fmt.Errorf("no sessions found for project")
	}

	// ListAll sorts oldest-first, so the last element is the newest
	latest := &sessionList[len(sessionList)-1]
	return latest, nil
}

// runLatest handles the "latest" keyword by resolving the newest session and routing
// to the appropriate output path (follow mode or normal conversion).
func runLatest(cfg *Config, projectPath string) (int, error) {
	latest, err := resolveLatestSession(projectPath)
	if err != nil {
		return 0, err
	}

	fmt.Fprintf(os.Stderr, "Using %s session %s from %s\n",
		sessions.DisplayName(latest.Source),
		latest.ID,
		latest.CreatedAt.Local().Format(time.RFC3339))

	resolver, err := sessions.NewResolver(projectPath)
	if err != nil {
		return 0, err
	}

	// Handle follow mode
	if cfg.Follow {
		// Sources that support follow mode (must be JSONL file-backed).
		// Kiro IDE is file-backed (.chat) but uses JSON, not JSONL,
		// so it cannot be followed by transcript.NewFollower.
		fileBackedSources := map[string]bool{
			sessions.SourceClaude:  true,
			sessions.SourceCodex:   true,
			sessions.SourceCopilot: true,
		}
		if !fileBackedSources[latest.Source] {
			return 0, fmt.Errorf("latest session is a %s session which cannot be followed (not file-backed)", sessions.DisplayName(latest.Source))
		}

		filePath, err := resolver.ResolvePath(latest.Source, latest.ID)
		if err != nil {
			return 0, err
		}

		opts := transcript.RenderOptions{
			Title: "Session Transcript",
		}
		return runFollow(filePath, opts), nil
	}

	// Normal mode: resolve to reader
	resolved, err := resolver.Resolve(latest.Source, latest.ID)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resolved.Reader.Close() }()

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

	return 0, convert(resolved.Reader, output, latest.ID, cfg.Format, cfg.Agent, resolved.Metadata.CostPath)
}

// deriveChatFileCostPath extracts the executionId from a .chat file and derives
// the cost path from the file's parent directory. Returns empty string on any error.
func deriveChatFileCostPath(chatPath string) string {
	data, err := os.ReadFile(chatPath)
	if err != nil {
		return ""
	}
	var header struct {
		ExecutionID string `json:"executionId"`
	}
	if err := json.Unmarshal(data, &header); err != nil || header.ExecutionID == "" {
		return ""
	}
	workspaceDir := filepath.Dir(chatPath)
	return transcript.KiroIDEExecutionDetailPath(workspaceDir, header.ExecutionID)
}

// resolveFollowInput resolves input to a file path (not io.Reader).
// Returns error for stdin input (requirement 2.3, 2.4).
func resolveFollowInput(input string, projectPath string) (string, error) {
	if input == "" {
		return "", fmt.Errorf("cannot follow stdin input. Please provide a file path or session ID")
	}

	if isFilePath(input) {
		return input, nil
	}

	// Treat as session ID — try each file-backed source via Resolver.ResolvePath
	resolver, err := sessions.NewResolver(projectPath)
	if err != nil {
		return "", err
	}

	fileSources := []string{
		sessions.SourceClaude,
		sessions.SourceCodex,
		sessions.SourceCopilot,
		// Kiro CLI is SQLite-backed, no file path
		// Kiro IDE could work but .chat files aren't JSONL
	}

	for _, source := range fileSources {
		path, err := resolver.ResolvePath(source, input)
		if err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("session not found: %s", input)
}

// runFollow executes follow mode with signal handling.
// Uses signal.NotifyContext for SIGINT handling (requirement 6.1-6.4).
// Returns the exit code: 0 for clean exit, 130 for SIGINT (128 + 2).
func runFollow(filePath string, opts transcript.RenderOptions) int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT)
	defer stop()

	follower, err := transcript.NewFollower(filePath, os.Stdout, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if err := follower.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if ctx.Err() != nil {
		return 130
	}

	return 0
}

// validateFollowMode checks for incompatible flag combinations.
func validateFollowMode(cfg *Config) error {
	if !cfg.Follow {
		return nil
	}

	if cfg.Output != "" {
		return fmt.Errorf("cannot use --output with --follow. Follow mode only supports stdout")
	}

	if strings.ToLower(cfg.Format) == "html" {
		return fmt.Errorf("HTML output is not supported in follow mode. Use markdown format instead")
	}

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
	case "kiro-ide":
		return transcript.FormatKiroIDE
	default:
		return transcript.FormatUnknown
	}
}

// convert reads a transcript file from input and writes formatted output (Markdown, HTML, or JSON).
func convert(input io.Reader, output io.Writer, sessionID string, format string, agent string, costPath string) error {
	if strings.ToLower(format) == "json" {
		return convertToJSON(input, output, agent)
	}

	var result *transcript.ParseResult
	var err error

	var parseOpts []transcript.ParseOptions
	if costPath != "" {
		parseOpts = append(parseOpts, transcript.ParseOptions{KiroIDECostPath: costPath})
	}

	if agent != "" {
		result, err = transcript.ParseJSONLWithFormat(input, agentToFormat(agent), parseOpts...)
	} else if costPath != "" {
		result, err = transcript.ParseJSONLWithFormat(input, transcript.FormatKiroIDE, parseOpts...)
	} else {
		result, err = transcript.Parse(input)
	}
	if err != nil {
		return fmt.Errorf("failed to parse transcript: %w", err)
	}

	for _, w := range result.Warnings {
		fmt.Fprintf(os.Stderr, "Warning: line %d: %s\n", w.Line, w.Message)
	}

	if len(result.Warnings) > 0 {
		fmt.Fprintf(os.Stderr, "Parsed with %d warning(s)\n", len(result.Warnings))
	}

	if len(result.Entries) == 0 {
		fmt.Fprintln(os.Stderr, "Session contains no entries")
		return nil
	}

	opts := transcript.RenderOptions{
		Title:     "Session Transcript",
		SessionID: sessionID,
	}

	if result.Metadata != nil {
		opts.TotalCost = result.Metadata.TotalCost
		opts.CostUnit = result.Metadata.CostUnit
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
func convertToJSON(input io.Reader, output io.Writer, agent string) error {
	data, err := io.ReadAll(input)
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	if len(data) == 0 {
		fmt.Fprintln(os.Stderr, "Session contains no data")
		return nil
	}

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

	if detectedFormat == transcript.FormatKiro || detectedFormat == transcript.FormatKiroIDE {
		if err := json.Unmarshal(data, &result); err != nil {
			return fmt.Errorf("failed to parse JSON: %w", err)
		}
	} else {
		var entries []json.RawMessage
		scanner := bufio.NewScanner(bytes.NewReader(data))
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 10*1024*1024)

		for scanner.Scan() {
			line := scanner.Bytes()
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			var check json.RawMessage
			if err := json.Unmarshal(line, &check); err != nil {
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

	prettyJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format JSON: %w", err)
	}

	_, err = output.Write(prettyJSON)
	if err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	_, _ = output.Write([]byte("\n"))

	return nil
}
