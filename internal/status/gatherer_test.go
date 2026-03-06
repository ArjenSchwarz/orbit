package status

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arjenschwarz/orbit/internal/logs"
	"github.com/arjenschwarz/orbit/internal/variants"
)

// mockGitClient implements variants.GitClient for testing.
type mockGitClient struct {
	commits    []variants.Commit
	commitsErr error
	isDirty    bool
	dirtyErr   error
}

func (m *mockGitClient) GetCurrentBranch() (string, error)               { return "", nil }
func (m *mockGitClient) GetHeadCommit() (string, error)                  { return "", nil }
func (m *mockGitClient) GetHeadCommitInPath(path string) (string, error) { return "", nil }
func (m *mockGitClient) CreateBranch(name, commit string) error          { return nil }
func (m *mockGitClient) CreateWorktree(ctx context.Context, path, branch string) error {
	return nil
}
func (m *mockGitClient) RemoveWorktree(ctx context.Context, path string) error { return nil }
func (m *mockGitClient) DeleteBranch(name string) error                        { return nil }
func (m *mockGitClient) GetDiff(ctx context.Context, worktreePath, baseCommit string) (string, error) {
	return "", nil
}
func (m *mockGitClient) Rebase(ctx context.Context, sourceBranch, targetBranch string) error {
	return nil
}
func (m *mockGitClient) BranchHasDiverged(branch, baseCommit string) (bool, error) { return false, nil }
func (m *mockGitClient) HasUncommittedChanges() (bool, error)                      { return false, nil }
func (m *mockGitClient) GetCommitLog(ctx context.Context, worktreePath, baseCommit string) ([]string, error) {
	return nil, nil
}
func (m *mockGitClient) GetDiffStat(ctx context.Context, worktreePath, baseCommit string) (string, error) {
	return "", nil
}

func (m *mockGitClient) HasUncommittedChangesInPath(path string) (bool, error) {
	return m.isDirty, m.dirtyErr
}

func (m *mockGitClient) GetRecentCommits(ctx context.Context, worktreePath, baseCommit string, limit int) ([]variants.Commit, error) {
	if m.commitsErr != nil {
		return nil, m.commitsErr
	}
	// Respect the limit
	if len(m.commits) <= limit {
		return m.commits, nil
	}
	return m.commits[:limit], nil
}

func TestNewGatherer(t *testing.T) {
	git := &mockGitClient{}
	g := NewGatherer(git, "test-spec", "specs/test-spec", "abc1234", "/repo")

	if g.git != git {
		t.Error("git client not set correctly")
	}
	if g.specName != "test-spec" {
		t.Errorf("specName = %q, want 'test-spec'", g.specName)
	}
	if g.specDir != "specs/test-spec" {
		t.Errorf("specDir = %q, want 'specs/test-spec'", g.specDir)
	}
	if g.baseCommit != "abc1234" {
		t.Errorf("baseCommit = %q, want 'abc1234'", g.baseCommit)
	}
	if g.repoRoot != "/repo" {
		t.Errorf("repoRoot = %q, want '/repo'", g.repoRoot)
	}
}

func TestGatherVariantInfo_NonActiveVariant(t *testing.T) {
	statuses := []variants.VariantStatus{
		variants.StatusPending,
		variants.StatusCompleted,
		variants.StatusCanceled,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			git := &mockGitClient{
				commits: []variants.Commit{{Hash: "abc1234", Subject: "Test"}},
				isDirty: true,
			}
			g := NewGatherer(git, "test-spec", "specs/test-spec", "base123", "/repo")

			v := &variants.Variant{
				ID:           1,
				Branch:       "test-branch",
				WorktreePath: "/path/to/worktree",
				Status:       status,
				AgentType:    "claude-code",
			}

			info := g.GatherVariantInfo(context.Background(), v)

			// Should have basic info
			if info.ID != 1 {
				t.Errorf("ID = %d, want 1", info.ID)
			}
			if info.Status != status {
				t.Errorf("Status = %v, want %v", info.Status, status)
			}

			// Should NOT have detailed info for non-active variants
			if info.GitInfo != nil {
				t.Error("GitInfo should be nil for non-active variant")
			}
			if info.LastAction != nil {
				t.Error("LastAction should be nil for non-active variant")
			}
			if info.TaskProgress != nil {
				t.Error("TaskProgress should be nil for non-active variant")
			}
		})
	}
}

func TestGatherVariantInfo_GitInfoSuccess(t *testing.T) {
	commits := []variants.Commit{
		{Hash: "abc1234", Subject: "First commit"},
		{Hash: "def5678", Subject: "Second commit"},
	}

	tests := []struct {
		name      string
		isDirty   bool
		wantState string
	}{
		{"clean worktree", false, "clean"},
		{"dirty worktree", true, "dirty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			git := &mockGitClient{
				commits: commits,
				isDirty: tt.isDirty,
			}
			g := NewGatherer(git, "test-spec", "specs/test-spec", "base123", "/repo")

			v := &variants.Variant{
				ID:           1,
				Branch:       "test-branch",
				WorktreePath: "/path/to/worktree",
				Status:       variants.StatusRunning,
				AgentType:    "other-agent", // Non-Claude to simplify test
			}

			info := g.GatherVariantInfo(context.Background(), v)

			if info.GitInfo == nil {
				t.Fatal("GitInfo should not be nil")
			}
			if len(info.GitInfo.Commits) != 2 {
				t.Errorf("Commits count = %d, want 2", len(info.GitInfo.Commits))
			}
			if info.GitInfo.DirtyState != tt.wantState {
				t.Errorf("DirtyState = %q, want %q", info.GitInfo.DirtyState, tt.wantState)
			}
			if info.GitInfo.IsDirty != tt.isDirty {
				t.Errorf("IsDirty = %v, want %v", info.GitInfo.IsDirty, tt.isDirty)
			}
		})
	}
}

func TestGatherVariantInfo_GitInfoFailure(t *testing.T) {
	tests := []struct {
		name       string
		commitsErr error
		dirtyErr   error
	}{
		{"commits error", errors.New("git error"), nil},
		{"dirty check error", nil, errors.New("git error")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			git := &mockGitClient{
				commits:    []variants.Commit{{Hash: "abc", Subject: "test"}},
				commitsErr: tt.commitsErr,
				dirtyErr:   tt.dirtyErr,
			}
			g := NewGatherer(git, "test-spec", "specs/test-spec", "base123", "/repo")

			v := &variants.Variant{
				ID:           1,
				Branch:       "test-branch",
				WorktreePath: "/path/to/worktree",
				Status:       variants.StatusRunning,
				AgentType:    "other-agent",
			}

			info := g.GatherVariantInfo(context.Background(), v)

			// GitInfo should be nil on error (graceful degradation)
			if info.GitInfo != nil {
				t.Error("GitInfo should be nil on error")
			}
		})
	}
}

func TestGatherVariantInfo_LastActionUnsupportedAgent(t *testing.T) {
	// Agents that don't support last action (not claude-code or kiro)
	git := &mockGitClient{
		commits: []variants.Commit{{Hash: "abc", Subject: "test"}},
	}
	g := NewGatherer(git, "test-spec", "specs/test-spec", "base123", "/repo")

	v := &variants.Variant{
		ID:           1,
		Branch:       "test-branch",
		WorktreePath: "/path/to/worktree",
		Status:       variants.StatusRunning,
		AgentType:    "codex", // Unsupported agent for last action
	}

	info := g.GatherVariantInfo(context.Background(), v)

	if info.LastAction == nil {
		t.Fatal("LastAction should not be nil")
	}
	if info.LastAction.State != LastActionNotSupported {
		t.Errorf("LastAction.State = %v, want LastActionNotSupported", info.LastAction.State)
	}
}

func TestGatherVariantInfo_LastActionKiro(t *testing.T) {
	// Kiro agent returns LastActionWaiting when summary.json doesn't exist
	git := &mockGitClient{
		commits: []variants.Commit{{Hash: "abc", Subject: "test"}},
	}
	g := NewGatherer(git, "test-spec", "specs/test-spec", "base123", "/nonexistent/repo")

	v := &variants.Variant{
		ID:           1,
		Branch:       "test-branch",
		WorktreePath: "/path/to/worktree",
		Status:       variants.StatusRunning,
		AgentType:    "kiro",
	}

	info := g.GatherVariantInfo(context.Background(), v)

	if info.LastAction == nil {
		t.Fatal("LastAction should not be nil")
	}
	// Without summary.json, Kiro returns LastActionWaiting
	if info.LastAction.State != LastActionWaiting {
		t.Errorf("LastAction.State = %v, want LastActionWaiting", info.LastAction.State)
	}
}

func TestGatherVariantInfo_FailedVariant(t *testing.T) {
	// Failed variants should also get detailed info
	git := &mockGitClient{
		commits: []variants.Commit{{Hash: "abc1234", Subject: "Last commit before failure"}},
		isDirty: true,
	}
	g := NewGatherer(git, "test-spec", "specs/test-spec", "base123", "/repo")

	v := &variants.Variant{
		ID:           1,
		Branch:       "test-branch",
		WorktreePath: "/path/to/worktree",
		Status:       variants.StatusFailed,
		Error:        "Some error occurred",
		AgentType:    "other-agent",
	}

	info := g.GatherVariantInfo(context.Background(), v)

	// Should have detailed info for failed variants
	if info.GitInfo == nil {
		t.Error("GitInfo should be populated for failed variant")
	}
	if info.Error != "Some error occurred" {
		t.Errorf("Error = %q, want 'Some error occurred'", info.Error)
	}
}

func TestGatherAllVariants_Concurrent(t *testing.T) {
	git := &mockGitClient{
		commits: []variants.Commit{{Hash: "abc", Subject: "test"}},
	}
	g := NewGatherer(git, "test-spec", "specs/test-spec", "base123", "/repo")

	variantList := []*variants.Variant{
		{ID: 1, Branch: "b1", WorktreePath: "/p1", Status: variants.StatusRunning, AgentType: "other"},
		{ID: 2, Branch: "b2", WorktreePath: "/p2", Status: variants.StatusPending, AgentType: "other"},
		{ID: 3, Branch: "b3", WorktreePath: "/p3", Status: variants.StatusFailed, AgentType: "other"},
	}

	results := g.GatherAllVariants(context.Background(), variantList)

	if len(results) != 3 {
		t.Fatalf("Expected 3 results, got %d", len(results))
	}

	// Verify order is preserved
	for i, r := range results {
		if r.ID != variantList[i].ID {
			t.Errorf("Result %d: ID = %d, want %d", i, r.ID, variantList[i].ID)
		}
	}

	// Variant 1 (running) and 3 (failed) should have GitInfo
	if results[0].GitInfo == nil {
		t.Error("Running variant should have GitInfo")
	}
	if results[2].GitInfo == nil {
		t.Error("Failed variant should have GitInfo")
	}

	// Variant 2 (pending) should NOT have GitInfo
	if results[1].GitInfo != nil {
		t.Error("Pending variant should not have GitInfo")
	}
}

func TestGetLiveTranscriptPath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create the variant log directory structure (simulates specs/<spec>/.orbit/logs/variant-1/)
	variantLogDir := filepath.Join(tmpDir, "specs", "test-spec", ".orbit", "logs", "variant-1")
	if err := os.MkdirAll(variantLogDir, 0755); err != nil {
		t.Fatalf("Failed to create variant log dir: %v", err)
	}

	t.Run("returns path when session ID exists", func(t *testing.T) {
		summary := logs.Summary{
			CurrentPhase: &logs.PhaseState{
				Phase:     1,
				SessionID: "test-session-123",
			},
		}
		data, _ := json.Marshal(summary)
		summaryPath := filepath.Join(variantLogDir, "summary.json")
		if err := os.WriteFile(summaryPath, data, 0644); err != nil {
			t.Fatalf("Failed to write summary: %v", err)
		}

		// worktreePath is the variant's worktree (Claude's working directory)
		worktreePath := filepath.Join(tmpDir, "worktrees", "impl-1")
		path, err := GetLiveTranscriptPath(worktreePath, variantLogDir)

		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if path == "" {
			t.Error("Expected non-empty path")
		}
		if !filepath.IsAbs(path) {
			t.Error("Expected absolute path")
		}
		// Path should end with session ID + .jsonl
		if filepath.Base(path) != "test-session-123.jsonl" {
			t.Errorf("Path should end with session ID, got %q", filepath.Base(path))
		}
	})

	t.Run("returns empty string when no active session", func(t *testing.T) {
		summary := logs.Summary{
			CurrentPhase: nil,
		}
		data, _ := json.Marshal(summary)
		summaryPath := filepath.Join(variantLogDir, "summary.json")
		if err := os.WriteFile(summaryPath, data, 0644); err != nil {
			t.Fatalf("Failed to write summary: %v", err)
		}

		worktreePath := filepath.Join(tmpDir, "worktrees", "impl-1")
		path, err := GetLiveTranscriptPath(worktreePath, variantLogDir)

		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if path != "" {
			t.Errorf("Expected empty path when no active session, got %q", path)
		}
	})

	t.Run("returns path when pre-prompt is active", func(t *testing.T) {
		// Bug T-259: When pre-prompt is running (status=started), CurrentPhase is nil.
		// GetLiveTranscriptPath should fall back to pre-prompt session ID.
		summary := logs.Summary{
			CurrentPhase: nil,
			PrePrompt: &logs.PrePromptState{
				SessionID: "pre-prompt-session-456",
				Status:    logs.PrePromptStatusStarted,
			},
		}
		data, _ := json.Marshal(summary)
		summaryPath := filepath.Join(variantLogDir, "summary.json")
		if err := os.WriteFile(summaryPath, data, 0644); err != nil {
			t.Fatalf("Failed to write summary: %v", err)
		}

		worktreePath := filepath.Join(tmpDir, "worktrees", "impl-1")
		path, err := GetLiveTranscriptPath(worktreePath, variantLogDir)

		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if path == "" {
			t.Error("Expected non-empty path when pre-prompt is active")
		}
		if filepath.Base(path) != "pre-prompt-session-456.jsonl" {
			t.Errorf("Path should use pre-prompt session ID, got %q", filepath.Base(path))
		}
	})

	t.Run("returns path when post-prompt is active", func(t *testing.T) {
		// Bug T-259: When post-prompt is running, CurrentPhase is nil.
		// GetLiveTranscriptPath should fall back to post-completion session ID.
		summary := logs.Summary{
			CurrentPhase: nil,
			PostCompletion: &logs.PostCompletionState{
				SessionID: "post-prompt-session-789",
			},
		}
		data, _ := json.Marshal(summary)
		summaryPath := filepath.Join(variantLogDir, "summary.json")
		if err := os.WriteFile(summaryPath, data, 0644); err != nil {
			t.Fatalf("Failed to write summary: %v", err)
		}

		worktreePath := filepath.Join(tmpDir, "worktrees", "impl-1")
		path, err := GetLiveTranscriptPath(worktreePath, variantLogDir)

		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if path == "" {
			t.Error("Expected non-empty path when post-prompt is active")
		}
		if filepath.Base(path) != "post-prompt-session-789.jsonl" {
			t.Errorf("Path should use post-prompt session ID, got %q", filepath.Base(path))
		}
	})

	t.Run("prefers current phase over pre-prompt", func(t *testing.T) {
		// When both CurrentPhase and PrePrompt have session IDs,
		// CurrentPhase should take priority (it means a phase is actively running).
		summary := logs.Summary{
			CurrentPhase: &logs.PhaseState{
				Phase:     1,
				SessionID: "phase-session-111",
			},
			PrePrompt: &logs.PrePromptState{
				SessionID: "pre-prompt-session-222",
				Status:    logs.PrePromptStatusCompleted,
			},
		}
		data, _ := json.Marshal(summary)
		summaryPath := filepath.Join(variantLogDir, "summary.json")
		if err := os.WriteFile(summaryPath, data, 0644); err != nil {
			t.Fatalf("Failed to write summary: %v", err)
		}

		worktreePath := filepath.Join(tmpDir, "worktrees", "impl-1")
		path, err := GetLiveTranscriptPath(worktreePath, variantLogDir)

		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if filepath.Base(path) != "phase-session-111.jsonl" {
			t.Errorf("Should prefer current phase session, got %q", filepath.Base(path))
		}
	})

	t.Run("ignores completed pre-prompt when no current phase", func(t *testing.T) {
		// A completed pre-prompt is not a live session — it finished already.
		// Only "started" pre-prompts have a live session.
		summary := logs.Summary{
			CurrentPhase: nil,
			PrePrompt: &logs.PrePromptState{
				SessionID: "pre-prompt-done-333",
				Status:    logs.PrePromptStatusCompleted,
			},
		}
		data, _ := json.Marshal(summary)
		summaryPath := filepath.Join(variantLogDir, "summary.json")
		if err := os.WriteFile(summaryPath, data, 0644); err != nil {
			t.Fatalf("Failed to write summary: %v", err)
		}

		worktreePath := filepath.Join(tmpDir, "worktrees", "impl-1")
		path, err := GetLiveTranscriptPath(worktreePath, variantLogDir)

		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if path != "" {
			t.Errorf("Expected empty path when pre-prompt is completed and no current phase, got %q", path)
		}
	})

	t.Run("returns empty string when session ID is empty", func(t *testing.T) {
		summary := logs.Summary{
			CurrentPhase: &logs.PhaseState{
				Phase:     1,
				SessionID: "", // Empty session ID
			},
		}
		data, _ := json.Marshal(summary)
		summaryPath := filepath.Join(variantLogDir, "summary.json")
		if err := os.WriteFile(summaryPath, data, 0644); err != nil {
			t.Fatalf("Failed to write summary: %v", err)
		}

		worktreePath := filepath.Join(tmpDir, "worktrees", "impl-1")
		path, err := GetLiveTranscriptPath(worktreePath, variantLogDir)

		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if path != "" {
			t.Errorf("Expected empty path when session ID is empty, got %q", path)
		}
	})

	t.Run("returns error when summary.json missing", func(t *testing.T) {
		// Use a different log dir that doesn't have summary.json
		nonexistentDir := filepath.Join(tmpDir, "specs", "nonexistent", ".orbit", "logs", "variant-1")
		worktreePath := filepath.Join(tmpDir, "worktrees", "impl-1")
		path, err := GetLiveTranscriptPath(worktreePath, nonexistentDir)

		if err == nil {
			t.Error("Expected error when summary.json is missing")
		}
		if path != "" {
			t.Errorf("Expected empty path on error, got %q", path)
		}
	})

	t.Run("converts relative worktree path to absolute", func(t *testing.T) {
		summary := logs.Summary{
			CurrentPhase: &logs.PhaseState{
				Phase:     1,
				SessionID: "test-session-rel",
			},
		}
		data, _ := json.Marshal(summary)
		summaryPath := filepath.Join(variantLogDir, "summary.json")
		if err := os.WriteFile(summaryPath, data, 0644); err != nil {
			t.Fatalf("Failed to write summary: %v", err)
		}

		// Use a relative worktree path (simulates what variants.json might contain)
		relativeWorktreePath := "specs/test-spec/.orbit/worktrees/impl-1"
		path, err := GetLiveTranscriptPath(relativeWorktreePath, variantLogDir)

		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if path == "" {
			t.Error("Expected non-empty path")
		}
		// The returned path should still be absolute (because we're looking for Claude files)
		if !filepath.IsAbs(path) {
			t.Error("Expected absolute path")
		}
		// The Claude project path portion should contain the absolute path components
		// It should NOT start with "specs-" but should start with a dash followed by root path
		if filepath.Base(path) != "test-session-rel.jsonl" {
			t.Errorf("Path should end with session ID, got %q", filepath.Base(path))
		}
		// Path should contain an absolute-style Claude project path (starts with -)
		// e.g., ~/.claude/projects/-home-user-specs-test-spec-...
		homeDir, _ := os.UserHomeDir()
		if !strings.HasPrefix(path, homeDir) {
			t.Errorf("Path should be under home directory, got %q", path)
		}
	})
}

func TestGetActiveSessionID(t *testing.T) {
	tests := []struct {
		name    string
		summary logs.Summary
		want    string
	}{
		{
			name:    "no active session",
			summary: logs.Summary{},
			want:    "",
		},
		{
			name: "current phase active",
			summary: logs.Summary{
				CurrentPhase: &logs.PhaseState{
					Phase:     2,
					SessionID: "phase-sess",
				},
			},
			want: "phase-sess",
		},
		{
			name: "pre-prompt started",
			summary: logs.Summary{
				PrePrompt: &logs.PrePromptState{
					SessionID: "pre-sess",
					Status:    logs.PrePromptStatusStarted,
				},
			},
			want: "pre-sess",
		},
		{
			name: "pre-prompt completed — not active",
			summary: logs.Summary{
				PrePrompt: &logs.PrePromptState{
					SessionID: "pre-sess",
					Status:    logs.PrePromptStatusCompleted,
				},
			},
			want: "",
		},
		{
			name: "post-completion active",
			summary: logs.Summary{
				PostCompletion: &logs.PostCompletionState{
					SessionID: "post-sess",
				},
			},
			want: "post-sess",
		},
		{
			name: "current phase takes priority over pre-prompt",
			summary: logs.Summary{
				CurrentPhase: &logs.PhaseState{
					Phase:     1,
					SessionID: "phase-sess",
				},
				PrePrompt: &logs.PrePromptState{
					SessionID: "pre-sess",
					Status:    logs.PrePromptStatusStarted,
				},
			},
			want: "phase-sess",
		},
		{
			name: "current phase takes priority over post-completion",
			summary: logs.Summary{
				CurrentPhase: &logs.PhaseState{
					Phase:     3,
					SessionID: "phase-sess",
				},
				PostCompletion: &logs.PostCompletionState{
					SessionID: "post-sess",
				},
			},
			want: "phase-sess",
		},
		{
			name: "current phase with empty session ID falls through to pre-prompt",
			summary: logs.Summary{
				CurrentPhase: &logs.PhaseState{
					Phase:     1,
					SessionID: "",
				},
				PrePrompt: &logs.PrePromptState{
					SessionID: "pre-sess",
					Status:    logs.PrePromptStatusStarted,
				},
			},
			want: "pre-sess",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getActiveSessionID(&tt.summary)
			if got != tt.want {
				t.Errorf("getActiveSessionID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFromRunePhaseSummary(t *testing.T) {
	// This is tested indirectly through the Gatherer tests
	// A direct test would require importing rune which creates a circular import
	// The function is a simple data transformation tested by integration
}
