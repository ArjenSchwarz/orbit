---
references:
    - requirements.md
    - design.md
    - decision_log.md
---
# Multi-Agent Support

## Phase 1: Foundation

- [x] 1. Create agent abstraction layer
  - Create internal/agents/agent.go with Agent interface, SessionExporter interface, SessionInfo, RunOptions, RunResult, CostMetrics types
  - Interface includes Name(), CLICommand(), IsInstalled(), Version(), DefaultSessionDir(), DiscoverSessions(), Run(), Resume()
  - Requirements: [1.1](requirements.md#1.1), [1.2](requirements.md#1.2), [1.3](requirements.md#1.3), [1.4](requirements.md#1.4), [1.5](requirements.md#1.5)
  - References: specs/multi-agent/design.md
  - [x] 1.1. Add tests for error classification
    - Create internal/agents/errors_test.go with tests for ErrorClass values, String(), IsRetryable(), ClassifiedError
    - Requirements: [8.1](requirements.md#8.1), [8.2](requirements.md#8.2)
  - [x] 1.2. Implement error classification
    - Create internal/agents/errors.go with ErrorClass enum, ClassifiedError type, ErrorClassifier interface
    - Requirements: [8.1](requirements.md#8.1), [8.2](requirements.md#8.2)
  - [x] 1.3. Add tests for agent registry
    - Create internal/agents/registry_test.go with tests for Register, Get, List, Default functions
    - Requirements: [1.6](requirements.md#1.6)
  - [x] 1.4. Implement agent registry
    - Create internal/agents/registry.go with AgentConfig struct and Register, Get, List, Default functions
    - Requirements: [1.6](requirements.md#1.6)

- [x] 2. Refactor Claude Code to agent implementation
  - Move internal/claude/ to internal/agents/claudecode/ per Decision 2 (breaking changes acceptable)
  - Requirements: [2.1](requirements.md#2.1)
  - [x] 2.1. Add tests for Claude Code agent
    - Create internal/agents/claudecode/agent_test.go testing Name(), CLICommand(), IsInstalled(), Run(), Resume(), CLI argument building
    - Requirements: [2.2](requirements.md#2.2), [2.5](requirements.md#2.5), [2.6](requirements.md#2.6), [2.7](requirements.md#2.7)
  - [x] 2.2. Implement Claude Code agent
    - Create internal/agents/claudecode/agent.go implementing Agent interface, refactor from internal/claude/client.go
    - Register as claude-code in init()
    - Requirements: [2.2](requirements.md#2.2), [2.5](requirements.md#2.5), [2.6](requirements.md#2.6), [2.7](requirements.md#2.7)
  - [x] 2.3. Add Claude Code error classifier
    - Create internal/agents/claudecode/errors.go with error pattern matching for rate limits, auth, session errors
    - Requirements: [8.2](requirements.md#8.2)
  - [x] 2.4. Update orchestrator to use agent interface
    - Modify internal/orbit/orbit.go to use agents.Agent interface instead of direct claude.Client calls
    - Requirements: [2.3](requirements.md#2.3)
  - [x] 2.5. Verify existing orchestrator tests pass
    - Run existing tests in internal/orbit/ to ensure refactoring did not break functionality
    - Requirements: [2.4](requirements.md#2.4)

## Phase 2: Session Parsing

- [x] 3. Extend format detection for new agents
  - Requirements: [7.1](requirements.md#7.1), [7.2](requirements.md#7.2), [7.3](requirements.md#7.3)
  - [x] 3.1. Add format enum values
    - Add FormatKiro and FormatCopilot to Format enum in internal/transcript/types.go
    - Requirements: [7.3](requirements.md#7.3)
  - [x] 3.2. Add tests for Kiro format detection
    - Add test cases to internal/transcript/parser_test.go for detecting Kiro JSON format
    - Copy sample file from specs/multi-agent/samples/kiro/ to internal/transcript/testdata/kiro/
    - Requirements: [7.1](requirements.md#7.1), [7.2](requirements.md#7.2)
  - [x] 3.3. Add tests for Copilot format detection
    - Add test cases to internal/transcript/parser_test.go for detecting Copilot JSONL format
    - Copy sample file from specs/multi-agent/samples/copilot/ to internal/transcript/testdata/copilot/
    - Requirements: [7.1](requirements.md#7.1), [7.2](requirements.md#7.2)
  - [x] 3.4. Implement improved DetectFormat
    - Update internal/transcript/parser.go with new detection logic: try Kiro JSON first, then JSONL detection
    - Cannot use first-byte check alone (both JSON and JSONL start with curly brace)
    - Requirements: [7.1](requirements.md#7.1), [7.2](requirements.md#7.2)

- [x] 4. Implement Kiro parser
  - Requirements: [4.6](requirements.md#4.6)
  - [x] 4.1. Add Kiro type definitions
    - Create internal/transcript/kiro_types.go with KiroSession, KiroHistoryItem, KiroUserMessage, etc.
    - Requirements: [4.6](requirements.md#4.6)
  - [x] 4.2. Add golden file tests for Kiro parser
    - Create internal/transcript/kiro_parser_test.go with golden file tests
    - Create expected.md golden file in testdata/kiro/
    - Requirements: [4.6](requirements.md#4.6)
  - [x] 4.3. Implement Kiro parser
    - Create internal/transcript/kiro_parser.go to parse plain JSON format and render to unified Entry format
    - Requirements: [4.6](requirements.md#4.6)

- [x] 5. Implement Copilot parser
  - Requirements: [5.6](requirements.md#5.6)
  - [x] 5.1. Add Copilot type definitions
    - Create internal/transcript/copilot_types.go with CopilotEntry, CopilotSessionStart, CopilotUserMessage, etc.
    - Type markers: session.start, session.info, user.message, assistant.turn_start, assistant.message, assistant.reasoning, assistant.turn_end, tool.execution_start, tool.execution_complete
    - Requirements: [5.6](requirements.md#5.6)
  - [x] 5.2. Add golden file tests for Copilot parser
    - Create internal/transcript/copilot_parser_test.go with golden file tests
    - Create expected.md golden file in testdata/copilot/
    - Requirements: [5.6](requirements.md#5.6)
  - [x] 5.3. Implement Copilot parser
    - Create internal/transcript/copilot_parser.go to parse JSONL format and render to unified Entry format
    - Requirements: [5.6](requirements.md#5.6)

## Phase 3: Additional Agents

- [x] 6. Implement Codex agent
  - Requirements: [3.1](requirements.md#3.1), [3.2](requirements.md#3.2), [3.3](requirements.md#3.3), [3.4](requirements.md#3.4), [3.5](requirements.md#3.5)
  - [x] 6.1. Add tests for Codex agent
    - Create internal/agents/codex/agent_test.go testing CLI invocation patterns
    - Tests for: codex exec prompt, --full-auto flag, codex exec resume id
    - Requirements: [3.2](requirements.md#3.2), [3.3](requirements.md#3.3), [3.4](requirements.md#3.4)
  - [x] 6.2. Implement Codex agent
    - Create internal/agents/codex/agent.go implementing Agent interface
    - DefaultSessionDir: ~/.codex/sessions/ with date-sharded structure
    - Requirements: [3.1](requirements.md#3.1), [3.2](requirements.md#3.2), [3.3](requirements.md#3.3), [3.4](requirements.md#3.4), [3.5](requirements.md#3.5)
  - [x] 6.3. Add Codex error classifier
    - Create internal/agents/codex/errors.go with error pattern matching
    - Requirements: [8.2](requirements.md#8.2)

- [x] 7. Implement Kiro agent
  - Requirements: [4.1](requirements.md#4.1), [4.2](requirements.md#4.2), [4.3](requirements.md#4.3), [4.4](requirements.md#4.4), [4.5](requirements.md#4.5)
  - [x] 7.1. Add tests for Kiro agent
    - Create internal/agents/kiro/agent_test.go testing CLI invocation and SessionExporter interface
    - Tests for: kiro-cli chat --no-interactive, --trust-all-tools, --resume, ExportSession
    - Requirements: [4.2](requirements.md#4.2), [4.3](requirements.md#4.3), [4.4](requirements.md#4.4), [4.5](requirements.md#4.5)
  - [x] 7.2. Implement Kiro agent
    - Create internal/agents/kiro/agent.go implementing Agent and SessionExporter interfaces
    - DefaultSessionDir returns empty (Kiro does not store logs automatically per Decision 7)
    - Requirements: [4.1](requirements.md#4.1), [4.2](requirements.md#4.2), [4.3](requirements.md#4.3), [4.4](requirements.md#4.4), [4.5](requirements.md#4.5)
  - [x] 7.3. Add Kiro error classifier
    - Create internal/agents/kiro/errors.go with error pattern matching
    - Requirements: [8.2](requirements.md#8.2)

- [x] 8. Implement Copilot agent
  - Requirements: [5.1](requirements.md#5.1), [5.2](requirements.md#5.2), [5.3](requirements.md#5.3), [5.4](requirements.md#5.4), [5.5](requirements.md#5.5)
  - [x] 8.1. Add tests for Copilot agent
    - Create internal/agents/copilot/agent_test.go testing CLI invocation patterns
    - Tests for: copilot -p prompt, --allow-all-paths, --continue (note: sessionID ignored per Known Limitation)
    - Requirements: [5.2](requirements.md#5.2), [5.3](requirements.md#5.3), [5.4](requirements.md#5.4)
  - [x] 8.2. Implement Copilot agent
    - Create internal/agents/copilot/agent.go implementing Agent interface
    - DefaultSessionDir: ~/.copilot/session-state/
    - Document that Resume ignores sessionID (Copilot limitation)
    - Requirements: [5.1](requirements.md#5.1), [5.2](requirements.md#5.2), [5.3](requirements.md#5.3), [5.4](requirements.md#5.4), [5.5](requirements.md#5.5)
  - [x] 8.3. Add Copilot error classifier
    - Create internal/agents/copilot/errors.go with error pattern matching
    - Requirements: [8.2](requirements.md#8.2)

## Phase 4: Configuration and CLI

- [ ] 9. Extend configuration for agents
  - Requirements: [9.1](requirements.md#9.1), [9.2](requirements.md#9.2), [9.3](requirements.md#9.3), [9.4](requirements.md#9.4), [9.5](requirements.md#9.5), [9.6](requirements.md#9.6), [9.7](requirements.md#9.7)
  - [ ] 9.1. Add tests for agent configuration
    - Add test cases to internal/config/config_test.go for agents section, GetAgentConfig method
    - Requirements: [9.1](requirements.md#9.1), [9.2](requirements.md#9.2), [9.3](requirements.md#9.3), [9.4](requirements.md#9.4), [9.5](requirements.md#9.5), [9.6](requirements.md#9.6), [9.7](requirements.md#9.7)
  - [ ] 9.2. Implement agent configuration
    - Add AgentConfig struct and Agents map to internal/config/config.go
    - Add GetAgentConfig method with timeout parsing
    - Requirements: [9.1](requirements.md#9.1), [9.2](requirements.md#9.2), [9.3](requirements.md#9.3), [9.4](requirements.md#9.4), [9.5](requirements.md#9.5), [9.6](requirements.md#9.6), [9.7](requirements.md#9.7)

- [ ] 10. Add agent selection to orbit run command
  - Requirements: [6.1](requirements.md#6.1), [6.2](requirements.md#6.2), [6.3](requirements.md#6.3), [6.4](requirements.md#6.4), [6.5](requirements.md#6.5), [6.6](requirements.md#6.6)
  - [ ] 10.1. Add --agent flag to orbit run
    - Modify cmd/orbit/run.go to add --agent flag
    - Implement selection priority: CLI flag > config file > default (claude-code)
    - Requirements: [6.1](requirements.md#6.1), [6.2](requirements.md#6.2), [6.3](requirements.md#6.3), [6.4](requirements.md#6.4)
  - [ ] 10.2. Add agent validation
    - Validate agent is installed before running, emit helpful error with install URL if not
    - Requirements: [6.5](requirements.md#6.5), [6.6](requirements.md#6.6)

- [ ] 11. Add agent override to apsis command
  - Requirements: [7.4](requirements.md#7.4)
  - [ ] 11.1. Add --agent flag to apsis
    - Modify cmd/apsis/main.go to add --agent flag for format override
    - Requirements: [7.4](requirements.md#7.4)

## Phase 5: Per-Variant Agent Selection

- [ ] 12. Add agent field to variants
  - Requirements: [10.3](requirements.md#10.3), [10.5](requirements.md#10.5)
  - [ ] 12.1. Add tests for variant agent assignment
    - Add tests to internal/variants/ for Agent field population, cycling behavior
    - Requirements: [10.3](requirements.md#10.3), [10.5](requirements.md#10.5)
  - [ ] 12.2. Extend Variant struct with Agent field
    - Add Agent string field to Variant struct in internal/variants/types.go
    - Implement assignVariantAgents function
    - Requirements: [10.5](requirements.md#10.5)

- [ ] 13. Add variant-agents flag
  - Requirements: [10.1](requirements.md#10.1), [10.6](requirements.md#10.6), [10.7](requirements.md#10.7)
  - [ ] 13.1. Add --variant-agents flag to orbit run
    - Modify cmd/orbit/run.go to add --variant-agents flag (comma-separated list)
    - Requirements: [10.1](requirements.md#10.1)
  - [ ] 13.2. Update comparison report
    - Modify comparison report generation to display agent used per variant
    - Requirements: [10.6](requirements.md#10.6)
  - [ ] 13.3. Update comparison prompt
    - Include agent information in comparison prompt for context
    - Requirements: [10.7](requirements.md#10.7)

## Phase 6: Integration and Cleanup

- [ ] 14. Wire Kiro session export in orchestrator
  - Requirements: [4.5](requirements.md#4.5)
  - [ ] 14.1. Add session export after phases for Kiro
    - Modify internal/orbit/orbit.go to check for SessionExporter interface and call ExportSession after each phase
    - Handle export failures gracefully (log warning, do not fail orchestration)
    - Per Decision 8: Export after each phase completes
    - Requirements: [4.5](requirements.md#4.5)

- [ ] 15. Remove deprecated claude package
  - Requirements: [2.1](requirements.md#2.1)
  - [ ] 15.1. Remove internal/claude/ package
    - Delete internal/claude/ directory after verifying all imports updated
    - Update any remaining imports across codebase
    - Per Decision 2: Breaking changes acceptable
    - Requirements: [2.1](requirements.md#2.1)

- [ ] 16. Integration testing
  - Requirements: [6.1](requirements.md#6.1), [6.2](requirements.md#6.2), [6.3](requirements.md#6.3), [6.4](requirements.md#6.4), [10.1](requirements.md#10.1), [10.3](requirements.md#10.3)
  - [ ] 16.1. Add integration test for agent selection
    - Create integration test verifying --agent flag and config file selection work correctly
    - Requirements: [6.1](requirements.md#6.1), [6.2](requirements.md#6.2), [6.3](requirements.md#6.3), [6.4](requirements.md#6.4)
  - [ ] 16.2. Add integration test for variant agents
    - Create integration test verifying --variant-agents cycling behavior
    - Requirements: [10.1](requirements.md#10.1), [10.3](requirements.md#10.3)
