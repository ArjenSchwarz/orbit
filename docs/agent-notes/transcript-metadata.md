# Transcript Metadata Flow (Apsis)

## Parser to Renderer Data Path

- All formats are normalized to `internal/transcript.Entry` before rendering.
- `Entry` carries `Timestamp` and parser-populated `Model`/`Provider`; both metadata fields have `json:"-"` and are not part of the source wire format.
- `ParseResult.Format` identifies every source parser. Auto-detection's Claude fallback sets `FormatUnknown` so a projection does not claim a detected source.
- `ParseResult.Metadata` stores `StartedAt`, `Project`, `FirstModel`, `FirstProvider` and `Origin` independently of rendered entries. It may be non-nil with a nil `TotalCost`; cost consumers must guard that pointer.
- `Entry.RecordID` and `Entry.Origin` (both `json:"-"`) exist for shared JSON only; no renderer reads them, and raw `-f json` passes source lines through without re-serialising `Entry`.
- Markdown and HTML renderers consume only normalized `Entry` values; they do not read format-specific structs directly.

## Current Metadata Coverage

- Claude parser preserves `Entry.Timestamp` from JSONL, records the first parseable decoded timestamp and first `cwd` before filtering, and leaves model extraction unchanged.
- Codex assistant entries receive the active `model_provider`. By default `Entry.Model` keeps the legacy `session_meta.payload.model` value; with `ParseOptions.PreserveModelChanges` it is the latest nonempty `turn_context.payload.model`, else the `session_meta` model. `FirstModel` is the first nonempty model from either record in transcript order. Summaries survive header-only/user-only input; project is the first header `cwd`, and start time is the first outer header timestamp that parses as RFC3339 (Q81).
- Gotcha: Codex 0.166.0 never writes `session_meta.payload.model`. Pinned `protocol.rs` `SessionMeta` has `model_provider` but no `model`; the model lives in `turn_context.payload.model` (`TurnContextItem`). Default rendering ignores it, so Markdown/HTML headers show the provider without a model (U4, Q95). An undecodable `turn_context` payload is skipped without a warning, as every `turn_context` line was before. `testdata/codex/turn_context.jsonl` is the synthetic current-shape fixture; `PROVENANCE.md` there cites the pinned definitions.
- Copilot conversion preserves user-message timestamps and assistant-turn start timestamps, and tracks `session.model_change` for assistant models. `StartedAt` is `session.start` `data.startTime` when it parses, else the first parseable event timestamp, never derived from entries (Decision 30). The first recognized model-change record supplies the model summary even without assistant content. Generated tool-result entries carry neither timestamp nor model.
- Kiro CLI conversion preserves optional user timestamps. Assistant entries receive timestamp/model from request metadata (`RequestStartTimestampMs`, `ModelID`); the summary retains the first known request model. The parser sets no `StartedAt`: the resolver's database `created_at` supplies the creation time, and the projection falls back to the first entry timestamp (Decision 30).
- Kiro IDE conversion sets `metadata.startTime` on the first entry only and preserves `metadata.modelId` on assistant entries, including execution-detail actions. Entries that take the model also take `metadata.modelProvider` as `Entry.Provider` (Q93), so their headers show `model (provider)`; user and tool-result entries take neither. The start time, model and provider also populate parse metadata without cost data or assistant content. Listing rows do not read the Kiro IDE model or provider yet; task 5 adds that to the existing whole-file listing read.

## Rendering Paths

- Standalone HTML output uses `RenderHTML`.
- Web UI transcript output uses `RenderHTMLFragment`, embedded in apsisweb templates.
- Both HTML paths share entry rendering via `renderEntriesToBuilder`.
- Web UI layout already localizes `<time datetime>` via client-side JavaScript.
- Follow mode renders Markdown via `RenderMarkdown`/`RenderEntries` and bypasses format-specific parser conversion (it unmarshals JSONL lines directly into `Entry`).
- `MaxToolInputRunes`, `MaxToolResultRunes` and `truncateString` remain defined, but production renderers do not call the truncation helper. The constants do not describe an active output limit.

## Grouped/Composite Sections

- Consecutive Read/Edit tool calls are grouped into consolidated blocks (`read_group`, `edit_group`) that render as assistant sections not tied to one original message header.
- Tool results may also render in standalone assistant sections (e.g., subagent or deferred tool result rendering).
- Any per-message metadata requirements need explicit behavior for grouped/deferred-render sections.

## Metadata Implementation Gotchas

- `ParseWarning.Line` is not always a physical line. Kiro IDE warnings carry `Unit: "chat-message"` or `Unit: "execution-action"` for their one-based indexes; physical-line parsers leave `Unit` empty. Existing CLI diagnostics still label both indexes as `line`.
- Every warning site sets `Code` from the `Warning*` constants in `parser.go` (Q80): JSON decode failures `malformed_line`, the Claude scanner limit `oversized_line`, missing fields or undecodable payloads `invalid_record`, unknown types/roles/actions `unrecognized_record`, and Codex outputs without a call `unmatched_tool_result`. Codex record errors carry their code through `recordError`; the warning `Message` text is unchanged.
- Codex listing creation time comes from the outer `CodexEntry.Timestamp` on `session_meta`. `CodexSessionMeta` also has a payload timestamp field, but using it as a new fallback would change the established provenance.
- Codex assigns model/provider when creating assistant entries. Identical headers preserve consolidation. Provider transitions create a new entry at the next model-produced item (message, reasoning, function call, agent message or agent reasoning); model-only transitions do so only with `ParseOptions.PreserveModelChanges`, intended for shared JSON. A `function_call_output` always joins the current entry (`joinAssistantEntry`), so a tool result stays with its call (Q82). Default provider-less rendering keeps legacy consolidation and timestamps. `AllowEmptyCodex` separately enables metadata-only results; the default keeps the legacy no-valid-entries error. Both flags pass through `Parse`, `ParseJSONLWithFormat` and `ParseCodexJSONL`; later CLI/helper refactors must forward them.
- `modelLabel` renders `model (provider)` or `(provider)` only in existing assistant model headers. Read/Edit groups, user headers and deferred tool-result sections keep their existing metadata behaviour; they do not gain provider labels.
- Copilot assistant entries are finalized on `assistant.turn_end`: the timestamp is captured at turn start, while the model uses the current model at finalization. Changes within a turn therefore need care.
- `BuildToolMeta`, the HTML tool metadata map and `preprocessEntries` match tool calls/results by ID across the whole transcript. A new parser for a source that reuses item IDs across turns must qualify its internal tool IDs and retain the original source ID separately.

## Origin and Record Identifiers

- `Metadata.Origin` is the first nonempty Claude `entrypoint` (decoded by the `Entry.UnmarshalJSON` alias into `Entry.Origin`, so there is no second decode per line) or Codex `session_meta.payload.originator`. Other sources leave it empty; Copilot's `producer` is deliberately unmapped (Q96).
- `Entry.RecordID` is set only where an entry comes from exactly one source record (Decision 32): every Claude line's `uuid`; Codex user entries from the `message` item `id`; Copilot `user.message` event `id`; Kiro CLI `message_id` of a `ToolUse`, `TextResponse` or `Response`; Kiro IDE `actionId` for every action entry, shared by a tool action's tool-use and tool-result entries. Merged entries (Codex assistant, Copilot assistant/tool-result) and Kiro IDE chat-file entries have none.
- `RecordID` is never a tool-linking identifier, even where the value coincides (Kiro IDE tool `ID`/`ToolUseID` equal the `actionId`). Renderer skill and slash-command lookups keep using `UUID`/`ParentUUID`.
