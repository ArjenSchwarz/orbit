// Package kiro provides the Kiro agent implementation.
package kiro

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
	"github.com/arjenschwarz/orbit/internal/agents/kiro/logs"
	"github.com/arjenschwarz/orbit/internal/transcript"
)

const defaultPrompt = "Run /next-task --phase and when complete run /commit"

// debugLog logs a message if ORBIT_DEBUG is enabled.
func debugLog(format string, args ...any) {
	if env := os.Getenv("ORBIT_DEBUG"); env == "true" || env == "1" {
		log.Printf("[kiro-agent] "+format, args...)
	}
}

func init() {
	agents.Register("kiro", New)
}

// Agent implements the agents.Agent interface for Kiro.
type Agent struct {
	config  agents.AgentConfig
	cliPath string
}

// Compile-time interface checks.
var _ agents.Agent = (*Agent)(nil)

// New creates a new Kiro agent.
func New(cfg agents.AgentConfig) agents.Agent {
	cliPath := cfg.CLIPath
	if cliPath == "" {
		cliPath = "kiro-cli"
	}
	return &Agent{
		config:  cfg,
		cliPath: cliPath,
	}
}

// Name returns the agent identifier.
func (a *Agent) Name() string { return "kiro" }

// CLICommand returns the CLI command to execute.
func (a *Agent) CLICommand() string { return a.cliPath }

// IsInstalled returns true if the Kiro CLI is available.
func (a *Agent) IsInstalled() bool {
	_, err := exec.LookPath(a.cliPath)
	return err == nil
}

// Version returns the installed Kiro CLI version.
func (a *Agent) Version() (string, error) {
	cmd := exec.Command(a.cliPath, "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(output)), nil
}

// DefaultSessionDir returns the default session storage directory.
// Kiro does not store sessions automatically per Decision 7.
func (a *Agent) DefaultSessionDir() string {
	return ""
}

// DiscoverSessions lists sessions for a given project directory.
// Sessions are retrieved from Kiro's SQLite database.
func (a *Agent) DiscoverSessions(ctx context.Context, projectDir string) ([]agents.SessionInfo, error) {
	sessions, err := logs.DiscoverForDirectory(ctx, projectDir)
	if err != nil {
		if errors.Is(err, logs.ErrDatabaseNotFound) {
			// Kiro not installed or never used, not an error
			return nil, nil
		}
		return nil, err
	}

	result := make([]agents.SessionInfo, len(sessions))
	for i, s := range sessions {
		result[i] = agents.SessionInfo{
			ID:        s.ConversationID,
			Agent:     "kiro",
			Path:      "", // No filesystem path - sessions are in SQLite
			CreatedAt: s.CreatedAt,
			Size:      s.Size,
			Project:   s.Directory,
		}
	}

	return result, nil
}

// Run executes a prompt in a new session.
func (a *Agent) Run(ctx context.Context, opts agents.RunOptions) (*agents.RunResult, error) {
	return a.execute(ctx, opts, false)
}

// Resume continues an existing session.
// Note: Kiro uses --resume to continue the current session.
func (a *Agent) Resume(ctx context.Context, sessionID string, opts agents.RunOptions) (*agents.RunResult, error) {
	opts.SessionID = sessionID
	return a.execute(ctx, opts, true)
}

// buildArgs constructs the command-line arguments for a Kiro session.
func (a *Agent) buildArgs(opts agents.RunOptions, resume bool) []string {
	prompt := opts.Prompt
	if prompt == "" {
		prompt = defaultPrompt
	}

	// Kiro uses: kiro-cli chat --no-interactive "<prompt>"
	// With --trust-all-tools for automatic approval
	// With --resume to continue previous session
	args := []string{"chat", "--no-interactive"}

	if a.config.AutoApprove {
		args = append(args, "--trust-all-tools")
	}

	if resume {
		args = append(args, "--resume")
	}

	// Add model flag if configured
	if model, ok := a.config.Options["model"]; ok && model != "" {
		args = append(args, "--model", model)
	}

	// Add config-level extra args
	args = append(args, a.config.ExtraArgs...)

	// Add per-invocation extra args
	args = append(args, opts.ExtraArgs...)

	// Prompt comes last
	args = append(args, prompt)

	return args
}

// execute runs the Kiro CLI with the given options.
func (a *Agent) execute(ctx context.Context, opts agents.RunOptions, resume bool) (*agents.RunResult, error) {
	args := a.buildArgs(opts, resume)

	cmd := exec.CommandContext(ctx, a.cliPath, args...)
	workDir := opts.WorkDir
	if workDir != "" {
		cmd.Dir = workDir
	} else {
		// Get current working directory for session lookup
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			workDir = ""
		}
	}

	// Set environment variables
	if len(opts.Env) > 0 {
		cmd.Env = appendEnv(opts.Env)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = nil // Explicitly close stdin so Kiro doesn't wait for input

	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	result := &agents.RunResult{
		SessionID: opts.SessionID,
		Duration:  duration,
		Output:    stdout.String(),
		Stderr:    stderr.String(),
	}

	// Get exit code
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
	}

	if err != nil {
		result.Error = err
	}

	// Try to extract usage info from the session
	if workDir != "" {
		if credits := a.extractSessionCredits(ctx, workDir); credits > 0 {
			result.Cost = &agents.CostMetrics{
				Credits: credits,
			}
		}
	}

	return result, err
}

// extractSessionCredits fetches the most recent session for the directory and extracts credit usage.
func (a *Agent) extractSessionCredits(ctx context.Context, workDir string) float64 {
	// Resolve to absolute path
	absPath, err := filepath.Abs(workDir)
	if err != nil {
		debugLog("extractSessionCredits: failed to resolve path %s: %v", workDir, err)
		return 0
	}

	// Discover sessions for this directory
	sessions, err := logs.DiscoverForDirectory(ctx, absPath)
	if err != nil {
		debugLog("extractSessionCredits: failed to discover sessions for %s: %v", absPath, err)
		return 0
	}
	if len(sessions) == 0 {
		debugLog("extractSessionCredits: no sessions found for %s", absPath)
		return 0
	}

	// Get the most recent session (DiscoverForDirectory returns sorted by UpdatedAt descending)
	mostRecent := sessions[0]
	debugLog("extractSessionCredits: found session %s", mostRecent.ConversationID)

	// Fetch the session JSON
	reader, err := logs.GetSession(ctx, mostRecent.ConversationID, absPath)
	if err != nil {
		debugLog("extractSessionCredits: failed to get session %s: %v", mostRecent.ConversationID, err)
		return 0
	}

	// Parse and extract credits
	credits, err := transcript.ParseKiroUsageInfo(reader)
	if err != nil {
		debugLog("extractSessionCredits: failed to parse usage info: %v", err)
		return 0
	}

	debugLog("extractSessionCredits: extracted %.4f credits", credits)
	return credits
}

// appendEnv appends environment variables to the current environment.
func appendEnv(env map[string]string) []string {
	result := os.Environ() // Start with existing environment
	for k, v := range env {
		result = append(result, k+"="+v)
	}
	return result
}
