# Agent Execution Pattern

## Shared Executor (`internal/agents/executor.go`)

All 5 agents delegate CLI command execution to `agents.Execute()`. This function handles the mechanical parts that were previously duplicated across all agents:

1. Create `exec.CommandContext` with the CLI path and args
2. Set working directory (if provided)
3. Merge environment variables with `os.Environ()` (only when env map is non-empty)
4. Capture stdout/stderr into buffers, set stdin=nil
5. Time the execution (wall clock)
6. Extract exit code from `*exec.ExitError` (or -1 for non-exit errors like command-not-found)

## Agent Responsibilities

Each agent still owns:
- **Arg building** (`buildArgs()`) — agent-specific CLI flags, prompt placement, resume handling
- **Post-processing** — parsing output, extracting costs, validating JSON

## Per-Agent Post-Processing

| Agent | Post-processing |
|-------|----------------|
| Claude Code | Parse JSON output for session ID, cost, turns, errors; override duration from API response |
| Codex | None (raw text output) |
| Kiro | Query SQLite for session credits via `extractSessionCredits()` |
| Copilot | `ParseUsage()` on combined stdout/stderr for premium requests, tokens, durations |
| OpenCode | Validate JSON output, detect errors from empty/invalid JSON |

## Kiro WorkDir Resolution

Kiro resolves the working directory independently of the executor for its post-processing. When `opts.WorkDir` is empty, it calls `os.Getwd()` to determine the directory for session credit lookup. The executor receives `opts.WorkDir` as-is (empty means inherit).

## Environment Handling

When `cfg.Env` is nil or empty, the command inherits the default environment (Go's `exec.Cmd` default behavior). When env vars are provided, the current environment is explicitly copied via `os.Environ()` and the additional vars are appended. This means provided vars override existing ones with the same name (last value wins in the env slice).

## Shared Retry Executor (`internal/agents/retry.go`)

All retry-with-backoff logic is consolidated in `agents.RunWithRetry()`. This replaces 5 previously divergent retry loops:

- `runPhaseWithRetry` (single-run mode)
- `runPostPromptWithRetry` (single-run mode)
- `runVariantPhaseWithRetry` (variant mode)
- `runVariantPostCompletion` (variant mode)
- `consolidation.runWithRetry` (consolidation mode)

### Design

Callback-based `RetryConfig` struct with:

| Field | Type | Purpose |
|-------|------|---------|
| `MaxRetries` | `int` | Maximum number of attempts |
| `Sleep` | `func(time.Duration)` | Injected sleep (Clock.Sleep for testability) |
| `Execute` | `func(ctx, attempt)` | Runs the operation |
| `Classify` | `func(result, err)` | Returns nil for success, ClassifiedError for failure |
| `OnRetry` | `func(attempt, max, classified, backoff)` | Pre-sleep logging/UI |
| `AfterWait` | `func()` | Post-sleep cleanup (e.g., spinner stop) |

### Classify Callback Patterns

Two helper functions in `internal/orbit/single.go`:

1. **`classifyReturned`** — for callers where Execute already returns a `*ClassifiedError` (single-run mode). Passes it through or wraps unknown errors as fatal.
2. **`classifyFromAgent(agentName)`** — for callers where Execute returns raw agent results (variant/consolidation mode). Uses `agents.GetClassifier()` from the registry.

### Key Behaviors

- **Rate-limit wait**: resets attempt counter to 0 after sleeping, giving a fresh set of retries
- **Last attempt optimization**: skips sleep after the final failed attempt (no retry follows)
- **Context cancellation**: checked before each Execute call
- **Backoff priority**: rate-limit duration > explicit RetryAfter > exponential backoff (1s, 2s, 4s, 8s, 16s capped)

### Gotchas

- `o.sleepFunc()` in orbit package returns `time.Sleep` if `o.config.Clock` is nil. Tests that construct `Orbit` directly (not via `New()`) may not set Clock.
- Consolidation uses `time.Sleep` directly since it doesn't have a Clock dependency.
- Long sleeps (rate-limit waits) are broken into 30-second chunks with context checks between them, so Ctrl+C is responsive even during multi-hour usage limit waits.
- Rate-limit resets are capped at 5 to prevent infinite loops if the condition never resolves.

## Variant Post-Prompt Session Lifecycle (`internal/orbit/variants.go`)

`runVariantPostCompletion` mirrors single-run `runPostPrompt`:

1. `logManager.StartPostCompletion(ContinueSession)` returns `(sessionID, isResume)`.
2. If `isResume`, call `agent.Resume(sessionID, opts)`. On invalid-session
   error, fall back to a fresh UUID + `agent.Run`, and update the manager
   via `SetPostCompletionSessionID`.
3. On success, reconcile the agent-returned session id via
   `ReconcilePostCompletionSessionID` and clear the in-progress entry via
   `CompletePostCompletion`.

Without these calls, variant post-prompt always opens a brand-new session,
losing phase context and bypassing the documented post-completion lifecycle
(T-715).

## Claude Code Usage Limit Parsing (`internal/agents/claudecode/errors.go`)

`parseUsageLimitReset()` extracts the reset time from messages like `"resets 3am (Australia/Melbourne)"` or `"resets 3am"` (no timezone). The timezone in parentheses is optional -- when absent, `time.Local` is used as the default. The regex uses 4 capture groups; `FindStringSubmatch` returns a 5-element slice on match (or nil), so `matches[4]` is `""` when the optional timezone group doesn't match.
