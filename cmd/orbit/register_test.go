package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/arjenschwarz/orbit/internal/logs"
	"github.com/arjenschwarz/orbit/internal/registry"
)

func TestIsValidOrbitLogDir(t *testing.T) {
	t.Run("valid with phase session files", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create phase-1-run-1-session.json
		if err := os.WriteFile(
			filepath.Join(tmpDir, "phase-1-run-1-session.json"),
			[]byte("{}"),
			0644,
		); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		if !isValidOrbitLogDir(tmpDir) {
			t.Error("isValidOrbitLogDir() = false, want true")
		}
	})

	t.Run("valid with multiple phase files", func(t *testing.T) {
		tmpDir := t.TempDir()

		files := []string{
			"phase-1-run-1-session.json",
			"phase-2-run-1-session.json",
			"phase-3-run-2-session.json",
		}
		for _, f := range files {
			if err := os.WriteFile(
				filepath.Join(tmpDir, f),
				[]byte("{}"),
				0644,
			); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}
		}

		if !isValidOrbitLogDir(tmpDir) {
			t.Error("isValidOrbitLogDir() = false, want true")
		}
	})

	t.Run("invalid empty directory", func(t *testing.T) {
		tmpDir := t.TempDir()

		if isValidOrbitLogDir(tmpDir) {
			t.Error("isValidOrbitLogDir() = true, want false")
		}
	})

	t.Run("valid with legacy format files", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create legacy format file (phase-N-session.json without run number)
		files := []string{
			"summary.json",
			"phase-1-session.json", // legacy format
			"transcript.md",
		}
		for _, f := range files {
			if err := os.WriteFile(
				filepath.Join(tmpDir, f),
				[]byte("{}"),
				0644,
			); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}
		}

		if !isValidOrbitLogDir(tmpDir) {
			t.Error("isValidOrbitLogDir() = false, want true (legacy format should be valid)")
		}
	})

	t.Run("invalid with non-matching files", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create files that don't match any pattern
		files := []string{
			"summary.json",
			"transcript.md",
			"random-file.json",
		}
		for _, f := range files {
			if err := os.WriteFile(
				filepath.Join(tmpDir, f),
				[]byte("{}"),
				0644,
			); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}
		}

		if isValidOrbitLogDir(tmpDir) {
			t.Error("isValidOrbitLogDir() = true, want false")
		}
	})

	t.Run("invalid path does not exist", func(t *testing.T) {
		if isValidOrbitLogDir("/non/existent/path") {
			t.Error("isValidOrbitLogDir() = true, want false")
		}
	})
}

func TestDeriveStatus(t *testing.T) {
	t.Run("completed from summary with success status", func(t *testing.T) {
		tmpDir := t.TempDir()

		summary := logs.Summary{
			Status:    "success",
			StartedAt: time.Now(),
		}
		data, _ := json.MarshalIndent(summary, "", "  ")
		if err := os.WriteFile(
			filepath.Join(tmpDir, "summary.json"),
			data,
			0644,
		); err != nil {
			t.Fatalf("Failed to create summary: %v", err)
		}

		got := deriveStatus(tmpDir)
		if got != registry.StatusCompleted {
			t.Errorf("deriveStatus() = %q, want %q", got, registry.StatusCompleted)
		}
	})

	t.Run("failed from summary with failed status", func(t *testing.T) {
		tmpDir := t.TempDir()

		summary := logs.Summary{
			Status:    "failed",
			StartedAt: time.Now(),
			Error:     "some error",
		}
		data, _ := json.MarshalIndent(summary, "", "  ")
		if err := os.WriteFile(
			filepath.Join(tmpDir, "summary.json"),
			data,
			0644,
		); err != nil {
			t.Fatalf("Failed to create summary: %v", err)
		}

		got := deriveStatus(tmpDir)
		if got != registry.StatusFailed {
			t.Errorf("deriveStatus() = %q, want %q", got, registry.StatusFailed)
		}
	})

	t.Run("completed when no summary exists", func(t *testing.T) {
		tmpDir := t.TempDir()

		got := deriveStatus(tmpDir)
		if got != registry.StatusCompleted {
			t.Errorf("deriveStatus() = %q, want %q", got, registry.StatusCompleted)
		}
	})

	t.Run("completed when summary has unknown status", func(t *testing.T) {
		tmpDir := t.TempDir()

		summary := logs.Summary{
			Status:    "running",
			StartedAt: time.Now(),
		}
		data, _ := json.MarshalIndent(summary, "", "  ")
		if err := os.WriteFile(
			filepath.Join(tmpDir, "summary.json"),
			data,
			0644,
		); err != nil {
			t.Fatalf("Failed to create summary: %v", err)
		}

		// For historical runs without explicit failed status, assume completed
		got := deriveStatus(tmpDir)
		if got != registry.StatusCompleted {
			t.Errorf("deriveStatus() = %q, want %q", got, registry.StatusCompleted)
		}
	})
}

func TestDerivePhases(t *testing.T) {
	t.Run("extracts phases from session files", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create phase files
		files := []string{
			"phase-1-run-1-session.json",
			"phase-2-run-1-session.json",
			"phase-2-run-2-session.json", // retry
			"phase-3-run-1-session.json",
		}
		for _, f := range files {
			if err := os.WriteFile(
				filepath.Join(tmpDir, f),
				[]byte("{}"),
				0644,
			); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}
		}

		phases := derivePhases(tmpDir)

		// Should have 3 phases
		if len(phases) != 3 {
			t.Fatalf("len(phases) = %d, want 3", len(phases))
		}

		// Check phase 1
		if phases[0].Number != 1 {
			t.Errorf("phases[0].Number = %d, want 1", phases[0].Number)
		}
		if phases[0].RunCount != 1 {
			t.Errorf("phases[0].RunCount = %d, want 1", phases[0].RunCount)
		}
		if phases[0].Status != registry.PhaseStatusCompleted {
			t.Errorf("phases[0].Status = %q, want %q", phases[0].Status, registry.PhaseStatusCompleted)
		}

		// Check phase 2 (has 2 runs)
		if phases[1].Number != 2 {
			t.Errorf("phases[1].Number = %d, want 2", phases[1].Number)
		}
		if phases[1].RunCount != 2 {
			t.Errorf("phases[1].RunCount = %d, want 2", phases[1].RunCount)
		}

		// Check phase 3
		if phases[2].Number != 3 {
			t.Errorf("phases[2].Number = %d, want 3", phases[2].Number)
		}
	})

	t.Run("empty for directory without session files", func(t *testing.T) {
		tmpDir := t.TempDir()

		phases := derivePhases(tmpDir)
		if len(phases) != 0 {
			t.Errorf("len(phases) = %d, want 0", len(phases))
		}
	})

	t.Run("sorted by phase number", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create files out of order
		files := []string{
			"phase-3-run-1-session.json",
			"phase-1-run-1-session.json",
			"phase-5-run-1-session.json",
			"phase-2-run-1-session.json",
		}
		for _, f := range files {
			if err := os.WriteFile(
				filepath.Join(tmpDir, f),
				[]byte("{}"),
				0644,
			); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}
		}

		phases := derivePhases(tmpDir)

		if len(phases) != 4 {
			t.Fatalf("len(phases) = %d, want 4", len(phases))
		}

		for i := 0; i < len(phases)-1; i++ {
			if phases[i].Number >= phases[i+1].Number {
				t.Errorf("phases not sorted: %d >= %d", phases[i].Number, phases[i+1].Number)
			}
		}
	})
}

func TestDeriveStartedAt(t *testing.T) {
	t.Run("uses summary started_at when available", func(t *testing.T) {
		tmpDir := t.TempDir()

		expectedTime := time.Date(2025, 1, 5, 10, 30, 0, 0, time.UTC)
		summary := logs.Summary{
			Status:    "success",
			StartedAt: expectedTime,
		}
		data, _ := json.MarshalIndent(summary, "", "  ")
		if err := os.WriteFile(
			filepath.Join(tmpDir, "summary.json"),
			data,
			0644,
		); err != nil {
			t.Fatalf("Failed to create summary: %v", err)
		}

		got := deriveStartedAt(tmpDir)
		if !got.Equal(expectedTime) {
			t.Errorf("deriveStartedAt() = %v, want %v", got, expectedTime)
		}
	})

	t.Run("uses file mtime when no summary", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create a phase file
		phaseFile := filepath.Join(tmpDir, "phase-1-run-1-session.json")
		if err := os.WriteFile(phaseFile, []byte("{}"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		got := deriveStartedAt(tmpDir)

		// Should be close to now (within a few seconds)
		if time.Since(got) > 5*time.Second {
			t.Errorf("deriveStartedAt() = %v, expected recent time", got)
		}
	})
}

func TestResolvePath(t *testing.T) {
	t.Run("dot resolves to .orbit subdirectory", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create .orbit directory with valid log files
		orbitDir := filepath.Join(tmpDir, ".orbit")
		if err := os.MkdirAll(orbitDir, 0755); err != nil {
			t.Fatalf("Failed to create .orbit dir: %v", err)
		}
		if err := os.WriteFile(
			filepath.Join(orbitDir, "phase-1-run-1-session.json"),
			[]byte("{}"),
			0644,
		); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		// Change to tmpDir for the test
		oldWd, _ := os.Getwd()
		if err := os.Chdir(tmpDir); err != nil {
			t.Fatalf("Failed to chdir: %v", err)
		}
		defer func() { _ = os.Chdir(oldWd) }()

		got, err := resolvePath(".")
		if err != nil {
			t.Fatalf("resolvePath() error: %v", err)
		}

		// Resolve symlinks on both paths for comparison (macOS uses /private/var vs /var)
		gotReal, _ := filepath.EvalSymlinks(got)
		wantReal, _ := filepath.EvalSymlinks(orbitDir)
		if gotReal != wantReal {
			t.Errorf("resolvePath('.') = %q, want %q", got, orbitDir)
		}
	})

	t.Run("absolute path returned unchanged", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create log files directly in tmpDir
		if err := os.WriteFile(
			filepath.Join(tmpDir, "phase-1-run-1-session.json"),
			[]byte("{}"),
			0644,
		); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		got, err := resolvePath(tmpDir)
		if err != nil {
			t.Fatalf("resolvePath() error: %v", err)
		}

		if got != tmpDir {
			t.Errorf("resolvePath() = %q, want %q", got, tmpDir)
		}
	})

	t.Run("error when dot has no .orbit directory", func(t *testing.T) {
		tmpDir := t.TempDir()

		oldWd, _ := os.Getwd()
		if err := os.Chdir(tmpDir); err != nil {
			t.Fatalf("Failed to chdir: %v", err)
		}
		defer func() { _ = os.Chdir(oldWd) }()

		_, err := resolvePath(".")
		if err == nil {
			t.Error("resolvePath('.') should error when no .orbit dir exists")
		}
	})
}

func TestRegisterCommand(t *testing.T) {
	// Setup a registry for testing
	setupTestRegistry := func(t *testing.T) (string, string) {
		t.Helper()
		tmpDir := t.TempDir()

		// Set HOME to tmpDir so registry uses tmpDir/.orbit/runs
		t.Setenv("HOME", tmpDir)

		// Create a valid log directory
		logDir := filepath.Join(tmpDir, "project", ".orbit")
		if err := os.MkdirAll(logDir, 0755); err != nil {
			t.Fatalf("Failed to create log dir: %v", err)
		}
		if err := os.WriteFile(
			filepath.Join(logDir, "phase-1-run-1-session.json"),
			[]byte("{}"),
			0644,
		); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		return tmpDir, logDir
	}

	t.Run("registers valid log directory", func(t *testing.T) {
		_, logDir := setupTestRegistry(t)

		err := registerCommand([]string{logDir})
		if err != nil {
			t.Fatalf("registerCommand() error: %v", err)
		}

		// Verify registration
		regDir, _ := getRegistryDir()
		reg, err := registry.New(regDir)
		if err != nil {
			t.Fatalf("Failed to open registry: %v", err)
		}

		entries, err := reg.List()
		if err != nil {
			t.Fatalf("List() error: %v", err)
		}

		if len(entries) != 1 {
			t.Fatalf("len(entries) = %d, want 1", len(entries))
		}

		if entries[0].LogDir != logDir {
			t.Errorf("LogDir = %q, want %q", entries[0].LogDir, logDir)
		}
	})

	t.Run("registers with custom name", func(t *testing.T) {
		_, logDir := setupTestRegistry(t)

		err := registerCommand([]string{"--name", "my-custom-run", logDir})
		if err != nil {
			t.Fatalf("registerCommand() error: %v", err)
		}

		regDir, _ := getRegistryDir()
		reg, _ := registry.New(regDir)
		entries, _ := reg.List()

		if len(entries) != 1 {
			t.Fatalf("len(entries) = %d, want 1", len(entries))
		}

		if entries[0].Name != "my-custom-run" {
			t.Errorf("Name = %q, want %q", entries[0].Name, "my-custom-run")
		}
	})

	t.Run("updates existing entry", func(t *testing.T) {
		tmpDir, logDir := setupTestRegistry(t)

		// First registration
		err := registerCommand([]string{"--name", "first-name", logDir})
		if err != nil {
			t.Fatalf("First registerCommand() error: %v", err)
		}

		// Get the original ID
		regDir := filepath.Join(tmpDir, ".orbit", "runs")
		reg, _ := registry.New(regDir)
		entries, _ := reg.List()
		originalID := entries[0].ID

		// Second registration with same log directory
		err = registerCommand([]string{"--name", "updated-name", logDir})
		if err != nil {
			t.Fatalf("Second registerCommand() error: %v", err)
		}

		// Verify update
		entries, _ = reg.List()
		if len(entries) != 1 {
			t.Fatalf("len(entries) = %d, want 1", len(entries))
		}

		if entries[0].ID != originalID {
			t.Errorf("ID changed: got %q, want %q", entries[0].ID, originalID)
		}
		if entries[0].Name != "updated-name" {
			t.Errorf("Name = %q, want %q", entries[0].Name, "updated-name")
		}
	})

	t.Run("error for invalid log directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		err := registerCommand([]string{"/non/existent/path"})
		if err == nil {
			t.Error("registerCommand() should error for invalid path")
		}
	})

	t.Run("error for directory without orbit logs", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		// Create an empty directory
		emptyDir := filepath.Join(tmpDir, "empty")
		if err := os.MkdirAll(emptyDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}

		err := registerCommand([]string{emptyDir})
		if err == nil {
			t.Error("registerCommand() should error for directory without orbit logs")
		}
	})

	t.Run("pid is nil for manual registration", func(t *testing.T) {
		_, logDir := setupTestRegistry(t)

		err := registerCommand([]string{logDir})
		if err != nil {
			t.Fatalf("registerCommand() error: %v", err)
		}

		regDir, _ := getRegistryDir()
		reg, _ := registry.New(regDir)
		entries, _ := reg.List()

		if entries[0].PID != nil {
			t.Errorf("PID = %v, want nil", entries[0].PID)
		}
	})

	t.Run("derives status from summary", func(t *testing.T) {
		tmpDir, logDir := setupTestRegistry(t)
		_ = tmpDir

		// Add summary.json with failed status
		summary := logs.Summary{
			Status:    "failed",
			StartedAt: time.Now(),
			Error:     "test error",
		}
		data, _ := json.MarshalIndent(summary, "", "  ")
		if err := os.WriteFile(
			filepath.Join(logDir, "summary.json"),
			data,
			0644,
		); err != nil {
			t.Fatalf("Failed to create summary: %v", err)
		}

		err := registerCommand([]string{logDir})
		if err != nil {
			t.Fatalf("registerCommand() error: %v", err)
		}

		regDir, _ := getRegistryDir()
		reg, _ := registry.New(regDir)
		entries, _ := reg.List()

		if entries[0].Status != registry.StatusFailed {
			t.Errorf("Status = %q, want %q", entries[0].Status, registry.StatusFailed)
		}
	})

	t.Run("derives phases from session files", func(t *testing.T) {
		tmpDir, logDir := setupTestRegistry(t)
		_ = tmpDir

		// Add more phase files
		files := []string{
			"phase-2-run-1-session.json",
			"phase-3-run-1-session.json",
		}
		for _, f := range files {
			if err := os.WriteFile(
				filepath.Join(logDir, f),
				[]byte("{}"),
				0644,
			); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}
		}

		err := registerCommand([]string{logDir})
		if err != nil {
			t.Fatalf("registerCommand() error: %v", err)
		}

		regDir, _ := getRegistryDir()
		reg, _ := registry.New(regDir)
		entries, _ := reg.List()

		if len(entries[0].Phases) != 3 {
			t.Errorf("len(Phases) = %d, want 3", len(entries[0].Phases))
		}
	})
}
