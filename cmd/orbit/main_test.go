package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectTasksFile(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	t.Cleanup(func() {
		os.Chdir(originalWd)
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
		os.Chdir(originalWd)
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
		os.Chdir(originalWd)
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
