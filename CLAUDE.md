# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This repository contains two related CLI tools for working with AI coding agents:

- **Orbit** - Orchestrates AI coding agent sessions to implement spec phases sequentially. Supports Claude Code, OpenAI Codex, AWS Kiro, GitHub Copilot, and OpenCode. Handles session lifecycle, error recovery, log management, and multi-variant comparison runs. Includes a web interface for viewing runs and transcripts.
- **Apsis** - Converts AI coding agent session transcripts to readable Markdown or HTML. Supports Claude Code (JSONL), OpenAI Codex (JSONL), and AWS Kiro (SQLite) sessions.

## Build and Development Commands

```bash
make build          # Build both binaries (orbit and apsis)
make build-orbit    # Build orbit only
make build-apsis    # Build apsis only (with version injection)
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

## Architecture

The codebase follows a clean internal package structure:

```
cmd/
  orbit/main.go      - Orbit CLI entry point, subcommand routing (run, serve, register, init, demo, status, cleanup, finalize, compare, consolidate)
  apsis/main.go      - Apsis CLI entry point, session listing and conversion
internal/
  agents/            - Agent abstraction layer
    agent.go         - Agent interface definition (Run, Resume, IsInstalled, etc.)
    registry.go      - Agent factory and lookup
    errors.go        - Error classification types
    claudecode/      - Claude Code agent implementation
    codex/           - OpenAI Codex agent implementation
    kiro/            - AWS Kiro agent implementation
    copilot/         - GitHub Copilot agent implementation
    opencode/        - OpenCode agent implementation
  orbit/orbit.go     - Main orchestration loop with retry logic
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
  consolidation/     - Variant consolidation support
    consolidator.go  - Applies improvements from non-chosen variants
    prompt.go        - AI prompt generation for consolidation
    recovery.go      - Rollback support for failed consolidations
    logger.go        - Consolidation logging and history
    types.go         - Config, result, and report types
  report/            - Comparison report generation
  claude/client.go   - Legacy Claude Code CLI wrapper (used by comparison)
  claude/paths.go    - Claude project path utilities (shared by orbit and apsis)
  rune/client.go     - Wrapper for rune CLI task management
  errors/errors.go   - Legacy error classification (deprecated, use agents/errors)
  logs/manager.go    - Session log storage and summary management
  config/config.go   - Configuration loading via Viper (files, env vars, defaults)
  debug/debug.go     - Debug logging utilities
  display/           - Terminal display utilities
    spinner.go       - Progress spinner for long operations
    hyperlink.go     - Terminal hyperlink support (OSC 8)
  transcript/        - JSONL parsing and Markdown/HTML rendering for apsis and web
    last_entry.go    - Efficient last entry extraction from live transcripts
  registry/          - Run registry for tracking orbit runs across repositories
  web/               - HTTP server, handlers, templates for web interface
```

### Orbit Flow

`main.go` parses flags and detects tasks file from git branch → resolves agent (claude-code, codex, kiro, copilot, opencode) → `Orbit.Run()` loops through phases → `agent.Run()` executes the configured agent with `/next-task --phase` → agent-specific errors are classified and retried or propagated → logs are saved per phase.

For multi-variant runs: `main.go` creates a `variants.Manager` → creates worktrees for each variant → runs orchestration in each worktree (optionally in parallel) → collects diffs and runs comparison → generates comparison report.

### Apsis Flow

Resolves input (session ID, file path, or stdin) → parses JSONL via `transcript.ParseJSONL()` → renders to Markdown via `transcript.RenderMarkdown()` → outputs to stdout or file.

## Tasks File Auto-Detection (Orbit)

When `--tasks-file` is not specified, Orbit detects it from the git branch:
1. Get current branch name
2. Strip everything before the first `/` (e.g., `feature/my-feature` → `my-feature`)
3. Look for `specs/{name}/tasks.md` or `specs/{name}/TASKS.md`

## Configuration (Orbit)

Configuration priority (highest to lowest):
1. CLI flags (`--agent`, `--command`, `--pre-prompt`, `--post-prompt`, etc.)
2. Environment variables (`ORBIT_AGENT`, `ORBIT_COMMAND`, `ORBIT_PRE_PROMPT`, `ORBIT_POST_PROMPT`, `ORBIT_DATE_SUBDIRS`, `ORBIT_CONTINUE_SESSION`)
3. Project config (`.orbit.yaml` in working directory)
4. Home config (`~/.orbit.yaml`)
5. Built-in defaults

Empty string environment variables explicitly disable features (e.g., `ORBIT_POST_PROMPT=""` disables post-prompt).

Agent configuration in `.orbit.yaml` supports per-agent settings under the `agents` key with `cli-path`, `auto-approve` (default: `true`), `timeout`, and `extra-args` options. Auto-approve is enabled by default to allow non-interactive operation.

### Commands and Prompts

Orbit supports two types of hooks that run at different points in the orchestration lifecycle:

**Prompts (AI agent interactions):**
- `pre-prompt` - AI prompt executed before phase 1, sharing the same session
- `post-prompt` - AI prompt executed after the final phase completes (renamed from `post-command`)

**Shell Commands (per-agent):**
- `agents.<agent>.pre-command` - Shell command run before the agent starts
- `agents.<agent>.post-command` - Shell command run after the agent completes

**Execution Order:**
1. Agent pre-command (shell) - failure aborts the run
2. Global pre-prompt (AI) - failure aborts the run
3. Phase loop (all task phases)
4. Global post-prompt (AI) - failure completes with warnings
5. Agent post-command (shell) - failure completes with warnings

**Key Differences:**
- **Prompts** are sent to the AI agent and can interact with the codebase (e.g., review code, run analysis, make changes)
- **Commands** are shell commands that run in your terminal (e.g., `make lint`, `npm test`)
- Shell commands must be non-interactive (no user input required)

**Command Timeout:**
- `command-timeout` - Duration string for shell command execution (default: `5m`)
- Applies to both pre-command and post-command
- Supports Go duration format (e.g., `30s`, `5m`, `1h30m`)

**Example Configuration:**
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
```

## External Dependencies

Orbit requires:
- **rune** - task management CLI for reading/tracking task phases
- At least one AI coding agent CLI:
  - **Claude Code CLI** (`claude`) - default agent
  - **OpenAI Codex** (`codex`)
  - **AWS Kiro** (`kiro-cli`)
  - **GitHub Copilot** (`copilot`)
  - **OpenCode** (`opencode`) - open-source agent supporting multiple LLM providers

Apsis has no external dependencies beyond access to `~/.claude/projects/`.

## Error Handling (Orbit)

Each agent has its own error classifier in `internal/agents/{agent}/errors.go`. Errors are classified into:
- `ErrorClassRetryable` - transient errors (rate limits, connection issues) - exponential backoff
- `ErrorClassFatal` - permanent errors (auth failures, invalid config) - stops immediately
- `ErrorClassSessionInvalid` - session expired or not found - retries with fresh session
- `ErrorClassRateLimitWait` - usage limits that reset at a specific time - waits until reset, then continues

The orchestrator uses these classifications for retry decisions with exponential backoff (1s, 2s, 4s, 8s, 16s for connection errors). Non-retryable errors stop orchestration and preserve state for manual intervention.

### Usage Limit Handling (Claude Code)

Claude Code has a 5-hour usage limit that displays messages like "You've hit your limit · resets 3am (Australia/Melbourne)". When this occurs, Orbit will:
1. Parse the reset time and timezone from the error message
2. Calculate the wait duration until that time (plus a 1-minute buffer)
3. Wait automatically until the limit resets
4. Resume execution without counting against the normal retry limit

Supported time formats: `3am`, `12:30pm`, `3:00 am`
Supported timezones: IANA names (e.g., `Australia/Melbourne`, `America/New_York`) and common abbreviations (EST, PST, UTC, AEST, etc.)

This feature is currently implemented for Claude Code only, as other agents (Codex, Kiro, Copilot, OpenCode) use standard rate limiting with retry-after headers rather than time-based usage limits. The infrastructure (`ErrorClassRateLimitWait`) is available for other agents to use if they implement similar patterns.

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

### Variant Session Recovery

When re-running `orbit run --variants` with an existing variant run, you're prompted to choose:
- **Continue [c]**: Resume from where it left off, keeping all variants as-is
- **New run [n]**: Restart unfinished variants only - completed variants are preserved, while pending/failed/running variants are cleaned up and recreated
- **Cancel [q]**: Abort without changes

This allows recovering from partial failures without losing completed work.

Consolidation allows merging good ideas from non-chosen variants:
- Reads cross-variant improvements from comparison report
- Agent applies beneficial changes to chosen variant
- Creates consolidation commit with applied improvements
- Supports `--rollback` to revert last consolidation

Variants are stored in `specs/{spec}/.orbit/worktrees/` with metadata in `variants.json`.
