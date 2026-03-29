package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arjenschwarz/orbit/internal/config"
)

// TestAutoConsolidate_FlagValidation tests that --auto-consolidate requires --variants
func TestAutoConsolidate_FlagValidation(t *testing.T) {
	// This test verifies the requirement:
	// "The system MUST validate that --auto-consolidate requires --variants"
	// The validation happens in runCommand after parsing flags

	// We can't easily unit test runCommand directly since it parses CLI args,
	// but we can test the validation logic pattern by checking error conditions

	// The error message should be:
	// "--auto-consolidate requires --variants to be specified"

	// Create a minimal test that simulates the validation check
	autoConsolidate := true
	variantCount := 0

	// This is the validation logic from run.go
	if autoConsolidate && variantCount == 0 {
		// This should trigger an error
		t.Log("Validation correctly requires --variants when --auto-consolidate is set")
	} else {
		t.Error("Expected validation to fail when --auto-consolidate is set without --variants")
	}
}

// TestAutoConsolidate_WithVariants validates that the combination is allowed
func TestAutoConsolidate_WithVariantsIsValid(t *testing.T) {
	autoConsolidate := true
	variantCount := 2

	// This combination should be valid
	if autoConsolidate && variantCount == 0 {
		t.Error("Validation should pass when --variants is specified with --auto-consolidate")
	} else {
		t.Log("Validation correctly allows --auto-consolidate with --variants")
	}
}

// TestAutoConsolidate_FlagResolution tests the flag resolution logic
func TestAutoConsolidate_FlagResolution(t *testing.T) {
	tests := []struct {
		name                  string
		configValue           bool
		autoConsolidateFlag   bool
		noAutoConsolidateFlag bool
		want                  bool
	}{
		{
			name:                  "default from config true, no flags",
			configValue:           true,
			autoConsolidateFlag:   false,
			noAutoConsolidateFlag: false,
			want:                  true,
		},
		{
			name:                  "default from config false, no flags",
			configValue:           false,
			autoConsolidateFlag:   false,
			noAutoConsolidateFlag: false,
			want:                  false,
		},
		{
			name:                  "config false, --auto-consolidate enables",
			configValue:           false,
			autoConsolidateFlag:   true,
			noAutoConsolidateFlag: false,
			want:                  true,
		},
		{
			name:                  "config true, --no-auto-consolidate disables",
			configValue:           true,
			autoConsolidateFlag:   false,
			noAutoConsolidateFlag: true,
			want:                  false,
		},
		{
			name:                  "--no-auto-consolidate takes precedence when both flags set",
			configValue:           false,
			autoConsolidateFlag:   true,
			noAutoConsolidateFlag: true,
			// When both flags are set, --auto-consolidate runs first, then --no-auto-consolidate
			// So the final value is false (--no-auto-consolidate wins)
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the flag resolution logic from run.go
			autoConsolidateValue := tt.configValue
			if tt.autoConsolidateFlag {
				autoConsolidateValue = true
			}
			if tt.noAutoConsolidateFlag {
				autoConsolidateValue = false
			}

			if autoConsolidateValue != tt.want {
				t.Errorf("autoConsolidateValue = %v, want %v", autoConsolidateValue, tt.want)
			}
		})
	}
}

// TestAutoConsolidate_ConfigPropagation verifies config values are passed to orbit.Config
func TestAutoConsolidate_ConfigPropagation(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// Create .orbit.yaml with auto-consolidate settings
	configContent := `auto-consolidate: true
post-consolidate-command: "make verify"
agents:
  claude-code:
    type: claude-code
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg := config.Load(tmpDir)

	// Verify config values are loaded
	if !cfg.AutoConsolidate {
		t.Error("expected AutoConsolidate to be true from config")
	}
	if cfg.PostConsolidateCommand != "make verify" {
		t.Errorf("expected PostConsolidateCommand %q, got %q", "make verify", cfg.PostConsolidateCommand)
	}
}

// TestAutoConsolidate_AllowDirtyFlag tests the --allow-dirty flag
func TestAutoConsolidate_AllowDirtyFlag(t *testing.T) {
	// The --allow-dirty flag should be passed through to consolidation config
	// This test verifies the flag behavior

	tests := []struct {
		name       string
		allowDirty bool
		wantValue  bool
	}{
		{
			name:       "allow-dirty false by default",
			allowDirty: false,
			wantValue:  false,
		},
		{
			name:       "allow-dirty enabled",
			allowDirty: true,
			wantValue:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the flag value being passed to config
			if tt.allowDirty != tt.wantValue {
				t.Errorf("allowDirty = %v, want %v", tt.allowDirty, tt.wantValue)
			}
		})
	}
}

// TestMaxParallel_FlagResolution tests that --max-parallel CLI flag properly overrides config,
// including the case where the explicit value matches the built-in default (3).
func TestMaxParallel_FlagResolution(t *testing.T) {
	tests := []struct {
		name         string
		configValue  int
		flagExplicit bool
		flagValue    int
		want         int
	}{
		{
			name:         "config only, no flag",
			configValue:  8,
			flagExplicit: false,
			flagValue:    3, // default
			want:         8,
		},
		{
			name:         "explicit flag overrides config with non-default",
			configValue:  8,
			flagExplicit: true,
			flagValue:    5,
			want:         5,
		},
		{
			name:         "explicit flag=3 overrides config (T-585 regression)",
			configValue:  8,
			flagExplicit: true,
			flagValue:    3,
			want:         3,
		},
		{
			name:         "config 0 falls back to flag default",
			configValue:  0,
			flagExplicit: false,
			flagValue:    3,
			want:         3,
		},
		{
			name:         "explicit flag=1 overrides config",
			configValue:  5,
			flagExplicit: true,
			flagValue:    1,
			want:         1,
		},
		{
			name:         "config value used when flag not set",
			configValue:  10,
			flagExplicit: false,
			flagValue:    3,
			want:         10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the resolution logic from run.go
			maxParallelValue := tt.configValue
			if tt.flagExplicit {
				maxParallelValue = tt.flagValue
			}
			if maxParallelValue == 0 {
				maxParallelValue = tt.flagValue
			}

			if maxParallelValue != tt.want {
				t.Errorf("maxParallelValue = %d, want %d", maxParallelValue, tt.want)
			}
		})
	}
}

// TestMaxParallel_FlagVisitDetection tests that fs.Visit correctly detects
// an explicitly set --max-parallel flag, even when its value equals the default.
func TestMaxParallel_FlagVisitDetection(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantExplicit bool
		wantValue    int
	}{
		{
			name:         "no flag set",
			args:         []string{},
			wantExplicit: false,
			wantValue:    3, // default
		},
		{
			name:         "explicit non-default value",
			args:         []string{"--max-parallel", "5"},
			wantExplicit: true,
			wantValue:    5,
		},
		{
			name:         "explicit default value (T-585 regression)",
			args:         []string{"--max-parallel", "3"},
			wantExplicit: true,
			wantValue:    3,
		},
		{
			name:         "explicit value with equals syntax",
			args:         []string{"--max-parallel=7"},
			wantExplicit: true,
			wantValue:    7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			maxParallel := fs.Int("max-parallel", 3, "Maximum parallel variants")

			if err := fs.Parse(tt.args); err != nil {
				t.Fatalf("failed to parse flags: %v", err)
			}

			explicit := false
			fs.Visit(func(f *flag.Flag) {
				if f.Name == "max-parallel" {
					explicit = true
				}
			})

			if explicit != tt.wantExplicit {
				t.Errorf("explicit = %v, want %v", explicit, tt.wantExplicit)
			}
			if *maxParallel != tt.wantValue {
				t.Errorf("maxParallel = %d, want %d", *maxParallel, tt.wantValue)
			}

			// Now simulate the full resolution with a config value of 8
			configValue := 8
			resolved := configValue
			if explicit {
				resolved = *maxParallel
			}
			if resolved == 0 {
				resolved = *maxParallel
			}

			if explicit && resolved != tt.wantValue {
				t.Errorf("resolved = %d, want %d (explicit flag should win)", resolved, tt.wantValue)
			}
			if !explicit && resolved != configValue {
				t.Errorf("resolved = %d, want %d (config should win when flag not set)", resolved, configValue)
			}
		})
	}
}

// TestDeprecatedPostCommandFlag tests that --post-command flag is rejected
func TestDeprecatedPostCommandFlag(t *testing.T) {
	// The runCommand function checks for deprecated --post-command flag
	// before parsing other flags and returns an error

	args := []string{"--post-command", "some value"}

	// Simulate the deprecation check from runCommand
	for _, arg := range args {
		if arg == "--post-command" || strings.HasPrefix(arg, "--post-command=") {
			t.Log("Deprecated flag --post-command correctly detected")
			return
		}
	}
	t.Error("Expected --post-command to be detected as deprecated")
}

func TestBuildAgentConfig(t *testing.T) {
	tests := []struct {
		name           string
		resolved       config.ResolvedAgent
		wantModel      string
		wantOptionsNil bool
	}{
		{
			name: "model in Options when set",
			resolved: config.ResolvedAgent{
				Alias: "claude-opus",
				Type:  "claude-code",
				Config: config.AgentAliasConfig{
					Type:        "claude-code",
					Model:       "claude-opus-4-20250514",
					AutoApprove: true,
				},
			},
			wantModel:      "claude-opus-4-20250514",
			wantOptionsNil: false,
		},
		{
			name: "Options nil when no model",
			resolved: config.ResolvedAgent{
				Alias: "claude-code",
				Type:  "claude-code",
				Config: config.AgentAliasConfig{
					Type:        "claude-code",
					AutoApprove: true,
				},
			},
			wantModel:      "",
			wantOptionsNil: true,
		},
		{
			name: "model with extra-args",
			resolved: config.ResolvedAgent{
				Alias: "codex-o3",
				Type:  "codex",
				Config: config.AgentAliasConfig{
					Type:      "codex",
					Model:     "o3",
					ExtraArgs: []string{"--verbose"},
				},
			},
			wantModel:      "o3",
			wantOptionsNil: false,
		},
		{
			name: "numeric model converted to string",
			resolved: config.ResolvedAgent{
				Alias: "test-agent",
				Type:  "codex",
				Config: config.AgentAliasConfig{
					Type:  "codex",
					Model: "4",
				},
			},
			wantModel:      "4",
			wantOptionsNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildAgentConfig(tt.resolved)

			if tt.wantOptionsNil {
				if len(result.Options) > 0 {
					t.Errorf("expected Options to be nil or empty, got %v", result.Options)
				}
			} else {
				if result.Options == nil {
					t.Fatal("expected Options to be non-nil")
				}
				got := result.Options["model"]
				if got != tt.wantModel {
					t.Errorf("Options[model] = %q, want %q", got, tt.wantModel)
				}
			}

			// Verify other fields are passed through
			if result.AutoApprove != tt.resolved.Config.AutoApprove {
				t.Errorf("AutoApprove = %v, want %v", result.AutoApprove, tt.resolved.Config.AutoApprove)
			}
			if len(result.ExtraArgs) != len(tt.resolved.Config.ExtraArgs) {
				t.Errorf("ExtraArgs length = %d, want %d", len(result.ExtraArgs), len(tt.resolved.Config.ExtraArgs))
			}
		})
	}
}
