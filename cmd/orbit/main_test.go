package main

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/arjenschwarz/orbit/internal/config"
)

func TestDetectTasksFile(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalWd)
	})

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}

	// Create specs directory with tasks file
	specsDir := filepath.Join(tmpDir, "specs", "my-feature")
	if err := os.MkdirAll(specsDir, 0755); err != nil {
		t.Fatalf("failed to create specs directory: %v", err)
	}

	tasksPath := filepath.Join(specsDir, "tasks.md")
	if err := os.WriteFile(tasksPath, []byte("# Tasks"), 0644); err != nil {
		t.Fatalf("failed to create tasks file: %v", err)
	}

	tests := map[string]struct {
		branchName string
		wantPath   string
		wantErr    bool
	}{
		"feature prefix": {
			branchName: "feature/my-feature",
			wantPath:   filepath.Join("specs", "my-feature", "tasks.md"),
			wantErr:    false,
		},
		"no prefix": {
			branchName: "my-feature",
			wantPath:   filepath.Join("specs", "my-feature", "tasks.md"),
			wantErr:    false,
		},
		"hotfix prefix": {
			branchName: "hotfix/my-feature",
			wantPath:   filepath.Join("specs", "my-feature", "tasks.md"),
			wantErr:    false,
		},
		"bugfix prefix": {
			branchName: "bugfix/my-feature",
			wantPath:   filepath.Join("specs", "my-feature", "tasks.md"),
			wantErr:    false,
		},
		"fix prefix": {
			branchName: "fix/my-feature",
			wantPath:   filepath.Join("specs", "my-feature", "tasks.md"),
			wantErr:    false,
		},
		"feat prefix": {
			branchName: "feat/my-feature",
			wantPath:   filepath.Join("specs", "my-feature", "tasks.md"),
			wantErr:    false,
		},
		"specs prefix": {
			branchName: "specs/my-feature",
			wantPath:   filepath.Join("specs", "my-feature", "tasks.md"),
			wantErr:    false,
		},
		"non-existent feature": {
			branchName: "feature/non-existent",
			wantPath:   "",
			wantErr:    true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := detectTasksFile(tc.branchName)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantPath {
				t.Errorf("got %q, want %q", got, tc.wantPath)
			}
		})
	}
}

func TestDetectTasksFile_UppercaseTasks(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalWd)
	})

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}

	// Create specs directory with TASKS.md (uppercase)
	specsDir := filepath.Join(tmpDir, "specs", "uppercase-test")
	if err := os.MkdirAll(specsDir, 0755); err != nil {
		t.Fatalf("failed to create specs directory: %v", err)
	}

	tasksPath := filepath.Join(specsDir, "TASKS.md")
	if err := os.WriteFile(tasksPath, []byte("# Tasks"), 0644); err != nil {
		t.Fatalf("failed to create tasks file: %v", err)
	}

	got, err := detectTasksFile("feature/uppercase-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// On case-insensitive filesystems (macOS), tasks.md may be returned
	// even when TASKS.md was created. Just verify the directory is correct.
	wantDir := filepath.Join("specs", "uppercase-test")
	if filepath.Dir(got) != wantDir {
		t.Errorf("got dir %q, want %q", filepath.Dir(got), wantDir)
	}
}

func TestDetectTasksFile_FullBranchNameFallback(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalWd)
	})

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}

	// Create specs directory using full branch name (with prefix)
	specsDir := filepath.Join(tmpDir, "specs", "feature", "nested-feature")
	if err := os.MkdirAll(specsDir, 0755); err != nil {
		t.Fatalf("failed to create specs directory: %v", err)
	}

	tasksPath := filepath.Join(specsDir, "tasks.md")
	if err := os.WriteFile(tasksPath, []byte("# Tasks"), 0644); err != nil {
		t.Fatalf("failed to create tasks file: %v", err)
	}

	got, err := detectTasksFile("feature/nested-feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := filepath.Join("specs", "feature", "nested-feature", "tasks.md")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// newConsolidateFlagSet builds a FlagSet that mirrors the flags registered by
// the consolidate subcommand. Tests use it to exercise reorderArgs against a
// realistic flag definition without coupling to the command's logic.
func newConsolidateFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("consolidate", flag.ContinueOnError)
	_ = fs.Int("variant", 0, "")
	_ = fs.Bool("allow-dirty", false, "")
	_ = fs.String("prompt", "", "")
	_ = fs.Bool("rollback", false, "")
	_ = fs.Bool("force", false, "")
	return fs
}

// newFinalizeFlagSet mirrors the finalize subcommand's flag definitions.
func newFinalizeFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("finalize", flag.ContinueOnError)
	_ = fs.Int("variant", 0, "")
	_ = fs.Bool("force", false, "")
	return fs
}

func TestReorderArgs(t *testing.T) {
	tests := map[string]struct {
		fs       *flag.FlagSet
		input    []string
		expected []string
	}{
		"flags before positional": {
			fs:       newConsolidateFlagSet(),
			input:    []string{"--variant", "1", "my-spec"},
			expected: []string{"--variant", "1", "my-spec"},
		},
		"positional before flags": {
			fs:       newConsolidateFlagSet(),
			input:    []string{"my-spec", "--variant", "1"},
			expected: []string{"--variant", "1", "my-spec"},
		},
		"mixed order": {
			fs:       newConsolidateFlagSet(),
			input:    []string{"my-spec", "--variant", "1", "--force"},
			expected: []string{"--variant", "1", "--force", "my-spec"},
		},
		"boolean flag between positional and value flag": {
			fs:       newConsolidateFlagSet(),
			input:    []string{"my-spec", "--force", "--variant", "1"},
			expected: []string{"--force", "--variant", "1", "my-spec"},
		},
		"flag with equals": {
			fs:       newConsolidateFlagSet(),
			input:    []string{"my-spec", "--variant=1"},
			expected: []string{"--variant=1", "my-spec"},
		},
		// Regression guard for the i++; continue branch in reorderArgs:
		// "--flag=value" must not absorb the following positional argument.
		"equals-flag before positional": {
			fs:       newConsolidateFlagSet(),
			input:    []string{"--variant=1", "my-feature"},
			expected: []string{"--variant=1", "my-feature"},
		},
		// Multiple boolean flags in sequence followed by a positional. Each
		// bool must keep its hands off the following token.
		"sequential bool flags then positional": {
			fs:       newConsolidateFlagSet(),
			input:    []string{"--allow-dirty", "--rollback", "my-spec"},
			expected: []string{"--allow-dirty", "--rollback", "my-spec"},
		},
		"only flags": {
			fs:       newConsolidateFlagSet(),
			input:    []string{"--variant", "1", "--force"},
			expected: []string{"--variant", "1", "--force"},
		},
		"only positional": {
			fs:       newConsolidateFlagSet(),
			input:    []string{"my-spec", "another-arg"},
			expected: []string{"my-spec", "another-arg"},
		},
		"empty": {
			fs:       newConsolidateFlagSet(),
			input:    []string{},
			expected: []string{},
		},
		// Regression tests for T-653: boolean flags must not consume the
		// following positional argument.
		"T-653: --rollback then positional": {
			fs:       newConsolidateFlagSet(),
			input:    []string{"--rollback", "my-feature"},
			expected: []string{"--rollback", "my-feature"},
		},
		"T-653: --force then positional then value flag (finalize)": {
			fs:       newFinalizeFlagSet(),
			input:    []string{"--force", "my-feature", "--variant", "1"},
			expected: []string{"--force", "--variant", "1", "my-feature"},
		},
		"T-653: --allow-dirty then positional": {
			fs:       newConsolidateFlagSet(),
			input:    []string{"--allow-dirty", "my-feature", "--variant", "2"},
			expected: []string{"--allow-dirty", "--variant", "2", "my-feature"},
		},
		"T-653: short bool flag then positional": {
			fs: func() *flag.FlagSet {
				fs := flag.NewFlagSet("t", flag.ContinueOnError)
				_ = fs.Bool("v", false, "")
				_ = fs.Int("variant", 0, "")
				return fs
			}(),
			input:    []string{"-v", "my-feature", "--variant", "1"},
			expected: []string{"-v", "--variant", "1", "my-feature"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := reorderArgs(tc.fs, tc.input)
			if len(got) != len(tc.expected) {
				t.Errorf("got %v, want %v", got, tc.expected)
				return
			}
			for i := range got {
				if got[i] != tc.expected[i] {
					t.Errorf("got %v, want %v", got, tc.expected)
					return
				}
			}
		})
	}
}

// TestReorderArgs_NilFlagSet verifies the legacy heuristic still applies when
// no FlagSet is provided. This preserves backward compatibility for any
// future callers that don't have a FlagSet handy.
func TestReorderArgs_NilFlagSet(t *testing.T) {
	tests := map[string]struct {
		input    []string
		expected []string
	}{
		"nil fs falls back to consume-next heuristic": {
			input:    []string{"my-spec", "--variant", "1"},
			expected: []string{"--variant", "1", "my-spec"},
		},
		"nil fs gobbles next token even for what would be a bool": {
			// Without a FlagSet the function cannot know --rollback is bool,
			// so the legacy heuristic consumes "my-spec" as if it were
			// --rollback's value. The combined output here is coincidentally
			// correct because "my-spec" is appended to flags rather than
			// positional, but a value flag following the absorbed token would
			// be lost — e.g. ["--rollback", "my-spec", "--variant", "1"]
			// would drop --variant entirely. Callers should always pass a
			// FlagSet when bool flags exist.
			input:    []string{"--rollback", "my-spec"},
			expected: []string{"--rollback", "my-spec"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := reorderArgs(nil, tc.input)
			if len(got) != len(tc.expected) {
				t.Errorf("got %v, want %v", got, tc.expected)
				return
			}
			for i := range got {
				if got[i] != tc.expected[i] {
					t.Errorf("got %v, want %v", got, tc.expected)
				}
			}
		})
	}
}

// TestReorderArgs_ConsolidatePositionalAfterBoolFlag verifies the end-to-end
// behaviour reported in T-653: invocations like
// `orbit consolidate --rollback my-feature` and
// `orbit consolidate --allow-dirty my-feature --variant 1` must parse with
// `my-feature` as the positional argument and the bool flag set, without
// dropping the value flag (`--variant 1`).
func TestReorderArgs_ConsolidatePositionalAfterBoolFlag(t *testing.T) {
	tests := map[string]struct {
		args         []string
		wantSpec     string
		wantVariant  int
		wantRollback bool
		wantDirty    bool
		wantForce    bool
	}{
		"--rollback then spec name": {
			args:         []string{"--rollback", "my-feature"},
			wantSpec:     "my-feature",
			wantRollback: true,
		},
		"--allow-dirty then spec then --variant": {
			args:        []string{"--allow-dirty", "my-feature", "--variant", "1"},
			wantSpec:    "my-feature",
			wantVariant: 1,
			wantDirty:   true,
		},
		"--force then spec then --variant": {
			args:        []string{"--force", "my-feature", "--variant", "2"},
			wantSpec:    "my-feature",
			wantVariant: 2,
			wantForce:   true,
		},
		"spec then --rollback (already worked)": {
			args:         []string{"my-feature", "--rollback"},
			wantSpec:     "my-feature",
			wantRollback: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fs := flag.NewFlagSet("consolidate", flag.ContinueOnError)
			variant := fs.Int("variant", 0, "")
			allowDirty := fs.Bool("allow-dirty", false, "")
			rollback := fs.Bool("rollback", false, "")
			force := fs.Bool("force", false, "")

			if err := fs.Parse(reorderArgs(fs, tc.args)); err != nil {
				t.Fatalf("parse failed: %v", err)
			}

			if got := fs.Arg(0); got != tc.wantSpec {
				t.Errorf("spec: got %q, want %q", got, tc.wantSpec)
			}
			if *variant != tc.wantVariant {
				t.Errorf("variant: got %d, want %d", *variant, tc.wantVariant)
			}
			if *rollback != tc.wantRollback {
				t.Errorf("rollback: got %v, want %v", *rollback, tc.wantRollback)
			}
			if *allowDirty != tc.wantDirty {
				t.Errorf("allow-dirty: got %v, want %v", *allowDirty, tc.wantDirty)
			}
			if *force != tc.wantForce {
				t.Errorf("force: got %v, want %v", *force, tc.wantForce)
			}
		})
	}
}

// TestReorderArgs_FinalizePositionalAfterBoolFlag exercises the finalize
// subcommand's flag set — the second example called out in T-653.
func TestReorderArgs_FinalizePositionalAfterBoolFlag(t *testing.T) {
	args := []string{"--force", "my-feature", "--variant", "1"}

	fs := flag.NewFlagSet("finalize", flag.ContinueOnError)
	variant := fs.Int("variant", 0, "")
	force := fs.Bool("force", false, "")

	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if got := fs.Arg(0); got != "my-feature" {
		t.Errorf("spec: got %q, want %q", got, "my-feature")
	}
	if *variant != 1 {
		t.Errorf("variant: got %d, want 1", *variant)
	}
	if !*force {
		t.Error("force: got false, want true")
	}
}

func TestResolvePrompts(t *testing.T) {
	tests := map[string]struct {
		cfg            *config.Config
		commandFlag    string
		prePromptFlag  string
		noPrePrompt    bool
		postPromptFlag string
		noPostPrompt   bool
		wantCommand    string
		wantPrePrompt  string
		wantPostPrompt string
	}{
		"defaults from config": {
			cfg: &config.Config{
				Command:    config.DefaultCommand,
				PostPrompt: config.DefaultPostPrompt,
			},
			wantCommand:    config.DefaultCommand,
			wantPrePrompt:  "",
			wantPostPrompt: config.DefaultPostPrompt,
		},
		"command flag overrides config": {
			cfg: &config.Config{
				Command:    "config command",
				PostPrompt: "config post prompt",
			},
			commandFlag:    "flag command",
			wantCommand:    "flag command",
			wantPrePrompt:  "",
			wantPostPrompt: "config post prompt",
		},
		"pre-prompt flag overrides config": {
			cfg: &config.Config{
				Command:   "config command",
				PrePrompt: "config pre prompt",
			},
			prePromptFlag: "flag pre prompt",
			wantCommand:   "config command",
			wantPrePrompt: "flag pre prompt",
		},
		"no-pre-prompt flag disables": {
			cfg: &config.Config{
				Command:   "config command",
				PrePrompt: "config pre prompt",
			},
			noPrePrompt:   true,
			wantCommand:   "config command",
			wantPrePrompt: "",
		},
		"no-pre-prompt flag overrides pre-prompt flag": {
			cfg: &config.Config{
				Command:   "config command",
				PrePrompt: "config pre prompt",
			},
			prePromptFlag: "flag pre prompt",
			noPrePrompt:   true,
			wantCommand:   "config command",
			wantPrePrompt: "",
		},
		"post-prompt flag overrides config": {
			cfg: &config.Config{
				Command:    "config command",
				PostPrompt: "config post prompt",
			},
			postPromptFlag: "flag post prompt",
			wantCommand:    "config command",
			wantPostPrompt: "flag post prompt",
		},
		"no-post-prompt flag disables": {
			cfg: &config.Config{
				Command:    "config command",
				PostPrompt: "config post prompt",
			},
			noPostPrompt:   true,
			wantCommand:    "config command",
			wantPostPrompt: "",
		},
		"no-post-prompt flag overrides post-prompt flag": {
			cfg: &config.Config{
				Command:    "config command",
				PostPrompt: "config post prompt",
			},
			postPromptFlag: "flag post prompt",
			noPostPrompt:   true,
			wantCommand:    "config command",
			wantPostPrompt: "",
		},
		"all flags override config": {
			cfg: &config.Config{
				Command:    "config command",
				PrePrompt:  "config pre prompt",
				PostPrompt: "config post prompt",
			},
			commandFlag:    "flag command",
			prePromptFlag:  "flag pre prompt",
			postPromptFlag: "flag post prompt",
			wantCommand:    "flag command",
			wantPrePrompt:  "flag pre prompt",
			wantPostPrompt: "flag post prompt",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			gotCommand, gotPrePrompt, gotPostPrompt := resolvePrompts(tc.cfg, tc.commandFlag, tc.prePromptFlag, tc.noPrePrompt, tc.postPromptFlag, tc.noPostPrompt)

			if gotCommand != tc.wantCommand {
				t.Errorf("command: got %q, want %q", gotCommand, tc.wantCommand)
			}
			if gotPrePrompt != tc.wantPrePrompt {
				t.Errorf("prePrompt: got %q, want %q", gotPrePrompt, tc.wantPrePrompt)
			}
			if gotPostPrompt != tc.wantPostPrompt {
				t.Errorf("postPrompt: got %q, want %q", gotPostPrompt, tc.wantPostPrompt)
			}
		})
	}
}
