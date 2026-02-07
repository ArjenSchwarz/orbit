# Requirements: Kiro IDE Support for Apsis

## Introduction

Apsis currently supports four session sources: Claude Code (JSONL), OpenAI Codex (JSONL), GitHub Copilot (JSONL), and Kiro CLI (SQLite). This feature adds support for **Kiro IDE** sessions — the IDE/editor variant of Kiro that stores sessions as JSON `.chat` files on the local filesystem.

Kiro IDE uses a different storage format from Kiro CLI: per-workspace directories identified by SHA-256 hashes of the workspace path, containing JSON `.chat` files for conversation data and execution detail files for action logs and cost data. This feature enables apsis to discover, list, and convert these sessions to Markdown/HTML, following the same patterns used by existing session sources.

The display label in `apsis -l` output will be `[kiro ide]`.

### Background: Kiro IDE Storage Model

Kiro IDE stores session data under a platform-specific base directory. Within that directory:

- **Workspace directories** are named as the first 32 hex characters of the SHA-256 hash of the absolute workspace path (e.g., `/Users/alice/myproject` → `55ea47a42687a8e8e76b24e2438997be/`).
- **`.chat` files** within a workspace directory are cumulative JSON snapshots of conversations. Multiple `.chat` files may share the same `executionId` — each is a snapshot at a different point in time, with the latest snapshot containing all previous messages plus new ones. The file with the most entries in its `chat` array is the complete transcript.
- **Execution detail files** are stored at a deterministic path: `{workspace_dir}/414d1636299d2b9e4ce7e17fb11f63e9/{sha256_32_of_execution_id}`. They contain action logs and usage/cost data. The intermediate directory `414d1636299d2b9e4ce7e17fb11f63e9` is `SHA-256("KIRO::EXECUTION::SAVES")[:32]`. This constant may change in future Kiro IDE versions.
- **`.chat` file format**: Each file is a JSON object with top-level fields `executionId` (UUID string), `chat` (array of `{role: string, content: string}` messages where role is `"human"`, `"bot"`, or `"tool"`), and `metadata` (object with `startTime` and `endTime` as milliseconds since Unix epoch, plus `modelId`, `modelProvider`, `workflow`, `workflowId`).

---

### 1. Session Discovery

**User Story:** As an apsis user, I want Kiro IDE sessions for my project to appear in `apsis -l`, so that I can find and convert them alongside sessions from other agents.

**Acceptance Criteria:**

1. <a name="1.1"></a>WHEN the user runs `apsis -l`, the system SHALL discover Kiro IDE sessions by normalizing the project path (via `filepath.Abs()` and `filepath.Clean()`), computing the SHA-256 hash (first 32 hex characters), and locating the workspace directory under the Kiro IDE storage base path
2. <a name="1.2"></a>The system SHALL scan `.chat` files in the workspace directory, group them by `executionId`, and select the file with the most `chat` array entries per `executionId` as the representative session file
3. <a name="1.3"></a>The system SHALL display each session with the source label `kiro ide` (shown as `[kiro ide]` in list output)
4. <a name="1.4"></a>The system SHALL use the `executionId` field from the `.chat` file as the session ID
5. <a name="1.5"></a>The system SHALL extract the session timestamp from the representative `.chat` file's `metadata.startTime` field (milliseconds since Unix epoch), falling back to the file's modification time if `startTime` is missing or zero
6. <a name="1.6"></a>The system SHALL use the file size of the representative `.chat` file as the session size
7. <a name="1.7"></a>The system SHALL sort `kiro ide` sessions with a priority of 4 in the tie-breaking order (after kiro-cli=3)
8. <a name="1.8"></a>IF the Kiro IDE storage directory does not exist, the system SHALL return an empty session list without error (graceful degradation)
9. <a name="1.9"></a>IF a `.chat` file cannot be parsed as valid JSON, the system SHALL skip that file and log a warning to stderr

---

### 2. Cross-Platform Storage Paths

**User Story:** As an apsis user on macOS, Linux, or Windows, I want Kiro IDE session discovery to work on my platform, so that I can use apsis regardless of my operating system.

**Acceptance Criteria:**

1. <a name="2.1"></a>On macOS, the system SHALL use `$HOME/Library/Application Support/Kiro/User/globalStorage/kiro.kiroagent/` as the Kiro IDE storage base path
2. <a name="2.2"></a>On Linux, the system SHALL use `$XDG_CONFIG_HOME/Kiro/User/globalStorage/kiro.kiroagent/` (falling back to `$HOME/.config/Kiro/User/globalStorage/kiro.kiroagent/` if `XDG_CONFIG_HOME` is not set) as the base path
3. <a name="2.3"></a>On Windows, the system SHALL use `%APPDATA%/Kiro/User/globalStorage/kiro.kiroagent/` as the base path
4. <a name="2.4"></a>The system SHALL resolve the platform-specific base path at runtime using Go's `os.UserConfigDir()` and appending `Kiro/User/globalStorage/kiro.kiroagent/`

---

### 3. Session Resolution by ID

**User Story:** As an apsis user, I want to convert a Kiro IDE session by providing its execution ID, so that I can generate a readable transcript without specifying the full file path.

**Acceptance Criteria:**

1. <a name="3.1"></a>WHEN the user provides a session ID argument to apsis, the system SHALL attempt to resolve it as a Kiro IDE session after checking Claude, Codex, Copilot, and Kiro CLI locations
2. <a name="3.2"></a>The system SHALL resolve a Kiro IDE session by normalizing the project path, computing SHA-256[:32] to find the workspace directory, then searching `.chat` files for a matching `executionId`
3. <a name="3.3"></a>WHEN a matching `.chat` file is found, the system SHALL return the file with the most `chat` entries for that `executionId` as the session content
4. <a name="3.4"></a>IF the session ID is not found in any source, the system SHALL return a "session not found" error

---

### 4. Transcript Parsing

**User Story:** As an apsis user, I want Kiro IDE sessions converted to the same readable Markdown/HTML format as other agents, so that I can review what the agent did in a consistent way.

**Acceptance Criteria:**

1. <a name="4.1"></a>The system SHALL parse `.chat` JSON files containing a top-level `chat` array of message objects, where each message has a `role` field (string) and a `content` field (string)
2. <a name="4.2"></a>The system SHALL map `role: "human"` messages to user entries in the common `Entry` format
3. <a name="4.3"></a>The system SHALL map `role: "bot"` messages to assistant entries in the common `Entry` format
4. <a name="4.4"></a>The system SHALL map `role: "tool"` messages to tool result entries in the common `Entry` format
5. <a name="4.5"></a>IF the first entry in the `chat` array has `role: "human"` AND its `content` string starts with `<identity>`, that entry SHALL be excluded from the output
6. <a name="4.6"></a>IF a `.chat` file contains an empty `chat` array, the system SHALL produce an empty entry list without error
7. <a name="4.7"></a>The system SHALL detect the Kiro IDE format during auto-detection by checking for the presence of `executionId`, `chat` (array), and `metadata` fields in the top-level JSON object, which is distinct from Kiro CLI detection (which checks for `conversation_id` and `history` fields)
8. <a name="4.8"></a>The system SHALL register `FormatKiroIDE` as a distinct format constant, separate from the existing `FormatKiro` (Kiro CLI)
9. <a name="4.9"></a>WHEN the user specifies `-a kiro-ide` on the command line, the system SHALL force parsing with the Kiro IDE parser
10. <a name="4.10"></a>IF a message in the `chat` array is missing the `role` field, the system SHALL skip that message and include a parse warning

---

### 5. Cost Extraction from Execution Details

**User Story:** As an apsis user, I want to see the credit cost of a Kiro IDE session in the transcript header, so that I can track my usage.

**Acceptance Criteria:**

1. <a name="5.1"></a>WHEN parsing a Kiro IDE session, the system SHALL attempt to locate the corresponding execution detail file at `{workspace_dir}/414d1636299d2b9e4ce7e17fb11f63e9/{sha256_32_of_execution_id}` (see Background section for details on this path structure)
2. <a name="5.2"></a>IF the execution detail file exists and contains a `usageSummary` array, the system SHALL sum all entries where `unit == "credit"` to compute total cost
3. <a name="5.3"></a>The system SHALL include the total cost in `ParseResult.Metadata` with `CostUnit` set to `"credits"`
4. <a name="5.4"></a>IF the execution detail file does not exist or cannot be parsed, the system SHALL proceed without cost data (no error)

---

### 6. JSON Output Support

**User Story:** As an apsis user, I want to output Kiro IDE sessions as pretty-printed JSON, so that I can process them programmatically.

**Acceptance Criteria:**

1. <a name="6.1"></a>WHEN the user specifies `-f json`, the system SHALL output the `.chat` file content as pretty-printed JSON
2. <a name="6.2"></a>The system SHALL preserve the logical structure of the `.chat` file in JSON output mode (field ordering may differ from the source file)

---

### 7. File Path Recognition

**User Story:** As an apsis user, I want to pass a `.chat` file path directly to apsis, so that I can convert a specific session file without looking up its ID.

**Acceptance Criteria:**

1. <a name="7.1"></a>The system SHALL recognize arguments ending in `.chat` as file paths (updating the `isFilePath()` function)
2. <a name="7.2"></a>WHEN a `.chat` file path is provided, the system SHALL parse it using the Kiro IDE parser

---

### 8. Integration with Existing Architecture

**User Story:** As a developer maintaining apsis, I want Kiro IDE support to follow the same patterns as other session sources, so that the codebase remains consistent and maintainable.

**Acceptance Criteria:**

1. <a name="8.1"></a>The system SHALL add `listKiroIDESessions()` to the `listAllSessions()` aggregation in `cmd/apsis/main.go`
2. <a name="8.2"></a>The system SHALL add Kiro IDE to the `resolveInput()` lookup chain, after Kiro CLI
3. <a name="8.3"></a>The system SHALL NOT add Kiro IDE to the `resolveFollowInput()` chain (follow mode is not supported for this format)
4. <a name="8.4"></a>The system SHALL add `"kiro ide"` to the `sourcePriority` map with priority value 4 (kiro-cli remains at priority 3)
5. <a name="8.5"></a>The system SHALL add `"kiro-ide"` as a valid value for the `-a/--agent` flag
6. <a name="8.6"></a>The system SHALL add `FormatKiroIDE` to the `Format` enum in `internal/transcript/types.go`
7. <a name="8.7"></a>The system SHALL add a `ParseKiroIDE()` function and case in `ParseJSONLWithFormat()` dispatcher in `internal/transcript/parser.go`
