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

func TestFormatUserMessage(t *testing.T) {
	entry := &transcriptEntry{
		Type: "user",
		Message: &transcriptMsg{
			Role: "user",
			Content: []contentItem{
				{Type: "text", Text: "Hello, can you help me?"},
			},
		},
	}

	result := formatUserMessage(entry)

	if !containsString(result, "👤 User") {
		t.Error("should contain user header")
	}
	if !containsString(result, "Hello, can you help me?") {
		t.Error("should contain user message")
	}
}

func TestFormatAssistantMessage(t *testing.T) {
	entry := &transcriptEntry{
		Type: "assistant",
		Message: &transcriptMsg{
			Role: "assistant",
			Content: []contentItem{
				{Type: "thinking", Thinking: "Let me analyze this request."},
				{Type: "text", Text: "Sure, I can help with that."},
				{Type: "tool_use", Name: "Read", Input: map[string]string{"file_path": "/test/file.go"}},
			},
		},
	}

	result := formatAssistantMessage(entry)

	if !containsString(result, "🤖 Assistant") {
		t.Error("should contain assistant header")
	}
	if !containsString(result, "💭 Thinking") {
		t.Error("should contain thinking section")
	}
	if !containsString(result, "Let me analyze this request.") {
		t.Error("should contain thinking content")
	}
	if !containsString(result, "Sure, I can help with that.") {
		t.Error("should contain text response")
	}
	if !containsString(result, "🔧 Tool: `Read`") {
		t.Error("should contain tool use header")
	}
}

func TestFormatAssistantMessage_ToolResult(t *testing.T) {
	entry := &transcriptEntry{
		Type: "assistant",
		Message: &transcriptMsg{
			Role: "assistant",
			Content: []contentItem{
				{Type: "tool_result", Content: "file contents here", IsError: false},
			},
		},
	}

	result := formatAssistantMessage(entry)

	if !containsString(result, "✅ Tool Result") {
		t.Error("should contain success tool result header")
	}
	if !containsString(result, "file contents here") {
		t.Error("should contain tool result content")
	}
}

func TestFormatAssistantMessage_ToolError(t *testing.T) {
	entry := &transcriptEntry{
		Type: "assistant",
		Message: &transcriptMsg{
			Role: "assistant",
			Content: []contentItem{
				{Type: "tool_result", Content: "file not found", IsError: true},
			},
		},
	}

	result := formatAssistantMessage(entry)

	if !containsString(result, "❌ Tool Error") {
		t.Error("should contain error tool result header")
	}
}

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
