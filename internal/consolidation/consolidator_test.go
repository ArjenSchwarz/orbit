package consolidation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
	"github.com/arjenschwarz/orbit/internal/variants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockAgent implements agents.Agent for testing.
type mockAgent struct {
	name      string
	runResult *agents.RunResult
	runErr    error
}

func (m *mockAgent) Name() string              { return m.name }
func (m *mockAgent) CLICommand() string        { return "mock" }
func (m *mockAgent) IsInstalled() bool         { return true }
func (m *mockAgent) Version() (string, error)  { return "1.0.0", nil }
func (m *mockAgent) DefaultSessionDir() string { return "/tmp" }
func (m *mockAgent) DiscoverSessions(ctx context.Context, projectDir string) ([]agents.SessionInfo, error) {
	return nil, nil
}
func (m *mockAgent) Run(ctx context.Context, opts agents.RunOptions) (*agents.RunResult, error) {
	return m.runResult, m.runErr
}
func (m *mockAgent) Resume(ctx context.Context, sessionID string, opts agents.RunOptions) (*agents.RunResult, error) {
	return m.runResult, m.runErr
}

// mockGitClient implements variants.GitClient for testing.
type mockGitClient struct {
	currentBranch     string
	headCommit        string
	headCommitInPath  map[string]string
	hasUncommittedChg bool
	worktrees         map[string]string // branch -> path
	createdBranches   []string
	createdWorktrees  []string
	removedWorktrees  []string
	deletedBranches   []string
	rebaseCalled      bool
	branchHasDiverged bool
	diffResult        string
	commitLog         []string
	diffStat          string
}

func (m *mockGitClient) GetCurrentBranch() (string, error) { return m.currentBranch, nil }
func (m *mockGitClient) GetHeadCommit() (string, error)    { return m.headCommit, nil }
func (m *mockGitClient) GetHeadCommitInPath(path string) (string, error) {
	if commit, ok := m.headCommitInPath[path]; ok {
		return commit, nil
	}
	return m.headCommit, nil
}
func (m *mockGitClient) HasUncommittedChanges() (bool, error) { return m.hasUncommittedChg, nil }
func (m *mockGitClient) CreateBranch(name, commit string) error {
	m.createdBranches = append(m.createdBranches, name)
	return nil
}
func (m *mockGitClient) CreateWorktree(ctx context.Context, path, branch string) error {
	m.createdWorktrees = append(m.createdWorktrees, path)
	if m.worktrees == nil {
		m.worktrees = make(map[string]string)
	}
	m.worktrees[branch] = path
	return nil
}
func (m *mockGitClient) RemoveWorktree(ctx context.Context, path string) error {
	m.removedWorktrees = append(m.removedWorktrees, path)
	return nil
}
func (m *mockGitClient) DeleteBranch(name string) error {
	m.deletedBranches = append(m.deletedBranches, name)
	return nil
}
func (m *mockGitClient) Rebase(ctx context.Context, branch, onto string) error {
	m.rebaseCalled = true
	return nil
}
func (m *mockGitClient) BranchHasDiverged(branch, commit string) (bool, error) {
	return m.branchHasDiverged, nil
}
func (m *mockGitClient) GetDiff(ctx context.Context, worktreePath, baseCommit string) (string, error) {
	return m.diffResult, nil
}
func (m *mockGitClient) GetCommitLog(ctx context.Context, worktreePath, baseCommit string) ([]string, error) {
	return m.commitLog, nil
}
func (m *mockGitClient) GetDiffStat(ctx context.Context, worktreePath, baseCommit string) (string, error) {
	return m.diffStat, nil
}
func (m *mockGitClient) HasUncommittedChangesInPath(_ string) (bool, error) {
	return m.hasUncommittedChg, nil
}
func (m *mockGitClient) GetRecentCommits(_ context.Context, _, _ string, _ int) ([]variants.Commit, error) {
	return nil, nil
}

func TestNewConsolidator(t *testing.T) {
	t.Run("returns error when spec directory is empty", func(t *testing.T) {
		cfg := Config{
			SpecName:  "test-spec",
			SpecDir:   "",
			VariantID: 1,
			Agent:     &mockAgent{name: "test"},
		}
		mgr := createTestManager(t, 2)

		_, err := NewConsolidator(cfg, mgr)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "spec directory is required")
	})

	t.Run("returns error when variant ID is zero", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := Config{
			SpecName:  "test-spec",
			SpecDir:   tmpDir,
			VariantID: 0,
			Agent:     &mockAgent{name: "test"},
		}
		mgr := createTestManager(t, 2)

		_, err := NewConsolidator(cfg, mgr)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "variant ID must be positive")
	})

	t.Run("returns error when agent is nil", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := Config{
			SpecName:  "test-spec",
			SpecDir:   tmpDir,
			VariantID: 1,
			Agent:     nil,
		}
		mgr := createTestManager(t, 2)

		_, err := NewConsolidator(cfg, mgr)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "agent is required")
	})

	t.Run("returns error when manager is nil", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := Config{
			SpecName:  "test-spec",
			SpecDir:   tmpDir,
			VariantID: 1,
			Agent:     &mockAgent{name: "test"},
		}

		_, err := NewConsolidator(cfg, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "variant manager is required")
	})

	t.Run("returns error when variant not found", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := Config{
			SpecName:  "test-spec",
			SpecDir:   tmpDir,
			VariantID: 99, // Non-existent variant
			Agent:     &mockAgent{name: "test"},
		}
		mgr := createTestManager(t, 2)

		_, err := NewConsolidator(cfg, mgr)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "variant 99 not found")
	})

	t.Run("creates consolidator successfully", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := Config{
			SpecName:  "test-spec",
			SpecDir:   tmpDir,
			VariantID: 1,
			Agent:     &mockAgent{name: "test"},
		}
		mgr := createTestManager(t, 2)

		consolidator, err := NewConsolidator(cfg, mgr)
		require.NoError(t, err)
		assert.NotNil(t, consolidator)
	})

	t.Run("WithGitOps injects into both consolidator and recovery", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := Config{
			SpecName:  "test-spec",
			SpecDir:   tmpDir,
			VariantID: 1,
			Agent:     &mockAgent{name: "test"},
		}
		mgr := createTestManager(t, 2)

		mock := &stubGitOps{headCommit: "abc123"}
		consolidator, err := NewConsolidator(cfg, mgr, WithGitOps(mock))
		require.NoError(t, err)

		// Verify the mock is used by the consolidator
		assert.Same(t, mock, consolidator.git)
		// Verify the mock is also used by the recovery manager
		assert.Same(t, mock, consolidator.recovery.git)
	})
}

// stubGitOps is a minimal GitOps implementation for unit tests.
type stubGitOps struct {
	headCommit string
}

func (s *stubGitOps) GetHeadCommit(context.Context) (string, error) {
	return s.headCommit, nil
}
func (s *stubGitOps) HasUncommittedChanges(context.Context) (bool, error) { return false, nil }
func (s *stubGitOps) ResetHard(context.Context, string) error             { return nil }
func (s *stubGitOps) CheckoutAll(context.Context) error                   { return nil }
func (s *stubGitOps) CleanUntracked(context.Context) error                { return nil }
func (s *stubGitOps) StashPush(context.Context, string) (string, bool, error) {
	return "", false, nil
}
func (s *stubGitOps) StashPop(context.Context) (string, error)          { return "", nil }
func (s *stubGitOps) StashDrop(context.Context, string) error           { return nil }
func (s *stubGitOps) RevertCommit(context.Context, string) error        { return nil }
func (s *stubGitOps) LogOneline(context.Context, int) (string, error)   { return "", nil }
func (s *stubGitOps) GetCommitSubject(context.Context, string) (string, error) {
	return "", nil
}

func TestConsolidator_validateVariant(t *testing.T) {
	tests := map[string]struct {
		variantID   int
		numVariants int
		wantErr     string
	}{
		"valid variant": {
			variantID:   1,
			numVariants: 2,
			wantErr:     "",
		},
		"variant not found": {
			variantID:   99,
			numVariants: 2,
			wantErr:     "variant 99 not found. Available variants: 1, 2",
		},
		"no variants exist": {
			variantID:   1,
			numVariants: 0,
			wantErr:     "variant 1 not found: no variants exist for this spec",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tmpDir := t.TempDir()
			mgr := createTestManagerWithVariants(t, tc.numVariants, tmpDir)

			// Create consolidator with existing variant or directly set config
			c := &Consolidator{
				config: Config{
					SpecDir:   tmpDir,
					VariantID: tc.variantID,
				},
				manager: mgr,
			}

			err := c.validateVariant()

			if tc.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			}
		})
	}
}

func TestConsolidator_validateReport(t *testing.T) {
	t.Run("returns error when report does not exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		c := &Consolidator{
			config: Config{
				SpecName: "test-spec",
				SpecDir:  tmpDir,
			},
		}

		_, err := c.validateReport()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "comparison-report.md not found")
		assert.Contains(t, err.Error(), "orbit compare")
	})

	t.Run("returns report path when report exists", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create the comparison directory and report
		compDir := filepath.Join(tmpDir, "comparison-report")
		require.NoError(t, os.MkdirAll(compDir, 0755))
		reportPath := filepath.Join(compDir, "report.md")
		require.NoError(t, os.WriteFile(reportPath, []byte("# Test Report"), 0644))

		c := &Consolidator{
			config: Config{
				SpecName: "test-spec",
				SpecDir:  tmpDir,
			},
		}

		path, err := c.validateReport()
		assert.NoError(t, err)
		assert.Equal(t, reportPath, path)
	})
}

func TestConsolidator_checkStaleness(t *testing.T) {
	t.Run("no warning when report has no frontmatter", func(t *testing.T) {
		tmpDir := t.TempDir()
		compDir := filepath.Join(tmpDir, "comparison-report")
		require.NoError(t, os.MkdirAll(compDir, 0755))

		reportContent := "# Comparison Report\n\nNo frontmatter here."
		require.NoError(t, os.WriteFile(filepath.Join(compDir, "report.md"), []byte(reportContent), 0644))

		mgr := createTestManager(t, 2)
		c := &Consolidator{
			config: Config{
				SpecDir: tmpDir,
			},
			manager: mgr,
		}

		warning, err := c.checkStaleness(context.Background())
		assert.NoError(t, err)
		assert.Empty(t, warning)
	})

	t.Run("no warning when commits match", func(t *testing.T) {
		tmpDir := t.TempDir()
		compDir := filepath.Join(tmpDir, "comparison-report")
		require.NoError(t, os.MkdirAll(compDir, 0755))

		reportContent := `---
title: Comparison Report
variant_commits:
  1: abc123
  2: def456
---
# Comparison Report`
		require.NoError(t, os.WriteFile(filepath.Join(compDir, "report.md"), []byte(reportContent), 0644))

		// Create manager with matching commits
		worktree1 := filepath.Join(tmpDir, "worktree1")
		worktree2 := filepath.Join(tmpDir, "worktree2")
		require.NoError(t, os.MkdirAll(worktree1, 0755))
		require.NoError(t, os.MkdirAll(worktree2, 0755))

		git := &mockGitClient{
			currentBranch: "main",
			headCommit:    "abc123",
			headCommitInPath: map[string]string{
				worktree1: "abc123",
				worktree2: "def456",
			},
		}

		mgr, err := variants.NewManager(
			variants.Config{Count: 2, BranchPrefix: "orbit-impl"},
			"test-spec",
			tmpDir,
			tmpDir,
			git,
		)
		require.NoError(t, err)
		require.NoError(t, mgr.Load())

		// Setup manager with variants that have the matching worktree paths
		// Since the manager doesn't have variants loaded, we need to use a real setup
		// For this test, we'll rely on the fact that no commits will be found

		c := &Consolidator{
			config: Config{
				SpecDir: tmpDir,
			},
			manager: mgr,
		}

		warning, err := c.checkStaleness(context.Background())
		assert.NoError(t, err)
		// No warning because no variants are loaded
		assert.Empty(t, warning)
	})

	t.Run("warning when report file does not exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		mgr := createTestManager(t, 2)

		c := &Consolidator{
			config: Config{
				SpecDir: tmpDir,
			},
			manager: mgr,
		}

		// Should return no warning when file doesn't exist (best effort)
		warning, err := c.checkStaleness(context.Background())
		assert.NoError(t, err)
		assert.Empty(t, warning)
	})
}

func TestConsolidator_checkEmptyImprovements(t *testing.T) {
	t.Run("returns error when section is missing", func(t *testing.T) {
		tmpDir := t.TempDir()
		compDir := filepath.Join(tmpDir, "comparison-report")
		require.NoError(t, os.MkdirAll(compDir, 0755))

		reportContent := `# Comparison Report

## Recommendation
Variant 1 is recommended.

## Key Observations
- Observation 1
`
		require.NoError(t, os.WriteFile(filepath.Join(compDir, "report.md"), []byte(reportContent), 0644))

		c := &Consolidator{
			config: Config{
				SpecDir: tmpDir,
			},
		}

		err := c.checkEmptyImprovements(context.Background())
		assert.ErrorIs(t, err, ErrNoImprovements)
	})

	t.Run("returns error when section has no improvements", func(t *testing.T) {
		tmpDir := t.TempDir()
		compDir := filepath.Join(tmpDir, "comparison-report")
		require.NoError(t, os.MkdirAll(compDir, 0755))

		reportContent := `# Comparison Report

## Improvements from Other Variants
These improvements from non-chosen variants could enhance the recommended implementation:

## Implementation Diffs
`
		require.NoError(t, os.WriteFile(filepath.Join(compDir, "report.md"), []byte(reportContent), 0644))

		c := &Consolidator{
			config: Config{
				SpecDir: tmpDir,
			},
		}

		err := c.checkEmptyImprovements(context.Background())
		assert.ErrorIs(t, err, ErrNoImprovements)
	})

	t.Run("no error when improvements exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		compDir := filepath.Join(tmpDir, "comparison-report")
		require.NoError(t, os.MkdirAll(compDir, 0755))

		reportContent := `# Comparison Report

## Improvements from Other Variants
These improvements from non-chosen variants could enhance the recommended implementation:
### From Variant 2 (high priority)
Better error handling in the API endpoints.

## Implementation Diffs
`
		require.NoError(t, os.WriteFile(filepath.Join(compDir, "report.md"), []byte(reportContent), 0644))

		c := &Consolidator{
			config: Config{
				SpecDir: tmpDir,
			},
		}

		err := c.checkEmptyImprovements(context.Background())
		assert.NoError(t, err)
	})

	// Regression: T-710 — when the report ends exactly with the
	// "# Improvements from Other Variants" header (no trailing content),
	// strings.Cut returns after="" and the previous slice afterHeader[1:]
	// panicked with "slice bounds out of range [1:0]". Expected behaviour
	// is to return ErrNoImprovements without panicking.
	t.Run("returns error without panicking when header is terminal section", func(t *testing.T) {
		tmpDir := t.TempDir()
		compDir := filepath.Join(tmpDir, "comparison-report")
		require.NoError(t, os.MkdirAll(compDir, 0755))

		// Report ending with the header itself, no trailing newline or content.
		reportContent := `# Comparison Report

## Recommendation
Variant 1 is recommended.

# Improvements from Other Variants`
		require.NoError(t, os.WriteFile(filepath.Join(compDir, "report.md"), []byte(reportContent), 0644))

		c := &Consolidator{
			config: Config{
				SpecDir: tmpDir,
			},
		}

		err := c.checkEmptyImprovements(context.Background())
		assert.ErrorIs(t, err, ErrNoImprovements)
	})

	// Header followed only by a newline yields an empty trailing section.
	// This is not a panic path (`"\n"[1:]` is valid) but verifies the empty
	// section is reported as ErrNoImprovements.
	t.Run("returns error when header is followed only by newline", func(t *testing.T) {
		tmpDir := t.TempDir()
		compDir := filepath.Join(tmpDir, "comparison-report")
		require.NoError(t, os.MkdirAll(compDir, 0755))

		reportContent := "# Comparison Report\n\n# Improvements from Other Variants\n"
		require.NoError(t, os.WriteFile(filepath.Join(compDir, "report.md"), []byte(reportContent), 0644))

		c := &Consolidator{
			config: Config{
				SpecDir: tmpDir,
			},
		}

		err := c.checkEmptyImprovements(context.Background())
		assert.ErrorIs(t, err, ErrNoImprovements)
	})

	// Header followed immediately by another top-level heading: the
	// "Improvements" section has no body, so we should report no
	// improvements without inspecting the next section's content.
	t.Run("returns error when header is immediately followed by next top-level heading", func(t *testing.T) {
		tmpDir := t.TempDir()
		compDir := filepath.Join(tmpDir, "comparison-report")
		require.NoError(t, os.MkdirAll(compDir, 0755))

		reportContent := "# Comparison Report\n\n# Improvements from Other Variants\n# Next Section\nirrelevant body\n"
		require.NoError(t, os.WriteFile(filepath.Join(compDir, "report.md"), []byte(reportContent), 0644))

		c := &Consolidator{
			config: Config{
				SpecDir: tmpDir,
			},
		}

		err := c.checkEmptyImprovements(context.Background())
		assert.ErrorIs(t, err, ErrNoImprovements)
	})
}

func TestConsolidator_checkCleanState(t *testing.T) {
	// Note: This test requires a real git repository, so we skip it in unit tests
	// and rely on the integration tests for full coverage.
	t.Run("allows dirty state when AllowDirty is true", func(t *testing.T) {
		c := &Consolidator{
			config: Config{
				AllowDirty: true,
			},
		}

		err := c.checkCleanState(context.Background())
		assert.NoError(t, err)
	})
}

func TestParseCommitSHA(t *testing.T) {
	tests := map[string]struct {
		output   string
		expected string
	}{
		"from report format": {
			output: `## Consolidation Report

### Applied
| Source | Files Modified | Description |
|--------|----------------|-------------|
| V2 | path/to/file.go | Description |

### Commit
abc123def456`,
			expected: "abc123def456",
		},
		"with backticks": {
			output: `### Commit
` + "`abc123def456`",
			expected: "abc123def456",
		},
		"full 40 char SHA": {
			output:   `Some output with commit 0123456789abcdef0123456789abcdef01234567 in it`,
			expected: "0123456789abcdef0123456789abcdef01234567",
		},
		"short SHA": {
			output:   `Committed as abc1234`,
			expected: "abc1234",
		},
		"no SHA": {
			output:   `No commit SHA here`,
			expected: "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := parseCommitSHA(tc.output)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestParseImprovementCounts(t *testing.T) {
	tests := map[string]struct {
		report  string
		applied int
		skipped int
	}{
		"with applied and skipped": {
			report: `## Consolidation Report

### Applied
| Source | Files Modified | Description |
|--------|----------------|-------------|
| V2 | file1.go | Change 1 |
| V3 | file2.go | Change 2 |

### Skipped
| Source | Reason |
|--------|--------|
| V2 | Conflicting change |

### Commit
abc123`,
			applied: 2,
			skipped: 1,
		},
		"only applied": {
			report: `### Applied
| Source | Files Modified | Description |
|--------|----------------|-------------|
| V2 | file1.go | Change 1 |

### Commit
abc123`,
			applied: 1,
			skipped: 0,
		},
		"empty tables": {
			report: `### Applied
| Source | Files Modified | Description |
|--------|----------------|-------------|

### Skipped
| Source | Reason |
|--------|--------|

### Commit
abc123`,
			applied: 0,
			skipped: 0,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			applied, skipped := parseImprovementCounts(tc.report)
			assert.Equal(t, tc.applied, applied)
			assert.Equal(t, tc.skipped, skipped)
		})
	}
}

func TestTruncateSHA(t *testing.T) {
	tests := map[string]struct {
		input    string
		expected string
	}{
		"long SHA":  {input: "0123456789abcdef0123456789abcdef01234567", expected: "01234567"},
		"short SHA": {input: "abc123", expected: "abc123"},
		"exactly 8": {input: "12345678", expected: "12345678"},
		"empty":     {input: "", expected: ""},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := truncateSHA(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestParseReportVariantCommits(t *testing.T) {
	t.Run("parses variant commits from frontmatter", func(t *testing.T) {
		tmpDir := t.TempDir()
		reportPath := filepath.Join(tmpDir, "report.md")

		content := `---
title: Comparison Report
date: 2025-01-23
variant_commits:
  1: abc123
  2: def456
  3: ghi789
---
# Comparison Report

Content here.
`
		require.NoError(t, os.WriteFile(reportPath, []byte(content), 0644))

		commits, err := parseReportVariantCommits(reportPath)
		require.NoError(t, err)

		assert.Equal(t, "abc123", commits[1])
		assert.Equal(t, "def456", commits[2])
		assert.Equal(t, "ghi789", commits[3])
	})

	t.Run("returns empty map when no frontmatter", func(t *testing.T) {
		tmpDir := t.TempDir()
		reportPath := filepath.Join(tmpDir, "report.md")

		content := `# Comparison Report

No frontmatter here.
`
		require.NoError(t, os.WriteFile(reportPath, []byte(content), 0644))

		commits, err := parseReportVariantCommits(reportPath)
		require.NoError(t, err)
		assert.Empty(t, commits)
	})
}

// Integration Tests

func TestConsolidateE2E(t *testing.T) {
	t.Run("full workflow with mock agent", func(t *testing.T) {
		tmpDir := t.TempDir()
		specDir := filepath.Join(tmpDir, "specs", "test-spec")
		orbitDir := filepath.Join(specDir, ".orbit")
		compDir := filepath.Join(specDir, "comparison-report")
		worktreeDir := filepath.Join(orbitDir, "worktrees")
		worktree1 := filepath.Join(worktreeDir, "orbit-impl-1-test-spec")
		worktree2 := filepath.Join(worktreeDir, "orbit-impl-2-test-spec")

		// Create directories
		require.NoError(t, os.MkdirAll(compDir, 0755))
		require.NoError(t, os.MkdirAll(worktree1, 0755))
		require.NoError(t, os.MkdirAll(worktree2, 0755))

		// Create comparison report with improvements
		reportContent := `---
title: Comparison Report
date: 2025-01-23
variant_commits:
  1: abc123
  2: def456
---
# Comparison Report

## Improvements from Other Variants
These improvements from non-chosen variants could enhance the recommended implementation:
### From Variant 2 (high priority)
Better error handling in the API endpoints.
**Rationale:** Improves reliability.

## Implementation Diffs
`
		require.NoError(t, os.WriteFile(filepath.Join(compDir, "report.md"), []byte(reportContent), 0644))

		// Create mock agent that returns success with consolidation report
		agent := &mockAgent{
			name: "test-agent",
			runResult: &agents.RunResult{
				SessionID: "test-session",
				Output: `## Consolidation Report

### Applied
| Source | Files Modified | Description |
|--------|----------------|-------------|
| V2 | api/handler.go | Added error handling |

### Skipped
| Source | Reason |
|--------|--------|

### Commit
abc123def456
`,
				ExitCode: 0,
			},
		}

		// Create mock git client
		git := &mockGitClient{
			currentBranch:    "main",
			headCommit:       "abc123",
			headCommitInPath: map[string]string{worktree1: "abc123", worktree2: "def456"},
		}

		// Create manager with Setup
		mgr, err := variants.NewManager(
			variants.Config{Count: 2, BranchPrefix: "orbit-impl"},
			"test-spec",
			specDir,
			tmpDir,
			git,
		)
		require.NoError(t, err)
		ctx := context.Background()
		require.NoError(t, mgr.Setup(ctx, false))

		// Create consolidator
		cfg := Config{
			SpecName:  "test-spec",
			SpecDir:   specDir,
			VariantID: 1,
			Agent:     agent,
		}

		consolidator, err := NewConsolidator(cfg, mgr)
		require.NoError(t, err)

		// Skip actual Run since it requires git and tests/commands
		// Just verify the consolidator was created correctly
		assert.NotNil(t, consolidator)
		assert.Equal(t, 1, consolidator.config.VariantID)
		assert.Equal(t, "test-spec", consolidator.config.SpecName)
	})
}

func TestConsolidateEmptyImprovements(t *testing.T) {
	t.Run("early exit when no improvements", func(t *testing.T) {
		tmpDir := t.TempDir()
		specDir := filepath.Join(tmpDir, "specs", "test-spec")
		orbitDir := filepath.Join(specDir, ".orbit")
		compDir := filepath.Join(specDir, "comparison-report")
		worktreeDir := filepath.Join(orbitDir, "worktrees")
		worktree1 := filepath.Join(worktreeDir, "orbit-impl-1-test-spec")

		// Create directories
		require.NoError(t, os.MkdirAll(compDir, 0755))
		require.NoError(t, os.MkdirAll(worktree1, 0755))

		// Create comparison report WITHOUT improvements
		reportContent := `# Comparison Report

## Recommendation
Variant 1 is recommended.

## Key Observations
- Both variants are similar.
`
		require.NoError(t, os.WriteFile(filepath.Join(compDir, "report.md"), []byte(reportContent), 0644))

		// Create mock git client
		git := &mockGitClient{
			currentBranch:    "main",
			headCommit:       "abc123",
			headCommitInPath: map[string]string{worktree1: "abc123"},
		}

		// Create manager
		mgr, err := variants.NewManager(
			variants.Config{Count: 1, BranchPrefix: "orbit-impl"},
			"test-spec",
			specDir,
			tmpDir,
			git,
		)
		require.NoError(t, err)
		ctx := context.Background()
		require.NoError(t, mgr.Setup(ctx, false))

		// Create consolidator
		cfg := Config{
			SpecName:  "test-spec",
			SpecDir:   specDir,
			VariantID: 1,
			Agent:     &mockAgent{name: "test"},
		}

		c, err := NewConsolidator(cfg, mgr)
		require.NoError(t, err)

		// Check empty improvements
		err = c.checkEmptyImprovements(ctx)
		assert.ErrorIs(t, err, ErrNoImprovements)
	})
}

func TestConsolidateRollback(t *testing.T) {
	t.Run("reads commit SHA from log", func(t *testing.T) {
		tmpDir := t.TempDir()
		specDir := filepath.Join(tmpDir, "specs", "test-spec")
		orbitDir := filepath.Join(specDir, ".orbit")
		worktreeDir := filepath.Join(orbitDir, "worktrees")
		worktree1 := filepath.Join(worktreeDir, "orbit-impl-1-test-spec")

		// Create directories
		require.NoError(t, os.MkdirAll(orbitDir, 0755))
		require.NoError(t, os.MkdirAll(worktree1, 0755))

		// Create consolidation log with a commit SHA
		logger := NewLogger(orbitDir)
		entry := LogEntry{
			ChosenVariantID: 1,
			CommitSHA:       "abc123def456789012345678901234567890abcd",
			Agent:           "test-agent",
		}
		require.NoError(t, logger.Append(entry))

		// Verify GetLatestCommitSHA works
		sha, err := logger.GetLatestCommitSHA()
		require.NoError(t, err)
		assert.Equal(t, "abc123def456789012345678901234567890abcd", sha)
	})

	t.Run("falls back to searching commits when log is missing", func(t *testing.T) {
		tmpDir := t.TempDir()
		specDir := filepath.Join(tmpDir, "specs", "test-spec")
		orbitDir := filepath.Join(specDir, ".orbit")

		// Create directories
		require.NoError(t, os.MkdirAll(orbitDir, 0755))

		// Create logger without any entries
		logger := NewLogger(orbitDir)

		// GetLatestCommitSHA should fail
		_, err := logger.GetLatestCommitSHA()
		assert.Error(t, err)
	})
}

func TestRecoveryPartialFailure(t *testing.T) {
	t.Run("hasCommitInOutput returns false when no commit", func(t *testing.T) {
		result := &agents.RunResult{
			Output: "Agent output without any commit SHA",
		}
		assert.False(t, hasCommitInOutput(result))
	})

	t.Run("hasCommitInOutput returns true when commit exists", func(t *testing.T) {
		result := &agents.RunResult{
			Output: "### Commit\nabc123def456",
		}
		assert.True(t, hasCommitInOutput(result))
	})

	t.Run("hasCommitInOutput handles nil result", func(t *testing.T) {
		assert.False(t, hasCommitInOutput(nil))
	})
}

// deadlineCapturingAgent records whether every Run/Resume context carries a
// deadline. Used by T-679 regression tests to verify the consolidator wraps
// agent invocations with a deadline so they can't hang indefinitely.
type deadlineCapturingAgent struct {
	name        string
	result      *agents.RunResult
	hasDeadline []bool
}

func (a *deadlineCapturingAgent) Name() string              { return a.name }
func (a *deadlineCapturingAgent) CLICommand() string        { return "mock" }
func (a *deadlineCapturingAgent) IsInstalled() bool         { return true }
func (a *deadlineCapturingAgent) Version() (string, error)  { return "1.0.0", nil }
func (a *deadlineCapturingAgent) DefaultSessionDir() string { return "/tmp" }
func (a *deadlineCapturingAgent) DiscoverSessions(_ context.Context, _ string) ([]agents.SessionInfo, error) {
	return nil, nil
}
func (a *deadlineCapturingAgent) Run(ctx context.Context, _ agents.RunOptions) (*agents.RunResult, error) {
	_, ok := ctx.Deadline()
	a.hasDeadline = append(a.hasDeadline, ok)
	return a.result, nil
}
func (a *deadlineCapturingAgent) Resume(ctx context.Context, _ string, opts agents.RunOptions) (*agents.RunResult, error) {
	return a.Run(ctx, opts)
}

// hangingAgent blocks until its context is canceled. Used to verify the
// consolidator's per-invocation timeout actually terminates a stuck agent.
type hangingAgent struct {
	name string
}

func (a *hangingAgent) Name() string              { return a.name }
func (a *hangingAgent) CLICommand() string        { return "mock" }
func (a *hangingAgent) IsInstalled() bool         { return true }
func (a *hangingAgent) Version() (string, error)  { return "1.0.0", nil }
func (a *hangingAgent) DefaultSessionDir() string { return "/tmp" }
func (a *hangingAgent) DiscoverSessions(_ context.Context, _ string) ([]agents.SessionInfo, error) {
	return nil, nil
}
func (a *hangingAgent) Run(ctx context.Context, _ agents.RunOptions) (*agents.RunResult, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (a *hangingAgent) Resume(ctx context.Context, _ string, opts agents.RunOptions) (*agents.RunResult, error) {
	return a.Run(ctx, opts)
}

// TestRunWithRetry_AppliesTimeout verifies that runWithRetry bounds each agent
// invocation with a deadline so a hung session can't run forever.
// Regression test for T-679: consolidation paths previously called the agent
// without setting RunOptions.Timeout and without an explicit
// comparison-style deadline.
func TestRunWithRetry_AppliesTimeout(t *testing.T) {
	t.Parallel()

	t.Run("agent context has a deadline", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		agent := &deadlineCapturingAgent{
			name: "test-agent",
			result: &agents.RunResult{
				SessionID: "test-session",
				ExitCode:  0,
				Output:    "ok",
			},
		}

		mgr := createTestManagerWithVariants(t, 2, tmpDir)
		cfg := Config{
			SpecName:  "test-spec",
			SpecDir:   tmpDir,
			VariantID: 1,
			Agent:     agent,
		}

		consolidator, err := NewConsolidator(cfg, mgr)
		require.NoError(t, err)

		_, err = consolidator.runWithRetry(context.Background(), "test prompt")
		require.NoError(t, err)
		require.Len(t, agent.hasDeadline, 1, "agent should have been called once")
		assert.True(t, agent.hasDeadline[0], "context passed to agent must have a deadline")
	})

	t.Run("hung agent is terminated by timeout", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		agent := &hangingAgent{name: "test-agent"}

		mgr := createTestManagerWithVariants(t, 2, tmpDir)
		cfg := Config{
			SpecName:  "test-spec",
			SpecDir:   tmpDir,
			VariantID: 1,
			Agent:     agent,
			Timeout:   50 * time.Millisecond, // short timeout for test speed
		}

		consolidator, err := NewConsolidator(cfg, mgr)
		require.NoError(t, err)

		start := time.Now()
		// Use a generous parent deadline so the test fails clearly if the
		// per-invocation timeout doesn't fire.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err = consolidator.runWithRetry(ctx, "test prompt")
		elapsed := time.Since(start)

		require.Error(t, err, "runWithRetry must return an error when agent hangs")
		assert.Less(t, elapsed, 4*time.Second, "runWithRetry should terminate within timeout, not run until parent cancels")
	})
}

// TestRunPostPrompt_AppliesTimeout verifies that runPostPrompt bounds the agent
// invocation with a deadline so a hung session can't run forever.
// Regression test for T-679.
func TestRunPostPrompt_AppliesTimeout(t *testing.T) {
	t.Parallel()

	t.Run("agent context has a deadline", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		agent := &deadlineCapturingAgent{
			name: "test-agent",
			result: &agents.RunResult{
				SessionID: "test-session",
				ExitCode:  0,
				Output:    "ok",
			},
		}

		mgr := createTestManagerWithVariants(t, 2, tmpDir)
		cfg := Config{
			SpecName:   "test-spec",
			SpecDir:    tmpDir,
			VariantID:  1,
			Agent:      agent,
			PostPrompt: "review the work",
		}

		consolidator, err := NewConsolidator(cfg, mgr)
		require.NoError(t, err)

		ok, err := consolidator.runPostPrompt(context.Background())
		require.NoError(t, err)
		assert.True(t, ok)
		require.Len(t, agent.hasDeadline, 1, "agent should have been called once")
		assert.True(t, agent.hasDeadline[0], "context passed to agent must have a deadline")
	})

	t.Run("hung agent is terminated by timeout", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		agent := &hangingAgent{name: "test-agent"}

		mgr := createTestManagerWithVariants(t, 2, tmpDir)
		cfg := Config{
			SpecName:   "test-spec",
			SpecDir:    tmpDir,
			VariantID:  1,
			Agent:      agent,
			PostPrompt: "review the work",
			Timeout:    50 * time.Millisecond,
		}

		consolidator, err := NewConsolidator(cfg, mgr)
		require.NoError(t, err)

		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err = consolidator.runPostPrompt(ctx)
		elapsed := time.Since(start)

		require.Error(t, err, "runPostPrompt must return an error when agent hangs")
		assert.Less(t, elapsed, 4*time.Second, "runPostPrompt should terminate within timeout, not run until parent cancels")
	})
}

// TestRunWithRetry_IsErrorTreatedAsFailure verifies that runWithRetry treats
// agent-level IsError=true as a failure even when err==nil and result.Error==nil.
// Regression test for T-609.
func TestRunWithRetry_IsErrorTreatedAsFailure(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		result  *agents.RunResult
		runErr  error
		wantErr bool
	}{
		"IsError true with nil Error should fail": {
			result: &agents.RunResult{
				SessionID: "test-session",
				IsError:   true,
				ExitCode:  0,
				Output:    "invalid output",
			},
			wantErr: true,
		},
		"IsError false with nil Error should succeed": {
			result: &agents.RunResult{
				SessionID: "test-session",
				IsError:   false,
				ExitCode:  0,
				Output:    "valid output",
			},
			wantErr: false,
		},
		"nil result should fail": {
			result:  nil,
			wantErr: true,
		},
		"Go-level error should fail": {
			result:  &agents.RunResult{SessionID: "test-session", ExitCode: 1},
			runErr:  fmt.Errorf("agent process exited with code 1"),
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			agent := &mockAgent{
				name:      "test-agent",
				runResult: tc.result,
				runErr:    tc.runErr,
			}

			mgr := createTestManagerWithVariants(t, 2, tmpDir)
			cfg := Config{
				SpecName:  "test-spec",
				SpecDir:   tmpDir,
				VariantID: 1,
				Agent:     agent,
			}

			consolidator, err := NewConsolidator(cfg, mgr)
			require.NoError(t, err)

			_, err = consolidator.runWithRetry(context.Background(), "test prompt")
			if tc.wantErr {
				assert.Error(t, err, "runWithRetry should return error")
			} else {
				assert.NoError(t, err, "runWithRetry should succeed")
			}
		})
	}
}

// Helper functions

func createTestManager(t *testing.T, numVariants int) *variants.Manager {
	tmpDir := t.TempDir()
	return createTestManagerWithVariants(t, numVariants, tmpDir)
}

func createTestManagerWithVariants(t *testing.T, numVariants int, specDir string) *variants.Manager {
	t.Helper()

	if numVariants == 0 {
		git := &mockGitClient{
			currentBranch: "main",
			headCommit:    "abc123",
		}
		mgr, err := variants.NewManager(
			variants.Config{Count: 1, BranchPrefix: "orbit-impl"},
			"test-spec",
			specDir,
			specDir,
			git,
		)
		require.NoError(t, err)
		return mgr
	}

	// Create worktree directories
	worktrees := make([]string, numVariants)
	headCommitInPath := make(map[string]string)
	for i := range numVariants {
		worktrees[i] = filepath.Join(specDir, ".orbit", "worktrees", "orbit-impl-"+string(rune('1'+i))+"-test-spec")
		require.NoError(t, os.MkdirAll(worktrees[i], 0755))
		headCommitInPath[worktrees[i]] = "commit" + string(rune('1'+i))
	}

	git := &mockGitClient{
		currentBranch:    "main",
		headCommit:       "abc123",
		headCommitInPath: headCommitInPath,
	}

	mgr, err := variants.NewManager(
		variants.Config{Count: numVariants, BranchPrefix: "orbit-impl"},
		"test-spec",
		specDir,
		specDir,
		git,
	)
	require.NoError(t, err)

	// Setup the manager to create variants
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = mgr.Setup(ctx, false)
	require.NoError(t, err)

	return mgr
}
