package variants

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/arjenschwarz/orbit/internal/cost"
	"github.com/google/uuid"
)

// Manager handles variant lifecycle including worktree creation, status tracking, and cleanup.
type Manager struct {
	config       Config
	specName     string
	specDir      string // Path to specs/{spec-name}
	repoRoot     string // Git repository root
	metadata     *VariantsMetadata
	metadataPath string // specs/{spec}/.orbit/variants.json
	worktreeDir  string // specs/{spec}/.orbit/worktrees/
	mu           sync.Mutex
	git          GitClient
}

// NewManager creates a variant manager for a spec.
func NewManager(cfg Config, specName, specDir, repoRoot string, git GitClient) (*Manager, error) {
	if specName == "" {
		return nil, errors.New("spec name is required")
	}
	if specDir == "" {
		return nil, errors.New("spec directory is required")
	}
	if repoRoot == "" {
		return nil, errors.New("repository root is required")
	}
	if git == nil {
		return nil, errors.New("git client is required")
	}
	if cfg.Count < 1 {
		return nil, errors.New("variant count must be at least 1")
	}

	orbitDir := filepath.Join(specDir, ".orbit")
	return &Manager{
		config:       cfg,
		specName:     specName,
		specDir:      specDir,
		repoRoot:     repoRoot,
		metadataPath: filepath.Join(orbitDir, "variants.json"),
		worktreeDir:  filepath.Join(orbitDir, "worktrees"),
		git:          git,
	}, nil
}

// Load reads existing variants.json if present.
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No existing metadata, which is fine
		}
		return fmt.Errorf("read variants.json: %w", err)
	}

	var metadata VariantsMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return fmt.Errorf("unmarshal variants.json: %w", err)
	}

	m.metadata = &metadata
	return nil
}

// HasExistingRun returns true if there's existing variant metadata from a previous run.
func (m *Manager) HasExistingRun() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.metadata != nil
}

// Save persists the current metadata to variants.json using atomic write.
func (m *Manager) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.saveLocked()
}

// saveLocked performs the save without acquiring the mutex (caller must hold it).
func (m *Manager) saveLocked() error {
	if m.metadata == nil {
		return errors.New("no metadata to save")
	}

	data, err := json.MarshalIndent(m.metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(m.metadataPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// Write to temp file first for atomic operation
	tmpPath := m.metadataPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, m.metadataPath); err != nil {
		_ = os.Remove(tmpPath) // Clean up on failure
		return fmt.Errorf("rename to final: %w", err)
	}

	return nil
}

// ensureGitignore creates or updates .orbit/.gitignore to ignore worktrees.
func (m *Manager) ensureGitignore() error {
	gitignorePath := filepath.Join(m.specDir, ".orbit", ".gitignore")

	// Check if file exists and already contains the entry
	content, err := os.ReadFile(gitignorePath)
	if err == nil {
		if strings.Contains(string(content), "worktrees/") {
			return nil // Already configured
		}
		// Append to existing file
		f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("open .gitignore for append: %w", err)
		}
		_, writeErr := f.WriteString("\n# Variant worktrees (managed by orbit)\nworktrees/\n")
		closeErr := f.Close()
		if writeErr != nil {
			return fmt.Errorf("append to .gitignore: %w", writeErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close .gitignore: %w", closeErr)
		}
		return nil
	}

	if !os.IsNotExist(err) {
		return fmt.Errorf("read .gitignore: %w", err)
	}

	// Create directory if needed
	dir := filepath.Dir(gitignorePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create .orbit directory: %w", err)
	}

	// Create new .gitignore
	content = []byte("# Orbit variant data (auto-generated)\nworktrees/\n")
	if err := os.WriteFile(gitignorePath, content, 0644); err != nil {
		return fmt.Errorf("create .gitignore: %w", err)
	}

	return nil
}

// Setup creates worktrees and branches for all variants.
// If continueExisting is true and metadata exists, reuses existing worktrees.
// If continueExisting is false and metadata exists, cleans up unfinished variants only,
// preserving completed ones.
func (m *Manager) Setup(ctx context.Context, continueExisting bool) error {
	// Check for existing metadata first - if continuing, skip all checks
	if m.metadata != nil && continueExisting {
		return nil
	}

	// Check for uncommitted changes (only for new runs)
	hasChanges, err := m.git.HasUncommittedChanges()
	if err != nil {
		return fmt.Errorf("check uncommitted changes: %w", err)
	}
	if hasChanges {
		return errors.New("working directory has uncommitted changes; commit or stash before running variants")
	}

	// Get current state
	currentBranch, err := m.git.GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("get current branch: %w", err)
	}

	headCommit, err := m.git.GetHeadCommit()
	if err != nil {
		return fmt.Errorf("get head commit: %w", err)
	}

	// Track which variant IDs already have completed variants
	completedIDs := make(map[int]bool)

	// Clean up only unfinished variants, preserving completed ones
	if m.metadata != nil {
		// Before mutating state, check if any completed variants would be preserved
		// with an incompatible base commit. New branches are created at HEAD, so if
		// HEAD != BaseCommit, different variants would have different bases, producing
		// incorrect diffs and comparisons (T-191). We check before CleanupUnfinished
		// to avoid removing worktrees/branches when the run will be rejected.
		if m.hasCompletedVariants() && m.metadata.BaseCommit != headCommit {
			return fmt.Errorf(
				"cannot start new run: HEAD (%s) differs from base commit (%s) of preserved completed variants; "+
					"checkout the original base commit or use 'orbit cleanup' to discard completed variants first",
				headCommit, m.metadata.BaseCommit,
			)
		}

		preserved, err := m.CleanupUnfinished(ctx)
		if err != nil {
			return fmt.Errorf("cleanup unfinished variants: %w", err)
		}
		for _, id := range preserved {
			completedIDs[id] = true
		}
	}

	// Ensure .gitignore is in place before creating worktrees
	if err := m.ensureGitignore(); err != nil {
		return fmt.Errorf("ensure gitignore: %w", err)
	}

	// Create worktrees directory
	if err := os.MkdirAll(m.worktreeDir, 0755); err != nil {
		return fmt.Errorf("create worktrees directory: %w", err)
	}

	// If we have preserved completed variants, reuse the existing metadata
	// Otherwise, create fresh metadata
	if m.metadata == nil {
		m.metadata = &VariantsMetadata{
			RunID:          uuid.New().String(),
			BaseCommit:     headCommit,
			OriginalBranch: currentBranch,
			StartedAt:      time.Now().UTC(),
			Variants:       make([]*Variant, 0, m.config.Count),
		}
	}

	// Create branches and worktrees for each variant that doesn't already exist
	sanitizedSpec := sanitizeSpecName(m.specName)
	var createdWorktrees []string

	for i := 1; i <= m.config.Count; i++ {
		// Skip variants that are already completed
		if completedIDs[i] {
			continue
		}

		branchName := fmt.Sprintf("%s-%d/%s", m.config.BranchPrefix, i, m.specName)
		worktreePath := filepath.Join(m.worktreeDir, fmt.Sprintf("%s-%d-%s", m.config.BranchPrefix, i, sanitizedSpec))

		// Create branch at the captured head commit so all variants share the
		// same base, even if HEAD advances between iterations.
		if err := m.git.CreateBranch(branchName, headCommit); err != nil {
			// Cleanup already created worktrees on failure
			m.cleanupCreated(ctx, createdWorktrees)
			return fmt.Errorf("create branch for variant %d: %w", i, err)
		}

		// Create worktree
		if err := m.git.CreateWorktree(ctx, worktreePath, branchName); err != nil {
			// Cleanup already created worktrees and this branch
			_ = m.git.DeleteBranch(branchName)
			m.cleanupCreated(ctx, createdWorktrees)
			return fmt.Errorf("create worktree for variant %d: %w", i, err)
		}

		createdWorktrees = append(createdWorktrees, worktreePath)

		// Get guidance for this variant if available
		var guidance string
		if i-1 < len(m.config.Guidance) {
			guidance = m.config.Guidance[i-1]
		}

		m.metadata.Variants = append(m.metadata.Variants, &Variant{
			ID:           i,
			Branch:       branchName,
			WorktreePath: worktreePath,
			Status:       StatusPending,
			Guidance:     guidance,
		})
	}

	// Save initial metadata
	m.mu.Lock()
	err = m.saveLocked()
	m.mu.Unlock()

	if err != nil {
		m.cleanupCreated(ctx, createdWorktrees)
		return fmt.Errorf("save metadata: %w", err)
	}

	return nil
}

// cleanupCreated removes worktrees that were created during a failed setup.
func (m *Manager) cleanupCreated(ctx context.Context, paths []string) {
	for _, path := range paths {
		_ = m.git.RemoveWorktree(ctx, path)
	}
}

// UpdateStatus updates a variant's status and persists to disk.
func (m *Manager) UpdateStatus(id int, status VariantStatus, err error) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	v := m.getVariantLocked(id)
	if v == nil {
		return fmt.Errorf("variant %d not found", id)
	}

	v.Status = status
	if err != nil {
		v.Error = err.Error()
	} else {
		v.Error = ""
	}

	return m.saveLocked()
}

// UpdateMetrics updates a variant's metrics after completion.
func (m *Manager) UpdateMetrics(id int, costValue float64, costUnit string, costTotals cost.Totals, duration time.Duration, turns int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	v := m.getVariantLocked(id)
	if v == nil {
		return fmt.Errorf("variant %d not found", id)
	}

	v.Cost = costValue
	v.CostUnit = costUnit
	v.CostTotals = costTotals
	v.Duration = duration
	v.NumTurns = turns

	return m.saveLocked()
}

// UpdateAgentInfo updates a variant's agent alias, resolved agent type, and model.
func (m *Manager) UpdateAgentInfo(id int, agentAlias, agentType, model string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	v := m.getVariantLocked(id)
	if v == nil {
		return fmt.Errorf("variant %d not found", id)
	}

	v.Agent = agentAlias
	v.AgentType = agentType
	v.Model = model

	return m.saveLocked()
}

// hasCompletedVariants returns true if any variant has StatusCompleted.
// Returns false when metadata is nil (no previous run).
// Does not acquire m.mu; safe to call during single-threaded Setup.
func (m *Manager) hasCompletedVariants() bool {
	if m.metadata == nil {
		return false
	}
	for _, v := range m.metadata.Variants {
		if v.Status == StatusCompleted {
			return true
		}
	}
	return false
}

// GetVariant returns a variant by ID.
func (m *Manager) GetVariant(id int) *Variant {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getVariantLocked(id)
}

// getVariantLocked finds a variant by ID (caller must hold mutex).
func (m *Manager) getVariantLocked(id int) *Variant {
	if m.metadata == nil {
		return nil
	}
	for _, v := range m.metadata.Variants {
		if v.ID == id {
			return v
		}
	}
	return nil
}

// GetVariantsSnapshot returns a deep copy of the variants slice for safe iteration.
// Callers receive independent copies and can safely read fields without locks.
func (m *Manager) GetVariantsSnapshot() []*Variant {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.metadata == nil {
		return nil
	}

	// Deep copy each variant to ensure complete thread safety
	snapshot := make([]*Variant, len(m.metadata.Variants))
	for i, v := range m.metadata.Variants {
		variantCopy := *v // Copy the struct
		snapshot[i] = &variantCopy
	}
	return snapshot
}

// CountByStatus returns the count of variants with the given status.
func (m *Manager) CountByStatus(status VariantStatus) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.metadata == nil {
		return 0
	}

	count := 0
	for _, v := range m.metadata.Variants {
		if v.Status == status {
			count++
		}
	}
	return count
}

// Cleanup removes all worktrees and branches.
// If keepID > 0, preserves that variant.
func (m *Manager) Cleanup(ctx context.Context, keepID int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.metadata == nil {
		return nil // Nothing to clean up
	}

	var errs []error
	var keptVariant *Variant

	for _, v := range m.metadata.Variants {
		if v.ID == keepID {
			keptVariant = v
			continue
		}

		// Remove worktree first
		if v.WorktreePath != "" {
			if err := m.git.RemoveWorktree(ctx, v.WorktreePath); err != nil {
				errs = append(errs, fmt.Errorf("remove worktree for variant %d: %w", v.ID, err))
			}
		}

		// Then delete branch
		if v.Branch != "" {
			if err := m.git.DeleteBranch(v.Branch); err != nil {
				errs = append(errs, fmt.Errorf("delete branch for variant %d: %w", v.ID, err))
			}
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	if keepID > 0 && keptVariant != nil {
		// Update metadata to only contain the kept variant
		m.metadata.Variants = []*Variant{keptVariant}
		return m.saveLocked()
	}

	// Remove variants.json
	if err := os.Remove(m.metadataPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove variants.json: %w", err)
	}

	// Try to remove the worktrees directory if empty
	_ = os.Remove(m.worktreeDir)

	m.metadata = nil
	return nil
}

// CleanupUnfinished removes worktrees and branches for non-completed variants only.
// Returns the list of completed variant IDs that were preserved.
func (m *Manager) CleanupUnfinished(ctx context.Context) ([]int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.metadata == nil {
		return nil, nil // Nothing to clean up
	}

	var errs []error
	var completedVariants []*Variant
	var completedIDs []int

	for _, v := range m.metadata.Variants {
		if v.Status == StatusCompleted {
			completedVariants = append(completedVariants, v)
			completedIDs = append(completedIDs, v.ID)
			continue
		}

		// Remove worktree first
		if v.WorktreePath != "" {
			if err := m.git.RemoveWorktree(ctx, v.WorktreePath); err != nil {
				errs = append(errs, fmt.Errorf("remove worktree for variant %d: %w", v.ID, err))
			}
		}

		// Then delete branch
		if v.Branch != "" {
			if err := m.git.DeleteBranch(v.Branch); err != nil {
				errs = append(errs, fmt.Errorf("delete branch for variant %d: %w", v.ID, err))
			}
		}
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	if len(completedVariants) > 0 {
		// Update metadata to only contain completed variants
		m.metadata.Variants = completedVariants
		if err := m.saveLocked(); err != nil {
			return nil, fmt.Errorf("save metadata: %w", err)
		}
		return completedIDs, nil
	}

	// No completed variants - remove everything
	if err := os.Remove(m.metadataPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove variants.json: %w", err)
	}

	_ = os.Remove(m.worktreeDir)
	m.metadata = nil
	return nil, nil
}

// Finalize rebases the chosen variant onto the original branch.
func (m *Manager) Finalize(ctx context.Context, variantID int) error {
	m.mu.Lock()
	metadata := m.metadata
	m.mu.Unlock()

	if metadata == nil {
		return errors.New("no variant metadata found")
	}

	// Find the variant
	var variant *Variant
	for _, v := range metadata.Variants {
		if v.ID == variantID {
			variant = v
			break
		}
	}
	if variant == nil {
		return fmt.Errorf("variant %d not found", variantID)
	}

	// Verify original branch has not diverged
	diverged, err := m.git.BranchHasDiverged(metadata.OriginalBranch, metadata.BaseCommit)
	if err != nil {
		return fmt.Errorf("check branch divergence: %w", err)
	}
	if diverged {
		return fmt.Errorf("original branch '%s' has diverged since variants were created; manual merge required",
			metadata.OriginalBranch)
	}

	// Rebase variant onto original branch
	if err := m.git.Rebase(ctx, variant.Branch, metadata.OriginalBranch); err != nil {
		return fmt.Errorf("rebase variant onto original branch: %w", err)
	}

	// Cleanup other variants
	return m.Cleanup(ctx, 0) // Remove all including the finalized one
}

// GetMetadata returns a copy of the current metadata (for status display).
// Returns nil if no metadata is loaded.
func (m *Manager) GetMetadata() *VariantsMetadata {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.metadata == nil {
		return nil
	}

	// Return a defensive copy to prevent callers from modifying internal state
	metaCopy := *m.metadata
	metaCopy.Variants = make([]*Variant, len(m.metadata.Variants))
	for i, v := range m.metadata.Variants {
		variantCopy := *v
		metaCopy.Variants[i] = &variantCopy
	}
	return &metaCopy
}

// GetVariantCommits returns a map of variant ID to HEAD commit SHA.
// Used for staleness detection in reports [Req 1.7].
// Errors are logged but not fatal - returns partial results on failure.
func (m *Manager) GetVariantCommits() map[int]string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.metadata == nil {
		return nil
	}

	commits := make(map[int]string)
	for _, v := range m.metadata.Variants {
		if v.WorktreePath == "" {
			continue
		}
		commit, err := m.git.GetHeadCommitInPath(v.WorktreePath)
		if err != nil {
			// Log but continue - staleness detection is best-effort
			continue
		}
		commits[v.ID] = commit
	}
	return commits
}

// sanitizeSpecName makes a spec name safe for filesystem and git branch names.
func sanitizeSpecName(name string) string {
	// Build result, replacing unsafe characters with dashes
	var result strings.Builder
	result.Grow(len(name))

	for _, r := range name {
		// Replace control characters, whitespace, and filesystem-unsafe characters
		if isUnsafeRune(r) {
			result.WriteRune('-')
		} else {
			result.WriteRune(r)
		}
	}

	sanitized := result.String()

	// Collapse multiple dashes
	for strings.Contains(sanitized, "--") {
		sanitized = strings.ReplaceAll(sanitized, "--", "-")
	}

	// Trim leading/trailing dashes
	sanitized = strings.Trim(sanitized, "-")

	return sanitized
}

// isUnsafeRune returns true if the rune is unsafe for filesystem/git branch names.
func isUnsafeRune(r rune) bool {
	// Control characters (including tabs, newlines, Unicode control chars)
	if unicode.IsControl(r) {
		return true
	}
	// Filesystem-unsafe characters
	switch r {
	case '/', '\\', ' ', ':', '*', '?', '"', '<', '>', '|':
		return true
	}
	return false
}
