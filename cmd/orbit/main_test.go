package main

import (
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

func TestReorderArgs(t *testing.T) {
	tests := map[string]struct {
		input    []string
		expected []string
	}{
		"flags before positional": {
			input:    []string{"--variant", "1", "my-spec"},
			expected: []string{"--variant", "1", "my-spec"},
		},
		"positional before flags": {
			input:    []string{"my-spec", "--variant", "1"},
			expected: []string{"--variant", "1", "my-spec"},
		},
		"mixed order": {
			input:    []string{"my-spec", "--variant", "1", "--force"},
			expected: []string{"--variant", "1", "--force", "my-spec"},
		},
		"boolean flag between positional and value flag": {
			input:    []string{"my-spec", "--force", "--variant", "1"},
			expected: []string{"--force", "--variant", "1", "my-spec"},
		},
		"flag with equals": {
			input:    []string{"my-spec", "--variant=1"},
			expected: []string{"--variant=1", "my-spec"},
		},
		"only flags": {
			input:    []string{"--variant", "1", "--force"},
			expected: []string{"--variant", "1", "--force"},
		},
		"only positional": {
			input:    []string{"my-spec", "another-arg"},
			expected: []string{"my-spec", "another-arg"},
		},
		"empty": {
			input:    []string{},
			expected: []string{},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := reorderArgs(tc.input)
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
