# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This repository contains two related CLI tools for working with Claude Code:

- **Orbit** - Orchestrates Claude Code sessions to implement spec phases sequentially. Handles session lifecycle, error recovery, and log management. Includes a web interface for viewing runs and transcripts.
- **Apsis** - Converts Claude Code session transcripts from JSONL format to readable Markdown or HTML. Lists and transforms session files stored in `~/.claude/projects/`.

## Build and Development Commands

```bash
make build          # Build both binaries (orbit and apsis)
make build-orbit    # Build orbit only
make build-apsis    # Build apsis only (with version injection)
make test           # Run all tests
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
  orbit/main.go      - Orbit CLI entry point, subcommand routing (run, serve, register)
  apsis/main.go      - Apsis CLI entry point, session listing and conversion
internal/
  orbit/orbit.go     - Main orchestration loop with retry logic
  claude/client.go   - Wrapper for Claude Code CLI execution
  claude/paths.go    - Claude project path utilities (shared by orbit and apsis)
  rune/client.go     - Wrapper for rune CLI task management
  errors/errors.go   - Error classification (rate limits, connection, overload)
  logs/manager.go    - Session log storage and summary management
  config/config.go   - Configuration loading via Viper (files, env vars, defaults)
  transcript/        - JSONL parsing and Markdown/HTML rendering for apsis and web
  registry/          - Run registry for tracking orbit runs across repositories
  web/               - HTTP server, handlers, templates for web interface
```

### Orbit Flow

`main.go` parses flags and detects tasks file from git branch → `Orbit.Run()` loops through phases → `claudeClient.RunPhase()` executes Claude with `/next-task --phase` → errors are classified and retried or propagated → logs are saved per phase.

### Apsis Flow

Resolves input (session ID, file path, or stdin) → parses JSONL via `transcript.ParseJSONL()` → renders to Markdown via `transcript.RenderMarkdown()` → outputs to stdout or file.

## Tasks File Auto-Detection (Orbit)

When `--tasks-file` is not specified, Orbit detects it from the git branch:
1. Get current branch name
2. Strip everything before the first `/` (e.g., `feature/my-feature` → `my-feature`)
3. Look for `specs/{name}/tasks.md` or `specs/{name}/TASKS.md`

## Configuration (Orbit)

Configuration priority (highest to lowest):
1. CLI flags
2. Environment variables (`ORBIT_COMMAND`, `ORBIT_POST_COMMAND`, `ORBIT_DATE_SUBDIRS`, `ORBIT_CONTINUE_SESSION`)
3. Project config (`.orbit.yaml` in working directory)
4. Home config (`~/.orbit.yaml`)
5. Built-in defaults

Empty string environment variables explicitly disable features (e.g., `ORBIT_POST_COMMAND=""` disables post-command).

## External Dependencies

Orbit requires two external CLIs:
- **Claude Code CLI** (`claude`) - must be installed and authenticated
- **rune** - task management CLI for reading/tracking task phases

Apsis has no external dependencies beyond access to `~/.claude/projects/`.

## Error Handling (Orbit)

The `errors` package classifies CLI output into retryable categories:
- `ErrRateLimit` - waits for retry-after duration (default 60s)
- `ErrOverloaded` - waits 30s
- `ErrConnection` - exponential backoff (1s, 2s, 4s, 8s, 16s)

Non-retryable errors stop orchestration and preserve state for manual intervention.

## Log Structure (Orbit)

Sessions are saved to `.orbit/` next to the tasks file (e.g., `specs/my-feature/.orbit/`):
- `summary.json` - persistent run summary with session tracking
- `phase-N-run-M-session.json` - full Claude output for phase N, run M
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
