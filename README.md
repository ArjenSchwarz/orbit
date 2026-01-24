# Orbit

Orbit is a CLI tool that orchestrates AI coding agents to implement spec phases sequentially. It handles session lifecycle, error recovery, and log management.

## Overview

Orbit solves the problem of running AI coding agents through multiple implementation phases without manual intervention. It:

- Supports multiple AI agents: Claude Code, OpenAI Codex, AWS Kiro, and GitHub Copilot
- Automatically detects tasks from your git branch
- Runs agents in non-interactive mode for each phase
- Handles rate limits and connection errors with appropriate retries
- Saves session logs for debugging and auditing
- Supports multi-variant comparison runs to evaluate different implementations

## Installation

```bash
go install github.com/arjenschwarz/orbit/cmd/orbit@latest
```

## Prerequisites

- At least one AI coding agent installed and authenticated:
  - [Claude Code CLI](https://docs.anthropic.com/en/docs/agents-and-tools/claude-code/overview) (default)
  - [OpenAI Codex](https://github.com/openai/codex)
  - [AWS Kiro](https://kiro.dev/docs/cli)
  - [GitHub Copilot CLI](https://docs.github.com/en/copilot/using-github-copilot/using-github-copilot-in-the-command-line)
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

### Core Options

| Flag | Default | Description |
|------|---------|-------------|
| `--tasks-file` | auto-detect | Path to rune tasks file |
| `--log-dir` | `.orbit` next to tasks file | Base directory for session logs |
| `--verbose` | `false` | Enable verbose output |
| `--debug` | `false` | Enable debug logging (detailed CLI execution info) |
| `--dry-run` | `false` | Show what would be executed without running |
| `--command` | see below | Custom prompt for agent phases |
| `--post-command` | see below | Command to run after all tasks complete |
| `--no-post-command` | `false` | Skip the post-completion command |
| `--date-subdirs` | `false` | Use date-based subdirectories for logs |
| `--no-continue-session` | `false` | Start fresh sessions instead of resuming |
| `--version` | - | Show version and exit |

### Agent Selection

| Flag | Default | Description |
|------|---------|-------------|
| `--agent` | `claude-code` | Agent to use: `claude-code`, `codex`, `kiro`, `copilot` |

### Multi-Variant Comparison

| Flag | Default | Description |
|------|---------|-------------|
| `--variants` | `0` | Number of implementation variants to run (0 = single-run mode) |
| `--variant-agents` | - | Comma-separated agent list for variants (cycles if fewer than variants) |
| `--parallel` | `false` | Run variants in parallel |
| `--max-parallel` | `3` | Maximum concurrent variants |
| `--branch-prefix` | `orbit-impl` | Branch naming prefix for variants |
| `--guidance-file` | - | YAML file with per-variant guidance |
| `--compare-command` | - | Custom comparison command |

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

Create a default configuration with:

```bash
orbit init
```

Example `.orbit.yaml`:

```yaml
command: "Run /next-task --phase and when complete run /commit"
post-command: "Run tests and verify everything works"
date_subdirs: false      # Use flat .orbit/ directory (default)
continue_session: true   # Resume unfinished sessions (default)
agent: claude-code       # Default agent alias

# Agent aliases - each combines an agent type with configuration
agents:
  claude-code:
    type: claude-code                   # Required: underlying agent type
    auto-approve: true                  # Tool approval behavior (default: true)
    timeout: 30m                        # Execution timeout
  codex:
    type: codex                         # Required: underlying agent type
    timeout: 1h
  kiro:
    type: kiro                          # Required: underlying agent type
    timeout: 1h
  copilot:
    type: copilot                       # Required: underlying agent type

# Agent aliases can also specify models for per-variant model selection
# claude-sonnet:
#   type: claude-code
#   model: claude-sonnet-4-20250514
# claude-opus:
#   type: claude-code
#   model: claude-opus-4-20250514
```

### Auto-Approve Behavior

By default, `auto-approve` is **enabled** (`true`) for all agents. This allows Orbit to run agents non-interactively without prompting for tool approvals.

Each agent uses its equivalent auto-approval flag:

| Agent | Flag Used |
|-------|-----------|
| Claude Code | `--dangerously-skip-permissions` |
| Codex | `--full-auto` |
| Kiro | `--trust-all-tools` |
| Copilot | `--yolo` (equivalent to `--allow-all-tools --allow-all-paths --allow-all-url`) |

To disable auto-approval for a specific agent (requiring manual tool approval):

```yaml
agents:
  claude-code:
    auto-approve: false
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
| `ORBIT_AGENT` | Default agent to use |

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
2. **Run Phase**: Executes the configured agent with the phase prompt (e.g., `/next-task --phase then /commit`)
3. **Handle Errors**: Classifies errors per-agent and retries appropriately:
   - Connection errors: Exponential backoff (1s, 2s, 4s, 8s, 16s)
   - Rate limits: Wait for retry-after duration or 60s default
   - API overload: Wait 30s and retry
   - Session invalid: Retry with fresh session
   - Other errors: Stop and preserve state
4. **Save Logs**: Stores session output and transcripts
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

Orbit tracks session IDs to enable crash recovery and session continuation across all supported agents.

### How It Works

1. **Session ID Generation**: Before each phase, Orbit generates a UUID session ID
2. **Persistence**: The session ID is saved to `summary.json` before invoking the agent
3. **Resume on Restart**: If Orbit is interrupted mid-phase, it detects the unfinished phase and resumes using agent-specific resume mechanisms
4. **Session Export**: Some agents (like Kiro) require explicit session export, which Orbit handles automatically
5. **Fallback**: If session resume fails (e.g., session expired), Orbit automatically starts a fresh session

### Disabling Session Continuation

To always start fresh sessions instead of resuming:

```bash
orbit run --no-continue-session
```

Or in `.orbit.yaml`:

```yaml
continue_session: false
```

## Resumption

Orbit is inherently resumable. Since task state is tracked in the rune tasks file, you can:

- Stop Orbit at any time (Ctrl+C)
- Complete tasks manually in interactive mode
- Run Orbit again to continue from where you left off

With session continuation enabled (default), Orbit will also resume the agent session context, allowing it to remember what it was working on.

## Multi-Variant Comparison

Orbit supports running multiple implementation variants in parallel using different agents or guidance, then comparing the results to choose the best implementation.

### Running Variants

```bash
# Run 3 variants with the default agent
orbit run --variants 3

# Run 2 variants in parallel
orbit run --variants 2 --parallel

# Compare different agents
orbit run --variants 3 --variant-agents claude-code,codex,kiro

# Use per-variant guidance
orbit run --variants 2 --guidance-file guidance.yaml
```

### Guidance File Format

Create a YAML file with per-variant instructions:

```yaml
global_guidance: "Focus on performance and code readability"

variants:
  - id: 1
    guidance: "Use a functional programming approach"
  - id: 2
    guidance: "Use an object-oriented approach with design patterns"
```

### Variant Subcommands

Once variants are running or complete, use these subcommands to manage them:

```bash
# Check variant status
orbit status my-feature

# Regenerate comparison report
orbit compare my-feature

# Adopt a variant and clean up others
orbit finalize my-feature --variant 1

# Clean up all variants
orbit cleanup my-feature

# Clean up but keep a specific variant
orbit cleanup my-feature --keep 2
```

### Variant Workflow

1. **Run variants**: `orbit run --variants N` creates N worktrees and runs the implementation in each
2. **Monitor progress**: `orbit status` shows the status of each variant
3. **Compare results**: After completion, a comparison report is generated in `specs/{spec}/comparison-report/`
4. **Finalize**: Use `orbit finalize --variant N` to adopt the best implementation and clean up

### Variant Structure

Variants are stored in worktrees alongside the `.orbit` directory:

```
specs/my-feature/.orbit/
├── variants.json                     # Variant metadata
└── worktrees/
    ├── orbit-impl-1-feature-branch/  # Variant 1 worktree
    ├── orbit-impl-2-feature-branch/  # Variant 2 worktree
    └── orbit-impl-3-feature-branch/  # Variant 3 worktree
```

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
