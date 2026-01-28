# PR Review Overview - Iteration 1

**PR**: #36 | **Branch**: feature/opencode-agent | **Date**: 2026-01-28

## Valid Issues

### Code-Level Issues

#### Issue 1: Empty stdout should trigger error in JSON mode
- **File**: `internal/agents/opencode/agent.go:315-318`
- **Reviewer**: @chatgpt-codex-connector
- **Comment**: When OpenCode exits with code 0 but fails before emitting JSON (e.g., auth/CLI errors that only write to stderr), the guard skips setting `IsError` because it only fires when `len(raw) > 0`. In JSON mode an empty stdout is already invalid output, so it should trigger an error classification even when stdout is empty.
- **Validation**: Valid. The condition `!isValidJSON(raw) && len(raw) > 0` fails when stdout is empty, meaning errors that write only to stderr would be treated as success. Since `isValidJSON` already returns `false` for empty input, the `len(raw) > 0` check is redundant and incorrectly allows empty stdout to pass without setting `IsError`.

### PR-Level Issues

#### Issue 2: Extract magic numbers to named constants
- **File**: `internal/agents/opencode/agent.go`, `internal/agents/opencode/errors.go`
- **Reviewer**: @claude
- **Comment**: Hardcoded values like `1_000_000_000_000`, `30 * time.Second`, `60 * time.Second` should be extracted to named constants.
- **Validation**: Valid. Magic numbers reduce code readability. Using named constants like `unixMillisecondThreshold`, `defaultOverloadRetryAfter`, `defaultRateLimitRetryAfter` improves clarity.

#### Issue 3: Context cancellation check in DiscoverSessions
- **File**: `internal/agents/opencode/agent.go:150-218`
- **Reviewer**: @claude
- **Comment**: `DiscoverSessions()` receives a `context.Context` but never checks if it's cancelled during the potentially long-running directory traversal.
- **Validation**: Valid. Long-running operations should respect context cancellation to allow clean shutdown.

#### Issue 4: Error message could include output preview
- **File**: `internal/agents/opencode/agent.go:317`
- **Reviewer**: @claude
- **Comment**: `"output is not valid JSON"` message should include a snippet of the actual output for debugging.
- **Validation**: Valid. Including a preview of the output helps debugging when JSON parsing fails.

## Invalid/Skipped Issues

### Issue A: Potential false positives in error detection
- **Location**: `internal/agents/opencode/errors.go:94-104`
- **Reviewer**: @claude
- **Comment**: Pattern matching for "connection", "network", "timeout" could match benign occurrences.
- **Reason**: Skipped - The same patterns are used in the Codex agent (`internal/agents/codex/errors.go:71-82`) which this implementation follows for consistency. Changing this would require updating all agents to maintain consistency.

### Issue B: Code duplication between Classify and classifyPlaintext
- **Location**: `internal/agents/opencode/errors.go`
- **Reviewer**: @claude
- **Comment**: Both functions contain nearly identical error pattern checks.
- **Reason**: Skipped - Some duplication is intentional. The functions serve different purposes: `Classify` handles general error output while `classifyPlaintext` specifically handles non-JSON output. The patterns could diverge in the future. Extracting shared code would add abstraction without significant benefit.

### Issue C: SessionID logic inconsistency
- **Location**: `internal/agents/opencode/agent.go:244-249`
- **Reviewer**: @claude
- **Comment**: Question about whether `--continue` path would ever be taken if all session IDs have `ses_` prefix.
- **Reason**: Skipped - The code handles both formats defensively. OpenCode documentation shows sessions with `ses_` prefix, but supporting both formats is safer for future compatibility.

### Issue D: Session discovery performance
- **Location**: `internal/agents/opencode/agent.go:174-207`
- **Reviewer**: @claude
- **Comment**: Consider adding a limit on total sessions scanned.
- **Reason**: Skipped - The code already breaks early after finding the first message per session. Session directories are expected to be small in typical use. Adding limits would add complexity without clear benefit.
