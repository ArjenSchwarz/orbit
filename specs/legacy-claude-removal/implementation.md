# Implementation Explanation: Legacy Claude Removal

This document explains the implementation at three expertise levels and validates completeness against requirements.

---

## Beginner Level

### What Changed

Orbit is a tool that runs AI coding assistants (like Claude Code, OpenAI Codex, etc.) to implement software features automatically. Before this change, Orbit had two different ways to talk to the Claude Code assistant:

1. **The old way** (`claudeRunner` interface): A custom wrapper specifically for Claude Code
2. **The new way** (`agents.Agent` interface): A unified system that works with any AI assistant

Having two systems created problems:
- Tests couldn't run fast because they had to wait for real time delays
- The code was harder to maintain with duplicate logic
- Adding new AI assistants required understanding both systems

This change removes the old way entirely, making everything use the new unified system.

### Why It Matters

**For developers working on Orbit:**
- Tests now run in milliseconds instead of 30-60 seconds (some were skipped entirely before)
- One consistent way to work with AI assistants
- Less code to maintain (deleted ~620 lines of legacy code)

**For users of Orbit:**
- No visible changes - the tool works exactly the same way
- Internal improvements mean fewer bugs and faster development of new features

### Key Concepts

- **Interface**: A contract that says "any code that wants to work with me must provide these specific functions." Like a power socket - any device with the right plug works.
- **Adapter Pattern**: A translator that makes one type of interface work with another. Like a travel adapter that lets your US plug work in a European socket.
- **Mock/Test Double**: A fake version of something used in tests. Like a crash test dummy instead of a real person.
- **FakeClock**: A pretend clock in tests that lets you instantly "skip ahead" in time instead of actually waiting.

---

## Intermediate Level

### Changes Overview

| Component | Change | Files Affected |
|-----------|--------|----------------|
| Orchestration | `runPhase()` now uses `agent.Run()`/`Resume()` | `internal/orbit/orbit.go` |
| Comparison | New `AgentAdapter` bridges to `promptRunner` interface | `internal/comparison/adapter.go` |
| Compare CLI | Uses agent registry instead of `claude.Client` | `cmd/orbit/compare.go` |
| Tests | Migrated from `mockClaudeClient` to `testutil.TestAgent` | `internal/orbit/orbit_test.go` |
| Deleted | Legacy Claude client | `internal/claude/client.go`, `client_test.go` |

### Implementation Approach

**1. AgentAdapter Pattern (Decision 4)**

The `comparison.Comparator` expected a `promptRunner` interface with `RunCustomPrompt(prompt string)`. Rather than modifying Comparator (invasive), we created a thin adapter:

```go
type AgentAdapter struct {
    agent   agents.Agent
    ctx     context.Context
    workDir string
}

func (a *AgentAdapter) RunCustomPrompt(prompt string) (*agents.RunResult, error) {
    return a.agent.Run(a.ctx, agents.RunOptions{Prompt: prompt, WorkDir: a.workDir})
}
```

**2. runPhase() Migration**

Replaced direct `claudeClient.RunPhase()` calls with:
- `agent.Run()` for new sessions
- `agent.Resume()` for continuing existing sessions

The session-invalid fallback logic (retry with fresh session when session expires) was preserved.

**3. Test Migration Strategy**

Previously skipped tests used `mockClaudeClient` with real `time.Sleep()` calls. Migrated to:
- `testutil.TestAgent` with `ScenarioBuilder` for defining expected behavior
- `testutil.FakeClock` for instant time simulation
- `clock.AssertSleeps()` to verify backoff timing without waiting

**4. TestAgent Error Behavior Fix**

Real agents return both a result AND an error when execution fails. `TestAgent` was updated to match:
```go
if result.IsError {
    return result, fmt.Errorf("agent error: exit code %d", result.ExitCode)
}
```

### Trade-offs

| Decision | Chosen | Alternative | Why |
|----------|--------|-------------|-----|
| Adapter location | `internal/comparison/` | `internal/orbit/` | Keeps adapter near its consumer, avoids circular imports |
| AutoApprove handling | Set in AgentConfig | Add to RunOptions | RunOptions.AutoApprove isn't used by any agent implementation |
| Test migration scope | All in one PR | Incremental | Ensures no orphaned legacy code |

---

## Expert Level

### Technical Deep Dive

**Session ID Contract**

The `Resume()` method takes sessionID as both a function argument and in `opts.SessionID`. The implementation follows this contract:
```go
func (a *Agent) Resume(ctx context.Context, sessionID string, opts RunOptions) (*RunResult, error) {
    opts.SessionID = sessionID  // First argument is authoritative
    return a.execute(ctx, opts, true)
}
```

**Error Classification Flow**

After migration, error handling remains agent-specific:
1. Agent returns `RunResult` with `IsError`, `Stderr`, `ErrorClass`
2. `runPhaseWithRetry()` calls `o.errorClassifier.Classify(result)` for classification
3. Classification determines retry behavior (exponential backoff, wait for rate limit, fail immediately)

The classifier is injected based on configured agent, ensuring proper handling of agent-specific error patterns.

**Data Race Fix (Unrelated to main feature)**

`internal/transcript/follow_test.go` had races where the Follower goroutine wrote to `bytes.Buffer` while tests read from it. Fixed with `syncBuffer`:
```go
type syncBuffer struct {
    mu  sync.RWMutex
    buf bytes.Buffer
}
```

### Architecture Impact

**Before:**
```
Orbit
├── claudeClient: claudeRunner (legacy interface)
├── rawClaudeClient: *claude.Client (for Comparator)
└── agent: agents.Agent
```

**After:**
```
Orbit
└── agent: agents.Agent (single unified interface)
        │
        ├── Used by: runPhase(), runPrePrompt(), runPostPrompt()
        └── Adapted by: AgentAdapter for Comparator
```

**Implications:**
- All agent invocations now go through one code path, simplifying debugging
- Error classification is pluggable per-agent via `AgentResolver`
- Future agents only need to implement `agents.Agent` interface

### Potential Issues

1. **Context Lifetime in Adapter**: The adapter captures context at construction. Long-lived adapters could use stale contexts. Mitigated by documenting "create per operation."

2. **AutoApprove Configuration**: Compare command explicitly sets `AutoApprove: true` when getting agent. If this default changes in registry, comparison would break.

3. **TestAgent Error Semantics**: Property tests in `generators_test.go` needed updating to expect errors for error scenarios. Any new property tests must account for this.

---

## Completeness Assessment

### Fully Implemented

| Requirement | Status | Evidence |
|-------------|--------|----------|
| [1.1] Replace claudeClient.RunPhase() | Complete | `orbit.go` uses `agent.Run()`/`Resume()` |
| [1.2] Pass same parameters through RunOptions | Complete | Prompt, WorkDir, SessionID all passed |
| [1.3] Preserve error classification | Complete | `errorClassifier.Classify()` unchanged |
| [1.4] Preserve session-invalid fallback | Complete | `isSessionInvalidError()` check preserved |
| [1.5] Maintain test behavior | Complete | All tests pass |
| [2.1-2.9] Remove legacy code | Complete | `internal/claude/` deleted, interface removed |
| [3.1-3.4] AgentAdapter | Complete | `comparison/adapter.go` with tests |
| [4.1-4.8] Skipped test migration | Complete | All 4 tests enabled with FakeClock |
| [5.1-5.5] mockClaudeClient migration | Complete | Type removed, all tests migrated |
| [6.1] Coverage not decreased | Complete | 27.1% > 22.4% baseline |
| [6.2-6.4] Tests/lint/race pass | Complete | Verified via `make test`, `make lint`, `-race` |

### Partially Implemented

None identified.

### Missing

None identified.

### Documentation Gap Found During Review

- CLAUDE.md referenced deleted `claude/client.go` and `claude/paths.go` - **Fixed** during this review
