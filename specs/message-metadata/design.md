# Design: Message Metadata (T-403)

## Overview

This feature adds per-message timestamps and model identifiers to Apsis transcript rendering. The data flows through three layers: parsers extract metadata from agent-specific formats into the unified `Entry` struct, then renderers display it inline with message headers in both Markdown and HTML output.

The design minimises changes by leveraging existing infrastructure: the `Entry.Timestamp` field already exists (just needs population in more parsers), the web interface already has JavaScript for formatting `<time datetime>` elements, and the rendering functions already have a consistent header pattern that can be extended.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Agent Transcript Files                   │
│  Claude JSONL │ Codex JSONL │ Copilot JSONL │ Kiro JSON/IDE │
└──────┬────────┴──────┬──────┴───────┬───────┴───────┬───────┘
       │               │              │               │
       ▼               ▼              ▼               ▼
┌─────────────┐ ┌────────────┐ ┌────────────┐ ┌──────────────┐
│ Claude      │ │ Codex      │ │ Copilot    │ │ Kiro CLI/IDE │
│ Parser      │ │ Parser     │ │ Parser     │ │ Parsers      │
│             │ │ +Model     │ │ +Timestamp │ │ +Timestamp   │
│ (Timestamp  │ │  extraction│ │ +Model     │ │ +Model       │
│  already    │ │            │ │  extraction│ │  extraction  │
│  populated) │ │            │ │            │ │              │
└──────┬──────┘ └──────┬─────┘ └─────┬──────┘ └──────┬───────┘
       │               │             │               │
       ▼               ▼             ▼               ▼
┌─────────────────────────────────────────────────────────────┐
│                    Entry (unified struct)                     │
│  Timestamp: string (RFC3339)                                 │
│  Model: string (NEW)                                         │
│  ... existing fields unchanged ...                           │
└──────────────────────────┬──────────────────────────────────┘
                           │
              ┌────────────┴────────────┐
              ▼                         ▼
┌──────────────────────┐  ┌──────────────────────┐
│  Markdown Renderer   │  │  HTML Renderer       │
│  RFC3339 format      │  │  <time datetime>     │
│  Local timezone      │  │  + JS locale fmt     │
│                      │  │  (already exists in  │
│                      │  │   web layout.html)   │
└──────────────────────┘  └──────────────────────┘
```

## Components and Interfaces

### 1. Entry Struct Extension

Add a `Model` field to the unified `Entry` struct. The `Timestamp` field already exists.

**File:** `internal/transcript/types.go`

```go
type Entry struct {
    // ... existing fields ...
    Timestamp string `json:"timestamp,omitempty"`
    Model     string `json:"-"` // Not serialized; populated by parsers
    // ... existing fields ...
}
```

The `Model` field uses `json:"-"` because it is not part of any agent's JSONL schema — it's a parser-derived field. The `entryAlias` in `UnmarshalJSON` does not need updating since `-` tags are skipped.

### 2. Metadata Rendering Helpers

New file: `internal/transcript/metadata.go`

Contains formatting functions used by both Markdown and HTML renderers.

```go
// FormatTimestampMarkdown parses an RFC3339 timestamp and re-formats it
// in the system's local timezone as RFC3339.
// Returns empty string if the timestamp is empty or unparseable.
func FormatTimestampMarkdown(ts string) string

// FormatTimestampHTML returns a <time> element with ISO datetime attribute
// and a server-rendered RFC3339 fallback.
// Returns empty string if the timestamp is empty or unparseable.
func FormatTimestampHTML(ts string) string

// FormatMessageMetaMarkdown builds the metadata suffix for a Markdown header.
// Example output: " · 2026-03-12T14:32:05+11:00" or " · 2026-03-12T14:32:05+11:00 · claude-opus"
// Returns empty string if no metadata is available.
func FormatMessageMetaMarkdown(timestamp, model string) string

// FormatMessageMetaHTML builds the metadata span for an HTML header.
// Returns empty string if no metadata is available.
func FormatMessageMetaHTML(timestamp, model string) string
```

### 3. Parser Changes

#### 3.1 Claude Code Parser (`parser.go`)

No changes needed. `Entry.Timestamp` is already populated from the JSON `"timestamp"` field during unmarshalling. Claude transcripts do not contain model information.

#### 3.2 Codex Parser (`codex_parser.go`, `codex_types.go`)

**`codex_types.go`:** Add `Model` field to `CodexSessionMeta`:
```go
type CodexSessionMeta struct {
    ID        string `json:"id"`
    Timestamp string `json:"timestamp"`
    Cwd       string `json:"cwd"`
    Model     string `json:"model"` // NEW
}
```

**`codex_parser.go`:** Store model in parser state, set on assistant entries only:
- Add `model string` field to `codexParser` struct
- In `processSessionMeta`: store `meta.Model` in parser state
- In `ensureAssistantEntry`: set `entry.Model` from parser state (assistant entries only — per requirement 2.1, model is not shown on user messages)

`Entry.Timestamp` is already populated by the Codex parser.

#### 3.3 Copilot Parser (`copilot_parser.go`, `copilot_types.go`)

Currently does NOT populate `Entry.Timestamp` or extract model info. Changes:

**`copilot_types.go`:** Add `Model` field to `CopilotData`:
```go
type CopilotData struct {
    // ... existing fields ...
    Model string `json:"model,omitempty"` // NEW: from session.model_change events
}
```

**`copilot_parser.go`:** In `convertCopilotToEntries`:
- Add `currentModel string` and `currentTurnTimestamp string` local variables
- Add `session.model_change` case in the second pass: set `currentModel = event.Data.Model`
- For `user.message` entries: set `Entry.Timestamp` from `event.Timestamp`
- For `assistant.turn_start`: capture `event.Timestamp` into `currentTurnTimestamp`
- For `assistant.turn_end` (where Entry is emitted): set `Entry.Timestamp` from `currentTurnTimestamp` and `Entry.Model` from `currentModel`

The `session.model_change` event applies the model to all subsequent assistant entries from that point. If the model changes mid-session, earlier entries retain whatever model was active at the time.

#### 3.4 Kiro CLI Parser (`kiro_parser.go`)

Currently does NOT populate `Entry.Timestamp` or `Entry.Model`. Changes:

- In `convertKiroUserMessage`: accept `*string` timestamp parameter, set `Entry.Timestamp` on all returned entries (both prompt and tool-result entries get the same user message timestamp). The timestamp format in Kiro CLI user messages is expected to be RFC3339; if it isn't parseable, the formatting helpers will silently produce empty strings per the error handling policy.
- In `convertKiroAssistantMessage`: accept `*KiroRequestMetadata` parameter, set:
  - `Entry.Timestamp` from `RequestStartTimestampMs` (convert ms epoch to RFC3339 string)
  - `Entry.Model` from `ModelID`
- In `convertKiroToEntries`: pass `historyEntry.RequestMetadata` to assistant conversion

#### 3.5 Kiro IDE Parser (`kiro_ide_parser.go`)

Currently does NOT populate `Entry.Timestamp` or `Entry.Model`. Changes:

- In `convertKiroIDEToEntries`: after building entries, set:
  - First entry's `Timestamp` from `KiroIDEMetadata.StartTime` (convert ms epoch to RFC3339 string)
  - All assistant entries' `Model` from `KiroIDEMetadata.ModelID`
- In `convertKiroIDEActionsToEntries`: same logic applied to action-based entries

### 4. Markdown Renderer Changes (`markdown.go`)

Update all header-writing locations to include metadata:

| Function | Current Header | New Header |
|----------|---------------|------------|
| `formatUserMessage` | `## 👤 User` | `## 👤 User · 2026-03-12T14:32:05+11:00` |
| `formatAssistantMessage` | `## 🤖 Assistant` | `## 🤖 Assistant · 2026-03-12T14:32:05+11:00 · claude-opus` |
| `formatReadGroup` | `## 🤖 Assistant` | `## 🤖 Assistant · 2026-03-12T14:32:05+11:00` |
| `formatEditGroup` | `## 🤖 Assistant` | `## 🤖 Assistant · 2026-03-12T14:32:05+11:00` |
| `formatSlashCommand` | `## 👤 User` | `## 👤 User · 2026-03-12T14:32:05+11:00` |

**Implementation:**
- `formatUserMessage`, `formatSlashCommand`: use `FormatMessageMetaMarkdown(entry.Timestamp, "")` — user messages show timestamp only, never model (per requirement 2.1)
- `formatAssistantMessage`: use `FormatMessageMetaMarkdown(entry.Timestamp, entry.Model)` — shows both timestamp and model when available
- `formatReadGroup`, `formatEditGroup`: accept a `timestamp string` parameter (from `renderGroup.Timestamp`), show timestamp only — grouped tool operations don't display model
- `formatToolResult` (standalone `## 🤖 Assistant` headers for subagent Tasks and deferred tool results): no metadata added. These are tool execution results, not conversational turns, and the function has no access to the parent Entry's metadata. This is acceptable as these headers are subordinate to the main assistant message.

### 5. HTML Renderer Changes (`html.go`)

Update all `<div class="message-header">` blocks to include a metadata span:

```html
<div class="message-header">
    <span class="icon">🤖</span>
    <span>Assistant</span>
    <span class="message-meta">
        <time datetime="2026-03-12T14:32:05Z">2026-03-12T14:32:05Z</time>
        <span class="meta-separator">·</span>
        <span>claude-opus</span>
    </span>
</div>
```

**Implementation mirrors Markdown changes** — same locations, using `FormatMessageMetaHTML`:
- User messages and slash commands: timestamp only (empty model)
- Assistant messages: timestamp and model
- Read/Edit groups: timestamp only (from `renderGroup.Timestamp`)
- Standalone tool-result assistant sections (`formatToolResultHTML` for subagent Tasks and deferred tools): no metadata added — same rationale as Markdown

### 6. Grouped Entry Timestamp Propagation

The grouping structs `readItem`, `editItem`, and `renderGroup` carry timestamps so that group headers can display the first entry's timestamp.

**`grouping.go`:** Add `Timestamp` field to `readItem`, `editItem`, and `renderGroup`:
```go
type renderGroup struct {
    Type      string
    Entries   []Entry
    Reads     []readItem
    Edits     []editItem
    Timestamp string // NEW: first entry's timestamp for group header
}

type readItem struct {
    FilePath  string
    Content   string
    IsError   bool
    ToolID    string
    Timestamp string // NEW: from source entry
}

type editItem struct {
    FilePath  string
    Patch     []PatchHunk
    Content   string
    IsError   bool
    ToolID    string
    Timestamp string // NEW: from source entry
}
```

**`extractReadItems` and `extractEditItems`:** These functions receive the full `*Entry`, so they already have access to `entry.Timestamp`. Set `item.Timestamp = entry.Timestamp` on each extracted item.

**`preprocessEntries`:** When flushing read/edit groups in the `flushReadGroup` / `flushEditGroup` closures, set `renderGroup.Timestamp = currentReadGroup[0].Timestamp` (or `currentEditGroup[0].Timestamp`). This avoids extracting the timestamp at render time and simplifies callers.

**`formatReadGroup` and `formatEditGroup`:** Accept a `timestamp string` parameter (passed from `renderGroup.Timestamp` by the caller). Grouped blocks show timestamp only, not model — tool groups are mechanical operations, not conversational turns where model identity is relevant.

### 7. CSS Changes (`transcript.css`)

Add styling for the metadata span in HTML output:

```css
.message-meta {
    margin-left: auto;
    font-size: 0.8rem;
    font-weight: 400;
    color: var(--text-secondary);
    display: flex;
    align-items: center;
    gap: 0.4rem;
}

.meta-separator {
    color: var(--text-secondary);
    opacity: 0.5;
}
```

The `margin-left: auto` pushes metadata to the right side of the flex header, keeping the icon and role label on the left. This works because `.message-header` is already a flex container (`display: flex; align-items: center; gap: 0.5rem;` in the existing CSS).

### 8. Web Interface and Standalone HTML

**Web interface (`apsis serve`):** No changes needed to templates or JavaScript. The existing `formatLocalDates()` function in `layout.html` already:
1. Finds all `<time datetime>` elements
2. Formats them using `Intl.DateTimeFormat` with the browser's locale
3. Runs on page load and after HTMX swaps (for live following)

The transcript CSS is already embedded via `TranscriptCSS()` and will pick up the new `.message-meta` styles.

**Standalone HTML (`RenderHTML`):** The standalone HTML document does not include the web interface's layout template. To satisfy requirement 1.3 (JavaScript locale formatting), `RenderHTML` must embed a small inline script at the end of `<body>` that runs the same `formatLocalDates` logic:

```html
<script>
(function() {
    var fmt = new Intl.DateTimeFormat(undefined, {
        day: 'numeric', month: 'short', year: 'numeric',
        hour: 'numeric', minute: '2-digit'
    });
    document.querySelectorAll('time[datetime]').forEach(function(el) {
        var d = new Date(el.getAttribute('datetime'));
        if (!isNaN(d)) el.textContent = fmt.format(d);
    });
})();
</script>
```

This is a self-contained IIFE that runs once on load. The `<time>` elements contain server-rendered fallback text, so the timestamps are visible even if JavaScript is disabled.

### 9. Follow Mode

No special changes needed. Follow mode uses `RenderEntries` which calls the same `formatUserMessage` / `formatAssistantMessage` functions. Since `Entry` structs carry their own timestamps, incremental rendering works naturally.

**Limitation:** Follow mode only works with Claude JSONL transcripts (line-by-line streaming). Claude entries already have timestamps populated via JSON unmarshalling, so timestamps will appear in follow mode. Model information will not appear since Claude transcripts don't contain it. Other agent formats (Codex, Copilot, Kiro) are not supported in follow mode, so their parser changes have no effect there.

## Data Models

### Entry (extended)

```go
type Entry struct {
    Type            string         `json:"type"`
    Message         *Message       `json:"message,omitempty"`
    Timestamp       string         `json:"timestamp,omitempty"` // Existing
    Model           string         `json:"-"`                   // NEW: parser-derived
    SessionID       string         `json:"sessionId,omitempty"`
    Cwd             string         `json:"cwd,omitempty"`
    IsMeta          bool           `json:"isMeta,omitempty"`
    UUID            string         `json:"uuid,omitempty"`
    ParentUUID      string         `json:"parentUuid,omitempty"`
    SourceToolUseID string         `json:"sourceToolUseID,omitempty"`
    ToolUseResult   *ToolUseResult `json:"toolUseResult,omitempty"`
}
```

### CodexSessionMeta (extended)

```go
type CodexSessionMeta struct {
    ID        string `json:"id"`
    Timestamp string `json:"timestamp"`
    Cwd       string `json:"cwd"`
    Model     string `json:"model"` // NEW
}
```

### readItem / editItem (extended)

```go
type readItem struct {
    FilePath  string
    Content   string
    IsError   bool
    ToolID    string
    Timestamp string // NEW
}

type editItem struct {
    FilePath  string
    Patch     []PatchHunk
    Content   string
    IsError   bool
    ToolID    string
    Timestamp string // NEW
}
```

## Error Handling

### Unparseable Timestamps

If a timestamp string cannot be parsed as RFC3339 (or RFC3339Nano), the formatting helpers return an empty string, and the metadata is silently omitted. This aligns with Decision 2 (omit unavailable fields silently).

### Missing Metadata

All metadata fields are optional. The rendering helpers check for empty strings before emitting any output. An entry with no timestamp and no model produces no metadata suffix — the header renders exactly as before.

### Epoch Millisecond Conversion (Kiro)

Kiro CLI's `RequestStartTimestampMs` and Kiro IDE's `StartTime` are `int64` epoch milliseconds. These are converted to RFC3339 UTC strings during parsing:
```go
time.UnixMilli(ms).UTC().Format(time.RFC3339)
```

A zero or negative value is treated as "no timestamp" and produces an empty string.

## Testing Strategy

### Unit Tests — Metadata Helpers (`metadata_test.go`)

Table-driven tests for each formatting function:

| Test | Inputs | Expected |
|------|--------|----------|
| `FormatTimestampMarkdown` valid | RFC3339 string | RFC3339 in local TZ |
| `FormatTimestampMarkdown` empty | `""` | `""` |
| `FormatTimestampMarkdown` invalid | `"not-a-date"` | `""` |
| `FormatTimestampHTML` valid | RFC3339 string | `<time datetime="...">fallback</time>` |
| `FormatTimestampHTML` empty | `""` | `""` |
| `FormatMessageMetaMarkdown` both | ts + model | `" · 2026-03-12T14:32:05Z · claude-opus"` |
| `FormatMessageMetaMarkdown` ts only | ts + `""` | `" · 2026-03-12T14:32:05Z"` |
| `FormatMessageMetaMarkdown` model only | `""` + model | `" · claude-opus"` |
| `FormatMessageMetaMarkdown` neither | `""` + `""` | `""` |

Note: Timestamp formatting tests should use a fixed timezone via `time.Local = time.FixedZone("TEST", 0)` in a `TestMain` to ensure deterministic output. Before adding a `TestMain`, verify that no existing tests in the `transcript` package depend on the default local timezone. If conflicts exist, use per-test timezone overrides instead.

### Parser Tests

Each parser's existing test file gets additional assertions verifying that `Entry.Timestamp` and `Entry.Model` are populated correctly from test fixtures:

- **Codex**: Verify `Model` is extracted from `session_meta` payload
- **Copilot**: Verify `Timestamp` is set on entries, `Model` is extracted from `session.model_change`. The existing Copilot test fixture needs to be extended with a `session.model_change` event
- **Kiro CLI**: Verify `Timestamp` from both user message and request metadata, `Model` from `ModelID`
- **Kiro IDE**: Verify first entry has `Timestamp`, all assistant entries have `Model`

### Rendering Tests

Golden file tests (or assertion-based) for both Markdown and HTML output:

- Entry with timestamp only → header includes timestamp
- Entry with timestamp and model → header includes both
- Entry with no metadata → header unchanged from current output
- Read group with timestamps → first entry's timestamp on group header
- Edit group with timestamps → first entry's timestamp on group header

### Integration Test

End-to-end test using a sample transcript file for each agent format, parsing through `Parse()` and rendering through `RenderMarkdown()` / `RenderHTML()`, verifying metadata appears in output.

### HTML `<time>` Element Test

Verify that HTML output contains `<time datetime="...">` elements with valid ISO 8601 `datetime` attributes and fallback text content. JavaScript formatting is not tested server-side (that's browser behaviour).
