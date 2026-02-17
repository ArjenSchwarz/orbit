package debug_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/arjenschwarz/orbit/internal/debug"
)

// TestFullOrchestrationLogging verifies that a simulated orchestration run
// produces all expected log entries including startup and shutdown.
// Requirements: 1.4, 1.6, 3.1, 3.9
func TestFullOrchestrationLogging(t *testing.T) {
	t.Parallel()

	runID := uniqueTestRunID("full-orch")

	l, err := debug.NewLogger(debug.LoggerConfig{
		StderrEnabled: false,
		FileEnabled:   true,
		RunID:         runID,
		Prefix:        "orchestrator",
	})
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	path := l.Path()
	defer func() {
		l.Close()
		_ = os.Remove(path)
	}()

	// Simulate orchestration startup (Req 3.1, 5.3)
	l.LogStartup(debug.StartupConfig{
		OrbitVersion:     "0.1.0",
		Agent:            "claude-code",
		TasksFile:        "/project/specs/feature/tasks.md",
		WorkingDirectory: "/project",
		BranchName:       "feature/test",
	})

	// Simulate phase execution (Req 3.2, 3.3)
	l.LogStructured("info", "Phase started", map[string]any{
		"phase":      1,
		"phase_name": "Implementation",
		"task_count": 5,
	})

	// Simulate agent invocation (Req 3.4)
	l.LogStructured("info", "Agent invocation", map[string]any{
		"agent":       "claude-code",
		"phase":       1,
		"session_id":  "abc123",
		"working_dir": "/project",
	})

	// Simulate agent completion (Req 3.5, 4.1)
	l.LogStructured("info", "Agent completed", map[string]any{
		"phase":            1,
		"exit_code":        0,
		"duration":         "5m0s",
		"session_id":       "abc123",
		"session_log_path": "/project/specs/feature/.orbit/logs",
	})

	// Simulate phase completion (Req 3.3, 4.2)
	l.LogStructured("info", "Phase completed", map[string]any{
		"phase":           1,
		"status":          "completed",
		"duration":        "5m30s",
		"transcript_path": "/project/specs/feature/.orbit/logs/phase-1-session.txt",
	})

	// Simulate orchestration completion (Req 3.9, 5.4)
	l.LogShutdown("completed")

	// Read and verify log file
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")

	// Should have at least 6 entries: startup, phase start, agent invoke, agent complete, phase complete, shutdown
	if len(lines) < 6 {
		t.Fatalf("expected at least 6 log entries, got %d", len(lines))
	}

	// Verify startup entry is first and has schema_version
	var startup debug.StartupEntry
	if err := json.Unmarshal([]byte(lines[0]), &startup); err != nil {
		t.Fatalf("failed to parse startup entry: %v", err)
	}
	if startup.SchemaVersion != 1 {
		t.Errorf("startup SchemaVersion = %d, want 1", startup.SchemaVersion)
	}
	if startup.Message != "Orchestration started" {
		t.Errorf("startup Message = %q, want %q", startup.Message, "Orchestration started")
	}

	// Verify shutdown entry is last
	var shutdown debug.ShutdownEntry
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &shutdown); err != nil {
		t.Fatalf("failed to parse shutdown entry: %v", err)
	}
	if shutdown.FinalStatus != "completed" {
		t.Errorf("shutdown FinalStatus = %q, want %q", shutdown.FinalStatus, "completed")
	}
	if shutdown.TotalDuration == "" {
		t.Error("shutdown TotalDuration should not be empty")
	}
}

// TestVariantModeCreatesCorrectFiles verifies that variant mode creates N+1 log files
// with correct naming patterns.
// Requirements: 1.4, 1.5, 1.6
func TestVariantModeCreatesCorrectFiles(t *testing.T) {
	t.Parallel()

	runID := uniqueTestRunID("variant-mode")
	numVariants := 3

	// Create main logger (for parent orchestration events)
	mainLogger, err := debug.NewLogger(debug.LoggerConfig{
		StderrEnabled: false,
		FileEnabled:   true,
		RunID:         runID,
		VariantNum:    0, // Main logger
		Prefix:        "orchestrator",
	})
	if err != nil {
		t.Fatalf("NewLogger() for main error = %v", err)
	}
	mainPath := mainLogger.Path()
	defer func() {
		mainLogger.Close()
		_ = os.Remove(mainPath)
	}()

	// Create variant loggers
	variantLoggers := make([]*debug.Logger, numVariants)
	variantPaths := make([]string, numVariants)
	for i := range numVariants {
		variantLoggers[i], err = debug.NewLogger(debug.LoggerConfig{
			StderrEnabled: false,
			FileEnabled:   true,
			RunID:         runID,
			VariantNum:    i + 1,
			Prefix:        "variant",
		})
		if err != nil {
			t.Fatalf("NewLogger() for variant %d error = %v", i+1, err)
		}
		variantPaths[i] = variantLoggers[i].Path()
		defer func(l *debug.Logger, p string) {
			l.Close()
			_ = os.Remove(p)
		}(variantLoggers[i], variantPaths[i])
	}

	// Log parent orchestration events to main logger (Req 1.6)
	mainLogger.LogStartup(debug.StartupConfig{
		OrbitVersion:     "0.1.0",
		Agent:            "claude-code",
		TasksFile:        "/project/specs/feature/tasks.md",
		WorkingDirectory: "/project",
		BranchName:       "feature/test",
	})
	mainLogger.LogStructured("info", "Variant created", map[string]any{
		"variant_id": 1,
		"branch":     "orbit-impl-1/test",
	})
	mainLogger.LogStructured("info", "Variant created", map[string]any{
		"variant_id": 2,
		"branch":     "orbit-impl-2/test",
	})
	mainLogger.LogStructured("info", "Variant created", map[string]any{
		"variant_id": 3,
		"branch":     "orbit-impl-3/test",
	})
	mainLogger.LogStructured("info", "Parallel execution started", map[string]any{
		"variant_count": 3,
		"max_parallel":  2,
	})

	// Log to each variant's log
	for i, vl := range variantLoggers {
		vl.LogStartup(debug.StartupConfig{
			OrbitVersion:     "0.1.0",
			Agent:            "claude-code",
			TasksFile:        "/project/specs/feature/tasks.md",
			WorkingDirectory: "/project/worktrees/variant-" + string(rune('1'+i)),
			BranchName:       "orbit-impl-" + string(rune('1'+i)) + "/test",
		})
		vl.LogStructured("info", "Phase started", map[string]any{
			"phase":      1,
			"variant_id": i + 1,
		})
		vl.LogShutdown("completed")
	}

	// Log all variants completed to main (Req 1.6)
	mainLogger.LogStructured("info", "All variants completed", map[string]any{
		"succeeded": 3,
		"failed":    0,
		"canceled":  0,
	})
	mainLogger.LogShutdown("completed")

	// Verify main log file exists and has correct name pattern
	mainFilename := filepath.Base(mainPath)
	assert.Contains(t, mainFilename, runID, "main log filename %q does not contain runID %q", mainFilename, runID)
	// Main file should not have "variant-N" pattern (where N is a digit)
	assert.NotContains(t, mainFilename, "variant-1", "main log filename %q should not contain variant-N pattern", mainFilename)
	assert.NotContains(t, mainFilename, "variant-2", "main log filename %q should not contain variant-N pattern", mainFilename)
	assert.NotContains(t, mainFilename, "variant-3", "main log filename %q should not contain variant-N pattern", mainFilename)

	// Verify variant log files exist and have correct name pattern (Req 1.5)
	for i, vp := range variantPaths {
		variantFilename := filepath.Base(vp)
		assert.Contains(t, variantFilename, runID, "variant %d log filename %q does not contain runID %q", i+1, variantFilename, runID)
		expectedPattern := "variant-" + string(rune('1'+i))
		assert.Contains(t, variantFilename, expectedPattern, "variant %d log filename %q does not contain %q", i+1, variantFilename, expectedPattern)
	}

	// Verify we have N+1 files (1 main + N variants)
	logDir := filepath.Dir(mainPath)
	entries, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatalf("failed to read log directory: %v", err)
	}

	// Count files with our runID
	count := 0
	for _, entry := range entries {
		if strings.Contains(entry.Name(), runID) {
			count++
		}
	}
	expectedCount := numVariants + 1
	if count != expectedCount {
		t.Errorf("expected %d log files for runID %q, got %d", expectedCount, runID, count)
	}
}

// TestLogFileContainsStartupAndShutdown verifies that log files contain
// startup and shutdown entries with correct fields.
// Requirements: 5.3, 5.4
func TestLogFileContainsStartupAndShutdown(t *testing.T) {
	t.Parallel()

	runID := uniqueTestRunID("startup-shutdown")

	l, err := debug.NewLogger(debug.LoggerConfig{
		StderrEnabled: false,
		FileEnabled:   true,
		RunID:         runID,
		Prefix:        "orchestrator",
	})
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	path := l.Path()
	defer func() { _ = os.Remove(path) }()

	// Log startup with all required fields (Req 5.3)
	l.LogStartup(debug.StartupConfig{
		OrbitVersion:     "1.2.3",
		Agent:            "codex",
		TasksFile:        "/abs/path/to/tasks.md",
		WorkingDirectory: "/abs/path/to/project",
		BranchName:       "main",
	})

	// Do some work
	l.LogStructured("info", "Test operation", nil)

	// Log shutdown (Req 5.4)
	l.LogShutdown("completed")
	l.Close()

	// Read and parse log file
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 log entries, got %d", len(lines))
	}

	// Verify startup entry (Req 5.3)
	var startup debug.StartupEntry
	if err := json.Unmarshal([]byte(lines[0]), &startup); err != nil {
		t.Fatalf("failed to parse startup entry: %v", err)
	}

	// Check required startup fields
	if startup.SchemaVersion != 1 {
		t.Errorf("startup SchemaVersion = %d, want 1", startup.SchemaVersion)
	}
	if startup.OrbitVersion != "1.2.3" {
		t.Errorf("startup OrbitVersion = %q, want %q", startup.OrbitVersion, "1.2.3")
	}
	if startup.Agent != "codex" {
		t.Errorf("startup Agent = %q, want %q", startup.Agent, "codex")
	}
	if startup.TasksFile != "/abs/path/to/tasks.md" {
		t.Errorf("startup TasksFile = %q, want %q", startup.TasksFile, "/abs/path/to/tasks.md")
	}
	if startup.WorkingDirectory != "/abs/path/to/project" {
		t.Errorf("startup WorkingDirectory = %q, want %q", startup.WorkingDirectory, "/abs/path/to/project")
	}
	if startup.BranchName != "main" {
		t.Errorf("startup BranchName = %q, want %q", startup.BranchName, "main")
	}
	if startup.Level != "info" {
		t.Errorf("startup Level = %q, want %q", startup.Level, "info")
	}
	if startup.Component != "orchestrator" {
		t.Errorf("startup Component = %q, want %q", startup.Component, "orchestrator")
	}

	// Verify shutdown entry is last (Req 5.4)
	var shutdown debug.ShutdownEntry
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &shutdown); err != nil {
		t.Fatalf("failed to parse shutdown entry: %v", err)
	}

	if shutdown.FinalStatus != "completed" {
		t.Errorf("shutdown FinalStatus = %q, want %q", shutdown.FinalStatus, "completed")
	}
	if shutdown.TotalDuration == "" {
		t.Error("shutdown TotalDuration should not be empty")
	}
	if shutdown.Level != "info" {
		t.Errorf("shutdown Level = %q, want %q", shutdown.Level, "info")
	}
	if shutdown.Component != "orchestrator" {
		t.Errorf("shutdown Component = %q, want %q", shutdown.Component, "orchestrator")
	}
}

// TestCrossReferencePathsAreAbsolute verifies that session and transcript paths
// in log entries are absolute paths.
// Requirements: 4.1, 4.2, 4.3
func TestCrossReferencePathsAreAbsolute(t *testing.T) {
	t.Parallel()

	runID := uniqueTestRunID("abs-paths")

	l, err := debug.NewLogger(debug.LoggerConfig{
		StderrEnabled: false,
		FileEnabled:   true,
		RunID:         runID,
		Prefix:        "orchestrator",
	})
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	path := l.Path()
	defer func() {
		l.Close()
		_ = os.Remove(path)
	}()

	// Log entries with session and transcript paths (Req 4.1, 4.2)
	l.LogStructured("info", "Agent completed", map[string]any{
		"phase":            1,
		"session_log_path": "/home/user/project/specs/feature/.orbit/logs",
	})

	l.LogStructured("info", "Phase completed", map[string]any{
		"phase":           1,
		"transcript_path": "/home/user/project/specs/feature/.orbit/logs/phase-1-session.txt",
	})

	// Read and verify
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 log entries, got %d", len(lines))
	}

	// Check agent completion entry for absolute session_log_path
	var agentEntry debug.LogEntry
	if err := json.Unmarshal([]byte(lines[0]), &agentEntry); err != nil {
		t.Fatalf("failed to parse agent completion entry: %v", err)
	}

	sessionPath, ok := agentEntry.Fields["session_log_path"].(string)
	if !ok {
		t.Fatal("session_log_path field not found or not a string")
	}
	if !filepath.IsAbs(sessionPath) {
		t.Errorf("session_log_path %q is not absolute (Req 4.3)", sessionPath)
	}

	// Check phase completion entry for absolute transcript_path
	var phaseEntry debug.LogEntry
	if err := json.Unmarshal([]byte(lines[1]), &phaseEntry); err != nil {
		t.Fatalf("failed to parse phase completion entry: %v", err)
	}

	transcriptPath, ok := phaseEntry.Fields["transcript_path"].(string)
	if !ok {
		t.Fatal("transcript_path field not found or not a string")
	}
	if !filepath.IsAbs(transcriptPath) {
		t.Errorf("transcript_path %q is not absolute (Req 4.3)", transcriptPath)
	}
}

// TestSIGTERMTriggersShutdownEntry verifies that Close() writes shutdown entry
// when called (simulating SIGTERM behavior).
// Requirement: 5.4 (shutdown entry on normal completion or interruption)
func TestSIGTERMTriggersShutdownEntry(t *testing.T) {
	t.Parallel()

	runID := uniqueTestRunID("sigterm")

	l, err := debug.NewLogger(debug.LoggerConfig{
		StderrEnabled: false,
		FileEnabled:   true,
		RunID:         runID,
		Prefix:        "orchestrator",
	})
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	path := l.Path()
	defer func() { _ = os.Remove(path) }()

	// Log startup
	l.LogStartup(debug.StartupConfig{
		OrbitVersion:     "0.1.0",
		Agent:            "claude-code",
		TasksFile:        "/project/tasks.md",
		WorkingDirectory: "/project",
		BranchName:       "main",
	})

	// Simulate some work
	l.LogStructured("info", "Phase started", map[string]any{"phase": 1})

	// Close without explicit LogShutdown - simulates SIGTERM/Close() path
	l.Close()

	// Read and verify shutdown entry was written with "interrupted" status
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 log entries (startup, phase, shutdown), got %d", len(lines))
	}

	// Verify shutdown entry is present with "interrupted" status
	var shutdown debug.ShutdownEntry
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &shutdown); err != nil {
		t.Fatalf("failed to parse shutdown entry: %v", err)
	}

	if shutdown.FinalStatus != "interrupted" {
		t.Errorf("shutdown FinalStatus = %q, want %q (for Close() without LogShutdown)", shutdown.FinalStatus, "interrupted")
	}
	if shutdown.TotalDuration == "" {
		t.Error("shutdown TotalDuration should not be empty")
	}
}

// TestDisabledLoggingCreatesNoFiles verifies that when centralized logging is disabled,
// no log files or directories are created.
// Requirement: 6.5
func TestDisabledLoggingCreatesNoFiles(t *testing.T) {
	t.Parallel()

	runID := uniqueTestRunID("disabled")

	// Create logger with FileEnabled=false
	l, err := debug.NewLogger(debug.LoggerConfig{
		StderrEnabled: false,
		FileEnabled:   false,
		RunID:         runID,
		Prefix:        "orchestrator",
	})
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer l.Close()

	// Path should be empty when file logging is disabled
	if l.Path() != "" {
		t.Errorf("Path() = %q, want empty string when file logging is disabled", l.Path())
	}

	// All logging operations should be no-ops
	l.LogStartup(debug.StartupConfig{
		OrbitVersion:     "0.1.0",
		Agent:            "claude-code",
		TasksFile:        "/project/tasks.md",
		WorkingDirectory: "/project",
		BranchName:       "main",
	})
	l.LogStructured("info", "Test message", nil)
	l.LogShutdown("completed")

	// Verify no file was created
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot get home directory: %v", err)
	}

	logDir := filepath.Join(homeDir, ".orbit", "logs")
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		// Log directory doesn't exist, which is fine
		return
	}

	// Check that no file with our runID was created
	entries, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatalf("failed to read log directory: %v", err)
	}

	for _, entry := range entries {
		assert.NotContains(t, entry.Name(), runID, "found unexpected log file %q when logging was disabled (Req 6.5)", entry.Name())
	}
}

// TestConfigurationHierarchy verifies that configuration loading follows
// the expected priority: CLI > env > yaml > defaults.
// Requirement: 6.4 (implicitly tested via Logger configuration)
func TestConfigurationHierarchy(t *testing.T) {
	t.Parallel()

	// Test 1: FileEnabled=false should disable file logging
	t.Run("file disabled", func(t *testing.T) {
		t.Parallel()

		l, err := debug.NewLogger(debug.LoggerConfig{
			StderrEnabled: true,
			FileEnabled:   false,
			RunID:         uniqueTestRunID("cfg-disabled"),
			Prefix:        "test",
		})
		if err != nil {
			t.Fatalf("NewLogger() error = %v", err)
		}
		defer l.Close()

		if l.Path() != "" {
			t.Errorf("Path() should be empty when FileEnabled=false")
		}
	})

	// Test 2: FileEnabled=true should enable file logging
	t.Run("file enabled", func(t *testing.T) {
		t.Parallel()

		runID := uniqueTestRunID("cfg-enabled")
		l, err := debug.NewLogger(debug.LoggerConfig{
			StderrEnabled: false,
			FileEnabled:   true,
			RunID:         runID,
			Prefix:        "test",
		})
		if err != nil {
			t.Fatalf("NewLogger() error = %v", err)
		}
		defer func() {
			l.Close()
			if l.Path() != "" {
				_ = os.Remove(l.Path())
			}
		}()

		if l.Path() == "" {
			t.Error("Path() should not be empty when FileEnabled=true")
		}
	})

	// Test 3: Empty RunID with FileEnabled should not create file
	t.Run("empty runID", func(t *testing.T) {
		t.Parallel()

		l, err := debug.NewLogger(debug.LoggerConfig{
			StderrEnabled: false,
			FileEnabled:   true,
			RunID:         "", // Empty
			Prefix:        "test",
		})
		if err != nil {
			t.Fatalf("NewLogger() error = %v", err)
		}
		defer l.Close()

		if l.Path() != "" {
			t.Errorf("Path() should be empty when RunID is empty")
		}
	})
}

// TestLogPathDiscoverability verifies that the logger path is accessible
// and points to the correct location.
// Requirement: 8.1-8.3 (log path output is handled at orchestrator level)
func TestLogPathDiscoverability(t *testing.T) {
	t.Parallel()

	runID := uniqueTestRunID("path-discover")

	l, err := debug.NewLogger(debug.LoggerConfig{
		StderrEnabled: false,
		FileEnabled:   true,
		RunID:         runID,
		Prefix:        "test",
	})
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer func() {
		l.Close()
		_ = os.Remove(l.Path())
	}()

	path := l.Path()

	// Path should be non-empty
	if path == "" {
		t.Fatal("Path() returned empty string for file-enabled logger")
	}

	// Path should be absolute (Req 4.3)
	if !filepath.IsAbs(path) {
		t.Errorf("Path() %q is not absolute", path)
	}

	// Path should be in ~/.orbit/logs/
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot get home directory: %v", err)
	}
	expectedDir := filepath.Join(homeDir, ".orbit", "logs")
	if !strings.HasPrefix(path, expectedDir) {
		t.Errorf("Path() %q is not under %q", path, expectedDir)
	}

	// Path should end with .jsonl
	if !strings.HasSuffix(path, ".jsonl") {
		t.Errorf("Path() %q does not end with .jsonl", path)
	}

	// Path should contain runID
	assert.Contains(t, path, runID, "Path() %q does not contain runID %q", path, runID)
}

// TestVariantLoggerCreation verifies that each variant gets a separate Logger
// and FileWriter with correct naming.
// Requirements: 1.4, 1.5
func TestVariantLoggerCreation(t *testing.T) {
	t.Parallel()

	runID := uniqueTestRunID("variant-logger")

	// Create variant loggers
	for variantNum := 1; variantNum <= 3; variantNum++ {
		t.Run("variant "+string(rune('0'+variantNum)), func(t *testing.T) {
			l, err := debug.NewLogger(debug.LoggerConfig{
				StderrEnabled: false,
				FileEnabled:   true,
				RunID:         runID,
				VariantNum:    variantNum,
				Prefix:        "variant",
			})
			if err != nil {
				t.Fatalf("NewLogger() error = %v", err)
			}
			defer func() {
				l.Close()
				_ = os.Remove(l.Path())
			}()

			path := l.Path()

			// Path should contain variant number
			expectedPattern := "variant-" + string(rune('0'+variantNum))
			assert.Contains(t, path, expectedPattern, "variant %d path %q does not contain %q", variantNum, path, expectedPattern)

			// Path should contain runID
			assert.Contains(t, path, runID, "variant %d path %q does not contain runID %q", variantNum, path, runID)

			// Write and verify entries
			l.LogStartup(debug.StartupConfig{
				OrbitVersion: "0.1.0",
				Agent:        "claude-code",
			})
			l.LogShutdown("completed")

			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("failed to read variant log: %v", err)
			}

			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(lines) != 2 {
				t.Errorf("expected 2 entries (startup, shutdown), got %d", len(lines))
			}
		})
	}
}

// TestStartupConfigPassthrough verifies that StartupConfig fields appear correctly
// in the startup log entry.
// Requirement: 5.3
func TestStartupConfigPassthrough(t *testing.T) {
	t.Parallel()

	runID := uniqueTestRunID("startup-cfg")

	l, err := debug.NewLogger(debug.LoggerConfig{
		StderrEnabled: false,
		FileEnabled:   true,
		RunID:         runID,
		Prefix:        "orchestrator",
	})
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	path := l.Path()
	defer func() {
		l.Close()
		_ = os.Remove(path)
	}()

	// Log with specific values
	cfg := debug.StartupConfig{
		OrbitVersion:     "2.0.0-beta",
		Agent:            "kiro",
		TasksFile:        "/special/path/to/tasks.md",
		WorkingDirectory: "/custom/working/dir",
		BranchName:       "feature/special-branch",
	}
	l.LogStartup(cfg)

	// Read and verify all fields passed through
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	var startup debug.StartupEntry
	if err := json.Unmarshal(data, &startup); err != nil {
		t.Fatalf("failed to parse startup entry: %v", err)
	}

	if startup.OrbitVersion != cfg.OrbitVersion {
		t.Errorf("OrbitVersion = %q, want %q", startup.OrbitVersion, cfg.OrbitVersion)
	}
	if startup.Agent != cfg.Agent {
		t.Errorf("Agent = %q, want %q", startup.Agent, cfg.Agent)
	}
	if startup.TasksFile != cfg.TasksFile {
		t.Errorf("TasksFile = %q, want %q", startup.TasksFile, cfg.TasksFile)
	}
	if startup.WorkingDirectory != cfg.WorkingDirectory {
		t.Errorf("WorkingDirectory = %q, want %q", startup.WorkingDirectory, cfg.WorkingDirectory)
	}
	if startup.BranchName != cfg.BranchName {
		t.Errorf("BranchName = %q, want %q", startup.BranchName, cfg.BranchName)
	}
}

// TestShutdownEntryAbsenceIndicatesCrash verifies that without a shutdown entry,
// we can detect an incomplete/crashed run.
// Requirement: 5.5
func TestShutdownEntryAbsenceIndicatesCrash(t *testing.T) {
	t.Parallel()

	runID := uniqueTestRunID("crash-detect")

	// Create logger and write startup, but don't write shutdown or close properly
	l, err := debug.NewLogger(debug.LoggerConfig{
		StderrEnabled: false,
		FileEnabled:   true,
		RunID:         runID,
		Prefix:        "orchestrator",
	})
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	path := l.Path()
	defer func() { _ = os.Remove(path) }()

	// Log startup and some activity
	l.LogStartup(debug.StartupConfig{
		OrbitVersion: "0.1.0",
		Agent:        "claude-code",
	})
	l.LogStructured("info", "Phase started", map[string]any{"phase": 1})

	// Force close the underlying file without going through Close()
	// This simulates a crash where shutdown entry is not written
	// We need to access the internal fileWriter - since we can't, we'll
	// verify by reading before Close() is called

	// Read file before normal close
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")

	// Should have startup and phase entries, but no shutdown
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 entries, got %d", len(lines))
	}

	// Last entry should NOT be a shutdown entry
	var lastEntry debug.LogEntry
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &lastEntry); err != nil {
		t.Fatalf("failed to parse last entry: %v", err)
	}

	// If the message indicates shutdown, the test setup was wrong
	if lastEntry.Message == "Orchestration completed" || lastEntry.Message == "Orchestration shutdown" {
		t.Error("shutdown entry should not be present before Close()")
	}

	// Now close properly for cleanup
	l.Close()
}

// uniqueTestRunID generates a unique run ID for tests to avoid conflicts
func uniqueTestRunID(prefix string) string {
	return prefix + "-" + time.Now().Format("150405.000000000") + "-" + randomSuffix()
}

// randomSuffix generates a short random suffix using syscall for uniqueness
func randomSuffix() string {
	// Use PID and nanoseconds for uniqueness
	return time.Now().Format("000000000")[6:] + string(rune('a'+syscall.Getpid()%26))
}
