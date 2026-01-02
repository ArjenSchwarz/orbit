// Package logs provides session log management for Orbit.
package logs

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/arjenschwarz/orbit/internal/claude"
	"github.com/arjenschwarz/orbit/internal/transcript"
	"github.com/google/uuid"
)

// ManagerOptions configures the log manager behavior.
type ManagerOptions struct {
	UseSubdirs bool // If true, use timestamped subdirectories
}

// PhaseState tracks an in-progress phase for crash recovery.
type PhaseState struct {
	Phase     int       `json:"phase"`
	SessionID string    `json:"session_id"`
	StartedAt time.Time `json:"started_at"`
}

// PostCompletionState tracks in-progress post-completion command.
type PostCompletionState struct {
	SessionID string    `json:"session_id"`
	StartedAt time.Time `json:"started_at"`
}

// SessionEntry records metadata about a completed Claude session.
type SessionEntry struct {
	Phase      int       `json:"phase"`
	SessionID  string    `json:"session_id"`
	DurationMS int64     `json:"duration_ms"`
	CostUSD    float64   `json:"cost_usd"`
	NumTurns   int       `json:"num_turns"`
	StartedAt  time.Time `json:"started_at"`
	EndedAt    time.Time `json:"ended_at"`
	IsError    bool      `json:"is_error,omitempty"`
	RunNumber  int       `json:"run_number"`
}

// Summary contains the overall orchestration run summary.
type Summary struct {
	StartedAt       time.Time      `json:"started_at"`
	CompletedAt     *time.Time     `json:"completed_at,omitempty"`
	Status          string         `json:"status"`
	PhasesCompleted int            `json:"phases_completed"`
	TotalCostUSD    float64        `json:"total_cost_usd"`
	TotalDurationMS int64          `json:"total_duration_ms"`
	Sessions        []SessionEntry `json:"sessions"`
	Error           string         `json:"error,omitempty"`
	// New fields for session management
	CurrentPhase   *PhaseState          `json:"current_phase,omitempty"`
	PostCompletion *PostCompletionState `json:"post_completion,omitempty"`
	RunNumber      int                  `json:"run_number"`
	BranchName     string               `json:"branch_name,omitempty"`
}

// Manager handles log storage and retrieval.
type Manager struct {
	baseDir    string
	sessionDir string
	workingDir string
	summary    Summary
	useSubdirs bool   // controls directory mode
	branchName string // stored for branch mismatch warning
}

// NewManager creates a new log manager with a timestamped session directory.
// workingDir is used to locate Claude session transcripts in ~/.claude/projects.
// Deprecated: Use NewManagerWithOptions for new code.
func NewManager(baseDir, branchName, workingDir string) (*Manager, error) {
	return NewManagerWithOptions(baseDir, branchName, workingDir, ManagerOptions{UseSubdirs: true})
}

// NewManagerWithOptions creates a new log manager with configurable options.
// In flat mode (UseSubdirs=false), logs are stored directly in baseDir.
// In subdir mode (UseSubdirs=true), logs are stored in timestamped subdirectories.
func NewManagerWithOptions(baseDir, branchName, workingDir string, opts ManagerOptions) (*Manager, error) {
	sessionDir := baseDir

	if opts.UseSubdirs {
		// Timestamped subdirectory (existing behavior)
		timestamp := time.Now().Format("2006-01-02-150405")
		sessionDir = filepath.Join(baseDir, fmt.Sprintf("%s-%s", timestamp, sanitizeName(branchName)))
	}

	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	m := &Manager{
		baseDir:    baseDir,
		sessionDir: sessionDir,
		workingDir: workingDir,
		useSubdirs: opts.UseSubdirs,
		branchName: branchName,
	}

	// Try to load existing summary in flat mode
	if !opts.UseSubdirs {
		if err := m.loadExistingSummary(); err != nil {
			// No existing summary or corrupt - start fresh
			m.summary = Summary{
				StartedAt:  time.Now(),
				Status:     "running",
				Sessions:   []SessionEntry{},
				RunNumber:  1,
				BranchName: branchName,
			}
		} else {
			// Check for branch mismatch
			if m.summary.BranchName != "" && m.summary.BranchName != branchName {
				log.Printf("Warning: Branch changed from '%s' to '%s'. Session continuation may have unexpected results.",
					m.summary.BranchName, branchName)
			}
			// Increment run number for new orchestration run
			m.summary.RunNumber++
			m.summary.Status = "running"
			m.summary.BranchName = branchName // Update to current branch
		}
	} else {
		// Fresh run with subdirectory
		m.summary = Summary{
			StartedAt:  time.Now(),
			Status:     "running",
			Sessions:   []SessionEntry{},
			RunNumber:  1,
			BranchName: branchName,
		}
	}

	if err := m.writeSummary(); err != nil {
		return nil, err
	}

	return m, nil
}

// SessionDir returns the current session directory path.
func (m *Manager) SessionDir() string {
	return m.sessionDir
}

// loadExistingSummary loads an existing summary.json from the session directory.
func (m *Manager) loadExistingSummary() error {
	path := filepath.Join(m.sessionDir, "summary.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read summary: %w", err)
	}

	if err := json.Unmarshal(data, &m.summary); err != nil {
		return fmt.Errorf("failed to parse summary: %w", err)
	}

	return nil
}

// StartPhase begins a new phase or resumes an existing one.
// Returns the session ID, whether this is a resume, and any error.
func (m *Manager) StartPhase(phase int, continueSession bool) (string, bool, error) {
	// Check for existing in-progress phase
	if m.summary.CurrentPhase != nil && m.summary.CurrentPhase.Phase == phase {
		if continueSession {
			// Resume existing session
			return m.summary.CurrentPhase.SessionID, true, nil
		}
		// Not continuing - clear old state
		m.summary.CurrentPhase = nil
	}

	// Generate new session ID
	sessionID := uuid.NewString()

	// Record phase start BEFORE invoking Claude (req 5.1)
	m.summary.CurrentPhase = &PhaseState{
		Phase:     phase,
		SessionID: sessionID,
		StartedAt: time.Now(),
	}

	if err := m.writeSummary(); err != nil {
		return "", false, err
	}

	return sessionID, false, nil
}

// SetCurrentPhaseSessionID updates the session ID for the current phase.
// Used when resume fails and a new session ID is generated.
func (m *Manager) SetCurrentPhaseSessionID(sessionID string) error {
	if m.summary.CurrentPhase == nil {
		return nil // No-op if no current phase
	}

	m.summary.CurrentPhase.SessionID = sessionID
	return m.writeSummary()
}

// ReconcileSessionID updates the stored session ID if Claude returned a different one.
func (m *Manager) ReconcileSessionID(returnedID string) {
	if m.summary.CurrentPhase == nil {
		return // No-op if no current phase
	}

	if m.summary.CurrentPhase.SessionID != returnedID {
		m.summary.CurrentPhase.SessionID = returnedID
		_ = m.writeSummary() // Best effort - don't fail the session
	}
}

// CompletePhase clears the current phase state after successful completion.
func (m *Manager) CompletePhase() error {
	m.summary.CurrentPhase = nil
	return m.writeSummary()
}

// StartPostCompletion begins a post-completion command or resumes an existing one.
// Returns the session ID, whether this is a resume, and any error.
func (m *Manager) StartPostCompletion(continueSession bool) (string, bool, error) {
	// Check for existing in-progress post-completion
	if m.summary.PostCompletion != nil {
		if continueSession {
			// Resume existing session
			return m.summary.PostCompletion.SessionID, true, nil
		}
		// Not continuing - clear old state
		m.summary.PostCompletion = nil
	}

	// Generate new session ID
	sessionID := uuid.NewString()

	// Record post-completion start BEFORE invoking Claude
	m.summary.PostCompletion = &PostCompletionState{
		SessionID: sessionID,
		StartedAt: time.Now(),
	}

	if err := m.writeSummary(); err != nil {
		return "", false, err
	}

	return sessionID, false, nil
}

// CompletePostCompletion clears the post-completion state after successful completion.
func (m *Manager) CompletePostCompletion() error {
	m.summary.PostCompletion = nil
	return m.writeSummary()
}

// SetPostCompletionSessionID updates the session ID for the current post-completion.
// This is called when a resume attempt fails and a new session is started.
func (m *Manager) SetPostCompletionSessionID(sessionID string) error {
	if m.summary.PostCompletion == nil {
		return nil // No-op if no post-completion in progress
	}
	m.summary.PostCompletion.SessionID = sessionID
	return m.writeSummary()
}

// ReconcilePostCompletionSessionID updates the stored session ID if Claude returned a different one.
func (m *Manager) ReconcilePostCompletionSessionID(returnedID string) {
	if m.summary.PostCompletion == nil {
		return // No-op if no post-completion in progress
	}

	if m.summary.PostCompletion.SessionID != returnedID {
		m.summary.PostCompletion.SessionID = returnedID
		_ = m.writeSummary() // Best effort - don't fail the session
	}
}

// phaseFileName generates the filename for phase files.
// Returns run-numbered filename when RunNumber > 1 in flat mode.
func (m *Manager) phaseFileName(phase int, suffix string) string {
	if m.summary.RunNumber > 1 && !m.useSubdirs {
		return fmt.Sprintf("phase-%d-run-%d-%s", phase, m.summary.RunNumber, suffix)
	}
	return fmt.Sprintf("phase-%d-%s", phase, suffix)
}

// postCompletionFileName generates the base filename for post-completion files.
// Returns run-numbered filename when RunNumber > 1 in flat mode.
func (m *Manager) postCompletionFileName() string {
	if m.summary.RunNumber > 1 && !m.useSubdirs {
		return fmt.Sprintf("post-completion-run-%d-session", m.summary.RunNumber)
	}
	return "post-completion-session"
}

// SaveSession records a completed Claude session.
func (m *Manager) SaveSession(phase int, result *claude.SessionResult, startTime time.Time) error {
	endTime := time.Now()

	entry := SessionEntry{
		Phase:      phase,
		SessionID:  result.SessionID,
		DurationMS: result.Duration.Milliseconds(),
		CostUSD:    result.Cost,
		NumTurns:   result.NumTurns,
		StartedAt:  startTime,
		EndedAt:    endTime,
		IsError:    result.IsError,
		RunNumber:  m.summary.RunNumber,
	}

	m.summary.Sessions = append(m.summary.Sessions, entry)
	m.summary.PhasesCompleted = phase
	m.summary.TotalCostUSD += result.Cost
	m.summary.TotalDurationMS += result.Duration.Milliseconds()

	// Write session JSON (result only)
	jsonPath := filepath.Join(m.sessionDir, m.phaseFileName(phase, "session.json"))
	if err := os.WriteFile(jsonPath, result.RawJSON, 0644); err != nil {
		return fmt.Errorf("failed to write session JSON: %w", err)
	}

	// Copy full transcript from ~/.claude/projects if available
	if result.SessionID != "" {
		if err := m.copySessionTranscript(phase, result.SessionID); err != nil {
			// Log but don't fail - transcript is supplementary
			fmt.Fprintf(os.Stderr, "Warning: could not copy session transcript: %v\n", err)
		}
	}

	// Write human-readable summary
	txtPath := filepath.Join(m.sessionDir, m.phaseFileName(phase, "session.txt"))
	transcript := formatTranscript(phase, result, startTime, endTime)
	if err := os.WriteFile(txtPath, []byte(transcript), 0644); err != nil {
		return fmt.Errorf("failed to write session transcript: %w", err)
	}

	// Update summary
	return m.writeSummary()
}

// Complete marks the orchestration run as complete.
func (m *Manager) Complete() error {
	now := time.Now()
	m.summary.CompletedAt = &now
	m.summary.Status = "success"
	if err := m.writeSummary(); err != nil {
		return err
	}
	// Write index files linking to all session transcripts
	if err := m.writeRunIndex(); err != nil {
		// Log but don't fail - index is supplementary
		fmt.Fprintf(os.Stderr, "Warning: could not write run index: %v\n", err)
	}
	return nil
}

// SavePostCompletionSession saves the post-command session with distinct naming.
func (m *Manager) SavePostCompletionSession(result *claude.SessionResult, startTime time.Time) error {
	endTime := time.Now()
	baseName := m.postCompletionFileName()

	// Add session entry to summary (Phase 0 indicates post-completion)
	entry := SessionEntry{
		Phase:      0, // Post-completion marker
		SessionID:  result.SessionID,
		DurationMS: result.Duration.Milliseconds(),
		CostUSD:    result.Cost,
		NumTurns:   result.NumTurns,
		StartedAt:  startTime,
		EndedAt:    endTime,
		IsError:    result.IsError,
		RunNumber:  m.summary.RunNumber,
	}
	m.summary.Sessions = append(m.summary.Sessions, entry)
	m.summary.TotalCostUSD += result.Cost
	m.summary.TotalDurationMS += result.Duration.Milliseconds()

	// Save JSON
	jsonPath := filepath.Join(m.sessionDir, baseName+".json")
	if err := os.WriteFile(jsonPath, result.RawJSON, 0644); err != nil {
		return fmt.Errorf("failed to write post-completion JSON: %w", err)
	}

	// Save transcript
	txtPath := filepath.Join(m.sessionDir, baseName+".txt")
	transcript := formatPostCompletionTranscript(result, startTime, endTime)
	if err := os.WriteFile(txtPath, []byte(transcript), 0644); err != nil {
		return fmt.Errorf("failed to write post-completion transcript: %w", err)
	}

	// Copy full transcript from ~/.claude/projects if available
	if result.SessionID != "" {
		if err := m.copyPostCompletionTranscript(result.SessionID, baseName); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not copy post-completion transcript: %v\n", err)
		}
	}

	// Update summary with the new session entry
	return m.writeSummary()
}

// copyPostCompletionTranscript copies the full session transcript for post-completion.
func (m *Manager) copyPostCompletionTranscript(sessionID, baseName string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	projectPath := claude.BuildProjectPath(m.workingDir)
	claudeProjectsDir := filepath.Join(homeDir, ".claude", "projects", projectPath)
	srcPath := filepath.Join(claudeProjectsDir, sessionID+".jsonl")

	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		return fmt.Errorf("session transcript not found: %s", srcPath)
	}

	// Copy raw JSONL
	jsonlDstPath := filepath.Join(m.sessionDir, baseName+"-transcript.jsonl")
	if err := copyFile(srcPath, jsonlDstPath); err != nil {
		return err
	}

	// Parse and generate Markdown
	mdDstPath := filepath.Join(m.sessionDir, baseName+"-transcript.md")
	if err := m.generatePostCompletionMarkdownTranscript(srcPath, mdDstPath, sessionID); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not generate Markdown transcript: %v\n", err)
	}

	return nil
}

// generatePostCompletionMarkdownTranscript parses a JSONL transcript and writes Markdown and HTML for post-completion.
func (m *Manager) generatePostCompletionMarkdownTranscript(srcPath, dstPath, sessionID string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open transcript: %w", err)
	}
	defer func() { _ = src.Close() }()

	result, err := transcript.ParseJSONL(src)
	if err != nil {
		return err
	}

	// Log warnings to stderr
	for _, w := range result.Warnings {
		fmt.Fprintf(os.Stderr, "Warning: line %d: %s\n", w.Line, w.Message)
	}

	opts := transcript.RenderOptions{
		Title:     "Post-Completion Session Transcript",
		SessionID: sessionID,
	}

	// Write Markdown
	markdown := transcript.RenderMarkdown(result.Entries, opts)
	if err := os.WriteFile(dstPath, []byte(markdown), 0644); err != nil {
		return err
	}

	// Write HTML (replace .md extension with .html)
	htmlPath := strings.TrimSuffix(dstPath, ".md") + ".html"
	htmlContent := transcript.RenderHTML(result.Entries, opts)
	if err := os.WriteFile(htmlPath, []byte(htmlContent), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not write HTML transcript: %v\n", err)
	}

	return nil
}

// formatPostCompletionTranscript creates a human-readable post-completion transcript.
func formatPostCompletionTranscript(result *claude.SessionResult, start, end time.Time) string {
	return fmt.Sprintf(`Orbit Post-Completion Session Log
========================================

Session ID: %s
Started:    %s
Ended:      %s
Duration:   %s
Cost:       $%.4f
Turns:      %d
Error:      %v

Output:
----------------------------------------
%s

Stderr:
----------------------------------------
%s
`,
		result.SessionID,
		start.Format(time.RFC3339),
		end.Format(time.RFC3339),
		result.Duration.String(),
		result.Cost,
		result.NumTurns,
		result.IsError,
		result.Output,
		result.Stderr,
	)
}

// Fail marks the orchestration run as failed with an error message.
func (m *Manager) Fail(err error) error {
	now := time.Now()
	m.summary.CompletedAt = &now
	m.summary.Status = "failed"
	m.summary.Error = err.Error()
	if writeErr := m.writeSummary(); writeErr != nil {
		return writeErr
	}
	// Write index files even on failure so users can see what happened
	if indexErr := m.writeRunIndex(); indexErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not write run index: %v\n", indexErr)
	}
	return nil
}

// writeSummary writes the current summary to disk.
func (m *Manager) writeSummary() error {
	data, err := json.MarshalIndent(m.summary, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal summary: %w", err)
	}

	path := filepath.Join(m.sessionDir, "summary.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write summary: %w", err)
	}

	return nil
}

// formatTranscript creates a human-readable session transcript.
func formatTranscript(phase int, result *claude.SessionResult, start, end time.Time) string {
	return fmt.Sprintf(`Orbit Session Log - Phase %d
========================================

Session ID: %s
Started:    %s
Ended:      %s
Duration:   %s
Cost:       $%.4f
Turns:      %d
Error:      %v

Output:
----------------------------------------
%s

Stderr:
----------------------------------------
%s
`,
		phase,
		result.SessionID,
		start.Format(time.RFC3339),
		end.Format(time.RFC3339),
		result.Duration.String(),
		result.Cost,
		result.NumTurns,
		result.IsError,
		result.Output,
		result.Stderr,
	)
}

// copySessionTranscript copies the full session transcript from ~/.claude/projects
// and generates a Markdown version.
func (m *Manager) copySessionTranscript(phase int, sessionID string) error {
	// Build the Claude projects path
	// Claude stores sessions in ~/.claude/projects/{project-path}/{session-id}.jsonl
	// where project-path has the leading separator removed and remaining separators replaced with dashes
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	projectPath := claude.BuildProjectPath(m.workingDir)
	claudeProjectsDir := filepath.Join(homeDir, ".claude", "projects", projectPath)
	srcPath := filepath.Join(claudeProjectsDir, sessionID+".jsonl")

	// Check if source file exists
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		return fmt.Errorf("session transcript not found: %s", srcPath)
	}

	// Copy raw JSONL to destination
	jsonlDstPath := filepath.Join(m.sessionDir, m.phaseFileName(phase, "transcript.jsonl"))
	if err := copyFile(srcPath, jsonlDstPath); err != nil {
		return err
	}

	// Parse and generate Markdown
	mdDstPath := filepath.Join(m.sessionDir, m.phaseFileName(phase, "transcript.md"))
	if err := m.generateMarkdownTranscript(srcPath, mdDstPath, phase, sessionID); err != nil {
		// Log but don't fail - Markdown is supplementary
		fmt.Fprintf(os.Stderr, "Warning: could not generate Markdown transcript: %v\n", err)
	}

	return nil
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source: %w", err)
	}
	defer func() { _ = srcFile.Close() }()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination: %w", err)
	}
	defer func() { _ = dstFile.Close() }()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy: %w", err)
	}

	return nil
}

// generateMarkdownTranscript parses a JSONL transcript and writes Markdown and HTML files.
func (m *Manager) generateMarkdownTranscript(srcPath, dstPath string, phase int, sessionID string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open transcript: %w", err)
	}
	defer func() { _ = src.Close() }()

	result, err := transcript.ParseJSONL(src)
	if err != nil {
		return err
	}

	// Log warnings to stderr
	for _, w := range result.Warnings {
		fmt.Fprintf(os.Stderr, "Warning: line %d: %s\n", w.Line, w.Message)
	}

	opts := transcript.RenderOptions{
		Title:     fmt.Sprintf("Phase %d Session Transcript", phase),
		SessionID: sessionID,
	}

	// Write Markdown
	markdown := transcript.RenderMarkdown(result.Entries, opts)
	if err := os.WriteFile(dstPath, []byte(markdown), 0644); err != nil {
		return err
	}

	// Write HTML (replace .md extension with .html)
	htmlPath := strings.TrimSuffix(dstPath, ".md") + ".html"
	htmlContent := transcript.RenderHTML(result.Entries, opts)
	if err := os.WriteFile(htmlPath, []byte(htmlContent), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not write HTML transcript: %v\n", err)
	}

	return nil
}


// sortedPhaseMap groups sessions by phase and returns sorted phase numbers.
// Post-completion sessions (phase 0) are excluded from the map.
func sortedPhaseMap(sessions []SessionEntry) (map[int][]SessionEntry, []int) {
	phaseMap := make(map[int][]SessionEntry)
	for _, session := range sessions {
		if session.Phase > 0 {
			phaseMap[session.Phase] = append(phaseMap[session.Phase], session)
		}
	}

	phases := make([]int, 0, len(phaseMap))
	for phase := range phaseMap {
		phases = append(phases, phase)
	}
	sort.Ints(phases)

	return phaseMap, phases
}

// writeRunIndex generates index.md and index.html files that link to all session transcripts.
func (m *Manager) writeRunIndex() error {
	// Generate markdown index
	mdContent := m.generateMarkdownIndex()
	mdPath := filepath.Join(m.sessionDir, "index.md")
	if err := os.WriteFile(mdPath, []byte(mdContent), 0644); err != nil {
		return fmt.Errorf("failed to write index.md: %w", err)
	}

	// Generate HTML index
	htmlContent := m.generateHTMLIndex()
	htmlPath := filepath.Join(m.sessionDir, "index.html")
	if err := os.WriteFile(htmlPath, []byte(htmlContent), 0644); err != nil {
		return fmt.Errorf("failed to write index.html: %w", err)
	}

	return nil
}

// generateMarkdownIndex creates a markdown index of all session transcripts.
func (m *Manager) generateMarkdownIndex() string {
	var sb strings.Builder

	sb.WriteString("# Orbit Run Summary\n\n")

	// Run metadata
	sb.WriteString("## Run Information\n\n")
	sb.WriteString(fmt.Sprintf("- **Branch:** %s\n", m.summary.BranchName))
	sb.WriteString(fmt.Sprintf("- **Status:** %s\n", m.summary.Status))
	sb.WriteString(fmt.Sprintf("- **Started:** %s\n", m.summary.StartedAt.Format(time.RFC3339)))
	if m.summary.CompletedAt != nil {
		sb.WriteString(fmt.Sprintf("- **Completed:** %s\n", m.summary.CompletedAt.Format(time.RFC3339)))
		duration := m.summary.CompletedAt.Sub(m.summary.StartedAt)
		sb.WriteString(fmt.Sprintf("- **Total Duration:** %s\n", duration.Round(time.Second)))
	}
	sb.WriteString(fmt.Sprintf("- **Phases Completed:** %d\n", m.summary.PhasesCompleted))
	sb.WriteString(fmt.Sprintf("- **Total Cost:** $%.4f\n", m.summary.TotalCostUSD))
	if m.summary.Error != "" {
		sb.WriteString(fmt.Sprintf("- **Error:** %s\n", m.summary.Error))
	}
	sb.WriteString("\n")

	// Phase transcripts
	sb.WriteString("## Session Transcripts\n\n")

	phaseMap, phases := sortedPhaseMap(m.summary.Sessions)

	for _, phase := range phases {
		sessions := phaseMap[phase]
		sb.WriteString(fmt.Sprintf("### Phase %d\n\n", phase))

		for _, session := range sessions {
			runLabel := ""
			if session.RunNumber > 1 || len(sessions) > 1 {
				runLabel = fmt.Sprintf(" (Run %d)", session.RunNumber)
			}

			statusIcon := "✅"
			if session.IsError {
				statusIcon = "❌"
			}

			sb.WriteString(fmt.Sprintf("- %s **Session%s** - Cost: $%.4f, Duration: %s, Turns: %d\n",
				statusIcon, runLabel, session.CostUSD,
				time.Duration(session.DurationMS*int64(time.Millisecond)).Round(time.Second),
				session.NumTurns))

			// Links to transcript files
			mdFile := m.phaseFileName(phase, "transcript.md")
			htmlFile := m.phaseFileName(phase, "transcript.html")
			sb.WriteString(fmt.Sprintf("  - [Markdown](%s) | [HTML](%s)\n", mdFile, htmlFile))
		}
		sb.WriteString("\n")
	}

	// Post-completion if exists
	for _, session := range m.summary.Sessions {
		if session.Phase == 0 {
			sb.WriteString("### Post-Completion\n\n")
			statusIcon := "✅"
			if session.IsError {
				statusIcon = "❌"
			}
			sb.WriteString(fmt.Sprintf("- %s **Session** - Cost: $%.4f, Duration: %s, Turns: %d\n",
				statusIcon, session.CostUSD,
				time.Duration(session.DurationMS*int64(time.Millisecond)).Round(time.Second),
				session.NumTurns))

			baseName := m.postCompletionFileName()
			mdFile := baseName + "-transcript.md"
			htmlFile := baseName + "-transcript.html"
			sb.WriteString(fmt.Sprintf("  - [Markdown](%s) | [HTML](%s)\n", mdFile, htmlFile))
			sb.WriteString("\n")
			break
		}
	}

	return sb.String()
}

// generateHTMLIndex creates an HTML index of all session transcripts.
func (m *Manager) generateHTMLIndex() string {
	var sb strings.Builder

	title := "Orbit Run Summary"

	sb.WriteString("<!DOCTYPE html>\n")
	sb.WriteString("<html lang=\"en\">\n")
	sb.WriteString("<head>\n")
	sb.WriteString("    <meta charset=\"UTF-8\">\n")
	sb.WriteString("    <meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\">\n")
	sb.WriteString(fmt.Sprintf("    <title>%s</title>\n", title))
	sb.WriteString("    <style>\n")
	sb.WriteString(indexCSS)
	sb.WriteString("    </style>\n")
	sb.WriteString("</head>\n")
	sb.WriteString("<body>\n")
	sb.WriteString("    <header>\n")
	sb.WriteString(fmt.Sprintf("        <h1>%s</h1>\n", title))
	sb.WriteString("    </header>\n")
	sb.WriteString("    <main>\n")

	// Run information section
	sb.WriteString("        <section class=\"run-info\">\n")
	sb.WriteString("            <h2>Run Information</h2>\n")
	sb.WriteString("            <dl>\n")
	sb.WriteString(fmt.Sprintf("                <dt>Branch</dt><dd>%s</dd>\n", html.EscapeString(m.summary.BranchName)))

	statusClass := "success"
	switch m.summary.Status {
	case "failed":
		statusClass = "error"
	case "running":
		statusClass = "running"
	}
	sb.WriteString(fmt.Sprintf("                <dt>Status</dt><dd class=\"status %s\">%s</dd>\n",
		statusClass, html.EscapeString(m.summary.Status)))

	sb.WriteString(fmt.Sprintf("                <dt>Started</dt><dd>%s</dd>\n",
		m.summary.StartedAt.Format(time.RFC3339)))
	if m.summary.CompletedAt != nil {
		sb.WriteString(fmt.Sprintf("                <dt>Completed</dt><dd>%s</dd>\n",
			m.summary.CompletedAt.Format(time.RFC3339)))
		duration := m.summary.CompletedAt.Sub(m.summary.StartedAt)
		sb.WriteString(fmt.Sprintf("                <dt>Total Duration</dt><dd>%s</dd>\n",
			duration.Round(time.Second)))
	}
	sb.WriteString(fmt.Sprintf("                <dt>Phases Completed</dt><dd>%d</dd>\n",
		m.summary.PhasesCompleted))
	sb.WriteString(fmt.Sprintf("                <dt>Total Cost</dt><dd>$%.4f</dd>\n",
		m.summary.TotalCostUSD))
	if m.summary.Error != "" {
		sb.WriteString(fmt.Sprintf("                <dt>Error</dt><dd class=\"error-text\">%s</dd>\n",
			html.EscapeString(m.summary.Error)))
	}
	sb.WriteString("            </dl>\n")
	sb.WriteString("        </section>\n")

	// Session transcripts section
	sb.WriteString("        <section class=\"transcripts\">\n")
	sb.WriteString("            <h2>Session Transcripts</h2>\n")

	phaseMap, phases := sortedPhaseMap(m.summary.Sessions)

	for _, phase := range phases {
		sessions := phaseMap[phase]
		sb.WriteString("            <div class=\"phase\">\n")
		sb.WriteString(fmt.Sprintf("                <h3>Phase %d</h3>\n", phase))

		for _, session := range sessions {
			runLabel := ""
			if session.RunNumber > 1 || len(sessions) > 1 {
				runLabel = fmt.Sprintf(" (Run %d)", session.RunNumber)
			}

			statusIcon := "✅"
			cardClass := "session-card"
			if session.IsError {
				statusIcon = "❌"
				cardClass += " error"
			}

			sb.WriteString(fmt.Sprintf("                <div class=\"%s\">\n", cardClass))
			sb.WriteString(fmt.Sprintf("                    <div class=\"session-header\">%s Session%s</div>\n",
				statusIcon, runLabel))
			sb.WriteString("                    <div class=\"session-stats\">\n")
			sb.WriteString(fmt.Sprintf("                        <span>Cost: $%.4f</span>\n", session.CostUSD))
			sb.WriteString(fmt.Sprintf("                        <span>Duration: %s</span>\n",
				time.Duration(session.DurationMS*int64(time.Millisecond)).Round(time.Second)))
			sb.WriteString(fmt.Sprintf("                        <span>Turns: %d</span>\n", session.NumTurns))
			sb.WriteString("                    </div>\n")
			sb.WriteString("                    <div class=\"session-links\">\n")

			mdFile := m.phaseFileName(phase, "transcript.md")
			htmlFile := m.phaseFileName(phase, "transcript.html")
			sb.WriteString(fmt.Sprintf("                        <a href=\"%s\">📄 Markdown</a>\n", mdFile))
			sb.WriteString(fmt.Sprintf("                        <a href=\"%s\">🌐 HTML</a>\n", htmlFile))
			sb.WriteString("                    </div>\n")
			sb.WriteString("                </div>\n")
		}
		sb.WriteString("            </div>\n")
	}

	// Post-completion if exists
	for _, session := range m.summary.Sessions {
		if session.Phase == 0 {
			sb.WriteString("            <div class=\"phase\">\n")
			sb.WriteString("                <h3>Post-Completion</h3>\n")

			statusIcon := "✅"
			cardClass := "session-card"
			if session.IsError {
				statusIcon = "❌"
				cardClass += " error"
			}

			sb.WriteString(fmt.Sprintf("                <div class=\"%s\">\n", cardClass))
			sb.WriteString(fmt.Sprintf("                    <div class=\"session-header\">%s Session</div>\n", statusIcon))
			sb.WriteString("                    <div class=\"session-stats\">\n")
			sb.WriteString(fmt.Sprintf("                        <span>Cost: $%.4f</span>\n", session.CostUSD))
			sb.WriteString(fmt.Sprintf("                        <span>Duration: %s</span>\n",
				time.Duration(session.DurationMS*int64(time.Millisecond)).Round(time.Second)))
			sb.WriteString(fmt.Sprintf("                        <span>Turns: %d</span>\n", session.NumTurns))
			sb.WriteString("                    </div>\n")
			sb.WriteString("                    <div class=\"session-links\">\n")

			baseName := m.postCompletionFileName()
			mdFile := baseName + "-transcript.md"
			htmlFile := baseName + "-transcript.html"
			sb.WriteString(fmt.Sprintf("                        <a href=\"%s\">📄 Markdown</a>\n", mdFile))
			sb.WriteString(fmt.Sprintf("                        <a href=\"%s\">🌐 HTML</a>\n", htmlFile))
			sb.WriteString("                    </div>\n")
			sb.WriteString("                </div>\n")
			sb.WriteString("            </div>\n")
			break
		}
	}

	sb.WriteString("        </section>\n")
	sb.WriteString("    </main>\n")
	sb.WriteString("</body>\n")
	sb.WriteString("</html>\n")

	return sb.String()
}

// indexCSS contains the embedded stylesheet for the HTML index.
const indexCSS = `
:root {
    --bg-primary: #ffffff;
    --bg-secondary: #f8f9fa;
    --bg-card: #ffffff;
    --text-primary: #212529;
    --text-secondary: #6c757d;
    --border-color: #dee2e6;
    --success-color: #198754;
    --error-color: #dc3545;
    --running-color: #0d6efd;
    --link-color: #0d6efd;
}

@media (prefers-color-scheme: dark) {
    :root {
        --bg-primary: #1a1a1a;
        --bg-secondary: #2d2d2d;
        --bg-card: #2d2d2d;
        --text-primary: #e9ecef;
        --text-secondary: #adb5bd;
        --border-color: #495057;
        --link-color: #6ea8fe;
    }
}

* {
    box-sizing: border-box;
}

body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
    line-height: 1.6;
    color: var(--text-primary);
    background-color: var(--bg-primary);
    max-width: 900px;
    margin: 0 auto;
    padding: 2rem;
}

header {
    margin-bottom: 2rem;
    padding-bottom: 1rem;
    border-bottom: 2px solid var(--border-color);
}

header h1 {
    margin: 0;
    font-size: 1.75rem;
}

h2 {
    font-size: 1.4rem;
    margin: 1.5rem 0 1rem 0;
    color: var(--text-primary);
}

h3 {
    font-size: 1.1rem;
    margin: 1rem 0 0.75rem 0;
    color: var(--text-secondary);
}

.run-info {
    background-color: var(--bg-secondary);
    padding: 1rem 1.5rem;
    border-radius: 8px;
    margin-bottom: 2rem;
}

.run-info dl {
    display: grid;
    grid-template-columns: auto 1fr;
    gap: 0.5rem 1rem;
    margin: 0;
}

.run-info dt {
    font-weight: 600;
    color: var(--text-secondary);
}

.run-info dd {
    margin: 0;
}

.status.success {
    color: var(--success-color);
    font-weight: 600;
}

.status.error {
    color: var(--error-color);
    font-weight: 600;
}

.status.running {
    color: var(--running-color);
    font-weight: 600;
}

.error-text {
    color: var(--error-color);
}

.phase {
    margin-bottom: 1.5rem;
}

.session-card {
    background-color: var(--bg-card);
    border: 1px solid var(--border-color);
    border-radius: 8px;
    padding: 1rem;
    margin-bottom: 0.75rem;
}

.session-card.error {
    border-left: 4px solid var(--error-color);
}

.session-header {
    font-weight: 600;
    margin-bottom: 0.5rem;
}

.session-stats {
    display: flex;
    gap: 1.5rem;
    color: var(--text-secondary);
    font-size: 0.9rem;
    margin-bottom: 0.75rem;
}

.session-links {
    display: flex;
    gap: 1rem;
}

.session-links a {
    color: var(--link-color);
    text-decoration: none;
    font-size: 0.9rem;
}

.session-links a:hover {
    text-decoration: underline;
}
`

// sanitizeName replaces characters that are invalid in filenames.
func sanitizeName(name string) string {
	result := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			result = append(result, c)
		} else if c == '/' {
			result = append(result, '-')
		}
	}
	return string(result)
}
