// Package orbithelpers provides test helpers that depend on the orbit package.
// This package is separated from testutil to avoid import cycles when
// testutil is used from within the orbit package.
package orbithelpers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/arjenschwarz/orbit/internal/agents"
	"github.com/arjenschwarz/orbit/internal/orbit"
	"github.com/arjenschwarz/orbit/internal/rune"
	"github.com/arjenschwarz/orbit/internal/testutil"
)

// orbitConfig holds configuration for CreateTestOrbit.
type orbitConfig struct {
	resolver   *testutil.TestAgentResolver
	clock      testutil.Clock
	agent      agents.Agent
	agents     map[string]agents.Agent
	runeClient *rune.Client
	tasksFile  string
	logDir     string
	prePrompt  string
	postPrompt string
}

// OrbitOption configures CreateTestOrbit.
type OrbitOption func(*orbitConfig)

// WithAgent sets a single agent for the Orbit instance.
func WithAgent(agent agents.Agent) OrbitOption {
	return func(cfg *orbitConfig) {
		cfg.agent = agent
	}
}

// WithAgents sets multiple agents for multi-variant testing.
func WithAgents(agents map[string]agents.Agent) OrbitOption {
	return func(cfg *orbitConfig) {
		cfg.agents = agents
	}
}

// WithRuneClient sets a mock rune client.
func WithRuneClient(client *rune.Client) OrbitOption {
	return func(cfg *orbitConfig) {
		cfg.runeClient = client
	}
}

// WithOrbitClock sets a custom clock for the Orbit instance.
// Note: This is named WithOrbitClock to avoid conflict with WithClock for TestAgent.
func WithOrbitClock(clock testutil.Clock) OrbitOption {
	return func(cfg *orbitConfig) {
		cfg.clock = clock
	}
}

// WithTasksFile sets the tasks file path.
func WithTasksFile(path string) OrbitOption {
	return func(cfg *orbitConfig) {
		cfg.tasksFile = path
	}
}

// WithPrePrompt sets the pre-prompt for the Orbit instance.
func WithPrePrompt(prompt string) OrbitOption {
	return func(cfg *orbitConfig) {
		cfg.prePrompt = prompt
	}
}

// WithPostPrompt sets the post-prompt for the Orbit instance.
func WithPostPrompt(prompt string) OrbitOption {
	return func(cfg *orbitConfig) {
		cfg.postPrompt = prompt
	}
}

// CreateTestOrbit creates a configured Orbit instance for testing.
// It wires up test agents via dependency injection instead of modifying global state.
func CreateTestOrbit(t testing.TB, opts ...OrbitOption) *orbit.Orbit {
	t.Helper()

	cfg := &orbitConfig{
		resolver: testutil.NewTestAgentResolver(),
		clock:    testutil.RealClock{},
	}

	for _, opt := range opts {
		opt(cfg)
	}

	// Wire the agent(s) into the resolver
	if cfg.agent != nil {
		cfg.resolver.Add(cfg.agent.Name(), cfg.agent)
	}
	for name, agent := range cfg.agents {
		cfg.resolver.Add(name, agent)
	}

	// Determine agent name from configuration
	agentName := "claude-code" // default
	if cfg.agent != nil {
		agentName = cfg.agent.Name()
	}

	// Create tasks file if not provided
	tasksFile := cfg.tasksFile
	if tasksFile == "" {
		tasksFile = testutil.CreateTasksFile(t, 1)
	}

	// Create log directory if not provided
	logDir := cfg.logDir
	if logDir == "" {
		logDir = filepath.Join(t.TempDir(), ".orbit")
		if err := os.MkdirAll(logDir, 0755); err != nil {
			t.Fatalf("CreateTestOrbit: failed to create log directory: %v", err)
		}
	}

	// Create Orbit with test dependencies
	orbitCfg := orbit.Config{
		TasksFile:     tasksFile,
		LogDir:        logDir,
		Agent:         agentName,
		AgentResolver: cfg.resolver,
		Clock:         cfg.clock,
		PrePrompt:     cfg.prePrompt,
		PostPrompt:    cfg.postPrompt,
		WorkingDir:    filepath.Dir(tasksFile),
	}

	o, err := orbit.New(orbitCfg)
	if err != nil {
		t.Fatalf("CreateTestOrbit: %v", err)
	}
	return o
}
