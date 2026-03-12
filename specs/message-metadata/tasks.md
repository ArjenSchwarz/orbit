---
references:
    - specs/message-metadata/requirements.md
    - specs/message-metadata/design.md
    - specs/message-metadata/decision_log.md
---
# Message Metadata (T-403)

## Foundation

- [x] 1. Add Model field to Entry struct and Timestamp to grouping structs <!-- id:03oa0ug -->
  - Modify types.go: add Model string field with json:"-" tag to Entry struct after Timestamp
  - Modify grouping.go: add Timestamp string field to renderGroup, readItem, and editItem structs
  - Type-only changes — no behaviour change, exempt from TDD requirement
  - Stream: 1
  - Requirements: [4.3](requirements.md#4.3)
  - References: internal/transcript/types.go, internal/transcript/grouping.go

## Core Helpers

- [x] 2. Write tests for metadata formatting helpers <!-- id:03oa0uh -->
  - Create metadata_test.go with table-driven tests (map-based)
  - Test FormatTimestampMarkdown: valid RFC3339 → local TZ RFC3339, empty → empty, invalid → empty
  - Test FormatTimestampHTML: valid → <time datetime> element with RFC3339 fallback, empty → empty
  - Test FormatMessageMetaMarkdown: both → " · ts · model", ts only → " · ts", model only → " · model", neither → empty
  - Test FormatMessageMetaHTML: same combinations, verify <time> element and spans
  - Use time.Local = time.FixedZone for deterministic output — check for existing TestMain first
  - Blocked-by: 03oa0ug (Add Model field to Entry struct and Timestamp to grouping structs)
  - Stream: 1
  - Requirements: [1.1](requirements.md#1.1), [1.2](requirements.md#1.2), [1.3](requirements.md#1.3), [1.4](requirements.md#1.4), [1.5](requirements.md#1.5), [3.1](requirements.md#3.1), [3.3](requirements.md#3.3), [3.4](requirements.md#3.4)
  - References: internal/transcript/metadata_test.go

- [x] 3. Implement metadata formatting helpers <!-- id:03oa0ui -->
  - Create metadata.go with FormatTimestampMarkdown, FormatTimestampHTML, FormatMessageMetaMarkdown, FormatMessageMetaHTML
  - FormatTimestampMarkdown: parse RFC3339, format in local TZ as RFC3339
  - FormatTimestampHTML: emit <time datetime=UTC> with RFC3339 fallback text
  - FormatMessageMetaMarkdown: join non-empty fields with " · " separator, prefix with " · "
  - FormatMessageMetaHTML: build <span class=message-meta> with <time> and <span class=meta-separator>
  - Return empty string for all functions when input is empty or unparseable
  - Blocked-by: 03oa0uh (Write tests for metadata formatting helpers)
  - Stream: 1
  - Requirements: [1.1](requirements.md#1.1), [1.2](requirements.md#1.2), [1.3](requirements.md#1.3), [1.4](requirements.md#1.4), [1.5](requirements.md#1.5), [3.1](requirements.md#3.1), [3.3](requirements.md#3.3), [3.4](requirements.md#3.4)
  - References: internal/transcript/metadata.go

## Grouping

- [x] 4. Write tests for grouping timestamp propagation <!-- id:03oa0uj -->
  - Add tests to verify extractReadItems and extractEditItems carry entry.Timestamp to item.Timestamp
  - Test preprocessEntries sets renderGroup.Timestamp from first item in each group
  - Test that a group of 3 reads uses the first read timestamp on the group
  - Blocked-by: 03oa0ug (Add Model field to Entry struct and Timestamp to grouping structs)
  - Stream: 1
  - Requirements: [3.6](requirements.md#3.6)
  - References: internal/transcript/grouping.go

- [x] 5. Implement grouping timestamp propagation <!-- id:03oa0uk -->
  - In extractReadItems: set readItem.Timestamp = entry.Timestamp
  - In extractEditItems: set editItem.Timestamp = entry.Timestamp
  - In flushReadGroup: set renderGroup.Timestamp = currentReadGroup[0].Timestamp
  - In flushEditGroup: set renderGroup.Timestamp = currentEditGroup[0].Timestamp
  - Blocked-by: 03oa0uj (Write tests for grouping timestamp propagation)
  - Stream: 1
  - Requirements: [3.6](requirements.md#3.6)
  - References: internal/transcript/grouping.go

## Parser Changes

- [x] 6. Write tests for Codex model extraction <!-- id:03oa0ul -->
  - Add test case to codex_parser_test.go verifying Model is extracted from session_meta
  - Extend or create test fixture with session_meta containing a model field
  - Verify Model is set only on assistant entries, not user entries
  - Verify existing timestamp extraction still works
  - Blocked-by: 03oa0ug (Add Model field to Entry struct and Timestamp to grouping structs)
  - Stream: 2
  - Requirements: [2.3](requirements.md#2.3)
  - References: internal/transcript/codex_parser_test.go, internal/transcript/codex_parser.go

- [x] 7. Implement Codex model extraction <!-- id:03oa0um -->
  - Add Model string with json:model tag to CodexSessionMeta in codex_types.go
  - Add model string field to codexParser struct
  - In processSessionMeta: store meta.Model in parser state
  - In ensureAssistantEntry: set entry.Model from parser state
  - Blocked-by: 03oa0ul (Write tests for Codex model extraction)
  - Stream: 2
  - Requirements: [2.3](requirements.md#2.3)
  - References: internal/transcript/codex_types.go, internal/transcript/codex_parser.go

- [x] 8. Write tests for Copilot timestamp and model extraction <!-- id:03oa0un -->
  - Add/extend test in copilot parser tests verifying Entry.Timestamp is populated
  - Add session.model_change event to test fixture
  - Verify Model is set on assistant entries from model_change event
  - Verify user messages get timestamp from event.Timestamp
  - Verify assistant messages get timestamp from turn_start event
  - Blocked-by: 03oa0ug (Add Model field to Entry struct and Timestamp to grouping structs)
  - Stream: 2
  - Requirements: [1.6](requirements.md#1.6), [2.3](requirements.md#2.3)
  - References: internal/transcript/copilot_parser.go

- [x] 9. Implement Copilot timestamp and model extraction <!-- id:03oa0uo -->
  - Add Model string with json:model,omitempty tag to CopilotData in copilot_types.go
  - Add currentModel and currentTurnTimestamp variables in convertCopilotToEntries
  - Handle session.model_change in second pass: set currentModel = event.Data.Model
  - Set Entry.Timestamp from event.Timestamp for user.message entries
  - Capture event.Timestamp into currentTurnTimestamp for assistant.turn_start
  - Set Entry.Timestamp from currentTurnTimestamp and Entry.Model from currentModel for assistant entries
  - Blocked-by: 03oa0un (Write tests for Copilot timestamp and model extraction)
  - Stream: 2
  - Requirements: [1.6](requirements.md#1.6), [2.3](requirements.md#2.3)
  - References: internal/transcript/copilot_types.go, internal/transcript/copilot_parser.go

- [x] 10. Write tests for Kiro CLI timestamp and model extraction <!-- id:03oa0up -->
  - Add tests to kiro_parser_test.go verifying timestamps and model are extracted
  - Verify user entry Timestamp from KiroUserMessage.Timestamp
  - Verify assistant entry Timestamp from RequestStartTimestampMs (ms epoch → RFC3339)
  - Verify assistant entry Model from RequestMetadata.ModelID
  - Test edge case: zero/negative RequestStartTimestampMs → no timestamp
  - Blocked-by: 03oa0ug (Add Model field to Entry struct and Timestamp to grouping structs)
  - Stream: 2
  - Requirements: [1.6](requirements.md#1.6), [2.3](requirements.md#2.3)
  - References: internal/transcript/kiro_parser_test.go, internal/transcript/kiro_parser.go

- [x] 11. Implement Kiro CLI timestamp and model extraction <!-- id:03oa0uq -->
  - In convertKiroUserMessage: accept *string timestamp param, set Entry.Timestamp on returned entries
  - In convertKiroAssistantMessage: accept *KiroRequestMetadata param
  - Set Entry.Timestamp from RequestStartTimestampMs via time.UnixMilli(ms).UTC().Format(time.RFC3339)
  - Set Entry.Model from ModelID
  - Skip timestamp for zero/negative ms values
  - Update convertKiroToEntries to pass timestamp and metadata to conversion functions
  - Blocked-by: 03oa0up (Write tests for Kiro CLI timestamp and model extraction)
  - Stream: 2
  - Requirements: [1.6](requirements.md#1.6), [2.3](requirements.md#2.3)
  - References: internal/transcript/kiro_parser.go

- [ ] 12. Write tests for Kiro IDE session-level timestamp and model <!-- id:03oa0ur -->
  - Add tests to kiro_ide_parser_test.go for both chat and action conversion paths
  - Verify first entry gets Timestamp from KiroIDEMetadata.StartTime (ms epoch → RFC3339)
  - Verify subsequent entries have no Timestamp
  - Verify all assistant entries get Model from KiroIDEMetadata.ModelID
  - Verify user entries do not get Model set
  - Blocked-by: 03oa0ug (Add Model field to Entry struct and Timestamp to grouping structs)
  - Stream: 2
  - Requirements: [1.6](requirements.md#1.6), [1.7](requirements.md#1.7), [2.3](requirements.md#2.3)
  - References: internal/transcript/kiro_ide_parser_test.go, internal/transcript/kiro_ide_parser.go

- [ ] 13. Implement Kiro IDE session-level timestamp and model <!-- id:03oa0us -->
  - In convertKiroIDEToEntries: after building entries, set first entry Timestamp from metadata.StartTime via time.UnixMilli(ms).UTC().Format(time.RFC3339)
  - Set Model from metadata.ModelID on all assistant entries only
  - Apply same logic in convertKiroIDEActionsToEntries
  - Skip timestamp for zero/negative StartTime values
  - Blocked-by: 03oa0ur (Write tests for Kiro IDE session-level timestamp and model)
  - Stream: 2
  - Requirements: [1.6](requirements.md#1.6), [1.7](requirements.md#1.7), [2.3](requirements.md#2.3)
  - References: internal/transcript/kiro_ide_parser.go

## Rendering

- [ ] 14. Write tests for Markdown metadata rendering <!-- id:03oa0ut -->
  - Add tests to markdown_test.go for metadata in message headers
  - Test user message header includes " · timestamp" suffix
  - Test assistant message header includes " · timestamp · model" suffix
  - Test entry with no metadata renders header unchanged (backwards compat)
  - Test read group header includes timestamp from renderGroup.Timestamp
  - Test edit group header includes timestamp from renderGroup.Timestamp
  - Test slash command header includes timestamp
  - Blocked-by: 03oa0ui (Implement metadata formatting helpers), 03oa0uk (Implement grouping timestamp propagation)
  - Stream: 1
  - Requirements: [1.1](requirements.md#1.1), [1.2](requirements.md#1.2), [3.1](requirements.md#3.1), [3.3](requirements.md#3.3), [3.5](requirements.md#3.5), [3.6](requirements.md#3.6), [4.1](requirements.md#4.1)
  - References: internal/transcript/markdown_test.go, internal/transcript/markdown.go

- [ ] 15. Implement Markdown metadata rendering <!-- id:03oa0uu -->
  - In formatUserMessage: append FormatMessageMetaMarkdown(entry.Timestamp, "") to header
  - In formatAssistantMessage: append FormatMessageMetaMarkdown(entry.Timestamp, entry.Model) to header
  - In formatSlashCommand: append FormatMessageMetaMarkdown(entry.Timestamp, "") to header
  - Update formatReadGroup to accept timestamp string param
  - Update formatEditGroup to accept timestamp string param
  - Update callers of formatReadGroup/formatEditGroup to pass renderGroup.Timestamp
  - Blocked-by: 03oa0ut (Write tests for Markdown metadata rendering)
  - Stream: 1
  - Requirements: [1.1](requirements.md#1.1), [1.2](requirements.md#1.2), [3.1](requirements.md#3.1), [3.3](requirements.md#3.3), [3.5](requirements.md#3.5), [3.6](requirements.md#3.6)
  - References: internal/transcript/markdown.go

- [ ] 16. Write tests for HTML metadata rendering <!-- id:03oa0uv -->
  - Add tests to html_test.go for metadata in HTML message headers
  - Test user header contains <span class=message-meta> with <time> element
  - Test assistant header contains <time> and model span
  - Test entry with no metadata renders header without message-meta span
  - Test standalone HTML (RenderHTML) contains inline JS for formatLocalDates
  - Test <time> element has datetime attribute and RFC3339 fallback text
  - Blocked-by: 03oa0ui (Implement metadata formatting helpers), 03oa0uk (Implement grouping timestamp propagation)
  - Stream: 1
  - Requirements: [1.1](requirements.md#1.1), [1.3](requirements.md#1.3), [1.4](requirements.md#1.4), [3.2](requirements.md#3.2), [3.4](requirements.md#3.4), [3.5](requirements.md#3.5), [3.6](requirements.md#3.6), [4.1](requirements.md#4.1)
  - References: internal/transcript/html_test.go, internal/transcript/html.go

- [ ] 17. Implement HTML metadata rendering, CSS, and standalone JS <!-- id:03oa0uw -->
  - Update all <div class=message-header> locations in html.go to include FormatMessageMetaHTML output
  - User messages and slash commands: timestamp only (empty model)
  - Assistant messages: timestamp and model
  - Read/Edit group headers: timestamp from renderGroup.Timestamp
  - Add .message-meta and .meta-separator CSS classes to transcript.css
  - In RenderHTML (standalone): embed inline IIFE script at end of <body> for formatLocalDates
  - Standalone script formats <time datetime> elements using Intl.DateTimeFormat
  - Blocked-by: 03oa0uv (Write tests for HTML metadata rendering)
  - Stream: 1
  - Requirements: [1.1](requirements.md#1.1), [1.3](requirements.md#1.3), [1.4](requirements.md#1.4), [3.2](requirements.md#3.2), [3.4](requirements.md#3.4), [3.5](requirements.md#3.5), [3.6](requirements.md#3.6)
  - References: internal/transcript/html.go, internal/transcript/transcript.css

## Integration

- [ ] 18. Write end-to-end integration tests <!-- id:03oa0ux -->
  - Create integration tests that parse sample transcripts and render to Markdown and HTML
  - Test Claude transcript: timestamps appear in output, no model (req 2.4)
  - Test Codex transcript: timestamps and model appear
  - Test Copilot transcript: timestamps and model appear
  - Test Kiro CLI transcript: timestamps and model appear
  - Test Kiro IDE transcript: first message has timestamp, all assistant messages have model (req 1.7)
  - Verify session-level metadata (cost, session ID) still renders at top (req 4.2)
  - Blocked-by: 03oa0um (Implement Codex model extraction), 03oa0uo (Implement Copilot timestamp and model extraction), 03oa0uq (Implement Kiro CLI timestamp and model extraction), 03oa0us (Implement Kiro IDE session-level timestamp and model), 03oa0uu (Implement Markdown metadata rendering), 03oa0uw (Implement HTML metadata rendering, CSS, and standalone JS)
  - Stream: 1
  - Requirements: [1.1](requirements.md#1.1), [1.6](requirements.md#1.6), [1.7](requirements.md#1.7), [2.1](requirements.md#2.1), [2.3](requirements.md#2.3), [2.4](requirements.md#2.4), [3.5](requirements.md#3.5), [4.1](requirements.md#4.1), [4.2](requirements.md#4.2)
  - References: internal/transcript/parser.go
