package orbit

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/arjenschwarz/orbit/internal/agents"
	configPkg "github.com/arjenschwarz/orbit/internal/config"
	"github.com/arjenschwarz/orbit/internal/debug"
	"github.com/arjenschwarz/orbit/internal/logs"
	"github.com/arjenschwarz/orbit/internal/registry"
	"github.com/arjenschwarz/orbit/internal/rune"
	"github.com/arjenschwarz/orbit/internal/testutil"
	"github.com/arjenschwarz/orbit/internal/variants"
)

func TestConfig_Struct(t *testing.T) {
	config := Config{
		TasksFile:       "specs/test/tasks.md",
		LogDir:          ".claude/logs",
		BranchName:      "feature/test",
		SkipPermissions: true,
		Verbose:         true,
		DryRun:          false,
		WorkingDir:      "/path/to/project",
	}

	if config.TasksFile != "specs/test/tasks.md" {
		t.Errorf("TasksFile = %q, want %q", config.TasksFile, "specs/test/tasks.md")
	}
	if config.LogDir != ".claude/logs" {
		t.Errorf("LogDir = %q, want %q", config.LogDir, ".claude/logs")
	}
	if config.BranchName != "feature/test" {
		t.Errorf("BranchName = %q, want %q", config.BranchName, "feature/test")
	}
	if !config.SkipPermissions {
		t.Error("SkipPermissions should be true")
	}
	if !config.Verbose {
		t.Error("Verbose should be true")
	}
	if config.DryRun {
		t.Error("DryRun should be false")
	}
	if config.WorkingDir != "/path/to/project" {
		t.Errorf("WorkingDir = %q, want %q", config.WorkingDir, "/path/to/project")
	}
}

func TestConfig_CommandFields(t *testing.T) {
	config := Config{
		TasksFile:  "specs/test/tasks.md",
		LogDir:     ".claude/logs",
		BranchName: "feature/test",
		WorkingDir: "/path/to/project",
		Command:    "Run /next-task --phase",
		PostPrompt: "Review the implementation",
	}

	if config.Command != "Run /next-task --phase" {
		t.Errorf("Command = %q, want %q", config.Command, "Run /next-task --phase")
	}
	if config.PostPrompt != "Review the implementation" {
		t.Errorf("PostPrompt = %q, want %q", config.PostPrompt, "Review the implementation")
	}
}

func TestConfig_EmptyPostPrompt(t *testing.T) {
	config := Config{
		TasksFile:  "specs/test/tasks.md",
		LogDir:     ".claude/logs",
		BranchName: "feature/test",
		WorkingDir: "/path/to/project",
		Command:    "Run /next-task --phase",
		PostPrompt: "", // Explicitly disabled
	}

	if config.PostPrompt != "" {
		t.Errorf("PostPrompt should be empty when disabled, got %q", config.PostPrompt)
	}
}

func TestMaxRetries_Constant(t *testing.T) {
	if maxRetries != 5 {
		t.Errorf("maxRetries = %d, want 5", maxRetries)
	}
}

func TestComplete_PostPromptSkippedWhenEmpty(t *testing.T) {
	// Create an Orbit instance with empty PostPrompt
	o := &Orbit{
		config: Config{
			PostPrompt: "",
		},
		logManager: nil, // No log manager for this test
	}

	// Call complete() - should not error because PostPrompt is empty
	err := o.complete()
	if err != nil {
		t.Errorf("complete() returned error when PostPrompt is empty: %v", err)
	}
}

func TestRunPostPromptWithRetry_Success(t *testing.T) {
	// Define expected agent behavior using ScenarioBuilder
	scenario := testutil.NewScenario().
		Success("test-session", 0.01).
		WithOutput("Success", "").
		Build()

	agent := testutil.NewTestAgent(t, "test-agent", scenario)
	t.Cleanup(func() { agent.AssertAllConsumed(t) })

	o := &Orbit{
		config: Config{
			PostPrompt: "test command",
		},
		agent:           agent,
		logManager:      nil,
		errorClassifier: agents.GetClassifier("test"), // Use default classifier
		shutdownCtx:     t.Context(),
	}

	err := o.runPostPromptWithRetry()
	if err != nil {
		t.Errorf("runPostPromptWithRetry() returned error: %v", err)
	}

	// Verify using Recorder
	agent.Recorder().AssertCallCount(t, 1)
}

func TestRunPostPromptWithRetry_NonRetryableError(t *testing.T) {
	// Define expected agent behavior - fatal error (non-retryable)
	scenario := testutil.NewScenario().
		FatalError("unknown error occurred").
		Build()

	agent := testutil.NewTestAgent(t, "test-agent", scenario)
	t.Cleanup(func() { agent.AssertAllConsumed(t) })

	o := &Orbit{
		config: Config{
			PostPrompt: "test command",
		},
		agent:           agent,
		logManager:      nil,
		errorClassifier: agents.GetClassifier("test"), // Use default classifier
		shutdownCtx:     t.Context(),
	}

	err := o.runPostPromptWithRetry()
	if err == nil {
		t.Error("expected error, got nil")
	}

	// Non-retryable errors should not retry - verify only 1 call was made
	agent.Recorder().AssertCallCount(t, 1)
}

func TestRunPostPromptWithRetry_RetryableError_EventualSuccess(t *testing.T) {
	// Import the claudecode package to register its classifier
	_ = agents.GetClassifier("claude-code")

	// Create FakeClock for deterministic timing
	clock := testutil.NewFakeClock(time.Now())

	// Define expected agent behavior: 2 connection timeouts then success
	// Use "connection timeout" in stderr which the claude-code classifier recognizes as retryable
	scenario := testutil.NewScenario().
		RetryableError("connection timeout").
		RetryableError("connection timeout").
		Success("test-session", 0.01).
		WithOutput("Success", "").
		Build()

	agent := testutil.NewTestAgent(t, "test-agent", scenario, testutil.WithClock(clock))
	t.Cleanup(func() { agent.AssertAllConsumed(t) })

	o := &Orbit{
		config: Config{
			PostPrompt: "test command",
			Clock:      clock,
		},
		agent:           agent,
		logManager:      nil,
		errorClassifier: agents.GetClassifier("claude-code"),
		shutdownCtx:     t.Context(),
	}

	err := o.runPostPromptWithRetry()
	if err != nil {
		t.Errorf("runPostPromptWithRetry() returned error: %v", err)
	}

	// Verify 3 calls: 2 retries then success
	agent.Recorder().AssertCallCount(t, 3)

	// Verify backoff durations: 1s after first failure, 2s after second failure
	clock.AssertSleeps(t, []time.Duration{time.Second, 2 * time.Second})
}

func TestRunPostPromptWithRetry_MaxRetriesExceeded(t *testing.T) {
	// Import the claudecode package to register its classifier
	_ = agents.GetClassifier("claude-code")

	// Create FakeClock for deterministic timing
	clock := testutil.NewFakeClock(time.Now())

	// Define expected agent behavior: always fail with connection error (retryable)
	// Use "connection" in stderr which the claude-code classifier recognizes as retryable
	// Need maxRetries (5) failures
	scenario := testutil.NewScenario().
		RetryableError("connection refused").Repeat(maxRetries).
		Build()

	agent := testutil.NewTestAgent(t, "test-agent", scenario, testutil.WithClock(clock))
	t.Cleanup(func() { agent.AssertAllConsumed(t) })

	o := &Orbit{
		config: Config{
			PostPrompt: "test command",
			Clock:      clock,
		},
		agent:           agent,
		logManager:      nil,
		errorClassifier: agents.GetClassifier("claude-code"),
		shutdownCtx:     t.Context(),
	}

	err := o.runPostPromptWithRetry()
	if err == nil {
		t.Error("expected error after max retries, got nil")
	}

	// Verify maxRetries (5) calls were made
	agent.Recorder().AssertCallCount(t, maxRetries)

	assert.Contains(t, err.Error(), "max retries exceeded", "expected 'max retries exceeded' in error message, got: %v", err)

	// Verify error is wrapped as ClassifiedError from agents package
	var classified *agents.ClassifiedError
	if !errors.As(err, &classified) {
		t.Error("expected wrapped agents.ClassifiedError")
	}

	// Verify backoff durations: 1s, 2s, 4s, 8s (4 sleeps for 5 attempts).
	// The shared retry executor skips the sleep after the final attempt
	// since no retry will follow.
	clock.AssertSleeps(t, []time.Duration{
		time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
	})
}

func TestRunPhaseWithRetry_RateLimitError(t *testing.T) {
	// Import the claudecode package to register its classifier
	_ = agents.GetClassifier("claude-code")

	// Create FakeClock for deterministic timing
	clock := testutil.NewFakeClock(time.Now())

	// Define expected agent behavior: rate limit error on first attempt, then success
	// Use "rate limit" in stderr which the claude-code classifier recognizes as retryable
	// The classifier will set a 60s RetryAfter for rate limit errors
	scenario := testutil.NewScenario().
		RetryableError("rate limit exceeded").
		Success("test-session", 0.01).
		WithOutput("Success", "").
		Build()

	agent := testutil.NewTestAgent(t, "test-agent", scenario,
		testutil.WithClock(clock),
	)
	t.Cleanup(func() { agent.AssertAllConsumed(t) })

	o := &Orbit{
		config: Config{
			Clock: clock,
		},
		agent:           agent,
		logManager:      nil,
		errorClassifier: agents.GetClassifier("claude-code"),
		shutdownCtx:     t.Context(),
		debug:           debug.New(false, ""),
	}

	err := o.runPhaseWithRetry(1)
	if err != nil {
		t.Errorf("runPhaseWithRetry() returned error: %v", err)
	}

	// Verify 2 calls: 1 failure then success
	agent.Recorder().AssertCallCount(t, 2)

	// Verify total backoff: 60s from rate limit RetryAfter (chunked into 30s pieces)
	clock.AssertTotalSleep(t, 60*time.Second)
}

func TestRunPhaseWithRetry_OverloadedError(t *testing.T) {
	// Import the claudecode package to register its classifier
	_ = agents.GetClassifier("claude-code")

	// Create FakeClock for deterministic timing
	clock := testutil.NewFakeClock(time.Now())

	// Define expected agent behavior: overloaded error on first attempt, then success
	// Use "503 service unavailable" in stderr which the claude-code classifier recognizes as retryable
	// The classifier will set a 30s RetryAfter for overloaded errors
	scenario := testutil.NewScenario().
		RetryableError("503 service unavailable").
		Success("test-session", 0.01).
		WithOutput("Success", "").
		Build()

	agent := testutil.NewTestAgent(t, "test-agent", scenario,
		testutil.WithClock(clock),
	)
	t.Cleanup(func() { agent.AssertAllConsumed(t) })

	o := &Orbit{
		config: Config{
			Clock: clock,
		},
		agent:           agent,
		logManager:      nil,
		errorClassifier: agents.GetClassifier("claude-code"),
		shutdownCtx:     t.Context(),
		debug:           debug.New(false, ""),
	}

	err := o.runPhaseWithRetry(1)
	if err != nil {
		t.Errorf("runPhaseWithRetry() returned error: %v", err)
	}

	// Verify 2 calls: 1 failure then success
	agent.Recorder().AssertCallCount(t, 2)

	// Verify backoff duration: 30s from overloaded RetryAfter
	clock.AssertSleeps(t, []time.Duration{30 * time.Second})
}

func TestIsSessionInvalidError(t *testing.T) {
	tests := map[string]struct {
		result *agents.RunResult
		want   bool
	}{
		"session not found in stderr": {
			result: &agents.RunResult{
				Stderr: "error: session not found",
				Output: "",
			},
			want: true,
		},
		"session not found in output": {
			result: &agents.RunResult{
				Stderr: "",
				Output: "Session not found for the given ID",
			},
			want: true,
		},
		"invalid session in stderr": {
			result: &agents.RunResult{
				Stderr: "invalid session ID provided",
				Output: "",
			},
			want: true,
		},
		"invalid session in output": {
			result: &agents.RunResult{
				Stderr: "",
				Output: "error: invalid session - cannot be resumed",
			},
			want: true,
		},
		"session expired in stderr": {
			result: &agents.RunResult{
				Stderr: "session expired",
				Output: "",
			},
			want: true,
		},
		"session expired in output": {
			result: &agents.RunResult{
				Stderr: "",
				Output: "error: session expired, please start a new one",
			},
			want: true,
		},
		"no such session": {
			result: &agents.RunResult{
				Stderr: "no such session exists",
				Output: "",
			},
			want: true,
		},
		"no conversation found": {
			result: &agents.RunResult{
				Stderr: "",
				Output: "No conversation found with session ID: e97e6f18-bbe5-4186-84e8-74ccd65c7dcf",
			},
			want: true,
		},
		"case insensitive matching": {
			result: &agents.RunResult{
				Stderr: "SESSION NOT FOUND",
				Output: "",
			},
			want: true,
		},
		"non-session error returns false": {
			result: &agents.RunResult{
				Stderr: "rate limit exceeded",
				Output: "",
			},
			want: false,
		},
		"connection error returns false": {
			result: &agents.RunResult{
				Stderr: "connection timeout",
				Output: "",
			},
			want: false,
		},
		"empty result returns false": {
			result: &agents.RunResult{
				Stderr: "",
				Output: "",
			},
			want: false,
		},
		"nil result returns false": {
			result: nil,
			want:   false,
		},
		"generic error returns false": {
			result: &agents.RunResult{
				Stderr: "unknown error occurred",
				Output: "Something went wrong",
			},
			want: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := isSessionInvalidError(tc.result)
			if got != tc.want {
				t.Errorf("isSessionInvalidError() = %v, want %v", got, tc.want)
			}
		})
	}
}


func TestRunPhase_SessionContinuation_NewSession(t *testing.T) {
	// Create scenario that returns success
	scenario := testutil.NewScenario().
		Success("test-session", 0.0).
		Build()

	agent := testutil.NewTestAgent(t, "mock", scenario)
	t.Cleanup(func() { agent.AssertAllConsumed(t) })

	o := &Orbit{
		config: Config{
			ContinueSession: true,
		},
		agent:           agent,
		errorClassifier: agents.GetClassifier("claude-code"),
		logManager:      nil, // No log manager - should generate fresh session
		shutdownCtx:     context.Background(),
		debug:           debug.New(false, ""),
	}

	err := o.runPhase(1)
	if err != nil {
		t.Errorf("runPhase() returned error: %v", err)
	}

	// Without log manager, should always be a new session (resume=false)
	// Check that Run was called (not Resume) via recorder
	calls := agent.Recorder().Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Method != "Run" {
		t.Errorf("expected method=Run for new session, got %s", calls[0].Method)
	}

	// Session ID should be non-empty (UUID generated in opts)
	if calls[0].Options.SessionID == "" {
		t.Error("expected non-empty session ID in options")
	}
}

func TestRunPhase_PopulatesTimeoutFromAgentConfig(t *testing.T) {
	// Regression test for T-584: AgentConfig.Timeout must be propagated to
	// RunOptions.Timeout so agent execution contexts are time-bounded.
	scenario := testutil.NewScenario().
		Success("test-session", 0.0).
		Build()

	agent := testutil.NewTestAgent(t, "mock", scenario)
	t.Cleanup(func() { agent.AssertAllConsumed(t) })

	timeout := 30 * time.Second
	o := &Orbit{
		config: Config{
			AgentConfig: agents.AgentConfig{
				Timeout: timeout,
			},
		},
		agent:           agent,
		errorClassifier: agents.GetClassifier("claude-code"),
		logManager:      nil,
		shutdownCtx:     context.Background(),
		debug:           debug.New(false, ""),
	}

	err := o.runPhase(1)
	if err != nil {
		t.Fatalf("runPhase() returned error: %v", err)
	}

	calls := agent.Recorder().Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Options.Timeout != timeout {
		t.Errorf("RunOptions.Timeout = %v, want %v", calls[0].Options.Timeout, timeout)
	}
}

func TestRunPrePrompt_PopulatesTimeoutFromAgentConfig(t *testing.T) {
	// Regression test for T-584: Pre-prompt should also use agent timeout.
	scenario := testutil.NewScenario().
		Success("test-session", 0.0).
		Build()

	agent := testutil.NewTestAgent(t, "mock", scenario)
	t.Cleanup(func() { agent.AssertAllConsumed(t) })

	timeout := 15 * time.Second
	o := &Orbit{
		config: Config{
			PrePrompt: "Test pre-prompt",
			AgentConfig: agents.AgentConfig{
				Timeout: timeout,
			},
		},
		agent:           agent,
		errorClassifier: agents.GetClassifier("claude-code"),
		logManager:      nil,
		shutdownCtx:     context.Background(),
		debug:           debug.New(false, ""),
	}

	err := o.runPrePrompt()
	if err != nil {
		t.Fatalf("runPrePrompt() returned error: %v", err)
	}

	calls := agent.Recorder().Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Options.Timeout != timeout {
		t.Errorf("RunOptions.Timeout = %v, want %v", calls[0].Options.Timeout, timeout)
	}
}

func TestRunPostPrompt_PopulatesTimeoutFromAgentConfig(t *testing.T) {
	// Regression test for T-584: Post-prompt should also use agent timeout.
	scenario := testutil.NewScenario().
		Success("test-session", 0.0).
		Build()

	agent := testutil.NewTestAgent(t, "mock", scenario)
	t.Cleanup(func() { agent.AssertAllConsumed(t) })

	timeout := 20 * time.Second
	o := &Orbit{
		config: Config{
			PostPrompt: "Test post-prompt",
			AgentConfig: agents.AgentConfig{
				Timeout: timeout,
			},
		},
		agent:           agent,
		errorClassifier: agents.GetClassifier("claude-code"),
		logManager:      nil,
		shutdownCtx:     context.Background(),
		debug:           debug.New(false, ""),
	}

	err := o.runPostPrompt()
	if err != nil {
		t.Fatalf("runPostPrompt() returned error: %v", err)
	}

	calls := agent.Recorder().Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Options.Timeout != timeout {
		t.Errorf("RunOptions.Timeout = %v, want %v", calls[0].Options.Timeout, timeout)
	}
}

func TestRunPhase_SessionContinuation_WithLogManager(t *testing.T) {
	// Create scenario that returns success
	scenario := testutil.NewScenario().
		Success("test-session", 0.0).
		Build()

	agent := testutil.NewTestAgent(t, "mock", scenario)
	t.Cleanup(func() { agent.AssertAllConsumed(t) })

	// Create log manager in temp directory
	tempDir := t.TempDir()
	logManager, err := logs.NewManagerWithOptions(tempDir, "test-branch", tempDir, logs.ManagerOptions{UseSubdirs: false})
	if err != nil {
		t.Fatalf("Failed to create log manager: %v", err)
	}

	o := &Orbit{
		config: Config{
			ContinueSession: true,
		},
		agent:           agent,
		errorClassifier: agents.GetClassifier("claude-code"),
		logManager:      logManager,
		shutdownCtx:     context.Background(),
		debug:           debug.New(false, ""),
	}

	// First run - should start a new session
	if err := o.runPhase(1); err != nil {
		t.Fatalf("First runPhase() returned error: %v", err)
	}

	calls := agent.Recorder().Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}

	// First call should be Run (not Resume) - new session
	if calls[0].Method != "Run" {
		t.Errorf("first call should be Run, got %s", calls[0].Method)
	}
	firstSessionID := calls[0].Options.SessionID
	if firstSessionID == "" {
		t.Error("expected non-empty session ID")
	}
}

func TestRunPhase_ResumeFallback(t *testing.T) {
	// Create scenario: first call (Resume) fails with session not found, second (Run) succeeds
	scenario := testutil.NewScenario().
		SessionInvalid().            // Resume will fail with session not found
		Success("new-session", 0.0). // Fresh Run succeeds
		Build()

	agent := testutil.NewTestAgent(t, "mock", scenario)
	t.Cleanup(func() { agent.AssertAllConsumed(t) })

	// Create log manager in temp directory
	tempDir := t.TempDir()
	logManager, err := logs.NewManagerWithOptions(tempDir, "test-branch", tempDir, logs.ManagerOptions{UseSubdirs: false})
	if err != nil {
		t.Fatalf("Failed to create log manager: %v", err)
	}

	// Manually set a CurrentPhase to simulate resuming
	// We need to call StartPhase first, then simulate an interruption
	originalSessionID, _, err := logManager.StartPhase(1, true)
	if err != nil {
		t.Fatalf("StartPhase() returned error: %v", err)
	}

	o := &Orbit{
		config: Config{
			ContinueSession: true,
		},
		agent:           agent,
		errorClassifier: agents.GetClassifier("claude-code"),
		logManager:      logManager,
		shutdownCtx:     context.Background(),
		debug:           debug.New(false, ""),
	}

	// Run phase - should try resume first, then fall back
	if err := o.runPhase(1); err != nil {
		t.Fatalf("runPhase() returned error: %v", err)
	}

	calls := agent.Recorder().Calls()
	// Should have 2 calls: first Resume (failed), second Run
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}

	// First call should be Resume with original session ID
	if calls[0].Method != "Resume" {
		t.Errorf("first call should be Resume, got %s", calls[0].Method)
	}
	if calls[0].SessionID != originalSessionID {
		t.Errorf("first call session ID = %q, want %q", calls[0].SessionID, originalSessionID)
	}

	// Second call should be Run with a new session ID
	if calls[1].Method != "Run" {
		t.Errorf("second call should be Run, got %s", calls[1].Method)
	}
	if calls[1].Options.SessionID == originalSessionID {
		t.Error("second call should have a different session ID")
	}
}

// --- Auto-Registration Tests (Phase 6) ---

func TestOrbit_RegistryIntegration_RegisterOnStart(t *testing.T) {
	// Test that registry entry is created on run start (requirement 3.1)
	tempDir := t.TempDir()
	registryDir := filepath.Join(tempDir, "runs")

	reg, err := registry.New(registryDir)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	logDir := filepath.Join(tempDir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatalf("Failed to create log dir: %v", err)
	}

	o := &Orbit{
		config: Config{
			BranchName: "feature/test",
			WorkingDir: tempDir,
			LogDir:     logDir,
		},
		registry: reg,
	}

	// Register the run
	runID, err := o.registerRun()
	if err != nil {
		t.Fatalf("registerRun() returned error: %v", err)
	}

	// Verify entry was created
	entry, err := reg.Get(runID)
	if err != nil {
		t.Fatalf("Failed to get registry entry: %v", err)
	}
	if entry == nil {
		t.Fatal("Registry entry not found after registration")
	}

	// Check required fields
	if entry.Status != registry.StatusRunning {
		t.Errorf("Status = %q, want %q", entry.Status, registry.StatusRunning)
	}
	if entry.PID == nil {
		t.Error("PID should be set for auto-registered runs")
	} else if *entry.PID != os.Getpid() {
		t.Errorf("PID = %d, want %d", *entry.PID, os.Getpid())
	}
	if entry.Branch != "feature/test" {
		t.Errorf("Branch = %q, want %q", entry.Branch, "feature/test")
	}
	if entry.LogDir != logDir {
		t.Errorf("LogDir = %q, want %q", entry.LogDir, logDir)
	}
}

func TestOrbit_RegistryIntegration_UpdatePhaseOnStart(t *testing.T) {
	// Test that phase status is updated when phases start (requirement 3.5)
	tempDir := t.TempDir()
	registryDir := filepath.Join(tempDir, "runs")

	reg, err := registry.New(registryDir)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	logDir := filepath.Join(tempDir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatalf("Failed to create log dir: %v", err)
	}

	o := &Orbit{
		config: Config{
			BranchName: "feature/test",
			WorkingDir: tempDir,
			LogDir:     logDir,
		},
		registry: reg,
	}

	// Register the run first
	runID, err := o.registerRun()
	if err != nil {
		t.Fatalf("registerRun() returned error: %v", err)
	}
	o.runID = runID

	// Update phase to running
	o.updatePhaseStatus(1, registry.PhaseStatusRunning, 1)

	// Verify phase was added
	entry, err := reg.Get(runID)
	if err != nil {
		t.Fatalf("Failed to get registry entry: %v", err)
	}

	if len(entry.Phases) != 1 {
		t.Fatalf("Expected 1 phase, got %d", len(entry.Phases))
	}
	if entry.Phases[0].Number != 1 {
		t.Errorf("Phase number = %d, want 1", entry.Phases[0].Number)
	}
	if entry.Phases[0].Status != registry.PhaseStatusRunning {
		t.Errorf("Phase status = %q, want %q", entry.Phases[0].Status, registry.PhaseStatusRunning)
	}
}

func TestOrbit_RegistryIntegration_UpdatePhaseOnComplete(t *testing.T) {
	// Test that phase status is updated when phases complete (requirement 3.6)
	tempDir := t.TempDir()
	registryDir := filepath.Join(tempDir, "runs")

	reg, err := registry.New(registryDir)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	logDir := filepath.Join(tempDir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatalf("Failed to create log dir: %v", err)
	}

	o := &Orbit{
		config: Config{
			BranchName: "feature/test",
			WorkingDir: tempDir,
			LogDir:     logDir,
		},
		registry: reg,
	}

	// Register and start a phase
	runID, err := o.registerRun()
	if err != nil {
		t.Fatalf("registerRun() returned error: %v", err)
	}
	o.runID = runID
	o.updatePhaseStatus(1, registry.PhaseStatusRunning, 1)

	// Complete the phase
	o.updatePhaseStatus(1, registry.PhaseStatusCompleted, 1)

	// Verify phase was updated
	entry, err := reg.Get(runID)
	if err != nil {
		t.Fatalf("Failed to get registry entry: %v", err)
	}

	if len(entry.Phases) != 1 {
		t.Fatalf("Expected 1 phase, got %d", len(entry.Phases))
	}
	if entry.Phases[0].Status != registry.PhaseStatusCompleted {
		t.Errorf("Phase status = %q, want %q", entry.Phases[0].Status, registry.PhaseStatusCompleted)
	}
}

func TestOrbit_RegistryIntegration_UpdateStatusOnComplete(t *testing.T) {
	// Test that run status is updated on successful completion (requirement 3.2)
	tempDir := t.TempDir()
	registryDir := filepath.Join(tempDir, "runs")

	reg, err := registry.New(registryDir)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	logDir := filepath.Join(tempDir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatalf("Failed to create log dir: %v", err)
	}

	o := &Orbit{
		config: Config{
			BranchName: "feature/test",
			WorkingDir: tempDir,
			LogDir:     logDir,
		},
		registry: reg,
	}

	// Register the run
	runID, err := o.registerRun()
	if err != nil {
		t.Fatalf("registerRun() returned error: %v", err)
	}
	o.runID = runID

	// Complete the run
	o.updateRunStatus(registry.StatusCompleted)

	// Verify status was updated
	entry, err := reg.Get(runID)
	if err != nil {
		t.Fatalf("Failed to get registry entry: %v", err)
	}
	if entry.Status != registry.StatusCompleted {
		t.Errorf("Status = %q, want %q", entry.Status, registry.StatusCompleted)
	}
	if entry.FinishedAt == nil {
		t.Error("FinishedAt should be set on completion")
	}
}

func TestOrbit_RegistryIntegration_UpdateStatusOnFail(t *testing.T) {
	// Test that run status is updated on failure (requirement 3.3)
	tempDir := t.TempDir()
	registryDir := filepath.Join(tempDir, "runs")

	reg, err := registry.New(registryDir)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	logDir := filepath.Join(tempDir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatalf("Failed to create log dir: %v", err)
	}

	o := &Orbit{
		config: Config{
			BranchName: "feature/test",
			WorkingDir: tempDir,
			LogDir:     logDir,
		},
		registry: reg,
	}

	// Register the run
	runID, err := o.registerRun()
	if err != nil {
		t.Fatalf("registerRun() returned error: %v", err)
	}
	o.runID = runID

	// Fail the run
	o.updateRunStatus(registry.StatusFailed)

	// Verify status was updated
	entry, err := reg.Get(runID)
	if err != nil {
		t.Fatalf("Failed to get registry entry: %v", err)
	}
	if entry.Status != registry.StatusFailed {
		t.Errorf("Status = %q, want %q", entry.Status, registry.StatusFailed)
	}
	if entry.FinishedAt == nil {
		t.Error("FinishedAt should be set on failure")
	}
}

func TestOrbit_RegistryIntegration_GracefulFailure(t *testing.T) {
	// Test that registration failures don't stop execution (requirement 3.7)
	// Use nil registry to simulate registration failure
	o := &Orbit{
		config: Config{
			BranchName: "feature/test",
			WorkingDir: t.TempDir(),
		},
		registry: nil, // No registry - should not panic or error
	}

	// These should not panic with nil registry
	_, err := o.registerRun()
	if err != nil {
		t.Errorf("registerRun() with nil registry should return nil error, got %v", err)
	}

	o.updatePhaseStatus(1, registry.PhaseStatusRunning, 1)
	o.updateRunStatus(registry.StatusCompleted)
	// If we get here without panic, test passes
}

func TestOrbit_RegistryIntegration_PhaseFailedStatus(t *testing.T) {
	// Test that phase status is set to failed when phase fails
	tempDir := t.TempDir()
	registryDir := filepath.Join(tempDir, "runs")

	reg, err := registry.New(registryDir)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	logDir := filepath.Join(tempDir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatalf("Failed to create log dir: %v", err)
	}

	o := &Orbit{
		config: Config{
			BranchName: "feature/test",
			WorkingDir: tempDir,
			LogDir:     logDir,
		},
		registry: reg,
	}

	// Register and start a phase
	runID, err := o.registerRun()
	if err != nil {
		t.Fatalf("registerRun() returned error: %v", err)
	}
	o.runID = runID
	o.updatePhaseStatus(1, registry.PhaseStatusRunning, 1)

	// Fail the phase
	o.updatePhaseStatus(1, registry.PhaseStatusFailed, 1)

	// Verify phase status
	entry, err := reg.Get(runID)
	if err != nil {
		t.Fatalf("Failed to get registry entry: %v", err)
	}
	if entry.Phases[0].Status != registry.PhaseStatusFailed {
		t.Errorf("Phase status = %q, want %q", entry.Phases[0].Status, registry.PhaseStatusFailed)
	}
}

func TestOrbit_RegistryIntegration_RunCountIncrement(t *testing.T) {
	// Test that run_count increments on retry
	tempDir := t.TempDir()
	registryDir := filepath.Join(tempDir, "runs")

	reg, err := registry.New(registryDir)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	logDir := filepath.Join(tempDir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatalf("Failed to create log dir: %v", err)
	}

	o := &Orbit{
		config: Config{
			BranchName: "feature/test",
			WorkingDir: tempDir,
			LogDir:     logDir,
		},
		registry: reg,
	}

	runID, err := o.registerRun()
	if err != nil {
		t.Fatalf("registerRun() returned error: %v", err)
	}
	o.runID = runID

	// First attempt at phase 1
	o.updatePhaseStatus(1, registry.PhaseStatusRunning, 1)
	o.updatePhaseStatus(1, registry.PhaseStatusFailed, 1)

	// Second attempt at phase 1
	o.updatePhaseStatus(1, registry.PhaseStatusRunning, 2)
	o.updatePhaseStatus(1, registry.PhaseStatusCompleted, 2)

	// Verify run_count
	entry, err := reg.Get(runID)
	if err != nil {
		t.Fatalf("Failed to get registry entry: %v", err)
	}
	if len(entry.Phases) != 1 {
		t.Fatalf("Expected 1 phase, got %d", len(entry.Phases))
	}
	if entry.Phases[0].RunCount != 2 {
		t.Errorf("RunCount = %d, want 2", entry.Phases[0].RunCount)
	}
}

// --- Integration Tests (Phase 7) ---

func TestIntegration_RunWithoutConfig(t *testing.T) {
	// Test that orbit run with no .orbit.yaml fails with exit code 1
	// and a message directing the user to run 'orbit init'.
	//
	// This test verifies requirement 2.2: When a user runs Orbit without
	// a .orbit.yaml file, the system SHALL fail with exit code 1 and
	// a message directing the user to run `orbit init`.

	// Create a temp directory with no .orbit.yaml
	tempDir := t.TempDir()
	homeDir := t.TempDir()

	// Isolate from real home config
	t.Setenv("HOME", homeDir)

	// Import config package to test RequireConfigFile
	cfg := configPkg.Load(tempDir)

	// Verify config file was not found
	if cfg.ConfigFileFound {
		t.Error("expected ConfigFileFound to be false when no config exists")
	}

	// Verify RequireConfigFile returns appropriate error
	err := cfg.RequireConfigFile()
	if err == nil {
		t.Fatal("expected error when config file not found")
	}

	// Verify error message mentions orbit init
	assert.Contains(t, err.Error(), "orbit init", "expected error to mention 'orbit init', got: %v", err)

	// Verify error message mentions .orbit.yaml
	assert.Contains(t, err.Error(), ".orbit.yaml", "expected error to mention '.orbit.yaml', got: %v", err)
}

func TestIntegration_VariantRunWithDifferentModels(t *testing.T) {
	// Test that variant runs with different model aliases correctly populate
	// the variants.json metadata with alias name, agent type, and model.
	//
	// This test verifies requirements:
	// - 4.4: The system SHALL store the resolved alias name in variant metadata
	// - 7.1: The variant metadata SHALL include the alias name and model used

	tempDir := t.TempDir()
	homeDir := t.TempDir()

	// Isolate from real home config
	t.Setenv("HOME", homeDir)

	// Create .orbit.yaml with two agent aliases using different models
	orbitConfig := `agents:
  claude-sonnet:
    type: claude-code
    model: claude-sonnet-4-20250514
  claude-opus:
    type: claude-code
    model: claude-opus-4-20250514
`
	if err := os.WriteFile(filepath.Join(tempDir, ".orbit.yaml"), []byte(orbitConfig), 0644); err != nil {
		t.Fatalf("failed to create .orbit.yaml: %v", err)
	}

	// Load and validate config
	cfg := configPkg.Load(tempDir)
	if !cfg.ConfigFileFound {
		t.Fatal("expected config file to be found")
	}

	if err := cfg.RequireConfigFile(); err != nil {
		t.Fatalf("RequireConfigFile failed: %v", err)
	}

	if err := cfg.ResolveAliases(); err != nil {
		t.Fatalf("ResolveAliases failed: %v", err)
	}

	// Verify both aliases are resolved
	if len(cfg.ResolvedAgents) != 2 {
		t.Fatalf("expected 2 resolved agents, got %d", len(cfg.ResolvedAgents))
	}

	// Test variant agent assignment with different aliases
	variantAgents := []string{"claude-sonnet", "claude-opus"}

	// Verify that each alias resolves correctly
	for i, aliasName := range variantAgents {
		resolved, err := cfg.GetResolvedAgent(aliasName)
		if err != nil {
			t.Fatalf("failed to resolve agent alias %q: %v", aliasName, err)
		}

		// Verify agent type is correct
		if resolved.Type != "claude-code" {
			t.Errorf("variant %d: expected Type %q, got %q", i+1, "claude-code", resolved.Type)
		}

		// Verify model is set correctly
		expectedModels := map[string]string{
			"claude-sonnet": "claude-sonnet-4-20250514",
			"claude-opus":   "claude-opus-4-20250514",
		}
		expectedModel := expectedModels[aliasName]
		if resolved.Config.Model != expectedModel {
			t.Errorf("variant %d: expected Model %q, got %q", i+1, expectedModel, resolved.Config.Model)
		}

		// Verify the model is correctly passed via GetResolvedAgentConfig
		agentCfg := configPkg.GetResolvedAgentConfig(resolved)
		if agentCfg.Options == nil {
			t.Errorf("variant %d: expected Options to be set", i+1)
		} else if agentCfg.Options["model"] != expectedModel {
			t.Errorf("variant %d: expected Options[model] = %q, got %q", i+1, expectedModel, agentCfg.Options["model"])
		}
	}

	// Test that variant cycling works correctly (fewer agents than variants cycles)
	variantAgents = []string{"claude-sonnet", "claude-opus"}
	variantCount := 4

	for i := range variantCount {
		expectedAlias := variantAgents[i%len(variantAgents)]
		resolved, err := cfg.GetResolvedAgent(expectedAlias)
		if err != nil {
			t.Fatalf("failed to resolve cycling agent alias: %v", err)
		}

		// Just verify it resolves - the cycling behavior is tested in variants package
		if resolved.Type != "claude-code" {
			t.Errorf("cycling variant %d: expected Type %q, got %q", i+1, "claude-code", resolved.Type)
		}
	}
}

// --- Single-Run Hooks Tests (Phase 5) ---

func TestRunAgentPreCommand_Success(t *testing.T) {
	tempDir := t.TempDir()

	// Use TestAgent with empty scenario - agent.Run() won't be called
	scenario := testutil.NewScenario().Build()
	agent := testutil.NewTestAgent(t, "test-agent", scenario)

	dbg, _ := debug.NewLogger(debug.LoggerConfig{})
	defer dbg.Close()

	o := &Orbit{
		config: Config{
			AgentPreCommand: "echo pre-command-executed",
			WorkingDir:      tempDir,
			CommandTimeout:  30 * time.Second,
		},
		agent:       agent,
		runeClient:  rune.NewClient(filepath.Join(tempDir, "tasks.md")),
		shutdownCtx: t.Context(),
		debug:       dbg,
	}

	err := o.runAgentPreCommand()
	if err != nil {
		t.Errorf("runAgentPreCommand() returned error: %v", err)
	}
}

func TestRunAgentPreCommand_SkipsWhenEmpty(t *testing.T) {
	tempDir := t.TempDir()

	// Use TestAgent with empty scenario - agent.Run() won't be called
	scenario := testutil.NewScenario().Build()
	agent := testutil.NewTestAgent(t, "test-agent", scenario)

	dbg, _ := debug.NewLogger(debug.LoggerConfig{})
	defer dbg.Close()

	o := &Orbit{
		config: Config{
			AgentPreCommand: "", // Empty - should skip
			WorkingDir:      tempDir,
			CommandTimeout:  30 * time.Second,
		},
		agent:       agent,
		runeClient:  rune.NewClient(filepath.Join(tempDir, "tasks.md")),
		shutdownCtx: t.Context(),
		debug:       dbg,
	}

	err := o.runAgentPreCommand()
	if err != nil {
		t.Errorf("runAgentPreCommand() should not return error when command is empty: %v", err)
	}
}

func TestRunAgentPreCommand_FailureAbortsRun(t *testing.T) {
	tempDir := t.TempDir()

	// Use TestAgent with empty scenario - agent.Run() won't be called
	scenario := testutil.NewScenario().Build()
	agent := testutil.NewTestAgent(t, "test-agent", scenario)

	dbg, _ := debug.NewLogger(debug.LoggerConfig{})
	defer dbg.Close()

	o := &Orbit{
		config: Config{
			AgentPreCommand: "exit 1", // Command that fails
			WorkingDir:      tempDir,
			CommandTimeout:  30 * time.Second,
		},
		agent:       agent,
		runeClient:  rune.NewClient(filepath.Join(tempDir, "tasks.md")),
		shutdownCtx: t.Context(),
		debug:       dbg,
	}

	err := o.runAgentPreCommand()
	if err == nil {
		t.Error("runAgentPreCommand() should return error when command fails")
	}
	assert.Contains(t, err.Error(), "pre-command failed", "expected 'pre-command failed' in error message, got: %v", err)
}

func TestRunAgentPostCommand_Success(t *testing.T) {
	tempDir := t.TempDir()

	// Use TestAgent with empty scenario - agent.Run() won't be called
	scenario := testutil.NewScenario().Build()
	agent := testutil.NewTestAgent(t, "test-agent", scenario)

	dbg, _ := debug.NewLogger(debug.LoggerConfig{})
	defer dbg.Close()

	o := &Orbit{
		config: Config{
			AgentPostCommand: "echo post-command-executed",
			WorkingDir:       tempDir,
			CommandTimeout:   30 * time.Second,
		},
		agent:       agent,
		runeClient:  rune.NewClient(filepath.Join(tempDir, "tasks.md")),
		shutdownCtx: t.Context(),
		debug:       dbg,
	}

	err := o.runAgentPostCommand()
	if err != nil {
		t.Errorf("runAgentPostCommand() returned error: %v", err)
	}
}

func TestRunAgentPostCommand_FailureWarns(t *testing.T) {
	tempDir := t.TempDir()

	// Use TestAgent with empty scenario - agent.Run() won't be called
	scenario := testutil.NewScenario().Build()
	agent := testutil.NewTestAgent(t, "test-agent", scenario)

	dbg, _ := debug.NewLogger(debug.LoggerConfig{})
	defer dbg.Close()

	o := &Orbit{
		config: Config{
			AgentPostCommand: "exit 1", // Command that fails
			WorkingDir:       tempDir,
			CommandTimeout:   30 * time.Second,
		},
		agent:       agent,
		runeClient:  rune.NewClient(filepath.Join(tempDir, "tasks.md")),
		shutdownCtx: t.Context(),
		debug:       dbg,
	}

	// Post-command failure should NOT return an error (warns only)
	err := o.runAgentPostCommand()
	if err != nil {
		t.Errorf("runAgentPostCommand() should not return error on failure (warning only): %v", err)
	}
}

func TestRunAgentPostCommand_SkipsWhenEmpty(t *testing.T) {
	tempDir := t.TempDir()

	// Use TestAgent with empty scenario - agent.Run() won't be called
	scenario := testutil.NewScenario().Build()
	agent := testutil.NewTestAgent(t, "test-agent", scenario)

	dbg, _ := debug.NewLogger(debug.LoggerConfig{})
	defer dbg.Close()

	o := &Orbit{
		config: Config{
			AgentPostCommand: "", // Empty - should skip
			WorkingDir:       tempDir,
			CommandTimeout:   30 * time.Second,
		},
		agent:       agent,
		runeClient:  rune.NewClient(filepath.Join(tempDir, "tasks.md")),
		shutdownCtx: t.Context(),
		debug:       dbg,
	}

	err := o.runAgentPostCommand()
	if err != nil {
		t.Errorf("runAgentPostCommand() should not return error when command is empty: %v", err)
	}
}

func TestRunPrePrompt_Success(t *testing.T) {
	tempDir := t.TempDir()

	// Define expected agent behavior - returns session ID
	scenario := testutil.NewScenario().
		Success("pre-prompt-session-123", 0.01).
		WithOutput("Pre-prompt executed successfully", "").
		Build()

	agent := testutil.NewTestAgent(t, "test-agent", scenario)
	t.Cleanup(func() { agent.AssertAllConsumed(t) })

	dbg, _ := debug.NewLogger(debug.LoggerConfig{})
	defer dbg.Close()

	o := &Orbit{
		config: Config{
			PrePrompt:      "Review the codebase before starting",
			WorkingDir:     tempDir,
			CommandTimeout: 30 * time.Second,
		},
		agent:       agent,
		shutdownCtx: t.Context(),
		debug:       dbg,
	}

	err := o.runPrePrompt()
	if err != nil {
		t.Errorf("runPrePrompt() returned error: %v", err)
	}

	// Verify session ID was stored for phase 1
	if o.prePromptSessionID != "pre-prompt-session-123" {
		t.Errorf("prePromptSessionID = %q, want %q", o.prePromptSessionID, "pre-prompt-session-123")
	}

	// Verify agent was called exactly once
	agent.Recorder().AssertCallCount(t, 1)
}

func TestRunPrePrompt_SkipsWhenEmpty(t *testing.T) {
	tempDir := t.TempDir()

	// Use TestAgent with empty scenario - agent should NOT be called
	scenario := testutil.NewScenario().Build()
	agent := testutil.NewTestAgent(t, "test-agent", scenario)

	dbg, _ := debug.NewLogger(debug.LoggerConfig{})
	defer dbg.Close()

	o := &Orbit{
		config: Config{
			PrePrompt:      "", // Empty - should skip
			WorkingDir:     tempDir,
			CommandTimeout: 30 * time.Second,
		},
		agent:       agent,
		shutdownCtx: t.Context(),
		debug:       dbg,
	}

	err := o.runPrePrompt()
	if err != nil {
		t.Errorf("runPrePrompt() should not return error when prompt is empty: %v", err)
	}

	// Verify agent was NOT called
	agent.Recorder().AssertCallCount(t, 0)
}

func TestRunPrePrompt_FailureAbortsRun(t *testing.T) {
	tempDir := t.TempDir()

	// Define expected agent behavior - fatal error
	scenario := testutil.NewScenario().
		FatalError("agent execution failed").
		Build()

	agent := testutil.NewTestAgent(t, "test-agent", scenario)
	t.Cleanup(func() { agent.AssertAllConsumed(t) })

	dbg, _ := debug.NewLogger(debug.LoggerConfig{})
	defer dbg.Close()

	o := &Orbit{
		config: Config{
			PrePrompt:      "Review the codebase",
			WorkingDir:     tempDir,
			CommandTimeout: 30 * time.Second,
		},
		agent:       agent,
		shutdownCtx: t.Context(),
		debug:       dbg,
	}

	err := o.runPrePrompt()
	if err == nil {
		t.Error("runPrePrompt() should return error when agent fails")
	}
	assert.Contains(t, err.Error(), "pre-prompt failed", "expected 'pre-prompt failed' in error message, got: %v", err)

	// Verify agent was called exactly once
	agent.Recorder().AssertCallCount(t, 1)
}

func TestRunPrePrompt_ResumesCompletedSession(t *testing.T) {
	tempDir := t.TempDir()

	// Create log manager with completed pre-prompt state
	logManager, err := logs.NewManagerWithOptions(tempDir, "test-branch", tempDir, logs.ManagerOptions{UseSubdirs: false})
	if err != nil {
		t.Fatalf("Failed to create log manager: %v", err)
	}

	// Start and complete pre-prompt to simulate previous run
	_, _, err = logManager.StartPrePrompt(false)
	if err != nil {
		t.Fatalf("StartPrePrompt() returned error: %v", err)
	}
	if err := logManager.CompletePrePrompt("completed-session-id"); err != nil {
		t.Fatalf("CompletePrePrompt() returned error: %v", err)
	}

	// Use TestAgent with empty scenario - agent should NOT be called
	scenario := testutil.NewScenario().Build()
	agent := testutil.NewTestAgent(t, "test-agent", scenario)

	dbg, _ := debug.NewLogger(debug.LoggerConfig{})
	defer dbg.Close()

	o := &Orbit{
		config: Config{
			PrePrompt:      "Review the codebase",
			WorkingDir:     tempDir,
			CommandTimeout: 30 * time.Second,
		},
		agent:       agent,
		logManager:  logManager,
		shutdownCtx: t.Context(),
		debug:       dbg,
	}

	err = o.runPrePrompt()
	if err != nil {
		t.Errorf("runPrePrompt() returned error: %v", err)
	}

	// Should use the stored session ID
	if o.prePromptSessionID != "completed-session-id" {
		t.Errorf("prePromptSessionID = %q, want %q", o.prePromptSessionID, "completed-session-id")
	}

	// Verify agent was NOT called (pre-prompt was already completed)
	agent.Recorder().AssertCallCount(t, 0)
}

func TestRunPhase_UsesPrePromptSession(t *testing.T) {
	tempDir := t.TempDir()

	// Create scenario that returns success
	scenario := testutil.NewScenario().
		Success("pre-prompt-session-123", 0.0).
		Build()

	agent := testutil.NewTestAgent(t, "mock", scenario)
	t.Cleanup(func() { agent.AssertAllConsumed(t) })

	dbg, _ := debug.NewLogger(debug.LoggerConfig{})
	defer dbg.Close()

	o := &Orbit{
		config: Config{
			WorkingDir: tempDir,
		},
		agent:              agent,
		errorClassifier:    agents.GetClassifier("claude-code"),
		prePromptSessionID: "pre-prompt-session-123", // Simulates pre-prompt having completed
		shutdownCtx:        context.Background(),
		debug:              dbg,
	}

	err := o.runPhase(1) // Phase 1 should use pre-prompt session
	if err != nil {
		t.Fatalf("runPhase() returned error: %v", err)
	}

	// Phase 1 should resume the pre-prompt session
	calls := agent.Recorder().Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Method != "Resume" {
		t.Errorf("Phase 1 should call Resume when pre-prompt session exists, got %s", calls[0].Method)
	}
	if calls[0].SessionID != "pre-prompt-session-123" {
		t.Errorf("Phase 1 should use pre-prompt session ID. Got %q, want %q", calls[0].SessionID, "pre-prompt-session-123")
	}
}

func TestRunPhase_DoesNotUsePrePromptSessionForPhase2(t *testing.T) {
	tempDir := t.TempDir()

	// Create scenario that returns success
	scenario := testutil.NewScenario().
		Success("new-session", 0.0).
		Build()

	agent := testutil.NewTestAgent(t, "mock", scenario)
	t.Cleanup(func() { agent.AssertAllConsumed(t) })

	dbg, _ := debug.NewLogger(debug.LoggerConfig{})
	defer dbg.Close()

	o := &Orbit{
		config: Config{
			WorkingDir: tempDir,
		},
		agent:              agent,
		errorClassifier:    agents.GetClassifier("claude-code"),
		prePromptSessionID: "pre-prompt-session-123",
		shutdownCtx:        context.Background(),
		debug:              dbg,
	}

	err := o.runPhase(2) // Phase 2 should NOT use pre-prompt session
	if err != nil {
		t.Fatalf("runPhase() returned error: %v", err)
	}

	// Phase 2 should not use pre-prompt session
	calls := agent.Recorder().Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Method != "Run" {
		t.Errorf("Phase 2 should call Run (not Resume) when pre-prompt session exists, got %s", calls[0].Method)
	}
	if calls[0].Options.SessionID == "pre-prompt-session-123" {
		t.Error("Phase 2 should NOT use pre-prompt session ID")
	}
}

func TestDryRun_PrintsHooksWithoutExecuting(t *testing.T) {
	tempDir := t.TempDir()

	// Use TestAgent with empty scenario - agent should NOT be called
	scenario := testutil.NewScenario().Build()
	agent := testutil.NewTestAgent(t, "test-agent", scenario)

	dbg, _ := debug.NewLogger(debug.LoggerConfig{})
	defer dbg.Close()

	o := &Orbit{
		config: Config{
			DryRun:           true,
			AgentPreCommand:  "make lint",
			AgentPostCommand: "make format",
			PrePrompt:        "Review the codebase",
			WorkingDir:       tempDir,
			CommandTimeout:   30 * time.Second,
		},
		agent:       agent,
		runeClient:  rune.NewClient(filepath.Join(tempDir, "tasks.md")),
		shutdownCtx: t.Context(),
		debug:       dbg,
	}

	// Pre-command in dry-run should not execute
	err := o.runAgentPreCommand()
	if err != nil {
		t.Errorf("runAgentPreCommand() in dry-run should not return error: %v", err)
	}

	// Post-command in dry-run should not execute
	err = o.runAgentPostCommand()
	if err != nil {
		t.Errorf("runAgentPostCommand() in dry-run should not return error: %v", err)
	}

	// Pre-prompt in dry-run should not execute
	err = o.runPrePrompt()
	if err != nil {
		t.Errorf("runPrePrompt() in dry-run should not return error: %v", err)
	}

	// Verify agent was NOT called in dry-run mode
	agent.Recorder().AssertCallCount(t, 0)
}

func TestExecutionOrder_SkipsUnconfigured(t *testing.T) {
	tempDir := t.TempDir()

	// Use TestAgent with empty scenario - agent should NOT be called
	scenario := testutil.NewScenario().Build()
	agent := testutil.NewTestAgent(t, "test-agent", scenario)

	dbg, _ := debug.NewLogger(debug.LoggerConfig{})
	defer dbg.Close()

	// Empty config - all hooks should be skipped
	o := &Orbit{
		config: Config{
			AgentPreCommand:  "", // Empty - skip
			AgentPostCommand: "", // Empty - skip
			PrePrompt:        "", // Empty - skip
			WorkingDir:       tempDir,
			CommandTimeout:   30 * time.Second,
		},
		agent:       agent,
		runeClient:  rune.NewClient(filepath.Join(tempDir, "tasks.md")),
		shutdownCtx: t.Context(),
		debug:       dbg,
	}

	// All hooks should succeed (no-ops)
	if err := o.runAgentPreCommand(); err != nil {
		t.Errorf("runAgentPreCommand() with empty config returned error: %v", err)
	}
	if err := o.runPrePrompt(); err != nil {
		t.Errorf("runPrePrompt() with empty config returned error: %v", err)
	}
	if err := o.runAgentPostCommand(); err != nil {
		t.Errorf("runAgentPostCommand() with empty config returned error: %v", err)
	}

	// Verify agent was NOT called
	agent.Recorder().AssertCallCount(t, 0)
}

func TestGetCostValue_PremiumRequests(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		result *agents.RunResult
		want   float64
	}{
		"nil result": {
			result: nil,
			want:   0,
		},
		"nil cost": {
			result: &agents.RunResult{},
			want:   0,
		},
		"usd cost": {
			result: &agents.RunResult{
				Cost: &agents.CostMetrics{CostUSD: 1.50},
			},
			want: 1.50,
		},
		"credits cost": {
			result: &agents.RunResult{
				Cost: &agents.CostMetrics{Credits: 3.25},
			},
			want: 3.25,
		},
		"premium requests": {
			result: &agents.RunResult{
				Cost: &agents.CostMetrics{PremiumRequests: 42.5},
			},
			want: 42.5,
		},
		"usd takes precedence over credits": {
			result: &agents.RunResult{
				Cost: &agents.CostMetrics{CostUSD: 2.0, Credits: 5.0},
			},
			want: 2.0,
		},
		"usd takes precedence over premium requests": {
			result: &agents.RunResult{
				Cost: &agents.CostMetrics{CostUSD: 2.0, PremiumRequests: 10.0},
			},
			want: 2.0,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := getCostValue(tc.result)
			if got != tc.want {
				t.Errorf("getCostValue() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRunVariantsSequential_CancelPreservesPending(t *testing.T) {
	t.Parallel()

	dbg, _ := debug.NewLogger(debug.LoggerConfig{})
	defer dbg.Close()

	o := &Orbit{
		config: Config{MaxParallel: 1},
		debug:  dbg,
	}

	// Pre-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	variantList := []*variants.Variant{
		{ID: 1, Status: variants.StatusPending},
		{ID: 2, Status: variants.StatusPending},
		{ID: 3, Status: variants.StatusPending},
	}

	o.runVariantsSequential(ctx, variantList)

	for _, v := range variantList {
		if v.Status != variants.StatusPending {
			t.Errorf("variant %d: status = %q, want %q", v.ID, v.Status, variants.StatusPending)
		}
	}
}

func TestRunVariantsParallel_CancelPreservesPending(t *testing.T) {
	t.Parallel()

	dbg, _ := debug.NewLogger(debug.LoggerConfig{})
	defer dbg.Close()

	o := &Orbit{
		config: Config{MaxParallel: 2},
		debug:  dbg,
	}

	// Pre-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	variantList := []*variants.Variant{
		{ID: 1, Status: variants.StatusPending},
		{ID: 2, Status: variants.StatusPending},
		{ID: 3, Status: variants.StatusPending},
	}

	o.runVariantsParallel(ctx, variantList)

	for _, v := range variantList {
		if v.Status != variants.StatusPending {
			t.Errorf("variant %d: status = %q, want %q", v.ID, v.Status, variants.StatusPending)
		}
	}
}

func TestRunVariantsSequential_CancelPreservesMixedStatuses(t *testing.T) {
	t.Parallel()

	dbg, _ := debug.NewLogger(debug.LoggerConfig{})
	defer dbg.Close()

	o := &Orbit{
		config: Config{MaxParallel: 1},
		debug:  dbg,
	}

	// Pre-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Simulate a partially-completed run: one completed, two still pending
	variantList := []*variants.Variant{
		{ID: 1, Status: variants.StatusCompleted},
		{ID: 2, Status: variants.StatusPending},
		{ID: 3, Status: variants.StatusPending},
	}

	o.runVariantsSequential(ctx, variantList)

	// Completed variants must stay completed
	if variantList[0].Status != variants.StatusCompleted {
		t.Errorf("variant 1: status = %q, want %q", variantList[0].Status, variants.StatusCompleted)
	}
	// Pending variants must stay pending (not cancelled)
	for _, v := range variantList[1:] {
		if v.Status != variants.StatusPending {
			t.Errorf("variant %d: status = %q, want %q", v.ID, v.Status, variants.StatusPending)
		}
	}
}

// nilResultAgent is a minimal agent mock that returns (nil, error) from Run,
// reproducing the scenario where an agent fails before producing any result.
type nilResultAgent struct {
	callCount int
	maxNils   int // How many nil results to return before succeeding
}

func (a *nilResultAgent) Name() string              { return "nil-result-mock" }
func (a *nilResultAgent) CLICommand() string        { return "mock" }
func (a *nilResultAgent) IsInstalled() bool         { return true }
func (a *nilResultAgent) Version() (string, error)  { return "1.0.0", nil }
func (a *nilResultAgent) DefaultSessionDir() string { return "" }
func (a *nilResultAgent) DiscoverSessions(_ context.Context, _ string) ([]agents.SessionInfo, error) {
	return nil, nil
}
func (a *nilResultAgent) Run(_ context.Context, _ agents.RunOptions) (*agents.RunResult, error) {
	a.callCount++
	if a.callCount <= a.maxNils {
		return nil, errors.New("agent binary not found")
	}
	return &agents.RunResult{SessionID: "recovered-session"}, nil
}
func (a *nilResultAgent) Resume(_ context.Context, _ string, opts agents.RunOptions) (*agents.RunResult, error) {
	return a.Run(context.Background(), opts)
}

// TestVariantPhaseRetry_NilRunResult verifies that runVariantPhaseWithRetry
// does not panic when the agent returns (nil, error). This is a regression test
// for T-78: the classifier.Classify call would dereference nil result fields.
func TestVariantPhaseRetry_NilRunResult(t *testing.T) {
	agent := &nilResultAgent{maxNils: 1}

	o := &Orbit{
		config: Config{},
		debug:  debug.New(false, ""),
	}

	v := &variants.Variant{
		ID:           1,
		WorktreePath: t.TempDir(),
	}

	// This should NOT panic. Before the fix, it would panic on nil dereference
	// at classifier.Classify(1, result.Stderr, result.Output, result.Errors).
	// The default classifier returns ErrorClassUnknown (non-retryable), so it
	// returns the ClassifiedError immediately without retrying.
	result, err := o.runVariantPhaseWithRetry(context.Background(), v, agent, "test prompt", "", 0)
	if err == nil {
		t.Fatal("expected ClassifiedError, got nil")
	}

	classified, ok := err.(*agents.ClassifiedError)
	if !ok {
		t.Fatalf("expected *agents.ClassifiedError, got %T: %v", err, err)
	}
	// The error message from the nil-result agent should be passed through to the classifier
	if classified.Message != "agent binary not found" {
		t.Errorf("classified message = %q, want %q", classified.Message, "agent binary not found")
	}
	// result is nil because the agent returned nil
	if result != nil {
		t.Errorf("expected nil result from nil-returning agent, got %+v", result)
	}
}

// TestVariantPhaseRetry_NilRunResult_AllFail verifies that runVariantPhaseWithRetry
// returns an error (not a panic) when all attempts return nil results.
func TestVariantPhaseRetry_NilRunResult_AllFail(t *testing.T) {
	agent := &nilResultAgent{maxNils: maxRetries + 1} // All attempts return nil

	o := &Orbit{
		config: Config{},
		debug:  debug.New(false, ""),
	}

	v := &variants.Variant{
		ID:           1,
		WorktreePath: t.TempDir(),
	}

	// Should not panic, should return a ClassifiedError on first attempt
	// (default classifier marks unknown errors as non-retryable)
	_, err := o.runVariantPhaseWithRetry(context.Background(), v, agent, "test prompt", "", 0)
	if err == nil {
		t.Fatal("expected error when all attempts fail, got nil")
	}
	if agent.callCount != 1 {
		t.Errorf("call count = %d, want 1 (non-retryable stops immediately)", agent.callCount)
	}
}

// TestVariantPostCompletion_NilRunResult verifies that runVariantPostCompletion
// does not panic when the agent returns (nil, error).
func TestVariantPostCompletion_NilRunResult(t *testing.T) {
	agent := &nilResultAgent{maxNils: 1}

	o := &Orbit{
		config: Config{
			PostPrompt: "review implementation",
		},
		debug: debug.New(false, ""),
	}

	v := &variants.Variant{
		ID:           1,
		WorktreePath: t.TempDir(),
	}

	// This should NOT panic. Before the fix, it would panic on nil dereference.
	// Like runVariantPhaseWithRetry, the default classifier returns non-retryable.
	_, err := o.runVariantPostCompletion(t.Context(), v, agent, nil, 0)
	if err == nil {
		t.Fatal("expected ClassifiedError, got nil")
	}
	_, ok := err.(*agents.ClassifiedError)
	if !ok {
		t.Fatalf("expected *agents.ClassifiedError, got %T: %v", err, err)
	}
}

// exitCodeCapturingClassifier records the exit code passed to Classify so tests
// can verify the correct value is forwarded from RunResult.ExitCode.
type exitCodeCapturingClassifier struct {
	capturedExitCode int
}

func (c *exitCodeCapturingClassifier) Classify(exitCode int, stderr, stdout string, errMsgs []string) *agents.ClassifiedError {
	c.capturedExitCode = exitCode
	return &agents.ClassifiedError{
		Class:   agents.ErrorClassUnknown,
		Message: "captured",
	}
}

// TestClassifyFromAgent_PassesExitCode verifies that classifyFromAgent forwards
// result.ExitCode to the classifier instead of a hardcoded value.
// Regression test for T-126.
func TestClassifyFromAgent_PassesExitCode(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		result       *agents.RunResult
		err          error
		wantExitCode int
	}{
		"exit code 42 from result": {
			result:       &agents.RunResult{ExitCode: 42, IsError: true, Errors: []string{"failed"}},
			err:          nil,
			wantExitCode: 42,
		},
		"exit code 0 with error flag": {
			result:       &agents.RunResult{ExitCode: 0, IsError: true, Errors: []string{"failed"}},
			err:          nil,
			wantExitCode: 0,
		},
		"exit code 137 (killed) with error": {
			result:       &agents.RunResult{ExitCode: 137, Stderr: "killed"},
			err:          errors.New("process killed"),
			wantExitCode: 137,
		},
		"exit code 2 from result with error": {
			result:       &agents.RunResult{ExitCode: 2, Stderr: "misuse"},
			err:          errors.New("misuse of shell"),
			wantExitCode: 2,
		},
		"nil result defaults to 1": {
			result:       nil,
			err:          errors.New("agent binary not found"),
			wantExitCode: 1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Register a per-subtest classifier with a unique agent name
			// to avoid shared mutable state and global registry pollution.
			agentName := "exit-code-test-" + name
			var classifier *exitCodeCapturingClassifier
			agents.RegisterClassifier(agentName, func() agents.ErrorClassifier {
				classifier = &exitCodeCapturingClassifier{}
				return classifier
			})
			t.Cleanup(func() { agents.UnregisterClassifier(agentName) })

			classify := classifyFromAgent(agentName)

			classified := classify(tc.result, tc.err)
			if classified == nil {
				t.Fatal("expected non-nil ClassifiedError")
			}
			if classifier.capturedExitCode != tc.wantExitCode {
				t.Errorf("exit code passed to Classify = %d, want %d",
					classifier.capturedExitCode, tc.wantExitCode)
			}
		})
	}
}

// TestDirectClassify_PassesExitCode verifies that the direct o.errorClassifier.Classify
// calls in runPhase and runPostPrompt forward result.ExitCode.
// Regression test for T-126.
func TestDirectClassify_PassesExitCode(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		result       *agents.RunResult
		err          error
		wantExitCode int
	}{
		"result with exit code 42": {
			result:       &agents.RunResult{ExitCode: 42, Stderr: "timeout"},
			err:          errors.New("timeout"),
			wantExitCode: 42,
		},
		"result with exit code 0 and IsError": {
			result:       &agents.RunResult{ExitCode: 0, IsError: true, Errors: []string{"failed"}},
			err:          nil,
			wantExitCode: 0,
		},
		"nil result defaults to 1": {
			result:       nil,
			err:          errors.New("binary not found"),
			wantExitCode: 1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			classifier := &exitCodeCapturingClassifier{}
			o := &Orbit{
				config:          Config{},
				debug:           debug.New(false, ""),
				errorClassifier: classifier,
			}

			classified := o.classifyRunError(tc.result, tc.err)
			if classified == nil {
				t.Fatal("expected non-nil ClassifiedError")
			}
			if classifier.capturedExitCode != tc.wantExitCode {
				t.Errorf("exit code passed to Classify = %d, want %d",
					classifier.capturedExitCode, tc.wantExitCode)
			}
		})
	}
}

func TestRunVariantsParallel_CancelPreservesMixedStatuses(t *testing.T) {
	t.Parallel()

	dbg, _ := debug.NewLogger(debug.LoggerConfig{})
	defer dbg.Close()

	o := &Orbit{
		config: Config{MaxParallel: 2},
		debug:  dbg,
	}

	// Pre-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Simulate a partially-completed run: one completed, two still pending
	variantList := []*variants.Variant{
		{ID: 1, Status: variants.StatusCompleted},
		{ID: 2, Status: variants.StatusPending},
		{ID: 3, Status: variants.StatusPending},
	}

	o.runVariantsParallel(ctx, variantList)

	// Completed variants must stay completed
	if variantList[0].Status != variants.StatusCompleted {
		t.Errorf("variant 1: status = %q, want %q", variantList[0].Status, variants.StatusCompleted)
	}
	// Pending variants must stay pending (not cancelled)
	for _, v := range variantList[1:] {
		if v.Status != variants.StatusPending {
			t.Errorf("variant %d: status = %q, want %q", v.ID, v.Status, variants.StatusPending)
		}
	}
}

// TestVariantPostCompletion_ResumesPriorSession is the regression test for T-715.
//
// Variant post-prompt should mirror single-run post-prompt session lifecycle: it
// must coordinate the session through the variant log manager so that when a
// post-completion is already in progress (interrupted run resumed via
// ContinueSession), the agent's Resume() path is used instead of starting a
// brand-new session via Run() with a fresh UUID.
//
// Before the fix, runVariantPostCompletion always generated a fresh UUID and
// invoked agent.Run(), regardless of the log manager state. Resume() was never
// called for variant post-prompt, diverging from the documented single-run
// semantics.
func TestVariantPostCompletion_ResumesPriorSession(t *testing.T) {
	tmpDir := t.TempDir()

	// Seed a variant log manager that already has an in-progress post-completion
	// state recorded — simulating a prior post-prompt run that was interrupted.
	logManager, err := logs.NewManagerWithOptions(tmpDir, "variant-1", tmpDir, logs.ManagerOptions{UseSubdirs: false})
	if err != nil {
		t.Fatalf("NewManagerWithOptions: %v", err)
	}
	priorSessionID, _, err := logManager.StartPostCompletion(false)
	if err != nil {
		t.Fatalf("StartPostCompletion seeding failed: %v", err)
	}

	// Configure a TestAgent that succeeds on first call. The recorder will let
	// us assert which method (Run vs Resume) was invoked and with which session.
	scenario := testutil.NewScenario().
		Success(priorSessionID, 0.01).
		Build()
	agent := testutil.NewTestAgent(t, "test-agent", scenario)
	t.Cleanup(func() { agent.AssertAllConsumed(t) })

	o := &Orbit{
		config: Config{
			PostPrompt:      "review implementation",
			ContinueSession: true,
		},
		debug: debug.New(false, ""),
	}

	v := &variants.Variant{
		ID:           1,
		WorktreePath: tmpDir,
	}

	// New post-completion call should resume the prior session via Resume().
	result, runErr := o.runVariantPostCompletion(t.Context(), v, agent, logManager, 0)
	if runErr != nil {
		t.Fatalf("runVariantPostCompletion failed: %v", runErr)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	calls := agent.Recorder().Calls()
	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 agent call, got %d", len(calls))
	}
	if calls[0].Method != "Resume" {
		t.Errorf("expected Resume method to be called, got %q", calls[0].Method)
	}
	if calls[0].SessionID != priorSessionID {
		t.Errorf("Resume session id = %q, want %q", calls[0].SessionID, priorSessionID)
	}

	// Post-completion state should be cleared after success — verify via the
	// on-disk summary.json that CompletePostCompletion was called.
	if pc := readPostCompletion(t, logManager.SessionDir()); pc != nil {
		t.Errorf("expected post_completion to be cleared after success, still set: %+v", pc)
	}
}

// readPostCompletion loads summary.json from sessionDir and returns the
// post_completion field as a generic map (or nil if absent). Test helper for
// verifying log-manager state without relying on internal summary access.
func readPostCompletion(t *testing.T, sessionDir string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(sessionDir, "summary.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read summary.json: %v", err)
	}
	var summary map[string]any
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("unmarshal summary.json: %v", err)
	}
	pc, ok := summary["post_completion"].(map[string]any)
	if !ok {
		return nil
	}
	return pc
}

// TestVariantPostCompletion_FreshSessionTracksLogManager verifies that even when
// no prior post-completion exists, the log manager is updated with a session ID
// (so orbit status can show live activity) and the post-completion state is
// cleared after success. Companion regression for T-715.
func TestVariantPostCompletion_FreshSessionTracksLogManager(t *testing.T) {
	tmpDir := t.TempDir()

	logManager, err := logs.NewManagerWithOptions(tmpDir, "variant-1", tmpDir, logs.ManagerOptions{UseSubdirs: false})
	if err != nil {
		t.Fatalf("NewManagerWithOptions: %v", err)
	}

	// Single Run call — agent returns its own session ID, exercising
	// ReconcilePostCompletionSessionID.
	scenario := testutil.NewScenario().
		Success("agent-returned-session", 0.02).
		Build()
	agent := testutil.NewTestAgent(t, "test-agent", scenario)
	t.Cleanup(func() { agent.AssertAllConsumed(t) })

	o := &Orbit{
		config: Config{PostPrompt: "review implementation"},
		debug:  debug.New(false, ""),
	}

	v := &variants.Variant{
		ID:           1,
		WorktreePath: tmpDir,
	}

	result, runErr := o.runVariantPostCompletion(t.Context(), v, agent, logManager, 0)
	if runErr != nil {
		t.Fatalf("runVariantPostCompletion failed: %v", runErr)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	calls := agent.Recorder().Calls()
	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 agent call, got %d", len(calls))
	}
	// Without an in-progress post-completion in the log manager, Run() is the
	// correct method (nothing to resume). The session ID should be a UUID
	// generated through StartPostCompletion, not blank.
	if calls[0].Method != "Run" {
		t.Errorf("expected Run method, got %q", calls[0].Method)
	}
	if calls[0].Options.SessionID == "" {
		t.Errorf("expected a session id to be passed to Run, got empty string")
	}

	// After success, post-completion state should be cleared.
	if pc := readPostCompletion(t, logManager.SessionDir()); pc != nil {
		t.Errorf("expected post_completion to be cleared after success, still set: %+v", pc)
	}
}

// TestVariantPostCompletion_FallsBackToFreshSessionOnInvalid verifies that when
// Resume() fails because the prior session is gone (ErrorClassSessionInvalid),
// runVariantPostCompletion generates a fresh session id, updates the log
// manager via SetPostCompletionSessionID, and retries with Run() in the same
// retry iteration. This is the most nuanced part of the T-715 fix.
func TestVariantPostCompletion_FallsBackToFreshSessionOnInvalid(t *testing.T) {
	tmpDir := t.TempDir()

	logManager, err := logs.NewManagerWithOptions(tmpDir, "variant-1", tmpDir, logs.ManagerOptions{UseSubdirs: false})
	if err != nil {
		t.Fatalf("NewManagerWithOptions: %v", err)
	}
	priorSessionID, _, err := logManager.StartPostCompletion(false)
	if err != nil {
		t.Fatalf("StartPostCompletion seeding failed: %v", err)
	}

	// Resume hits SessionInvalid; the function should regenerate a session id
	// and call Run() within the same Execute closure (no retry-loop bounce).
	scenario := testutil.NewScenario().
		SessionInvalid().
		Success("fresh-agent-session", 0.02).
		Build()
	agent := testutil.NewTestAgent(t, "test-agent", scenario)
	t.Cleanup(func() { agent.AssertAllConsumed(t) })

	o := &Orbit{
		config: Config{
			PostPrompt:      "review implementation",
			ContinueSession: true,
		},
		debug: debug.New(false, ""),
	}

	v := &variants.Variant{
		ID:           1,
		WorktreePath: tmpDir,
	}

	result, runErr := o.runVariantPostCompletion(t.Context(), v, agent, logManager, 0)
	if runErr != nil {
		t.Fatalf("runVariantPostCompletion failed: %v", runErr)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	calls := agent.Recorder().Calls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 agent calls (Resume + Run fallback), got %d", len(calls))
	}
	if calls[0].Method != "Resume" {
		t.Errorf("call 0: expected Resume, got %q", calls[0].Method)
	}
	if calls[0].SessionID != priorSessionID {
		t.Errorf("call 0: Resume session id = %q, want %q", calls[0].SessionID, priorSessionID)
	}
	if calls[1].Method != "Run" {
		t.Errorf("call 1: expected Run, got %q", calls[1].Method)
	}
	if calls[1].Options.SessionID == "" || calls[1].Options.SessionID == priorSessionID {
		t.Errorf("call 1: Run should use a fresh session id, got %q (prior was %q)",
			calls[1].Options.SessionID, priorSessionID)
	}

	// State should be cleared after the successful fallback Run().
	if pc := readPostCompletion(t, logManager.SessionDir()); pc != nil {
		t.Errorf("expected post_completion to be cleared after success, still set: %+v", pc)
	}
}
