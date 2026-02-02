package orbithelpers

import (
	"testing"

	"github.com/arjenschwarz/orbit/internal/agents"
	"github.com/arjenschwarz/orbit/internal/testutil"
)

func TestCreateTestOrbit(t *testing.T) {
	// Note: CreateTestOrbit creates a real Orbit instance which requires
	// the full orbit package initialization. This test verifies the wiring works.
	t.Run("with agent", func(t *testing.T) {
		scenario := testutil.NewScenario().Success("session-1", 0.05).Build()
		agent := testutil.NewTestAgent(t, "test-agent", scenario)

		// Creating should not panic or error
		orbit := CreateTestOrbit(t, WithAgent(agent))
		if orbit == nil {
			t.Fatal("CreateTestOrbit: returned nil")
		}
	})

	t.Run("with custom tasks file", func(t *testing.T) {
		scenario := testutil.NewScenario().Success("session-1", 0.05).Build()
		agent := testutil.NewTestAgent(t, "test-agent", scenario)
		tasksFile := testutil.CreateTasksFile(t, 3)

		orbit := CreateTestOrbit(t,
			WithAgent(agent),
			WithTasksFile(tasksFile),
		)
		if orbit == nil {
			t.Fatal("CreateTestOrbit: returned nil")
		}
	})

	t.Run("with multiple agents", func(t *testing.T) {
		scenario1 := testutil.NewScenario().Success("session-1", 0.05).Build()
		scenario2 := testutil.NewScenario().Success("session-2", 0.05).Build()

		// When using WithAgents, one of the agents must be named to match
		// the default or you must also use WithAgent to set the primary agent
		agentMap := map[string]agents.Agent{
			"agent-1": testutil.NewTestAgent(t, "agent-1", scenario1),
			"agent-2": testutil.NewTestAgent(t, "agent-2", scenario2),
		}

		// Use the first agent as the primary one via WithAgent
		primaryAgent := testutil.NewTestAgent(t, "agent-1", scenario1)
		orbit := CreateTestOrbit(t,
			WithAgent(primaryAgent),
			WithAgents(agentMap),
		)
		if orbit == nil {
			t.Fatal("CreateTestOrbit: returned nil")
		}
	})
}
