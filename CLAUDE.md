# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This repository contains two related CLI tools for working with AI coding agents:

- **Orbit** - Orchestrates AI coding agent sessions to implement spec phases sequentially. Supports Claude Code, OpenAI Codex, AWS Kiro, GitHub Copilot, and OpenCode. Handles session lifecycle, error recovery, log management, and multi-variant comparison runs. Includes a web interface for viewing runs and transcripts.
- **Apsis** - Converts AI coding agent session transcripts to readable Markdown or HTML. Supports Claude Code (JSONL), OpenAI Codex (JSONL), GitHub Copilot (JSONL), AWS Kiro CLI (SQLite), and Kiro IDE sessions. Includes a web interface (`apsis serve`) for browsing sessions.

## Build and Development Commands

```bash
make build          # Build both binaries (orbit and apsis)
make build-orbit    # Build orbit only
make build-apsis    # Build apsis only
make test           # Run all tests (~10s total)
make test-short     # Run tests with -short flag
make test-verbose   # Run tests with verbose output
make test-coverage  # Run tests with coverage report
make lint           # Run golangci-lint
make modernize      # Update code to modern Go idioms
make install        # Install both binaries to GOPATH/bin
make clean          # Remove build artifacts

# Run single test
go test ./internal/orbit -run TestName
```

## Testing Guidelines

When adding tests, ensure they provide actual value:

- **Do NOT create documentation-only tests** - Tests that only contain `t.Log()` statements documenting expected behavior are not real tests. If a test doesn't have assertions that can fail, it shouldn't exist.
- **Tests must verify behavior** - Every test should have assertions (`t.Error`, `t.Errorf`, `t.Fatal`, etc.) that verify actual behavior matches expected behavior.
- **Prefer testing real functionality** - Use mocks to isolate dependencies, but test actual code paths. Tests that just verify struct field initialization or duplicate validation logic from production code add little value.
- **If proper testing isn't feasible, skip the test** - It's better to have no test than a fake test that gives false confidence. Document the gap and create a spec to address it properly.

## Testing with testutil

Orbit uses a custom test framework in `internal/testutil/` for integration testing. The framework provides mock agents that simulate real agent behavior without invoking CLIs.

### Quick Start

```go
func TestMyFeature(t *testing.T) {
    // 1. Define expected agent behavior
    scenario := testutil.NewScenario().
        Success("session-1", 0.05).
        Success("session-2", 0.03).
        Build()

    // 2. Create test agent and orbit
    agent := testutil.NewTestAgent(t, "mock", scenario)
    t.Cleanup(func() { agent.AssertAllConsumed(t) })
    orbit := orbithelpers.CreateTestOrbit(t, orbithelpers.WithAgent(agent))

    // 3. Run and verify
    err := orbit.Run()
    require.NoError(t, err)
    agent.Recorder().AssertCallCount(t, 2)
}
```

### Common Patterns

**Testing Error Recovery:**
```go
scenario := testutil.NewScenario().
    RetryableError("timeout").
    RetryableError("timeout").
    Success("session-1", 0.05).
    Build()
```

**Multiple Identical Responses:**
```go
scenario := testutil.NewScenario().
    Success("session-1", 0.05).Repeat(5).
    Build()
```

**Testing with Timing (FakeClock):**
```go
clock := testutil.NewFakeClock(time.Now())
agent := testutil.NewTestAgent(t, "mock", scenario, testutil.WithClock(clock))
orbit := orbithelpers.CreateTestOrbit(t,
    orbithelpers.WithAgent(agent),
    orbithelpers.WithOrbitClock(clock),
)
// After run, verify backoff durations
clock.AssertSleeps(t, []time.Duration{time.Second, 2*time.Second})
```

See `internal/testutil/doc.go` for complete API documentation including error injection, multi-variant testing, and property-based testing with rapid.

## Architecture

The codebase follows a clean internal package structure:

```
cmd/
  orbit/main.go      - Orbit CLI entry point, subcommand routing (run, serve, register, init, demo, status, cleanup, finalize, compare, consolidate)
  apsis/main.go      - Apsis CLI entry point, session listing, conversion, and web server
internal/
  agents/            - Agent abstraction layer
    agent.go         - Agent interface definition (Run, Resume, IsInstalled, etc.)
    registry.go      - Agent factory and lookup
    errors.go        - Shared error classification (ErrorClass types, ParseRetryAfter, classifier registry)
    executor.go      - Common agent execution (build command, capture stdout/stderr, extract exit code)
    retry.go         - Shared retry executor (RunWithRetry with backoff, rate-limit handling, context-aware sleep)
    claudecode/      - Claude Code agent implementation
    codex/           - OpenAI Codex agent implementation
    kiro/            - AWS Kiro agent implementation (includes kiro/logs/ for SQLite session discovery)
    copilot/         - GitHub Copilot agent implementation
    opencode/        - OpenCode agent implementation
  orbit/             - Main orchestration loop
    orbit.go         - Orchestration loop with retry logic
    shell.go         - Shell command execution (pre/post-command hooks)
  variants/          - Multi-variant comparison support
    types.go         - Variant struct and status types
    manager.go       - Variant lifecycle (create, run, finalize, cleanup)
    agent.go         - Per-variant agent assignment
    git.go           - Git operations (commits, dirty state)
  status/            - Enhanced status command support
    gatherer.go      - Collects variant status data (git, transcript, tasks)
    types.go         - Status data structures and output types
  comparison/        - Variant comparison logic
    compare.go       - Comparator for analyzing variant diffs
    adapter.go       - AgentAdapter wrapping agents.Agent for Comparator
    diff.go          - DiffGatherer for collecting variant diffs
    learnings.go     - Learning extraction from variant implementations
    prompt.go        - Comparison prompt generation
    types.go         - Result, VariantData, VariantLearning, CrossVariantImprovement types
  consolidation/     - Variant consolidation support
    consolidator.go  - Applies improvements from non-chosen variants
    git.go           - GitOps interface and implementation for git operations
    prompt.go        - AI prompt generation for consolidation
    recovery.go      - Rollback support for failed consolidations
    logger.go        - Consolidation logging and history
    types.go         - Config, result, and report types
  report/            - Comparison report generation
  cost/              - Cost formatting utilities for multi-unit cost display (USD, credits, premium requests)
  sessions/          - Unified session listing and resolution across all agents
    types.go         - SessionInfo, SessionMetadata, ResolvedSession types
    lister.go        - Session discovery across Claude, Codex, Copilot, Kiro CLI, Kiro IDE
    resolver.go      - Session resolution (ID or file path to parsed transcript)
  apsisweb/          - Apsis web interface (apsis serve)
    server.go        - HTTP server with middleware
    handlers.go      - Request handlers for session listing and viewing
    templates/       - HTML templates
    static/          - Static assets (CSS, JS)
  rune/client.go     - Wrapper for rune CLI task management
  logs/manager.go    - Session log storage and summary management
  config/config.go   - Configuration loading via Viper (files, env vars, defaults)
  debug/             - Debug logging utilities
    debug.go         - Debug flag and Printf
    entry.go         - Structured log entries
    writer.go        - Log file writer
  display/           - Terminal display utilities
    spinner.go       - Progress spinner for long operations
    hyperlink.go     - Terminal hyperlink support (OSC 8)
  transcript/        - JSONL parsing and Markdown/HTML rendering for apsis and web
    last_entry.go    - Efficient last entry extraction from live transcripts
    follow.go        - File following for live transcript updates
  registry/          - Run registry for tracking orbit runs across repositories
  web/               - HTTP server, handlers, templates for Orbit web interface
  testutil/          - Test framework with mock agents and helpers
    scenario.go      - ScenarioBuilder fluent API for defining agent behavior
    agent.go         - TestAgent implementing agents.Agent
    clock.go         - FakeClock for deterministic time testing
    recorder.go      - Call tracking and assertions
    generators.go    - Property-based testing generators (rapid)
    orbithelpers/    - Orbit construction helpers (avoids import cycles)
```

### Orbit Flow

`main.go` parses flags and detects tasks file from git branch → resolves agent (claude-code, codex, kiro, copilot, opencode) → `Orbit.Run()` loops through phases → `agent.Run()` executes the configured agent with `/next-task --phase` (agents use shared `executor.Execute()` for CLI invocation) → errors are classified and retried via `agents.RunWithRetry()` → logs are saved per phase.

For multi-variant runs: `main.go` creates a `variants.Manager` → creates worktrees for each variant → runs orchestration in each worktree (optionally in parallel) → collects diffs and runs comparison → generates comparison report.

### Apsis Flow

Resolves input (session ID, file path, or stdin) → uses `sessions.Resolver` to locate and parse the transcript → renders to Markdown or HTML via `transcript.RenderMarkdown()` / `transcript.RenderHTML()` → outputs to stdout or file. The `apsis serve` subcommand starts a web interface via `apsisweb.Server` for browsing sessions across all supported agents.

## Tasks File Auto-Detection (Orbit)

When `--tasks-file` is not specified, Orbit detects it from the git branch:
1. Get current branch name
2. Strip everything before the first `/` (e.g., `feature/my-feature` → `my-feature`)
3. Look for `specs/{name}/tasks.md` or `specs/{name}/TASKS.md`

## Configuration (Orbit)

Configuration priority (highest to lowest):
1. CLI flags (`--agent`, `--command`, `--pre-prompt`, `--post-prompt`, `--auto-consolidate`, etc.)
2. Environment variables (`ORBIT_AGENT`, `ORBIT_COMMAND`, `ORBIT_PRE_PROMPT`, `ORBIT_POST_PROMPT`, `ORBIT_DATE_SUBDIRS`, `ORBIT_CONTINUE_SESSION`, `ORBIT_AUTO_CONSOLIDATE`, `ORBIT_POST_CONSOLIDATE_COMMAND`)
3. Project config (`.orbit.yaml` in working directory)
4. Home config (`~/.orbit.yaml`)
5. Built-in defaults

Empty string environment variables explicitly disable features (e.g., `ORBIT_POST_PROMPT=""` disables post-prompt).

Agent configuration in `.orbit.yaml` supports per-agent settings under the `agents` key with `cli-path`, `auto-approve` (default: `true`), `timeout`, `extra-args`, `pre-command`, and `post-command` options. Auto-approve is enabled by default to allow non-interactive operation.

## Command Hooks and Prompts (Orbit)

Orbit supports both shell command hooks and AI prompts at different stages of execution. Understanding the distinction is important:

- **Commands** (pre-command, post-command) - Shell commands executed on the host system
- **Prompts** (pre-prompt, post-prompt) - AI agent interactions

### Execution Order

When all hooks and prompts are configured, they execute in this order:

1. **Agent pre-command** (shell) - Runs once before any phases
2. **Global pre-prompt** (AI) - Runs once, starts session that phase 1 continues
3. **Phase loop** - Phases 1 through N execute sequentially
4. **Global post-prompt** (AI) - Runs once after all phases complete
5. **Agent post-command** (shell) - Runs once after post-prompt

Any unconfigured hook or prompt is skipped.

### Global Prompts (AI Agent Interactions)

Global prompts are AI agent interactions that run before or after the phase loop:

**Pre-Prompt:**
- Runs before the first phase begins
- Starts a new agent session that phase 1 continues
- Useful for initial codebase review or context setup
- Failure aborts the run
- Configuration:
  - Config file: `pre-prompt: "Review the codebase..."`
  - CLI flag: `--pre-prompt "Review the codebase..."`
  - Environment: `ORBIT_PRE_PROMPT="Review the codebase..."`
  - Disable: `--no-pre-prompt`
- No default value (empty by default)
- If resuming an interrupted run where pre-prompt already completed, it will not re-run

**Post-Prompt:**
- Runs after all phases complete
- Uses the same session as the last phase
- Useful for final review, testing, or cleanup
- Failure is retried (up to 5 attempts with exponential backoff)
- Configuration:
  - Config file: `post-prompt: "Review implementation..."`
  - CLI flag: `--post-prompt "Review implementation..."`
  - Environment: `ORBIT_POST_PROMPT="Review implementation..."`
  - Disable: `--no-post-prompt`
- Default: "Review the implementation to verify it meets the requirements and all tests pass. If issues are found, fix them."

### Agent-Level Commands (Shell Execution)

Agent-level commands are shell commands that run on the host system. They are configured per-agent and only execute for the configured agent:

**Pre-Command:**
- Runs once before the first phase (before pre-prompt if configured)
- Executes as a shell command: `/bin/sh -c "<command>"`
- Working directory: repository root (or worktree root in variant mode)
- Environment variables available:
  - `ORBIT_PHASE_COUNT` - total number of phases
  - `ORBIT_AGENT` - agent name being used
  - `ORBIT_VARIANT` - variant ID (in variant mode only)
- Failure (non-zero exit code) aborts the run
- Configuration: under `agents.<agent-name>.pre-command` in `.orbit.yaml`
- No default value (empty by default)
- Example use cases: run linters, update dependencies, verify environment

**Post-Command:**
- Runs once after the last phase (after post-prompt if configured)
- Same execution environment as pre-command
- Failure logs a warning but marks run as completed
- Configuration: under `agents.<agent-name>.post-command` in `.orbit.yaml`
- No default value (empty by default)
- Example use cases: run tests, format code, generate documentation

**Important Notes:**
- Commands must be non-interactive (no user input)
- Commands have a configurable timeout (default: 5 minutes)
- Commands are agent-specific (no global commands that apply to all agents)
- Empty string values are treated as not configured (no-op)
- In dry-run mode (`--dry-run`), commands are printed but not executed

### Command Timeout Configuration

Shell commands have a configurable timeout to prevent hanging:

- Default: 5 minutes
- Config file: `command-timeout: "10m"` (duration string)
- Environment: `ORBIT_COMMAND_TIMEOUT="15m"`
- If a command exceeds the timeout, it is terminated and treated as a failure

Supported duration formats: `"5m"`, `"1h"`, `"30s"`, `"1h30m"`

### Configuration Example

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

### Variant Mode Behavior

In variant mode, each variant executes its own complete hook sequence independently:
- Each variant runs in its own worktree
- Pre-commands and post-commands execute in the variant's worktree
- Pre-prompts and post-prompts execute with the variant's working directory
- `ORBIT_VARIANT` environment variable is set to the variant ID
- In parallel mode, hooks may run concurrently across variants

### Hook Logging

Hook and prompt execution is logged:
- Pre-command output: `.orbit/pre-command-run-N.txt`
- Post-command output: `.orbit/post-command-run-N.txt`
- Pre-prompt and post-prompt: included in phase transcripts
- Shell command metadata (exit code, duration) tracked in `summary.json`

### Migration from Deprecated post-command

The top-level `post-command` configuration has been renamed to `post-prompt` for clarity. The old configuration is no longer supported:

**Deprecated (will cause error):**
```yaml
post-command: "Review implementation..."  # Top-level - ERROR
```

**New (correct):**
```yaml
post-prompt: "Review implementation..."   # Top-level - AI prompt

agents:
  claude-code:
    post-command: "make test"             # Agent-level - shell command (OK)
```

If you have existing configuration using top-level `post-command`, `ORBIT_POST_COMMAND` environment variable, or `--post-command` CLI flag, Orbit will exit with a clear error message explaining how to migrate.

## External Dependencies

Orbit requires:
- **rune** - task management CLI for reading/tracking task phases
- At least one AI coding agent CLI:
  - **Claude Code CLI** (`claude`) - default agent
  - **OpenAI Codex** (`codex`)
  - **AWS Kiro** (`kiro-cli`)
  - **GitHub Copilot** (`copilot`)
  - **OpenCode** (`opencode`) - open-source agent supporting multiple LLM providers

Apsis has no external dependencies beyond access to agent session directories (`~/.claude/projects/`, `~/.codex/sessions/`, etc.).

## Error Handling (Orbit)

Error classification is shared across all agents via `internal/agents/errors.go`, with agent-specific classifiers in `internal/agents/{agent}/errors.go`. Errors are classified into:
- `ErrorClassRetryable` - transient errors (rate limits, connection issues) - exponential backoff
- `ErrorClassFatal` - permanent errors (auth failures, invalid config) - stops immediately
- `ErrorClassSessionInvalid` - session expired or not found - retries with fresh session
- `ErrorClassRateLimitWait` - usage limits that reset at a specific time - waits until reset, then continues

Retry logic is centralized in `internal/agents/retry.go` via `RunWithRetry()`, which handles exponential backoff, rate-limit wait resets (capped at 5), and context-aware interruptible sleep. Non-retryable errors stop orchestration and preserve state for manual intervention.

### Usage Limit Handling (Claude Code)

Claude Code displays session-limit messages like "You've hit your session limit · resets 3am (Australia/Melbourne)". Orbit also accepts the older "You've hit your limit" wording. When this occurs, Orbit will:
1. Parse the reset time and timezone from the error message
2. Calculate the wait duration until that time (plus a 1-minute buffer)
3. Wait automatically until the limit resets
4. Resume execution without counting against the normal retry limit

Supported time formats: `3am`, `12:30pm`, `3:00 am`
Supported timezones: IANA names (e.g., `Australia/Melbourne`, `America/New_York`) and common abbreviations (EST, PST, UTC, AEST, etc.)

This feature is currently implemented for Claude Code only, as other agents (Codex, Kiro, Copilot, OpenCode) use standard rate limiting with retry-after headers rather than time-based usage limits. The infrastructure (`ErrorClassRateLimitWait`) is available for other agents to use if they implement similar patterns.

### Comparison Failure Recovery

When variant comparison fails (agent timeout, garbled response, JSON validation failure), `CompareUnified()` automatically checks if the agent wrote the comparison JSON file (`specs/<name>/.orbit/comparison.json`) before failing. If the file was created or updated during the comparison, it is loaded as a fallback result instead of propagating the error. A pre-existing file from a previous run is not used unless its mod-time shows it was updated during the current comparison. Both `orbit run --variants` and `orbit compare` benefit from this fallback.

## Log Structure (Orbit)

Sessions are saved to `.orbit/` next to the tasks file (e.g., `specs/my-feature/.orbit/`):
- `summary.json` - persistent run summary with session tracking and agent info
- `phase-N-run-M-session.json` - full agent output for phase N, run M
- `phase-N-run-M-session.txt` - human-readable transcript

With `--date-subdirs`, logs are organized by timestamp subdirectories instead.

## Web Interface (Orbit)

Orbit includes a web interface (`orbit serve`) for viewing runs and transcripts:
- Dashboard showing all runs grouped by repository
- Run detail page with phase status and summary
- Transcript viewer with navigation between phases
- HTMX-powered auto-refresh for running sessions
- Mobile-responsive design with dark mode support

## Run Registry (Orbit)

Runs are tracked in `~/.orbit/runs/` as individual JSON files (one per run).
- Auto-registered during orchestration
- Manually registered via `orbit register`
- Registry failures are non-fatal (logged as warnings)
- Atomic writes using temp file + rename pattern

## Multi-Variant Workflow (Orbit)

Orbit supports running multiple implementation variants with different agents or guidance:
- `orbit run --variants N` creates N git worktrees and runs implementation in each
- `--variant-agents agent1,agent2` assigns different agents to variants (cycles if fewer than variants)
- `--guidance-file guidance.yaml` provides per-variant instructions
- `--parallel` runs variants concurrently (limited by `--max-parallel`)

Variant workflow (in order):
1. `orbit run --variants N` - create and run variants
2. `orbit status <spec>` - monitor variant progress and status
3. `orbit compare <spec>` - regenerate comparison report (auto-generated after run)
4. `orbit consolidate <spec> --variant N` - (optional) merge improvements from other variants
5. `orbit finalize <spec> --variant N` - adopt variant N and clean up others
6. `orbit cleanup <spec>` - removes all variant worktrees and branches (alternative to finalize)

### Auto-Consolidation

Orbit can automatically run consolidation on the recommended variant after comparison:
- `--auto-consolidate` flag enables automatic consolidation after comparison
- `--no-auto-consolidate` disables when enabled via config
- `--allow-dirty` allows consolidation even if worktree has uncommitted changes
- `auto-consolidate: true` in `.orbit.yaml` enables by default
- `post-consolidate-command` in `.orbit.yaml` specifies a shell command to run after consolidation

Environment variables:
- `ORBIT_AUTO_CONSOLIDATE=true` enables auto-consolidation
- `ORBIT_POST_CONSOLIDATE_COMMAND="make test"` sets the post-consolidate command

Auto-consolidation:
- Requires `--variants` to be specified (validation error otherwise)
- Skips gracefully when fewer than 2 variants succeed
- Skips if worktree has uncommitted changes (unless `--allow-dirty`)
- Runs `post-consolidate-command` after consolidation (even if no improvements found)
- Failures are non-fatal; variant run continues to report generation

### Variant Session Recovery

When re-running `orbit run --variants` with an existing variant run, you're prompted to choose:
- **Continue [c]**: Resume from where it left off, keeping all variants as-is
- **New run [n]**: Restart unfinished variants only - completed variants are preserved, while pending/failed/running variants are cleaned up and recreated
- **Cancel [q]**: Abort without changes

This allows recovering from partial failures without losing completed work.

When a run is interrupted (e.g., Ctrl+C), variant status is preserved as follows:
- **Running variants**: Marked as "canceled" (they were mid-execution)
- **Pending variants**: Remain "pending" (never started, can be picked up by "continue")
- **Completed variants**: Unchanged

This means interrupting a run and choosing "continue" will naturally re-execute any variants that hadn't started yet, without requiring a full "new run".

Consolidation allows merging good ideas from non-chosen variants:
- Reads cross-variant improvements from comparison report
- Agent applies beneficial changes to chosen variant
- Creates consolidation commit with applied improvements
- Supports `--rollback` to revert last consolidation

Variants are stored in `specs/{spec}/.orbit/worktrees/` with metadata in `variants.json`.
