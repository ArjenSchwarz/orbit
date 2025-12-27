# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

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
