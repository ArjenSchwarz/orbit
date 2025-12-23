---
references:
    - specs/apsis/requirements.md
    - specs/apsis/design.md
    - specs/apsis/decision_log.md
---
# Apsis Implementation Tasks

## Phase 1: Shared Transcript Package

- [x] 1. Create internal/transcript/types.go with exported data structures
  - Create Entry, Message, ContentItem structs (exported versions of existing types)
  - Create RenderOptions struct with Title and SessionID fields
  - Add JSON tags matching existing format
  - Add doc comments for all exported types
  - Requirements: [1.4](requirements.md#1.4), [1.5](requirements.md#1.5)

- [x] 2. Create internal/transcript/parser.go with JSONL parsing
  - Implement ParseJSONL(io.Reader) (*ParseResult, error)
  - Implement ParseFirstTimestamp(io.Reader) (time.Time, error)
  - Create ParseResult and ParseWarning structs
  - Add line number tracking for warnings
  - Use 64KB initial buffer, 10MB max per line
  - Handle bufio.ErrTooLong with line number in warning
  - Requirements: [1.1](requirements.md#1.1), [1.6](requirements.md#1.6), [1.7](requirements.md#1.7), [1.8](requirements.md#1.8)

- [x] 3. Create internal/transcript/markdown.go with Markdown rendering
  - Implement RenderMarkdown(entries []Entry, opts RenderOptions) string
  - Implement formatUserMessage with User heading
  - Implement formatAssistantMessage with thinking, text, tool_use, tool_result handling
  - Implement truncateString with UTF-8 safe rune-based truncation (Decision 12)
  - Use 2000 runes max for tool inputs, 3000 for results
  - Add horizontal rules between messages
  - Requirements: [1.2](requirements.md#1.2), [1.3](requirements.md#1.3), [1.9](requirements.md#1.9)

- [x] 4. Create internal/transcript/parser_test.go
  - Test ParseJSONL with valid JSONL input
  - Test empty file returns empty slice
  - Test malformed lines generate warnings with line numbers
  - Test unknown entry types are skipped silently
  - Test buffer overflow handling (near 10MB line)
  - Test ParseFirstTimestamp extracts first timestamp
  - Requirements: [1.1](requirements.md#1.1), [1.7](requirements.md#1.7), [1.8](requirements.md#1.8), [3.4](requirements.md#3.4), [4.1](requirements.md#4.1), [4.2](requirements.md#4.2), [4.7](requirements.md#4.7)

- [x] 5. Create internal/transcript/markdown_test.go
  - Test user message rendering with User heading
  - Test assistant message with thinking block in details tag
  - Test tool_use rendering with JSON code block
  - Test tool_result with success and error variants
  - Test truncation at rune boundaries (UTF-8 safe)
  - Test RenderOptions.Title customization
  - Test horizontal rules between messages
  - Test unknown content types are skipped silently
  - Requirements: [1.2](requirements.md#1.2), [1.3](requirements.md#1.3), [1.4](requirements.md#1.4), [1.9](requirements.md#1.9), [4.8](requirements.md#4.8), [5.1](requirements.md#5.1), [5.2](requirements.md#5.2), [5.3](requirements.md#5.3), [5.4](requirements.md#5.4), [5.5](requirements.md#5.5), [5.6](requirements.md#5.6), [5.7](requirements.md#5.7)

- [x] 6. Create internal/transcript/testdata/ fixtures
  - Create valid.jsonl with user, assistant, tool_use, tool_result entries
  - Create malformed.jsonl with invalid JSON lines
  - Create empty.jsonl (empty file)
  - Create unknown_types.jsonl with unknown entry and content types
  - Create unicode.jsonl with multi-byte UTF-8 content for truncation tests

## Phase 2: Orbit Integration

- [x] 7. Refactor internal/logs/manager.go to use transcript package
  - Add import for internal/transcript
  - Remove local transcriptEntry, transcriptMsg, contentItem types
  - Update generateMarkdownTranscript to use transcript.ParseJSONL and transcript.RenderMarkdown
  - Update generatePostCompletionMarkdownTranscript similarly
  - Use RenderOptions with phase-specific titles
  - Log warnings from ParseResult to stderr
  - Requirements: [6.1](requirements.md#6.1), [6.4](requirements.md#6.4), [6.5](requirements.md#6.5)

- [x] 8. Fix path normalization in copySessionTranscript
  - Update projectPath conversion to remove leading separator (Decision 11)
  - Use strings.TrimPrefix before replacing separators
  - Handle both Unix and Windows path separators
  - Requirements: [2.10](requirements.md#2.10)

- [x] 9. Verify existing Orbit tests pass
  - Run make test to ensure all existing tests pass
  - Manually verify Markdown output is visually similar (Decision 13)
  - Fix any regressions identified
  - Requirements: [6.2](requirements.md#6.2), [6.3](requirements.md#6.3), [6.6](requirements.md#6.6)

## Phase 3: Apsis CLI

- [x] 10. Create cmd/apsis/main.go with flag parsing and version
  - Create Config struct with List, Output, Project, Version, Help, Input fields
  - Implement flag parsing with -l/--list, -o/--output, -p/--project, -v/--version, -h/--help
  - Add version variable for build-time injection
  - Implement usage help display
  - Handle --version flag to print version and exit
  - Requirements: [2.7](requirements.md#2.7), [2.9](requirements.md#2.9), [2.11](requirements.md#2.11), [2.12](requirements.md#2.12)

- [x] 11. Implement input source resolution (file path vs session ID vs stdin)
  - Implement isFilePath(arg) checking for path separators, .jsonl extension, or file existence
  - Implement isInputFromPipe() using os.Stdin.Stat()
  - Implement resolveInput returning io.ReadCloser and session ID
  - Show help and exit 1 when TTY with no args
  - Build Claude project path using buildClaudePath with leading separator fix
  - Requirements: [2.1](requirements.md#2.1), [2.2](requirements.md#2.2), [2.3](requirements.md#2.3), [2.4](requirements.md#2.4), [2.5](requirements.md#2.5), [2.8](requirements.md#2.8), [2.10](requirements.md#2.10)

- [x] 12. Implement session discovery (--list flag)
  - Implement ListSessions reading .jsonl files from Claude projects directory
  - Use ParseFirstTimestamp for creation date, file mtime as fallback
  - Create SessionInfo struct with ID, CreatedAt, Size
  - Sort sessions by creation date (most recent first)
  - Format output as tab-separated: SESSION_ID, CREATED_AT (RFC3339), SIZE (human-readable)
  - Handle no sessions found with informative message
  - Handle project directory not found with error message
  - Requirements: [3.1](requirements.md#3.1), [3.2](requirements.md#3.2), [3.3](requirements.md#3.3), [3.4](requirements.md#3.4), [3.5](requirements.md#3.5), [3.6](requirements.md#3.6), [3.7](requirements.md#3.7), [3.8](requirements.md#3.8), [3.9](requirements.md#3.9), [3.10](requirements.md#3.10)

- [x] 13. Implement conversion logic
  - Implement convert(input io.Reader, output io.Writer, sessionID string) error
  - Use transcript.ParseJSONL to parse input
  - Use transcript.RenderMarkdown with Session Transcript title
  - Write warnings to stderr
  - Handle empty file with Session contains no entries message
  - Write output to stdout by default, or file if -o specified
  - Exit 0 on success, 1 on error
  - Requirements: [2.6](requirements.md#2.6), [4.3](requirements.md#4.3), [4.4](requirements.md#4.4), [4.5](requirements.md#4.5), [4.6](requirements.md#4.6), [5.1](requirements.md#5.1)

- [x] 14. Create cmd/apsis/main_test.go
  - Test isFilePath returns true for paths with /
  - Test isFilePath returns true for paths ending in .jsonl
  - Test isFilePath returns true for existing files
  - Test isFilePath returns false for session ID format
  - Test buildClaudePath removes leading separator and replaces / with -
  - Test listSessions returns sorted sessions
  - Test convert writes to output correctly
  - Requirements: [2.2](requirements.md#2.2), [2.3](requirements.md#2.3), [2.10](requirements.md#2.10)

## Phase 4: Build System

- [x] 15. Update Makefile with apsis build targets
  - Add VERSION variable from git describe
  - Add build-orbit target for orbit binary only
  - Add build-apsis target with ldflags for version injection
  - Update build target to build both binaries
  - Update install target to install both binaries
  - Update clean target to remove both binaries
  - Requirements: [7.1](requirements.md#7.1), [7.2](requirements.md#7.2), [7.3](requirements.md#7.3), [7.4](requirements.md#7.4)

- [x] 16. Verify build and test targets
  - Run make build and verify both binaries are created
  - Run make test and verify all tests pass including transcript package
  - Run make lint and fix any linting issues
  - Test apsis --version shows injected version
  - Requirements: [7.5](requirements.md#7.5), [7.6](requirements.md#7.6)
