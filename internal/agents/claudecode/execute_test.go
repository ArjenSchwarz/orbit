package claudecode

import (
	"errors"
	"testing"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
)

// TestProcessExecResult_ValidJSON verifies that valid JSON output is parsed correctly.
func TestProcessExecResult_ValidJSON(t *testing.T) {
	t.Parallel()
	agent := New(agents.AgentConfig{}).(*Agent)

	execResult := &agents.ExecuteResult{
		Stdout: []byte(`{
			"type": "result",
			"session_id": "sess-123",
			"result": "All tasks complete",
			"is_error": false,
			"num_turns": 5,
			"total_cost_usd": 0.42,
			"duration_ms": 30000
		}`),
		Duration: 31 * time.Second,
	}

	result, err := agent.processExecResult(execResult, "pre-gen-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Error("IsError should be false for valid JSON with is_error=false")
	}
	if result.SessionID != "sess-123" {
		t.Errorf("SessionID = %q, want %q", result.SessionID, "sess-123")
	}
	if result.Output != "All tasks complete" {
		t.Errorf("Output = %q, want %q", result.Output, "All tasks complete")
	}
	if result.NumTurns != 5 {
		t.Errorf("NumTurns = %d, want 5", result.NumTurns)
	}
	if result.Cost == nil || result.Cost.CostUSD != 0.42 {
		t.Errorf("Cost = %v, want 0.42 USD", result.Cost)
	}
	if result.Duration != 30*time.Second {
		t.Errorf("Duration = %v, want 30s (from API response)", result.Duration)
	}
}

// TestProcessExecResult_NonJSONOutput_ShouldError verifies that non-JSON stdout
// is classified as an error, not silently treated as success.
// This is the primary regression test for T-530.
func TestProcessExecResult_NonJSONOutput_ShouldError(t *testing.T) {
	t.Parallel()
	agent := New(agents.AgentConfig{}).(*Agent)

	execResult := &agents.ExecuteResult{
		Stdout:   []byte("Some plain text output that is not JSON"),
		ExitCode: 0,
		Duration: 5 * time.Second,
	}

	result, err := agent.processExecResult(execResult, "test-session")

	// The error must be surfaced — not silently ignored.
	if err == nil {
		t.Fatal("expected non-nil error for non-JSON output, got nil")
	}
	if !result.IsError {
		t.Error("IsError should be true when stdout is not valid JSON")
	}
	if len(result.Errors) == 0 {
		t.Error("Errors should contain a description of the JSON parse failure")
	}
	if result.Error == nil {
		t.Error("result.Error should be set for non-JSON output")
	}
	// Raw stdout should be preserved for debugging.
	if string(result.RawJSON) != "Some plain text output that is not JSON" {
		t.Errorf("RawJSON should preserve raw stdout, got %q", string(result.RawJSON))
	}
}

// TestProcessExecResult_TruncatedJSON_ShouldError verifies that truncated JSON
// (valid prefix but incomplete) is classified as an error.
func TestProcessExecResult_TruncatedJSON_ShouldError(t *testing.T) {
	t.Parallel()
	agent := New(agents.AgentConfig{}).(*Agent)

	execResult := &agents.ExecuteResult{
		Stdout:   []byte(`{"type": "result", "session_id": "sess-123", "result": "partial`),
		ExitCode: 0,
		Duration: 5 * time.Second,
	}

	result, err := agent.processExecResult(execResult, "test-session")

	if err == nil {
		t.Fatal("expected non-nil error for truncated JSON, got nil")
	}
	if !result.IsError {
		t.Error("IsError should be true when JSON is truncated")
	}
	if len(result.Errors) == 0 {
		t.Error("Errors should contain a description of the JSON parse failure")
	}
}

// TestProcessExecResult_EmptyStdout_NoError verifies that empty stdout
// does not produce a spurious error (existing behavior preserved).
func TestProcessExecResult_EmptyStdout_NoError(t *testing.T) {
	t.Parallel()
	agent := New(agents.AgentConfig{}).(*Agent)

	execResult := &agents.ExecuteResult{
		Stdout:   nil,
		ExitCode: 0,
		Duration: 1 * time.Second,
	}

	result, err := agent.processExecResult(execResult, "test-session")

	if err != nil {
		t.Fatalf("unexpected error for empty stdout: %v", err)
	}
	if result.IsError {
		t.Error("IsError should be false for empty stdout with exit code 0")
	}
}

// TestProcessExecResult_NonJSON_WithRateLimitMessage verifies that non-JSON
// output containing a rate-limit message is classified as a rate-limit error
// when the CLI exits successfully (no exec error to override classification).
func TestProcessExecResult_NonJSON_WithRateLimitMessage(t *testing.T) {
	t.Parallel()
	agent := New(agents.AgentConfig{}).(*Agent)

	execResult := &agents.ExecuteResult{
		Stdout:   []byte("Error: Rate limit exceeded. Too many requests, please retry after 30s."),
		ExitCode: 0,
	}

	result, err := agent.processExecResult(execResult, "test-session")

	if !result.IsError {
		t.Error("IsError should be true for non-JSON rate limit output")
	}
	if err == nil {
		t.Fatal("expected non-nil error for non-JSON rate limit output")
	}
	var ce *agents.ClassifiedError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *ClassifiedError, got %T", err)
	}
	if ce.Class != agents.ErrorClassRetryable {
		t.Errorf("expected ErrorClassRetryable for rate limit, got %v", ce.Class)
	}
}

// TestProcessExecResult_NonJSON_RetryableByDefault verifies that unrecognizable
// non-JSON output defaults to a retryable error.
func TestProcessExecResult_NonJSON_RetryableByDefault(t *testing.T) {
	t.Parallel()
	agent := New(agents.AgentConfig{}).(*Agent)

	execResult := &agents.ExecuteResult{
		Stdout:   []byte("Something completely unexpected happened"),
		ExitCode: 0,
		Duration: 2 * time.Second,
	}

	_, err := agent.processExecResult(execResult, "test-session")

	if err == nil {
		t.Fatal("expected non-nil error for unrecognizable non-JSON output")
	}
	var ce *agents.ClassifiedError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *ClassifiedError, got %T", err)
	}
	if ce.Class != agents.ErrorClassRetryable {
		t.Errorf("expected ErrorClassRetryable, got %v", ce.Class)
	}
	if ce.Agent != "claude-code" {
		t.Errorf("expected agent %q, got %q", "claude-code", ce.Agent)
	}
}

// TestProcessExecResult_ValidJSON_IsError verifies that when Claude reports
// is_error=true in valid JSON, the existing behavior is preserved (error on
// result but nil returned error).
func TestProcessExecResult_ValidJSON_IsError(t *testing.T) {
	t.Parallel()
	agent := New(agents.AgentConfig{}).(*Agent)

	execResult := &agents.ExecuteResult{
		Stdout: []byte(`{
			"type": "result",
			"session_id": "sess-err",
			"result": "",
			"is_error": true,
			"errors": ["something went wrong"],
			"num_turns": 1
		}`),
		ExitCode: 0,
		Duration: 5 * time.Second,
	}

	result, err := agent.processExecResult(execResult, "pre-id")

	// Existing behavior: is_error in JSON sets result.Error but does not
	// return a non-nil error from execute(). Preserve this.
	if err != nil {
		t.Fatalf("expected nil returned error for is_error JSON, got %v", err)
	}
	if !result.IsError {
		t.Error("IsError should be true when JSON has is_error=true")
	}
	if result.Error == nil {
		t.Error("result.Error should be set when is_error=true")
	}
}

// TestProcessExecResult_ExecError_TakesPrecedence verifies that when the CLI
// itself fails (non-zero exit), the exec error takes precedence in the return.
func TestProcessExecResult_ExecError_TakesPrecedence(t *testing.T) {
	t.Parallel()
	agent := New(agents.AgentConfig{}).(*Agent)

	execErr := errors.New("exit status 1")
	execResult := &agents.ExecuteResult{
		Stdout:   []byte("not json"),
		Stderr:   []byte("something broke"),
		ExitCode: 1,
		Err:      execErr,
		Duration: 1 * time.Second,
	}

	result, err := agent.processExecResult(execResult, "test-session")

	// Exec error should be returned.
	if err == nil {
		t.Fatal("expected non-nil error when exec fails")
	}
	if !errors.Is(err, execErr) {
		t.Errorf("returned error should be exec error, got %v", err)
	}
	// IsError should still be set from JSON parse failure.
	if !result.IsError {
		t.Error("IsError should be true even when exec error takes precedence")
	}
}

// TestProcessExecResult_NonJSON_AuthError verifies that non-JSON output
// containing auth error patterns is classified as fatal when the CLI exits
// successfully (no exec error to override classification).
func TestProcessExecResult_NonJSON_AuthError(t *testing.T) {
	t.Parallel()
	agent := New(agents.AgentConfig{}).(*Agent)

	execResult := &agents.ExecuteResult{
		Stdout:   []byte("Error: Not authenticated. Please run 'claude login' first."),
		ExitCode: 0,
	}

	result, err := agent.processExecResult(execResult, "test-session")

	if !result.IsError {
		t.Error("IsError should be true for non-JSON auth error output")
	}
	if err == nil {
		t.Fatal("expected non-nil error for non-JSON auth error output")
	}
	var ce *agents.ClassifiedError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *ClassifiedError, got %T", err)
	}
	if ce.Class != agents.ErrorClassFatal {
		t.Errorf("expected ErrorClassFatal for auth error, got %v", ce.Class)
	}
}
