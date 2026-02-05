package comparison

import (
	"context"
	"testing"

	"github.com/arjenschwarz/orbit/internal/agents"
	"github.com/arjenschwarz/orbit/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestAgentAdapter_RunCustomPrompt(t *testing.T) {
	scenario := testutil.NewScenario().
		Success("test-session", 0.05).
		Build()

	agent := testutil.NewTestAgent(t, "mock", scenario)
	t.Cleanup(func() { agent.AssertAllConsumed(t) })

	ctx := context.Background()
	workDir := "/test/workdir"
	adapter := NewAgentAdapter(agent, ctx, workDir)

	result, err := adapter.RunCustomPrompt("test prompt")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "test-session", result.SessionID)
	require.Equal(t, 0, result.ExitCode)

	// Verify the agent was called with correct options
	agent.Recorder().AssertCallCount(t, 1)
	calls := agent.Recorder().Calls()
	require.Len(t, calls, 1)
	require.Equal(t, "Run", calls[0].Method)
	require.Equal(t, "test prompt", calls[0].Options.Prompt)
	require.Equal(t, workDir, calls[0].Options.WorkDir)
}

func TestAgentAdapter_PassesWorkDir(t *testing.T) {
	scenario := testutil.NewScenario().
		Success("session-1", 0.01).
		Build()

	agent := testutil.NewTestAgent(t, "mock", scenario)
	t.Cleanup(func() { agent.AssertAllConsumed(t) })

	ctx := context.Background()
	workDir := "/different/path/to/project"
	adapter := NewAgentAdapter(agent, ctx, workDir)

	_, err := adapter.RunCustomPrompt("any prompt")
	require.NoError(t, err)

	calls := agent.Recorder().Calls()
	require.Len(t, calls, 1)
	require.Equal(t, workDir, calls[0].Options.WorkDir)
}

func TestAgentAdapter_PropagatesErrors(t *testing.T) {
	scenario := testutil.NewScenario().
		FatalError("agent failed").
		Build()

	agent := testutil.NewTestAgent(t, "mock", scenario)
	t.Cleanup(func() { agent.AssertAllConsumed(t) })

	ctx := context.Background()
	adapter := NewAgentAdapter(agent, ctx, "/workdir")

	result, err := adapter.RunCustomPrompt("test prompt")

	// TestAgent returns errors via result, not error return
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.IsError)
	require.Equal(t, agents.ErrorClassFatal, result.ErrorClass)
}
