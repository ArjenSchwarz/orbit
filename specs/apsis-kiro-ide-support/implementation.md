# Implementation Explanation: Kiro IDE Session Support in Apsis

## Beginner Level

### What Changed
Apsis is a tool that reads transcripts from AI coding agents (like Claude Code, Codex, Copilot) and converts them into readable Markdown or HTML. Before this change, Apsis could read sessions from Kiro's CLI tool (which stores data in a SQLite database), but not from Kiro running inside an IDE (like VS Code). This change adds support for reading those IDE sessions too.

Kiro IDE stores its sessions as `.chat` files — JSON files containing a back-and-forth conversation between the user and the AI. These files live in a specific directory on your computer, organized by a hash of the project path.

### Why It Matters
Without this change, if you used Kiro through your IDE, there was no way to convert your session transcripts to readable formats. Now you can run `apsis <executionId>` or `apsis session.chat` and get a formatted transcript, just like you can with Claude Code or Codex sessions.

### Key Concepts
- **`.chat` file**: A JSON file that Kiro IDE writes to disk with the conversation history. Each file has an `executionId` (unique session identifier), a `chat` array (messages), and `metadata` (timestamps, model info).
- **Workspace directory**: Kiro IDE organizes session files by project. It takes your project path, hashes it with SHA-256, and uses the first 32 hex characters as a folder name.
- **Execution detail file**: A separate file that tracks how many credits the session used. Found at a deterministic path based on the executionId hash.
- **Format detection**: When Apsis receives a file, it peeks at the content to figure out which agent produced it. Kiro IDE files are identified by having `executionId`, `chat`, and `metadata` fields.

---

## Intermediate Level

### Changes Overview

8 commits across 13 new/modified Go files adding ~2900 lines (net). The implementation spans two packages:

- **`internal/transcript/`** — New files: `kiro_ide_types.go` (5 types), `kiro_ide_path.go` (path resolution), `kiro_ide_parser.go` (parsing + cost extraction). Modified: `types.go` (+FormatKiroIDE), `parser.go` (format detection + dispatch).
- **`cmd/apsis/`** — Modified `main.go`: session discovery (`listKiroIDESessions`), session resolution (`resolveKiroIDESession`), integration into existing lookup chains, cost path threading.

### Implementation Approach

**Layered architecture following existing patterns:**

1. **Types** (`kiro_ide_types.go`): Minimal structs matching `.chat` file schema. Only fields needed for transcript conversion are included — unused fields like `actionId`, `context`, `validations` are intentionally omitted.

2. **Path resolution** (`kiro_ide_path.go`): Three functions compute filesystem paths:
   - `KiroIDEBasePath()` → platform-specific base via `os.UserConfigDir()`
   - `KiroIDEWorkspaceDir(projectPath)` → base + SHA-256[:32] of normalized absolute path
   - `KiroIDEExecutionDetailPath(workspaceDir, executionID)` → workspace + magic hash + SHA-256[:32] of executionId

3. **Parser** (`kiro_ide_parser.go`): Role mapping (human→user, bot→assistant, tool→tool_result), system prompt filtering (first message starting with `<identity>`), and cost extraction from a separate execution detail file.

4. **Discovery** (`main.go:listKiroIDESessions`): Scans workspace directory, groups `.chat` files by `executionId`, selects the representative file per group (most entries, then newest mtime, then lexicographic filename as tie-breakers).

5. **Resolution** (`main.go:resolveKiroIDESession`): Similar grouping logic, but for a single executionId. Returns a reader + deterministic cost path.

6. **Format detection** (`parser.go:detectKiroFormat`): Extended to check for Kiro IDE markers (`executionId` + `chat` + `metadata`) after checking Kiro CLI markers. Both full-parse and string-based fallback for truncated content.

**Cost threading**: Cost data lives in a separate file from the session. The `ParseOptions` struct with variadic parameter on `ParseJSONLWithFormat()` threads the cost path through to the parser without breaking the existing API.

### Trade-offs

- **Separate cost file vs. embedding cost in ParseResult**: Cost requires reading a second file at a path derived from the workspace directory. This knowledge lives in the CLI layer (`cmd/apsis`), not the parser. The `ParseOptions` approach keeps the parser testable in isolation while allowing the CLI to thread context.
- **Reading entire `.chat` files for discovery**: Each `.chat` file is fully read and JSON-parsed during listing. This is acceptable because Kiro IDE session files are small (typically <1MB). A streaming approach would add complexity for negligible gain.
- **Lightweight header struct for discovery**: `kiroIDEChatHeader` uses `[]json.RawMessage` for the chat array to count entries without deserializing individual messages.

---

## Expert Level

### Technical Deep Dive

**Path hashing**: The workspace directory hash uses `SHA-256(filepath.Abs(filepath.Clean(path)))[:32]`. The `executionSavesDir` constant (`414d1636299d2b9e4ce7e17fb11f63e9`) is `SHA-256("KIRO::EXECUTION::SAVES")[:32]` — a magic constant from Kiro's source. Property-based tests via `pgregory.net/rapid` verify hash determinism and format invariants.

**File grouping logic**: Multiple `.chat` files can exist per `executionId` (Kiro IDE writes cumulative snapshots). Selection criteria: most entries > newest mtime > lexicographic filename. Both `listKiroIDESessions` and `resolveKiroIDESession` implement this independently — the listing version uses a map of candidates, the resolution version tracks a running best.

**Format detection ordering**: `detectKiroFormat()` checks Kiro CLI format first (must have `conversation_id` + non-empty `history`), then Kiro IDE format (`executionId` + `chat` + `metadata`). For truncated content (string-based fallback), the same ordering applies. This prevents false positives since the field names are distinct between formats.

**Cost extraction**: `extractKiroIDECost()` reads the execution detail file, sums all entries where `unit == "credit"`, and returns 0 for missing/malformed files (graceful degradation). The cost path is computed deterministically: `{workspaceDir}/{executionSavesHash}/{sha256_32(executionId)}`.

### Architecture Impact

- `FormatKiroIDE` is the 5th format constant (after Claude, Codex, Kiro, Copilot). The `ParseJSONLWithFormat` dispatch and `convertToJSON` both handle it.
- `resolveInput()` now returns 4 values (added `costPath`). This is format-specific threading — only Kiro IDE needs it because other formats embed cost data in the session file itself.
- Session sort priority: kiro-ide=4 (after kiro-cli=3), ensuring IDE sessions appear after CLI sessions in listings.
- No follow mode for Kiro IDE (decision 6): `.chat` files are cumulative snapshots, not append-only, so `tail -f` semantics don't apply.

### Potential Issues

- **HOME environment mutation in tests**: Integration tests (`setupKiroIDEWorkspace`) set `HOME` env var to a temp directory and restore it in cleanup. These tests cannot run in parallel with other tests that depend on `HOME` / `os.UserConfigDir()`. The `t.Cleanup` restoration mitigates but doesn't eliminate race risk under `-parallel`.
- **No validation that `.chat` file content matches its `executionId` filename**: The filename is arbitrary (Kiro uses UUIDs). Only the JSON content's `executionId` field matters. A renamed file still works correctly.
- **Workspace directory hash is platform-dependent**: `filepath.Abs()` produces different results on macOS vs. Linux vs. Windows for the same logical path, so workspace hashes are platform-specific. This is correct behavior (matches Kiro's own hashing) but means session data isn't portable across platforms.

## Completeness Assessment

### Fully Implemented
- **Requirement 1 (Session Discovery)**: All 7 acceptance criteria met — discovery, grouping by executionId, representative file selection, `[kiro ide]` label, executionId as session ID, timestamp extraction, graceful handling of missing directories.
- **Requirement 2 (Cross-Platform Paths)**: macOS, Linux, Windows paths via `os.UserConfigDir()` + hardcoded subpath. Path normalization with `filepath.Abs` + `filepath.Clean`.
- **Requirement 3 (Session Resolution by ID)**: Resolution by executionId with best-file selection. Returns `ErrKiroIDENotFound` for missing IDs.
- **Requirement 4 (Transcript Parsing)**: Role mapping, `<identity>` filtering, format detection (full and truncated), `-a kiro-ide` flag.
- **Requirement 5 (Cost Extraction)**: Execution detail file reading, credit summing, graceful degradation.
- **Requirement 6 (JSON Output)**: `.chat` files handled in `convertToJSON()` as native JSON (already valid JSON).
- **Requirement 7 (File Path Recognition)**: `.chat` extension added to `isFilePath()`.
- **Requirement 8 (Integration)**: Added to `listAllSessions()`, `resolveInput()` chain, priority 4, `-a` flag, `FormatKiroIDE` constant.

### No Gaps Identified
All 33 acceptance criteria from the requirements document are addressed. The implementation matches the design document's architecture and component specifications.
