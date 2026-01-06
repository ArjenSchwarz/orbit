# Codex Log Format Support - Design Document

## Overview

This document describes the design for adding OpenAI Codex CLI session log support to Apsis. The implementation extends the existing transcript parsing pipeline with automatic format detection and a Codex-specific parser that normalizes entries to the existing `Entry` type, allowing full reuse of the Markdown and HTML rendering code.

### Design Goals

1. **Transparent operation** - No CLI changes required; format detected automatically
2. **Code reuse** - Normalize Codex entries to existing types; reuse all rendering code
3. **Maintainability** - Clear separation between format-specific parsing and shared rendering
4. **Forward compatibility** - Graceful handling of unknown Codex event types

### Requirements Coverage

| Requirement Section | Design Component |
|---------------------|------------------|
| 1. Format Detection | `DetectFormat()` function in parser.go |
| 2. Codex Session Discovery | `findCodexSession()` in main.go |
| 3. Session Listing | Extended `listSessions()` with source indicator |
| 4. Codex JSONL Parsing | `ParseCodexJSONL()` in codex_parser.go |
| 5. Content Type Mapping | Conversion functions in codex_parser.go |
| 6. Metadata Event Filtering | Filter logic in `ParseCodexJSONL()` |
| 7. Tool Name Display | Direct display in ContentItem.Name |
| 8. Output Compatibility | Existing `RenderMarkdown()` / `RenderHTML()` |
| 9. Error Handling | Warning collection in `ParseResult` |

---

## Architecture

### Component Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                         cmd/apsis/main.go                       │
├─────────────────────────────────────────────────────────────────┤
│  resolveInput()  ──┬──► findClaudeSession()                     │
│                    └──► findCodexSession()  [NEW]               │
│                                                                 │
│  listSessions()  ──┬──► listClaudeSessions()                    │
│                    └──► listCodexSessions()  [NEW]              │
│                                                                 │
│  convert()       ───────► transcript.ParseJSONL()               │
└─────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────┐
│                   internal/transcript/parser.go                  │
├─────────────────────────────────────────────────────────────────┤
│  ParseJSONL()    ──┬──► DetectFormat()  [NEW]                   │
│                    │                                             │
│                    ├──► parseClaudeJSONL()  (existing logic)    │
│                    │                                             │
│                    └──► ParseCodexJSONL()  [NEW]                │
│                              │                                   │
│                              ▼                                   │
│                    internal/transcript/codex_parser.go  [NEW]   │
└─────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────┐
│                   internal/transcript/types.go                   │
├─────────────────────────────────────────────────────────────────┤
│  Entry, Message, ContentItem  (unchanged)                       │
│                                                                 │
│  Format enum  [NEW]                                             │
│  CodexEntry, CodexResponseItem, etc.  [NEW - codex_types.go]   │
└─────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────┐
│              internal/transcript/markdown.go, html.go            │
├─────────────────────────────────────────────────────────────────┤
│  RenderMarkdown(), RenderHTML()  (unchanged)                    │
│  Works with normalized []Entry from either parser               │
└─────────────────────────────────────────────────────────────────┘
```

### Data Flow

```
Input JSONL
    │
    ▼
┌─────────────────┐
│ DetectFormat()  │ ◄── Reads first non-empty line
└────────┬────────┘
         │
    ┌────┴────┐
    │         │
    ▼         ▼
┌───────┐ ┌───────┐
│Claude │ │ Codex │
│Parser │ │Parser │
└───┬───┘ └───┬───┘
    │         │
    └────┬────┘
         │
         ▼
    []Entry (normalized)
         │
         ▼
┌─────────────────┐
│ RenderMarkdown  │
│ or RenderHTML   │
└─────────────────┘
         │
         ▼
    Output (MD/HTML)
```

---

## Components and Interfaces

### 1. Format Detection

**File:** `internal/transcript/parser.go`

```go
// Format represents the detected log format.
type Format int

const (
    FormatUnknown Format = iota
    FormatClaude
    FormatCodex
)

// DetectFormat examines the first non-empty line to determine the log format.
// Returns FormatUnknown if the format cannot be determined.
func DetectFormat(r io.Reader) (Format, []byte, error)
```

**Implementation Notes:**
- Reads lines until finding first non-empty line
- Handles UTF-8 BOM by stripping 3-byte prefix `\xEF\xBB\xBF`
- Returns the first line bytes for reuse (avoids re-reading)
- Parses JSON to extract `type` field value
- Maps type values to format:
  - `user`, `assistant` → `FormatClaude`
  - `session_meta`, `response_item`, `event_msg`, `turn_context` → `FormatCodex`

**Requirements:** [1.1](requirements.md#1.1), [1.2](requirements.md#1.2), [1.3](requirements.md#1.3), [1.4](requirements.md#1.4), [1.5](requirements.md#1.5), [1.6](requirements.md#1.6), [1.8](requirements.md#1.8)

### 2. Codex Types

**File:** `internal/transcript/codex_types.go`

```go
// CodexEntry represents a single line in Codex JSONL.
type CodexEntry struct {
    Timestamp string          `json:"timestamp"`
    Type      string          `json:"type"`    // session_meta, response_item, event_msg, turn_context
    Payload   json.RawMessage `json:"payload"`
}

// CodexResponseItem is the payload for response_item entries.
type CodexResponseItem struct {
    Type      string         `json:"type"`      // message, function_call, function_call_output, reasoning, ghost_snapshot
    Role      string         `json:"role,omitempty"`
    Name      string         `json:"name,omitempty"`
    Arguments string         `json:"arguments,omitempty"`
    CallID    string         `json:"call_id,omitempty"`
    Output    string         `json:"output,omitempty"`
    Content   []CodexContent `json:"content,omitempty"`
    Summary   []CodexSummary `json:"summary,omitempty"`
}

// CodexContent represents content items in Codex messages.
type CodexContent struct {
    Type string `json:"type"`  // input_text, output_text
    Text string `json:"text"`
}

// CodexSummary represents reasoning summary items.
type CodexSummary struct {
    Type string `json:"type"`  // summary_text
    Text string `json:"text"`
}

// CodexEventMsg is the payload for event_msg entries.
type CodexEventMsg struct {
    Type    string `json:"type"`    // agent_reasoning, agent_message, user_message, token_count
    Text    string `json:"text,omitempty"`
    Message string `json:"message,omitempty"`
}

// CodexSessionMeta is the payload for session_meta entries.
type CodexSessionMeta struct {
    ID        string `json:"id"`
    Timestamp string `json:"timestamp"`
    Cwd       string `json:"cwd"`
}
```

**Requirements:** [4.1](requirements.md#4.1)

### 3. Codex Parser

**File:** `internal/transcript/codex_parser.go`

```go
// ParseCodexJSONL parses Codex format JSONL and normalizes to []Entry.
// The reader should contain the complete JSONL content (format already detected).
func ParseCodexJSONL(r io.Reader) (*ParseResult, error)

// Internal helper functions:
func (p *codexParser) parseEntry(line []byte, lineNum int) error
func (p *codexParser) convertMessage(item *CodexResponseItem, timestamp string) *Entry
func (p *codexParser) convertFunctionCall(item *CodexResponseItem, timestamp string) *ContentItem
func (p *codexParser) convertFunctionCallOutput(item *CodexResponseItem, timestamp string) *ContentItem
func (p *codexParser) convertReasoning(item *CodexResponseItem, timestamp string) *ContentItem
func (p *codexParser) convertEventMsg(msg *CodexEventMsg, timestamp string) *Entry
func (p *codexParser) linkToolCalls() // Links function_call_output to function_call by call_id
func (p *codexParser) buildEntries() []Entry // Assembles final Entry list
```

**Parser State:**
```go
type codexParser struct {
    sessionID     string
    entries       []Entry
    warnings      []ParseWarning
    functionCalls map[string]*pendingCall  // call_id -> pending function_call
    currentEntry  *Entry                   // Current entry being built
}

type pendingCall struct {
    callID    string
    toolUse   *ContentItem
    timestamp string
}
```

**Conversion Flow:**
1. Parse `session_meta` first to extract `sessionID` and session timestamp
2. Process each line, building `Entry` objects
3. For `function_call`: Create `tool_use` ContentItem, store in `functionCalls` map
4. For `function_call_output`: Look up `call_id` in map, link as `tool_result`
   - **Important:** Do NOT delete entries from `functionCalls` map after linking (supports 1:N mapping for multiple outputs per call)
5. Skip metadata events (`session_meta`, `turn_context`, `token_count`, `user_message`, `ghost_snapshot`)
6. Build final `[]Entry` list with linked tool calls

**Multi-Output Tool Call Support:**
A single `function_call` may have multiple `function_call_output` entries (e.g., streaming output). The parser supports this by:
- Keeping entries in the `functionCalls` map after linking (no deletion)
- Each `function_call_output` creates a separate `tool_result` ContentItem
- All `tool_result` items reference the same `ToolUseID`
- Outputs are rendered in the order they appear in the JSONL file

**Entry Consolidation Logic:**
The Codex parser groups ContentItems into Entries based on role boundaries:

```go
// Entry consolidation rules:
// 1. Each user message creates a new Entry with Type: "user"
// 2. Each assistant message creates a new Entry with Type: "assistant"
// 3. Consecutive assistant events (function_call, reasoning, agent_reasoning, agent_message)
//    are consolidated into the SAME Entry by appending ContentItems
// 4. A new user message always starts a new Entry (breaks consolidation)
// 5. function_call_output is added to the Entry containing its matching function_call

func (p *codexParser) processEvent(entry *CodexEntry) {
    switch {
    case isUserMessage(entry):
        // Finalize current entry if exists, start new user entry
        p.finalizeCurrentEntry()
        p.currentEntry = newUserEntry(entry)

    case isAssistantEvent(entry):
        // If no current entry or current is user, start new assistant entry
        if p.currentEntry == nil || p.currentEntry.Type == "user" {
            p.finalizeCurrentEntry()
            p.currentEntry = newAssistantEntry(entry)
        }
        // Append content to current assistant entry
        p.currentEntry.Message.Content = append(
            p.currentEntry.Message.Content,
            convertContent(entry)...,
        )

    case isFunctionCallOutput(entry):
        // Find the entry containing the matching function_call and append there
        p.linkOutputToCall(entry)
    }
}
```

**Ordering Guarantees:**
- ContentItems within an Entry appear in JSONL file order
- Entries appear in JSONL file order
- Multi-output tool results appear consecutively after their tool_use

**Requirements:** [4.2](requirements.md#4.2) - [4.11](requirements.md#4.11), [5.1](requirements.md#5.1) - [5.7](requirements.md#5.7), [6.1](requirements.md#6.1) - [6.5](requirements.md#6.5)

### 4. Updated ParseJSONL

**File:** `internal/transcript/parser.go`

```go
// ParseJSONL reads JSONL from the provided reader and returns parsed entries.
// Automatically detects format (Claude or Codex) and delegates to appropriate parser.
// Preserves streaming architecture to avoid memory issues with large files.
func ParseJSONL(r io.Reader) (*ParseResult, error) {
    bufReader := bufio.NewReader(r)

    // Read first line for format detection (preserves streaming)
    firstLine, err := readFirstNonEmptyLine(bufReader)
    if err != nil {
        if err == io.EOF {
            return nil, fmt.Errorf("empty file")
        }
        return nil, fmt.Errorf("failed to read first line: %w", err)
    }

    // Strip BOM if present
    firstLine = stripBOM(firstLine)

    // Detect format from first line
    format, err := detectFormatFromLine(firstLine)
    if err != nil {
        return nil, err
    }

    // Combine first line with remaining content for streaming parse
    combined := io.MultiReader(bytes.NewReader(append(firstLine, '\n')), bufReader)

    // Delegate to appropriate parser
    switch format {
    case FormatCodex:
        return ParseCodexJSONL(combined)
    case FormatClaude:
        return parseClaudeJSONL(combined)
    default:
        return nil, fmt.Errorf("unrecognized log format")
    }
}

// readFirstNonEmptyLine reads lines until finding a non-empty line.
// Returns io.EOF if no non-empty line is found.
func readFirstNonEmptyLine(r *bufio.Reader) ([]byte, error) {
    for {
        line, err := r.ReadBytes('\n')
        if err != nil && err != io.EOF {
            return nil, err
        }
        line = bytes.TrimSpace(line)
        if len(line) > 0 {
            return line, nil
        }
        if err == io.EOF {
            return nil, io.EOF
        }
    }
}
```

**Design Note:** The streaming approach using `bufio.Reader` + `io.MultiReader` preserves the existing memory characteristics. The current parser uses a 64KB initial buffer with 10MB max per line; this design maintains that behavior rather than loading entire files into memory.

**Requirements:** [1.1](requirements.md#1.1) - [1.8](requirements.md#1.8)

### 5. Session Discovery Extension

**File:** `cmd/apsis/main.go`

```go
// SessionInfo extended with source indicator
type SessionInfo struct {
    ID        string
    CreatedAt time.Time
    Size      int64
    Source    string  // "claude" or "codex"
}

// Session Timestamp Extraction
//
// For Claude sessions: Use file modification time (existing behavior)
// For Codex sessions: Extract from first session_meta event's timestamp field
//
// Example session_meta:
// {"timestamp":"2026-01-05T00:22:15.725Z","type":"session_meta","payload":{...}}
//
// The timestamp is parsed as ISO 8601 and used for sorting in session listings.
func getCodexSessionTimestamp(path string) (time.Time, error) {
    f, err := os.Open(path)
    if err != nil {
        return time.Time{}, err
    }
    defer f.Close()

    scanner := bufio.NewScanner(f)
    if scanner.Scan() {
        var entry struct {
            Timestamp string `json:"timestamp"`
            Type      string `json:"type"`
        }
        if err := json.Unmarshal(scanner.Bytes(), &entry); err == nil {
            if entry.Type == "session_meta" {
                return time.Parse(time.RFC3339, entry.Timestamp)
            }
        }
    }
    // Fallback to file modification time if session_meta parsing fails
    info, err := os.Stat(path)
    if err != nil {
        return time.Time{}, err
    }
    return info.ModTime(), nil
}

// findCodexSession searches ~/.codex/sessions/ for a session by UUID.
func findCodexSession(homeDir, sessionID string) (string, error)

// listCodexSessions returns all Codex sessions.
// Uses walkDirFollowSymlinks for consistency with findCodexSession.
func listCodexSessions(homeDir string) ([]SessionInfo, error)

// Updated resolveInput to check both locations
func resolveInput(arg string, projectPath string) (io.ReadCloser, string, error) {
    // ... existing file path handling ...

    // Try Claude location first
    claudePath := findClaudeSession(homeDir, projectPath, arg)
    if claudePath != "" {
        return os.Open(claudePath)
    }

    // Try Codex location
    codexPath, err := findCodexSession(homeDir, arg)
    if err == nil && codexPath != "" {
        return os.Open(codexPath)
    }

    return nil, "", fmt.Errorf("session not found: %s", arg)
}

// Updated listSessions to merge both sources
func listSessions(projectPath string) error {
    // ... get Claude sessions ...
    // ... get Codex sessions ...
    // Merge and sort by timestamp (Claude first for ties)
    // Display with source indicator
}
```

**Codex Session Search Algorithm:**
```go
// uuidPattern matches standard UUID format: 8-4-4-4-12 hex digits (case-insensitive)
var uuidPattern = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

func findCodexSession(homeDir, sessionID string) (string, error) {
    // Validate sessionID is a proper UUID (36 chars with hyphens)
    if len(sessionID) != 36 || !uuidPattern.MatchString(sessionID) {
        return "", nil  // Not a valid UUID, skip Codex search
    }

    codexDir := filepath.Join(homeDir, ".codex", "sessions")

    // Check directory exists (resolve symlinks first)
    realDir, err := filepath.EvalSymlinks(codexDir)
    if err != nil {
        if os.IsNotExist(err) {
            return "", nil  // No error, just not found
        }
        return "", err
    }

    var foundPath string
    err = walkDirFollowSymlinks(realDir, func(path string, d fs.DirEntry, err error) error {
        if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
            return nil
        }

        // Extract UUID from filename and match exactly
        filename := filepath.Base(path)
        if match := uuidPattern.FindString(filename); match == sessionID {
            foundPath = path
            return filepath.SkipAll
        }
        return nil
    })

    return foundPath, err
}

// walkDirFollowSymlinks walks a directory tree, following symlinks with cycle detection.
// Unlike filepath.WalkDir, this resolves symlinks to directories while preventing
// infinite loops from circular symlinks.
func walkDirFollowSymlinks(root string, fn fs.WalkDirFunc) error {
    visited := make(map[string]bool)
    return walkDirFollowSymlinksInternal(root, fn, visited)
}

func walkDirFollowSymlinksInternal(root string, fn fs.WalkDirFunc, visited map[string]bool) error {
    // Resolve to absolute path for cycle detection
    absRoot, err := filepath.Abs(root)
    if err != nil {
        return err
    }

    // Check for cycles
    if visited[absRoot] {
        return nil // Already visited, skip to prevent infinite recursion
    }
    visited[absRoot] = true

    return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
        if err != nil {
            return fn(path, d, err)
        }

        // If it's a symlink, resolve it
        if d.Type()&fs.ModeSymlink != 0 {
            realPath, err := filepath.EvalSymlinks(path)
            if err != nil {
                // Broken symlink - log warning and continue
                return fn(path, d, err)
            }

            info, err := os.Stat(realPath)
            if err != nil {
                return fn(path, d, err)
            }

            // If symlink points to directory, walk it (with cycle detection)
            if info.IsDir() {
                absReal, _ := filepath.Abs(realPath)
                if visited[absReal] {
                    return nil // Cycle detected, skip
                }
                return walkDirFollowSymlinksInternal(realPath, fn, visited)
            }

            // If symlink points to file, call fn with resolved info
            return fn(realPath, fs.FileInfoToDirEntry(info), nil)
        }

        return fn(path, d, err)
    })
}
```

**Design Notes:**
- **UUID Matching:** Uses exact 36-character UUID match via case-insensitive regex, not `strings.Contains()`. This prevents false matches and handles both lowercase and uppercase UUIDs.
- **Symlink Handling:** Custom walker resolves symlinks explicitly using `filepath.EvalSymlinks()` with cycle detection via visited path tracking. This supports users who symlink their session directories while preventing infinite loops.
- **Consistency:** Both `findCodexSession` and `listCodexSessions` use `walkDirFollowSymlinks` to ensure consistent behavior.

**Requirements:** [2.1](requirements.md#2.1) - [2.8](requirements.md#2.8), [3.1](requirements.md#3.1) - [3.6](requirements.md#3.6)

---

## Data Models

### Entry Normalization

The Codex parser normalizes all events to the existing `Entry` and `ContentItem` types:

```
┌─────────────────────────────────────────────────────────────────┐
│                        Entry (normalized)                        │
├─────────────────────────────────────────────────────────────────┤
│  Type: "user" | "assistant"                                     │
│  Timestamp: ISO 8601 from Codex event                          │
│  SessionID: from session_meta.payload.id                        │
│  Message:                                                       │
│    └── Content: []ContentItem                                   │
│          ├── {Type: "text", Text: "..."}                       │
│          ├── {Type: "thinking", Thinking: "..."}               │
│          ├── {Type: "tool_use", ID, Name, Input}               │
│          └── {Type: "tool_result", ToolUseID, Content}         │
└─────────────────────────────────────────────────────────────────┘
```

### Codex Event to Entry Mapping

| Codex Event | Entry.Type | ContentItem Handling |
|-------------|------------|---------------------|
| `response_item` (role=user) | `"user"` | `input_text` → `text` |
| `response_item` (role=assistant) | `"assistant"` | `output_text` → `text` |
| `response_item` (function_call) | `"assistant"` | → `tool_use` |
| `response_item` (function_call_output) | `"assistant"` | → `tool_result` |
| `response_item` (reasoning) | `"assistant"` | → `thinking` |
| `event_msg` (agent_reasoning) | `"assistant"` | → `thinking` |
| `event_msg` (agent_message) | `"assistant"` | → `text` |

### Tool Call Linking

```
function_call (call_id: "abc123")          function_call_output (call_id: "abc123")
         │                                              │
         ▼                                              ▼
ContentItem {                              ContentItem {
    Type: "tool_use",                          Type: "tool_result",
    ID: "abc123",          ◄── linked by ──►   ToolUseID: "abc123",
    Name: "shell_command",                     Content: "Exit code: 0\n..."
    Input: {"command": "ls"}               }
}
```

---

## Error Handling

### Error Categories

| Category | Behavior | User Feedback |
|----------|----------|---------------|
| Empty file | Return error | "empty file" |
| Invalid JSON (first line) | Return error | "failed to parse first line as JSON" |
| Unrecognized format | Return error | "unrecognized log format: type field value '{value}'" |
| Malformed line (after first) | Warn, continue | "line N: failed to parse JSON: {error}" |
| Missing required field | Warn, skip entry | "line N: missing required field: {field}" |
| Unrecognized event type | Warn, skip entry | "line N: unrecognized event type: {type}" |
| Orphaned function_call_output | Warn, render standalone | "line N: no matching function_call for call_id: {id}" |
| All lines malformed | Return error | "no valid entries found in file" |

### Warning Collection

```go
type ParseResult struct {
    Entries  []Entry
    Warnings []ParseWarning
}

type ParseWarning struct {
    Line    int
    Message string
}
```

Warnings are written to stderr in format: `line N: {message}`

After parsing completes: `parsed with N warning(s)`

**Requirements:** [9.1](requirements.md#9.1) - [9.6](requirements.md#9.6)

---

## Testing Strategy

### Unit Tests

| Component | Test File | Key Test Cases |
|-----------|-----------|----------------|
| Format Detection | `parser_test.go` | Claude format, Codex format, empty file, invalid JSON, BOM handling, whitespace-only |
| Codex Types | `codex_types_test.go` | JSON unmarshaling for each type, missing fields, extra fields |
| Codex Parser | `codex_parser_test.go` | Message conversion, function call linking, reasoning extraction, event filtering, orphaned outputs |
| Session Discovery | `main_test.go` | UUID matching, directory traversal, empty directory, symlinks |

### Property-Based Testing Candidates

Format detection and entry normalization are good candidates for property-based testing:

**Property 1: Format Detection Idempotence**
```go
// For any valid JSONL content, DetectFormat should return the same result
// when called multiple times
func TestPropertyFormatDetectionIdempotent(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        content := generateValidJSONL(t)
        format1, _, _ := DetectFormat(bytes.NewReader(content))
        format2, _, _ := DetectFormat(bytes.NewReader(content))
        if format1 != format2 {
            t.Fatalf("format detection not idempotent: %v != %v", format1, format2)
        }
    })
}
```

**Property 2: Entry Normalization Preserves Content**
```go
// For any Codex entry with text content, the normalized Entry should
// contain the same text in some ContentItem
func TestPropertyTextPreserved(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        codexEntry := generateCodexTextEntry(t)
        entry := convertToEntry(codexEntry)
        originalText := extractText(codexEntry)
        normalizedText := extractText(entry)
        if originalText != normalizedText {
            t.Fatalf("text not preserved: %q != %q", originalText, normalizedText)
        }
    })
}
```

**Property 3: Tool Call Linking Invariant**
```go
// For any function_call_output with a valid call_id, the resulting
// tool_result should have ToolUseID == call_id
func TestPropertyToolLinkingCorrect(t *rapid.T) {
    rapid.Check(t, func(t *rapid.T) {
        callID := rapid.StringMatching(`call_[a-z0-9]{8}`).Draw(t, "callID")
        output := generateFunctionCallOutput(t, callID)
        result := convertToToolResult(output)
        if result.ToolUseID != callID {
            t.Fatalf("tool linking incorrect: %q != %q", result.ToolUseID, callID)
        }
    })
}
```

**Framework:** `pgregory.net/rapid` (per Go language rules)

### Integration Tests

| Test | Description |
|------|-------------|
| `TestCodexToMarkdown` | Full pipeline: Codex JSONL → ParseJSONL → RenderMarkdown |
| `TestCodexToHTML` | Full pipeline: Codex JSONL → ParseJSONL → RenderHTML |
| `TestMixedSessionListing` | List sessions from both Claude and Codex locations |
| `TestCodexSessionResolution` | Resolve session ID to Codex file path |

### Negative Test Cases

| Test | Input | Expected Behavior |
|------|-------|-------------------|
| `TestEmptyFile` | Zero-byte file | Error: "empty file" |
| `TestWhitespaceOnly` | File with only whitespace/newlines | Error: "empty file" |
| `TestInvalidFirstLineJSON` | `{not valid json}` | Error: "failed to parse first line as JSON" |
| `TestUnknownFormatType` | `{"type": "unknown_type"}` | Error: "unrecognized log format: type field value 'unknown_type'" |
| `TestBOMHandling` | UTF-8 BOM prefix + valid JSON | Success (BOM stripped) |
| `TestMalformedMiddleLine` | Valid first line, malformed middle | Warning logged, continues parsing |
| `TestAllLinesMalformed` | All lines invalid JSON (except first) | Error: "no valid entries found in file" |
| `TestOrphanedOutput` | function_call_output with no matching call | Warning logged, rendered standalone |
| `TestInvalidUUIDSearch` | Search with "abc123" (not valid UUID) | Returns not found (no false matches) |
| `TestSymlinkToMissing` | Symlink pointing to non-existent path | Handles gracefully, continues search |

### Golden File Tests

Golden file tests compare actual output against expected "golden" reference files. This ensures rendering consistency across changes.

**Structure:**
```
internal/transcript/testdata/
├── golden/
│   ├── codex_simple.md        # Expected Markdown output
│   ├── codex_simple.html      # Expected HTML output
│   ├── codex_with_tools.md
│   ├── codex_with_tools.html
│   ├── codex_reasoning.md
│   └── codex_reasoning.html
├── codex_simple.jsonl         # Input: simple user/assistant conversation
├── codex_with_tools.jsonl     # Input: conversation with shell_command calls
└── codex_reasoning.jsonl      # Input: conversation with reasoning blocks
```

**Test Implementation:**
```go
func TestGoldenMarkdown(t *testing.T) {
    testCases := []string{"codex_simple", "codex_with_tools", "codex_reasoning"}
    for _, tc := range testCases {
        t.Run(tc, func(t *testing.T) {
            input := readTestFile(t, tc+".jsonl")
            result, err := ParseJSONL(bytes.NewReader(input))
            require.NoError(t, err)

            actual := RenderMarkdown(result.Entries, DefaultOptions())
            golden := readTestFile(t, "golden/"+tc+".md")

            if *update {
                writeTestFile(t, "golden/"+tc+".md", actual)
            }
            assert.Equal(t, string(golden), actual)
        })
    }
}
```

**Update Flag:** Run with `-update` to regenerate golden files after intentional changes.

### Test Data

**File:** `internal/transcript/testdata/codex_valid.jsonl`
- Complete Codex session with all event types
- Multiple function calls with outputs
- Reasoning blocks with summaries
- agent_reasoning and agent_message events

**File:** `internal/transcript/testdata/codex_edge_cases.jsonl`
- Malformed lines (should warn and continue)
- Orphaned function_call_output (no matching call)
- Unknown event types (should skip with warning)
- Empty content arrays

**Requirements Coverage:**

| Requirement | Test Coverage |
|-------------|---------------|
| 1.1-1.8 | `TestDetectFormat*` |
| 2.1-2.8 | `TestFindCodexSession*` |
| 3.1-3.6 | `TestListSessions*` |
| 4.1-4.11 | `TestParseCodexJSONL*` |
| 5.1-5.7 | `TestConvert*` |
| 6.1-6.5 | `TestFilterMetadata*` |
| 7.1-7.3 | `TestToolNameDisplay*` |
| 8.1-8.4 | `TestCodexTo{Markdown,HTML}` |
| 9.1-9.6 | `TestErrorHandling*` |

---

## File Structure

```
internal/transcript/
├── parser.go            # Add DetectFormat(), update ParseJSONL()
├── codex_types.go       # NEW: Codex struct definitions
├── codex_parser.go      # NEW: ParseCodexJSONL() and conversion logic
├── types.go             # Add Format enum (minor change)
├── parser_test.go       # Add format detection tests
├── codex_parser_test.go # NEW: Codex parser tests
└── testdata/
    ├── valid.jsonl           # Existing Claude test data
    ├── codex_valid.jsonl     # NEW: Valid Codex test data
    └── codex_edge_cases.jsonl # NEW: Edge case test data

cmd/apsis/
├── main.go              # Add Codex session discovery
└── main_test.go         # Add Codex discovery tests
```

---

## Implementation Sequence

### Phase 1: Types and Format Detection
1. Add `Format` enum to `types.go`
2. Create `codex_types.go` with Codex structs
3. Implement `DetectFormat()` in `parser.go`
4. Write unit tests for format detection

### Phase 2: Codex Parser
1. Create `codex_parser.go` with `ParseCodexJSONL()`
2. Implement entry conversion functions
3. Implement tool call linking
4. Write unit tests for parsing

### Phase 3: Parser Integration
1. Update `ParseJSONL()` to detect and dispatch
2. Move existing Claude logic to `parseClaudeJSONL()`
3. Write integration tests

### Phase 4: Session Discovery
1. Add `findCodexSession()` to `main.go`
2. Update `resolveInput()` to check both locations
3. Extend `listSessions()` for unified listing
4. Write discovery tests

### Phase 5: Test Data and Documentation
1. Create `codex_valid.jsonl` test file
2. Create `codex_edge_cases.jsonl` test file
3. Run full test suite
4. Update README with Codex examples

---

## Appendix: Codex Log Sample

```json
{"timestamp":"2026-01-04T13:22:15.725Z","type":"session_meta","payload":{"id":"019b892c-3a14-7773-bd76-6465a8a0b634","cwd":"/Users/arjen/projects/orbit"}}
{"timestamp":"2026-01-04T13:22:15.725Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"List files"}]}}
{"timestamp":"2026-01-04T13:22:21.499Z","type":"event_msg","payload":{"type":"agent_reasoning","text":"**Preparing to list files**"}}
{"timestamp":"2026-01-04T13:22:21.885Z","type":"response_item","payload":{"type":"function_call","name":"shell_command","arguments":"{\"command\":\"ls\"}","call_id":"call_abc123"}}
{"timestamp":"2026-01-04T13:22:21.912Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_abc123","output":"Exit code: 0\nOutput:\nfile1.txt\nfile2.txt"}}
{"timestamp":"2026-01-04T13:22:56.849Z","type":"response_item","payload":{"type":"reasoning","summary":[{"type":"summary_text","text":"**Analyzing output**"}],"encrypted_content":"..."}}
{"timestamp":"2026-01-04T13:23:16.617Z","type":"event_msg","payload":{"type":"agent_message","message":"I found 2 files in the directory."}}
```
