// Package logs provides session log management for Orbit.
package logs

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/arjenschwarz/orbit/internal/claude"
	"github.com/google/uuid"
)

// transcriptEntry represents a line in the Claude session JSONL.
type transcriptEntry struct {
	Type      string         `json:"type"`
	Message   *transcriptMsg `json:"message,omitempty"`
	Timestamp string         `json:"timestamp,omitempty"`
	SessionID string         `json:"sessionId,omitempty"`
}

// transcriptMsg represents the message content.
type transcriptMsg struct {
	Role    string        `json:"role"`
	Content []contentItem `json:"content"`
}

// contentItem represents a content block in a message.
type contentItem struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Thinking string `json:"thinking,omitempty"`
	Name     string `json:"name,omitempty"`
	Input    any    `json:"input,omitempty"`
	Content  string `json:"content,omitempty"`
	IsError  bool   `json:"is_error,omitempty"`
}

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
	return m.writeSummary()
}

// SavePostCompletionSession saves the post-command session with distinct naming.
func (m *Manager) SavePostCompletionSession(result *claude.SessionResult, startTime time.Time) error {
	endTime := time.Now()
	baseName := m.postCompletionFileName()

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

	return nil
}

// copyPostCompletionTranscript copies the full session transcript for post-completion.
func (m *Manager) copyPostCompletionTranscript(sessionID, baseName string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	projectPath := strings.ReplaceAll(m.workingDir, "/", "-")
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

// generatePostCompletionMarkdownTranscript parses a JSONL transcript and writes Markdown for post-completion.
func (m *Manager) generatePostCompletionMarkdownTranscript(srcPath, dstPath, sessionID string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open transcript: %w", err)
	}
	defer func() { _ = src.Close() }()

	var sb strings.Builder
	sb.WriteString("# Post-Completion Session Transcript\n\n")
	sb.WriteString(fmt.Sprintf("**Session ID:** `%s`\n\n", sessionID))
	sb.WriteString("---\n\n")

	scanner := bufio.NewScanner(src)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry transcriptEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		switch entry.Type {
		case "user":
			sb.WriteString(formatUserMessage(&entry))
		case "assistant":
			sb.WriteString(formatAssistantMessage(&entry))
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan transcript: %w", err)
	}

	return os.WriteFile(dstPath, []byte(sb.String()), 0644)
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
	return m.writeSummary()
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
	// where project-path has slashes replaced with dashes
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	projectPath := strings.ReplaceAll(m.workingDir, "/", "-")
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

// generateMarkdownTranscript parses a JSONL transcript and writes a Markdown file.
func (m *Manager) generateMarkdownTranscript(srcPath, dstPath string, phase int, sessionID string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open transcript: %w", err)
	}
	defer func() { _ = src.Close() }()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Phase %d Session Transcript\n\n", phase))
	sb.WriteString(fmt.Sprintf("**Session ID:** `%s`\n\n", sessionID))
	sb.WriteString("---\n\n")

	scanner := bufio.NewScanner(src)
	// Increase buffer size for long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry transcriptEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue // Skip malformed lines
		}

		switch entry.Type {
		case "user":
			sb.WriteString(formatUserMessage(&entry))
		case "assistant":
			sb.WriteString(formatAssistantMessage(&entry))
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan transcript: %w", err)
	}

	return os.WriteFile(dstPath, []byte(sb.String()), 0644)
}

// formatUserMessage formats a user message as Markdown.
func formatUserMessage(entry *transcriptEntry) string {
	if entry.Message == nil || len(entry.Message.Content) == 0 {
		return ""
	}

	// Collect text content first to check if there's anything to output
	var texts []string
	for _, item := range entry.Message.Content {
		if item.Text != "" {
			texts = append(texts, item.Text)
		}
	}

	// Skip if no actual text content
	if len(texts) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## 👤 User\n\n")

	for _, text := range texts {
		sb.WriteString(text)
		sb.WriteString("\n\n")
	}

	sb.WriteString("---\n\n")
	return sb.String()
}

// formatAssistantMessage formats an assistant message as Markdown.
func formatAssistantMessage(entry *transcriptEntry) string {
	if entry.Message == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## 🤖 Assistant\n\n")

	for _, item := range entry.Message.Content {
		switch item.Type {
		case "thinking":
			if item.Thinking != "" {
				sb.WriteString("<details>\n<summary>💭 Thinking</summary>\n\n")
				sb.WriteString(item.Thinking)
				sb.WriteString("\n\n</details>\n\n")
			}

		case "text":
			if item.Text != "" {
				sb.WriteString(item.Text)
				sb.WriteString("\n\n")
			}

		case "tool_use":
			sb.WriteString(fmt.Sprintf("### 🔧 Tool: `%s`\n\n", item.Name))
			if item.Input != nil {
				inputJSON, err := json.MarshalIndent(item.Input, "", "  ")
				if err == nil {
					sb.WriteString("```json\n")
					// Truncate very long inputs
					inputStr := string(inputJSON)
					if len(inputStr) > 2000 {
						inputStr = inputStr[:2000] + "\n... (truncated)"
					}
					sb.WriteString(inputStr)
					sb.WriteString("\n```\n\n")
				}
			}

		case "tool_result":
			content := item.Content
			if len(content) > 3000 {
				content = content[:3000] + "\n... (truncated)"
			}
			if item.IsError {
				sb.WriteString("#### ❌ Tool Error\n\n")
			} else {
				sb.WriteString("#### ✅ Tool Result\n\n")
			}
			sb.WriteString("```\n")
			sb.WriteString(content)
			sb.WriteString("\n```\n\n")
		}
	}

	sb.WriteString("---\n\n")
	return sb.String()
}

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
