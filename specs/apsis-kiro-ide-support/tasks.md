---
references:
    - specs/apsis-kiro-ide-support/requirements.md
    - specs/apsis-kiro-ide-support/design.md
    - specs/apsis-kiro-ide-support/decision_log.md
---
# Kiro IDE Support for Apsis

## Foundation Types

- [x] 1. Add FormatKiroIDE constant and ParseOptions type <!-- id:1jh0z60 -->
  - Add `FormatKiroIDE` to the `Format` enum in `internal/transcript/types.go`
  - Add `ParseOptions` struct with `KiroIDECostPath string` field in `internal/transcript/parser.go`
  - Update `ParseJSONLWithFormat()` signature to accept variadic `...ParseOptions`
  - Stream: 1
  - Requirements: [4.8](requirements.md#4.8), [8.6](requirements.md#8.6), [8.7](requirements.md#8.7)
  - References: internal/transcript/types.go, internal/transcript/parser.go

- [x] 2. Create Kiro IDE types <!-- id:1jh0z61 -->
  - Create `internal/transcript/kiro_ide_types.go`
  - Define `KiroIDEChatFile`, `KiroIDEMessage`, `KiroIDEMetadata` structs
  - Define `KiroIDEExecutionDetail`, `KiroIDEUsageSummary` structs for cost extraction
  - Note: `usage` field (not `value`) differs from Kiro CLI
  - Stream: 1
  - Requirements: [4.1](requirements.md#4.1)
  - References: internal/transcript/kiro_ide_types.go

## Path Resolution

- [x] 3. Write path resolution tests <!-- id:1jh0z62 -->
  - Create `internal/transcript/kiro_ide_path_test.go`
  - Write `TestSha256Hex32` with known input/output pairs including `KIRO::EXECUTION::SAVES` -> `414d1636299d2b9e4ce7e17fb11f63e9`
  - Write `TestKiroIDEBasePath` verifying subdirectory appended to `os.UserConfigDir()` result
  - Write `TestKiroIDEWorkspaceDir` for path normalization and non-existent directory handling
  - Write `TestKiroIDEExecutionDetailPath` verifying deterministic path construction
  - Write property-based test `TestPropertySha256Hex32` using `pgregory.net/rapid`: output always 32 hex chars, deterministic, same input same output
  - Blocked-by: 1jh0z61 (Create Kiro IDE types)
  - Stream: 1
  - Requirements: [1.1](requirements.md#1.1), [2.1](requirements.md#2.1), [2.2](requirements.md#2.2), [2.3](requirements.md#2.3), [2.4](requirements.md#2.4), [5.1](requirements.md#5.1)
  - References: internal/transcript/kiro_ide_path_test.go

- [x] 4. Implement path resolution functions <!-- id:1jh0z63 -->
  - Create `internal/transcript/kiro_ide_path.go`
  - Implement `sha256Hex32(input string) string` helper using crypto/sha256
  - Implement `KiroIDEBasePath() (string, error)` using `os.UserConfigDir()` + `Kiro/User/globalStorage/kiro.kiroagent/`
  - Implement `KiroIDEWorkspaceDir(projectPath string) (string, error)` with `filepath.Abs()` + `filepath.Clean()` normalization
  - Implement `KiroIDEExecutionDetailPath(workspaceDir, executionID string) string`
  - Define `executionSavesDir` constant (`414d1636299d2b9e4ce7e17fb11f63e9`)
  - Define `ErrKiroIDENotFound` sentinel error
  - Run tests to verify: `go test ./internal/transcript/ -run TestSha256Hex32 -run TestKiroIDE`
  - Blocked-by: 1jh0z62 (Write path resolution tests)
  - Stream: 1
  - Requirements: [1.1](requirements.md#1.1), [2.1](requirements.md#2.1), [2.2](requirements.md#2.2), [2.3](requirements.md#2.3), [2.4](requirements.md#2.4), [5.1](requirements.md#5.1)
  - References: internal/transcript/kiro_ide_path.go

## Parser

- [x] 5. Write parser unit tests <!-- id:1jh0z64 -->
  - Create `internal/transcript/kiro_ide_parser_test.go`
  - Write `TestParseKiroIDE` map-based table-driven tests with cases: basic conversation, system prompt filtering (<identity> prefix), tool messages, empty chat array, missing role, empty content messages, no identity prefix, only system prompt
  - Write `TestExtractKiroIDECost` tests: valid usage summary, mixed units, missing file, invalid json, empty usage summary
  - Write `TestConvertKiroIDEToEntries` tests: role mapping (human->user, bot->assistant, tool->user/tool_result), content preservation, warning generation
  - Use inline JSON strings following the pattern in `kiro_parser_test.go`
  - Use float tolerance for cost comparison (e.g., 0.0001)
  - Blocked-by: 1jh0z60 (Add FormatKiroIDE constant and ParseOptions type), 1jh0z61 (Create Kiro IDE types)
  - Stream: 1
  - Requirements: [4.1](requirements.md#4.1), [4.2](requirements.md#4.2), [4.3](requirements.md#4.3), [4.4](requirements.md#4.4), [4.5](requirements.md#4.5), [4.6](requirements.md#4.6), [4.10](requirements.md#4.10), [5.2](requirements.md#5.2), [5.3](requirements.md#5.3), [5.4](requirements.md#5.4)
  - References: internal/transcript/kiro_ide_parser_test.go, internal/transcript/kiro_parser_test.go

- [x] 6. Implement Kiro IDE parser <!-- id:1jh0z65 -->
  - Create `internal/transcript/kiro_ide_parser.go`
  - Implement `ParseKiroIDE(r io.Reader) (*ParseResult, error)` — parse JSON, convert entries, no cost
  - Implement `ParseKiroIDEWithCostPath(r io.Reader, executionDetailPath string) (*ParseResult, error)` — parse JSON, convert entries, extract cost
  - Implement `convertKiroIDEToEntries(chatFile *KiroIDEChatFile) ([]Entry, []ParseWarning)` with system prompt filtering, role mapping, empty content skipping, missing role warnings
  - Implement `extractKiroIDECost(path string) float64` — read execution detail file, sum `unit == credit` entries, return 0 on any error
  - Set `ParseResult.Metadata.CostUnit` to `credits` when cost > 0
  - Run tests: `go test ./internal/transcript/ -run TestParseKiroIDE -run TestExtractKiroIDECost -run TestConvertKiroIDE`
  - Blocked-by: 1jh0z63 (Implement path resolution functions), 1jh0z64 (Write parser unit tests)
  - Stream: 1
  - Requirements: [4.1](requirements.md#4.1), [4.2](requirements.md#4.2), [4.3](requirements.md#4.3), [4.4](requirements.md#4.4), [4.5](requirements.md#4.5), [4.6](requirements.md#4.6), [4.10](requirements.md#4.10), [5.1](requirements.md#5.1), [5.2](requirements.md#5.2), [5.3](requirements.md#5.3), [5.4](requirements.md#5.4)
  - References: internal/transcript/kiro_ide_parser.go, internal/transcript/kiro_parser.go

- [x] 7. Write format detection tests and implement <!-- id:1jh0z66 -->
  - Add `TestDetectKiroIDEFormat` to `internal/transcript/parser_test.go`
  - Test cases: .chat JSON with executionId+chat+metadata -> FormatKiroIDE, Kiro CLI JSON -> FormatKiro (no false positive), truncated .chat content -> FormatKiroIDE (string fallback), JSONL content -> not detected as Kiro IDE
  - Extend `detectKiroFormat()` in `parser.go` to check for executionId+chat+metadata after existing Kiro CLI check
  - Add string-based fallback for truncated buffers (check for executionId, chat, metadata field names)
  - Add `FormatKiroIDE` case to `ParseJSONLWithFormat()` dispatcher using `ParseOptions`
  - Run tests: `go test ./internal/transcript/ -run TestDetect`
  - Blocked-by: 1jh0z60 (Add FormatKiroIDE constant and ParseOptions type)
  - Stream: 1
  - Requirements: [4.7](requirements.md#4.7), [4.8](requirements.md#4.8), [4.9](requirements.md#4.9), [8.7](requirements.md#8.7)
  - References: internal/transcript/parser.go, internal/transcript/parser_test.go

## Integration

- [x] 8. Implement session discovery and listing <!-- id:1jh0z67 -->
  - Add `listKiroIDESessions(projectPath string) ([]SessionInfo, error)` to `cmd/apsis/main.go`
  - Define unexported `kiroIDEChatHeader` struct with `[]json.RawMessage` for lightweight parsing
  - Implement: compute workspace dir, scan .chat files, group by executionId, select representative file (most entries, tie-break by mtime then filename), build SessionInfo with source `kiro ide`
  - Use `os.Stat()` for file size
  - Return `nil, nil` on `ErrKiroIDENotFound` (graceful degradation)
  - Log warning to stderr for unparseable .chat files, skip and continue
  - Add `listKiroIDESessions()` call in `listAllSessions()`
  - Add `"kiro ide": 4` to `sourcePriority` map in `sortSessionsByTimestamp()`
  - Blocked-by: 1jh0z63 (Implement path resolution functions), 1jh0z65 (Implement Kiro IDE parser)
  - Stream: 2
  - Requirements: [1.1](requirements.md#1.1), [1.2](requirements.md#1.2), [1.3](requirements.md#1.3), [1.4](requirements.md#1.4), [1.5](requirements.md#1.5), [1.6](requirements.md#1.6), [1.7](requirements.md#1.7), [1.8](requirements.md#1.8), [1.9](requirements.md#1.9), [8.1](requirements.md#8.1), [8.4](requirements.md#8.4)
  - References: cmd/apsis/main.go

- [x] 9. Implement session resolution and cost threading <!-- id:1jh0z68 -->
  - Add `resolveKiroIDESession(sessionID, projectPath string) (io.ReadCloser, string, error)` to `cmd/apsis/main.go` — returns reader + cost path
  - Update `resolveInput()` signature to return cost path: `(io.ReadCloser, string, string, error)` — add Kiro IDE lookup after Kiro CLI with `ErrKiroIDENotFound` fall-through
  - Update `convert()` signature to accept `costPath string` parameter
  - Pass `costPath` through to `ParseJSONLWithFormat()` via `ParseOptions` when non-empty
  - Update call site in `run()` to handle the new return values
  - Add `.chat` suffix check to `isFilePath()`
  - Add `"kiro-ide": FormatKiroIDE` case to `agentToFormat()`
  - Add `kiro-ide` to agent format help text in `printUsage()`
  - Add `FormatKiroIDE` to JSON output handling in `convertToJSON()`
  - Blocked-by: 1jh0z67 (Implement session discovery and listing)
  - Stream: 2
  - Requirements: [3.1](requirements.md#3.1), [3.2](requirements.md#3.2), [3.3](requirements.md#3.3), [3.4](requirements.md#3.4), [5.1](requirements.md#5.1), [5.3](requirements.md#5.3), [7.1](requirements.md#7.1), [7.2](requirements.md#7.2), [8.2](requirements.md#8.2), [8.3](requirements.md#8.3), [8.5](requirements.md#8.5)
  - References: cmd/apsis/main.go

## Testing

- [x] 10. Write integration tests for session discovery and resolution <!-- id:1jh0z69 -->
  - Add `TestListKiroIDESessions` to `cmd/apsis/` tests using temp directory with mock .chat files: multiple files for same executionId -> single session with most messages, multiple executionIds -> multiple sessions, non-existent workspace dir -> empty list, malformed .chat -> skipped with warning, tie-breaking with same entry count
  - Add `TestResolveKiroIDESession` tests: valid executionId -> returns reader and cost path, unknown executionId -> not found, non-existent workspace dir -> not found
  - Add `TestCostPathIntegration`: set up temp workspace with .chat + execution detail file, resolve session, parse with cost path, verify ParseResult.Metadata.TotalCost matches expected
  - Blocked-by: 1jh0z67 (Implement session discovery and listing), 1jh0z68 (Implement session resolution and cost threading)
  - Stream: 2
  - Requirements: [1.1](requirements.md#1.1), [1.2](requirements.md#1.2), [1.4](requirements.md#1.4), [1.8](requirements.md#1.8), [1.9](requirements.md#1.9), [3.1](requirements.md#3.1), [3.2](requirements.md#3.2), [3.3](requirements.md#3.3), [3.4](requirements.md#3.4), [5.1](requirements.md#5.1), [5.2](requirements.md#5.2), [5.3](requirements.md#5.3)
  - References: cmd/apsis/main.go

## Verification

- [ ] 11. Run full test suite and lint <!-- id:1jh0z6a -->
  - Run `make test` to verify all tests pass including new and existing tests
  - Run `make lint` to check for lint issues
  - Run `make modernize` to verify modern Go idioms are used
  - Fix any issues found
  - Blocked-by: 1jh0z66 (Write format detection tests and implement), 1jh0z69 (Write integration tests for session discovery and resolution)
  - Stream: 1
