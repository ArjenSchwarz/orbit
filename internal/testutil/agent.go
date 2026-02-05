package testutil

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/arjenschwarz/orbit/internal/agents"
)

// TestAgentConfig holds identity configuration for a TestAgent.
type TestAgentConfig struct {
	Name       string
	CLICommand string
	Installed  bool
	Version    string
	SessionDir string
}

// TestAgent is a mock agent for integration testing.
// It returns pre-configured responses based on a scenario.
type TestAgent struct {
	t        testing.TB
	name     string
	scenario *Scenario
	recorder *Recorder
	clock    Clock
	config   TestAgentConfig

	mu         sync.Mutex
	callIndex  int
	exportPath string // if set, implements SessionExporter interface
}

// Verify TestAgent implements agents.Agent interface.
var _ agents.Agent = (*TestAgent)(nil)

// NewTestAgent creates a test agent with the given scenario.
func NewTestAgent(t testing.TB, name string, scenario *Scenario, opts ...TestAgentOption) *TestAgent {
	t.Helper()
	agent := &TestAgent{
		t:        t,
		name:     name,
		scenario: scenario,
		recorder: NewRecorder(),
		clock:    RealClock{},
		config: TestAgentConfig{
			Name:       name,
			CLICommand: name,
			Installed:  true,
			Version:    "test-1.0.0",
			SessionDir: "",
		},
	}

	for _, opt := range opts {
		opt(agent)
	}

	return agent
}

// Name returns the agent name.
func (a *TestAgent) Name() string {
	return a.config.Name
}

// CLICommand returns the CLI command for this agent.
func (a *TestAgent) CLICommand() string {
	return a.config.CLICommand
}

// IsInstalled returns whether the agent CLI is installed.
func (a *TestAgent) IsInstalled() bool {
	return a.config.Installed
}

// Version returns the agent version.
func (a *TestAgent) Version() (string, error) {
	return a.config.Version, nil
}

// DefaultSessionDir returns the default session directory.
func (a *TestAgent) DefaultSessionDir() string {
	return a.config.SessionDir
}

// DiscoverSessions returns an empty list (test agents don't have real sessions).
func (a *TestAgent) DiscoverSessions(ctx context.Context, projectDir string) ([]agents.SessionInfo, error) {
	return nil, nil
}

// Run executes the agent with the given options and returns the next scenario response.
func (a *TestAgent) Run(ctx context.Context, opts agents.RunOptions) (*agents.RunResult, error) {
	a.mu.Lock()
	index := a.callIndex
	a.callIndex++
	a.mu.Unlock()

	// Extract context values (don't store the context itself)
	deadline, hasDeadline := ctx.Deadline()

	// Record the call
	call := AgentCall{
		Index:       index,
		Method:      "Run",
		Options:     opts,
		Timestamp:   a.clock.Now(),
		HasDeadline: hasDeadline,
		Deadline:    deadline,
	}
	a.recorder.record(call)

	// Check bounds
	if index >= a.scenario.Len() {
		a.t.Fatalf("TestAgent %q: unexpected call at index %d (scenario has %d responses)\n"+
			"  Prompt: %q\n"+
			"  Previous calls:\n%s",
			a.name, index, a.scenario.Len(), opts.Prompt, a.recorder.formatCalls())
	}

	// Get response and handle custom function
	resp := a.scenario.responses[index]
	if resp.CustomFunc != nil {
		customResp := resp.CustomFunc(&call)
		if customResp != nil {
			resp = *customResp
		}
	}

	// Apply delay via clock
	if resp.Delay > 0 {
		a.clock.Sleep(resp.Delay)
	}

	// Build result, applying output/stderr if specified
	result := resp.Result
	if result == nil {
		result = &agents.RunResult{}
	}

	// Apply output/stderr from response (overwrites if set)
	if resp.Output != "" {
		result.Output = resp.Output
	}
	if resp.Stderr != "" {
		result.Stderr = resp.Stderr
	}

	// Set error class on result
	if resp.ErrorClass != agents.ErrorClassUnknown {
		result.ErrorClass = resp.ErrorClass
	}

	// Return an error if the result indicates an error (matches real agent behavior)
	if result.IsError {
		return result, fmt.Errorf("agent error: exit code %d", result.ExitCode)
	}

	return result, nil
}

// Resume continues an existing session with the given options.
func (a *TestAgent) Resume(ctx context.Context, sessionID string, opts agents.RunOptions) (*agents.RunResult, error) {
	a.mu.Lock()
	index := a.callIndex
	a.callIndex++
	a.mu.Unlock()

	// Extract context values (don't store the context itself)
	deadline, hasDeadline := ctx.Deadline()

	// Record the call
	call := AgentCall{
		Index:       index,
		Method:      "Resume",
		Options:     opts,
		SessionID:   sessionID,
		Timestamp:   a.clock.Now(),
		HasDeadline: hasDeadline,
		Deadline:    deadline,
	}
	a.recorder.record(call)

	// Check bounds
	if index >= a.scenario.Len() {
		a.t.Fatalf("TestAgent %q: unexpected call at index %d (scenario has %d responses)\n"+
			"  SessionID: %q\n"+
			"  Prompt: %q\n"+
			"  Previous calls:\n%s",
			a.name, index, a.scenario.Len(), sessionID, opts.Prompt, a.recorder.formatCalls())
	}

	// Get response and handle custom function
	resp := a.scenario.responses[index]
	if resp.CustomFunc != nil {
		customResp := resp.CustomFunc(&call)
		if customResp != nil {
			resp = *customResp
		}
	}

	// Apply delay via clock
	if resp.Delay > 0 {
		a.clock.Sleep(resp.Delay)
	}

	// Build result, applying output/stderr if specified
	result := resp.Result
	if result == nil {
		result = &agents.RunResult{}
	}

	// Apply output/stderr from response (overwrites if set)
	if resp.Output != "" {
		result.Output = resp.Output
	}
	if resp.Stderr != "" {
		result.Stderr = resp.Stderr
	}

	// Set error class on result
	if resp.ErrorClass != agents.ErrorClassUnknown {
		result.ErrorClass = resp.ErrorClass
	}

	// Return an error if the result indicates an error (matches real agent behavior)
	if result.IsError {
		return result, fmt.Errorf("agent error: exit code %d", result.ExitCode)
	}

	return result, nil
}

// Recorder returns the recorder for this agent.
func (a *TestAgent) Recorder() *Recorder {
	return a.recorder
}

// AssertAllConsumed verifies that all scenario responses were consumed.
// Call this at the end of a test or via t.Cleanup() to ensure the expected
// number of calls were made.
func (a *TestAgent) AssertAllConsumed(t testing.TB) {
	t.Helper()
	a.mu.Lock()
	consumed := a.callIndex
	total := a.scenario.Len()
	a.mu.Unlock()

	if consumed < total {
		t.Fatalf("TestAgent %q: %d unconsumed scenario responses (consumed %d of %d)\n"+
			"Calls made:\n%s",
			a.name,
			total-consumed,
			consumed,
			total,
			a.recorder.formatCalls())
	}
}

// TestAgentOption configures a TestAgent.
type TestAgentOption func(*TestAgent)

// WithClock sets a custom clock for the TestAgent.
// Use with FakeClock for deterministic timing tests.
func WithClock(clock Clock) TestAgentOption {
	return func(a *TestAgent) {
		a.clock = clock
	}
}

// WithConfig sets the agent identity configuration.
func WithConfig(config TestAgentConfig) TestAgentOption {
	return func(a *TestAgent) {
		a.config = config
	}
}

// WithSessionExport enables SessionExporter interface support.
// When enabled, ExportSession() can be called to simulate session export.
func WithSessionExport(path string) TestAgentOption {
	return func(a *TestAgent) {
		a.exportPath = path
	}
}

// ExportSession implements agents.SessionExporter for test agents with export enabled.
func (a *TestAgent) ExportSession(ctx context.Context, filename string) error {
	if a.exportPath == "" {
		a.t.Fatalf("TestAgent %q: ExportSession called but WithSessionExport not configured", a.name)
	}
	// In tests, we don't actually write files - just return success
	return nil
}

// HasSessionExport returns true if the agent has session export enabled.
func (a *TestAgent) HasSessionExport() bool {
	return a.exportPath != ""
}

// Verify testAgentExporter implements agents.SessionExporter interface.
var _ agents.SessionExporter = (*TestAgent)(nil)
