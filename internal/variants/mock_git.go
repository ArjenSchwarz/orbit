package variants

import (
	"context"
	"sync"
)

// MockGit implements GitClient with configurable responses for testing.
type MockGit struct {
	mu sync.Mutex

	// Configurable return values
	CurrentBranch      string
	CurrentBranchErr   error
	HeadCommit         string
	HeadCommitErr      error
	CreateBranchErr    error
	CreateWorktreeErr  error
	RemoveWorktreeErr  error
	DeleteBranchErr    error
	Diff               string
	DiffErr            error
	RebaseErr          error
	Diverged           bool
	DivergedErr        error
	UncommittedChanges bool
	UncommittedErr     error

	// Call tracking
	CreatedBranches  []string
	CreatedWorktrees []MockWorktreeCall
	RemovedWorktrees []string
	DeletedBranches  []string
	DiffCalls        []MockDiffCall
	RebaseCalls      []MockRebaseCall
	DivergedCalls    []MockDivergedCall

	// Per-call overrides (for more complex scenarios)
	CreateBranchErrors   map[string]error
	CreateWorktreeErrors map[string]error
	DiffResults          map[string]string
}

// MockWorktreeCall records a CreateWorktree call.
type MockWorktreeCall struct {
	Path   string
	Branch string
}

// MockDiffCall records a GetDiff call.
type MockDiffCall struct {
	WorktreePath string
	BaseCommit   string
}

// MockRebaseCall records a Rebase call.
type MockRebaseCall struct {
	SourceBranch string
	TargetBranch string
}

// MockDivergedCall records a BranchHasDiverged call.
type MockDivergedCall struct {
	Branch     string
	BaseCommit string
}

// NewMockGit creates a new MockGit with reasonable defaults.
func NewMockGit() *MockGit {
	return &MockGit{
		CurrentBranch:        "main",
		HeadCommit:           "abc123def456",
		CreatedBranches:      []string{},
		CreatedWorktrees:     []MockWorktreeCall{},
		RemovedWorktrees:     []string{},
		DeletedBranches:      []string{},
		DiffCalls:            []MockDiffCall{},
		RebaseCalls:          []MockRebaseCall{},
		DivergedCalls:        []MockDivergedCall{},
		CreateBranchErrors:   map[string]error{},
		CreateWorktreeErrors: map[string]error{},
		DiffResults:          map[string]string{},
	}
}

// GetCurrentBranch returns the configured current branch.
func (m *MockGit) GetCurrentBranch() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.CurrentBranch, m.CurrentBranchErr
}

// GetHeadCommit returns the configured head commit.
func (m *MockGit) GetHeadCommit() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.HeadCommit, m.HeadCommitErr
}

// CreateBranch records the call and returns the configured error.
func (m *MockGit) CreateBranch(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CreatedBranches = append(m.CreatedBranches, name)

	if err, ok := m.CreateBranchErrors[name]; ok {
		return err
	}
	return m.CreateBranchErr
}

// CreateWorktree records the call and returns the configured error.
func (m *MockGit) CreateWorktree(_ context.Context, path, branch string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CreatedWorktrees = append(m.CreatedWorktrees, MockWorktreeCall{
		Path:   path,
		Branch: branch,
	})

	if err, ok := m.CreateWorktreeErrors[path]; ok {
		return err
	}
	return m.CreateWorktreeErr
}

// RemoveWorktree records the call and returns the configured error.
func (m *MockGit) RemoveWorktree(_ context.Context, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RemovedWorktrees = append(m.RemovedWorktrees, path)
	return m.RemoveWorktreeErr
}

// DeleteBranch records the call and returns the configured error.
func (m *MockGit) DeleteBranch(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DeletedBranches = append(m.DeletedBranches, name)
	return m.DeleteBranchErr
}

// GetDiff records the call and returns the configured diff.
func (m *MockGit) GetDiff(_ context.Context, worktreePath, baseCommit string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DiffCalls = append(m.DiffCalls, MockDiffCall{
		WorktreePath: worktreePath,
		BaseCommit:   baseCommit,
	})

	if diff, ok := m.DiffResults[worktreePath]; ok {
		return diff, nil
	}
	return m.Diff, m.DiffErr
}

// Rebase records the call and returns the configured error.
func (m *MockGit) Rebase(_ context.Context, sourceBranch, targetBranch string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RebaseCalls = append(m.RebaseCalls, MockRebaseCall{
		SourceBranch: sourceBranch,
		TargetBranch: targetBranch,
	})
	return m.RebaseErr
}

// BranchHasDiverged records the call and returns the configured result.
func (m *MockGit) BranchHasDiverged(branch, baseCommit string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DivergedCalls = append(m.DivergedCalls, MockDivergedCall{
		Branch:     branch,
		BaseCommit: baseCommit,
	})
	return m.Diverged, m.DivergedErr
}

// HasUncommittedChanges returns the configured result.
func (m *MockGit) HasUncommittedChanges() (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.UncommittedChanges, m.UncommittedErr
}

// GetCommitLog returns mock commit messages.
func (m *MockGit) GetCommitLog(_ context.Context, _, _ string) ([]string, error) {
	return []string{"abc123 Mock commit message"}, nil
}

// GetDiffStat returns mock diff stats.
func (m *MockGit) GetDiffStat(_ context.Context, _, _ string) (string, error) {
	return " 3 files changed, 50 insertions(+), 10 deletions(-)", nil
}

// Reset clears all recorded calls.
func (m *MockGit) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CreatedBranches = []string{}
	m.CreatedWorktrees = []MockWorktreeCall{}
	m.RemovedWorktrees = []string{}
	m.DeletedBranches = []string{}
	m.DiffCalls = []MockDiffCall{}
	m.RebaseCalls = []MockRebaseCall{}
	m.DivergedCalls = []MockDivergedCall{}
}

// Verify MockGit implements GitClient.
var _ GitClient = (*MockGit)(nil)
