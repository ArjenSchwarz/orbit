# Requirements: Per-Variant Model Selection

## Introduction

This feature enables users to run parallel implementation variants using the same underlying agent (e.g., Claude Code) but with different models (e.g., claude-sonnet-4 vs claude-opus-4). Instead of adding model configuration per-variant in guidance files, this feature introduces **agent aliases** - named configurations in `.orbit.yaml` that combine an agent type with model and other settings.

Users define aliases like `claude-sonnet` or `claude-opus` in their config, then reference them via the existing `--variant-agents` flag. This approach reuses existing infrastructure, centralizes configuration, and provides reusable named configurations across runs.

**Key change:** Agent names are now configurable aliases that map to an underlying agent type plus settings. All agents must be configured in `.orbit.yaml`. Users must run `orbit init` to create the configuration file before first use.

---

## Requirements

### 1. Agent Alias Configuration

**User Story:** As a developer, I want to define named agent configurations that combine an agent type with a model, so that I can easily run variants with different model configurations.

**Acceptance Criteria:**

1. <a name="1.1"></a>The system SHALL support an `agents` section in `.orbit.yaml` where each key is an alias name
2. <a name="1.2"></a>Alias names SHALL match the pattern `[a-z0-9]+(-[a-z0-9]+)*` (lowercase alphanumeric with hyphens, not starting or ending with hyphen)
3. <a name="1.3"></a>Alias names SHALL be case-insensitive (normalized to lowercase internally)
4. <a name="1.4"></a>Alias names SHALL be unique after case normalization; duplicate aliases SHALL cause a configuration error
5. <a name="1.5"></a>Each agent alias SHALL require a `type` field specifying the underlying agent type
6. <a name="1.6"></a>Each agent alias SHALL support an optional `model` field specifying the model to use
7. <a name="1.7"></a>Each agent alias SHALL support existing agent config options (cli-path, auto-approve, extra-args, timeout)
8. <a name="1.8"></a>WHEN an alias does not specify a model, THEN the agent SHALL use its default model

**Example configuration:**
```yaml
agents:
  claude-sonnet:
    type: claude-code
    model: claude-sonnet-4-20250514
  claude-opus:
    type: claude-code
    model: claude-opus-4-20250514
    timeout: 30m
  codex-o3:
    type: codex
    model: o3
  kiro-default:
    type: kiro
    # No model field - uses agent default
```

**Valid agent types:** `claude-code`, `codex`, `kiro`, `copilot`

---

### 2. Agent Configuration Requirement

**User Story:** As a developer, I want all agents to be explicitly configured, so that Orbit works consistently with any agent CLI including ones that don't exist yet.

**Acceptance Criteria:**

1. <a name="2.1"></a>The system SHALL require a `.orbit.yaml` file to exist before running
2. <a name="2.2"></a>WHEN a user runs Orbit without a `.orbit.yaml` file, THEN the system SHALL fail with exit code 1 and a message directing the user to run `orbit init`
3. <a name="2.3"></a>The system SHALL require at least one agent to be defined in the `agents` section
4. <a name="2.4"></a>WHEN the `agents` section is missing or empty, THEN the system SHALL fail with exit code 1 indicating no agents are configured
5. <a name="2.5"></a>WHEN a user specifies an agent that is not configured in `.orbit.yaml`, THEN the system SHALL fail with exit code 1 and list available configured agents
6. <a name="2.6"></a>The error message for unconfigured agents SHALL include example syntax for adding the agent

---

### 3. Configuration Initialization

**User Story:** As a developer, I want to easily create an initial configuration file, so that I can get started with Orbit quickly.

**Acceptance Criteria:**

1. <a name="3.1"></a>The system SHALL provide an `orbit init` subcommand to generate a configuration file
2. <a name="3.2"></a>The `orbit init` command SHALL create `.orbit.yaml` in the current directory
3. <a name="3.3"></a>WHEN `.orbit.yaml` already exists, THEN `orbit init` SHALL fail with exit code 1 and a message indicating the file already exists
4. <a name="3.4"></a>The `orbit init` command SHALL support a `--force` flag to overwrite existing config
5. <a name="3.5"></a>The generated config SHALL contain a single `claude-code` agent with type `claude-code` and `auto-approve: true`
6. <a name="3.6"></a>The system SHALL log a success message when the configuration file is created

**Default configuration file content:**
```yaml
# Orbit configuration - see documentation for all options
agents:
  claude-code:
    type: claude-code
    auto-approve: true
```

---

### 4. Variant Agent Assignment with Aliases

**User Story:** As a developer, I want to use my configured agent aliases with the existing `--variant-agents` flag, so that I can compare different model configurations without learning new syntax.

**Acceptance Criteria:**

1. <a name="4.1"></a>The `--variant-agents` flag SHALL accept alias names defined in `.orbit.yaml`
2. <a name="4.2"></a>The system SHALL resolve each alias to its underlying agent type and configuration before execution
3. <a name="4.3"></a>WHEN fewer aliases are provided than variants, THEN the system SHALL cycle through the alias list (existing behavior)
4. <a name="4.4"></a>The system SHALL store the resolved alias name (not just agent type) in variant metadata for traceability

---

### 5. Validation and Error Handling

**User Story:** As a developer, I want clear error messages when my configuration is invalid, so that I can fix problems quickly.

**Acceptance Criteria:**

1. <a name="5.1"></a>The system SHALL validate that the `type` field is present for all agent aliases at startup
2. <a name="5.2"></a>WHEN the `type` field is missing, THEN the system SHALL fail with exit code 1 specifying which alias lacks a type
3. <a name="5.3"></a>WHEN an alias name does not match the required pattern, THEN the system SHALL fail with exit code 1 specifying the invalid name
4. <a name="5.4"></a>WHEN duplicate alias names exist after case normalization, THEN the system SHALL fail with exit code 1 listing the conflicting names
5. <a name="5.5"></a>WHEN an alias specifies a `cli-path` that does not exist, THEN the system SHALL fail with exit code 2 including the path
6. <a name="5.6"></a>WHEN an alias references an agent type without a `cli-path` and the type is not in PATH, THEN the system SHALL fail with exit code 2
7. <a name="5.7"></a>WHEN an agent CLI fails due to invalid model, THEN the system SHALL propagate the agent's error and exit code
8. <a name="5.8"></a>The system SHALL NOT validate model values before passing them to agent CLIs

**Exit codes:**
- 0: Success
- 1: Configuration error (missing config file, missing type, invalid alias name, duplicate alias names, unconfigured agent, empty agents section)
- 2: Agent CLI not found (cli-path doesn't exist, command not in PATH)
- Exit code from agent CLI: Agent execution failure (including invalid model)

---

### 6. Model Passing to Agent CLIs

**User Story:** As a developer, I want models to be correctly passed to each agent's CLI, so that variants actually run with the specified models.

**Acceptance Criteria:**

1. <a name="6.1"></a>WHEN a model is configured for an alias, THEN the system SHALL pass the model to the agent via the `--model` CLI flag
2. <a name="6.2"></a>WHEN no model is configured for an alias, THEN the system SHALL not pass a model flag (using agent default)
3. <a name="6.3"></a>The model value SHALL be passed as-is to the CLI without transformation
4. <a name="6.4"></a>The model value SHALL be converted to a string if provided as another YAML type (int, float)

---

### 7. Logging and Traceability

**User Story:** As a developer, I want to see which model was used for each variant in logs and reports, so that I can understand and compare results.

**Acceptance Criteria:**

1. <a name="7.1"></a>The variant metadata (variants.json) SHALL include the alias name and model used for each variant
2. <a name="7.2"></a>The summary.json log SHALL include the agent alias and model for each phase execution
3. <a name="7.3"></a>WHEN verbose mode is enabled, THEN the system SHALL log the resolved agent configuration including model at startup
4. <a name="7.4"></a>The comparison report SHALL identify which alias/model combination was used for each variant

---

## Out of Scope

- Model validation against agent-specific model lists (agents handle their own validation)
- Per-variant model specification in guidance files (superseded by alias approach)
- Automatic model discovery from agent CLIs
- Model cost estimation or budget limits
- Detection of installed agents (users configure what they need)
- Built-in agent names (new tools are constantly released)
