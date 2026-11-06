# Apsis Session Listing

## How `apsis -l` works

Entry point: `cmd/apsis/main.go`, function `listSessions(w, projectPath, format)`, which constructs `sessions.NewLister(opts...)` and calls `Lister.ListAll(projectPath)`. An empty `projectPath` is all-projects scope (`--all-projects`); the text listing then adds a fifth tab-separated project column (`-` when unrecorded). `-f shared-json` writes a `sharedjson.Listing` document instead. Listings always go to stdout and ignore `-o` (Q104).

`parseFlags` uses the standard `flag` package, which stops at the first positional argument: in `apsis latest --all-projects` the flag is silently ignored. Options must come first (`apsis --all-projects latest`); write documentation examples that way. The older `apsis latest -F` and `apsis latest -f html` examples in the help text and README have the same problem.

`internal/sessions/lister.go` calls these collectors sequentially. Each returns `(rows, warnings, error)`; a collector error becomes a `source_failed` warning. Results are merged and sorted:

- `listClaude(projectPath)` - filters via encoded project path in directory name
- `listCopilot(projectPath)` - filters via `workspace.yaml` metadata
- `listCodex(projectPath)` - filters via `cwd` in `session_meta`
- `listKiro(projectPath)` - filters via SQLite directory discovery
- `listKiroIDE(projectPath)` - filters via workspace directory lookup
- `listCodexExec(projectPath, rolloutThreads)` - only with exec roots; see [codex-exec-streams.md](codex-exec-streams.md)

`listCodex` (through `listCodexIndexed`) also returns the lower-case recorded IDs of the rollouts `selectCodexCandidate` would open whose heads pass `codexHeadWarning`, in both scopes; `ListAll` hands them to `listCodexExec` to suppress streams with those thread IDs. A failed walk returns no index. `Lister.Sources()` omits `codex-exec` without roots, and `WithCodexExecRoots` with roots also turns on extended ordering (Q114).

## Options and ordering

`NewLister` and `NewResolver` take the same `Option` functions (`WithExtendedOrdering`) applied to a shared `Options` value (Q61). Without extended ordering, `ListAll` keeps the legacy `sortSessionsByTimestamp` (time, then source priority; `sort.Slice` keeps collection order for small equal groups). Extended ordering (`compareSessionsExtended`, stable sort) puts unknown times first, then time, source priority, source name and ID, so it is total and `latest` is the last element. The CLI enables it for `-f shared-json` listings and for `--all-projects` listings/`latest`; `apsis latest` in project scope keeps the legacy order whatever the format (Q83). The web enables it for `apsis serve --all-projects`.

## Listing row metadata

`SessionInfo` carries `TimeFromFilesystem` (set exactly where a timestamp-to-mtime fallback happens), `Project`, `Model`, `Provider` and `Origin`:

- Claude: timestamp from `ParseFirstTimestamp` (unchanged); then `readClaudeHead` re-reads the first 50 lines/1 MiB for the first `cwd` and `entrypoint`. Values beyond that bound are absent from the row only; the session document is authoritative.
- Codex: one `readCodexHead` scan (see below).
- Copilot: project is `workspace.yaml` `git_root`, else `cwd`.
- Kiro CLI: project is the database `key`.
- Kiro IDE: model and provider from `.chat` `metadata`; no project (the workspace hash is one-way).

Warning codes: `source_failed`, `path_unreadable` (Copilot `workspace.yaml`, unreadable Codex winner), `duplicate_session` and `invalid_record` (all-projects scope only).

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

The `session_meta` header records the session ID and starting working directory; its top-level timestamp supplies the recorded listing time. `readCodexHead(path)` searches up to `codexMetaScanLimit` (50) physical lines with a 10 MiB per-line limit, rather than assuming that the first line is usable. It replaced the separate timestamp and cwd helpers and keeps their semantics: id, cwd, `model_provider`, `originator` and the fallback model come from the first `session_meta` whose payload decodes; the timestamp from the first `session_meta` whose timestamp parses; the model from the first `turn_context` (Decision 29). It stops once all are found. Values found before a scan error are kept.

Gotcha: the old timestamp helper returned the modification time *without an error* when no `session_meta` was found, so the resolver reported it as recorded. `readCodexHead` returns a zero timestamp instead and both the lister and resolver flag the fallback (Q105).

## Existing timestamp scan contracts

- Claude listing uses `transcript.ParseFirstTimestamp`: it scans until a timestamp is found, with a 10 MiB per-line limit and no 50-line or 1-MiB total cap. An invalid nonempty timestamp returns an error; listing then falls back to modification time.
- `readCodexHead` retains the 50-line, 10-MiB-per-line scan behaviour. Timestamp extraction failures fall back to modification time.
- A metadata cache must preserve those outcomes. Applying a new global head limit to existing timestamps can change listing order and `latest` even without a feature flag.

## Session resolution by ID

`resolveInput()` and `resolveFollowInput()` delegate to `sessions.Resolver`; with `-s`/`--source` they try only that source and skip file-path detection. Codex branches use `selectCodexCandidate()`. It searches globally without project filtering, matches a filename UUID or a full non-UUID basename case-insensitively, and keeps the first walk-order match before validating its content/readability. It walks through `walkCodexRollouts`, the same walk `listCodex` uses, so the listing's first row per ID is the resolver's choice. (A followed directory symlink used to let a later match overwrite the first; Q107.)

Duplicate filename IDs can therefore shadow a later valid file. Any derived index that claims an ID is openable must validate the actual selected candidate, rather than any matching copy.

## All-projects scope

`NewResolver("")` opens every source without turning `""` into the working directory: Claude searches project directories in sorted order; Codex and Copilot are global anyway; Kiro CLI uses `logs.GetSessionByID` (`updated_at DESC, key ASC`, the same order `DiscoverAll` now de-duplicates with); Kiro IDE scans every workspace under `KiroIDEBasePath` and takes `CostPath` from the winning workspace.

`ListAll("")` lists each `(source, id)` once, choosing the copy the resolver opens, with a `duplicate_session` warning per skipped copy. Each collector does this beside its own rule (Q106): Claude keeps the first project directory; Codex keeps the first walk match and lists it only if `codexHeadWarning` passes (readable, `session_meta` present, recorded id equals the filename UUID), otherwise omits the ID without promoting a later copy (Decision 22, Decision 33); Kiro IDE uses `bestKiroIDECandidate` across workspaces. Project-scope Codex rows keep per-file cwd filtering, even when the global winner is elsewhere (Q62).

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

`internal/sessions/kiro_ide.go` holds the candidate rule the lister and resolver share: `kiroIDEWorkspaceCandidates` scans one workspace and `bestKiroIDECandidate` picks most chat messages, then newest mtime, then lexically smallest path. `resolveKiroIDE` and `ResolvePath(SourceKiroIDE)` go through `locateKiroIDE`. Symlinks resolving outside the workspace are skipped inside the scan loop (not after it), so a symlink with more messages cannot shadow a legitimate file.

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
