// Package testutil provides testing utilities for Orbit integration tests.
//
// The primary components are:
//
//   - TestAgent: A mock agent implementing agents.Agent
//   - ScenarioBuilder: Fluent API for defining agent behavior sequences
//   - Recorder: Call recording and assertions
//   - FakeClock: Deterministic time control
//   - Fixtures: Helpers for test setup (CreateTestOrbit, CreateTasksFile, etc.)
//   - Generators: Property-based testing support with rapid
//
// # Basic Usage
//
// Define the expected agent behavior as a sequence of responses, create a test
// agent with that scenario, and wire it into Orbit via CreateTestOrbit:
//
//	func TestOrchestration(t *testing.T) {
//	    // Define the expected agent behavior sequence
//	    scenario := testutil.NewScenario().
//	        Success("session-1", 0.05).
//	        Success("session-2", 0.03).
//	        Build()
//
//	    // Create a test agent
//	    agent := testutil.NewTestAgent(t, "mock-agent", scenario)
//	    t.Cleanup(func() { agent.AssertAllConsumed(t) })
//
//	    // Create an Orbit instance with the test agent
//	    orbit := testutil.CreateTestOrbit(t,
//	        testutil.WithAgent(agent),
//	        testutil.WithTasksFile(testutil.CreateTasksFile(t, 2)),
//	    )
//
//	    // Run and verify
//	    err := orbit.Run()
//	    require.NoError(t, err)
//
//	    agent.Recorder().AssertCallCount(t, 2)
//	}
//
// # Error Injection
//
// Use RetryableError, FatalError, SessionInvalid, and RateLimitWait to test
// error handling and retry logic:
//
//	scenario := testutil.NewScenario().
//	    RetryableError("connection timeout").  // Phase 1, attempt 1 - retryable
//	    RetryableError("connection timeout").  // Phase 1, attempt 2 - retryable
//	    Success("session-1", 0.05).            // Phase 1, attempt 3 - succeeds
//	    Build()
//
//	agent := testutil.NewTestAgent(t, "mock", scenario)
//	// ... run orbit and verify retry behavior
//
// Different error types trigger different orchestration behaviors:
//
//   - RetryableError: Exponential backoff retry (1s, 2s, 4s, ...)
//   - FatalError: Immediate failure, no retry
//   - SessionInvalid: Session expired, starts fresh session
//   - RateLimitWait: Waits for rate limit reset, then continues
//
// # Using Repeat for Multiple Identical Responses
//
// When multiple phases need the same response, use Repeat to avoid repetition:
//
//	scenario := testutil.NewScenario().
//	    Success("session-1", 0.05).Repeat(5).  // 5 identical successes
//	    Build()
//
// Note that Repeat(5) creates 5 total responses (the original plus 4 copies).
//
// # Timing Tests with FakeClock
//
// Use FakeClock to test timing-dependent behavior without real delays:
//
//	func TestRetryBackoff(t *testing.T) {
//	    scenario := testutil.NewScenario().
//	        RetryableError("timeout").
//	        RetryableError("timeout").
//	        Success("session-1", 0.05).
//	        Build()
//
//	    clock := testutil.NewFakeClock(time.Now())
//	    agent := testutil.NewTestAgent(t, "mock", scenario, testutil.WithClock(clock))
//
//	    orbit := testutil.CreateTestOrbit(t,
//	        testutil.WithAgent(agent),
//	        testutil.WithOrbitClock(clock),
//	    )
//
//	    err := orbit.Run()
//	    require.NoError(t, err)
//
//	    // Verify exponential backoff: 1s, 2s
//	    clock.AssertSleeps(t, []time.Duration{time.Second, 2*time.Second})
//	}
//
// FakeClock.Sleep() records the sleep duration and returns immediately, making
// tests fast and deterministic. Use FakeClock.Advance() to move time forward
// manually if needed.
//
// # FakeClock Scope Limitations
//
// FakeClock only supports Now() and Sleep(). Timer-based code (time.After,
// time.NewTimer, time.NewTicker) is NOT supported and will use real time.
// This is a deliberate MVP limitation. If your code uses timers, either:
//
//   - Refactor to use Clock.Sleep() instead
//   - Use real time for those tests (slower but accurate)
//
// # Custom Responses
//
// For rare edge cases requiring dynamic behavior, use Custom():
//
//	scenario := testutil.NewScenario().
//	    Custom(func(call *testutil.AgentCall) *testutil.CallResponse {
//	        // Return different response based on call properties
//	        if strings.Contains(call.Options.Prompt, "phase") {
//	            return &testutil.CallResponse{
//	                Result: &agents.RunResult{SessionID: "dynamic-session"},
//	            }
//	        }
//	        return &testutil.CallResponse{
//	            Result:     &agents.RunResult{IsError: true, ExitCode: 1},
//	            ErrorClass: agents.ErrorClassFatal,
//	        }
//	    }).
//	    Build()
//
// Use Custom sparingly - prefer explicit scenario sequences when possible.
//
// # Multi-Variant Testing
//
// For testing variant execution with different agents per variant:
//
//	agents := map[string]agents.Agent{
//	    "claude-sonnet": testutil.NewTestAgent(t, "claude-sonnet", scenario1),
//	    "claude-opus":   testutil.NewTestAgent(t, "claude-opus", scenario2),
//	}
//	orbit := testutil.CreateTestOrbit(t, testutil.WithAgents(agents))
//
// # Call Recording and Assertions
//
// TestAgent records all calls, accessible via Recorder():
//
//	recorder := agent.Recorder()
//
//	// Basic assertions
//	recorder.AssertCallCount(t, 3)
//
//	// Pattern matching on prompts
//	phaseCalls := recorder.PhasePromptCalls()  // Matches /next-task --phase
//	matched := recorder.CallsWithPrompt(`phase \d+`)  // Regex matching
//
//	// Verify call sequence
//	recorder.AssertCallOrder(t,
//	    `pre-prompt`,
//	    `/next-task --phase 1`,
//	    `/next-task --phase 2`,
//	)
//
// # Property-Based Testing
//
// Use rapid generators for fuzz testing:
//
//	import "pgregory.net/rapid"
//
//	func TestProperty_OrchestrationNeverPanics(t *testing.T) {
//	    rapid.Check(t, func(rt *rapid.T) {
//	        length := rapid.IntRange(1, 10).Draw(rt, "length")
//	        scenario := testutil.RandomScenarioGen(length).Draw(rt, "scenario")
//
//	        agent := testutil.NewTestAgent(t, "mock", scenario)
//	        orbit := testutil.CreateTestOrbit(t, testutil.WithAgent(agent))
//
//	        // Should not panic regardless of error sequence
//	        _ = orbit.Run()
//	    })
//	}
//
// Available generators:
//
//   - RunResultGen(): Valid agents.RunResult values
//   - CostMetricsGen(): Valid agents.CostMetrics values
//   - ErrorClassGen(): Error classification types
//   - RandomScenarioGen(length): Random scenario sequences
package testutil
