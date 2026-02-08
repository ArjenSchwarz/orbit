# Apsis Session Listing

## How `apsis -l` works

Entry point: `cmd/apsis/main.go`, function `listAllSessions(projectPath)`.

Calls agent-specific list functions in parallel, then merges and sorts results:
- `listClaudeSessions(projectPath)` - filters via encoded project path in directory name
- `listCopilotSessions(projectPath)` - filters via `workspace.yaml` metadata
- `listCodexSessions(homeDir, projectPath)` - filters via `cwd` field in `session_meta`
- `listKiroSessions(projectPath)` - filters via SQLite directory discovery
- `listKiroIDESessions(projectPath)` - filters via workspace directory lookup

## Codex session structure

Sessions stored in `~/.codex/sessions/YYYY/MM/DD/session-{uuid}.jsonl`.

First line is always `session_meta` type with payload containing `id`, `timestamp`, and `cwd`.
The `cwd` field contains the working directory where the session was started.

Two functions read from session_meta:
- `getCodexSessionTimestamp(path)` - reads top-level `timestamp` field
- `getCodexSessionCwd(path)` - reads `payload.cwd` field

## Session resolution by ID

`resolveInput()` and `resolveFollowInput()` use `findCodexSession()` to locate a session by UUID. This scans all codex sessions without project filtering (intentional — user explicitly asked for a specific session).
