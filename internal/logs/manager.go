// Package logs provides session log management for Orbit.
package logs

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/arjenschwarz/orbit/internal/claude"
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
}

// Manager handles log storage and retrieval.
type Manager struct {
	baseDir    string
	sessionDir string
	workingDir string
	summary    Summary
}

// NewManager creates a new log manager with a timestamped session directory.
// workingDir is used to locate Claude session transcripts in ~/.claude/projects.
func NewManager(baseDir, branchName, workingDir string) (*Manager, error) {
	// Create timestamped directory
	timestamp := time.Now().Format("2006-01-02-150405")
	sessionDir := filepath.Join(baseDir, fmt.Sprintf("%s-%s", timestamp, sanitizeName(branchName)))

	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	m := &Manager{
		baseDir:    baseDir,
		sessionDir: sessionDir,
		workingDir: workingDir,
		summary: Summary{
			StartedAt: time.Now(),
			Status:    "running",
			Sessions:  []SessionEntry{},
		},
	}

	// Write initial summary
	if err := m.writeSummary(); err != nil {
		return nil, err
	}

	return m, nil
}

// SessionDir returns the current session directory path.
func (m *Manager) SessionDir() string {
	return m.sessionDir
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
	}

	m.summary.Sessions = append(m.summary.Sessions, entry)
	m.summary.PhasesCompleted = phase
	m.summary.TotalCostUSD += result.Cost
	m.summary.TotalDurationMS += result.Duration.Milliseconds()

	// Write session JSON (result only)
	jsonPath := filepath.Join(m.sessionDir, fmt.Sprintf("phase-%d-session.json", phase))
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
	txtPath := filepath.Join(m.sessionDir, fmt.Sprintf("phase-%d-session.txt", phase))
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
	jsonlDstPath := filepath.Join(m.sessionDir, fmt.Sprintf("phase-%d-transcript.jsonl", phase))
	if err := copyFile(srcPath, jsonlDstPath); err != nil {
		return err
	}

	// Parse and generate Markdown
	mdDstPath := filepath.Join(m.sessionDir, fmt.Sprintf("phase-%d-transcript.md", phase))
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
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination: %w", err)
	}
	defer dstFile.Close()

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
	defer src.Close()

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
