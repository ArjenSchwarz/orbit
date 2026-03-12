# Requirements: Message Metadata (T-403)

## Introduction

Apsis renders AI coding agent transcripts into Markdown and HTML for review. Currently, per-message metadata such as timestamps and model identifiers is parsed from transcript files but not included in the rendered output. This feature adds inline metadata display to each message in both Markdown and HTML rendering, making it easier to understand when messages occurred and which model produced them.

The metadata is displayed subtly alongside each message header. Fields that are unavailable for a given agent format are omitted silently — no placeholders or "unknown" labels are shown.

---

### 1. Per-Message Timestamp Display

**User Story:** As a user reviewing a transcript, I want to see when each message was sent, so that I can understand the timing and sequence of the conversation.

**Acceptance Criteria:**

1. <a name="1.1"></a>The system SHALL display a human-friendly timestamp alongside each user and assistant message header in both Markdown and HTML output
2. <a name="1.2"></a>For Markdown output, the system SHALL display timestamps in RFC3339 format in the system's local timezone
3. <a name="1.3"></a>For HTML output (both standalone and web interface), the system SHALL emit timestamps as UTC ISO 8601 values in a `<time datetime="...">` element and use JavaScript to format them in the browser's local timezone and locale
4. <a name="1.4"></a>For HTML output, the `<time>` element SHALL contain a server-rendered fallback in RFC3339 format so the timestamp is visible even without JavaScript
5. <a name="1.5"></a>WHEN a timestamp is not available for a message, the system SHALL omit the timestamp silently without showing a placeholder
6. <a name="1.6"></a>The system SHALL extract and display timestamps from all supported agent formats WHERE the data is available:
   - Claude Code: `Entry.Timestamp` (RFC3339)
   - Codex: `CodexEntry.Timestamp`
   - Copilot: `CopilotEvent.Timestamp`
   - Kiro CLI user messages: `KiroUserMessage.Timestamp`
   - Kiro CLI assistant messages: `KiroRequestMetadata.RequestStartTimestampMs`
   - Kiro IDE: `KiroIDEMetadata.StartTime` (session-level, see [1.7](#1.7))
7. <a name="1.7"></a>For Kiro IDE transcripts, WHERE only session-level timestamps are available, the system SHALL display the session start time on the first message only and omit timestamps on subsequent messages

---

### 2. Per-Message Model Display

**User Story:** As a user reviewing a transcript, I want to see which AI model generated each response, so that I can understand model behaviour differences and identify which model was used.

**Acceptance Criteria:**

1. <a name="2.1"></a>The system SHALL display the model identifier alongside assistant message headers WHERE the model information is available
2. <a name="2.2"></a>WHEN model information is not available in the transcript data, the system SHALL omit the model field silently
3. <a name="2.3"></a>The system SHALL extract model information from agent formats WHERE it is present:
   - Kiro CLI: `KiroRequestMetadata.ModelID` (per-message, from request metadata on each history entry)
   - Kiro IDE: `KiroIDEMetadata.ModelID` (session-level, applied to all assistant messages)
   - Codex: `CodexSessionMeta` model field (session-level, applied to all assistant messages — currently not parsed, needs `Model` field added to struct)
   - Copilot: `session.model_change` event data (session-level, applied to all assistant messages from that point — currently not extracted by parser)
4. <a name="2.4"></a>The system SHALL NOT display model information for Claude Code transcripts, which do not include model identifiers in their transcript data

---

### 3. Metadata Presentation

**User Story:** As a user reviewing a transcript, I want metadata to be visible but unobtrusive, so that it enhances readability without cluttering the conversation flow.

**Acceptance Criteria:**

1. <a name="3.1"></a>The system SHALL display metadata inline with the message header (e.g., "User · 2026-03-12T14:32:05+11:00" or "Assistant · 2026-03-12T14:32:05+11:00 · claude-opus")
2. <a name="3.2"></a>The metadata SHALL be visually styled with reduced emphasis compared to message content (e.g., muted colour, smaller font size in HTML)
3. <a name="3.3"></a>In Markdown output, the system SHALL append metadata to the message header line using a ` · ` separator
4. <a name="3.4"></a>In HTML output, the system SHALL render metadata in a styled `<span>` element within the message header, using the `<time>` element for timestamps
5. <a name="3.5"></a>The metadata display format (styling, position, separators) SHALL be consistent across all agent transcript formats, even though the set of displayed fields varies by agent
6. <a name="3.6"></a>For grouped Read/Edit blocks, the system SHALL display the timestamp of the first entry in the group on the group header

---

### 4. Backwards Compatibility

**User Story:** As a developer consuming Apsis output, I want existing functionality to remain unchanged, so that my workflows are not disrupted.

**Acceptance Criteria:**

1. <a name="4.1"></a>The system SHALL NOT remove or alter any existing output fields or structure
2. <a name="4.2"></a>The system SHALL continue to display session-level metadata (cost, session ID) at the top of transcripts as before
3. <a name="4.3"></a>The system SHALL NOT remove or modify existing Entry struct fields or change existing parsing behaviour. New fields MAY be added to Entry to carry metadata
