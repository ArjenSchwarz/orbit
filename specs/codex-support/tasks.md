---
references:
    - requirements.md
    - design.md
    - decision_log.md
---
# Codex Log Format Support - Implementation Tasks

## Phase 1: Types and Format Detection

- [x] 1. Add Format enum and helper types to types.go
  - Add Format enum (FormatUnknown, FormatClaude, FormatCodex)
  - Add ParseWarning struct
  - Requirements: [1.1](requirements.md#1.1), [1.2](requirements.md#1.2), [1.3](requirements.md#1.3)
  - References: internal/transcript/types.go

- [x] 2. Write unit tests for format detection
  - Test Claude format detection
  - Test Codex format detection
  - Test empty file
  - Test invalid JSON
  - Test BOM handling
  - Test whitespace-only
  - Test unrecognized type
  - Requirements: [1.1](requirements.md#1.1), [1.2](requirements.md#1.2), [1.3](requirements.md#1.3), [1.4](requirements.md#1.4), [1.5](requirements.md#1.5), [1.6](requirements.md#1.6), [1.8](requirements.md#1.8)
  - References: internal/transcript/parser_test.go

- [x] 3. Implement DetectFormat and readFirstNonEmptyLine functions
  - Implement format detection from first non-empty line
  - Handle BOM stripping
  - Return appropriate errors
  - Requirements: [1.1](requirements.md#1.1), [1.2](requirements.md#1.2), [1.3](requirements.md#1.3), [1.4](requirements.md#1.4), [1.5](requirements.md#1.5), [1.6](requirements.md#1.6), [1.8](requirements.md#1.8)
  - References: internal/transcript/parser.go

## Phase 2: Codex Types and Parser

- [x] 4. Create codex_types.go with Codex struct definitions
  - Add CodexEntry struct
  - Add CodexResponseItem struct
  - Add CodexContent struct
  - Add CodexSummary struct
  - Add CodexEventMsg struct
  - Add CodexSessionMeta struct
  - Requirements: [4.1](requirements.md#4.1)
  - References: internal/transcript/codex_types.go

- [x] 5. Write unit tests for Codex type unmarshaling
  - Test JSON unmarshaling for each Codex type
  - Test missing fields handling
  - Test extra fields handling
  - Requirements: [4.1](requirements.md#4.1)
  - References: internal/transcript/codex_types_test.go

- [x] 6. Create test data files for Codex parsing
  - Create codex_valid.jsonl with all event types
  - Create codex_edge_cases.jsonl with malformed lines and edge cases
  - Requirements: [4.1](requirements.md#4.1), [4.7](requirements.md#4.7), [4.8](requirements.md#4.8), [4.11](requirements.md#4.11), [9.1](requirements.md#9.1), [9.2](requirements.md#9.2)
  - References: internal/transcript/testdata/codex_valid.jsonl, internal/transcript/testdata/codex_edge_cases.jsonl

- [x] 7. Write unit tests for ParseCodexJSONL
  - Test message conversion
  - Test function call linking
  - Test reasoning extraction
  - Test event filtering
  - Test orphaned outputs
  - Test multi-output tool calls
  - Test entry consolidation
  - Requirements: [4.2](requirements.md#4.2), [4.3](requirements.md#4.3), [4.4](requirements.md#4.4), [4.5](requirements.md#4.5), [4.6](requirements.md#4.6), [4.7](requirements.md#4.7), [4.8](requirements.md#4.8), [4.9](requirements.md#4.9), [4.10](requirements.md#4.10), [4.11](requirements.md#4.11), [5.1](requirements.md#5.1), [5.2](requirements.md#5.2), [5.3](requirements.md#5.3), [5.4](requirements.md#5.4), [5.5](requirements.md#5.5), [5.6](requirements.md#5.6), [5.7](requirements.md#5.7), [6.1](requirements.md#6.1), [6.2](requirements.md#6.2), [6.3](requirements.md#6.3), [6.4](requirements.md#6.4), [6.5](requirements.md#6.5)
  - References: internal/transcript/codex_parser_test.go

- [x] 8. Implement ParseCodexJSONL and entry conversion functions
  - Implement codexParser struct
  - Implement parseEntry function
  - Implement convertMessage function
  - Implement convertFunctionCall function
  - Implement convertFunctionCallOutput function
  - Implement convertReasoning function
  - Implement convertEventMsg function
  - Implement linkToolCalls function
  - Implement buildEntries function
  - Implement processEvent with entry consolidation
  - Requirements: [4.2](requirements.md#4.2), [4.3](requirements.md#4.3), [4.4](requirements.md#4.4), [4.5](requirements.md#4.5), [4.6](requirements.md#4.6), [4.7](requirements.md#4.7), [4.8](requirements.md#4.8), [4.9](requirements.md#4.9), [4.10](requirements.md#4.10), [4.11](requirements.md#4.11), [5.1](requirements.md#5.1), [5.2](requirements.md#5.2), [5.3](requirements.md#5.3), [5.4](requirements.md#5.4), [5.5](requirements.md#5.5), [5.6](requirements.md#5.6), [5.7](requirements.md#5.7)
  - References: internal/transcript/codex_parser.go

- [x] 9. Implement metadata event filtering
  - Skip session_meta events during transcript rendering
  - Skip turn_context events
  - Skip token_count events
  - Skip user_message events
  - Skip ghost_snapshot events
  - Requirements: [6.1](requirements.md#6.1), [6.2](requirements.md#6.2), [6.3](requirements.md#6.3), [6.4](requirements.md#6.4), [6.5](requirements.md#6.5)
  - References: internal/transcript/codex_parser.go

- [x] 10. Write property-based tests for format detection and entry normalization
  - Test format detection idempotence
  - Test text preservation during normalization
  - Test tool call linking invariant
  - Use pgregory.net/rapid framework
  - Requirements: [4.4](requirements.md#4.4), [4.5](requirements.md#4.5), [4.6](requirements.md#4.6), [5.1](requirements.md#5.1), [5.2](requirements.md#5.2)
  - References: internal/transcript/codex_parser_test.go

## Phase 3: Parser Integration

- [x] 11. Write unit tests for updated ParseJSONL with format dispatch
  - Test auto-detection and delegation to Claude parser
  - Test auto-detection and delegation to Codex parser
  - Test error propagation
  - Requirements: [1.1](requirements.md#1.1), [1.7](requirements.md#1.7)
  - References: internal/transcript/parser_test.go

- [x] 12. Update ParseJSONL to detect format and dispatch to appropriate parser
  - Refactor existing Claude parsing to parseClaudeJSONL
  - Add format detection using DetectFormat
  - Use io.MultiReader for streaming
  - Dispatch to ParseCodexJSONL or parseClaudeJSONL based on format
  - Requirements: [1.1](requirements.md#1.1), [1.7](requirements.md#1.7)
  - References: internal/transcript/parser.go

- [x] 13. Write integration tests for Codex to Markdown/HTML rendering
  - Test full pipeline: Codex JSONL to RenderMarkdown
  - Test full pipeline: Codex JSONL to RenderHTML
  - Requirements: [8.1](requirements.md#8.1), [8.2](requirements.md#8.2), [8.3](requirements.md#8.3), [8.4](requirements.md#8.4)
  - References: internal/transcript/parser_test.go

- [x] 14. Create golden file tests for rendering consistency
  - Create codex_simple.jsonl test input
  - Create codex_with_tools.jsonl test input
  - Create codex_reasoning.jsonl test input
  - Create golden/codex_simple.md expected output
  - Create golden/codex_simple.html expected output
  - Create golden/codex_with_tools.md expected output
  - Create golden/codex_with_tools.html expected output
  - Create golden/codex_reasoning.md expected output
  - Create golden/codex_reasoning.html expected output
  - Implement TestGoldenMarkdown and TestGoldenHTML with -update flag
  - Requirements: [8.1](requirements.md#8.1), [8.2](requirements.md#8.2)
  - References: internal/transcript/testdata/golden/, internal/transcript/parser_test.go

## Phase 4: Session Discovery

- [x] 15. Write unit tests for Codex session discovery
  - Test UUID matching with exact 36-char match
  - Test directory traversal
  - Test empty directory
  - Test symlink following
  - Test cycle detection
  - Test invalid UUID rejection
  - Requirements: [2.1](requirements.md#2.1), [2.2](requirements.md#2.2), [2.3](requirements.md#2.3), [2.4](requirements.md#2.4), [2.5](requirements.md#2.5), [2.6](requirements.md#2.6), [2.7](requirements.md#2.7), [2.8](requirements.md#2.8)
  - References: cmd/apsis/main_test.go

- [x] 16. Implement findCodexSession with UUID matching and symlink handling
  - Implement uuidPattern regex (case-insensitive)
  - Implement findCodexSession function
  - Implement walkDirFollowSymlinks with cycle detection using visited map
  - Requirements: [2.1](requirements.md#2.1), [2.2](requirements.md#2.2), [2.3](requirements.md#2.3), [2.4](requirements.md#2.4), [2.5](requirements.md#2.5), [2.6](requirements.md#2.6), [2.7](requirements.md#2.7), [2.8](requirements.md#2.8)
  - References: cmd/apsis/main.go

- [x] 17. Write unit tests for getCodexSessionTimestamp
  - Test timestamp extraction from session_meta
  - Test fallback to file mtime
  - Requirements: [3.3](requirements.md#3.3)
  - References: cmd/apsis/main_test.go

- [x] 18. Implement getCodexSessionTimestamp function
  - Extract timestamp from first session_meta event
  - Parse ISO 8601 timestamp
  - Fallback to file modification time on parse failure
  - Requirements: [3.3](requirements.md#3.3)
  - References: cmd/apsis/main.go

- [x] 19. Write unit tests for unified session listing
  - Test listing sessions from both Claude and Codex locations
  - Test source indicator display
  - Test sorting by timestamp
  - Test Claude-first tie-breaking
  - Requirements: [3.1](requirements.md#3.1), [3.2](requirements.md#3.2), [3.3](requirements.md#3.3), [3.4](requirements.md#3.4), [3.5](requirements.md#3.5), [3.6](requirements.md#3.6)
  - References: cmd/apsis/main_test.go

- [x] 20. Implement listCodexSessions and update listSessions for unified listing
  - Implement listCodexSessions using walkDirFollowSymlinks
  - Extend SessionInfo with Source field
  - Merge Claude and Codex sessions
  - Sort by timestamp with Claude-first tie-breaking
  - Display source indicator in output
  - Requirements: [3.1](requirements.md#3.1), [3.2](requirements.md#3.2), [3.3](requirements.md#3.3), [3.4](requirements.md#3.4), [3.5](requirements.md#3.5), [3.6](requirements.md#3.6)
  - References: cmd/apsis/main.go

- [x] 21. Update resolveInput to check both Claude and Codex locations
  - Check Claude location first
  - Check Codex location second
  - Return appropriate reader
  - Requirements: [2.4](requirements.md#2.4), [2.5](requirements.md#2.5)
  - References: cmd/apsis/main.go

## Phase 5: Error Handling and Negative Tests

- [ ] 22. Write negative test cases for error handling
  - Test empty file error
  - Test whitespace-only error
  - Test invalid first line JSON error
  - Test unknown format type error
  - Test malformed middle line warning
  - Test all lines malformed error
  - Test orphaned output warning
  - Test invalid UUID search behavior
  - Test symlink to missing path handling
  - Requirements: [9.1](requirements.md#9.1), [9.2](requirements.md#9.2), [9.3](requirements.md#9.3), [9.4](requirements.md#9.4), [9.5](requirements.md#9.5), [9.6](requirements.md#9.6)
  - References: internal/transcript/parser_test.go, cmd/apsis/main_test.go

- [ ] 23. Implement warning collection and error messages
  - Ensure ParseResult.Warnings is populated correctly
  - Format warning messages with line numbers: line N: message
  - Report total warnings to stderr: parsed with N warning(s)
  - Requirements: [9.1](requirements.md#9.1), [9.2](requirements.md#9.2), [9.3](requirements.md#9.3), [9.4](requirements.md#9.4), [9.5](requirements.md#9.5), [9.6](requirements.md#9.6)
  - References: internal/transcript/codex_parser.go, cmd/apsis/main.go

- [ ] 24. Write tests for tool name display
  - Test shell_command displays as shell_command (not mapped to Bash)
  - Test arguments JSON parsing to extract command field
  - Test raw arguments fallback for invalid JSON
  - Requirements: [7.1](requirements.md#7.1), [7.2](requirements.md#7.2), [7.3](requirements.md#7.3)
  - References: internal/transcript/codex_parser_test.go

## Phase 6: Final Integration

- [ ] 25. Run full test suite and fix any issues
  - Run make test
  - Run make lint
  - Fix any test failures
  - Fix any lint errors
  - References: Makefile

- [ ] 26. Add integration test for CLI with Codex session
  - Test apsis with Codex session ID resolution
  - Test apsis --list with mixed Claude and Codex sessions
  - Requirements: [2.3](requirements.md#2.3), [3.1](requirements.md#3.1)
  - References: cmd/apsis/main_test.go
