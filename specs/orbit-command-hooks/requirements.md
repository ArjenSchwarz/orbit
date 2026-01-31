# Requirements: Orbit Command Hooks

## Introduction

This feature introduces a clearer separation between shell commands and AI prompts in Orbit's configuration, along with agent-level command hooks. The changes include:

1. **Renaming**: The current `post-command` (which is actually an AI prompt) is renamed to `post-prompt` for clarity
2. **New global pre-prompt**: A new `pre-prompt` option that runs before any phase and starts the session that phase 1 continues
3. **Agent-level commands**: Per-agent `pre-command` and `post-command` options that execute shell commands before the first phase and after the last phase
4. **Migration enforcement**: Detection and rejection of deprecated `post-command` configuration to enforce explicit migration

---

## Requirements

### 1. Rename Post-Command to Post-Prompt

**User Story:** As an Orbit user, I want the existing post-command configuration to be renamed to post-prompt, so that the naming clearly reflects that it's an AI prompt rather than a shell command.

**Acceptance Criteria:**

1. <a name="1.1"></a>The system SHALL accept `post-prompt` as the configuration key in `.orbit.yaml` files
2. <a name="1.2"></a>The system SHALL accept `--post-prompt` as the CLI flag (replacing `--post-command`)
3. <a name="1.3"></a>The system SHALL accept `ORBIT_POST_PROMPT` as the environment variable (replacing `ORBIT_POST_COMMAND`)
4. <a name="1.4"></a>The system SHALL accept `--no-post-prompt` as the CLI flag to disable the post-prompt (replacing `--no-post-command`)
5. <a name="1.5"></a>The system SHALL use the same default prompt text currently used for post-command
6. <a name="1.6"></a>The system SHALL execute the post-prompt after all phases complete, using the same logic as the current post-command

### 2. Add Global Pre-Prompt

**User Story:** As an Orbit user, I want to configure a pre-prompt that runs before any phase starts, so that I can set up context or perform initial checks with the AI agent before implementation begins.

**Acceptance Criteria:**

1. <a name="2.1"></a>The system SHALL accept `pre-prompt` as a configuration key in `.orbit.yaml` files
2. <a name="2.2"></a>The system SHALL accept `--pre-prompt` as a CLI flag to specify the pre-prompt text
3. <a name="2.3"></a>The system SHALL accept `ORBIT_PRE_PROMPT` as an environment variable
4. <a name="2.4"></a>The system SHALL accept `--no-pre-prompt` as a CLI flag to explicitly disable the pre-prompt
5. <a name="2.5"></a>The system SHALL execute the pre-prompt before the first phase begins
6. <a name="2.6"></a>The system SHALL start a new agent session for the pre-prompt execution
7. <a name="2.7"></a>The system SHALL pass the session ID from the pre-prompt execution to phase 1, so that phase 1 continues the same session
8. <a name="2.8"></a>The system SHALL NOT have a default pre-prompt (empty by default)
9. <a name="2.9"></a>IF the pre-prompt execution fails, THEN the system SHALL abort the run with an error
10. <a name="2.10"></a>IF the agent's Resume() call fails for the pre-prompt session (returns SessionInvalid error or session not found), THEN the system SHALL start a fresh session for phase 1 and log a warning
11. <a name="2.11"></a>The system SHALL track pre-prompt state in summary.json including: session_id, started_at, and completed_at
12. <a name="2.12"></a>IF resuming an interrupted run where pre-prompt already completed, THEN the system SHALL NOT re-run the pre-prompt and SHALL use the stored session_id
13. <a name="2.13"></a>IF `--continue-session <session_id>` is provided with `--pre-prompt`, THEN the pre-prompt SHALL use that existing session instead of starting a new one
14. <a name="2.14"></a>The system SHALL execute pre-prompt with the working directory set to the repository root (or worktree root in variant mode)

### 3. Add Agent-Level Pre-Command

**User Story:** As an Orbit user, I want to configure shell commands that run before an agent starts, so that I can prepare the environment (e.g., run linters, update dependencies) before the AI agent begins work.

**Acceptance Criteria:**

1. <a name="3.1"></a>The system SHALL accept `pre-command` as a configuration key under `agents.<agent-name>` in `.orbit.yaml` files
2. <a name="3.2"></a>The system SHALL execute the pre-command as a shell command on the host system
3. <a name="3.3"></a>The system SHALL execute the pre-command once before the first phase starts (per run, not per phase)
4. <a name="3.4"></a>The system SHALL execute the pre-command before the global pre-prompt (if configured)
5. <a name="3.5"></a>IF the pre-command exits with a non-zero exit code, THEN the system SHALL abort the run with an error message including the exit code and command output
6. <a name="3.6"></a>The system SHALL NOT have a default pre-command (empty by default)
7. <a name="3.7"></a>The pre-command configuration SHALL be agent-specific; there is no global pre-command that agents inherit
8. <a name="3.8"></a>An empty string value for pre-command SHALL be treated as not configured (no-op)

### 4. Add Agent-Level Post-Command

**User Story:** As an Orbit user, I want to configure shell commands that run after an agent completes, so that I can perform cleanup or validation (e.g., run tests, format code) after the AI agent finishes work.

**Acceptance Criteria:**

1. <a name="4.1"></a>The system SHALL accept `post-command` as a configuration key under `agents.<agent-name>` in `.orbit.yaml` files
2. <a name="4.2"></a>The system SHALL execute the post-command as a shell command on the host system
3. <a name="4.3"></a>The system SHALL execute the post-command once after the last phase completes (per run, not per phase)
4. <a name="4.4"></a>The system SHALL execute the post-command after the global post-prompt (if configured)
5. <a name="4.5"></a>IF the post-command exits with a non-zero exit code, THEN the system SHALL report the failure but mark the run as completed with warnings
6. <a name="4.6"></a>The system SHALL NOT have a default post-command (empty by default)
7. <a name="4.7"></a>The post-command configuration SHALL be agent-specific; there is no global post-command that agents inherit
8. <a name="4.8"></a>An empty string value for post-command SHALL be treated as not configured (no-op)

### 5. Deprecation Detection and Migration Enforcement

**User Story:** As an Orbit user, I want clear error messages when I have deprecated configuration, so that I know exactly what to update and the system doesn't silently misbehave.

**Acceptance Criteria:**

1. <a name="5.1"></a>IF the system detects `post-command` as a top-level key in any `.orbit.yaml` file, THEN the system SHALL exit with an error before running
2. <a name="5.2"></a>IF the system detects `ORBIT_POST_COMMAND` environment variable is set, THEN the system SHALL exit with an error before running
3. <a name="5.3"></a>IF the system detects `--post-command` CLI flag is used, THEN the system SHALL exit with an error before running
4. <a name="5.4"></a>The deprecation error message SHALL clearly state: the deprecated configuration name, the new configuration name to use, and instructions for updating
5. <a name="5.5"></a>The system SHALL check for deprecated configuration before any other processing
6. <a name="5.6"></a>The system SHALL distinguish between deprecated top-level `post-command` (error) and valid agent-level `post-command` (allowed) by checking the YAML structure depth

### 6. Execution Order

**User Story:** As an Orbit user, I want a predictable execution order for all hooks and prompts, so that I can reason about when my commands and prompts will run.

**Acceptance Criteria:**

1. <a name="6.1"></a>The system SHALL execute hooks and prompts in the following order:
   1. Agent pre-command (shell)
   2. Global pre-prompt (AI)
   3. Phase loop (phases 1 through N)
   4. Global post-prompt (AI)
   5. Agent post-command (shell)
2. <a name="6.2"></a>The system SHALL skip any hook or prompt that is not configured
3. <a name="6.3"></a>The system SHALL apply the same execution order in both single-run mode and variant mode
4. <a name="6.4"></a>In variant mode, each variant SHALL execute its own complete hook sequence independently in its own worktree
5. <a name="6.5"></a>In parallel variant mode, pre-commands and post-commands MAY run concurrently across variants since each variant operates in its own isolated worktree

### 7. Shell Command Execution Environment

**User Story:** As an Orbit user, I want predictable shell command execution, so that my pre-commands and post-commands behave consistently.

**Acceptance Criteria:**

1. <a name="7.1"></a>The system SHALL execute shell commands using `/bin/sh -c "<command>"`
2. <a name="7.2"></a>The system SHALL set the working directory to the repository root (or worktree root in variant mode) before executing shell commands
3. <a name="7.3"></a>The system SHALL inherit the current process environment variables when executing shell commands
4. <a name="7.4"></a>The system SHALL set `ORBIT_PHASE_COUNT` environment variable to the total number of phases
5. <a name="7.5"></a>The system SHALL set `ORBIT_AGENT` environment variable to the agent name being used
6. <a name="7.6"></a>The system SHALL apply a configurable timeout to shell command execution, defaulting to 5 minutes
7. <a name="7.7"></a>The system SHALL accept `command-timeout` as a configuration key in `.orbit.yaml` files (duration string, e.g., "10m", "1h")
8. <a name="7.8"></a>The system SHALL accept `ORBIT_COMMAND_TIMEOUT` as an environment variable to override the timeout
9. <a name="7.9"></a>IF a shell command exceeds the timeout, THEN the system SHALL terminate the command and treat it as a failure
10. <a name="7.10"></a>The system SHALL capture both stdout and stderr from shell commands
11. <a name="7.11"></a>Shell commands MUST be non-interactive; commands that require user input will hang until timeout and fail
12. <a name="7.12"></a>IF `--dry-run` is enabled, THEN the system SHALL print the shell command that would be executed (including working directory) but SHALL NOT execute it

### 8. Shell Command Logging

**User Story:** As an Orbit user, I want shell command output captured in the centralized logs, so that I can debug failures and understand what happened alongside phase logs.

**Acceptance Criteria:**

1. <a name="8.1"></a>The system SHALL track pre-command and post-command execution in summary.json with: command, exit_code, started_at, completed_at, and duration_ms
2. <a name="8.2"></a>The system SHALL save pre-command output to `.orbit/pre-command-run-N.txt` following the existing run-numbered naming convention
3. <a name="8.3"></a>The system SHALL save post-command output to `.orbit/post-command-run-N.txt` following the existing run-numbered naming convention
4. <a name="8.4"></a>The log files SHALL include: the command executed, exit code, stdout, stderr, and execution duration
5. <a name="8.5"></a>In variant mode, each variant SHALL have its own command log files in its worktree's `.orbit/` directory
6. <a name="8.6"></a>The system SHALL include pre-command and post-command status in the run index (index.md and index.html)

### 9. Prompt Failure Handling

**User Story:** As an Orbit user, I want consistent failure handling for prompts, so that I understand what happens when AI interactions fail.

**Acceptance Criteria:**

1. <a name="9.1"></a>IF the pre-prompt execution fails, THEN the system SHALL abort the run with an error
2. <a name="9.2"></a>IF the post-prompt execution fails, THEN the system SHALL apply the existing retry logic (up to 5 attempts with exponential backoff)
3. <a name="9.3"></a>IF the post-prompt fails after all retry attempts, THEN the system SHALL mark the run as completed with warnings (phases succeeded, post-prompt failed)
4. <a name="9.4"></a>The system SHALL log all prompt failures with error details

### 10. Configuration Examples

**User Story:** As an Orbit user, I want clear documentation with examples, so that I can correctly configure the new hooks and prompts.

**Acceptance Criteria:**

1. <a name="10.1"></a>The CLAUDE.md file SHALL be updated with the new configuration options
2. <a name="10.2"></a>The documentation SHALL include an example `.orbit.yaml` showing all hook and prompt options
3. <a name="10.3"></a>The documentation SHALL explain the execution order
4. <a name="10.4"></a>The documentation SHALL explain the difference between commands (shell) and prompts (AI)
5. <a name="10.5"></a>The documentation SHALL note that shell commands must be non-interactive

---

## Example Configuration

```yaml
# Global prompts (AI agent interactions)
pre-prompt: "Review the codebase structure and identify potential areas of concern before we begin implementation."
post-prompt: "Review the implementation to verify it meets the requirements and all tests pass. If issues are found, fix them."

# Shell command timeout (default: 5m)
command-timeout: "15m"

# Agent configuration with shell commands
agents:
  claude-code:
    type: claude-code
    auto-approve: true
    pre-command: "make lint && make test-short"
    post-command: "make format && make lint"

  codex:
    type: codex
    pre-command: "npm run lint"
    post-command: "npm run format"
```

---

## Out of Scope

- Per-phase hooks (hooks run once per run, not per phase)
- Global shell commands that apply to all agents (agent-level only)
- Automatic migration of old config to new config (manual migration required)
- Per-command timeout configuration (global timeout only)
- Shell command retry logic (only prompts have retry)
- Windows support for shell commands (requires /bin/sh)
