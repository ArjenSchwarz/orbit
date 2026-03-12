# Decision Log: Message Metadata

## Decision 1: Always Visible Metadata

**Date**: 2026-03-12
**Status**: accepted

### Context

Per-message metadata (timestamps, model) needs to be displayed in transcript output. The question is whether it should be always visible, collapsible, or toggle-controlled.

### Decision

Display metadata always visible, styled subtly inline with message headers.

### Rationale

Timestamps and model info are lightweight metadata that enhances readability. Hiding them behind clicks adds friction for the common case where users want to see timing. Subtle styling (muted colour, separator dots) keeps it unobtrusive.

### Alternatives Considered

- **Collapsible details block**: Hidden by default, click to expand — Rejected because it adds unnecessary interaction for small metadata
- **Global toggle**: Show/hide all metadata at once — Rejected because it requires UI state management and adds complexity for minimal benefit

### Consequences

**Positive:**
- Metadata is immediately visible without interaction
- Simple implementation with no state management

**Negative:**
- Slightly more visual noise in transcripts (mitigated by subtle styling)

---

## Decision 2: Omit Unavailable Fields Silently

**Date**: 2026-03-12
**Status**: accepted

### Context

Not all agent formats include all metadata fields. Claude, Codex, and Copilot transcripts do not contain model identifiers. Some entries may lack timestamps. Kiro IDE only has session-level metadata, not per-message.

### Decision

Omit any metadata field silently when the data is not available. No placeholders or "unknown" labels.

### Rationale

Showing "Unknown" for fields that structurally cannot exist in a format adds visual clutter without providing information. Users familiar with an agent format already know its limitations.

### Alternatives Considered

- **Show "Unknown" placeholder**: Makes gaps visible — Rejected because it draws attention to the absence of data that was never expected to be there
- **Show format indicator**: Display the agent format name (e.g., "Claude", "Kiro CLI") so users know what metadata to expect — Rejected because it adds noise and users already know which format they are viewing

### Consequences

**Positive:**
- Clean output with no unnecessary noise
- Consistent experience — metadata appears only when meaningful

**Negative:**
- Users cannot distinguish "data not available in format" from "data missing from this specific entry" (acceptable trade-off)

---

## Decision 3: RFC3339 in Markdown, JS Locale in HTML

**Date**: 2026-03-12
**Status**: accepted

### Context

Timestamps need a display format. Go's standard library does not support locale-aware date formatting. The feature outputs to both Markdown (CLI) and HTML (standalone and web interface).

### Decision

Use RFC3339 format in the system's local timezone for Markdown output. For HTML output, emit UTC timestamps in `<time datetime="...">` elements with an RFC3339 fallback, and use JavaScript to format in the browser's local timezone and locale.

### Rationale

RFC3339 is unambiguous, machine-parseable, and internationally understood. Using a custom fixed format like `2 Jan, 3:04 PM` would pretend to be human-friendly while still not being locale-aware. RFC3339 is honest about what it is. HTML gets true locale formatting via JavaScript.

### Alternatives Considered

- **Custom fixed format (`2 Jan, 3:04 PM`)**: More readable at a glance — Rejected because it uses English month abbreviations without locale awareness, creating a false sense of localisation
- **Third-party Go locale library**: True locale-aware formatting in Go — Rejected because it adds a dependency for marginal benefit in CLI output
- **ISO 8601 everywhere**: Same format in both Markdown and HTML — Rejected for HTML because browsers can do better with client-side locale formatting

### Consequences

**Positive:**
- Markdown timestamps are unambiguous and machine-parseable
- HTML output respects the viewer's locale and timezone via JavaScript
- No custom format constants to maintain
- Fallback in `<time>` element ensures visibility without JS

**Negative:**
- RFC3339 is less scannable than a human-friendly format in Markdown output
- CLI output uses system timezone, which may differ from the session's timezone

---

## Decision 4: No Derived Timing Calculations

**Date**: 2026-03-12
**Status**: accepted

### Context

Response time (duration between user message and assistant reply) could be calculated from timestamps. Thinking time metadata is available in some formats (Kiro CLI's `TimeToFirstChunk`).

### Decision

Display only raw timestamps. Do not calculate or display derived timing metrics like response duration.

### Rationale

Raw timestamps are the source data and are always accurate. Derived metrics introduce complexity (handling gaps, tool calls, subagents) and can be misleading. Users can mentally compare timestamps when needed.

### Alternatives Considered

- **Show response duration**: Calculate elapsed time between user and assistant — Rejected to keep the feature simple and avoid misleading metrics from complex conversation flows

### Consequences

**Positive:**
- Simpler implementation
- No risk of misleading timing data

**Negative:**
- Users must manually compare timestamps to understand response times

---

## Decision 5: Grouped Blocks Use First Entry's Timestamp

**Date**: 2026-03-12
**Status**: accepted

### Context

The rendering code groups consecutive Read and Edit tool calls into consolidated blocks with a single header. These grouped blocks could show the first entry's timestamp, the last entry's, or no timestamp.

### Decision

Display the timestamp of the first entry in the group on the group header.

### Rationale

The first timestamp represents when the group of operations began, which is the most intuitive reference point for the user. It answers "when did this batch of reads/edits start?"

### Alternatives Considered

- **No timestamp on groups**: Skip timestamps for grouped blocks — Rejected because it creates gaps in the timeline that could be confusing
- **Last entry's timestamp**: Show when the group ended — Rejected because the start time is more intuitive as a reference point

### Consequences

**Positive:**
- Preserves timeline continuity across all message types
- Intuitive "when did this start" semantics

**Negative:**
- Does not reflect the duration of the group (acceptable given Decision 4)

---

## Decision 6: Kiro IDE Session-Level Timestamp on First Message Only

**Date**: 2026-03-12
**Status**: accepted

### Context

Kiro IDE transcripts (`KiroIDEChatFile`) have only session-level timestamps (`Metadata.StartTime`, `Metadata.EndTime`). Individual `KiroIDEMessage` entries have no timestamp field. Showing the same session start time on every message would be misleading.

### Decision

Display the session start time on the first message only. Omit timestamps on all subsequent messages in Kiro IDE transcripts.

### Rationale

The session start time provides a useful reference point without creating the false impression that per-message timing is available. This is consistent with the "omit silently" principle for unavailable data.

### Alternatives Considered

- **Omit timestamps entirely**: Show no timestamps for Kiro IDE — Rejected because the session start time is still useful context
- **Show on every message**: Apply session start time to all messages — Rejected because it falsely implies per-message timing

### Consequences

**Positive:**
- Provides a useful session timing reference
- Does not mislead users about per-message timing granularity

**Negative:**
- Most messages in Kiro IDE transcripts will have no timestamp (acceptable)

---

## Decision 7: Extract Model Info From Codex and Copilot Transcripts

**Date**: 2026-03-12
**Status**: accepted

### Context

The original assumption was that only Kiro CLI and Kiro IDE transcripts contain model information. Investigation revealed that Codex and Copilot also include model data in their transcript files, but it is currently dropped during parsing:
- Codex: `session_meta` entries contain a `"model"` field (e.g., `"gpt-4"`) but `CodexSessionMeta` has no `Model` field, so it's silently discarded during unmarshalling
- Copilot: `session.model_change` events carry model names (e.g., `"claude-3-5-sonnet"`) but the parser ignores these events entirely

Only Claude Code transcripts truly lack model information.

### Decision

Extract and display model information from Codex (`session_meta` model field) and Copilot (`session.model_change` events) in addition to Kiro CLI and Kiro IDE. Both are session-level and applied to all assistant messages.

### Rationale

The data is already present in the transcript files. Not extracting it would be a missed opportunity to provide useful context. The implementation cost is minimal — adding a struct field for Codex and a case statement for Copilot.

### Alternatives Considered

- **Only extract from Kiro formats**: Simpler implementation — Rejected because the data is available and useful for Codex and Copilot users
- **Extract per-message model changes for Copilot**: Track model changes mid-session — Rejected for now as session-level is sufficient; can be refined later if needed

### Consequences

**Positive:**
- Model info displayed for 4 of 5 agent formats instead of 2
- Leverages data that already exists in transcript files

**Negative:**
- Slightly more parser changes needed (adding struct fields and extraction logic)

---
