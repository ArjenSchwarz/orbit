# Decision Log: Per-Variant Model Selection

## Decision 1: Agent Alias Approach Over Per-Variant Model Configuration

**Date**: 2026-01-24
**Status**: accepted

### Context

Users want to run parallel variants with the same underlying agent but different models (e.g., claude-sonnet-4 vs claude-opus-4). The initial approach considered adding model configuration directly to the guidance file schema or introducing a new `--variant-models` CLI flag.

### Decision

Use an agent alias approach where named configurations in `.orbit.yaml` combine an agent type with model and other settings. Users define aliases like `claude-sonnet` and reference them via the existing `--variant-agents` flag.

### Rationale

This approach reuses existing infrastructure (`--variant-agents` flag, config loading), centralizes configuration in `.orbit.yaml`, and provides reusable named configurations. It's simpler than adding model fields to guidance files and more flexible than a dedicated CLI flag.

### Alternatives Considered

- **Guidance file model field**: Adding `model` to per-variant entries in guidance.yaml - Rejected because it spreads configuration across files and couples model selection to guidance text
- **--variant-models CLI flag**: New flag similar to --variant-agents - Rejected because it adds redundant mechanism when aliases achieve the same goal more elegantly
- **Both guidance file and CLI flag**: Maximum flexibility - Rejected due to unnecessary complexity

### Consequences

**Positive:**
- Reuses existing `--variant-agents` mechanism
- Named configurations are reusable across runs
- Centralizes all agent config in one place
- No changes to guidance file schema

**Negative:**
- Requires `.orbit.yaml` configuration (mitigated by auto-generation)
- Agent names become aliases requiring type field

---

## Decision 2: Require Explicit Type Field for Agent Aliases

**Date**: 2026-01-24
**Status**: accepted

### Context

When defining agent aliases in `.orbit.yaml`, users need to specify which underlying agent type the alias represents. Options included requiring an explicit `type` field, inferring from alias prefix (e.g., `claude-sonnet` → `claude-code`), or inferring when alias matches a known agent name.

### Decision

Require an explicit `type` field for all agent aliases. No inference from naming patterns.

### Rationale

Explicit configuration is clearer and less error-prone. Inference based on naming patterns could lead to subtle bugs when alias names don't follow expected patterns or when new agent types are added.

### Alternatives Considered

- **Infer from alias prefix**: claude-sonnet infers type: claude-code - Rejected because naming patterns may not always be consistent
- **Infer from existing agent name match**: If alias equals known agent, use that - Rejected to maintain consistency (all aliases work the same way)

### Consequences

**Positive:**
- Configuration is explicit and unambiguous
- No magic naming conventions to remember
- Easier to understand what an alias does by reading config

**Negative:**
- Slightly more verbose configuration
- Users must know the exact agent type names

---

## Decision 3: Require Agent Configuration in .orbit.yaml

**Date**: 2026-01-24
**Status**: accepted

### Context

Previously, built-in agent names (claude-code, codex, kiro, copilot) worked without explicit configuration. The question was whether to maintain this behavior or require all agents to be configured.

### Decision

Require all agents to be defined in `.orbit.yaml`. When a user runs Orbit without a config file, offer to create a default configuration with basic settings for all installed agents.

### Rationale

Requiring configuration ensures users explicitly understand what agents are available and how they're configured. The auto-generation feature mitigates the onboarding friction by detecting installed agents and creating sensible defaults.

### Alternatives Considered

- **Built-ins always available**: Can use claude-code without config - Rejected to maintain consistency between built-in and user-defined aliases
- **Hybrid approach**: Built-ins available but overridable - Rejected due to complexity in precedence rules

### Consequences

**Positive:**
- Consistent behavior for all agent references
- Users have visibility into their configuration
- Encourages explicit configuration practices

**Negative:**
- Breaking change from current behavior (mitigated by helpful error messages and auto-generation)
- Requires config file to exist

---

## Decision 4: Validate Structure, Not Values

**Date**: 2026-01-24
**Status**: accepted

### Context

When agent configuration has errors, the system needs to decide what to validate upfront versus what to let the agent CLI handle.

### Decision

Validate configuration structure (required fields like `type`, existence of `cli-path` if specified) at startup. Do not validate model values - pass them through to agent CLIs and let them fail if invalid.

### Rationale

We cannot know what models each agent supports, and new models are released frequently. Validating structure catches obvious configuration mistakes early, while letting agent CLIs handle model validation ensures we don't reject valid configurations.

### Alternatives Considered

- **Validate everything upfront**: Maintain lists of valid models per agent - Rejected because model lists change constantly
- **Validate nothing**: Let all errors come from agent CLIs - Rejected because obvious config errors (missing type) should fail fast

### Consequences

**Positive:**
- Structural errors caught immediately
- No false positives on valid model names
- System works with any future models

**Negative:**
- Invalid model errors only surface during execution

---

## Decision 5: Auto-Generate Default Config Without Prompting

**Date**: 2026-01-24
**Status**: accepted

### Context

When a user runs Orbit without a `.orbit.yaml` file, the system needs to decide how to handle this. Options included prompting interactively, failing with instructions, or auto-generating a default.

### Decision

Auto-generate a minimal default configuration file with a single `claude-code` agent. No prompting, no detection of installed agents. Log a message indicating the file was created.

### Rationale

Interactive prompts break CI/CD and automation. Detecting installed agents adds complexity and creates portability issues when configs are committed to version control. A simple default with claude-code covers the most common case and users can customize from there.

### Alternatives Considered

- **Interactive prompt**: Ask user where to create config - Rejected because it breaks non-interactive execution
- **Detect installed agents**: Auto-populate with all installed agents - Rejected because it creates portability problems when config is committed
- **Fail with instructions**: Require manual config creation - Rejected because it adds friction for new users

### Consequences

**Positive:**
- Works in both interactive and non-interactive modes
- Simple, predictable behavior
- No portability issues from machine-specific detection

**Negative:**
- Users with other agents must edit the config manually
- Default only includes claude-code

---

## Decision 6: Unified Model Flag Across All Agents

**Date**: 2026-01-24
**Status**: accepted

### Context

Different agent CLIs might use different flags for model selection. The question was whether to maintain per-agent flag mappings or use a consistent flag.

### Decision

Use `--model` flag for all agent types. This is the standard flag used by Claude Code, Codex, Kiro, and Copilot CLIs.

### Rationale

All four supported agent CLIs use `--model` for model selection. Using a consistent flag simplifies implementation and user mental model.

### Alternatives Considered

- **Per-agent flag configuration**: Allow configuring which flag to use - Rejected as unnecessary given all agents use `--model`

### Consequences

**Positive:**
- Simple, consistent implementation
- No per-agent special cases

**Negative:**
- If a future agent uses a different flag, code changes would be needed (can use extra-args as workaround)

---

## Decision 7: Require `orbit init`, No Auto-Generation

**Date**: 2026-01-24
**Status**: accepted

### Context

When a user runs Orbit without a `.orbit.yaml` file, there were two options: auto-generate a default config, or require explicit initialization via `orbit init`.

### Decision

Require explicit `orbit init` before first use. When no config exists, fail with exit code 1 and a message directing users to run `orbit init`.

### Rationale

Auto-creating files can be surprising and breaks the principle of explicit user control. Requiring `orbit init` follows established CLI patterns (git init, npm init) where users explicitly initialize configuration. This also makes it clear that the config file is intentional and can be customized before first run.

### Alternatives Considered

- **Auto-generate on first run**: Create config automatically - Rejected because auto-creating files is surprising behavior
- **Both auto-gen and init**: Auto-gen on run, init for explicit - Rejected due to conflicting UX patterns

### Consequences

**Positive:**
- Explicit control over config creation
- Familiar pattern from other tools
- No surprising file creation
- Config can be customized before first run

**Negative:**
- Extra step for first-time users

---

## Decision 8: Hardcode Model Flag Per Agent Type

**Date**: 2026-01-24
**Status**: accepted (supersedes previous model-flag decision)

### Context

Initially considered adding a `model-flag` field to support custom agents with different CLI flags. However, we decided not to support custom agent types - only registered types (claude-code, codex, kiro, copilot) are valid.

### Decision

Remove the `model-flag` configuration field. Each agent implementation hardcodes its own model flag. All registered agents use `--model`.

### Rationale

Without custom agent support, there's no need for configurable model flags. All four registered agents use `--model`. Hardcoding the flag in agent implementations:
- Simplifies configuration (one less field to understand)
- Keeps agent-specific details in agent code
- Reduces configuration surface area

### Alternatives Considered

- **Keep model-flag for future flexibility**: Allows easier extension - Rejected because YAGNI; can add later if needed
- **Use extra-args for model**: `extra-args: [--model, value]` - Rejected because loses semantic meaning and traceability

### Consequences

**Positive:**
- Simpler configuration
- Agent implementation controls its own CLI interface
- Less documentation needed

**Negative:**
- If a future registered agent uses a different flag, agent code must handle it

---

## Decision 9: Alias Naming Constraints

**Date**: 2026-01-24
**Status**: accepted

### Context

Alias names in `.orbit.yaml` need validation rules to ensure they work correctly in all contexts (CLI flags, logs, metadata).

### Decision

Alias names must match the pattern `[a-z0-9]+(-[a-z0-9]+)*` (lowercase alphanumeric with hyphens, not starting or ending with hyphen). Names are case-insensitive and normalized to lowercase internally. Duplicate aliases after normalization are rejected.

### Rationale

This pattern matches common CLI naming conventions, avoids shell escaping issues, and ensures aliases work correctly as command-line arguments. Case-insensitivity prevents confusing situations where `Claude-opus` and `claude-opus` could be defined with different configurations, leading to unexpected behavior.

### Alternatives Considered

- **No restrictions**: Allow any valid YAML key - Rejected because special characters could cause shell issues
- **Case-sensitive**: Match YAML's default behavior - Rejected because users could accidentally define `Claude-opus` and `claude-opus` with different configs, causing confusion

### Consequences

**Positive:**
- Safe for use in shell commands
- Consistent with CLI conventions
- Prevents confusion from case variants

**Negative:**
- Users cannot use uppercase in alias names (normalized away)

---
