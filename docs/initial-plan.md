# Plan: Orbit - Claude Code Task Orchestrator

## Overview

Build **Orbit**, a Go CLI tool that orchestrates Claude Code sessions to implement spec phases sequentially. It handles session lifecycle, error recovery, and log management.

**Repository:** `github.com/arjenschwarz/orbit` (standalone repo, similar to rune)

## Core Workflow

```
┌─────────────────────────────────────────────────────────────┐
│  Check remaining tasks (rune list --filter pending)         │
│  ↓                                                          │
│  If no tasks → Exit success                                 │
│  ↓                                                          │
│  Run Claude session: "/next-task --phase" then "/commit"    │
│  ↓                                                          │
│  Parse result → Handle errors based on type                 │
│  ↓                                                          │
│  Save session log → Loop                                    │
└─────────────────────────────────────────────────────────────┘
```

## Project Structure

```
orbit/
├── cmd/
│   └── orbit/
│       └── main.go            # Entry point, CLI flags
├── internal/
│   ├── orbit/
│   │   └── orbit.go           # Main orchestration loop
│   ├── claude/
│   │   └── client.go          # Claude CLI wrapper
│   ├── rune/
│   │   └── client.go          # Rune CLI wrapper
│   ├── errors/
│   │   └── errors.go          # Error classification
│   └── logs/
│       └── manager.go         # Log management
├── go.mod
├── go.sum
├── LICENSE
└── README.md
```

## Implementation Details

### 1. Entry Point (cmd/orbit/main.go)

**CLI Flags:**
- `--tasks-file` - Path to rune tasks file (optional, auto-detects from branch)
- `--log-dir` - Base directory for logs (default: `.claude/orchestration-logs/`)
- `--dry-run` - Show what would be executed without running
- `--verbose` - Enable detailed output
- `--skip-permissions` - Run Claude with `--dangerously-skip-permissions` (default: true)

**Auto-detection logic:**
1. Get current git branch name
2. Strip common prefixes (feature/, hotfix/, bugfix/)
3. Look for `specs/{branch-name}/tasks.md`
4. If not found, try `specs/{branch-name}/TASKS.md`
5. If still not found, error with helpful message

**Initialization:**
- Parse flags
- Auto-detect or validate tasks file
- Create timestamped log directory: `{log-dir}/{timestamp}-{branch-name}/`
- Start orchestration loop

**Resumption:** Orbit is inherently resumable - it always checks `rune list --filter pending` at the start of each iteration. If tasks were completed manually via Claude interactive mode or a previous Orbit run, they'll be skipped automatically.

### 2. Orchestration Loop (internal/orbit/orbit.go)

```go
func (o *Orbit) Run() error {
    for {
        // 1. Check for remaining tasks
        pending, err := o.rune.ListPending()
        if err != nil { return err }
        if len(pending) == 0 {
            log.Println("All tasks complete")
            return nil
        }

        // 2. Run Claude session for next phase
        result, err := o.runPhase()
        if err != nil {
            return o.handleError(err)
        }

        // 3. Save session log
        o.logs.SaveSession(result)
    }
}
```

### 3. Claude CLI Wrapper (internal/claude/client.go)

**Execute method:**
```go
func (c *ClaudeClient) RunPhase() (*SessionResult, error) {
    // Combine commands into single prompt
    prompt := "Run /next-task --phase and when complete run /commit"

    args := []string{"-p", prompt, "--output-format", "json"}
    if c.skipPermissions {
        args = append(args, "--dangerously-skip-permissions")
    }
    cmd := exec.Command("claude", args...)

    // Capture output
    output, err := cmd.Output()

    // Parse JSON result
    var result ClaudeResult
    json.Unmarshal(output, &result)

    return &SessionResult{
        SessionID: result.SessionID,
        Cost:      result.TotalCostUSD,
        Duration:  result.DurationMS,
        Output:    result.Result,
        IsError:   result.IsError,
    }, nil
}
```

### 4. Error Classification (internal/errors/errors.go)

**Error types and handling:**

| Error Type | Detection | Action |
|------------|-----------|--------|
| Connection error | Exit code + "connection" in stderr | Exponential backoff (1s, 2s, 4s, 8s, max 5 retries) |
| Rate limit | "rate limit" or "429" in output | Parse retry-after header or wait 60s, then continue |
| API overloaded | "overloaded" in output | Wait 30s, retry |
| Other errors | Any other non-zero exit | Stop, preserve state, report |

```go
func classifyError(exitCode int, stderr, stdout string) ErrorType {
    combined := stderr + stdout
    switch {
    case strings.Contains(combined, "rate limit") || strings.Contains(combined, "429"):
        return ErrRateLimit
    case strings.Contains(combined, "connection") || strings.Contains(combined, "network"):
        return ErrConnection
    case strings.Contains(combined, "overloaded"):
        return ErrOverloaded
    default:
        return ErrUnknown
    }
}
```

### 5. Rune CLI Wrapper (internal/rune/client.go)

```go
func (r *RuneClient) ListPending() ([]Task, error) {
    cmd := exec.Command("rune", "list", r.tasksFile, "--filter", "pending", "--format", "json")
    output, err := cmd.Output()
    // Parse JSON tasks
    var tasks []Task
    json.Unmarshal(output, &tasks)
    return tasks, nil
}

func (r *RuneClient) GetNextPhase() (*Phase, error) {
    cmd := exec.Command("rune", "next", r.tasksFile, "--phase", "--format", "json")
    // ...
}
```

### 6. Log Management (internal/logs/manager.go)

**Directory structure:**
```
.claude/orchestration-logs/
└── 2025-01-15-143022-feature-branch/
    ├── summary.json           # Overall run summary
    ├── phase-1-session.json   # Full Claude output for phase 1
    ├── phase-1-session.txt    # Human-readable transcript
    ├── phase-2-session.json
    ├── phase-2-session.txt
    └── ...
```

**Summary file format:**
```json
{
  "started_at": "2025-01-15T14:30:22Z",
  "completed_at": "2025-01-15T15:45:00Z",
  "status": "success",
  "phases_completed": 3,
  "total_cost_usd": 0.45,
  "sessions": [
    {"phase": 1, "session_id": "abc123", "duration_ms": 45000, "cost_usd": 0.15},
    {"phase": 2, "session_id": "def456", "duration_ms": 60000, "cost_usd": 0.20}
  ]
}
```

## Usage Example

```bash
# From project root on a feature branch (auto-detects tasks file)
cd /path/to/project
git checkout feature/my-feature
orbit   # Detects specs/my-feature/tasks.md automatically

# With explicit tasks file
orbit --tasks-file specs/my-feature/tasks.md

# With options
orbit --verbose --log-dir ./logs

# Dry run to see what would happen
orbit --dry-run
```

## Design Decisions

1. **Auto-detect tasks file** - Infers from git branch name → `specs/{branch}/tasks.md`
2. **Inherent resumption** - No explicit resume flag needed; rune tracks task state, so orchestrator always checks pending tasks and continues from where it left off
3. **Standalone repo** - Easier to version, release, and install independently (like rune)
