# Requirements: Enhanced Status Command

## Introduction

This feature enhances the `orbit status` command to provide detailed real-time visibility into active variant implementations. Currently, the status command shows a simple table with variant ID, branch, path, and status. This enhancement adds four new information sections for variants with status "running" or "failed":

1. **Recent Commits**: Last 3 commits made by the agent in each variant's worktree
2. **Git State**: Whether the worktree has uncommitted changes (clean/dirty indicator)
3. **Last Action Summary**: The most recent agent action parsed from the live transcript (Claude Code only)
4. **Task Progress**: Phase-by-phase breakdown of completed vs total tasks

These additions enable users to monitor implementation progress without interrupting the running agents or navigating to individual worktrees. For failed variants, this information helps diagnose what went wrong.

---

## Definitions

- **Base Commit**: The git commit SHA stored in the `base_commit` field of the variants.json metadata file. This is the commit from which all variant branches diverged.
- **Spec Name**: The name of the spec directory, derived from the variant's worktree path. For a worktree at `specs/enhanced-status/.orbit/worktrees/orbit-impl-1-enhanced-status`, the spec name is `enhanced-status`.
- **Active Variant**: A variant with status "running" or "failed".

---

## Requirements

### 1. Recent Commits Display

**User Story:** As an orbit user monitoring variant implementations, I want to see the recent commits in each active variant, so that I can understand what the agent has accomplished and track implementation progress.

**Acceptance Criteria:**

1. <a name="1.1"></a>WHEN displaying status for an active variant, the system SHALL show the 3 most recent commits since the base commit
2. <a name="1.2"></a>Each commit SHALL display the short hash (7 characters) and commit message subject line in the format `{hash} {subject}`
3. <a name="1.3"></a>Commits SHALL be ordered with the most recent commit first
4. <a name="1.4"></a>IF an active variant has no commits since the base commit, THEN the system SHALL display "No commits yet"
5. <a name="1.5"></a>The system SHALL NOT display commit information for variants with status "pending", "completed", or "canceled"

### 2. Git Dirty State Indicator

**User Story:** As an orbit user, I want to see whether an active variant has uncommitted changes, so that I can understand if the agent is in the middle of making changes.

**Acceptance Criteria:**

1. <a name="2.1"></a>WHEN displaying status for an active variant, the system SHALL show the git working tree state
2. <a name="2.2"></a>IF the worktree has staged or unstaged changes to tracked files, THEN the system SHALL display "dirty"
3. <a name="2.3"></a>IF the worktree has no staged or unstaged changes to tracked files, THEN the system SHALL display "clean"
4. <a name="2.4"></a>Untracked files SHALL NOT be considered when determining dirty state
5. <a name="2.5"></a>The dirty/clean indicator SHALL be displayed in parentheses after the status in the variant header, e.g., "running (dirty)" or "failed (clean)"
6. <a name="2.6"></a>IF git operations fail (including worktree path not existing), THEN the status SHALL be displayed without a dirty/clean indicator
7. <a name="2.7"></a>The system SHALL NOT display git state for variants with status "pending", "completed", or "canceled"

### 3. Last Action Summary (Claude Code Only)

**User Story:** As an orbit user, I want to see the last action the agent performed, so that I can understand what it's currently working on without viewing the full transcript.

**Acceptance Criteria:**

1. <a name="3.1"></a>The Last Action feature SHALL only be available for variants using the Claude Code agent
2. <a name="3.2"></a>WHEN displaying status for an active Claude Code variant, the system SHALL locate the live transcript file at Claude's session storage location (`~/.claude/projects/{project-hash}/{session-id}.jsonl`) using the session ID from the variant's summary.json
3. <a name="3.3"></a>The system SHALL use the existing claude.BuildProjectPath function to resolve the project hash from the variant's worktree path
4. <a name="3.4"></a>The system SHALL identify and display the most recent displayable entry from the transcript, defined as:
   - Entries with assistant role containing tool_use content items
   - Entries with assistant role containing text content items
   - Excluding entries with `isMeta: true`
   - Excluding thinking content items
5. <a name="3.5"></a>IF an assistant entry contains both tool_use and text content items, THEN tool_use SHALL take precedence for display
6. <a name="3.6"></a>For tool_use entries, the system SHALL display using the format `{ToolName}: {key_input}` where key_input is the value of the first matching parameter in priority order: file_path, path, command, pattern, query, url, prompt; if none match, use the first parameter value; truncate to 60 characters
7. <a name="3.7"></a>For text entries from the assistant, the system SHALL display the first 80 characters followed by "..." if truncated
8. <a name="3.8"></a>IF no session ID is available or the transcript file does not exist, THEN the system SHALL display "Waiting for activity..."
9. <a name="3.9"></a>IF the transcript file cannot be read or parsed, THEN the system SHALL display "Transcript unavailable" and continue without error
10. <a name="3.10"></a>IF the transcript file has an incomplete JSON line at the end (agent actively writing), the system SHALL skip that line and use the last complete entry
11. <a name="3.11"></a>The system SHALL NOT display action summary for variants with status "pending", "completed", or "canceled"
12. <a name="3.12"></a>For non-Claude agents (Codex, Copilot), the system SHALL display "Last action tracking not available for {agent_type}"
13. <a name="3.13"></a>The system SHOULD read transcript files efficiently, preferring to read from the end of the file rather than parsing the entire file when possible

### 4. Task Progress Overview

**User Story:** As an orbit user, I want to see task completion progress for each active variant, so that I can gauge how far along the implementation is and estimate remaining work.

**Acceptance Criteria:**

1. <a name="4.1"></a>WHEN displaying status for an active variant, the system SHALL display task progress for all phases
2. <a name="4.2"></a>Each phase SHALL show: phase name, completed task count, and total task count
3. <a name="4.3"></a>The progress display SHALL use the format `{phase_name}: {completed}/{total}` for each phase
4. <a name="4.4"></a>The system SHALL read task data from `{worktree_path}/specs/{spec_name}/tasks.md` using a rune client instance
5. <a name="4.5"></a>IF the tasks file cannot be read, rune CLI is unavailable, or rune command fails, THEN the system SHALL display "Task progress unavailable" and continue without error
6. <a name="4.6"></a>The system SHALL NOT display task progress for variants with status "pending", "completed", or "canceled"
7. <a name="4.7"></a>The current phase (with status "in progress") SHALL be prefixed with `→ ` to indicate it is active

### 5. Output Format and Organization

**User Story:** As an orbit user, I want the enhanced status output to be well-organized and scannable, so that I can quickly understand the state of all active variants.

**Acceptance Criteria:**

1. <a name="5.1"></a>The system SHALL replace the existing simple table format with a new enhanced format
2. <a name="5.2"></a>The system SHALL use the go-output library to enable multiple output formats (table, JSON, markdown) in the future
3. <a name="5.3"></a>Each active variant SHALL be displayed in its own section
4. <a name="5.4"></a>Variant sections SHALL be separated by a blank line followed by a horizontal rule using three dashes (`---`)
5. <a name="5.5"></a>The variant section header SHALL use the format: `Variant {ID}: {branch} [{status}]` or `Variant {ID}: {branch} [{status} ({git_state})]` when git state is available
6. <a name="5.6"></a>Within each variant section, the subsections SHALL appear in this order: Commits, Last Action, Task Progress
7. <a name="5.7"></a>Each subsection SHALL have a label on its own line (e.g., "Commits:", "Last Action:", "Tasks:")
8. <a name="5.8"></a>Variants with status "pending", "completed", or "canceled" SHALL be listed in a compact summary section at the end
9. <a name="5.9"></a>The compact summary SHALL display as a simple list: `Variant {ID}: {branch} [{status}]`
10. <a name="5.10"></a>IF there are no active variants, THEN the system SHALL display only the compact summary with a note: "No active variants"

### 6. Error Handling and Resilience

**User Story:** As an orbit user, I want the status command to handle errors gracefully, so that partial information is still shown even when some data sources fail.

**Acceptance Criteria:**

1. <a name="6.1"></a>IF git operations fail for a variant (including worktree not existing), THEN the system SHALL omit the git state indicator from the header and display "Git info unavailable" in the Commits section
2. <a name="6.2"></a>IF transcript parsing fails for a variant, THEN the system SHALL display "Transcript unavailable" in the Last Action section and continue
3. <a name="6.3"></a>IF task progress retrieval fails for a variant, THEN the system SHALL display "Task progress unavailable" in the Tasks section and continue
4. <a name="6.4"></a>The system SHALL NOT exit with a non-zero exit code due to individual variant data retrieval failures
5. <a name="6.5"></a>The system SHALL exit with code 0 if variants.json loads successfully, regardless of individual variant data failures
6. <a name="6.6"></a>The system SHALL exit with code 1 if variants.json cannot be loaded or does not exist
