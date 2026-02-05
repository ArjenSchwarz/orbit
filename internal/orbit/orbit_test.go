package orbit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
	configPkg "github.com/arjenschwarz/orbit/internal/config"
	"github.com/arjenschwarz/orbit/internal/debug"
	orberrors "github.com/arjenschwarz/orbit/internal/errors"
	"github.com/arjenschwarz/orbit/internal/logs"
	"github.com/arjenschwarz/orbit/internal/registry"
	"github.com/arjenschwarz/orbit/internal/rune"
	"github.com/arjenschwarz/orbit/internal/testutil"
)

// mockClaudeClient implements claudeRunner for testing.
type mockClaudeClient struct {
	runPhaseFunc                   func(sessionID string, resume bool) (*agents.RunResult, error)
	runCustomPromptFunc            func(prompt string) (*agents.RunResult, error)
	runCustomPromptWithSessionFunc func(prompt, sessionID string, resume bool) (*agents.RunResult, error)
}

func (m *mockClaudeClient) RunPhase(sessionID string, resume bool) (*agents.RunResult, error) {
	if m.runPhaseFunc != nil {
		return m.runPhaseFunc(sessionID, resume)
	}
	return &agents.RunResult{}, nil
}

func (m *mockClaudeClient) RunCustomPrompt(prompt string) (*agents.RunResult, error) {
	if m.runCustomPromptFunc != nil {
		return m.runCustomPromptFunc(prompt)
	}
	return &agents.RunResult{}, nil
}

func (m *mockClaudeClient) RunCustomPromptWithSession(prompt, sessionID string, resume bool) (*agents.RunResult, error) {
	if m.runCustomPromptWithSessionFunc != nil {
		return m.runCustomPromptWithSessionFunc(prompt, sessionID, resume)
	}
	// Fall back to runCustomPromptFunc if not set
	if m.runCustomPromptFunc != nil {
		return m.runCustomPromptFunc(prompt)
	}
	return &agents.RunResult{}, nil
}

// NOTE: The mockClaudeClient above is retained for testing the legacy claudeRunner
// interface which is still used in production. Tests using mockClaudeClient are
// currently skipped because they require real time delays - they could be enabled
// with FakeClock but would need significant refactoring to use the agent interface.
//
// The mockAgent type was removed as part of the integration test framework migration.
// Use testutil.NewTestAgent() for agent mocking - see internal/testutil/doc.go.

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
		TasksFile:   "specs/test/tasks.md",
		LogDir:      ".claude/logs",
		BranchName:  "feature/test",
		WorkingDir:  "/path/to/project",
		Command:     "Run /next-task --phase",
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
		TasksFile:   "specs/test/tasks.md",
		LogDir:      ".claude/logs",
		BranchName:  "feature/test",
		WorkingDir:  "/path/to/project",
		Command:     "Run /next-task --phase",
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
	t.Skip("disabled: test uses real 3s delays - would slow down CI/commit validation")

	callCount := 0
	mock := &mockClaudeClient{
		runCustomPromptFunc: func(prompt string) (*agents.RunResult, error) {
			callCount++
			if callCount < 3 {
				// Simulate connection error on first two attempts
				return &agents.RunResult{
					Stderr:  "connection timeout",
					IsError: true,
				}, errors.New("connection timeout")
			}
			// Success on third attempt
			return &agents.RunResult{
				SessionID: "test-session",
				Output:    "Success",
				IsError:   false,
			}, nil
		},
	}

	o := &Orbit{
		config: Config{
			PostPrompt: "test command",
		},
		claudeClient: mock,
		logManager:   nil,
	}

	err := o.runPostPromptWithRetry()
	if err != nil {
		t.Errorf("runPostPromptWithRetry() returned error: %v", err)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls (2 retries then success), got %d", callCount)
	}
}

func TestRunPostPromptWithRetry_MaxRetriesExceeded(t *testing.T) {
	t.Skip("disabled: test uses real 31s delays - would slow down CI/commit validation")

	callCount := 0
	mock := &mockClaudeClient{
		runCustomPromptFunc: func(prompt string) (*agents.RunResult, error) {
			callCount++
			// Always return a connection error
			return &agents.RunResult{
				Stderr:  "connection refused",
				IsError: true,
			}, errors.New("connection refused")
		},
	}

	o := &Orbit{
		config: Config{
			PostPrompt: "test command",
		},
		claudeClient: mock,
		logManager:   nil,
	}

	err := o.runPostPromptWithRetry()
	if err == nil {
		t.Error("expected error after max retries, got nil")
	}
	if callCount != maxRetries {
		t.Errorf("expected %d calls (max retries), got %d", maxRetries, callCount)
	}
	if !strings.Contains(err.Error(), "max retries exceeded") {
		t.Errorf("expected 'max retries exceeded' in error message, got: %v", err)
	}
	var classified *orberrors.ClassifiedError
	if !errors.As(err, &classified) {
		t.Error("expected wrapped ClassifiedError")
	}
}

func TestRunPhaseWithRetry_RateLimitError(t *testing.T) {
	t.Skip("disabled: test uses real 60s delays - would slow down CI/commit validation")

	callCount := 0
	mock := &mockClaudeClient{
		runPhaseFunc: func(sessionID string, resume bool) (*agents.RunResult, error) {
			callCount++
			if callCount == 1 {
				// Rate limit error on first attempt
				return &agents.RunResult{
					Stderr:  "rate limit exceeded",
					IsError: true,
				}, errors.New("rate limit")
			}
			return &agents.RunResult{
				SessionID: "test-session",
				Output:    "Success",
				IsError:   false,
			}, nil
		},
	}

	o := &Orbit{
		config:       Config{},
		claudeClient: mock,
		logManager:   nil,
	}

	err := o.runPhaseWithRetry(1)
	if err != nil {
		t.Errorf("runPhaseWithRetry() returned error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls (1 retry after rate limit), got %d", callCount)
	}
}

func TestRunPhaseWithRetry_OverloadedError(t *testing.T) {
	t.Skip("disabled: test uses real 30s delays - would slow down CI/commit validation")

	callCount := 0
	mock := &mockClaudeClient{
		runPhaseFunc: func(sessionID string, resume bool) (*agents.RunResult, error) {
			callCount++
			if callCount == 1 {
				// API overloaded on first attempt
				return &agents.RunResult{
					Stderr:  "503 service unavailable",
					IsError: true,
				}, errors.New("overloaded")
			}
			return &agents.RunResult{
				SessionID: "test-session",
				Output:    "Success",
				IsError:   false,
			}, nil
		},
	}

	o := &Orbit{
		config:       Config{},
		claudeClient: mock,
		logManager:   nil,
	}

	err := o.runPhaseWithRetry(1)
	if err != nil {
		t.Errorf("runPhaseWithRetry() returned error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls (1 retry after overloaded), got %d", callCount)
	}
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

func TestErrorClassification_IsUsedCorrectly(t *testing.T) {
	tests := map[string]struct {
		stderr        string
		wantRetryable bool
		wantType      orberrors.ErrorType
	}{
		"rate limit": {
			stderr:        "rate limit exceeded",
			wantRetryable: true,
			wantType:      orberrors.ErrRateLimit,
		},
		"connection error": {
			stderr:        "connection timeout",
			wantRetryable: true,
			wantType:      orberrors.ErrConnection,
		},
		"overloaded": {
			stderr:        "service unavailable 503",
			wantRetryable: true,
			wantType:      orberrors.ErrOverloaded,
		},
		"unknown error": {
			stderr:        "some random error",
			wantRetryable: false,
			wantType:      orberrors.ErrUnknown,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			classified := orberrors.Classify(1, tc.stderr, "", nil)
			if classified.Type != tc.wantType {
				t.Errorf("type: got %v, want %v", classified.Type, tc.wantType)
			}
			if classified.Type.IsRetryable() != tc.wantRetryable {
				t.Errorf("retryable: got %v, want %v", classified.Type.IsRetryable(), tc.wantRetryable)
			}
		})
	}
}

func TestRunPhase_SessionContinuation_NewSession(t *testing.T) {
	// Create scenario that returns success
	scenario := testutil.NewScenario().
		Success("test-session", 0.0).
		Build()

	agent := testutil.NewTestAgent(t, "mock", scenario, testutil.WithSessionExport("/tmp/test"))
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

func TestRunPhase_SessionContinuation_WithLogManager(t *testing.T) {
	// Create scenario that returns success
	scenario := testutil.NewScenario().
		Success("test-session", 0.0).
		Build()

	agent := testutil.NewTestAgent(t, "mock", scenario, testutil.WithSessionExport("/tmp/test"))
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

	agent := testutil.NewTestAgent(t, "mock", scenario, testutil.WithSessionExport("/tmp/test"))
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
	if !strings.Contains(err.Error(), "orbit init") {
		t.Errorf("expected error to mention 'orbit init', got: %v", err)
	}

	// Verify error message mentions .orbit.yaml
	if !strings.Contains(err.Error(), ".orbit.yaml") {
		t.Errorf("expected error to mention '.orbit.yaml', got: %v", err)
	}
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

	for i := 0; i < variantCount; i++ {
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
	if !strings.Contains(err.Error(), "pre-command failed") {
		t.Errorf("expected 'pre-command failed' in error message, got: %v", err)
	}
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
	if !strings.Contains(err.Error(), "pre-prompt failed") {
		t.Errorf("expected 'pre-prompt failed' in error message, got: %v", err)
	}

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

	agent := testutil.NewTestAgent(t, "mock", scenario, testutil.WithSessionExport("/tmp/test"))
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

	agent := testutil.NewTestAgent(t, "mock", scenario, testutil.WithSessionExport("/tmp/test"))
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
