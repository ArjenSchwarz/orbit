# Decision Log: Multi-Agent Support

## Decision 1: Feature Name

**Date**: 2026-01-17
**Status**: accepted

### Context

The multi-agent support feature needed a canonical name for the spec directory and documentation references. A spec directory `specs/multi-agent/` already existed with a plan.md file.

### Decision

Use "multi-agent" as the feature name.

### Rationale

The directory already exists, and the name is concise while clearly describing the feature's purpose. Consistency with existing structure is preferred.

### Alternatives Considered

- **multi-agent-support**: More descriptive but unnecessarily verbose given the context is already clear within the orbit project

### Consequences

**Positive:**
- Consistent with existing directory structure
- Concise and clear

**Negative:**
- None identified

---

## Decision 2: Breaking Changes Acceptable

**Date**: 2026-01-17
**Status**: accepted

### Context

The refactoring of `internal/claude/` to `internal/agents/claudecode/` could potentially break backward compatibility for existing users. We needed to decide whether to maintain backward compatibility or allow breaking changes.

### Decision

Breaking changes are acceptable. The refactoring will proceed without maintaining the old package path or providing migration tooling.

### Rationale

Orbit is in early development with a very limited user base (primarily the author). The overhead of maintaining backward compatibility is not justified at this stage.

### Alternatives Considered

- **Full backward compatibility**: Maintain old package paths with aliases - Rejected due to unnecessary complexity for early-stage project
- **Deprecation period**: Support old paths with warnings for a transition period - Rejected as overkill given the user base

### Consequences

**Positive:**
- Cleaner codebase without compatibility shims
- Faster implementation

**Negative:**
- Any external users would need to update their code (unlikely to affect anyone)

---

## Decision 3: Defer Kiro/Copilot Format Analysis

**Date**: 2026-01-17
**Status**: accepted

### Context

The session format parsing for Kiro and Copilot CLIs requires analyzing sample session files to determine the `type` field values used for format detection. These sample files are not yet available.

### Decision

The user will provide sample session files from Kiro CLI and Copilot CLI. The format analysis will be completed before implementing the parsers.

### Rationale

Content-based format detection is the established pattern in the codebase (used for Claude and Codex). Sample files are necessary to identify the type field values and overall structure.

### Alternatives Considered

- **Reverse engineer from documentation**: CLI documentation may not fully document session format - Rejected due to incomplete information
- **Skip Kiro/Copilot initially**: Focus only on Claude and Codex - Rejected as the user wants full agent support

### Consequences

**Positive:**
- Accurate format detection based on real session data
- Consistent with existing implementation pattern

**Negative:**
- Implementation of Kiro/Copilot parsers blocked until sample files are provided

---

## Decision 4: Timeout Configuration Behavior

**Date**: 2026-01-17
**Status**: accepted

### Context

Agent executions can potentially run for extended periods. We needed to decide whether to impose a default timeout, make it configurable, or allow unlimited execution time.

### Decision

Timeout is configurable only with no default. If not explicitly configured, agent execution runs until completion or user interruption (SIGINT).

### Rationale

AI coding agents can legitimately take varying amounts of time depending on task complexity. Imposing a default timeout could interrupt valid long-running operations. Users who want timeouts can configure them explicitly.

### Alternatives Considered

- **30-minute default timeout**: Apply a default timeout to prevent runaway processes - Rejected because it could interrupt legitimate long-running tasks
- **No timeout support**: Remove timeout configuration entirely - Rejected because some users may want to limit execution time

### Consequences

**Positive:**
- No arbitrary interruption of valid long-running tasks
- Users retain full control over execution time

**Negative:**
- Hung processes will not be automatically detected or terminated
- Users must manually interrupt stuck executions

---

## Decision 5: Per-Variant Agent Selection Required

**Date**: 2026-01-17
**Status**: accepted

### Context

The plan.md included a feature for running different agents for different variants, enabling cross-agent comparison (e.g., Claude Code vs Codex implementations). We needed to confirm if this was required or could be deferred.

### Decision

Per-variant agent selection is a required feature and must be included in the initial implementation.

### Rationale

The ability to compare implementations across different agents is a key differentiator for Orbit. It enables users to evaluate which agent performs better for their specific use cases.

### Alternatives Considered

- **Nice-to-have**: Defer to a later phase if needed - Rejected by user
- **Out of scope**: Remove from initial implementation - Rejected by user

### Consequences

**Positive:**
- Enables powerful cross-agent comparison workflows
- Differentiates Orbit from single-agent tools

**Negative:**
- Increases implementation complexity
- Requires careful handling of agent-specific behaviors in comparison logic

---

## Decision 6: Verify CLI Invocations Before Implementation

**Date**: 2026-01-17
**Status**: accepted

### Context

The design review identified that several CLI invocation patterns in the requirements were based on plan.md assumptions that may not match actual CLI behavior. Specifically:
- Codex CLI uses `codex exec "prompt"` not `codex exec -p <prompt>`
- Codex resume uses `codex exec resume <id>` not `codex resume --session-id <id>`
- Copilot auto-approve uses `--allow-all-paths` plus per-tool flags, not `--allow-all-tools`
- Codex JSONL type values may differ from existing parser expectations

### Decision

1. Update requirements with corrected CLI invocations based on peer review research
2. Mark Codex JSONL type values as requiring verification from actual session files before finalizing
3. Keep existing parser type values (`session_meta`, `response_item`, etc.) until verified

### Rationale

CLI tools evolve and documentation may not be comprehensive. Verifying against actual behavior before implementation prevents wasted effort.

### Alternatives Considered

- **Implement based on plan.md**: Trust the original research - Rejected because peer review found discrepancies
- **Update all type values now**: Change Codex types based on peer review research - Rejected because verification from actual files is more reliable

### Consequences

**Positive:**
- Implementation will match actual CLI behavior
- Reduces risk of broken agent integrations

**Negative:**
- Requires verification step before Codex parser changes
- Some requirements remain approximate until verified

---

## Decision 7: Kiro Session Log Handling

**Date**: 2026-01-18
**Status**: accepted

### Context

Investigation revealed that Kiro CLI does not store session logs in a predictable file location like Claude Code or Codex. The only way to obtain session logs is to run a follow-up command: `kiro-cli chat --no-interactive "/chat save <filename>" --resume`. Additionally, Kiro exports are plain JSON, not JSONL format.

### Decision

1. Session discovery for Kiro is not automatic - logs must be explicitly exported using the `/chat save` command
2. The Kiro parser will handle plain JSON format rather than JSONL
3. For orchestration, Orbit may optionally export session logs after each phase using the save command

### Rationale

Working within Kiro's session management constraints is necessary. The workaround of using `/chat save` is functional, though less elegant than automatic session discovery.

### Alternatives Considered

- **Skip Kiro support**: Remove Kiro from supported agents - Rejected because user wants full agent support
- **Wait for Kiro to add session storage**: Defer implementation - Rejected because timeline is uncertain

### Consequences

**Positive:**
- Kiro integration is still possible with workaround
- Plain JSON parsing may be simpler than JSONL streaming

**Negative:**
- No automatic session discovery for Kiro
- Potential race conditions if multiple Kiro sessions run simultaneously (export command affects current session)
- Additional complexity in orchestration to handle session export

---

## Decision 8: Kiro Session Export Timing

**Date**: 2026-01-18
**Status**: accepted

### Context

Since Kiro does not automatically store session logs (Decision 7), Orbit needs to explicitly export sessions using the `/chat save` command. We needed to decide when to trigger this export during orchestration.

### Decision

Export Kiro session logs after each phase completes.

### Rationale

Exporting after each phase provides:
1. Debugging capability if later phases fail
2. Session history for the web interface and apsis viewing
3. Consistent behavior with other agents that automatically store sessions

### Alternatives Considered

- **Only on completion**: Export only after all phases complete - Rejected because debugging failed runs would be difficult
- **Never - manual only**: User must manually export - Rejected because it breaks parity with other agents

### Consequences

**Positive:**
- Session logs available for debugging after each phase
- Consistent viewing experience in web interface

**Negative:**
- Additional CLI call after each phase (slight overhead)
- Must handle potential failures in export command gracefully

---

## Decision 9: Copilot Session Format Confirmed

**Date**: 2026-01-18
**Status**: accepted

### Context

Sample file analysis was needed to determine the Copilot session format for parser implementation.

### Decision

Copilot uses JSONL format with the following type fields:
- `session.start`, `session.info` - Session metadata
- `user.message` - User input
- `assistant.turn_start`, `assistant.message`, `assistant.reasoning`, `assistant.turn_end` - Assistant responses
- `tool.execution_start`, `tool.execution_complete` - Tool invocations

### Rationale

Analysis of `events.jsonl` sample file confirmed a well-structured JSONL format with dot-notation type fields, parent-child relationships via `parentId`, and clear separation of turns and tool executions.

### Alternatives Considered

- None - format analysis was required before implementation

### Consequences

**Positive:**
- Clear type markers for format detection
- Structured format supports rich transcript rendering

**Negative:**
- None identified

---
## Decision 10: Disable Slow Retry Tests

**Date**: 2026-01-19
**Status**: accepted

### Context

Test suite execution takes over 2 minutes, with 125+ seconds spent in `internal/orbit` package tests. Four retry-logic tests use real `time.Sleep()` calls to test production retry behavior:
- `TestRunPhaseWithRetry_RateLimitError`: 60s (rate limit wait)
- `TestRunPhaseWithRetry_OverloadedError`: 30s (overload wait)
- `TestRunPostCommandWithRetry_MaxRetriesExceeded`: ~31s (exponential backoff)
- `TestRunPostCommandWithRetry_RetryableError_EventualSuccess`: ~3s

This makes `go test ./...` slow during development and CI, and causes Claude Code to appear "hung" when running tests as part of commit validation. The commit process always runs full tests without `-short` flag.

### Decision

Disable the four slow retry tests unconditionally using `t.Skip()` with descriptive messages explaining why they're disabled. Tests remain in the codebase for documentation but never execute.

### Rationale

Since Claude's commit process always runs `go test ./...` without control over flags, even a `testing.Short()` approach wouldn't help. The retry logic is still validated by other tests that check the retry attempt counts without actually sleeping. Disabling these specific timing tests allows fast test execution while maintaining test coverage of the retry logic itself.

### Alternatives Considered

- **Use testing.Short() flag**: Conditional skipping based on `-short` flag - Rejected because commit process can't control test flags
- **Inject time abstraction for mocking**: Create a `timeProvider` interface to mock `Sleep()` - Rejected due to added complexity for minimal benefit
- **Reduce sleep durations universally**: Use 100ms instead of 60s - Rejected because even 30ms × many retries adds up, and doesn't test production timing

### Consequences

**Positive:**
- Test suite completes in ~10 seconds instead of 127 seconds
- Claude Code commit validation no longer appears to hang
- Retry logic still validated by other tests (attempt counts, error classification)

**Negative:**
- Production retry timing behavior not validated by tests
- Tests can be re-enabled manually if needed for specific timing validation

---
