package orbit

import (
	"testing"
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
