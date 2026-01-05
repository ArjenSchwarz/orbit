package orbit

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arjenschwarz/orbit/internal/claude"
	orberrors "github.com/arjenschwarz/orbit/internal/errors"
	"github.com/arjenschwarz/orbit/internal/logs"
	"github.com/arjenschwarz/orbit/internal/registry"
)

// mockClaudeClient implements claudeRunner for testing.
type mockClaudeClient struct {
	runPhaseFunc                   func(sessionID string, resume bool) (*claude.SessionResult, error)
	runCustomPromptFunc            func(prompt string) (*claude.SessionResult, error)
	runCustomPromptWithSessionFunc func(prompt, sessionID string, resume bool) (*claude.SessionResult, error)
}

func (m *mockClaudeClient) RunPhase(sessionID string, resume bool) (*claude.SessionResult, error) {
	if m.runPhaseFunc != nil {
		return m.runPhaseFunc(sessionID, resume)
	}
	return &claude.SessionResult{}, nil
}

func (m *mockClaudeClient) RunCustomPrompt(prompt string) (*claude.SessionResult, error) {
	if m.runCustomPromptFunc != nil {
		return m.runCustomPromptFunc(prompt)
	}
	return &claude.SessionResult{}, nil
}

func (m *mockClaudeClient) RunCustomPromptWithSession(prompt, sessionID string, resume bool) (*claude.SessionResult, error) {
	if m.runCustomPromptWithSessionFunc != nil {
		return m.runCustomPromptWithSessionFunc(prompt, sessionID, resume)
	}
	// Fall back to runCustomPromptFunc if not set
	if m.runCustomPromptFunc != nil {
		return m.runCustomPromptFunc(prompt)
	}
	return &claude.SessionResult{}, nil
}

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
		PostCommand: "Review the implementation",
	}

	if config.Command != "Run /next-task --phase" {
		t.Errorf("Command = %q, want %q", config.Command, "Run /next-task --phase")
	}
	if config.PostCommand != "Review the implementation" {
		t.Errorf("PostCommand = %q, want %q", config.PostCommand, "Review the implementation")
	}
}

func TestConfig_EmptyPostCommand(t *testing.T) {
	config := Config{
		TasksFile:   "specs/test/tasks.md",
		LogDir:      ".claude/logs",
		BranchName:  "feature/test",
		WorkingDir:  "/path/to/project",
		Command:     "Run /next-task --phase",
		PostCommand: "", // Explicitly disabled
	}

	if config.PostCommand != "" {
		t.Errorf("PostCommand should be empty when disabled, got %q", config.PostCommand)
	}
}

func TestMaxRetries_Constant(t *testing.T) {
	if maxRetries != 5 {
		t.Errorf("maxRetries = %d, want 5", maxRetries)
	}
}

func TestComplete_PostCommandSkippedWhenEmpty(t *testing.T) {
	// Create an Orbit instance with empty PostCommand
	o := &Orbit{
		config: Config{
			PostCommand: "",
		},
		logManager: nil, // No log manager for this test
	}

	// Call complete() - should not error because PostCommand is empty
	err := o.complete()
	if err != nil {
		t.Errorf("complete() returned error when PostCommand is empty: %v", err)
	}
}

func TestRunPostCommandWithRetry_Success(t *testing.T) {
	callCount := 0
	mock := &mockClaudeClient{
		runCustomPromptFunc: func(prompt string) (*claude.SessionResult, error) {
			callCount++
			return &claude.SessionResult{
				SessionID: "test-session",
				Cost:      0.01,
				Duration:  time.Second,
				NumTurns:  5,
				Output:    "Success",
				IsError:   false,
			}, nil
		},
	}

	o := &Orbit{
		config: Config{
			PostCommand: "test command",
		},
		claudeClient: mock,
		logManager:   nil,
	}

	err := o.runPostCommandWithRetry()
	if err != nil {
		t.Errorf("runPostCommandWithRetry() returned error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
}

func TestRunPostCommandWithRetry_NonRetryableError(t *testing.T) {
	callCount := 0
	mock := &mockClaudeClient{
		runCustomPromptFunc: func(prompt string) (*claude.SessionResult, error) {
			callCount++
			return &claude.SessionResult{
				Stderr:  "unknown error occurred",
				IsError: true,
			}, errors.New("exit status 1")
		},
	}

	o := &Orbit{
		config: Config{
			PostCommand: "test command",
		},
		claudeClient: mock,
		logManager:   nil,
	}

	err := o.runPostCommandWithRetry()
	if err == nil {
		t.Error("expected error, got nil")
	}
	// Non-retryable errors should not retry
	if callCount != 1 {
		t.Errorf("expected 1 call for non-retryable error, got %d", callCount)
	}
}

func TestRunPostCommandWithRetry_RetryableError_EventualSuccess(t *testing.T) {
	callCount := 0
	mock := &mockClaudeClient{
		runCustomPromptFunc: func(prompt string) (*claude.SessionResult, error) {
			callCount++
			if callCount < 3 {
				// Simulate connection error on first two attempts
				return &claude.SessionResult{
					Stderr:  "connection timeout",
					IsError: true,
				}, errors.New("connection timeout")
			}
			// Success on third attempt
			return &claude.SessionResult{
				SessionID: "test-session",
				Output:    "Success",
				IsError:   false,
			}, nil
		},
	}

	o := &Orbit{
		config: Config{
			PostCommand: "test command",
		},
		claudeClient: mock,
		logManager:   nil,
	}

	err := o.runPostCommandWithRetry()
	if err != nil {
		t.Errorf("runPostCommandWithRetry() returned error: %v", err)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls (2 retries then success), got %d", callCount)
	}
}

func TestRunPostCommandWithRetry_MaxRetriesExceeded(t *testing.T) {
	callCount := 0
	mock := &mockClaudeClient{
		runCustomPromptFunc: func(prompt string) (*claude.SessionResult, error) {
			callCount++
			// Always return a connection error
			return &claude.SessionResult{
				Stderr:  "connection refused",
				IsError: true,
			}, errors.New("connection refused")
		},
	}

	o := &Orbit{
		config: Config{
			PostCommand: "test command",
		},
		claudeClient: mock,
		logManager:   nil,
	}

	err := o.runPostCommandWithRetry()
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
	callCount := 0
	mock := &mockClaudeClient{
		runPhaseFunc: func(sessionID string, resume bool) (*claude.SessionResult, error) {
			callCount++
			if callCount == 1 {
				// Rate limit error on first attempt
				return &claude.SessionResult{
					Stderr:  "rate limit exceeded",
					IsError: true,
				}, errors.New("rate limit")
			}
			return &claude.SessionResult{
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
	callCount := 0
	mock := &mockClaudeClient{
		runPhaseFunc: func(sessionID string, resume bool) (*claude.SessionResult, error) {
			callCount++
			if callCount == 1 {
				// API overloaded on first attempt
				return &claude.SessionResult{
					Stderr:  "503 service unavailable",
					IsError: true,
				}, errors.New("overloaded")
			}
			return &claude.SessionResult{
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
		result *claude.SessionResult
		want   bool
	}{
		"session not found in stderr": {
			result: &claude.SessionResult{
				Stderr: "error: session not found",
				Output: "",
			},
			want: true,
		},
		"session not found in output": {
			result: &claude.SessionResult{
				Stderr: "",
				Output: "Session not found for the given ID",
			},
			want: true,
		},
		"invalid session in stderr": {
			result: &claude.SessionResult{
				Stderr: "invalid session ID provided",
				Output: "",
			},
			want: true,
		},
		"invalid session in output": {
			result: &claude.SessionResult{
				Stderr: "",
				Output: "error: invalid session - cannot be resumed",
			},
			want: true,
		},
		"session expired in stderr": {
			result: &claude.SessionResult{
				Stderr: "session expired",
				Output: "",
			},
			want: true,
		},
		"session expired in output": {
			result: &claude.SessionResult{
				Stderr: "",
				Output: "error: session expired, please start a new one",
			},
			want: true,
		},
		"no such session": {
			result: &claude.SessionResult{
				Stderr: "no such session exists",
				Output: "",
			},
			want: true,
		},
		"case insensitive matching": {
			result: &claude.SessionResult{
				Stderr: "SESSION NOT FOUND",
				Output: "",
			},
			want: true,
		},
		"non-session error returns false": {
			result: &claude.SessionResult{
				Stderr: "rate limit exceeded",
				Output: "",
			},
			want: false,
		},
		"connection error returns false": {
			result: &claude.SessionResult{
				Stderr: "connection timeout",
				Output: "",
			},
			want: false,
		},
		"empty result returns false": {
			result: &claude.SessionResult{
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
			result: &claude.SessionResult{
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
			classified := orberrors.Classify(1, tc.stderr, "")
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
	// Track what RunPhase was called with
	var capturedSessionID string
	var capturedResume bool

	mock := &mockClaudeClient{
		runPhaseFunc: func(sessionID string, resume bool) (*claude.SessionResult, error) {
			capturedSessionID = sessionID
			capturedResume = resume
			return &claude.SessionResult{
				SessionID: sessionID, // Return same session ID
				Output:    "Success",
				IsError:   false,
			}, nil
		},
	}

	// Create a temp dir for log manager
	tempDir := t.TempDir()

	o := &Orbit{
		config: Config{
			ContinueSession: true,
		},
		claudeClient: mock,
		logManager:   nil, // No log manager - should generate fresh session
	}

	err := o.runPhase(1)
	if err != nil {
		t.Errorf("runPhase() returned error: %v", err)
	}

	// Without log manager, should always be a new session (resume=false)
	if capturedResume {
		t.Error("expected resume=false without log manager")
	}

	// Session ID should be non-empty (UUID generated)
	if capturedSessionID == "" {
		t.Error("expected non-empty session ID")
	}

	_ = tempDir // Silence unused warning
}

func TestRunPhase_SessionContinuation_WithLogManager(t *testing.T) {
	// Track calls to RunPhase
	var calls []struct {
		sessionID string
		resume    bool
	}

	mock := &mockClaudeClient{
		runPhaseFunc: func(sessionID string, resume bool) (*claude.SessionResult, error) {
			calls = append(calls, struct {
				sessionID string
				resume    bool
			}{sessionID, resume})
			return &claude.SessionResult{
				SessionID: sessionID,
				Output:    "Success",
				IsError:   false,
			}, nil
		},
	}

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
		claudeClient: mock,
		logManager:   logManager,
	}

	// First run - should start a new session
	if err := o.runPhase(1); err != nil {
		t.Fatalf("First runPhase() returned error: %v", err)
	}

	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}

	// First call should be resume=false (new session)
	if calls[0].resume {
		t.Error("first call should have resume=false")
	}
	firstSessionID := calls[0].sessionID
	if firstSessionID == "" {
		t.Error("expected non-empty session ID")
	}
}

func TestRunPhase_ResumeFallback(t *testing.T) {
	// Track calls to RunPhase
	var calls []struct {
		sessionID string
		resume    bool
	}

	mock := &mockClaudeClient{
		runPhaseFunc: func(sessionID string, resume bool) (*claude.SessionResult, error) {
			calls = append(calls, struct {
				sessionID string
				resume    bool
			}{sessionID, resume})

			// First call with resume=true should fail with session not found
			if resume {
				return &claude.SessionResult{
					Stderr:  "session not found",
					IsError: true,
				}, errors.New("session not found")
			}

			// New session should succeed
			return &claude.SessionResult{
				SessionID: sessionID,
				Output:    "Success",
				IsError:   false,
			}, nil
		},
	}

	// Create log manager in temp directory
	tempDir := t.TempDir()
	logManager, err := logs.NewManagerWithOptions(tempDir, "test-branch", tempDir, logs.ManagerOptions{UseSubdirs: false})
	if err != nil {
		t.Fatalf("Failed to create log manager: %v", err)
	}

	// Manually set a CurrentPhase to simulate resuming
	// We need to call StartPhase first, then simulate an interruption
	sessionID, _, err := logManager.StartPhase(1, true)
	if err != nil {
		t.Fatalf("StartPhase() returned error: %v", err)
	}

	o := &Orbit{
		config: Config{
			ContinueSession: true,
		},
		claudeClient: mock,
		logManager:   logManager,
	}

	// Run phase - should try resume first, then fall back
	if err := o.runPhase(1); err != nil {
		t.Fatalf("runPhase() returned error: %v", err)
	}

	// Should have 2 calls: first with resume=true (failed), second with resume=false
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}

	// First call should be resume=true with original session ID
	if !calls[0].resume {
		t.Error("first call should have resume=true")
	}
	if calls[0].sessionID != sessionID {
		t.Errorf("first call session ID = %q, want %q", calls[0].sessionID, sessionID)
	}

	// Second call should be resume=false with a new session ID
	if calls[1].resume {
		t.Error("second call should have resume=false")
	}
	if calls[1].sessionID == sessionID {
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
