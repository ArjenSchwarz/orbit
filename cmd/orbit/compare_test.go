package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arjenschwarz/orbit/internal/variants"
)

// TestRunCompare_HonorsCancelledContext is a regression test for T-683.
//
// Bug: orbit compare ignored shutdown cancellation. The subcommand built
// its diff-gathering and agent contexts from context.Background(), so a
// SIGINT (Ctrl+C) during long-running git diff collection or agent calls
// could not stop the work — orbit compare would appear hung.
//
// Fix: compareCommand now installs a signal-aware context (SIGINT/SIGTERM)
// that propagates into GatherAll and the comparison agent. This test drives
// the extracted runCompare helper with a pre-cancelled context and verifies
// the function returns context.Canceled before doing any heavy work.
func TestRunCompare_HonorsCancelledContext(t *testing.T) {
	tmpDir := t.TempDir()

	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalWd)
	})
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir to temp dir: %v", err)
	}

	// Initialise a git repo so getRepoRoot succeeds.
	runCmd(t, tmpDir, "git", "init")
	runCmd(t, tmpDir, "git", "config", "user.email", "test@example.com")
	runCmd(t, tmpDir, "git", "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# Test"), 0o644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}
	runCmd(t, tmpDir, "git", "add", "README.md")
	runCmd(t, tmpDir, "git", "commit", "-m", "Initial commit")

	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = tmpDir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	baseCommit := strings.TrimSpace(string(out))

	// Build a minimal variants.json with two completed variants. We do not
	// need real worktrees because the function should return before touching
	// the git CLI.
	specName := "test-feature"
	specOrbitDir := filepath.Join(tmpDir, "specs", specName, ".orbit")
	if err := os.MkdirAll(specOrbitDir, 0o755); err != nil {
		t.Fatalf("failed to create spec .orbit dir: %v", err)
	}

	metadata := variants.VariantsMetadata{
		RunID:          "test-run-cancel",
		BaseCommit:     baseCommit,
		OriginalBranch: "main",
		StartedAt:      time.Now(),
		Variants: []*variants.Variant{
			{
				ID:           1,
				Branch:       "orbit-impl-1-test-feature",
				WorktreePath: filepath.Join(tmpDir, "wt1"),
				Status:       variants.StatusCompleted,
			},
			{
				ID:           2,
				Branch:       "orbit-impl-2-test-feature",
				WorktreePath: filepath.Join(tmpDir, "wt2"),
				Status:       variants.StatusCompleted,
			},
		},
	}
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(specOrbitDir, "variants.json"), data, 0o644); err != nil {
		t.Fatalf("write variants.json: %v", err)
	}

	// Provide a .orbit.yaml so RequireConfigFile passes if we ever reach the
	// agent path. With a cancelled context we should not get there, but the
	// file makes the test robust to other early-validation paths.
	if err := os.WriteFile(filepath.Join(tmpDir, ".orbit.yaml"), []byte("agent: claude-code\n"), 0o644); err != nil {
		t.Fatalf("write .orbit.yaml: %v", err)
	}

	// Cancel the context before invoking runCompare. The function must
	// observe cancellation and bail out before doing diff or agent work.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Use --from-file pointing at a non-existent path so that even if the
	// guard was missing, we would not actually invoke the agent. The
	// regression we are guarding against is *no early exit at all* — pre-fix
	// the function would happily call GatherAll with a Background context.
	err = runCompare(ctx, []string{specName})
	if err == nil {
		t.Fatal("expected error from runCompare with cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected error wrapping context.Canceled, got: %v", err)
	}

	// Report directory must NOT have been created. If it was, that means the
	// function progressed past the cancellation guard and into report work.
	reportDir := filepath.Join(tmpDir, "specs", specName, "comparison-report")
	if _, statErr := os.Stat(reportDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("comparison-report directory should not exist after cancellation; stat err: %v", statErr)
	}
}

// runCmd is a small test helper that runs a command in a directory and
// fails the test if it exits non-zero.
func runCmd(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
}

// TestReadSpecContext_UppercaseTasksFile verifies that readSpecContext picks up
// TASKS.md when tasks.md does not exist (T-1008).
func TestReadSpecContext_UppercaseTasksFile(t *testing.T) {
	specDir := t.TempDir()

	// Create only TASKS.md (uppercase)
	if err := os.WriteFile(filepath.Join(specDir, "TASKS.md"), []byte("# Tasks\n- task 1"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := readSpecContext(specDir)
	if !strings.Contains(result, "# Tasks") {
		t.Fatalf("expected TASKS.md content in spec context, got: %q", result)
	}
}
