# Decision Log: Apsis

## Decision 1: Tool Name

**Date**: 2025-12-23
**Status**: accepted

### Context

The project needed a name for the standalone CLI tool that converts Claude session transcripts to Markdown. The name should fit with the Orbit project theme.

### Decision

Use "apsis" as the tool name.

### Rationale

In orbital mechanics, apsis (plural: apsides) refers to the extreme points of an orbit - periapsis (closest) and apoapsis (furthest). This technical term fits the Orbit theme while being unique and memorable.

### Alternatives Considered

- **sessionmd**: Descriptive but generic - Rejected because it doesn't fit the space/orbit theme
- **signal**: Space communications theme - Rejected in favor of more technical orbital term
- **relay**: Data relay concept - Rejected as less distinctive
- **telemetry**: Accurate but longer - Rejected for brevity

### Consequences

**Positive:**
- Unique, memorable name
- Fits the Orbit project naming theme
- Short and easy to type

**Negative:**
- Less immediately descriptive of function
- Users may need to learn what "apsis" means

---

## Decision 2: Architecture - Same Repo with Shared Package

**Date**: 2025-12-23
**Status**: accepted

### Context

The session parsing and Markdown rendering functionality needed to be accessible both from Orbit (embedded) and as a standalone tool. Options included: keeping in Orbit only, separate repository, or shared package in same repo.

### Decision

Create a shared `internal/transcript` package in the same repository, with apsis as a separate command in `cmd/apsis/`.

### Rationale

This approach provides code reuse without premature API commitment. The internal package can evolve freely, and maintaining a single repository simplifies development. If external demand appears, the package can be promoted to a public module later.

### Alternatives Considered

- **Keep in Orbit only**: No standalone tool - Rejected because users want to convert sessions without running orchestration
- **Separate repository**: Independent tool - Rejected due to maintenance overhead and potential version drift
- **Public library**: Exported Go module - Rejected as premature; no external demand yet

### Consequences

**Positive:**
- Single repo simplifies maintenance
- Coordinated releases
- No premature API commitment
- Code reuse between Orbit and apsis

**Negative:**
- Both tools versioned together (may not always be desired)
- Slightly more complex build process

---

## Decision 3: Markdown Format Compatibility (Revised)

**Date**: 2025-12-23
**Status**: accepted

### Context

Apsis could either maintain exact compatibility with Orbit's current Markdown output or evolve independently. During review, it was identified that "identical output" is impossible because Orbit uses phase-specific headers (e.g., "Phase 1 Session Transcript") while apsis needs generic headers.

### Decision

Apsis and Orbit SHALL produce identical Markdown for message body content. Headers are context-specific and configurable via `RenderOptions.Title`. The shared transcript package provides consistent rendering for user messages, assistant messages, tool uses, and tool results.

### Rationale

Consistency in message rendering is valuable while allowing context-appropriate headers. This resolves the contradiction between "identical output" and different header needs.

### Alternatives Considered

- **Byte-for-byte identical including headers**: Impossible given different contexts - Rejected
- **Independent format**: Could diverge message rendering - Rejected because consistency is valuable

### Consequences

**Positive:**
- Consistent user experience for message content
- Context-appropriate headers for each tool
- Simpler testing for message body (compare excluding headers)

**Negative:**
- Headers differ between tools (intentional)

---

## Decision 4: Session List Functionality

**Date**: 2025-12-23
**Status**: accepted

### Context

The `--list` flag could provide varying levels of functionality, from basic listing to filtering by date or content search.

### Decision

Implement basic list only: show session ID, creation date, and file size. No filtering capabilities initially.

### Rationale

YAGNI - start simple and add features if needed. Basic listing covers the primary use case of finding a session ID.

### Alternatives Considered

- **Date filtering**: `--since`, `--until` flags - Rejected for initial version; can add later
- **Content search**: Search within sessions - Rejected as over-engineering for initial version

### Consequences

**Positive:**
- Simpler implementation
- Faster to deliver
- Can add filtering later if needed

**Negative:**
- Users with many sessions may need to scroll through list

---

## Decision 5: Error Handling for Malformed JSONL

**Date**: 2025-12-23
**Status**: accepted

### Context

Claude session JSONL files could be malformed or partially corrupted. The tool needs a strategy for handling parse errors.

### Decision

Skip malformed entries and emit warnings to stderr, continuing to process remaining entries.

### Rationale

This provides the best user experience - partial output is more useful than no output. Users are informed of issues via warnings but still get usable results.

### Alternatives Considered

- **Fail on any error**: Exit immediately - Rejected because it provides no value to user
- **Silent best effort**: No warnings - Rejected because users should know about data loss

### Consequences

**Positive:**
- Users get partial results even with some corruption
- Warnings inform users of issues
- Robust handling of real-world file issues

**Negative:**
- Users may not notice warnings in pipeline usage

---

## Decision 6: Stdin Support

**Date**: 2025-12-23
**Status**: accepted

### Context

Users may want to pipe JSONL content to apsis from other tools or processes.

### Decision

Support reading JSONL from stdin when no positional argument is provided and stdin is not a TTY.

### Rationale

This enables pipeline workflows and integration with other tools. Standard Unix convention.

### Alternatives Considered

- **File/session ID only**: No stdin support - Rejected because it limits flexibility

### Consequences

**Positive:**
- Enables pipeline workflows
- Standard Unix behavior
- Integration with other tools

**Negative:**
- Slightly more complex argument handling

---

## Decision 7: Future Output Formats

**Date**: 2025-12-23
**Status**: accepted

### Context

The shared transcript package could be designed for extensibility to support multiple output formats (HTML, JSON summary, etc.) or focus only on Markdown.

### Decision

Implement Markdown only for now. Do not design for extensibility.

### Rationale

YAGNI - there's no current need for other formats. Adding abstraction for hypothetical future formats adds complexity without benefit. The code can be refactored later if needed.

### Alternatives Considered

- **Design for extensibility**: Renderer interface, format plugins - Rejected as over-engineering

### Consequences

**Positive:**
- Simpler implementation
- Faster delivery
- Less code to maintain

**Negative:**
- Adding new formats later may require refactoring

---

## Decision 8: Session ID vs File Path Disambiguation

**Date**: 2025-12-23
**Status**: accepted

### Context

The CLI accepts a positional argument that could be either a session ID or a file path. A disambiguation rule is needed to determine which interpretation to use.

### Decision

Treat the argument as a file path IF it contains a path separator (`/` or `\`) OR ends with `.jsonl` OR a file exists at that path. Otherwise, treat it as a session ID.

### Rationale

This heuristic covers the common cases: explicit paths have separators, JSONL files have the extension, and existing files should be used directly. Session IDs (UUIDs) don't contain separators or the `.jsonl` extension.

### Alternatives Considered

- **Explicit flags**: `--session` vs `--file` - Rejected for being verbose; the heuristic handles most cases
- **UUID format detection**: Check if argument matches UUID pattern - Rejected because not all session IDs may be UUIDs

### Consequences

**Positive:**
- Intuitive behavior for common cases
- No extra flags needed
- File path takes precedence (explicit is better than implicit)

**Negative:**
- Edge case: UUID that happens to match an existing file name would be treated as file

---

## Decision 9: Session List Output Format

**Date**: 2025-12-23
**Status**: accepted

### Context

The `--list` flag needs a defined output format. Options include human-readable tables, tab-separated values, or JSON.

### Decision

Use tab-separated columns: SESSION_ID, CREATED_AT (RFC3339), SIZE (human-readable like "1.2 MB"). Sort by creation date, most recent first.

### Rationale

Tab-separated format is human-readable and easily parsed by tools like `cut`, `awk`, and `sort`. RFC3339 timestamps are unambiguous and sortable. Human-readable file sizes are more useful than raw bytes.

### Alternatives Considered

- **JSON output**: Machine-readable - Rejected for MVP; can add `--json` flag later
- **Aligned table**: Pretty but harder to parse - Rejected for simplicity

### Consequences

**Positive:**
- Human-readable
- Easy to parse with standard Unix tools
- Sortable timestamps

**Negative:**
- Not as pretty as aligned columns
- No machine-readable JSON option (can add later)

---

## Decision 10: Session Creation Date Source

**Date**: 2025-12-23
**Status**: accepted

### Context

The `--list` command needs to display session creation dates. The date could come from the file system (modification time) or from within the JSONL content (first entry timestamp).

### Decision

Parse the timestamp from the first entry in the JSONL file. If parsing fails, fall back to file modification time.

### Rationale

The first entry timestamp is more accurate (represents when the session actually started) than file modification time (which could change if the file is copied). Fallback ensures robustness.

### Alternatives Considered

- **File mtime only**: Faster but less accurate - Rejected for accuracy
- **First entry only, no fallback**: Fails on malformed files - Rejected for robustness

### Consequences

**Positive:**
- Accurate creation dates
- Robust fallback for edge cases

**Negative:**
- Slightly slower (must read first line of each file)

---

## Decision 11: Preserve Leading Dash in Path Conversion

**Date**: 2025-12-23
**Status**: accepted (revised 2025-12-26)

### Context

The implementation in `internal/logs/manager.go` converts project paths by replacing `/` with `-`. This results in `/Users/foo/project` becoming `-Users-foo-project`.

Initially this was considered a bug that needed fixing. However, investigation revealed that Claude's actual project path format includes the leading dash. The `-Users-foo-project` format matches how Claude stores sessions in `~/.claude/projects/`.

### Decision

Preserve the current behavior. The leading dash is an integral part of Claude's path format and must be kept for session lookup to work correctly.

### Rationale

Claude stores project sessions using paths with leading dashes. Removing the leading dash would break session lookup because the paths wouldn't match Claude's directory structure.

### Alternatives Considered

- **Remove leading separator**: Would produce "cleaner" paths like `Users-foo-project` - Rejected because it would break compatibility with Claude's actual directory structure

### Consequences

**Positive:**
- Session lookup works correctly with Claude's directory structure
- No behavioral change needed in existing code

**Negative:**
- Path strings have a leading dash (matches Claude's convention)

---

## Decision 12: UTF-8 Safe Truncation

**Date**: 2025-12-23
**Status**: accepted

### Context

The current truncation implementation uses byte slicing (`s[:2000]`), which can split UTF-8 multi-byte sequences and produce invalid UTF-8 output.

### Decision

Implement rune-based truncation that preserves UTF-8 character boundaries.

### Rationale

Producing valid UTF-8 output is more important than exact byte-position compatibility with the old implementation.

### Alternatives Considered

- **Keep byte-based truncation**: Preserve current behavior - Rejected because it produces corrupted output

### Consequences

**Positive:**
- Valid UTF-8 output
- No broken characters in truncated text

**Negative:**
- Truncation position may differ slightly from old implementation for non-ASCII content

---

## Decision 13: Similar Output vs Identical Output

**Date**: 2025-12-23
**Status**: accepted

### Context

Requirements originally specified "identical" Markdown output. During design review, the impracticality of byte-for-byte identical output was identified, given the improvements being made (UTF-8 truncation, path normalization).

### Decision

Change requirement from "identical" to "similar" output. Manual verification is sufficient for testing; golden file tests are not required.

### Rationale

The improvements (UTF-8 safe truncation, path normalization) are more valuable than strict byte-identical compatibility. Manual verification during development will ensure the output is visually and functionally similar.

### Alternatives Considered

- **Golden file testing**: Generate reference output before refactor, compare after - Rejected as overkill given the intentional improvements
- **Strict byte-identical**: No improvements allowed - Rejected because it perpetuates bugs

### Consequences

**Positive:**
- Allows quality improvements
- Simpler testing approach
- Faster development

**Negative:**
- Less automated verification of output compatibility

---
