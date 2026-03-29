// Package copilot provides the GitHub Copilot agent implementation.
package copilot

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
	"github.com/arjenschwarz/orbit/internal/cost"
	"gopkg.in/yaml.v3"
)

// Compile-time interface check.
var _ agents.Agent = (*Agent)(nil)

const defaultPrompt = "Run /next-task --phase and when complete run /commit"

func init() {
	agents.Register("copilot", New)
}

// Agent implements the agents.Agent interface for GitHub Copilot.
type Agent struct {
	config     agents.AgentConfig
	cliPath    string
	sessionDir string // override for testing; empty uses DefaultSessionDir()
}

// New creates a new Copilot agent.
func New(cfg agents.AgentConfig) agents.Agent {
	cliPath := cfg.CLIPath
	if cliPath == "" {
		cliPath = "copilot"
	}
	return &Agent{
		config:  cfg,
		cliPath: cliPath,
	}
}

// Name returns the agent identifier.
func (a *Agent) Name() string { return "copilot" }

// CLICommand returns the CLI command to execute.
func (a *Agent) CLICommand() string { return a.cliPath }

// IsInstalled returns true if the Copilot CLI is available.
func (a *Agent) IsInstalled() bool {
	_, err := exec.LookPath(a.cliPath)
	return err == nil
}

// Version returns the installed Copilot CLI version.
func (a *Agent) Version() (string, error) {
	cmd := exec.Command(a.cliPath, "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(output)), nil
}

// DefaultSessionDir returns the default session storage directory.
func (a *Agent) DefaultSessionDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".copilot", "session-state")
}

// DiscoverSessions lists sessions for a given project directory.
// Copilot stores sessions as directories under ~/.copilot/session-state/<session-id>/
// containing events.jsonl and workspace.yaml.
func (a *Agent) DiscoverSessions(ctx context.Context, projectDir string) ([]agents.SessionInfo, error) {
	sessionDir := a.sessionDir
	if sessionDir == "" {
		sessionDir = a.DefaultSessionDir()
	}
	if sessionDir == "" {
		return nil, nil
	}

	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var sessions []agents.SessionInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		sessionPath := filepath.Join(sessionDir, entry.Name())
		eventsPath := filepath.Join(sessionPath, "events.jsonl")
		workspacePath := filepath.Join(sessionPath, "workspace.yaml")

		// Verify events.jsonl exists and is non-empty.
		eventsInfo, err := os.Stat(eventsPath)
		if err != nil || eventsInfo.Size() == 0 {
			continue
		}

		// Parse workspace.yaml to filter by project directory.
		ws, err := parseCopilotWorkspace(workspacePath)
		if err != nil || ws == nil {
			continue
		}

		matchPath := ws.GitRoot
		if matchPath == "" {
			matchPath = ws.Cwd
		}
		if projectDir != "" && matchPath != "" && normalizePath(matchPath) != normalizePath(projectDir) {
			continue
		}

		var createdAt time.Time
		if ws.CreatedAt != nil {
			createdAt = *ws.CreatedAt
		} else {
			createdAt = eventsInfo.ModTime()
		}

		sessions = append(sessions, agents.SessionInfo{
			ID:        entry.Name(),
			Agent:     "copilot",
			Path:      eventsPath,
			CreatedAt: createdAt,
			Size:      eventsInfo.Size(),
		})
	}

	return sessions, nil
}

// copilotWorkspace represents the parsed contents of a Copilot workspace.yaml file.
type copilotWorkspace struct {
	ID        string     `yaml:"id"`
	Cwd       string     `yaml:"cwd"`
	GitRoot   string     `yaml:"git_root"`
	CreatedAt *time.Time `yaml:"created_at"`
}

// parseCopilotWorkspace parses a Copilot workspace.yaml file.
func parseCopilotWorkspace(path string) (*copilotWorkspace, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var ws copilotWorkspace
	if err := yaml.Unmarshal(data, &ws); err != nil {
		return nil, nil
	}

	return &ws, nil
}

// normalizePath resolves symlinks and cleans a file path for reliable comparison.
func normalizePath(p string) string {
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return filepath.Clean(p)
	}
	return filepath.Clean(resolved)
}

// Run executes a prompt in a new session.
func (a *Agent) Run(ctx context.Context, opts agents.RunOptions) (*agents.RunResult, error) {
	return a.execute(ctx, opts, false)
}

// Resume continues an existing session.
// LIMITATION: Copilot CLI only supports resuming the most recent session via --continue.
// The sessionID parameter is accepted for interface compatibility but ignored.
// Orchestrator should be aware that Copilot cannot resume arbitrary sessions by ID.
func (a *Agent) Resume(ctx context.Context, sessionID string, opts agents.RunOptions) (*agents.RunResult, error) {
	opts.SessionID = sessionID
	return a.execute(ctx, opts, true)
}

// buildArgs constructs the command-line arguments for a Copilot session.
func (a *Agent) buildArgs(opts agents.RunOptions, resume bool) []string {
	prompt := opts.Prompt
	if prompt == "" {
		prompt = defaultPrompt
	}

	// Copilot uses: copilot -p "<prompt>"
	// With --yolo for automatic approval (equivalent to --allow-all-tools --allow-all-paths --allow-all-url)
	// With --continue to resume previous session
	var args []string

	if a.config.AutoApprove {
		args = append(args, "--yolo")
	}

	if resume {
		args = append(args, "--continue")
	}

	// Add model flag if configured
	if model, ok := a.config.Options["model"]; ok && model != "" {
		args = append(args, "--model", model)
	}

	// Add config-level extra args
	args = append(args, a.config.ExtraArgs...)

	// Add per-invocation extra args
	args = append(args, opts.ExtraArgs...)

	// Prompt comes with -p flag
	args = append(args, "-p", prompt)

	return args
}

// execute runs the Copilot CLI with the given options.
func (a *Agent) execute(ctx context.Context, opts agents.RunOptions, resume bool) (*agents.RunResult, error) {
	args := a.buildArgs(opts, resume)

	execResult := agents.Execute(ctx, agents.ExecuteConfig{
		CLIPath: a.cliPath,
		Args:    args,
		WorkDir: opts.WorkDir,
		Env:     opts.Env,
		Timeout: opts.Timeout,
	})

	stdoutStr := string(execResult.Stdout)

	result := &agents.RunResult{
		SessionID: opts.SessionID,
		Duration:  execResult.Duration,
		ExitCode:  execResult.ExitCode,
		Output:    stdoutStr,
		Stderr:    string(execResult.Stderr),
	}

	if execResult.Err != nil {
		result.Error = execResult.Err
	}

	// Extract usage metrics from CLI output
	if usage := ParseUsage(stdoutStr, string(execResult.Stderr)); usage != nil {
		result.Cost = &agents.CostMetrics{
			PremiumRequests: usage.PremiumRequests,
			InputTokens:     usage.InputTokens,
			OutputTokens:    usage.OutputTokens,
			CachedTokens:    usage.CachedTokens,
			APIDuration:     usage.APIDuration,
			SessionDuration: usage.SessionDuration,
			LinesAdded:      usage.LinesAdded,
			LinesRemoved:    usage.LinesRemoved,
			CostUnit:        cost.UnitPremiumRequests,
		}
		debugLog("Extracted Copilot usage: %.2f premium requests, %d tokens in, %d tokens out",
			usage.PremiumRequests, usage.InputTokens, usage.OutputTokens)
	}

	return result, execResult.Err
}
