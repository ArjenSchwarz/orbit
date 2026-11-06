# Codex exec stream fixture provenance

All fixtures in this directory are synthetic. None was captured from a real
session. Shapes follow the pinned Codex
[rust-v0.166.0 `exec_events.rs`](https://raw.githubusercontent.com/openai/codex/rust-v0.166.0/codex-rs/exec/src/exec_events.rs)
definitions (`ThreadEvent`, `ThreadItem`, `ThreadItemDetails` and their
payload structs), as emitted by the pinned
[JSONL emitter](https://raw.githubusercontent.com/openai/codex/rust-v0.166.0/codex-rs/exec/src/event_processor_with_jsonl_output.rs).

| Shape | Source |
|---|---|
| `thread.started`, `turn.started`, `turn.completed` (`Usage`), `turn.failed`, top-level `error` | `ThreadEvent` variants in `exec_events.rs` |
| `item.started`, `item.updated`, `item.completed` | `ItemStartedEvent`, `ItemUpdatedEvent`, `ItemCompletedEvent` |
| `agent_message`, `reasoning` | `AgentMessageItem`, `ReasoningItem` |
| `command_execution` (`in_progress`, `completed`, `failed`, `declined`; `exit_code` may be null) | `CommandExecutionItem`, `CommandExecutionStatus` |
| `file_change` (`add`, `delete`, `update`) | `FileChangeItem`, `FileUpdateChange`, `PatchChangeKind` |
| `mcp_tool_call` with a result or an error | `McpToolCallItem`, `McpToolCallItemResult`, `McpToolCallItemError` |
| `web_search` with two `id` keys | `WebSearchItem` has its own `id` beside the flattened `ThreadItem.id`; the emitter sets the outer one to `item_N` and the inner one to the raw search ID. `action` follows `WebSearchAction` in the pinned [`models.rs`](https://raw.githubusercontent.com/openai/codex/rust-v0.166.0/codex-rs/protocol/src/models.rs) |
| `todo_list` | `TodoListItem`, `TodoItem` |
| `error` item | `ErrorItem` |
| `collab_tool_call` | `CollabToolCallItem`; apsis has no specialised translation, so it stays a generic event |

Structures the pinned definitions do not cover are labelled here rather than
claimed as observed:

- `future_counter` in `all_items.jsonl` usage and the `session.compacted`
  event in `failures.jsonl` are invented to exercise unknown fields and event
  types.
- `failures.jsonl` line 4 is deliberately truncated JSON.
- The repeated `thread.started` in `turns.jsonl` models resumed output appended
  to one file: the pinned [resume path](https://raw.githubusercontent.com/openai/codex/rust-v0.166.0/codex-rs/exec/src/lib.rs)
  reuses the thread ID and the emitter restarts `item_N` numbering per process.
- `pre_thread_failure.jsonl` models a run that fails before a thread exists.
