package config

import (
	"os"
	"path/filepath"
	"testing"
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
