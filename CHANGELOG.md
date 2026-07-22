# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `orbit finalize` now displays the variant's agent (alias, type, model) in its preamble, falling back to "Agent: unknown" when none of the fields are populated.
- `orbit finalize` now reads the spec's `consolidation-log.json` and prints a warning before the confirmation prompt when the requested `--variant` differs from the most recent consolidation entry. The warning fires under `--force` as well so it remains visible in CI output.

### Fixed

- Claude Code session-limit messages using the new `"You've hit your session limit"` wording now wait until the reported reset time while retaining support for the older message.

## [0.9.0] - 2026-04-11

Initial public release of Orbit and Apsis.

### Orbit - AI Coding Agent Orchestrator

#### Multi-Agent Support

- Support for 5 AI coding agents: Claude Code, OpenAI Codex, AWS Kiro, GitHub Copilot, and OpenCode
- Agent abstraction layer with pluggable agent implementations via registry pattern
- Per-agent error classification with appropriate retry strategies (rate limits, connection errors, session invalid, fatal)
- Shared retry executor with exponential backoff and context-aware interruptible sleep
- Claude Code usage limit detection: automatically waits until reset time and resumes execution
- Agent selection via `--agent` flag, `ORBIT_AGENT` environment variable, or `.orbit.yaml` config

#### Phase-Based Orchestration

- Sequential phase execution driven by rune task files with automatic task detection from git branch
- Session continuation across phases and crash recovery via session ID tracking
- Pre-prompt and post-prompt AI interactions for setup/review around the phase loop
- Agent-level pre-command and post-command shell hooks with configurable timeout
- Execution order: pre-command -> pre-prompt -> phases -> post-prompt -> post-command
- Dry-run mode for previewing execution without running agents

#### Multi-Variant Comparison

- `orbit run --variants N` creates git worktrees and runs parallel or sequential implementations
- `--variant-agents` assigns different agents to variants with cycling support
- `--guidance-file` provides per-variant YAML instructions
- Per-variant model selection via agent aliases in `.orbit.yaml`
- AI-powered comparison with structured JSON output, recommendation, and confidence level
- HTML and Markdown comparison reports with per-file analysis and learnings
- Cross-variant improvement identification for consolidation
- Comparison failure recovery: falls back to written JSON file if agent session fails

#### Consolidation and Finalization

- `orbit consolidate` merges improvements from non-chosen variants into the chosen one
- Auto-consolidation after comparison with `--auto-consolidate` flag
- Rollback support for failed consolidations
- `orbit finalize` adopts a variant via fast-forward merge and cleans up worktrees
- `orbit cleanup` removes all variant worktrees without adopting any
- Variant session recovery when re-running: continue, new run (preserving completed), or cancel

#### Status and Monitoring

- `orbit status` shows variant progress with recent commits, git state, last action, and task progress
- Text and JSON output formats
- `orbit serve` web interface with dashboard, run details, transcript viewer, and HTMX live updates
- Run registry at `~/.orbit/runs/` for tracking runs across repositories
- Centralized structured logging to `~/.orbit/logs/` in JSON Lines format

#### Configuration

- Layered configuration: CLI flags > environment variables > project `.orbit.yaml` > home `~/.orbit.yaml` > defaults
- `orbit init` generates default configuration file
- Per-agent settings: cli-path, auto-approve, timeout, extra-args, model, pre-command, post-command
- Agent aliases for per-variant model selection (e.g., `claude-sonnet`, `claude-opus`)

### Apsis - Session Transcript Viewer

#### Transcript Conversion

- Convert session transcripts from all 5 agent formats to Markdown or HTML
- Auto-detection of transcript format (Claude JSONL, Codex JSONL, Copilot JSONL, Kiro CLI SQLite/JSON, Kiro IDE JSON)
- `apsis latest` to view the most recent session
- Follow mode (`-F`) for live session monitoring (like `tail -f`)
- JSON output (`-f json`) for raw session data inspection
- Force agent format with `-a` flag when auto-detection is insufficient

#### Session Discovery

- Unified session listing across all agent types via `apsis -l`
- Session resolution by ID or file path
- Kiro IDE `.chat` file support with credit cost extraction
- Kiro CLI SQLite session discovery with path normalization and symlink support

#### Web Interface

- `apsis serve` starts a local web server for browsing sessions
- Session filtering by agent type and search by ID
- Dark mode, mobile-responsive design
- HTMX-powered auto-refresh (15s polling)
- Security headers and input validation middleware

### Build and CI

- GitHub Actions CI workflow for testing and linting
- Multi-platform release workflow (linux/windows/darwin x amd64/arm64)
- Build metadata injection: version, build time, and git commit via ldflags
