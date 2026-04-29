package variants

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// branchCreateCall records the name and commit passed to CreateBranch.
type branchCreateCall struct {
	name, commit string
}

// mockGitClient implements GitClient for testing.
type mockGitClient struct {
	mu sync.Mutex

	currentBranch       string
	headCommit          string
	uncommittedChanges  bool
	branchDiverged      bool
	createdBranches     []string
	branchCreateCalls   []branchCreateCall
	createdWorktrees    map[string]string // path -> branch
	removedWorktrees    []string
	deletedBranches     []string
	rebaseCalls         []rebaseCall
	createBranchError   error
	createWorktreeError error
	removeWorktreeError error
	deleteBranchError   error
	rebaseError         error
}

type rebaseCall struct {
	source, target string
}

func newMockGitClient() *mockGitClient {
	return &mockGitClient{
		currentBranch:    "feature/test-spec",
		headCommit:       "abc123def456",
		createdWorktrees: make(map[string]string),
	}
}

func (m *mockGitClient) GetCurrentBranch() (string, error) {
	return m.currentBranch, nil
}

func (m *mockGitClient) GetHeadCommit() (string, error) {
	return m.headCommit, nil
}

func (m *mockGitClient) GetHeadCommitInPath(_ string) (string, error) {
	return m.headCommit, nil
}

func (m *mockGitClient) CreateBranch(name, commit string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createBranchError != nil {
		return m.createBranchError
	}
	m.createdBranches = append(m.createdBranches, name)
	m.branchCreateCalls = append(m.branchCreateCalls, branchCreateCall{name: name, commit: commit})
	return nil
}

func (m *mockGitClient) CreateWorktree(_ context.Context, path, branch string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createWorktreeError != nil {
		return m.createWorktreeError
	}
	m.createdWorktrees[path] = branch
	return nil
}

func (m *mockGitClient) RemoveWorktree(_ context.Context, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.removeWorktreeError != nil {
		return m.removeWorktreeError
	}
	m.removedWorktrees = append(m.removedWorktrees, path)
	delete(m.createdWorktrees, path)
	return nil
}

func (m *mockGitClient) DeleteBranch(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.deleteBranchError != nil {
		return m.deleteBranchError
	}
	m.deletedBranches = append(m.deletedBranches, name)
	return nil
}

func (m *mockGitClient) GetDiff(_ context.Context, worktreePath, baseCommit string) (string, error) {
	return "mock diff", nil
}

func (m *mockGitClient) Rebase(_ context.Context, sourceBranch, targetBranch string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rebaseError != nil {
		return m.rebaseError
	}
	m.rebaseCalls = append(m.rebaseCalls, rebaseCall{sourceBranch, targetBranch})
	return nil
}

func (m *mockGitClient) BranchHasDiverged(branch, baseCommit string) (bool, error) {
	return m.branchDiverged, nil
}

func (m *mockGitClient) HasUncommittedChanges() (bool, error) {
	return m.uncommittedChanges, nil
}

func (m *mockGitClient) GetCommitLog(_ context.Context, _, _ string) ([]string, error) {
	return []string{"abc123 Mock commit"}, nil
}

func (m *mockGitClient) GetDiffStat(_ context.Context, _, _ string) (string, error) {
	return " 3 files changed, 50 insertions(+), 10 deletions(-)", nil
}

func (m *mockGitClient) HasUncommittedChangesInPath(_ string) (bool, error) {
	return m.uncommittedChanges, nil
}

func (m *mockGitClient) GetRecentCommits(_ context.Context, _, _ string, _ int) ([]Commit, error) {
	return nil, nil
}

func TestNewManager(t *testing.T) {
	git := newMockGitClient()

	tests := []struct {
		name      string
		cfg       Config
		specName  string
		specDir   string
		repoRoot  string
		git       GitClient
		wantError bool
	}{
		{
			name:      "valid config",
			cfg:       Config{Count: 2, BranchPrefix: "orbit-impl"},
			specName:  "test-spec",
			specDir:   "/path/to/specs/test-spec",
			repoRoot:  "/path/to/repo",
			git:       git,
			wantError: false,
		},
		{
			name:      "missing spec name",
			cfg:       Config{Count: 2},
			specName:  "",
			specDir:   "/path/to/specs",
			repoRoot:  "/path/to/repo",
			git:       git,
			wantError: true,
		},
		{
			name:      "missing spec dir",
			cfg:       Config{Count: 2},
			specName:  "test-spec",
			specDir:   "",
			repoRoot:  "/path/to/repo",
			git:       git,
			wantError: true,
		},
		{
			name:      "missing repo root",
			cfg:       Config{Count: 2},
			specName:  "test-spec",
			specDir:   "/path/to/specs",
			repoRoot:  "",
			git:       git,
			wantError: true,
		},
		{
			name:      "nil git client",
			cfg:       Config{Count: 2},
			specName:  "test-spec",
			specDir:   "/path/to/specs",
			repoRoot:  "/path/to/repo",
			git:       nil,
			wantError: true,
		},
		{
			name:      "zero variant count",
			cfg:       Config{Count: 0},
			specName:  "test-spec",
			specDir:   "/path/to/specs",
			repoRoot:  "/path/to/repo",
			git:       git,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewManager(tt.cfg, tt.specName, tt.specDir, tt.repoRoot, tt.git)
			if (err != nil) != tt.wantError {
				t.Errorf("NewManager() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestHasExistingRun(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")
	git := newMockGitClient()

	cfg := Config{Count: 2, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// Initially no existing run
	if mgr.HasExistingRun() {
		t.Error("expected HasExistingRun() = false initially")
	}

	// Set metadata
	mgr.metadata = &VariantsMetadata{
		RunID:      "test-run",
		BaseCommit: "abc123",
	}

	// Now should have existing run
	if !mgr.HasExistingRun() {
		t.Error("expected HasExistingRun() = true after setting metadata")
	}
}

func TestSetup_CreatesWorktreesAndBranches(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")
	git := newMockGitClient()

	cfg := Config{Count: 3, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx := context.Background()
	if err := mgr.Setup(ctx, false); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	// Verify branches created
	if len(git.createdBranches) != 3 {
		t.Errorf("expected 3 branches, got %d", len(git.createdBranches))
	}

	// Verify worktrees created
	if len(git.createdWorktrees) != 3 {
		t.Errorf("expected 3 worktrees, got %d", len(git.createdWorktrees))
	}

	// Verify metadata
	if mgr.metadata == nil {
		t.Fatal("metadata is nil")
	}
	if mgr.metadata.BaseCommit != "abc123def456" {
		t.Errorf("unexpected base commit: %s", mgr.metadata.BaseCommit)
	}
	if mgr.metadata.OriginalBranch != "feature/test-spec" {
		t.Errorf("unexpected original branch: %s", mgr.metadata.OriginalBranch)
	}
	if len(mgr.metadata.Variants) != 3 {
		t.Errorf("expected 3 variants, got %d", len(mgr.metadata.Variants))
	}

	// Verify all variants are pending
	for _, v := range mgr.metadata.Variants {
		if v.Status != StatusPending {
			t.Errorf("variant %d status = %s, want pending", v.ID, v.Status)
		}
	}
}

func TestSetup_ReusesCompatibleWorktrees(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")
	git := newMockGitClient()

	cfg := Config{Count: 2, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// Pre-populate metadata with matching base commit
	mgr.metadata = &VariantsMetadata{
		RunID:          "existing-run",
		BaseCommit:     "abc123def456", // Same as mock
		OriginalBranch: "feature/test-spec",
		StartedAt:      time.Now(),
		Variants:       []*Variant{{ID: 1}, {ID: 2}},
	}

	ctx := context.Background()
	if err := mgr.Setup(ctx, true); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	// Should not create new branches/worktrees
	if len(git.createdBranches) != 0 {
		t.Errorf("expected 0 new branches, got %d", len(git.createdBranches))
	}
	if len(git.createdWorktrees) != 0 {
		t.Errorf("expected 0 new worktrees, got %d", len(git.createdWorktrees))
	}
}

func TestSetup_CleansUpOnNewRun(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")
	git := newMockGitClient()

	cfg := Config{Count: 2, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// Pre-populate metadata with different base commit
	mgr.metadata = &VariantsMetadata{
		RunID:          "existing-run",
		BaseCommit:     "different123",
		OriginalBranch: "feature/test-spec",
		StartedAt:      time.Now(),
		Variants: []*Variant{
			{ID: 1, Branch: "orbit-impl-1/test-spec", WorktreePath: "/tmp/wt1"},
			{ID: 2, Branch: "orbit-impl-2/test-spec", WorktreePath: "/tmp/wt2"},
		},
	}

	// Create metadata file so cleanup can remove it
	orbitDir := filepath.Join(specDir, ".orbit")
	if err := os.MkdirAll(orbitDir, 0755); err != nil {
		t.Fatalf("create orbit dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(orbitDir, "variants.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("create variants.json: %v", err)
	}

	ctx := context.Background()
	if err := mgr.Setup(ctx, false); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	// Should have cleaned up old worktrees and created new ones
	if len(git.removedWorktrees) != 2 {
		t.Errorf("expected 2 removed worktrees, got %d", len(git.removedWorktrees))
	}
	if len(git.createdBranches) != 2 {
		t.Errorf("expected 2 new branches, got %d", len(git.createdBranches))
	}
}

func TestSetup_PreservesCompletedVariants(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")
	git := newMockGitClient()

	cfg := Config{Count: 3, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// Pre-populate metadata with mixed statuses:
	// - Variant 1: completed (should be preserved)
	// - Variant 2: failed (should be cleaned up and recreated)
	// - Variant 3: pending (should be cleaned up and recreated)
	// BaseCommit must match mock HEAD ("abc123def456") so preserved
	// completed variants share the same base as new ones (T-191).
	mgr.metadata = &VariantsMetadata{
		RunID:          "existing-run",
		BaseCommit:     "abc123def456",
		OriginalBranch: "feature/test-spec",
		StartedAt:      time.Now(),
		Variants: []*Variant{
			{ID: 1, Branch: "orbit-impl-1/test-spec", WorktreePath: "/tmp/wt1", Status: StatusCompleted},
			{ID: 2, Branch: "orbit-impl-2/test-spec", WorktreePath: "/tmp/wt2", Status: StatusFailed},
			{ID: 3, Branch: "orbit-impl-3/test-spec", WorktreePath: "/tmp/wt3", Status: StatusPending},
		},
	}

	// Create metadata file so cleanup can update it
	orbitDir := filepath.Join(specDir, ".orbit")
	if err := os.MkdirAll(orbitDir, 0755); err != nil {
		t.Fatalf("create orbit dir: %v", err)
	}
	metadataBytes, _ := json.Marshal(mgr.metadata)
	if err := os.WriteFile(filepath.Join(orbitDir, "variants.json"), metadataBytes, 0644); err != nil {
		t.Fatalf("create variants.json: %v", err)
	}

	ctx := context.Background()
	if err := mgr.Setup(ctx, false); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	// Should have cleaned up only non-completed worktrees (2 and 3)
	if len(git.removedWorktrees) != 2 {
		t.Errorf("expected 2 removed worktrees, got %d: %v", len(git.removedWorktrees), git.removedWorktrees)
	}
	// Verify correct worktrees were removed
	for _, removed := range git.removedWorktrees {
		if removed == "/tmp/wt1" {
			t.Errorf("completed variant worktree /tmp/wt1 should not have been removed")
		}
	}

	// Should have created only 2 new branches (for variants 2 and 3)
	if len(git.createdBranches) != 2 {
		t.Errorf("expected 2 new branches, got %d: %v", len(git.createdBranches), git.createdBranches)
	}

	// Verify metadata has all 3 variants
	variants := mgr.GetVariantsSnapshot()
	if len(variants) != 3 {
		t.Fatalf("expected 3 variants, got %d", len(variants))
	}

	// Verify variant 1 is still completed
	v1 := mgr.GetVariant(1)
	if v1 == nil {
		t.Fatal("variant 1 not found")
	}
	if v1.Status != StatusCompleted {
		t.Errorf("variant 1 status should be completed, got %s", v1.Status)
	}

	// Verify variants 2 and 3 are pending
	v2 := mgr.GetVariant(2)
	if v2 == nil {
		t.Fatal("variant 2 not found")
	}
	if v2.Status != StatusPending {
		t.Errorf("variant 2 status should be pending, got %s", v2.Status)
	}

	v3 := mgr.GetVariant(3)
	if v3 == nil {
		t.Fatal("variant 3 not found")
	}
	if v3.Status != StatusPending {
		t.Errorf("variant 3 status should be pending, got %s", v3.Status)
	}
}

func TestCleanupUnfinished(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")
	git := newMockGitClient()

	cfg := Config{Count: 3, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// Create metadata file
	orbitDir := filepath.Join(specDir, ".orbit")
	if err := os.MkdirAll(orbitDir, 0755); err != nil {
		t.Fatalf("create orbit dir: %v", err)
	}

	// Set up metadata with mixed statuses
	mgr.metadata = &VariantsMetadata{
		RunID:          "test-run",
		BaseCommit:     "abc123",
		OriginalBranch: "feature/test-spec",
		StartedAt:      time.Now(),
		Variants: []*Variant{
			{ID: 1, Branch: "orbit-impl-1/test-spec", WorktreePath: "/tmp/wt1", Status: StatusCompleted},
			{ID: 2, Branch: "orbit-impl-2/test-spec", WorktreePath: "/tmp/wt2", Status: StatusFailed},
			{ID: 3, Branch: "orbit-impl-3/test-spec", WorktreePath: "/tmp/wt3", Status: StatusRunning},
		},
	}

	ctx := context.Background()
	completedIDs, err := mgr.CleanupUnfinished(ctx)
	if err != nil {
		t.Fatalf("CleanupUnfinished: %v", err)
	}

	// Should return completed variant IDs
	if len(completedIDs) != 1 || completedIDs[0] != 1 {
		t.Errorf("expected completedIDs=[1], got %v", completedIDs)
	}

	// Should have removed only non-completed worktrees
	if len(git.removedWorktrees) != 2 {
		t.Errorf("expected 2 removed worktrees, got %d", len(git.removedWorktrees))
	}

	// Should have deleted only non-completed branches
	if len(git.deletedBranches) != 2 {
		t.Errorf("expected 2 deleted branches, got %d", len(git.deletedBranches))
	}

	// Metadata should only contain completed variant
	if len(mgr.metadata.Variants) != 1 {
		t.Errorf("expected 1 variant in metadata, got %d", len(mgr.metadata.Variants))
	}
	if mgr.metadata.Variants[0].ID != 1 {
		t.Errorf("expected variant ID 1, got %d", mgr.metadata.Variants[0].ID)
	}
}

func TestCleanupUnfinished_AllCompleted(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")
	git := newMockGitClient()

	cfg := Config{Count: 2, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// Create metadata file
	orbitDir := filepath.Join(specDir, ".orbit")
	if err := os.MkdirAll(orbitDir, 0755); err != nil {
		t.Fatalf("create orbit dir: %v", err)
	}

	// All variants completed
	mgr.metadata = &VariantsMetadata{
		RunID:          "test-run",
		BaseCommit:     "abc123",
		OriginalBranch: "feature/test-spec",
		StartedAt:      time.Now(),
		Variants: []*Variant{
			{ID: 1, Branch: "orbit-impl-1/test-spec", WorktreePath: "/tmp/wt1", Status: StatusCompleted},
			{ID: 2, Branch: "orbit-impl-2/test-spec", WorktreePath: "/tmp/wt2", Status: StatusCompleted},
		},
	}

	ctx := context.Background()
	completedIDs, err := mgr.CleanupUnfinished(ctx)
	if err != nil {
		t.Fatalf("CleanupUnfinished: %v", err)
	}

	// Should return all completed variant IDs
	if len(completedIDs) != 2 {
		t.Errorf("expected 2 completedIDs, got %d", len(completedIDs))
	}

	// Should not have removed any worktrees
	if len(git.removedWorktrees) != 0 {
		t.Errorf("expected 0 removed worktrees, got %d", len(git.removedWorktrees))
	}

	// Metadata should still contain both variants
	if len(mgr.metadata.Variants) != 2 {
		t.Errorf("expected 2 variants in metadata, got %d", len(mgr.metadata.Variants))
	}
}

func TestCleanupUnfinished_NoneCompleted(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")
	git := newMockGitClient()

	cfg := Config{Count: 2, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// Create metadata file
	orbitDir := filepath.Join(specDir, ".orbit")
	if err := os.MkdirAll(orbitDir, 0755); err != nil {
		t.Fatalf("create orbit dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(orbitDir, "variants.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("create variants.json: %v", err)
	}

	// No variants completed
	mgr.metadata = &VariantsMetadata{
		RunID:          "test-run",
		BaseCommit:     "abc123",
		OriginalBranch: "feature/test-spec",
		StartedAt:      time.Now(),
		Variants: []*Variant{
			{ID: 1, Branch: "orbit-impl-1/test-spec", WorktreePath: "/tmp/wt1", Status: StatusFailed},
			{ID: 2, Branch: "orbit-impl-2/test-spec", WorktreePath: "/tmp/wt2", Status: StatusPending},
		},
	}

	ctx := context.Background()
	completedIDs, err := mgr.CleanupUnfinished(ctx)
	if err != nil {
		t.Fatalf("CleanupUnfinished: %v", err)
	}

	// Should return no completed IDs
	if len(completedIDs) != 0 {
		t.Errorf("expected 0 completedIDs, got %d", len(completedIDs))
	}

	// Should have removed all worktrees
	if len(git.removedWorktrees) != 2 {
		t.Errorf("expected 2 removed worktrees, got %d", len(git.removedWorktrees))
	}

	// Metadata should be nil (fully cleaned up)
	if mgr.metadata != nil {
		t.Errorf("expected nil metadata, got %+v", mgr.metadata)
	}
}

func TestSetup_FailsOnDirtyWorkingDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")
	git := newMockGitClient()
	git.uncommittedChanges = true

	cfg := Config{Count: 2, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx := context.Background()
	err = mgr.Setup(ctx, false)
	if err == nil {
		t.Fatal("expected error for dirty working directory")
	}
	if !contains(err.Error(), "uncommitted changes") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSetup_CreatesGitignore(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")
	git := newMockGitClient()

	cfg := Config{Count: 1, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx := context.Background()
	if err := mgr.Setup(ctx, false); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	// Check .gitignore was created
	gitignorePath := filepath.Join(specDir, ".orbit", ".gitignore")
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if !contains(string(content), "worktrees/") {
		t.Errorf(".gitignore missing worktrees/ entry: %s", content)
	}
}

func TestSetup_UpdatesExistingGitignore(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")
	git := newMockGitClient()

	// Create existing .gitignore without worktrees/
	orbitDir := filepath.Join(specDir, ".orbit")
	if err := os.MkdirAll(orbitDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	gitignorePath := filepath.Join(orbitDir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte("existing-entry\n"), 0644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	cfg := Config{Count: 1, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx := context.Background()
	if err := mgr.Setup(ctx, false); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	// Check .gitignore was updated
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if !contains(string(content), "existing-entry") {
		t.Errorf(".gitignore missing original entry: %s", content)
	}
	if !contains(string(content), "worktrees/") {
		t.Errorf(".gitignore missing worktrees/ entry: %s", content)
	}
}

func TestUpdateStatus(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")
	git := newMockGitClient()

	cfg := Config{Count: 2, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx := context.Background()
	if err := mgr.Setup(ctx, false); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	// Update status
	testErr := os.ErrNotExist
	if err := mgr.UpdateStatus(1, StatusFailed, testErr); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	v := mgr.GetVariant(1)
	if v.Status != StatusFailed {
		t.Errorf("variant status = %s, want failed", v.Status)
	}
	if !contains(v.Error, "not exist") {
		t.Errorf("variant error = %s, want to contain 'not exist'", v.Error)
	}

	// Verify persisted
	data, err := os.ReadFile(mgr.metadataPath)
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	var loaded VariantsMetadata
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if loaded.Variants[0].Status != StatusFailed {
		t.Errorf("persisted status = %s, want failed", loaded.Variants[0].Status)
	}
}

func TestUpdateAgentInfo(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")
	git := newMockGitClient()

	cfg := Config{Count: 2, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx := context.Background()
	if err := mgr.Setup(ctx, false); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	// Update agent info for variant 1
	if err := mgr.UpdateAgentInfo(1, "claude-sonnet", "claude-code", "claude-3-5-sonnet"); err != nil {
		t.Fatalf("UpdateAgentInfo: %v", err)
	}

	v := mgr.GetVariant(1)
	if v.Agent != "claude-sonnet" {
		t.Errorf("variant Agent = %s, want claude-sonnet", v.Agent)
	}
	if v.AgentType != "claude-code" {
		t.Errorf("variant AgentType = %s, want claude-code", v.AgentType)
	}
	if v.Model != "claude-3-5-sonnet" {
		t.Errorf("variant Model = %s, want claude-3-5-sonnet", v.Model)
	}

	// Verify persisted to disk
	data, err := os.ReadFile(mgr.metadataPath)
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	var loaded VariantsMetadata
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if loaded.Variants[0].Agent != "claude-sonnet" {
		t.Errorf("persisted Agent = %s, want claude-sonnet", loaded.Variants[0].Agent)
	}
	if loaded.Variants[0].AgentType != "claude-code" {
		t.Errorf("persisted AgentType = %s, want claude-code", loaded.Variants[0].AgentType)
	}
	if loaded.Variants[0].Model != "claude-3-5-sonnet" {
		t.Errorf("persisted Model = %s, want claude-3-5-sonnet", loaded.Variants[0].Model)
	}
}

func TestUpdateAgentInfo_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")
	git := newMockGitClient()

	cfg := Config{Count: 2, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx := context.Background()
	if err := mgr.Setup(ctx, false); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	// Try to update non-existent variant
	err = mgr.UpdateAgentInfo(99, "alias", "claude-code", "model")
	if err == nil {
		t.Fatal("expected error for non-existent variant")
	}
	if !contains(err.Error(), "not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSave_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")
	git := newMockGitClient()

	cfg := Config{Count: 1, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx := context.Background()
	if err := mgr.Setup(ctx, false); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	// Verify the temp file doesn't remain
	tmpPath := mgr.metadataPath + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("temp file should not exist after save")
	}

	// Verify main file exists and is valid JSON
	data, err := os.ReadFile(mgr.metadataPath)
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	var loaded VariantsMetadata
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
}

func TestSave_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")
	git := newMockGitClient()

	cfg := Config{Count: 3, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx := context.Background()
	if err := mgr.Setup(ctx, false); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	// Concurrent status updates
	var wg sync.WaitGroup
	for i := 1; i <= 3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for range 10 {
				_ = mgr.UpdateStatus(id, StatusRunning, nil)
			}
		}(i)
	}
	wg.Wait()

	// Verify file is valid JSON
	data, err := os.ReadFile(mgr.metadataPath)
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	var loaded VariantsMetadata
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("invalid JSON after concurrent access: %v", err)
	}
}

func TestGetVariantsSnapshot_ReturnsCopy(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")
	git := newMockGitClient()

	cfg := Config{Count: 2, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx := context.Background()
	if err := mgr.Setup(ctx, false); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	snapshot := mgr.GetVariantsSnapshot()
	originalLen := len(snapshot)

	// Modify snapshot slice (this shouldn't affect the original)
	_ = append(snapshot, &Variant{ID: 99})

	// Original should be unchanged (re-get to verify)
	newSnapshot := mgr.GetVariantsSnapshot()
	if len(newSnapshot) != originalLen {
		t.Errorf("snapshot modification affected original: got %d, want %d", len(newSnapshot), originalLen)
	}

	// Also verify internal metadata unchanged
	if len(mgr.metadata.Variants) != 2 {
		t.Errorf("original variants modified: got %d, want 2", len(mgr.metadata.Variants))
	}
}

func TestCountByStatus(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")
	git := newMockGitClient()

	cfg := Config{Count: 3, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx := context.Background()
	if err := mgr.Setup(ctx, false); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	// All start as pending
	if count := mgr.CountByStatus(StatusPending); count != 3 {
		t.Errorf("pending count = %d, want 3", count)
	}

	// Update one to completed
	if err := mgr.UpdateStatus(1, StatusCompleted, nil); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if count := mgr.CountByStatus(StatusCompleted); count != 1 {
		t.Errorf("completed count = %d, want 1", count)
	}
	if count := mgr.CountByStatus(StatusPending); count != 2 {
		t.Errorf("pending count = %d, want 2", count)
	}
}

func TestCleanup_RemovesAllWorktrees(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")
	git := newMockGitClient()

	cfg := Config{Count: 2, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx := context.Background()
	if err := mgr.Setup(ctx, false); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	// Cleanup all
	if err := mgr.Cleanup(ctx, 0); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	// Verify worktrees removed
	if len(git.removedWorktrees) != 2 {
		t.Errorf("removed worktrees = %d, want 2", len(git.removedWorktrees))
	}

	// Verify branches deleted
	if len(git.deletedBranches) != 2 {
		t.Errorf("deleted branches = %d, want 2", len(git.deletedBranches))
	}

	// Verify metadata cleared
	if mgr.metadata != nil {
		t.Error("metadata should be nil after cleanup")
	}

	// Verify variants.json removed
	if _, err := os.Stat(mgr.metadataPath); !os.IsNotExist(err) {
		t.Error("variants.json should be removed")
	}
}

func TestCleanup_PreservesKeptVariant(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")
	git := newMockGitClient()

	cfg := Config{Count: 3, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx := context.Background()
	if err := mgr.Setup(ctx, false); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	// Cleanup but keep variant 2
	if err := mgr.Cleanup(ctx, 2); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	// Only 2 worktrees should be removed (1 and 3)
	if len(git.removedWorktrees) != 2 {
		t.Errorf("removed worktrees = %d, want 2", len(git.removedWorktrees))
	}

	// Only 2 branches should be deleted
	if len(git.deletedBranches) != 2 {
		t.Errorf("deleted branches = %d, want 2", len(git.deletedBranches))
	}

	// Metadata should still exist with variant 2
	if mgr.metadata == nil {
		t.Fatal("metadata should not be nil")
	}
	if len(mgr.metadata.Variants) != 1 {
		t.Errorf("variants count = %d, want 1", len(mgr.metadata.Variants))
	}
	if mgr.metadata.Variants[0].ID != 2 {
		t.Errorf("kept variant ID = %d, want 2", mgr.metadata.Variants[0].ID)
	}
}

func TestFinalize_RebasesVariant(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")
	git := newMockGitClient()

	cfg := Config{Count: 2, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx := context.Background()
	if err := mgr.Setup(ctx, false); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	// Finalize variant 1
	if err := mgr.Finalize(ctx, 1); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	// Verify rebase was called
	if len(git.rebaseCalls) != 1 {
		t.Fatalf("rebase calls = %d, want 1", len(git.rebaseCalls))
	}
	if git.rebaseCalls[0].target != "feature/test-spec" {
		t.Errorf("rebase target = %s, want feature/test-spec", git.rebaseCalls[0].target)
	}

	// Verify cleanup happened
	if len(git.removedWorktrees) != 2 {
		t.Errorf("removed worktrees = %d, want 2", len(git.removedWorktrees))
	}
}

func TestFinalize_FailsOnDivergedBranch(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")
	git := newMockGitClient()
	git.branchDiverged = true

	cfg := Config{Count: 2, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx := context.Background()
	if err := mgr.Setup(ctx, false); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	err = mgr.Finalize(ctx, 1)
	if err == nil {
		t.Fatal("expected error for diverged branch")
	}
	if !contains(err.Error(), "diverged") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoad_ParsesExistingMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")
	orbitDir := filepath.Join(specDir, ".orbit")
	if err := os.MkdirAll(orbitDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Write existing metadata
	metadata := VariantsMetadata{
		RunID:          "test-run-id",
		BaseCommit:     "existing123",
		OriginalBranch: "main",
		StartedAt:      time.Now(),
		Variants: []*Variant{
			{ID: 1, Branch: "branch-1", Status: StatusCompleted},
			{ID: 2, Branch: "branch-2", Status: StatusFailed, Error: "some error"},
		},
	}
	data, _ := json.MarshalIndent(metadata, "", "  ")
	metadataPath := filepath.Join(orbitDir, "variants.json")
	if err := os.WriteFile(metadataPath, data, 0644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	git := newMockGitClient()
	cfg := Config{Count: 2, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	if err := mgr.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if mgr.metadata == nil {
		t.Fatal("metadata is nil after load")
	}
	if mgr.metadata.RunID != "test-run-id" {
		t.Errorf("run ID = %s, want test-run-id", mgr.metadata.RunID)
	}
	if len(mgr.metadata.Variants) != 2 {
		t.Errorf("variants count = %d, want 2", len(mgr.metadata.Variants))
	}
	if mgr.metadata.Variants[1].Error != "some error" {
		t.Errorf("variant 2 error = %s, want 'some error'", mgr.metadata.Variants[1].Error)
	}
}

func TestLoad_ParsesAgentMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")
	orbitDir := filepath.Join(specDir, ".orbit")
	if err := os.MkdirAll(orbitDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Write metadata with agent info
	metadata := VariantsMetadata{
		RunID:          "test-run-id",
		BaseCommit:     "existing123",
		OriginalBranch: "main",
		StartedAt:      time.Now(),
		Variants: []*Variant{
			{ID: 1, Branch: "branch-1", Agent: "sonnet", AgentType: "claude-code", Model: "claude-3-5-sonnet"},
			{ID: 2, Branch: "branch-2", Agent: "codex", AgentType: "codex", Model: "o3"},
		},
	}
	data, _ := json.MarshalIndent(metadata, "", "  ")
	metadataPath := filepath.Join(orbitDir, "variants.json")
	if err := os.WriteFile(metadataPath, data, 0644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	git := newMockGitClient()
	cfg := Config{Count: 2, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	if err := mgr.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Verify agent metadata was loaded
	v1 := mgr.GetVariant(1)
	if v1.Agent != "sonnet" {
		t.Errorf("variant 1 Agent = %s, want sonnet", v1.Agent)
	}
	if v1.AgentType != "claude-code" {
		t.Errorf("variant 1 AgentType = %s, want claude-code", v1.AgentType)
	}
	if v1.Model != "claude-3-5-sonnet" {
		t.Errorf("variant 1 Model = %s, want claude-3-5-sonnet", v1.Model)
	}

	v2 := mgr.GetVariant(2)
	if v2.AgentType != "codex" {
		t.Errorf("variant 2 AgentType = %s, want codex", v2.AgentType)
	}
	if v2.Model != "o3" {
		t.Errorf("variant 2 Model = %s, want o3", v2.Model)
	}
}

func TestSanitizeSpecName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with space", "with-space"},
		{"with/slash", "with-slash"},
		{"with\\backslash", "with-backslash"},
		{"with:colon", "with-colon"},
		{"multi--dash", "multi-dash"},
		{"-leading-dash", "leading-dash"},
		{"trailing-dash-", "trailing-dash"},
		{"", ""},
		{"a/b/c", "a-b-c"},
		{"file<name>test", "file-name-test"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeSpecName(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeSpecName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestSetup_PinsBranchesToCapturedCommit is the regression test for T-112.
// It verifies that all variant branches are created at the HEAD commit captured
// at the start of Setup, not at whatever HEAD happens to be when each individual
// branch is created. Without this fix, if HEAD advances between branch creation
// calls (e.g., another process pushes to main), different variants would start
// from different base commits, leading to inconsistent comparison results.
func TestSetup_PinsBranchesToCapturedCommit(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")
	git := newMockGitClient()

	cfg := Config{Count: 3, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx := context.Background()
	if err := mgr.Setup(ctx, false); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	// All branches must have been created at the captured head commit
	if len(git.branchCreateCalls) != 3 {
		t.Fatalf("expected 3 branch create calls, got %d", len(git.branchCreateCalls))
	}
	for _, call := range git.branchCreateCalls {
		if call.commit != "abc123def456" {
			t.Errorf("branch %q created at commit %q, want %q",
				call.name, call.commit, "abc123def456")
		}
	}
}

// contains is a helper for checking substring presence.
// TestSetup_ContinuePreservesPendingVariants verifies that choosing "continue"
// after an interrupted run preserves pending variants so they can be executed.
// This tests the recovery flow: run is interrupted, pending variants stay pending,
// user re-runs with "continue", and pending variants are picked up.
func TestSetup_ContinuePreservesPendingVariants(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")
	git := newMockGitClient()

	cfg := Config{Count: 3, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// Simulate state after an interrupted run:
	// - Variant 1: completed (finished before interrupt)
	// - Variant 2: canceled (was running when interrupted)
	// - Variant 3: pending (never started, interrupt preserved its status)
	mgr.metadata = &VariantsMetadata{
		RunID:          "interrupted-run",
		BaseCommit:     "abc123",
		OriginalBranch: "feature/test-spec",
		StartedAt:      time.Now(),
		Variants: []*Variant{
			{ID: 1, Branch: "orbit-impl-1/test-spec", WorktreePath: "/tmp/wt1", Status: StatusCompleted},
			{ID: 2, Branch: "orbit-impl-2/test-spec", WorktreePath: "/tmp/wt2", Status: StatusCanceled},
			{ID: 3, Branch: "orbit-impl-3/test-spec", WorktreePath: "/tmp/wt3", Status: StatusPending},
		},
	}

	// Create metadata file
	orbitDir := filepath.Join(specDir, ".orbit")
	if err := os.MkdirAll(orbitDir, 0755); err != nil {
		t.Fatalf("create orbit dir: %v", err)
	}
	metadataBytes, _ := json.Marshal(mgr.metadata)
	if err := os.WriteFile(filepath.Join(orbitDir, "variants.json"), metadataBytes, 0644); err != nil {
		t.Fatalf("create variants.json: %v", err)
	}

	// Continue the existing run (continueExisting=true)
	ctx := context.Background()
	if err := mgr.Setup(ctx, true); err != nil {
		t.Fatalf("Setup with continue: %v", err)
	}

	// All variants should be preserved as-is
	variants := mgr.GetVariantsSnapshot()
	if len(variants) != 3 {
		t.Fatalf("expected 3 variants, got %d", len(variants))
	}

	// Completed stays completed
	if v := mgr.GetVariant(1); v.Status != StatusCompleted {
		t.Errorf("variant 1: status = %q, want %q", v.Status, StatusCompleted)
	}
	// Canceled stays canceled (was already running when interrupted)
	if v := mgr.GetVariant(2); v.Status != StatusCanceled {
		t.Errorf("variant 2: status = %q, want %q", v.Status, StatusCanceled)
	}
	// Pending stays pending (can be executed on this continue run)
	if v := mgr.GetVariant(3); v.Status != StatusPending {
		t.Errorf("variant 3: status = %q, want %q", v.Status, StatusPending)
	}

	// No worktrees should have been removed or created (continue mode skips setup)
	if len(git.removedWorktrees) != 0 {
		t.Errorf("expected 0 removed worktrees in continue mode, got %d", len(git.removedWorktrees))
	}
	if len(git.createdBranches) != 0 {
		t.Errorf("expected 0 created branches in continue mode, got %d", len(git.createdBranches))
	}
}

// TestSetup_NewRunErrorsWhenHEADDiffersFromBaseCommit is the regression test for T-191.
// When starting a new run (continueExisting=false) with completed variants preserved,
// if HEAD has moved since the original run, the old BaseCommit and new HEAD differ.
// New variant branches would be created at HEAD while comparisons use BaseCommit,
// producing incorrect diffs. Setup must reject this scenario.
func TestSetup_NewRunErrorsWhenHEADDiffersFromBaseCommit(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")
	git := newMockGitClient()
	// Mock HEAD is "abc123def456" — differs from metadata BaseCommit "old-base-commit"

	cfg := Config{Count: 3, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// Metadata from a previous run with a different base commit
	mgr.metadata = &VariantsMetadata{
		RunID:          "previous-run",
		BaseCommit:     "old-base-commit", // Different from current HEAD
		OriginalBranch: "feature/test-spec",
		StartedAt:      time.Now(),
		Variants: []*Variant{
			{ID: 1, Branch: "orbit-impl-1/test-spec", WorktreePath: "/tmp/wt1", Status: StatusCompleted},
			{ID: 2, Branch: "orbit-impl-2/test-spec", WorktreePath: "/tmp/wt2", Status: StatusFailed},
		},
	}

	// Create metadata file
	orbitDir := filepath.Join(specDir, ".orbit")
	if err := os.MkdirAll(orbitDir, 0755); err != nil {
		t.Fatalf("create orbit dir: %v", err)
	}
	metadataBytes, err := json.Marshal(mgr.metadata)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(orbitDir, "variants.json"), metadataBytes, 0644); err != nil {
		t.Fatalf("create variants.json: %v", err)
	}

	err = mgr.Setup(t.Context(), false)

	// Must return an error because HEAD != BaseCommit with preserved completed variants
	if err == nil {
		t.Fatal("expected error when HEAD differs from BaseCommit with preserved completed variants, got nil")
	}
	if !contains(err.Error(), "base commit") {
		t.Errorf("error should mention base commit mismatch, got: %v", err)
	}

	// Verify no worktrees were removed — error should fire before cleanup (T-191 review fix)
	if len(git.removedWorktrees) != 0 {
		t.Errorf("expected 0 removed worktrees (error before cleanup), got %d", len(git.removedWorktrees))
	}
}

// TestSetup_NewRunSucceedsWhenHEADMatchesBaseCommit verifies that the T-191 fix
// allows new runs when HEAD matches the preserved metadata's BaseCommit. This is
// the happy path: completed variants remain valid because all branches share the
// same base.
func TestSetup_NewRunSucceedsWhenHEADMatchesBaseCommit(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")
	git := newMockGitClient()
	// Mock HEAD is "abc123def456"

	cfg := Config{Count: 3, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// Metadata from a previous run with MATCHING base commit
	mgr.metadata = &VariantsMetadata{
		RunID:          "previous-run",
		BaseCommit:     "abc123def456", // Same as current HEAD
		OriginalBranch: "feature/test-spec",
		StartedAt:      time.Now(),
		Variants: []*Variant{
			{ID: 1, Branch: "orbit-impl-1/test-spec", WorktreePath: "/tmp/wt1", Status: StatusCompleted},
			{ID: 2, Branch: "orbit-impl-2/test-spec", WorktreePath: "/tmp/wt2", Status: StatusFailed},
			{ID: 3, Branch: "orbit-impl-3/test-spec", WorktreePath: "/tmp/wt3", Status: StatusPending},
		},
	}

	// Create metadata file
	orbitDir := filepath.Join(specDir, ".orbit")
	if err := os.MkdirAll(orbitDir, 0755); err != nil {
		t.Fatalf("create orbit dir: %v", err)
	}
	metadataBytes, err := json.Marshal(mgr.metadata)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(orbitDir, "variants.json"), metadataBytes, 0644); err != nil {
		t.Fatalf("create variants.json: %v", err)
	}

	err = mgr.Setup(t.Context(), false)

	// Must succeed — HEAD matches BaseCommit
	if err != nil {
		t.Fatalf("Setup should succeed when HEAD matches BaseCommit, got: %v", err)
	}

	// Verify completed variant was preserved and new ones were created
	variants := mgr.GetVariantsSnapshot()
	if len(variants) != 3 {
		t.Fatalf("expected 3 variants, got %d", len(variants))
	}

	v1 := mgr.GetVariant(1)
	if v1 == nil || v1.Status != StatusCompleted {
		t.Errorf("variant 1 should be completed, got %v", v1)
	}
}

// TestSetup_NewRunNoCompletedVariantsAllowsDifferentHEAD verifies that when
// all previous variants are non-completed, a fresh run is started regardless
// of HEAD vs BaseCommit. The T-191 check only applies when completed variants
// are being preserved.
func TestSetup_NewRunNoCompletedVariantsAllowsDifferentHEAD(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")
	git := newMockGitClient()
	// Mock HEAD is "abc123def456" — differs from metadata BaseCommit

	cfg := Config{Count: 2, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// All variants are non-completed — cleanup will remove everything
	mgr.metadata = &VariantsMetadata{
		RunID:          "previous-run",
		BaseCommit:     "old-base-commit", // Different from HEAD
		OriginalBranch: "feature/test-spec",
		StartedAt:      time.Now(),
		Variants: []*Variant{
			{ID: 1, Branch: "orbit-impl-1/test-spec", WorktreePath: "/tmp/wt1", Status: StatusFailed},
			{ID: 2, Branch: "orbit-impl-2/test-spec", WorktreePath: "/tmp/wt2", Status: StatusPending},
		},
	}

	// Create metadata file
	orbitDir := filepath.Join(specDir, ".orbit")
	if err := os.MkdirAll(orbitDir, 0755); err != nil {
		t.Fatalf("create orbit dir: %v", err)
	}
	metadataBytes, err := json.Marshal(mgr.metadata)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(orbitDir, "variants.json"), metadataBytes, 0644); err != nil {
		t.Fatalf("create variants.json: %v", err)
	}

	err = mgr.Setup(t.Context(), false)

	// Should succeed — no completed variants to preserve, so a fresh run starts
	if err != nil {
		t.Fatalf("Setup should succeed when no completed variants exist, got: %v", err)
	}

	// BaseCommit should be the current HEAD, not the old one
	if mgr.metadata.BaseCommit != "abc123def456" {
		t.Errorf("BaseCommit should be current HEAD 'abc123def456', got %q", mgr.metadata.BaseCommit)
	}
}

// TestSetup_CleansUpBranchesOnPartialFailure is the regression test for T-821.
// When variant setup fails partway through (e.g., 3rd branch creation fails),
// branches that were already created must be cleaned up to avoid orphaned branches.
func TestSetup_CleansUpBranchesOnPartialFailure(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")

	// Use a mock that fails on the 3rd CreateBranch call
	git := &failOnNthBranchMock{
		mockGitClient: newMockGitClient(),
		failOnCall:    3,
	}

	cfg := Config{Count: 3, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx := context.Background()
	err = mgr.Setup(ctx, false)
	if err == nil {
		t.Fatal("expected error from Setup")
	}

	// Branches 1 and 2 were created successfully before branch 3 failed.
	// They must be deleted during cleanup.
	if len(git.deletedBranches) != 2 {
		t.Errorf("expected 2 deleted branches, got %d: %v",
			len(git.deletedBranches), git.deletedBranches)
	}

	// Worktrees 1 and 2 were also created and must be removed.
	if len(git.removedWorktrees) != 2 {
		t.Errorf("expected 2 removed worktrees, got %d: %v",
			len(git.removedWorktrees), git.removedWorktrees)
	}
}

// TestSetup_CleansUpBranchesOnWorktreeFailure verifies that when worktree creation
// fails, the branch for that worktree AND all previously created branches are cleaned up.
func TestSetup_CleansUpBranchesOnWorktreeFailure(t *testing.T) {
	tmpDir := t.TempDir()
	specDir := filepath.Join(tmpDir, "specs", "test-spec")

	// Use a mock that fails on the 2nd CreateWorktree call
	git := &failOnNthWorktreeMock{
		mockGitClient: newMockGitClient(),
		failOnCall:    2,
	}

	cfg := Config{Count: 3, BranchPrefix: "orbit-impl"}
	mgr, err := NewManager(cfg, "test-spec", specDir, tmpDir, git)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx := context.Background()
	err = mgr.Setup(ctx, false)
	if err == nil {
		t.Fatal("expected error from Setup")
	}

	// Branch 1 was created with its worktree, branch 2 was created but worktree failed.
	// Both branches must be deleted.
	if len(git.deletedBranches) != 2 {
		t.Errorf("expected 2 deleted branches, got %d: %v",
			len(git.deletedBranches), git.deletedBranches)
	}

	// Only worktree 1 was successfully created, so only it should be removed.
	if len(git.removedWorktrees) != 1 {
		t.Errorf("expected 1 removed worktree, got %d: %v",
			len(git.removedWorktrees), git.removedWorktrees)
	}
}

// failOnNthBranchMock wraps mockGitClient but fails CreateBranch on the Nth call.
type failOnNthBranchMock struct {
	*mockGitClient
	failOnCall int
	callCount  int
}

func (m *failOnNthBranchMock) CreateBranch(name, commit string) error {
	m.callCount++
	if m.callCount == m.failOnCall {
		return fmt.Errorf("simulated branch creation failure")
	}
	return m.mockGitClient.CreateBranch(name, commit)
}

// failOnNthWorktreeMock wraps mockGitClient but fails CreateWorktree on the Nth call.
type failOnNthWorktreeMock struct {
	*mockGitClient
	failOnCall int
	callCount  int
}

func (m *failOnNthWorktreeMock) CreateWorktree(ctx context.Context, path, branch string) error {
	m.callCount++
	if m.callCount == m.failOnCall {
		return fmt.Errorf("simulated worktree creation failure")
	}
	return m.mockGitClient.CreateWorktree(ctx, path, branch)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && searchString(s, substr)))
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
