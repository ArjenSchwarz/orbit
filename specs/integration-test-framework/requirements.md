# Integration Test Framework Requirements

## Introduction

This specification defines the requirements for a test framework that enables integration testing of Orbit's orchestration layer without invoking real AI agents (Claude Code, Codex, Kiro, Copilot, OpenCode). The framework provides mock agents with configurable behaviors, call recording for verification, and helpers for testing complex multi-phase scenarios including variant execution.

The framework will live in `internal/testutil/` and provide:
- A `TestAgent` implementing the full `agents.Agent` interface
- A fluent `ScenarioBuilder` for defining agent response sequences
- Call recording and assertion utilities
- Fixtures for tasks files, configs, and test setup with dependency injection
- Fake clock for deterministic timing tests
- Property-based testing support with rapid generators

All existing tests in `orbit_test.go` and `integration_test.go` will be migrated to use this framework.

---

## Requirements

### 1. Test Agent Implementation

**User Story:** As a developer, I want a mock agent that implements the full Agent interface, so that I can test orchestration without invoking real CLI tools.

**Acceptance Criteria:**

1. <a name="1.1"></a>The system SHALL provide a `TestAgent` type that implements all 8 methods of the `agents.Agent` interface
2. <a name="1.2"></a>The `TestAgent` SHALL return configurable responses from `Run()` and `Resume()` based on a pre-defined scenario
3. <a name="1.3"></a>The `TestAgent` SHALL record all calls to `Run()` and `Resume()` for later verification
4. <a name="1.4"></a>The `TestAgent` SHALL support configurable values for `Name()`, `CLICommand()`, `IsInstalled()`, and `Version()`
5. <a name="1.5"></a>The `TestAgent` SHALL be safe for concurrent use when testing parallel variant execution, specifically: multiple goroutines MAY call `Run()`/`Resume()` concurrently and must pass `go test -race`
6. <a name="1.6"></a>The `TestAgent` SHALL implement `SessionExporter` interface when `WithSessionExport(path)` option is provided, for testing Kiro-style export flows
7. <a name="1.7"></a>The `TestAgent` SHALL require a `testing.TB` parameter for failure reporting

### 2. Scenario Builder

**User Story:** As a developer, I want a fluent API to define agent behavior sequences, so that I can easily set up complex multi-phase test scenarios.

**Acceptance Criteria:**

1. <a name="2.1"></a>The system SHALL provide a `ScenarioBuilder` type with a fluent API for defining call responses
2. <a name="2.2"></a>The `ScenarioBuilder` SHALL support `Success(sessionID, cost)` for successful executions
3. <a name="2.3"></a>The `ScenarioBuilder` SHALL support `RetryableError(message)` for transient errors
4. <a name="2.4"></a>The `ScenarioBuilder` SHALL support `FatalError(message)` for non-recoverable errors
5. <a name="2.5"></a>The `ScenarioBuilder` SHALL support `SessionInvalid()` for expired session errors
6. <a name="2.6"></a>The `ScenarioBuilder` SHALL support `RateLimitWait(duration)` for usage limit errors with wait times
7. <a name="2.7"></a>The `ScenarioBuilder` SHALL support `WithDelay(duration)` to simulate execution time
8. <a name="2.8"></a>The `ScenarioBuilder` SHALL support `WithOutput(output, stderr)` to set custom agent output
9. <a name="2.9"></a>The `ScenarioBuilder` SHALL support `WithCost(metrics)` for detailed cost tracking verification
10. <a name="2.10"></a>The `ScenarioBuilder` SHALL return responses in sequence order, one per `Run()`/`Resume()` call
11. <a name="2.11"></a>WHEN more calls are made than responses defined, THEN the `TestAgent` SHALL call `t.Fatalf()` with a descriptive message including: call index, expected count, prompt received, and call stack
12. <a name="2.12"></a>The scenario configuration SHALL be immutable after `Build()` is called; concurrent calls to `Run()`/`Resume()` SHALL safely read from the scenario

### 3. Call Recording and Assertions

**User Story:** As a developer, I want to verify what calls were made to the agent, so that I can assert correct orchestration behavior.

**Acceptance Criteria:**

1. <a name="3.1"></a>The system SHALL provide a `Recorder` type that captures all agent calls
2. <a name="3.2"></a>The `Recorder` SHALL track for each call: method name (Run/Resume), options, session ID, context, timestamp, and call index
3. <a name="3.3"></a>The `Recorder` SHALL provide `CallCount()` returning total number of calls
4. <a name="3.4"></a>The `Recorder` SHALL provide `Calls()` returning a copy of all recorded `AgentCall` structs (copy-on-read for thread safety)
5. <a name="3.5"></a>The `Recorder` SHALL provide `CallsWithPrompt(pattern)` returning calls matching a prompt regex
6. <a name="3.6"></a>The `Recorder` SHALL provide `PhasePromptCalls()` returning calls matching `/next-task --phase`
7. <a name="3.7"></a>The `Recorder` SHALL provide `AssertCallOrder(t, ...patterns)` for verifying call sequence
8. <a name="3.8"></a>The `Recorder` SHALL provide `AssertCallCount(t, expected)` for verifying total calls
9. <a name="3.9"></a>The `Recorder` SHALL be safe for concurrent access: recording uses mutex, reading returns copies
10. <a name="3.10"></a>All assertion methods SHALL produce descriptive failure messages including: expected vs actual, call index, and relevant context

### 4. Test Fixtures

**User Story:** As a developer, I want helper functions to set up common test scenarios, so that I can write tests quickly without boilerplate.

**Acceptance Criteria:**

1. <a name="4.1"></a>The system SHALL provide `CreateTasksFile(t, phases)` that creates a minimal tasks.md with N phases in a temp directory
2. <a name="4.2"></a>The system SHALL provide `CreateConfig(t, options)` that creates an `.orbit.yaml` with specified options
3. <a name="4.3"></a>The system SHALL provide `CreateTestOrbit(t, options...)` that returns a configured `*Orbit` ready for testing, using functional options pattern
4. <a name="4.4"></a>The `CreateTestOrbit` SHALL accept `WithAgent(agent)` option for single-agent tests
5. <a name="4.5"></a>The `CreateTestOrbit` SHALL accept `WithAgents(map[string]Agent)` option for multi-variant tests with different agents per variant
6. <a name="4.6"></a>The `CreateTestOrbit` SHALL accept `WithRuneClient(client)` option for injecting mock rune client
7. <a name="4.7"></a>The `CreateTestOrbit` SHALL accept `WithClock(clock)` option for injecting fake clock
8. <a name="4.8"></a>The fixtures SHALL use `t.TempDir()` for automatic cleanup
9. <a name="4.9"></a>The fixtures SHALL use `t.Helper()` for accurate error line reporting
10. <a name="4.10"></a>The system SHALL provide `CreateRuneClient(t, tasksFile)` that returns a mock rune client with phase data

### 5. Dependency Injection for Variant Testing

**User Story:** As a developer, I want to inject multiple test agents for variant testing without modifying global state, so that tests can run in parallel safely.

**Acceptance Criteria:**

1. <a name="5.1"></a>The system SHALL support injecting agents via `CreateTestOrbit` options instead of modifying global registry
2. <a name="5.2"></a>The `WithAgents(map[string]Agent)` option SHALL allow specifying different agents for each variant by name
3. <a name="5.3"></a>The system SHALL provide composable behavior functions: `SuccessfulRun(cost)`, `RetryableError(msg)`, `FatalError(msg)`
4. <a name="5.4"></a>The system SHALL provide `FailAfterN(n, failBehavior, successBehavior)` for retry scenario testing
5. <a name="5.5"></a>The system SHALL provide `WithCallRecorder(recorder, behavior)` for wrapping behaviors with recording
6. <a name="5.6"></a>All variant testing helpers SHALL be safe for parallel test execution without any global state modification

### 6. Fake Clock for Deterministic Timing

**User Story:** As a developer, I want controllable time in tests, so that I can test timeout and delay logic without waiting.

**Acceptance Criteria:**

1. <a name="6.1"></a>The system SHALL provide a `FakeClock` type that implements a `Clock` interface
2. <a name="6.2"></a>The `FakeClock` SHALL provide `Now()` returning the controlled current time
3. <a name="6.3"></a>The `FakeClock` SHALL provide `Advance(duration)` to move time forward
4. <a name="6.4"></a>The `FakeClock` SHALL provide `Sleep(duration)` that returns immediately but records the sleep request
5. <a name="6.5"></a>The `TestAgent` SHALL accept an optional clock via `WithClock(clock)` option for delay simulation
6. <a name="6.6"></a>WHEN a scenario specifies `WithDelay(d)` and a fake clock is provided, THEN the agent SHALL call `clock.Sleep(d)` instead of real sleep
7. <a name="6.7"></a>The `FakeClock` documentation SHALL clearly state its scope limitation: it provides time control for `Now()` and `Sleep()` only; timer-based code (`time.After`, `time.NewTimer`) is not supported in MVP

### 7. Property-Based Testing Support

**User Story:** As a developer, I want to generate random agent behaviors for fuzz testing, so that I can discover edge cases in the orchestration logic.

**Acceptance Criteria:**

1. <a name="7.1"></a>The system SHALL provide rapid generators for `agents.RunResult` with valid random values
2. <a name="7.2"></a>The system SHALL provide rapid generators for `agents.CostMetrics` with valid random values
3. <a name="7.3"></a>The system SHALL provide rapid generators for error classifications (retryable, fatal, session invalid)
4. <a name="7.4"></a>The system SHALL provide a `RandomScenario(length)` generator that produces valid scenario sequences
5. <a name="7.5"></a>The generators SHALL produce values that satisfy the following invariants:
   - `RunResult.SessionID` is non-empty when `IsError` is false
   - `RunResult.ExitCode` is 0 for success, non-zero for errors
   - `CostMetrics.CostUSD` is non-negative
   - Error classifications match their corresponding exit codes and error messages
6. <a name="7.6"></a>The system SHALL provide at least two property tests demonstrating usage:
   - Orchestration handles any valid error sequence without panicking
   - Retry count for retryable errors is bounded by maxRetries

### 8. Documentation

**User Story:** As a developer, I want clear documentation on how to use the test framework, so that I can write effective tests without studying the implementation.

**Acceptance Criteria:**

1. <a name="8.1"></a>The system SHALL provide a `doc.go` file with package-level documentation including usage examples
2. <a name="8.2"></a>The documentation SHALL include examples for: basic scenario tests, error injection, variant testing, and property-based tests
3. <a name="8.3"></a>The system SHALL add a "Testing" section to `CLAUDE.md` that references the testutil package
4. <a name="8.4"></a>The CLAUDE.md section SHALL include quick-start examples for common test patterns
5. <a name="8.5"></a>All exported types and functions SHALL have GoDoc comments explaining their purpose and usage

### 9. Migration of Existing Tests

**User Story:** As a maintainer, I want existing tests that use mock agents migrated to the new framework, so that there is one consistent mocking approach.

**Acceptance Criteria:**

1. <a name="9.1"></a>All tests in `internal/orbit/orbit_test.go` that use `mockAgent` or `mockClaudeClient` SHALL be migrated to use the new testutil package
2. <a name="9.2"></a>All tests in `internal/orbit/integration_test.go` SHALL be migrated to use the new testutil package
3. <a name="9.3"></a>The existing `mockAgent` and `mockClaudeClient` types SHALL be removed after migration
4. <a name="9.4"></a>All migrated tests SHALL pass with `go test -race`
5. <a name="9.5"></a>Test coverage SHALL not decrease after migration (verified via `go test -coverprofile`)
6. <a name="9.6"></a>Simple unit tests (e.g., `TestConfig_Struct`) that don't require agent mocking SHALL NOT be modified

### 10. Test Coverage Scenarios

**User Story:** As a maintainer, I want the framework to enable testing of all key orchestration scenarios, so that the test suite is comprehensive.

**Acceptance Criteria:**

1. <a name="10.1"></a>The framework SHALL enable testing of session management: pre-prompt continuation, phase resume, session invalid recovery
2. <a name="10.2"></a>The framework SHALL enable testing of error handling: retryable errors with backoff, fatal errors, max retries exceeded
3. <a name="10.3"></a>The framework SHALL enable testing of hook execution order: pre-command → pre-prompt → phases → post-prompt → post-command
4. <a name="10.4"></a>The framework SHALL enable testing of variant execution: sequential, parallel with semaphore, different agents per variant
5. <a name="10.5"></a>The framework SHALL enable testing of cancellation: graceful shutdown between phases, context cancellation in parallel variants
6. <a name="10.6"></a>The framework SHALL enable testing of rate limit wait: parsing reset time, waiting until reset, resuming execution

### 11. Framework Self-Testing

**User Story:** As a maintainer, I want the test framework itself to have tests, so that I can be confident in its correctness.

**Acceptance Criteria:**

1. <a name="11.1"></a>The testutil package SHALL have unit tests for ScenarioBuilder behavior
2. <a name="11.2"></a>The testutil package SHALL have unit tests for Recorder concurrency safety
3. <a name="11.3"></a>The testutil package SHALL have unit tests for FakeClock behavior
4. <a name="11.4"></a>All testutil tests SHALL pass with `go test -race`
