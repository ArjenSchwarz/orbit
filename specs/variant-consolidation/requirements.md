# Variant Consolidation Requirements

## Introduction

This feature enhances Orbit's multi-variant comparison workflow by adding two capabilities:

1. **Markdown Report Export**: Comparison reports will be generated in both HTML and Markdown formats, making them easier for AI coding agents to consume and reason about.

2. **Consolidate Command**: A new `orbit consolidate <spec> --variant <id>` command that uses an AI agent to analyze improvements from non-chosen variants and apply them to the chosen variant. Unlike `finalize` (which simply adopts one variant), `consolidate` merges the best ideas from all variants into one.

---

## Requirements

### 1. Markdown Report Generation

**User Story:** As a developer using AI coding agents, I want comparison reports generated in Markdown format, so that agents can easily parse and reason about variant differences.

**Acceptance Criteria:**

1. <a name="1.1"></a>WHEN a comparison report is generated, the system SHALL create both an HTML file and a Markdown file in the output directory (using go-output v2 library for consistent multi-format rendering from single data structure)
2. <a name="1.2"></a>The Markdown report SHALL contain the same sections as the HTML report: recommendation, observations, file analyses, documentation assessment, cross-variant improvements, and diffs
3. <a name="1.3"></a>The Markdown report SHALL use standard GitHub-Flavored Markdown syntax for formatting (headers, code blocks, tables, lists)
4. <a name="1.4"></a>The Markdown file SHALL be named `comparison-report.md` alongside the existing `index.html`
5. <a name="1.5"></a>IF a section contains no data, the system SHALL omit that section from the Markdown output rather than showing empty placeholders
6. <a name="1.6"></a>The Markdown report SHALL include relative links to any separate diff files generated for large diffs
7. <a name="1.7"></a>The comparison report SHALL include metadata section with: generation timestamp, base commit SHA, and per-variant commit SHAs (for staleness detection by consolidate command)

### 2. Consolidate Command - Basic Operation

**User Story:** As a developer, I want to consolidate improvements from non-chosen variants into my chosen variant, so that I get the best implementation combining ideas from all variants.

**Acceptance Criteria:**

1. <a name="2.1"></a>The system SHALL provide an `orbit consolidate <spec> --variant <id>` command
2. <a name="2.2"></a>IF the spec argument is omitted, the system SHALL auto-detect the spec from the current git branch name (same logic as other orbit commands)
3. <a name="2.3"></a>The system SHALL validate that the specified variant ID exists in the variants metadata
4. <a name="2.4"></a>The system SHALL validate that a Markdown comparison report (`comparison-report.md`) exists before attempting consolidation
5. <a name="2.5"></a>The consolidate command SHALL work on variants in any state (not requiring finalization first)
6. <a name="2.6"></a>The system SHALL use the default agent (as configured in `.orbit.yaml` or command-line) for consolidation
7. <a name="2.7"></a>The system SHALL require a clean git state (no uncommitted changes) in the target worktree, or accept a `--allow-dirty` flag to proceed anyway
8. <a name="2.8"></a>The system SHALL accept an optional `--prompt` flag with custom instructions that influence which improvements are applied and how
9. <a name="2.9"></a>IF the comparison report was generated from different commit SHAs than the current variant HEADs, the system SHALL display a warning about potential staleness

### 3. Consolidate Command - Agent Execution

**User Story:** As a developer, I want the AI agent to autonomously analyze and implement improvements, so that I get a consolidated result I can review and either keep or revert.

**Acceptance Criteria:**

1. <a name="3.1"></a>The system SHALL provide the consolidation agent with: the Markdown comparison report, the paths to all variant worktrees, and the chosen variant ID
2. <a name="3.2"></a>The system SHALL instruct the agent to use the "cross-variant improvements" section to identify changes to adopt from non-chosen variants
3. <a name="3.3"></a>The agent SHALL analyze each improvement by examining the source variant's code, understanding the approach, and determining how to implement it in the chosen variant (adapting rather than copying directly)
4. <a name="3.4"></a>The agent SHALL autonomously decide which improvements to apply based on feasibility and value
5. <a name="3.5"></a>AFTER completing the consolidation, the agent SHALL produce a report showing: improvements applied (with source variant), improvements skipped (with reason), and the commit SHA

### 4. Consolidate Command - Commit and Validation

**User Story:** As a developer, I want improvements committed and validated, so that I can review the result and decide whether to keep or revert it.

**Acceptance Criteria:**

1. <a name="4.1"></a>IF the chosen variant has an active worktree, the system SHALL apply changes to that worktree; IF no worktree exists but a finalized branch does, the system SHALL check out that branch before applying; IF neither exists, the system SHALL display an error
2. <a name="4.2"></a>The agent SHALL commit all applied improvements as a single commit with the message format: `feat(consolidate): Apply improvements from variants X, Y to variant Z for <spec>`
3. <a name="4.3"></a>AFTER the agent completes, the system SHALL run the project's test suite and the configured post-run command (if any) to validate changes
4. <a name="4.4"></a>IF tests fail, the system SHALL report the failures and leave the commit in place for user review (user can run `--rollback` if desired)
5. <a name="4.5"></a>The system SHALL NOT automatically clean up non-chosen variant worktrees after consolidation
6. <a name="4.6"></a>The system SHALL create a consolidation log in the `.orbit` directory recording what was applied

### 5. Consolidate Command - Error Handling

**User Story:** As a developer, I want clear feedback when consolidation fails, so that I can understand what went wrong and recover gracefully.

**Acceptance Criteria:**

1. <a name="5.1"></a>IF the specified variant does not exist, the system SHALL display an error listing available variants
2. <a name="5.2"></a>IF no Markdown comparison report exists, the system SHALL display an error and offer to run `orbit compare` for the user
3. <a name="5.3"></a>IF the agent encounters an error during analysis, the system SHALL display the error and exit without modifying files
4. <a name="5.4"></a>BEFORE running the agent, the system SHALL capture the worktree state to enable recovery
5. <a name="5.5"></a>IF the worktree has uncommitted changes (when using `--allow-dirty`), the system SHALL create a git stash as a recovery snapshot
6. <a name="5.6"></a>IF the agent fails or is interrupted without creating a commit, the system SHALL restore the worktree to the pre-session state
7. <a name="5.7"></a>The system SHALL provide a `--rollback` flag that reverts the consolidation commit by: (1) checking the consolidation log for the commit SHA, or (2) searching recent commits for the message pattern as fallback
8. <a name="5.8"></a>The system SHALL classify agent errors using the existing error classification system (retryable, fatal, session-invalid)
9. <a name="5.9"></a>WHEN a retryable error occurs, the system SHALL apply exponential backoff retry logic consistent with other orbit commands

### 6. Consolidation Logging and Tracking

**User Story:** As a developer, I want consolidation activity logged, so that I can review what was changed and troubleshoot issues.

**Acceptance Criteria:**

1. <a name="6.1"></a>The system SHALL create a `consolidation-log.json` file in the spec's `.orbit` directory with a `schema_version` field for future compatibility
2. <a name="6.2"></a>The consolidation log SHALL record: schema_version, timestamp, chosen variant ID, consolidation commit SHA, agent used, improvements attempted, improvements applied, improvements failed
3. <a name="6.3"></a>The system SHALL save the agent's session transcript in the `.orbit` directory (same pattern as phase logs)
4. <a name="6.4"></a>The system SHALL save the agent's consolidation report to a timestamped markdown file in the `.orbit` directory
5. <a name="6.5"></a>IF consolidation is run multiple times, the system SHALL append to the consolidation log with a new entry

### 7. Progress Indication

**User Story:** As a developer, I want to see progress during consolidation, so that I know the system is working and what stage it's in.

**Acceptance Criteria:**

1. <a name="7.1"></a>The system SHALL display stage indicators with a spinner during consolidation: "Validating prerequisites...", "Running consolidation agent...", "Running tests...", "Running post-command..."
