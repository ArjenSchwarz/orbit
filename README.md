# Orbit

Orbit is a CLI tool that orchestrates Claude Code sessions to implement spec phases sequentially. It handles session lifecycle, error recovery, and log management.

## Overview

Orbit solves the problem of running Claude Code through multiple implementation phases without manual intervention. It:

- Automatically detects tasks from your git branch
- Runs Claude Code in non-interactive mode for each phase
- Handles rate limits and connection errors with appropriate retries
- Saves session logs for debugging and auditing

## Installation

```bash
go install github.com/arjenschwarz/orbit/cmd/orbit@latest
```

## Prerequisites

- [Claude Code CLI](https://claude.ai/code) installed and authenticated
- [rune](https://github.com/arjenschwarz/rune) CLI installed
- Git repository with a spec containing a tasks file

## Usage

```bash
# From project root on a feature branch (auto-detects tasks file)
cd /path/to/project
git checkout feature/my-feature
orbit   # Detects specs/my-feature/tasks.md automatically

# With explicit tasks file
orbit --tasks-file specs/my-feature/tasks.md

# With options
orbit --verbose --log-dir ./logs

# Preview without executing
orbit --dry-run

# With custom commands
orbit --command "Run /next-task --phase" --post-command "Run all tests"

# Skip the post-completion review
orbit --no-post-command
```

## Options

| Flag | Default | Description |
|------|---------|-------------|
| `--tasks-file` | auto-detect | Path to rune tasks file |
| `--log-dir` | `.orbit` next to tasks file | Base directory for session logs |
| `--skip-permissions` | `true` | Run Claude with `--dangerously-skip-permissions` |
| `--verbose` | `false` | Enable verbose output |
| `--dry-run` | `false` | Show what would be executed without running |
| `--command` | see below | Custom prompt for Claude phases |
| `--post-command` | see below | Command to run after all tasks complete |
| `--no-post-command` | `false` | Skip the post-completion command |
| `--version` | - | Show version and exit |

### Default Commands

The default phase command is:
```
Run /next-task --phase and when complete run /commit
```

The default post-completion command is:
```
Review the implementation to verify it meets the requirements and all tests pass. If issues are found, fix them.
```

## Configuration

Orbit supports configuration via YAML files and environment variables.

### Configuration Files

Orbit loads configuration from two locations (in order of priority):

1. **Project config**: `.orbit.yaml` in the current directory
2. **Home config**: `~/.orbit.yaml` in your home directory

Example `.orbit.yaml`:

```yaml
command: "Run /next-task --phase and when complete run /commit"
post-command: "Run tests and verify everything works"
```

To disable the post-completion command in config, set it to an empty string:

```yaml
post-command: ""
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `ORBIT_COMMAND` | Override the phase command |
| `ORBIT_POST_COMMAND` | Override the post-completion command (empty string disables) |

### Priority Order

Configuration is resolved in this order (highest priority first):

1. CLI flags (`--command`, `--post-command`, `--no-post-command`)
2. Environment variables (`ORBIT_COMMAND`, `ORBIT_POST_COMMAND`)
3. Project config (`.orbit.yaml` in working directory)
4. Home config (`~/.orbit.yaml`)
5. Built-in defaults

## How It Works

1. **Check Tasks**: Orbit queries `rune list --filter pending` to find remaining tasks
2. **Run Phase**: Executes `claude -p "/next-task --phase then /commit"` for the next phase
3. **Handle Errors**: Classifies errors and retries appropriately:
   - Connection errors: Exponential backoff (1s, 2s, 4s, 8s, 16s)
   - Rate limits: Wait for retry-after duration or 60s default
   - API overload: Wait 30s and retry
   - Other errors: Stop and preserve state
4. **Save Logs**: Stores session JSON and transcripts in timestamped directories
5. **Repeat**: Loops until all tasks are complete

## Log Structure

Logs are saved to `.orbit/` next to the tasks file (e.g., `specs/my-feature/.orbit/`):

```
specs/my-feature/.orbit/
└── 2025-01-15-143022-feature-branch/
    ├── summary.json                    # Overall run summary
    ├── phase-1-session.json            # Full Claude output for phase 1
    ├── phase-1-session.txt             # Human-readable transcript
    ├── phase-2-session.json
    ├── phase-2-session.txt
    ├── ...
    ├── post-completion-session.json    # Post-completion command output
    └── post-completion-session.txt
```

## Resumption

Orbit is inherently resumable. Since task state is tracked in the rune tasks file, you can:

- Stop Orbit at any time (Ctrl+C)
- Complete tasks manually in Claude interactive mode
- Run Orbit again to continue from where you left off

## License

MIT
