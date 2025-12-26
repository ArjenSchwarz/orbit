# Apsis Design Document

## Overview

Apsis is a standalone CLI tool for converting Claude Code session transcripts from JSONL format to readable Markdown. It extracts transcript parsing and rendering functionality from Orbit into a shared `internal/transcript` package, enabling both tools to produce consistent output.

### Goals

1. Extract transcript parsing and Markdown rendering into a reusable package
2. Create a CLI that converts sessions from file path, session ID, or stdin
3. Support session discovery via `--list` flag
4. Produce similar message body rendering between Orbit and apsis (with improvements)

### Non-Goals

- Multiple output formats (Markdown only for MVP)
- Session filtering or search capabilities
- Public library API (internal package only)

### Improvements Over Existing Implementation

Per design decisions, the new implementation will include these improvements:
1. **UTF-8 safe truncation**: Truncate at rune boundaries instead of byte boundaries
2. **Warning collection**: Report malformed entries with line numbers (was silent before)

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         CLI Layer                                │
├─────────────────────────────────────────────────────────────────┤
│  cmd/apsis/main.go          │  cmd/orbit/main.go                │
│  - Flag parsing             │  - Existing orchestration         │
│  - Input resolution         │  - Uses internal/logs             │
│  - Session discovery        │                                   │
└─────────────────────────────┴───────────────────────────────────┘
                │                              │
                ▼                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Shared Package Layer                          │
├─────────────────────────────────────────────────────────────────┤
│  internal/transcript/                                            │
│  ├── types.go      - Entry, Message, ContentItem                │
│  ├── parser.go     - ParseJSONL(io.Reader) ([]Entry, error)     │
│  └── markdown.go   - RenderMarkdown([]Entry, RenderOptions)     │
└─────────────────────────────────────────────────────────────────┘
                │
                ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Integration Layer                             │
├─────────────────────────────────────────────────────────────────┤
│  internal/logs/manager.go                                        │
│  - Imports internal/transcript                                   │
│  - Uses transcript.ParseJSONL and transcript.RenderMarkdown     │
│  - Provides Orbit-specific context (phase numbers, etc.)        │
└─────────────────────────────────────────────────────────────────┘
```

---

## Components and Interfaces

### 1. Package: `internal/transcript`

#### 1.1 types.go

```go
// Entry represents a single line in the Claude session JSONL.
// Maps to requirement 1.5
type Entry struct {
    Type      string   `json:"type"`
    Message   *Message `json:"message,omitempty"`
    Timestamp string   `json:"timestamp,omitempty"`
    SessionID string   `json:"sessionId,omitempty"`
}

// Message represents the message content within an entry.
type Message struct {
    Role    string        `json:"role"`
    Content []ContentItem `json:"content"`
}

// ContentItem represents a content block in a message.
type ContentItem struct {
    Type     string `json:"type"`
    Text     string `json:"text,omitempty"`
    Thinking string `json:"thinking,omitempty"`
    Name     string `json:"name,omitempty"`
    Input    any    `json:"input,omitempty"`
    Content  string `json:"content,omitempty"`
    IsError  bool   `json:"is_error,omitempty"`
}

// RenderOptions configures Markdown rendering.
// Maps to requirement 1.4
type RenderOptions struct {
    Title     string // Document title (e.g., "Session Transcript" or "Phase 1 Session Transcript")
    SessionID string // Session ID to display in header
}
```

#### 1.2 parser.go

```go
// ParseResult contains the parsed entries and any warnings encountered.
type ParseResult struct {
    Entries  []Entry
    Warnings []ParseWarning
}

// ParseWarning represents a non-fatal parsing issue.
type ParseWarning struct {
    Line    int
    Message string
}

// ParseJSONL reads JSONL from the provided reader and returns parsed entries.
// Maps to requirements 1.1, 1.6, 1.7, 1.8
func ParseJSONL(r io.Reader) (*ParseResult, error)

// ParseFirstTimestamp reads only the first entry's timestamp from JSONL.
// Used for efficient session listing without parsing entire file.
// Maps to requirement 3.4
func ParseFirstTimestamp(r io.Reader) (time.Time, error)
```

**Line Number Tracking Implementation:**

Since `bufio.Scanner` doesn't provide line numbers, we track them manually:

```go
func ParseJSONL(r io.Reader) (*ParseResult, error) {
    scanner := bufio.NewScanner(r)
    buf := make([]byte, 0, 64*1024)
    scanner.Buffer(buf, 10*1024*1024)

    result := &ParseResult{}
    lineNum := 0

    for scanner.Scan() {
        lineNum++
        line := scanner.Bytes()
        if len(line) == 0 {
            continue
        }

        var entry Entry
        if err := json.Unmarshal(line, &entry); err != nil {
            result.Warnings = append(result.Warnings, ParseWarning{
                Line:    lineNum,
                Message: fmt.Sprintf("failed to parse JSON: %v", err),
            })
            continue
        }
        // ... process entry
    }

    if err := scanner.Err(); err != nil {
        if errors.Is(err, bufio.ErrTooLong) {
            result.Warnings = append(result.Warnings, ParseWarning{
                Line:    lineNum + 1,
                Message: "line exceeds 10MB buffer limit",
            })
        } else {
            return nil, fmt.Errorf("scanner error: %w", err)
        }
    }

    return result, nil
}
```

#### 1.3 markdown.go

```go
// RenderMarkdown converts parsed entries to Markdown format.
// Maps to requirements 1.2, 1.3, 1.9
func RenderMarkdown(entries []Entry, opts RenderOptions) string

// Internal helper functions (unexported):
// - formatUserMessage(entry *Entry) string
// - formatAssistantMessage(entry *Entry) string
// - truncateString(s string, maxRunes int) string
```

**UTF-8 Safe Truncation Implementation:**

```go
// truncateString truncates a string to maxRunes, preserving UTF-8 boundaries.
func truncateString(s string, maxRunes int) string {
    if utf8.RuneCountInString(s) <= maxRunes {
        return s
    }

    var b strings.Builder
    n := 0
    for _, r := range s {
        if n >= maxRunes {
            break
        }
        b.WriteRune(r)
        n++
    }
    return b.String() + "\n... (truncated)"
}
```

This is an improvement over the existing byte-based truncation which could produce invalid UTF-8.

### 2. Package: `cmd/apsis`

#### 2.1 Version Injection

```go
// Set at build time via -ldflags
var version = "dev"
```

**Makefile target:**
```makefile
VERSION := $(shell git describe --tags --always 2>/dev/null || echo "dev")

build-apsis:
	go build -ldflags="-X main.version=$(VERSION)" -o apsis ./cmd/apsis
```

#### 2.2 main.go Structure

```go
// Config holds CLI configuration
type Config struct {
    List       bool   // -l, --list
    Output     string // -o, --output
    Project    string // -p, --project
    Version    bool   // -v, --version
    Help       bool   // -h, --help
    Input      string // positional argument (session ID or file path)
}

// Main flow:
// 1. Parse flags
// 2. Determine input source (file/session ID/stdin)
// 3. Execute appropriate action (list/convert)
// 4. Handle errors and exit codes

func main()
func parseFlags() *Config
func run(cfg *Config) error
func listSessions(projectPath string) error
func convert(input io.Reader, output io.Writer, sessionID string) error
func resolveInput(arg string, projectPath string) (io.ReadCloser, string, error)
func isFilePath(arg string) bool
func buildClaudePath(projectPath string) string
```

#### 2.3 Path Normalization

Convert project paths to Claude's format (per Decision 11, preserves leading dash):

```go
// buildClaudePath converts a project path to Claude's projects directory format.
// Example: /Users/foo/project -> -Users-foo-project
// The leading dash is preserved to match Claude's directory structure.
func buildClaudePath(projectPath string) string {
    // Replace path separators with dashes (leading separator becomes leading dash)
    p := strings.ReplaceAll(projectPath, "/", "-")
    p = strings.ReplaceAll(p, "\\", "-")
    return p
}
```

#### 2.4 TTY Detection

```go
// isInputFromPipe returns true if stdin is receiving piped input.
func isInputFromPipe() bool {
    stat, _ := os.Stdin.Stat()
    return (stat.Mode() & os.ModeCharDevice) == 0
}
```

### 3. Session Discovery (for `--list`)

```go
// SessionInfo contains metadata about a session file.
type SessionInfo struct {
    ID        string
    CreatedAt time.Time
    Size      int64
}

// ListSessions returns all sessions for a project.
func ListSessions(claudeProjectDir string) ([]SessionInfo, error)

// FormatSessionList formats sessions for display.
// Output format: SESSION_ID<TAB>CREATED_AT<TAB>SIZE
func FormatSessionList(sessions []SessionInfo) string
```

---

## Data Models

### JSONL Entry Types

The Claude JSONL format contains these entry types:

| Type | Description | Processing |
|------|-------------|------------|
| `user` | User message | Render with "👤 User" heading |
| `assistant` | Assistant response | Render with "🤖 Assistant" heading |
| `queue-operation` | Internal Claude operation | Skip silently (req 4.7) |
| Unknown | Other types | Skip silently (req 4.7) |

### Content Item Types

| Type | Description | Rendering |
|------|-------------|-----------|
| `text` | Plain text | Render as-is |
| `thinking` | Thinking block | Wrap in `<details>` |
| `tool_use` | Tool invocation | "🔧 Tool:" + JSON input |
| `tool_result` | Tool response | "✅ Tool Result" or "❌ Tool Error" |
| Unknown | Other types | Skip silently (req 4.8) |

### Session List Output

```
SESSION_ID                              CREATED_AT                   SIZE
550e8400-e29b-41d4-a716-446655440000    2025-12-23T10:30:00+11:00    1.2 MB
7c9e6679-7425-40de-944b-e07fc1f90ae7    2025-12-22T15:45:00+11:00    856 KB
```

---

## Error Handling

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Error (invalid input, file not found, etc.) |

### Error Scenarios

| Scenario | Handling | Requirement |
|----------|----------|-------------|
| Session ID not found | Error with expected path | 4.3 |
| Malformed JSONL entry | Warning to stderr, skip, continue | 4.1, 4.2 |
| Empty JSONL file | "Session contains no entries", exit 0 | 4.4 |
| Unknown entry type | Skip silently | 4.7 |
| Unknown content type | Skip silently | 4.8 |
| Line exceeds 10MB | Error to stderr, skip, continue | 1.8 |
| Project not found | Error message | 3.10 |
| No sessions for project | "No sessions found..." message | 3.9 |
| TTY with no args | Show help, exit 1 | 2.5 |

### Warning Format

```
Warning: line 42: failed to parse JSON: unexpected end of JSON input
```

---

## Testing Strategy

### Unit Tests

#### transcript/parser_test.go

| Test Case | Requirement |
|-----------|-------------|
| Parse valid JSONL | 1.1 |
| Parse empty file returns empty slice | 4.4 |
| Skip malformed lines with warning | 4.1, 4.2 |
| Handle large buffer (near 10MB) | 1.7 |
| Parse first timestamp only | 3.4 |
| Skip unknown entry types | 4.7 |

#### transcript/markdown_test.go

| Test Case | Requirement |
|-----------|-------------|
| Render user message | 5.2 |
| Render assistant message | 5.3 |
| Render thinking block in details | 5.4 |
| Render tool use with JSON | 5.5 |
| Render tool result (success) | 5.6 |
| Render tool error | 5.6 |
| Truncate long tool input (>2000 chars) | 1.9 |
| Truncate long tool result (>3000 chars) | 1.9 |
| Custom title via RenderOptions | 1.4 |
| Horizontal rules between messages | 5.7 |

#### apsis/main_test.go

| Test Case | Requirement |
|-----------|-------------|
| Disambiguate file path (contains /) | 2.2 |
| Disambiguate file path (ends .jsonl) | 2.2 |
| Disambiguate file path (file exists) | 2.2 |
| Treat as session ID otherwise | 2.3 |
| Read from stdin when not TTY | 2.4 |
| Show help when TTY and no args | 2.5 |
| Output to stdout by default | 2.6 |
| Output to file with -o flag | 2.7 |
| Resolve Claude project path | 2.8, 2.10 |

### Integration Tests

| Test Case | Requirement |
|-----------|-------------|
| Convert real session file to Markdown | 1.3, 5.* |
| List sessions for project | 3.* |
| End-to-end stdin → stdout | 2.4, 2.6 |

### Orbit Compatibility Tests

| Test Case | Requirement |
|-----------|-------------|
| Orbit output similar to pre-refactor (manual verification) | 6.3 |
| Existing Orbit tests pass | 6.2 |

**Note**: Per Decision 13, output verification is manual rather than automated golden file comparison. The intentional improvements (UTF-8 truncation, path normalization) mean output will not be byte-identical.

### Test Data

Create `internal/transcript/testdata/` with:
- `valid.jsonl` - Normal session with various entry types
- `malformed.jsonl` - Contains invalid JSON lines
- `empty.jsonl` - Empty file
- `large_entry.jsonl` - Entry near 10MB limit
- `unknown_types.jsonl` - Contains unknown entry/content types

---

## File Structure

```
orbit/
├── cmd/
│   ├── orbit/
│   │   └── main.go              # Existing (unchanged)
│   └── apsis/
│       └── main.go              # NEW: apsis CLI entry point
├── internal/
│   ├── transcript/              # NEW: shared package
│   │   ├── types.go             # Entry, Message, ContentItem, RenderOptions
│   │   ├── parser.go            # ParseJSONL, ParseFirstTimestamp
│   │   ├── parser_test.go
│   │   ├── markdown.go          # RenderMarkdown
│   │   ├── markdown_test.go
│   │   └── testdata/            # Test fixtures
│   │       ├── valid.jsonl
│   │       ├── malformed.jsonl
│   │       └── ...
│   └── logs/
│       └── manager.go           # MODIFIED: import transcript package
└── Makefile                     # MODIFIED: add build targets
```

---

## Makefile Changes

```makefile
.PHONY: build build-orbit build-apsis test lint clean install modernize

# Build both binaries
build: build-orbit build-apsis

# Build orbit only
build-orbit:
	go build -o orbit ./cmd/orbit

# Build apsis only
build-apsis:
	go build -o apsis ./cmd/apsis

# Run all tests (includes transcript package)
test:
	go test ./...

# ... (existing targets)

# Clean build artifacts
clean:
	rm -f orbit apsis coverage.out coverage.html

# Install both binaries
install:
	go install ./cmd/orbit
	go install ./cmd/apsis
```

---

## Migration Strategy for internal/logs/manager.go

### Step 1: Extract Types

Move from `manager.go` to `transcript/types.go`:
- `transcriptEntry` → `Entry` (exported)
- `transcriptMsg` → `Message` (exported)
- `contentItem` → `ContentItem` (exported)

### Step 2: Extract Parser

Move from `manager.go` to `transcript/parser.go`:
- Buffer setup logic (64KB initial, 10MB max)
- JSONL scanning loop
- Add warning collection for malformed entries

### Step 3: Extract Renderer

Move from `manager.go` to `transcript/markdown.go`:
- `formatUserMessage` → keep as internal helper
- `formatAssistantMessage` → keep as internal helper
- Create `RenderMarkdown` as public function
- Add `RenderOptions` for configurable title

### Step 4: Update manager.go

```go
import "github.com/arjenschwarz/orbit/internal/transcript"

func (m *Manager) generateMarkdownTranscript(srcPath, dstPath string, phase int, sessionID string) error {
    src, err := os.Open(srcPath)
    if err != nil {
        return fmt.Errorf("failed to open transcript: %w", err)
    }
    defer src.Close()

    result, err := transcript.ParseJSONL(src)
    if err != nil {
        return err
    }

    // Log warnings
    for _, w := range result.Warnings {
        fmt.Fprintf(os.Stderr, "Warning: line %d: %s\n", w.Line, w.Message)
    }

    opts := transcript.RenderOptions{
        Title:     fmt.Sprintf("Phase %d Session Transcript", phase),
        SessionID: sessionID,
    }
    markdown := transcript.RenderMarkdown(result.Entries, opts)

    return os.WriteFile(dstPath, []byte(markdown), 0644)
}
```

---

## CLI Usage Examples

```bash
# Convert session by ID (uses current project)
apsis 550e8400-e29b-41d4-a716-446655440000

# Convert session from different project
apsis -p /path/to/project 550e8400-e29b-41d4-a716-446655440000

# Convert from file path
apsis /path/to/session.jsonl

# Convert from stdin
cat session.jsonl | apsis

# Save to file
apsis 550e8400-... -o transcript.md

# List sessions for current project
apsis --list

# List sessions for different project
apsis --list -p /path/to/project

# Show version
apsis --version

# Show help
apsis --help
```

---

## Requirements Traceability

| Requirement | Design Element |
|-------------|----------------|
| 1.1 | `transcript.ParseJSONL()` |
| 1.2 | `transcript.RenderMarkdown()` |
| 1.3 | Message body formatting in `markdown.go` (with UTF-8 improvements per Decision 12) |
| 1.4 | `RenderOptions.Title` |
| 1.5 | `Entry`, `Message`, `ContentItem` types |
| 1.6 | `io.Reader` parameter in `ParseJSONL()` |
| 1.7 | Buffer configuration (64KB initial, 10MB max) |
| 1.8 | Line tracking in parser + `ParseWarning` struct |
| 1.9 | `truncateString()` with rune-safe truncation |
| 2.1-2.3 | `isFilePath()` disambiguation logic |
| 2.4-2.5 | `isInputFromPipe()` TTY detection |
| 2.6-2.7 | Output to stdout/file handling |
| 2.8-2.10 | `buildClaudePath()` preserving leading dash (Decision 11) |
| 2.11-2.12 | Flag parsing with version injection |
| 3.1-3.10 | `ListSessions()`, `FormatSessionList()` |
| 4.1-4.8 | `ParseWarning`, error handling throughout |
| 5.1-5.8 | `markdown.go` formatting |
| 6.1-6.6 | `manager.go` refactoring |
| 7.1-7.6 | Makefile targets with version injection |
