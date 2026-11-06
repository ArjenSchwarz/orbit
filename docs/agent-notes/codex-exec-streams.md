# Codex Exec Streams (Apsis)

Source `codex-exec`: event streams saved from `codex exec --json`. Spec: `specs/apsis-shared-json/` (PR 3, tasks 9–14). Shapes follow the pinned Codex 0.166.0 `exec_events.rs`; fixtures in `internal/transcript/testdata/codex_exec/` are synthetic, with provenance in `PROVENANCE.md`.

## Parsing (`internal/transcript/codex_exec_parser.go`)

- `ParseCodexExec` reads physical lines with `readExecLine` (10 MiB limit; an oversized line is discarded through its newline with `oversized_line`). Only a reader error is fatal. A malformed line gives `malformed_line` with the `json.SyntaxError` offset and never the line text (Q45).
- Turns: records before the first `turn.started` are turn 0 (`Entry.Turn` 0, no `turn` in JSON). Items key by `(turn, item.id)`; an item without a valid string ID keys by its physical line, so invalid items never merge. Later lifecycle events replace the stored JSON; `item.completed` marks it done; otherwise every block from it gets `Incomplete`.
- A repeated `thread.started` with the same ID is dropped and turn numbering continues; a different ID becomes an `event` block plus `thread_mismatch` (Q44).
- Tool IDs are `t<turn>:<item id>` so resumed output that restarts `item_N` stays unambiguous (Q42). Every tool item emits a `tool_result`, because renderers draw non-Task tools only with their result.
- `web_search` items carry two `id` keys; `encoding/json` keeps the last (the raw search ID), consistently across lifecycle events.
- Usage counters keep source key order and raw JSON values (`orderedCounters`), basis `thread_total_at_turn_completion`; nothing is summed.
- Item warnings are added at translation (EOF), so warnings are stably sorted by line at the end.

## Detection

`ProbeExecStream` returns `exec` (first JSON object is `thread.started` with a nonempty string `thread_id`), `exec-without-thread` (another exec event type first) or `other`. `DetectFormat` keeps structurally validated Kiro JSON first, then runs the probe over the 64 KiB chunk plus a `TeeReader` of the rest, so it can read past the chunk and still return every consumed byte for replay (Decision 21). Only `exec` promotes; a substring-only Kiro match comes next, then the legacy JSONL detection on the chunk. A large Kiro file is read in full by the probe when no line parses as an object; the replay buffer then holds the whole file.

## Identity and project (`internal/sessions/codexexec.go`)

- `ExecIdentifier(threadID, canonicalPath)`: thread ID with bytes outside `[A-Za-z0-9_-]` mapped to `_`, cut to 191 bytes, `-`, full SHA-256 hex of the canonical path (Decision 14). `pathDigest` is a package variable so tests can force conflicts.
- `CanonicalPath`: `Abs`, `EvalSymlinks`, then on-disk spelling per component (exact entry, else a unique case-insensitive match). macOS temp volumes are case-insensitive, so the case-alias test runs there.
- `RepositoryRoot`: nearest ancestor with a `.git` directory or a `.git` file starting with `gitdir:`; no `git` executable (Q52).
- Direct CLI file conversion derives the same identifier and project after parsing (`execInput` in `cmd/apsis/main.go`); stdin gets neither, and the filename stem is never used.

## Discovery

`listCodexExec` canonicalises roots (`root_unreadable` on failure), walks each with `filepath.WalkDir` (never follows directory symlinks; skips `.git`, `node_modules`, `testdata` below a root), keeps `.jsonl` symlinks only when the target is a regular file inside a root, de-duplicates by canonical path, probes each candidate (`missing_thread` for `exec-without-thread`, silent for `other`), omits every stream of a conflicting identifier (`identifier_conflict`), drops suppressed thread IDs before project filtering, and memoises repository roots per directory within one pass. Rows carry `ThreadID` and the modification time with `TimeFromFilesystem`.

Benchmark (`BenchmarkListAllCodexExec`, 2026-11-06): 10,000 files of about 100 bytes in 100 directories plus a skipped `node_modules` subtree (200 files skipped, 1,000 non-exec JSONL, 8,800 listed), darwin/arm64, APFS, warm page cache: about 167 ms and 71 MB allocated per `ListAll("")` pass. A 64 KiB probe buffer had cost about 674 MB per pass (Q118). For task 16's cache.

## Resolution

`Resolver.locateCodexExec` (both scopes, no project filter, no suppression): split off the 64-hex digest, walk the roots by metadata only, probe only files whose path digest matches, require the whole identifier to match exactly once (Q115), recheck containment after opening. `ResolvePath` returns the stream path; follow callers reject it with `follow mode does not support Codex exec streams`. Bare CLI identifiers try `codex-exec` last; without roots it is never found.
