package comparison

import (
	"context"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
)

// AgentAdapter adapts agents.Agent to the promptRunner interface.
// This allows Comparator to use any agent implementation.
type AgentAdapter struct {
	agent     agents.Agent
	workDir   string
	extraArgs []string
	timeout   time.Duration
}

// NewAgentAdapter creates a new adapter wrapping the given agent.
// Panics if agent is nil or workDir is empty, as these are
// programming errors that should be caught at initialization.
func NewAgentAdapter(agent agents.Agent, workDir string) *AgentAdapter {
	if agent == nil {
		panic("agent cannot be nil")
	}
	if workDir == "" {
		panic("workDir cannot be empty")
	}
	return &AgentAdapter{
		agent:   agent,
		workDir: workDir,
	}
}

// WithExtraArgs returns a copy of the adapter with additional CLI arguments.
// These are passed through to the agent's ExtraArgs on each invocation.
func (a *AgentAdapter) WithExtraArgs(args ...string) *AgentAdapter {
	cp := *a
	cp.extraArgs = args
	return &cp
}

// WithTimeout returns a copy of the adapter with a per-execution timeout.
// The timeout is forwarded to RunOptions.Timeout so the underlying agent's
// executor enforces it (mirroring how phase execution honors
// AgentConfig.Timeout). A zero duration means no per-execution timeout.
func (a *AgentAdapter) WithTimeout(timeout time.Duration) *AgentAdapter {
	cp := *a
	cp.timeout = timeout
	return &cp
}

// RunCustomPrompt implements the promptRunner interface by delegating to agent.Run().
func (a *AgentAdapter) RunCustomPrompt(ctx context.Context, prompt string) (*agents.RunResult, error) {
	opts := agents.RunOptions{
		Prompt:    prompt,
		WorkDir:   a.workDir,
		ExtraArgs: a.extraArgs,
		Timeout:   a.timeout,
	}
	return a.agent.Run(ctx, opts)
}
