---
references:
    - requirements.md
    - design.md
    - decision_log.md
---
# Integration Test Framework

## Pre-work (Orbit Changes)

- [ ] 1. Add Clock interface to Orbit <!-- id:h55dbl7 -->
  - Add Clock interface with Now() and Sleep() methods
  - Implement RealClock using time.Now() and time.Sleep()
  - Add Clock field to orbit.Config with RealClock default
  - Replace time.Sleep calls in retry logic with o.config.Clock.Sleep()
  - Stream: 1
  - Requirements: [6.5](requirements.md#6.5), [6.6](requirements.md#6.6), [9](requirements.md#9)

- [ ] 2. Add AgentResolver interface to Orbit <!-- id:h55dbl8 -->
  - Create AgentResolver interface with GetAgent(name, cfg) method
  - Implement registryResolver wrapping agents.Get()
  - Add AgentResolver field to orbit.Config with DefaultAgentResolver default
  - Replace agents.Get calls with config.AgentResolver.GetAgent()
  - Blocked-by: h55dbl7 (Add Clock interface to Orbit)
  - Stream: 1
  - Requirements: [5.1](requirements.md#5.1), [5.2](requirements.md#5.2)

## Core Framework

- [ ] 3. Define core types and interfaces <!-- id:h55dbl9 -->
  - Create internal/testutil/ package
  - Define Clock interface (matches Orbit's interface)
  - Define AgentCall struct with Index, Method, Options, SessionID, Timestamp, HasDeadline, Deadline fields
  - Define CallResponse struct with Result, Delay, ErrorClass, Output, Stderr, CustomFunc fields
  - Define Scenario struct with responses slice
  - Stream: 2
  - Requirements: [3.2](requirements.md#3.2), [2.12](requirements.md#2.12)

- [ ] 4. Implement Recorder with thread-safe call tracking <!-- id:h55dbla -->
  - Implement Recorder struct with mutex and calls slice
  - Implement record() with mutex lock
  - Implement CallCount() returning count
  - Implement Calls() returning copy of slice
  - Implement CallsWithPrompt(pattern) with regex matching
  - Implement PhasePromptCalls() filtering /next-task --phase
  - Blocked-by: h55dbl9 (Define core types and interfaces)
  - Stream: 2
  - Requirements: [3.1](requirements.md#3.1), [3.3](requirements.md#3.3), [3.4](requirements.md#3.4), [3.5](requirements.md#3.5), [3.6](requirements.md#3.6), [3.9](requirements.md#3.9)

- [ ] 5. Implement Recorder assertion methods <!-- id:h55dblb -->
  - Implement AssertCallCount(t, expected) with descriptive failure message
  - Implement AssertCallOrder(t, ...patterns) with regex matching and failure details
  - Implement formatCalls() helper for error messages
  - Blocked-by: h55dbla (Implement Recorder with thread-safe call tracking)
  - Stream: 2
  - Requirements: [3.7](requirements.md#3.7), [3.8](requirements.md#3.8), [3.10](requirements.md#3.10)

- [ ] 6. Write Recorder unit tests <!-- id:h55dblc -->
  - Test concurrent access with multiple goroutines
  - Test Calls() returns copy (modify returned slice, verify original unchanged)
  - Test AssertCallCount failure messages
  - Test AssertCallOrder with various patterns
  - All tests must pass go test -race
  - Blocked-by: h55dblb (Implement Recorder assertion methods)
  - Stream: 2
  - Requirements: [11.2](requirements.md#11.2), [11.4](requirements.md#11.4)

- [ ] 7. Implement ScenarioBuilder fluent API <!-- id:h55dbld -->
  - Implement NewScenario() returning *ScenarioBuilder
  - Implement Success(sessionID, cost) adding success response
  - Implement RetryableError(message) adding retryable error
  - Implement FatalError(message) adding fatal error
  - Implement SessionInvalid() adding session invalid error
  - Implement RateLimitWait(duration) adding rate limit response
  - Blocked-by: h55dbl9 (Define core types and interfaces)
  - Stream: 2
  - Requirements: [2.1](requirements.md#2.1), [2.2](requirements.md#2.2), [2.3](requirements.md#2.3), [2.4](requirements.md#2.4), [2.5](requirements.md#2.5), [2.6](requirements.md#2.6)

- [ ] 8. Implement ScenarioBuilder modifiers <!-- id:h55dble -->
  - Implement WithDelay(d) setting delay on last response
  - Implement WithOutput(output, stderr) setting output fields
  - Implement WithCost(metrics) setting cost metrics
  - Implement Repeat(n) duplicating last response n times
  - Implement Custom(fn) for dynamic behavior escape hatch
  - Implement Build() returning immutable *Scenario
  - Blocked-by: h55dbld (Implement ScenarioBuilder fluent API)
  - Stream: 2
  - Requirements: [2.7](requirements.md#2.7), [2.8](requirements.md#2.8), [2.9](requirements.md#2.9), [2.10](requirements.md#2.10), [2.12](requirements.md#2.12)

- [ ] 9. Write ScenarioBuilder unit tests <!-- id:h55dblf -->
  - Test immutability after Build()
  - Test method chaining returns same builder
  - Test all response methods create correct CallResponse
  - Test Repeat() duplicates correctly
  - Test Custom() escape hatch invocation
  - Test modifiers apply to last response only
  - Blocked-by: h55dble (Implement ScenarioBuilder modifiers)
  - Stream: 2
  - Requirements: [11.1](requirements.md#11.1)

- [ ] 10. Implement FakeClock <!-- id:h55dblg -->
  - Implement FakeClock struct with mutex, current time, sleeps slice
  - Implement NewFakeClock(start) constructor
  - Implement Now() returning controlled time
  - Implement Advance(d) moving time forward
  - Implement Sleep(d) recording sleep and returning immediately
  - Implement Sleeps() returning recorded sleeps
  - Implement AssertSleeps(t, expected) for verification
  - Blocked-by: h55dbl9 (Define core types and interfaces)
  - Stream: 2
  - Requirements: [6.1](requirements.md#6.1), [6.2](requirements.md#6.2), [6.3](requirements.md#6.3), [6.4](requirements.md#6.4)

- [ ] 11. Write FakeClock unit tests <!-- id:h55dblh -->
  - Test Advance moves time correctly
  - Test Sleep records duration without blocking
  - Test concurrent access safety
  - Test AssertSleeps failure messages
  - All tests must pass go test -race
  - Blocked-by: h55dblg (Implement FakeClock)
  - Stream: 2
  - Requirements: [11.3](requirements.md#11.3), [11.4](requirements.md#11.4)

## TestAgent Implementation

- [ ] 12. Implement TestAgent core <!-- id:h55dbli -->
  - Implement TestAgent struct with t, name, scenario, recorder, clock, config, mutex, callIndex fields
  - Implement TestAgentConfig struct for identity configuration
  - Implement NewTestAgent(t, name, scenario, ...opts) constructor
  - Implement Name(), CLICommand(), IsInstalled(), Version() returning values from config
  - Implement DefaultSessionDir(), DiscoverSessions() returning empty defaults
  - Blocked-by: h55dbla (Implement Recorder with thread-safe call tracking), h55dble (Implement ScenarioBuilder modifiers), h55dblg (Implement FakeClock)
  - Stream: 2
  - Requirements: [1.1](requirements.md#1.1), [1.4](requirements.md#1.4), [1.7](requirements.md#1.7)

- [ ] 13. Implement TestAgent Run and Resume <!-- id:h55dblj -->
  - Implement Run() with mutex-protected callIndex increment
  - Extract context deadline to HasDeadline/Deadline (do not store context)
  - Record AgentCall via recorder
  - Check bounds and call t.Fatalf with descriptive message on overflow
  - Handle CustomFunc escape hatch
  - Apply delay via clock.Sleep
  - Apply Output/Stderr to result
  - Implement Resume() with same pattern plus SessionID recording
  - Blocked-by: h55dbli (Implement TestAgent core)
  - Stream: 2
  - Requirements: [1.2](requirements.md#1.2), [1.3](requirements.md#1.3), [1.5](requirements.md#1.5), [2.11](requirements.md#2.11)

- [ ] 14. Implement TestAgent options and SessionExporter <!-- id:h55dblk -->
  - Define TestAgentOption function type
  - Implement WithClock(clock) option
  - Implement WithSessionExport(path) option enabling SessionExporter
  - Implement Recorder() accessor method
  - Implement AssertAllConsumed(t) for scenario exhaustion verification
  - Blocked-by: h55dblj (Implement TestAgent Run and Resume)
  - Stream: 2
  - Requirements: [1.6](requirements.md#1.6), [6.5](requirements.md#6.5), [6.6](requirements.md#6.6)

- [ ] 15. Write TestAgent unit tests <!-- id:h55dbll -->
  - Test AssertAllConsumed detects unconsumed responses
  - Test unexpected call triggers t.Fatalf
  - Test Run/Resume record calls correctly
  - Test concurrent calls are thread-safe
  - All tests must pass go test -race
  - Blocked-by: h55dblk (Implement TestAgent options and SessionExporter)
  - Stream: 2
  - Requirements: [11.4](requirements.md#11.4)

## Fixtures and Generators

- [ ] 16. Implement test fixtures <!-- id:h55dblm -->
  - Implement TestAgentResolver struct with agents map
  - Implement CreateTasksFile(t, phases) using t.TempDir()
  - Implement CreateConfig(t, opts) with ConfigOptions struct
  - Implement CreateTestOrbit(t, ...opts) wiring TestAgentResolver and Clock
  - Implement WithAgent(), WithAgents(), WithRuneClient(), WithClock() options
  - Implement CreateRuneClient(t, tasksFile) mock
  - Implement SuccessScenario(t, phases) convenience helper
  - All fixtures use t.Helper()
  - Blocked-by: h55dbl8 (Add AgentResolver interface to Orbit), h55dblk (Implement TestAgent options and SessionExporter)
  - Stream: 2
  - Requirements: [4.1](requirements.md#4.1), [4.2](requirements.md#4.2), [4.3](requirements.md#4.3), [4.4](requirements.md#4.4), [4.5](requirements.md#4.5), [4.6](requirements.md#4.6), [4.7](requirements.md#4.7), [4.8](requirements.md#4.8), [4.9](requirements.md#4.9), [4.10](requirements.md#4.10), [5.3](requirements.md#5.3), [5.4](requirements.md#5.4), [5.5](requirements.md#5.5), [5.6](requirements.md#5.6)

- [ ] 17. Write fixtures unit tests <!-- id:h55dbln -->
  - Test CreateTasksFile creates valid tasks.md
  - Test CreateConfig creates valid .orbit.yaml
  - Test CreateTestOrbit injects agent correctly
  - Test WithAgents supports multi-variant testing
  - Blocked-by: h55dblm (Implement test fixtures)
  - Stream: 2

- [ ] 18. Implement rapid generators <!-- id:h55dblo -->
  - Implement RunResultGen() with invariants: SessionID non-empty when !IsError, ExitCode 0 for success
  - Implement CostMetricsGen() with invariant: CostUSD non-negative
  - Implement ErrorClassGen() sampling valid error classes
  - Implement RandomScenarioGen(length) generating valid scenarios
  - Blocked-by: h55dble (Implement ScenarioBuilder modifiers)
  - Stream: 2
  - Requirements: [7.1](requirements.md#7.1), [7.2](requirements.md#7.2), [7.3](requirements.md#7.3), [7.4](requirements.md#7.4), [7.5](requirements.md#7.5)

- [ ] 19. Write generator invariant tests and property tests <!-- id:h55dblp -->
  - Test RunResultGen produces valid values
  - Test CostMetricsGen produces non-negative costs
  - Implement TestProperty_OrchestrationHandlesAnyErrorSequence
  - Implement TestProperty_RetryCountBounded
  - All tests must pass go test -race
  - Blocked-by: h55dblo (Implement rapid generators), h55dblm (Implement test fixtures)
  - Stream: 2
  - Requirements: [7.6](requirements.md#7.6), [11.4](requirements.md#11.4)

## Documentation

- [ ] 20. Write package documentation <!-- id:h55dblq -->
  - Create doc.go with package overview
  - Include basic usage example
  - Include error injection example
  - Include Repeat usage example
  - Include FakeClock timing test example
  - Document FakeClock scope limitation (Now/Sleep only)
  - Blocked-by: h55dblp (Write generator invariant tests and property tests)
  - Stream: 2
  - Requirements: [8.1](requirements.md#8.1), [8.2](requirements.md#8.2), [8.5](requirements.md#8.5), [6.7](requirements.md#6.7)

- [ ] 21. Add Testing section to CLAUDE.md <!-- id:h55dblr -->
  - Add Testing section after existing documentation
  - Include quick-start code example
  - Include common patterns (error recovery, Repeat, timing)
  - Reference internal/testutil/doc.go for full API
  - Blocked-by: h55dblq (Write package documentation)
  - Stream: 2
  - Requirements: [8.3](requirements.md#8.3), [8.4](requirements.md#8.4)

## Migration

- [ ] 22. Migrate orbit_test.go tests using mockAgent <!-- id:h55dbls -->
  - Identify tests using mockAgent in orbit_test.go
  - Replace mockAgent with NewTestAgent and ScenarioBuilder
  - Add t.Cleanup for AssertAllConsumed
  - Add Recorder assertions where appropriate
  - Verify tests pass with go test -race
  - Do not modify simple unit tests that don't use mockAgent
  - Blocked-by: h55dblm (Implement test fixtures)
  - Stream: 1
  - Requirements: [9.1](requirements.md#9.1), [9.4](requirements.md#9.4), [9.6](requirements.md#9.6)

- [ ] 23. Migrate integration_test.go tests using mockAgent <!-- id:h55dblt -->
  - Identify tests using mock agents in integration_test.go
  - Replace with NewTestAgent and ScenarioBuilder
  - Add t.Cleanup for AssertAllConsumed
  - Add Recorder assertions where appropriate
  - Verify tests pass with go test -race
  - Blocked-by: h55dbls (Migrate orbit_test.go tests using mockAgent)
  - Stream: 1
  - Requirements: [9.2](requirements.md#9.2), [9.4](requirements.md#9.4)

- [ ] 24. Remove old mock implementations and verify coverage <!-- id:h55dblu -->
  - Remove mockAgent type from test files
  - Remove mockClaudeClient type from test files
  - Run go test -coverprofile and compare to baseline
  - Verify coverage has not decreased
  - Blocked-by: h55dblt (Migrate integration_test.go tests using mockAgent)
  - Stream: 1
  - Requirements: [9.3](requirements.md#9.3), [9.5](requirements.md#9.5)
