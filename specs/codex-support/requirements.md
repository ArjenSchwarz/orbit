# Codex Log Format Support - Requirements

## Introduction

This feature extends Apsis to support OpenAI Codex CLI session logs alongside the existing Claude Code format. Users can convert Codex session transcripts to Markdown or HTML using the same workflow they use for Claude Code sessions, with automatic format detection and transparent operation.

The feature includes:
- Automatic format detection from the first JSONL line
- Codex session discovery in `~/.codex/sessions/` (date-based directory structure)
- Codex JSONL parsing with normalization to existing Entry type
- Unified session listing showing both Claude and Codex sessions
- Reuse of existing Markdown/HTML rendering pipeline

This is an Apsis-only feature for v1. Orbit integration is deferred to future work.

### Definitions

- **Thinking block**: An assistant message content item rendered with distinct styling to indicate internal reasoning. In the data model, this corresponds to a `ContentItem` with `Type: "thinking"` and the reasoning text in the `Thinking` field.
- **Entry**: The normalized internal representation of a conversation turn, as defined in `internal/transcript/types.go`.
- **ContentItem**: A single piece of content within an Entry's message (text, thinking, tool_use, or tool_result).

### Format Stability Note

The Codex CLI log format is based on observed behavior as of January 2026. The format is not officially documented by OpenAI and may change. The parser includes warning logging for unrecognized event types to support forward compatibility.

---

## Requirements

### 1. Format Detection

**User Story:** As a user, I want Apsis to automatically detect whether a session file is from Claude Code or Codex, so that I don't have to specify the format manually.

**Acceptance Criteria:**

1. <a name="1.1"></a>The system SHALL detect the log format by examining the first non-empty line of the JSONL file
2. <a name="1.2"></a>IF the first non-empty line contains a top-level `type` field (case-sensitive) with value `session_meta`, `response_item`, `event_msg`, or `turn_context`, THEN the system SHALL classify it as Codex format
3. <a name="1.3"></a>IF the first non-empty line contains a top-level `type` field with value `user` or `assistant`, THEN the system SHALL classify it as Claude Code format
4. <a name="1.4"></a>IF the first non-empty line cannot be parsed as valid JSON, THEN the system SHALL return an error with message "failed to parse first line as JSON"
5. <a name="1.5"></a>IF the first non-empty line is valid JSON but contains no recognized `type` field, THEN the system SHALL return an error with message "unrecognized log format: type field value '{value}'"
6. <a name="1.6"></a>IF the file is empty (zero bytes) or contains only whitespace, THEN the system SHALL return an error with message "empty file"
7. <a name="1.7"></a>The system SHALL NOT require any CLI flag changes to support format detection
8. <a name="1.8"></a>The system SHALL handle UTF-8 encoded files and strip any BOM (Byte Order Mark) before parsing

### 2. Codex Session Discovery

**User Story:** As a user, I want Apsis to find Codex sessions automatically, so that I can convert them without providing full file paths.

**Acceptance Criteria:**

1. <a name="2.1"></a>The system SHALL search `~/.codex/sessions/` for Codex session files
2. <a name="2.2"></a>The system SHALL support the Codex date-based directory structure: `YYYY/MM/DD/{session-id}.jsonl`
3. <a name="2.3"></a>The system SHALL match session IDs by full UUID (36 characters with hyphens) when searching (e.g., `019b892c-3a14-7773-bd76-6465a8a0b634` matches `rollout-2026-01-05T00-22-15-019b892c-3a14-7773-bd76-6465a8a0b634.jsonl`)
4. <a name="2.4"></a>WHEN resolving a session path, the system SHALL check Claude Code locations first, then Codex locations
5. <a name="2.5"></a>IF a session ID matches files in both Claude Code and Codex locations, THEN the system SHALL use the Claude Code version
6. <a name="2.6"></a>IF the `~/.codex/sessions/` directory does not exist, THEN the system SHALL skip Codex discovery without error
7. <a name="2.7"></a>IF a file in the session directory is empty (zero bytes), THEN the system SHALL ignore it during discovery
8. <a name="2.8"></a>The system SHALL follow symlinks when traversing the `~/.codex/sessions/` directory

### 3. Session Listing

**User Story:** As a user, I want to see all my sessions from both Claude Code and Codex in a single list, so that I can easily find and access any session.

**Acceptance Criteria:**

1. <a name="3.1"></a>WHEN the user runs `apsis --list`, the system SHALL display sessions from both Claude Code and Codex locations
2. <a name="3.2"></a>The system SHALL indicate the source (Claude or Codex) for each listed session
3. <a name="3.3"></a>The system SHALL sort sessions by timestamp (from the first `session_meta` event for Codex, or file modification time for Claude) with most recent first
4. <a name="3.4"></a>IF either session location is unavailable, THEN the system SHALL list sessions from the available location without error
5. <a name="3.5"></a>IF both session locations are unavailable, THEN the system SHALL display a message "no sessions found" without error
6. <a name="3.6"></a>IF two sessions have identical timestamps, THEN the system SHALL sort Claude sessions before Codex sessions

### 4. Codex JSONL Parsing

**User Story:** As a user, I want Apsis to parse Codex session logs correctly, so that I can view them as formatted transcripts.

**Acceptance Criteria:**

1. <a name="4.1"></a>The system SHALL parse Codex JSONL files with the following event types: `session_meta`, `response_item`, `event_msg`, `turn_context`
2. <a name="4.2"></a>The system SHALL convert `response_item` entries with `payload.role: "user"` to Entry with `Type: "user"`
3. <a name="4.3"></a>The system SHALL convert `response_item` entries with `payload.role: "assistant"` to Entry with `Type: "assistant"`
4. <a name="4.4"></a>The system SHALL convert `function_call` response items to ContentItem with `Type: "tool_use"`, mapping `call_id` to `ID`, `name` to `Name`, and parsed `arguments` to `Input`
5. <a name="4.5"></a>The system SHALL convert `function_call_output` response items to ContentItem with `Type: "tool_result"`, mapping `call_id` to `ToolUseID` and `output` to `Content`
6. <a name="4.6"></a>The system SHALL link `function_call` and `function_call_output` entries by matching `call_id` values within the same session file
7. <a name="4.7"></a>IF a `function_call_output` has no matching `function_call`, THEN the system SHALL render it as a standalone tool result with the call_id displayed in the output header
8. <a name="4.8"></a>IF multiple `function_call_output` entries share the same `call_id`, THEN the system SHALL render each as a separate tool_result linked to the same tool_use
9. <a name="4.9"></a>The system SHALL populate `Entry.Timestamp` from the `timestamp` field of each Codex event in ISO 8601 format
10. <a name="4.10"></a>The system SHALL populate `Entry.SessionID` from the `id` field of the `session_meta` payload
11. <a name="4.11"></a>The system SHALL log a warning and skip any event with an unrecognized `type` value not listed in requirements 4.1, 5.5, 5.6, or 6.1-6.5

### 5. Content Type Mapping

**User Story:** As a user, I want Codex content types to be rendered appropriately, so that I can read the transcript naturally.

**Acceptance Criteria:**

1. <a name="5.1"></a>The system SHALL convert Codex `input_text` content items to ContentItem with `Type: "text"` and text in `Text` field
2. <a name="5.2"></a>The system SHALL convert Codex `output_text` content items to ContentItem with `Type: "text"` and text in `Text` field
3. <a name="5.3"></a>The system SHALL convert `reasoning` response items to ContentItem with `Type: "thinking"`, extracting text from `summary[].text` into the `Thinking` field
4. <a name="5.4"></a>The system SHALL NOT render the `encrypted_content` field from reasoning entries
5. <a name="5.5"></a>The system SHALL convert `agent_reasoning` event messages to ContentItem with `Type: "thinking"` and `payload.text` in `Thinking` field
6. <a name="5.6"></a>The system SHALL convert `agent_message` event messages to ContentItem with `Type: "text"` and `payload.message` in `Text` field
7. <a name="5.7"></a>IF a Codex content item has an unrecognized type, THEN the system SHALL render it as text with the raw JSON content

### 6. Metadata Event Filtering

**User Story:** As a user, I want non-essential metadata to be filtered out, so that transcripts remain readable and focused on the conversation.

**Acceptance Criteria:**

1. <a name="6.1"></a>The system SHALL skip `session_meta` entries during transcript rendering (used only for metadata extraction)
2. <a name="6.2"></a>The system SHALL skip `turn_context` entries during transcript rendering
3. <a name="6.3"></a>The system SHALL skip `token_count` event messages during transcript rendering
4. <a name="6.4"></a>The system SHALL skip `user_message` event messages as these duplicate the content in `response_item` message entries
5. <a name="6.5"></a>The system SHALL skip `ghost_snapshot` response items during transcript rendering

### 7. Tool Name Display

**User Story:** As a user, I want Codex tool names to be displayed authentically, so that transcripts accurately reflect the Codex CLI experience.

**Acceptance Criteria:**

1. <a name="7.1"></a>The system SHALL display Codex tool names exactly as they appear in the log (e.g., `shell_command` displays as `shell_command`)
2. <a name="7.2"></a>The system SHALL parse `shell_command` arguments JSON to extract the `command` field for display
3. <a name="7.3"></a>IF the `arguments` field is not valid JSON, THEN the system SHALL display the raw arguments string

### 8. Output Compatibility

**User Story:** As a user, I want Codex transcripts to have the same output options and quality as Claude Code transcripts.

**Acceptance Criteria:**

1. <a name="8.1"></a>The system SHALL support Markdown output for Codex transcripts using `RenderMarkdown()`
2. <a name="8.2"></a>The system SHALL support HTML output for Codex transcripts using `RenderHTML()` and `RenderHTMLFragment()`
3. <a name="8.3"></a>The system SHALL normalize Codex entries to `[]Entry` before passing to rendering functions
4. <a name="8.4"></a>The system SHALL apply the same CSS styling to Codex transcripts as Claude Code transcripts

### 9. Error Handling

**User Story:** As a user, I want clear feedback when something goes wrong with Codex parsing, without losing the ability to view the rest of the transcript.

**Acceptance Criteria:**

1. <a name="9.1"></a>IF a Codex JSONL line is malformed JSON, THEN the system SHALL log a warning to stderr and continue parsing subsequent lines
2. <a name="9.2"></a>IF required fields are missing from a Codex entry (e.g., `type`, `payload`), THEN the system SHALL skip that entry and log a warning to stderr
3. <a name="9.3"></a>The system SHALL report the total number of warnings to stderr after parsing completes in format "parsed with N warning(s)"
4. <a name="9.4"></a>The system SHALL include line numbers in warning messages in format "line N: {message}"
5. <a name="9.5"></a>IF all lines in a file are malformed, THEN the system SHALL return an error with message "no valid entries found in file"
6. <a name="9.6"></a>IF the final line of a file is incomplete JSON (truncated), THEN the system SHALL treat it as malformed and log a warning

---

## Appendix A: Field Mapping Table

This table defines the exact mapping from Codex event types to Entry and ContentItem fields.

### Event Type to Entry Mapping

| Codex Event | Condition | Entry.Type | Entry.Timestamp | Entry.SessionID |
|-------------|-----------|------------|-----------------|-----------------|
| `response_item` | `payload.type="message"`, `payload.role="user"` | `"user"` | From event `timestamp` | From `session_meta.payload.id` |
| `response_item` | `payload.type="message"`, `payload.role="assistant"` | `"assistant"` | From event `timestamp` | From `session_meta.payload.id` |
| `response_item` | `payload.type="function_call"` | `"assistant"` | From event `timestamp` | From `session_meta.payload.id` |
| `response_item` | `payload.type="reasoning"` | `"assistant"` | From event `timestamp` | From `session_meta.payload.id` |
| `event_msg` | `payload.type="agent_reasoning"` | `"assistant"` | From event `timestamp` | From `session_meta.payload.id` |
| `event_msg` | `payload.type="agent_message"` | `"assistant"` | From event `timestamp` | From `session_meta.payload.id` |

### Content Type to ContentItem Mapping

| Codex Source | ContentItem.Type | Field Mappings |
|--------------|------------------|----------------|
| `payload.content[].type="input_text"` | `"text"` | `Text` = `content[].text` |
| `payload.content[].type="output_text"` | `"text"` | `Text` = `content[].text` |
| `payload.type="function_call"` | `"tool_use"` | `ID` = `call_id`, `Name` = `name`, `Input` = parsed `arguments` JSON |
| `payload.type="function_call_output"` | `"tool_result"` | `ToolUseID` = `call_id`, `Content` = `output` |
| `payload.type="reasoning"` | `"thinking"` | `Thinking` = concatenated `summary[].text` |
| `event_msg.payload.type="agent_reasoning"` | `"thinking"` | `Thinking` = `payload.text` |
| `event_msg.payload.type="agent_message"` | `"text"` | `Text` = `payload.message` |

### Skipped Event Types

| Codex Event | Reason |
|-------------|--------|
| `session_meta` | Metadata only, used for SessionID extraction |
| `turn_context` | Internal context tracking |
| `token_count` event_msg | Usage statistics |
| `user_message` event_msg | Duplicates response_item content |
| `ghost_snapshot` response_item | Git tracking metadata |

---

## Out of Scope for v1

- Orbit integration for Codex run tracking
- Codex-specific web interface features
- Token count display in transcripts
- Git ghost snapshot display
- Encrypted reasoning content decryption
- Filtering by source (Claude vs Codex) in session listing
- Configurable session search paths (hardcoded to `~/.codex/sessions/`)
