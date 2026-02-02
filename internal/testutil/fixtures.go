package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arjenschwarz/orbit/internal/agents"
	"github.com/arjenschwarz/orbit/internal/orbit"
	"github.com/arjenschwarz/orbit/internal/rune"
)

// TestAgentResolver implements orbit.AgentResolver for tests.
// It allows injecting test agents without modifying the global registry.
type TestAgentResolver struct {
	agents map[string]agents.Agent
}

// NewTestAgentResolver creates a new test agent resolver.
func NewTestAgentResolver() *TestAgentResolver {
	return &TestAgentResolver{agents: make(map[string]agents.Agent)}
}

// Add registers an agent with the resolver.
func (r *TestAgentResolver) Add(name string, agent agents.Agent) {
	r.agents[name] = agent
}

// GetAgent returns an agent by name.
func (r *TestAgentResolver) GetAgent(name string, _ agents.AgentConfig) (agents.Agent, error) {
	if agent, ok := r.agents[name]; ok {
		return agent, nil
	}
	return nil, fmt.Errorf("test agent %q not registered", name)
}

// CreateTasksFile creates a minimal tasks.md with N phases in a temp directory.
// Returns the path to the created file.
func CreateTasksFile(t testing.TB, phases int) string {
	t.Helper()

	dir := t.TempDir()
	tasksFile := filepath.Join(dir, "tasks.md")

	var sb strings.Builder
	sb.WriteString("# Test Tasks\n\n")
	for i := range phases {
		phaseNum := i + 1
		sb.WriteString(fmt.Sprintf("## Phase %d: Test Phase %d\n\n", phaseNum, phaseNum))
		sb.WriteString(fmt.Sprintf("- [ ] %d.1 Task %d.1\n", phaseNum, phaseNum))
		sb.WriteString("\n")
	}

	if err := os.WriteFile(tasksFile, []byte(sb.String()), 0644); err != nil {
		t.Fatalf("CreateTasksFile: failed to write tasks file: %v", err)
	}

	return tasksFile
}

// ConfigOptions configures CreateConfig.
type ConfigOptions struct {
	Agent       string
	PrePrompt   string
	PostPrompt  string
	AutoApprove bool
}

// CreateConfig creates an .orbit.yaml with the specified options.
// Returns the path to the created file.
func CreateConfig(t testing.TB, opts ConfigOptions) string {
	t.Helper()

	dir := t.TempDir()
	configFile := filepath.Join(dir, ".orbit.yaml")

	// Build minimal YAML config
	content := ""

	if opts.Agent != "" {
		content += fmt.Sprintf("agent: %s\n", opts.Agent)
	}
	if opts.PrePrompt != "" {
		content += fmt.Sprintf("pre-prompt: %q\n", opts.PrePrompt)
	}
	if opts.PostPrompt != "" {
		content += fmt.Sprintf("post-prompt: %q\n", opts.PostPrompt)
	}
	if opts.AutoApprove {
		content += "agents:\n  claude-code:\n    auto-approve: true\n"
	}

	if content == "" {
		content = "# Empty test config\n"
	}

	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("CreateConfig: failed to write config file: %v", err)
	}

	return configFile
}

// orbitConfig holds configuration for CreateTestOrbit.
type orbitConfig struct {
	resolver   *TestAgentResolver
	clock      Clock
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
func WithOrbitClock(clock Clock) OrbitOption {
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
		resolver: NewTestAgentResolver(),
		clock:    RealClock{},
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
		tasksFile = CreateTasksFile(t, 1)
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

// CreateRuneClient creates a rune client for the specified tasks file.
// Note: This creates a real rune.Client that invokes the rune CLI.
// For tests that need full isolation, consider using mock responses via TestAgent.
func CreateRuneClient(t testing.TB, tasksFile string) *rune.Client {
	t.Helper()
	return rune.NewClient(tasksFile)
}

// SuccessScenario creates a scenario with N successful responses.
// This is a convenience helper for the common case of all-success scenarios.
func SuccessScenario(t testing.TB, phases int) *Scenario {
	t.Helper()

	builder := NewScenario()
	for i := range phases {
		sessionID := fmt.Sprintf("session-%d", i+1)
		builder.Success(sessionID, 0.05)
	}
	return builder.Build()
}
