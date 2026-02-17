package agents

import (
	"context"
	"testing"
	"time"
)

// mockAgent is a test implementation of the Agent interface.
type mockAgent struct {
	name       string
	cliCommand string
	installed  bool
}

func (m *mockAgent) Name() string              { return m.name }
func (m *mockAgent) CLICommand() string        { return m.cliCommand }
func (m *mockAgent) IsInstalled() bool         { return m.installed }
func (m *mockAgent) Version() (string, error)  { return "1.0.0", nil }
func (m *mockAgent) DefaultSessionDir() string { return "/tmp/sessions" }
func (m *mockAgent) DiscoverSessions(ctx context.Context, projectDir string) ([]SessionInfo, error) {
	return nil, nil
}
func (m *mockAgent) Run(ctx context.Context, opts RunOptions) (*RunResult, error) {
	return &RunResult{SessionID: "test-session"}, nil
}
func (m *mockAgent) Resume(ctx context.Context, sessionID string, opts RunOptions) (*RunResult, error) {
	return &RunResult{SessionID: sessionID}, nil
}

func TestRegister(t *testing.T) {
	// Clear registry before test
	clearRegistry()

	factory := func(cfg AgentConfig) Agent {
		return &mockAgent{
			name:       "test-agent",
			cliCommand: "test",
			installed:  true,
		}
	}

	Register("test-agent", factory)

	// Verify agent can be retrieved
	agent, err := Get("test-agent", AgentConfig{})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if agent == nil {
		t.Fatal("Get() returned nil agent")
	}
	if agent.Name() != "test-agent" {
		t.Errorf("agent.Name() = %q, want %q", agent.Name(), "test-agent")
	}
}

func TestGet_UnknownAgent(t *testing.T) {
	clearRegistry()

	_, err := Get("nonexistent-agent", AgentConfig{})
	if err == nil {
		t.Error("Get() expected error for unknown agent, got nil")
	}
}

func TestGet_WithConfig(t *testing.T) {
	clearRegistry()

	var receivedConfig AgentConfig
	factory := func(cfg AgentConfig) Agent {
		receivedConfig = cfg
		return &mockAgent{
			name:       "configured-agent",
			cliCommand: cfg.CLIPath,
			installed:  true,
		}
	}

	Register("configured-agent", factory)

	cfg := AgentConfig{
		CLIPath:     "/custom/path/agent",
		AutoApprove: true,
		ExtraArgs:   []string{"--verbose"},
		Timeout:     30 * time.Minute,
		Options:     map[string]string{"model": "gpt-4"},
	}

	agent, err := Get("configured-agent", cfg)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	// Verify config was passed to factory
	if receivedConfig.CLIPath != cfg.CLIPath {
		t.Errorf("factory received CLIPath = %q, want %q", receivedConfig.CLIPath, cfg.CLIPath)
	}
	if receivedConfig.AutoApprove != cfg.AutoApprove {
		t.Errorf("factory received AutoApprove = %v, want %v", receivedConfig.AutoApprove, cfg.AutoApprove)
	}
	if len(receivedConfig.ExtraArgs) != len(cfg.ExtraArgs) {
		t.Errorf("factory received ExtraArgs = %v, want %v", receivedConfig.ExtraArgs, cfg.ExtraArgs)
	}
	if receivedConfig.Timeout != cfg.Timeout {
		t.Errorf("factory received Timeout = %v, want %v", receivedConfig.Timeout, cfg.Timeout)
	}

	// Verify agent uses config
	if agent.CLICommand() != "/custom/path/agent" {
		t.Errorf("agent.CLICommand() = %q, want %q", agent.CLICommand(), "/custom/path/agent")
	}
}

func TestList(t *testing.T) {
	clearRegistry()

	// Register multiple agents
	Register("agent-a", func(cfg AgentConfig) Agent {
		return &mockAgent{name: "agent-a"}
	})
	Register("agent-b", func(cfg AgentConfig) Agent {
		return &mockAgent{name: "agent-b"}
	})
	Register("agent-c", func(cfg AgentConfig) Agent {
		return &mockAgent{name: "agent-c"}
	})

	list := List()

	// Should be sorted alphabetically
	if len(list) != 3 {
		t.Fatalf("List() returned %d agents, want 3", len(list))
	}
	if list[0] != "agent-a" {
		t.Errorf("List()[0] = %q, want %q", list[0], "agent-a")
	}
	if list[1] != "agent-b" {
		t.Errorf("List()[1] = %q, want %q", list[1], "agent-b")
	}
	if list[2] != "agent-c" {
		t.Errorf("List()[2] = %q, want %q", list[2], "agent-c")
	}
}

func TestList_Empty(t *testing.T) {
	clearRegistry()

	list := List()
	if len(list) != 0 {
		t.Errorf("List() returned %d agents, want 0", len(list))
	}
}

func TestDefault(t *testing.T) {
	defaultAgent := Default()
	if defaultAgent != "claude-code" {
		t.Errorf("Default() = %q, want %q", defaultAgent, "claude-code")
	}
}

func TestRegister_Overwrite(t *testing.T) {
	clearRegistry()

	// Register first version
	Register("overwrite-agent", func(cfg AgentConfig) Agent {
		return &mockAgent{name: "version1", cliCommand: "v1"}
	})

	// Register second version with same name
	Register("overwrite-agent", func(cfg AgentConfig) Agent {
		return &mockAgent{name: "version2", cliCommand: "v2"}
	})

	agent, err := Get("overwrite-agent", AgentConfig{})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	// Should get the second version
	if agent.CLICommand() != "v2" {
		t.Errorf("agent.CLICommand() = %q, want %q (overwritten)", agent.CLICommand(), "v2")
	}
}

func TestAgentConfig_Fields(t *testing.T) {
	cfg := AgentConfig{
		CLIPath:     "/usr/local/bin/claude",
		AutoApprove: true,
		ExtraArgs:   []string{"--debug", "--verbose"},
		Timeout:     1 * time.Hour,
		Options: map[string]string{
			"model":  "claude-3-opus",
			"format": "json",
		},
	}

	if cfg.CLIPath != "/usr/local/bin/claude" {
		t.Errorf("CLIPath = %q, want %q", cfg.CLIPath, "/usr/local/bin/claude")
	}
	if !cfg.AutoApprove {
		t.Error("AutoApprove = false, want true")
	}
	if len(cfg.ExtraArgs) != 2 {
		t.Errorf("len(ExtraArgs) = %d, want 2", len(cfg.ExtraArgs))
	}
	if cfg.Timeout != 1*time.Hour {
		t.Errorf("Timeout = %v, want %v", cfg.Timeout, 1*time.Hour)
	}
	if cfg.Options["model"] != "claude-3-opus" {
		t.Errorf("Options[model] = %q, want %q", cfg.Options["model"], "claude-3-opus")
	}
}

// clearRegistry resets the global registry for testing.
func clearRegistry() {
	mu.Lock()
	defer mu.Unlock()
	registry = make(map[string]func(AgentConfig) Agent)
}
