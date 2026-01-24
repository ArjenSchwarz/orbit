---
references:
    - requirements.md
    - design.md
    - decision_log.md
---
# Variant Consolidation Implementation

## Phase 1: Report Generation Enhancement

- [x] 1. Add VariantCommits field to ReportData
  - Add VariantCommits map[int]string field to ReportData struct in internal/report/types.go
  - Update report generation to populate this field with HEAD commit SHA for each variant
  - Requirements: [1.7](requirements.md#1.7)
  - References: internal/report/types.go

- [x] 2. Implement dual-format report generation with go-output v2
  - Modify Generator to use go-output v2 document builder pattern
  - Generate both HTML (index.html) and Markdown (comparison-report.md) from single data structure
  - Include YAML frontmatter metadata in Markdown output (generated_at, base_commit, variant_commits)
  - Omit empty sections rather than showing placeholders
  - Requirements: [1.1](requirements.md#1.1), [1.2](requirements.md#1.2), [1.3](requirements.md#1.3), [1.4](requirements.md#1.4), [1.5](requirements.md#1.5)
  - References: internal/report/generator.go

- [x] 3. Add relative links to diff files in Markdown report
  - Update Markdown renderer to include relative links to separate diff files for large diffs
  - Requirements: [1.6](requirements.md#1.6)
  - References: internal/report/generator.go

- [x] 4. Write tests for dual-format report generation
  - Add TestReportMultiFormat for HTML + Markdown generation
  - Test metadata inclusion, empty section omission, and diff file linking
  - Requirements: [1.1](requirements.md#1.1), [1.2](requirements.md#1.2), [1.3](requirements.md#1.3), [1.4](requirements.md#1.4), [1.5](requirements.md#1.5), [1.6](requirements.md#1.6), [1.7](requirements.md#1.7)
  - References: internal/report/generator_test.go

## Phase 2: Consolidation Package Foundation

- [x] 5. Create consolidation types
  - Create internal/consolidation/types.go with Config, ConsolidationResult, ConsolidationReport, AppliedImprovement, SkippedImprovement structs
  - Include CustomPrompt field in Config for --prompt flag support
  - Requirements: [2.8](requirements.md#2.8), [3.5](requirements.md#3.5), [4.2](requirements.md#4.2)
  - References: internal/consolidation/types.go

- [x] 6. Implement ConsolidationLogger with file locking
  - Create internal/consolidation/logger.go with LogEntry struct including ImprovementsAttempted/Applied/Skipped fields
  - Implement flock-style locking for concurrent run safety
  - Implement Append with atomic write (temp file + rename)
  - Implement SaveReport for timestamped markdown files
  - Implement GetLatestCommitSHA for rollback support
  - Requirements: [6.1](requirements.md#6.1), [6.2](requirements.md#6.2), [6.3](requirements.md#6.3), [6.4](requirements.md#6.4), [6.5](requirements.md#6.5)
  - References: internal/consolidation/logger.go

- [x] 7. Write tests for ConsolidationLogger
  - Test append behavior, schema versioning, file locking for concurrent access
  - Test SaveReport with timestamped file creation
  - Test GetLatestCommitSHA with single and multiple entries
  - Requirements: [6.1](requirements.md#6.1), [6.2](requirements.md#6.2), [6.3](requirements.md#6.3), [6.4](requirements.md#6.4), [6.5](requirements.md#6.5)
  - References: internal/consolidation/logger_test.go

- [x] 8. Implement RecoveryManager
  - Create internal/consolidation/recovery.go with RecoveryManager struct
  - Implement CaptureState to record worktree state before agent runs
  - Implement CreateSnapshot for git stash when --allow-dirty
  - Implement RestoreOnFailure using git checkout -- . and git clean -fd
  - Implement RestoreStash with conflict handling (leave stash, warn user)
  - Implement Cleanup for post-success artifact removal
  - Requirements: [5.4](requirements.md#5.4), [5.5](requirements.md#5.5), [5.6](requirements.md#5.6)
  - References: internal/consolidation/recovery.go

- [x] 9. Write tests for RecoveryManager
  - Test stash/restore operations
  - Test stash conflict handling (leaves stash, warns user)
  - Test RestoreOnFailure with partial agent modifications
  - Requirements: [5.4](requirements.md#5.4), [5.5](requirements.md#5.5), [5.6](requirements.md#5.6)
  - References: internal/consolidation/recovery_test.go

- [x] 10. Implement PromptBuilder
  - Create internal/consolidation/prompt.go with PromptBuilder struct
  - Implement Build() to generate consolidation prompt with context, instructions, conflict resolution policy, scope constraints
  - Include conditional Custom Instructions section when customPrompt is provided
  - Add edge case handling guidance (binary files, renames, idempotency)
  - Requirements: [2.8](requirements.md#2.8), [3.1](requirements.md#3.1), [3.2](requirements.md#3.2), [3.3](requirements.md#3.3), [3.4](requirements.md#3.4), [4.2](requirements.md#4.2)
  - References: internal/consolidation/prompt.go

- [x] 11. Write tests for PromptBuilder
  - Test prompt construction with and without custom prompt
  - Test escaping of special characters in paths and names
  - Verify all required sections are included
  - Requirements: [2.8](requirements.md#2.8), [3.1](requirements.md#3.1), [3.2](requirements.md#3.2), [3.3](requirements.md#3.3), [3.4](requirements.md#3.4)
  - References: internal/consolidation/prompt_test.go

## Phase 3: Consolidator Core Implementation

- [x] 12. Implement Consolidator validation methods
  - Create internal/consolidation/consolidator.go with Consolidator struct
  - Implement validateVariant to check variant exists, list available if not
  - Implement validateReport to check comparison-report.md exists
  - Implement checkStaleness to compare report metadata against current variant HEADs
  - Implement checkEmptyImprovements for early exit when no improvements
  - Requirements: [2.3](requirements.md#2.3), [2.4](requirements.md#2.4), [2.9](requirements.md#2.9), [5.1](requirements.md#5.1), [5.2](requirements.md#5.2)
  - References: internal/consolidation/consolidator.go

- [x] 13. Write tests for Consolidator validation
  - Table-driven tests for variant not found, no markdown report, stale report, empty improvements
  - Test error messages include helpful suggestions
  - Requirements: [2.3](requirements.md#2.3), [2.4](requirements.md#2.4), [2.9](requirements.md#2.9), [5.1](requirements.md#5.1), [5.2](requirements.md#5.2)
  - References: internal/consolidation/consolidator_test.go

- [x] 14. Implement Consolidator.Run main workflow
  - Implement Run with spinner stages (Validating, Running agent, Running tests, Running post-command)
  - Call CaptureState before agent runs
  - Call CreateSnapshot when --allow-dirty
  - Run single agent session with built prompt
  - Check for SessionExporter interface and call ExportSession for agents like Kiro
  - Parse agent output for commit SHA and improvement counts
  - Run tests and post-command after agent completes
  - Log consolidation entry with results
  - Requirements: [2.5](requirements.md#2.5), [2.6](requirements.md#2.6), [2.7](requirements.md#2.7), [3.1](requirements.md#3.1), [3.5](requirements.md#3.5), [4.1](requirements.md#4.1), [4.3](requirements.md#4.3), [4.4](requirements.md#4.4), [4.6](requirements.md#4.6), [7.1](requirements.md#7.1)
  - References: internal/consolidation/consolidator.go

- [x] 15. Implement Consolidator error handling and recovery
  - Implement classifyError using existing agents.ErrorClassifier
  - Implement runWithRetry with exponential backoff for retryable errors
  - Call RestoreOnFailure when agent fails without committing
  - Implement signal handling for graceful interrupt
  - Requirements: [5.3](requirements.md#5.3), [5.6](requirements.md#5.6), [5.8](requirements.md#5.8), [5.9](requirements.md#5.9)
  - References: internal/consolidation/consolidator.go

- [x] 16. Implement Consolidator.Rollback
  - Check consolidation-log.json for stored commit SHA (primary mechanism)
  - Fall back to searching recent commits (git log -n 20) for message pattern
  - Validate commit exists and message matches pattern before reverting
  - Use git revert to undo the consolidation commit
  - Requirements: [5.7](requirements.md#5.7)
  - References: internal/consolidation/consolidator.go

- [x] 17. Write integration tests for Consolidator
  - TestConsolidateE2E with mock agent for full workflow
  - TestConsolidateRollback for rollback functionality
  - TestConsolidateEmptyImprovements for early exit
  - TestRecoveryPartialFailure for agent fails mid-execution
  - Requirements: [2.5](requirements.md#2.5), [2.6](requirements.md#2.6), [3.5](requirements.md#3.5), [4.1](requirements.md#4.1), [4.3](requirements.md#4.3), [4.4](requirements.md#4.4), [5.3](requirements.md#5.3), [5.6](requirements.md#5.6), [5.7](requirements.md#5.7)
  - References: internal/consolidation/consolidator_test.go

## Phase 4: CLI Command Integration

- [x] 18. Implement consolidate command
  - Create cmd/orbit/consolidate.go with consolidateCommand function
  - Add flags: --variant (required for consolidation), --allow-dirty, --prompt, --rollback
  - Implement spec auto-detection from git branch name when spec argument omitted
  - Wire up to main.go subcommand routing
  - Requirements: [2.1](requirements.md#2.1), [2.2](requirements.md#2.2), [2.7](requirements.md#2.7), [2.8](requirements.md#2.8)
  - References: cmd/orbit/consolidate.go, cmd/orbit/main.go

- [x] 19. Write CLI tests for consolidate command
  - Test flag parsing and validation
  - Test spec auto-detection from branch name
  - Test --rollback mode (does not require --variant)
  - Requirements: [2.1](requirements.md#2.1), [2.2](requirements.md#2.2), [2.7](requirements.md#2.7), [2.8](requirements.md#2.8)
  - References: cmd/orbit/consolidate_test.go
