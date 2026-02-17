package agents

import (
	"context"
	"time"
)

// Cost unit constants - use cost.UnitUSD, cost.UnitCredits, cost.UnitPremiumRequests
// from the internal/cost package for setting CostMetrics.CostUnit values.

// Agent defines the interface for AI coding agent implementations.
// All supported agents (Claude Code, Codex, Kiro, Copilot) implement this interface.
type Agent interface {
	// Identity methods
	Name() string       // e.g., "claude-code", "codex", "kiro", "copilot"
	CLICommand() string // Command to execute (may be path or name, resolved via exec.LookPath)

	// Capability detection
	IsInstalled() bool
	Version() (string, error)

	// Session management
	// Note: Session parsing is handled by internal/transcript package, not agents.
	// Format detection uses content-based approach (see DetectFormat).
	DefaultSessionDir() string
	DiscoverSessions(ctx context.Context, projectDir string) ([]SessionInfo, error)

	// Execution
	Run(ctx context.Context, opts RunOptions) (*RunResult, error)
	Resume(ctx context.Context, sessionID string, opts RunOptions) (*RunResult, error)
}

// SessionExporter is an optional interface for agents that require explicit session export.
// Kiro implements this interface because it doesn't store sessions automatically.
type SessionExporter interface {
	ExportSession(ctx context.Context, filename string) error
}

// SessionInfo contains metadata about a discovered session.
type SessionInfo struct {
	ID        string
	Agent     string
	Path      string
	CreatedAt time.Time
	Size      int64
	Project   string
}

// RunOptions configures an execution.
type RunOptions struct {
	Prompt      string
	WorkDir     string
	SessionID   string            // Pre-generated UUID for tracking
	Timeout     time.Duration     // 0 = no timeout
	Env         map[string]string
	ExtraArgs   []string          // Agent-specific CLI arguments
}

// RunResult contains the outcome of an execution.
type RunResult struct {
	SessionID  string
	ExitCode   int
	Duration   time.Duration
	Cost       *CostMetrics
	LogPath    string
	Output     string
	Stderr     string
	RawJSON    []byte
	Error      error
	ErrorClass ErrorClass
	NumTurns   int      // Number of conversation turns
	IsError    bool     // Whether the agent reported an error condition
	Errors     []string // Error messages from agent output
}

// CostMetrics tracks usage costs in agent-specific units.
type CostMetrics struct {
	// Token-based (Claude, Codex, Copilot)
	InputTokens  int
	OutputTokens int
	CachedTokens int // Copilot cached tokens
	TotalTokens  int

	// Credit-based (Kiro)
	Credits float64

	// Premium request-based (Copilot)
	PremiumRequests float64 // float64 because Copilot outputs fractional values

	// Time metrics
	APIDuration     *time.Duration // API time spent (pointer for optional)
	SessionDuration *time.Duration // Session time (pointer for optional)

	// Code change metrics
	LinesAdded   *int // Pointer for optional
	LinesRemoved *int // Pointer for optional

	// Cost unit
	CostUnit string // "USD", "credits", or "premium_requests"

	// Universal (kept for backward compat)
	CostUSD float64
}
