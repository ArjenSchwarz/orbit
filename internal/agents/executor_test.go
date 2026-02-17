package agents_test

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
)

func TestExecute_SuccessfulCommand(t *testing.T) {
	result := agents.Execute(context.Background(), agents.ExecuteConfig{
		CLIPath: "echo",
		Args:    []string{"hello world"},
	})

	if result.Err != nil {
		t.Fatalf("Execute() error = %v", result.Err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	if got := strings.TrimSpace(string(result.Stdout)); got != "hello world" {
		t.Errorf("Stdout = %q, want %q", got, "hello world")
	}
	if result.Duration <= 0 {
		t.Errorf("Duration = %v, want > 0", result.Duration)
	}
}

func TestExecute_NonZeroExitCode(t *testing.T) {
	result := agents.Execute(context.Background(), agents.ExecuteConfig{
		CLIPath: "sh",
		Args:    []string{"-c", "exit 42"},
	})

	if result.Err == nil {
		t.Fatal("Execute() expected error for non-zero exit")
	}
	if result.ExitCode != 42 {
		t.Errorf("ExitCode = %d, want 42", result.ExitCode)
	}
}

func TestExecute_CommandNotFound(t *testing.T) {
	result := agents.Execute(context.Background(), agents.ExecuteConfig{
		CLIPath: "nonexistent-binary-that-should-not-exist-xyz",
		Args:    []string{},
	})

	if result.Err == nil {
		t.Fatal("Execute() expected error for missing binary")
	}
	if result.ExitCode != -1 {
		t.Errorf("ExitCode = %d, want -1 for command not found", result.ExitCode)
	}
}

func TestExecute_CapturesStderr(t *testing.T) {
	result := agents.Execute(context.Background(), agents.ExecuteConfig{
		CLIPath: "sh",
		Args:    []string{"-c", "echo error output >&2"},
	})

	if result.Err != nil {
		t.Fatalf("Execute() error = %v", result.Err)
	}
	if got := strings.TrimSpace(result.Stderr); got != "error output" {
		t.Errorf("Stderr = %q, want %q", got, "error output")
	}
}

func TestExecute_WorkDir(t *testing.T) {
	dir := t.TempDir()

	result := agents.Execute(context.Background(), agents.ExecuteConfig{
		CLIPath: "pwd",
		WorkDir: dir,
	})

	if result.Err != nil {
		t.Fatalf("Execute() error = %v", result.Err)
	}

	// On macOS, /tmp is a symlink to /private/tmp
	got := strings.TrimSpace(string(result.Stdout))
	if runtime.GOOS == "darwin" && strings.HasPrefix(dir, "/tmp") {
		dir = "/private" + dir
	}
	if got != dir {
		t.Errorf("pwd output = %q, want %q", got, dir)
	}
}

func TestExecute_EmptyWorkDir(t *testing.T) {
	// When WorkDir is empty, the command inherits the current directory.
	result := agents.Execute(context.Background(), agents.ExecuteConfig{
		CLIPath: "pwd",
	})

	if result.Err != nil {
		t.Fatalf("Execute() error = %v", result.Err)
	}
	if len(strings.TrimSpace(string(result.Stdout))) == 0 {
		t.Error("Expected non-empty pwd output")
	}
}

func TestExecute_EnvMerging(t *testing.T) {
	result := agents.Execute(context.Background(), agents.ExecuteConfig{
		CLIPath: "sh",
		Args:    []string{"-c", "echo $ORBIT_TEST_VAR"},
		Env:     map[string]string{"ORBIT_TEST_VAR": "test-value-123"},
	})

	if result.Err != nil {
		t.Fatalf("Execute() error = %v", result.Err)
	}
	if got := strings.TrimSpace(string(result.Stdout)); got != "test-value-123" {
		t.Errorf("env var output = %q, want %q", got, "test-value-123")
	}
}

func TestExecute_NilEnvInheritsEnvironment(t *testing.T) {
	// When Env is nil, the command should inherit the current environment (default exec behavior).
	// PATH should be available for the command to find 'echo'.
	result := agents.Execute(context.Background(), agents.ExecuteConfig{
		CLIPath: "echo",
		Args:    []string{"inherited"},
		Env:     nil,
	})

	if result.Err != nil {
		t.Fatalf("Execute() error = %v", result.Err)
	}
	if got := strings.TrimSpace(string(result.Stdout)); got != "inherited" {
		t.Errorf("Stdout = %q, want %q", got, "inherited")
	}
}

func TestExecute_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result := agents.Execute(ctx, agents.ExecuteConfig{
		CLIPath: "sleep",
		Args:    []string{"10"},
	})

	if result.Err == nil {
		t.Fatal("Execute() expected error for cancelled context")
	}
	if result.Duration < 40*time.Millisecond {
		t.Errorf("Duration = %v, expected at least ~50ms", result.Duration)
	}
}

func TestExecute_Duration(t *testing.T) {
	result := agents.Execute(context.Background(), agents.ExecuteConfig{
		CLIPath: "sleep",
		Args:    []string{"0.1"},
	})

	if result.Err != nil {
		t.Fatalf("Execute() error = %v", result.Err)
	}
	if result.Duration < 80*time.Millisecond {
		t.Errorf("Duration = %v, expected at least ~100ms", result.Duration)
	}
}

func TestExecute_StdoutAndStderrSimultaneous(t *testing.T) {
	result := agents.Execute(context.Background(), agents.ExecuteConfig{
		CLIPath: "sh",
		Args:    []string{"-c", "echo out; echo err >&2"},
	})

	if result.Err != nil {
		t.Fatalf("Execute() error = %v", result.Err)
	}
	if got := strings.TrimSpace(string(result.Stdout)); got != "out" {
		t.Errorf("Stdout = %q, want %q", got, "out")
	}
	if got := strings.TrimSpace(result.Stderr); got != "err" {
		t.Errorf("Stderr = %q, want %q", got, "err")
	}
}
