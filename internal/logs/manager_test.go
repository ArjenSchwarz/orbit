package logs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/arjenschwarz/orbit/internal/claude"
)

func TestNewManager(t *testing.T) {
	tmpDir := t.TempDir()

	m, err := NewManager(tmpDir, "feature/test-branch", "/tmp/test-project")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Check session directory was created
	if _, err := os.Stat(m.SessionDir()); os.IsNotExist(err) {
		t.Error("session directory was not created")
	}

	// Check summary.json was created
	summaryPath := filepath.Join(m.SessionDir(), "summary.json")
	if _, err := os.Stat(summaryPath); os.IsNotExist(err) {
		t.Error("summary.json was not created")
	}

	// Verify summary content
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("failed to read summary: %v", err)
	}

	var summary Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("failed to parse summary: %v", err)
	}

	if summary.Status != "running" {
		t.Errorf("got status %q, want %q", summary.Status, "running")
	}
}

func TestManager_SaveSession(t *testing.T) {
	tmpDir := t.TempDir()

	m, err := NewManager(tmpDir, "test-branch", "/tmp/test-project")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	result := &claude.SessionResult{
		SessionID: "test-session-123",
		Cost:      0.15,
		Duration:  45 * time.Second,
		NumTurns:  5,
		Output:    "Test output",
		IsError:   false,
		RawJSON:   []byte(`{"session_id": "test-session-123"}`),
		Stderr:    "",
	}

	startTime := time.Now().Add(-45 * time.Second)
	if err := m.SaveSession(1, result, startTime); err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	// Check phase JSON file
	jsonPath := filepath.Join(m.SessionDir(), "phase-1-session.json")
	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		t.Error("phase JSON file was not created")
	}

	// Check phase transcript file
	txtPath := filepath.Join(m.SessionDir(), "phase-1-session.txt")
	if _, err := os.Stat(txtPath); os.IsNotExist(err) {
		t.Error("phase transcript file was not created")
	}

	// Verify summary was updated
	summaryPath := filepath.Join(m.SessionDir(), "summary.json")
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("failed to read summary: %v", err)
	}

	var summary Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("failed to parse summary: %v", err)
	}

	if summary.PhasesCompleted != 1 {
		t.Errorf("got phases_completed %d, want 1", summary.PhasesCompleted)
	}
	if len(summary.Sessions) != 1 {
		t.Errorf("got %d sessions, want 1", len(summary.Sessions))
	}
	if summary.TotalCostUSD != 0.15 {
		t.Errorf("got total cost %f, want 0.15", summary.TotalCostUSD)
	}
}

func TestManager_Complete(t *testing.T) {
	tmpDir := t.TempDir()

	m, err := NewManager(tmpDir, "test-branch", "/tmp/test-project")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	if err := m.Complete(); err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	summaryPath := filepath.Join(m.SessionDir(), "summary.json")
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("failed to read summary: %v", err)
	}

	var summary Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("failed to parse summary: %v", err)
	}

	if summary.Status != "success" {
		t.Errorf("got status %q, want %q", summary.Status, "success")
	}
	if summary.CompletedAt == nil {
		t.Error("completed_at should be set")
	}
}

func TestManager_Fail(t *testing.T) {
	tmpDir := t.TempDir()

	m, err := NewManager(tmpDir, "test-branch", "/tmp/test-project")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	testErr := os.ErrNotExist
	if err := m.Fail(testErr); err != nil {
		t.Fatalf("Fail failed: %v", err)
	}

	summaryPath := filepath.Join(m.SessionDir(), "summary.json")
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("failed to read summary: %v", err)
	}

	var summary Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("failed to parse summary: %v", err)
	}

	if summary.Status != "failed" {
		t.Errorf("got status %q, want %q", summary.Status, "failed")
	}
	if summary.Error == "" {
		t.Error("error message should be set")
	}
}

func TestSanitizeName(t *testing.T) {
	tests := map[string]struct {
		input string
		want  string
	}{
		"simple name":      {"test-branch", "test-branch"},
		"with slash":       {"feature/test", "feature-test"},
		"with spaces":      {"my branch", "mybranch"},
		"special chars":    {"test@branch#1", "testbranch1"},
		"underscores kept": {"test_branch", "test_branch"},
		"numbers kept":     {"branch123", "branch123"},
		"mixed case":       {"Feature/Test", "Feature-Test"},
		"multiple slashes": {"a/b/c", "a-b-c"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := sanitizeName(tc.input)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatTranscript(t *testing.T) {
	result := &claude.SessionResult{
		SessionID: "abc123",
		Cost:      0.25,
		Duration:  30 * time.Second,
		NumTurns:  3,
		Output:    "Test output content",
		IsError:   false,
		Stderr:    "some warning",
	}

	start := time.Date(2025, 1, 15, 14, 30, 0, 0, time.UTC)
	end := time.Date(2025, 1, 15, 14, 30, 30, 0, time.UTC)

	transcript := formatTranscript(1, result, start, end)

	// Check key elements are present
	if !containsString(transcript, "Phase 1") {
		t.Error("transcript should contain phase number")
	}
	if !containsString(transcript, "abc123") {
		t.Error("transcript should contain session ID")
	}
	if !containsString(transcript, "Test output content") {
		t.Error("transcript should contain output")
	}
	if !containsString(transcript, "some warning") {
		t.Error("transcript should contain stderr")
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Note: Tests for formatUserMessage and formatAssistantMessage have been moved to
// internal/transcript/markdown_test.go as part of the transcript package extraction.

func TestManager_SavePostCompletionSession(t *testing.T) {
	tmpDir := t.TempDir()

	m, err := NewManager(tmpDir, "test-branch", "/tmp/test-project")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	result := &claude.SessionResult{
		SessionID: "post-completion-session-456",
		Cost:      0.25,
		Duration:  60 * time.Second,
		NumTurns:  8,
		Output:    "Review complete. All tests pass.",
		IsError:   false,
		RawJSON:   []byte(`{"session_id": "post-completion-session-456"}`),
		Stderr:    "",
	}

	startTime := time.Now().Add(-60 * time.Second)
	if err := m.SavePostCompletionSession(result, startTime); err != nil {
		t.Fatalf("SavePostCompletionSession failed: %v", err)
	}

	// Check post-completion JSON file was created
	jsonPath := filepath.Join(m.SessionDir(), "post-completion-session.json")
	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		t.Error("post-completion JSON file was not created")
	}

	// Verify JSON content
	jsonData, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("failed to read JSON file: %v", err)
	}
	if string(jsonData) != `{"session_id": "post-completion-session-456"}` {
		t.Errorf("unexpected JSON content: %s", string(jsonData))
	}

	// Check post-completion transcript file was created
	txtPath := filepath.Join(m.SessionDir(), "post-completion-session.txt")
	if _, err := os.Stat(txtPath); os.IsNotExist(err) {
		t.Error("post-completion transcript file was not created")
	}

	// Verify transcript content
	txtData, err := os.ReadFile(txtPath)
	if err != nil {
		t.Fatalf("failed to read transcript file: %v", err)
	}

	transcript := string(txtData)
	if !containsString(transcript, "Post-Completion Session Log") {
		t.Error("transcript should contain post-completion header")
	}
	if !containsString(transcript, "post-completion-session-456") {
		t.Error("transcript should contain session ID")
	}
	if !containsString(transcript, "Review complete. All tests pass.") {
		t.Error("transcript should contain output")
	}
}

func TestFormatPostCompletionTranscript(t *testing.T) {
	result := &claude.SessionResult{
		SessionID: "post-123",
		Cost:      0.30,
		Duration:  45 * time.Second,
		NumTurns:  5,
		Output:    "Verification complete",
		IsError:   false,
		Stderr:    "warning: something",
	}

	start := time.Date(2025, 1, 15, 14, 30, 0, 0, time.UTC)
	end := time.Date(2025, 1, 15, 14, 30, 45, 0, time.UTC)

	transcript := formatPostCompletionTranscript(result, start, end)

	// Check key elements are present
	if !containsString(transcript, "Post-Completion Session Log") {
		t.Error("transcript should contain post-completion header")
	}
	if !containsString(transcript, "post-123") {
		t.Error("transcript should contain session ID")
	}
	if !containsString(transcript, "Verification complete") {
		t.Error("transcript should contain output")
	}
	if !containsString(transcript, "warning: something") {
		t.Error("transcript should contain stderr")
	}
	if !containsString(transcript, "$0.3000") {
		t.Error("transcript should contain cost")
	}
	if !containsString(transcript, "Turns:      5") {
		t.Error("transcript should contain turn count")
	}
}

func TestNewManagerWithOptions_FlatMode(t *testing.T) {
	tmpDir := t.TempDir()

	m, err := NewManagerWithOptions(tmpDir, "feature/test-branch", "/tmp/test-project", ManagerOptions{UseSubdirs: false})
	if err != nil {
		t.Fatalf("NewManagerWithOptions failed: %v", err)
	}

	// In flat mode, session dir should be the same as base dir
	if m.SessionDir() != tmpDir {
		t.Errorf("session dir should equal base dir in flat mode, got %q, want %q", m.SessionDir(), tmpDir)
	}

	// Check summary.json was created directly in base dir
	summaryPath := filepath.Join(tmpDir, "summary.json")
	if _, err := os.Stat(summaryPath); os.IsNotExist(err) {
		t.Error("summary.json was not created in base dir")
	}

	// Verify summary content
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("failed to read summary: %v", err)
	}

	var summary Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("failed to parse summary: %v", err)
	}

	if summary.Status != "running" {
		t.Errorf("got status %q, want %q", summary.Status, "running")
	}
	if summary.RunNumber != 1 {
		t.Errorf("got run_number %d, want 1", summary.RunNumber)
	}
	if summary.BranchName != "feature/test-branch" {
		t.Errorf("got branch_name %q, want %q", summary.BranchName, "feature/test-branch")
	}
}

func TestNewManagerWithOptions_LoadExistingSummary(t *testing.T) {
	tmpDir := t.TempDir()

	// Create existing summary
	existingSummary := Summary{
		StartedAt:       time.Now().Add(-time.Hour),
		Status:          "success",
		PhasesCompleted: 3,
		TotalCostUSD:    1.5,
		Sessions:        []SessionEntry{{Phase: 1, SessionID: "old-session"}},
		RunNumber:       2,
		BranchName:      "feature/test-branch",
	}
	data, _ := json.MarshalIndent(existingSummary, "", "  ")
	if err := os.WriteFile(filepath.Join(tmpDir, "summary.json"), data, 0644); err != nil {
		t.Fatalf("failed to write existing summary: %v", err)
	}

	m, err := NewManagerWithOptions(tmpDir, "feature/test-branch", "/tmp/test-project", ManagerOptions{UseSubdirs: false})
	if err != nil {
		t.Fatalf("NewManagerWithOptions failed: %v", err)
	}

	// Run number should be incremented
	summaryPath := filepath.Join(tmpDir, "summary.json")
	newData, _ := os.ReadFile(summaryPath)
	var summary Summary
	if err := json.Unmarshal(newData, &summary); err != nil {
		t.Fatalf("failed to parse summary: %v", err)
	}

	if summary.RunNumber != 3 {
		t.Errorf("got run_number %d, want 3 (incremented from 2)", summary.RunNumber)
	}
	if summary.Status != "running" {
		t.Errorf("got status %q, want %q", summary.Status, "running")
	}
	// Sessions should be preserved
	if len(summary.Sessions) != 1 {
		t.Errorf("got %d sessions, want 1 (preserved)", len(summary.Sessions))
	}

	_ = m // use m to avoid unused variable error
}

func TestNewManagerWithOptions_BranchMismatchWarning(t *testing.T) {
	tmpDir := t.TempDir()

	// Create existing summary with different branch
	existingSummary := Summary{
		StartedAt:  time.Now().Add(-time.Hour),
		Status:     "success",
		RunNumber:  1,
		BranchName: "feature/old-branch",
	}
	data, _ := json.MarshalIndent(existingSummary, "", "  ")
	if err := os.WriteFile(filepath.Join(tmpDir, "summary.json"), data, 0644); err != nil {
		t.Fatalf("failed to write existing summary: %v", err)
	}

	// This should succeed but warn (we can't easily capture log output, but we verify it doesn't fail)
	m, err := NewManagerWithOptions(tmpDir, "feature/new-branch", "/tmp/test-project", ManagerOptions{UseSubdirs: false})
	if err != nil {
		t.Fatalf("NewManagerWithOptions failed: %v", err)
	}

	// Branch name should be updated to current
	summaryPath := filepath.Join(tmpDir, "summary.json")
	newData, _ := os.ReadFile(summaryPath)
	var summary Summary
	if err := json.Unmarshal(newData, &summary); err != nil {
		t.Fatalf("failed to parse summary: %v", err)
	}

	if summary.BranchName != "feature/new-branch" {
		t.Errorf("got branch_name %q, want %q", summary.BranchName, "feature/new-branch")
	}

	_ = m
}

func TestNewManagerWithOptions_MalformedSummary(t *testing.T) {
	tmpDir := t.TempDir()

	// Create malformed summary
	if err := os.WriteFile(filepath.Join(tmpDir, "summary.json"), []byte("{invalid json"), 0644); err != nil {
		t.Fatalf("failed to write malformed summary: %v", err)
	}

	// Should start fresh when summary is malformed
	m, err := NewManagerWithOptions(tmpDir, "feature/test-branch", "/tmp/test-project", ManagerOptions{UseSubdirs: false})
	if err != nil {
		t.Fatalf("NewManagerWithOptions failed: %v", err)
	}

	summaryPath := filepath.Join(tmpDir, "summary.json")
	newData, _ := os.ReadFile(summaryPath)
	var summary Summary
	if err := json.Unmarshal(newData, &summary); err != nil {
		t.Fatalf("failed to parse summary: %v", err)
	}

	if summary.RunNumber != 1 {
		t.Errorf("got run_number %d, want 1 (fresh start after malformed)", summary.RunNumber)
	}

	_ = m
}

func TestNewManagerWithOptions_SubdirMode(t *testing.T) {
	tmpDir := t.TempDir()

	m, err := NewManagerWithOptions(tmpDir, "feature/test-branch", "/tmp/test-project", ManagerOptions{UseSubdirs: true})
	if err != nil {
		t.Fatalf("NewManagerWithOptions failed: %v", err)
	}

	// In subdir mode, session dir should be different from base dir
	if m.SessionDir() == tmpDir {
		t.Error("session dir should differ from base dir in subdir mode")
	}

	// Session dir should contain timestamp pattern
	sessionDir := m.SessionDir()
	if !containsString(sessionDir, "feature-test-branch") {
		t.Error("session dir should contain sanitized branch name")
	}

	// Verify fresh start in subdir mode (always run_number 1)
	summaryPath := filepath.Join(m.SessionDir(), "summary.json")
	data, _ := os.ReadFile(summaryPath)
	var summary Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("failed to parse summary: %v", err)
	}

	if summary.RunNumber != 1 {
		t.Errorf("got run_number %d, want 1 in subdir mode", summary.RunNumber)
	}
}

func TestLoadExistingSummary_Success(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a valid summary file
	existingSummary := Summary{
		StartedAt:       time.Now().Add(-time.Hour),
		Status:          "success",
		PhasesCompleted: 2,
		TotalCostUSD:    0.75,
		Sessions:        []SessionEntry{{Phase: 1, SessionID: "session-1"}, {Phase: 2, SessionID: "session-2"}},
		RunNumber:       1,
		BranchName:      "test-branch",
	}
	data, _ := json.MarshalIndent(existingSummary, "", "  ")
	if err := os.WriteFile(filepath.Join(tmpDir, "summary.json"), data, 0644); err != nil {
		t.Fatalf("failed to write summary: %v", err)
	}

	m := &Manager{sessionDir: tmpDir}
	err := m.loadExistingSummary()
	if err != nil {
		t.Fatalf("loadExistingSummary failed: %v", err)
	}

	if m.summary.RunNumber != 1 {
		t.Errorf("got run_number %d, want 1", m.summary.RunNumber)
	}
	if len(m.summary.Sessions) != 2 {
		t.Errorf("got %d sessions, want 2", len(m.summary.Sessions))
	}
}

func TestLoadExistingSummary_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	m := &Manager{sessionDir: tmpDir}
	err := m.loadExistingSummary()
	if err == nil {
		t.Error("loadExistingSummary should return error for missing file")
	}
}

func TestLoadExistingSummary_MalformedJSON(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a malformed summary file
	if err := os.WriteFile(filepath.Join(tmpDir, "summary.json"), []byte("not valid json{"), 0644); err != nil {
		t.Fatalf("failed to write malformed summary: %v", err)
	}

	m := &Manager{sessionDir: tmpDir}
	err := m.loadExistingSummary()
	if err == nil {
		t.Error("loadExistingSummary should return error for malformed JSON")
	}
}

func TestStartPhase_NewSession(t *testing.T) {
	tmpDir := t.TempDir()

	m, err := NewManagerWithOptions(tmpDir, "test-branch", "/tmp/test-project", ManagerOptions{UseSubdirs: false})
	if err != nil {
		t.Fatalf("NewManagerWithOptions failed: %v", err)
	}

	sessionID, isResume, err := m.StartPhase(1, true)
	if err != nil {
		t.Fatalf("StartPhase failed: %v", err)
	}

	if sessionID == "" {
		t.Error("session ID should not be empty")
	}
	if isResume {
		t.Error("should not be a resume for new session")
	}

	// Verify current_phase was written
	summaryPath := filepath.Join(tmpDir, "summary.json")
	data, _ := os.ReadFile(summaryPath)
	var summary Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("failed to parse summary: %v", err)
	}

	if summary.CurrentPhase == nil {
		t.Fatal("current_phase should be set")
	}
	if summary.CurrentPhase.Phase != 1 {
		t.Errorf("got phase %d, want 1", summary.CurrentPhase.Phase)
	}
	if summary.CurrentPhase.SessionID != sessionID {
		t.Errorf("session ID mismatch: got %q, want %q", summary.CurrentPhase.SessionID, sessionID)
	}
}

func TestStartPhase_ResumeExistingSession(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a manager with existing current_phase
	existingSummary := Summary{
		StartedAt:  time.Now().Add(-time.Hour),
		Status:     "running",
		RunNumber:  1,
		BranchName: "test-branch",
		CurrentPhase: &PhaseState{
			Phase:     1,
			SessionID: "existing-session-123",
			StartedAt: time.Now().Add(-10 * time.Minute),
		},
	}
	data, _ := json.MarshalIndent(existingSummary, "", "  ")
	if err := os.WriteFile(filepath.Join(tmpDir, "summary.json"), data, 0644); err != nil {
		t.Fatalf("failed to write summary: %v", err)
	}

	m, err := NewManagerWithOptions(tmpDir, "test-branch", "/tmp/test-project", ManagerOptions{UseSubdirs: false})
	if err != nil {
		t.Fatalf("NewManagerWithOptions failed: %v", err)
	}

	sessionID, isResume, err := m.StartPhase(1, true)
	if err != nil {
		t.Fatalf("StartPhase failed: %v", err)
	}

	if sessionID != "existing-session-123" {
		t.Errorf("got session ID %q, want 'existing-session-123'", sessionID)
	}
	if !isResume {
		t.Error("should be a resume for existing session")
	}
}

func TestStartPhase_ContinueSessionFalse(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a manager with existing current_phase
	existingSummary := Summary{
		StartedAt:  time.Now().Add(-time.Hour),
		Status:     "running",
		RunNumber:  1,
		BranchName: "test-branch",
		CurrentPhase: &PhaseState{
			Phase:     1,
			SessionID: "existing-session-123",
			StartedAt: time.Now().Add(-10 * time.Minute),
		},
	}
	data, _ := json.MarshalIndent(existingSummary, "", "  ")
	if err := os.WriteFile(filepath.Join(tmpDir, "summary.json"), data, 0644); err != nil {
		t.Fatalf("failed to write summary: %v", err)
	}

	m, err := NewManagerWithOptions(tmpDir, "test-branch", "/tmp/test-project", ManagerOptions{UseSubdirs: false})
	if err != nil {
		t.Fatalf("NewManagerWithOptions failed: %v", err)
	}

	// continueSession = false should start a fresh session
	sessionID, isResume, err := m.StartPhase(1, false)
	if err != nil {
		t.Fatalf("StartPhase failed: %v", err)
	}

	if sessionID == "existing-session-123" {
		t.Error("should have generated a new session ID when continueSession=false")
	}
	if isResume {
		t.Error("should not be a resume when continueSession=false")
	}
}

func TestStartPhase_SummaryWrittenBeforeReturn(t *testing.T) {
	tmpDir := t.TempDir()

	m, err := NewManagerWithOptions(tmpDir, "test-branch", "/tmp/test-project", ManagerOptions{UseSubdirs: false})
	if err != nil {
		t.Fatalf("NewManagerWithOptions failed: %v", err)
	}

	sessionID, _, err := m.StartPhase(1, true)
	if err != nil {
		t.Fatalf("StartPhase failed: %v", err)
	}

	// Verify summary was written to disk (not just in memory)
	summaryPath := filepath.Join(tmpDir, "summary.json")
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("failed to read summary: %v", err)
	}

	var summary Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("failed to parse summary: %v", err)
	}

	if summary.CurrentPhase == nil || summary.CurrentPhase.SessionID != sessionID {
		t.Error("summary on disk should have current_phase with session ID")
	}
}

func TestSetCurrentPhaseSessionID_UpdatesSessionID(t *testing.T) {
	tmpDir := t.TempDir()

	m, err := NewManagerWithOptions(tmpDir, "test-branch", "/tmp/test-project", ManagerOptions{UseSubdirs: false})
	if err != nil {
		t.Fatalf("NewManagerWithOptions failed: %v", err)
	}

	// Start a phase first
	_, _, err = m.StartPhase(1, true)
	if err != nil {
		t.Fatalf("StartPhase failed: %v", err)
	}

	// Update the session ID
	newSessionID := "new-session-456"
	if err := m.SetCurrentPhaseSessionID(newSessionID); err != nil {
		t.Fatalf("SetCurrentPhaseSessionID failed: %v", err)
	}

	// Verify it was updated in memory
	if m.summary.CurrentPhase.SessionID != newSessionID {
		t.Errorf("got session ID %q, want %q", m.summary.CurrentPhase.SessionID, newSessionID)
	}

	// Verify it was written to disk
	summaryPath := filepath.Join(tmpDir, "summary.json")
	data, _ := os.ReadFile(summaryPath)
	var summary Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("failed to parse summary: %v", err)
	}

	if summary.CurrentPhase.SessionID != newSessionID {
		t.Errorf("disk session ID %q, want %q", summary.CurrentPhase.SessionID, newSessionID)
	}
}

func TestSetCurrentPhaseSessionID_NoCurrentPhase(t *testing.T) {
	tmpDir := t.TempDir()

	m, err := NewManagerWithOptions(tmpDir, "test-branch", "/tmp/test-project", ManagerOptions{UseSubdirs: false})
	if err != nil {
		t.Fatalf("NewManagerWithOptions failed: %v", err)
	}

	// Try to set session ID without starting a phase - should be a no-op
	err = m.SetCurrentPhaseSessionID("some-session")
	if err != nil {
		t.Fatalf("SetCurrentPhaseSessionID should not error when no current phase: %v", err)
	}
}

func TestReconcileSessionID_UpdatesWhenDifferent(t *testing.T) {
	tmpDir := t.TempDir()

	m, err := NewManagerWithOptions(tmpDir, "test-branch", "/tmp/test-project", ManagerOptions{UseSubdirs: false})
	if err != nil {
		t.Fatalf("NewManagerWithOptions failed: %v", err)
	}

	// Start a phase
	originalID, _, err := m.StartPhase(1, true)
	if err != nil {
		t.Fatalf("StartPhase failed: %v", err)
	}

	// Reconcile with a different ID
	returnedID := "claude-returned-different-id"
	m.ReconcileSessionID(returnedID)

	// Verify it was updated
	if m.summary.CurrentPhase.SessionID != returnedID {
		t.Errorf("got session ID %q, want %q", m.summary.CurrentPhase.SessionID, returnedID)
	}
	if m.summary.CurrentPhase.SessionID == originalID {
		t.Error("session ID should have been updated")
	}
}

func TestReconcileSessionID_NoOpWhenSame(t *testing.T) {
	tmpDir := t.TempDir()

	m, err := NewManagerWithOptions(tmpDir, "test-branch", "/tmp/test-project", ManagerOptions{UseSubdirs: false})
	if err != nil {
		t.Fatalf("NewManagerWithOptions failed: %v", err)
	}

	// Start a phase
	originalID, _, err := m.StartPhase(1, true)
	if err != nil {
		t.Fatalf("StartPhase failed: %v", err)
	}

	// Reconcile with the same ID (should be a no-op)
	m.ReconcileSessionID(originalID)

	// Verify it's still the same
	if m.summary.CurrentPhase.SessionID != originalID {
		t.Errorf("session ID should not have changed, got %q, want %q", m.summary.CurrentPhase.SessionID, originalID)
	}
}

func TestCompletePhase_ClearsCurrentPhase(t *testing.T) {
	tmpDir := t.TempDir()

	m, err := NewManagerWithOptions(tmpDir, "test-branch", "/tmp/test-project", ManagerOptions{UseSubdirs: false})
	if err != nil {
		t.Fatalf("NewManagerWithOptions failed: %v", err)
	}

	// Start a phase
	_, _, err = m.StartPhase(1, true)
	if err != nil {
		t.Fatalf("StartPhase failed: %v", err)
	}

	// Verify current_phase is set
	if m.summary.CurrentPhase == nil {
		t.Fatal("current_phase should be set before CompletePhase")
	}

	// Complete the phase
	if err := m.CompletePhase(); err != nil {
		t.Fatalf("CompletePhase failed: %v", err)
	}

	// Verify current_phase is cleared in memory
	if m.summary.CurrentPhase != nil {
		t.Error("current_phase should be nil after CompletePhase")
	}

	// Verify current_phase is cleared on disk
	summaryPath := filepath.Join(tmpDir, "summary.json")
	data, _ := os.ReadFile(summaryPath)
	var summary Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("failed to parse summary: %v", err)
	}

	if summary.CurrentPhase != nil {
		t.Error("current_phase on disk should be nil after CompletePhase")
	}
}

func TestCompletePhase_WritesSummary(t *testing.T) {
	tmpDir := t.TempDir()

	m, err := NewManagerWithOptions(tmpDir, "test-branch", "/tmp/test-project", ManagerOptions{UseSubdirs: false})
	if err != nil {
		t.Fatalf("NewManagerWithOptions failed: %v", err)
	}

	// Start and complete a phase
	_, _, _ = m.StartPhase(1, true)
	if err := m.CompletePhase(); err != nil {
		t.Fatalf("CompletePhase failed: %v", err)
	}

	// Read and verify summary was written
	summaryPath := filepath.Join(tmpDir, "summary.json")
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("failed to read summary: %v", err)
	}

	var summary Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("failed to parse summary: %v", err)
	}

	// Verify the summary was updated
	if summary.Status != "running" {
		t.Errorf("status should still be 'running', got %q", summary.Status)
	}
}

func TestStartPostCompletion_NewSession(t *testing.T) {
	tmpDir := t.TempDir()

	m, err := NewManagerWithOptions(tmpDir, "test-branch", "/tmp/test-project", ManagerOptions{UseSubdirs: false})
	if err != nil {
		t.Fatalf("NewManagerWithOptions failed: %v", err)
	}

	sessionID, isResume, err := m.StartPostCompletion(true)
	if err != nil {
		t.Fatalf("StartPostCompletion failed: %v", err)
	}

	if sessionID == "" {
		t.Error("session ID should not be empty")
	}
	if isResume {
		t.Error("should not be a resume for new post-completion session")
	}

	// Verify post_completion was written
	summaryPath := filepath.Join(tmpDir, "summary.json")
	data, _ := os.ReadFile(summaryPath)
	var summary Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("failed to parse summary: %v", err)
	}

	if summary.PostCompletion == nil {
		t.Fatal("post_completion should be set")
	}
	if summary.PostCompletion.SessionID != sessionID {
		t.Errorf("session ID mismatch: got %q, want %q", summary.PostCompletion.SessionID, sessionID)
	}
}

func TestStartPostCompletion_ResumeExisting(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a manager with existing post_completion
	existingSummary := Summary{
		StartedAt:  time.Now().Add(-time.Hour),
		Status:     "running",
		RunNumber:  1,
		BranchName: "test-branch",
		PostCompletion: &PostCompletionState{
			SessionID: "existing-post-completion-123",
			StartedAt: time.Now().Add(-10 * time.Minute),
		},
	}
	data, _ := json.MarshalIndent(existingSummary, "", "  ")
	if err := os.WriteFile(filepath.Join(tmpDir, "summary.json"), data, 0644); err != nil {
		t.Fatalf("failed to write summary: %v", err)
	}

	m, err := NewManagerWithOptions(tmpDir, "test-branch", "/tmp/test-project", ManagerOptions{UseSubdirs: false})
	if err != nil {
		t.Fatalf("NewManagerWithOptions failed: %v", err)
	}

	sessionID, isResume, err := m.StartPostCompletion(true)
	if err != nil {
		t.Fatalf("StartPostCompletion failed: %v", err)
	}

	if sessionID != "existing-post-completion-123" {
		t.Errorf("got session ID %q, want 'existing-post-completion-123'", sessionID)
	}
	if !isResume {
		t.Error("should be a resume for existing post-completion session")
	}
}

func TestCompletePostCompletion_ClearsState(t *testing.T) {
	tmpDir := t.TempDir()

	m, err := NewManagerWithOptions(tmpDir, "test-branch", "/tmp/test-project", ManagerOptions{UseSubdirs: false})
	if err != nil {
		t.Fatalf("NewManagerWithOptions failed: %v", err)
	}

	// Start a post-completion session
	_, _, err = m.StartPostCompletion(true)
	if err != nil {
		t.Fatalf("StartPostCompletion failed: %v", err)
	}

	// Verify post_completion is set
	if m.summary.PostCompletion == nil {
		t.Fatal("post_completion should be set before CompletePostCompletion")
	}

	// Complete the post-completion
	if err := m.CompletePostCompletion(); err != nil {
		t.Fatalf("CompletePostCompletion failed: %v", err)
	}

	// Verify post_completion is cleared in memory
	if m.summary.PostCompletion != nil {
		t.Error("post_completion should be nil after CompletePostCompletion")
	}

	// Verify post_completion is cleared on disk
	summaryPath := filepath.Join(tmpDir, "summary.json")
	data, _ := os.ReadFile(summaryPath)
	var summary Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("failed to parse summary: %v", err)
	}

	if summary.PostCompletion != nil {
		t.Error("post_completion on disk should be nil after CompletePostCompletion")
	}
}

func TestPhaseFileName_RunNumberGreaterThanOne(t *testing.T) {
	tmpDir := t.TempDir()

	// Create existing summary with run number 2
	existingSummary := Summary{
		StartedAt:  time.Now().Add(-time.Hour),
		Status:     "success",
		RunNumber:  2,
		BranchName: "test-branch",
	}
	data, _ := json.MarshalIndent(existingSummary, "", "  ")
	if err := os.WriteFile(filepath.Join(tmpDir, "summary.json"), data, 0644); err != nil {
		t.Fatalf("failed to write summary: %v", err)
	}

	m, err := NewManagerWithOptions(tmpDir, "test-branch", "/tmp/test-project", ManagerOptions{UseSubdirs: false})
	if err != nil {
		t.Fatalf("NewManagerWithOptions failed: %v", err)
	}

	// Run number should now be 3
	fileName := m.phaseFileName(1, "session.json")
	expected := "phase-1-run-3-session.json"
	if fileName != expected {
		t.Errorf("got filename %q, want %q", fileName, expected)
	}
}

func TestPhaseFileName_RunNumberOne(t *testing.T) {
	tmpDir := t.TempDir()

	m, err := NewManagerWithOptions(tmpDir, "test-branch", "/tmp/test-project", ManagerOptions{UseSubdirs: false})
	if err != nil {
		t.Fatalf("NewManagerWithOptions failed: %v", err)
	}

	// Run number is 1, should use standard filename
	fileName := m.phaseFileName(1, "session.json")
	expected := "phase-1-session.json"
	if fileName != expected {
		t.Errorf("got filename %q, want %q", fileName, expected)
	}
}

func TestPhaseFileName_WithSubdirs(t *testing.T) {
	tmpDir := t.TempDir()

	m, err := NewManagerWithOptions(tmpDir, "test-branch", "/tmp/test-project", ManagerOptions{UseSubdirs: true})
	if err != nil {
		t.Fatalf("NewManagerWithOptions failed: %v", err)
	}

	// In subdir mode, always use standard filename
	fileName := m.phaseFileName(1, "session.json")
	expected := "phase-1-session.json"
	if fileName != expected {
		t.Errorf("got filename %q, want %q", fileName, expected)
	}
}

func TestPostCompletionFileName_RunNumberGreaterThanOne(t *testing.T) {
	tmpDir := t.TempDir()

	// Create existing summary with run number 2
	existingSummary := Summary{
		StartedAt:  time.Now().Add(-time.Hour),
		Status:     "success",
		RunNumber:  2,
		BranchName: "test-branch",
	}
	data, _ := json.MarshalIndent(existingSummary, "", "  ")
	if err := os.WriteFile(filepath.Join(tmpDir, "summary.json"), data, 0644); err != nil {
		t.Fatalf("failed to write summary: %v", err)
	}

	m, err := NewManagerWithOptions(tmpDir, "test-branch", "/tmp/test-project", ManagerOptions{UseSubdirs: false})
	if err != nil {
		t.Fatalf("NewManagerWithOptions failed: %v", err)
	}

	// Run number should now be 3
	fileName := m.postCompletionFileName()
	expected := "post-completion-run-3-session"
	if fileName != expected {
		t.Errorf("got filename %q, want %q", fileName, expected)
	}
}

func TestPostCompletionFileName_RunNumberOne(t *testing.T) {
	tmpDir := t.TempDir()

	m, err := NewManagerWithOptions(tmpDir, "test-branch", "/tmp/test-project", ManagerOptions{UseSubdirs: false})
	if err != nil {
		t.Fatalf("NewManagerWithOptions failed: %v", err)
	}

	// Run number is 1, should use standard filename
	fileName := m.postCompletionFileName()
	expected := "post-completion-session"
	if fileName != expected {
		t.Errorf("got filename %q, want %q", fileName, expected)
	}
}

func TestPostCompletionFileName_WithSubdirs(t *testing.T) {
	tmpDir := t.TempDir()

	m, err := NewManagerWithOptions(tmpDir, "test-branch", "/tmp/test-project", ManagerOptions{UseSubdirs: true})
	if err != nil {
		t.Fatalf("NewManagerWithOptions failed: %v", err)
	}

	// In subdir mode, always use standard filename
	fileName := m.postCompletionFileName()
	expected := "post-completion-session"
	if fileName != expected {
		t.Errorf("got filename %q, want %q", fileName, expected)
	}
}

func TestSortedPhaseMap(t *testing.T) {
	sessions := []SessionEntry{
		{Phase: 3, SessionID: "session-3"},
		{Phase: 1, SessionID: "session-1a"},
		{Phase: 1, SessionID: "session-1b"},
		{Phase: 0, SessionID: "post-completion"}, // Phase 0 should be excluded
		{Phase: 2, SessionID: "session-2"},
	}

	phaseMap, phases := sortedPhaseMap(sessions)

	// Check phases are sorted
	if len(phases) != 3 {
		t.Errorf("got %d phases, want 3", len(phases))
	}
	if phases[0] != 1 || phases[1] != 2 || phases[2] != 3 {
		t.Errorf("phases not sorted correctly: got %v, want [1 2 3]", phases)
	}

	// Check phase 0 is excluded
	if _, found := phaseMap[0]; found {
		t.Error("phase 0 should be excluded from phaseMap")
	}

	// Check phase 1 has 2 sessions
	if len(phaseMap[1]) != 2 {
		t.Errorf("phase 1 should have 2 sessions, got %d", len(phaseMap[1]))
	}
}

func TestSortedPhaseMap_EmptySessions(t *testing.T) {
	phaseMap, phases := sortedPhaseMap([]SessionEntry{})

	if len(phases) != 0 {
		t.Errorf("got %d phases, want 0", len(phases))
	}
	if len(phaseMap) != 0 {
		t.Errorf("got %d entries in phaseMap, want 0", len(phaseMap))
	}
}

func TestWriteRunIndex_CreatesFiles(t *testing.T) {
	tmpDir := t.TempDir()

	m, err := NewManager(tmpDir, "test-branch", "/tmp/test-project")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Add some sessions
	result := &claude.SessionResult{
		SessionID: "test-session-123",
		Cost:      0.15,
		Duration:  45 * time.Second,
		NumTurns:  5,
		Output:    "Test output",
		IsError:   false,
		RawJSON:   []byte(`{"session_id": "test-session-123"}`),
	}
	startTime := time.Now().Add(-45 * time.Second)
	if err := m.SaveSession(1, result, startTime); err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	// Complete the run (which should write the index files)
	if err := m.Complete(); err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	// Check index.md was created
	mdPath := filepath.Join(m.SessionDir(), "index.md")
	if _, err := os.Stat(mdPath); os.IsNotExist(err) {
		t.Error("index.md was not created")
	}

	// Check index.html was created
	htmlPath := filepath.Join(m.SessionDir(), "index.html")
	if _, err := os.Stat(htmlPath); os.IsNotExist(err) {
		t.Error("index.html was not created")
	}
}

func TestWriteRunIndex_OnFail(t *testing.T) {
	tmpDir := t.TempDir()

	m, err := NewManager(tmpDir, "test-branch", "/tmp/test-project")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Fail the run (which should also write the index files)
	testErr := os.ErrNotExist
	if err := m.Fail(testErr); err != nil {
		t.Fatalf("Fail failed: %v", err)
	}

	// Check index.md was created even on failure
	mdPath := filepath.Join(m.SessionDir(), "index.md")
	if _, err := os.Stat(mdPath); os.IsNotExist(err) {
		t.Error("index.md was not created on Fail")
	}

	// Check index.html was created even on failure
	htmlPath := filepath.Join(m.SessionDir(), "index.html")
	if _, err := os.Stat(htmlPath); os.IsNotExist(err) {
		t.Error("index.html was not created on Fail")
	}
}

func TestGenerateMarkdownIndex_Content(t *testing.T) {
	tmpDir := t.TempDir()

	m, err := NewManager(tmpDir, "feature/test-branch", "/tmp/test-project")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Add a session
	m.summary.Sessions = []SessionEntry{
		{
			Phase:      1,
			SessionID:  "session-1",
			DurationMS: 45000,
			CostUSD:    0.15,
			NumTurns:   5,
			RunNumber:  1,
		},
	}
	m.summary.PhasesCompleted = 1
	m.summary.TotalCostUSD = 0.15
	now := time.Now()
	m.summary.CompletedAt = &now

	markdown := m.generateMarkdownIndex()

	// Check key elements are present
	if !containsString(markdown, "# Orbit Run Summary") {
		t.Error("markdown should contain title")
	}
	if !containsString(markdown, "feature/test-branch") {
		t.Error("markdown should contain branch name")
	}
	if !containsString(markdown, "### Phase 1") {
		t.Error("markdown should contain phase heading")
	}
	if !containsString(markdown, "phase-1-transcript.md") {
		t.Error("markdown should contain link to transcript.md")
	}
	if !containsString(markdown, "phase-1-transcript.html") {
		t.Error("markdown should contain link to transcript.html")
	}
}

func TestGenerateHTMLIndex_Content(t *testing.T) {
	tmpDir := t.TempDir()

	m, err := NewManager(tmpDir, "feature/test-branch", "/tmp/test-project")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Add a session
	m.summary.Sessions = []SessionEntry{
		{
			Phase:      1,
			SessionID:  "session-1",
			DurationMS: 45000,
			CostUSD:    0.15,
			NumTurns:   5,
			RunNumber:  1,
		},
	}
	m.summary.PhasesCompleted = 1
	m.summary.TotalCostUSD = 0.15

	html := m.generateHTMLIndex()

	// Check key elements are present
	if !containsString(html, "<!DOCTYPE html>") {
		t.Error("HTML should contain doctype")
	}
	if !containsString(html, "<title>Orbit Run Summary</title>") {
		t.Error("HTML should contain title")
	}
	if !containsString(html, "feature/test-branch") {
		t.Error("HTML should contain branch name")
	}
	if !containsString(html, "Phase 1") {
		t.Error("HTML should contain phase heading")
	}
	if !containsString(html, "phase-1-transcript.html") {
		t.Error("HTML should contain link to transcript.html")
	}
}

func TestGenerateHTMLIndex_EscapesUserContent(t *testing.T) {
	tmpDir := t.TempDir()

	m, err := NewManager(tmpDir, "test-branch", "/tmp/test-project")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Set potentially dangerous content
	m.summary.BranchName = "<script>alert('xss')</script>"
	m.summary.Error = "<img src=x onerror=alert('xss')>"
	m.summary.Status = "failed"

	html := m.generateHTMLIndex()

	// Check that content is escaped
	if containsString(html, "<script>alert") {
		t.Error("branch name should be HTML escaped")
	}
	if containsString(html, "<img src=x") {
		t.Error("error message should be HTML escaped")
	}
	// Escaped versions should be present
	if !containsString(html, "&lt;script&gt;") {
		t.Error("HTML should contain escaped branch name")
	}
	if !containsString(html, "&lt;img") {
		t.Error("HTML should contain escaped error message")
	}
}

func TestGenerateMarkdownIndex_WithPostCompletion(t *testing.T) {
	tmpDir := t.TempDir()

	m, err := NewManager(tmpDir, "test-branch", "/tmp/test-project")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Add sessions including post-completion (phase 0)
	m.summary.Sessions = []SessionEntry{
		{Phase: 1, SessionID: "session-1", RunNumber: 1, CostUSD: 0.10},
		{Phase: 0, SessionID: "post-session", RunNumber: 1, CostUSD: 0.05}, // Post-completion
	}

	markdown := m.generateMarkdownIndex()

	if !containsString(markdown, "### Post-Completion") {
		t.Error("markdown should contain post-completion section")
	}
	if !containsString(markdown, "post-completion-session-transcript.md") {
		t.Error("markdown should contain link to post-completion transcript")
	}
}

func TestGenerateMarkdownIndex_WithMultipleRuns(t *testing.T) {
	tmpDir := t.TempDir()

	m, err := NewManager(tmpDir, "test-branch", "/tmp/test-project")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Add sessions from multiple runs of the same phase
	m.summary.Sessions = []SessionEntry{
		{Phase: 1, SessionID: "session-1a", RunNumber: 1, CostUSD: 0.10, IsError: true},
		{Phase: 1, SessionID: "session-1b", RunNumber: 2, CostUSD: 0.15},
	}

	markdown := m.generateMarkdownIndex()

	if !containsString(markdown, "(Run 1)") {
		t.Error("markdown should indicate run 1")
	}
	if !containsString(markdown, "(Run 2)") {
		t.Error("markdown should indicate run 2")
	}
}
