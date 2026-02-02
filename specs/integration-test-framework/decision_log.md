# Decision Log: Integration Test Framework

## Decision 1: Primary Testing Strategy - Scenario-Based Mock Agent

**Date**: 2025-02-02
**Status**: accepted

### Context

The integration test framework needs a strategy for mocking agent behavior. Three approaches were considered during the design phase: scenario-based mocking (pre-programmed response sequences), behavior-based mocking (pattern matching on inputs), and registry override with behavior functions.

### Decision

Use scenario-based mock agent as the primary testing approach, with registry override as a supplementary approach for variant testing.

### Rationale

The scenario-based approach provides the clearest test intent. Responses are defined in sequence order matching the expected execution flow (pre-prompt → phase 1 → phase 2 → post-prompt). This aligns with the existing `MockGit` pattern in the codebase which uses call recording with sequential expectations.

### Alternatives Considered

- **Behavior-based with matchers**: More flexible but less explicit about execution order - Rejected because orchestration tests need to verify specific call sequences
- **Registry override only**: Good for variants but requires more setup per test - Kept as supplementary approach for multi-agent variant scenarios

### Consequences

**Positive:**
- Highly readable test cases via fluent builder
- Call recording enables detailed verification
- Aligns with existing codebase patterns

**Negative:**
- Index-based call matching can be fragile if execution order changes unexpectedly
- Scenarios must be defined upfront (less flexible for dynamic tests)

---

## Decision 2: Include Fake Clock for Deterministic Timing

**Date**: 2025-02-02
**Status**: accepted

### Context

Tests involving delays, timeouts, and rate limit waits need to be both fast and deterministic. Using real time would make tests slow and potentially flaky.

### Decision

Include a fake clock implementation that allows tests to control time progression.

### Rationale

The orchestration layer has several time-dependent behaviors: exponential backoff for retries, rate limit wait parsing, agent timeout handling. Testing these with real time would require either long-running tests or skipping the tests entirely. A fake clock enables comprehensive testing of timing logic without slowdown.

### Alternatives Considered

- **Real time only**: Simpler implementation - Rejected because tests would be slow and flaky
- **Add later**: Defer fake clock to future iteration - Rejected because timing tests are critical for retry logic

### Consequences

**Positive:**
- Tests run quickly regardless of simulated delays
- Deterministic timing behavior, no flakiness
- Can test edge cases like very long waits

**Negative:**
- Additional complexity in TestAgent to support clock injection
- Tests must remember to use fake clock for timing scenarios

---

## Decision 3: Full Migration of Existing Tests

**Date**: 2025-02-02
**Status**: accepted

### Context

The existing test files `orbit_test.go` (1750 lines) and `integration_test.go` (1846 lines) contain custom mock implementations (`mockAgent`, `mockClaudeClient`) that duplicate functionality the new framework provides.

### Decision

Migrate all existing tests to use the new testutil package and remove the old mock implementations.

### Rationale

Having two mock implementations creates maintenance burden and inconsistency. The existing mocks are simpler but lack features like call recording and scenario building. Full migration ensures one consistent approach and removes 100+ lines of duplicate mock code.

### Alternatives Considered

- **Gradual migration**: New tests use framework, migrate old tests over time - Rejected because it leaves duplicate mocks indefinitely
- **Parallel coexistence**: Keep both approaches - Rejected because it creates inconsistency and maintenance burden

### Consequences

**Positive:**
- Single consistent testing approach
- Removes duplicate mock implementations
- All tests benefit from improved features

**Negative:**
- Larger initial implementation effort
- Risk of introducing bugs during migration

---

## Decision 4: Include Property-Based Testing with Rapid

**Date**: 2025-02-02
**Status**: accepted

### Context

The orchestration layer handles many combinations of agent responses, errors, and state transitions. Manual test cases may miss edge cases.

### Decision

Include rapid generators for agent types to enable property-based testing.

### Rationale

Property-based testing can discover edge cases that scenario-based tests miss. For example, testing that "orchestration handles any valid error sequence without panicking" is easier to express as a property than as exhaustive scenarios.

### Alternatives Considered

- **Scenario-based only**: Simpler, focus on known scenarios - Rejected because edge cases are important for orchestration reliability

### Consequences

**Positive:**
- Can discover unexpected edge cases
- Tests invariants rather than specific scenarios
- Aligns with Go testing best practices (project uses rapid elsewhere)

**Negative:**
- Generators require careful design to produce valid values
- Property test failures can be harder to debug

---

## Decision 5: Package doc.go Plus CLAUDE.md Section for Documentation

**Date**: 2025-02-02
**Status**: accepted

### Context

The test framework needs documentation that helps developers write tests. Options include standard Go docs, a separate guide, or integration with existing project docs.

### Decision

Provide package-level documentation in `doc.go` with examples, plus add a "Testing" section to `CLAUDE.md` with quick-start patterns.

### Rationale

Go developers expect to find usage examples in package documentation. Adding a CLAUDE.md section ensures the testing approach is visible when working on the project and provides a quick reference without needing to read full package docs.

### Alternatives Considered

- **Separate testing guide**: Full docs/testing-guide.md - Rejected because it fragments documentation
- **Doc.go only**: Standard Go documentation - Rejected because CLAUDE.md is the primary reference for project conventions

### Consequences

**Positive:**
- Documentation in standard Go location (doc.go)
- Quick reference in project's main instruction file
- Examples discoverable via `go doc`

**Negative:**
- Two places to update when patterns change

---

## Decision 6: Use t.Fatalf Instead of Panic for Unexpected Calls

**Date**: 2025-02-02
**Status**: accepted

### Context

When a test makes more calls to the TestAgent than scenarios are defined, the framework needs to fail the test. The original proposal was to panic with a descriptive message. Design review (including external validation from Gemini, Codex, and Kiro) unanimously identified this as problematic.

### Decision

Use `t.Fatalf()` instead of `panic()` when unexpected calls occur.

### Rationale

Panics are atypical in Go tests. Using `t.Fatalf()`:
- Produces clear test failure output
- Allows other tests to continue in parallel
- Integrates properly with Go's test runner
- Provides consistent failure behavior across the framework

The TestAgent will require a `testing.TB` parameter to enable proper failure reporting.

### Alternatives Considered

- **Panic with descriptive message**: Original proposal - Rejected because panics disrupt parallel test execution and are harder to debug
- **t.Fatalf with optional Strict mode**: Provide panic as opt-in for stack traces - Rejected as unnecessary complexity; t.Fatalf is sufficient

### Consequences

**Positive:**
- Standard Go testing behavior
- Better parallel test experience
- Consistent with rest of codebase

**Negative:**
- TestAgent API requires testing.TB parameter (minor inconvenience)

---

## Decision 7: Dependency Injection Instead of Global Registry Override

**Date**: 2025-02-02
**Status**: accepted

### Context

The original design proposed modifying the global agent registry to inject test agents, using `t.Cleanup()` to restore original factories. Design review identified this as a critical flaw - global state modification creates test pollution risks and makes parallel testing fragile.

### Decision

Use dependency injection via `CreateTestOrbit` options instead of registry modification.

### Rationale

Dependency injection:
- Eliminates global state modification
- Makes tests fully isolated
- Allows true parallel execution without coordination
- Follows established Go testing patterns

The `CreateTestOrbit(t, WithAgents(map[string]Agent{...}))` pattern allows injecting multiple agents for variant testing without touching global state.

### Alternatives Considered

- **Registry override with unique names**: Use unique agent names and t.Cleanup - Rejected because concurrent test interference is still possible, and requires name coordination
- **Registry override with locks**: Add mutex to registry operations - Rejected because it adds complexity and still has global state

### Consequences

**Positive:**
- Fully isolated tests
- Safe parallel execution
- No global state modification
- Simpler mental model

**Negative:**
- Requires Orbit to accept injected agent resolver
- Slightly more setup in CreateTestOrbit implementation

---

## Decision 8: Concrete Thread-Safety Specifications

**Date**: 2025-02-02
**Status**: accepted

### Context

Design review identified that "thread-safe" claims were too vague without specifying exact concurrency guarantees and how they would be verified.

### Decision

Add concrete thread-safety specifications to requirements:
- All concurrent code must pass `go test -race`
- Recorder uses mutex for writes, copy-on-read for reads
- Scenario configuration is immutable after Build()
- Explicitly document which operations can be concurrent

### Rationale

Specific guarantees are testable and verifiable. "Thread-safe" without definition is meaningless - specifying primitives (mutex, copy-on-read) and verification method (`go test -race`) makes requirements actionable.

### Alternatives Considered

- **Leave vague**: Trust implementers to "do the right thing" - Rejected because it leads to subtle bugs
- **Over-specify with channel-based design**: More complex concurrency model - Rejected as unnecessary for this use case

### Consequences

**Positive:**
- Clear verification criteria
- Implementers know exactly what to build
- Reviewers know exactly what to check

**Negative:**
- Slightly constrains implementation choices (acceptable)

---

## Decision 9: Add Clock Interface to Orbit for Testable Timing

**Date**: 2025-02-02
**Status**: accepted

### Context

The FakeClock in the test framework is only useful if Orbit itself uses `Clock.Sleep()` instead of `time.Sleep()` directly. Without this change, timing tests (retry backoff, rate limit waits) would require real delays.

### Decision

Add a `Clock` interface to Orbit's Config and use it for all sleep operations.

### Rationale

Orbit's retry logic uses exponential backoff (1s, 2s, 4s, 8s, 16s). Testing this with real time would make tests slow and potentially flaky. The Clock interface provides a seam for injecting FakeClock in tests.

The change is backward-compatible: `Clock` defaults to `RealClock{}` which uses `time.Sleep()`.

### Alternatives Considered

- **Document limitation only**: FakeClock for TestAgent delays only - Rejected because it prevents testing the core retry logic
- **Use external clock library**: github.com/benbjohnson/clock - Rejected because it's an external dependency for a simple interface

### Consequences

**Positive:**
- Enables deterministic timing tests
- Tests run fast regardless of simulated delays
- Can verify exact backoff durations

**Negative:**
- Small production code change required
- All `time.Sleep()` calls in Orbit must be changed to `o.clock.Sleep()`

---

## Decision 10: Consolidate to ScenarioBuilder Only (Remove AgentBehavior)

**Date**: 2025-02-02
**Status**: accepted

### Context

The original design proposed two mechanisms for defining agent behavior: ScenarioBuilder (sequence-based) and AgentBehavior (function-based). Design review identified this as redundant and confusing.

### Decision

Remove AgentBehavior and use ScenarioBuilder exclusively. Add `Repeat(n)` and `Custom(fn)` methods to handle edge cases.

### Rationale

Having two approaches creates confusion about which to use. ScenarioBuilder provides:
- One mental model for developers
- Explicit test intent (forces thinking about exact call sequences)
- Alignment with existing `MockGit` pattern in codebase
- Easier debugging (clear which call in sequence failed)

The `Repeat(n)` modifier handles "multiple identical responses". The `Custom(fn)` escape hatch handles truly dynamic edge cases.

### Alternatives Considered

- **Keep both**: Different use cases for each - Rejected because overlap creates confusion
- **AgentBehavior only**: Functions are more flexible - Rejected because less explicit about expected call sequences

### Consequences

**Positive:**
- Single API to learn
- Simpler file structure (no behaviors.go)
- More explicit test intentions

**Negative:**
- Slightly more verbose for "always succeed" case (use SuccessScenario helper)

---

## Decision 11: Remove Context Storage from AgentCall

**Date**: 2025-02-02
**Status**: accepted

### Context

The original design stored `context.Context` in `AgentCall` for "context propagation testing". Design review identified this as an anti-pattern in Go - contexts should be passed explicitly, not stored.

### Decision

Do not store `context.Context` in AgentCall. Instead, extract and store specific values that tests might need: `HasDeadline` and `Deadline`.

### Rationale

Contexts are meant to be short-lived and carry cancelation signals. Storing them prevents garbage collection and violates Go best practices. Tests that need context information can use the extracted deadline values.

### Alternatives Considered

- **Store context**: Original proposal - Rejected as anti-pattern
- **Recorder hook**: `OnCall func(ctx, call)` - Rejected as unnecessary complexity

### Consequences

**Positive:**
- Follows Go best practices
- No risk of context leakage
- Still enables deadline verification tests

**Negative:**
- Cannot access other context values (acceptable - no use case identified)

---

## Decision 12: Add Scenario Exhaustion Verification

**Date**: 2025-02-02
**Status**: accepted

### Context

The design specifies that exceeding scenario responses calls `t.Fatalf()`. However, there was no mechanism to verify that all defined responses were consumed. A test could pass while leaving responses unused, masking bugs.

### Decision

Add `AssertAllConsumed(t)` method to TestAgent. Recommend using via `t.Cleanup()`.

### Rationale

Verifying all responses were consumed catches bugs where expected calls didn't happen. Using `t.Cleanup()` ensures the check runs even if the test fails early.

### Alternatives Considered

- **No verification**: Trust test to check call count - Rejected because it's easy to forget
- **Automatic via t.Cleanup in NewTestAgent**: Always verify - Rejected because some tests intentionally don't consume all responses

### Consequences

**Positive:**
- Catches missing calls
- Clean pattern via t.Cleanup()
- Optional (not forced on all tests)

**Negative:**
- Requires explicit call or cleanup registration

---
