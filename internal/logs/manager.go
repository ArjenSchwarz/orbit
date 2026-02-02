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

	"github.com/arjenschwarz/orbit/internal/agents"
	"github.com/arjenschwarz/orbit/internal/agents/claudecode"
	"github.com/arjenschwarz/orbit/internal/cost"
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

// PrePromptState tracks pre-prompt execution for crash recovery.
type PrePromptState struct {
	SessionID   string     `json:"session_id"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Status      string     `json:"status"` // "started", "completed"
}

// PrePromptStatus constants for tracking pre-prompt state.
const (
	PrePromptStatusNotStarted = ""          // nil PrePrompt in summary
	PrePromptStatusStarted    = "started"   // Started but not completed
	PrePromptStatusCompleted  = "completed" // Successfully completed
)

// ShellCommandState tracks shell command execution.
type ShellCommandState struct {
	Command     string    `json:"command"`
	ExitCode    int       `json:"exit_code"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	DurationMS  int64     `json:"duration_ms"`
}

// SessionEntry records metadata about a completed Claude session.
type SessionEntry struct {
	Phase      int       `json:"phase"`
	SessionID  string    `json:"session_id"`
	DurationMS int64     `json:"duration_ms"`
	CostUSD    float64   `json:"cost_usd"`              // Kept for backward compat
	CostValue  float64   `json:"cost_value,omitempty"`  // NEW: actual cost value
	CostUnit   string    `json:"cost_unit,omitempty"`   // NEW: unit type ("USD", "credits", "premium_requests")
	NumTurns   int       `json:"num_turns"`
	StartedAt  time.Time `json:"started_at"`
	EndedAt    time.Time `json:"ended_at"`
	IsError    bool      `json:"is_error,omitempty"`
	RunNumber  int       `json:"run_number"`
	AgentAlias string    `json:"agent_alias,omitempty"` // Agent alias used for this session
	AgentType  string    `json:"agent_type,omitempty"`  // Underlying agent type (e.g., "claude-code")
	Model      string    `json:"model,omitempty"`       // Model used for this session
}

// GetCost returns the cost value and unit, handling backward compatibility.
//
// Decision logic:
//  1. If CostUnit is set (non-empty), this is a new-format entry → use CostValue + CostUnit
//  2. If CostUnit is empty but AgentType is set, this is legacy → use CostUSD + infer unit
//  3. If both are empty, this is legacy with unknown agent → use CostUSD + "USD"
//
// This ensures zero-cost new-format entries (CostValue=0, CostUnit="premium_requests")
// are handled correctly and don't fall through to legacy inference.
func (e *SessionEntry) GetCost() (float64, string) {
	// New format: CostUnit is explicitly set
	if e.CostUnit != "" {
		return e.CostValue, e.CostUnit
	}

	// Legacy format: infer unit from agent type
	unit := cost.InferUnitFromAgent(e.AgentType)
	if e.AgentType == "" {
		// Both CostUnit and AgentType are empty - default to USD with warning
		debugLog("SessionEntry has no cost_unit or agent_type, defaulting to USD")
	}
	return e.CostUSD, unit
}

// debugLog logs a message if ORBIT_DEBUG is enabled.
func debugLog(format string, args ...any) {
	if env := os.Getenv("ORBIT_DEBUG"); env == "true" || env == "1" {
		log.Printf("[logs] "+format, args...)
	}
}

// Summary contains the overall orchestration run summary.
type Summary struct {
	StartedAt       time.Time      `json:"started_at"`
	CompletedAt     *time.Time     `json:"completed_at,omitempty"`
	Status          string         `json:"status"`
	PhasesCompleted int            `json:"phases_completed"`
	TotalCostUSD    float64        `json:"total_cost_usd"`            // Kept for backward compat
	CostTotals      *cost.Totals   `json:"cost_totals,omitempty"`     // NEW: aggregated costs by unit
	TotalDurationMS int64          `json:"total_duration_ms"`
	Sessions        []SessionEntry `json:"sessions"`
	Error           string         `json:"error,omitempty"`
	// New fields for session management
	CurrentPhase   *PhaseState          `json:"current_phase,omitempty"`
	PostCompletion *PostCompletionState `json:"post_completion,omitempty"`
	RunNumber      int                  `json:"run_number"`
	BranchName     string               `json:"branch_name,omitempty"`
	// Fields for pre-prompt and shell command tracking
	PrePrompt   *PrePromptState     `json:"pre_prompt,omitempty"`
	PreCommand  *ShellCommandState  `json:"pre_command,omitempty"`
	PostCommand *ShellCommandState  `json:"post_command,omitempty"`
}

// GetCostTotals returns aggregated costs, computing from sessions if needed.
// If CostTotals is already set, returns it directly.
// Otherwise, computes totals from session entries for backward compatibility.
func (s *Summary) GetCostTotals() cost.Totals {
	if s.CostTotals != nil {
		return *s.CostTotals
	}

	// Compute from sessions (backward compat)
	var totals cost.Totals
	for _, session := range s.Sessions {
		value, unit := session.GetCost()
		switch unit {
		case cost.UnitUSD:
			totals.USD += value
		case cost.UnitCredits:
			totals.Credits += value
		case cost.UnitPremiumRequests:
			totals.PremiumRequests += value
		}
	}
	return totals
}

// AgentInfo holds the resolved agent configuration for the current run.
type AgentInfo struct {
	Alias string // Agent alias name used
	Type  string // Underlying agent type (e.g., "claude-code")
	Model string // Model used (optional)
}

// Manager handles log storage and retrieval.
type Manager struct {
	baseDir    string
	sessionDir string
	workingDir string
	summary    Summary
	useSubdirs bool      // controls directory mode
	branchName string    // stored for branch mismatch warning
	agentInfo  AgentInfo // agent context for session logging
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
	// Ensure workingDir is absolute for correct Claude project path resolution
	if workingDir != "" && !filepath.IsAbs(workingDir) {
		absWorkingDir, err := filepath.Abs(workingDir)
		if err == nil {
			workingDir = absWorkingDir
		}
		// If Abs fails, continue with relative path (best effort)
	}

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

// SetAgentInfo sets the agent context for session logging.
// Call this once after agent resolution to include agent metadata in session entries.
func (m *Manager) SetAgentInfo(alias, agentType, model string) {
	m.agentInfo = AgentInfo{
		Alias: alias,
		Type:  agentType,
		Model: model,
	}
}

// RunNumber returns the current run number for file naming purposes.
func (m *Manager) RunNumber() int {
	return m.summary.RunNumber
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
// If overrideSessionID is provided (non-empty) and phase is 1, that session ID is used
// and isResume is set to true (to continue the pre-prompt session).
// Returns the session ID, whether this is a resume, and any error.
func (m *Manager) StartPhase(phase int, continueSession bool, overrideSessionID ...string) (string, bool, error) {
	// Check for override session ID (from pre-prompt) for phase 1
	if len(overrideSessionID) > 0 && overrideSessionID[0] != "" && phase == 1 {
		sessionID := overrideSessionID[0]
		// Record that we're using the pre-prompt session
		m.summary.CurrentPhase = &PhaseState{
			Phase:     phase,
			SessionID: sessionID,
			StartedAt: time.Now(),
		}
		if err := m.writeSummary(); err != nil {
			return "", false, err
		}
		// Return the override session ID with isResume=true since we're continuing pre-prompt session
		return sessionID, true, nil
	}

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

// GetPrePromptState returns the pre-prompt session ID and status for resumption decisions.
// Returns: sessionID, status (one of PrePromptStatus* constants)
func (m *Manager) GetPrePromptState() (sessionID string, status string) {
	if m.summary.PrePrompt == nil {
		return "", PrePromptStatusNotStarted
	}
	return m.summary.PrePrompt.SessionID, m.summary.PrePrompt.Status
}

// StartPrePrompt begins pre-prompt execution tracking.
// Returns the session ID, whether this is a resume, and any error.
func (m *Manager) StartPrePrompt(continueSession bool) (string, bool, error) {
	// Check for existing pre-prompt state
	if m.summary.PrePrompt != nil {
		if m.summary.PrePrompt.Status == PrePromptStatusStarted && continueSession {
			// Resume existing session that was interrupted
			return m.summary.PrePrompt.SessionID, true, nil
		}
		if m.summary.PrePrompt.Status == PrePromptStatusCompleted {
			// Already completed - caller should have checked GetPrePromptState first
			return m.summary.PrePrompt.SessionID, false, nil
		}
		// Status is "started" but not continuing - clear and start fresh
		m.summary.PrePrompt = nil
	}

	// Generate new session ID
	sessionID := uuid.NewString()

	// Record pre-prompt start BEFORE invoking agent
	m.summary.PrePrompt = &PrePromptState{
		SessionID: sessionID,
		StartedAt: time.Now(),
		Status:    PrePromptStatusStarted,
	}

	if err := m.writeSummary(); err != nil {
		return "", false, err
	}

	return sessionID, false, nil
}

// CompletePrePrompt marks pre-prompt as completed with the given session ID.
func (m *Manager) CompletePrePrompt(sessionID string) error {
	if m.summary.PrePrompt == nil {
		return nil
	}
	now := time.Now()
	m.summary.PrePrompt.CompletedAt = &now
	m.summary.PrePrompt.Status = PrePromptStatusCompleted
	m.summary.PrePrompt.SessionID = sessionID // Update in case agent returned different ID
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

// RecordShellCommand records a shell command execution in summary.json.
// The name parameter should be "pre-command" or "post-command".
func (m *Manager) RecordShellCommand(name, command string, exitCode int, startedAt, completedAt time.Time, duration time.Duration) error {
	state := &ShellCommandState{
		Command:     command,
		ExitCode:    exitCode,
		StartedAt:   startedAt,
		CompletedAt: completedAt,
		DurationMS:  duration.Milliseconds(),
	}

	switch name {
	case "pre-command":
		m.summary.PreCommand = state
	case "post-command":
		m.summary.PostCommand = state
	default:
		// Unknown command name, ignore
		return nil
	}

	return m.writeSummary()
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

// SaveSession records a completed agent session.
func (m *Manager) SaveSession(phase int, result *agents.RunResult, startTime time.Time) error {
	endTime := time.Now()

	// Extract cost value and unit from CostMetrics
	var costValue float64
	var costUnit string

	if result.Cost != nil {
		costUnit = result.Cost.CostUnit
		switch costUnit {
		case cost.UnitCredits:
			costValue = result.Cost.Credits
		case cost.UnitPremiumRequests:
			costValue = result.Cost.PremiumRequests
		default:
			// Default to USD for backward compatibility
			costValue = result.Cost.CostUSD
			costUnit = cost.UnitUSD
		}
	}

	entry := SessionEntry{
		Phase:      phase,
		SessionID:  result.SessionID,
		DurationMS: result.Duration.Milliseconds(),
		CostUSD:    costValue, // Write to both for backward compat
		CostValue:  costValue,
		CostUnit:   costUnit,
		NumTurns:   result.NumTurns,
		StartedAt:  startTime,
		EndedAt:    endTime,
		IsError:    result.IsError,
		RunNumber:  m.summary.RunNumber,
		AgentAlias: m.agentInfo.Alias,
		AgentType:  m.agentInfo.Type,
		Model:      m.agentInfo.Model,
	}

	m.summary.Sessions = append(m.summary.Sessions, entry)
	m.summary.PhasesCompleted = phase

	// Update CostTotals aggregation by unit
	if m.summary.CostTotals == nil {
		m.summary.CostTotals = &cost.Totals{}
	}
	switch costUnit {
	case cost.UnitUSD:
		m.summary.CostTotals.USD += costValue
	case cost.UnitCredits:
		m.summary.CostTotals.Credits += costValue
	case cost.UnitPremiumRequests:
		m.summary.CostTotals.PremiumRequests += costValue
	}

	// Backward compatibility: TotalCostUSD receives ALL cost values regardless of unit.
	// This ensures old Orbit versions see a non-zero total for runs using Kiro/Copilot.
	m.summary.TotalCostUSD += costValue
	m.summary.TotalDurationMS += result.Duration.Milliseconds()

	// Write session JSON (result only)
	jsonPath := filepath.Join(m.sessionDir, m.phaseFileName(phase, "session.json"))
	if err := os.WriteFile(jsonPath, result.RawJSON, 0644); err != nil {
		return fmt.Errorf("failed to write session JSON: %w", err)
	}

	// Copy full transcript from ~/.claude/projects if available (Claude Code only)
	// Other agents use SessionExporter interface for transcript export
	if result.SessionID != "" && m.agentInfo.Type == "claude-code" {
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
func (m *Manager) SavePostCompletionSession(result *agents.RunResult, startTime time.Time) error {
	endTime := time.Now()
	baseName := m.postCompletionFileName()

	// Extract cost value and unit from CostMetrics
	var costValue float64
	var costUnit string

	if result.Cost != nil {
		costUnit = result.Cost.CostUnit
		switch costUnit {
		case cost.UnitCredits:
			costValue = result.Cost.Credits
		case cost.UnitPremiumRequests:
			costValue = result.Cost.PremiumRequests
		default:
			// Default to USD for backward compatibility
			costValue = result.Cost.CostUSD
			costUnit = cost.UnitUSD
		}
	}

	// Add session entry to summary (Phase 0 indicates post-completion)
	entry := SessionEntry{
		Phase:      0, // Post-completion marker
		SessionID:  result.SessionID,
		DurationMS: result.Duration.Milliseconds(),
		CostUSD:    costValue, // Write to both for backward compat
		CostValue:  costValue,
		CostUnit:   costUnit,
		NumTurns:   result.NumTurns,
		StartedAt:  startTime,
		EndedAt:    endTime,
		IsError:    result.IsError,
		RunNumber:  m.summary.RunNumber,
		AgentAlias: m.agentInfo.Alias,
		AgentType:  m.agentInfo.Type,
		Model:      m.agentInfo.Model,
	}
	m.summary.Sessions = append(m.summary.Sessions, entry)

	// Update CostTotals aggregation by unit
	if m.summary.CostTotals == nil {
		m.summary.CostTotals = &cost.Totals{}
	}
	switch costUnit {
	case cost.UnitUSD:
		m.summary.CostTotals.USD += costValue
	case cost.UnitCredits:
		m.summary.CostTotals.Credits += costValue
	case cost.UnitPremiumRequests:
		m.summary.CostTotals.PremiumRequests += costValue
	}

	// Backward compatibility: TotalCostUSD receives ALL cost values regardless of unit.
	m.summary.TotalCostUSD += costValue
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

	// Copy full transcript from ~/.claude/projects if available (Claude Code only)
	// Other agents use SessionExporter interface for transcript export
	if result.SessionID != "" && m.agentInfo.Type == "claude-code" {
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

	projectPath := claudecode.BuildProjectPath(m.workingDir)
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

	// Copy cost metadata from ParseResult if available
	if result.Metadata != nil {
		opts.TotalCost = result.Metadata.TotalCost
		opts.CostUnit = result.Metadata.CostUnit
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
func formatPostCompletionTranscript(result *agents.RunResult, start, end time.Time) string {
	costStr := "-"
	if result.Cost != nil {
		unit := result.Cost.CostUnit
		var value float64
		switch unit {
		case cost.UnitCredits:
			value = result.Cost.Credits
		case cost.UnitPremiumRequests:
			value = result.Cost.PremiumRequests
		default:
			value = result.Cost.CostUSD
			unit = cost.UnitUSD
		}
		costStr = cost.FormatWithPrecision(value, unit, 4)
	}
	return fmt.Sprintf(`Orbit Post-Completion Session Log
========================================

Session ID: %s
Started:    %s
Ended:      %s
Duration:   %s
Cost:       %s
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
		costStr,
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
func formatTranscript(phase int, result *agents.RunResult, start, end time.Time) string {
	costStr := "-"
	if result.Cost != nil {
		unit := result.Cost.CostUnit
		var value float64
		switch unit {
		case cost.UnitCredits:
			value = result.Cost.Credits
		case cost.UnitPremiumRequests:
			value = result.Cost.PremiumRequests
		default:
			value = result.Cost.CostUSD
			unit = cost.UnitUSD
		}
		costStr = cost.FormatWithPrecision(value, unit, 4)
	}
	return fmt.Sprintf(`Orbit Session Log - Phase %d
========================================

Session ID: %s
Started:    %s
Ended:      %s
Duration:   %s
Cost:       %s
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
		costStr,
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

	projectPath := claudecode.BuildProjectPath(m.workingDir)
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

	// Copy cost metadata from ParseResult if available
	if result.Metadata != nil {
		opts.TotalCost = result.Metadata.TotalCost
		opts.CostUnit = result.Metadata.CostUnit
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
	sb.WriteString(fmt.Sprintf("- **Total Cost:** %s\n", cost.FormatTotals(m.summary.GetCostTotals())))
	if m.summary.Error != "" {
		sb.WriteString(fmt.Sprintf("- **Error:** %s\n", m.summary.Error))
	}
	sb.WriteString("\n")

	// Pre-command status
	if m.summary.PreCommand != nil {
		statusIcon := "✅"
		if m.summary.PreCommand.ExitCode != 0 {
			statusIcon = "❌"
		}
		sb.WriteString("### Pre-Command\n\n")
		sb.WriteString(fmt.Sprintf("- %s **Command:** `%s`\n", statusIcon, m.summary.PreCommand.Command))
		sb.WriteString(fmt.Sprintf("- **Exit Code:** %d\n", m.summary.PreCommand.ExitCode))
		sb.WriteString(fmt.Sprintf("- **Duration:** %s\n",
			time.Duration(m.summary.PreCommand.DurationMS*int64(time.Millisecond)).Round(time.Second)))
		sb.WriteString("\n")
	}

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

			costValue, costUnit := session.GetCost()
			sb.WriteString(fmt.Sprintf("- %s **Session%s** - Cost: %s, Duration: %s, Turns: %d\n",
				statusIcon, runLabel, cost.FormatWithPrecision(costValue, costUnit, 4),
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
			costValue, costUnit := session.GetCost()
			sb.WriteString(fmt.Sprintf("- %s **Session** - Cost: %s, Duration: %s, Turns: %d\n",
				statusIcon, cost.FormatWithPrecision(costValue, costUnit, 4),
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

	// Post-command status
	if m.summary.PostCommand != nil {
		statusIcon := "✅"
		if m.summary.PostCommand.ExitCode != 0 {
			statusIcon = "❌"
		}
		sb.WriteString("### Post-Command\n\n")
		sb.WriteString(fmt.Sprintf("- %s **Command:** `%s`\n", statusIcon, m.summary.PostCommand.Command))
		sb.WriteString(fmt.Sprintf("- **Exit Code:** %d\n", m.summary.PostCommand.ExitCode))
		sb.WriteString(fmt.Sprintf("- **Duration:** %s\n",
			time.Duration(m.summary.PostCommand.DurationMS*int64(time.Millisecond)).Round(time.Second)))
		sb.WriteString("\n")
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
	sb.WriteString(fmt.Sprintf("                <dt>Total Cost</dt><dd>%s</dd>\n",
		cost.FormatTotals(m.summary.GetCostTotals())))
	if m.summary.Error != "" {
		sb.WriteString(fmt.Sprintf("                <dt>Error</dt><dd class=\"error-text\">%s</dd>\n",
			html.EscapeString(m.summary.Error)))
	}
	sb.WriteString("            </dl>\n")
	sb.WriteString("        </section>\n")

	// Pre-command section
	if m.summary.PreCommand != nil {
		sb.WriteString("        <section class=\"shell-command\">\n")
		sb.WriteString("            <h2>Pre-Command</h2>\n")
		statusIcon := "✅"
		cardClass := "command-card"
		if m.summary.PreCommand.ExitCode != 0 {
			statusIcon = "❌"
			cardClass += " error"
		}
		sb.WriteString(fmt.Sprintf("            <div class=\"%s\">\n", cardClass))
		sb.WriteString(fmt.Sprintf("                <div class=\"command-header\">%s <code>%s</code></div>\n",
			statusIcon, html.EscapeString(m.summary.PreCommand.Command)))
		sb.WriteString("                <div class=\"command-stats\">\n")
		sb.WriteString(fmt.Sprintf("                    <span>Exit Code: %d</span>\n", m.summary.PreCommand.ExitCode))
		sb.WriteString(fmt.Sprintf("                    <span>Duration: %s</span>\n",
			time.Duration(m.summary.PreCommand.DurationMS*int64(time.Millisecond)).Round(time.Second)))
		sb.WriteString("                </div>\n")
		sb.WriteString("            </div>\n")
		sb.WriteString("        </section>\n")
	}

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
			costValue, costUnit := session.GetCost()
			sb.WriteString(fmt.Sprintf("                        <span>Cost: %s</span>\n", cost.FormatWithPrecision(costValue, costUnit, 4)))
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
			costValue, costUnit := session.GetCost()
			sb.WriteString(fmt.Sprintf("                        <span>Cost: %s</span>\n", cost.FormatWithPrecision(costValue, costUnit, 4)))
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

	// Post-command section
	if m.summary.PostCommand != nil {
		sb.WriteString("        <section class=\"shell-command\">\n")
		sb.WriteString("            <h2>Post-Command</h2>\n")
		statusIcon := "✅"
		cardClass := "command-card"
		if m.summary.PostCommand.ExitCode != 0 {
			statusIcon = "❌"
			cardClass += " error"
		}
		sb.WriteString(fmt.Sprintf("            <div class=\"%s\">\n", cardClass))
		sb.WriteString(fmt.Sprintf("                <div class=\"command-header\">%s <code>%s</code></div>\n",
			statusIcon, html.EscapeString(m.summary.PostCommand.Command)))
		sb.WriteString("                <div class=\"command-stats\">\n")
		sb.WriteString(fmt.Sprintf("                    <span>Exit Code: %d</span>\n", m.summary.PostCommand.ExitCode))
		sb.WriteString(fmt.Sprintf("                    <span>Duration: %s</span>\n",
			time.Duration(m.summary.PostCommand.DurationMS*int64(time.Millisecond)).Round(time.Second)))
		sb.WriteString("                </div>\n")
		sb.WriteString("            </div>\n")
		sb.WriteString("        </section>\n")
	}

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

.shell-command {
    margin-bottom: 2rem;
}

.command-card {
    background-color: var(--bg-card);
    border: 1px solid var(--border-color);
    border-radius: 8px;
    padding: 1rem;
    margin-bottom: 0.75rem;
}

.command-card.error {
    border-left: 4px solid var(--error-color);
}

.command-header {
    font-weight: 600;
    margin-bottom: 0.5rem;
}

.command-header code {
    background-color: var(--bg-secondary);
    padding: 0.2rem 0.5rem;
    border-radius: 4px;
    font-size: 0.9rem;
}

.command-stats {
    display: flex;
    gap: 1.5rem;
    color: var(--text-secondary);
    font-size: 0.9rem;
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
