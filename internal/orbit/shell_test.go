package orbit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/arjenschwarz/orbit/internal/debug"
	"github.com/arjenschwarz/orbit/internal/logs"
	"github.com/arjenschwarz/orbit/internal/rune"
	"github.com/arjenschwarz/orbit/internal/testutil"
)

// setupTestOrbit creates a minimal Orbit instance for shell command testing.
func setupTestOrbit(t *testing.T, workingDir string, timeout time.Duration) *Orbit {
	t.Helper()

	// Create TestAgent with empty scenario - agent.Run() won't be called
	scenario := testutil.NewScenario().Build()
	agent := testutil.NewTestAgent(t, "test-agent", scenario)

	// Create rune client with a dummy tasks file (won't be used in shell tests)
	runeClient := rune.NewClient(filepath.Join(workingDir, "tasks.md"))

	// Create debug logger
	dbg, err := debug.NewLogger(debug.LoggerConfig{})
	if err != nil {
		t.Fatalf("Failed to create debug logger: %v", err)
	}
	t.Cleanup(func() { dbg.Close() })

	return &Orbit{
		config: Config{
			WorkingDir:     workingDir,
			CommandTimeout: timeout,
		},
		agent:       agent,
		runeClient:  runeClient,
		shutdownCtx: t.Context(),
		debug:       dbg,
	}
}

func TestExecuteShellCommand_Success(t *testing.T) {
	tempDir := t.TempDir()
	o := setupTestOrbit(t, tempDir, 30*time.Second)

	result, err := o.executeShellCommand("echo hello", "test-command")
	if err != nil {
		t.Fatalf("executeShellCommand() returned error: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	assert.Contains(t, result.Stdout, "hello", "Stdout = %q, want to contain 'hello'", result.Stdout)
	if result.Duration <= 0 {
		t.Error("Duration should be positive")
	}
	if result.StartedAt.IsZero() {
		t.Error("StartedAt should be set")
	}
	if result.CompletedAt.IsZero() {
		t.Error("CompletedAt should be set")
	}
}

func TestExecuteShellCommand_NonZeroExit(t *testing.T) {
	tempDir := t.TempDir()
	o := setupTestOrbit(t, tempDir, 30*time.Second)

	result, err := o.executeShellCommand("exit 42", "test-command")
	if err == nil {
		t.Fatal("expected error for non-zero exit code")
	}

	if result.ExitCode != 42 {
		t.Errorf("ExitCode = %d, want 42", result.ExitCode)
	}
}

func TestExecuteShellCommand_Timeout(t *testing.T) {
	tempDir := t.TempDir()
	// Use a very short timeout
	o := setupTestOrbit(t, tempDir, 100*time.Millisecond)

	result, err := o.executeShellCommand("sleep 10", "test-command")
	if err == nil {
		t.Fatal("expected timeout error")
	}

	assert.Contains(t, err.Error(), "timed out", "expected timeout error, got: %v", err)

	// Exit code should be -1 or negative for killed process
	if result.ExitCode >= 0 && result.ExitCode != 137 { // 137 = 128 + SIGKILL(9)
		// On some systems the exit code might differ
		t.Logf("ExitCode = %d (expected -1 or 137 for killed process)", result.ExitCode)
	}
}

func TestExecuteShellCommand_WorkingDir(t *testing.T) {
	// Create a temp directory and a file in it
	tempDir := t.TempDir()
	testFile := "test-marker.txt"
	if err := os.WriteFile(filepath.Join(tempDir, testFile), []byte("marker"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	o := setupTestOrbit(t, tempDir, 30*time.Second)

	// Command should see the file because working directory is set correctly
	result, err := o.executeShellCommand("ls "+testFile, "test-command")
	if err != nil {
		t.Fatalf("executeShellCommand() returned error: %v", err)
	}

	assert.Contains(t, result.Stdout, testFile, "Command should find file in working directory. Stdout = %q", result.Stdout)
}

func TestExecuteShellCommand_EnvVars(t *testing.T) {
	tempDir := t.TempDir()

	// Create a tasks file to test phase count
	tasksContent := `# Tasks

## Phase 1: First Phase
- [ ] Task 1

## Phase 2: Second Phase
- [ ] Task 2
`
	if err := os.WriteFile(filepath.Join(tempDir, "tasks.md"), []byte(tasksContent), 0644); err != nil {
		t.Fatalf("Failed to create tasks file: %v", err)
	}

	o := setupTestOrbit(t, tempDir, 30*time.Second)

	// Print environment variables
	result, err := o.executeShellCommand("echo PHASE=$ORBIT_PHASE_COUNT AGENT=$ORBIT_AGENT", "test-command")
	if err != nil {
		t.Fatalf("executeShellCommand() returned error: %v", err)
	}

	// Check that ORBIT_AGENT is set to the mock agent name
	assert.Contains(t, result.Stdout, "AGENT=test-agent", "Expected ORBIT_AGENT=test-agent in output. Stdout = %q", result.Stdout)

	// ORBIT_PHASE_COUNT should be set (may be 0 if rune client can't parse the file,
	// but the env var should still be present)
	assert.Contains(t, result.Stdout, "PHASE=", "Expected ORBIT_PHASE_COUNT in output. Stdout = %q", result.Stdout)
}

func TestExecuteShellCommand_CapturesOutput(t *testing.T) {
	tempDir := t.TempDir()
	o := setupTestOrbit(t, tempDir, 30*time.Second)

	// Command that writes to both stdout and stderr
	result, err := o.executeShellCommand("echo stdout-message; echo stderr-message >&2", "test-command")
	if err != nil {
		t.Fatalf("executeShellCommand() returned error: %v", err)
	}

	assert.Contains(t, result.Stdout, "stdout-message", "Expected 'stdout-message' in Stdout. Got: %q", result.Stdout)
	assert.Contains(t, result.Stderr, "stderr-message", "Expected 'stderr-message' in Stderr. Got: %q", result.Stderr)
}

func TestSaveShellCommandLog(t *testing.T) {
	tempDir := t.TempDir()

	// Create log manager
	logManager, err := logs.NewManagerWithOptions(tempDir, "test-branch", tempDir, logs.ManagerOptions{UseSubdirs: false})
	if err != nil {
		t.Fatalf("Failed to create log manager: %v", err)
	}

	// Create TestAgent with empty scenario - agent.Run() won't be called
	scenario := testutil.NewScenario().Build()
	agent := testutil.NewTestAgent(t, "test-agent", scenario)

	dbg, _ := debug.NewLogger(debug.LoggerConfig{})
	t.Cleanup(func() { dbg.Close() })

	o := &Orbit{
		config: Config{
			WorkingDir:     tempDir,
			CommandTimeout: 30 * time.Second,
		},
		agent:       agent,
		runeClient:  rune.NewClient(filepath.Join(tempDir, "tasks.md")),
		logManager:  logManager,
		shutdownCtx: t.Context(),
		debug:       dbg,
	}

	// Execute a command to trigger log saving
	result, err := o.executeShellCommand("echo test-output", "pre-command")
	if err != nil {
		t.Fatalf("executeShellCommand() returned error: %v", err)
	}

	// Check log file was created
	logFile := filepath.Join(tempDir, "pre-command-run-1.txt")
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	// Verify log content
	logContent := string(content)
	assert.Contains(t, logContent, "echo test-output", "Log should contain the command. Got: %s", logContent)
	assert.Contains(t, logContent, "Exit Code: 0", "Log should contain exit code. Got: %s", logContent)
	assert.Contains(t, logContent, "test-output", "Log should contain stdout. Got: %s", logContent)
	assert.Contains(t, logContent, "Duration:", "Log should contain duration. Got: %s", logContent)

	// Verify result is correct
	if result.Command != "echo test-output" {
		t.Errorf("Result.Command = %q, want %q", result.Command, "echo test-output")
	}
}

func TestExecuteShellCommand_ShutdownInterrupt(t *testing.T) {
	tempDir := t.TempDir()

	// Create a context that we'll cancel to simulate shutdown
	ctx, cancel := context.WithCancel(context.Background())

	// Create TestAgent with empty scenario - agent.Run() won't be called
	scenario := testutil.NewScenario().Build()
	agent := testutil.NewTestAgent(t, "test-agent", scenario)

	dbg, _ := debug.NewLogger(debug.LoggerConfig{})
	t.Cleanup(func() { dbg.Close() })

	o := &Orbit{
		config: Config{
			WorkingDir:     tempDir,
			CommandTimeout: 10 * time.Second,
		},
		agent:       agent,
		runeClient:  rune.NewClient(filepath.Join(tempDir, "tasks.md")),
		shutdownCtx: ctx,
		debug:       dbg,
	}

	// Cancel the context after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// Execute a long-running command
	_, err := o.executeShellCommand("sleep 10", "test-command")
	if err == nil {
		t.Fatal("expected error due to shutdown")
	}

	errMsg := err.Error()
	shutdownOrCanceled := strings.Contains(errMsg, "shutdown") || strings.Contains(errMsg, "context canceled")
	assert.True(t, shutdownOrCanceled, "expected shutdown/canceled error, got: %v", err)
}

func TestShellCommandResult_Struct(t *testing.T) {
	result := ShellCommandResult{
		Command:     "echo hello",
		ExitCode:    0,
		Stdout:      "hello\n",
		Stderr:      "",
		Duration:    100 * time.Millisecond,
		StartedAt:   time.Now(),
		CompletedAt: time.Now().Add(100 * time.Millisecond),
	}

	if result.Command != "echo hello" {
		t.Errorf("Command = %q, want %q", result.Command, "echo hello")
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	if result.Stdout != "hello\n" {
		t.Errorf("Stdout = %q, want %q", result.Stdout, "hello\n")
	}
	if result.Stderr != "" {
		t.Errorf("Stderr = %q, want empty", result.Stderr)
	}
	if result.Duration != 100*time.Millisecond {
		t.Errorf("Duration = %v, want 100ms", result.Duration)
	}
}
