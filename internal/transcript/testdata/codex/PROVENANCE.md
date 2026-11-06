# Codex rollout fixture provenance

All fixtures in this directory are synthetic. None was captured from a real session.

- `turn_context.jsonl` follows the pinned Codex
  [rust-v0.166.0 `protocol.rs`](https://raw.githubusercontent.com/openai/codex/rust-v0.166.0/codex-rs/protocol/src/protocol.rs)
  definitions: `SessionMeta` records `model_provider` and `originator` but no
  `model`, and `TurnContextItem` records the turn's `model`. Its user message
  `id` follows the optional `ResponseItem::Message` `id` in the pinned
  [`models.rs`](https://raw.githubusercontent.com/openai/codex/rust-v0.166.0/codex-rs/protocol/src/models.rs).
  Only the fields apsis reads are included.
- `basic.jsonl` and `reasoning.jsonl` predate this note and record neither a
  model nor a provider.
