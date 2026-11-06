# Shared JSON Output (Apsis)

`apsis -f shared-json` projects a `transcript.ParseResult` into a versioned document described by `docs/schemas/apsis-shared-json.v1.schema.json`. Spec: `specs/apsis-shared-json/`.

## How It Works

- `internal/sharedjson` holds the document types, `Session` (the projection) and `Write` (two-space indent, no HTML escaping, trailing newline). It imports `transcript` and `sessions`; neither imports it.
- The projection reads parsed data only and never calls renderer helpers, so tool content keeps its full length.
- `cmd/apsis` `convert` enables `ParseOptions.AllowEmptyCodex` and `PreserveModelChanges` only for `shared-json`, and dispatches shared JSON before the `Session contains no entries` early return. Explicit `-a` still selects the parser first.
- Session metadata comes from parsed values first; resolver metadata fills only gaps. `created_at` order (Decision 30): parsed `StartedAt`, then the Kiro CLI `DatabaseCreatedAt`, then a resolver `CreatedAt` not flagged `TimeFromFilesystem`, then the first entry timestamp. Times that do not parse as RFC3339 are omitted; resolver times are written in UTC.
- `session.source` comes from the input context, then the resolver, then `ParseResult.Format`; the Claude fallback (`FormatUnknown`) has none. `session.id` is the resolver ID or the `.jsonl` filename stem; stdin has none.
- `Listing` (`internal/sharedjson/listing.go`) projects `[]sessions.SessionInfo` in the order given; `ListingRow` embeds `Metadata`, adding `size_bytes` and `file_modified_at`. A row carries `created_at` for a recorded time or `file_modified_at` when `TimeFromFilesystem` is set, both in UTC. `Scope` is `{project}` or `{all_projects: true}`. `apsis --list -f shared-json` writes it with extended ordering.
- Warning positions map by `ParseWarning.Unit`: empty is `line`, `transcript.WarningUnitChatMessage` is `chat_message`, `WarningUnitExecutionAction` is `execution_action`.
- Content kinds other than text, thinking, tool_use and tool_result become `other` blocks with `source_type`.

## Schema and Tests

- The schema is open: no `additionalProperties: false`. Known block types are disjoint `oneOf` branches; `extensionBlock` accepts only types outside that list, so a malformed known block cannot slip through as an unknown one. Warning codes are listed as `examples`, not an `enum`, so later codes stay valid (Q101).
- Tests validate against three compiled schemas: the published file, `internal/sharedjson/testdata/apsis-shared-json.v1.initial.schema.json` (a byte copy of the first published schema; never edit it), and a closed copy built in the test with `unevaluatedProperties: false` on apsis-owned definitions. `unevaluatedProperties` is required because listing rows and blocks reuse definitions through `$ref` (Q98).
- The validator (`github.com/santhosh-tekuri/jsonschema/v6`) is imported only from `_test.go` files. Each schema compiles in its own compiler because the copies share an `$id`; `UseLoader(jsonschema.SchemeURLLoader{})` stops any file or network loading.
- Goldens live in `internal/sharedjson/testdata/golden`; regenerate with `go test ./internal/sharedjson -run TestSession_Goldens -update` and review the diff. `testdata/handwritten` holds documents for producers that did not exist when the schema was published (exec streams, listings); they must keep validating against the initial schema.
- Exec streams (PR 3) added the remaining Go types: `Metadata.ThreadID`, `Entry.Turn`, an embedded `BlockCommon` (`item_id`, `status`, `incomplete`) on every block, `ToolResultBlock.ExitCode` (`*int`, so 0 is emitted), and `PlanBlock`, `ErrorBlock`, `EventBlock` and `UsageBlock`. `Counters` marshals usage counters as one object in source key order with raw values, so a counter beyond float64 precision is written unchanged. `ErrorBlock` emits `message` only; the parsed `Payload` is not projected (Q112).
- `SessionContext.Project` carries an exec stream file's containing repository, which the CLI derives (Q110); the precedence is parsed project, then context project, then resolver project.
- `testdata/golden/codex-exec.json` is the exec golden, parsed with `transcript.ParseCodexExec` from `internal/transcript/testdata/codex_exec/all_items.jsonl`.
