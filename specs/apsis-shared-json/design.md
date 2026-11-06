# Design: Apsis — Shared JSON Output, Codex Exec Streams and Cross-Project Listing

## Overview

`transcript.Entry` stays the one shared representation that every parser produces and every renderer consumes. This feature adds the content kinds exec streams need, a provider field, the recorded session origin and per-entry record identifiers (shared JSON and listings only), and a separate projection of that representation into versioned, harness-neutral JSON (`-f shared-json`). `codex exec --json` streams become source `codex-exec`, discovered only under configured roots. The existing `ListAll("")` path becomes reachable through `--all-projects`, and resolution gains all-projects branches so every listed session opens from any directory. Requirements are in [requirements.md](requirements.md); decisions referenced below are in [decision_log.md](decision_log.md).

## Architecture

```
harness files ──parse──▶ transcript.ParseResult (Entries, Warnings, Metadata, Format)
                               │
            ┌──────────────────┼──────────────────────────┐
     RenderMarkdown     RenderHTML / RenderHTMLFragment   sharedjson.Session
                                                          sharedjson.Listing ◀── sessions.Lister.ListAll
```

### Placement

| Path | Contents |
|---|---|
| `internal/transcript/types.go`, `parser.go` | New `Entry`/`ContentItem` fields (including `RecordID` and the Claude `entrypoint` carrier `Origin`), `FormatCodexExec`, `ParseResult.Format`, metadata fields, warning codes/units, exec probe in `DetectFormat` |
| `internal/transcript/codex_exec_types.go`, `codex_exec_parser.go` (new) | Exec event structs, `ParseCodexExec`, `ProbeExecStream` |
| `internal/transcript/codex_types.go`, `codex_parser.go` | `model_provider` capture and propagation; `turn_context` model capture (Decision 29); `originator` and message item `id` capture (Q87, Decision 32) |
| `internal/transcript/copilot_parser.go`, `kiro_parser.go`, `kiro_ide_parser.go` | `RecordID` at one-record entry sites (Decision 32) |
| `internal/transcript/markdown.go`, `html.go`, `metadata.go` | Rendering of new blocks; `modelLabel` |
| `internal/sharedjson/` (new) | Document types, `Session`, `Listing`, `Write`, `SchemaVersion`; imports `transcript` and `sessions`, so neither imports it |
| `docs/schemas/apsis-shared-json.v1.schema.json` (new) | Published JSON Schema (draft 2020-12) for both document kinds |
| `internal/sessions/codexexec.go` (new) | Root canonicalisation, walk, qualification, identifier, repository root, suppression |
| `internal/sessions/cache.go` (new) | Stat-keyed listing-metadata cache |
| `internal/fileidentity/` (new) | Metadata-only Unix/Windows identity/change fingerprints behind one injectable interface; the cache consumes this isolated package (Decision 26) |
| `internal/sessions/lister.go`, `resolver.go`, `types.go` | New source, listing fields, all-projects resolution, de-duplication, ordering |
| `internal/sessions/parse.go` (new) | `ParseResolved`, shared by CLI and web |
| `internal/agents/kiro/logs/session.go`, `discover.go` | `GetSessionByID`; deterministic tie order in `DiscoverAll` |
| `cmd/apsis/main.go` | Flags, input context, shared-JSON dispatch, follow checks |
| `internal/apsisweb/` | Config, list and transcript views, warning banner in the polled fragment, source badge CSS |
| `internal/transcript/testdata/codex_exec/` (new) | Synthetic exec fixtures plus `PROVENANCE.md` |

### Parity audit: new source and format

| Site | Needs equivalent | Note |
|---|---|---|
| `sessions/types.go` `allSources`, `displayNames`, `IsValidSource` | yes | `codex-exec` |
| `Lister.ListAll` collectors | yes | `listCodexExec` runs after `listCodex`, which supplies the suppression index |
| `sortSessionsByTimestamp` | yes | priority 5, then identifier tiebreak for every source |
| `Resolver.Resolve`, `ResolvePath` | yes | `ResolvePath` returns the stream path; follow callers reject it |
| `Resolver.resolveFileCreatedAt` | yes | exec returns no recorded time; modification time only as the flagged fallback |
| `cmd/apsis` `resolveInput` bare-ID source list | yes | `codex-exec` last, only when roots are configured |
| `cmd/apsis` `resolveFollowInput`, `runLatest` `fileBackedSources` | no | exec is rejected with a named error ([Error Handling](#error-handling)) |
| `cmd/apsis` `agentToFormat`, `-a`/`-f` help | yes | `codex-exec`, `shared-json` |
| `cmd/apsis` `convertToJSON` | yes | strips a leading BOM for exec input; the JSONL line path is otherwise unchanged |
| `transcript` `DetectFormat`, `detectJSONLFormat`, `Parse`, `ParseJSONLWithFormat` | yes | exec probe and dispatch |
| `transcript` `detectFormatFromLine`, `ParseJSONL` | no | used by follow mode and orbit's web view, neither of which accepts exec |
| `apsisweb` `ValidateSource` | yes | via `IsValidSource` |
| `apsisweb` `buildSessionListData` `Sources` | yes | `codex-exec` pill only when roots are configured |
| `apsisweb` `handleTranscript` parse branch | yes | replaced by `sessions.ParseResolved` |
| `apsisweb/static/style.css` badges, `CSSVersion` | yes | `.source-codex-exec` light and dark; bump version |
| `internal/web` (orbit), `internal/status` last-entry | no | Claude and Codex rollouts only |

### Parity audit: new content kinds

| Site | Needs equivalent | Note |
|---|---|---|
| `markdown.go` `formatAssistantMessage` | yes | `plan`, `error`, `event`, `usage`, incomplete state |
| `html.go` `formatAssistantMessageHTML` | yes | both the `hasContent` check and the render pass, else entries holding only new kinds vanish |
| `formatToolInput`, `formatToolInputHTML` | yes | `command_execution`, `file_change`, `mcp_tool_call`, `web_search` |
| `formatToolResult`, `formatToolResultHTML` | yes | exit code, incomplete marker |
| `formatUserMessage`, `formatUserMessageHTML` | no | exec entries are assistant entries |
| `grouping.go` `isReadOnlyEntry`, `isEditOnlyEntry` | no | the `default` branch rejects unknown kinds; exec tool names are never `Read`/`Edit` |
| `BuildToolMeta`, `preprocessEntries` result map | no | exec tool IDs are turn-qualified, so transcript-wide maps keyed by tool ID stay unambiguous |
| `last_entry.go` | no | orbit status only |

### Parity audit: provider

| Site | Needs equivalent | Note |
|---|---|---|
| `codex_parser.go` `processSessionMeta`, `ensureAssistantEntry` | yes | provider state beside model state; an entry takes both when it is created; the model comes from `turn_context` (Decision 29) |
| Assistant header meta in `formatAssistantMessage` and `formatAssistantMessageHTML` | yes | `modelLabel(entry.Model, entry.Provider)` |
| Read/Edit group headers, user headers, slash commands, deferred tool-result sections | no | they show no model today; the provider appears wherever the model appears (Decision 16) |
| `kiro_ide_parser.go` assistant entries | yes | entries that take `metadata.modelId` also take `metadata.modelProvider`, so headers show `modelLabel(model, provider)` (requirements revision 5, Q93) |
| Copilot, Kiro CLI parsers | no | 3.1 names only the Codex and Kiro IDE providers |

### Parity audit: origin and entry identifiers

| Site | Needs equivalent | Note |
|---|---|---|
| `sharedjson` session metadata, entry and listing row | yes | `origin` and entry `id` (Q88) |
| Claude and Codex listing extraction | yes | bounded head scans also return `entrypoint`/`originator` (Q90) |
| Markdown, standalone HTML, fragment, web views | no | Decision 31 keeps origin and identifiers out of rendered output |
| raw `-f json` (`convertToJSON`) | no | passes source lines through as `json.RawMessage`; `Entry` is never re-serialised |
| `grouping.go` and renderer skill/slash-command lookups on `Entry.UUID`/`ParentUUID` | no | `RecordID` is a separate field, so these lookups and their output are unchanged |
| `internal/web` (orbit), `internal/status` last-entry | no | they do not read shared JSON metadata |

## Components and Interfaces

### 1. Shared representation additions (`transcript`)

```go
type Entry struct {
    // existing fields unchanged
    Provider string `json:"-"` // recorded provider; set by the Codex rollout and Kiro IDE parsers (Q93)
    Turn     int    `json:"-"` // 1-based exec turn; 0 = no turn
    RecordID string `json:"-"` // identifier of the single source record this entry is built from (Decision 32)
    Origin   string `json:"-"` // Claude `entrypoint`, decoded by the UnmarshalJSON alias; feeds Metadata.Origin (Q89)
}

type ContentItem struct {
    // existing fields unchanged; new fields are `json:"-"` so Claude unmarshalling is unaffected
    ItemID     string          // source item identifier (exec)
    Status     string          // source item status as recorded
    Incomplete bool            // started, never completed before EOF
    ExitCode   *int            // command exit code; nil when unrecorded
    Plan       []PlanItem      // Type "plan"
    EventType  string          // Type "event": source event or item type
    Payload    json.RawMessage // Type "event": parsed payload, compact
    Usage      *Usage          // Type "usage"
    // Type "error" uses Text for the message
}
type PlanItem struct { Text string; Completed bool }
type Usage struct {
    Basis    string         // "thread_total_at_turn_completion" for Codex 0.166.0
    Counters []UsageCounter // source key order; unknown keys kept
}
type UsageCounter struct { Name string; Value json.RawMessage }

type ParseResult struct { /* existing */ Format Format } // format parsed; FormatUnknown for the Claude fallback
type ParseResultMetadata struct { /* existing */ StartedAt, Project, ThreadID, FirstModel, FirstProvider, Origin string }
type ParseWarning struct { Line int; Message string; Code string; Unit string } // Unit "" = physical line, "chat-message" = chat index, "execution-action" = execution-detail action index
```

`ParseResult.Metadata` may now be non-nil with a nil `TotalCost`; renderers already guard on `TotalCost != nil`. Kiro IDE chat-conversion warnings use `Unit: "chat-message"`; execution-detail action warnings use `Unit: "execution-action"`. Their existing one-based `Line` values and stderr messages remain unchanged (Q75 supersedes Q39).

Session-level metadata comes from parsed content first; resolver metadata fills only fields still missing and never supplies a filesystem time as `created_at`. `created_at` follows Decision 30: a parsed session-start record, then a resolver-recorded creation time that is not a filesystem fallback, then the first entry timestamp. Parsers set `StartedAt` only from start records. Parsers retain the first nonempty model/provider they already recognise in `FirstModel`/`FirstProvider`, including source headers, `turn_context` records and model-change records before assistant content. Entries keep the active recorded values. This introduces no Claude model extraction or Copilot origin mapping (U1 and U5 answered: leave out); Kiro IDE provider extraction follows the U2 answer (Q93).

The current Codex parser errors when it produces no entries, including a header-only rollout. Preserve that default. Add `ParseOptions.AllowEmptyCodex`, passed through backward-compatible variadic options on `Parse` and `ParseCodexJSONL` and through `ParseJSONLWithFormat`. Only shared-JSON conversion enables it, returning the parsed header metadata with `entries: []`; existing Markdown, HTML and web callers retain their current acceptance and errors. User-only rollouts retain their existing entry behaviour (Q71).

Codex 0.166.0 records the model in `turn_context.payload.model`, not in `session_meta` (Decision 29). The active model is the latest nonempty `turn_context` model, else `session_meta.model` for older files. The current `ensureAssistantEntry` consolidates adjacent assistant content, and a repeated header alone creates no boundary. Preserve consolidation for identical metadata. At the next model-produced item (message, reasoning, function call, agent message or reasoning; a function call output joins the current entry, Q82), a provider transition starts a new metadata-bearing entry under 3.1. With `ParseOptions.PreserveModelChanges`, enabled by shared-JSON callers alongside `AllowEmptyCodex`, assistant entries take the active model and a model-only transition starts a new entry. Without it, `Entry.Model` keeps its legacy session_meta-only value, so default Markdown/HTML/web output changes only by the provider label; whether headers show the Codex model is question U4. Never retroactively relabel prior content or split on an identical header. This one parser/common-entry path reconciles approved 1.6 with protected 5.1 output (Decision 29); tests cover interleaved tool links across these boundaries. Renderer Read/Edit grouping and its metadata-free headers remain unchanged under Decision 16.

| Source | `created_at` | `project` | `model` | `provider` | `origin` |
|---|---|---|---|---|---|
| claude | first parseable entry timestamp | first entry `cwd` | omitted (question U1) | omitted | first nonempty `entrypoint` |
| codex | first `session_meta` outer `CodexEntry.Timestamp` that parses as RFC3339 (Q81); no payload timestamp fallback | `session_meta.cwd` | first recorded `turn_context` model, else `session_meta` model (Decision 29) | first recorded provider, including session_meta | first nonempty `session_meta.payload.originator` |
| copilot | `session.start` `data.startTime`, else first parseable event timestamp; then resolver `workspace.yaml` `created_at` (Decision 30) | resolver: `workspace.yaml` `git_root`, else `cwd` | first recognised model-change/entry model | omitted | omitted (question U5) |
| kiro-cli | resolver database `created_at` (`DatabaseCreatedAt`, Q103), else first entry timestamp; the parser sets no `StartedAt` (Decision 30) | resolver: database `key` | first recognised request/entry model | omitted | omitted |
| kiro-ide | `metadata.startTime` | omitted (the workspace hash is one-way) | metadata.modelId, else first entry model | first recorded `metadata.modelProvider` (Q93) | omitted |
| codex-exec | omitted | containing repository of the input path; none for stdin | omitted | omitted | omitted (streams hold no `session_meta`) |

Origin is the value as recorded, with no classification into interactive or automated (1.7, Decision 31). Like model and provider, it is a first-known summary, but entries do not carry it: 1.7 places origin in session and listing metadata only, so a later different value in the same transcript is not reported (Q87).

Each entry's `RecordID` follows Decision 32: it is set only where an entry is built from exactly one record that records an identifier, and entries split from one record share it.

| Source | Entries with `RecordID` | Entries without |
|---|---|---|
| claude | every entry: its line's `uuid` | lines without `uuid` |
| codex | user entries: the `message` item's `payload.id` when recorded (optional in pinned `models.rs`) | assistant entries, which consolidate several response items |
| copilot | `user.message` entries: the event `id` | assistant turn entries and tool-result entries, each built from several events |
| kiro-cli | entries from one assistant message (tool use or text response): its `message_id` | prompt and tool-result entries, which record no identifier |
| kiro-ide | entries from one execution-detail action: its `actionId`; a tool action's tool-use and tool-result entries share it | entries from chat-file messages |
| codex-exec | none | every turn entry; item identifiers stay on blocks as `item_id` |

`RecordID` never replaces or feeds a tool-linking field (`ID`, `ToolUseID`), even where values coincide, as with Kiro IDE action IDs. `Entry.UUID` and `ParentUUID` keep their Claude-only renderer uses.

### 2. Codex exec stream parser

`ParseCodexExec(r io.Reader) (*ParseResult, error)` returns an error only on reader failure. An empty or event-less stream yields zero entries. Lines are decoded with `json.Decoder.UseNumber` or kept as `json.RawMessage`, so numbers in inputs, payloads and usage keep their precision.

**Detection** (2.1, Decision 21). `ProbeExecStream(r)` reads whole lines until the first line that parses as a JSON object, skipping blank, malformed and over-10-MiB lines, and classifies the stream: `exec` when that object is `thread.started` with a nonempty string `thread_id`; `exec-without-thread` when its `type` is another exec event (`thread.started` without a valid ID, `turn.*`, `item.*`, `error`); `other` otherwise. `DetectFormat` strips the BOM and retains the existing detection result as a fallback. Complete, structurally validated Kiro JSON retains its existing recognition; substring-only Kiro matches are provisional. When the first complete JSON object is not yet available in the 64 KiB chunk, the exec probe continues even if a Kiro heuristic matched, recombining a split physical line and retaining every consumed byte for replay. Only `exec` selects `FormatCodexExec`; `exec-without-thread` and `other` retain the legacy result. Extended probing never promotes another format that the legacy detector rejected. `-a codex-exec` forces parsing; discovery uses the same complete-line probe and warns/omits threadless exec candidates.

**Lifecycle** (2.3, Decision 15). The parser keeps an ordered list of turns. Turn 0 holds records before the first `turn.started`; each `turn.started` opens the next turn. Within a turn, records are keyed and kept in first-appearance order:

| Event | Key | Effect |
|---|---|---|
| `thread.started` | — | the first nonempty `thread_id` becomes `Metadata.ThreadID`; a later one with the same ID is dropped and does not reset turn numbering; a different ID becomes an `event` record plus a `thread_mismatch` warning |
| `item.started`, `item.updated`, `item.completed` | `(turn, item.id)`; a missing ID keys by physical line | first sighting appends; later events replace the stored item JSON; `completed` marks it done |
| `turn.completed` | own record | `usage` block |
| `turn.failed`, top-level `error` | own record | `error` block with `message`; the original JSON is kept as payload |
| any other event type | own record | `event` block |

At EOF each turn becomes one assistant `Entry` with `Turn` set and no timestamp, model or provider. Its content is the translation of each record's final state. Items without completion get `Incomplete: true`, on their tool result as well. Lifecycle events never render separately.

**Item translation** (2.2; shapes from the pinned 0.166.0 `exec_events.rs`):

| Item `type` | Content |
|---|---|
| `agent_message` | `text` |
| `reasoning` | `thinking` |
| `command_execution` | `tool_use` (name `command_execution`, input `{command}`) + `tool_result` (content `aggregated_output`, `ExitCode`, `Status`; `IsError` when status is `failed`/`declined` or exit code ≠ 0) |
| `file_change` | `tool_use` (input `{changes}`) + `tool_result` (one `kind path` line per change, `Status`; `IsError` when `failed`) |
| `mcp_tool_call` | `tool_use` (input `{server, tool, arguments}`) + `tool_result` (compact JSON of `result`, or `error.message`; `IsError` when `failed` or `error` is set) |
| `web_search` | `tool_use` (input `{query, action}`) + `tool_result` (compact JSON of `results` when present) |
| `todo_list` | `plan` |
| `error` | `error` |
| anything else, including `collab_tool_call` | `event` with `EventType` = item type, payload = final item JSON |

Every exec tool item emits its `tool_result`, because the renderers draw non-Task tools only when their result arrives. Tool IDs are `t<turn>:<item id>`; `ItemID` keeps the original. Specialised translation requires the nonempty string item ID required by the pinned definition as well as valid typed payload fields. A known item with a missing/empty/invalid ID or invalid payload becomes an `event` block with an `item_decode_failed` warning; it creates no tool links. ID-less records remain individually keyed by physical line, so unrelated invalid shapes cannot collide (5.6).

**Warnings** (5.2). A malformed line yields `ParseWarning{Line: <physical 1-based line>, Code: "malformed_line", Message: "malformed JSON event"}`, with ` (offset N)` appended for a `*json.SyntaxError`. A line over 10 MiB is discarded through its newline with `oversized_line`. Messages never contain source text. Parsing continues at the next line; I/O errors are fatal.

### 3. Shared JSON projection (`internal/sharedjson`)

```go
const SchemaVersion = 1
type SessionContext struct {
    Source   string                    // from the resolver; else from ParseResult.Format
    ID       string                    // listing identifier when known
    Project  string                    // input's project: an exec stream file's containing repository (Q110)
    Resolved *sessions.SessionMetadata // nil for file and stdin input
}
func Session(res *transcript.ParseResult, ctx SessionContext) SessionDocument
func Listing(list []sessions.SessionInfo, warnings []sessions.ListWarning, scope Scope) ListingDocument
func Write(w io.Writer, doc any) error // json.Encoder, two-space indent, SetEscapeHTML(false), trailing newline
```

- The projection reads parsed data only and never calls renderer helpers, so content is never shortened (1.5, Q29).
- Document-level `model`/`provider` use parsed `FirstModel`/`FirstProvider`, with the first nonempty entry value as a fallback for parsers without a source summary; entries keep their own values (1.6). Header-only and user-only Codex input therefore retains recorded summary metadata.
- Document-level `origin` uses parsed `Metadata.Origin` only; there is no entry fallback and no resolver source for it. Each entry's `id` is its `RecordID`, omitted when empty (1.7, 1.8).
- Root arrays are `[]`, never `null`.
- `convert` dispatches shared JSON before its `Session contains no entries` early return, so an empty session still emits a document.
- File and stdin conversion need no listing: everything comes from the parse result and the input context.

### 4. Listing, scope and cache (`sessions`)

```go
type SessionInfo struct {
    ID, Source string; CreatedAt time.Time; Size int64 // existing; CreatedAt stays the sort time
    TimeFromFilesystem bool  // CreatedAt is the file modification time, not a recorded time
    Project            string
    Model, Provider    string // when the listing read yields them
    Origin             string // Claude `entrypoint` or Codex `originator` from the bounded head scan (Q90)
    ThreadID           string // codex-exec only
}
type SessionMetadata struct { /* existing */ TimeFromFilesystem bool; DatabaseCreatedAt time.Time; Project, ThreadID, Path string } // DatabaseCreatedAt: Kiro CLI only (Q103)
type ListWarning struct { Source string; Code string; Err error } // Code added

type Options struct { CodexExecRoots []string; ExtendedOrdering bool }
type Option func(*Options) // shared settings, applied by both constructors
func WithCodexExecRoots(roots []string) Option
func WithExtendedOrdering(enabled bool) Option
func NewLister(opts ...Option) (*Lister, error)
func NewResolver(projectPath string, opts ...Option) (*Resolver, error) // "" = all projects
func (l *Lister) Sources() []string // AllSources minus codex-exec when no roots are configured
```

`ListAll("")` is the all-projects listing. A `Lister` is long-lived in `apsis serve` and per-command in the CLI (Decision 23). Each pass:

1. Holds `l.mu` for the whole pass, so a page load and an HTMX poll never interleave cache updates.
2. Enumerates directories and stats candidates and their companions without opening transcript bodies.
3. Looks up `cache[path]`. Unix keys include size, mtime, ctime, inode and device. Windows keys include size, last-write time, change time, volume serial number and file ID, queried through a metadata-only handle using the existing `golang.org/x/sys/windows` module. Companion files are part of the key, and an absent companion is recorded as absent, so creating one changes the key. A fingerprint failure is an explicit warning/error outcome, never a cache hit based on size/mtime alone; the platform capability gate below must pass before this cache slice is admitted.
4. On a hit, reuses the cached metadata. On a miss, runs the source's extraction (table below), restats, and caches the result only when the key is unchanged; a changed key uses the result for this pass and retries on the next.
5. Omits a changed file whose extraction fails, with a warning, rather than serving its previous metadata. An existing source's documented timestamp-to-mtime fallback is a successful extraction outcome, not a new fatal failure.
6. Sweeps cache entries not seen in the pass.
7. Returns a freshly allocated slice; callers such as the web handler's `slices.Reverse` cannot reorder cached state.

| Source | Key files | Extracted metadata | Project | Time |
|---|---|---|---|---|
| claude | transcript | timestamp uses existing `ParseFirstTimestamp` scan and error semantics (10 MiB per-line limit, no total 50-line/1-MiB cap); optional `cwd` and `entrypoint` extraction stops at 50 lines/1 MiB or when both are found (Q90) | first extracted `cwd` | existing timestamp result, else mtime (flagged) |
| codex | rollout | one `session_meta` scan preserving existing `codexMetaScanLimit` and 10 MiB per-line limit: `id`, `timestamp`, `cwd`, `model_provider` and `originator` from `session_meta` and `model` from the first `turn_context` (else `session_meta`) within the same limit (Decision 29, Q90) | `cwd` | existing timestamp result, else mtime (flagged) |
| copilot | `events.jsonl`, `workspace.yaml` | `workspace.yaml` fields | `git_root`, else `cwd` | `created_at`, else mtime (flagged) |
| kiro-cli | `data.sqlite3`, `-wal`, `-journal` | `DiscoverAll` result; unchanged keys skip opening the database | database `key` | `created_at` |
| kiro-ide | `.chat` file | execution ID, chat count, `startTime`, `modelId`, `modelProvider` (whole file, as today) | omitted | `startTime`, else mtime (flagged) |
| codex-exec | stream | probe result and `thread_id` | containing repository | mtime (flagged) |

Repository roots for exec streams are found by statting ancestors on each pass, memoised per directory within the pass only, so a newly created nearer repository is picked up on the next pass.

Listing `model`/`provider`/`origin` come only from these extraction reads; the session document is authoritative. Copilot, Kiro CLI, Kiro IDE and codex-exec rows carry no origin.

The Kiro discovery injection must cover both project and all-projects branches: today `listKiro("")` bypasses `kiroDiscover` and calls `logs.DiscoverAll` directly. Count both paths in cache tests. The fingerprint package uses local Windows ABI structs for `FileIdInfo`/`FileBasicInfo`, since the existing x/sys version exposes the query and class constants but not those structs; promote x/sys to a direct module requirement only after the schema-validator task releases module-file ownership.

**Ordering and uniqueness** (4.4). In extended mode sort by `CreatedAt` ascending, unknown times first, then source priority (claude, copilot, codex, kiro-cli, kiro-ide, codex-exec), then identifier. The CLI enables `ExtendedOrdering` for shared JSON, all-projects scope or configured exec roots; the web enables it for all-projects scope or configured roots. No-flag project listings/latest retain the existing sort and timestamp/fallback behaviour (5.1). In all-projects scope a `(source, id)` pair appears once: a duplicate keeps the candidate the resolver opens and adds a `duplicate_session` warning. The resolver's choice is first in sorted directory order for Claude, the existing first filename match in walk order for Codex rollouts, the shared best-candidate rule for Kiro IDE, and most recent `updated_at` then lowest `key` for Kiro CLI. Codex all-projects row deduplication, resolution and global suppression share `selectCodexCandidate`; an invalid selected head is omitted from all-projects rows with a warning, and a later valid duplicate is not silently promoted. Project-scope Codex rows retain existing per-file enumeration and cwd filtering, even when a different project holds the global winner (Q49/Q62). Each collector de-duplicates beside its own selection rule before sorting, returning its warnings (Q106), and every selected Codex rollout, duplicated or not, must pass the head checks to be listed across projects (Decision 33).

### 5. Exec discovery, identity, project and suppression

**Roots** (2.5, 5.5). Each configured root is made absolute, passed through `filepath.EvalSymlinks`, and rewritten to its on-disk spelling component by component (an exact name in the parent's listing wins; otherwise a unique case-insensitive match), so case aliases on case-insensitive volumes canonicalise to one path. Identical canonical roots collapse to one. A root that fails yields a `root_unreadable` warning and is skipped. `filepath.WalkDir` walks each root and never descends directory symlinks. Subdirectories named `.git`, `node_modules` or `testdata` are skipped during descent; a configured root is scanned whatever its name, including when it is nested inside another root's skipped subdirectory. A `.jsonl` regular file is a candidate at its walked path. A `.jsonl` symlink is resolved and canonicalised the same way, and kept only when the target is a regular file inside some canonical root. Candidates are de-duplicated by canonical path. Unreadable directories and files yield warnings and the walk continues.

**Qualification** (2.1, 2.11). The cached probe result decides: `exec` is listed; `exec-without-thread` is omitted with a `missing_thread` warning; `other` is ignored silently.

**Identifier** (2.8, Decision 14). `sanitise(thread_id)[:191] + "-" + hex(sha256(canonical path))`, at most 256 characters. `sanitise` maps every byte outside `[A-Za-z0-9_-]` to `_`. The identifier depends only on the stream's own canonical path and recorded thread ID, so appends, added copies, overlapping roots and root order cannot change it. If two distinct canonical paths produce the same identifier, both are omitted with an `identifier_conflict` warning. `ThreadID` keeps the original value. Direct file conversion computes the same identifier without configured roots; stdin has none.

Tests inject only the digest helper to force a conflict between distinct paths with identical thread prefixes, then assert both omissions and the warning under either root order. Production keeps full SHA-256; real collision generation is not a test prerequisite.

**Project** (2.6). Walk up from the stream's directory to the nearest ancestor holding `.git`: a directory for a repository, or a file starting with `gitdir:` for a worktree. No `git` executable is invoked. Project scope keeps a stream only when `normalizePath(root) == normalizePath(project)`. A stream outside any repository lists only in all-projects scope.

**Suppression** (2.7, Decision 22). `selectCodexCandidate` applies the existing filename/UUID matching and first-walk-order choice globally, and is shared by the resolver, suppression index and all-projects row deduplication. This global selection does not replace legacy project-row enumeration or per-file cwd filtering. The suppression index contains only that winner's recorded `session_meta.payload.id`, after confirming the selected file is contained, readable, has a parseable Codex head and records the filename UUID (ignoring case). An invalid winner contributes no index entry, even if a later duplicate is valid; streams remain available with a source warning. A matching indexed stream is dropped before project filtering in every scope. A failed or incomplete rollout scan never manufactures a suppressing candidate. Suppression never blocks explicit stream-file or `codex-exec` identifier opening.

### 6. Resolution in all-projects scope

`Resolver` with `projectPath == ""`:

| Source | Behaviour |
|---|---|
| claude | existing all-projects search, sorted directory order |
| codex | global lookup through shared `selectCodexCandidate`, preserving its first filename-match choice |
| copilot | unchanged (already global) |
| kiro-cli | new `logs.(*DB).GetSessionByID(ctx, id)`: `SELECT key, value … WHERE conversation_id = ? ORDER BY updated_at DESC, key ASC LIMIT 1`; `DiscoverAll` uses the same order, so both pick the same row; returns `Project = key` |
| kiro-ide | scans every workspace under `KiroIDEBasePath` with the `bestKiroIDECandidate` function the lister also uses (most chat entries, newest mtime, lexically smallest path); `CostPath` derives from the winning workspace |
| codex-exec | both scopes: take the 64-hex suffix, hash each candidate canonical path from a metadata-only walk of the roots, probe only the matching file, require its sanitised thread prefix to match, recheck root containment on the resolved path, then open |

These branches stop `logs.GetSession` and `KiroIDEWorkspaceDir` from turning `""` into the current directory through `filepath.Abs`. The resolver also returns `Path`, `Project`, `ThreadID` and `TimeFromFilesystem` in `SessionMetadata`.

In project scope Kiro CLI must also return the actual database key that matched after its normalised/resolved-key attempts. Add a metadata-returning companion to `GetSession` or extend the internal result while preserving the existing reader API for current callers; do not substitute the requested directory for the matched key.

`sessions.ParseResolved(rs *ResolvedSession, opts ...transcript.ParseOptions) (*transcript.ParseResult, error)` receives the caller's parse options. It copies the option value, overlays a nonempty resolved Kiro IDE cost path without discarding `AllowEmptyCodex`/`PreserveModelChanges`, forces `FormatKiroIDE` for that source, forces `FormatCodexExec` for codex-exec, and auto-detects otherwise. The resolved Kiro IDE cost path stays paired with its selected transcript/workspace.

The CLI keeps explicit `-a` dispatch before the helper and passes the same options to `ParseJSONLWithFormat`, so source-based selection cannot override a forced format. Without `-a`, resolved CLI input uses the helper; the CLI alone enables both Codex flags for shared JSON. The web calls the helper with default options. Task 14 regresses the same header-only/model-changing Codex session through explicit-source ID and `latest`: shared JSON retains empty metadata or separate model runs, while default Markdown/HTML/web retains prior errors/grouping. Kiro IDE cost and forced-format parity are tested alongside these cases (Q74).

### 7. CLI surface (`apsis`)

| Option | Behaviour |
|---|---|
| `-f shared-json` | session document for an ID, file, stdin or `latest`; listing document with `--list` |
| `--all-projects` | lists, `latest` and ID lookups use project `""`; usage error with `-p`/`--project`; no environment variable (Q31) |
| `-s`, `--source <source>` | the positional argument is an identifier of that source, using source names (`claude`, `codex`, `codex-exec`, `copilot`, `kiro-cli`, `kiro-ide`); no file-path detection; usage error without an argument, with an unknown source, or with stdin, `--list` or `latest` |
| `--codex-exec-dir <dir>` (repeatable) | exec roots; any occurrence replaces `APSIS_CODEX_EXEC_DIRS` entirely; a single empty value means no roots; mixing empty and nonempty values is a usage error |
| `APSIS_CODEX_EXEC_DIRS` | split with `filepath.SplitList` (`:` on Unix, `;` on Windows); unset or empty means no roots |
| `-a codex-exec` | forces the exec parser |

`apsis serve` accepts `--all-projects` and `--codex-exec-dir`, and reads the same environment variable (5.4). Relative roots resolve against the working directory.

`resolveInput` returns an input context: reader, source, ID, canonical path, cost path and resolver metadata. `convert` builds `SessionContext` from it. For exec input, the Markdown/HTML header shows `Thread ID` from content and `Session ID` (the derived identifier) when a path exists; it never uses the filename stem (2.11). Text listing output is unchanged except:

- In all-projects scope a fifth tab-separated column holds the project, or `-`.
- codex-exec rows print the time followed by ` (file modified)`.
- `apsis latest` on a codex-exec session adds the same suffix to its `Using … from …` stderr line.

### 8. `apsis serve`

`apsisweb.Config` gains `AllProjects bool` and `CodexExecRoots []string`. One `Lister` lives for the server's lifetime and carries the cache across refreshes. `SessionView` gains `Project` (all-projects scope only) and `FileTime bool`, which renders `file modified` beside codex-exec times. The `sessions_list` fragment renders the warning banner, so warnings follow each refresh; the page includes the fragment, so the banner is not duplicated. `TranscriptViewData` gains `Warnings`. For codex-exec sessions only, the transcript view shows parse warnings with the existing `.warning-banner` style and shows thread ID and modification time in `session-meta` (Q47).

### 9. Rendering of new content

The existing patterns are the baseline; only differences are listed.

| Content | Markdown | HTML (standalone and fragment share `renderEntriesToBuilder`) |
|---|---|---|
| assistant meta | `modelLabel`: `model (provider)`, provider alone `(provider)`; exec turns append `turn N` as another ` · ` part | same parts via `FormatMessageMetaHTML` |
| `command_execution` input | same block as `Bash` | same as `Bash` in `formatToolInputHTML` |
| `file_change` input | `- kind path` list | same list markup |
| exit code | `**Result (exit code N):**` | same label |
| incomplete item | ⏳ and `(incomplete)` in the summary instead of ✅/❌ | same |
| `plan` | checkbox list matching the `TodoWrite` rendering | `TodoWrite` markup |
| `error` | `❌ **Error:** message` | failed-tool styling |
| `event` | collapsed `<details>` `📦 Event: type` with an escaped JSON block | collapsed block like non-Task tools; JSON escaped, never raw HTML |
| `usage` | `**Token usage (thread totals at turn completion):** name value · …` | same text in a `usage` div; one CSS rule |

## Data Models

### Session document

```json
{
  "schema_version": 1,
  "kind": "session",
  "session": {
    "source": "codex-exec",
    "id": "0199aaaa-0000-7000-8000-000000000001-3f2a9c0d1b7e4a56c8d90e1f2a3b4c5d6e7f8091a2b3c4d5e6f708192a3b4c5d",
    "thread_id": "0199aaaa-0000-7000-8000-000000000001",
    "project": "/work/example-repo"
  },
  "entries": [
    {
      "role": "assistant",
      "turn": 1,
      "content": [
        { "type": "tool_use", "id": "t1:item_1", "item_id": "item_1", "name": "command_execution", "input": { "command": "make test" } },
        { "type": "tool_result", "tool_use_id": "t1:item_1", "item_id": "item_1", "content": "ok\n", "is_error": false, "exit_code": 0, "status": "completed" },
        { "type": "plan", "item_id": "item_2", "incomplete": true, "items": [{ "text": "Run tests", "completed": true }] },
        { "type": "usage", "basis": "thread_total_at_turn_completion", "counters": { "input_tokens": 1200, "cached_input_tokens": 800, "output_tokens": 90 } }
      ]
    }
  ],
  "warnings": [{ "code": "malformed_line", "message": "malformed JSON event (offset 14)", "line": 4 }]
}
```

| Field | Derivation | Omitted when |
|---|---|---|
| `session.source` | resolver source, else `ParseResult.Format` | Claude fallback for unrecognised input |
| `session.id` | listing identifier; for file input the existing filename stem, or the derived exec identifier | stdin; exec without thread |
| `session.thread_id` | `Metadata.ThreadID` | not exec |
| `session.created_at`, `project`, `model`, `provider`, `origin` | metadata table in [Components 1](#1-shared-representation-additions-transcript); `origin` exactly as recorded (1.7) | unknown |
| `session.cost` | `{amount, unit}` from `TotalCost`/`CostUnit` | `TotalCost` nil |
| entry | `id` (`RecordID`, Decision 32; not a tool-linking identifier and not unique across entries), `role` (message role, else entry type), `timestamp`, `model`, `provider`, `turn`, `content` | empty values |
| block common | `type`, `item_id`, `status`, `incomplete` | empty/false |
| block types | `text` and `thinking` (`text`); `tool_use` (`id`, `name`, `input`); `tool_result` (`tool_use_id`, `content`, `is_error` always present, `exit_code` emitted for 0 and omitted only when unrecorded); `plan` (`items`); `error` (`message`); `event` (`event_type`, `payload`); `usage` (`basis`, `counters`); `other` (`source_type`, `text`) for parsed content kinds outside this list | — |
| `warnings[]` | `code`, `message`, and the matching one-based position: `line` (physical line), `chat_message` (Kiro IDE chat index), or `execution_action` (execution-detail action index) | no position |

### Listing document

```json
{
  "schema_version": 1,
  "kind": "listing",
  "scope": { "all_projects": true },
  "sessions": [
    { "source": "codex-exec", "id": "0199aaaa-…-3f2a9c0d…", "thread_id": "0199aaaa-…", "size_bytes": 5120,
      "file_modified_at": "2026-10-01T09:00:00Z", "project": "/work/example-repo" },
    { "source": "codex", "id": "0199bbbb-…", "size_bytes": 40960, "created_at": "2026-10-02T10:00:00Z",
      "project": "/work/other", "model": "example-model", "provider": "openai", "origin": "codex_exec" }
  ],
  "warnings": [{ "source": "codex-exec", "code": "missing_thread", "message": "stream has no thread.started event; omitted" }]
}
```

Listing rows use the same `$defs` session-metadata object as the session document, plus `size_bytes` and `file_modified_at`. `scope` is `{"project": "<absolute path>"}` or `{"all_projects": true}`. Each row carries `created_at` (recorded) or `file_modified_at` (the marked fallback that sets its sort position), and neither when no time is known. Rows follow CLI listing order (oldest first).

Warning codes: `malformed_line`, `oversized_line`, `thread_mismatch`, `item_decode_failed`, `root_unreadable`, `path_unreadable`, `missing_thread`, `identifier_conflict`, `duplicate_session`, `source_failed`, `invalid_record`, `unrecognized_record`, `unmatched_tool_result`. Existing per-source listing failures map to `source_failed` and existing Copilot `workspace.yaml` warnings to `path_unreadable`. Existing transcript parser warnings take their code where they are created (Q80): JSON decode failures `malformed_line`, the Claude buffer limit `oversized_line`, decodable records with missing fields or undecodable payloads `invalid_record`, unknown record/event/item/role/content/action types `unrecognized_record`, and Codex outputs without a call `unmatched_tool_result`; stderr text is unchanged.

### Schema file

- `$schema` draft 2020-12; `$id` is the file's raw URL on upstream `main`.
- Top-level `oneOf` on `kind` (`session`, `listing`); `schema_version` is `const: 1`.
- `$defs` hold session metadata, entry, block (`oneOf` on `type`), warning and scope.
- No object sets `additionalProperties: false`, so version-1 consumers accept additive fields (1.3). `input`, `payload` and `counters` are open objects or arbitrary JSON by definition.
- Publish the complete known v1 contract in PR 1, including both document kinds and every block/metadata/listing field above, among them `origin` in the session-metadata definition and `id` on the entry definition (Q88). Later PRs add producers and projections against that initial schema; they need no premature Go types. Keep an exact initial-schema test fixture and validate later listing/exec outputs against it (Decision 25, superseding Decision 18).
- Known block branches have distinct `type` constants. An extension branch accepts only type strings outside that known set, so unknown future types remain valid without letting malformed known blocks bypass validation. Consumers skip unsupported block types as well as unknown fields; the current projection still maps unknown parsed source kinds to `other`. Removing a field or changing its meaning raises the version. Unknown top-level document kinds remain invalid.
- The test-only closed copy closes only known apsis-owned structures; arbitrary payloads and the unknown-type extension branch remain open. The local-only validator is a reversible technical admission, not a human U3 answer.
- Include `execution_action` in the initial warning definition; tests project each source unit to its matching key and validate both Kiro IDE warning paths. This corrects an unpublished planning omission, without changing existing stderr diagnostic labels or numeric positions.

## Delivery and ownership

The rune plan has four PR phases: shared JSON sessions/provider (tasks 1–4), listing across projects (5–8), exec streams (9–14), then unchanged-refresh caching (15–16). Requirement 5.9 remains unfinished until the fourth slice passes. Native Windows evidence gates only the fingerprint green task and cache consumers; it never blocks the first three slices. See [prerequisites.md](prerequisites.md) for the exact native checks; cross-compilation is insufficient and no CI change is implied.

Streams split transcript/projection work, sessions/CLI/web work, and the isolated fingerprint package. Dependencies serialize shared files: listing follows the session CLI, exec rendering follows listing projection, direct exec CLI follows scope wiring, and fingerprint module mutations follow the validator's go.mod/go.sum changes. README/CHANGELOG updates and Makefile checks ride with each slice's last implementation task. Documents and local implementation admissions are agent decisions; no design/tasks human approval is recorded.

## Error Handling

| Condition | Behaviour |
|---|---|
| `-F` with `-f shared-json` | `follow mode does not support -f shared-json` |
| `-F` with `-a codex-exec`, an exec file, a `--source codex-exec` ID, or `latest` resolving to codex-exec | `follow mode does not support Codex exec streams`; files are probed before following; `latest` and IDs are checked after resolution |
| `--all-projects` with `-p`; `--source` misuse; mixed empty and nonempty `--codex-exec-dir` | usage error |
| configured root missing or unreadable; unreadable subdirectory or file; exec-looking file without thread; identifier conflict; duplicate `(source, id)` in all-projects scope | `ListWarning` with its code; other results kept (5.9) |
| exec malformed or oversized line | warning on stderr (existing `Warning: line N:` format), in `warnings[]`, and in the web transcript view |
| `-f json` on an exec stream | leading BOM stripped, then the existing JSONL path emits the event array |
| empty exec stream with `-f shared-json` | document with `entries: []`; Markdown/HTML keep `Session contains no entries` |
| codex-exec identifier that no longer matches a contained file | `session not found` |

## Risks and Assumptions

- Risk: walking large configured roots every 15 s may be slow even with zero body reads | Verify: benchmark `ListAll` on a synthetic tree of 10,000 files in the first discovery task | If wrong: reuse a directory's cached entry list while its own mtime is unchanged, still statting every known file so in-place edits are detected.
- Risk: Kiro CLI may commit through WAL without changing the stat key of `data.sqlite3`, `-wal` or `-journal` | Verify: test two writes through a WAL-mode connection between refreshes | If wrong: run the discover query on every refresh and record its measured page reads against 5.9.
- Risk: a supported Windows filesystem may not provide the identity/change metadata required for zero-read invalidation | Verify: an early fingerprint task tests equal-size preserved-mtime replacement through the injected adapter and native Windows integration, before cache consumers | If wrong: block cache admission and record the storage/platform limitation for a requirements decision; do not substitute size/mtime or claim 5.9 passes.
- Risk: cold listings stay costly because Kiro IDE extraction reads whole `.chat` files | Verify: benchmark a cold multi-source listing with generated fixtures in the cache task | If wrong: stream-decode only `executionId`, `chat` length and `metadata`.
- Assumption: serde writes `web_search` items with two `id` keys and `encoding/json` keeps the last consistently across started and completed events, so lifecycle keying stays stable | Verify: fixture built from the pinned definition with both keys, in the parser task.

For both performance risks record generated source/file/chat counts, sizes, directory shape, platform/filesystem, command, cold/warm state and measurements. A 1.5 s local investigation budget may trigger a comparison with the stated fallback; it is not a portable acceptance threshold. Adopt an optimisation only when those measurements justify it, preserving zero unchanged transcript-body bytes as a hard assertion. If WAL fingerprints fail, audit metadata-only query/page reads explicitly; that fallback does not waive 5.9 or permit an unrecorded read of conversation bodies.

## Testing Strategy

Fixtures are synthetic. Exec fixtures record their provenance (pinned definition or sanitised structural observation) in `testdata/codex_exec/PROVENANCE.md`. Tests need no external CLI; implementation runs `make test` and `make lint`.

| Requirement | Tests |
|---|---|
| 1.1–1.4, 1.6 | Session documents for every source's existing fixtures and the exec fixtures, and listing documents (empty, warnings, omitted metadata), validated against the first-published complete v1 schema, current schema and a test copy closing only known apsis-owned structures (Decision 25); golden JSON per source; first-known summaries and header-only/user-only cases with unchanged legacy acceptance; unknown block types/fields and arbitrary payload keys accepted, malformed known blocks rejected |
| 1.5 | Tool output longer than 3,000 runes survives byte-for-byte |
| 1.7, 1.8 | Per source: first-known `origin` from Claude `entrypoint` and Codex `originator`, absent elsewhere; entry `id` exactly where Decision 32's table sets it, absent on merged entries, separate from tool-linking fields and shared by a Kiro IDE tool action's two entries; listing rows carry the bounded-scan origin; Markdown, HTML and raw `-f json` output unchanged |
| 2.1 | Detection table: BOM, leading blanks, malformed lines, malformed prefixes longer than 64 KiB containing all Kiro heuristic keys, a split long first exec object with those keys, missing/empty `thread_id`, other first events and all three probe outcomes; existing large Kiro recognition and other existing-format results unchanged; forced parsing of a pre-thread failure |
| 2.2–2.4, 5.6, 5.7 | Fixtures per item type; multiple turns, interleaved tool links, `item.updated`, EOF without completion, unknown event, item and usage fields; two turns reusing `item_0` render each result beside its own command; two missing-ID tool items become distinct generic events without renderer links; Markdown, standalone HTML and fragment goldens, including entries holding only new kinds |
| 2.5, 5.5, 5.8 | Temp trees: overlapping roots, root permutation, skipped names below a root and as a root, nested explicit root inside a skipped name, escaping file symlink, directory-symlink cycle, root symlink, case-alias roots where the volume is case-insensitive |
| 2.6, 2.7 | Worktree (`.git` file) versus parent repository; rollout and stream in different projects across both scopes; filename/recorded-ID mismatch; invalid first duplicate followed by a valid duplicate, asserting all-projects rows/opening/index use the same winner and keep the stream; a later duplicate in another project remains that project's legacy per-file row while suppression still follows the global winner |
| 2.8 | Property test (`rapid`): identifiers match `^[A-Za-z0-9_-]{1,256}$`, are deterministic, and are unchanged under root permutation, added copies and appends |
| 2.3 | Property test: random lifecycle sequences give one record per `(turn, id)` in first-appearance order with the last payload, incomplete exactly when uncompleted |
| 2.9 | Usage counters preserved, labelled, not summed across turns |
| 2.10, 4.3, 4.4 | Stdin has no time; listing time fields; property test that extended sort order is total, unknown times first, ties by source then identifier, and `latest` is the last element; no-flag equal-time fixtures retain the legacy winner/order |
| 3.1 | Rollout with `model_provider` and `turn_context` models in Markdown, HTML and JSON, including provider without model; shared JSON carries the turn_context model and its changes while default headers keep the legacy model value; rollout without a provider unchanged against existing golden files; Read/Edit group output unchanged; a Kiro IDE chat with `metadata.modelProvider` shows `model (provider)` in Markdown and HTML headers and carries the provider in shared JSON and listings, while one without it is unchanged |
| 4.1, 4.2, 5.8 | `t.Setenv("HOME", …)` (plus XDG on Linux) with synthetic fixtures for every source, including a Kiro CLI test database with one conversation under two keys and Kiro IDE execution details; list through the CLI and `httptest`, then open each row from an unrelated working directory; identical identifiers across sources |
| 5.1 | Existing golden and CLI tests unchanged; no-flags Claude input with first timestamp beyond 50 lines/1 MiB and a long Codex metadata line preserve listing/`latest`; with no roots configured, output and web pills are unchanged |
| 5.2, 5.3, 5.4 | Malformed middle line on all three surfaces; every follow combination including `latest`; flag and environment precedence, empty environment value, empty flag value, mixed values; raw JSON of a BOM-prefixed stream keeps `thread.started` |
| 5.9 | Early Unix/Windows fingerprint tests (adapter plus native Windows): equal-size preserved-mtime replacement/rewrite invalidates, unsupported metadata cannot yield a weak cache hit; then counting-opener/discover counters assert unchanged refresh reads 0 body bytes, normal and companion-only changes and nested repositories refresh, unaffected IDs remain stable, and web reversal does not mutate the next listing |

The schema validator, `github.com/santhosh-tekuri/jsonschema/v6`, is imported only from `_test.go` files, loads the schema from disk with network loading disabled, and stays out of the binaries (Decision 25, unanswered question U3; technical admission only).
