# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Spinner and hyperlink integration in Orbit orchestration loop:
  - Spinner displays during phase execution with phase number and elapsed time
  - Spinner shows wait countdown during retry delays (rate limit, connection errors)
  - Post-completion command displays "Post-completion" spinner
  - OSC 8 terminal hyperlinks printed on completion (success or failure) with paths to index.md and index.html
  - Graceful shutdown via `signal.NotifyContext` for SIGINT/SIGTERM handling
  - `Close()` method on Orbit struct for resource cleanup (called via defer in main)
- `internal/display` package for terminal display functionality:
  - `hyperlink.go` with OSC 8 terminal hyperlink support:
    - `FormatOSC8Link()` creates clickable terminal hyperlinks using OSC 8 escape sequences
    - `FormatFileLink()` creates properly percent-encoded file:// URIs
    - `PrintIndexLinks()` outputs labeled links for index.md and index.html to stderr
    - `IsTTY()` helper for TTY detection using mattn/go-isatty
  - `spinner.go` wrapping briandowns/spinner with orbit-specific behavior:
    - Braille character set with cyan color and 100ms update interval
    - `Start()` and `Stop()` methods with idempotency guards
    - `StartPostCompletion()` for post-completion command spinner
    - `UpdateWait()` and `ResumePhase()` for retry countdown display
    - `Pause()` and `Resume()` for log output coordination
    - Thread-safe via mutex, nil-safe method receivers
- briandowns/spinner dependency (v1.23.2) for terminal spinner animation
- Spec for Orbit UX improvements feature:
  - Requirements document with 5 sections covering completion links, progress indicator, terminal compatibility, integration, and demo command
  - Design document with architecture, components, interfaces, and testing strategy for spinner and hyperlink display
  - Decision log documenting 8 design decisions (OSC 8 hyperlinks, briandowns/spinner library, signal.NotifyContext, etc.)
  - Task list with 4 implementation phases: display package foundation, orbit integration, demo command, and integration tests

- Slash command rendering with description support:
  - Slash commands (e.g., `/catchup`) display with `⚡` icon
  - Descriptions from meta entries linked via `parentUuid` render in collapsible blocks
  - `parseSlashCommand()` and `isSlashCommandEntry()` helpers in grouping.go
  - `formatSlashCommand()` and `formatSlashCommandHTML()` for rendering
- Skill tool description support:
  - Meta entries with `sourceToolUseID` now provide descriptions for Skill tools
  - Descriptions render as collapsible content in the Skill block
  - `buildSkillDescriptionMap()` function to link descriptions via `sourceToolUseID` or `parentUuid`
  - `SourceToolUseID` field added to Entry struct
- TodoWrite tool checklist rendering:
  - Display todos as checklist with `[ ]` pending, `[-]` in progress, `[x]` completed
  - Expanded by default using `<details open>` attribute
  - Result section hidden as it provides no useful information
  - CSS styling for `.todo-list` class with monospace font
- Edit tool grouping and patch display:
  - Consecutive Edit calls grouped into single assistant block (like Read)
  - Show `structuredPatch` instead of cat output with color-coded additions/deletions
  - File path shown in summary: `🔧 Edit: <code>path/to/file</code>`
  - CSS styling for `.patch-content`, `.patch-line`, `.addition`, `.deletion`, `.context`
- Project-relative file paths in Read and Edit tool display:
  - Strip project directory prefix from file paths using `cwd` from JSONL entries
  - `stripProjectDir()` helper function in grouping.go
  - `Cwd` field added to Entry struct for working directory extraction
- `ToolUseResult` and `PatchHunk` structs for parsing Edit tool structured patches

### Fixed

- Edit tool error message preservation: when Edit operations fail or logs lack `structuredPatch`, the tool_result content (error message or legacy format) is now rendered as fallback instead of showing empty blocks
- Polymorphic `toolUseResult` field parsing: handles both string and object values in JSONL
  - Custom `UnmarshalJSON` method on Entry struct
  - Prevents parse warnings for string `toolUseResult` values
- Skill tool rendering simplified:
  - Result no longer rendered (just "Launching skill: X" which is redundant)
  - Without description, renders as simple non-collapsible block
- Meta entry filtering improved to preserve skill/command descriptions:
  - `hasNonCaveatTextContent()` helper distinguishes descriptions from "Caveat:" warnings
  - Meta entries with `sourceToolUseID` are preserved for skill descriptions
  - Meta entries with text content (not "Caveat:") are preserved for command descriptions
- Lint issues in logs/manager.go: converted if/else chain to tagged switch, removed unnecessary fmt.Sprintf

- Markdown-to-HTML conversion for HTML transcript renderer using goldmark library:
  - Assistant text messages render markdown formatting (headers, lists, code blocks, etc.)
  - Subagent results render as formatted HTML instead of raw text
  - Thinking blocks render with markdown formatting
  - Added `.markdown-content` CSS class with styling for all markdown elements
  - GitHub Flavored Markdown (GFM) support for tables, strikethrough, autolinks
- Subagent rendering with robot emoji prefix (🤖🔧) for Task tools with `subagent_type`:
  - Combined Prompt/Result display in single collapsible block
  - JSON array result parsing to extract text content from `[{"text":"..."}]` format
  - Deferred rendering pattern - tool_use stores metadata, renders at tool_result
- Combined tool call rendering for non-Task/Skill tools:
  - Tool input and result displayed together in single collapsible block
  - Tool-specific input formatting (Bash shows command, Write/Edit shows file path, etc.)
  - Empty assistant sections prevented when only deferred tools present
- Read tool grouping: consecutive Read-only entries combined into single assistant block
- Context-aware filtering of local command entries in parser:
  - Meta entries (`isMeta: true`) always filtered with UUID tracking
  - Command entries (`<command-name>`) filtered when parentUuid points to filtered meta
  - Local command stdout filtered when parentUuid points to filtered command
- `UUID` and `ParentUUID` fields added to `Entry` struct for context-aware filtering
- Distinct background colors for user and assistant messages in HTML output:
  - User messages: light blue (`#e7f1ff` light / `#1e3a5f` dark)
  - Assistant messages: light purple (`#f3e8ff` light / `#2d1f3d` dark)
- Thinking block styling with amber background and left border (no longer collapsible)
- `grouping.go` with entry preprocessing for Read tool grouping
- `extractSubagentResultText()` function for parsing subagent JSON array results

### Changed

- Removed text truncation from transcript rendering (content preserved in full within `<details>` blocks)
- Reduced HTML output spacing (margins, padding) for more compact layout
- Thinking blocks changed from collapsible `<details>` to always-visible div with distinct styling
- Updated backward compatibility tests to verify content is NOT truncated

### Fixed

- Subagent results no longer display as raw JSON arrays `[{"text":"..."}]`

- Collapsible `<details>` blocks for Task and Skill tools in Markdown and HTML transcript renderers:
  - Task tools always collapse with summary format "🔧 {subagent_type}: {description}"
  - Skill tools always collapse with summary format "🔧 Skill: {skill_name}"
  - Fallback summaries ("🔧 Task" or "🔧 Skill") when input fields are missing
  - Tool results inherit collapse behavior and summary from matching tool_use via ID linking
  - Cross-entry tool matching: tool_use in assistant entries links to tool_result in user entries
- CSS styles for `details.tool-collapsible` class in HTML renderer:
  - Styled summary with cursor pointer and flex layout
  - Error variant with distinct icon color for failed tool results
  - Consistent styling with existing theme variables
- Tests for collapsible HTML rendering:
  - Task/Skill tool collapsing with proper summaries
  - Short tools use uncollapsed `div.tool-use` format
  - Long tools (>500 runes) collapse automatically
  - Cross-entry tool result matching
  - Error results with `.error` class
  - CSS inclusion verification
  - Golden file integration tests for Task, Skill, long output, and short output scenarios
  - Backward compatibility tests for old JSONL without ID fields, truncation preservation, and pre-truncation collapse decisions
- Threshold-based collapsing for non-Task/Skill tools exceeding 500 runes (JSON-serialized)
  - Zero-length and nil inputs never collapse
  - Tools at or below threshold use standard heading format
- Test data for collapsible blocks in `internal/transcript/testdata/collapsible/`:
  - JSONL samples for Task tool, Skill tool, long output, and short output scenarios
  - Golden files (.md.golden) for expected Markdown output
- Tests for collapsible Markdown rendering:
  - Task/Skill always-collapse behavior
  - Fallback summary generation
  - Case-sensitive tool name matching
  - Threshold boundary conditions
  - Cross-entry tool result matching
  - Output format verification (details structure and heading format)
  - Golden file integration tests for Task, Skill, long output, and short output scenarios
  - Backward compatibility tests for old JSONL without ID fields, truncation preservation, and pre-truncation collapse decisions
- Helper functions for collapsible tool blocks in transcript renderer:
  - `CollapseThresholdRunes` constant (500 runes) for threshold-based collapsing
  - `toolMetadata` struct to store tool name and summary for result matching
  - `getToolSummary()` to extract readable summaries from Task and Skill tool inputs
  - `shouldCollapse()` to determine if a tool should be wrapped in `<details>` element
  - `escapeSummary()` to escape summary text for safe HTML inclusion
- Tests for collapsible helper functions:
  - Task tool summary extraction with full and partial fields
  - Skill tool summary extraction
  - Case-sensitive tool name matching (Task/Skill vs task/TASK)
  - Threshold boundary tests (499, 500, 501 runes)
  - XSS prevention through HTML escaping
- `ID` and `ToolUseID` fields to `ContentItem` struct for tool_use/tool_result linking:
  - `ID` field with JSON tag `id` for tool_use blocks
  - `ToolUseID` field with JSON tag `tool_use_id` for tool_result blocks
  - Updated `UnmarshalJSON` to parse new fields
  - Parser tests for ID field extraction and backward compatibility with older JSONL files
- Custom `UnmarshalJSON` methods for polymorphic content handling in transcript parser:
  - `Message.UnmarshalJSON` handles content as either string (user messages) or array (assistant messages)
  - `ContentItem.UnmarshalJSON` handles tool result content as either string or array of content blocks
- Tests for polymorphic content parsing: string content in user messages, array content in tool results, mixed formats
- HTML export format for session transcripts:
  - `RenderHTML()` function in `internal/transcript/html.go` with embedded CSS
  - Dark mode support via `prefers-color-scheme` media query
  - Responsive layout with mobile-friendly viewport settings
  - Collapsible thinking blocks using `<details>/<summary>` elements
  - Styled tool use and result blocks with success/error indicators
  - Proper HTML escaping to prevent XSS vulnerabilities
- `--format`/`-f` flag for apsis CLI to select output format (`md`, `markdown`, or `html`)
- Automatic HTML transcript generation in Orbit's log manager alongside Markdown files
- `apsis` CLI tool for converting Claude Code session transcripts to Markdown:
  - Flag parsing with `-l/--list`, `-o/--output`, `-p/--project`, `-v/--version`, `-h/--help`
  - Input source resolution: file path, session ID, or stdin
  - Session discovery via `--list` flag showing ID, creation date, and size
  - Conversion using shared `internal/transcript` package
  - Path normalization that preserves leading separator to match Claude's directory structure
  - Human-readable file size formatting
- Makefile targets for building apsis:
  - `VERSION` variable from `git describe --tags --always` for version injection
  - `build-orbit` to build only orbit binary
  - `build-apsis` to build only apsis binary with ldflags for version injection
  - `build` now builds both binaries
  - `install` now installs both binaries
  - `clean` now removes both binaries
- `internal/transcript` package with JSONL parsing and Markdown rendering:
  - `ParseJSONL()` function to parse Claude session JSONL with warning collection for malformed lines
  - `ParseFirstTimestamp()` function for efficient session timestamp extraction
  - `RenderMarkdown()` function to convert parsed entries to readable Markdown
  - UTF-8 safe string truncation at rune boundaries (fixes invalid UTF-8 from byte-based truncation)
  - Configurable document title via `RenderOptions`
  - 64KB initial buffer with 10MB max per line for large session files
  - Exported `Entry`, `Message`, `ContentItem`, and `RenderOptions` types
- Apsis feature spec with requirements, design, decisions, and tasks for:
  - Standalone CLI tool to convert Claude Code session transcripts to Markdown
  - Shared `internal/transcript` package for parsing and rendering
  - Session discovery via `--list` flag
  - Input from file path, session ID, or stdin
  - UTF-8 safe truncation and path normalization improvements
- `DateSubdirs` and `ContinueSession` configuration fields in `orbit.Config` for session management
- `isSessionInvalidError` function to detect session-related errors (session not found, invalid session, session expired)
- Tests for `isSessionInvalidError` covering detection of various session error messages
- Session ID support in Claude client's `RunPhase` method with `--session-id` and `--resume` flags
- `buildRunPhaseArgs` helper method for constructing Claude CLI arguments with session handling
- Tests for Claude client session parameter handling (new session, resume, skip permissions, arg order)
- Session management spec with requirements, design, decisions, and tasks for:
  - Flat log directory storage (default) with optional date-based subdirectories
  - Session ID generation and storage before Claude phase execution
  - Session continuation using `--resume` flag when resuming unfinished phases
- CLI flags `--date-subdirs` and `--no-continue-session` for controlling session behavior
- Environment variables `ORBIT_DATE_SUBDIRS` and `ORBIT_CONTINUE_SESSION` for session configuration
- `RunCustomPromptWithSession` method in Claude client for post-completion session tracking
- Log manager methods for session lifecycle: `StartPhase`, `CompletePhase`, `StartPostCompletion`, `CompletePostCompletion`
- Log manager methods for session ID reconciliation: `SetCurrentPhaseSessionID`, `ReconcileSessionID`, `SetPostCompletionSessionID`, `ReconcilePostCompletionSessionID`
- `PhaseState` and `PostCompletionState` types for tracking in-progress phases
- `ManagerOptions` type and `NewManagerWithOptions` constructor for configurable log directory modes
- Summary fields: `CurrentPhase`, `PostCompletion`, `RunNumber`, `BranchName` for crash recovery and session tracking
- Resume failure fallback: automatic retry with fresh session when `--resume` fails
- Branch mismatch warning when resuming in flat mode with a different branch
- Tests for session continuation, resume fallback, and log manager options
- Tests for config priority chain (`TestLoad_FullPriorityChain`, `TestLoad_PartialPriorityChain`, `TestLoad_EnvOverridesAllConfigs`, `TestLoad_EmptyEnvOverridesNonEmptyConfig`)
- Tests for CLI flag priority resolution (`TestResolveCommands`)
- Tests for post-command retry logic (`TestRunPostCommandWithRetry_*`, `TestRunPhaseWithRetry_*`)
- Tests for home config with empty post-command (`TestLoad_HomeEmptyPostCommand`, `TestLoad_HomeEmptyPostCommand_ProjectOmits`)
- `claudeRunner` interface in orbit package to support mocking in tests
- `resolveCommands()` helper function in main.go for testable CLI flag resolution
- Tests for post-completion session logging (SavePostCompletionSession and formatPostCompletionTranscript)
- UUID dependency (`github.com/google/uuid`) for session ID generation
- Viper dependency for configuration management (custom-commands feature)
- Phase overview table at startup showing all phases with status and task counts
- Full session transcript copying from `~/.claude/projects/{project-path}/{session-id}.jsonl`
- Markdown transcript generation with formatted user messages, assistant responses, thinking blocks, and tool usage
- Phase start and completion logging during orchestration
- `ListAll()` and `GetPhaseSummaries()` methods to rune client for phase statistics

### Changed

- Modified `formatAssistantMessageHTML` and `formatUserMessageHTML` signatures to accept `toolMeta map[string]toolMetadata` for cross-entry tool matching
- `formatUserMessageHTML` now handles `tool_result` blocks (previously only handled text)
- Extracted `formatToolUseHTML` and `formatToolResultHTML` functions from `formatAssistantMessageHTML` for cleaner separation of concerns
- `RenderHTML` now initializes and passes a render-level tool metadata map to enable tool_use/tool_result linking
- Modified `formatAssistantMessage` and `formatUserMessage` signatures to accept `toolMeta map[string]toolMetadata` for cross-entry tool matching
- `formatUserMessage` now handles `tool_result` blocks (previously only handled text)
- Extracted `formatToolUse` and `formatToolResult` functions from `formatAssistantMessage` for cleaner separation of concerns
- `RenderMarkdown` now initializes and passes a render-level tool metadata map to enable tool_use/tool_result linking
- Updated CLAUDE.md to document both Orbit and Apsis tools, including architecture details and Apsis workflow
- Added `/apsis` binary to `.gitignore`
- Refactored `internal/logs/manager.go` to use `internal/transcript` package for JSONL parsing and Markdown rendering
- `generateMarkdownTranscript` and `generatePostCompletionMarkdownTranscript` now use `transcript.ParseJSONL` and `transcript.RenderMarkdown`
- Phase transcripts now use `RenderOptions` with phase-specific titles ("Phase N Session Transcript" and "Post-Completion Session Transcript")
- Removed duplicate type definitions (`transcriptEntry`, `transcriptMsg`, `contentItem`) and formatting functions from logs package
- Moved format-related tests to `internal/transcript/markdown_test.go`
- `orbit.New` now uses `logs.NewManagerWithOptions` to support configurable log directory modes
- Updated `claudeRunner` interface signature: `RunPhase()` now requires `sessionID string` and `resume bool` parameters
- Orbit now generates UUID session IDs for each phase execution
- Simplified config environment variable handling by removing Viper's AutomaticEnv() in favor of os.LookupEnv for proper empty string detection
- Enhanced config.Load() documentation explaining the priority chain and design rationale
- Log directory now defaults to `.orbit` folder next to the tasks file instead of `.claude/orchestration-logs`
- Phase numbers now reflect actual phase order from tasks file instead of iteration counter
- Updated rune client to handle wrapper object format from rune CLI JSON output

### Fixed

- Path normalization in `copySessionTranscript` and `copyPostCompletionTranscript`: leading path separator is now correctly preserved to match Claude's directory structure (e.g., `/Users/foo/project` becomes `-Users-foo-project`)
- Added `BuildProjectPath` helper function in `internal/claude/paths.go` with proper Unix and Windows path handling
- Fixed meaningless `errors.Is(err, err)` assertion in TestRunPostCommandWithRetry_MaxRetriesExceeded
- Config priority bug: home config's explicit `post-command: ""` now correctly disables post-command
- Dry-run output now shows actual command value instead of "(default)"
- Linter errors (errcheck violations) in test files and logs/manager.go
- Error messages in config loading now include file paths for better debugging
- Test isolation issues with environment variables using `t.Setenv()`
- JSON parsing error when rune CLI returns wrapper object `{"Title": "...", "Tasks": [...]}`
- Empty user sections no longer appear in Markdown transcripts
- Correct handling of different JSON formats between `rune list` and `rune next --phase` commands
