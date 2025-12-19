# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Orbit is a CLI tool that orchestrates Claude Code sessions to implement spec phases sequentially. It handles session lifecycle, error recovery, and log management. The tool automates running Claude Code through multiple implementation phases without manual intervention.

## Build and Development Commands

```bash
make build          # Build binary
make test           # Run all tests
make test-verbose   # Run tests with verbose output
make test-coverage  # Run tests with coverage report
make lint           # Run golangci-lint
make modernize      # Update code to modern Go idioms
make install        # Install to GOPATH/bin
make clean          # Remove build artifacts

# Run single test
go test ./internal/orbit -run TestName
```

## Architecture

The codebase follows a clean internal package structure:

```
cmd/orbit/main.go    - CLI entry point, flag parsing, branch detection
internal/
  orbit/orbit.go     - Main orchestration loop with retry logic
  claude/client.go   - Wrapper for Claude Code CLI execution
  rune/client.go     - Wrapper for rune CLI task management
  errors/errors.go   - Error classification (rate limits, connection, overload)
  logs/manager.go    - Session log storage and summary management
```

**Key flow**: `main.go` parses flags and detects tasks file from git branch → `Orbit.Run()` loops through phases → `claudeClient.RunPhase()` executes Claude with `/next-task --phase` → errors are classified and retried or propagated → logs are saved per phase.

## Tasks File Auto-Detection

When `--tasks-file` is not specified, Orbit detects it from the git branch:
1. Get current branch name
2. Strip everything before the first `/` (e.g., `feature/my-feature` → `my-feature`)
3. Look for `specs/{name}/tasks.md` or `specs/{name}/TASKS.md`

## Resumption

Orbit is inherently resumable - no explicit resume flag needed. Each iteration checks `rune list --filter pending` for remaining tasks. If tasks were completed manually or by a previous run, they're skipped automatically. You can stop Orbit (Ctrl+C), work in Claude interactive mode, then run Orbit again to continue.

## External Dependencies

Orbit requires two external CLIs to function:
- **Claude Code CLI** (`claude`) - must be installed and authenticated
- **rune** - task management CLI for reading/tracking task phases

## Error Handling

The `errors` package classifies CLI output into retryable categories:
- `ErrRateLimit` - waits for retry-after duration (default 60s)
- `ErrOverloaded` - waits 30s
- `ErrConnection` - exponential backoff (1s, 2s, 4s, 8s, 16s)

Non-retryable errors stop orchestration and preserve state for manual intervention.

## Log Structure

Sessions are saved to `.claude/orchestration-logs/{timestamp}-{branch}/`:
- `summary.json` - overall run metadata and session entries
- `phase-N-session.json` - raw Claude JSON output
- `phase-N-session.txt` - human-readable transcript
