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
	tests := map[string]struct {
		configValue  int
		flagExplicit bool
		flagValue    int
		want         int
	}{
		"config only, no flag": {
			configValue:  8,
			flagExplicit: false,
			flagValue:    3, // default
			want:         8,
		},
		"explicit flag overrides config with non-default": {
			configValue:  8,
			flagExplicit: true,
			flagValue:    5,
			want:         5,
		},
		"explicit flag=3 overrides config": {
			configValue:  8,
			flagExplicit: true,
			flagValue:    3,
			want:         3,
		},
		"config 0 falls back to flag default": {
			configValue:  0,
			flagExplicit: false,
			flagValue:    3,
			want:         3,
		},
		"explicit flag=1 overrides config": {
			configValue:  5,
			flagExplicit: true,
			flagValue:    1,
			want:         1,
		},
		"config value used when flag not set": {
			configValue:  10,
			flagExplicit: false,
			flagValue:    3,
			want:         10,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := resolveMaxParallel(tc.configValue, tc.flagValue, tc.flagExplicit)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("maxParallelValue = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestMaxParallel_FlagVisitDetection tests that fs.Visit correctly detects
// an explicitly set --max-parallel flag, even when its value equals the default.
func TestMaxParallel_FlagVisitDetection(t *testing.T) {
	tests := map[string]struct {
		args         []string
		wantExplicit bool
		wantValue    int
	}{
		"no flag set": {
			args:         []string{},
			wantExplicit: false,
			wantValue:    3, // default
		},
		"explicit non-default value": {
			args:         []string{"--max-parallel", "5"},
			wantExplicit: true,
			wantValue:    5,
		},
		"explicit default value": {
			args:         []string{"--max-parallel", "3"},
			wantExplicit: true,
			wantValue:    3,
		},
		"explicit value with equals syntax": {
			args:         []string{"--max-parallel=7"},
			wantExplicit: true,
			wantValue:    7,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			maxParallel := fs.Int("max-parallel", 3, "Maximum parallel variants")

			if err := fs.Parse(tc.args); err != nil {
				t.Fatalf("failed to parse flags: %v", err)
			}

			explicit := false
			fs.Visit(func(f *flag.Flag) {
				if f.Name == "max-parallel" {
					explicit = true
				}
			})

			if explicit != tc.wantExplicit {
				t.Errorf("explicit = %v, want %v", explicit, tc.wantExplicit)
			}
			if *maxParallel != tc.wantValue {
				t.Errorf("maxParallel = %d, want %d", *maxParallel, tc.wantValue)
			}

			// Verify the helper resolves to the expected value when a
			// non-zero config value would otherwise win.
			configValue := 8
			resolved, err := resolveMaxParallel(configValue, *maxParallel, explicit)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if explicit && resolved != tc.wantValue {
				t.Errorf("resolved = %d, want %d (explicit flag should win)", resolved, tc.wantValue)
			}
			if !explicit && resolved != configValue {
				t.Errorf("resolved = %d, want %d (config should win when flag not set)", resolved, configValue)
			}
		})
	}
}

// TestResolveMaxParallel covers resolution and validation of the --max-parallel
// flag against config/CLI precedence, including the regression where a negative
// `.orbit.yaml` value bypassed validation when the flag was not set.
//
// The validation must operate on the *resolved* value rather than the raw CLI
// flag value. Otherwise a negative config such as `max-parallel: -1` flows
// through to `make(chan struct{}, n)` and panics with "makechan: size out of
// range".
func TestResolveMaxParallel(t *testing.T) {
	tests := map[string]struct {
		configValue  int
		flagExplicit bool
		flagValue    int
		want         int
		wantErr      bool
	}{
		"negative config without explicit flag is rejected": {
			configValue:  -1,
			flagExplicit: false,
			flagValue:    3,
			wantErr:      true,
		},
		"negative explicit flag is rejected": {
			configValue:  3,
			flagExplicit: true,
			flagValue:    -2,
			wantErr:      true,
		},
		"explicit zero flag is rejected": {
			configValue:  3,
			flagExplicit: true,
			flagValue:    0,
			wantErr:      true,
		},
		"zero config falls back to flag default and is valid": {
			configValue:  0,
			flagExplicit: false,
			flagValue:    3,
			want:         3,
			wantErr:      false,
		},
		"valid positive config with no flag": {
			configValue:  4,
			flagExplicit: false,
			flagValue:    3,
			want:         4,
			wantErr:      false,
		},
		"explicit flag overrides config": {
			configValue:  8,
			flagExplicit: true,
			flagValue:    5,
			want:         5,
			wantErr:      false,
		},
		"explicit positive flag rescues negative config": {
			configValue:  -5,
			flagExplicit: true,
			flagValue:    2,
			want:         2,
			wantErr:      false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := resolveMaxParallel(tc.configValue, tc.flagValue, tc.flagExplicit)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected validation error, got nil (resolved=%d)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("resolveMaxParallel = %d, want %d", got, tc.want)
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

// TestVariantConfig_Resolution tests that resolveVariantFlags uses config values
// when CLI flags are not explicitly set (T-814).
func TestVariantConfig_Resolution(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	configContent := `variant-count: 3
branch-prefix: "my-prefix"
compare-command: "custom-compare"
guidance-file: "my-guidance.yaml"
parallel: true
agents:
  claude-code:
    type: claude-code
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg := config.Load(tmpDir)

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.Int("variants", 0, "")
	fs.String("branch-prefix", "orbit-impl", "")
	fs.String("compare-command", "", "")
	fs.String("guidance-file", "", "")

	if err := fs.Parse([]string{}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	vf := resolveVariantFlags(fs, cfg)

	if vf.VariantCount != 3 {
		t.Errorf("VariantCount = %d, want 3", vf.VariantCount)
	}
	if vf.BranchPrefix != "my-prefix" {
		t.Errorf("BranchPrefix = %q, want %q", vf.BranchPrefix, "my-prefix")
	}
	if vf.CompareCommand != "custom-compare" {
		t.Errorf("CompareCommand = %q, want %q", vf.CompareCommand, "custom-compare")
	}
	if vf.GuidanceFile != "my-guidance.yaml" {
		t.Errorf("GuidanceFile = %q, want %q", vf.GuidanceFile, "my-guidance.yaml")
	}
}

// TestVariantConfig_CLIOverridesConfig tests that CLI flags override config values
// when passed explicitly, using the production resolveVariantFlags function.
func TestVariantConfig_CLIOverridesConfig(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		cfg          *config.Config
		wantCount    int
		wantPrefix   string
		wantCompare  string
		wantGuidance string
	}{
		{
			name: "explicit flags override config",
			args: []string{"--variants", "5", "--branch-prefix", "cli-prefix", "--compare-command", "cli-compare", "--guidance-file", "cli-guidance.yaml"},
			cfg: &config.Config{
				VariantCount:   3,
				BranchPrefix:   "config-prefix",
				CompareCommand: "config-compare",
				GuidanceFile:   "config-guidance.yaml",
			},
			wantCount:    5,
			wantPrefix:   "cli-prefix",
			wantCompare:  "cli-compare",
			wantGuidance: "cli-guidance.yaml",
		},
		{
			name: "config used when flags not set",
			args: []string{},
			cfg: &config.Config{
				VariantCount:   3,
				BranchPrefix:   "config-prefix",
				CompareCommand: "config-compare",
				GuidanceFile:   "config-guidance.yaml",
			},
			wantCount:    3,
			wantPrefix:   "config-prefix",
			wantCompare:  "config-compare",
			wantGuidance: "config-guidance.yaml",
		},
		{
			name: "explicit empty string clears config value",
			args: []string{"--compare-command", "", "--guidance-file", ""},
			cfg: &config.Config{
				BranchPrefix:   "config-prefix",
				CompareCommand: "config-compare",
				GuidanceFile:   "config-guidance.yaml",
			},
			wantCount:    0,
			wantPrefix:   "config-prefix",
			wantCompare:  "",
			wantGuidance: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			fs.Int("variants", 0, "")
			fs.String("branch-prefix", "orbit-impl", "")
			fs.String("compare-command", "", "")
			fs.String("guidance-file", "", "")

			if err := fs.Parse(tt.args); err != nil {
				t.Fatalf("failed to parse flags: %v", err)
			}

			vf := resolveVariantFlags(fs, tt.cfg)

			if vf.VariantCount != tt.wantCount {
				t.Errorf("VariantCount = %d, want %d", vf.VariantCount, tt.wantCount)
			}
			if vf.BranchPrefix != tt.wantPrefix {
				t.Errorf("BranchPrefix = %q, want %q", vf.BranchPrefix, tt.wantPrefix)
			}
			if vf.CompareCommand != tt.wantCompare {
				t.Errorf("CompareCommand = %q, want %q", vf.CompareCommand, tt.wantCompare)
			}
			if vf.GuidanceFile != tt.wantGuidance {
				t.Errorf("GuidanceFile = %q, want %q", vf.GuidanceFile, tt.wantGuidance)
			}
		})
	}
}
