# Transcript Metadata Flow (Apsis)

## Parser to Renderer Data Path

- All formats are normalized to `internal/transcript.Entry` before rendering.
- `Entry` currently has `Timestamp` but no model field.
- Markdown and HTML renderers consume only normalized `Entry` values; they do not read format-specific structs directly.

## Current Metadata Coverage

- Claude parser preserves `Entry.Timestamp` from JSONL.
- Codex parser maps `CodexEntry.Timestamp` into `Entry.Timestamp`.
- Copilot parser currently drops event timestamps when converting to `Entry`.
- Kiro CLI parser currently drops both user timestamps and request metadata timestamps/model IDs during conversion.
- Kiro IDE parser currently drops chat metadata (`metadata.startTime`, `metadata.modelId`) during conversion.

## Rendering Paths

- Standalone HTML output uses `RenderHTML`.
- Web UI transcript output uses `RenderHTMLFragment`, embedded in apsisweb templates.
- Both HTML paths share entry rendering via `renderEntriesToBuilder`.
- Web UI layout already localizes `<time datetime>` via client-side JavaScript.
- Follow mode renders Markdown via `RenderMarkdown`/`RenderEntries` and bypasses format-specific parser conversion (it unmarshals JSONL lines directly into `Entry`).

## Grouped/Composite Sections

- Consecutive Read/Edit tool calls are grouped into consolidated blocks (`read_group`, `edit_group`) that render as assistant sections not tied to one original message header.
- Tool results may also render in standalone assistant sections (e.g., subagent or deferred tool result rendering).
- Any per-message metadata requirements need explicit behavior for grouped/deferred-render sections.

## Metadata Implementation Gotchas

- HTML tool-result rendering can create standalone assistant `<section>` blocks (`formatToolResultHTML`) without access to the parent `Entry`; metadata must be threaded explicitly (or carried via `toolMeta`) for these headers.
- Copilot assistant entries are finalized on `assistant.turn_end`, so timestamp/model selection needs explicit per-turn capture logic (not just "current value at end of turn").
- Grouped Read/Edit headers currently have no model source; if assistant headers should always show model when available, grouping types must propagate model similarly to timestamp.
- `session.model_change` is recognized by format detection tests but is not present in current Copilot sample fixtures; keep parser logic defensive and add fixture coverage when implementing model extraction.
