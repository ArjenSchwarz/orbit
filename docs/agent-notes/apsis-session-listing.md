# Apsis Session Listing

## How `apsis -l` works

Entry point: `cmd/apsis/main.go`, function `listSessions(projectPath)`, which constructs `sessions.NewLister()` and calls `Lister.ListAll(projectPath)`.

`internal/sessions/lister.go` calls these collectors sequentially, retains source warnings, then merges and sorts results:

- `listClaude(projectPath)` - filters via encoded project path in directory name
- `listCopilot(projectPath)` - filters via `workspace.yaml` metadata
- `listCodex(projectPath)` - filters via `cwd` in `session_meta`
- `listKiro(projectPath)` - filters via SQLite directory discovery
- `listKiroIDE(projectPath)` - filters via workspace directory lookup

## Path normalization

When filtering sessions by project path, always use `normalizePath()` (EvalSymlinks + filepath.Clean) for comparisons. This is critical on macOS where `/tmp` symlinks to `/private/tmp`.

- `listCodex` and `listCopilot` both use `normalizePath` for path comparison
- `listClaude` uses an encoded project path in the directory name (different approach, not affected)
- `listKiro` and `listKiroIDE` delegate path filtering to their respective libraries

Bug T-290 was caused by `listCopilot` using plain string equality instead of `normalizePath`.

## Empty project path handling

When `projectPath` is empty (no filtering), each agent source must return ALL sessions:
- **Claude**: `listClaude` iterates all subdirectories under `~/.claude/projects/` and collects sessions from each. Uses `listClaudeAllProjects()` internally.
- **Codex**: `listCodex` skips the `cwd` filter check when `projectPath` is empty.
- **Copilot**: `listCopilot` skips the `matchPath` comparison entirely when `projectPath` is empty (T-374). It also includes sessions whose `workspace.yaml` is missing or unparseable when no filter is set (T-701) — workspace metadata is only required for filtering, not for listing. Parse errors are surfaced as `ListWarning` entries.

This was the subject of T-146 — `listClaude` previously called `BuildProjectPath("")` which returned `""`, causing it to look in the root `~/.claude/projects/` directory (which contains subdirectories, not `.jsonl` files).

## Copilot workspace.yaml handling

`parseCopilotWorkspace` in both `internal/sessions/lister.go` and `internal/agents/copilot/agent.go`:
- Returns `(nil, nil)` when `workspace.yaml` does not exist (treat as missing metadata).
- Returns `(nil, error)` when the file exists but cannot be read or parsed (callers may surface a warning).

Callers must distinguish "missing metadata" from "invalid session". A session with valid non-empty `events.jsonl` is a valid session even when `workspace.yaml` is missing or malformed; metadata is only needed when filtering by project path. T-701 was caused by the previous `if err != nil || ws == nil { continue }` guard which conflated the two.

## Codex session structure

Sessions are JSONL files below `~/.codex/sessions/`, normally in date directories. Listing extracts a UUID substring from the filename when present; otherwise it uses the basename without `.jsonl`.

The `session_meta` header records the session ID and starting working directory; its top-level timestamp supplies the recorded listing time. The listing helpers search up to `codexMetaScanLimit` (50) physical lines with a 10 MiB per-line limit, rather than assuming that the first line is usable.

Two functions read from session_meta:
- `getCodexSessionTimestamp(path)` - reads top-level `timestamp` field
- `getCodexSessionCwd(path)` - reads `payload.cwd` field

## Existing timestamp scan contracts

- Claude listing uses `transcript.ParseFirstTimestamp`: it scans until a timestamp is found, with a 10 MiB per-line limit and no 50-line or 1-MiB total cap. An invalid nonempty timestamp returns an error; listing then falls back to modification time.
- Codex timestamp/cwd helpers retain their 50-line, 10-MiB-per-line scan behaviour. Timestamp extraction failures fall back to modification time.
- A metadata cache must preserve those outcomes. Applying a new global head limit to existing timestamps can change listing order and `latest` even without a feature flag.

## Session resolution by ID

`resolveInput()` and `resolveFollowInput()` delegate to `sessions.Resolver`; Codex branches use `findCodexSession()`. It searches globally without project filtering, matches a filename UUID or a full non-UUID basename case-insensitively, and stops at the first walk-order match before validating its content/readability.

Duplicate filename IDs can therefore shadow a later valid file. Any derived index that claims an ID is openable must validate the actual selected candidate, rather than any matching copy. All-projects listing and suppression proposals must also keep their new selection rules distinct from legacy project-listing behaviour.

`resolveInput()` returns the reader plus an `inputContext` (source, ID, Kiro IDE cost path, resolver metadata); file and stdin input have no source or resolver metadata. Resolver `SessionMetadata` now also carries:

- `Path`: the opened file (Claude, Codex, Copilot, Kiro IDE); empty for Kiro CLI.
- `TimeFromFilesystem`: set wherever `CreatedAt` falls back to the file modification time. Shared JSON never reports such a time as `created_at`.
- `Project`: Copilot `workspace.yaml` `git_root`, else `cwd`; for Kiro CLI the database key that actually matched (`logs.GetSessionRecord` tries the normalised then the symlink-resolved directory).
- `DatabaseCreatedAt`: the Kiro CLI database `created_at`. Kiro CLI `CreatedAt` deliberately stays zero because the web transcript view prints `CreatedAt` whenever it is set (Q103, open question U6).

## OpenCode agent session structure (not an Apsis source)

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
