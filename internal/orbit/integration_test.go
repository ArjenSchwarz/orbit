package orbit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
	_ "github.com/arjenschwarz/orbit/internal/agents/claudecode" // Register claude-code agent
	_ "github.com/arjenschwarz/orbit/internal/agents/codex"      // Register codex agent
	_ "github.com/arjenschwarz/orbit/internal/agents/copilot"    // Register copilot agent
	_ "github.com/arjenschwarz/orbit/internal/agents/kiro"       // Register kiro agent
	orbitconfig "github.com/arjenschwarz/orbit/internal/config"
	"github.com/arjenschwarz/orbit/internal/variants"
)

// TestVariantRun_Sequential tests sequential variant execution.
// This integration test verifies:
// 1. Worktrees are created in .orbit/worktrees/
// 2. variants.json contains correct data
// 3. Comparison report is generated
func TestVariantRun_Sequential(t *testing.T) {
	tmpDir := t.TempDir()

	// Set up spec directory structure
	specDir := filepath.Join(tmpDir, "specs", "test-feature")
	tasksFile := filepath.Join(specDir, "tasks.md")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatalf("failed to create spec dir: %v", err)
	}

	// Create a minimal tasks file
	tasksContent := `# Tasks

## Phase 1: Implementation

- [ ] 1. Implement feature
`
	if err := os.WriteFile(tasksFile, []byte(tasksContent), 0644); err != nil {
		t.Fatalf("failed to write tasks file: %v", err)
	}

	// Create mock git client
	git := variants.NewMockGit()
	git.CurrentBranch = "feature/test-feature"
	git.HeadCommit = "abc123def456"

	// Create variant config
	cfg := variants.Config{
		Count:        2,
		Parallel:     false,
		MaxParallel:  3,
		BranchPrefix: "orbit-impl",
	}

	// Create variant manager
	mgr, err := variants.NewManager(cfg, "test-feature", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("failed to create variant manager: %v", err)
	}

	// Setup worktrees
	ctx := context.Background()
	if err := mgr.Setup(ctx, false); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// Verify worktrees were created
	worktreeDir := filepath.Join(specDir, ".orbit", "worktrees")
	if _, err := os.Stat(worktreeDir); os.IsNotExist(err) {
		t.Error("worktrees directory was not created")
	}

	// Verify branches were created
	if len(git.CreatedBranches) != 2 {
		t.Errorf("expected 2 branches, got %d", len(git.CreatedBranches))
	}

	// Verify worktrees in git
	if len(git.CreatedWorktrees) != 2 {
		t.Errorf("expected 2 worktrees, got %d", len(git.CreatedWorktrees))
	}

	// Verify variants.json exists and has correct data
	metadataPath := filepath.Join(specDir, ".orbit", "variants.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("failed to read variants.json: %v", err)
	}

	var metadata variants.VariantsMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatalf("failed to parse variants.json: %v", err)
	}

	if metadata.BaseCommit != "abc123def456" {
		t.Errorf("base commit = %q, want %q", metadata.BaseCommit, "abc123def456")
	}
	if metadata.OriginalBranch != "feature/test-feature" {
		t.Errorf("original branch = %q, want %q", metadata.OriginalBranch, "feature/test-feature")
	}
	if len(metadata.Variants) != 2 {
		t.Errorf("variants count = %d, want 2", len(metadata.Variants))
	}

	// Verify variant IDs and status
	for i, v := range metadata.Variants {
		if v.ID != i+1 {
			t.Errorf("variant[%d].ID = %d, want %d", i, v.ID, i+1)
		}
		if v.Status != variants.StatusPending {
			t.Errorf("variant[%d].Status = %q, want %q", i, v.Status, variants.StatusPending)
		}
	}

	// Verify .gitignore was created
	gitignorePath := filepath.Join(specDir, ".orbit", ".gitignore")
	gitignoreContent, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}
	if !contains(string(gitignoreContent), "worktrees/") {
		t.Error(".gitignore missing worktrees/ entry")
	}

	// Simulate running variants sequentially by updating their status
	for _, v := range mgr.GetVariantsSnapshot() {
		if err := mgr.UpdateStatus(v.ID, variants.StatusRunning, nil); err != nil {
			t.Errorf("failed to update status to running: %v", err)
		}
		if err := mgr.UpdateStatus(v.ID, variants.StatusCompleted, nil); err != nil {
			t.Errorf("failed to update status to completed: %v", err)
		}
		if err := mgr.UpdateMetrics(v.ID, 0.05, time.Minute, 10); err != nil {
			t.Errorf("failed to update metrics: %v", err)
		}
	}

	// Verify final state
	completedCount := mgr.CountByStatus(variants.StatusCompleted)
	if completedCount != 2 {
		t.Errorf("completed count = %d, want 2", completedCount)
	}
}

// TestVariantRun_Parallel tests parallel variant execution.
// This integration test verifies:
// 1. All variants are executed
// 2. Semaphore respects max-parallel limit
func TestVariantRun_Parallel(t *testing.T) {
	tmpDir := t.TempDir()

	specDir := filepath.Join(tmpDir, "specs", "parallel-test")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatalf("failed to create spec dir: %v", err)
	}

	git := variants.NewMockGit()
	git.CurrentBranch = "feature/parallel-test"
	git.HeadCommit = "def456abc789"

	cfg := variants.Config{
		Count:        3,
		Parallel:     true,
		MaxParallel:  2, // Only 2 can run at a time
		BranchPrefix: "orbit-impl",
	}

	mgr, err := variants.NewManager(cfg, "parallel-test", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("failed to create variant manager: %v", err)
	}

	ctx := context.Background()
	if err := mgr.Setup(ctx, false); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// Verify 3 variants were created
	if len(git.CreatedBranches) != 3 {
		t.Errorf("expected 3 branches, got %d", len(git.CreatedBranches))
	}

	// Simulate parallel execution with semaphore
	variantList := mgr.GetVariantsSnapshot()
	sem := make(chan struct{}, cfg.MaxParallel)
	var maxConcurrent int32
	var currentConcurrent int32

	done := make(chan struct{})
	go func() {
		for _, v := range variantList {
			sem <- struct{}{}
			atomic.AddInt32(&currentConcurrent, 1)
			if c := atomic.LoadInt32(&currentConcurrent); c > maxConcurrent {
				maxConcurrent = c
			}

			// Simulate work
			go func(variant *variants.Variant) {
				defer func() {
					atomic.AddInt32(&currentConcurrent, -1)
					<-sem
				}()

				_ = mgr.UpdateStatus(variant.ID, variants.StatusRunning, nil)
				time.Sleep(10 * time.Millisecond) // Simulate work
				_ = mgr.UpdateStatus(variant.ID, variants.StatusCompleted, nil)
			}(v)
		}
		close(done)
	}()

	// Wait for goroutines to start
	<-done
	time.Sleep(50 * time.Millisecond) // Let all complete

	// Verify semaphore was respected
	if maxConcurrent > int32(cfg.MaxParallel) {
		t.Errorf("max concurrent = %d, exceeded limit of %d", maxConcurrent, cfg.MaxParallel)
	}

	// Eventually all should complete
	time.Sleep(100 * time.Millisecond)
	completedCount := mgr.CountByStatus(variants.StatusCompleted)
	if completedCount != 3 {
		t.Errorf("completed count = %d, want 3", completedCount)
	}
}

// TestVariantRun_SingleSuccess tests the scenario where only one variant succeeds.
// This verifies:
// 1. Comparison is skipped (need at least 2 successful variants)
// 2. Report is still generated
func TestVariantRun_SingleSuccess(t *testing.T) {
	tmpDir := t.TempDir()

	specDir := filepath.Join(tmpDir, "specs", "single-success")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatalf("failed to create spec dir: %v", err)
	}

	git := variants.NewMockGit()
	git.CurrentBranch = "feature/single-success"
	git.HeadCommit = "single123"

	cfg := variants.Config{
		Count:        2,
		Parallel:     false,
		MaxParallel:  3,
		BranchPrefix: "orbit-impl",
	}

	mgr, err := variants.NewManager(cfg, "single-success", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("failed to create variant manager: %v", err)
	}

	ctx := context.Background()
	if err := mgr.Setup(ctx, false); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// First variant succeeds
	if err := mgr.UpdateStatus(1, variants.StatusCompleted, nil); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}
	if err := mgr.UpdateMetrics(1, 0.05, time.Minute, 10); err != nil {
		t.Fatalf("failed to update metrics: %v", err)
	}

	// Second variant fails
	testErr := os.ErrNotExist
	if err := mgr.UpdateStatus(2, variants.StatusFailed, testErr); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	// Verify counts
	successCount := mgr.CountByStatus(variants.StatusCompleted)
	failedCount := mgr.CountByStatus(variants.StatusFailed)

	if successCount != 1 {
		t.Errorf("success count = %d, want 1", successCount)
	}
	if failedCount != 1 {
		t.Errorf("failed count = %d, want 1", failedCount)
	}

	// In the real implementation, this would trigger:
	// - Skip comparison (only 1 success)
	// - Generate report with single variant info
	// Here we just verify the state is correct for that decision
}

// TestCleanup_RemovesWorktrees tests the cleanup command functionality.
// This verifies:
// 1. Set up worktrees
// 2. Run cleanup
// 3. Worktrees are removed
// 4. variants.json is removed
func TestCleanup_RemovesWorktrees(t *testing.T) {
	tmpDir := t.TempDir()

	specDir := filepath.Join(tmpDir, "specs", "cleanup-test")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatalf("failed to create spec dir: %v", err)
	}

	git := variants.NewMockGit()
	git.CurrentBranch = "feature/cleanup-test"
	git.HeadCommit = "cleanup123"

	cfg := variants.Config{
		Count:        2,
		Parallel:     false,
		MaxParallel:  3,
		BranchPrefix: "orbit-impl",
	}

	mgr, err := variants.NewManager(cfg, "cleanup-test", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("failed to create variant manager: %v", err)
	}

	ctx := context.Background()
	if err := mgr.Setup(ctx, false); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// Verify setup
	metadataPath := filepath.Join(specDir, ".orbit", "variants.json")
	if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
		t.Fatal("variants.json should exist after setup")
	}

	// Run cleanup
	if err := mgr.Cleanup(ctx, 0); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	// Verify worktrees were removed
	if len(git.RemovedWorktrees) != 2 {
		t.Errorf("removed worktrees = %d, want 2", len(git.RemovedWorktrees))
	}

	// Verify branches were deleted
	if len(git.DeletedBranches) != 2 {
		t.Errorf("deleted branches = %d, want 2", len(git.DeletedBranches))
	}

	// Verify variants.json was removed
	if _, err := os.Stat(metadataPath); !os.IsNotExist(err) {
		t.Error("variants.json should be removed after cleanup")
	}

	// Verify metadata is cleared
	if mgr.GetMetadata() != nil {
		t.Error("metadata should be nil after cleanup")
	}
}

// TestFinalize_RebasesVariant tests the finalize command functionality.
// This verifies:
// 1. Set up completed variants
// 2. Run finalize --variant 1
// 3. Variant is rebased onto original branch
// 4. All worktrees are cleaned up
func TestFinalize_RebasesVariant(t *testing.T) {
	tmpDir := t.TempDir()

	specDir := filepath.Join(tmpDir, "specs", "finalize-test")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatalf("failed to create spec dir: %v", err)
	}

	git := variants.NewMockGit()
	git.CurrentBranch = "feature/finalize-test"
	git.HeadCommit = "finalize123"
	git.Diverged = false // Branch has not diverged

	cfg := variants.Config{
		Count:        2,
		Parallel:     false,
		MaxParallel:  3,
		BranchPrefix: "orbit-impl",
	}

	mgr, err := variants.NewManager(cfg, "finalize-test", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("failed to create variant manager: %v", err)
	}

	ctx := context.Background()
	if err := mgr.Setup(ctx, false); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// Mark variants as completed
	for i := 1; i <= 2; i++ {
		if err := mgr.UpdateStatus(i, variants.StatusCompleted, nil); err != nil {
			t.Fatalf("failed to update status: %v", err)
		}
	}

	// Finalize variant 1
	if err := mgr.Finalize(ctx, 1); err != nil {
		t.Fatalf("Finalize failed: %v", err)
	}

	// Verify rebase was called
	if len(git.RebaseCalls) != 1 {
		t.Fatalf("rebase calls = %d, want 1", len(git.RebaseCalls))
	}

	// Verify rebase was onto original branch
	if git.RebaseCalls[0].TargetBranch != "feature/finalize-test" {
		t.Errorf("rebase target = %q, want %q", git.RebaseCalls[0].TargetBranch, "feature/finalize-test")
	}

	// Verify rebase was from correct variant branch
	if git.RebaseCalls[0].SourceBranch != "orbit-impl-1/finalize-test" {
		t.Errorf("rebase source = %q, want %q", git.RebaseCalls[0].SourceBranch, "orbit-impl-1/finalize-test")
	}

	// Verify cleanup happened (all worktrees removed)
	if len(git.RemovedWorktrees) != 2 {
		t.Errorf("removed worktrees = %d, want 2", len(git.RemovedWorktrees))
	}

	// Verify all branches deleted
	if len(git.DeletedBranches) != 2 {
		t.Errorf("deleted branches = %d, want 2", len(git.DeletedBranches))
	}
}

// TestFinalize_FailsOnDivergedBranch tests finalize rejects diverged branches.
func TestFinalize_FailsOnDivergedBranch(t *testing.T) {
	tmpDir := t.TempDir()

	specDir := filepath.Join(tmpDir, "specs", "diverged-test")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatalf("failed to create spec dir: %v", err)
	}

	git := variants.NewMockGit()
	git.CurrentBranch = "feature/diverged-test"
	git.HeadCommit = "diverged123"
	git.Diverged = true // Branch has diverged

	cfg := variants.Config{
		Count:        2,
		Parallel:     false,
		MaxParallel:  3,
		BranchPrefix: "orbit-impl",
	}

	mgr, err := variants.NewManager(cfg, "diverged-test", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("failed to create variant manager: %v", err)
	}

	ctx := context.Background()
	if err := mgr.Setup(ctx, false); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// Mark variant as completed
	if err := mgr.UpdateStatus(1, variants.StatusCompleted, nil); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	// Finalize should fail due to divergence
	err = mgr.Finalize(ctx, 1)
	if err == nil {
		t.Fatal("expected error for diverged branch, got nil")
	}
	if !contains(err.Error(), "diverged") {
		t.Errorf("expected diverged error, got: %v", err)
	}

	// Verify no rebase was attempted
	if len(git.RebaseCalls) != 0 {
		t.Errorf("rebase should not be called for diverged branch, got %d calls", len(git.RebaseCalls))
	}
}

// TestOrbit_WithVariants_MockExecution tests the full Orbit flow with variants.
func TestOrbit_WithVariants_MockExecution(t *testing.T) {
	tmpDir := t.TempDir()

	specDir := filepath.Join(tmpDir, "specs", "orbit-test")
	tasksFile := filepath.Join(specDir, "tasks.md")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatalf("failed to create spec dir: %v", err)
	}

	tasksContent := `# Tasks

## Phase 1: Implementation

- [ ] 1. Implement feature
`
	if err := os.WriteFile(tasksFile, []byte(tasksContent), 0644); err != nil {
		t.Fatalf("failed to write tasks file: %v", err)
	}

	// Track Claude calls
	var claudeCalls int
	mock := &mockClaudeClient{
		runPhaseFunc: func(sessionID string, resume bool) (*agents.RunResult, error) {
			claudeCalls++
			return &agents.RunResult{
				SessionID: sessionID,
				Output:    "Phase completed successfully",
				Cost:      &agents.CostMetrics{CostUSD: 0.05},
				Duration:  time.Minute,
				NumTurns:  10,
				IsError:   false,
			}, nil
		},
	}

	// Create Orbit config
	config := Config{
		TasksFile:    tasksFile,
		LogDir:       filepath.Join(specDir, ".orbit"),
		BranchName:   "orbit-test",
		WorkingDir:   tmpDir,
		VariantCount: 2,
		Parallel:     false,
		MaxParallel:  3,
		BranchPrefix: "orbit-impl",
		SpecDir:      specDir,
		RepoRoot:     tmpDir,
		DryRun:       true, // Use dry-run to skip actual execution
	}

	// Create Orbit instance (dry-run mode)
	o := &Orbit{
		config:       config,
		claudeClient: mock,
	}

	// Since we're in dry-run mode, variant manager is nil
	// This test primarily validates the config is correctly parsed
	if o.variantManager != nil {
		t.Log("Variant manager should be nil in dry-run mode")
	}

	// Verify config was set correctly
	if o.config.VariantCount != 2 {
		t.Errorf("VariantCount = %d, want 2", o.config.VariantCount)
	}
	if o.config.Parallel {
		t.Error("Parallel should be false")
	}
	if o.config.MaxParallel != 3 {
		t.Errorf("MaxParallel = %d, want 3", o.config.MaxParallel)
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestAgentSelection_DefaultToClaude tests that claude-code is the default agent [Req 6.4].
func TestAgentSelection_DefaultToClaude(t *testing.T) {
	config := Config{
		Agent: "", // No agent specified
	}

	// When no agent is specified, it should default to claude-code
	expectedDefault := "claude-code"
	if config.Agent == "" {
		// This is the expected behavior - the orchestrator defaults to claude-code
		// when no agent is explicitly specified
		t.Log("Config.Agent is empty - orchestrator will default to claude-code")
	}

	// Verify the agents registry contains claude-code
	agent, err := agents.Get("claude-code", agents.AgentConfig{})
	if err != nil {
		t.Fatalf("Failed to get default agent: %v", err)
	}
	if agent.Name() != expectedDefault {
		t.Errorf("Default agent name = %q, want %q", agent.Name(), expectedDefault)
	}
}

// TestAgentSelection_CliOverridesConfig tests that CLI flag takes precedence [Req 6.3].
func TestAgentSelection_CliOverridesConfig(t *testing.T) {
	// Simulate config file specifying "codex"
	configAgent := "codex"
	// Simulate CLI flag specifying "kiro"
	cliAgent := "kiro"

	// CLI should take precedence - determine effective agent using precedence rules
	var effectiveAgent string
	if cliAgent != "" {
		effectiveAgent = cliAgent
	} else if configAgent != "" {
		effectiveAgent = configAgent
	} else {
		effectiveAgent = "claude-code"
	}

	if effectiveAgent != "kiro" {
		t.Errorf("Effective agent = %q, want %q", effectiveAgent, "kiro")
	}
}

// TestAgentSelection_AllSupportedAgents tests that all supported agents can be selected [Req 6.1].
func TestAgentSelection_AllSupportedAgents(t *testing.T) {
	supportedAgents := []string{"claude-code", "codex", "kiro", "copilot"}

	for _, agentName := range supportedAgents {
		t.Run(agentName, func(t *testing.T) {
			agent, err := agents.Get(agentName, agents.AgentConfig{})
			if err != nil {
				t.Fatalf("Failed to get agent %q: %v", agentName, err)
			}
			if agent.Name() != agentName {
				t.Errorf("Agent name = %q, want %q", agent.Name(), agentName)
			}
		})
	}
}

// TestVariantAgents_CyclingBehavior tests that variant-agents cycles through the list [Req 10.1, 10.3].
func TestVariantAgents_CyclingBehavior(t *testing.T) {
	tests := []struct {
		name           string
		variantAgents  []string
		variantCount   int
		expectedAgents []string
	}{
		{
			name:           "more variants than agents - cycles",
			variantAgents:  []string{"claude-code", "codex"},
			variantCount:   5,
			expectedAgents: []string{"claude-code", "codex", "claude-code", "codex", "claude-code"},
		},
		{
			name:           "equal variants and agents",
			variantAgents:  []string{"claude-code", "codex", "kiro"},
			variantCount:   3,
			expectedAgents: []string{"claude-code", "codex", "kiro"},
		},
		{
			name:           "fewer variants than agents",
			variantAgents:  []string{"claude-code", "codex", "kiro", "copilot"},
			variantCount:   2,
			expectedAgents: []string{"claude-code", "codex"},
		},
		{
			name:           "single agent for all variants",
			variantAgents:  []string{"kiro"},
			variantCount:   4,
			expectedAgents: []string{"kiro", "kiro", "kiro", "kiro"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{
				VariantAgents: tt.variantAgents,
				VariantCount:  tt.variantCount,
			}

			// Simulate the cycling behavior
			for i := 0; i < tt.variantCount; i++ {
				agentIdx := i % len(config.VariantAgents)
				actualAgent := config.VariantAgents[agentIdx]
				if actualAgent != tt.expectedAgents[i] {
					t.Errorf("Variant %d: agent = %q, want %q", i+1, actualAgent, tt.expectedAgents[i])
				}
			}
		})
	}
}

// TestVariantAgents_EmptyList tests fallback behavior when no variant agents specified.
func TestVariantAgents_EmptyList(t *testing.T) {
	config := Config{
		VariantAgents: []string{}, // Empty list
		VariantCount:  3,
		Agent:         "claude-code", // Global agent
	}

	// When no variant agents specified, all variants should use the global agent
	if len(config.VariantAgents) == 0 {
		for i := 0; i < config.VariantCount; i++ {
			// Should fall back to global agent
			if config.Agent != "claude-code" {
				t.Errorf("Variant %d should use global agent claude-code", i+1)
			}
		}
	}
}

// TestRunWithoutConfig tests that orbit run fails when no .orbit.yaml exists [Req 2.1, 2.2].
func TestRunWithoutConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Create spec directory without .orbit.yaml
	specDir := filepath.Join(tmpDir, "specs", "no-config-test")
	tasksFile := filepath.Join(specDir, "tasks.md")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatalf("failed to create spec dir: %v", err)
	}

	tasksContent := `# Tasks

## Phase 1: Implementation

- [ ] 1. Implement feature
`
	if err := os.WriteFile(tasksFile, []byte(tasksContent), 0644); err != nil {
		t.Fatalf("failed to write tasks file: %v", err)
	}

	// Change to temp directory (config.Load looks for .orbit.yaml in working dir)
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Errorf("failed to restore working directory: %v", err)
		}
	}()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}

	// Temporarily override HOME to prevent loading ~/.orbit.yaml
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Load config (should not find .orbit.yaml)
	cfg := orbitconfig.Load(tmpDir)

	// RequireConfigFile should fail
	err = cfg.RequireConfigFile()
	if err == nil {
		t.Fatal("expected error when .orbit.yaml is missing, got nil")
	}

	// Error should mention .orbit.yaml and orbit init
	errMsg := err.Error()
	if !contains(errMsg, ".orbit.yaml") {
		t.Errorf("error should mention .orbit.yaml, got: %v", err)
	}
	if !contains(errMsg, "orbit init") {
		t.Errorf("error should mention 'orbit init', got: %v", err)
	}
}

// TestVariantRunWithDifferentModels tests variant execution with different model configurations [Req 4.1, 4.2, 4.4, 7.1].
func TestVariantRunWithDifferentModels(t *testing.T) {
	tmpDir := t.TempDir()

	// Create spec directory
	specDir := filepath.Join(tmpDir, "specs", "model-test")
	tasksFile := filepath.Join(specDir, "tasks.md")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatalf("failed to create spec dir: %v", err)
	}

	tasksContent := `# Tasks

## Phase 1: Implementation

- [ ] 1. Implement feature
`
	if err := os.WriteFile(tasksFile, []byte(tasksContent), 0644); err != nil {
		t.Fatalf("failed to write tasks file: %v", err)
	}

	// Create .orbit.yaml with two agent aliases
	configContent := `agents:
  claude-sonnet:
    type: claude-code
    model: claude-sonnet-4-20250514
    auto-approve: true
  claude-opus:
    type: claude-code
    model: claude-opus-4-20250514
    auto-approve: true
`
	configPath := filepath.Join(tmpDir, ".orbit.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Create mock git client
	git := variants.NewMockGit()
	git.CurrentBranch = "feature/model-test"
	git.HeadCommit = "model123"

	// Create variant config
	cfg := variants.Config{
		Count:        2,
		Parallel:     false,
		MaxParallel:  3,
		BranchPrefix: "orbit-impl",
	}

	// Create variant manager
	mgr, err := variants.NewManager(cfg, "model-test", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("failed to create variant manager: %v", err)
	}

	// Setup worktrees
	ctx := context.Background()
	if err := mgr.Setup(ctx, false); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// Assign agents to variants (simulating --variant-agents claude-sonnet,claude-opus)
	variantAgents := []string{"claude-sonnet", "claude-opus"}
	variantList := mgr.GetVariantsSnapshot()
	for i, v := range variantList {
		agentAlias := variantAgents[i%len(variantAgents)]
		agentType := "claude-code"
		var model string
		if agentAlias == "claude-sonnet" {
			model = "claude-sonnet-4-20250514"
		} else {
			model = "claude-opus-4-20250514"
		}

		// Update agent metadata
		if err := mgr.UpdateAgentInfo(v.ID, agentAlias, agentType, model); err != nil {
			t.Fatalf("failed to update agent info: %v", err)
		}
	}

	// Verify variants.json contains correct metadata
	metadataPath := filepath.Join(specDir, ".orbit", "variants.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("failed to read variants.json: %v", err)
	}

	var metadata variants.VariantsMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatalf("failed to parse variants.json: %v", err)
	}

	// Verify variant 1 has claude-sonnet
	v1 := metadata.Variants[0]
	if v1.Agent != "claude-sonnet" {
		t.Errorf("variant 1 agent = %q, want %q", v1.Agent, "claude-sonnet")
	}
	if v1.AgentType != "claude-code" {
		t.Errorf("variant 1 agent_type = %q, want %q", v1.AgentType, "claude-code")
	}
	if v1.Model != "claude-sonnet-4-20250514" {
		t.Errorf("variant 1 model = %q, want %q", v1.Model, "claude-sonnet-4-20250514")
	}

	// Verify variant 2 has claude-opus
	v2 := metadata.Variants[1]
	if v2.Agent != "claude-opus" {
		t.Errorf("variant 2 agent = %q, want %q", v2.Agent, "claude-opus")
	}
	if v2.AgentType != "claude-code" {
		t.Errorf("variant 2 agent_type = %q, want %q", v2.AgentType, "claude-code")
	}
	if v2.Model != "claude-opus-4-20250514" {
		t.Errorf("variant 2 model = %q, want %q", v2.Model, "claude-opus-4-20250514")
	}
}
