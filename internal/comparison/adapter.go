package comparison

import (
	"context"

	"github.com/arjenschwarz/orbit/internal/agents"
)

// AgentAdapter adapts agents.Agent to the promptRunner interface.
// This allows Comparator to use any agent implementation.
//
// Context Lifetime: The adapter captures the context at construction time.
// Create a new adapter for each operation rather than caching it across runs,
// as the context may become stale or cancelled.
type AgentAdapter struct {
	agent   agents.Agent
	ctx     context.Context
	workDir string
}

// NewAgentAdapter creates a new adapter wrapping the given agent.
func NewAgentAdapter(agent agents.Agent, ctx context.Context, workDir string) *AgentAdapter {
	return &AgentAdapter{
		agent:   agent,
		ctx:     ctx,
		workDir: workDir,
	}
}

// RunCustomPrompt implements the promptRunner interface by delegating to agent.Run().
// Note: AutoApprove is controlled by the agent's config, not RunOptions.
// The agent passed to the adapter should already be configured with AutoApprove if needed.
func (a *AgentAdapter) RunCustomPrompt(prompt string) (*agents.RunResult, error) {
	opts := agents.RunOptions{
		Prompt:  prompt,
		WorkDir: a.workDir,
	}
	return a.agent.Run(a.ctx, opts)
}
