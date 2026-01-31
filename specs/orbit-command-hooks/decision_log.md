# Decision Log: Orbit Command Hooks

## Decision 1: Feature Naming

**Date**: 2025-01-31
**Status**: accepted

### Context

The feature encompasses multiple related changes: renaming post-command to post-prompt, adding pre-prompt, and adding agent-level shell commands. A clear feature name was needed for the spec directory and branch.

### Decision

Use "orbit-command-hooks" as the feature name.

### Rationale

This name is broader than "agent-pre-post-commands" and encompasses both the command hooks and the prompt renaming. It clearly indicates the feature is about Orbit's hook system.

### Alternatives Considered

- **agent-pre-post-commands**: Focused on agent-level commands - Rejected because it doesn't capture the prompt renaming aspect
- **agent-lifecycle-hooks**: Emphasizes hook pattern - Rejected because it's less descriptive of what the hooks do

### Consequences

**Positive:**
- Broad enough to cover all aspects of the feature
- Clear connection to Orbit tool

**Negative:**
- Slightly generic; could be confused with other hook systems

---

## Decision 2: Agent-Level Commands as Shell Commands

**Date**: 2025-01-31
**Status**: accepted

### Context

Agent-level pre/post hooks could either be shell commands (executed on host) or AI prompts (sent to the agent). The distinction needed to be clear.

### Decision

Agent-level pre-command and post-command are shell commands executed on the host system.

### Rationale

This provides a clear separation of concerns: prompts are for AI interactions (pre-prompt, post-prompt), commands are for shell operations (pre-command, post-command). Users often need to run linters, tests, or formatters around agent execution.

### Alternatives Considered

- **AI prompts**: Send prompts to agent before/after phases - Rejected because this overlaps with pre-prompt/post-prompt
- **Both options**: Support both shell and AI at agent level - Rejected to keep configuration simple

### Consequences

**Positive:**
- Clear separation: prompts = AI, commands = shell
- Enables common workflows (lint, test, format)

**Negative:**
- Less flexible than supporting both types

---

## Decision 3: Command Execution Timing

**Date**: 2025-01-31
**Status**: accepted

### Context

Agent-level commands could run per-phase (before/after each phase) or per-run (once at start, once at end).

### Decision

Agent-level commands run per-run: pre-command before the first phase, post-command after the last phase.

### Rationale

Running per-phase would be excessive for most use cases (running lint 5 times vs once). Per-run execution is simpler to reason about and more efficient.

### Alternatives Considered

- **Per phase**: Run before/after each phase - Rejected because it's excessive and slow
- **Configurable**: Let users choose - Rejected to keep configuration simple

### Consequences

**Positive:**
- Efficient (runs once, not N times)
- Simple to understand
- Matches common CI/CD patterns

**Negative:**
- Cannot run validation between phases (out of scope)

---

## Decision 4: Pre-Command Failure Behavior

**Date**: 2025-01-31
**Status**: accepted

### Context

When an agent-level pre-command fails (non-zero exit), the system needs to decide whether to continue or abort.

### Decision

If a pre-command fails, abort the entire run.

### Rationale

Pre-commands are preparation steps. If they fail, proceeding could lead to wasted agent execution or incorrect results. Fail-fast is safer and more predictable.

### Alternatives Considered

- **Skip the phase**: Continue with next phase - Rejected because it's unclear behavior
- **Configurable**: Let users choose fail behavior - Rejected to keep configuration simple

### Consequences

**Positive:**
- Fail-fast prevents wasted execution
- Predictable behavior
- Forces users to fix preparation issues

**Negative:**
- Less flexible for cases where pre-command is optional

---

## Decision 8: Configurable Shell Command Timeout

**Date**: 2025-01-31
**Status**: accepted

### Context

The initial design specified a fixed 5-minute timeout for shell commands. Reviewers noted that common operations like `npm install` or full test suites often exceed 5 minutes.

### Decision

Make the timeout configurable via `command-timeout` in `.orbit.yaml` and `ORBIT_COMMAND_TIMEOUT` environment variable, with a default of 5 minutes.

### Rationale

A fixed timeout is too restrictive for diverse use cases. Making it configurable gives users control while keeping a sensible default for simple commands.

### Alternatives Considered

- **Increase default to 15-30 minutes**: Would accommodate more use cases - Rejected because it delays failure detection for broken commands
- **No timeout**: Let commands run indefinitely - Rejected because it could hang forever

### Consequences

**Positive:**
- Users can adjust for their specific workflows
- Maintains fail-fast behavior for stuck commands
- Default is still conservative

**Negative:**
- Another configuration option to learn

---

## Decision 9: Pre-Prompt State Tracking

**Date**: 2025-01-31
**Status**: accepted

### Context

The existing summary.json tracks phase progress and post-completion state. Pre-prompt needs similar tracking for crash recovery and to avoid re-running on resume.

### Decision

Track pre-prompt state in summary.json including session_id, started_at, and completed_at. When resuming an interrupted run where pre-prompt completed, skip re-running it and use the stored session_id.

### Rationale

Consistent with existing phase and post-completion tracking. Allows crash recovery without losing pre-prompt context.

### Alternatives Considered

- **No tracking**: Always re-run pre-prompt on resume - Rejected because it wastes time and may corrupt context

### Consequences

**Positive:**
- Enables proper crash recovery
- Consistent with existing summary.json patterns

**Negative:**
- Additional fields in summary.json schema

---

## Decision 10: Dry-Run Behavior for Shell Commands

**Date**: 2025-01-31
**Status**: accepted

### Context

Orbit has a `--dry-run` flag that prevents agent execution. The requirements didn't specify behavior for shell commands during dry-run.

### Decision

When `--dry-run` is enabled, print the shell command that would be executed (including working directory) but do not execute it.

### Rationale

Executing shell commands during dry-run defeats the purpose of previewing what would happen. Users need to see the full execution plan without side effects.

### Alternatives Considered

- **Execute shell commands during dry-run**: Actually run them - Rejected because dry-run should be side-effect free

### Consequences

**Positive:**
- Dry-run is truly side-effect free
- Users can preview full execution plan

**Negative:**
- None significant

---

## Decision 11: Non-Interactive Commands Only

**Date**: 2025-01-31
**Status**: accepted

### Context

Shell commands that prompt for user input (passwords, confirmations) would hang until timeout in an automated orchestration context.

### Decision

Document that shell commands must be non-interactive. Commands requiring user input will hang until timeout and fail.

### Rationale

Orbit runs in automated/unattended mode. Interactive commands cannot work in this context. This is a constraint to document, not a feature to implement.

### Alternatives Considered

- **Detect and reject interactive commands**: Proactively check - Rejected because detecting interactive commands is not reliably possible

### Consequences

**Positive:**
- Clear expectation for users
- No complex detection logic needed

**Negative:**
- Users may hit timeout if they accidentally use interactive commands

---

## Decision 5: Pre-Prompt Session Continuity

**Date**: 2025-01-31
**Status**: accepted

### Context

The global pre-prompt starts an agent session. Phase 1 could either continue that session or start a fresh session.

### Decision

The session started by pre-prompt is continued by phase 1.

### Rationale

This allows the pre-prompt to establish context (e.g., review codebase, understand requirements) that carries forward into implementation. Separate sessions would lose this context.

### Alternatives Considered

- **Separate session**: Pre-prompt in its own session - Rejected because it loses context benefits

### Consequences

**Positive:**
- Context from pre-prompt carries into phases
- More efficient (reuses session)
- Natural conversation flow

**Negative:**
- Pre-prompt failures may corrupt session state (mitigated by fresh session on retry)

---

## Decision 6: Deprecation Handling

**Date**: 2025-01-31
**Status**: accepted

### Context

The rename from post-command to post-prompt is a breaking change. Users with existing configuration need to update it.

### Decision

Detect deprecated configuration (post-command at top level, ORBIT_POST_COMMAND env var, --post-command flag) and exit with an error before running.

### Rationale

Fail-fast with a clear error message ensures users explicitly update their configuration. Silent migration or warnings could lead to confusion about which setting is in effect.

### Alternatives Considered

- **Warning and continue**: Treat old config as new name - Rejected because it's unclear which setting takes precedence
- **Auto-migrate**: Automatically rename in config file - Rejected because it modifies user files without consent

### Consequences

**Positive:**
- Clear migration path
- No ambiguity about configuration
- Forces explicit user action

**Negative:**
- Breaking change requires manual intervention
- Users must update before upgrading

---

## Decision 7: No Global Default Commands

**Date**: 2025-01-31
**Status**: accepted

### Context

Agent-level commands could inherit from a global default, or each agent must configure its own commands.

### Decision

Agent-level commands are agent-specific only. There is no global pre-command or post-command that agents inherit.

### Rationale

Different agents may need different preparation/cleanup commands. A global default would either be too generic or require constant overriding.

### Alternatives Considered

- **Global default + agent override**: Define global commands that agents can override - Rejected because different agents have different needs

### Consequences

**Positive:**
- Clear, explicit configuration per agent
- No inheritance confusion

**Negative:**
- Repeated configuration if same command needed for multiple agents

---

## Decision 12: Pre-Prompt Session Handoff via StartPhase Override

**Date**: 2025-01-31
**Status**: accepted

### Context

Phase 1 needs to continue the session started by pre-prompt. The existing `StartPhase()` method generates new session IDs. A mechanism is needed to pass the pre-prompt session ID to phase 1.

### Decision

Modify `StartPhase()` to accept an optional `overrideSessionID` parameter. When phase 1 is called with a non-empty override, it uses that session ID and returns `isResume=true`.

### Rationale

This is a minimal change to the existing interface that preserves backward compatibility. Callers that don't use pre-prompt pass empty string and get existing behavior.

### Alternatives Considered

- **Store in Orbit struct and check in runPhase**: More coupling between components - Rejected for cleaner interface
- **New method StartPhaseWithSession**: Separate method - Rejected because it duplicates logic

### Consequences

**Positive:**
- Minimal interface change
- Backward compatible (empty string = existing behavior)
- Clear contract between orbit and log manager

**Negative:**
- Slightly more complex method signature

---

## Decision 13: Three-State Pre-Prompt Tracking

**Date**: 2025-01-31
**Status**: accepted

### Context

Crash recovery needs to distinguish between: pre-prompt never started, pre-prompt started but crashed, and pre-prompt completed. The original design only tracked "completed or not".

### Decision

Add explicit `Status` field to `PrePromptState` with values: "started" and "completed". Nil `PrePrompt` indicates never started.

### Rationale

Explicit status field is clearer than inferring state from presence/absence of `CompletedAt` field. Makes the state machine obvious.

### Alternatives Considered

- **Infer from CompletedAt nil check**: Less explicit - Rejected for clarity
- **Separate boolean fields**: Started bool, Completed bool - Rejected because status enum is cleaner

### Consequences

**Positive:**
- Clear state machine
- Easy to add new states if needed
- Explicit over implicit

**Negative:**
- Slightly more verbose JSON

---

## Decision 14: Staged Deprecation Checking

**Date**: 2025-01-31
**Status**: accepted

### Context

Deprecation must be checked before running, but some checks (env var, CLI flag) don't need workingDir while config file checks do. The original design implied all checks happen together.

### Decision

Split deprecation checking into stages:
1. CLI flag check (before flag parsing, no workingDir needed)
2. Environment variable check (before flag parsing, no workingDir needed)
3. Config file check (after workingDir is known)

### Rationale

This allows fail-fast behavior for the most common cases (CLI flag, env var) while still checking config files when appropriate.

### Alternatives Considered

- **All checks after workingDir known**: Delays CLI flag error - Rejected for faster failure
- **Assume cwd for early check**: May be wrong directory - Rejected for correctness

### Consequences

**Positive:**
- Fastest possible failure for CLI/env errors
- Correct config file checking with actual workingDir

**Negative:**
- Slightly more complex control flow

---

## Decision 15: Phase Count from Rune Client at Execution Time

**Date**: 2025-01-31
**Status**: accepted

### Context

The `ORBIT_PHASE_COUNT` environment variable should be set for shell commands. The original design used `len(o.phaseSummaries)`, but this is empty during pre-command execution (before `displayPhaseOverview()` populates it).

### Decision

Query `runeClient.GetPhaseSummaries()` directly at shell command execution time to get the current phase count.

### Rationale

This ensures the environment variable is accurate regardless of when in the execution flow the shell command runs.

### Alternatives Considered

- **Move displayPhaseOverview earlier**: Would require restructuring runSingle - Rejected for complexity
- **Set to 0 during pre-command**: Incorrect value - Rejected for accuracy

### Consequences

**Positive:**
- Accurate phase count at all execution points
- No dependency on internal state population order

**Negative:**
- Additional rune client call (minor performance impact)

---

## Decision 16: Variant Mode Hook Execution

**Date**: 2025-01-31
**Status**: accepted

### Context

Requirements 6.4 and 6.5 specify that variant mode must execute hooks independently per variant. The design needed to specify how hooks integrate with the existing `runVariant()` method and how parallel execution is handled.

### Decision

Each variant executes its own complete hook sequence independently:
1. Agent pre-command from the variant's assigned agent config
2. Global pre-prompt executed in the variant's worktree
3. Phase loop (existing)
4. Global post-prompt executed in the variant's worktree
5. Agent post-command from the variant's assigned agent config

In parallel mode, hooks run concurrently across variants since each variant operates in its own isolated worktree.

### Rationale

This approach leverages the existing isolation provided by git worktrees. Each variant already has:
- Its own working directory (worktree)
- Its own agent instance
- Its own log directory
- Its own rune client (task state)

The hooks naturally fit into this isolation model without requiring synchronization.

### Alternatives Considered

- **Shared pre-command for all variants**: Run one pre-command before any variant starts - Rejected because different agents may need different preparation
- **Sequential hooks even in parallel mode**: Add synchronization around hooks - Rejected because it defeats the purpose of parallel execution and worktrees are already isolated

### Consequences

**Positive:**
- Consistent execution model between single-run and variant mode
- No additional synchronization complexity
- Each variant can use different agent commands (useful when comparing agents)
- Hook failures are isolated to the failing variant

**Negative:**
- Pre-commands run N times (once per variant) instead of once
- More complex logging (per-variant log directories)

---

## Decision 17: Variant-Specific Environment Variable

**Date**: 2025-01-31
**Status**: accepted

### Context

Shell commands in variant mode need to know which variant they're running in, for logging or conditional behavior.

### Decision

Add `ORBIT_VARIANT` environment variable set to the variant ID (integer) when executing shell commands in variant mode. This is in addition to `ORBIT_PHASE_COUNT` and `ORBIT_AGENT`.

### Rationale

This enables shell commands to include variant context in their output or take variant-specific actions if needed.

### Alternatives Considered

- **No additional env var**: Rely on working directory differences - Rejected because explicit is better than implicit
- **Full variant metadata**: Pass branch name, guidance, etc. - Rejected for simplicity; ID is sufficient

### Consequences

**Positive:**
- Shell commands can identify their variant context
- Useful for debugging and logging

**Negative:**
- Additional environment variable to document
