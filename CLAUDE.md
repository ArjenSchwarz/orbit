# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This repository contains two related CLI tools for working with AI coding agents:

- **Orbit** - Orchestrates AI coding agent sessions to implement spec phases sequentially. Supports Claude Code, OpenAI Codex, AWS Kiro, and GitHub Copilot. Handles session lifecycle, error recovery, log management, and multi-variant comparison runs. Includes a web interface for viewing runs and transcripts.
- **Apsis** - Converts Claude Code session transcripts from JSONL format to readable Markdown or HTML. Lists and transforms session files stored in `~/.claude/projects/`.

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
  orbit/main.go      - Orbit CLI entry point, subcommand routing (run, serve, register, status, cleanup, finalize, compare)
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
  orbit/orbit.go     - Main orchestration loop with retry logic
  variants/          - Multi-variant comparison support
    types.go         - Variant struct and status types
    manager.go       - Variant lifecycle (create, run, finalize, cleanup)
    agent.go         - Per-variant agent assignment
  comparison/        - Variant comparison logic
    compare.go       - Comparator for analyzing variant diffs
  report/            - Comparison report generation
  claude/client.go   - Legacy Claude Code CLI wrapper (used by comparison)
  claude/paths.go    - Claude project path utilities (shared by orbit and apsis)
  rune/client.go     - Wrapper for rune CLI task management
  errors/errors.go   - Legacy error classification (deprecated, use agents/errors)
  logs/manager.go    - Session log storage and summary management
  config/config.go   - Configuration loading via Viper (files, env vars, defaults)
  transcript/        - JSONL parsing and Markdown/HTML rendering for apsis and web
  registry/          - Run registry for tracking orbit runs across repositories
  web/               - HTTP server, handlers, templates for web interface
```

### Orbit Flow

`main.go` parses flags and detects tasks file from git branch → resolves agent (claude-code, codex, kiro, copilot) → `Orbit.Run()` loops through phases → `agent.Run()` executes the configured agent with `/next-task --phase` → agent-specific errors are classified and retried or propagated → logs are saved per phase.

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
1. CLI flags (`--agent`, `--command`, `--post-command`, etc.)
2. Environment variables (`ORBIT_AGENT`, `ORBIT_COMMAND`, `ORBIT_POST_COMMAND`, `ORBIT_DATE_SUBDIRS`, `ORBIT_CONTINUE_SESSION`)
3. Project config (`.orbit.yaml` in working directory)
4. Home config (`~/.orbit.yaml`)
5. Built-in defaults

Empty string environment variables explicitly disable features (e.g., `ORBIT_POST_COMMAND=""` disables post-command).

Agent configuration in `.orbit.yaml` supports per-agent settings under the `agents` key with `cli-path`, `auto-approve`, `timeout`, and `extra-args` options.

## External Dependencies

Orbit requires:
- **rune** - task management CLI for reading/tracking task phases
- At least one AI coding agent CLI:
  - **Claude Code CLI** (`claude`) - default agent
  - **OpenAI Codex** (`codex`)
  - **AWS Kiro** (`kiro-cli`)
  - **GitHub Copilot** (`copilot`)

Apsis has no external dependencies beyond access to `~/.claude/projects/`.

## Error Handling (Orbit)

Each agent has its own error classifier in `internal/agents/{agent}/errors.go`. Errors are classified into:
- `ErrorClassRetryable` - transient errors (rate limits, connection issues) - exponential backoff
- `ErrorClassFatal` - permanent errors (auth failures, invalid config) - stops immediately
- `ErrorClassSessionInvalid` - session expired or not found - retries with fresh session

The orchestrator uses these classifications for retry decisions with exponential backoff (1s, 2s, 4s, 8s, 16s for connection errors). Non-retryable errors stop orchestration and preserve state for manual intervention.

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

## Multi-Variant Comparison (Orbit)

Orbit supports running multiple implementation variants with different agents or guidance:
- `orbit run --variants N` creates N git worktrees and runs implementation in each
- `--variant-agents agent1,agent2` assigns different agents to variants (cycles if fewer agents than variants)
- `--guidance-file guidance.yaml` provides per-variant instructions
- `--parallel` runs variants concurrently (limited by `--max-parallel`)

Variant management subcommands:
- `orbit status <spec>` - shows variant status
- `orbit compare <spec>` - regenerates comparison report
- `orbit finalize <spec> --variant N` - adopts variant N and cleans up others
- `orbit cleanup <spec>` - removes all variant worktrees and branches

Variants are stored in `specs/{spec}/.orbit/worktrees/` with metadata in `variants.json`.
