# Custom Commands Feature Requirements

## Introduction

This feature adds configurable commands to Orbit, allowing users to customize the prompts sent to Claude during task orchestration. Currently, Orbit uses a hardcoded prompt (`Run /next-task --phase and when complete run /commit`) for all phase executions. This feature enables:

1. **Custom phase command** - Replace the default prompt with a user-defined command
2. **Post-completion command** - Execute an additional Claude session after all tasks complete successfully

Configuration follows a priority hierarchy: CLI flags override config files, project-level config (current working directory) overrides home-level config, and defaults apply when nothing is specified.

---

## Requirements

### 1. Configuration File Support

**User Story:** As an Orbit user, I want to configure commands in a YAML file, so that I don't have to specify them on every invocation.

**Acceptance Criteria:**

1. <a name="1.1"></a>The system SHALL load configuration from `.orbit.yaml` in the current working directory if present
2. <a name="1.2"></a>The system SHALL load configuration from `$HOME/.orbit.yaml` if present
3. <a name="1.3"></a>The system SHALL merge configurations where project-level values override home-level values for non-empty fields
4. <a name="1.4"></a>The system SHALL continue without error if no configuration files exist
5. <a name="1.5"></a>The system SHALL log a warning including the file path and parse error if a configuration file contains invalid YAML, then continue with remaining config sources
6. <a name="1.6"></a>The system SHALL load configuration once at startup; changes to config files during orchestration are not picked up

---

### 2. Custom Phase Command

**User Story:** As an Orbit user, I want to customize the prompt sent to Claude for each phase, so that I can include additional workflow steps like running agents or custom commands.

**Acceptance Criteria:**

1. <a name="2.1"></a>The system SHALL accept a `command` field in the configuration file
2. <a name="2.2"></a>The system SHALL accept a `--command` CLI flag that overrides the configuration file value
3. <a name="2.3"></a>The system SHALL use the configured command as the prompt for Claude during phase execution
4. <a name="2.4"></a>The system SHALL default to `"Run /next-task --phase and when complete run /commit"` when no command is configured

---

### 3. Post-Completion Command

**User Story:** As an Orbit user, I want to run a command after all tasks are complete, so that I can automatically perform verification or cleanup steps.

**Acceptance Criteria:**

1. <a name="3.1"></a>The system SHALL accept a `post-command` field in the configuration file
2. <a name="3.2"></a>The system SHALL accept a `--post-command` CLI flag that overrides the configuration file value
3. <a name="3.3"></a>The system SHALL execute the post-command as a separate Claude session after all tasks complete successfully
4. <a name="3.4"></a>The system SHALL NOT execute the post-command if orchestration fails with a non-retryable error
5. <a name="3.5"></a>The system SHALL default to `"Review the implementation to verify it meets the requirements and all tests pass. If issues are found, fix them."` when no post-command is configured
6. <a name="3.6"></a>The system SHALL log the start and completion of the post-command execution
7. <a name="3.7"></a>The system SHALL exit with a non-zero exit code if the post-command fails, clearly indicating in logs that orchestration succeeded but post-command failed
8. <a name="3.8"></a>The system SHALL accept a `--no-post-command` flag to skip post-command execution entirely
9. <a name="3.9"></a>The system SHALL treat `post-command: ""` (empty string) in configuration as explicitly disabled, not as "not set"

---

### 4. Configuration Priority

**User Story:** As an Orbit user, I want clear precedence rules for configuration, so that I can predictably override settings at different levels.

**Acceptance Criteria:**

1. <a name="4.1"></a>The system SHALL apply configuration in this priority order (highest to lowest): CLI flags, environment variables, project config (.orbit.yaml in cwd), home config ($HOME/.orbit.yaml), defaults
2. <a name="4.2"></a>The system SHALL treat omitted fields in config files as "not set" for merging purposes
3. <a name="4.3"></a>The system SHALL treat empty string (`""`) for `post-command` as explicitly disabled
4. <a name="4.4"></a>The system SHALL read `ORBIT_COMMAND` environment variable for the phase command
5. <a name="4.5"></a>The system SHALL read `ORBIT_POST_COMMAND` environment variable for the post-completion command

---

### 5. Logging and Visibility

**User Story:** As an Orbit user, I want the post-command session to be logged, so that I can review what was executed.

**Acceptance Criteria:**

1. <a name="5.1"></a>The system SHALL save post-command session logs in the same session directory as phase logs
2. <a name="5.2"></a>The system SHALL use `post-completion-session.json` and `post-completion-session.txt` as filenames for post-command logs
3. <a name="5.3"></a>The system SHALL label post-command in Markdown transcripts as "Post-Completion" rather than a phase number
4. <a name="5.4"></a>The system SHALL display the configured post-command in dry-run mode output

---

### 6. Dry-Run Mode

**User Story:** As an Orbit user, I want dry-run mode to show all configured commands, so that I can verify my configuration before running.

**Acceptance Criteria:**

1. <a name="6.1"></a>The system SHALL display the configured phase command in dry-run output
2. <a name="6.2"></a>The system SHALL display the configured post-command in dry-run output (or indicate if disabled)
