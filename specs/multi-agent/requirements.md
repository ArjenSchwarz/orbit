# Multi-Agent Support - Requirements

## Introduction

Orbit currently only supports Claude Code for orchestration and session viewing. This feature extends Orbit to support multiple AI coding agents through a unified abstraction layer, enabling users to orchestrate tasks using any supported agent, browse session history across all agents, and benefit from consistent error handling and retry logic regardless of the agent used.

**Supported Agents:**

| Agent          | CLI Command | Session Storage                                | Format          |
|----------------|-------------|------------------------------------------------|-----------------|
| Claude Code    | `claude`    | `~/.claude/projects/<project>/*.jsonl`         | JSONL streaming |
| OpenAI Codex   | `codex`     | `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl` | JSONL streaming |
| AWS Kiro       | `kiro-cli`  | Exported via `/chat save` command              | Plain JSON      |
| GitHub Copilot | `copilot`   | `~/.copilot/session-state/*.jsonl`             | JSONL           |

---

## Requirements

### 1. Agent Abstraction Layer

**User Story:** As a developer, I want Orbit to support multiple AI coding agents through a common interface, so that I can use my preferred agent without changing my workflow.

**Acceptance Criteria:**

1. <a name="1.1"></a>The system SHALL define an `Agent` interface in `internal/agents/agent.go`
2. <a name="1.2"></a>The interface SHALL include identity methods: `Name()` returning the agent identifier (e.g., "claude-code"), and `CLICommand()` returning the executable name (e.g., "claude")
3. <a name="1.3"></a>The interface SHALL include capability methods: `IsInstalled()` returning whether the CLI is available, and `Version()` returning the installed version
4. <a name="1.4"></a>The interface SHALL include execution methods: `Run()` for executing prompts, and `Resume()` for continuing existing sessions
5. <a name="1.5"></a>The interface SHALL include session methods: `DefaultSessionDir()` returning the agent's session storage path, `DiscoverSessions()` for listing sessions, and `ParseSession()` for parsing session content
6. <a name="1.6"></a>The system SHALL provide an agent registry in `internal/agents/registry.go` for looking up agents by name
7. <a name="1.7"></a>Each supported agent SHALL have its own implementation package under `internal/agents/<agent-name>/`
8. <a name="1.8"></a>The `Run()` method SHALL accept arbitrary prompts, supporting both rune phase orchestration and custom commands

---

### 2. Claude Code Agent (Reference Implementation)

**User Story:** As a developer using Orbit, I want the existing Claude Code functionality preserved as the reference implementation, so that my current workflows continue working.

**Acceptance Criteria:**

1. <a name="2.1"></a>The system SHALL refactor `internal/claude/` into `internal/agents/claudecode/`
2. <a name="2.2"></a>The Claude Code agent SHALL implement the complete `Agent` interface
3. <a name="2.3"></a>The orchestrator in `internal/orbit/orbit.go` SHALL use the agent interface instead of direct claude client calls
4. <a name="2.4"></a>All existing orchestrator tests SHALL pass after refactoring
5. <a name="2.5"></a>The agent SHALL invoke `claude -p <prompt> --session-id <id> --output-format json` for new sessions
6. <a name="2.6"></a>The agent SHALL invoke `claude -p <prompt> --resume <id>` for session continuation
7. <a name="2.7"></a>WHERE auto-approve is enabled, the agent SHALL pass `--dangerously-skip-permissions`

---

### 3. OpenAI Codex Agent

**User Story:** As a developer using OpenAI Codex CLI, I want Orbit to orchestrate my tasks and view my session history, so that I can use Codex with the same workflow as Claude Code.

**Acceptance Criteria:**

1. <a name="3.1"></a>The system SHALL provide a Codex agent implementation in `internal/agents/codex/`
2. <a name="3.2"></a>The agent SHALL invoke `codex exec "<prompt>"` for non-interactive execution
3. <a name="3.3"></a>WHERE auto-approve is enabled, the agent SHALL pass `--full-auto`
4. <a name="3.4"></a>The agent SHALL support session resume via `codex exec resume <session-id>` or `codex exec resume --last`
5. <a name="3.5"></a>The agent SHALL discover sessions from `~/.codex/sessions/` with date-sharded directory structure (YYYY/MM/DD)
6. <a name="3.6"></a>The parser SHALL handle Codex JSONL format (type field values to be verified from sample files; existing parser uses `session_meta`, `response_item`, `event_msg`, `turn_context`)

---

### 4. AWS Kiro Agent

**User Story:** As a developer using AWS Kiro CLI, I want Orbit to orchestrate my tasks and view my session history, so that I can use Kiro with the same workflow as Claude Code.

**Acceptance Criteria:**

1. <a name="4.1"></a>The system SHALL provide a Kiro agent implementation in `internal/agents/kiro/`
2. <a name="4.2"></a>The agent SHALL invoke `kiro-cli chat --no-interactive "<prompt>"` for execution
3. <a name="4.3"></a>WHERE auto-approve is enabled, the agent SHALL pass `--trust-all-tools`
4. <a name="4.4"></a>The agent SHALL support session resume via `kiro-cli chat --resume`
5. <a name="4.5"></a>The agent MAY export session logs by running a follow-up command: `kiro-cli chat --no-interactive "/chat save <filename>" --resume`
6. <a name="4.6"></a>The parser SHALL handle Kiro plain JSON format (not JSONL; structure to be determined from sample files)

---

### 5. GitHub Copilot Agent

**User Story:** As a developer using GitHub Copilot CLI, I want Orbit to orchestrate my tasks and view my session history, so that I can use Copilot with the same workflow as Claude Code.

**Acceptance Criteria:**

1. <a name="5.1"></a>The system SHALL provide a Copilot agent implementation in `internal/agents/copilot/`
2. <a name="5.2"></a>The agent SHALL invoke `copilot -p "<prompt>"` for non-interactive execution
3. <a name="5.3"></a>WHERE auto-approve is enabled, the agent SHALL pass `--allow-all-paths` and appropriate per-tool approval flags
4. <a name="5.4"></a>The agent SHALL support session resume via `copilot --continue` for most recent session or `copilot --resume` for session picker
5. <a name="5.5"></a>The agent SHALL discover sessions from `~/.copilot/session-state/`
6. <a name="5.6"></a>The parser SHALL handle Copilot JSONL format (type field values to be determined from sample files)

---

### 6. Agent Selection

**User Story:** As a developer, I want to specify which agent to use for orchestration, so that I can choose the best tool for each project or task.

**Acceptance Criteria:**

1. <a name="6.1"></a>The `orbit run` command SHALL accept an `--agent <name>` flag to specify the agent
2. <a name="6.2"></a>The system SHALL support agent configuration in `.orbit.yaml` via an `agent` key at the root level
3. <a name="6.3"></a>WHERE both CLI flag and config file specify an agent, the CLI flag SHALL take precedence
4. <a name="6.4"></a>WHERE no agent is specified, the system SHALL default to `claude-code`
5. <a name="6.5"></a>The system SHALL validate that the specified agent is installed before running
6. <a name="6.6"></a>IF the agent CLI is not found, the system SHALL emit an error message stating which CLI is required and how to install it

---

### 7. Session Discovery and Viewing

**User Story:** As a developer, I want to view session transcripts from any supported agent in both apsis and the web interface, so that I can review my work regardless of which agent I used.

**Acceptance Criteria:**

1. <a name="7.1"></a>The apsis CLI SHALL auto-detect the agent format from session file content using the `DetectFormat()` function
2. <a name="7.2"></a>Auto-detection SHALL examine the file content to determine format:
   - JSONL with `type` field `user` or `assistant` → Claude Code format
   - JSONL with `type` field `session_meta`, `response_item`, `event_msg`, or `turn_context` → Codex format
   - Plain JSON structure → Kiro format (structure to be determined)
   - JSONL with Copilot type values → Copilot format (to be determined)
3. <a name="7.3"></a>The `Format` enum in `internal/transcript/types.go` SHALL be extended with `FormatKiro` and `FormatCopilot`
4. <a name="7.4"></a>The apsis CLI SHALL accept an `--agent <name>` flag to override auto-detection when needed
5. <a name="7.5"></a>The web interface SHALL display sessions from all configured agents
6. <a name="7.6"></a>All agents SHALL render session content to the same unified Markdown/HTML format
7. <a name="7.7"></a>The system SHALL preserve agent-specific cost metrics in rendered output (tokens for Claude/Codex, credits for Kiro/Copilot where applicable)

---

### 8. Error Handling

**User Story:** As a developer, I want Orbit to handle errors consistently across agents with appropriate retry logic, so that transient failures don't interrupt my workflow.

**Acceptance Criteria:**

1. <a name="8.1"></a>The system SHALL define agent-specific error classifiers in `internal/agents/errors.go`
2. <a name="8.2"></a>Each agent SHALL classify errors into three categories: `retryable`, `fatal`, or `session-invalid`
3. <a name="8.3"></a>The orchestrator SHALL apply the same retry logic (max 5 attempts with backoff) regardless of agent
4. <a name="8.4"></a>Rate limit errors SHALL be classified as retryable for all agents
5. <a name="8.5"></a>Authentication errors SHALL be classified as fatal for all agents
6. <a name="8.6"></a>Session-not-found errors SHALL trigger automatic retry with a fresh session ID
7. <a name="8.7"></a>The error classification system SHALL map existing error types (`ErrConnection`, `ErrRateLimit`, `ErrOverloaded`) to the new categories (`retryable`, `fatal`, `session-invalid`)
8. <a name="8.8"></a>WHERE a timeout is configured and execution exceeds the timeout, the error SHALL be classified as `retryable`

---

### 9. Agent Configuration

**User Story:** As a developer, I want to configure agent-specific settings, so that I can customize behavior per agent.

**Acceptance Criteria:**

1. <a name="9.1"></a>The system SHALL support an `agents` section in `.orbit.yaml` for per-agent configuration
2. <a name="9.2"></a>Each agent configuration SHALL support `cli-path` to override the default CLI command path
3. <a name="9.3"></a>Each agent configuration SHALL support `auto-approve` to control tool approval behavior
4. <a name="9.4"></a>Each agent configuration SHALL support `extra-args` as a list of additional CLI arguments to pass through
5. <a name="9.5"></a>Agent-specific options (e.g., `model`) SHALL be passed through to the CLI where supported
6. <a name="9.6"></a>Each agent configuration SHALL support `timeout` as a duration string (e.g., "30m", "1h") to limit phase execution time
7. <a name="9.7"></a>WHERE timeout is not configured, agent execution SHALL have no time limit (run until completion or user interruption)

**Example configuration:**
```yaml
agent: claude-code

agents:
  claude-code:
    cli-path: claude
    auto-approve: false
    timeout: 30m
  codex:
    cli-path: codex
    auto-approve: true
    extra-args:
      - "--search"
  kiro:
    cli-path: kiro-cli
    model: auto
    timeout: 1h
  copilot:
    cli-path: copilot
    auto-approve: false
```

---

### 10. Per-Variant Agent Selection

**User Story:** As a developer, I want to run different agents for different variants, so that I can compare implementations across agents (e.g., Claude Code vs Codex).

**Acceptance Criteria:**

1. <a name="10.1"></a>The `orbit run --variants` command SHALL support a `--variant-agents <list>` flag accepting a comma-separated list of agent names
2. <a name="10.2"></a>The guidance file (`guidance.yaml`) SHALL support an `agent` field per variant entry
3. <a name="10.3"></a>WHERE `--variant-agents` is specified, variant N SHALL use the Nth agent in the list, cycling if fewer agents than variants
4. <a name="10.4"></a>WHERE variant guidance specifies an agent, it SHALL override the global `--agent` setting for that variant
5. <a name="10.5"></a>The `internal/variants/Variant` struct SHALL include an `Agent` field to track which agent was used
6. <a name="10.6"></a>The comparison report SHALL display the agent used for each variant
7. <a name="10.7"></a>The comparison prompt SHALL include agent information to provide context for the analysis

**Example usage:**
```bash
# Compare Claude Code vs Codex implementations
orbit run --variants 2 --variant-agents claude-code,codex
```

---

## Scope Decisions

The following items are explicitly **out of scope** for the initial implementation:

1. **Mixed-agent orchestration within phases:** A single orchestration run uses one agent for all phases of a single variant. Users can switch agents between separate runs.

2. **Agent fallback on unavailability:** If the configured agent CLI is not installed, Orbit fails with a clear error rather than falling back to another agent silently.

3. **Session context transfer:** Session context is not transferred between runs or between agents. Each run starts fresh.

4. **Agent-specific features:** Unique capabilities (Codex web search, Copilot GitHub integration) are not directly configurable through Orbit. They can be enabled through the agent's own configuration or via prompts.

---

## Dependencies

- Each agent CLI must be installed separately by the user
- No new Go dependencies required (reuses existing `os/exec`, `encoding/json`)
- Kiro and Copilot session format analysis requires sample session files
