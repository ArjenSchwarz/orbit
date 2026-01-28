package status

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/arjenschwarz/orbit/internal/agents/claudecode"
	"github.com/arjenschwarz/orbit/internal/logs"
	"github.com/arjenschwarz/orbit/internal/rune"
	"github.com/arjenschwarz/orbit/internal/transcript"
	"github.com/arjenschwarz/orbit/internal/variants"
)

// Gatherer collects status information for variants.
type Gatherer struct {
	git        variants.GitClient
	specName   string
	specDir    string // Path to spec directory in main repo (e.g., "specs/feature")
	baseCommit string
	repoRoot   string
}

// NewGatherer creates a Gatherer for the given spec.
func NewGatherer(git variants.GitClient, specName, specDir, baseCommit, repoRoot string) *Gatherer {
	return &Gatherer{
		git:        git,
		specName:   specName,
		specDir:    specDir,
		baseCommit: baseCommit,
		repoRoot:   repoRoot,
	}
}

// GatherAllVariants collects status information for all variants concurrently.
// This improves performance by parallelizing git, transcript, and rune operations.
func (g *Gatherer) GatherAllVariants(ctx context.Context, variantList []*variants.Variant) []*VariantInfo {
	results := make([]*VariantInfo, len(variantList))
	var wg sync.WaitGroup

	for i, v := range variantList {
		wg.Add(1)
		go func(idx int, variant *variants.Variant) {
			defer wg.Done()
			results[idx] = g.GatherVariantInfo(ctx, variant)
		}(i, v)
	}

	wg.Wait()
	return results
}

// GatherVariantInfo collects all available information for a single variant.
// Errors in individual data sources are captured, not propagated.
func (g *Gatherer) GatherVariantInfo(ctx context.Context, v *variants.Variant) *VariantInfo {
	info := &VariantInfo{
		ID:           v.ID,
		Branch:       v.Branch,
		WorktreePath: v.WorktreePath,
		Status:       v.Status,
		AgentType:    v.AgentType,
		Error:        v.Error,
	}

	// Only gather details for active variants (running or failed)
	if v.Status != variants.StatusRunning && v.Status != variants.StatusFailed {
		return info
	}

	// Git info (commits + dirty state)
	info.GitInfo = g.gatherGitInfo(ctx, v.WorktreePath)

	// Last action from transcript
	info.LastAction = g.gatherLastAction(v)

	// Task progress via rune
	info.TaskProgress = g.gatherTaskProgress(v.WorktreePath)

	return info
}

// gatherGitInfo collects git commits and dirty state for a worktree.
func (g *Gatherer) gatherGitInfo(ctx context.Context, worktreePath string) *GitInfo {
	gitInfo := &GitInfo{}

	// Get commits (up to 3 most recent since base commit)
	commits, err := g.git.GetRecentCommits(ctx, worktreePath, g.baseCommit, 3)
	if err != nil {
		return nil // Git info unavailable
	}
	gitInfo.Commits = commits

	// Get dirty state
	isDirty, err := g.git.HasUncommittedChangesInPath(worktreePath)
	if err != nil {
		return nil // Git info unavailable
	}
	gitInfo.IsDirty = isDirty
	if isDirty {
		gitInfo.DirtyState = "dirty"
	} else {
		gitInfo.DirtyState = "clean"
	}

	return gitInfo
}

// gatherLastAction retrieves the last action from the transcript.
func (g *Gatherer) gatherLastAction(v *variants.Variant) *LastActionResult {
	// Check agent type first - only Claude Code supports live transcript access
	if v.AgentType != "claude-code" {
		return &LastActionResult{State: LastActionNotSupported}
	}

	// Build path to variant's log directory in main repo
	// Logs are stored at: specs/<spec>/.orbit/logs/variant-<id>/summary.json
	variantLogDir := filepath.Join(g.repoRoot, g.specDir, ".orbit", "logs", fmt.Sprintf("variant-%d", v.ID))

	// Get transcript path
	transcriptPath, err := GetLiveTranscriptPath(v.WorktreePath, variantLogDir)
	if err != nil || transcriptPath == "" {
		return &LastActionResult{State: LastActionWaiting}
	}

	// Check if file exists
	if _, err := os.Stat(transcriptPath); os.IsNotExist(err) {
		return &LastActionResult{State: LastActionWaiting}
	}

	// Read last displayable entry
	entry, err := transcript.GetLastDisplayableEntry(transcriptPath)
	if err != nil {
		return &LastActionResult{State: LastActionUnavailable}
	}
	if entry == nil {
		return &LastActionResult{State: LastActionWaiting}
	}

	return &LastActionResult{
		State:   LastActionFound,
		Summary: transcript.FormatLastAction(entry),
	}
}

// gatherTaskProgress retrieves task progress via rune CLI.
func (g *Gatherer) gatherTaskProgress(worktreePath string) *TaskProgress {
	// Construct path to tasks.md within the variant's worktree
	tasksFile := filepath.Join(worktreePath, "specs", g.specName, "tasks.md")

	// Create a rune client for this specific tasks file
	runeClient := rune.NewClient(tasksFile)

	// Get phase summaries
	summaries, err := runeClient.GetPhaseSummaries()
	if err != nil {
		return nil // Task progress unavailable
	}

	return &TaskProgress{
		Phases: FromRunePhaseSummary(summaries),
	}
}

// GetLiveTranscriptPath returns the path to the live Claude transcript.
//
// Parameters:
// - worktreePath: The variant's worktree path (e.g., "/repo/specs/feature/.orbit/worktrees/impl-1")
//   This is used as the working directory for BuildProjectPath since Claude is invoked from here.
// - variantLogDir: Path to the variant's log directory in the main repo
//   (e.g., "/repo/specs/feature/.orbit/logs/variant-1")
//
// Returns empty string if session ID not available or agent is not Claude.
func GetLiveTranscriptPath(worktreePath, variantLogDir string) (string, error) {
	// Read summary.json from variant's log directory in main repo
	summaryPath := filepath.Join(variantLogDir, "summary.json")
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		return "", err
	}

	var summary logs.Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		return "", err
	}

	// Get current session ID from in-progress phase
	var sessionID string
	if summary.CurrentPhase != nil {
		sessionID = summary.CurrentPhase.SessionID
	}
	if sessionID == "" {
		return "", nil
	}

	// Build Claude project path
	// Claude is invoked from the worktree root, so that's the project path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	// Ensure worktreePath is absolute for correct Claude project path resolution
	// (worktreePath from variants.json may be relative)
	absWorktreePath := worktreePath
	if !filepath.IsAbs(worktreePath) {
		if abs, err := filepath.Abs(worktreePath); err == nil {
			absWorktreePath = abs
		}
	}

	// Example: worktreePath = "/Users/foo/repo/specs/feature/.orbit/worktrees/impl-1"
	// BuildProjectPath converts to: "-Users-foo-repo-specs-feature--orbit-worktrees-impl-1"
	// Claude stores at: ~/.claude/projects/{project-hash}/{session-id}.jsonl
	projectHash := claudecode.BuildProjectPath(absWorktreePath)
	return filepath.Join(homeDir, ".claude", "projects", projectHash, sessionID+".jsonl"), nil
}
