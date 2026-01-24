package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestConsolidateCommand_FlagParsing tests flag parsing and validation.
func TestConsolidateCommand_FlagParsing(t *testing.T) {
	tests := map[string]struct {
		args    []string
		wantErr string
	}{
		"missing variant flag": {
			args:    []string{"my-spec"},
			wantErr: "--variant is required",
		},
		"zero variant ID": {
			args:    []string{"--variant", "0", "my-spec"},
			wantErr: "--variant is required and must be a positive integer",
		},
		"negative variant ID": {
			args:    []string{"--variant", "-1", "my-spec"},
			wantErr: "--variant is required and must be a positive integer",
		},
		"rollback does not require variant": {
			args:    []string{"--rollback", "my-spec"},
			wantErr: "no variant run found for spec", // Expected to fail at spec lookup, not flag validation
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := consolidateCommand(tc.args)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// TestConsolidateCommand_SpecAutoDetection tests spec auto-detection from branch name.
func TestConsolidateCommand_SpecAutoDetection(t *testing.T) {
	// This test verifies that when spec name is omitted, the command attempts
	// to auto-detect from git branch. We can't easily test successful detection
	// without a git repo, but we can verify the error path.

	// Create a temp directory that is NOT a git repo
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

	// Try to run without spec name - should fail trying to detect from branch
	err = consolidateCommand([]string{"--variant", "1"})
	if err == nil {
		t.Fatal("expected error when spec name not provided and not in git repo")
	}

	// Error should mention git branch detection failure
	errStr := err.Error()
	if !strings.Contains(errStr, "git") && !strings.Contains(errStr, "branch") && !strings.Contains(errStr, "spec") {
		t.Errorf("error should mention git/branch/spec detection, got: %v", err)
	}
}

// TestConsolidateCommand_RollbackModeValidation tests that --rollback mode
// does not require --variant flag.
func TestConsolidateCommand_RollbackModeValidation(t *testing.T) {
	// Create temp directory structure for spec
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

	// With --rollback, we should NOT get a "variant required" error
	// Instead, we should get a "no variant run found" error (which is expected
	// since we don't have a real spec setup)
	err = consolidateCommand([]string{"--rollback", "test-spec"})
	if err == nil {
		t.Fatal("expected error (no variant run), got nil")
	}

	// Should NOT fail on variant validation
	if strings.Contains(err.Error(), "--variant is required") {
		t.Errorf("--rollback should not require --variant flag, got: %v", err)
	}

	// Should fail on variant metadata lookup instead
	if !strings.Contains(err.Error(), "no variant run found") {
		t.Errorf("expected 'no variant run found' error, got: %v", err)
	}
}

// TestConsolidateCommand_VariantNotFound tests error message when variant doesn't exist.
func TestConsolidateCommand_VariantNotFound(t *testing.T) {
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

	// Initialize a git repo (required by the command)
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to init git repo: %v\n%s", err, out)
	}

	// Create a minimal spec directory structure with variants.json
	specDir := filepath.Join(tmpDir, "specs", "test-feature", ".orbit")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatalf("failed to create spec directory: %v", err)
	}

	// Create a minimal variants.json with variant 1
	variantsJSON := `{
		"spec_name": "test-feature",
		"variants": [
			{"id": 1, "status": "completed", "worktree_path": "/tmp/fake/path"}
		]
	}`
	if err := os.WriteFile(filepath.Join(specDir, "variants.json"), []byte(variantsJSON), 0644); err != nil {
		t.Fatalf("failed to create variants.json: %v", err)
	}

	// Request variant 99 which doesn't exist
	err = consolidateCommand([]string{"--variant", "99", "test-feature"})
	if err == nil {
		t.Fatal("expected error for non-existent variant")
	}

	// Error should mention the variant was not found
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

// TestTruncateString tests the truncateString helper function.
func TestTruncateString(t *testing.T) {
	tests := map[string]struct {
		input  string
		maxLen int
		want   string
	}{
		"short string unchanged": {
			input:  "hello",
			maxLen: 10,
			want:   "hello",
		},
		"exact length unchanged": {
			input:  "hello",
			maxLen: 5,
			want:   "hello",
		},
		"long string truncated": {
			input:  "hello world",
			maxLen: 8,
			want:   "hello...",
		},
		"very short maxLen": {
			input:  "hello world",
			maxLen: 4,
			want:   "h...",
		},
		"maxLen 3 no ellipsis": {
			input:  "hello world",
			maxLen: 3,
			want:   "hel",
		},
		"maxLen 2 truncates": {
			input:  "hello",
			maxLen: 2,
			want:   "he",
		},
		"maxLen 1 truncates": {
			input:  "hello",
			maxLen: 1,
			want:   "h",
		},
		"maxLen 0 returns empty": {
			input:  "hello",
			maxLen: 0,
			want:   "",
		},
		"negative maxLen returns empty": {
			input:  "hello",
			maxLen: -5,
			want:   "",
		},
		"maxLen 3 short string fits": {
			input:  "hi",
			maxLen: 3,
			want:   "hi",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := truncateString(tc.input, tc.maxLen)
			if got != tc.want {
				t.Errorf("truncateString(%q, %d) = %q, want %q", tc.input, tc.maxLen, got, tc.want)
			}
		})
	}
}

// TestIsAutomatedEnvironment tests CI environment detection.
func TestIsAutomatedEnvironment(t *testing.T) {
	// Save and restore environment
	ciVars := []string{"CI", "GITHUB_ACTIONS", "GITLAB_CI", "JENKINS_URL", "BUILDKITE", "CIRCLECI", "TRAVIS"}
	savedValues := make(map[string]string)
	for _, v := range ciVars {
		savedValues[v] = os.Getenv(v)
		_ = os.Unsetenv(v)
	}
	t.Cleanup(func() {
		for v, val := range savedValues {
			if val != "" {
				_ = os.Setenv(v, val)
			} else {
				_ = os.Unsetenv(v)
			}
		}
	})

	// Test with no CI variables set
	if isAutomatedEnvironment() {
		t.Error("should return false when no CI variables are set")
	}

	// Test with CI variable set
	_ = os.Setenv("CI", "true")
	if !isAutomatedEnvironment() {
		t.Error("should return true when CI=true")
	}
	_ = os.Unsetenv("CI")

	// Test with GITHUB_ACTIONS variable set
	_ = os.Setenv("GITHUB_ACTIONS", "true")
	if !isAutomatedEnvironment() {
		t.Error("should return true when GITHUB_ACTIONS=true")
	}
}
