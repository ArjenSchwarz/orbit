# Multi-Spec Comparison Design

## Overview

This document describes the technical design for enabling Orbit to run multiple implementation variants of the same specification in parallel or sequentially, each in isolated git worktrees, and generate comparison reports.

The design extends Orbit's existing orchestration architecture with three new packages and five new CLI subcommands while maintaining full backwards compatibility with single-run mode.

### Key Design Principles

1. **Backwards Compatibility**: `orbit run` without `--variants` behaves exactly as today
2. **Isolation**: Each variant runs in its own git worktree with no shared state
3. **Resilience**: Partial failures don't block other variants or comparison
4. **Simplicity**: Leverage existing patterns (logs.Manager, registry, transcript)

### Design Constraints

1. **Single-Process Concurrency**: All variant goroutines run within a single `orbit` process. The mutex and atomic write patterns protect against concurrent goroutines, not multiple processes. Cross-process locking is not required because only one `orbit` process orchestrates a given spec at a time.

2. **Worktrees Inside Repository**: Git worktrees are created at `specs/{spec}/.orbit/worktrees/`. This works because:
   - Git worktrees can be created inside the repository directory structure
   - The system automatically creates/updates `.orbit/.gitignore` to ignore `worktrees/`
   - Worktrees inside the repo don't cause issues since they are separate git working trees

3. **Dirty Working Directory Check**: The system checks for uncommitted changes before creating worktrees and fails with an error if found. This prevents confusion about which changes belong to which variant.

---

## Architecture

### System Context

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Orbit CLI                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│  orbit run          │ orbit status    │ orbit cleanup   │ orbit finalize   │
│  orbit compare      │                 │                 │                   │
└─────────┬───────────┴────────┬────────┴────────┬────────┴─────────┬─────────┘
          │                    │                 │                   │
          ▼                    ▼                 ▼                   ▼
┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  ┌──────────────┐
│ internal/orbit  │  │internal/variants│  │internal/compare │  │internal/report│
│                 │  │                 │  │                 │  │               │
│ Orchestration   │  │ Worktree mgmt   │  │ Diff generation │  │ HTML report  │
│ Phase loop      │  │ Branch creation │  │ Claude compare  │  │ generation   │
│ Retry logic     │  │ Status tracking │  │ Result parsing  │  │              │
└────────┬────────┘  └────────┬────────┘  └────────┬────────┘  └──────┬───────┘
         │                    │                    │                   │
         ▼                    ▼                    ▼                   ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           File System                                        │
├─────────────────────────────────────────────────────────────────────────────┤
│  specs/{spec}/.orbit/                                                        │
│  ├── variants.json              # Variant metadata                           │
│  ├── variant-1/                 # Logs for variant 1                         │
│  ├── variant-2/                 # Logs for variant 2                         │
│  ├── worktrees/                 # Git worktrees                              │
│  │   ├── orbit-impl-1-{spec}/   # Worktree for variant 1                     │
│  │   └── orbit-impl-2-{spec}/   # Worktree for variant 2                     │
│  └── comparison-report/         # HTML report                                │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Package Dependencies

```
cmd/orbit/main.go
    ├── cmd/orbit/run.go      (modified: add variant flags)
    ├── cmd/orbit/status.go   (new)
    ├── cmd/orbit/cleanup.go  (new)
    ├── cmd/orbit/finalize.go (new)
    └── cmd/orbit/compare.go  (new)

internal/orbit/orbit.go
    ├── internal/variants/manager.go    (new)
    ├── internal/comparison/compare.go  (new)
    └── internal/report/generator.go    (new)

internal/variants/
    ├── manager.go      # Worktree/branch lifecycle
    ├── types.go        # Variant, VariantsMetadata types
    ├── git.go          # Git operations wrapper
    └── manager_test.go

internal/comparison/
    ├── compare.go      # Comparison orchestration
    ├── diff.go         # Git diff extraction
    ├── prompt.go       # Claude comparison prompt
    └── compare_test.go

internal/report/
    ├── generator.go    # HTML report generation
    ├── templates.go    # Embedded HTML templates
    ├── report.css      # Embedded styles
    └── generator_test.go
```

---

## Components and Interfaces

### 1. Variants Package (`internal/variants`)

Manages the lifecycle of variant worktrees and branches.

```go
// VariantStatus represents the state of a variant.
type VariantStatus string

const (
    StatusPending   VariantStatus = "pending"
    StatusRunning   VariantStatus = "running"
    StatusCompleted VariantStatus = "completed"
    StatusFailed    VariantStatus = "failed"
    StatusCanceled  VariantStatus = "canceled"
)

// Variant represents a single implementation variant.
type Variant struct {
    ID           int           `json:"id"`
    Branch       string        `json:"branch"`
    WorktreePath string        `json:"worktree_path"`
    Status       VariantStatus `json:"status"`
    Error        string        `json:"error,omitempty"`
    Guidance     string        `json:"guidance,omitempty"`

    // Metrics (populated after completion)
    Cost         float64       `json:"cost,omitempty"`
    Duration     time.Duration `json:"duration,omitempty"`
    NumTurns     int           `json:"num_turns,omitempty"`
}

// VariantsMetadata is the root structure for variants.json.
type VariantsMetadata struct {
    RunID          string     `json:"run_id"`
    BaseCommit     string     `json:"base_commit"`
    OriginalBranch string     `json:"original_branch"`
    StartedAt      time.Time  `json:"started_at"`
    Variants       []*Variant `json:"variants"`
}

// Config holds variant execution configuration.
type Config struct {
    Count        int
    Parallel     bool
    MaxParallel  int
    BranchPrefix string
    Guidance     []string // Per-variant guidance from file
}

// Manager handles variant lifecycle.
type Manager struct {
    config       Config
    specName     string
    specDir      string      // Path to specs/{spec-name}
    repoRoot     string      // Git repository root
    metadata     *VariantsMetadata
    metadataPath string      // specs/{spec}/.orbit/variants.json
    worktreeDir  string      // specs/{spec}/.orbit/worktrees/
    mu           sync.Mutex  // Protects metadata during parallel execution
    git          GitClient   // Interface for git operations (testable)
}

// NewManager creates a variant manager for a spec.
func NewManager(cfg Config, specName, specDir, repoRoot string) (*Manager, error)

// Setup creates worktrees and branches for all variants.
// Returns error if worktrees exist with different base commit.
func (m *Manager) Setup(ctx context.Context) error

// GetVariant returns a variant by ID.
func (m *Manager) GetVariant(id int) *Variant

// UpdateStatus updates a variant's status and persists to disk.
func (m *Manager) UpdateStatus(id int, status VariantStatus, err error) error

// UpdateMetrics updates a variant's metrics after completion.
func (m *Manager) UpdateMetrics(id int, cost float64, duration time.Duration, turns int) error

// Cleanup removes all worktrees and branches.
// If keepID > 0, preserves that variant.
func (m *Manager) Cleanup(ctx context.Context, keepID int) error

// Finalize rebases the chosen variant onto the original branch.
func (m *Manager) Finalize(ctx context.Context, variantID int) error

// Load reads existing variants.json if present.
func (m *Manager) Load() error

// Save persists the current metadata to variants.json.
func (m *Manager) Save() error

// GetVariantsSnapshot returns a copy of the variants slice for safe iteration.
// Use this before parallel execution to avoid race conditions.
func (m *Manager) GetVariantsSnapshot() []*Variant

// CountByStatus returns the count of variants with the given status.
func (m *Manager) CountByStatus(status VariantStatus) int
```

#### Git Operations (`internal/variants/git.go`)

```go
// GitClient interface for git operations (enables testing with mocks).
// Long-running operations accept context.Context for cancellation support.
type GitClient interface {
    GetCurrentBranch() (string, error)
    GetHeadCommit() (string, error)
    CreateBranch(name string) error
    CreateWorktree(ctx context.Context, path, branch string) error
    RemoveWorktree(ctx context.Context, path string) error
    DeleteBranch(name string) error
    GetDiff(ctx context.Context, worktreePath, baseCommit string) (string, error)
    Rebase(ctx context.Context, sourceBranch, targetBranch string) error
    BranchHasDiverged(branch, baseCommit string) (bool, error)
    HasUncommittedChanges() (bool, error)
}

// Git implements GitClient with real git command execution.
type Git struct {
    repoRoot string
}

func NewGit(repoRoot string) *Git

// GetCurrentBranch returns the current branch name.
func (g *Git) GetCurrentBranch() (string, error)

// GetHeadCommit returns the current HEAD commit SHA.
func (g *Git) GetHeadCommit() (string, error)

// CreateBranch creates a new branch from HEAD.
func (g *Git) CreateBranch(name string) error

// CreateWorktree creates a worktree for a branch at the specified path.
func (g *Git) CreateWorktree(path, branch string) error

// RemoveWorktree removes a worktree.
func (g *Git) RemoveWorktree(path string) error

// DeleteBranch deletes a local branch.
func (g *Git) DeleteBranch(name string) error

// GetDiff returns unified diff from base commit for a worktree.
func (g *Git) GetDiff(worktreePath, baseCommit string) (string, error)

// Rebase rebases source branch onto target branch.
func (g *Git) Rebase(sourceBranch, targetBranch string) error

// BranchHasDiverged checks if branch has new commits since baseCommit.
func (g *Git) BranchHasDiverged(branch, baseCommit string) (bool, error)
```

### 2. Comparison Package (`internal/comparison`)

Orchestrates the comparison of completed variants.

```go
// Result holds comparison output.
type Result struct {
    Recommendation int               `json:"recommendation"`
    Confidence     string            `json:"confidence"` // high, medium, low
    Summary        string            `json:"summary"`
    FileAnalyses   []FileAnalysis    `json:"file_analyses"`
    Observations   []string          `json:"observations"`
}

// FileAnalysis contains per-file comparison details.
type FileAnalysis struct {
    Path       string            `json:"path"`
    Variants   map[int]string    `json:"variants"` // variant ID -> assessment
    Preference int               `json:"preference,omitempty"`
}

// VariantData holds data for a single variant's comparison input.
type VariantData struct {
    ID      int
    Diff    string
    Metrics VariantMetrics
}

// VariantMetrics holds execution metrics.
type VariantMetrics struct {
    Cost     float64
    Duration time.Duration
    NumTurns int
}

// Comparator generates comparisons between variants.
type Comparator struct {
    claudeClient *claude.Client
    customCmd    string // Empty for built-in
    maxRetries   int    // Default: 3
}

func NewComparator(claudeClient *claude.Client, customCmd string) *Comparator

// Compare analyzes variants and returns structured results.
// Uses JSON validation with retry on malformed responses.
func (c *Comparator) Compare(ctx context.Context, specName string, variants []VariantData) (*Result, error)

// buildPrompt constructs the comparison prompt for Claude.
func (c *Comparator) buildPrompt(specName string, variants []VariantData) string

// parseAndValidate extracts JSON from Claude response and validates structure.
// Returns error if JSON is malformed or missing required fields.
func (c *Comparator) parseAndValidate(response string) (*Result, error)

// extractJSON finds and extracts JSON from Claude's text response.
// Handles cases where JSON is wrapped in markdown code blocks.
func (c *Comparator) extractJSON(response string) (string, error)
```

#### Comparison Prompt Structure

The built-in comparison prompt will be structured as:

```
You are comparing {N} implementation variants of the specification "{specName}".

## Variant Diffs

### Variant 1
<diff>
{unified diff from base commit}
</diff>

### Variant 2
<diff>
{unified diff from base commit}
</diff>

...

## Metrics

| Variant | Cost | Duration | Turns |
|---------|------|----------|-------|
| 1       | $X.XX | Xm Xs   | X     |
| 2       | $X.XX | Xm Xs   | X     |

## Instructions

Analyze these implementations and provide:
1. A recommendation (which variant number is best)
2. Confidence level (high/medium/low)
3. Executive summary (2-3 sentences)
4. Per-file analysis noting significant differences
5. Key observations about each approach

Output your analysis as JSON matching this schema:
{JSON schema for Result}
```

### 3. Report Package (`internal/report`)

Generates the HTML comparison report.

```go
// ReportData holds all data for report generation.
type ReportData struct {
    SpecName      string
    GeneratedAt   time.Time
    Variants      []VariantReportData
    Comparison    *comparison.Result
    BaseCommit    string
    OriginalBranch string
}

// VariantReportData holds per-variant report data.
type VariantReportData struct {
    ID           int
    Branch       string
    Status       string
    Error        string
    Diff         string
    Metrics      VariantMetrics
}

// VariantMetrics for report display.
type VariantMetrics struct {
    Cost     float64
    Duration string
    NumTurns int
}

// Generator creates HTML comparison reports.
type Generator struct {
    outputDir string
}

func NewGenerator(outputDir string) *Generator

// Generate creates the HTML report.
func (g *Generator) Generate(data *ReportData) error

// generateMainReport creates index.html.
func (g *Generator) generateMainReport(data *ReportData) error

// generateDiffFile creates a separate diff file for large diffs.
func (g *Generator) generateDiffFile(variantID int, diff string) (string, error)
```

#### Report Structure

```
specs/{spec-name}/comparison-report/
├── index.html              # Main report (self-contained)
└── diffs/                  # Large diffs (>500 lines)
    ├── variant-1-file-a.html
    └── variant-2-file-b.html
```

### 4. Modified Orbit Package (`internal/orbit`)

Extends the existing Orbit struct to support variant execution.

```go
// Config changes (additions shown)
type Config struct {
    // Existing fields...

    // Variant configuration
    VariantCount   int
    Parallel       bool
    MaxParallel    int
    BranchPrefix   string
    GuidanceFile   string
    CompareCommand string
}

// Orbit changes
type Orbit struct {
    // Existing fields...

    variantManager *variants.Manager  // nil for single-run mode
    isVariantRun   bool
}

// Run is modified to check for variant mode
func (o *Orbit) Run() error {
    if o.variantManager != nil {
        return o.runWithVariants(context.Background())
    }
    return o.runSingle()  // Existing behavior
}

// runWithVariants orchestrates multi-variant execution
func (o *Orbit) runWithVariants(ctx context.Context) error
```

### 5. CLI Commands

#### Modified: `orbit run`

New flags:
```
--variants N          Number of implementation variants (default: 1, disabled)
--parallel            Run variants in parallel
--max-parallel N      Maximum parallel variants (default: 3)
--branch-prefix STR   Branch naming prefix (default: orbit-impl)
--guidance-file PATH  YAML file with per-variant guidance
--compare-command CMD Custom comparison command
```

#### New: `orbit status <spec-name>`

```go
func statusCommand(args []string) error {
    // 1. Load variants.json from specs/{spec}/.orbit/
    // 2. Display table: ID, Branch, Path, Status
    // 3. Show base commit and original branch
}
```

Output example:
```
Variant Status: my-feature

Base Commit:     abc123
Original Branch: feature/my-feature
Started:         2026-01-11 10:30:00

ID  Branch                      Path                                        Status
1   orbit-impl-1/my-feature     .orbit/worktrees/orbit-impl-1-my-feature    completed
2   orbit-impl-2/my-feature     .orbit/worktrees/orbit-impl-2-my-feature    running
3   orbit-impl-3/my-feature     .orbit/worktrees/orbit-impl-3-my-feature    pending
```

#### New: `orbit cleanup <spec-name>`

```
--keep N      Preserve variant N, remove others
--force       Skip confirmation
--dry-run     Show what would be deleted
```

#### New: `orbit finalize <spec-name>`

```
--variant N   Variant to adopt (required)
--force       Skip confirmation
```

#### New: `orbit compare <spec-name>`

```
--compare-command CMD  Custom comparison command
```

---

## Data Models

### 1. variants.json

Location: `specs/{spec-name}/.orbit/variants.json`

```json
{
  "run_id": "550e8400-e29b-41d4-a716-446655440000",
  "base_commit": "abc123def456",
  "original_branch": "feature/my-feature",
  "started_at": "2026-01-11T10:30:00Z",
  "variants": [
    {
      "id": 1,
      "branch": "orbit-impl-1/my-feature",
      "worktree_path": ".orbit/worktrees/orbit-impl-1-my-feature",
      "status": "completed",
      "guidance": "Prioritize simplicity",
      "cost": 0.0523,
      "duration": 180000000000,
      "num_turns": 42
    },
    {
      "id": 2,
      "branch": "orbit-impl-2/my-feature",
      "worktree_path": ".orbit/worktrees/orbit-impl-2-my-feature",
      "status": "failed",
      "error": "Rate limit exceeded after max retries"
    }
  ]
}
```

### 2. Guidance File Schema

```yaml
variants:
  - id: 1
    guidance: "Prioritize simplicity and maintainability"
  - id: 2
    guidance: "Optimize for performance"
  - id: 3
    guidance: "Use idiomatic patterns from existing codebase"
global_guidance: "Ensure all public functions have documentation"
```

### 3. Comparison Result Schema

```json
{
  "recommendation": 1,
  "confidence": "high",
  "summary": "Variant 1 provides cleaner abstractions with better separation of concerns, while maintaining equivalent performance characteristics.",
  "file_analyses": [
    {
      "path": "internal/service.go",
      "variants": {
        "1": "Uses dependency injection pattern",
        "2": "Uses global state"
      },
      "preference": 1
    }
  ],
  "observations": [
    "Variant 1 has 20% fewer lines of code",
    "Variant 2 uses more aggressive caching"
  ]
}
```

---

## Error Handling

### Variant Setup Errors

| Error | Handling |
|-------|----------|
| Uncommitted changes in working directory | Fail with error suggesting commit or stash |
| Worktree exists, different base commit | Fail with error suggesting `orbit cleanup` |
| Worktrees directory not writable | Fail with descriptive error |
| Git worktree add fails | Clean up any created worktrees, fail with git error |
| Branch already exists | Fail with error suggesting cleanup or different prefix |

### Variant Execution Errors

| Error | Handling |
|-------|----------|
| Single variant fails | Mark failed in metadata, continue with others |
| All variants fail | Generate partial report with failure info |
| Rate limit hit | Each variant handles independently with existing retry logic |
| SIGINT received | Stop scheduling, allow running phases to complete (30s timeout), preserve worktrees |

### Comparison Errors

| Error | Handling |
|-------|----------|
| Diffs exceed context limit | Fail with descriptive error |
| Claude comparison fails | Preserve worktrees, fail with error |
| Custom command fails | Preserve worktrees, fail with error |
| JSON validation fails | Retry up to 3 times with clarification prompt |
| JSON validation exhausted | Fail with error showing validation issues |

### Finalize Errors

| Error | Handling |
|-------|----------|
| Original branch diverged | Fail with error explaining divergence |
| Rebase conflicts | Pause, print instructions for manual resolution |
| Variant not found | Fail with error |

---

## Testing Strategy

### Unit Tests

**variants/manager_test.go:**
- `TestSetup_CreatesWorktreesAndBranches`
- `TestSetup_ReusesCompatibleWorktrees`
- `TestSetup_FailsOnDivergentWorktrees`
- `TestSetup_WorktreesInOrbitDirectory`
- `TestSetup_FailsOnDirtyWorkingDirectory`
- `TestSetup_CreatesGitignore`
- `TestSetup_UpdatesExistingGitignore`
- `TestUpdateStatus_PersistsToFile`
- `TestUpdateStatus_ConcurrentSafe`
- `TestCleanup_RemovesAllWorktrees`
- `TestCleanup_PreservesKeptVariant`
- `TestLoad_ParsesExistingMetadata`
- `TestSave_AtomicWrite`
- `TestSave_ConcurrentAccess`
- `TestGetVariantsSnapshot_ReturnsCopy`
- `TestCountByStatus`

**variants/git_test.go:**
- `TestGetCurrentBranch`
- `TestGetHeadCommit`
- `TestCreateWorktree`
- `TestGetDiff`
- `TestBranchHasDiverged`
- `TestHasUncommittedChanges`

**variants/mock_git_test.go:**
Tests using mock GitClient for unit testing without real git:
- `TestSetup_WithMockGit`
- `TestCleanup_WithMockGit`
- `TestFinalize_WithMockGit`

**comparison/compare_test.go:**
- `TestBuildPrompt_IncludesAllVariants`
- `TestBuildPrompt_IncludesMetrics`
- `TestCompare_ParsesClaudeResponse`
- `TestCompare_HandlesPartialFailure`
- `TestParseAndValidate_ValidJSON`
- `TestParseAndValidate_MissingFields`
- `TestParseAndValidate_InvalidConfidence`
- `TestExtractJSON_PlainJSON`
- `TestExtractJSON_MarkdownCodeBlock`
- `TestCompare_RetriesOnValidationFailure`
- `TestCompare_FailsAfterMaxRetries`

**report/generator_test.go:**
- `TestGenerate_CreatesIndexHTML`
- `TestGenerate_EscapesContent`
- `TestGenerate_SplitsLargeDiffs`
- `TestGenerate_IncludesFailedVariants`

### Integration Tests

**TestVariantRun_Sequential:**
1. Create test repository with tasks.md
2. Run `orbit run --variants 2`
3. Verify worktrees created in `.orbit/worktrees/`
4. Verify variants.json contains correct data
5. Verify comparison report generated
6. Cleanup

**TestVariantRun_Parallel:**
1. Create test repository
2. Run `orbit run --variants 3 --parallel`
3. Verify all variants executed
4. Verify semaphore limits respected
5. Cleanup

**TestVariantRun_SingleSuccess:**
1. Create test repository
2. Run `orbit run --variants 2` with one variant configured to fail
3. Verify comparison skipped (log message)
4. Verify report generated with single variant

**TestVariantRun_DirtyWorkingDirectory:**
1. Create test repository
2. Create uncommitted changes
3. Run `orbit run --variants 2`
4. Verify error about uncommitted changes

**TestCleanup_RemovesWorktrees:**
1. Set up variant worktrees
2. Run `orbit cleanup <spec>`
3. Verify worktrees removed
4. Verify branches deleted
5. Verify variants.json removed

**TestFinalize_RebasesVariant:**
1. Set up completed variant worktrees
2. Run `orbit finalize <spec> --variant 1`
3. Verify changes rebased onto original branch
4. Verify other worktrees removed
5. Verify variant branches deleted

### Property-Based Testing

**Spec Name Sanitization:**
Using `rapid` for property-based testing of the spec name sanitization function:

```go
func TestPropertySanitizeName(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        name := rapid.String().Draw(t, "name")
        sanitized := sanitizeSpecName(name)

        // Properties:
        // 1. Result contains only safe filesystem characters
        for _, c := range sanitized {
            if !isFilesystemSafe(c) {
                t.Fatalf("unsafe character in sanitized name: %c", c)
            }
        }

        // 2. Empty input produces empty output
        if name == "" && sanitized != "" {
            t.Fatal("empty input should produce empty output")
        }

        // 3. Idempotent: sanitizing twice gives same result
        doubleSanitized := sanitizeSpecName(sanitized)
        if sanitized != doubleSanitized {
            t.Fatalf("not idempotent: %q != %q", sanitized, doubleSanitized)
        }
    })
}
```

---

## Requirement Traceability

| Requirement | Design Element |
|-------------|----------------|
| [1.1](#1.1) --variants N | `Config.VariantCount`, `runCommand` flag |
| [1.2](#1.2) Backwards compatible | `o.variantManager == nil` check in `Run()` |
| [1.3](#1.3) --parallel | `Config.Parallel`, semaphore in `runWithVariants` |
| [1.9](#1.9) Guidance file | `Config.GuidanceFile`, YAML parsing |
| [2.1](#2.1) Worktrees in .orbit/worktrees/ | `Manager.Setup()`, `worktreePath()` function |
| [2.2](#2.2) Branch naming | `Git.CreateBranch()` with pattern |
| [2.9](#2.9) Auto .gitignore | `Manager.ensureGitignore()` |
| [2.3](#2.3) Reuse compatible | `Manager.Load()`, base commit check |
| [2.5](#2.5) variants.json | `VariantsMetadata` struct, `Manager.Save()` |
| [3.3](#3.3) Continue on failure | `runWithVariants` error handling |
| [3.6](#3.6) Capture metrics | `Variant` struct metrics fields |
| [4.2](#4.2) Semaphore | `sync.Semaphore` in parallel execution |
| [5.1](#5.1) Git diffs | `Git.GetDiff()` |
| [5.2](#5.2) Claude comparison | `Comparator.Compare()` |
| [5.8](#5.8) Context limit | Prompt size check in `buildPrompt()` |
| [6.1](#6.1) Report location | `Generator.outputDir` |
| [6.9](#6.9) HTML escape | `html.EscapeString()` in templates |
| [7.1](#7.1) Status command | `statusCommand()` |
| [8.1](#8.1) Cleanup command | `cleanupCommand()` |
| [9.1](#9.1) Finalize command | `finalizeCommand()` |
| [9.2](#9.2) Verify unchanged | `Git.BranchHasDiverged()` |
| [11.1](#11.1) SIGINT handling | Context cancellation in `runWithVariants` |

---

## Implementation Notes

### Worktree Path Calculation

Worktrees are stored inside the spec's `.orbit` directory to avoid polluting sibling directories:

```go
func worktreePath(specDir, prefix string, variantID int, specName string) string {
    sanitized := sanitizeSpecName(specName)
    // Path: specs/{spec}/.orbit/worktrees/{prefix}-{N}-{spec}/
    return filepath.Join(specDir, ".orbit", "worktrees",
        fmt.Sprintf("%s-%d-%s", prefix, variantID, sanitized))
}
```

### File Locking and Atomic Writes

The `variants.json` file may be accessed by parallel variant executions. To prevent corruption:

```go
// Save persists metadata atomically with file locking.
func (m *Manager) Save() error {
    m.mu.Lock()
    defer m.mu.Unlock()

    data, err := json.MarshalIndent(m.metadata, "", "  ")
    if err != nil {
        return fmt.Errorf("marshal metadata: %w", err)
    }

    // Write to temp file first
    tmpPath := m.metadataPath + ".tmp"
    if err := os.WriteFile(tmpPath, data, 0644); err != nil {
        return fmt.Errorf("write temp file: %w", err)
    }

    // Atomic rename
    if err := os.Rename(tmpPath, m.metadataPath); err != nil {
        os.Remove(tmpPath) // Clean up on failure
        return fmt.Errorf("rename to final: %w", err)
    }

    return nil
}

// Load reads metadata with the mutex held.
func (m *Manager) Load() error {
    m.mu.Lock()
    defer m.mu.Unlock()

    // ... read and unmarshal ...
}
```

### Guidance Injection Mechanism

Per-variant guidance is injected into the initial prompt for each variant's Claude session:

```go
// runVariant executes a single variant's implementation phases.
func (o *Orbit) runVariant(ctx context.Context, v *variants.Variant) error {
    // Build the phase prompt with variant-specific guidance
    basePrompt := o.buildPhasePrompt(phase)

    var fullPrompt string
    if v.Guidance != "" {
        fullPrompt = fmt.Sprintf(`%s

## Guidance for this Implementation

%s
`, basePrompt, v.Guidance)
    } else {
        fullPrompt = basePrompt
    }

    // Include global guidance if present
    if o.config.GlobalGuidance != "" {
        fullPrompt = fmt.Sprintf(`%s

## Global Guidance

%s
`, fullPrompt, o.config.GlobalGuidance)
    }

    return o.claudeClient.RunPhase(ctx, fullPrompt, v.WorktreePath)
}
```

The guidance is prepended to the phase prompt, giving Claude context about the desired approach before it begins implementation.

### Automatic .gitignore Management

The system automatically ensures worktrees are gitignored to prevent accidental commits:

```go
// ensureGitignore creates or updates .orbit/.gitignore to ignore worktrees.
func (m *Manager) ensureGitignore() error {
    gitignorePath := filepath.Join(m.specDir, ".orbit", ".gitignore")

    // Check if file exists and already contains the entry
    content, err := os.ReadFile(gitignorePath)
    if err == nil {
        if strings.Contains(string(content), "worktrees/") {
            return nil  // Already configured
        }
        // Append to existing file
        f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_WRONLY, 0644)
        if err != nil {
            return err
        }
        defer f.Close()
        _, err = f.WriteString("\n# Variant worktrees (managed by orbit)\nworktrees/\n")
        return err
    }

    // Create new .gitignore
    return os.WriteFile(gitignorePath, []byte(
        "# Orbit variant data (auto-generated)\nworktrees/\n"), 0644)
}
```

This is called during `Setup()` before creating any worktrees.

### JSON Validation and Retry for Comparison

Claude's comparison output must be valid JSON matching the Result schema. The comparator implements retry logic:

```go
// Compare with validation and retry.
func (c *Comparator) Compare(ctx context.Context, specName string, variants []VariantData) (*Result, error) {
    originalPrompt := c.buildPrompt(specName, variants)
    prompt := originalPrompt

    for attempt := 0; attempt < c.maxRetries; attempt++ {
        response, err := c.claudeClient.RunCustomPrompt(prompt)
        if err != nil {
            return nil, fmt.Errorf("claude execution failed: %w", err)
        }

        result, err := c.parseAndValidate(response.Content, len(variants))
        if err == nil {
            return result, nil
        }

        // On validation failure, retry with clarification (prepend to original prompt)
        if attempt < c.maxRetries-1 {
            log.Printf("Comparison JSON validation failed (attempt %d/%d): %v",
                attempt+1, c.maxRetries, err)
            prompt = fmt.Sprintf(`Your previous response was not valid JSON. Error: %s

Please provide the comparison result as valid JSON only, with no additional text.

---

%s`, err.Error(), originalPrompt)  // Prepend to original, don't rebuild
        }
    }

    return nil, fmt.Errorf("comparison failed after %d attempts: JSON validation errors", c.maxRetries)
}

// parseAndValidate extracts and validates JSON from response.
func (c *Comparator) parseAndValidate(response string, numVariants int) (*Result, error) {
    jsonStr, err := c.extractJSON(response)
    if err != nil {
        return nil, fmt.Errorf("extract JSON: %w", err)
    }

    // Use strict parsing to catch unknown fields
    decoder := json.NewDecoder(strings.NewReader(jsonStr))
    decoder.DisallowUnknownFields()

    var result Result
    if err := decoder.Decode(&result); err != nil {
        return nil, fmt.Errorf("unmarshal: %w", err)
    }

    // Validate required fields with range checks
    if result.Recommendation < 1 || result.Recommendation > numVariants {
        return nil, fmt.Errorf("recommendation must be between 1 and %d, got %d",
            numVariants, result.Recommendation)
    }
    if result.Confidence == "" {
        return nil, errors.New("missing required field: confidence")
    }
    if result.Confidence != "high" && result.Confidence != "medium" && result.Confidence != "low" {
        return nil, fmt.Errorf("invalid confidence value: %s", result.Confidence)
    }
    if result.Summary == "" {
        return nil, errors.New("missing required field: summary")
    }

    return &result, nil
}
```

### Parallel Execution Flow

```go
func (o *Orbit) runWithVariants(ctx context.Context) error {
    // Check for uncommitted changes before setup
    if hasChanges, err := o.variantManager.git.HasUncommittedChanges(); err != nil {
        return fmt.Errorf("check git status: %w", err)
    } else if hasChanges {
        return errors.New("working directory has uncommitted changes; commit or stash before running variants")
    }

    // Setup worktrees
    if err := o.variantManager.Setup(ctx); err != nil {
        return err
    }

    // Snapshot variants slice under lock to avoid race condition during parallel execution
    variants := o.variantManager.GetVariantsSnapshot()

    // Create semaphore for parallel limit
    sem := make(chan struct{}, o.config.MaxParallel)
    var wg sync.WaitGroup
    var mu sync.Mutex
    var errs []error

    for _, v := range variants {
        if !o.config.Parallel {
            // Sequential: run directly
            if err := o.runVariant(ctx, v); err != nil {
                errs = append(errs, err)
            }
            continue
        }

        // Parallel: spawn goroutine with semaphore
        wg.Add(1)
        go func(variant *variants.Variant) {
            defer wg.Done()

            sem <- struct{}{}  // Acquire
            defer func() { <-sem }()  // Release

            if err := o.runVariant(ctx, variant); err != nil {
                mu.Lock()
                errs = append(errs, err)
                mu.Unlock()
            }
        }(v)
    }

    wg.Wait()

    // Run comparison if at least one succeeded
    successCount := o.variantManager.CountByStatus(variants.StatusCompleted)
    if successCount == 0 {
        // Generate partial report with failure info
        return o.generatePartialReport()
    }
    if successCount == 1 {
        // Skip comparison for single successful variant
        log.Printf("Only one variant succeeded; skipping comparison")
    } else {
        // Compare multiple successful variants
        if err := o.runComparison(ctx); err != nil {
            return err
        }
    }

    // Generate report
    return o.generateReport()
}
```

### SIGINT Handling

```go
func (o *Orbit) runWithVariants(ctx context.Context) error {
    ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
    defer cancel()

    // ... execution ...

    select {
    case <-ctx.Done():
        // Stop scheduling new phases
        // Wait up to 30s for running phases
        o.variantManager.UpdateStatus(v.ID, variants.StatusCanceled, nil)
        log.Printf("Interrupted. Worktrees preserved at: %s", o.variantManager.specDir)
        return ctx.Err()
    default:
        // Continue
    }
}
```

---

## File Changes Summary

| File | Change Type | Description |
|------|-------------|-------------|
| `cmd/orbit/main.go` | Modify | Add subcommand routing for status, cleanup, finalize, compare |
| `cmd/orbit/run.go` | Modify | Add variant flags |
| `cmd/orbit/status.go` | New | Status subcommand |
| `cmd/orbit/cleanup.go` | New | Cleanup subcommand |
| `cmd/orbit/finalize.go` | New | Finalize subcommand |
| `cmd/orbit/compare.go` | New | Compare subcommand |
| `internal/orbit/orbit.go` | Modify | Add variant execution logic |
| `internal/config/config.go` | Modify | Add variant configuration |
| `internal/variants/manager.go` | New | Worktree lifecycle |
| `internal/variants/types.go` | New | Data types |
| `internal/variants/git.go` | New | Git operations |
| `internal/variants/manager_test.go` | New | Tests |
| `internal/comparison/compare.go` | New | Comparison logic |
| `internal/comparison/diff.go` | New | Diff extraction |
| `internal/comparison/prompt.go` | New | Prompt building |
| `internal/comparison/compare_test.go` | New | Tests |
| `internal/report/generator.go` | New | HTML generation |
| `internal/report/templates.go` | New | HTML templates |
| `internal/report/report.css` | New | Styles |
| `internal/report/generator_test.go` | New | Tests |
