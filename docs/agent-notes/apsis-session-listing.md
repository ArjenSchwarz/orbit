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

## OpenCode session structure

Sessions stored in `~/.local/share/opencode/storage/message/<sessionID>/`.

Each session directory contains `msg_<id>.json` files. The `DiscoverSessions` method reads the first `msg_` file to extract `time.created` for `CreatedAt`.

The `parseCreatedTime` function handles multiple timestamp formats: RFC3339, RFC3339Nano, unix seconds, unix milliseconds, and numeric strings. It accepts a `fallback` parameter used when parsing fails.

**Gotcha**: `unixToTime` returns `time.Time{}` for values <= 0. Callers of `parseCreatedTime` should not assume the fallback is always honored — `unixToTime` results must be checked for zero before returning. (Fixed in T-273.)

**Gotcha**: If no `msg_` files exist in the session directory (or all fail to read/unmarshal), the `CreatedAt` will be zero unless there's an explicit fallback to directory modTime after the msg_ file loop. (Fixed in T-273.)

## Kiro IDE path resolution

`resolveKiroIDE` and `ResolvePath(SourceKiroIDE)` both delegate to `findKiroIDEPath(workspaceDir, sessionID)` for scanning `.chat` files. The `IsPathWithinDir` check is inside the scan loop (not after it) so that symlinked files pointing outside the workspace are skipped without shadowing legitimate files. This is important because the scan selects the file with the most chat messages -- a symlink with more messages must not prevent a valid file from being returned.

Note: `ResolvePath` for Codex and Copilot does not include `IsPathWithinDir` validation (unlike their `Resolve` counterparts). This is a pre-existing gap.
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
