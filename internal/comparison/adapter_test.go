package comparison

import (
	"context"
	"testing"

	"github.com/arjenschwarz/orbit/internal/agents"
	"github.com/arjenschwarz/orbit/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentAdapter_RunCustomPrompt(t *testing.T) {
	scenario := testutil.NewScenario().
		Success("test-session", 0.05).
		Build()

	agent := testutil.NewTestAgent(t, "mock", scenario)
	t.Cleanup(func() { agent.AssertAllConsumed(t) })

	workDir := "/test/workdir"
	adapter := NewAgentAdapter(agent, workDir)

	ctx := context.Background()
	result, err := adapter.RunCustomPrompt(ctx, "test prompt")

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

	workDir := "/different/path/to/project"
	adapter := NewAgentAdapter(agent, workDir)

	_, err := adapter.RunCustomPrompt(context.Background(), "any prompt")
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

	adapter := NewAgentAdapter(agent, "/workdir")

	result, err := adapter.RunCustomPrompt(context.Background(), "test prompt")

	// TestAgent returns errors like real agents do when IsError is true
	require.Error(t, err)
	require.NotNil(t, result)
	require.True(t, result.IsError)
	require.Equal(t, agents.ErrorClassFatal, result.ErrorClass)
}

func TestNewAgentAdapter_PanicsOnInvalidInputs(t *testing.T) {
	scenario := testutil.NewScenario().Build()
	validAgent := testutil.NewTestAgent(t, "mock", scenario)

	tests := map[string]struct {
		agent   agents.Agent
		workDir string
	}{
		"nil agent panics": {
			agent:   nil,
			workDir: "/tmp/test",
		},
		"empty workDir panics": {
			agent:   validAgent,
			workDir: "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Panics(t, func() {
				NewAgentAdapter(tc.agent, tc.workDir)
			})
		})
	}
}

func TestNewAgentAdapter_ValidInputsSucceed(t *testing.T) {
	scenario := testutil.NewScenario().Build()
	agent := testutil.NewTestAgent(t, "mock", scenario)

	adapter := NewAgentAdapter(agent, "/tmp/test")

	require.NotNil(t, adapter)
}

func TestAgentAdapter_ContextPropagation(t *testing.T) {
	scenario := testutil.NewScenario().
		Success("session-1", 0.01).
		Build()

	agent := testutil.NewTestAgent(t, "mock", scenario)
	t.Cleanup(func() { agent.AssertAllConsumed(t) })

	adapter := NewAgentAdapter(agent, "/tmp/test")

	type contextKey string
	testKey := contextKey("test-key")
	ctx := context.WithValue(context.Background(), testKey, "test-value")

	_, err := adapter.RunCustomPrompt(ctx, "test")
	require.NoError(t, err)

	// Verify the call was made successfully with the provided context
	agent.Recorder().AssertCallCount(t, 1)
}

func TestAgentAdapter_CancelledContext(t *testing.T) {
	scenario := testutil.NewScenario().
		RetryableError("context canceled").
		Build()

	agent := testutil.NewTestAgent(t, "mock", scenario)
	t.Cleanup(func() { agent.AssertAllConsumed(t) })

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	adapter := NewAgentAdapter(agent, "/tmp/test")

	_, err := adapter.RunCustomPrompt(ctx, "test")
	// TestAgent will return the retryable error we configured
	require.Error(t, err)
}

func TestAgentAdapter_WithExtraArgs(t *testing.T) {
	scenario := testutil.NewScenario().
		Success("session-1", 0.01).
		Build()

	agent := testutil.NewTestAgent(t, "mock", scenario)
	t.Cleanup(func() { agent.AssertAllConsumed(t) })

	adapter := NewAgentAdapter(agent, "/tmp/test").
		WithExtraArgs("--tools", "")

	_, err := adapter.RunCustomPrompt(context.Background(), "test prompt")
	require.NoError(t, err)

	calls := agent.Recorder().Calls()
	require.Len(t, calls, 1)
	assert.Equal(t, []string{"--tools", ""}, calls[0].Options.ExtraArgs)
}

func TestAgentAdapter_WithExtraArgs_DoesNotMutateOriginal(t *testing.T) {
	scenario := testutil.NewScenario().
		Success("session-1", 0.01).
		Success("session-2", 0.01).
		Build()

	agent := testutil.NewTestAgent(t, "mock", scenario)
	t.Cleanup(func() { agent.AssertAllConsumed(t) })

	original := NewAgentAdapter(agent, "/tmp/test")
	withArgs := original.WithExtraArgs("--tools", "")

	// Call original - should have no extra args
	_, err := original.RunCustomPrompt(context.Background(), "prompt1")
	require.NoError(t, err)

	calls := agent.Recorder().Calls()
	require.Len(t, calls, 1)
	assert.Empty(t, calls[0].Options.ExtraArgs)

	// Call copy - should have extra args
	_, err = withArgs.RunCustomPrompt(context.Background(), "prompt2")
	require.NoError(t, err)

	calls = agent.Recorder().Calls()
	require.Len(t, calls, 2)
	assert.Equal(t, []string{"--tools", ""}, calls[1].Options.ExtraArgs)
}
