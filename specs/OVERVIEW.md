# Specs Overview

| Name | Creation Date | Status | Summary |
|------|---------------|--------|---------|
| [Custom Commands](#custom-commands) | 2025-12-22 | Done | Configure custom prompts for Orbit task orchestration instead of the hardcoded `/next-task --phase` command. |
| [Session Management](#session-management) | 2025-12-24 | Done | Improve log storage and Claude session handling with three targeted enhancements. |
| [Apsis](#apsis) | 2025-12-26 | Done | Standalone CLI converting Claude Code JSONL session transcripts into readable Markdown. |
| [HTML Export](#html-export) | 2025-12-27 | No Tasks | Render Apsis transcripts and Orbit logs as styled HTML in addition to Markdown. |
| [Collapsible Details](#collapsible-details) | 2025-12-28 | Done | Wrap long tool outputs in `<details>` blocks for readable Apsis transcripts. |
| [Multi Spec Comparison](#multi-spec-comparison) | 2025-12-28 | Done | Run parallel implementations of one spec in worktrees, then compare and recommend a winner. |
| [Log Improvements](#log-improvements) | 2025-12-30 | No Tasks | Tracking notes for incremental improvements to apsis/orbit session log output. |
| [Orbit UX Improvements](#orbit-ux-improvements) | 2026-01-04 | Done | Two UX enhancements to make orbit runs friendlier and clearer. |
| [Web Interface](#web-interface) | 2026-01-05 | Done | Web UI for browsing Orbit runs, transcripts, and summaries from any device. |
| [Codex Support](#codex-support) | 2026-01-07 | Done | Extend Apsis to convert OpenAI Codex CLI session transcripts alongside Claude Code. |
| [Multi Agent](#multi-agent) | 2026-01-21 | Done | Unified abstraction layer letting Orbit orchestrate sessions across multiple AI coding agents. |
| [Variant Consolidation](#variant-consolidation) | 2026-01-24 | Done | Add consolidation and rollback capabilities to Orbit's multi-variant comparison workflow. |
| [Per Variant Model Selection](#per-variant-model-selection) | 2026-01-25 | Done | Run variants on the same agent but different models via named agent aliases. |
| [Apsis Follow](#apsis-follow) | 2026-01-26 | Done | Live-tail JSONL transcripts via `apsis -F` as agent sessions progress. |
| [Enhanced Status](#enhanced-status) | 2026-01-27 | Done | Detailed real-time visibility into running and failed variants in `orbit status`. |
| [OpenCode Agent](#opencode-agent) | 2026-01-28 | Done | Add OpenCode as a supported agent for Orbit orchestration. |
| [Centralized Logging](#centralized-logging) | 2026-01-29 | Done | Write Orbit debug logs as structured JSONL into `~/.orbit/logs/`. |
| [Kiro SQLite Logs](#kiro-sqlite-logs) | 2026-01-31 | Done | Parse Kiro CLI sessions directly from SQLite, replacing the `/chat save` workaround. |
| [Orbit Command Hooks](#orbit-command-hooks) | 2026-01-31 | Done | Separate shell commands from AI prompts and add agent-level command hooks. |
| [Apsis Copilot Support](#apsis-copilot-support) | 2026-02-01 | Done | Bring Apsis GitHub Copilot session support to parity with Claude/Codex/Kiro. |
| [Auto Consolidate](#auto-consolidate) | 2026-02-01 | Done | Auto-run consolidation on the recommended variant after `orbit run --variants`. |
| [Kiro Transcript Improvements](#kiro-transcript-improvements) | 2026-02-01 | Done | Show Kiro session credits and parse Json-variant tool results in Apsis. |
| [Copilot Usage Tracking](#copilot-usage-tracking) | 2026-02-02 | Done | Parse Copilot usage stats and fix multi-unit cost display across agents. |
| [Integration Test Framework](#integration-test-framework) | 2026-02-03 | Done | Mock-agent test framework for Orbit's orchestration without invoking real CLIs. |
| [Comparison Learnings](#comparison-learnings) | 2026-02-05 | Done | Add a Learnings section to variant comparison reports with code references. |
| [Legacy Claude Removal](#legacy-claude-removal) | 2026-02-05 | Done | Remove the legacy `claudeRunner` interface and migrate remaining tests to `TestAgent`. |
| [Apsis Kiro IDE Support](#apsis-kiro-ide-support) | 2026-02-08 | Done | Support Kiro IDE `.chat` session files alongside the existing CLI sources. |
| [Apsis Serve](#apsis-serve) | 2026-02-09 | Done | Local web server for browsing Apsis sessions across all supported agents. |
| [Vibes](#vibes) | 2026-02-10 | No Tasks | Three improvements to variant comparison: timeout, JSON persistence, offline reports. |
| [Apsis Latest](#apsis-latest) | 2026-02-11 | Done | Open the most recent project session via `apsis latest`, no ID required. |
| [Shared Agent Execution](#shared-agent-execution) | 2026-02-17 | No Tasks | Extract the shared exec scaffolding duplicated across all five agent `Run()` methods. |
| [Shared Retry Executor](#shared-retry-executor) | 2026-02-18 | No Tasks | Consolidate four near-identical retry executors into a single shared implementation. |
| [Message Metadata](#message-metadata) | 2026-03-12 | Done | Show timestamps and model identifiers inline in rendered Apsis transcripts. |
| [Finalize Show Verify](#finalize-show-verify) | 2026-05-10 | Planned | Show variant agent and warn on consolidation mismatch in `orbit finalize`. |

---

## Custom Commands

Configure custom prompts for Orbit task orchestration instead of the hardcoded `/next-task --phase` command.

- [decision_log.md](custom-commands/decision_log.md)
- [design.md](custom-commands/design.md)
- [requirements.md](custom-commands/requirements.md)
- [tasks.md](custom-commands/tasks.md)

## Session Management

Improve log storage and Claude session handling with three targeted enhancements.

- [decision_log.md](session-management/decision_log.md)
- [design.md](session-management/design.md)
- [requirements.md](session-management/requirements.md)
- [tasks.md](session-management/tasks.md)

## Apsis

Standalone CLI converting Claude Code JSONL session transcripts into readable Markdown.

- [decision_log.md](apsis/decision_log.md)
- [design.md](apsis/design.md)
- [requirements.md](apsis/requirements.md)
- [tasks.md](apsis/tasks.md)

## HTML Export

Render Apsis transcripts and Orbit logs as styled HTML in addition to Markdown.

- [PLAN.md](html-export/PLAN.md)

## Collapsible Details

Wrap long tool outputs in `<details>` blocks for readable Apsis transcripts.

- [decision_log.md](collapsible-details/decision_log.md)
- [design.md](collapsible-details/design.md)
- [plan.md](collapsible-details/plan.md)
- [requirements.md](collapsible-details/requirements.md)
- [tasks.md](collapsible-details/tasks.md)

## Multi Spec Comparison

Run parallel implementations of one spec in worktrees, then compare and recommend a winner.

- [decision_log.md](multi-spec-comparison/decision_log.md)
- [design-2026-01-11.md](multi-spec-comparison/design-2026-01-11.md)
- [design.md](multi-spec-comparison/design.md)
- [plan.md](multi-spec-comparison/plan.md)
- [requirements.md](multi-spec-comparison/requirements.md)
- [tasks.md](multi-spec-comparison/tasks.md)

## Log Improvements

Tracking notes for incremental improvements to apsis/orbit session log output.

- [session-log.md](log-improvements/session-log.md)

## Orbit UX Improvements

Two UX enhancements to make orbit runs friendlier and clearer.

- [decision_log.md](orbit-ux-improvements/decision_log.md)
- [design.md](orbit-ux-improvements/design.md)
- [requirements.md](orbit-ux-improvements/requirements.md)
- [tasks.md](orbit-ux-improvements/tasks.md)

## Web Interface

Web UI for browsing Orbit runs, transcripts, and summaries from any device.

- [decision_log.md](web-interface/decision_log.md)
- [design.md](web-interface/design.md)
- [PLAN.md](web-interface/PLAN.md)
- [requirements.md](web-interface/requirements.md)
- [tasks.md](web-interface/tasks.md)

## Codex Support

Extend Apsis to convert OpenAI Codex CLI session transcripts alongside Claude Code.

- [decision_log.md](codex-support/decision_log.md)
- [design.md](codex-support/design.md)
- [requirements.md](codex-support/requirements.md)
- [tasks.md](codex-support/tasks.md)

## Multi Agent

Unified abstraction layer letting Orbit orchestrate sessions across multiple AI coding agents.

- [decision_log.md](multi-agent/decision_log.md)
- [design.md](multi-agent/design.md)
- [plan.md](multi-agent/plan.md)
- [requirements.md](multi-agent/requirements.md)
- [tasks.md](multi-agent/tasks.md)

## Variant Consolidation

Add consolidation and rollback capabilities to Orbit's multi-variant comparison workflow.

- [decision_log.md](variant-consolidation/decision_log.md)
- [design.md](variant-consolidation/design.md)
- [requirements.md](variant-consolidation/requirements.md)
- [review-fixes-1.md](variant-consolidation/review-fixes-1.md)
- [review-overview-1.md](variant-consolidation/review-overview-1.md)
- [tasks.md](variant-consolidation/tasks.md)

## Per Variant Model Selection

Run variants on the same agent but different models via named agent aliases.

- [decision_log.md](per-variant-model-selection/decision_log.md)
- [design.md](per-variant-model-selection/design.md)
- [requirements.md](per-variant-model-selection/requirements.md)
- [tasks.md](per-variant-model-selection/tasks.md)

## Apsis Follow

Live-tail JSONL transcripts via `apsis -F` as agent sessions progress.

- [decision_log.md](apsis-follow/decision_log.md)
- [design.md](apsis-follow/design.md)
- [requirements.md](apsis-follow/requirements.md)
- [review-fixes-1.md](apsis-follow/review-fixes-1.md)
- [review-overview-1.md](apsis-follow/review-overview-1.md)
- [tasks.md](apsis-follow/tasks.md)

## Enhanced Status

Detailed real-time visibility into running and failed variants in `orbit status`.

- [decision_log.md](enhanced-status/decision_log.md)
- [design.md](enhanced-status/design.md)
- [requirements.md](enhanced-status/requirements.md)
- [review-fixes-1.md](enhanced-status/review-fixes-1.md)
- [review-overview-1.md](enhanced-status/review-overview-1.md)
- [tasks.md](enhanced-status/tasks.md)

## OpenCode Agent

Add OpenCode as a supported agent for Orbit orchestration.

- [review-fixes-1.md](opencode-agent/review-fixes-1.md)
- [review-overview-1.md](opencode-agent/review-overview-1.md)
- [smolspec.md](opencode-agent/smolspec.md)
- [tasks.md](opencode-agent/tasks.md)

## Centralized Logging

Write Orbit debug logs as structured JSONL into `~/.orbit/logs/`.

- [decision_log.md](centralized-logging/decision_log.md)
- [design.md](centralized-logging/design.md)
- [requirements.md](centralized-logging/requirements.md)
- [review-fixes-1.md](centralized-logging/review-fixes-1.md)
- [review-overview-1.md](centralized-logging/review-overview-1.md)
- [tasks.md](centralized-logging/tasks.md)

## Kiro SQLite Logs

Parse Kiro CLI sessions directly from SQLite, replacing the `/chat save` workaround.

- [decision_log.md](kiro-sqlite-logs/decision_log.md)
- [design.md](kiro-sqlite-logs/design.md)
- [requirements.md](kiro-sqlite-logs/requirements.md)
- [tasks.md](kiro-sqlite-logs/tasks.md)

## Orbit Command Hooks

Separate shell commands from AI prompts and add agent-level command hooks.

- [decision_log.md](orbit-command-hooks/decision_log.md)
- [design.md](orbit-command-hooks/design.md)
- [requirements.md](orbit-command-hooks/requirements.md)
- [review-fixes-1.md](orbit-command-hooks/review-fixes-1.md)
- [review-overview-1.md](orbit-command-hooks/review-overview-1.md)
- [tasks.md](orbit-command-hooks/tasks.md)

## Apsis Copilot Support

Bring Apsis GitHub Copilot session support to parity with Claude/Codex/Kiro.

- [smolspec.md](apsis-copilot-support/smolspec.md)
- [tasks.md](apsis-copilot-support/tasks.md)

## Auto Consolidate

Auto-run consolidation on the recommended variant after `orbit run --variants`.

- [smolspec.md](auto-consolidate/smolspec.md)
- [tasks.md](auto-consolidate/tasks.md)

## Kiro Transcript Improvements

Show Kiro session credits and parse Json-variant tool results in Apsis.

- [smolspec.md](kiro-transcript-improvements/smolspec.md)
- [tasks.md](kiro-transcript-improvements/tasks.md)

## Copilot Usage Tracking

Parse Copilot usage stats and fix multi-unit cost display across agents.

- [decision_log.md](copilot-usage-tracking/decision_log.md)
- [design.md](copilot-usage-tracking/design.md)
- [requirements.md](copilot-usage-tracking/requirements.md)
- [tasks.md](copilot-usage-tracking/tasks.md)

## Integration Test Framework

Mock-agent test framework for Orbit's orchestration without invoking real CLIs.

- [decision_log.md](integration-test-framework/decision_log.md)
- [design.md](integration-test-framework/design.md)
- [requirements.md](integration-test-framework/requirements.md)
- [tasks.md](integration-test-framework/tasks.md)
- [test-migration-completion.md](integration-test-framework/test-migration-completion.md)

## Comparison Learnings

Add a Learnings section to variant comparison reports with code references.

- [decision_log.md](comparison-learnings/decision_log.md)
- [design.md](comparison-learnings/design.md)
- [implementation.md](comparison-learnings/implementation.md)
- [requirements.md](comparison-learnings/requirements.md)
- [tasks.md](comparison-learnings/tasks.md)

## Legacy Claude Removal

Remove the legacy `claudeRunner` interface and migrate remaining tests to `TestAgent`.

- [decision_log.md](legacy-claude-removal/decision_log.md)
- [design.md](legacy-claude-removal/design.md)
- [implementation.md](legacy-claude-removal/implementation.md)
- [requirements.md](legacy-claude-removal/requirements.md)
- [tasks.md](legacy-claude-removal/tasks.md)
- [verification-report.md](legacy-claude-removal/verification-report.md)

## Apsis Kiro IDE Support

Support Kiro IDE `.chat` session files alongside the existing CLI sources.

- [decision_log.md](apsis-kiro-ide-support/decision_log.md)
- [design.md](apsis-kiro-ide-support/design.md)
- [implementation.md](apsis-kiro-ide-support/implementation.md)
- [requirements.md](apsis-kiro-ide-support/requirements.md)
- [review-fixes-1.md](apsis-kiro-ide-support/review-fixes-1.md)
- [review-overview-1.md](apsis-kiro-ide-support/review-overview-1.md)
- [tasks.md](apsis-kiro-ide-support/tasks.md)

## Apsis Serve

Local web server for browsing Apsis sessions across all supported agents.

- [decision_log.md](apsis-serve/decision_log.md)
- [design.md](apsis-serve/design.md)
- [implementation.md](apsis-serve/implementation.md)
- [requirements.md](apsis-serve/requirements.md)
- [review-fixes-1.md](apsis-serve/review-fixes-1.md)
- [review-overview-1.md](apsis-serve/review-overview-1.md)
- [tasks.md](apsis-serve/tasks.md)

## Vibes

Three improvements to variant comparison: timeout, JSON persistence, offline reports.

- [compare-improvements.md](vibes/compare-improvements.md)

## Apsis Latest

Open the most recent project session via `apsis latest`, no ID required.

- [implementation.md](apsis-latest/implementation.md)
- [smolspec.md](apsis-latest/smolspec.md)
- [tasks.md](apsis-latest/tasks.md)

## Shared Agent Execution

Extract the shared exec scaffolding duplicated across all five agent `Run()` methods.

- [smolspec.md](shared-agent-execution/smolspec.md)

## Shared Retry Executor

Consolidate four near-identical retry executors into a single shared implementation.

- [smolspec.md](shared-retry-executor/smolspec.md)

## Message Metadata

Show timestamps and model identifiers inline in rendered Apsis transcripts.

- [decision_log.md](message-metadata/decision_log.md)
- [design.md](message-metadata/design.md)
- [implementation.md](message-metadata/implementation.md)
- [requirements.md](message-metadata/requirements.md)
- [tasks.md](message-metadata/tasks.md)

## Finalize Show Verify

Show variant agent and warn on consolidation mismatch in `orbit finalize`.

- [decision_log.md](finalize-show-verify/decision_log.md)
- [smolspec.md](finalize-show-verify/smolspec.md)
- [tasks.md](finalize-show-verify/tasks.md)
