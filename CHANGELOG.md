# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Session management spec with requirements, design, decisions, and tasks for:
  - Flat log directory storage (default) with optional date-based subdirectories
  - Session ID generation and storage before Claude phase execution
  - Session continuation using `--resume` flag when resuming unfinished phases
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

- Simplified config environment variable handling by removing Viper's AutomaticEnv() in favor of os.LookupEnv for proper empty string detection
- Enhanced config.Load() documentation explaining the priority chain and design rationale
- Log directory now defaults to `.orbit` folder next to the tasks file instead of `.claude/orchestration-logs`
- Phase numbers now reflect actual phase order from tasks file instead of iteration counter
- Updated rune client to handle wrapper object format from rune CLI JSON output

### Fixed

- Fixed meaningless `errors.Is(err, err)` assertion in TestRunPostCommandWithRetry_MaxRetriesExceeded
- Config priority bug: home config's explicit `post-command: ""` now correctly disables post-command
- Dry-run output now shows actual command value instead of "(default)"
- Linter errors (errcheck violations) in test files and logs/manager.go
- Error messages in config loading now include file paths for better debugging
- Test isolation issues with environment variables using `t.Setenv()`
- JSON parsing error when rune CLI returns wrapper object `{"Title": "...", "Tasks": [...]}`
- Empty user sections no longer appear in Markdown transcripts
- Correct handling of different JSON formats between `rune list` and `rune next --phase` commands
