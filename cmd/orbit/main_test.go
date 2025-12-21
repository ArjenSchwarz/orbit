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

func TestResolveCommands(t *testing.T) {
	tests := map[string]struct {
		cfg             *config.Config
		commandFlag     string
		postCommandFlag string
		noPostCommand   bool
		wantCommand     string
		wantPostCommand string
	}{
		"defaults from config": {
			cfg: &config.Config{
				Command:     config.DefaultCommand,
				PostCommand: config.DefaultPostCommand,
			},
			wantCommand:     config.DefaultCommand,
			wantPostCommand: config.DefaultPostCommand,
		},
		"command flag overrides config": {
			cfg: &config.Config{
				Command:     "config command",
				PostCommand: "config post command",
			},
			commandFlag:     "flag command",
			wantCommand:     "flag command",
			wantPostCommand: "config post command",
		},
		"post-command flag overrides config": {
			cfg: &config.Config{
				Command:     "config command",
				PostCommand: "config post command",
			},
			postCommandFlag: "flag post command",
			wantCommand:     "config command",
			wantPostCommand: "flag post command",
		},
		"no-post-command flag disables": {
			cfg: &config.Config{
				Command:     "config command",
				PostCommand: "config post command",
			},
			noPostCommand:   true,
			wantCommand:     "config command",
			wantPostCommand: "",
		},
		"no-post-command flag overrides post-command flag": {
			cfg: &config.Config{
				Command:     "config command",
				PostCommand: "config post command",
			},
			postCommandFlag: "flag post command",
			noPostCommand:   true,
			wantCommand:     "config command",
			wantPostCommand: "",
		},
		"both flags override config": {
			cfg: &config.Config{
				Command:     "config command",
				PostCommand: "config post command",
			},
			commandFlag:     "flag command",
			postCommandFlag: "flag post command",
			wantCommand:     "flag command",
			wantPostCommand: "flag post command",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			gotCommand, gotPostCommand := resolveCommands(tc.cfg, tc.commandFlag, tc.postCommandFlag, tc.noPostCommand)

			if gotCommand != tc.wantCommand {
				t.Errorf("command: got %q, want %q", gotCommand, tc.wantCommand)
			}
			if gotPostCommand != tc.wantPostCommand {
				t.Errorf("postCommand: got %q, want %q", gotPostCommand, tc.wantPostCommand)
			}
		})
	}
}
