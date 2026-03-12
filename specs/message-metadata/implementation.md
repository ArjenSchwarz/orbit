# Implementation Explanation: Message Metadata (T-403)

## Beginner Level

### What Changed
Apsis converts AI coding agent transcripts into readable documents (Markdown and HTML). Before this change, you could see what was said in a conversation but not *when* each message was sent or *which AI model* generated each response. Now, every message header shows this metadata inline — like seeing timestamps and sender info in a chat app.

### Why It Matters
When reviewing AI coding sessions, timing matters. You might want to know how long the AI took to respond, or whether a particular model was used for a specific task. This feature makes that information visible without requiring users to dig into raw transcript files.

### Key Concepts
- **Transcript**: A log of the conversation between a user and an AI coding agent (like Claude, Codex, Copilot, or Kiro)
- **Entry**: A single message in the transcript — either from the user or the AI assistant
- **Parser**: Code that reads agent-specific file formats and converts them into a common structure
- **Renderer**: Code that takes the common structure and produces readable output (Markdown or HTML)
- **RFC3339**: A standard timestamp format like `2026-03-12T14:32:05+11:00` — unambiguous and machine-readable

---

## Intermediate Level

### Changes Overview
The implementation spans three layers across 15+ files in `internal/transcript/`:

1. **Data model** (`types.go`, `grouping.go`): Added `Model` field to `Entry` struct and `Timestamp` to grouping structs (`readItem`, `editItem`, `renderGroup`)
2. **Parsers** (5 agent formats): Extended Codex, Copilot, Kiro CLI, and Kiro IDE parsers to extract timestamps and model identifiers. Claude already had timestamps; no agent changes needed there.
3. **Renderers** (`markdown.go`, `html.go`): Updated all message header locations to include metadata via shared formatting helpers
4. **Helpers** (`metadata.go`): Centralised timestamp parsing, formatting, and metadata assembly for both output formats
5. **Styling** (`transcript.css`): Added `.message-meta` and `.meta-separator` CSS classes

### Implementation Approach
The design follows a pipeline pattern: parsers populate `Entry.Timestamp` and `Entry.Model` during parsing, then renderers call `FormatMessageMetaMarkdown()` or `FormatMessageMetaHTML()` to produce the display string. This keeps parsers and renderers decoupled — each parser only needs to set fields, and the formatting logic lives in one place.

For HTML, timestamps use `<time datetime="...">` elements with UTC ISO 8601 in the `datetime` attribute and a server-rendered RFC3339 fallback as text content. Standalone HTML documents embed a small inline script that uses `Intl.DateTimeFormat` for browser-locale formatting. The web interface (`apsis serve`) already had this JavaScript in its layout template.

Grouped Read/Edit blocks propagate the first entry's timestamp to the group header via the `renderGroup.Timestamp` field, set during preprocessing.

### Trade-offs
- **RFC3339 in Markdown vs human-friendly format**: RFC3339 was chosen because Go lacks locale-aware formatting, and a fixed English format like "12 Mar, 2:32 PM" would create a false sense of localisation. HTML gets true locale formatting via JavaScript.
- **Model on assistant messages only**: Model info is only meaningful for AI responses. Showing it on user messages would be noise.
- **Kiro IDE first-message-only timestamp**: Session-level timestamps applied to every message would be misleading. Showing it once provides context without implying per-message granularity.

---

## Expert Level

### Technical Deep Dive
The `Entry.Model` field uses `json:"-"` to prevent serialisation — it's a parser-derived field that doesn't exist in any agent's JSONL schema. The existing `entryAlias` in `UnmarshalJSON` skips `-` tags, so no unmarshalling changes were needed.

Timestamp parsing uses a shared `parseTimestamp()` helper that tries `RFC3339Nano` first, then `RFC3339`. This handles Claude's nanosecond timestamps and other agents' second-precision timestamps uniformly. A companion `formatUnixMilliTimestamp()` helper handles Kiro's epoch-millisecond timestamps, returning empty string for zero/negative values.

The HTML metadata helper (`writeMessageMetaHTML`) was extracted to eliminate a 5-line pattern that appeared 5 times across `html.go`. Similarly, `extractKiroIDESessionMetadata()` was extracted from two identical blocks in the Kiro IDE parser.

For Copilot, model extraction tracks `session.model_change` events with a `currentModel` variable that applies to all subsequent assistant entries. This correctly handles mid-session model changes, though the current implementation treats it as session-level (the model at the time of each assistant turn).

### Architecture Impact
The change is additive — no existing interfaces or output formats were altered. The `Entry` struct gained one field, grouping structs gained timestamp fields, and renderers gained metadata suffixes. All changes are backwards-compatible: entries without metadata render headers exactly as before.

The formatting helpers are exported (`FormatTimestampMarkdown`, etc.) which means external consumers of the `transcript` package can use them. This is intentional — the web interface templates may need them in the future.

### Potential Issues
- **Timezone sensitivity in tests**: `TestMain` sets `time.Local` to a fixed UTC+0 zone. This affects all tests in the `transcript` package. If future tests need a different timezone, they'll need per-test overrides.
- **Follow mode**: Only works with Claude JSONL (line-by-line streaming). Claude entries already have timestamps, so metadata appears. Other formats aren't supported in follow mode, so their parser changes have no effect there.
- **Standalone HTML script**: The inline `<script>` for locale formatting runs once on load. If content is dynamically added (not currently the case for standalone HTML), timestamps won't be formatted. The web interface handles this via HTMX swap events.

## Completeness Assessment

### Fully Implemented
- All 18 tasks completed and marked done
- All 4 requirement sections (Timestamps, Model Display, Presentation, Backwards Compatibility) satisfied
- All 5 agent formats (Claude, Codex, Copilot, Kiro CLI, Kiro IDE) supported
- Both output formats (Markdown, HTML) updated
- Integration tests covering all agent formats end-to-end
- CSS styling for HTML metadata display
- Standalone HTML JavaScript for locale formatting

### Partially Implemented
- None identified

### Missing
- None identified — all requirements from the spec have corresponding implementation and test coverage
