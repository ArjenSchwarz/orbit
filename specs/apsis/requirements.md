# Apsis - Claude Session Transcript Converter

## Introduction

Apsis is a standalone CLI tool for converting Claude Code session transcripts from JSONL format to readable Markdown. The name comes from orbital mechanics, where apsis refers to the extreme points of an orbit, fitting the Orbit project theme.

The tool extracts the session log parsing and Markdown rendering functionality currently embedded in Orbit's `internal/logs/manager.go` into a shared `internal/transcript` package. Both Orbit and apsis will use this shared package, ensuring consistent message body rendering.

**Key capabilities:**
- Convert any Claude Code session transcript to Markdown
- List available sessions for any project on the system
- Read from file path, session ID, or stdin
- Consistent message rendering with Orbit (headers are context-specific)

---

## Requirements

### 1. Shared Transcript Package

**User Story:** As a developer, I want session parsing and Markdown rendering in a reusable package, so that both Orbit and apsis produce consistent output.

**Acceptance Criteria:**

1. <a name="1.1"></a>The system SHALL provide an `internal/transcript` package that exports JSONL parsing functionality
2. <a name="1.2"></a>The system SHALL provide Markdown rendering functionality in the transcript package
3. <a name="1.3"></a>The transcript package SHALL produce similar Markdown for message body content (user messages, assistant messages, tool uses, tool results) as the current `internal/logs/manager.go` implementation, with improvements to UTF-8 handling and path normalization
4. <a name="1.4"></a>The renderer SHALL accept a `RenderOptions` struct with a configurable `Title` field for the document header
5. <a name="1.5"></a>The system SHALL export `Entry`, `Message`, and `ContentItem` types for parsed transcript data
6. <a name="1.6"></a>The parser SHALL accept an `io.Reader` interface for input flexibility
7. <a name="1.7"></a>The parser SHALL use buffering with 64KB initial size and 10MB maximum per line
8. <a name="1.8"></a>WHEN a JSONL line exceeds the 10MB buffer limit, the system SHALL emit an error to stderr with the line number and skip to the next line
9. <a name="1.9"></a>The renderer SHALL truncate tool inputs at 2000 characters and tool results at 3000 characters

---

### 2. Session Conversion

**User Story:** As a user, I want to convert Claude session transcripts to Markdown, so that I can read and share my session history in a human-friendly format.

**Acceptance Criteria:**

1. <a name="2.1"></a>The apsis CLI SHALL accept a positional argument that is either a session ID or a file path
2. <a name="2.2"></a>The system SHALL treat the positional argument as a file path IF it contains a path separator (`/` or `\`) OR ends with `.jsonl` OR a file exists at that path
3. <a name="2.3"></a>The system SHALL treat the positional argument as a session ID IF it does not match the file path criteria in [2.2](#2.2)
4. <a name="2.4"></a>The apsis CLI SHALL read JSONL from stdin WHEN no positional argument is provided AND stdin is not a TTY
5. <a name="2.5"></a>WHEN stdin is a TTY and no positional argument is provided, the system SHALL display usage help and exit with code 1
6. <a name="2.6"></a>The system SHALL write Markdown output to stdout by default
7. <a name="2.7"></a>The system SHALL support `-o, --output <file>` flag to write to a specific file
8. <a name="2.8"></a>WHEN given a session ID, the system SHALL locate the JSONL file in `~/.claude/projects/{project-path}/{session-id}.jsonl`
9. <a name="2.9"></a>The system SHALL support `-p, --project <path>` flag to specify the project directory (defaults to current working directory)
10. <a name="2.10"></a>The system SHALL convert project paths to Claude's format by replacing path separators with dashes and removing the leading separator
11. <a name="2.11"></a>The system SHALL support `-h, --help` flag to display usage information
12. <a name="2.12"></a>The system SHALL support `-v, --version` flag to display the version number

---

### 3. Session Discovery

**User Story:** As a user, I want to list available sessions for a project, so that I can find the session ID I want to convert.

**Acceptance Criteria:**

1. <a name="3.1"></a>The apsis CLI SHALL support `-l, --list` flag to list available sessions
2. <a name="3.2"></a>WHEN `--list` is specified, the system SHALL display sessions for the current project (or project specified by `--project`)
3. <a name="3.3"></a>The session list SHALL display: session ID, creation date/time, and file size in tab-separated columns
4. <a name="3.4"></a>The creation date/time SHALL be parsed from the first entry's timestamp in the JSONL file
5. <a name="3.5"></a>IF the first entry's timestamp cannot be parsed, the system SHALL use the file modification time as a fallback
6. <a name="3.6"></a>The creation date/time SHALL be formatted in RFC3339 format
7. <a name="3.7"></a>The file size SHALL be displayed in human-readable format (e.g., "1.2 MB")
8. <a name="3.8"></a>The session list SHALL be sorted by creation date (most recent first)
9. <a name="3.9"></a>IF no sessions exist for the project, the system SHALL display "No sessions found for project: {project-path}"
10. <a name="3.10"></a>IF the project path cannot be resolved to a Claude projects directory, the system SHALL display an error message

---

### 4. Error Handling

**User Story:** As a user, I want clear feedback when something goes wrong, so that I can understand and resolve issues.

**Acceptance Criteria:**

1. <a name="4.1"></a>WHEN parsing malformed JSONL entries, the system SHALL skip the entry and emit a warning to stderr including the line number
2. <a name="4.2"></a>The system SHALL continue processing remaining entries after encountering a malformed entry
3. <a name="4.3"></a>WHEN a session ID is not found, the system SHALL display an error with the expected file path
4. <a name="4.4"></a>WHEN the JSONL file is empty, the system SHALL display "Session contains no entries" and exit with code 0
5. <a name="4.5"></a>The system SHALL exit with code 0 on success, code 1 on error
6. <a name="4.6"></a>Error messages SHALL be written to stderr, not stdout
7. <a name="4.7"></a>WHEN unknown entry types are encountered, the system SHALL skip them silently
8. <a name="4.8"></a>WHEN unknown content item types are encountered within known entries, the system SHALL skip them silently

---

### 5. Markdown Output Format

**User Story:** As a user, I want readable Markdown output with clear visual structure, so that I can easily navigate the session content.

**Acceptance Criteria:**

1. <a name="5.1"></a>The apsis Markdown output SHALL begin with `# Session Transcript` followed by `**Session ID:** \`{session-id}\`` on a new line
2. <a name="5.2"></a>User messages SHALL be rendered with `## 👤 User` heading (H2 level)
3. <a name="5.3"></a>Assistant messages SHALL be rendered with `## 🤖 Assistant` heading (H2 level)
4. <a name="5.4"></a>Thinking blocks SHALL be wrapped in collapsible `<details>` tags with `<summary>💭 Thinking</summary>`
5. <a name="5.5"></a>Tool use blocks SHALL be rendered with `### 🔧 Tool: \`{tool_name}\`` heading (H3 level) and input as fenced JSON code block
6. <a name="5.6"></a>Tool results SHALL be rendered with `#### ✅ Tool Result` (H4 level) for success or `#### ❌ Tool Error` for errors
7. <a name="5.7"></a>Each message section SHALL be followed by a horizontal rule (`---`)
8. <a name="5.8"></a>Text content SHALL be rendered as-is without additional formatting

---

### 6. Orbit Integration

**User Story:** As an Orbit maintainer, I want Orbit to use the shared transcript package, so that code duplication is eliminated.

**Acceptance Criteria:**

1. <a name="6.1"></a>The `internal/logs/manager.go` SHALL be refactored to import and use `internal/transcript`
2. <a name="6.2"></a>All existing Orbit tests SHALL pass after the refactor
3. <a name="6.3"></a>The Markdown message body content from Orbit (excluding headers) SHALL be similar to pre-refactor output, with improvements to UTF-8 handling
4. <a name="6.4"></a>Orbit SHALL use `RenderOptions{Title: "Phase {N} Session Transcript"}` for phase sessions
5. <a name="6.5"></a>Orbit SHALL use `RenderOptions{Title: "Post-Completion Session Transcript"}` for post-completion sessions
6. <a name="6.6"></a>The system SHALL not introduce breaking changes to Orbit's external behavior

---

### 7. Build and Installation

**User Story:** As a developer, I want apsis to be built and installed alongside Orbit, so that both tools are easily accessible.

**Acceptance Criteria:**

1. <a name="7.1"></a>The Makefile SHALL include a `build-orbit` target to build only the orbit binary
2. <a name="7.2"></a>The Makefile SHALL include a `build-apsis` target to build only the apsis binary
3. <a name="7.3"></a>The `make build` target SHALL build both orbit and apsis binaries
4. <a name="7.4"></a>The `make install` target SHALL install both binaries to GOPATH/bin
5. <a name="7.5"></a>The apsis binary source SHALL be located at `cmd/apsis/main.go`
6. <a name="7.6"></a>The `make test` target SHALL include tests for the transcript package
