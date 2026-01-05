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
| `--date-subdirs` | `false` | Use date-based subdirectories for logs |
| `--continue-session` | `true` | Resume unfinished Claude sessions on restart |
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
date_subdirs: false      # Use flat .orbit/ directory (default)
continue_session: true   # Resume unfinished sessions (default)
```

To disable the post-completion command in config, set it to an empty string:

```yaml
post-command: ""
```

To use date-based subdirectories (legacy mode):

```yaml
date_subdirs: true
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `ORBIT_COMMAND` | Override the phase command |
| `ORBIT_POST_COMMAND` | Override the post-completion command (empty string disables) |
| `ORBIT_DATE_SUBDIRS` | Use date-based subdirectories (`true`/`false`) |
| `ORBIT_CONTINUE_SESSION` | Enable session continuation (`true`/`false`) |

Setting an environment variable to an empty string explicitly overrides config file values:

```bash
# Disable post-command even if config files set one
ORBIT_POST_COMMAND="" orbit

# Use empty command (not recommended, but supported)
ORBIT_COMMAND="" orbit
```

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

Logs are saved to `.orbit/` next to the tasks file (e.g., `specs/my-feature/.orbit/`).

### Flat Mode (Default)

```
specs/my-feature/.orbit/
├── summary.json                      # Persistent run summary with session tracking
├── phase-1-run-1-session.json        # Full Claude output for phase 1, run 1
├── phase-1-run-1-session.txt         # Human-readable transcript
├── phase-2-run-1-session.json
├── phase-2-run-1-session.txt
├── ...
├── post-completion-run-1-session.json
└── post-completion-run-1-session.txt
```

When you run Orbit multiple times, files are numbered by run (e.g., `phase-1-run-2-session.json`).

### Date Subdirectories Mode (`--date-subdirs`)

```
specs/my-feature/.orbit/
└── 2025-01-15-143022-feature-branch/
    ├── summary.json                    # Run summary for this session
    ├── phase-1-session.json
    ├── phase-1-session.txt
    └── ...
```

## Session Management

Orbit tracks Claude session IDs to enable crash recovery and session continuation.

### How It Works

1. **Session ID Generation**: Before each phase, Orbit generates a UUID session ID
2. **Persistence**: The session ID is saved to `summary.json` before invoking Claude
3. **Resume on Restart**: If Orbit is interrupted mid-phase, it detects the unfinished phase and resumes the same Claude session using `--resume`
4. **Fallback**: If session resume fails (e.g., session expired), Orbit automatically starts a fresh session

### Disabling Session Continuation

To always start fresh sessions instead of resuming:

```bash
orbit --continue-session=false
```

Or in `.orbit.yaml`:

```yaml
continue_session: false
```

## Resumption

Orbit is inherently resumable. Since task state is tracked in the rune tasks file, you can:

- Stop Orbit at any time (Ctrl+C)
- Complete tasks manually in Claude interactive mode
- Run Orbit again to continue from where you left off

With session continuation enabled (default), Orbit will also resume the Claude session context, allowing Claude to remember what it was working on.

## Web Interface

Orbit includes a built-in web interface for viewing runs and transcripts.

### Starting the Server

```bash
# Start on default port (8080)
orbit serve

# Start on custom port
orbit serve --port 3000

# Bind to all interfaces (not just localhost)
orbit serve --bind 0.0.0.0
```

### Features

- **Dashboard**: View all runs grouped by repository
- **Run Details**: See phase status, duration, and summary
- **Transcript Viewer**: Read transcripts with syntax highlighting and navigation
- **Live Updates**: Auto-refresh for running sessions via HTMX
- **Mobile Responsive**: Works on phones and tablets
- **Dark Mode**: Follows system preference

### Run Registry

Orbit automatically registers runs when orchestrating. Runs are tracked in `~/.orbit/runs/` and persist across sessions.

To manually register an existing orbit log directory:

```bash
# Register from current directory (auto-detects .orbit/)
orbit register

# Register a specific path
orbit register specs/my-feature

# Register with a custom name
orbit register --name "My Feature" specs/my-feature/.orbit
```

The web interface shows all registered runs, their status, and provides links to view transcripts.

---

# Apsis

Apsis is a CLI tool for converting Claude Code session transcripts from JSONL format to readable Markdown or HTML.

## Installation

```bash
go install github.com/arjenschwarz/orbit/cmd/apsis@latest
```

## Usage

```bash
# Convert session by ID (looks in ~/.claude/projects)
apsis 550e8400-e29b-41d4-a716-446655440000

# Convert from file path
apsis /path/to/session.jsonl

# Convert from stdin
cat session.jsonl | apsis

# Save to file
apsis -o transcript.md session-id

# Export as HTML
apsis -f html -o transcript.html session-id

# List available sessions for current project
apsis --list

# List sessions for a different project
apsis --list -p /path/to/project
```

## Options

| Flag | Description |
|------|-------------|
| `-l, --list` | List available sessions for the project |
| `-o, --output <file>` | Write output to file (default: stdout) |
| `-p, --project <path>` | Project directory (default: current directory) |
| `-f, --format <format>` | Output format: `md`, `markdown`, `html` (default: `md`) |
| `-v, --version` | Show version |
| `-h, --help` | Show help |

## Output Formats

### Markdown (default)

Generates a Markdown document with:
- Session header with ID
- User messages with 👤 icon
- Assistant messages with 🤖 icon
- Collapsible thinking blocks using `<details>` tags
- Tool usage with JSON input
- Tool results with success/error indicators

### HTML

Generates a styled HTML document with:
- Embedded CSS (no external dependencies)
- Dark mode support (follows system preference)
- Responsive layout for mobile viewing
- Collapsible thinking blocks
- Syntax-highlighted code blocks
- Color-coded success/error indicators

## License

MIT
