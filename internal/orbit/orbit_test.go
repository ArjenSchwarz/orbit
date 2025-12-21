package orbit

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/arjenschwarz/orbit/internal/claude"
	orberrors "github.com/arjenschwarz/orbit/internal/errors"
)

// mockClaudeClient implements claudeRunner for testing.
type mockClaudeClient struct {
	runPhaseFunc        func() (*claude.SessionResult, error)
	runCustomPromptFunc func(prompt string) (*claude.SessionResult, error)
}

func (m *mockClaudeClient) RunPhase() (*claude.SessionResult, error) {
	if m.runPhaseFunc != nil {
		return m.runPhaseFunc()
	}
	return &claude.SessionResult{}, nil
}

func (m *mockClaudeClient) RunCustomPrompt(prompt string) (*claude.SessionResult, error) {
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
		runPhaseFunc: func() (*claude.SessionResult, error) {
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
		runPhaseFunc: func() (*claude.SessionResult, error) {
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
