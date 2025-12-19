package rune

import (
	"testing"
)

func TestNewClient(t *testing.T) {
	tasksFile := "specs/test/tasks.md"
	client := NewClient(tasksFile)

	if client == nil {
		t.Fatal("NewClient returned nil")
	}

	if got := client.TasksFile(); got != tasksFile {
		t.Errorf("TasksFile() = %q, want %q", got, tasksFile)
	}
}

func TestStatus_Values(t *testing.T) {
	// Verify status constants have expected values
	tests := map[string]struct {
		status Status
		want   int
	}{
		"pending":     {StatusPending, 0},
		"in_progress": {StatusInProgress, 1},
		"completed":   {StatusCompleted, 2},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if int(tc.status) != tc.want {
				t.Errorf("got %d, want %d", tc.status, tc.want)
			}
		})
	}
}

func TestTask_Struct(t *testing.T) {
	// Verify Task struct can be created with all fields
	task := Task{
		ID:      "task-1",
		Title:   "Test Task",
		Status:  StatusPending,
		Details: "Some details",
		Parent:  "parent-1",
		Phase:   "Phase 1",
		Subtasks: []Task{
			{ID: "subtask-1", Title: "Subtask", Status: StatusCompleted},
		},
	}

	if task.ID != "task-1" {
		t.Errorf("ID = %q, want %q", task.ID, "task-1")
	}
	if task.Title != "Test Task" {
		t.Errorf("Title = %q, want %q", task.Title, "Test Task")
	}
	if task.Status != StatusPending {
		t.Errorf("Status = %d, want %d", task.Status, StatusPending)
	}
	if len(task.Subtasks) != 1 {
		t.Errorf("Subtasks length = %d, want 1", len(task.Subtasks))
	}
}

func TestNextPhaseResult_AllComplete(t *testing.T) {
	result := NextPhaseResult{
		AllComplete: true,
	}

	if !result.AllComplete {
		t.Error("AllComplete should be true")
	}
	if len(result.Tasks) != 0 {
		t.Errorf("Tasks should be empty, got %d", len(result.Tasks))
	}
}

func TestNextPhaseResult_WithTasks(t *testing.T) {
	result := NextPhaseResult{
		PhaseName: "Phase 1: Setup",
		Tasks: []Task{
			{ID: "1", Title: "Task 1", Status: StatusPending},
			{ID: "2", Title: "Task 2", Status: StatusPending},
		},
		FrontMatterReferences: []string{"requirements.md", "design.md"},
		AllComplete:           false,
	}

	if result.PhaseName != "Phase 1: Setup" {
		t.Errorf("PhaseName = %q, want %q", result.PhaseName, "Phase 1: Setup")
	}
	if len(result.Tasks) != 2 {
		t.Errorf("Tasks length = %d, want 2", len(result.Tasks))
	}
	if len(result.FrontMatterReferences) != 2 {
		t.Errorf("FrontMatterReferences length = %d, want 2", len(result.FrontMatterReferences))
	}
}
