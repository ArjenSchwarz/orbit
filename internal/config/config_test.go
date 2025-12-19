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

	// Set HOME to temp directory
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", homeDir)

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

	// Set HOME to temp directory
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", homeDir)

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

	// Set HOME to temp directory without config
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", homeDir)

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
	// Create temp directory with invalid YAML
	tmpDir := t.TempDir()

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

	// Set environment variable to override
	originalEnv := os.Getenv("ORBIT_COMMAND")
	defer os.Setenv("ORBIT_COMMAND", originalEnv)
	os.Setenv("ORBIT_COMMAND", "env command")

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

	// Set environment variable
	originalEnv := os.Getenv("ORBIT_POST_COMMAND")
	defer func() {
		if originalEnv == "" {
			os.Unsetenv("ORBIT_POST_COMMAND")
		} else {
			os.Setenv("ORBIT_POST_COMMAND", originalEnv)
		}
	}()
	os.Setenv("ORBIT_POST_COMMAND", "env post command")

	cfg := Load(tmpDir)

	if cfg.PostCommand != "env post command" {
		t.Errorf("expected PostCommand %q from env var, got %q", "env post command", cfg.PostCommand)
	}
}

func TestLoad_EnvVarEmptyPostCommand(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Set environment variable to empty string (explicitly disable)
	originalEnv := os.Getenv("ORBIT_POST_COMMAND")
	defer func() {
		if originalEnv == "" {
			os.Unsetenv("ORBIT_POST_COMMAND")
		} else {
			os.Setenv("ORBIT_POST_COMMAND", originalEnv)
		}
	}()
	os.Setenv("ORBIT_POST_COMMAND", "")

	cfg := Load(tmpDir)

	// Empty env var should disable post-command
	if cfg.PostCommand != "" {
		t.Errorf("expected empty PostCommand from env var, got %q", cfg.PostCommand)
	}
	if !cfg.IsPostCommandDisabled() {
		t.Error("expected IsPostCommandDisabled() to return true when env var is explicitly empty")
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
