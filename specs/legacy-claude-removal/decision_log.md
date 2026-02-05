# Decision Log: Legacy Claude Removal

## Decision 1: Remove internal/claude/client.go Entirely

**Date**: 2025-02-05
**Status**: accepted

### Context

The `internal/claude/client.go` file implements the `claudeRunner` interface used by the legacy code path in `runPhase()`. After migrating to the agent interface, this file may no longer be needed.

### Decision

Remove the `internal/claude/client.go` file entirely rather than keeping it for potential future use.

### Rationale

The file serves only the legacy interface being removed. Keeping deprecated code adds maintenance burden and confusion. If similar functionality is needed in the future, it can be reimplemented using the agent interface patterns.

### Alternatives Considered

- **Keep but deprecate**: Mark as deprecated but retain for reference - Rejected because deprecated code still requires maintenance and creates confusion
- **Keep if still used**: Only remove if truly unused - This is what we'll verify, but the expectation is it will be unused

### Consequences

**Positive:**
- Cleaner codebase with no dead code
- Reduced maintenance burden
- Clear signal that agent interface is the only supported path

**Negative:**
- None identified - the code can be recovered from git history if ever needed

---

## Decision 2: Enable Skipped Tests Immediately

**Date**: 2025-02-05
**Status**: accepted

### Context

There are 4 tests currently skipped because they require real-time delays (3-60 seconds). After migrating to `testutil.TestAgent` with `FakeClock`, they can run instantly.

### Decision

Remove `t.Skip()` calls and enable all 4 tests as part of this migration work.

### Rationale

The primary goal of this work is to enable these tests. Keeping them skipped after the migration defeats the purpose. Enabling them immediately ensures the migration is complete and verified in one PR.

### Alternatives Considered

- **Enable in separate PR**: Keep skipped, enable after verification in follow-up - Rejected because it adds unnecessary delay and risk of the follow-up being forgotten

### Consequences

**Positive:**
- Complete migration in one PR
- Immediate benefit from deterministic fast tests
- No risk of follow-up work being forgotten

**Negative:**
- Slightly larger PR scope (acceptable given the cohesion)

---

## Decision 3: No Backwards Compatibility Concerns

**Date**: 2025-02-05
**Status**: accepted

### Context

When removing internal interfaces and changing code paths, backwards compatibility with external tools or scripts must be considered.

### Decision

Treat this as internal refactoring only with no external compatibility concerns.

### Rationale

The `claudeRunner` interface is internal to the `orbit` package and not exported. No external tools or scripts depend on this interface. The public API of the `orbit` command remains unchanged.

### Alternatives Considered

- **Need to check**: Verify external dependencies before proceeding - Not necessary as the interface is unexported and internal

### Consequences

**Positive:**
- Simpler migration without compatibility shims
- Clean removal of legacy code

**Negative:**
- None - the interface is internal

---

## Decision 4: Use Adapter Pattern for Comparator

**Date**: 2025-02-05
**Status**: accepted

### Context

The `comparison.Comparator` uses `rawClaudeClient.RunCustomPrompt()` to execute comparison prompts. After removing the legacy Claude client, Comparator needs a way to execute prompts through the agent interface.

### Decision

Create an adapter type that wraps `agents.Agent` to satisfy the `promptRunner` interface required by Comparator.

### Rationale

The adapter pattern allows Comparator to remain unchanged while providing the interface it expects. This is the least invasive approach and follows the existing pattern of adapting interfaces in the codebase.

### Alternatives Considered

- **Modify Comparator directly**: Change Comparator to accept `agents.Agent` instead of `promptRunner` - Rejected because it would require more invasive changes to Comparator and its tests
- **Keep rawClaudeClient for now**: Defer Comparator migration to a separate spec - Rejected because it would leave the codebase in an inconsistent state with partial legacy code

### Consequences

**Positive:**
- Minimal changes to existing Comparator code
- Clean separation of concerns
- Adapter can be tested independently

**Negative:**
- One additional type to maintain (small cost)

---
