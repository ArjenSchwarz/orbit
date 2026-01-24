# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Per-variant model selection specification documents:
  - `specs/per-variant-model-selection/requirements.md` with 7 requirement sections covering agent alias configuration, config requirement, config initialization (`orbit init`), variant agent assignment, validation/error handling, model passing to agent CLIs, and logging/traceability
  - `specs/per-variant-model-selection/design.md` with architecture, components (AgentAliasConfig, ResolvedAgent types), data models, YAML type coercion, config merge behavior, error handling with exit codes, and testing strategy including property-based tests
  - `specs/per-variant-model-selection/decision_log.md` with 9 design decisions (agent alias approach, explicit type field, required config, validate structure not values, require `orbit init`, unified model flag, hardcoded model flag per agent, alias naming constraints)
  - `specs/per-variant-model-selection/tasks.md` with 7 implementation phases and 34 tasks covering config foundation, config loading/validation, init command, agent resolution changes, agent model flag implementation, metadata/logging, and integration tests

### Fixed

- Consolidator now generates a unique SessionID for each consolidation run, preventing session ID collisions and log file overwrites when agents like Claude Code or Kiro expect non-empty session IDs
- `RestoreOnFailure` now resets HEAD to the captured commit before cleanup, properly restoring the worktree if the agent checks out a different branch/commit during execution
- `truncateString` helper now handles edge cases where `maxLen <= 3` to prevent negative slice index panic

### Changed

- Replaced manual bubble sort with `slices.Sort()` in markdown report frontmatter generation for sorting variant IDs

### Added

- PR review documentation: `specs/variant-consolidation/review-overview-1.md` with issue analysis and `review-fixes-1.md` task tracking

### Fixed

- Consolidator report path: Changed from `.orbit/comparison/report.md` to `comparison-report/report.md` to match where `orbit compare` actually generates the report
- Consolidator improvements section detection: Changed from looking for `## Improvements from Other Variants` (h2) to `# Improvements from Other Variants` (h1) to match the actual heading level in the report
- Markdown report escaping: Changed `b.Text()` to `b.Raw()` for lines containing markdown syntax (`###`, `**`) so go-output doesn't escape them as `\#\#\#`

- Markdown report frontmatter now includes `generated_at`, `base_commit`, and `variant_commits` fields for staleness detection (requirement 1.7). Previously only included `title` and `date`. The `variant_commits` map enables the consolidate command to detect when the comparison report is stale relative to current variant HEAD commits.

### Added

- `orbit consolidate` CLI command integration (Phase 4 of variant consolidation):
  - `cmd/orbit/consolidate.go` with `consolidateCommand()` function implementing `orbit consolidate <spec> --variant <id>` command
  - Flags: `--variant` (required for consolidation), `--allow-dirty`, `--prompt`, `--rollback`
  - Spec auto-detection from git branch name when spec argument omitted (matches other orbit commands)
  - `handleRollback()` function for `--rollback` mode handling
  - `truncateString()` helper for displaying truncated custom prompts
  - `isAutomatedEnvironment()` for CI/automation detection (skips confirmation prompt)
  - Interactive confirmation prompt before consolidation proceeds
  - `NewConsolidatorForRollback()` constructor in consolidation package for rollback-only operations (no agent required)
  - Subcommand routing in `main.go` for `consolidate` command
  - Help text updated with consolidate command and example
  - Unit tests covering flag parsing validation, spec auto-detection, rollback mode validation, variant not found errors, truncateString, and CI environment detection
  - Subcommand tests for consolidate command parsing

- `internal/consolidation` Consolidator core implementation (Phase 3 of variant consolidation):
  - `consolidator.go` with `Consolidator` struct orchestrating the consolidation workflow:
    - `NewConsolidator()` constructor with validation for spec directory, variant ID, agent, and manager
    - `validateVariant()` checking variant exists, listing available variants if not found
    - `validateReport()` checking comparison report (report.md) exists
    - `checkStaleness()` comparing report metadata against current variant HEADs with warning
    - `checkEmptyImprovements()` for early exit when no cross-variant improvements exist
    - `checkCleanState()` validating worktree has no uncommitted changes (unless --allow-dirty)
    - `Run()` executing the full consolidation workflow with spinner stages
    - `runWithRetry()` for agent execution with error classification and retry support
    - `Rollback()` reverting the most recent consolidation commit using log or git search
    - Helper functions for parsing commit SHA, improvement counts from agent output
  - `ErrNoImprovements` sentinel error for empty improvements early exit
  - `truncateSHA()` helper for displaying abbreviated commit SHAs
  - `parseReportVariantCommits()` for extracting variant commits from YAML frontmatter
  - Comprehensive test coverage:
    - Unit tests for constructor validation, variant validation, report validation
    - Unit tests for staleness detection, empty improvements detection, clean state checks
    - Unit tests for commit SHA parsing, improvement count parsing, SHA truncation
    - Integration tests for E2E workflow, rollback, empty improvements, partial failure recovery

- `internal/consolidation` package foundation (Phase 2 of variant consolidation):
  - `types.go` with core types: `Config` struct for consolidation configuration (spec name, variant ID, agent, allow-dirty flag, custom prompt), `ConsolidationResult` for outcomes, `ConsolidationReport` parsed from agent output, `AppliedImprovement` and `SkippedImprovement` for tracking changes
  - `logger.go` with `Logger` struct for consolidation log management:
    - `LogEntry` struct with schema version, timestamp, variant ID, commit SHA, agent, improvements counts, and test/post-command results
    - `Append()` method with flock-style file locking for concurrent access safety
    - Atomic writes using temp file + rename pattern with UUID-based temp filenames
    - `SaveReport()` for timestamped markdown report files
    - `GetLatestCommitSHA()` for rollback support
  - `recovery.go` with `RecoveryManager` struct for git state management:
    - `CaptureState()` to record HEAD commit before agent runs
    - `CreateSnapshot()` to stash uncommitted changes with `--include-untracked` flag
    - `RestoreOnFailure()` using `git checkout -- .` and `git clean -fd` to remove partial modifications
    - `RestoreStash()` with conflict handling (leaves stash in place, returns warning)
    - `Cleanup()` to drop stash after successful completion
  - `prompt.go` with `PromptBuilder` struct for agent prompt construction:
    - Context section with comparison report path and all variant worktree paths
    - Optional custom instructions section when `--prompt` flag is provided
    - Instructions for analyzing cross-variant improvements, implementing changes, and committing
    - Conflict resolution policy prioritizing chosen variant's patterns
    - Scope constraints preventing unrelated changes, dependency additions, binary modifications
    - Edge case handling for renamed files, idempotency checks, missing paths
    - Report format template for agent output (applied/skipped improvements, commit SHA)
  - Comprehensive tests for all components including concurrent append, stash/restore operations, conflict handling, and prompt generation

- Dual-format report generation: Reports now generate both HTML (`index.html`) and Markdown (`report.md`) for GitHub-friendly browsing
- Markdown report uses go-output v2 for document building with YAML frontmatter metadata
- `VariantCommits` field in `ReportData` struct to track variant HEAD commits for staleness detection
- `GetHeadCommitInPath` method to `GitClient` interface for getting HEAD commit SHA in worktrees
- `GetVariantCommits` method to `variants.Manager` for collecting all variant HEAD commits
- Tests for markdown report generation including content verification and large diff linking

- Variant consolidation specification documents:
  - `specs/variant-consolidation/requirements.md` with 7 requirement sections covering markdown report generation, consolidate command (basic operation, agent execution, commit/validation, error handling), logging/tracking, and progress indication
  - `specs/variant-consolidation/design.md` with architecture, component diagram, data flow, interfaces (Consolidator, PromptBuilder, RecoveryManager, ConsolidationLogger), data models, error handling, and testing strategy
  - `specs/variant-consolidation/decision_log.md` with 22 design decisions covering report formats, conflict handling, agent design, recovery mechanisms, and peer review enhancements
  - `specs/variant-consolidation/tasks.md` with 4 implementation phases and 19 tasks for report enhancement, consolidation package foundation, consolidator core, and CLI integration

- Variant session logging: Each variant now gets its own log manager at `specs/{spec}/.orbit/logs/variant-{id}/` with phase-by-phase session storage
- Variant web interface registration: Each variant is registered separately in the run registry with variant-specific metadata
- Variant fields in `registry.RunEntry`: `IsVariant`, `VariantID`, `VariantRunID`, `VariantTotal`, `VariantAgent`, `VariantBranch`
- Variant display in web dashboard: Variants show purple left border, "variant" badge, and agent name
- Variant display in run detail page: Shows variant badge with N/M counter, agent name, variant branch, and related variants list with navigation links
- Related variants navigation: Run detail page shows all variants from the same run with links to switch between them
- CSS styles for variant badges, agent names, and variant run cards

### Changed

- Updated README.md and CLAUDE.md documentation to reflect multi-agent support:
  - Project description now mentions support for multiple AI coding agents
  - Added all 4 supported agents (Claude Code, Codex, Kiro, Copilot) to prerequisites
  - Added new CLI flags section for agent selection (`--agent`) and multi-variant comparison
  - Added per-agent configuration example in `.orbit.yaml`
  - Added Multi-Variant Comparison section with usage examples, guidance file format, subcommands, and workflow
  - Updated architecture diagram with new packages (agents, variants, comparison, report)
  - Updated configuration and error handling sections for agent-based architecture

### Fixed

- Kiro agent environment variable bug: `appendEnv()` now includes `os.Environ()` as the base before appending custom environment variables, preventing agent execution failures due to missing PATH and system environment
- Registered error classifiers for all agents: Kiro, Codex, and Copilot now call `agents.RegisterClassifier()` in their `init()` functions, enabling agent-specific error classification
- Per-variant agent selection: Variants now use their assigned agent via `agents.Get()` instead of always using the Claude client, enabling true multi-agent comparison with `--variant-agents` flag
- Linter warning in `internal/agents/codex/errors_test.go`: Removed redundant type assertion in interface check

### Added

- Kiro session export support in orchestrator:
  - Automatically exports Kiro sessions after each phase completes
  - Uses `SessionExporter` interface check for agents that require explicit export
  - Export files stored in log directory with pattern `phase-N-run-M-kiro-session.json`
  - Graceful error handling (logs warning but doesn't fail orchestration)

- Integration tests for multi-agent support:
  - Agent selection tests verifying default to claude-code
  - CLI override tests confirming flag takes precedence over config
  - All supported agents validation (claude-code, codex, kiro, copilot)
  - Variant agents cycling behavior tests for various scenarios

### Changed

- Migrated session result types from `claude.SessionResult` to `agents.RunResult`:
  - Updated `claudeRunner` interface to use `agents.RunResult`
  - Updated `logs.Manager` to accept `*agents.RunResult` in session save methods
  - Updated `comparison.Comparator` to use abstract `promptRunner` interface
  - Added `NumTurns`, `IsError`, and `Errors` fields to `agents.RunResult`
  - Added `getCostUSD()` helper for safe cost extraction from `*CostMetrics`

- Moved `BuildProjectPath` utility function to `claudecode` package:
  - Updated `apsis` and `logs` packages to import from new location
  - Function converts project paths to Claude's directory format

- Per-variant agent selection for multi-agent comparison (Phase 5):
  - `--variant-agents` flag for `orbit run` to specify comma-separated agent list
  - Agents cycle if fewer agents than variants (e.g., `--variant-agents claude-code,codex` for 4 variants assigns claude-code, codex, claude-code, codex)
  - `Agent` field added to `Variant` struct for tracking which agent ran each variant
  - Comparison report displays agent column in variants overview table
  - Agent badge displayed next to variant ID in implementation diffs section
  - Comparison prompt includes agent information in variant headers and metrics table
  - `AssignVariantAgents()` function in `internal/variants/agent.go` for agent assignment logic
  - CSS styling for `.agent-badge` in report templates

- Agent configuration and CLI integration (Phase 4):
  - `agent:` field in `.orbit.yaml` to set default agent per project
  - `agents:` section in `.orbit.yaml` for per-agent configuration:
    - `cli-path`: Override CLI command path
    - `auto-approve`: Tool approval behavior (maps to agent-specific flags)
    - `extra-args`: Additional CLI arguments
    - `timeout`: Execution timeout as duration string (e.g., "30m", "1h")
    - `model`: Agent-specific model option
  - `--agent` flag for `orbit run` to select agent (claude-code, codex, kiro, copilot)
  - Agent validation with helpful error messages including install URLs
  - `ORBIT_AGENT` environment variable for default agent
  - `GetAgentConfig()` method on config.Config returning agents.AgentConfig
  - `--agent`/`-a` flag for `apsis` to force agent format parsing
  - `ParseJSONLWithFormat()` function in transcript package for forced format parsing
  - Agent classifier registry with `RegisterClassifier()` and `GetClassifier()` functions
  - Default error classifier fallback for unregistered agents

- Multi-agent transcript parsing for Kiro and Copilot formats (Phases 3-4):
  - `FormatKiro` and `FormatCopilot` enum values in `internal/transcript/types.go`
  - `internal/transcript/kiro_types.go` with complete type definitions:
    - `KiroSession`, `KiroHistoryEntry`, `KiroUserMessage`, `KiroAssistantMessage`
    - `KiroUserContent` with `KiroPrompt` and `KiroToolUseResults` variants
    - `KiroToolUse`, `KiroToolCall`, `KiroTextResponse` for assistant responses
    - `KiroRequestMetadata`, `KiroDuration` for telemetry data
  - `internal/transcript/kiro_parser.go` implementing `ParseKiro()` function:
    - Parses plain JSON format with conversation_id and history array
    - Converts Kiro sessions to unified Entry format for markdown rendering
  - `internal/transcript/copilot_types.go` with complete type definitions:
    - `CopilotEvent` with polymorphic `CopilotData` for all event types
    - Support for session.start, session.info, user.message, assistant.turn_start/end
    - Support for assistant.message, assistant.reasoning, tool.execution_start/complete
    - `CopilotToolRequest`, `CopilotToolResult`, `CopilotToolTelemetry` for tool handling
  - `internal/transcript/copilot_parser.go` implementing `ParseCopilot()` function:
    - Parses JSONL format with event-based streaming structure
    - Converts Copilot events to unified Entry format for markdown rendering
  - Improved `DetectFormat()` function with multi-format detection strategy:
    - Reads 8KB chunk for Kiro detection (plain JSON with specific markers)
    - Falls back to JSONL first-line detection for Claude/Codex/Copilot
    - Added `copilotTypes` map for dot-notation type field recognition
  - Golden file tests for both Kiro and Copilot parsers
  - Sample session files in `testdata/kiro/` and `testdata/copilot/`

- Kiro agent implementation for multi-agent support (Phase 3):
  - `internal/agents/kiro/agent.go` implementing Agent and SessionExporter interfaces:
    - CLI invocation: `kiro-cli chat --no-interactive "<prompt>"`
    - Auto-approve: `--trust-all-tools` flag when enabled
    - Session resume: `--resume` flag to continue previous session
    - `ExportSession()` method to save sessions via `/chat save <filename>` command
    - DefaultSessionDir returns empty (Kiro doesn't store logs automatically per Decision 7)
    - Auto-registration via `init()` function
  - `internal/agents/kiro/errors.go` with Kiro-specific error classifier:
    - Rate limit detection (429, "rate limit", "throttl")
    - Authentication error detection ("credentials", "unauthorized", "access denied")
    - Session invalid detection ("session not found", "no active session")
    - Connection error detection ("timeout", "connection", "econnrefused")
    - API overload detection (503, "service unavailable")
  - `internal/agents/kiro/agent_test.go` with comprehensive unit tests

- Copilot agent implementation for multi-agent support (Phase 3):
  - `internal/agents/copilot/agent.go` implementing Agent interface:
    - CLI invocation: `copilot -p "<prompt>"`
    - Auto-approve: `--allow-all-paths` flag when enabled
    - Session resume: `--continue` flag (note: sessionID ignored per Known Limitation)
    - DefaultSessionDir: `~/.copilot/session-state/`
    - Auto-registration via `init()` function
  - `internal/agents/copilot/errors.go` with Copilot-specific error classifier:
    - Rate limit detection (429, "rate limit", "throttled")
    - Authentication error detection ("not logged in", "gh auth login")
    - Session invalid detection ("no session to continue", "no previous session")
    - Connection error detection ("timeout", "connection", "enotfound")
    - API overload detection (503, "service unavailable")
  - `internal/agents/copilot/agent_test.go` with comprehensive unit tests

- Agent abstraction layer for multi-agent support (Phase 1):
  - `internal/agents` package with core types and interfaces:
    - `agent.go` with `Agent` interface defining execution, session, and capability methods
    - `SessionInfo`, `RunOptions`, `RunResult`, and `CostMetrics` types for agent execution
    - `SessionExporter` optional interface for agents requiring explicit session export
  - `errors.go` with unified error classification:
    - `ErrorClass` enum (Unknown, Retryable, Fatal, SessionInvalid) for orchestrator retry logic
    - `ClassifiedError` struct wrapping errors with classification metadata and retry-after duration
    - `ErrorClassifier` interface for agent-specific error pattern recognition
  - `registry.go` with factory pattern for agent management:
    - `Register()` function for adding agent factories
    - `Get()` function returning configured agent instances
    - `List()` function returning sorted registered agent names
    - `Default()` function returning "claude-code" as the default agent
    - `AgentConfig` struct for CLI path, auto-approve, extra args, timeout, and agent-specific options
  - `internal/agents/claudecode` package with Claude Code agent implementation:
    - `agent.go` implementing the `Agent` interface with CLI execution
    - Session discovery via `~/.claude/projects/` directory scanning
    - Argument building for `--session-id`, `--resume`, `--output-format json`, and `--dangerously-skip-permissions`
    - JSON output parsing for session ID, cost, and error detection
    - Auto-registration via `init()` function
  - `errors.go` with Claude Code-specific error classifier:
    - Rate limit detection (429, "rate limit", "too many requests")
    - Authentication error detection (401, "api key", "unauthorized")
    - Session invalid detection ("session not found", "session expired")
    - Connection error detection ("timeout", "connection", "dns")
    - API overload detection (503, "overloaded", "service unavailable")
    - Retry-after duration parsing from error messages
  - Comprehensive test coverage for all components

### Changed

- Orbit orchestrator now initializes agent and error classifier (default: Claude Code)
- Fixed `range maxRetries` syntax errors (Go 1.22+ range-over-int not supported in older versions)

- Multi-agent support specification documents:
  - `specs/multi-agent/requirements.md` with 10 requirement sections covering agent abstraction layer, Claude Code/Codex/Kiro/Copilot agent implementations, agent selection, session discovery and viewing, error handling, agent configuration, and per-variant agent selection
  - `specs/multi-agent/design.md` with architecture, agent interface definition, registry pattern, error classification system, per-agent implementations, format detection extension, CLI integration, and known limitations
  - `specs/multi-agent/decision_log.md` with 9 design decisions (feature name, breaking changes acceptable, Kiro/Copilot format analysis deferral, timeout configuration, per-variant agent selection requirement, CLI invocation verification, Kiro session log handling, Kiro session export timing, Copilot session format)
  - `specs/multi-agent/tasks.md` with 6 implementation phases and 16 top-level tasks covering foundation, session parsing, additional agents, configuration/CLI, per-variant selection, and integration/cleanup
  - `specs/multi-agent/samples/kiro/newlog.json` sample Kiro session file for format analysis
  - `specs/multi-agent/samples/copilot/events.jsonl` sample Copilot session file for format analysis
  - `specs/multi-agent/plan.md` original planning document for multi-agent support

- Multi-spec comparison feature specification documents:
  - `specs/multi-spec-comparison/requirements.md` with 13 requirement sections covering variant configuration, git worktree management, variant execution, parallel execution, comparison, report generation, status/cleanup/finalize/compare commands, interrupt handling, performance, and error handling
  - `specs/multi-spec-comparison/design.md` with architecture, package dependencies, component designs for variants/comparison/report packages, CLI command specifications, and testing strategy
  - `specs/multi-spec-comparison/design-2026-01-11.md` with initial design draft
  - `specs/multi-spec-comparison/decision_log.md` with 11 design decisions (backwards compatibility, worktree reuse strategy, worktree location, dirty working directory handling, parallel retry independence, report format, comparison prompt strategy, partial failure handling, cleanup safety, finalize operation, and sanitization strategy)
  - `specs/multi-spec-comparison/tasks.md` with 7 implementation phases and detailed task breakdowns

- Integration tests for multi-spec comparison feature (Phase 7):
  - `internal/orbit/integration_test.go` with comprehensive variant integration tests:
    - `TestVariantRun_Sequential` for sequential variant execution with worktree and metadata verification
    - `TestVariantRun_Parallel` for parallel execution with semaphore limit verification
    - `TestVariantRun_SingleSuccess` for partial success scenarios (comparison skipped with single variant)
    - `TestCleanup_RemovesWorktrees` for cleanup command functionality
    - `TestFinalize_RebasesVariant` for finalize command with rebase verification
    - `TestFinalize_FailsOnDivergedBranch` for divergence detection
    - `TestOrbit_WithVariants_MockExecution` for Orbit config validation
  - `internal/variants/sanitize_property_test.go` with property-based tests using pgregory.net/rapid:
    - `TestPropertySanitizeName` verifying filesystem safety, no consecutive dashes, no edge dashes, and idempotence
    - `TestPropertySanitizeName_PreservesAlphanumeric` for alphanumeric preservation
    - `TestPropertySanitizeName_DashesPreserved` for valid dash-separated strings
    - `TestPropertySanitizeName_ReplacesUnsafeChars` for unsafe character replacement
    - `TestPropertySanitizeName_CollapsesMultipleDashes` for dash collapsing
    - `TestPropertySanitizeName_TrimsEdgeDashes` for edge dash trimming
    - `TestPropertySanitizeName_AllUnsafe` for all-unsafe input handling

### Changed

- `Rebase()` in `internal/variants/git.go` now uses fast-forward merge approach:
  - Checks out target branch first, then merges source branch with `--ff-only`
  - Keeps repository on target branch after completion (consistent working directory state)
  - Fails cleanly if fast-forward is not possible due to divergence
  - Added `TestRebase_FailsWhenDiverged` test for divergence detection

### Fixed

- Context size validation in `internal/comparison/compare.go`:
  - Added `MaxPromptTokens` constant (150000) to enforce context limits
  - Compare now checks estimated token count before calling Claude API
  - Returns descriptive error when combined diff size exceeds limit (Requirement 5.8)
  - Added `TestCompare_RejectsOversizedPrompt` test for validation

- Guidance file parsing in `cmd/orbit/run.go`:
  - Fixed logic that caused leading newlines when variant-specific guidance was empty
  - Separated into two passes: collect variant guidance, then apply global guidance
  - Global guidance now correctly appends to variant guidance or replaces empty guidance

- SpecDir validation in `internal/orbit/orbit.go`:
  - Added validation for SpecDir when variant mode is enabled
  - Fails early with clear error if SpecDir is empty or does not exist

- Added rapid property test failure files to `.gitignore`:
  - Pattern `**/testdata/rapid/**/*.fail` prevents test artifacts from being committed

- `sanitizeSpecName()` in `internal/variants/manager.go` now handles Unicode control characters:
  - Uses `unicode.IsControl()` for proper control character detection including extended ASCII (0x80+)
  - Refactored from `strings.NewReplacer` to rune-by-rune processing with `isUnsafeRune()` helper
  - Property-based testing discovered the bug (tab and Unicode control chars were not sanitized)

- Orbit integration for multi-spec comparison feature (Phase 6):
  - Variant configuration in `internal/config/config.go`:
    - `VariantCount`, `Parallel`, `MaxParallel`, `BranchPrefix`, `GuidanceFile`, `CompareCommand`, `GlobalGuidance` fields
    - Environment variable support: `ORBIT_VARIANT_COUNT`, `ORBIT_PARALLEL`, `ORBIT_MAX_PARALLEL`, `ORBIT_BRANCH_PREFIX`, `ORBIT_GUIDANCE_FILE`, `ORBIT_COMPARE_COMMAND`, `ORBIT_GLOBAL_GUIDANCE`
    - `DefaultMaxParallel` (3) and `DefaultBranchPrefix` ("orbit-impl") constants
    - `parsePositiveInt()` helper function for integer environment variable parsing
  - Variant support in `internal/orbit/orbit.go`:
    - `variantManager` field for variant lifecycle management
    - `rawClaudeClient` field for comparison operations
    - `comparisonResult` field for report generation
    - `SpecDir` and `RepoRoot` fields in Config for worktree paths
    - Variant manager initialization when `VariantCount > 0`
  - `runWithVariants()` method for multi-variant orchestration:
    - Worktree setup via variant manager
    - Sequential and parallel variant execution
    - SIGINT handling with graceful cancellation
    - Success/failure counting and reporting
  - `runVariant()` method for single variant execution:
    - Per-variant Claude client with worktree-specific working directory
    - Phase loop with retry logic
    - Metrics accumulation (cost, duration, turns)
    - Status updates to variant manager
  - `buildVariantPrompt()` for variant-specific and global guidance injection
  - `runVariantPhaseWithRetry()` with error classification and backoff
  - `runComparison()` for comparing successful variants:
    - Diff gathering via `comparison.NewDiffGatherer`
    - Claude-based comparison via `comparison.NewComparator`
    - Result storage for report generation
  - `generateReport()` and `generatePartialReport()` for HTML report generation:
    - Report creation via `report.NewGenerator`
    - Variant metrics and diffs included in report
    - Partial report for all-failed scenarios

- CLI commands for multi-spec comparison feature (Phase 5):
  - `orbit status <spec-name>` command to display variant implementation status:
    - Shows base commit, original branch, and start time
    - Displays table with variant ID, branch, worktree path, and status
    - Auto-detects spec name from current git branch if not provided
  - `orbit cleanup <spec-name>` command to remove variant worktrees and branches:
    - `--keep N` flag to preserve a specific variant while removing others
    - `--force` flag to skip confirmation prompt
    - `--dry-run` flag to preview what would be deleted
    - Confirmation prompt before destructive operations
  - `orbit finalize <spec-name> --variant N` command to adopt a variant:
    - Rebases chosen variant onto original branch
    - Cleans up all variant worktrees and branches after successful rebase
    - Divergence detection with helpful error messages
    - Conflict handling with manual resolution instructions
  - `orbit compare <spec-name>` command to regenerate comparison reports:
    - Collects diffs for all completed variants
    - Runs Claude comparison analysis
    - Generates HTML report with recommendation
- Variant flags for `orbit run` command:
  - `--variants N` to specify number of implementation variants
  - `--parallel` to run variants concurrently
  - `--max-parallel N` to limit concurrent variants (default: 3)
  - `--branch-prefix PREFIX` to customize branch naming (default: orbit-impl)
  - `--guidance-file PATH` for per-variant YAML guidance
  - `--compare-command CMD` for custom comparison commands
- Guidance file parsing for per-variant configuration:
  - YAML schema with `variants` array and `global_guidance` field
  - Validation of variant IDs against `--variants` count
  - Global guidance applied to variants without specific guidance
- Variant configuration in `orbit.Config` struct:
  - `VariantCount`, `Parallel`, `MaxParallel`, `BranchPrefix` fields
  - `Guidance` slice for per-variant guidance strings
  - `CompareCommand` for custom comparison command

- `internal/report` package for HTML comparison report generation (Phase 4):
  - `types.go` with report types: `ReportData` (spec name, variants, comparison result, metadata), `VariantReportData` (per-variant data with diff and metrics), `VariantMetrics` (cost, duration, turns)
  - `templates.go` with embedded HTML templates using `//go:embed`:
    - Template helper functions: `formatCost()` for currency formatting, `trimTrailingZeros()` for decimal cleanup, `add()` and `sub()` for template arithmetic
  - `templates/index.html` main report template with:
    - Recommendation section with variant recommendation and confidence level
    - Run metadata section (base commit, original branch, generation time)
    - Variants overview table with status, cost, duration, and turn metrics
    - Key observations list from comparison analysis
    - Per-file analysis with variant assessments and preferences
    - Collapsible diff sections for each variant implementation
  - `templates/diff.html` for separate large diff files (>500 lines)
  - `templates/style.css` with self-contained CSS:
    - CSS variables for consistent theming
    - Dark mode support via `prefers-color-scheme` media query
    - Responsive design for mobile and desktop
    - Print-friendly styles
    - Status badges for completed/failed/pending states
    - Confidence level styling (high/medium/low)
  - `generator.go` with `Generator` struct for report creation:
    - `NewGenerator()` constructor with output directory configuration
    - `Generate()` method creating index.html with automatic HTML escaping via html/template
    - `processReportData()` for handling large diffs by extracting to separate files
    - `generateDiffFile()` for creating variant-N.html files in diffs/ subdirectory
    - `countLines()` helper with 500-line threshold for diff splitting
  - `generator_test.go` with unit tests:
    - `TestGenerate_CreatesIndexHTML` verifying report creation with all content
    - `TestGenerate_EscapesContent` verifying XSS prevention via HTML escaping
    - `TestGenerate_SplitsLargeDiffs` verifying 500-line threshold behavior
    - `TestGenerate_IncludesFailedVariants` verifying failed variant error display
    - `TestGenerate_NoComparison` verifying report generation without comparison result
    - `TestCountLines`, `TestFormatCost`, `TestTrimTrailingZeros` for helper functions

- `internal/comparison` package for multi-variant comparison (Phase 3):
  - `types.go` with comparison types: `Result` (recommendation, confidence, summary, file analyses, observations), `FileAnalysis` (per-file comparison details), `VariantData` (variant input data with diff and metrics), `VariantMetrics` (cost, duration, turns)
  - `diff.go` with `DiffGatherer` struct for collecting unified diffs from variants using GitClient
  - `prompt.go` with `buildPrompt()` function to construct Claude comparison prompts with variant diffs and metrics table, `formatDuration()` for duration display, `estimatePromptTokens()` for context size estimation
  - `compare.go` with `Comparator` struct for orchestrating variant comparison:
    - `NewComparator()` constructor with Claude client and optional custom command
    - `Compare()` method with retry loop for JSON validation failures
    - `parseAndValidate()` for strict JSON parsing with range checking on recommendation and confidence values
    - `extractJSON()` to handle JSON in plain text or markdown code blocks
    - `extractJSONObject()` for brace-balanced JSON object extraction with proper string handling
  - `compare_test.go` with unit tests:
    - `TestBuildPrompt_IncludesAllVariants` and `TestBuildPrompt_IncludesMetrics` for prompt construction
    - `TestParseAndValidate_ValidJSON`, `TestParseAndValidate_MissingFields`, `TestParseAndValidate_InvalidConfidence`, `TestParseAndValidate_RecommendationOutOfRange` for validation
    - `TestExtractJSON_PlainJSON`, `TestExtractJSON_MarkdownCodeBlock`, `TestExtractJSON_PlainCodeBlock`, `TestExtractJSON_JSONInText`, `TestExtractJSON_NoJSON`, `TestExtractJSON_NestedObjects`, `TestExtractJSON_StringWithBraces` for JSON extraction
    - `TestFormatDuration` and `TestEstimatePromptTokens` for helper functions

- `internal/variants` package Manager implementation (Phase 2):
  - `manager.go` with `Manager` struct for variant lifecycle management:
    - `NewManager()` constructor with validation for spec name, directory, repo root, and git client
    - `Load()` to read existing `variants.json` metadata
    - `Save()` with atomic writes using temp file + rename pattern and mutex protection
    - `ensureGitignore()` to automatically create/update `.orbit/.gitignore` with `worktrees/` entry
    - `Setup()` to create branches and worktrees for all variants with cleanup on failure
    - `UpdateStatus()` and `UpdateMetrics()` for tracking variant execution state
    - `GetVariant()`, `GetVariantsSnapshot()`, and `CountByStatus()` for querying variants
    - `Cleanup()` to remove worktrees and branches with optional variant preservation
    - `Finalize()` to rebase chosen variant onto original branch with divergence check
    - `sanitizeSpecName()` for filesystem-safe spec names
  - `manager_test.go` with unit tests using mock GitClient:
    - `TestNewManager` for constructor validation
    - `TestSetup_*` for worktree creation, reuse, divergence detection, dirty directory check, gitignore handling
    - `TestUpdateStatus` and `TestSave_*` for status persistence and atomic/concurrent writes
    - `TestGetVariantsSnapshot_ReturnsCopy` and `TestCountByStatus` for query methods
    - `TestCleanup_*` for full cleanup and variant preservation
    - `TestFinalize_*` for rebase and divergence handling
    - `TestLoad_ParsesExistingMetadata` for metadata loading
    - `TestSanitizeSpecName` for filesystem safety

- `internal/variants` package for multi-variant spec implementation support (Phase 1):
  - `types.go` with core types: `VariantStatus` enum (pending, running, completed, failed, canceled), `Variant` struct with metrics, `VariantsMetadata` for variants.json, `Config` for variant execution settings
  - `git.go` with `GitClient` interface and `Git` implementation for git operations:
    - `GetCurrentBranch()`, `GetHeadCommit()` for repository state
    - `CreateBranch()`, `DeleteBranch()` for branch management
    - `CreateWorktree()`, `RemoveWorktree()` for worktree lifecycle
    - `GetDiff()` for comparing variant changes from base commit
    - `Rebase()` for applying variant changes to original branch
    - `BranchHasDiverged()`, `HasUncommittedChanges()` for state validation
    - Context support for cancellation of long-running operations
  - `git_test.go` with unit tests using real git operations in temp directories
  - `mock_git.go` with `MockGit` implementation for unit testing without real git:
    - Configurable return values for all GitClient methods
    - Call tracking for verification in tests
    - Per-call error overrides for complex test scenarios
    - Thread-safe with mutex protection
- Final integration tests for Codex support (Phase 6):
  - CLI integration tests for Codex session conversion to Markdown
  - CLI integration tests for Codex session conversion to HTML
  - Integration tests for Codex reasoning block rendering
  - Integration tests for apsis with Codex file path input
  - Integration tests for mixed Claude/Codex session listing (`apsis --list`)
  - Integration tests verifying correct source indicators (`[claude]` / `[codex]`)
  - Integration tests verifying timestamp-based sorting with Claude-first tie-breaking
  - Full test suite verification: all existing and new tests pass
  - Linter verification: no lint errors or warnings
- Error handling and negative tests for Codex support (Phase 5):
  - Negative tests for empty file, whitespace-only file, invalid first line JSON, and unknown format type errors
  - Negative tests for malformed middle line warnings with line number validation
  - Negative tests for all-lines-malformed errors and truncated last line handling
  - Negative tests for Codex-specific error cases: orphaned output warnings, missing type/payload fields, unrecognized event types
  - Negative tests for session discovery: invalid UUID search behavior, symlink to missing path, broken symlinks, empty file filtering
  - Warning summary output: "Parsed with N warning(s)" printed to stderr after parsing
  - Tool name display tests verifying shell_command and other Codex tool names are preserved exactly
  - Arguments JSON parsing tests verifying complex structures and raw fallback for invalid JSON
- Codex session discovery and unified session listing (Phase 4):
  - `findCodexSession()` function for searching `~/.codex/sessions/` by UUID
  - Case-insensitive UUID matching with exact 36-character validation
  - `walkDirFollowSymlinks()` function with cycle detection for symlink-safe directory traversal
  - `getCodexSessionTimestamp()` function to extract timestamp from Codex `session_meta` events with file mtime fallback
  - `listCodexSessions()` function to enumerate all Codex sessions with UUID extraction from filenames
  - `listClaudeSessions()` function extracted from `listSessions()` for modular session discovery
  - `listAllSessions()` function merging Claude and Codex sessions with unified sorting
  - `sortSessionsByTimestamp()` with Claude-first tie-breaking for identical timestamps
  - Updated `SessionInfo` struct with `Source` field ("claude" or "codex")
  - Updated `listSessions()` to display unified listing with source indicator (`[claude]` or `[codex]`)
  - Updated `resolveInput()` to check Claude location first, then Codex location for session ID resolution
  - Unit tests for all session discovery functions: UUID matching, directory traversal, empty/non-existent directories, symlink following, cycle detection, timestamp extraction, unified listing, Claude-first priority
- Parser integration for Codex format (Phase 3):
  - `ParseJSONL()` now auto-detects format and dispatches to appropriate parser (Claude or Codex)
  - `parseClaudeJSONL()` internal function extracted from previous `ParseJSONL()` implementation
  - `readFirstNonEmptyLineFromBufReader()` function for buffered reader position-preserving line reading
  - Streaming architecture preserved using `io.MultiReader` to combine first line with remaining content
  - Unit tests for format dispatch: Claude format detection, Codex format detection, error propagation
  - Integration tests for Codex to Markdown/HTML rendering pipeline
  - Golden file tests for Codex rendering consistency (`testdata/codex/basic.jsonl`, `testdata/codex/reasoning.jsonl`)
- Codex JSONL parser implementation (Phase 2):
  - `codex_types.go` with Codex struct definitions: `CodexEntry`, `CodexResponseItem`, `CodexContent`, `CodexSummary`, `CodexEventMsg`, `CodexSessionMeta`
  - `codex_parser.go` with `ParseCodexJSONL()` function and entry conversion functions
  - Entry consolidation logic that groups consecutive assistant events into single entries
  - Function call linking via `call_id` between `function_call` and `function_call_output` events
  - Support for multiple outputs per function call (streaming tool results)
  - Reasoning extraction from both `reasoning` response items and `agent_reasoning` event messages
  - Metadata event filtering: skips `session_meta`, `turn_context`, `token_count`, `user_message`, and `ghost_snapshot` events
  - Orphaned output handling with warnings for unmatched `function_call_output` entries
  - Unknown content type fallback to raw JSON text rendering
  - Test data files: `codex_valid.jsonl` and `codex_edge_cases.jsonl`
  - Unit tests for all Codex type unmarshaling with missing/extra field handling
  - Unit tests for parser covering message conversion, function call linking, reasoning extraction, event filtering, entry consolidation, malformed lines, and edge cases
  - Property-based tests using pgregory.net/rapid for format detection idempotence, text preservation in normalization, and tool call linking correctness
- Format detection for Codex log support (Phase 1):
  - `Format` enum type with `FormatUnknown`, `FormatClaude`, and `FormatCodex` values in types.go
  - `DetectFormat()` function to automatically detect log format from first non-empty JSONL line
  - Claude format detection for `user` and `assistant` type values
  - Codex format detection for `session_meta`, `response_item`, `event_msg`, and `turn_context` type values
  - UTF-8 BOM stripping for cross-platform compatibility
  - `readFirstNonEmptyLine()` helper function for robust format detection
  - Error messages following requirements: "empty file", "failed to parse first line as JSON", "unrecognized log format: type field value '{value}'"
  - Unit tests covering Claude detection, Codex detection, empty file, whitespace-only, invalid JSON, BOM handling, and unrecognized types
- Codex log format support specification documents:
  - `specs/codex-support/requirements.md` with 9 requirement sections covering format detection, Codex session discovery, session listing, JSONL parsing, content type mapping, metadata event filtering, tool name display, output compatibility, and error handling
  - `specs/codex-support/design.md` with architecture, components, data models, error handling, and testing strategy including property-based testing and golden file tests
  - `specs/codex-support/decision_log.md` with 9 design decisions (transparent CLI interface, unified session listing, warn-and-continue error handling, reasoning summary display, metadata filtering, Apsis-only scope, Claude-first discovery priority, authentic tool names, robust format detection)
  - `specs/codex-support/tasks.md` with 6 implementation phases and 26 tasks following TDD approach
- Post-completion transcript viewing in web interface:
  - `hasPostCompletionTranscript()` method to detect post-completion transcript files
  - `findPostCompletionTranscript()` method to locate transcript files
  - Post-completion section in run detail page with link to transcript
  - Support for `/runs/{id}/transcript/post` URL pattern
  - `IsPostCompletion` field in `TranscriptData` for template conditional rendering
- CSS cache busting with version query parameter:
  - `CSSVersion` constant for cache invalidation (bump when CSS changes)
  - `CSSVersion` field in `TemplateData` propagated to all page templates
  - CSS link updated to include `?v={{.CSSVersion}}` query parameter
- Transcript styling in web interface:
  - Full transcript CSS from standalone HTML renderer added to `style.css`
  - Message styles (user/assistant with distinct colors)
  - Thinking block styles with amber accent
  - Tool use/result styles with collapsible details
  - Markdown content styles (headings, lists, code, blockquotes)
  - Patch/diff styles with colored additions/deletions
  - Navigation bar styles with mobile responsive layout
  - Dark mode support for all transcript elements
- Auto-detection of `.orbit` subdirectory in `orbit register`:
  - When registering a specs directory, automatically uses `.orbit` subdirectory if present
  - Allows `orbit register specs/feature` instead of `orbit register specs/feature/.orbit`

### Added

- Auto-registration integration for web interface (Phase 6):
  - Orbit runs automatically register with the web interface registry on start
  - Registry entry created with status "running", PID, and log directory
  - Phase status updates tracked during execution (running, completed, failed)
  - Run count incremented on phase retries
  - Run status updated on completion (completed/failed) with finished timestamp
  - Graceful failure handling: registry errors logged as warnings, execution continues
  - Unit tests for all auto-registration scenarios
- `internal/web` package for web interface:
  - `server.go` with `Server` struct, `New()`, `Start()`, and `Shutdown()` methods
  - Graceful shutdown with 5-second timeout for in-flight requests
  - Signal handling for SIGINT/SIGTERM
  - `middleware.go` with security middleware:
    - `SecurityHeaders` adding X-Content-Type-Options, X-Frame-Options, Referrer-Policy, and CSP headers
    - `ValidateUUID` middleware for UUID v4 validation with regex
    - `PathSanitizer` middleware for path traversal prevention
    - `isPathWithinDir` helper with symlink resolution for security
  - `handlers.go` with page handlers:
    - `handleDashboard` rendering run list grouped by repository
    - `handleDashboardStatus` for htmx polling fragment
    - `handleRunDetail` rendering run info and phase list
    - `handleRunStatus` for htmx run status polling
    - `handleTranscript` rendering transcript viewer with navigation
    - `handleError` and `handleNotFound` for error pages
  - `static.go` with embedded static file handler using go:embed
  - `static/htmx.min.js` (htmx 1.9.12)
  - `static/style.css` with responsive design:
    - CSS variables for theming
    - Dark mode support via prefers-color-scheme
    - Mobile-responsive layout with 44px touch targets
    - Status indicator styling for running/completed/failed states
  - HTML templates in `templates/`:
    - `layout.html` base template with htmx connection status handling
    - `dashboard.html` for run list display
    - `dashboard_status.html` htmx polling fragment
    - `run_detail.html` for run info and phase list
    - `run_status.html` htmx polling fragment
    - `transcript.html` for transcript viewer with navigation
    - `error.html` for generic error pages
- `orbit serve` command for starting the web interface:
  - `--port` flag for port configuration (default from config or 8080)
  - `--bind` flag for bind address (default from config or localhost)
  - `--version` and `--help` flags
  - Configuration via environment variables or config files
  - Integration with registry for run discovery

### Fixed

- CSP header timing in web interface middleware: headers are now set before the response is written using a response wrapper that intercepts `WriteHeader()` and `Write()` calls
- Dashboard sort order: runs are now sorted chronologically using `time.Time.After()` instead of alphabetically by formatted date strings

### Changed

- `ParseJSONL()` empty file handling: now returns `"empty file"` error instead of empty result (breaking change for callers expecting empty result)
- Removed placeholder `serveCommand` from `cmd/orbit/main.go`
- Removed unused `fmt` import from `cmd/orbit/main.go`

- `NavigationContext` struct for transcript navigation with prev/next/back links
- `RenderHTMLFragment()` function for rendering transcript content without document wrapper (for embedding in web templates)
- `renderEntriesToBuilder()` shared function extracted from `RenderHTML()` to support both full document and fragment rendering
- `renderNavigationHTML()` helper function to generate navigation bar HTML with prev/next/back links
- Navigation CSS styles with mobile-responsive layout (44px touch targets, flexbox layout)
- Navigation support in `RenderHTML()` - adds navigation at top and bottom when `Navigation` is set in `RenderOptions`
- Unit tests for `RenderHTMLFragment`, navigation HTML generation, and HTML escaping

### Changed

- Extended `RenderOptions` with optional `Navigation` field for navigation context
- Refactored `RenderHTML()` to use shared `renderEntriesToBuilder()` function
- `orbit register` command for manually registering existing orbit log directories with the run registry
  - Validates log directories by checking for `phase-*-run-*-session.json` files
  - Derives status from `summary.json` when present (defaults to "completed" for historical runs)
  - Derives phases array by scanning session files and tracking run counts
  - Derives start time from `summary.json` or earliest file modification time
  - Derives branch name from `summary.json` or directory structure
  - Derives repository from git remote origin or directory name as fallback
  - Supports `--name` flag for custom display names
  - Updates existing entries when re-registering the same log directory (preserves ID)
  - Special handling for "." path to look for `.orbit/` in current directory
  - Sets PID to nil for manual registrations (vs auto-registered runs)
- CLI subcommand routing with `orbit run`, `orbit serve`, and `orbit register` subcommands
- Backward compatibility: existing `orbit --flags` syntax defaults to `orbit run`
- `ServePort` and `ServeBind` configuration options for the web server (defaults: 8080, localhost)
- Environment variable support: `ORBIT_SERVE_PORT` and `ORBIT_SERVE_BIND`
- Integration tests for CLI subcommand routing (`subcommand_test.go`)
- Unit tests for serve configuration options

### Changed

- Refactored `cmd/orbit/main.go` to use subcommand router pattern
- Extracted run command logic to `cmd/orbit/run.go`

### Added

- `internal/registry` package for run registration and discovery:
  - `types.go` with `RunStatus`, `PhaseStatus`, `Phase`, and `RunEntry` types with JSON serialization
  - `git.go` with `ParseGitRemote()` for extracting owner/repo from HTTPS, SSH, and SSH-alt git URLs
  - `GetRepository()` function with fallback to directory name when git parsing fails
  - `registry.go` with `Registry` struct for managing run entries in `~/.orbit/runs/`
  - Atomic file operations using temp file + rename pattern
  - `Register()`, `Get()`, `List()`, `FindByLogDir()`, `UpdateStatus()`, `UpdatePhase()` methods
  - Property-based tests using pgregory.net/rapid for git URL parsing
  - Schema version field for future migration support
- Web interface specification documents:
  - `requirements.md` with 11 requirement sections covering web server, registry, auto-registration, manual registration, dashboard, run detail, transcript viewer, mobile responsiveness, security, frontend architecture, and live updates
  - `design.md` with architecture, components, interfaces, data models, and testing strategy
  - `decision_log.md` with 10 design decisions (historical run discovery, htmx embedding, missing log handling, read-only v1, mobile viewport, transcript rendering, deferred REST API, schema versioning, per-phase tracking, security headers)
  - `tasks.md` with 8 implementation phases and 40+ tasks
- pgregory.net/rapid dependency (v1.2.0) for property-based testing

### Fixed

- Race condition in spinner `updateLoop`: the done channel is now captured and passed as a parameter to prevent goroutine leaks during rapid Stop()/Start() sequences
- Graceful shutdown now properly checks for interrupt signals between phases in the orchestration loop
- Context leak in demo command: `waitCancel()` is now properly deferred in a scoped closure
- Added documentation comments explaining goroutine safety in spinner `Start()` and `StartPostCompletion()` methods

### Added

- `orbit demo` subcommand for previewing spinner and hyperlink functionality:
  - Displays simulated phase overview table with sample data (Setup complete, Implementation running, Testing pending)
  - Runs animated spinner cycling through phases 1-3 with 10s per phase
  - Simulates retry wait countdown on even phases (5s wait)
  - Displays sample completion links to /tmp/orbit-demo on Ctrl+C exit
  - Requires TTY terminal (fails gracefully with error if not a TTY)
  - Subcommand dispatch in main.go before flag parsing
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

- Spinner now pauses before log output during retry waits to prevent visual artifacts (implements Requirement 4.5)
- Consistent error handling in `runPhase` and `runPostCommand`: errors now use `fail()` method to display index links on all failure paths
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
