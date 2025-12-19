# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Viper dependency for configuration management (custom-commands feature)
- Phase overview table at startup showing all phases with status and task counts
- Full session transcript copying from `~/.claude/projects/{project-path}/{session-id}.jsonl`
- Markdown transcript generation with formatted user messages, assistant responses, thinking blocks, and tool usage
- Phase start and completion logging during orchestration
- `ListAll()` and `GetPhaseSummaries()` methods to rune client for phase statistics

### Changed

- Log directory now defaults to `.orbit` folder next to the tasks file instead of `.claude/orchestration-logs`
- Phase numbers now reflect actual phase order from tasks file instead of iteration counter
- Updated rune client to handle wrapper object format from rune CLI JSON output

### Fixed

- JSON parsing error when rune CLI returns wrapper object `{"Title": "...", "Tasks": [...]}`
- Empty user sections no longer appear in Markdown transcripts
- Correct handling of different JSON formats between `rune list` and `rune next --phase` commands
