---
references:
    - specs/per-variant-model-selection/requirements.md
    - specs/per-variant-model-selection/design.md
    - specs/per-variant-model-selection/decision_log.md
---
# Per-Variant Model Selection

## Phase 1: Config Package Foundation

- [x] 1. Create AgentAliasConfig and ResolvedAgent types
  - Add new structs to internal/config/config.go with yaml tags for type, model, cli-path, auto-approve, extra-args, timeout
  - Requirements: [1.5](requirements.md#1.5), [1.6](requirements.md#1.6), [1.7](requirements.md#1.7)
  - References: internal/config/config.go

- [x] 2. Add alias name validation function
  - Implement ValidateAliasName() with pattern [a-z0-9]+(-[a-z0-9]+)* and NormalizeAliasName() for case normalization
  - Requirements: [1.2](requirements.md#1.2), [1.3](requirements.md#1.3)
  - References: internal/config/config.go

- [x] 3. Add property-based tests for alias validation
  - Use pgregory.net/rapid to test ValidateAliasName and NormalizeAliasName with generated inputs
  - Requirements: [1.2](requirements.md#1.2), [1.3](requirements.md#1.3)
  - References: internal/config/config_test.go

- [x] 4. Add unit tests for alias validation
  - Table-driven tests for valid names, invalid patterns (starts/ends with hyphen, underscore, dot), case normalization
  - Requirements: [1.2](requirements.md#1.2), [1.3](requirements.md#1.3), [1.4](requirements.md#1.4)
  - References: internal/config/config_test.go

## Phase 2: Config Loading and Validation

- [x] 5. Implement parseAgentsConfig with YAML type coercion
  - Parse agents section from YAML, handle model field type coercion (string/int/float valid, bool/array/map error)
  - Requirements: [6.4](requirements.md#6.4)
  - References: internal/config/config.go

- [x] 6. Implement ResolveAliases validation
  - Validate all aliases have type field, check for duplicates after normalization, verify type is registered agent
  - Requirements: [1.4](requirements.md#1.4), [1.5](requirements.md#1.5), [5.1](requirements.md#5.1), [5.2](requirements.md#5.2), [5.4](requirements.md#5.4)
  - References: internal/config/config.go

- [x] 7. Add RequireConfigFile and GetResolvedAgent methods
  - RequireConfigFile returns error if no config found, GetResolvedAgent looks up alias and returns error if not found
  - Requirements: [2.1](requirements.md#2.1), [2.2](requirements.md#2.2), [2.5](requirements.md#2.5), [2.6](requirements.md#2.6)
  - References: internal/config/config.go

- [x] 8. Add ConfigFileFound tracking
  - Set ConfigFileFound=true when loading config, check in RequireConfigFile
  - Requirements: [2.1](requirements.md#2.1)
  - References: internal/config/config.go

- [x] 9. Add unit tests for YAML type coercion
  - Test string, unquoted string, integer, float models (valid) and boolean, array, map models (error)
  - Requirements: [6.4](requirements.md#6.4)
  - References: internal/config/config_test.go

- [x] 10. Add unit tests for ResolveAliases
  - Test missing type field, unknown type, empty agents section, duplicate aliases after normalization
  - Requirements: [1.4](requirements.md#1.4), [1.5](requirements.md#1.5), [2.3](requirements.md#2.3), [2.4](requirements.md#2.4), [5.2](requirements.md#5.2), [5.4](requirements.md#5.4)
  - References: internal/config/config_test.go

- [x] 11. Add unit tests for config merge behavior
  - Test home only, project only, deep merge of same alias, different aliases from each
  - Requirements: [2.1](requirements.md#2.1)
  - References: internal/config/config_test.go

## Phase 3: Init Command

- [x] 12. Implement GenerateDefaultConfig function
  - Return YAML bytes for default config with claude-code agent with type and auto-approve true
  - Requirements: [3.5](requirements.md#3.5)
  - References: internal/config/config.go

- [x] 13. Implement orbit init subcommand
  - Create cmd/orbit/init.go with --force flag, check existing file, write default config, log success
  - Requirements: [3.1](requirements.md#3.1), [3.2](requirements.md#3.2), [3.3](requirements.md#3.3), [3.4](requirements.md#3.4), [3.6](requirements.md#3.6)
  - References: cmd/orbit/init.go, cmd/orbit/main.go

- [x] 14. Add unit tests for init command
  - Test no existing config creates file, existing config fails, --force overwrites, write permission error
  - Requirements: [3.1](requirements.md#3.1), [3.2](requirements.md#3.2), [3.3](requirements.md#3.3), [3.4](requirements.md#3.4)
  - References: cmd/orbit/init_test.go

## Phase 4: Agent Resolution Changes

- [x] 15. Add buildAgentConfig function
  - Create agents.AgentConfig from ResolvedAgent, put model in Options map if set
  - Requirements: [6.1](requirements.md#6.1), [6.2](requirements.md#6.2)
  - References: cmd/orbit/run.go

- [x] 16. Modify run command to use new config flow
  - Call RequireConfigFile, ResolveAliases, GetResolvedAgent, pass type to agents.Get
  - Requirements: [4.1](requirements.md#4.1), [4.2](requirements.md#4.2), [5.1](requirements.md#5.1)
  - References: cmd/orbit/run.go

- [x] 17. Update compare command to require config
  - Add RequireConfigFile check since compare uses agent for AI analysis
  - Requirements: [5.1](requirements.md#5.1)
  - References: cmd/orbit/compare.go

- [x] 18. Update consolidate command to require config
  - Add RequireConfigFile check since consolidate uses agent
  - Requirements: [5.1](requirements.md#5.1)
  - References: cmd/orbit/consolidate.go

- [x] 19. Add unit tests for buildAgentConfig
  - Test model in Options when set, Options nil/empty when no model
  - Requirements: [6.1](requirements.md#6.1), [6.2](requirements.md#6.2)
  - References: cmd/orbit/run_test.go

## Phase 5: Agent Model Flag Implementation

- [x] 20. Add model flag to claude-code agent
  - In buildArgs, check Options["model"] and append --model flag
  - Requirements: [6.1](requirements.md#6.1), [6.3](requirements.md#6.3)
  - References: internal/agents/claudecode/agent.go

- [x] 21. Add model flag to codex agent
  - In buildArgs, check Options["model"] and append --model flag
  - Requirements: [6.1](requirements.md#6.1), [6.3](requirements.md#6.3)
  - References: internal/agents/codex/agent.go

- [x] 22. Add model flag to kiro agent
  - In buildArgs, check Options["model"] and append --model flag
  - Requirements: [6.1](requirements.md#6.1), [6.3](requirements.md#6.3)
  - References: internal/agents/kiro/agent.go

- [x] 23. Add model flag to copilot agent
  - In buildArgs, check Options["model"] and append --model flag
  - Requirements: [6.1](requirements.md#6.1), [6.3](requirements.md#6.3)
  - References: internal/agents/copilot/agent.go

- [x] 24. Add unit tests for agent model flags
  - Test each agent buildArgs with and without model in Options
  - Requirements: [6.1](requirements.md#6.1), [6.2](requirements.md#6.2), [6.3](requirements.md#6.3)
  - References: internal/agents/claudecode/agent_test.go, internal/agents/codex/agent_test.go, internal/agents/kiro/agent_test.go, internal/agents/copilot/agent_test.go

## Phase 6: Metadata and Logging

- [x] 25. Add AgentType and Model fields to Variant struct
  - Add AgentType string and Model string fields with json tags to Variant in types.go
  - Requirements: [7.1](requirements.md#7.1)
  - References: internal/variants/types.go

- [x] 26. Update variant creation to populate new fields
  - When creating variants, set Agent (alias name), AgentType, and Model from resolved config
  - Requirements: [4.4](requirements.md#4.4), [7.1](requirements.md#7.1)
  - References: internal/variants/manager.go

- [x] 27. Add AgentAlias, AgentType, Model to SessionEntry
  - Add three new fields to SessionEntry struct in logs/manager.go
  - Requirements: [7.2](requirements.md#7.2)
  - References: internal/logs/manager.go

- [x] 28. Update session logging to populate new fields
  - When logging session, include alias, type, and model from resolved agent config
  - Requirements: [7.2](requirements.md#7.2)
  - References: internal/logs/manager.go

- [x] 29. Add verbose logging of resolved agent config
  - When verbose flag set, log alias name, type, model before agent execution
  - Requirements: [7.3](requirements.md#7.3)
  - References: cmd/orbit/run.go

- [x] 30. Add unit tests for variant metadata
  - Test that Variant struct includes agent, agent_type, model in JSON output
  - Requirements: [7.1](requirements.md#7.1)
  - References: internal/variants/types_test.go

- [x] 31. Add unit tests for session entry metadata
  - Test that SessionEntry includes agent_alias, agent_type, model in JSON output
  - Requirements: [7.2](requirements.md#7.2)
  - References: internal/logs/manager_test.go

## Phase 7: Integration Tests

- [x] 32. Add integration test for run without config
  - Test orbit run with no .orbit.yaml exits 1 with message about orbit init
  - Requirements: [2.1](requirements.md#2.1), [2.2](requirements.md#2.2)
  - References: internal/orbit/orbit_test.go

- [x] 33. Add integration test for variant run with different models
  - Create .orbit.yaml with two aliases, run --variants 2 --variant-agents, verify variants.json has correct metadata
  - Requirements: [4.1](requirements.md#4.1), [4.2](requirements.md#4.2), [4.4](requirements.md#4.4), [7.1](requirements.md#7.1)
  - References: internal/orbit/orbit_test.go

- [x] 34. Add integration test for init command
  - Test orbit init creates file, running again fails, --force overwrites
  - Requirements: [3.1](requirements.md#3.1), [3.2](requirements.md#3.2), [3.3](requirements.md#3.3), [3.4](requirements.md#3.4)
  - References: cmd/orbit/init_test.go
