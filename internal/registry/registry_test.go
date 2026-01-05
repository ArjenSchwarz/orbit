package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	t.Run("creates directory if not exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		regDir := filepath.Join(tmpDir, ".orbit", "runs")

		reg, err := New(regDir)
		if err != nil {
			t.Fatalf("New() error: %v", err)
		}

		if reg == nil {
			t.Fatal("New() returned nil registry")
		}

		// Verify directory was created
		info, err := os.Stat(regDir)
		if err != nil {
			t.Fatalf("Registry directory not created: %v", err)
		}
		if !info.IsDir() {
			t.Error("Registry path is not a directory")
		}
	})

	t.Run("uses existing directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		regDir := filepath.Join(tmpDir, ".orbit", "runs")

		// Create directory first
		if err := os.MkdirAll(regDir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		reg, err := New(regDir)
		if err != nil {
			t.Fatalf("New() error: %v", err)
		}

		if reg == nil {
			t.Fatal("New() returned nil registry")
		}
	})
}

func TestRegister(t *testing.T) {
	t.Run("creates new entry", func(t *testing.T) {
		reg := newTestRegistry(t)

		entry := &RunEntry{
			ID:            "550e8400-e29b-41d4-a716-446655440000",
			SchemaVersion: 1,
			Name:          "test-run",
			Repository:    "owner/repo",
			LogDir:        "/path/to/logs",
			Status:        StatusRunning,
			StartedAt:     time.Now(),
			Branch:        "main",
		}

		if err := reg.Register(entry); err != nil {
			t.Fatalf("Register() error: %v", err)
		}

		// Verify file was created
		filePath := filepath.Join(reg.dir, entry.ID+".json")
		if _, err := os.Stat(filePath); err != nil {
			t.Errorf("Registry file not created: %v", err)
		}

		// Verify content
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("Failed to read registry file: %v", err)
		}

		var got RunEntry
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("Failed to unmarshal entry: %v", err)
		}

		if got.ID != entry.ID {
			t.Errorf("ID = %q, want %q", got.ID, entry.ID)
		}
		if got.Name != entry.Name {
			t.Errorf("Name = %q, want %q", got.Name, entry.Name)
		}
	})

	t.Run("updates existing entry", func(t *testing.T) {
		reg := newTestRegistry(t)

		entry := &RunEntry{
			ID:            "550e8400-e29b-41d4-a716-446655440000",
			SchemaVersion: 1,
			Name:          "test-run",
			Repository:    "owner/repo",
			LogDir:        "/path/to/logs",
			Status:        StatusRunning,
			StartedAt:     time.Now(),
			Branch:        "main",
		}

		if err := reg.Register(entry); err != nil {
			t.Fatalf("First Register() error: %v", err)
		}

		// Update entry
		entry.Status = StatusCompleted
		now := time.Now()
		entry.FinishedAt = &now

		if err := reg.Register(entry); err != nil {
			t.Fatalf("Second Register() error: %v", err)
		}

		// Verify update
		got, err := reg.Get(entry.ID)
		if err != nil {
			t.Fatalf("Get() error: %v", err)
		}

		if got.Status != StatusCompleted {
			t.Errorf("Status = %q, want %q", got.Status, StatusCompleted)
		}
		if got.FinishedAt == nil {
			t.Error("FinishedAt is nil, want non-nil")
		}
	})
}

func TestGet(t *testing.T) {
	t.Run("returns existing entry", func(t *testing.T) {
		reg := newTestRegistry(t)

		entry := &RunEntry{
			ID:            "550e8400-e29b-41d4-a716-446655440000",
			SchemaVersion: 1,
			Name:          "test-run",
			Repository:    "owner/repo",
			LogDir:        "/path/to/logs",
			Status:        StatusRunning,
			StartedAt:     time.Now(),
			Branch:        "main",
		}

		if err := reg.Register(entry); err != nil {
			t.Fatalf("Register() error: %v", err)
		}

		got, err := reg.Get(entry.ID)
		if err != nil {
			t.Fatalf("Get() error: %v", err)
		}

		if got.ID != entry.ID {
			t.Errorf("ID = %q, want %q", got.ID, entry.ID)
		}
	})

	t.Run("returns nil for non-existent entry", func(t *testing.T) {
		reg := newTestRegistry(t)

		got, err := reg.Get("non-existent-id")
		if err != nil {
			t.Fatalf("Get() error: %v", err)
		}

		if got != nil {
			t.Errorf("Get() = %v, want nil", got)
		}
	})
}

func TestList(t *testing.T) {
	t.Run("returns all entries", func(t *testing.T) {
		reg := newTestRegistry(t)

		entries := []*RunEntry{
			{
				ID:            "550e8400-e29b-41d4-a716-446655440001",
				SchemaVersion: 1,
				Name:          "run-1",
				Repository:    "owner/repo",
				LogDir:        "/path/to/logs1",
				Status:        StatusCompleted,
				StartedAt:     time.Now(),
				Branch:        "main",
			},
			{
				ID:            "550e8400-e29b-41d4-a716-446655440002",
				SchemaVersion: 1,
				Name:          "run-2",
				Repository:    "owner/repo",
				LogDir:        "/path/to/logs2",
				Status:        StatusRunning,
				StartedAt:     time.Now(),
				Branch:        "feature",
			},
		}

		for _, entry := range entries {
			if err := reg.Register(entry); err != nil {
				t.Fatalf("Register() error: %v", err)
			}
		}

		got, err := reg.List()
		if err != nil {
			t.Fatalf("List() error: %v", err)
		}

		if len(got) != len(entries) {
			t.Errorf("len(List()) = %d, want %d", len(got), len(entries))
		}
	})

	t.Run("returns empty list for empty registry", func(t *testing.T) {
		reg := newTestRegistry(t)

		got, err := reg.List()
		if err != nil {
			t.Fatalf("List() error: %v", err)
		}

		if len(got) != 0 {
			t.Errorf("len(List()) = %d, want 0", len(got))
		}
	})

	t.Run("skips malformed JSON", func(t *testing.T) {
		reg := newTestRegistry(t)

		// Create a valid entry
		entry := &RunEntry{
			ID:            "550e8400-e29b-41d4-a716-446655440001",
			SchemaVersion: 1,
			Name:          "valid-run",
			Repository:    "owner/repo",
			LogDir:        "/path/to/logs",
			Status:        StatusCompleted,
			StartedAt:     time.Now(),
			Branch:        "main",
		}
		if err := reg.Register(entry); err != nil {
			t.Fatalf("Register() error: %v", err)
		}

		// Create a malformed JSON file
		malformedPath := filepath.Join(reg.dir, "malformed.json")
		if err := os.WriteFile(malformedPath, []byte("{invalid json"), 0644); err != nil {
			t.Fatalf("Failed to create malformed file: %v", err)
		}

		got, err := reg.List()
		if err != nil {
			t.Fatalf("List() error: %v", err)
		}

		// Should only return the valid entry
		if len(got) != 1 {
			t.Errorf("len(List()) = %d, want 1", len(got))
		}
	})

	t.Run("skips non-json files", func(t *testing.T) {
		reg := newTestRegistry(t)

		// Create a valid entry
		entry := &RunEntry{
			ID:            "550e8400-e29b-41d4-a716-446655440001",
			SchemaVersion: 1,
			Name:          "valid-run",
			Repository:    "owner/repo",
			LogDir:        "/path/to/logs",
			Status:        StatusCompleted,
			StartedAt:     time.Now(),
			Branch:        "main",
		}
		if err := reg.Register(entry); err != nil {
			t.Fatalf("Register() error: %v", err)
		}

		// Create a non-JSON file
		otherPath := filepath.Join(reg.dir, "readme.txt")
		if err := os.WriteFile(otherPath, []byte("not a json file"), 0644); err != nil {
			t.Fatalf("Failed to create other file: %v", err)
		}

		got, err := reg.List()
		if err != nil {
			t.Fatalf("List() error: %v", err)
		}

		if len(got) != 1 {
			t.Errorf("len(List()) = %d, want 1", len(got))
		}
	})
}

func TestFindByLogDir(t *testing.T) {
	t.Run("finds entry by log directory", func(t *testing.T) {
		reg := newTestRegistry(t)

		entry := &RunEntry{
			ID:            "550e8400-e29b-41d4-a716-446655440001",
			SchemaVersion: 1,
			Name:          "test-run",
			Repository:    "owner/repo",
			LogDir:        "/path/to/specific/logs",
			Status:        StatusCompleted,
			StartedAt:     time.Now(),
			Branch:        "main",
		}
		if err := reg.Register(entry); err != nil {
			t.Fatalf("Register() error: %v", err)
		}

		got, err := reg.FindByLogDir("/path/to/specific/logs")
		if err != nil {
			t.Fatalf("FindByLogDir() error: %v", err)
		}

		if got == nil {
			t.Fatal("FindByLogDir() returned nil")
		}
		if got.ID != entry.ID {
			t.Errorf("ID = %q, want %q", got.ID, entry.ID)
		}
	})

	t.Run("returns nil for non-existent log directory", func(t *testing.T) {
		reg := newTestRegistry(t)

		got, err := reg.FindByLogDir("/non/existent/path")
		if err != nil {
			t.Fatalf("FindByLogDir() error: %v", err)
		}

		if got != nil {
			t.Errorf("FindByLogDir() = %v, want nil", got)
		}
	})
}

func TestUpdateStatus(t *testing.T) {
	t.Run("updates status", func(t *testing.T) {
		reg := newTestRegistry(t)

		entry := &RunEntry{
			ID:            "550e8400-e29b-41d4-a716-446655440001",
			SchemaVersion: 1,
			Name:          "test-run",
			Repository:    "owner/repo",
			LogDir:        "/path/to/logs",
			Status:        StatusRunning,
			StartedAt:     time.Now(),
			Branch:        "main",
		}
		if err := reg.Register(entry); err != nil {
			t.Fatalf("Register() error: %v", err)
		}

		if err := reg.UpdateStatus(entry.ID, StatusCompleted); err != nil {
			t.Fatalf("UpdateStatus() error: %v", err)
		}

		got, err := reg.Get(entry.ID)
		if err != nil {
			t.Fatalf("Get() error: %v", err)
		}

		if got.Status != StatusCompleted {
			t.Errorf("Status = %q, want %q", got.Status, StatusCompleted)
		}
	})

	t.Run("returns error for non-existent entry", func(t *testing.T) {
		reg := newTestRegistry(t)

		err := reg.UpdateStatus("non-existent-id", StatusCompleted)
		if err == nil {
			t.Error("UpdateStatus() should return error for non-existent entry")
		}
	})
}

func TestUpdatePhase(t *testing.T) {
	t.Run("adds new phase", func(t *testing.T) {
		reg := newTestRegistry(t)

		entry := &RunEntry{
			ID:            "550e8400-e29b-41d4-a716-446655440001",
			SchemaVersion: 1,
			Name:          "test-run",
			Repository:    "owner/repo",
			LogDir:        "/path/to/logs",
			Status:        StatusRunning,
			StartedAt:     time.Now(),
			Branch:        "main",
			Phases:        []Phase{},
		}
		if err := reg.Register(entry); err != nil {
			t.Fatalf("Register() error: %v", err)
		}

		phase := Phase{
			Number:   1,
			Status:   PhaseStatusRunning,
			RunCount: 1,
		}
		if err := reg.UpdatePhase(entry.ID, phase); err != nil {
			t.Fatalf("UpdatePhase() error: %v", err)
		}

		got, err := reg.Get(entry.ID)
		if err != nil {
			t.Fatalf("Get() error: %v", err)
		}

		if len(got.Phases) != 1 {
			t.Fatalf("len(Phases) = %d, want 1", len(got.Phases))
		}
		if got.Phases[0].Number != 1 {
			t.Errorf("Phase.Number = %d, want 1", got.Phases[0].Number)
		}
		if got.Phases[0].Status != PhaseStatusRunning {
			t.Errorf("Phase.Status = %q, want %q", got.Phases[0].Status, PhaseStatusRunning)
		}
	})

	t.Run("updates existing phase", func(t *testing.T) {
		reg := newTestRegistry(t)

		entry := &RunEntry{
			ID:            "550e8400-e29b-41d4-a716-446655440001",
			SchemaVersion: 1,
			Name:          "test-run",
			Repository:    "owner/repo",
			LogDir:        "/path/to/logs",
			Status:        StatusRunning,
			StartedAt:     time.Now(),
			Branch:        "main",
			Phases: []Phase{
				{Number: 1, Status: PhaseStatusRunning, RunCount: 1},
			},
		}
		if err := reg.Register(entry); err != nil {
			t.Fatalf("Register() error: %v", err)
		}

		phase := Phase{
			Number:   1,
			Status:   PhaseStatusCompleted,
			RunCount: 1,
		}
		if err := reg.UpdatePhase(entry.ID, phase); err != nil {
			t.Fatalf("UpdatePhase() error: %v", err)
		}

		got, err := reg.Get(entry.ID)
		if err != nil {
			t.Fatalf("Get() error: %v", err)
		}

		if len(got.Phases) != 1 {
			t.Fatalf("len(Phases) = %d, want 1", len(got.Phases))
		}
		if got.Phases[0].Status != PhaseStatusCompleted {
			t.Errorf("Phase.Status = %q, want %q", got.Phases[0].Status, PhaseStatusCompleted)
		}
	})

	t.Run("returns error for non-existent entry", func(t *testing.T) {
		reg := newTestRegistry(t)

		phase := Phase{
			Number:   1,
			Status:   PhaseStatusRunning,
			RunCount: 1,
		}
		err := reg.UpdatePhase("non-existent-id", phase)
		if err == nil {
			t.Error("UpdatePhase() should return error for non-existent entry")
		}
	})
}

func TestAtomicWrite(t *testing.T) {
	t.Run("write is atomic", func(t *testing.T) {
		reg := newTestRegistry(t)

		entry := &RunEntry{
			ID:            "550e8400-e29b-41d4-a716-446655440001",
			SchemaVersion: 1,
			Name:          "test-run",
			Repository:    "owner/repo",
			LogDir:        "/path/to/logs",
			Status:        StatusRunning,
			StartedAt:     time.Now(),
			Branch:        "main",
		}

		if err := reg.Register(entry); err != nil {
			t.Fatalf("Register() error: %v", err)
		}

		// Verify no temp files remain
		files, err := os.ReadDir(reg.dir)
		if err != nil {
			t.Fatalf("ReadDir() error: %v", err)
		}

		for _, f := range files {
			if filepath.Ext(f.Name()) != ".json" {
				t.Errorf("Found non-JSON file: %s (temp file leaked?)", f.Name())
			}
		}
	})
}

// newTestRegistry creates a new registry in a temporary directory for testing.
func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	tmpDir := t.TempDir()
	regDir := filepath.Join(tmpDir, ".orbit", "runs")

	reg, err := New(regDir)
	if err != nil {
		t.Fatalf("Failed to create test registry: %v", err)
	}

	return reg
}
