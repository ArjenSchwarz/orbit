package sessions

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents/claudecode"
	"github.com/arjenschwarz/orbit/internal/agents/kiro/logs"
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

// setupCopilotSession creates a mock Copilot session directory.
func setupCopilotSession(t *testing.T, homeDir, projectPath, sessionID string, createdAt time.Time) {
	t.Helper()
	sessionDir := filepath.Join(homeDir, ".copilot", "session-state", sessionID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("failed to create session dir: %v", err)
	}

	eventsData := `{"type":"event","data":"test"}` + "\n"
	if err := os.WriteFile(filepath.Join(sessionDir, "events.jsonl"), []byte(eventsData), 0644); err != nil {
		t.Fatalf("failed to write events file: %v", err)
	}

	yamlContent := fmt.Sprintf("id: %s\ncwd: %s\ngit_root: %s\ncreated_at: %s\n",
		sessionID, projectPath, projectPath, createdAt.Format(time.RFC3339))
	if err := os.WriteFile(filepath.Join(sessionDir, "workspace.yaml"), []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write workspace file: %v", err)
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

func TestListAllCopilotSessions(t *testing.T) {
	homeDir := t.TempDir()
	projectPath := t.TempDir()

	t1 := time.Date(2025, 1, 15, 14, 30, 0, 0, time.UTC)
	sessionID := "12345678-1234-1234-1234-123456789abc"

	setupCopilotSession(t, homeDir, projectPath, sessionID, t1)

	lister := newTestLister(homeDir)
	sessions, _, err := lister.ListAll(projectPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var copilotSessions []SessionInfo
	for _, s := range sessions {
		if s.Source == SourceCopilot {
			copilotSessions = append(copilotSessions, s)
		}
	}

	if len(copilotSessions) != 1 {
		t.Fatalf("expected 1 Copilot session, got %d", len(copilotSessions))
	}

	if copilotSessions[0].ID != sessionID {
		t.Errorf("session.ID = %q, want %q", copilotSessions[0].ID, sessionID)
	}
	if copilotSessions[0].Source != SourceCopilot {
		t.Errorf("session.Source = %q, want %q", copilotSessions[0].Source, SourceCopilot)
	}
}

func TestListCopilotNormalizesPathsForComparison(t *testing.T) {
	homeDir := t.TempDir()

	// realDir is the actual project directory on disk.
	realDir := t.TempDir()

	// Create a symlink that points to realDir. On macOS t.TempDir() itself
	// may go through /var -> /private/var, so EvalSymlinks gives us the
	// canonical target we store in the workspace.yaml.
	symlinkDir := filepath.Join(t.TempDir(), "link-to-project")
	realTarget, err := filepath.EvalSymlinks(realDir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", realDir, err)
	}
	if err := os.Symlink(realTarget, symlinkDir); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	t1 := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	sessionID := "symlink-copilot-session"

	// Setup stores the real (resolved) path inside workspace.yaml.
	setupCopilotSession(t, homeDir, realTarget, sessionID, t1)

	lister := newTestLister(homeDir)

	// Query via the symlink path — before the fix this returned 0 sessions
	// because listCopilot compared paths with plain string equality.
	sessions, _, err := lister.ListAll(symlinkDir)
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}

	var copilotSessions []SessionInfo
	for _, s := range sessions {
		if s.Source == SourceCopilot {
			copilotSessions = append(copilotSessions, s)
		}
	}

	if len(copilotSessions) != 1 {
		t.Fatalf("expected 1 Copilot session when querying via symlink, got %d", len(copilotSessions))
	}
	if copilotSessions[0].ID != sessionID {
		t.Errorf("session.ID = %q, want %q", copilotSessions[0].ID, sessionID)
	}
}

func TestListAllCodexSessions(t *testing.T) {
	homeDir := t.TempDir()
	projectPath := t.TempDir()

	t1 := time.Date(2025, 1, 15, 14, 30, 0, 0, time.UTC)
	sessionID := "12345678-1234-1234-1234-123456789abc"

	setupCodexSession(t, homeDir, projectPath, sessionID, t1)

	lister := newTestLister(homeDir)
	sessions, _, err := lister.ListAll(projectPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var codexSessions []SessionInfo
	for _, s := range sessions {
		if s.Source == SourceCodex {
			codexSessions = append(codexSessions, s)
		}
	}

	if len(codexSessions) != 1 {
		t.Fatalf("expected 1 Codex session, got %d", len(codexSessions))
	}

	if codexSessions[0].ID != sessionID {
		t.Errorf("session.ID = %q, want %q", codexSessions[0].ID, sessionID)
	}
	if codexSessions[0].Source != SourceCodex {
		t.Errorf("session.Source = %q, want %q", codexSessions[0].Source, SourceCodex)
	}
}

func TestListAllSortOrder(t *testing.T) {
	homeDir := t.TempDir()
	projectPath := t.TempDir()

	// Create sessions with different timestamps out of order
	t3 := time.Date(2025, 1, 17, 10, 0, 0, 0, time.UTC) // Newest
	t1 := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC) // Oldest
	t2 := time.Date(2025, 1, 16, 10, 0, 0, 0, time.UTC) // Middle

	setupClaudeSession(t, homeDir, projectPath, "session-3", t3)
	setupClaudeSession(t, homeDir, projectPath, "session-1", t1)
	setupClaudeSession(t, homeDir, projectPath, "session-2", t2)

	lister := newTestLister(homeDir)
	sessions, _, err := lister.ListAll(projectPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var claudeSessions []SessionInfo
	for _, s := range sessions {
		if s.Source == SourceClaude {
			claudeSessions = append(claudeSessions, s)
		}
	}

	if len(claudeSessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(claudeSessions))
	}

	// Verify oldest-first order
	if claudeSessions[0].ID != "session-1" {
		t.Errorf("sessions[0].ID = %q, want session-1", claudeSessions[0].ID)
	}
	if claudeSessions[1].ID != "session-2" {
		t.Errorf("sessions[1].ID = %q, want session-2", claudeSessions[1].ID)
	}
	if claudeSessions[2].ID != "session-3" {
		t.Errorf("sessions[2].ID = %q, want session-3", claudeSessions[2].ID)
	}
}

// TestListAllClaudeSessionsEmptyProjectPath verifies that ListAll returns Claude
// sessions when projectPath is empty (no project filtering). This is a regression
// test for T-146: when projectPath is empty, BuildProjectPath("") returns "",
// causing listClaude to look in the wrong directory (~/.claude/projects/ root
// instead of iterating project subdirectories).
func TestListAllClaudeSessionsEmptyProjectPath(t *testing.T) {
	homeDir := t.TempDir()

	// Create sessions under two different project paths
	t1 := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 1, 16, 10, 0, 0, 0, time.UTC)

	setupClaudeSession(t, homeDir, "/projects/alpha", "session-alpha", t1)
	setupClaudeSession(t, homeDir, "/projects/beta", "session-beta", t2)

	lister := newTestLister(homeDir)

	// Empty projectPath should return ALL Claude sessions across all projects
	sessions, _, err := lister.ListAll("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var claudeSessions []SessionInfo
	for _, s := range sessions {
		if s.Source == SourceClaude {
			claudeSessions = append(claudeSessions, s)
		}
	}

	if len(claudeSessions) != 2 {
		t.Fatalf("expected 2 Claude sessions across all projects, got %d", len(claudeSessions))
	}

	// Verify both sessions are present (oldest-first sort)
	ids := map[string]bool{}
	for _, s := range claudeSessions {
		ids[s.ID] = true
	}
	if !ids["session-alpha"] {
		t.Error("missing session-alpha from project /projects/alpha")
	}
	if !ids["session-beta"] {
		t.Error("missing session-beta from project /projects/beta")
	}
}

// TestListAllCodexSessionsEmptyProjectPath verifies that ListAll returns Codex
// sessions when projectPath is empty (no project filtering).
func TestListAllCodexSessionsEmptyProjectPath(t *testing.T) {
	homeDir := t.TempDir()

	t1 := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	sessionID := "00000000-0000-0000-0000-000000000042"

	// Create a Codex session associated with a specific project path
	setupCodexSession(t, homeDir, "/some/project", sessionID, t1)

	lister := newTestLister(homeDir)

	// Empty projectPath should return all Codex sessions regardless of their cwd
	sessions, _, err := lister.ListAll("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var codexSessions []SessionInfo
	for _, s := range sessions {
		if s.Source == SourceCodex {
			codexSessions = append(codexSessions, s)
		}
	}

	if len(codexSessions) != 1 {
		t.Fatalf("expected 1 Codex session with empty projectPath, got %d", len(codexSessions))
	}

	if codexSessions[0].ID != sessionID {
		t.Errorf("session.ID = %q, want %q", codexSessions[0].ID, sessionID)
	}
}

// TestListAllCopilotSessionsEmptyProjectPath verifies that ListAll returns Copilot
// sessions when projectPath is empty (no project filtering). This is a regression
// test for T-374: when projectPath is empty, listCopilot incorrectly compares
// session paths against an empty string, filtering out all sessions that have a
// non-empty git_root or cwd.
func TestListAllCopilotSessionsEmptyProjectPath(t *testing.T) {
	homeDir := t.TempDir()

	t1 := time.Date(2025, 1, 15, 14, 30, 0, 0, time.UTC)
	t2 := time.Date(2025, 1, 16, 10, 0, 0, 0, time.UTC)
	sessionID1 := "aaaaaaaa-1111-2222-3333-444444444444"
	sessionID2 := "bbbbbbbb-5555-6666-7777-888888888888"

	// Create two sessions associated with different project paths
	setupCopilotSession(t, homeDir, "/projects/alpha", sessionID1, t1)
	setupCopilotSession(t, homeDir, "/projects/beta", sessionID2, t2)

	lister := newTestLister(homeDir)

	// Empty projectPath should return ALL Copilot sessions across all projects
	sessions, _, err := lister.ListAll("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var copilotSessions []SessionInfo
	for _, s := range sessions {
		if s.Source == SourceCopilot {
			copilotSessions = append(copilotSessions, s)
		}
	}

	if len(copilotSessions) != 2 {
		t.Fatalf("expected 2 Copilot sessions with empty projectPath, got %d", len(copilotSessions))
	}

	ids := map[string]bool{}
	for _, s := range copilotSessions {
		ids[s.ID] = true
	}
	if !ids[sessionID1] {
		t.Error("missing session from /projects/alpha")
	}
	if !ids[sessionID2] {
		t.Error("missing session from /projects/beta")
	}
}

// setupCopilotSessionMissingWorkspace creates a Copilot session directory
// with a non-empty events.jsonl but no workspace.yaml at all.
func setupCopilotSessionMissingWorkspace(t *testing.T, homeDir, sessionID string, modTime time.Time) {
	t.Helper()
	sessionDir := filepath.Join(homeDir, ".copilot", "session-state", sessionID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("failed to create session dir: %v", err)
	}
	eventsPath := filepath.Join(sessionDir, "events.jsonl")
	if err := os.WriteFile(eventsPath, []byte(`{"type":"event","data":"test"}`+"\n"), 0644); err != nil {
		t.Fatalf("failed to write events file: %v", err)
	}
	if err := os.Chtimes(eventsPath, modTime, modTime); err != nil {
		t.Fatalf("failed to set events mtime: %v", err)
	}
}

// setupCopilotSessionInvalidWorkspace creates a Copilot session directory with
// a non-empty events.jsonl and a workspace.yaml that fails to parse as YAML.
func setupCopilotSessionInvalidWorkspace(t *testing.T, homeDir, sessionID string, modTime time.Time) {
	t.Helper()
	sessionDir := filepath.Join(homeDir, ".copilot", "session-state", sessionID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("failed to create session dir: %v", err)
	}
	eventsPath := filepath.Join(sessionDir, "events.jsonl")
	if err := os.WriteFile(eventsPath, []byte(`{"type":"event","data":"test"}`+"\n"), 0644); err != nil {
		t.Fatalf("failed to write events file: %v", err)
	}
	if err := os.Chtimes(eventsPath, modTime, modTime); err != nil {
		t.Fatalf("failed to set events mtime: %v", err)
	}
	// Intentionally malformed YAML — unterminated block, bad indent, control chars.
	bad := []byte("id: \"unterminated\ncwd: : :\n\t- not yaml\n")
	if err := os.WriteFile(filepath.Join(sessionDir, "workspace.yaml"), bad, 0644); err != nil {
		t.Fatalf("failed to write workspace file: %v", err)
	}
}

// TestListAllCopilotSessionsMissingWorkspace is a regression test for T-701.
// Sessions with a valid non-empty events.jsonl but no workspace.yaml at all
// must be listed when no project filter is applied. Previously listCopilot
// dropped them silently because parseCopilotWorkspace returned (nil, nil) for
// missing files and the caller treated ws == nil as "skip".
func TestListAllCopilotSessionsMissingWorkspace(t *testing.T) {
	homeDir := t.TempDir()

	sessionID := "missing-workspace-session"
	modTime := time.Date(2025, 4, 1, 12, 0, 0, 0, time.UTC)
	setupCopilotSessionMissingWorkspace(t, homeDir, sessionID, modTime)

	lister := newTestLister(homeDir)

	sessions, _, err := lister.ListAll("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var copilotSessions []SessionInfo
	for _, s := range sessions {
		if s.Source == SourceCopilot {
			copilotSessions = append(copilotSessions, s)
		}
	}

	if len(copilotSessions) != 1 {
		t.Fatalf("expected 1 Copilot session with missing workspace.yaml, got %d", len(copilotSessions))
	}
	if copilotSessions[0].ID != sessionID {
		t.Errorf("session.ID = %q, want %q", copilotSessions[0].ID, sessionID)
	}
	if !copilotSessions[0].CreatedAt.Equal(modTime) {
		t.Errorf("CreatedAt = %v, want events mtime %v", copilotSessions[0].CreatedAt, modTime)
	}
}

// TestListAllCopilotSessionsInvalidWorkspace is a regression test for T-701.
// Sessions with a valid non-empty events.jsonl but a workspace.yaml that
// cannot be parsed must still be listed when no project filter is applied.
func TestListAllCopilotSessionsInvalidWorkspace(t *testing.T) {
	homeDir := t.TempDir()

	sessionID := "invalid-workspace-session"
	modTime := time.Date(2025, 4, 2, 9, 0, 0, 0, time.UTC)
	setupCopilotSessionInvalidWorkspace(t, homeDir, sessionID, modTime)

	lister := newTestLister(homeDir)

	sessions, warnings, err := lister.ListAll("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var copilotSessions []SessionInfo
	for _, s := range sessions {
		if s.Source == SourceCopilot {
			copilotSessions = append(copilotSessions, s)
		}
	}

	if len(copilotSessions) != 1 {
		t.Fatalf("expected 1 Copilot session with invalid workspace.yaml, got %d", len(copilotSessions))
	}
	if copilotSessions[0].ID != sessionID {
		t.Errorf("session.ID = %q, want %q", copilotSessions[0].ID, sessionID)
	}

	// Verify a warning was emitted for the unparseable workspace.yaml so the
	// listing is observable even though the session is still returned.
	var copilotWarnings []ListWarning
	for _, w := range warnings {
		if w.Source == SourceCopilot {
			copilotWarnings = append(copilotWarnings, w)
		}
	}
	if len(copilotWarnings) == 0 {
		t.Fatal("expected a ListWarning for invalid workspace.yaml, got none")
	}
	if !strings.Contains(copilotWarnings[0].Err.Error(), sessionID) {
		t.Errorf("warning %q does not reference session ID %q", copilotWarnings[0].Err.Error(), sessionID)
	}
}

// TestListCopilotSessionsMalformedWorkspaceWithFilter verifies that when a
// project filter IS supplied, sessions with missing or invalid workspace.yaml
// do not falsely match (they have no metadata to compare against). They must
// be skipped — silently or with a warning, but never matched.
func TestListCopilotSessionsMalformedWorkspaceWithFilter(t *testing.T) {
	homeDir := t.TempDir()
	projectPath := t.TempDir()

	modTime := time.Date(2025, 4, 3, 9, 0, 0, 0, time.UTC)
	// One session with valid workspace pointing at projectPath — should match.
	setupCopilotSession(t, homeDir, projectPath, "valid-session", modTime)
	// One session with no workspace.yaml at all — must NOT match a project filter.
	setupCopilotSessionMissingWorkspace(t, homeDir, "missing-workspace-session", modTime)
	// One session with malformed workspace.yaml — must NOT match a project filter.
	setupCopilotSessionInvalidWorkspace(t, homeDir, "invalid-workspace-session", modTime)

	lister := newTestLister(homeDir)

	sessions, _, err := lister.ListAll(projectPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var copilotSessions []SessionInfo
	for _, s := range sessions {
		if s.Source == SourceCopilot {
			copilotSessions = append(copilotSessions, s)
		}
	}

	if len(copilotSessions) != 1 {
		t.Fatalf("expected 1 Copilot session matching project filter, got %d", len(copilotSessions))
	}
	if copilotSessions[0].ID != "valid-session" {
		t.Errorf("session.ID = %q, want %q", copilotSessions[0].ID, "valid-session")
	}
}

// setupCodexSessionWithLeadingLines creates a Codex session file where
// session_meta is NOT the first line. Non-meta entries appear before it.
func setupCodexSessionWithLeadingLines(t *testing.T, homeDir, projectPath, sessionID string, createdAt time.Time, leadingLines int) {
	t.Helper()
	dir := filepath.Join(homeDir, ".codex", "sessions",
		createdAt.Format("2006"), createdAt.Format("01"), createdAt.Format("02"))
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}

	var content []byte
	for i := range leadingLines {
		entry := map[string]any{
			"type":      "response_item",
			"timestamp": createdAt.Format(time.RFC3339),
			"payload": map[string]any{
				"type": "message",
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": fmt.Sprintf("message %d", i)},
				},
			},
		}
		data, err := json.Marshal(entry)
		if err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}
		content = append(content, data...)
		content = append(content, '\n')
	}

	meta := map[string]any{
		"type":      "session_meta",
		"timestamp": createdAt.Format(time.RFC3339),
		"payload": map[string]any{
			"id":  sessionID,
			"cwd": projectPath,
		},
	}
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	content = append(content, data...)
	content = append(content, '\n')

	filePath := filepath.Join(dir, fmt.Sprintf("session-%s.jsonl", sessionID))
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("failed to write session file: %v", err)
	}
}

// TestCodexSessionMetaNotFirstLine verifies that Codex sessions are discovered
// even when session_meta is not the first line in the JSONL file.
// Regression test for T-644.
func TestCodexSessionMetaNotFirstLine(t *testing.T) {
	t.Parallel()
	homeDir := t.TempDir()
	projectPath := t.TempDir()

	t1 := time.Date(2025, 1, 15, 14, 30, 0, 0, time.UTC)
	sessionID := "12345678-1234-1234-1234-123456789abc"

	// session_meta is on line 4 (after 3 response_item lines)
	setupCodexSessionWithLeadingLines(t, homeDir, projectPath, sessionID, t1, 3)

	lister := newTestLister(homeDir)
	sessions, _, err := lister.ListAll(projectPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var codexSessions []SessionInfo
	for _, s := range sessions {
		if s.Source == SourceCodex {
			codexSessions = append(codexSessions, s)
		}
	}

	if len(codexSessions) != 1 {
		t.Fatalf("expected 1 Codex session (session_meta on line 4), got %d", len(codexSessions))
	}

	if codexSessions[0].ID != sessionID {
		t.Errorf("session.ID = %q, want %q", codexSessions[0].ID, sessionID)
	}
	if !codexSessions[0].CreatedAt.Equal(t1) {
		t.Errorf("session.CreatedAt = %v, want %v", codexSessions[0].CreatedAt, t1)
	}
}

// TestGetCodexSessionCwdScansMultipleLines verifies getCodexSessionCwd scans
// past non-session_meta lines.
func TestGetCodexSessionCwdScansMultipleLines(t *testing.T) {
	t.Parallel()
	homeDir := t.TempDir()

	ts := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	sessionID := "cwd-scan-test"
	projectPath := "/home/user/my-project"

	setupCodexSessionWithLeadingLines(t, homeDir, projectPath, sessionID, ts, 5)

	dir := filepath.Join(homeDir, ".codex", "sessions",
		ts.Format("2006"), ts.Format("01"), ts.Format("02"))
	filePath := filepath.Join(dir, fmt.Sprintf("session-%s.jsonl", sessionID))

	cwd := getCodexSessionCwd(filePath)
	if cwd != projectPath {
		t.Errorf("getCodexSessionCwd() = %q, want %q", cwd, projectPath)
	}
}

// TestGetCodexSessionTimestampScansMultipleLines verifies getCodexSessionTimestamp
// scans past non-session_meta lines.
func TestGetCodexSessionTimestampScansMultipleLines(t *testing.T) {
	t.Parallel()
	homeDir := t.TempDir()

	ts := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	sessionID := "ts-scan-test"

	setupCodexSessionWithLeadingLines(t, homeDir, "/some/path", sessionID, ts, 5)

	dir := filepath.Join(homeDir, ".codex", "sessions",
		ts.Format("2006"), ts.Format("01"), ts.Format("02"))
	filePath := filepath.Join(dir, fmt.Sprintf("session-%s.jsonl", sessionID))

	got, err := getCodexSessionTimestamp(filePath)
	if err != nil {
		t.Fatalf("getCodexSessionTimestamp() error: %v", err)
	}
	if !got.Equal(ts) {
		t.Errorf("getCodexSessionTimestamp() = %v, want %v", got, ts)
	}
}

// TestCodexSessionMetaBeyondScanLimit verifies that when session_meta appears
// after the scan limit, the functions fall back gracefully (mod-time for
// timestamp, empty string for cwd) rather than scanning the entire file.
func TestCodexSessionMetaBeyondScanLimit(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	ts := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	sessionID := "beyond-limit-test"
	projectPath := "/home/user/project"

	// Place session_meta at line 51 (beyond the 50-line limit)
	setupCodexSessionWithLeadingLines(t, homeDir, projectPath, sessionID, ts, codexMetaScanLimit)

	dir := filepath.Join(homeDir, ".codex", "sessions",
		ts.Format("2006"), ts.Format("01"), ts.Format("02"))
	filePath := filepath.Join(dir, fmt.Sprintf("session-%s.jsonl", sessionID))

	// cwd should be empty — session_meta is beyond the scan limit
	cwd := getCodexSessionCwd(filePath)
	if cwd != "" {
		t.Errorf("getCodexSessionCwd() = %q, want empty string (beyond scan limit)", cwd)
	}

	// timestamp should fall back to file mod-time
	got, err := getCodexSessionTimestamp(filePath)
	if err != nil {
		t.Fatalf("getCodexSessionTimestamp() error: %v", err)
	}
	if got.Equal(ts) {
		t.Errorf("getCodexSessionTimestamp() returned session_meta timestamp %v, expected file mod-time (beyond scan limit)", got)
	}
}

func TestListAllPartialFailure(t *testing.T) {
	homeDir := t.TempDir()
	projectPath := t.TempDir()

	// Create a valid Claude session
	t1 := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	setupClaudeSession(t, homeDir, projectPath, "valid-session", t1)

	lister := newTestLister(homeDir)
	sessions, warnings, err := lister.ListAll(projectPath)

	// Should not return fatal error even if some sources fail
	if err != nil {
		t.Fatalf("ListAll() returned fatal error: %v", err)
	}

	// Should have the Claude session
	var claudeSessions []SessionInfo
	for _, s := range sessions {
		if s.Source == SourceClaude {
			claudeSessions = append(claudeSessions, s)
		}
	}
	if len(claudeSessions) != 1 {
		t.Errorf("expected 1 Claude session, got %d", len(claudeSessions))
	}

	// Kiro sources may produce warnings (database not found), that's expected
	for _, w := range warnings {
		if w.Source == SourceClaude || w.Source == SourceCodex || w.Source == SourceCopilot {
			t.Errorf("unexpected warning for %s: %v", w.Source, w.Err)
		}
	}
}

// TestListKiroUsesCreatedAtNotUpdatedAt is a regression test for T-550:
// listKiro was mapping SessionMetadata.UpdatedAt to SessionInfo.CreatedAt
// instead of SessionMetadata.CreatedAt.
func TestListKiroUsesCreatedAtNotUpdatedAt(t *testing.T) {
	created := time.Date(2025, 1, 10, 10, 0, 0, 0, time.UTC)
	updated := time.Date(2025, 1, 20, 14, 30, 0, 0, time.UTC)

	lister := &Lister{
		homeDir: t.TempDir(),
		kiroDiscover: func(_ context.Context, _ string) ([]logs.SessionMetadata, error) {
			return []logs.SessionMetadata{
				{
					ConversationID: "kiro-session-1",
					Directory:      "/my/project",
					CreatedAt:      created,
					UpdatedAt:      updated,
					Size:           1024,
				},
			}, nil
		},
	}

	sessions, err := lister.listKiro("/my/project")
	if err != nil {
		t.Fatalf("listKiro returned error: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	s := sessions[0]
	if s.ID != "kiro-session-1" {
		t.Errorf("session.ID = %q, want %q", s.ID, "kiro-session-1")
	}
	if s.Source != SourceKiroCLI {
		t.Errorf("session.Source = %q, want %q", s.Source, SourceKiroCLI)
	}
	if !s.CreatedAt.Equal(created) {
		t.Errorf("session.CreatedAt = %v, want %v (got UpdatedAt instead of CreatedAt)",
			s.CreatedAt, created)
	}
	if s.Size != 1024 {
		t.Errorf("session.Size = %d, want %d", s.Size, 1024)
	}
}
