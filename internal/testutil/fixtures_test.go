package testutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arjenschwarz/orbit/internal/agents"
)

func TestCreateTasksFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		phases int
	}{
		{"single phase", 1},
		{"two phases", 2},
		{"five phases", 5},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tasksFile := CreateTasksFile(t, tc.phases)

			// Verify file exists
			if _, err := os.Stat(tasksFile); os.IsNotExist(err) {
				t.Fatalf("CreateTasksFile: file was not created at %s", tasksFile)
			}

			// Verify file has expected filename
			if filepath.Base(tasksFile) != "tasks.md" {
				t.Errorf("CreateTasksFile: expected filename tasks.md, got %s", filepath.Base(tasksFile))
			}

			// Read and verify content
			content, err := os.ReadFile(tasksFile)
			if err != nil {
				t.Fatalf("CreateTasksFile: failed to read file: %v", err)
			}

			// Verify header is present
			if !strings.Contains(string(content), "# Test Tasks") {
				t.Error("CreateTasksFile: missing header")
			}

			// Verify correct number of phases
			for i := 1; i <= tc.phases; i++ {
				phaseHeader := "## Phase " + string(rune('0'+i))
				if !strings.Contains(string(content), phaseHeader) {
					t.Errorf("CreateTasksFile: missing phase %d header", i)
				}
			}

			// Verify no extra phases exist
			extraPhase := "## Phase " + string(rune('0'+tc.phases+1))
			if strings.Contains(string(content), extraPhase) {
				t.Errorf("CreateTasksFile: found unexpected phase %d", tc.phases+1)
			}
		})
	}
}

func TestCreateConfig(t *testing.T) {
	t.Parallel()

	t.Run("empty config", func(t *testing.T) {
		t.Parallel()

		configFile := CreateConfig(t, ConfigOptions{})

		// Verify file exists
		if _, err := os.Stat(configFile); os.IsNotExist(err) {
			t.Fatalf("CreateConfig: file was not created at %s", configFile)
		}

		// Verify file has expected filename
		if filepath.Base(configFile) != ".orbit.yaml" {
			t.Errorf("CreateConfig: expected filename .orbit.yaml, got %s", filepath.Base(configFile))
		}
	})

	t.Run("with agent", func(t *testing.T) {
		t.Parallel()

		configFile := CreateConfig(t, ConfigOptions{Agent: "test-agent"})

		content, err := os.ReadFile(configFile)
		if err != nil {
			t.Fatalf("CreateConfig: failed to read file: %v", err)
		}

		if !strings.Contains(string(content), "agent: test-agent") {
			t.Error("CreateConfig: missing agent configuration")
		}
	})

	t.Run("with prompts", func(t *testing.T) {
		t.Parallel()

		configFile := CreateConfig(t, ConfigOptions{
			PrePrompt:  "Setup environment",
			PostPrompt: "Run tests",
		})

		content, err := os.ReadFile(configFile)
		if err != nil {
			t.Fatalf("CreateConfig: failed to read file: %v", err)
		}

		if !strings.Contains(string(content), "pre-prompt:") {
			t.Error("CreateConfig: missing pre-prompt configuration")
		}
		if !strings.Contains(string(content), "post-prompt:") {
			t.Error("CreateConfig: missing post-prompt configuration")
		}
	})
}

func TestTestAgentResolver(t *testing.T) {
	t.Parallel()

	t.Run("returns registered agent", func(t *testing.T) {
		t.Parallel()

		resolver := NewTestAgentResolver()
		scenario := NewScenario().Success("session-1", 0.05).Build()
		agent := NewTestAgent(t, "test-agent", scenario)

		resolver.Add("test-agent", agent)

		resolved, err := resolver.GetAgent("test-agent", agents.AgentConfig{})
		if err != nil {
			t.Fatalf("GetAgent: unexpected error: %v", err)
		}

		if resolved.Name() != "test-agent" {
			t.Errorf("GetAgent: expected name test-agent, got %s", resolved.Name())
		}
	})

	t.Run("errors on missing agent", func(t *testing.T) {
		t.Parallel()

		resolver := NewTestAgentResolver()

		_, err := resolver.GetAgent("missing-agent", agents.AgentConfig{})
		if err == nil {
			t.Fatal("GetAgent: expected error for missing agent")
		}

		if !strings.Contains(err.Error(), "not registered") {
			t.Errorf("GetAgent: expected 'not registered' error, got: %v", err)
		}
	})
}

// Note: TestCreateTestOrbit is in internal/testutil/orbithelpers/helpers_test.go
// to avoid import cycles. The CreateTestOrbit function depends on the orbit package.

func TestSuccessScenario(t *testing.T) {
	t.Parallel()

	t.Run("creates correct number of responses", func(t *testing.T) {
		t.Parallel()

		scenario := SuccessScenario(t, 5)

		if scenario.Len() != 5 {
			t.Errorf("SuccessScenario: expected 5 responses, got %d", scenario.Len())
		}
	})

	t.Run("all responses are successes", func(t *testing.T) {
		t.Parallel()

		scenario := SuccessScenario(t, 3)

		for i, resp := range scenario.Responses() {
			if resp.Result == nil {
				t.Errorf("SuccessScenario: response %d has nil result", i)
				continue
			}
			if resp.Result.IsError {
				t.Errorf("SuccessScenario: response %d is an error, expected success", i)
			}
			if resp.Result.ExitCode != 0 {
				t.Errorf("SuccessScenario: response %d has exit code %d, expected 0", i, resp.Result.ExitCode)
			}
		}
	})

	t.Run("session IDs are unique", func(t *testing.T) {
		t.Parallel()

		scenario := SuccessScenario(t, 4)

		seen := make(map[string]bool)
		for i, resp := range scenario.Responses() {
			if resp.Result == nil {
				t.Errorf("SuccessScenario: response %d has nil result", i)
				continue
			}
			if seen[resp.Result.SessionID] {
				t.Errorf("SuccessScenario: duplicate session ID %q at response %d", resp.Result.SessionID, i)
			}
			seen[resp.Result.SessionID] = true
		}
	})
}
