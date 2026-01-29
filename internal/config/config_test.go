package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	// Import agent packages to trigger their init() registration.
	// This is needed for tests that validate against registered agent types.
	_ "github.com/arjenschwarz/orbit/internal/agents/claudecode"
	_ "github.com/arjenschwarz/orbit/internal/agents/codex"
	_ "github.com/arjenschwarz/orbit/internal/agents/copilot"
	_ "github.com/arjenschwarz/orbit/internal/agents/kiro"
)

func TestLoad_ProjectOnly(t *testing.T) {
	// Create temp directory for project config
	tmpDir := t.TempDir()

	// Write project config
	projectConfig := `command: "custom project command"
post-command: "custom post command"
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(projectConfig), 0644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	cfg := Load(tmpDir)

	if cfg.Command != "custom project command" {
		t.Errorf("expected Command %q, got %q", "custom project command", cfg.Command)
	}
	if cfg.PostCommand != "custom post command" {
		t.Errorf("expected PostCommand %q, got %q", "custom post command", cfg.PostCommand)
	}
	if cfg.IsPostCommandDisabled() {
		t.Error("expected IsPostCommandDisabled() to return false")
	}
}

func TestLoad_HomeOnly(t *testing.T) {
	// Create temp directories
	tmpDir := t.TempDir()
	homeDir := t.TempDir()

	// Set HOME to temp directory (t.Setenv restores original after test)
	t.Setenv("HOME", homeDir)

	// Write home config
	homeConfig := `command: "home command"
post-command: "home post command"
`
	if err := os.WriteFile(filepath.Join(homeDir, ".orbit.yaml"), []byte(homeConfig), 0644); err != nil {
		t.Fatalf("failed to write home config: %v", err)
	}

	cfg := Load(tmpDir)

	if cfg.Command != "home command" {
		t.Errorf("expected Command %q, got %q", "home command", cfg.Command)
	}
	if cfg.PostCommand != "home post command" {
		t.Errorf("expected PostCommand %q, got %q", "home post command", cfg.PostCommand)
	}
}

func TestLoad_MergesBoth(t *testing.T) {
	// Create temp directories
	tmpDir := t.TempDir()
	homeDir := t.TempDir()

	// Set HOME to temp directory (t.Setenv restores original after test)
	t.Setenv("HOME", homeDir)

	// Write home config with both values
	homeConfig := `command: "home command"
post-command: "home post command"
`
	if err := os.WriteFile(filepath.Join(homeDir, ".orbit.yaml"), []byte(homeConfig), 0644); err != nil {
		t.Fatalf("failed to write home config: %v", err)
	}

	// Write project config that overrides only command
	projectConfig := `command: "project command"
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(projectConfig), 0644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	cfg := Load(tmpDir)

	// Project command should override home command
	if cfg.Command != "project command" {
		t.Errorf("expected Command %q, got %q", "project command", cfg.Command)
	}
	// Home post-command should be preserved since project didn't set it
	if cfg.PostCommand != "home post command" {
		t.Errorf("expected PostCommand %q, got %q", "home post command", cfg.PostCommand)
	}
}

func TestLoad_NoFiles(t *testing.T) {
	// Create empty temp directory
	tmpDir := t.TempDir()
	homeDir := t.TempDir()

	// Set HOME to temp directory without config (t.Setenv restores original after test)
	t.Setenv("HOME", homeDir)

	cfg := Load(tmpDir)

	if cfg.Command != DefaultCommand {
		t.Errorf("expected default Command %q, got %q", DefaultCommand, cfg.Command)
	}
	if cfg.PostCommand != DefaultPostCommand {
		t.Errorf("expected default PostCommand %q, got %q", DefaultPostCommand, cfg.PostCommand)
	}
	if cfg.IsPostCommandDisabled() {
		t.Error("expected IsPostCommandDisabled() to return false with defaults")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	// Create temp directories
	tmpDir := t.TempDir()
	homeDir := t.TempDir()

	// Isolate from real home config
	t.Setenv("HOME", homeDir)

	invalidConfig := `command: [this is not valid yaml
post-command: {broken
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(invalidConfig), 0644); err != nil {
		t.Fatalf("failed to write invalid config: %v", err)
	}

	// Should not panic, should return defaults
	cfg := Load(tmpDir)

	// With invalid YAML, defaults should be used
	if cfg.Command != DefaultCommand {
		t.Errorf("expected default Command after invalid YAML, got %q", cfg.Command)
	}
	if cfg.PostCommand != DefaultPostCommand {
		t.Errorf("expected default PostCommand after invalid YAML, got %q", cfg.PostCommand)
	}
}

func TestLoad_EmptyPostCommand(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Write config with explicitly empty post-command
	projectConfig := `command: "custom command"
post-command: ""
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(projectConfig), 0644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	cfg := Load(tmpDir)

	if cfg.Command != "custom command" {
		t.Errorf("expected Command %q, got %q", "custom command", cfg.Command)
	}
	if cfg.PostCommand != "" {
		t.Errorf("expected empty PostCommand, got %q", cfg.PostCommand)
	}
	if !cfg.IsPostCommandDisabled() {
		t.Error("expected IsPostCommandDisabled() to return true when post-command is explicitly empty")
	}
}

func TestLoad_EnvVarOverride(t *testing.T) {
	// Create temp directory with config
	tmpDir := t.TempDir()

	projectConfig := `command: "config command"
post-command: "config post command"
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(projectConfig), 0644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	// Set environment variable to override (t.Setenv restores original after test)
	t.Setenv("ORBIT_COMMAND", "env command")

	cfg := Load(tmpDir)

	// Environment variable should override config file
	if cfg.Command != "env command" {
		t.Errorf("expected Command %q from env var, got %q", "env command", cfg.Command)
	}
	// Post-command should still come from config since not overridden
	if cfg.PostCommand != "config post command" {
		t.Errorf("expected PostCommand %q, got %q", "config post command", cfg.PostCommand)
	}
}

func TestLoad_EnvVarPostCommand(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Set environment variable (t.Setenv restores original after test)
	t.Setenv("ORBIT_POST_COMMAND", "env post command")

	cfg := Load(tmpDir)

	if cfg.PostCommand != "env post command" {
		t.Errorf("expected PostCommand %q from env var, got %q", "env post command", cfg.PostCommand)
	}
}

func TestLoad_EnvVarEmptyPostCommand(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Set environment variable to empty string (explicitly disable)
	// t.Setenv restores original after test
	t.Setenv("ORBIT_POST_COMMAND", "")

	cfg := Load(tmpDir)

	// Empty env var should disable post-command
	if cfg.PostCommand != "" {
		t.Errorf("expected empty PostCommand from env var, got %q", cfg.PostCommand)
	}
	if !cfg.IsPostCommandDisabled() {
		t.Error("expected IsPostCommandDisabled() to return true when env var is explicitly empty")
	}
}

func TestLoad_HomeEmptyPostCommand(t *testing.T) {
	// Create temp directories
	tmpDir := t.TempDir()
	homeDir := t.TempDir()

	// Set HOME to temp directory (t.Setenv restores original after test)
	t.Setenv("HOME", homeDir)

	// Write home config with explicitly empty post-command
	homeConfig := `command: "home command"
post-command: ""
`
	if err := os.WriteFile(filepath.Join(homeDir, ".orbit.yaml"), []byte(homeConfig), 0644); err != nil {
		t.Fatalf("failed to write home config: %v", err)
	}

	cfg := Load(tmpDir)

	if cfg.Command != "home command" {
		t.Errorf("expected Command %q, got %q", "home command", cfg.Command)
	}
	// Home config explicitly disabled post-command
	if cfg.PostCommand != "" {
		t.Errorf("expected empty PostCommand, got %q", cfg.PostCommand)
	}
	if !cfg.IsPostCommandDisabled() {
		t.Error("expected IsPostCommandDisabled() to return true when home config sets empty post-command")
	}
}

func TestLoad_HomeEmptyPostCommand_ProjectOmits(t *testing.T) {
	// Create temp directories
	tmpDir := t.TempDir()
	homeDir := t.TempDir()

	// Set HOME to temp directory (t.Setenv restores original after test)
	t.Setenv("HOME", homeDir)

	// Write home config with explicitly empty post-command
	homeConfig := `post-command: ""
`
	if err := os.WriteFile(filepath.Join(homeDir, ".orbit.yaml"), []byte(homeConfig), 0644); err != nil {
		t.Fatalf("failed to write home config: %v", err)
	}

	// Project config omits post-command
	projectConfig := `command: "project command"
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(projectConfig), 0644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	cfg := Load(tmpDir)

	if cfg.Command != "project command" {
		t.Errorf("expected Command %q, got %q", "project command", cfg.Command)
	}
	// Home config explicitly disabled post-command, project didn't override
	if cfg.PostCommand != "" {
		t.Errorf("expected empty PostCommand (disabled by home config), got %q", cfg.PostCommand)
	}
	if !cfg.IsPostCommandDisabled() {
		t.Error("expected IsPostCommandDisabled() to return true when home config disabled and project omits")
	}
}

func TestLoad_FullPriorityChain(t *testing.T) {
	// This test verifies the complete priority chain:
	// env vars > project config > home config > defaults
	// All sources are set with different values to ensure correct precedence.

	// Create temp directories
	tmpDir := t.TempDir()
	homeDir := t.TempDir()

	// Set HOME to temp directory
	t.Setenv("HOME", homeDir)

	// Write home config (lowest priority among files)
	homeConfig := `command: "home command"
post-command: "home post command"
`
	if err := os.WriteFile(filepath.Join(homeDir, ".orbit.yaml"), []byte(homeConfig), 0644); err != nil {
		t.Fatalf("failed to write home config: %v", err)
	}

	// Write project config (higher priority than home)
	projectConfig := `command: "project command"
post-command: "project post command"
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(projectConfig), 0644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	// Set environment variables (highest priority)
	t.Setenv("ORBIT_COMMAND", "env command")
	t.Setenv("ORBIT_POST_COMMAND", "env post command")

	cfg := Load(tmpDir)

	// Environment variables should win over both config files
	if cfg.Command != "env command" {
		t.Errorf("expected Command %q (from env), got %q", "env command", cfg.Command)
	}
	if cfg.PostCommand != "env post command" {
		t.Errorf("expected PostCommand %q (from env), got %q", "env post command", cfg.PostCommand)
	}
}

func TestLoad_PartialPriorityChain(t *testing.T) {
	// Test that each level properly falls through to the next when not set.
	// Sets: env command only, project post-command only, home has both as fallback.

	tmpDir := t.TempDir()
	homeDir := t.TempDir()

	t.Setenv("HOME", homeDir)

	// Home config provides fallback values
	homeConfig := `command: "home command"
post-command: "home post command"
`
	if err := os.WriteFile(filepath.Join(homeDir, ".orbit.yaml"), []byte(homeConfig), 0644); err != nil {
		t.Fatalf("failed to write home config: %v", err)
	}

	// Project config only sets post-command
	projectConfig := `post-command: "project post command"
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(projectConfig), 0644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	// Env var only sets command
	t.Setenv("ORBIT_COMMAND", "env command")

	cfg := Load(tmpDir)

	// Command: env var wins
	if cfg.Command != "env command" {
		t.Errorf("expected Command %q (from env), got %q", "env command", cfg.Command)
	}
	// PostCommand: project config wins (no env var set)
	if cfg.PostCommand != "project post command" {
		t.Errorf("expected PostCommand %q (from project), got %q", "project post command", cfg.PostCommand)
	}
}

func TestLoad_EnvOverridesAllConfigs(t *testing.T) {
	// Verify that env vars override both home and project configs simultaneously.

	tmpDir := t.TempDir()
	homeDir := t.TempDir()

	t.Setenv("HOME", homeDir)

	// Both configs set values
	homeConfig := `command: "home command"
post-command: "home post command"
`
	if err := os.WriteFile(filepath.Join(homeDir, ".orbit.yaml"), []byte(homeConfig), 0644); err != nil {
		t.Fatalf("failed to write home config: %v", err)
	}

	projectConfig := `command: "project command"
post-command: "project post command"
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(projectConfig), 0644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	// Env vars override everything
	t.Setenv("ORBIT_COMMAND", "env wins")
	t.Setenv("ORBIT_POST_COMMAND", "env also wins")

	cfg := Load(tmpDir)

	if cfg.Command != "env wins" {
		t.Errorf("expected env var to override all configs, got Command %q", cfg.Command)
	}
	if cfg.PostCommand != "env also wins" {
		t.Errorf("expected env var to override all configs, got PostCommand %q", cfg.PostCommand)
	}
}

func TestLoad_EmptyEnvOverridesNonEmptyConfig(t *testing.T) {
	// Critical test: empty env var should override non-empty config values.
	// This validates that os.LookupEnv correctly detects empty strings.

	tmpDir := t.TempDir()

	projectConfig := `command: "project command"
post-command: "project post command"
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(projectConfig), 0644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	// Set env vars to empty strings
	t.Setenv("ORBIT_COMMAND", "")
	t.Setenv("ORBIT_POST_COMMAND", "")

	cfg := Load(tmpDir)

	// Empty env vars should override config file values
	if cfg.Command != "" {
		t.Errorf("expected empty Command from env var, got %q", cfg.Command)
	}
	if cfg.PostCommand != "" {
		t.Errorf("expected empty PostCommand from env var, got %q", cfg.PostCommand)
	}
	if !cfg.IsPostCommandDisabled() {
		t.Error("expected IsPostCommandDisabled() to return true when env var is empty")
	}
}

func TestIsPostCommandDisabled(t *testing.T) {
	tests := []struct {
		name                string
		postCommand         string
		postCommandExplicit bool
		want                bool
	}{
		{
			name:                "not set uses default",
			postCommand:         DefaultPostCommand,
			postCommandExplicit: false,
			want:                false,
		},
		{
			name:                "explicitly set to value",
			postCommand:         "some command",
			postCommandExplicit: true,
			want:                false,
		},
		{
			name:                "explicitly set to empty",
			postCommand:         "",
			postCommandExplicit: true,
			want:                true,
		},
		{
			name:                "empty but not explicit (default empty)",
			postCommand:         "",
			postCommandExplicit: false,
			want:                false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				PostCommand:         tt.postCommand,
				postCommandExplicit: tt.postCommandExplicit,
			}
			if got := cfg.IsPostCommandDisabled(); got != tt.want {
				t.Errorf("IsPostCommandDisabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Tests for serve configuration (ServePort, ServeBind)

func TestLoad_ServeDefaults(t *testing.T) {
	// Create empty temp directory without config
	tmpDir := t.TempDir()
	homeDir := t.TempDir()

	// Isolate from real home config
	t.Setenv("HOME", homeDir)

	cfg := Load(tmpDir)

	if cfg.ServePort != DefaultServePort {
		t.Errorf("expected default ServePort %d, got %d", DefaultServePort, cfg.ServePort)
	}
	if cfg.ServeBind != DefaultServeBind {
		t.Errorf("expected default ServeBind %q, got %q", DefaultServeBind, cfg.ServeBind)
	}
}

func TestLoad_ServeFromConfig(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()

	t.Setenv("HOME", homeDir)

	// Write project config with serve values
	projectConfig := `serve-port: 9000
serve-bind: "0.0.0.0"
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(projectConfig), 0644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	cfg := Load(tmpDir)

	if cfg.ServePort != 9000 {
		t.Errorf("expected ServePort 9000, got %d", cfg.ServePort)
	}
	if cfg.ServeBind != "0.0.0.0" {
		t.Errorf("expected ServeBind %q, got %q", "0.0.0.0", cfg.ServeBind)
	}
}

func TestLoad_ServeEnvOverride(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()

	t.Setenv("HOME", homeDir)

	// Write project config with serve values
	projectConfig := `serve-port: 9000
serve-bind: "0.0.0.0"
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(projectConfig), 0644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	// Set environment variables to override
	t.Setenv("ORBIT_SERVE_PORT", "3000")
	t.Setenv("ORBIT_SERVE_BIND", "192.168.1.1")

	cfg := Load(tmpDir)

	if cfg.ServePort != 3000 {
		t.Errorf("expected ServePort 3000 from env var, got %d", cfg.ServePort)
	}
	if cfg.ServeBind != "192.168.1.1" {
		t.Errorf("expected ServeBind %q from env var, got %q", "192.168.1.1", cfg.ServeBind)
	}
}

func TestLoad_ServeHomeConfigMerge(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()

	t.Setenv("HOME", homeDir)

	// Home config sets both serve options
	homeConfig := `serve-port: 8888
serve-bind: "127.0.0.1"
`
	if err := os.WriteFile(filepath.Join(homeDir, ".orbit.yaml"), []byte(homeConfig), 0644); err != nil {
		t.Fatalf("failed to write home config: %v", err)
	}

	// Project config only overrides port
	projectConfig := `serve-port: 9999
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(projectConfig), 0644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	cfg := Load(tmpDir)

	// Project port should override home port
	if cfg.ServePort != 9999 {
		t.Errorf("expected ServePort 9999 from project config, got %d", cfg.ServePort)
	}
	// Home bind should be preserved since project didn't override
	if cfg.ServeBind != "127.0.0.1" {
		t.Errorf("expected ServeBind %q from home config, got %q", "127.0.0.1", cfg.ServeBind)
	}
}

func TestLoad_ServeInvalidEnvPort(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()

	t.Setenv("HOME", homeDir)
	t.Setenv("ORBIT_SERVE_PORT", "invalid")

	cfg := Load(tmpDir)

	// Invalid port should fall back to default
	if cfg.ServePort != DefaultServePort {
		t.Errorf("expected default ServePort %d for invalid env, got %d", DefaultServePort, cfg.ServePort)
	}
}

// Tests for agent configuration

func TestLoad_AgentsSection(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()

	t.Setenv("HOME", homeDir)

	// Write project config with agents section
	projectConfig := `agent: codex
agents:
  claude-code:
    cli-path: /usr/local/bin/claude
    auto-approve: false
    timeout: 30m
  codex:
    cli-path: codex
    auto-approve: true
    extra-args:
      - "--search"
      - "--verbose"
    timeout: 1h
  kiro:
    cli-path: kiro-cli
    model: auto
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(projectConfig), 0644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	cfg := Load(tmpDir)

	// Check default agent selection
	if cfg.Agent != "codex" {
		t.Errorf("expected Agent %q, got %q", "codex", cfg.Agent)
	}

	// Check agents map is populated
	if len(cfg.Agents) != 3 {
		t.Errorf("expected 3 agents, got %d", len(cfg.Agents))
	}

	// Check claude-code config
	claudeCfg, ok := cfg.Agents["claude-code"]
	if !ok {
		t.Fatal("expected claude-code in agents map")
	}
	if claudeCfg.CLIPath != "/usr/local/bin/claude" {
		t.Errorf("expected claude-code CLIPath %q, got %q", "/usr/local/bin/claude", claudeCfg.CLIPath)
	}
	if claudeCfg.AutoApprove {
		t.Error("expected claude-code AutoApprove to be false")
	}
	if claudeCfg.Timeout != "30m" {
		t.Errorf("expected claude-code Timeout %q, got %q", "30m", claudeCfg.Timeout)
	}

	// Check codex config
	codexCfg, ok := cfg.Agents["codex"]
	if !ok {
		t.Fatal("expected codex in agents map")
	}
	if codexCfg.CLIPath != "codex" {
		t.Errorf("expected codex CLIPath %q, got %q", "codex", codexCfg.CLIPath)
	}
	if !codexCfg.AutoApprove {
		t.Error("expected codex AutoApprove to be true")
	}
	if len(codexCfg.ExtraArgs) != 2 {
		t.Errorf("expected 2 extra args, got %d", len(codexCfg.ExtraArgs))
	}
	if codexCfg.Timeout != "1h" {
		t.Errorf("expected codex Timeout %q, got %q", "1h", codexCfg.Timeout)
	}

	// Check kiro config
	kiroCfg, ok := cfg.Agents["kiro"]
	if !ok {
		t.Fatal("expected kiro in agents map")
	}
	if kiroCfg.Model != "auto" {
		t.Errorf("expected kiro Model %q, got %q", "auto", kiroCfg.Model)
	}
}

func TestGetAgentConfig_WithTimeout(t *testing.T) {
	cfg := &Config{
		Agents: map[string]AgentConfig{
			"claude-code": {
				CLIPath:     "/usr/local/bin/claude",
				AutoApprove: true,
				Timeout:     "30m",
			},
		},
	}

	agentCfg := cfg.GetAgentConfig("claude-code")

	if agentCfg.CLIPath != "/usr/local/bin/claude" {
		t.Errorf("expected CLIPath %q, got %q", "/usr/local/bin/claude", agentCfg.CLIPath)
	}
	if !agentCfg.AutoApprove {
		t.Error("expected AutoApprove to be true")
	}
	expectedTimeout := 30 * time.Minute
	if agentCfg.Timeout != expectedTimeout {
		t.Errorf("expected Timeout %v, got %v", expectedTimeout, agentCfg.Timeout)
	}
}

func TestGetAgentConfig_InvalidTimeout(t *testing.T) {
	cfg := &Config{
		Agents: map[string]AgentConfig{
			"test-agent": {
				CLIPath: "test",
				Timeout: "invalid-duration",
			},
		},
	}

	agentCfg := cfg.GetAgentConfig("test-agent")

	// Invalid timeout should result in zero duration
	if agentCfg.Timeout != 0 {
		t.Errorf("expected zero Timeout for invalid duration, got %v", agentCfg.Timeout)
	}
	// Other fields should still be set
	if agentCfg.CLIPath != "test" {
		t.Errorf("expected CLIPath %q, got %q", "test", agentCfg.CLIPath)
	}
}

func TestGetAgentConfig_NotConfigured(t *testing.T) {
	cfg := &Config{
		Agents: map[string]AgentConfig{
			"other-agent": {
				CLIPath: "other",
			},
		},
	}

	agentCfg := cfg.GetAgentConfig("missing-agent")

	// Should return default AgentConfig with AutoApprove enabled for non-interactive operation
	if agentCfg.CLIPath != "" {
		t.Errorf("expected empty CLIPath for unconfigured agent, got %q", agentCfg.CLIPath)
	}
	if !agentCfg.AutoApprove {
		t.Error("expected AutoApprove to be true by default for non-interactive operation")
	}
	if agentCfg.Timeout != 0 {
		t.Errorf("expected zero Timeout for unconfigured agent, got %v", agentCfg.Timeout)
	}
}

func TestGetAgentConfig_WithModel(t *testing.T) {
	cfg := &Config{
		Agents: map[string]AgentConfig{
			"kiro": {
				CLIPath: "kiro-cli",
				Model:   "auto",
			},
		},
	}

	agentCfg := cfg.GetAgentConfig("kiro")

	if agentCfg.Options == nil {
		t.Fatal("expected Options to be set")
	}
	if agentCfg.Options["model"] != "auto" {
		t.Errorf("expected model %q in Options, got %q", "auto", agentCfg.Options["model"])
	}
}

func TestGetAgentConfig_WithExtraArgs(t *testing.T) {
	cfg := &Config{
		Agents: map[string]AgentConfig{
			"codex": {
				CLIPath:   "codex",
				ExtraArgs: []string{"--search", "--verbose"},
			},
		},
	}

	agentCfg := cfg.GetAgentConfig("codex")

	if len(agentCfg.ExtraArgs) != 2 {
		t.Errorf("expected 2 ExtraArgs, got %d", len(agentCfg.ExtraArgs))
	}
	if agentCfg.ExtraArgs[0] != "--search" {
		t.Errorf("expected ExtraArgs[0] %q, got %q", "--search", agentCfg.ExtraArgs[0])
	}
	if agentCfg.ExtraArgs[1] != "--verbose" {
		t.Errorf("expected ExtraArgs[1] %q, got %q", "--verbose", agentCfg.ExtraArgs[1])
	}
}

func TestLoad_AgentMergeHomeAndProject(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()

	t.Setenv("HOME", homeDir)

	// Home config with default agent settings
	homeConfig := `agent: claude-code
agents:
  claude-code:
    cli-path: claude
    auto-approve: false
    timeout: 30m
`
	if err := os.WriteFile(filepath.Join(homeDir, ".orbit.yaml"), []byte(homeConfig), 0644); err != nil {
		t.Fatalf("failed to write home config: %v", err)
	}

	// Project config overrides agent and adds codex
	projectConfig := `agent: codex
agents:
  codex:
    cli-path: /usr/local/bin/codex
    auto-approve: true
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(projectConfig), 0644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	cfg := Load(tmpDir)

	// Agent selection should be from project config
	if cfg.Agent != "codex" {
		t.Errorf("expected Agent %q from project, got %q", "codex", cfg.Agent)
	}

	// Should have both agents merged
	if len(cfg.Agents) != 2 {
		t.Errorf("expected 2 agents after merge, got %d", len(cfg.Agents))
	}

	// Claude-code from home should be present
	if _, ok := cfg.Agents["claude-code"]; !ok {
		t.Error("expected claude-code from home config to be present")
	}

	// Codex from project should be present
	codexCfg, ok := cfg.Agents["codex"]
	if !ok {
		t.Fatal("expected codex from project config to be present")
	}
	if codexCfg.CLIPath != "/usr/local/bin/codex" {
		t.Errorf("expected codex CLIPath from project, got %q", codexCfg.CLIPath)
	}
}

// Property-based tests for alias validation using rapid

func TestProperty_ValidAliasName_AcceptsValidPatterns(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate valid alias names matching [a-z0-9]+(-[a-z0-9]+)*
		validName := rapid.StringMatching(`[a-z0-9]+(-[a-z0-9]+)*`).Draw(t, "validName")
		err := ValidateAliasName(validName)
		if err != nil {
			t.Fatalf("valid name %q rejected: %v", validName, err)
		}
	})
}

func TestProperty_ValidAliasName_RejectsStartingHyphen(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Names starting with hyphen should fail
		suffix := rapid.StringMatching(`[a-z0-9]+`).Draw(t, "suffix")
		invalidName := "-" + suffix
		err := ValidateAliasName(invalidName)
		if err == nil {
			t.Fatalf("invalid name %q (starts with hyphen) was accepted", invalidName)
		}
	})
}

func TestProperty_ValidAliasName_RejectsEndingHyphen(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Names ending with hyphen should fail
		prefix := rapid.StringMatching(`[a-z0-9]+`).Draw(t, "prefix")
		invalidName := prefix + "-"
		err := ValidateAliasName(invalidName)
		if err == nil {
			t.Fatalf("invalid name %q (ends with hyphen) was accepted", invalidName)
		}
	})
}

func TestProperty_NormalizeAliasName_CaseInsensitive(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Any case variant normalizes to same result
		name := rapid.StringMatching(`[a-zA-Z0-9]+(-[a-zA-Z0-9]+)*`).Draw(t, "name")
		n1 := NormalizeAliasName(name)
		n2 := NormalizeAliasName(strings.ToUpper(name))
		n3 := NormalizeAliasName(strings.ToLower(name))

		if n1 != n2 || n2 != n3 {
			t.Fatalf("inconsistent normalization: %q -> %q, %q, %q", name, n1, n2, n3)
		}
	})
}

func TestProperty_NormalizeAliasName_AlwaysLowercase(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Normalization always produces lowercase output
		name := rapid.StringMatching(`[a-zA-Z0-9-]+`).Draw(t, "name")
		normalized := NormalizeAliasName(name)
		if normalized != strings.ToLower(normalized) {
			t.Fatalf("normalized name %q is not lowercase", normalized)
		}
	})
}

func TestProperty_NormalizeAliasName_Idempotent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Normalizing twice gives the same result as normalizing once
		name := rapid.StringMatching(`[a-zA-Z0-9-]+`).Draw(t, "name")
		once := NormalizeAliasName(name)
		twice := NormalizeAliasName(once)
		if once != twice {
			t.Fatalf("normalization not idempotent: %q -> %q -> %q", name, once, twice)
		}
	})
}

// Table-driven unit tests for alias validation

func TestValidateAliasName(t *testing.T) {
	tests := []struct {
		name      string
		aliasName string
		wantErr   bool
	}{
		// Valid names
		{name: "simple lowercase", aliasName: "claude", wantErr: false},
		{name: "with numbers", aliasName: "agent1", wantErr: false},
		{name: "with hyphen", aliasName: "claude-sonnet", wantErr: false},
		{name: "multiple hyphens", aliasName: "my-agent-v2", wantErr: false},
		{name: "single letter", aliasName: "a", wantErr: false},
		{name: "single number", aliasName: "1", wantErr: false},
		{name: "numbers and letters", aliasName: "agent2test", wantErr: false},
		{name: "complex valid", aliasName: "claude-opus-4-20250514", wantErr: false},

		// Invalid names - empty
		{name: "empty string", aliasName: "", wantErr: true},

		// Invalid names - starts with hyphen
		{name: "starts with hyphen", aliasName: "-agent", wantErr: true},
		{name: "starts with hyphen with suffix", aliasName: "-claude-code", wantErr: true},

		// Invalid names - ends with hyphen
		{name: "ends with hyphen", aliasName: "agent-", wantErr: true},
		{name: "ends with hyphen complex", aliasName: "claude-code-", wantErr: true},

		// Invalid names - underscore
		{name: "underscore instead of hyphen", aliasName: "my_agent", wantErr: true},
		{name: "underscore at start", aliasName: "_agent", wantErr: true},
		{name: "underscore at end", aliasName: "agent_", wantErr: true},

		// Invalid names - dot
		{name: "dot separator", aliasName: "my.agent", wantErr: true},
		{name: "version with dot", aliasName: "agent.v1", wantErr: true},

		// Invalid names - uppercase (these get normalized but the normalization is tested separately)
		// ValidateAliasName normalizes first, so uppercase should pass after normalization
		{name: "uppercase", aliasName: "CLAUDE", wantErr: false},
		{name: "mixed case", aliasName: "Claude-Sonnet", wantErr: false},
		{name: "camelCase", aliasName: "claudeCode", wantErr: false},

		// Invalid names - special characters
		{name: "space", aliasName: "my agent", wantErr: true},
		{name: "slash", aliasName: "my/agent", wantErr: true},
		{name: "colon", aliasName: "my:agent", wantErr: true},
		{name: "at sign", aliasName: "my@agent", wantErr: true},

		// Invalid names - consecutive hyphens
		{name: "double hyphen", aliasName: "my--agent", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAliasName(tt.aliasName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAliasName(%q) error = %v, wantErr %v", tt.aliasName, err, tt.wantErr)
			}
		})
	}
}

func TestNormalizeAliasName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "already lowercase", input: "claude", expected: "claude"},
		{name: "uppercase", input: "CLAUDE", expected: "claude"},
		{name: "mixed case", input: "Claude-Sonnet", expected: "claude-sonnet"},
		{name: "camelCase", input: "claudeCode", expected: "claudecode"},
		{name: "numbers unchanged", input: "Agent123", expected: "agent123"},
		{name: "hyphens preserved", input: "MY-AGENT-V2", expected: "my-agent-v2"},
		{name: "empty string", input: "", expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeAliasName(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeAliasName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// Tests for YAML type coercion (model field)

func TestLoad_ModelTypeCoercion_String(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectConfig := `agents:
  claude-code:
    type: claude-code
    model: "gpt-4"
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(projectConfig), 0644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	cfg := Load(tmpDir)

	if len(cfg.ConfigParseError) > 0 {
		t.Errorf("unexpected parse errors: %v", cfg.ConfigParseError)
	}

	aliasCfg, ok := cfg.AgentAliases["claude-code"]
	if !ok {
		t.Fatal("expected claude-code alias in AgentAliases")
	}
	if aliasCfg.Model != "gpt-4" {
		t.Errorf("expected Model %q, got %q", "gpt-4", aliasCfg.Model)
	}
}

func TestLoad_ModelTypeCoercion_UnquotedString(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectConfig := `agents:
  claude-code:
    type: claude-code
    model: gpt-4
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(projectConfig), 0644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	cfg := Load(tmpDir)

	if len(cfg.ConfigParseError) > 0 {
		t.Errorf("unexpected parse errors: %v", cfg.ConfigParseError)
	}

	aliasCfg, ok := cfg.AgentAliases["claude-code"]
	if !ok {
		t.Fatal("expected claude-code alias in AgentAliases")
	}
	if aliasCfg.Model != "gpt-4" {
		t.Errorf("expected Model %q, got %q", "gpt-4", aliasCfg.Model)
	}
}

func TestLoad_ModelTypeCoercion_Integer(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectConfig := `agents:
  claude-code:
    type: claude-code
    model: 4
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(projectConfig), 0644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	cfg := Load(tmpDir)

	if len(cfg.ConfigParseError) > 0 {
		t.Errorf("unexpected parse errors: %v", cfg.ConfigParseError)
	}

	aliasCfg, ok := cfg.AgentAliases["claude-code"]
	if !ok {
		t.Fatal("expected claude-code alias in AgentAliases")
	}
	if aliasCfg.Model != "4" {
		t.Errorf("expected Model %q, got %q", "4", aliasCfg.Model)
	}
}

func TestLoad_ModelTypeCoercion_Float(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectConfig := `agents:
  claude-code:
    type: claude-code
    model: 4.5
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(projectConfig), 0644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	cfg := Load(tmpDir)

	if len(cfg.ConfigParseError) > 0 {
		t.Errorf("unexpected parse errors: %v", cfg.ConfigParseError)
	}

	aliasCfg, ok := cfg.AgentAliases["claude-code"]
	if !ok {
		t.Fatal("expected claude-code alias in AgentAliases")
	}
	if aliasCfg.Model != "4.5" {
		t.Errorf("expected Model %q, got %q", "4.5", aliasCfg.Model)
	}
}

func TestLoad_ModelTypeCoercion_Boolean_Error(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectConfig := `agents:
  claude-code:
    type: claude-code
    model: true
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(projectConfig), 0644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	cfg := Load(tmpDir)

	if len(cfg.ConfigParseError) == 0 {
		t.Fatal("expected parse error for boolean model")
	}
	if !strings.Contains(cfg.ConfigParseError[0].Error(), "model must be a string or number, got bool") {
		t.Errorf("expected error to mention bool type, got: %v", cfg.ConfigParseError[0])
	}
}

func TestLoad_ModelTypeCoercion_Array_Error(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectConfig := `agents:
  claude-code:
    type: claude-code
    model:
      - a
      - b
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(projectConfig), 0644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	cfg := Load(tmpDir)

	if len(cfg.ConfigParseError) == 0 {
		t.Fatal("expected parse error for array model")
	}
	if !strings.Contains(cfg.ConfigParseError[0].Error(), "model must be a string or number, got array") {
		t.Errorf("expected error to mention array type, got: %v", cfg.ConfigParseError[0])
	}
}

func TestLoad_ModelTypeCoercion_Map_Error(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectConfig := `agents:
  claude-code:
    type: claude-code
    model:
      key: value
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(projectConfig), 0644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	cfg := Load(tmpDir)

	if len(cfg.ConfigParseError) == 0 {
		t.Fatal("expected parse error for map model")
	}
	if !strings.Contains(cfg.ConfigParseError[0].Error(), "model must be a string or number, got map") {
		t.Errorf("expected error to mention map type, got: %v", cfg.ConfigParseError[0])
	}
}

// Tests for ResolveAliases validation

func TestResolveAliases_MissingType(t *testing.T) {
	cfg := &Config{
		ConfigFileFound: true,
		AgentAliases: map[string]AgentAliasConfig{
			"my-agent": {
				Model: "gpt-4",
				// Type is missing
			},
		},
	}

	err := cfg.ResolveAliases()
	if err == nil {
		t.Fatal("expected error for missing type field")
	}
	if !strings.Contains(err.Error(), "missing required \"type\" field") {
		t.Errorf("expected error to mention missing type, got: %v", err)
	}
}

func TestResolveAliases_UnknownType(t *testing.T) {
	cfg := &Config{
		ConfigFileFound: true,
		AgentAliases: map[string]AgentAliasConfig{
			"my-agent": {
				Type:  "unknown-agent-type",
				Model: "gpt-4",
			},
		},
	}

	err := cfg.ResolveAliases()
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
	if !strings.Contains(err.Error(), "unknown agent type") {
		t.Errorf("expected error to mention unknown type, got: %v", err)
	}
}

func TestResolveAliases_EmptyAgentsSection(t *testing.T) {
	cfg := &Config{
		ConfigFileFound: true,
		AgentAliases:    map[string]AgentAliasConfig{},
	}

	err := cfg.ResolveAliases()
	if err == nil {
		t.Fatal("expected error for empty agents section")
	}
	if !strings.Contains(err.Error(), "no agents configured") {
		t.Errorf("expected error to mention no agents configured, got: %v", err)
	}
}

func TestResolveAliases_DuplicateAfterNormalization(t *testing.T) {
	cfg := &Config{
		ConfigFileFound: true,
		AgentAliases: map[string]AgentAliasConfig{
			"claude-code": {
				Type: "claude-code",
			},
			"Claude-Code": {
				Type: "claude-code",
			},
		},
	}

	err := cfg.ResolveAliases()
	if err == nil {
		t.Fatal("expected error for duplicate aliases after normalization")
	}
	if !strings.Contains(err.Error(), "duplicate agent aliases") {
		t.Errorf("expected error to mention duplicate aliases, got: %v", err)
	}
}

func TestResolveAliases_Valid(t *testing.T) {
	cfg := &Config{
		ConfigFileFound: true,
		AgentAliases: map[string]AgentAliasConfig{
			"claude-sonnet": {
				Type:  "claude-code",
				Model: "claude-sonnet-4",
			},
			"claude-opus": {
				Type:  "claude-code",
				Model: "claude-opus-4",
			},
		},
	}

	err := cfg.ResolveAliases()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.ResolvedAgents) != 2 {
		t.Errorf("expected 2 resolved agents, got %d", len(cfg.ResolvedAgents))
	}

	sonnet, ok := cfg.ResolvedAgents["claude-sonnet"]
	if !ok {
		t.Fatal("expected claude-sonnet in ResolvedAgents")
	}
	if sonnet.Type != "claude-code" {
		t.Errorf("expected Type %q, got %q", "claude-code", sonnet.Type)
	}
	if sonnet.Config.Model != "claude-sonnet-4" {
		t.Errorf("expected Model %q, got %q", "claude-sonnet-4", sonnet.Config.Model)
	}
}

func TestResolveAliases_PropagatesParseErrors(t *testing.T) {
	cfg := &Config{
		ConfigFileFound:  true,
		ConfigParseError: []error{fmt.Errorf("alias %q: model must be a string or number, got bool", "test")},
		AgentAliases: map[string]AgentAliasConfig{
			"test": {
				Type: "claude-code",
			},
		},
	}

	err := cfg.ResolveAliases()
	if err == nil {
		t.Fatal("expected error from parse errors")
	}
	if !strings.Contains(err.Error(), "model must be a string or number") {
		t.Errorf("expected parse error to be propagated, got: %v", err)
	}
}

// Tests for RequireConfigFile

func TestRequireConfigFile_Found(t *testing.T) {
	cfg := &Config{
		ConfigFileFound: true,
	}

	err := cfg.RequireConfigFile()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRequireConfigFile_NotFound(t *testing.T) {
	cfg := &Config{
		ConfigFileFound: false,
	}

	err := cfg.RequireConfigFile()
	if err == nil {
		t.Fatal("expected error when config file not found")
	}
	if !strings.Contains(err.Error(), "orbit init") {
		t.Errorf("expected error to mention orbit init, got: %v", err)
	}
}

// Tests for GetResolvedAgent

func TestGetResolvedAgent_Found(t *testing.T) {
	cfg := &Config{
		ConfigFileFound: true,
		AgentAliases: map[string]AgentAliasConfig{
			"claude-code": {
				Type:  "claude-code",
				Model: "claude-sonnet-4",
			},
		},
	}

	if err := cfg.ResolveAliases(); err != nil {
		t.Fatalf("failed to resolve aliases: %v", err)
	}

	resolved, err := cfg.GetResolvedAgent("claude-code")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Type != "claude-code" {
		t.Errorf("expected Type %q, got %q", "claude-code", resolved.Type)
	}
}

func TestGetResolvedAgent_CaseInsensitive(t *testing.T) {
	cfg := &Config{
		ConfigFileFound: true,
		AgentAliases: map[string]AgentAliasConfig{
			"claude-code": {
				Type:  "claude-code",
				Model: "claude-sonnet-4",
			},
		},
	}

	if err := cfg.ResolveAliases(); err != nil {
		t.Fatalf("failed to resolve aliases: %v", err)
	}

	// Should work with uppercase
	resolved, err := cfg.GetResolvedAgent("Claude-Code")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Type != "claude-code" {
		t.Errorf("expected Type %q, got %q", "claude-code", resolved.Type)
	}
}

func TestGetResolvedAgent_NotFound(t *testing.T) {
	cfg := &Config{
		ConfigFileFound: true,
		AgentAliases: map[string]AgentAliasConfig{
			"claude-code": {
				Type: "claude-code",
			},
		},
	}

	if err := cfg.ResolveAliases(); err != nil {
		t.Fatalf("failed to resolve aliases: %v", err)
	}

	_, err := cfg.GetResolvedAgent("unknown")
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Errorf("expected error to mention not configured, got: %v", err)
	}
}

func TestGetResolvedAgent_BeforeResolve(t *testing.T) {
	cfg := &Config{
		ConfigFileFound: true,
		AgentAliases: map[string]AgentAliasConfig{
			"claude-code": {
				Type: "claude-code",
			},
		},
	}

	// Don't call ResolveAliases
	_, err := cfg.GetResolvedAgent("claude-code")
	if err == nil {
		t.Fatal("expected error when ResolveAliases not called")
	}
	if !strings.Contains(err.Error(), "ResolveAliases() must be called") {
		t.Errorf("expected error about ResolveAliases, got: %v", err)
	}
}

// Tests for config merge behavior

func TestLoad_ConfigMerge_HomeOnly(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	homeConfig := `agents:
  claude-code:
    type: claude-code
    model: home-model
    timeout: 30m
`
	if err := os.WriteFile(filepath.Join(homeDir, ".orbit.yaml"), []byte(homeConfig), 0644); err != nil {
		t.Fatalf("failed to write home config: %v", err)
	}

	cfg := Load(tmpDir)

	if !cfg.ConfigFileFound {
		t.Error("expected ConfigFileFound to be true")
	}

	aliasCfg, ok := cfg.AgentAliases["claude-code"]
	if !ok {
		t.Fatal("expected claude-code alias")
	}
	if aliasCfg.Type != "claude-code" {
		t.Errorf("expected Type %q, got %q", "claude-code", aliasCfg.Type)
	}
	if aliasCfg.Model != "home-model" {
		t.Errorf("expected Model %q, got %q", "home-model", aliasCfg.Model)
	}
	if aliasCfg.Timeout != "30m" {
		t.Errorf("expected Timeout %q, got %q", "30m", aliasCfg.Timeout)
	}
}

func TestLoad_ConfigMerge_ProjectOnly(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectConfig := `agents:
  claude-code:
    type: claude-code
    model: project-model
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(projectConfig), 0644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	cfg := Load(tmpDir)

	if !cfg.ConfigFileFound {
		t.Error("expected ConfigFileFound to be true")
	}

	aliasCfg, ok := cfg.AgentAliases["claude-code"]
	if !ok {
		t.Fatal("expected claude-code alias")
	}
	if aliasCfg.Model != "project-model" {
		t.Errorf("expected Model %q, got %q", "project-model", aliasCfg.Model)
	}
}

func TestLoad_ConfigMerge_DeepMerge(t *testing.T) {
	// Tests that Viper's deep merge works: project overrides home fields but
	// unset fields inherit from home
	tmpDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	homeConfig := `agents:
  claude-code:
    type: claude-code
    model: home-model
    timeout: 30m
`
	if err := os.WriteFile(filepath.Join(homeDir, ".orbit.yaml"), []byte(homeConfig), 0644); err != nil {
		t.Fatalf("failed to write home config: %v", err)
	}

	// Project only overrides model, not timeout
	projectConfig := `agents:
  claude-code:
    type: claude-code
    model: project-model
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(projectConfig), 0644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	cfg := Load(tmpDir)

	aliasCfg, ok := cfg.AgentAliases["claude-code"]
	if !ok {
		t.Fatal("expected claude-code alias")
	}

	// Model should be overridden by project
	if aliasCfg.Model != "project-model" {
		t.Errorf("expected Model %q (from project), got %q", "project-model", aliasCfg.Model)
	}
	// Timeout should be inherited from home (Viper deep merges nested maps)
	if aliasCfg.Timeout != "30m" {
		t.Errorf("expected Timeout %q (from home), got %q", "30m", aliasCfg.Timeout)
	}
}

func TestLoad_ConfigMerge_DifferentAliases(t *testing.T) {
	// Tests that aliases from both configs are available
	tmpDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	homeConfig := `agents:
  claude-sonnet:
    type: claude-code
    model: claude-sonnet-4
`
	if err := os.WriteFile(filepath.Join(homeDir, ".orbit.yaml"), []byte(homeConfig), 0644); err != nil {
		t.Fatalf("failed to write home config: %v", err)
	}

	projectConfig := `agents:
  claude-opus:
    type: claude-code
    model: claude-opus-4
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(projectConfig), 0644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	cfg := Load(tmpDir)

	if len(cfg.AgentAliases) != 2 {
		t.Errorf("expected 2 aliases, got %d", len(cfg.AgentAliases))
	}

	sonnet, ok := cfg.AgentAliases["claude-sonnet"]
	if !ok {
		t.Error("expected claude-sonnet from home config")
	} else if sonnet.Model != "claude-sonnet-4" {
		t.Errorf("expected claude-sonnet Model %q, got %q", "claude-sonnet-4", sonnet.Model)
	}

	opus, ok := cfg.AgentAliases["claude-opus"]
	if !ok {
		t.Error("expected claude-opus from project config")
	} else if opus.Model != "claude-opus-4" {
		t.Errorf("expected claude-opus Model %q, got %q", "claude-opus-4", opus.Model)
	}
}

func TestLoad_ConfigMerge_AliasShadowsTypeName(t *testing.T) {
	// Tests that an alias named the same as a type works correctly
	tmpDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectConfig := `agents:
  claude-code:
    type: claude-code
    model: claude-opus-4
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(projectConfig), 0644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	cfg := Load(tmpDir)

	if err := cfg.ResolveAliases(); err != nil {
		t.Fatalf("failed to resolve aliases: %v", err)
	}

	resolved, err := cfg.GetResolvedAgent("claude-code")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should use the alias config, not default agent
	if resolved.Config.Model != "claude-opus-4" {
		t.Errorf("expected Model %q, got %q", "claude-opus-4", resolved.Config.Model)
	}
}

// Test for GetResolvedAgentConfig helper

func TestGetResolvedAgentConfig(t *testing.T) {
	resolved := ResolvedAgent{
		Alias: "my-alias",
		Type:  "claude-code",
		Config: AgentAliasConfig{
			Type:        "claude-code",
			Model:       "claude-opus-4",
			CLIPath:     "/usr/local/bin/claude",
			AutoApprove: true,
			ExtraArgs:   []string{"--verbose"},
			Timeout:     "30m",
		},
	}

	agentCfg := GetResolvedAgentConfig(resolved)

	if agentCfg.CLIPath != "/usr/local/bin/claude" {
		t.Errorf("expected CLIPath %q, got %q", "/usr/local/bin/claude", agentCfg.CLIPath)
	}
	if !agentCfg.AutoApprove {
		t.Error("expected AutoApprove to be true")
	}
	if len(agentCfg.ExtraArgs) != 1 || agentCfg.ExtraArgs[0] != "--verbose" {
		t.Errorf("expected ExtraArgs [--verbose], got %v", agentCfg.ExtraArgs)
	}
	if agentCfg.Timeout != 30*time.Minute {
		t.Errorf("expected Timeout 30m, got %v", agentCfg.Timeout)
	}
	if agentCfg.Options == nil || agentCfg.Options["model"] != "claude-opus-4" {
		t.Errorf("expected Options[model] = %q, got %v", "claude-opus-4", agentCfg.Options)
	}
}

// Tests for CentralizedLog configuration

func TestLoad_CentralizedLog_DefaultTrue(t *testing.T) {
	// Create empty temp directories to isolate from real config
	tmpDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	cfg := Load(tmpDir)

	if !cfg.CentralizedLog {
		t.Error("expected CentralizedLog to default to true")
	}
}

func TestLoad_CentralizedLog_EnvFalseDisables(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("ORBIT_CENTRALIZED_LOG", "false")

	cfg := Load(tmpDir)

	if cfg.CentralizedLog {
		t.Error("expected CentralizedLog to be false when ORBIT_CENTRALIZED_LOG=false")
	}
}

func TestLoad_CentralizedLog_EnvZeroDisables(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("ORBIT_CENTRALIZED_LOG", "0")

	cfg := Load(tmpDir)

	if cfg.CentralizedLog {
		t.Error("expected CentralizedLog to be false when ORBIT_CENTRALIZED_LOG=0")
	}
}

func TestLoad_CentralizedLog_EnvTrueEnables(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("ORBIT_CENTRALIZED_LOG", "true")

	cfg := Load(tmpDir)

	if !cfg.CentralizedLog {
		t.Error("expected CentralizedLog to be true when ORBIT_CENTRALIZED_LOG=true")
	}
}

func TestLoad_CentralizedLog_EnvOneEnables(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("ORBIT_CENTRALIZED_LOG", "1")

	cfg := Load(tmpDir)

	if !cfg.CentralizedLog {
		t.Error("expected CentralizedLog to be true when ORBIT_CENTRALIZED_LOG=1")
	}
}

func TestLoad_CentralizedLog_EnvEmptyEnables(t *testing.T) {
	// Empty string (not "false" or "0") should enable logging
	tmpDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("ORBIT_CENTRALIZED_LOG", "")

	cfg := Load(tmpDir)

	if !cfg.CentralizedLog {
		t.Error("expected CentralizedLog to be true when ORBIT_CENTRALIZED_LOG is empty string")
	}
}

func TestLoad_CentralizedLog_YAMLFalseDisables(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectConfig := `centralized-log: false
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(projectConfig), 0644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	cfg := Load(tmpDir)

	if cfg.CentralizedLog {
		t.Error("expected CentralizedLog to be false when set in YAML")
	}
}

func TestLoad_CentralizedLog_YAMLTrueEnables(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectConfig := `centralized-log: true
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(projectConfig), 0644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	cfg := Load(tmpDir)

	if !cfg.CentralizedLog {
		t.Error("expected CentralizedLog to be true when set in YAML")
	}
}

func TestLoad_CentralizedLog_EnvOverridesYAML(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// YAML enables it
	projectConfig := `centralized-log: true
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(projectConfig), 0644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	// Env var disables it
	t.Setenv("ORBIT_CENTRALIZED_LOG", "false")

	cfg := Load(tmpDir)

	if cfg.CentralizedLog {
		t.Error("expected env var to override YAML config")
	}
}

func TestLoad_CentralizedLog_ProjectOverridesHome(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// Home config enables it
	homeConfig := `centralized-log: true
`
	if err := os.WriteFile(filepath.Join(homeDir, ".orbit.yaml"), []byte(homeConfig), 0644); err != nil {
		t.Fatalf("failed to write home config: %v", err)
	}

	// Project config disables it
	projectConfig := `centralized-log: false
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(projectConfig), 0644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	cfg := Load(tmpDir)

	if cfg.CentralizedLog {
		t.Error("expected project config to override home config")
	}
}
