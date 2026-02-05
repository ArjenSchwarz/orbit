# Legacy Claude Removal Design

## Overview

This design describes how to remove the legacy `claudeRunner` interface from `internal/orbit/orbit.go` and migrate all tests to use `testutil.TestAgent`. The migration involves three main components:

1. **runPhase() Migration** - Replace `claudeClient.RunPhase()` calls with `agent.Run()`/`agent.Resume()` calls
2. **Comparator Adapter** - Create an adapter to satisfy the `promptRunner` interface using `agents.Agent`
3. **Test Migration** - Migrate all `mockClaudeClient` tests to `testutil.TestAgent` with `FakeClock`

The design follows patterns already established in `runPostPrompt()` and `runPrePrompt()`, which already use the agent interface successfully.

## Architecture

### Current State

```
┌─────────────────────────────────────────────────────────────────┐
│                           Orbit                                  │
├─────────────────────────────────────────────────────────────────┤
│  claudeClient: claudeRunner     ← Legacy interface              │
│  rawClaudeClient: *claude.Client ← Used by Comparator           │
│  agent: agents.Agent            ← Modern interface              │
├─────────────────────────────────────────────────────────────────┤
│  runPhase()       → claudeClient.RunPhase()     ✗ Legacy        │
│  runPrePrompt()   → agent.Run()/Resume()        ✓ Migrated      │
│  runPostPrompt()  → agent.Run()/Resume()        ✓ Migrated      │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
              ┌───────────────────────────────┐
              │   comparison.Comparator        │
              │   promptRunner: rawClaudeClient │
              └───────────────────────────────┘
```

### Target State

```
┌─────────────────────────────────────────────────────────────────┐
│                           Orbit                                  │
├─────────────────────────────────────────────────────────────────┤
│  agent: agents.Agent            ← Single interface              │
├─────────────────────────────────────────────────────────────────┤
│  runPhase()       → agent.Run()/Resume()        ✓ Migrated      │
│  runPrePrompt()   → agent.Run()/Resume()        ✓ Already done  │
│  runPostPrompt()  → agent.Run()/Resume()        ✓ Already done  │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
              ┌───────────────────────────────┐
              │   comparison.Comparator        │
              │   promptRunner: AgentAdapter   │
              └───────────────────────────────┘
                              │
                              ▼
              ┌───────────────────────────────┐
              │        AgentAdapter            │
              │   agent: agents.Agent          │
              │   ctx: context.Context         │
              │   workDir: string              │
              └───────────────────────────────┘
```

## Components and Interfaces

### 1. AgentAdapter (New Component)

**Location:** `internal/comparison/adapter.go`

The adapter is placed in `internal/comparison/` rather than `internal/orbit/` because:
1. It's used by `comparison.Comparator` to satisfy the `promptRunner` interface
2. Both `internal/orbit/` and `cmd/orbit/compare.go` need to create adapters
3. Placing it in `comparison` avoids circular imports and keeps adapter near its consumer

The adapter wraps `agents.Agent` to satisfy the `promptRunner` interface required by `comparison.Comparator`.

```go
// AgentAdapter adapts agents.Agent to the promptRunner interface.
// This allows Comparator to use any agent implementation.
//
// Context Lifetime: The adapter captures the context at construction time.
// Create a new adapter for each operation rather than caching it across runs,
// as the context may become stale or cancelled.
type AgentAdapter struct {
    agent   agents.Agent
    ctx     context.Context
    workDir string
}

// NewAgentAdapter creates a new adapter wrapping the given agent.
func NewAgentAdapter(agent agents.Agent, ctx context.Context, workDir string) *AgentAdapter {
    return &AgentAdapter{
        agent:   agent,
        ctx:     ctx,
        workDir: workDir,
    }
}

// RunCustomPrompt implements the promptRunner interface by delegating to agent.Run().
// Note: AutoApprove is controlled by the agent's config, not RunOptions.
// The agent passed to the adapter should already be configured with AutoApprove if needed.
func (a *AgentAdapter) RunCustomPrompt(prompt string) (*agents.RunResult, error) {
    opts := agents.RunOptions{
        Prompt:  prompt,
        WorkDir: a.workDir,
    }
    return a.agent.Run(a.ctx, opts)
}
```

**Rationale:** This follows Decision 4 (Use Adapter Pattern for Comparator). The adapter is minimal and testable independently.

**Traceability:** Requirements [3.1](requirements.md#3.1), [3.2](requirements.md#3.2), [3.3](requirements.md#3.3), [3.4](requirements.md#3.4)

### 2. runPhase() Migration

**Location:** `internal/orbit/orbit.go`

Replace the legacy `claudeClient.RunPhase()` calls with agent interface calls, following the pattern established in `runPostPrompt()`.

**Current code (lines 820, 840):**
```go
result, err := o.claudeClient.RunPhase(sessionID, isResume)
```

**Migrated code:**
```go
opts := agents.RunOptions{
    Prompt:    o.config.Command,
    WorkDir:   o.config.WorkingDir,
    SessionID: sessionID,
}

var result *agents.RunResult
var err error
if isResume {
    result, err = o.agent.Resume(o.shutdownCtx, sessionID, opts)
} else {
    result, err = o.agent.Run(o.shutdownCtx, opts)
}
```

**Traceability:** Requirements [1.1](requirements.md#1.1), [1.2](requirements.md#1.2), [1.3](requirements.md#1.3), [1.4](requirements.md#1.4)

### 3. Orbit Struct Changes

**Location:** `internal/orbit/orbit.go`

Remove the following fields from the `Orbit` struct:
- `claudeClient claudeRunner` (line 180)
- `rawClaudeClient *claude.Client` (line 193)

Remove the following from `New()`:
- Claude client initialization (lines 221-226)
- Assignment to `claudeClient` and `rawClaudeClient` fields

Update Comparator instantiation to use `AgentAdapter`:
```go
// Before
comparator := comparison.NewComparator(o.rawClaudeClient, o.config.CompareCommand)

// After
adapter := comparison.NewAgentAdapter(o.agent, o.shutdownCtx, o.config.WorkingDir)
comparator := comparison.NewComparator(adapter, o.config.CompareCommand)
```

**Traceability:** Requirements [2.1](requirements.md#2.1), [2.2](requirements.md#2.2), [2.3](requirements.md#2.3), [2.4](requirements.md#2.4), [2.9](requirements.md#2.9)

### 4. Interface Removal

**Location:** `internal/orbit/orbit.go`

Remove the `claudeRunner` interface definition (lines 74-80):
```go
// REMOVE THIS:
type claudeRunner interface {
    RunPhase(sessionID string, resume bool) (*agents.RunResult, error)
    RunCustomPrompt(prompt string) (*agents.RunResult, error)
    RunCustomPromptWithSession(prompt, sessionID string, resume bool) (*agents.RunResult, error)
}
```

**Traceability:** Requirement [2.1](requirements.md#2.1)

### 5. Update cmd/orbit/compare.go

**Location:** `cmd/orbit/compare.go`

The standalone `orbit compare` command also uses `claude.Client` directly. It must be updated to use the agent interface with `AgentAdapter`.

**Current code (line 123):**
```go
claudeClient := claude.NewClient(claude.Config{
    WorkingDir: workDir,
})
comparator := comparison.NewComparator(claudeClient, *compareCmd)
```

**Migrated code:**
```go
// Get the default agent (claude-code) with AutoApprove for non-interactive use
agent, err := agents.Get("claude-code", agents.AgentConfig{
    WorkDir:     workDir,
    AutoApprove: true, // Comparison runs non-interactively
})
if err != nil {
    return fmt.Errorf("failed to get agent: %w", err)
}

adapter := comparison.NewAgentAdapter(agent, ctx, workDir)
comparator := comparison.NewComparator(adapter, *compareCmd)
```

**Import path:** `github.com/arjenschwarz/orbit/internal/comparison`

**Traceability:** Requirements [2.5](requirements.md#2.5), [3.3](requirements.md#3.3)

### 6. File Deletions

**Files to delete:**
- `internal/claude/client.go` - Legacy Claude client implementation
- `internal/claude/client_test.go` - Tests for legacy client

**Files to verify before deletion:**
- Confirm `claude.Result`, `claude.SessionResult`, `claude.Config` types have no external dependencies beyond `internal/orbit/orbit.go` and `cmd/orbit/compare.go` (both will be migrated)

**Directory cleanup:**
- After deleting `client.go` and `client_test.go`, the `internal/claude/` directory will be empty and should be removed

**Traceability:** Requirements [2.5](requirements.md#2.5), [2.6](requirements.md#2.6), [2.7](requirements.md#2.7), [2.8](requirements.md#2.8), [1.6](requirements.md#1.6)

## Data Models

No new data models are introduced. The existing `agents.RunResult`, `agents.RunOptions`, and `agents.Agent` interface are reused.

The `AgentAdapter` is a simple wrapper struct with no persistent state.

## Error Handling

Error handling behavior is preserved exactly as-is:

1. **Error Classification** - Uses `o.errorClassifier.Classify()` which is agent-specific
2. **Session Invalid Fallback** - Detects via `isSessionInvalidError(result)` and retries with fresh session
3. **Retry Logic** - Handled by `runPhaseWithRetry()` which remains unchanged

The agent interface already returns classified errors via `RunResult.ErrorClass`, ensuring consistent behavior.

**Traceability:** Requirements [1.3](requirements.md#1.3), [1.4](requirements.md#1.4)

## Testing Strategy

### Test Categories

#### 1. AgentAdapter Unit Tests

**File:** `internal/comparison/adapter_test.go`

| Test | Purpose | Traceability |
|------|---------|--------------|
| `TestAgentAdapter_RunCustomPrompt` | Verify adapter delegates to agent.Run() correctly | [3.2](requirements.md#3.2) |
| `TestAgentAdapter_PassesWorkDir` | Verify working directory is passed | [3.2](requirements.md#3.2) |
| `TestAgentAdapter_PropagatesErrors` | Verify errors from agent are returned | [3.4](requirements.md#3.4) |

#### 2. Skipped Test Migration

These 4 tests currently use `mockClaudeClient` and are skipped due to real-time delays. After migration, they will use `testutil.TestAgent` with `FakeClock`.

| Test | Current Delay | After Migration | Traceability |
|------|---------------|-----------------|--------------|
| `TestRunPostPromptWithRetry_RetryableError_EventualSuccess` | ~3s | <100ms | [4.1](requirements.md#4.1) |
| `TestRunPostPromptWithRetry_MaxRetriesExceeded` | ~31s | <100ms | [4.2](requirements.md#4.2) |
| `TestRunPhaseWithRetry_RateLimitError` | ~60s | <100ms | [4.3](requirements.md#4.3) |
| `TestRunPhaseWithRetry_OverloadedError` | ~30s | <100ms | [4.4](requirements.md#4.4) |

**Migration Pattern:**
```go
// Before (using mockClaudeClient with real delays)
t.Skip("disabled: test uses real delays")
mock := &mockClaudeClient{
    runPhaseFunc: func(sessionID string, resume bool) (*agents.RunResult, error) {
        callCount++
        if callCount < 3 {
            return &agents.RunResult{IsError: true, Stderr: "timeout"}, errors.New("timeout")
        }
        return &agents.RunResult{SessionID: "success"}, nil
    },
}

// After (using testutil.TestAgent with FakeClock)
clock := testutil.NewFakeClock(time.Now())
scenario := testutil.NewScenario().
    RetryableError("timeout").
    RetryableError("timeout").
    Success("success-session", 0.05).
    Build()
agent := testutil.NewTestAgent(t, "mock", scenario, testutil.WithClock(clock))
t.Cleanup(func() { agent.AssertAllConsumed(t) })

orbit := orbithelpers.CreateTestOrbit(t,
    orbithelpers.WithAgent(agent),
    orbithelpers.WithOrbitClock(clock),
)

err := orbit.Run()
require.NoError(t, err)
clock.AssertSleeps(t, []time.Duration{time.Second, 2*time.Second})
```

**Traceability:** Requirements [4.5](requirements.md#4.5), [4.6](requirements.md#4.6), [4.7](requirements.md#4.7), [4.8](requirements.md#4.8)

#### 3. Remaining mockClaudeClient Test Migration

Additional tests in `orbit_test.go` and `integration_test.go` using `mockClaudeClient`:

| File | Test Count | Lines | Migration Approach |
|------|------------|-------|-------------------|
| `orbit_test.go` | 9 tests | 211, 251, 290, 327, 518, 566, 621, 1543, 1588 | Replace with ScenarioBuilder patterns |
| `integration_test.go` | 1 test | 521 | Replace with ScenarioBuilder patterns |

All migrated tests will:
- Use `testutil.TestAgent` with appropriate scenarios
- Add `t.Cleanup(func() { agent.AssertAllConsumed(t) })`
- Pass with `go test -race`
- **Verify assertion parity**: Each migrated test must make equivalent assertions to the original `mockClaudeClient` test

**Traceability:** Requirements [5.1](requirements.md#5.1), [5.2](requirements.md#5.2), [5.3](requirements.md#5.3), [5.4](requirements.md#5.4), [5.5](requirements.md#5.5)

#### 4. Coverage Verification

**Current baseline:** 22.4% for `internal/orbit/`

**Pre-migration baseline:**
```bash
go test -coverprofile=baseline.out ./internal/orbit/...
go tool cover -func=baseline.out | grep total
```

**Post-migration verification:**
```bash
go test -coverprofile=final.out ./internal/orbit/...
go tool cover -func=final.out | grep total
```

Coverage of remaining code must not decrease below 22.4%. Note that deleting `internal/claude/client.go` (which has its own tests) will not affect `internal/orbit/` coverage since they are separate packages.

**Traceability:** Requirements [6.1](requirements.md#6.1), [6.2](requirements.md#6.2), [6.3](requirements.md#6.3), [6.4](requirements.md#6.4)

### Verification Commands

```bash
# Run all tests
make test

# Run tests with race detector
go test -race ./internal/orbit/...

# Run linter
make lint

# Verify no imports of removed types
grep -r "claude\.Result\|claude\.SessionResult\|claude\.Config" --include="*.go" | grep -v internal/claude/

# Verify no package imports of internal/claude (should only show orbit.go and compare.go before migration)
grep -r '"github.com/arjenschwarz/orbit/internal/claude"' --include="*.go"
```

## Technical Notes

### Session ID Contract

The `agents.Agent.Resume()` method takes `sessionID` as both a function argument and potentially in `opts.SessionID`. The contract is:

```go
// Resume continues an existing session.
func (a *Agent) Resume(ctx context.Context, sessionID string, opts RunOptions) (*RunResult, error) {
    opts.SessionID = sessionID  // First argument is authoritative
    return a.execute(ctx, opts, true)
}
```

The first argument (`sessionID`) is **authoritative**. It is copied to `opts.SessionID` before execution. This matches the pattern used in the existing `runPostPrompt()` migration.

### AutoApprove Behavior

**Important:** `AutoApprove` is controlled by `agents.AgentConfig.AutoApprove`, NOT by `RunOptions`.

While `RunOptions.AutoApprove` exists in the struct, it is **not used by any agent implementation**. All agents (claude-code, codex, kiro, copilot) check `a.config.AutoApprove` when building CLI arguments:

```go
// From claudecode/agent.go:165-167
if a.config.AutoApprove {
    args = append(args, "--dangerously-skip-permissions")
}
```

This means the adapter **cannot** control AutoApprove - it must be set when the agent is created.

For comparison prompts to run non-interactively:
- In `Orbit`: The agent is already configured with `AutoApprove: true` from the config file (default is true per CLAUDE.md)
- In `cmd/orbit/compare.go`: Must explicitly set `AutoApprove: true` when creating the agent

The design explicitly specifies `AutoApprove: true` in the migrated `cmd/orbit/compare.go` code.

### Error Handling Parity

Both `claude.Client` and `claudecode.Agent` handle `exec.ExitError` identically:

```go
// From claudecode/agent.go lines 217-223
if err != nil {
    if exitErr, ok := err.(*exec.ExitError); ok {
        result.ExitCode = exitErr.ExitCode()
    } else {
        result.ExitCode = -1
    }
}
```

The agent implementations also populate `result.IsError`, `result.Stderr`, and parse JSON output in the same manner as the legacy client. Error classification happens in `runPhaseWithRetry()` after the call, using the agent-specific error classifier.

## Implementation Order

The implementation should proceed in this order to maintain a working codebase at each step:

1. **Create AgentAdapter** - Add `internal/comparison/adapter.go` with `AgentAdapter` type
2. **Write AgentAdapter unit tests** - Add `internal/comparison/adapter_test.go` to validate adapter behavior
3. **Migrate runPhase()** - Replace claudeClient calls with agent calls
4. **Update Comparator usage in Orbit** - Switch to AgentAdapter
5. **Update cmd/orbit/compare.go** - Switch from claude.Client to agent + AgentAdapter
6. **Migrate tests one by one** - Convert mockClaudeClient tests to TestAgent (verify assertion parity)
7. **Remove mockClaudeClient** - Delete the type after all tests are migrated
8. **Remove legacy interface** - Delete claudeRunner interface
9. **Remove legacy fields** - Delete claudeClient and rawClaudeClient from Orbit
10. **Delete legacy files** - Remove client.go, client_test.go, and internal/claude/ directory
11. **Verify coverage** - Ensure no regression below 22.4%
