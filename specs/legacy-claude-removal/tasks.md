---
references:
    - requirements.md
    - design.md
    - decision_log.md
---
# Legacy Claude Removal

## AgentAdapter Implementation

- [x] 1. Create AgentAdapter type <!-- id:kc125xx -->
  - Create internal/comparison/adapter.go with AgentAdapter struct containing agent (agents.Agent), ctx (context.Context), and workDir (string) fields.
  - Implement NewAgentAdapter(agent, ctx, workDir) constructor.
  - Implement RunCustomPrompt(prompt string) (*agents.RunResult, error) that delegates to agent.Run() with RunOptions{Prompt: prompt, WorkDir: workDir}.
  - Stream: 1
  - Requirements: [3.1](requirements.md#3.1), [3.2](requirements.md#3.2)

- [x] 2. Write AgentAdapter unit tests <!-- id:kc125xy -->
  - Create internal/comparison/adapter_test.go with tests for:
  - RunCustomPrompt delegates to agent.Run() with correct prompt
  - WorkDir is passed through correctly
  - Errors from agent.Run() are propagated unchanged
  - Use testutil.TestAgent with ScenarioBuilder for mock behavior.
  - Blocked-by: kc125xx (Create AgentAdapter type)
  - Stream: 1
  - Requirements: [3.2](requirements.md#3.2), [3.4](requirements.md#3.4)

## Production Code Migration

- [x] 3. Migrate runPhase() to use agent interface <!-- id:kc125xz -->
  - Replace o.claudeClient.RunPhase() calls at lines 820 and 840 in orbit.go with o.agent.Run()/Resume().
  - Follow the runPostPrompt() pattern: create RunOptions with Prompt, WorkDir, SessionID.
  - Use agent.Resume() when isResume is true, agent.Run() otherwise.
  - Preserve error classification and session-invalid fallback logic.
  - Blocked-by: kc125xx (Create AgentAdapter type)
  - Stream: 1
  - Requirements: [1.1](requirements.md#1.1), [1.2](requirements.md#1.2), [1.3](requirements.md#1.3), [1.4](requirements.md#1.4)

- [x] 4. Update Comparator usage in Orbit <!-- id:kc125y0 -->
  - In internal/orbit/orbit.go, change the comparison.NewComparator() call.
  - Use comparison.NewAgentAdapter(o.agent, o.shutdownCtx, o.config.WorkingDir) instead of o.rawClaudeClient.
  - Blocked-by: kc125xz (Migrate runPhase() to use agent interface)
  - Stream: 1
  - Requirements: [3.3](requirements.md#3.3)

- [x] 5. Update cmd/orbit/compare.go <!-- id:kc125y1 -->
  - In cmd/orbit/compare.go, replace claude.NewClient() with:
  - agents.Get("claude-code", agents.AgentConfig{WorkDir: workDir, AutoApprove: true})
  - Create adapter with comparison.NewAgentAdapter(agent, ctx, workDir).
  - Pass adapter to comparison.NewComparator().
  - Blocked-by: kc125xx (Create AgentAdapter type)
  - Stream: 1
  - Requirements: [2.7](requirements.md#2.7), [3.3](requirements.md#3.3)

## Test Migration - Skipped Tests

- [ ] 6. Migrate TestRunPostPromptWithRetry_RetryableError_EventualSuccess <!-- id:kc125y2 -->
  - At line 211 in orbit_test.go:
  - Replace mockClaudeClient with testutil.TestAgent using ScenarioBuilder.
  - Configure RetryableError() followed by Success().
  - Use testutil.FakeClock for deterministic timing.
  - Remove t.Skip().
  - Add t.Cleanup with agent.AssertAllConsumed(t).
  - Add clock.AssertSleeps(t, expectedDurations) to verify backoff timing.
  - Blocked-by: kc125xz (Migrate runPhase() to use agent interface)
  - Stream: 1
  - Requirements: [4.1](requirements.md#4.1), [4.5](requirements.md#4.5), [4.6](requirements.md#4.6), [4.7](requirements.md#4.7), [4.8](requirements.md#4.8)

- [ ] 7. Migrate TestRunPostPromptWithRetry_MaxRetriesExceeded <!-- id:kc125y3 -->
  - At line 251 in orbit_test.go:
  - Replace mockClaudeClient with testutil.TestAgent.
  - Configure multiple RetryableError() calls to exceed max retries.
  - Use FakeClock.
  - Remove t.Skip().
  - Add AssertAllConsumed cleanup.
  - Add AssertSleeps verification for all backoff durations.
  - Blocked-by: kc125xz (Migrate runPhase() to use agent interface)
  - Stream: 1
  - Requirements: [4.2](requirements.md#4.2), [4.5](requirements.md#4.5), [4.6](requirements.md#4.6), [4.7](requirements.md#4.7), [4.8](requirements.md#4.8)

- [ ] 8. Migrate TestRunPhaseWithRetry_RateLimitError <!-- id:kc125y4 -->
  - At line 290 in orbit_test.go:
  - Replace mockClaudeClient with testutil.TestAgent.
  - Configure RateLimitError with reset time.
  - Use FakeClock to simulate time passing.
  - Remove t.Skip().
  - Add AssertAllConsumed cleanup.
  - Verify wait duration matches reset time.
  - Blocked-by: kc125xz (Migrate runPhase() to use agent interface)
  - Stream: 1
  - Requirements: [4.3](requirements.md#4.3), [4.5](requirements.md#4.5), [4.6](requirements.md#4.6), [4.7](requirements.md#4.7), [4.8](requirements.md#4.8)

- [ ] 9. Migrate TestRunPhaseWithRetry_OverloadedError <!-- id:kc125y5 -->
  - At line 327 in orbit_test.go:
  - Replace mockClaudeClient with testutil.TestAgent.
  - Configure OverloadedError.
  - Use FakeClock.
  - Remove t.Skip().
  - Add AssertAllConsumed cleanup.
  - Add AssertSleeps verification for overload backoff behavior.
  - Blocked-by: kc125xz (Migrate runPhase() to use agent interface)
  - Stream: 1
  - Requirements: [4.4](requirements.md#4.4), [4.5](requirements.md#4.5), [4.6](requirements.md#4.6), [4.7](requirements.md#4.7), [4.8](requirements.md#4.8)

## Test Migration - Other Tests

- [ ] 10. Migrate remaining orbit_test.go mockClaudeClient tests <!-- id:kc125y6 -->
  - Migrate tests at lines 518, 566, 621, 1543, 1588 in orbit_test.go.
  - For each: Replace mockClaudeClient with testutil.TestAgent.
  - Configure appropriate scenarios using ScenarioBuilder.
  - Add t.Cleanup with agent.AssertAllConsumed(t).
  - Verify assertion parity with original tests.
  - Blocked-by: kc125xz (Migrate runPhase() to use agent interface)
  - Stream: 1
  - Requirements: [5.1](requirements.md#5.1), [5.4](requirements.md#5.4), [5.5](requirements.md#5.5)

- [ ] 11. Migrate integration_test.go mockClaudeClient test <!-- id:kc125y7 -->
  - Migrate test at line 521 in integration_test.go.
  - Replace mockClaudeClient with testutil.TestAgent.
  - Configure scenario matching original test behavior.
  - Add t.Cleanup with agent.AssertAllConsumed(t).
  - Blocked-by: kc125xz (Migrate runPhase() to use agent interface)
  - Stream: 1
  - Requirements: [5.2](requirements.md#5.2), [5.4](requirements.md#5.4), [5.5](requirements.md#5.5)

## Legacy Code Removal

- [ ] 12. Remove mockClaudeClient type <!-- id:kc125y8 -->
  - Delete the mockClaudeClient struct definition and all its methods from orbit_test.go (lines 21-55).
  - Verify no remaining references to mockClaudeClient in the test file.
  - Blocked-by: kc125y2 (Migrate TestRunPostPromptWithRetry_RetryableError_EventualSuccess), kc125y3 (Migrate TestRunPostPromptWithRetry_MaxRetriesExceeded), kc125y4 (Migrate TestRunPhaseWithRetry_RateLimitError), kc125y5 (Migrate TestRunPhaseWithRetry_OverloadedError), kc125y6 (Migrate remaining orbit_test.go mockClaudeClient tests), kc125y7 (Migrate integration_test.go mockClaudeClient test)
  - Stream: 1
  - Requirements: [5.3](requirements.md#5.3)

- [ ] 13. Remove claudeRunner interface <!-- id:kc125y9 -->
  - Delete the claudeRunner interface definition from internal/orbit/orbit.go (lines 74-80).
  - This interface defined RunPhase() which is no longer used.
  - Blocked-by: kc125y8 (Remove mockClaudeClient type)
  - Stream: 1
  - Requirements: [2.1](requirements.md#2.1)

- [ ] 14. Remove legacy fields from Orbit struct <!-- id:kc125ya -->
  - Remove claudeClient (line 180) and rawClaudeClient (line 193) fields from the Orbit struct.
  - Remove their initialization in New() function.
  - Update any struct literals that reference these fields.
  - Blocked-by: kc125y9 (Remove claudeRunner interface)
  - Stream: 1
  - Requirements: [2.2](requirements.md#2.2), [2.3](requirements.md#2.3), [2.4](requirements.md#2.4), [2.9](requirements.md#2.9)

- [ ] 15. Delete legacy files and directory <!-- id:kc125yb -->
  - Delete internal/claude/client.go and internal/claude/client_test.go.
  - Remove the internal/claude/ directory entirely.
  - Verify no imports of github.com/arjenschwarz/orbit/internal/claude remain in the codebase.
  - Blocked-by: kc125ya (Remove legacy fields from Orbit struct)
  - Stream: 1
  - Requirements: [2.5](requirements.md#2.5), [2.6](requirements.md#2.6), [2.8](requirements.md#2.8)

## Verification

- [ ] 16. Verify test coverage and quality <!-- id:kc125yc -->
  - Run: make test, go test -race ./..., make lint.
  - Verify coverage is at least 22.4% for internal/orbit/.
  - Run: grep -r "internal/claude" --include="*.go" to ensure no imports remain.
  - Verify all 4 previously-skipped tests now pass and execute in under 1 second each.
  - Blocked-by: kc125yb (Delete legacy files and directory)
  - Stream: 1
  - Requirements: [6.1](requirements.md#6.1), [6.2](requirements.md#6.2), [6.3](requirements.md#6.3), [6.4](requirements.md#6.4)
