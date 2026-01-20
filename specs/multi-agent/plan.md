# Multi-Agent Support - Plan

## Problem Statement

Orbit currently only supports Claude Code for orchestration and session viewing. Users working with multiple AI coding agents (OpenAI Codex CLI, AWS Kiro CLI, GitHub Copilot CLI) cannot use Orbit to orchestrate tasks or browse session history from these tools.

This feature extends Orbit to support multiple AI coding agents through a unified abstraction layer, enabling:

1. Orchestration of tasks using any supported agent
1. Session browsing and transcript viewing across all agents
1. Consistent error handling and retry logic per agent

## Supported Agents

|Agent         |CLI Command|Session Storage                               |Format         |
|--------------|-----------|----------------------------------------------|---------------|
|Claude Code   |`claude`   |`~/.claude/projects/<project>/*.jsonl`        |JSONL streaming|
|OpenAI Codex  |`codex`    |`~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl`|JSONL streaming|
|AWS Kiro      |`kiro-cli` |`~/.kiro/` + workspace-based                  |JSON export    |
|GitHub Copilot|`copilot`  |`~/.copilot/session-state/*.jsonl`            |JSONL          |

## Requirements

### 1. Agent Abstraction Layer

**User Story:** As a developer, I want Orbit to support multiple AI coding agents through a common interface, so that I can use my preferred agent without changing my workflow.

**Acceptance Criteria:**

1. <a name="1.1"></a>The system SHALL define an `Agent` interface in `internal/agents/agent.go`
1. <a name="1.2"></a>The interface SHALL include methods for: `Name()`, `CLICommand()`, `IsInstalled()`, `Version()`
1. <a name="1.3"></a>The interface SHALL include execution methods: `Run()` for arbitrary prompts, `Resume()` for continuing sessions
1. <a name="1.4"></a>The interface SHALL include session methods: `DefaultSessionDir()`, `DiscoverSessions()`, `ParseSession()`
1. <a name="1.5"></a>The system SHALL provide an agent registry for looking up agents by name
1. <a name="1.6"></a>Each supported agent SHALL have its own implementation package under `internal/agents/`
1. <a name="1.7"></a>The `Run()` method SHALL accept arbitrary prompts, supporting both rune phase orchestration and custom commands (e.g., post-completion)

-----

### 2. Claude Code Agent (Refactor)

**User Story:** As a developer, I want the existing Claude Code functionality preserved as the reference implementation, so that current Orbit users experience no disruption.

**Acceptance Criteria:**

1. <a name="2.1"></a>The system SHALL refactor `internal/claude/` into `internal/agents/claudecode/`
1. <a name="2.2"></a>The Claude Code agent SHALL implement the full `Agent` interface
1. <a name="2.3"></a>The system SHALL maintain backward compatibility with existing `.orbit/` log directories
1. <a name="2.4"></a>The orchestrator SHALL use the agent interface instead of direct claude client calls
1. <a name="2.5"></a>All existing tests SHALL pass after refactoring

-----

### 3. OpenAI Codex Agent

**User Story:** As a developer using OpenAI Codex CLI, I want Orbit to orchestrate my tasks and view my session history, so that I can use Codex with the same workflow as Claude Code.

**Acceptance Criteria:**

1. <a name="3.1"></a>The system SHALL provide a Codex agent implementation in `internal/agents/codex/`
1. <a name="3.2"></a>The agent SHALL invoke `codex exec` for non-interactive execution
1. <a name="3.3"></a>The agent SHALL pass `--full-auto` when auto-approve is enabled
1. <a name="3.4"></a>The agent SHALL support session resume via `codex resume --session-id`
1. <a name="3.5"></a>The agent SHALL discover sessions from `~/.codex/sessions/` with date-sharded structure
1. <a name="3.6"></a>The parser SHALL handle Codex JSONL format with `event_msg` and `response_item` types

-----

### 4. AWS Kiro Agent

**User Story:** As a developer using AWS Kiro CLI, I want Orbit to orchestrate my tasks and view my session history, so that I can use Kiro with the same workflow as Claude Code.

**Acceptance Criteria:**

1. <a name="4.1"></a>The system SHALL provide a Kiro agent implementation in `internal/agents/kiro/`
1. <a name="4.2"></a>The agent SHALL invoke `kiro-cli chat --no-interactive` for execution
1. <a name="4.3"></a>The agent SHALL pass `--trust-all-tools` when auto-approve is enabled
1. <a name="4.4"></a>The agent SHALL support session resume via `kiro-cli chat --resume`
1. <a name="4.5"></a>The agent SHALL discover sessions from workspace-based storage
1. <a name="4.6"></a>The parser SHALL handle Kiro JSON export format

-----

### 5. GitHub Copilot Agent

**User Story:** As a developer using GitHub Copilot CLI, I want Orbit to orchestrate my tasks and view my session history, so that I can use Copilot with the same workflow as Claude Code.

**Acceptance Criteria:**

1. <a name="5.1"></a>The system SHALL provide a Copilot agent implementation in `internal/agents/copilot/`
1. <a name="5.2"></a>The agent SHALL invoke `copilot -p` for non-interactive execution
1. <a name="5.3"></a>The agent SHALL pass `--allow-all-tools` when auto-approve is enabled
1. <a name="5.4"></a>The agent SHALL support session resume via `copilot --resume` or `--continue`
1. <a name="5.5"></a>The agent SHALL discover sessions from `~/.copilot/session-state/`
1. <a name="5.6"></a>The parser SHALL handle Copilot JSONL format

-----

### 6. Agent Selection

**User Story:** As a developer, I want to specify which agent to use for orchestration, so that I can choose the best tool for each project or task.

**Acceptance Criteria:**

1. <a name="6.1"></a>The `orbit run` command SHALL accept an `--agent` flag to specify the agent
1. <a name="6.2"></a>The system SHALL support agent configuration in `.orbit.yaml` via an `agent` key
1. <a name="6.3"></a>The CLI flag SHALL take precedence over configuration file
1. <a name="6.4"></a>The system SHALL default to `claude-code` when no agent is specified
1. <a name="6.5"></a>The system SHALL validate that the specified agent is installed before running
1. <a name="6.6"></a>The system SHALL emit a clear error message if the agent CLI is not found

-----

### 7. Session Discovery and Viewing

**User Story:** As a developer, I want to view session transcripts from any supported agent in both apsis and the web interface, so that I can review my work regardless of which agent I used.

**Acceptance Criteria:**

1. <a name="7.1"></a>The apsis CLI SHALL auto-detect the agent from session file content using the existing `DetectFormat()` pattern
1. <a name="7.2"></a>Auto-detection SHALL examine the first non-empty JSONL line’s `type` field:
- `user`, `assistant` → Claude Code (existing)
- `session_meta`, `response_item`, `event_msg`, `turn_context` → Codex (existing)
- Kiro and Copilot type values → To be determined during design phase (see note below)
1. <a name="7.3"></a>The `Format` enum SHALL be extended: `FormatKiro`, `FormatCopilot`
1. <a name="7.4"></a>The apsis CLI SHALL accept an `--agent` flag to override auto-detection when needed
1. <a name="7.5"></a>The web interface SHALL display sessions from all configured agents
1. <a name="7.6"></a>All agents SHALL render to the same unified Markdown/HTML format
1. <a name="7.7"></a>The system SHALL preserve agent-specific cost metrics in rendered output (tokens for Claude/Codex, credits/premium requests for Kiro/Copilot)

**Note:** Kiro and Copilot session format analysis must be completed during the design phase before implementation. The implementer should request example session files from the user to determine the type field values used by each agent.

-----

### 8. Error Handling

**User Story:** As a developer, I want Orbit to handle errors consistently across agents with appropriate retry logic, so that transient failures don’t interrupt my workflow.

**Acceptance Criteria:**

1. <a name="8.1"></a>The system SHALL extend `internal/errors/` with agent-specific error classifiers
1. <a name="8.2"></a>Each agent SHALL classify errors as: retryable, fatal, or session-invalid
1. <a name="8.3"></a>The orchestrator SHALL apply the same retry logic regardless of agent
1. <a name="8.4"></a>Rate limit errors SHALL be classified as retryable for all agents
1. <a name="8.5"></a>Authentication errors SHALL be classified as fatal for all agents

-----

### 9. Configuration

**User Story:** As a developer, I want to configure agent-specific settings, so that I can customize behavior per agent.

**Acceptance Criteria:**

1. <a name="9.1"></a>The system SHALL support an `agents` section in `.orbit.yaml` for per-agent configuration
1. <a name="9.2"></a>Each agent configuration SHALL support `cli-path` to override the default command
1. <a name="9.3"></a>Each agent configuration SHALL support `auto-approve` to control tool approval behavior
1. <a name="9.4"></a>Agent-specific options (e.g., `model`, `extra-args`) SHALL be passed through to the CLI

-----

### 10. Per-Variant Agent Selection

**User Story:** As a developer, I want to run different agents for different variants, so that I can compare implementations across agents.

**Acceptance Criteria:**

1. <a name="10.1"></a>The `orbit run --variants` command SHALL support a `--variant-agents` flag accepting a comma-separated list of agent names
1. <a name="10.2"></a>The guidance file (`guidance.yaml`) SHALL support an `agent` field per variant
1. <a name="10.3"></a>WHERE `--variant-agents` is specified, variant N SHALL use the Nth agent in the list (cycling if fewer agents than variants)
1. <a name="10.4"></a>WHERE variant guidance specifies an agent, it SHALL override the global `--agent` setting for that variant
1. <a name="10.5"></a>The `internal/variants/Variant` struct SHALL include an `Agent` field to track which agent is used
1. <a name="10.6"></a>The comparison report SHALL display the agent used for each variant
1. <a name="10.7"></a>The comparison prompt SHALL include agent information to provide context for the analysis

**Example usage:**

```bash
# Compare Claude Code vs Codex implementations
orbit run --variants 2 --variant-agents claude-code,codex

# Or via guidance.yaml
# guidance.yaml
variants:
  - id: 1
    agent: claude-code
    guidance: "Focus on readability"
  - id: 2
    agent: codex
    guidance: "Focus on readability"
```

-----

## Architecture

### Package Structure

```
internal/
├── agents/                     # NEW - Agent abstraction layer
│   ├── agent.go                # Agent interface definition
│   ├── registry.go             # Agent lookup and registration
│   ├── errors.go               # Agent-specific error types
│   ├── claudecode/
│   │   ├── client.go           # CLI wrapper (refactored from internal/claude/)
│   │   ├── parser.go           # JSONL parser
│   │   ├── sessions.go         # Session discovery
│   │   └── errors.go           # Error classification
│   ├── codex/
│   │   ├── client.go
│   │   ├── parser.go
│   │   ├── sessions.go
│   │   └── errors.go
│   ├── kiro/
│   │   ├── client.go
│   │   ├── parser.go
│   │   ├── sessions.go
│   │   └── errors.go
│   └── copilot/
│       ├── client.go
│       ├── parser.go
│       ├── sessions.go
│       └── errors.go
├── claude/                     # EXISTING - to be refactored into agents/claudecode/
│   └── client.go
├── transcript/                 # EXISTING - unified rendering, extend DetectFormat()
│   ├── parser.go               # Add FormatKiro, FormatCopilot
│   ├── codex_parser.go         # Already implemented
│   └── ...
├── variants/                   # EXISTING - worktree management (from multi-spec comparison)
│   ├── manager.go              # Update to use Agent interface
│   ├── types.go
│   └── git.go
├── comparison/                 # EXISTING - variant comparison (from multi-spec comparison)
│   ├── compare.go              # Uses Claude for comparison (keep as-is)
│   ├── diff.go
│   └── prompt.go
├── report/                     # EXISTING - HTML report generation (from multi-spec comparison)
│   ├── generator.go            # Add agent info to variant display
│   └── templates.go
├── orbit/                      # EXISTING - main orchestration
│   └── orbit.go                # Update to use Agent interface
├── logs/                       # EXISTING - session logging
│   └── manager.go              # Agent-aware storage
└── config/                     # EXISTING - configuration
    └── config.go               # Add agents section
```

### Agent Interface

```go
package agents

import (
    "context"
    "io"
    "time"
    
    "github.com/arjenschwarz/orbit/internal/transcript"
)

// Agent defines the interface for AI coding agent implementations.
type Agent interface {
    // Identity
    Name() string           // e.g., "claude-code", "codex", "kiro", "copilot"
    CLICommand() string     // e.g., "claude", "codex", "kiro-cli", "copilot"
    
    // Capability detection
    IsInstalled() bool
    Version() (string, error)
    
    // Session management
    DefaultSessionDir() string
    DiscoverSessions(projectDir string) ([]SessionInfo, error)
    ParseSession(r io.Reader) (*transcript.ParseResult, error)
    
    // Execution (supports arbitrary prompts, not just rune phases)
    Run(ctx context.Context, opts RunOptions) (*RunResult, error)
    Resume(ctx context.Context, sessionID string, opts RunOptions) (*RunResult, error)
}

// SessionInfo contains metadata about a discovered session.
type SessionInfo struct {
    ID        string
    Agent     string
    Path      string
    CreatedAt time.Time
    Size      int64
    Project   string    // If determinable from path
}

// RunOptions configures an execution.
type RunOptions struct {
    Prompt       string
    WorkDir      string
    SessionID    string              // Pre-generated UUID for tracking
    AutoApprove  bool
    Timeout      time.Duration
    Env          map[string]string
    ExtraArgs    []string            // Agent-specific CLI arguments
}

// RunResult contains the outcome of an execution.
type RunResult struct {
    SessionID    string
    ExitCode     int
    Duration     time.Duration
    Cost         *CostMetrics        // Optional, agent-specific cost tracking
    LogPath      string
    Error        error
    ErrorClass   ErrorClass          // retryable, fatal, session-invalid
}

// CostMetrics tracks usage costs in agent-specific units.
// Different agents use different billing models:
// - Claude/Codex: tokens (input/output) and USD cost
// - Kiro/Copilot: credits or premium requests
type CostMetrics struct {
    // Token-based (Claude, Codex)
    InputTokens  int
    OutputTokens int
    TotalTokens  int
    
    // Credit-based (Kiro, Copilot)
    Credits         float64
    PremiumRequests int
    
    // Universal
    CostUSD float64              // Estimated cost if calculable
}

// ErrorClass categorizes errors for retry logic.
type ErrorClass int

const (
    ErrorClassUnknown ErrorClass = iota
    ErrorClassRetryable          // Rate limits, transient network issues
    ErrorClassFatal              // Auth failures, invalid config
    ErrorClassSessionInvalid     // Session expired or not found
)
```

### Registry

```go
package agents

import "fmt"

var registry = make(map[string]func() Agent)

// Register adds an agent factory to the registry.
func Register(name string, factory func() Agent) {
    registry[name] = factory
}

// Get returns an agent by name.
func Get(name string) (Agent, error) {
    factory, ok := registry[name]
    if !ok {
        return nil, fmt.Errorf("unknown agent: %s", name)
    }
    return factory(), nil
}

// List returns all registered agent names.
func List() []string {
    names := make([]string, 0, len(registry))
    for name := range registry {
        names = append(names, name)
    }
    return names
}

// Default returns the default agent name.
func Default() string {
    return "claude-code"
}
```

### Configuration Schema

```yaml
# .orbit.yaml

# Default agent for this project
agent: claude-code

# Per-agent configuration
agents:
  claude-code:
    cli-path: claude                    # Override CLI path
    auto-approve: false                 # Require approval for tools
    
  codex:
    cli-path: codex
    auto-approve: true
    extra-args:                         # Additional CLI arguments
      - "--search"                      # Enable web search
    
  kiro:
    cli-path: kiro-cli
    model: auto                         # Model selection
    
  copilot:
    cli-path: copilot
    model: gpt-5
    auto-approve: false
```

-----

## CLI Invocation Patterns

### Claude Code

```bash
# Non-interactive execution
claude -p "implement phase 1" \
    --session-id $UUID \
    --dangerously-skip-permissions \  # if auto-approve
    --output-format json

# Resume
claude -p "continue" \
    --session-id $UUID \
    --resume
```

### OpenAI Codex

```bash
# Non-interactive execution
codex exec \
    --full-auto \                      # if auto-approve
    -p "implement phase 1"

# Resume
codex resume --session-id $UUID
```

### AWS Kiro

```bash
# Non-interactive execution
kiro-cli chat \
    --no-interactive \
    --trust-all-tools \                # if auto-approve
    "implement phase 1"

# Resume
kiro-cli chat --resume
```

### GitHub Copilot

```bash
# Non-interactive execution
copilot \
    --allow-all-tools \                # if auto-approve
    -p "implement phase 1"

# Resume
copilot --continue                     # Most recent session
copilot --resume                       # Session picker
```

-----

## Session Format Mapping

All agents’ session formats map to the unified `transcript.Entry` type:

|Source Field     |Claude Code                    |Codex                         |Kiro               |Copilot            |
|-----------------|-------------------------------|------------------------------|-------------------|-------------------|
|Entry type       |`type`                         |`type`                        |inferred           |`type`             |
|User message     |`message.role=="user"`         |`payload.type=="user_message"`|`role=="user"`     |`role=="user"`     |
|Assistant message|`message.role=="assistant"`    |`type=="response_item"`       |`role=="assistant"`|`role=="assistant"`|
|Tool use         |`content[].type=="tool_use"`   |`payload.type contains tool`  |`tool_calls[]`     |similar to Codex   |
|Tool result      |`content[].type=="tool_result"`|nested in response            |`tool_results[]`   |similar to Codex   |
|Timestamp        |`timestamp`                    |`timestamp`                   |`created_at`       |`timestamp`        |

### Format Detection Strategy

The existing `internal/transcript/parser.go` already implements content-based format detection via `DetectFormat()`. This reads the first non-empty JSONL line and examines the `type` field to determine the format. The multi-agent implementation extends this pattern:

```go
// Existing in parser.go
var claudeTypes = map[string]bool{"user": true, "assistant": true}
var codexTypes = map[string]bool{"session_meta": true, "response_item": true, "event_msg": true, "turn_context": true}

// To be added for new agents
var kiroTypes = map[string]bool{...}    // TBD: requires format analysis
var copilotTypes = map[string]bool{...} // TBD: requires format analysis
```

This approach is preferred over path-based detection because:

1. Works with piped input (stdin)
1. Works with copied/moved session files
1. Already proven with Claude/Codex implementation
1. Single source of truth for format identification

-----

## Existing Infrastructure

The multi-spec comparison feature has been implemented and provides infrastructure that the multi-agent support will build upon:

### Existing Packages

|Package               |Purpose                                |Multi-Agent Integration               |
|----------------------|---------------------------------------|--------------------------------------|
|`internal/variants/`  |Worktree management, parallel execution|Agent interface wraps execution logic |
|`internal/comparison/`|Diff gathering, Claude-based comparison|Comparator uses agent for comparison  |
|`internal/report/`    |HTML report generation                 |Agent-agnostic, reuse as-is           |
|`internal/claude/`    |Claude Code CLI wrapper                |Refactor to `agents/claudecode/`      |
|`internal/transcript/`|Unified JSONL parsing, rendering       |Extend `DetectFormat()` for new agents|

### Variant Manager Integration

The `internal/variants/Manager` orchestrates parallel execution:

```go
// Current implementation uses claude.Client directly
func (o *Orbit) runVariant(ctx context.Context, v *variants.Variant) error {
    claudeClient := claude.NewClient(claude.Config{
        WorkingDir: v.WorktreePath,
    })
    // ...
}
```

With multi-agent support, this becomes:

```go
func (o *Orbit) runVariant(ctx context.Context, v *variants.Variant) error {
    agent, err := agents.Get(v.Agent)  // Per-variant agent selection
    if err != nil {
        return err
    }
    // ...
    result, err := agent.Run(ctx, agents.RunOptions{
        WorkDir: v.WorktreePath,
        Prompt:  prompt,
    })
    // ...
}
```

### Comparison Integration

The `internal/comparison/Comparator` currently uses `claude.Client` for analysis. This will be updated to use the agent interface, with Claude remaining the default for comparison:

```go
// Comparator uses agent interface for comparison
type Comparator struct {
    agent     agents.Agent      // Configurable, defaults to Claude
    customCmd string
}

// Usage
comparator := comparison.NewComparator(agents.Get("claude-code"), "")
// Or with a different agent:
comparator := comparison.NewComparator(agents.Get("codex"), "")
```

The comparison agent is configured separately from the implementation agent, allowing flexibility (e.g., use Codex for implementation but Claude for comparison analysis).

### CLI Commands Already Implemented

|Command                 |Description                   |Agent Integration                             |
|------------------------|------------------------------|----------------------------------------------|
|`orbit run --variants N`|Run N parallel implementations|Add `--agent` and `--variant-agents` flags    |
|`orbit status <spec>`   |Show variant status           |Show agent per variant                        |
|`orbit cleanup <spec>`  |Remove worktrees              |Agent-agnostic                                |
|`orbit finalize <spec>` |Adopt chosen variant          |Agent-agnostic                                |
|`orbit compare <spec>`  |Regenerate comparison         |Uses comparison agent, shows agent per variant|

### Key Files to Modify

|File                           |Change                                               |
|-------------------------------|-----------------------------------------------------|
|`internal/orbit/orbit.go`      |Replace `claude.Client` with `agents.Agent` interface|
|`internal/orbit/variants.go`   |Update `runVariant()` to use per-variant agent       |
|`internal/variants/types.go`   |Add `Agent` field to `Variant` struct                |
|`cmd/orbit/run.go`             |Add `--agent` and `--variant-agents` flags           |
|`internal/config/config.go`    |Add `agents` section, parse guidance.yaml agent field|
|`internal/comparison/prompt.go`|Include agent info in comparison prompt              |
|`internal/report/generator.go` |Display agent used per variant                       |

-----

## Implementation Phases

### Phase 1: Agent Abstraction (Foundation)

**Goal:** Extract Claude Code into agent interface without behavior change.

1. Create `internal/agents/agent.go` with interface definition
1. Create `internal/agents/registry.go` for agent lookup
1. Move `internal/claude/` to `internal/agents/claudecode/`
1. Implement `Agent` interface for Claude Code (wrap existing client)
1. Update `internal/orbit/orbit.go` to use agent interface
1. Update `internal/orbit/variants.go` (`runVariant()`) to use agent interface
1. Update imports throughout codebase
1. Verify all existing tests pass (including variant/comparison tests)

**Deliverable:** Orbit and variant execution work exactly as before, but through agent abstraction.

### Phase 2: Session Parsing (Viewing)

**Goal:** Support viewing sessions from all agents in apsis and web.

1. Analyze Kiro and Copilot session formats to identify type field values
1. Extend `DetectFormat()` in `internal/transcript/parser.go` with `FormatKiro` and `FormatCopilot`
1. Implement `internal/agents/codex/parser.go` (leverage existing `ParseCodexJSONL`)
1. Implement `internal/agents/copilot/parser.go`
1. Implement `internal/agents/kiro/parser.go`
1. Add session discovery for each agent
1. Extend apsis with `--agent` flag for override
1. Update web interface to show multi-agent sessions

**Deliverable:** View transcripts from any agent.

### Phase 3: Orchestration Support

**Goal:** Run and resume tasks with any agent, including per-variant agent selection.

1. Implement `Run()` for Codex agent (based on CLI research)
1. Implement `Run()` for Copilot agent
1. Implement `Run()` for Kiro agent
1. Implement `Resume()` for each agent
1. Add `--agent` flag to `orbit run` (works with `--variants`)
1. Add `--variant-agents` flag for per-variant agent selection
1. Update guidance.yaml parsing to support `agent` field per variant
1. Update `internal/variants/Variant` struct with `Agent` field
1. Add agent configuration to `.orbit.yaml`
1. Implement error classification per agent (for retry logic)

**Deliverable:** Orchestrate tasks with any supported agent, including mixed-agent variant comparison.

### Phase 4: Polish and Edge Cases

**Goal:** Production-ready multi-agent support.

1. Auto-detect agent from existing `.orbit/` logs
1. Unified cost tracking (tokens for Claude/Codex, credits for Kiro/Copilot)
1. Agent capability detection (some features may not exist on all agents)
1. Update comparison prompt to include agent information per variant
1. Update comparison report to display agent used per variant
1. Documentation and examples
1. Integration tests with each agent CLI

**Deliverable:** Complete multi-agent support.

-----

## Testing Strategy

### Unit Tests

|Component          |Test                                              |Requirements    |
|-------------------|--------------------------------------------------|----------------|
|`agents/registry`  |TestRegister, TestGet, TestList                   |1.5             |
|`agents/claudecode`|TestRun, TestResume, TestParseSession             |2.2-2.5         |
|`agents/codex`     |TestRun, TestParseSession, TestDiscoverSessions   |3.2-3.6         |
|`agents/kiro`      |TestRun, TestParseSession                         |4.2-4.6         |
|`agents/copilot`   |TestRun, TestParseSession, TestDiscoverSessions   |5.2-5.6         |
|`config`           |TestAgentConfig, TestAgentSelection               |6.1-6.4, 9.1-9.4|
|`variants`         |TestVariantAgentParsing, TestGuidanceAgentOverride|10.1-10.4       |

### Integration Tests

|Test                                |Description                              |Requirements |
|------------------------------------|-----------------------------------------|-------------|
|TestClaudeCodeOrchestration         |Full phase execution with Claude         |2.*          |
|TestCodexOrchestration              |Full phase execution with Codex          |3.*          |
|TestKiroOrchestration               |Full phase execution with Kiro           |4.*          |
|TestCopilotOrchestration            |Full phase execution with Copilot        |5.*          |
|TestAgentNotInstalled               |Error handling when CLI missing          |6.5, 6.6     |
|TestSessionAutoDetect               |Detect agent from session file content   |7.1, 7.2, 7.3|
|TestPerVariantAgentSelection        |Each variant uses its configured agent   |10.1-10.5    |
|TestVariantComparisonWithMixedAgents|Comparison report shows agent per variant|10.6, 10.7   |

### Parser Golden File Tests

Each agent parser should have golden file tests with sample JSONL/JSON input and expected `transcript.Entry` output, similar to existing `internal/transcript/testdata/`.

-----

## Scope Decisions

1. **Mixed-agent orchestration within phases:** Out of scope. A single orchestration run uses one agent for all phases of a single variant. Users can switch agents between runs.
1. **Agent fallback on unavailability:** If the configured agent CLI is not installed, Orbit fails with a clear error rather than falling back silently.
1. **Session context transfer:** Out of scope. Session context is not transferred between runs, regardless of whether the same agent or a different agent is used. This is existing behavior.
1. **Agent-specific features:** Out of scope for direct configuration. Unique capabilities (Codex web search, Copilot GitHub integration) can be enabled through prompts or the agent’s own configuration files.

-----

## Future Improvements

These features are explicitly out of scope for the initial implementation but may be valuable additions later.

### Agent Fallback on Rate Limits

**Concept:** When an agent hits an API rate limit, Orbit could automatically switch to another configured agent to continue the orchestration without manual intervention.

**Use case:** User is running a long orchestration with Claude Code, hits the rate limit at Phase 3 of 5. Instead of waiting or failing, Orbit switches to Codex for the remaining phases.

**Implementation sketch:**

```yaml
# .orbit.yaml
agent: claude-code
fallback:
  enabled: true
  agents: [codex, copilot]        # Fallback order
  triggers:
    - rate-limit
    - overloaded
```

**Considerations:**

- Context from prior phases would need to be summarized/injected for the new agent
- Different agents may interpret the same task differently
- Cost implications if fallback agent is more expensive
- Session continuity becomes complex (multiple agent logs per run)

### Mixed-Agent Orchestration

**Concept:** Allow specifying different agents for different phases within a single run.

**Use case:** Use Claude for planning phases (better reasoning), Codex for implementation phases (faster execution).

```yaml
# tasks.md front matter or .orbit.yaml
phases:
  1:
    agent: claude-code
  2-4:
    agent: codex
  5:
    agent: claude-code
```

### Cross-Agent Session Context

**Concept:** When switching agents (either via fallback or user choice), automatically extract and inject relevant context from prior sessions.

**Implementation:** Generate a summary of completed phases and inject it into the new agent’s system prompt or instructions file.

-----

## Dependencies

- Each agent CLI must be installed separately by the user
- No new Go dependencies expected (reuse existing `os/exec`, `encoding/json`)
- May need to vendor agent-specific JSONL schemas if not publicly documented

-----

## Risks and Mitigations

|Risk                                      |Likelihood|Impact|Mitigation                                          |
|------------------------------------------|----------|------|----------------------------------------------------|
|Agent CLI changes break parsing           |Medium    |High  |Version detection, graceful degradation             |
|Session format undocumented               |Medium    |Medium|Reverse-engineer from samples, monitor for changes  |
|Agent doesn’t support non-interactive mode|Low       |High  |Research before implementation, document limitations|
|Performance varies significantly by agent |Medium    |Low   |Document expectations, allow timeout configuration  |