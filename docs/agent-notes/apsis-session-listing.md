# Apsis Session Listing

## How `apsis -l` works

Entry point: `cmd/apsis/main.go`, function `listAllSessions(projectPath)`.

Calls agent-specific list functions in parallel, then merges and sorts results:
- `listClaudeSessions(projectPath)` - filters via encoded project path in directory name
- `listCopilotSessions(projectPath)` - filters via `workspace.yaml` metadata
- `listCodexSessions(homeDir, projectPath)` - filters via `cwd` field in `session_meta`
- `listKiroSessions(projectPath)` - filters via SQLite directory discovery
- `listKiroIDESessions(projectPath)` - filters via workspace directory lookup

## Path normalization

When filtering sessions by project path, always use `normalizePath()` (EvalSymlinks + filepath.Clean) for comparisons. This is critical on macOS where `/tmp` symlinks to `/private/tmp`.

- `listCodex` and `listCopilot` both use `normalizePath` for path comparison
- `listClaude` uses an encoded project path in the directory name (different approach, not affected)
- `listKiro` and `listKiroIDE` delegate path filtering to their respective libraries

Bug T-290 was caused by `listCopilot` using plain string equality instead of `normalizePath`.

## Codex session structure

Sessions stored in `~/.codex/sessions/YYYY/MM/DD/session-{uuid}.jsonl`.

First line is always `session_meta` type with payload containing `id`, `timestamp`, and `cwd`.
The `cwd` field contains the working directory where the session was started.

Two functions read from session_meta:
- `getCodexSessionTimestamp(path)` - reads top-level `timestamp` field
- `getCodexSessionCwd(path)` - reads `payload.cwd` field

## Session resolution by ID

`resolveInput()` and `resolveFollowInput()` use `findCodexSession()` to locate a session by UUID. This scans all codex sessions without project filtering (intentional — user explicitly asked for a specific session).

## Kiro IDE execution detail actions

When a `costPath` (execution detail file path) is provided, the Kiro IDE parser reads the `actions` array from the execution detail file and converts them to `tool_use`/`tool_result` entry pairs. This produces much richer transcripts than the `.chat` file alone, which only has conversational messages.

Action type → tool name mapping:
- `readFiles` → Read, `replace`/`append` → Edit, `create` → Write, `runCommand` → Bash, `search` → Grep
- `say` → assistant text entry, `taskStatus` → assistant text, `userInput` → user text
- `model`, `steering`, `intentClassification`, `specAgent` → skipped (internal)

Key details:
- User messages come from the `.chat` file's `human` role messages (not from actions)
- Actions are in the same execution detail file already used for cost extraction
- `readKiroIDEExecutionDetail()` reads the file once for both cost and actions
- Falls back to chat-based entries when no actions or no costPath available
- Action states: Success, Accepted, Rejected, Error, Canceled, Running
- `runCommand` actions have `output.output` (string) and `output.exitCode` (number)
- File modification actions (`replace`, `create`, `append`) use `kiro-diff://` URIs — actual diff content is stored separately
