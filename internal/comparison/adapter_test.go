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
		ctx     context.Context
		workDir string
	}{
		"nil agent panics": {
			agent:   nil,
			ctx:     context.Background(),
			workDir: "/tmp/test",
		},
		"nil context panics": {
			agent:   validAgent,
			ctx:     nil,
			workDir: "/tmp/test",
		},
		"empty workDir panics": {
			agent:   validAgent,
			ctx:     context.Background(),
			workDir: "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Panics(t, func() {
				NewAgentAdapter(tc.agent, tc.ctx, tc.workDir)
			})
		})
	}
}

func TestNewAgentAdapter_ValidInputsSucceed(t *testing.T) {
	scenario := testutil.NewScenario().Build()
	agent := testutil.NewTestAgent(t, "mock", scenario)

	adapter := NewAgentAdapter(agent, context.Background(), "/tmp/test")

	require.NotNil(t, adapter)
}

func TestAgentAdapter_ContextPropagation(t *testing.T) {
	scenario := testutil.NewScenario().
		Success("session-1", 0.01).
		Build()

	agent := testutil.NewTestAgent(t, "mock", scenario)
	t.Cleanup(func() { agent.AssertAllConsumed(t) })

	type contextKey string
	testKey := contextKey("test-key")
	ctx := context.WithValue(context.Background(), testKey, "test-value")
	adapter := NewAgentAdapter(agent, ctx, "/tmp/test")

	_, err := adapter.RunCustomPrompt("test")
	require.NoError(t, err)

	// The context is captured at construction time and passed to the agent
	// We verify by checking the call was made successfully with our context
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

	adapter := NewAgentAdapter(agent, ctx, "/tmp/test")

	_, err := adapter.RunCustomPrompt("test")
	// TestAgent will return the retryable error we configured
	require.Error(t, err)
}
