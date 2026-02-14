package main

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arjenschwarz/orbit/internal/logs"
	"github.com/arjenschwarz/orbit/internal/status"
	"github.com/arjenschwarz/orbit/internal/variants"
)

// TestStatusCommand_NoVariantsJSON tests that the command returns an error
// when variants.json does not exist (requirement 6.6).
func TestStatusCommand_NoVariantsJSON(t *testing.T) {
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

	// Create spec directory without variants.json
	specDir := filepath.Join(tmpDir, "specs", "test-feature", ".orbit")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatalf("failed to create spec directory: %v", err)
	}

	// Initialize git repo (required by the command)
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to init git repo: %v\n%s", err, out)
	}

	// Run status command - should fail with exit code 1 per requirement 6.6
	err = statusCommand([]string{"test-feature"})
	if err == nil {
		t.Fatal("expected error when variants.json does not exist")
	}

	// Verify error message mentions the missing variants.json
	if !strings.Contains(err.Error(), "variants.json") {
		t.Errorf("expected error to mention variants.json, got: %v", err)
	}
}

// TestStatusCommand_Integration tests the complete status command flow
// with a fully set up worktree structure.
func TestStatusCommand_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Check if rune is available
	if _, err := exec.LookPath("rune"); err != nil {
		t.Skip("rune CLI not installed, skipping integration test")
	}

	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalWd)
	})

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to init git repo: %v\n%s", err, out)
	}

	// Configure git user for commits
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to configure git user.email: %v\n%s", err, out)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to configure git user.name: %v\n%s", err, out)
	}

	// Create initial commit (required for worktree operations)
	if err := os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# Test"), 0644); err != nil {
		t.Fatalf("failed to create README: %v", err)
	}

	cmd = exec.Command("git", "add", "README.md")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to add README: %v\n%s", err, out)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to commit: %v\n%s", err, out)
	}

	// Get the commit hash
	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = tmpDir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to get HEAD commit: %v", err)
	}
	baseCommit := strings.TrimSpace(string(out))

	// Create the spec directory structure
	specName := "test-feature"
	specOrbitDir := filepath.Join(tmpDir, "specs", specName, ".orbit")
	if err := os.MkdirAll(specOrbitDir, 0755); err != nil {
		t.Fatalf("failed to create spec .orbit directory: %v", err)
	}

	// Create worktree directory (simulated - not a real git worktree for simplicity)
	worktreePath := filepath.Join(specOrbitDir, "worktrees", "orbit-impl-1-test-feature")
	worktreeSpecOrbitDir := filepath.Join(worktreePath, "specs", specName, ".orbit")
	if err := os.MkdirAll(worktreeSpecOrbitDir, 0755); err != nil {
		t.Fatalf("failed to create worktree spec .orbit directory: %v", err)
	}

	// Create variants.json with a running variant
	metadata := variants.VariantsMetadata{
		RunID:          "test-run-123",
		BaseCommit:     baseCommit,
		OriginalBranch: "main",
		StartedAt:      time.Now(),
		Variants: []*variants.Variant{
			{
				ID:           1,
				Branch:       "orbit-impl-1-test-feature",
				WorktreePath: worktreePath,
				Status:       variants.StatusRunning,
				AgentType:    "codex", // Use non-Claude agent to avoid transcript path issues
			},
		},
	}

	variantsData, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal variants.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(specOrbitDir, "variants.json"), variantsData, 0644); err != nil {
		t.Fatalf("failed to write variants.json: %v", err)
	}

	// Create tasks.md in the worktree
	tasksContent := `# Test Feature

## Phase 1: Setup

- [x] 1. Create initial structure
  - Set up directories
- [ ] 2. Add configuration
  - Create config file
`
	if err := os.WriteFile(filepath.Join(worktreePath, "specs", specName, "tasks.md"), []byte(tasksContent), 0644); err != nil {
		t.Fatalf("failed to write tasks.md: %v", err)
	}

	// Create summary.json in the worktree spec's .orbit directory
	summary := logs.Summary{
		CurrentPhase: &logs.PhaseState{
			Phase:     1,
			SessionID: "test-session-456",
		},
	}
	summaryData, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("failed to marshal summary.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktreeSpecOrbitDir, "summary.json"), summaryData, 0644); err != nil {
		t.Fatalf("failed to write summary.json: %v", err)
	}

	// Initialize git in the worktree directory (simulated worktree)
	cmd = exec.Command("git", "init")
	cmd.Dir = worktreePath
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to init worktree git: %v\n%s", err, out)
	}

	// Configure git user for worktree
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = worktreePath
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to configure worktree git user.email: %v\n%s", err, out)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = worktreePath
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to configure worktree git user.name: %v\n%s", err, out)
	}

	// Create a commit in the worktree
	if err := os.WriteFile(filepath.Join(worktreePath, "feature.go"), []byte("package feature"), 0644); err != nil {
		t.Fatalf("failed to create feature.go: %v", err)
	}

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = worktreePath
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to add files in worktree: %v\n%s", err, out)
	}

	cmd = exec.Command("git", "commit", "-m", "Add feature implementation")
	cmd.Dir = worktreePath
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to commit in worktree: %v\n%s", err, out)
	}

	// Change to the temp directory
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}

	// Run the status command
	err = statusCommand([]string{"test-feature"})
	if err != nil {
		t.Fatalf("status command failed: %v", err)
	}

	// The command ran successfully - the output verification happens implicitly
	// through the render functions. A more thorough test would capture stdout,
	// but that would require refactoring to use io.Writer.
}

// TestStatusCommand_JSONFormat tests JSON output format.
func TestStatusCommand_JSONFormat(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalWd)
	})

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to init git repo: %v\n%s", err, out)
	}

	// Configure git user
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to configure git: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to configure git: %v\n%s", err, out)
	}

	// Create initial commit
	if err := os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# Test"), 0644); err != nil {
		t.Fatalf("failed to create README: %v", err)
	}
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to add: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "commit", "-m", "Initial")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to commit: %v\n%s", err, out)
	}

	// Get base commit
	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = tmpDir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to get HEAD: %v", err)
	}
	baseCommit := strings.TrimSpace(string(out))

	// Create spec structure with a completed variant (simpler case)
	specName := "test-feature"
	specOrbitDir := filepath.Join(tmpDir, "specs", specName, ".orbit")
	if err := os.MkdirAll(specOrbitDir, 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	metadata := variants.VariantsMetadata{
		RunID:          "test-run",
		BaseCommit:     baseCommit,
		OriginalBranch: "main",
		StartedAt:      time.Now(),
		Variants: []*variants.Variant{
			{
				ID:           1,
				Branch:       "impl-1",
				WorktreePath: "/tmp/fake",
				Status:       variants.StatusCompleted,
				AgentType:    "claude-code",
			},
		},
	}

	variantsData, _ := json.MarshalIndent(metadata, "", "  ")
	if err := os.WriteFile(filepath.Join(specOrbitDir, "variants.json"), variantsData, 0644); err != nil {
		t.Fatalf("failed to write variants.json: %v", err)
	}

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	// Run with JSON format - should not fail
	err = statusCommand([]string{"--format", "json", specName})
	if err != nil {
		t.Fatalf("status command with JSON format failed: %v", err)
	}
}

// TestStatusCommand_AutoDetectSpec tests branch-based spec auto-detection.
func TestStatusCommand_AutoDetectSpec(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalWd)
	})

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to init git repo: %v\n%s", err, out)
	}

	// Configure git
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tmpDir
	_ = cmd.Run()
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	_ = cmd.Run()

	// Create initial commit on main
	if err := os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# Test"), 0644); err != nil {
		t.Fatalf("failed to create README: %v", err)
	}
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	_ = cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "Initial")
	cmd.Dir = tmpDir
	_ = cmd.Run()

	// Create feature branch
	cmd = exec.Command("git", "checkout", "-b", "feature/my-spec")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to create branch: %v\n%s", err, out)
	}

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	// Run without spec name - should try to auto-detect from branch
	// Should fail because specs/my-spec/.orbit/variants.json doesn't exist
	err = statusCommand([]string{})
	if err == nil {
		t.Fatal("expected error when variants.json doesn't exist")
	}

	// The extracted spec name should be "my-spec" from branch "feature/my-spec"
	if !strings.Contains(err.Error(), "variants.json") {
		t.Errorf("error should mention variants.json, got: %v", err)
	}
}

// TestExtractSpecName tests the spec name extraction function.
func TestExtractSpecName(t *testing.T) {
	tests := []struct {
		branch string
		want   string
	}{
		{"feature/my-feature", "my-feature"},
		{"orbit-impl-1/enhanced-status", "enhanced-status"},
		{"main", "main"},
		{"develop", "develop"},
		{"feature/sub/deep", "sub/deep"},
	}

	for _, tt := range tests {
		t.Run(tt.branch, func(t *testing.T) {
			got := extractSpecName(tt.branch)
			if got != tt.want {
				t.Errorf("extractSpecName(%q) = %q, want %q", tt.branch, got, tt.want)
			}
		})
	}
}

// TestStatusCommand_TimestampLocalTimezone verifies that the "Started" timestamp
// in status output is displayed in local timezone, not UTC (regression test for T-64).
func TestStatusCommand_TimestampLocalTimezone(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalWd)
	})

	// Set up git repo with initial commit
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = tmpDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	if err := os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("x"), 0644); err != nil {
		t.Fatalf("failed to create README: %v", err)
	}

	cmd := exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}

	cmd = exec.Command("git", "commit", "-m", "init")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}

	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = tmpDir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to get HEAD: %v", err)
	}
	baseCommit := strings.TrimSpace(string(out))

	// Use a specific UTC time — this is how StartedAt is stored (see manager.go:237)
	utcTime := time.Date(2026, 2, 14, 6, 52, 3, 0, time.UTC)

	specOrbitDir := filepath.Join(tmpDir, "specs", "tz-test", ".orbit")
	if err := os.MkdirAll(specOrbitDir, 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	metadata := variants.VariantsMetadata{
		RunID:          "tz-test",
		BaseCommit:     baseCommit,
		OriginalBranch: "main",
		StartedAt:      utcTime,
		Variants: []*variants.Variant{
			{
				ID:        1,
				Branch:    "impl-1",
				Status:    variants.StatusCompleted,
				AgentType: "codex",
			},
		},
	}

	variantsData, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal variants.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(specOrbitDir, "variants.json"), variantsData, 0644); err != nil {
		t.Fatalf("failed to write variants.json: %v", err)
	}

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	// Capture JSON output by redirecting stdout
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = w

	cmdErr := statusCommand([]string{"--format", "json", "tz-test"})

	_ = w.Close()
	os.Stdout = oldStdout

	captured, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("failed to read captured output: %v", err)
	}

	if cmdErr != nil {
		t.Fatalf("status command failed: %v", cmdErr)
	}

	var result status.StatusOutput
	if err := json.Unmarshal(captured, &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nraw: %s", err, captured)
	}

	// The timestamp should be in local timezone, not UTC
	wantLocal := utcTime.Local().Format("2006-01-02 15:04:05")
	if result.StartedAt != wantLocal {
		t.Errorf("StartedAt = %q, want %q (local timezone)", result.StartedAt, wantLocal)
	}

	// If we're not in UTC, also verify it's NOT the raw UTC representation
	_, offset := time.Now().Zone()
	if offset != 0 {
		utcFormatted := utcTime.Format("2006-01-02 15:04:05")
		if result.StartedAt == utcFormatted {
			t.Errorf("StartedAt should be local time, but got UTC: %q", result.StartedAt)
		}
	}
}

// TestBuildVariantHeader tests the variant header building function.
func TestBuildVariantHeader(t *testing.T) {
	tests := []struct {
		name     string
		id       int
		branch   string
		stat     string
		gitState string
		expected string
	}{
		{
			name:     "running with clean state",
			id:       1,
			branch:   "impl-1",
			stat:     "running",
			gitState: "clean",
			expected: "Variant 1: impl-1 [running (clean)]",
		},
		{
			name:     "running with dirty state",
			id:       2,
			branch:   "impl-2",
			stat:     "running",
			gitState: "dirty",
			expected: "Variant 2: impl-2 [running (dirty)]",
		},
		{
			name:     "failed without git state",
			id:       3,
			branch:   "impl-3",
			stat:     "failed",
			gitState: "",
			expected: "Variant 3: impl-3 [failed]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vo := &status.VariantOutput{
				ID:       tt.id,
				Branch:   tt.branch,
				Status:   tt.stat,
				GitState: tt.gitState,
			}
			got := buildVariantHeader(vo)
			if got != tt.expected {
				t.Errorf("buildVariantHeader() = %q, want %q", got, tt.expected)
			}
		})
	}
}
