package sessions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents/claudecode"
)

// newTestLister creates a Lister with a custom homeDir for testing.
func newTestLister(homeDir string) *Lister {
	return &Lister{homeDir: homeDir}
}

// setupClaudeSession creates a mock Claude session file in the temp directory.
// Returns the project path that should be used with the lister.
func setupClaudeSession(t *testing.T, homeDir, projectPath, sessionID string, createdAt time.Time) {
	t.Helper()
	claudeProjectPath := claudecode.BuildProjectPath(projectPath)
	dir := filepath.Join(homeDir, ".claude", "projects", claudeProjectPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}

	// Write a minimal JSONL file with a timestamp on the first line
	entry := map[string]any{
		"type":      "system",
		"timestamp": createdAt.Format(time.RFC3339),
		"message":   "test",
	}
	data, _ := json.Marshal(entry)
	filePath := filepath.Join(dir, sessionID+".jsonl")
	if err := os.WriteFile(filePath, append(data, '\n'), 0644); err != nil {
		t.Fatalf("failed to write session file: %v", err)
	}
}

// setupCodexSession creates a mock Codex session file.
func setupCodexSession(t *testing.T, homeDir, projectPath, sessionID string, createdAt time.Time) {
	t.Helper()
	// Codex sessions are in ~/.codex/sessions/YYYY/MM/DD/session-{uuid}.jsonl
	dir := filepath.Join(homeDir, ".codex", "sessions",
		createdAt.Format("2006"), createdAt.Format("01"), createdAt.Format("02"))
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}

	meta := map[string]any{
		"type":      "session_meta",
		"timestamp": createdAt.Format(time.RFC3339),
		"payload": map[string]any{
			"id":  sessionID,
			"cwd": projectPath,
		},
	}
	data, _ := json.Marshal(meta)
	filePath := filepath.Join(dir, fmt.Sprintf("session-%s.jsonl", sessionID))
	if err := os.WriteFile(filePath, append(data, '\n'), 0644); err != nil {
		t.Fatalf("failed to write session file: %v", err)
	}
}

func TestListAllNoSessions(t *testing.T) {
	homeDir := t.TempDir()
	projectPath := t.TempDir()

	lister := newTestLister(homeDir)
	sessions, warnings, err := lister.ListAll(projectPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}

	// Kiro CLI and Kiro IDE may produce warnings if they can't find databases,
	// but other sources should not produce warnings for missing directories
	for _, w := range warnings {
		if w.Source == SourceClaude || w.Source == SourceCodex || w.Source == SourceCopilot {
			t.Errorf("unexpected warning for %s: %v", w.Source, w.Err)
		}
	}
}

func TestListAllClaudeSessions(t *testing.T) {
	homeDir := t.TempDir()
	projectPath := t.TempDir()

	t1 := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 1, 16, 10, 0, 0, 0, time.UTC)

	setupClaudeSession(t, homeDir, projectPath, "session-001", t1)
	setupClaudeSession(t, homeDir, projectPath, "session-002", t2)

	lister := newTestLister(homeDir)
	sessions, _, err := lister.ListAll(projectPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Filter to Claude sessions only (other sources may or may not return results)
	var claudeSessions []SessionInfo
	for _, s := range sessions {
		if s.Source == SourceClaude {
			claudeSessions = append(claudeSessions, s)
		}
	}

	if len(claudeSessions) != 2 {
		t.Fatalf("expected 2 Claude sessions, got %d", len(claudeSessions))
	}

	// Verify oldest-first sort
	if claudeSessions[0].ID != "session-001" {
		t.Errorf("expected first session to be session-001, got %s", claudeSessions[0].ID)
	}
	if claudeSessions[1].ID != "session-002" {
		t.Errorf("expected second session to be session-002, got %s", claudeSessions[1].ID)
	}
}

func TestListAllMultipleSources(t *testing.T) {
	homeDir := t.TempDir()
	projectPath := t.TempDir()

	t1 := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 1, 14, 10, 0, 0, 0, time.UTC)

	setupClaudeSession(t, homeDir, projectPath, "claude-session", t1)
	setupCodexSession(t, homeDir, projectPath, "00000000-0000-0000-0000-000000000001", t2)

	lister := newTestLister(homeDir)
	sessions, _, err := lister.ListAll(projectPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Filter to only Claude and Codex
	var filtered []SessionInfo
	for _, s := range sessions {
		if s.Source == SourceClaude || s.Source == SourceCodex {
			filtered = append(filtered, s)
		}
	}

	if len(filtered) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(filtered))
	}

	// Codex session (t2) is older, should be first
	if filtered[0].Source != SourceCodex {
		t.Errorf("expected first session to be codex (older), got %s", filtered[0].Source)
	}
	if filtered[1].Source != SourceClaude {
		t.Errorf("expected second session to be claude (newer), got %s", filtered[1].Source)
	}
}

func TestSortSessionsByTimestamp(t *testing.T) {
	now := time.Now()
	sessions := []SessionInfo{
		{ID: "3", CreatedAt: now.Add(2 * time.Hour), Source: SourceCodex},
		{ID: "1", CreatedAt: now, Source: SourceClaude},
		{ID: "2", CreatedAt: now.Add(time.Hour), Source: SourceCopilot},
	}

	sortSessionsByTimestamp(sessions)

	if sessions[0].ID != "1" || sessions[1].ID != "2" || sessions[2].ID != "3" {
		t.Errorf("expected oldest-first sort, got: %s, %s, %s",
			sessions[0].ID, sessions[1].ID, sessions[2].ID)
	}
}

func TestSortSessionsByTimestampTieBreaking(t *testing.T) {
	now := time.Now()
	sessions := []SessionInfo{
		{ID: "codex", CreatedAt: now, Source: SourceCodex},
		{ID: "claude", CreatedAt: now, Source: SourceClaude},
		{ID: "copilot", CreatedAt: now, Source: SourceCopilot},
	}

	sortSessionsByTimestamp(sessions)

	// Priority: claude=0, copilot=1, codex=2
	if sessions[0].Source != SourceClaude {
		t.Errorf("expected claude first in tie, got %s", sessions[0].Source)
	}
	if sessions[1].Source != SourceCopilot {
		t.Errorf("expected copilot second in tie, got %s", sessions[1].Source)
	}
	if sessions[2].Source != SourceCodex {
		t.Errorf("expected codex third in tie, got %s", sessions[2].Source)
	}
}
