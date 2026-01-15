---
references:
    - requirements.md
    - design.md
    - decision_log.md
---
# Multi-Spec Comparison Implementation Tasks

## Phase 1: Core Types and Git Operations

- [x] 1. Create variant types package
  - Create internal/variants/types.go with VariantStatus, Variant, VariantsMetadata, Config structs
  - Define all type constants (StatusPending, StatusRunning, etc.), JSON tags for serialization
  - Requirements: [2.5](requirements.md#2.5), [3.4](requirements.md#3.4)
  - References: design.md

- [x] 2. Create GitClient interface and implementation
  - Create internal/variants/git.go with GitClient interface
  - Methods: GetCurrentBranch, GetHeadCommit, CreateBranch, CreateWorktree, RemoveWorktree, DeleteBranch, GetDiff, Rebase, BranchHasDiverged, HasUncommittedChanges
  - Include context.Context for long-running operations
  - Requirements: [2.1](requirements.md#2.1), [2.2](requirements.md#2.2), [2.3](requirements.md#2.3), [2.4](requirements.md#2.4)
  - References: design.md

- [x] 3. Write unit tests for Git implementation
  - Create internal/variants/git_test.go with tests for all GitClient methods
  - Test GetCurrentBranch, GetHeadCommit, CreateWorktree, GetDiff, BranchHasDiverged, HasUncommittedChanges using real git operations in temp directories
  - Requirements: [12.1](requirements.md#12.1)
  - References: design.md

- [x] 4. Create mock GitClient for testing
  - Create internal/variants/mock_git.go with mock implementation
  - Implement GitClient interface with configurable responses for unit testing Manager without real git
  - References: design.md

## Phase 2: Variant Manager

- [x] 5. Create Manager struct and core methods
  - Create internal/variants/manager.go with Manager struct
  - Include config, specName, specDir, repoRoot, metadata, metadataPath, worktreeDir, mutex, gitClient fields
  - Implement NewManager constructor
  - Requirements: [2.5](requirements.md#2.5)
  - References: design.md

- [x] 6. Implement Load and Save with atomic writes
  - Add Load() and Save() methods to Manager
  - Save uses temp file + atomic rename pattern with mutex protection
  - Load reads and unmarshals variants.json
  - Requirements: [2.5](requirements.md#2.5)
  - References: design.md

- [x] 7. Implement ensureGitignore method
  - Add ensureGitignore() to Manager
  - Create or update .orbit/.gitignore to include worktrees/ entry
  - Requirements: [2.9](requirements.md#2.9)
  - References: design.md, decision_log.md

- [x] 8. Implement Setup method
  - Add Setup(ctx) method to Manager
  - Check for uncommitted changes, create .orbit/worktrees/ directory
  - Call ensureGitignore, create branches and worktrees for each variant
  - Handle worktree reuse when base commit matches
  - Requirements: [2.1](requirements.md#2.1), [2.3](requirements.md#2.3), [2.4](requirements.md#2.4), [2.6](requirements.md#2.6), [2.7](requirements.md#2.7), [2.8](requirements.md#2.8)
  - References: design.md

- [x] 9. Implement status update methods
  - Add UpdateStatus, UpdateMetrics, GetVariant, GetVariantsSnapshot, CountByStatus methods
  - All methods use mutex protection, persist changes via Save()
  - Requirements: [3.4](requirements.md#3.4), [3.6](requirements.md#3.6)
  - References: design.md

- [x] 10. Implement Cleanup method
  - Add Cleanup(ctx, keepID) method to Manager
  - Remove worktrees and delete branches recorded in variants.json
  - Optionally preserve one variant
  - Requirements: [8.2](requirements.md#8.2), [8.3](requirements.md#8.3), [8.4](requirements.md#8.4), [8.5](requirements.md#8.5), [8.8](requirements.md#8.8)
  - References: design.md, decision_log.md

- [x] 11. Implement Finalize method
  - Add Finalize(ctx, variantID) method to Manager
  - Verify original branch has not diverged
  - Rebase variant onto original, cleanup other variants
  - Requirements: [9.2](requirements.md#9.2), [9.3](requirements.md#9.3), [9.4](requirements.md#9.4), [9.5](requirements.md#9.5), [9.6](requirements.md#9.6), [9.7](requirements.md#9.7)
  - References: design.md, decision_log.md

- [x] 12. Write Manager unit tests
  - Create internal/variants/manager_test.go
  - Test Setup (creates worktrees, reuses compatible, fails on divergent, fails on dirty, creates gitignore)
  - Test UpdateStatus, Save (atomic, concurrent), GetVariantsSnapshot, Cleanup, Finalize using mock GitClient
  - Requirements: [2.1](requirements.md#2.1), [2.3](requirements.md#2.3), [2.4](requirements.md#2.4), [2.5](requirements.md#2.5), [2.6](requirements.md#2.6), [2.7](requirements.md#2.7), [2.8](requirements.md#2.8), [2.9](requirements.md#2.9)
  - References: design.md

## Phase 3: Comparison Package

- [x] 13. Create comparison types
  - Create internal/comparison/types.go with Result, FileAnalysis, VariantData, VariantMetrics structs
  - JSON tags for all fields, validation-friendly structure
  - Requirements: [5.4](requirements.md#5.4)
  - References: design.md

- [x] 14. Implement diff extraction
  - Create internal/comparison/diff.go with functions to gather diffs from variants
  - Use GitClient.GetDiff to get unified diffs from base commit for each variant
  - Requirements: [5.1](requirements.md#5.1), [5.5](requirements.md#5.5)
  - References: design.md, decision_log.md

- [x] 15. Implement comparison prompt builder
  - Create internal/comparison/prompt.go with buildPrompt function
  - Construct prompt with variant diffs, metrics table, JSON schema for output
  - Include context size estimation
  - Requirements: [5.2](requirements.md#5.2), [5.8](requirements.md#5.8)
  - References: design.md

- [x] 16. Implement Comparator with JSON validation
  - Create internal/comparison/compare.go with Comparator struct
  - Compare method with retry loop, parseAndValidate with range checking
  - extractJSON for markdown code blocks, DisallowUnknownFields for strict parsing
  - Requirements: [5.2](requirements.md#5.2), [5.3](requirements.md#5.3), [5.4](requirements.md#5.4), [5.7](requirements.md#5.7)
  - References: design.md

- [x] 17. Write comparison unit tests
  - Create internal/comparison/compare_test.go
  - Test buildPrompt includes all variants and metrics
  - Test parseAndValidate (valid JSON, missing fields, invalid confidence, range validation)
  - Test extractJSON (plain and markdown), retry on validation failure, fail after max retries
  - Requirements: [5.1](requirements.md#5.1), [5.2](requirements.md#5.2), [5.3](requirements.md#5.3), [5.4](requirements.md#5.4), [5.5](requirements.md#5.5), [5.7](requirements.md#5.7), [5.8](requirements.md#5.8)
  - References: design.md

## Phase 4: Report Generation

- [x] 18. Create report types and templates
  - Create internal/report/types.go and internal/report/templates.go
  - ReportData, VariantReportData structs, embedded HTML templates using html/template
  - Requirements: [6.2](requirements.md#6.2), [6.3](requirements.md#6.3)
  - References: design.md

- [x] 19. Create report CSS
  - Create internal/report/report.css embedded in templates.go
  - Self-contained styles for report, collapsible sections, responsive layout, print-friendly
  - Requirements: [6.5](requirements.md#6.5), [6.7](requirements.md#6.7)
  - References: design.md

- [x] 20. Implement report Generator
  - Create internal/report/generator.go with Generator struct
  - Generate method creates index.html, handles large diffs (>500 lines) as separate files
  - Escapes all content for HTML safety
  - Requirements: [6.1](requirements.md#6.1), [6.4](requirements.md#6.4), [6.6](requirements.md#6.6), [6.8](requirements.md#6.8), [6.9](requirements.md#6.9)
  - References: design.md

- [x] 21. Write report unit tests
  - Create internal/report/generator_test.go
  - Test Generate creates index.html, content is escaped
  - Test large diffs split to separate files, failed variants included
  - Requirements: [6.1](requirements.md#6.1), [6.2](requirements.md#6.2), [6.3](requirements.md#6.3), [6.4](requirements.md#6.4), [6.5](requirements.md#6.5), [6.6](requirements.md#6.6), [6.7](requirements.md#6.7), [6.8](requirements.md#6.8), [6.9](requirements.md#6.9)
  - References: design.md

## Phase 5: CLI Commands

- [x] 22. Add variant flags to run command
  - Modify cmd/orbit/run.go to add --variants, --parallel, --max-parallel, --branch-prefix, --guidance-file, --compare-command flags
  - Parse flags, validate values, update Config struct
  - Requirements: [1.1](requirements.md#1.1), [1.3](requirements.md#1.3), [1.5](requirements.md#1.5), [1.6](requirements.md#1.6), [1.8](requirements.md#1.8), [1.9](requirements.md#1.9)
  - References: design.md

- [x] 23. Implement guidance file parsing
  - Add guidance file parsing to cmd/orbit/run.go or internal/config
  - Parse YAML schema with variants array and global_guidance
  - Validate against variant count
  - Requirements: [1.9](requirements.md#1.9), [1.10](requirements.md#1.10)
  - References: design.md

- [x] 24. Implement status command
  - Create cmd/orbit/status.go with statusCommand function
  - Load variants.json, display table with ID, Branch, Path, Status
  - Show base commit and original branch
  - Requirements: [7.1](requirements.md#7.1), [7.2](requirements.md#7.2), [7.3](requirements.md#7.3), [7.4](requirements.md#7.4)
  - References: design.md

- [x] 25. Implement cleanup command
  - Create cmd/orbit/cleanup.go with cleanupCommand function
  - Support --keep, --force, --dry-run flags
  - Confirmation prompt, call Manager.Cleanup
  - Requirements: [8.1](requirements.md#8.1), [8.2](requirements.md#8.2), [8.3](requirements.md#8.3), [8.4](requirements.md#8.4), [8.5](requirements.md#8.5), [8.6](requirements.md#8.6), [8.7](requirements.md#8.7), [8.8](requirements.md#8.8)
  - References: design.md

- [x] 26. Implement finalize command
  - Create cmd/orbit/finalize.go with finalizeCommand function
  - Require --variant flag, support --force
  - Confirmation prompt, call Manager.Finalize
  - Requirements: [9.1](requirements.md#9.1), [9.2](requirements.md#9.2), [9.3](requirements.md#9.3), [9.4](requirements.md#9.4), [9.5](requirements.md#9.5), [9.6](requirements.md#9.6), [9.7](requirements.md#9.7), [9.8](requirements.md#9.8), [9.9](requirements.md#9.9)
  - References: design.md

- [x] 27. Implement compare command
  - Create cmd/orbit/compare.go with compareCommand function
  - Load existing variants, run comparison, regenerate report
  - Requirements: [10.1](requirements.md#10.1), [10.2](requirements.md#10.2), [10.3](requirements.md#10.3), [10.4](requirements.md#10.4)
  - References: design.md

- [x] 28. Update main.go for subcommand routing
  - Modify cmd/orbit/main.go to route status, cleanup, finalize, compare subcommands
  - Add cases to command switch, wire up to respective command functions
  - References: design.md

## Phase 6: Orbit Integration

- [x] 29. Add variant configuration to Config
  - Modify internal/config/config.go to add variant fields
  - Add VariantCount, Parallel, MaxParallel, BranchPrefix, GuidanceFile, CompareCommand, GlobalGuidance fields
  - Requirements: [1.7](requirements.md#1.7)
  - References: design.md

- [x] 30. Add variant support to Orbit struct
  - Modify internal/orbit/orbit.go to add variantManager field
  - Add variantManager *variants.Manager, modify Run() to check for variant mode
  - Requirements: [1.2](requirements.md#1.2)
  - References: design.md, decision_log.md

- [x] 31. Implement runWithVariants orchestration
  - Add runWithVariants(ctx) method to Orbit
  - Check dirty working directory, setup worktrees, snapshot variants
  - Run with semaphore for parallel limit, handle SIGINT
  - Run comparison if multiple succeed, generate report
  - Requirements: [3.1](requirements.md#3.1), [3.2](requirements.md#3.2), [3.3](requirements.md#3.3), [4.1](requirements.md#4.1), [4.2](requirements.md#4.2), [4.3](requirements.md#4.3), [4.4](requirements.md#4.4), [11.1](requirements.md#11.1), [11.2](requirements.md#11.2), [11.3](requirements.md#11.3), [11.4](requirements.md#11.4), [11.5](requirements.md#11.5)
  - References: design.md

- [x] 32. Implement runVariant method
  - Add runVariant(ctx, variant) method to Orbit
  - Run all spec phases in variant worktree
  - Inject guidance into prompts, capture metrics, update status
  - Requirements: [3.1](requirements.md#3.1), [3.5](requirements.md#3.5), [3.6](requirements.md#3.6), [3.7](requirements.md#3.7)
  - References: design.md

- [x] 33. Wire comparison and report generation
  - Add runComparison and generateReport methods to Orbit
  - Create Comparator, gather diffs, run comparison
  - Create Generator, generate report, handle single-variant case
  - Requirements: [5.1](requirements.md#5.1), [5.2](requirements.md#5.2), [6.1](requirements.md#6.1)
  - References: design.md

## Phase 7: Integration Tests

- [x] 34. Write integration test for sequential variant run
  - Create test in internal/orbit/orbit_test.go or integration_test.go
  - Create temp git repo with tasks.md, run orbit with --variants 2
  - Verify worktrees in .orbit/worktrees/, verify variants.json, verify comparison report
  - References: design.md

- [x] 35. Write integration test for parallel variant run
  - Add test for parallel execution
  - Run with --variants 3 --parallel, verify all executed, verify semaphore respected
  - Requirements: [4.1](requirements.md#4.1), [4.2](requirements.md#4.2)
  - References: design.md

- [x] 36. Write integration test for single successful variant
  - Add test for partial success scenario
  - Configure one variant to fail, verify comparison skipped, report still generated
  - Requirements: [3.3](requirements.md#3.3), [3.7](requirements.md#3.7)
  - References: design.md

- [x] 37. Write integration test for cleanup
  - Add test for cleanup command
  - Set up worktrees, run cleanup, verify removed, verify variants.json removed
  - Requirements: [8.1](requirements.md#8.1), [8.2](requirements.md#8.2), [8.3](requirements.md#8.3), [8.4](requirements.md#8.4), [8.5](requirements.md#8.5), [8.6](requirements.md#8.6), [8.7](requirements.md#8.7), [8.8](requirements.md#8.8)
  - References: design.md

- [x] 38. Write integration test for finalize
  - Add test for finalize command
  - Set up completed variants, run finalize --variant 1, verify rebased, verify cleanup
  - Requirements: [9.1](requirements.md#9.1), [9.2](requirements.md#9.2), [9.3](requirements.md#9.3), [9.4](requirements.md#9.4), [9.5](requirements.md#9.5), [9.6](requirements.md#9.6), [9.7](requirements.md#9.7), [9.8](requirements.md#9.8), [9.9](requirements.md#9.9)
  - References: design.md

- [x] 39. Write property-based test for spec name sanitization
  - Add property test using rapid
  - Test sanitizeSpecName produces only filesystem-safe characters, is idempotent, handles empty input
  - Requirements: [2.8](requirements.md#2.8)
  - References: design.md
