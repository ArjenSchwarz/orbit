// Package testutil provides testing utilities for Orbit integration tests.
package testutil

import (
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
)

// AgentCall represents a recorded call to Run or Resume.
type AgentCall struct {
	Index       int               // 0-based call index
	Method      string            // "Run" or "Resume"
	Options     agents.RunOptions // Copy of options passed
	SessionID   string            // For Resume calls
	Timestamp   time.Time         // When call was made (uses clock if provided)
	HasDeadline bool              // Whether context had a deadline
	Deadline    time.Time         // Context deadline if present
}

// CallResponse defines what the agent returns for one call.
type CallResponse struct {
	Result     *agents.RunResult
	Delay      time.Duration
	ErrorClass agents.ErrorClass
	Output     string // Agent stdout
	Stderr     string // Agent stderr
	CustomFunc func(*AgentCall) *CallResponse // For dynamic behavior (rare)
}

// Scenario holds an immutable sequence of responses.
type Scenario struct {
	responses []CallResponse
}

// Responses returns the scenario's responses.
// This is intentionally read-only access after Build().
func (s *Scenario) Responses() []CallResponse {
	return s.responses
}

// Len returns the number of responses in the scenario.
func (s *Scenario) Len() int {
	return len(s.responses)
}

// Clock interface for time operations.
// This matches the Clock interface defined in internal/orbit/orbit.go.
type Clock interface {
	Now() time.Time
	Sleep(d time.Duration)
}
