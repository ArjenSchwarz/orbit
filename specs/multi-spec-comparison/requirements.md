# Multi-Spec Comparison Requirements

## Introduction

This feature enables Orbit to run multiple implementations of the same specification in parallel (or sequentially), each in isolated git worktrees. After all implementations complete, Orbit compares the results and generates an HTML report with a recommendation on which implementation is best.

The feature leverages per-variant guidance to explore different solution approaches for the same specification, allowing users to select the best approach based on code quality, test results, and comparative analysis.

**Key capabilities:**
- Run N variants of a spec implementation in isolated git worktrees
- Execute variants sequentially or in parallel
- Provide per-variant guidance to steer implementations toward different approaches
- Generate comparison reports with diffs, metrics, and recommendations
- Provide commands to adopt, compare, clean up, or check status of variants

---

## Requirements

### 1. Variant Configuration

**User Story:** As a developer, I want to configure how many implementation variants to run and how they execute, so that I can explore multiple solutions without manual setup.

**Acceptance Criteria:**

1. <a name="1.1"></a>WHERE `--variants N` flag is provided, the system SHALL create N isolated implementations of the spec
2. <a name="1.2"></a>WHERE `--variants` is not provided, the system SHALL execute in single-run mode with no worktrees (backwards compatible)
3. <a name="1.3"></a>WHERE `--parallel` flag is provided, the system SHALL execute variants concurrently
4. <a name="1.4"></a>WHERE `--parallel` is not provided, the system SHALL execute variants sequentially
5. <a name="1.5"></a>The system SHALL support a maximum of 3 parallel variants by default
6. <a name="1.6"></a>The system SHALL allow the parallel limit to be configured via `--max-parallel N` flag
7. <a name="1.7"></a>The system SHALL support variant configuration via `.orbit.yaml` config file
8. <a name="1.8"></a>The system SHALL allow `--branch-prefix PREFIX` to customize branch naming (default: `orbit-impl`)
9. <a name="1.9"></a>The system SHALL support per-variant guidance via `--guidance-file FILE` using the following YAML schema:
   ```yaml
   variants:
     - id: 1
       guidance: "Prioritize simplicity and maintainability"
     - id: 2
       guidance: "Optimize for performance"
   global_guidance: "Follow existing codebase patterns"  # optional, applied to all
   ```
10. <a name="1.10"></a>WHERE guidance file is provided, the system SHALL validate it against the schema and fail with a descriptive error if invalid

---

### 2. Git Worktree Management

**User Story:** As a developer, I want each variant to run in an isolated git worktree, so that implementations don't interfere with each other or my working directory.

**Acceptance Criteria:**

1. <a name="2.1"></a>The system SHALL create worktrees in `specs/{spec-name}/.orbit/worktrees/{prefix}-{N}-{spec-name}/`
2. <a name="2.2"></a>The system SHALL create branches with pattern `{prefix}-{N}/{spec-name}` from current HEAD
3. <a name="2.3"></a>WHERE worktrees exist from a previous run AND base commit matches current HEAD, the system SHALL reuse existing worktrees
4. <a name="2.4"></a>WHERE worktrees exist from a previous run AND base commit differs, the system SHALL fail with an error message suggesting cleanup
5. <a name="2.5"></a>The system SHALL store worktree metadata in `specs/{spec-name}/.orbit/variants.json` with the following schema:
   ```json
   {
     "runId": "uuid",
     "baseCommit": "sha",
     "originalBranch": "branch-name",
     "startedAt": "ISO8601",
     "variants": [
       {
         "id": 1,
         "branch": "orbit-impl-1/spec-name",
         "worktreePath": ".orbit/worktrees/orbit-impl-1-spec-name",
         "status": "pending|running|completed|failed",
         "error": "optional error message"
       }
     ]
   }
   ```
6. <a name="2.6"></a>The system SHALL validate that the `.orbit/worktrees/` directory is writable before creating worktrees
7. <a name="2.7"></a>WHERE worktree creation fails for any variant, the system SHALL clean up all created worktrees and fail with a descriptive error
8. <a name="2.8"></a>The system SHALL sanitize spec names for filesystem safety (replace slashes, spaces, special characters)
9. <a name="2.9"></a>The system SHALL create or update `.orbit/.gitignore` to include `worktrees/` before creating worktrees

---

### 3. Variant Execution

**User Story:** As a developer, I want variants to execute reliably with proper error handling, so that I get useful results even when some variants fail.

**Acceptance Criteria:**

1. <a name="3.1"></a>The system SHALL run all spec phases (as defined in tasks.md) within each variant's worktree
2. <a name="3.2"></a>The system SHALL save logs to variant-specific subdirectories: `specs/{spec-name}/.orbit/variant-{N}/`
3. <a name="3.3"></a>WHERE a variant fails, the system SHALL continue execution of remaining variants
4. <a name="3.4"></a>The system SHALL track variant status in variants.json as: pending, running, completed, or failed
5. <a name="3.5"></a>WHERE per-variant guidance is configured, the system SHALL include the guidance in the initial prompt for that variant
6. <a name="3.6"></a>The system SHALL capture cost, duration, and API turn count for each variant
7. <a name="3.7"></a>WHERE all variants fail, the system SHALL generate a partial report showing failure information

---

### 4. Parallel Execution

**User Story:** As a developer, I want to run variants in parallel to save time, so that I can compare multiple approaches efficiently.

**Acceptance Criteria:**

1. <a name="4.1"></a>WHERE parallel mode is enabled, the system SHALL spawn concurrent goroutines for each variant (up to max-parallel limit)
2. <a name="4.2"></a>The system SHALL use a semaphore to enforce the max-parallel limit
3. <a name="4.3"></a>Each variant SHALL handle rate limits independently using existing Orbit retry logic
4. <a name="4.4"></a>The system SHALL log variant status changes (started, completed, failed, retrying) for debugging

---

### 5. Comparison

**User Story:** As a developer, I want an automated comparison of all variant implementations, so that I can make an informed decision about which approach to adopt.

**Acceptance Criteria:**

1. <a name="5.1"></a>The system SHALL gather git diffs from base commit for each successful variant
2. <a name="5.2"></a>The system SHALL use Claude to perform comparison analysis by default
3. <a name="5.3"></a>The system SHALL allow custom comparison via `--compare-command CMD` flag
4. <a name="5.4"></a>The comparison output SHALL include: recommendation (variant number), confidence level (high/medium/low), executive summary, and per-file analysis
5. <a name="5.5"></a>The system SHALL send unified diffs to the comparison model showing changes each variant made from the base commit
6. <a name="5.6"></a>WHERE test results exist (test output captured in logs), the comparison SHALL include test pass/fail breakdown per variant
7. <a name="5.7"></a>The comparison model SHALL be configurable via `.orbit.yaml` (default: Claude)
8. <a name="5.8"></a>WHERE combined diff content exceeds the model's context limit, the system SHALL fail with an error indicating the variants are too large to compare

---

### 6. Report Generation

**User Story:** As a developer, I want a readable HTML report summarizing the comparison, so that I can review results and share them with my team.

**Acceptance Criteria:**

1. <a name="6.1"></a>The system SHALL generate an HTML report at `specs/{spec-name}/comparison-report/index.html`
2. <a name="6.2"></a>The report SHALL include: executive summary with recommendation, overview table with per-variant metrics, syntax-highlighted file diffs, test results (if available), and detailed analysis
3. <a name="6.3"></a>The report SHALL display cost, duration, and API turn metrics for each variant
4. <a name="6.4"></a>The report SHALL use collapsible sections for large diffs
5. <a name="6.5"></a>The main index.html SHALL be self-contained with embedded CSS for core content
6. <a name="6.6"></a>WHERE individual file diffs exceed 500 lines, the system MAY store them as separate HTML files in `comparison-report/diffs/` and link from the main report
7. <a name="6.7"></a>The report SHALL be responsive and print-friendly
8. <a name="6.8"></a>WHERE a variant failed, the report SHALL include failure information and error messages
9. <a name="6.9"></a>The report SHALL escape all injected content to prevent HTML/script injection

---

### 7. Status Command

**User Story:** As a developer, I want to check the status of variant runs, so that I can see which variants exist and their current state.

**Acceptance Criteria:**

1. <a name="7.1"></a>The system SHALL provide an `orbit status <spec-name>` command
2. <a name="7.2"></a>The status command SHALL display a table showing: variant ID, branch name, worktree path, and status for each variant
3. <a name="7.3"></a>The status command SHALL show the base commit and original branch from metadata
4. <a name="7.4"></a>WHERE no variants exist for the spec, the system SHALL indicate that no variant run is in progress

---

### 8. Cleanup Command

**User Story:** As a developer, I want to remove variant worktrees and branches when I no longer need them, so that I can keep my workspace clean.

**Acceptance Criteria:**

1. <a name="8.1"></a>The system SHALL provide an `orbit cleanup <spec-name>` command
2. <a name="8.2"></a>The cleanup command SHALL remove all worktrees for the specified spec
3. <a name="8.3"></a>The cleanup command SHALL delete only branches recorded in variants.json (not by pattern matching)
4. <a name="8.4"></a>The cleanup command SHALL NOT delete remote branches
5. <a name="8.5"></a>WHERE `--keep N` flag is provided, the system SHALL preserve variant N and remove all others
6. <a name="8.6"></a>The cleanup command SHALL confirm before deleting (unless `--force` is provided)
7. <a name="8.7"></a>The cleanup command SHALL support `--dry-run` to show what would be deleted without deleting
8. <a name="8.8"></a>The cleanup command SHALL remove the variants.json file after successful cleanup (unless `--keep` is used)

---

### 9. Finalize Command

**User Story:** As a developer, I want to adopt a chosen variant as my final implementation, so that I can merge it into my working branch and clean up the others.

**Acceptance Criteria:**

1. <a name="9.1"></a>The system SHALL provide an `orbit finalize <spec-name> --variant N` command
2. <a name="9.2"></a>The finalize command SHALL verify the original branch (from variants.json) has not diverged from the base commit
3. <a name="9.3"></a>WHERE the original branch has new commits since the run started, the system SHALL fail with an error explaining the divergence
4. <a name="9.4"></a>The finalize command SHALL rebase changes from the chosen variant branch onto the original branch
5. <a name="9.5"></a>The finalize command SHALL delete all worktrees for the spec after successful rebase
6. <a name="9.6"></a>The finalize command SHALL delete all local variant branches after successful rebase
7. <a name="9.7"></a>The finalize command SHALL NOT delete remote branches
8. <a name="9.8"></a>WHERE rebase conflicts occur, the system SHALL pause and provide instructions for manual resolution
9. <a name="9.9"></a>The finalize command SHALL confirm before proceeding (unless `--force` is provided)

---

### 10. Compare Command

**User Story:** As a developer, I want to re-run the comparison on existing variants, so that I can regenerate the report after making manual modifications.

**Acceptance Criteria:**

1. <a name="10.1"></a>The system SHALL provide an `orbit compare <spec-name>` command
2. <a name="10.2"></a>The compare command SHALL work with existing variant worktrees
3. <a name="10.3"></a>The compare command SHALL regenerate the HTML comparison report
4. <a name="10.4"></a>WHERE no variants exist, the system SHALL fail with a descriptive error message

---

### 11. Interrupt Handling

**User Story:** As a developer, I want graceful handling when I cancel a variant run, so that my workspace is left in a clean state.

**Acceptance Criteria:**

1. <a name="11.1"></a>WHERE SIGINT or SIGTERM is received, the system SHALL stop scheduling new variant phases
2. <a name="11.2"></a>The system SHALL allow currently running phases to complete (with a 30-second timeout)
3. <a name="11.3"></a>The system SHALL update variants.json with final status for each variant (completed, failed, or canceled)
4. <a name="11.4"></a>The system SHALL preserve all worktrees on interrupt (no automatic cleanup)
5. <a name="11.5"></a>The system SHALL log which variants were interrupted and how to resume or cleanup

---

## Non-Functional Requirements

### 12. Performance

**User Story:** As a developer, I want variant execution to be efficient, so that running multiple variants doesn't significantly increase my wait time.

**Acceptance Criteria:**

1. <a name="12.1"></a>The system SHALL create worktrees using `git worktree add` (O(1) operation, not full clone)
2. <a name="12.2"></a>The system SHALL avoid redundant git operations when worktrees already exist
3. <a name="12.3"></a>The report generation SHALL complete within 10 seconds for typical comparisons (3 variants, <50 changed files)

---

### 13. Error Handling

**User Story:** As a developer, I want clear error messages when things go wrong, so that I can understand and fix issues quickly.

**Acceptance Criteria:**

1. <a name="13.1"></a>WHERE git worktree operations fail, the system SHALL provide the underlying git error message
2. <a name="13.2"></a>WHERE comparison fails, the system SHALL preserve variant worktrees for manual inspection
3. <a name="13.3"></a>The system SHALL log all variant status changes for debugging
4. <a name="13.4"></a>WHERE a partial failure occurs (some variants succeed, some fail), the system SHALL clearly indicate which variants failed and why

---

## Out of Scope

The following are explicitly out of scope for this feature:

1. **Multi-agent comparison** - Running different AI agents (Claude, Codex, Gemini) is deferred to a future release
2. **Remote branch operations** - Pushing to or deleting from remote repositories
3. **Alternative report formats** - Markdown, JSON, or PDF output (HTML only for now)
4. **Code complexity metrics** - Cyclomatic complexity, security scanning, etc.
5. **Performance benchmarking** - Automated performance comparison between variants
6. **Screenshot capture** - UI screenshot comparison is deferred to a future release
7. **Cost estimation/budgets** - Pre-run cost estimation and budget limits are deferred to a future release
8. **Cross-variant rate limit coordination** - Complex rate limit sharing between parallel variants (each handles independently)
