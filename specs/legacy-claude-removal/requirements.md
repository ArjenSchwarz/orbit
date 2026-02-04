# Legacy Claude Removal Requirements

## Introduction

This feature completes the test migration started in the integration-test-framework spec by removing the legacy `claudeRunner` interface from `internal/orbit/orbit.go` and migrating all remaining tests to use `testutil.TestAgent`.

Currently, two code paths still use the legacy `claudeClient`:
1. The `runPhase()` method uses `claudeClient.RunPhase()` instead of the agent interface
2. The `comparison.Comparator` uses `rawClaudeClient.RunCustomPrompt()` for variant comparison

This prevents tests from using `FakeClock` for deterministic timing, forcing 4 tests to be skipped because they require real-time delays of 3-60 seconds. Additionally, there are 10+ tests using `mockClaudeClient` that should be migrated to `testutil.TestAgent`.

After this migration:
- All agent invocations will go through the unified `agents.Agent` interface
- The `internal/claude/client.go` and `internal/claude/client_test.go` files will be removed
- All skipped tests will be enabled with fast, deterministic execution using `FakeClock`
- The codebase will have reduced complexity and improved consistency

## Requirements

### 1. Replace claudeRunner with Agent Interface

**User Story:** As a developer, I want the `runPhase()` method to use the `agents.Agent` interface instead of the legacy `claudeRunner` interface, so that all agent invocations follow the same code path and can be tested with `testutil.TestAgent`.

**Acceptance Criteria:**

1. <a name="1.1"></a>The system SHALL replace `o.claudeClient.RunPhase()` calls in `runPhase()` with calls to `o.agent.Run()` or `o.agent.Resume()` based on the `isResume` parameter
2. <a name="1.2"></a>The system SHALL pass the same prompt, working directory, and session ID through `agents.RunOptions` as the current `claudeClient` implementation
3. <a name="1.3"></a>The system SHALL preserve the existing error classification behavior (retryable, fatal, session-invalid, rate-limit-wait)
4. <a name="1.4"></a>The system SHALL preserve the session-invalid fallback logic that retries with a fresh session when session errors occur
5. <a name="1.5"></a>The system SHALL maintain identical behavior for all existing passing tests after the migration
6. <a name="1.6"></a>The system SHALL verify that `claude.Result`, `claude.SessionResult`, and `claude.Config` types have no external dependencies outside `internal/claude/` before deletion

### 2. Remove Legacy claudeRunner Interface

**User Story:** As a maintainer, I want the legacy `claudeRunner` interface and related code removed from the codebase, so that there is a single, consistent way to invoke agents.

**Acceptance Criteria:**

1. <a name="2.1"></a>The system SHALL remove the `claudeRunner` interface definition from `internal/orbit/orbit.go`
2. <a name="2.2"></a>The system SHALL remove the `claudeClient` field from the `Orbit` struct
3. <a name="2.3"></a>The system SHALL remove the `rawClaudeClient` field from the `Orbit` struct
4. <a name="2.4"></a>The system SHALL remove the Claude client initialization from the `New()` function
5. <a name="2.5"></a>The system SHALL delete the `internal/claude/client.go` file entirely
6. <a name="2.6"></a>The system SHALL delete the `internal/claude/` directory after all files are removed
7. <a name="2.7"></a>The system SHALL update `cmd/orbit/compare.go` to use the agent interface with `AgentAdapter` instead of `claude.Client`
8. <a name="2.8"></a>The system SHALL delete `internal/claude/client_test.go` along with `client.go`
9. <a name="2.9"></a>The system SHALL update `NewOrbit()` to remove initialization of `claudeClient` and `rawClaudeClient` fields

### 3. Migrate Comparator to Use Agent Adapter

**User Story:** As a developer, I want the `comparison.Comparator` to work without `rawClaudeClient`, so that the legacy Claude client can be fully removed.

**Acceptance Criteria:**

1. <a name="3.1"></a>The system SHALL create an adapter type that wraps `agents.Agent` to satisfy the `promptRunner` interface required by `Comparator`
2. <a name="3.2"></a>The adapter SHALL implement `RunCustomPrompt(prompt string) (*agents.RunResult, error)` by delegating to `agent.Run()` with appropriate options
3. <a name="3.3"></a>The system SHALL update `Orbit` to pass the adapter to `comparison.NewComparator()` instead of `rawClaudeClient`
4. <a name="3.4"></a>The adapter SHALL preserve the existing comparison behavior (prompt execution, result handling)

### 4. Migrate Skipped Tests to testutil.TestAgent

**User Story:** As a developer, I want the 4 skipped tests migrated to use `testutil.TestAgent` with `FakeClock`, so that they run quickly and deterministically without real-time delays.

**Acceptance Criteria:**

1. <a name="4.1"></a>The test `TestRunPostPromptWithRetry_RetryableError_EventualSuccess` SHALL be migrated to use `testutil.TestAgent` and `FakeClock`
2. <a name="4.2"></a>The test `TestRunPostPromptWithRetry_MaxRetriesExceeded` SHALL be migrated to use `testutil.TestAgent` and `FakeClock`
3. <a name="4.3"></a>The test `TestRunPhaseWithRetry_RateLimitError` SHALL be migrated to use `testutil.TestAgent` and `FakeClock`
4. <a name="4.4"></a>The test `TestRunPhaseWithRetry_OverloadedError` SHALL be migrated to use `testutil.TestAgent` and `FakeClock`
5. <a name="4.5"></a>All migrated tests SHALL have their `t.Skip()` calls removed
6. <a name="4.6"></a>All migrated tests SHALL execute in under 1 second each
7. <a name="4.7"></a>All migrated tests SHALL use `FakeClock.AssertSleeps()` to verify backoff timing behavior
8. <a name="4.8"></a>All migrated tests SHALL pass with `go test -race`

### 5. Migrate Remaining mockClaudeClient Tests

**User Story:** As a developer, I want all tests using `mockClaudeClient` migrated to `testutil.TestAgent`, so that the legacy mock type can be removed.

**Acceptance Criteria:**

1. <a name="5.1"></a>The system SHALL migrate all tests in `orbit_test.go` that use `mockClaudeClient` to use `testutil.TestAgent`
2. <a name="5.2"></a>The system SHALL migrate all tests in `integration_test.go` that use `mockClaudeClient` to use `testutil.TestAgent`
3. <a name="5.3"></a>The system SHALL remove the `mockClaudeClient` type definition from test files
4. <a name="5.4"></a>All migrated tests SHALL use `t.Cleanup()` with `agent.AssertAllConsumed(t)` to verify scenario exhaustion
5. <a name="5.5"></a>All migrated tests SHALL pass with `go test -race`

### 6. Verify Test Coverage

**User Story:** As a maintainer, I want test coverage to be maintained or improved after the migration, so that confidence in the code is not reduced.

**Acceptance Criteria:**

1. <a name="6.1"></a>The test coverage of remaining code (excluding deleted files) SHALL NOT decrease compared to the baseline before migration
2. <a name="6.2"></a>All tests SHALL pass when run with `make test`
3. <a name="6.3"></a>All tests SHALL pass when run with `make test` including the `-race` flag
4. <a name="6.4"></a>The linter SHALL report no new errors when run with `make lint`
