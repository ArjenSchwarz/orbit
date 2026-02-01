# Design: Copilot Usage Tracking

## Overview

This design document describes the implementation of usage tracking for GitHub Copilot sessions and fixes for cost display issues across all agents. The implementation follows patterns established by the Kiro agent for usage extraction while introducing a unified cost formatting system.

### Goals

1. Parse Copilot CLI stdout/stderr to extract usage metrics after session completion
2. Introduce cost unit tracking to display costs in their native units (USD, credits, premium requests)
3. Fix the bug where Kiro credits are displayed with a `$` prefix
4. Support aggregated cost display when multiple unit types are present in a run
5. Maintain backward compatibility with existing summary.json files

### Non-Goals

- Converting between cost units
- Per-model token breakdown storage or display
- Parsing Copilot JSONL session files for usage data

## Package Dependencies

The new `internal/cost` package will only depend on standard library packages (`fmt`, `strings`). This ensures no circular dependencies.

**Import Graph (relevant packages):**

```
internal/cost           → (stdlib only)
internal/agents         → (stdlib only)
internal/logs           → internal/agents, internal/cost, internal/transcript
internal/orbit          → internal/agents, internal/cost, internal/logs, ...
internal/web            → internal/logs, internal/cost, ...
internal/report         → internal/cost, ...
```

**Verification:** The `cost` package is a leaf package with no internal dependencies, making circular imports impossible.

**Justification for new package:** The existing `formatCost` in `internal/report/templates.go` is tightly coupled to USD formatting. A dedicated `cost` package provides:
1. Clean separation of cost-related logic
2. Single source of truth for unit constants
3. No risk of circular dependencies (leaf package)
4. Testable in isolation

## Architecture

The implementation spans four layers of the system:

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Agent Layer                                  │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐     │
│  │  Claude Code    │  │      Kiro       │  │     Copilot     │     │
│  │  (CostUSD)      │  │  (Credits)      │  │ (PremiumReqs)   │     │
│  └────────┬────────┘  └────────┬────────┘  └────────┬────────┘     │
│           │                    │                    │               │
│           └────────────────────┼────────────────────┘               │
│                                ▼                                    │
│                    ┌───────────────────────┐                        │
│                    │    CostMetrics        │                        │
│                    │  + CostUnit field     │                        │
│                    └───────────┬───────────┘                        │
└────────────────────────────────┼────────────────────────────────────┘
                                 │
┌────────────────────────────────┼────────────────────────────────────┐
│                         Storage Layer                               │
│                                ▼                                    │
│                    ┌───────────────────────┐                        │
│                    │    SessionEntry       │                        │
│                    │  + CostValue field    │                        │
│                    │  + CostUnit field     │                        │
│                    └───────────┬───────────┘                        │
│                                │                                    │
│                    ┌───────────▼───────────┐                        │
│                    │      Summary          │                        │
│                    │  + CostTotals map     │                        │
│                    └───────────────────────┘                        │
└─────────────────────────────────────────────────────────────────────┘
                                 │
┌────────────────────────────────┼────────────────────────────────────┐
│                        Formatting Layer                             │
│                                ▼                                    │
│                    ┌───────────────────────┐                        │
│                    │    cost.Format()      │                        │
│                    │  Centralized helper   │                        │
│                    └───────────────────────┘                        │
└─────────────────────────────────────────────────────────────────────┘
                                 │
┌────────────────────────────────┼────────────────────────────────────┐
│                         Display Layer                               │
│           ┌────────────┬───────┴───────┬────────────┐              │
│           ▼            ▼               ▼            ▼              │
│     ┌──────────┐ ┌──────────┐   ┌──────────┐ ┌──────────┐         │
│     │ Terminal │ │   Web    │   │ Markdown │ │   HTML   │         │
│     └──────────┘ └──────────┘   └──────────┘ └──────────┘         │
└─────────────────────────────────────────────────────────────────────┘
```

## Components and Interfaces

### 1. Copilot Usage Parser

**Location:** `internal/agents/copilot/usage.go` (new file)

**Purpose:** Parse Copilot CLI output to extract usage metrics.

```go
// UsageInfo contains parsed usage metrics from Copilot CLI output.
type UsageInfo struct {
    PremiumRequests float64
    APIDuration     *time.Duration
    SessionDuration *time.Duration
    InputTokens     int
    OutputTokens    int
    CachedTokens    int
    LinesAdded      *int
    LinesRemoved    *int
}

// ParseUsage extracts usage information from Copilot CLI output.
// It searches both stdout and stderr for the usage summary.
// Returns nil if no usage summary is found.
func ParseUsage(stdout, stderr string) *UsageInfo
```

**Parsing Implementation:**

```go
import (
    "fmt"
    "math"
    "regexp"
    "strconv"
    "strings"
    "time"

    "github.com/arjenschwarz/orbit/internal/debug"
)

var (
    // Numeric pattern: matches valid floats like "0.33", "146.4", "1"
    // Uses \d+(?:\.\d+)? instead of [\d.]+ to reject malformed "1.2.3"
    numericPattern = `\d+(?:\.\d+)?`

    // Case-insensitive patterns with flexible whitespace
    premiumRequestsRe = regexp.MustCompile(`(?i)total\s+usage\s+est:\s+(` + numericPattern + `)\s+premium\s+requests`)

    // Duration patterns: allow optional space after minutes (handles "1m 36s" and "1m36s")
    apiTimeRe     = regexp.MustCompile(`(?i)api\s+time\s+spent:\s+(?:(\d+)m\s*)?(` + numericPattern + `)s`)
    sessionTimeRe = regexp.MustCompile(`(?i)total\s+session\s+time:\s+(?:(\d+)m\s*)?(` + numericPattern + `)s`)

    codeChangesRe = regexp.MustCompile(`(?i)total\s+code\s+changes:\s+\+(\d+)\s+-(\d+)`)

    // Token breakdown regex: matches lines with model usage stats.
    // Anchored to start with whitespace + model name to avoid false positives.
    // Handles optional cached tokens (some models may not report caching).
    // Groups: 1=in_value, 2=in_suffix, 3=out_value, 4=out_suffix, 5=cached_value, 6=cached_suffix
    tokenBreakdownRe = regexp.MustCompile(`(?i)^\s+\S+\s+(` + numericPattern + `)([km])?\s+in,\s+(` + numericPattern + `)([km])?\s+out(?:,\s+(` + numericPattern + `)([km])?\s+cached)?`)
)

func ParseUsage(stdout, stderr string) *UsageInfo {
    // Combine and search both streams
    combined := stdout + "\n" + stderr

    info := &UsageInfo{}
    found := false

    // Extract premium requests (use last match if multiple)
    if matches := premiumRequestsRe.FindAllStringSubmatch(combined, -1); len(matches) > 0 {
        last := matches[len(matches)-1]
        if val, err := strconv.ParseFloat(last[1], 64); err != nil {
            debugLog("Failed to parse premium requests value '%s': %v", last[1], err)
        } else if val < 0 {
            debugLog("Invalid negative premium requests value: %v", val)
        } else {
            info.PremiumRequests = val
            found = true
        }
    }

    // Extract API time
    if matches := apiTimeRe.FindAllStringSubmatch(combined, -1); len(matches) > 0 {
        last := matches[len(matches)-1]
        dur, err := parseDuration(last[1], last[2])
        if err != nil {
            debugLog("Failed to parse API time '%s': %v", last[0], err)
        } else {
            info.APIDuration = &dur
            found = true
        }
    }

    // Extract session time
    if matches := sessionTimeRe.FindAllStringSubmatch(combined, -1); len(matches) > 0 {
        last := matches[len(matches)-1]
        dur, err := parseDuration(last[1], last[2])
        if err != nil {
            debugLog("Failed to parse session time '%s': %v", last[0], err)
        } else {
            info.SessionDuration = &dur
            found = true
        }
    }

    // Extract code changes
    if matches := codeChangesRe.FindAllStringSubmatch(combined, -1); len(matches) > 0 {
        last := matches[len(matches)-1]
        added, err1 := strconv.Atoi(last[1])
        removed, err2 := strconv.Atoi(last[2])
        if err1 != nil || err2 != nil {
            debugLog("Failed to parse code changes '%s': added=%v, removed=%v", last[0], err1, err2)
        } else {
            info.LinesAdded = &added
            info.LinesRemoved = &removed
            found = true
        }
    }

    // Aggregate tokens from all model breakdown lines
    // Process line-by-line to use the anchored regex correctly
    for _, line := range strings.Split(combined, "\n") {
        if match := tokenBreakdownRe.FindStringSubmatch(line); match != nil {
            info.InputTokens += parseTokenValue(match[1], match[2])
            info.OutputTokens += parseTokenValue(match[3], match[4])
            // Cached tokens are optional (groups 5,6 may be empty)
            if match[5] != "" {
                info.CachedTokens += parseTokenValue(match[5], match[6])
            }
            found = true
        }
    }

    if !found {
        debugLog("No usage summary found in Copilot output")
        return nil
    }
    return info
}

// debugLog writes to debug output if debugging is enabled.
// Uses the same debug infrastructure as other agents.
var debugLog = func(format string, args ...any) {
    debug.Log("copilot", format, args...)
}

func parseDuration(minutes, seconds string) (time.Duration, error) {
    var total float64
    if minutes != "" {
        m, err := strconv.Atoi(minutes)
        if err != nil {
            return 0, fmt.Errorf("invalid minutes: %w", err)
        }
        total += float64(m) * 60
    }
    s, err := strconv.ParseFloat(seconds, 64)
    if err != nil {
        return 0, fmt.Errorf("invalid seconds: %w", err)
    }
    total += s
    return time.Duration(total * float64(time.Second)), nil
}

// parseTokenValue parses a token count with optional k/m suffix.
// Uses math.Round to avoid truncation issues (e.g., "1.999k" → 2000, not 1999).
// Returns 0 on parse error (tokens are non-critical, errors logged elsewhere).
func parseTokenValue(value, suffix string) int {
    v, err := strconv.ParseFloat(value, 64)
    if err != nil {
        return 0
    }
    switch strings.ToLower(suffix) {
    case "k":
        v *= 1000
    case "m":
        v *= 1000000
    }
    return int(math.Round(v))
}
```

### 2. CostMetrics Updates

**Location:** `internal/agents/agent.go`

**Changes to existing struct:**

```go
type CostMetrics struct {
    // Token-based (Claude, Codex, Copilot)
    InputTokens  int
    OutputTokens int
    CachedTokens int  // NEW: Copilot cached tokens
    TotalTokens  int

    // Credit-based (Kiro)
    Credits float64

    // Premium request-based (Copilot)
    PremiumRequests float64  // CHANGED: int → float64

    // Time metrics (NEW)
    APIDuration     *time.Duration  // Pointer for optional
    SessionDuration *time.Duration  // Pointer for optional

    // Code change metrics (NEW)
    LinesAdded   *int  // Pointer for optional
    LinesRemoved *int  // Pointer for optional

    // Cost unit (NEW)
    CostUnit string  // "USD", "credits", or "premium_requests"

    // Universal (kept for backward compat)
    CostUSD float64
}
```

**Requirement Traceability:**
- `CachedTokens`: Req 1.7
- `PremiumRequests float64`: Req 1.2
- `APIDuration`, `SessionDuration`: Req 3.3
- `LinesAdded`, `LinesRemoved`: Req 1.5
- `CostUnit`: Req 2.1

**Note on Requirement 6.4 Exception:**

Requirement 6.4 states "no type changes" for backward compatibility. However, `PremiumRequests` changes from `int` to `float64` because Copilot outputs fractional values like "0.33 Premium requests". This exception is documented in the requirements (Section "Struct Changes") and is necessary for correct parsing. The field was previously unused, so no existing code depends on the `int` type.

### 3. Kiro Agent Modification

**Location:** `internal/agents/kiro/agent.go`

**Purpose:** Kiro must set `CostUnit` so `SaveSession` correctly categorizes the cost.

**Current code (lines 210-214):**
```go
if credits := a.extractSessionCredits(ctx, workDir); credits > 0 {
    result.Cost = &agents.CostMetrics{
        Credits: credits,
    }
}
```

**Modified code:**
```go
if credits := a.extractSessionCredits(ctx, workDir); credits > 0 {
    result.Cost = &agents.CostMetrics{
        Credits:  credits,
        CostUnit: "credits",  // NEW: explicit unit type
    }
}
```

This is a one-line change. Without it, `SaveSession` would fall through to the default case and treat Kiro credits as USD.

### 4. Copilot Agent Integration

**Location:** `internal/agents/copilot/agent.go`

**Modified `execute` function (adds usage extraction after CLI execution):**

```go
func (a *Agent) execute(ctx context.Context, opts agents.RunOptions, resume bool) (*agents.RunResult, error) {
    args := a.buildArgs(opts, resume)
    cmd := exec.CommandContext(ctx, a.cliPath, args...)

    // ... existing setup code ...

    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr

    startTime := time.Now()
    err := cmd.Run()
    duration := time.Since(startTime)

    result := &agents.RunResult{
        SessionID: opts.SessionID,
        Duration:  duration,
        Output:    stdout.String(),
        Stderr:    stderr.String(),
    }

    // ... existing error handling ...

    // NEW: Extract usage metrics from CLI output
    if usage := ParseUsage(stdout.String(), stderr.String()); usage != nil {
        result.Cost = &agents.CostMetrics{
            PremiumRequests: usage.PremiumRequests,
            InputTokens:     usage.InputTokens,
            OutputTokens:    usage.OutputTokens,
            CachedTokens:    usage.CachedTokens,
            APIDuration:     usage.APIDuration,
            SessionDuration: usage.SessionDuration,
            LinesAdded:      usage.LinesAdded,
            LinesRemoved:    usage.LinesRemoved,
            CostUnit:        "premium_requests",
        }
        debugLog("Extracted Copilot usage: %.2f premium requests, %d tokens in, %d tokens out",
            usage.PremiumRequests, usage.InputTokens, usage.OutputTokens)
    }

    return result, err
}
```

### 5. Cost Formatting Package

**Location:** `internal/cost/format.go` (new package)

**Purpose:** Centralized cost formatting used by all display contexts (Req 4.6).

```go
package cost

import (
    "fmt"
    "strings"
)

// Unit constants for cost types
const (
    UnitUSD             = "USD"
    UnitCredits         = "credits"
    UnitPremiumRequests = "premium_requests"
)

// Format formats a cost value according to its unit type.
// Returns "-" if value is zero or negative.
// Implements requirements 2.6, 2.7, 2.8, 4.5.
func Format(value float64, unit string) string {
    if value <= 0 {
        return "-"
    }

    switch unit {
    case UnitUSD:
        return fmt.Sprintf("$%.2f", value)
    case UnitCredits:
        return fmt.Sprintf("%.2f credits", value)
    case UnitPremiumRequests:
        return fmt.Sprintf("%.2f premium requests", value)
    default:
        // Unknown unit - format as generic number
        return fmt.Sprintf("%.2f", value)
    }
}

// FormatWithPrecision formats with specified decimal places.
// Used for detailed reports that need more precision.
func FormatWithPrecision(value float64, unit string, precision int) string {
    if value <= 0 {
        return "-"
    }

    format := fmt.Sprintf("%%.%df", precision)
    switch unit {
    case UnitUSD:
        return "$" + fmt.Sprintf(format, value)
    case UnitCredits:
        return fmt.Sprintf(format+" credits", value)
    case UnitPremiumRequests:
        return fmt.Sprintf(format+" premium requests", value)
    default:
        return fmt.Sprintf(format, value)
    }
}

// Totals represents aggregated costs by unit type.
type Totals struct {
    USD             float64
    Credits         float64
    PremiumRequests float64
}

// FormatTotals formats aggregated cost totals.
// Displays in order: USD, credits, premium requests.
// Omits unit types with zero values.
// Implements requirements 5.1, 5.2, 5.3, 5.4, 5.6.
func FormatTotals(totals Totals) string {
    var parts []string

    if totals.USD > 0 {
        parts = append(parts, Format(totals.USD, UnitUSD))
    }
    if totals.Credits > 0 {
        parts = append(parts, Format(totals.Credits, UnitCredits))
    }
    if totals.PremiumRequests > 0 {
        parts = append(parts, Format(totals.PremiumRequests, UnitPremiumRequests))
    }

    if len(parts) == 0 {
        return "-"
    }

    return strings.Join(parts, ", ")
}

// InferUnitFromAgent returns the cost unit for an agent type.
// Used for backward compatibility with legacy summary.json files.
// Implements requirement 2.4.
func InferUnitFromAgent(agentType string) string {
    switch agentType {
    case "kiro":
        return UnitCredits
    case "copilot":
        return UnitPremiumRequests
    default:
        return UnitUSD
    }
}
```

### 6. SessionEntry Updates

**Location:** `internal/logs/manager.go`

**Updated struct:**

```go
type SessionEntry struct {
    Phase      int       `json:"phase"`
    SessionID  string    `json:"session_id"`
    DurationMS int64     `json:"duration_ms"`
    CostUSD    float64   `json:"cost_usd"`              // Kept for backward compat
    CostValue  float64   `json:"cost_value,omitempty"`  // NEW: actual cost value
    CostUnit   string    `json:"cost_unit,omitempty"`   // NEW: unit type
    NumTurns   int       `json:"num_turns"`
    StartedAt  time.Time `json:"started_at"`
    EndedAt    time.Time `json:"ended_at"`
    IsError    bool      `json:"is_error,omitempty"`
    RunNumber  int       `json:"run_number"`
    AgentAlias string    `json:"agent_alias,omitempty"`
    AgentType  string    `json:"agent_type,omitempty"`
    Model      string    `json:"model,omitempty"`
}

// GetCost returns the cost value and unit, handling backward compatibility.
//
// Decision logic:
// 1. If CostUnit is set (non-empty), this is a new-format entry → use CostValue + CostUnit
// 2. If CostUnit is empty but AgentType is set, this is legacy → use CostUSD + infer unit
// 3. If both are empty, this is legacy with unknown agent → use CostUSD + "USD"
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
    if unit == "" {
        // Both CostUnit and AgentType are empty - default to USD with warning
        debugLog("SessionEntry has no cost_unit or agent_type, defaulting to USD")
        unit = cost.UnitUSD
    }
    return e.CostUSD, unit
}
```

**Updated Summary struct:**

```go
type Summary struct {
    RunID           string             `json:"run_id"`
    TasksFile       string             `json:"tasks_file"`
    BranchName      string             `json:"branch_name"`
    StartedAt       time.Time          `json:"started_at"`
    CompletedAt     *time.Time         `json:"completed_at,omitempty"`
    Status          string             `json:"status"`
    TotalPhases     int                `json:"total_phases"`
    PhasesCompleted int                `json:"phases_completed"`
    TotalCostUSD    float64            `json:"total_cost_usd"`      // Kept for backward compat
    CostTotals      *cost.Totals       `json:"cost_totals,omitempty"` // NEW
    Sessions        []SessionEntry     `json:"sessions"`
    Agent           string             `json:"agent,omitempty"`
    AgentVersion    string             `json:"agent_version,omitempty"`
    // ... other fields unchanged ...
}

// GetCostTotals returns aggregated costs, computing from sessions if needed.
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
```

**Updated SaveSession:**

```go
func (m *Manager) SaveSession(phase int, result *agents.RunResult, startTime time.Time) error {
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
            costValue = result.Cost.CostUSD
            costUnit = cost.UnitUSD
        }
    }

    entry := SessionEntry{
        Phase:      phase,
        SessionID:  result.SessionID,
        DurationMS: result.Duration.Milliseconds(),
        CostUSD:    costValue,  // Write to both for backward compat
        CostValue:  costValue,
        CostUnit:   costUnit,
        // ... other fields ...
    }

    m.summary.Sessions = append(m.summary.Sessions, entry)

    // Update totals
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
    // Old versions will display this with a "$" prefix (the bug we're fixing),
    // but at least they show something rather than "$0.00" for non-USD runs.
    // New versions use CostTotals and ignore TotalCostUSD.
    m.summary.TotalCostUSD += costValue

    return m.save()
}
```

### 7. Display Layer Updates

All display locations will use the centralized `cost.Format()` function.

#### Session Duration Fallback (Requirement 3.4)

**Location:** `internal/orbit/orbit.go`

When displaying session duration, fall back to measured execution time if agent-reported duration is unavailable:

```go
// getSessionDuration returns the session duration for display.
// Uses agent-reported SessionDuration if available, otherwise falls back to measured Duration.
func getSessionDuration(result *agents.RunResult) time.Duration {
    if result.Cost != nil && result.Cost.SessionDuration != nil {
        return *result.Cost.SessionDuration
    }
    return result.Duration
}
```

#### Code Changes Display (Requirement 4.7)

**Location:** `internal/cost/format.go`

```go
// FormatCodeChanges formats lines added/removed as "+N/-M lines".
// Returns "-" if both values are nil (data unavailable).
// Returns "+0/-0 lines" if both are explicitly zero (no changes made).
func FormatCodeChanges(added, removed *int) string {
    if added == nil && removed == nil {
        return "-"
    }
    a, r := 0, 0
    if added != nil {
        a = *added
    }
    if removed != nil {
        r = *removed
    }
    return fmt.Sprintf("+%d/-%d lines", a, r)
}
```

**Display locations for code changes:**
- Terminal: Shown in phase completion summary
- Transcripts: Included in session metadata header
- Web: Added to session detail view (future enhancement, not in initial scope)

#### Terminal Display (`internal/orbit/orbit.go`)

```go
import "github.com/arjenschwarz/orbit/internal/cost"

// Replace existing formatCost function
func formatCost(result *agents.RunResult) string {
    if result == nil || result.Cost == nil {
        return "-"
    }

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

    return cost.Format(value, unit)
}
```

#### Web Interface (`internal/web/templates/run_detail.html`)

Update to use the `GetCostTotals` method and format appropriately:

```html
{{if .Summary.CostTotals}}
<dt>Total Cost</dt>
<dd>{{formatCostTotals .Summary.CostTotals}}</dd>
{{else if .Summary.TotalCostUSD}}
<dt>Total Cost</dt>
<dd>${{printf "%.4f" .Summary.TotalCostUSD}}</dd>
{{end}}
```

With template helper in `internal/web/handlers.go`:

```go
template.FuncMap{
    "formatCostTotals": func(totals *cost.Totals) string {
        if totals == nil {
            return "-"
        }
        return cost.FormatTotals(*totals)
    },
}
```

#### Markdown/HTML Export (`internal/logs/manager.go`)

Update `formatTranscript` and report generation to use `cost.Format()`:

```go
func formatTranscript(phase int, result *agents.RunResult, start, end time.Time) string {
    costStr := "-"
    if result.Cost != nil {
        costStr = cost.FormatWithPrecision(
            getCostValue(result.Cost),
            result.Cost.CostUnit,
            4,  // 4 decimal places for transcripts
        )
    }
    // ... use costStr in template ...
}
```

## Data Models

### JSON Schema Changes

#### summary.json

```json
{
  "run_id": "abc123",
  "total_cost_usd": 1.23,
  "cost_totals": {
    "usd": 1.23,
    "credits": 0.45,
    "premium_requests": 0.33
  },
  "sessions": [
    {
      "phase": 1,
      "session_id": "session-1",
      "cost_usd": 1.23,
      "cost_value": 1.23,
      "cost_unit": "USD",
      "agent_type": "claude-code"
    },
    {
      "phase": 2,
      "session_id": "session-2",
      "cost_usd": 0.45,
      "cost_value": 0.45,
      "cost_unit": "credits",
      "agent_type": "kiro"
    }
  ]
}
```

#### Backward Compatibility

Legacy files without `cost_unit` or `cost_value`:

```json
{
  "sessions": [
    {
      "phase": 1,
      "cost_usd": 0.45,
      "agent_type": "kiro"
    }
  ]
}
```

When read:
1. `cost_value` is missing → use `cost_usd`
2. `cost_unit` is missing → infer from `agent_type` ("kiro" → "credits")
3. Result: display as "0.45 credits"

**Concurrency Note:** The `logs.Manager` type is not concurrency-safe. `SaveSession` must not be called from multiple goroutines simultaneously. This is a pre-existing limitation; the current codebase runs variants sequentially with separate managers per worktree. This design does not change the concurrency model.

## Error Handling

### Parse Failures

| Scenario | Handling | Logging |
|----------|----------|---------|
| No usage summary in output | Return nil, leave `Cost` unset | Debug: "No usage summary found in Copilot output" |
| Partial parse (some fields fail) | Set successful fields, leave others nil | Debug: "Failed to parse {field}: {raw_line}" |
| Negative values | Treat as parse error, leave nil | Debug: "Invalid negative value for {field}" |
| Malformed duration format | Leave duration nil | Debug: "Failed to parse duration: {raw}" |

### Storage Failures

| Scenario | Handling |
|----------|----------|
| Missing `cost_unit` in JSON | Infer from `agent_type` |
| Missing both `cost_unit` and `agent_type` | Default to "USD", log debug warning |
| Invalid `cost_unit` value | Treat as "USD" |

### Display Failures

| Scenario | Display |
|----------|---------|
| Nil cost | "-" |
| Zero cost value | "-" (per Decision 2) |
| Unknown unit type | Format as plain number |

## Testing Strategy

### Unit Tests

#### Parser Tests (`internal/agents/copilot/usage_test.go`)

```go
func TestParseUsage(t *testing.T) {
    tests := map[string]struct {
        stdout   string
        stderr   string
        expected *UsageInfo
    }{
        "complete output": {
            stdout: `Total usage est:        0.33 Premium requests
API time spent:         28s
Total session time:     33s
Total code changes:     +10 -5
Breakdown by AI model:
 claude-haiku-4.5        146.4k in, 2.6k out, 88.2k cached`,
            expected: &UsageInfo{
                PremiumRequests: 0.33,
                APIDuration:     ptr(28 * time.Second),
                SessionDuration: ptr(33 * time.Second),
                InputTokens:     146400,
                OutputTokens:    2600,
                CachedTokens:    88200,
                LinesAdded:      intPtr(10),
                LinesRemoved:    intPtr(5),
            },
        },
        "minutes and seconds format": {
            stdout: `Total usage est:        1.5 Premium requests
API time spent:         1m 36.11s
Total session time:     1m 48.964s`,
            expected: &UsageInfo{
                PremiumRequests: 1.5,
                APIDuration:     ptr(96110 * time.Millisecond),
                SessionDuration: ptr(108964 * time.Millisecond),
            },
        },
        "million token suffix": {
            stdout: `Breakdown by AI model:
 claude-haiku-4.5        1.3m in, 8.7k out, 1.3m cached`,
            expected: &UsageInfo{
                InputTokens:  1300000,
                OutputTokens: 8700,
                CachedTokens: 1300000,
            },
        },
        "multiple model lines aggregated": {
            stdout: `Breakdown by AI model:
 claude-haiku-4.5        100k in, 10k out, 50k cached
 gpt-4                   200k in, 20k out, 100k cached`,
            expected: &UsageInfo{
                InputTokens:  300000,
                OutputTokens: 30000,
                CachedTokens: 150000,
            },
        },
        "case insensitive": {
            stdout: `TOTAL USAGE EST:        0.5 PREMIUM REQUESTS
API TIME SPENT:         10s`,
            expected: &UsageInfo{
                PremiumRequests: 0.5,
                APIDuration:     ptr(10 * time.Second),
            },
        },
        "no usage summary": {
            stdout:   "Some other output",
            expected: nil,
        },
        "partial output": {
            stdout: `Total usage est:        0.33 Premium requests`,
            expected: &UsageInfo{
                PremiumRequests: 0.33,
            },
        },
        "in stderr": {
            stderr: `Total usage est:        0.33 Premium requests`,
            expected: &UsageInfo{
                PremiumRequests: 0.33,
            },
        },
        "last occurrence used": {
            stdout: `Total usage est:        0.10 Premium requests
Total usage est:        0.33 Premium requests`,
            expected: &UsageInfo{
                PremiumRequests: 0.33,
            },
        },
        "duration without space after minutes": {
            stdout: `API time spent:         1m36s
Total session time:     2m15.5s`,
            expected: &UsageInfo{
                APIDuration:     ptr(96 * time.Second),
                SessionDuration: ptr(135500 * time.Millisecond),
            },
        },
        "tokens without cached": {
            stdout: `Breakdown by AI model:
 gpt-4                   100k in, 10k out`,
            expected: &UsageInfo{
                InputTokens:  100000,
                OutputTokens: 10000,
                CachedTokens: 0,
            },
        },
        "malformed number rejected": {
            stdout: `Total usage est:        1.2.3 Premium requests`,
            expected: nil,  // Regex rejects "1.2.3" with strict pattern
        },
    }

    for name, tc := range tests {
        t.Run(name, func(t *testing.T) {
            got := ParseUsage(tc.stdout, tc.stderr)
            if diff := cmp.Diff(tc.expected, got); diff != "" {
                t.Errorf("ParseUsage() mismatch (-want +got):\n%s", diff)
            }
        })
    }
}
```

#### Cost Formatting Tests (`internal/cost/format_test.go`)

```go
func TestFormat(t *testing.T) {
    tests := map[string]struct {
        value    float64
        unit     string
        expected string
    }{
        "USD":              {1.23, UnitUSD, "$1.23"},
        "credits":          {0.45, UnitCredits, "0.45 credits"},
        "premium_requests": {0.33, UnitPremiumRequests, "0.33 premium requests"},
        "zero value":       {0, UnitUSD, "-"},
        "negative value":   {-1, UnitUSD, "-"},
        "unknown unit":     {1.5, "unknown", "1.50"},
    }

    for name, tc := range tests {
        t.Run(name, func(t *testing.T) {
            got := Format(tc.value, tc.unit)
            if got != tc.expected {
                t.Errorf("Format(%v, %q) = %q, want %q", tc.value, tc.unit, got, tc.expected)
            }
        })
    }
}

func TestFormatTotals(t *testing.T) {
    tests := map[string]struct {
        totals   Totals
        expected string
    }{
        "all units": {
            Totals{USD: 1.23, Credits: 0.45, PremiumRequests: 0.33},
            "$1.23, 0.45 credits, 0.33 premium requests",
        },
        "USD only": {
            Totals{USD: 1.23},
            "$1.23",
        },
        "credits only": {
            Totals{Credits: 0.45},
            "0.45 credits",
        },
        "USD and credits": {
            Totals{USD: 1.23, Credits: 0.45},
            "$1.23, 0.45 credits",
        },
        "all zero": {
            Totals{},
            "-",
        },
    }

    for name, tc := range tests {
        t.Run(name, func(t *testing.T) {
            got := FormatTotals(tc.totals)
            if got != tc.expected {
                t.Errorf("FormatTotals(%+v) = %q, want %q", tc.totals, got, tc.expected)
            }
        })
    }
}
```

#### Backward Compatibility Tests (`internal/logs/manager_test.go`)

```go
func TestSessionEntry_GetCost_BackwardCompat(t *testing.T) {
    tests := map[string]struct {
        entry        SessionEntry
        expectedVal  float64
        expectedUnit string
    }{
        "new format": {
            SessionEntry{CostValue: 0.45, CostUnit: "credits"},
            0.45, "credits",
        },
        "legacy kiro": {
            SessionEntry{CostUSD: 0.45, AgentType: "kiro"},
            0.45, "credits",
        },
        "legacy copilot": {
            SessionEntry{CostUSD: 0.33, AgentType: "copilot"},
            0.33, "premium_requests",
        },
        "legacy claude": {
            SessionEntry{CostUSD: 1.23, AgentType: "claude-code"},
            1.23, "USD",
        },
        "legacy no agent": {
            SessionEntry{CostUSD: 1.00},
            1.00, "USD",
        },
    }

    for name, tc := range tests {
        t.Run(name, func(t *testing.T) {
            val, unit := tc.entry.GetCost()
            if val != tc.expectedVal || unit != tc.expectedUnit {
                t.Errorf("GetCost() = (%v, %q), want (%v, %q)",
                    val, unit, tc.expectedVal, tc.expectedUnit)
            }
        })
    }
}
```

### Integration Tests

```go
func TestCopilotUsageExtraction_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }

    // Create mock Copilot output file
    mockOutput := `Session completed.
Total usage est:        0.33 Premium requests
API time spent:         28s
Total session time:     33s
Total code changes:     +10 -5
Breakdown by AI model:
 claude-haiku-4.5        146.4k in, 2.6k out, 88.2k cached`

    // Test that agent extracts usage correctly
    // ... setup and execution ...

    if result.Cost == nil {
        t.Fatal("expected Cost to be set")
    }
    if result.Cost.PremiumRequests != 0.33 {
        t.Errorf("PremiumRequests = %v, want 0.33", result.Cost.PremiumRequests)
    }
    if result.Cost.CostUnit != "premium_requests" {
        t.Errorf("CostUnit = %q, want %q", result.Cost.CostUnit, "premium_requests")
    }
}
```

### Property-Based Tests

The parser is a good candidate for property-based testing to verify robustness against format variations.

```go
func TestParseUsage_Properties(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        // Generate valid premium request values
        premiumReqs := rapid.Float64Range(0, 1000).Draw(t, "premiumReqs")

        // Generate output with varying whitespace
        spaces := rapid.StringMatching(`[ \t]+`).Draw(t, "spaces")
        output := fmt.Sprintf("Total usage est:%s%.2f Premium requests", spaces, premiumReqs)

        result := ParseUsage(output, "")

        if result == nil {
            t.Fatal("expected result, got nil")
        }

        // Property: parsed value should match generated value (within floating point tolerance)
        if math.Abs(result.PremiumRequests-premiumReqs) > 0.01 {
            t.Errorf("PremiumRequests = %v, want %v", result.PremiumRequests, premiumReqs)
        }
    })
}
```

## Existing Tests Requiring Modification

The following existing tests will break and need updates:

| Test File | Test Function | Reason | Fix |
|-----------|--------------|--------|-----|
| `internal/agents/agent.go` | N/A (struct change) | `PremiumRequests` type changes from `int` to `float64` | No test changes needed (type is internal) |
| `internal/report/generator_test.go` | `TestFormatCost` | Tests USD-only formatting | Update to test new `cost.Format` function, add unit-aware tests |
| `internal/logs/manager_test.go` | `TestManager_SaveSession` | Uses `CostMetrics{CostUSD: 0.15}` | Add `CostUnit` field to test fixtures |
| `internal/logs/manager_test.go` | `TestManager_SavePostCompletionSession` | Uses `CostMetrics{CostUSD: 0.25}` | Add `CostUnit` field to test fixtures |
| `internal/logs/manager_test.go` | `TestSaveSession_IncludesAgentInfo` | Validates session entry fields | Update expected fields to include `CostUnit`, `CostValue` |

**Test fixture pattern update:**

Before:
```go
Cost: &agents.CostMetrics{CostUSD: 0.15}
```

After:
```go
Cost: &agents.CostMetrics{CostUSD: 0.15, CostUnit: cost.UnitUSD}
```

**Note:** The `internal/report/generator_test.go:TestFormatCost` tests the package-local `formatCost` function which will be replaced by `cost.Format`. This test should be moved to `internal/cost/format_test.go` or removed after the refactor.

## File Summary

| File | Change Type | Purpose |
|------|-------------|---------|
| `internal/agents/copilot/usage.go` | New | Copilot usage parser |
| `internal/agents/copilot/usage_test.go` | New | Parser tests |
| `internal/agents/copilot/agent.go` | Modify | Integrate usage extraction |
| `internal/agents/kiro/agent.go` | Modify | Add CostUnit to cost metrics |
| `internal/agents/agent.go` | Modify | Update CostMetrics struct |
| `internal/cost/format.go` | New | Centralized cost formatting |
| `internal/cost/format_test.go` | New | Format tests |
| `internal/logs/manager.go` | Modify | Update SessionEntry, Summary, SaveSession |
| `internal/logs/manager_test.go` | Modify | Update fixtures, add backward compat tests |
| `internal/orbit/orbit.go` | Modify | Update formatCost to use cost package |
| `internal/web/handlers.go` | Modify | Add template helper |
| `internal/web/templates/run_detail.html` | Modify | Use new cost formatting |
| `internal/report/templates.go` | Modify | Use cost.Format |
| `internal/report/generator_test.go` | Modify | Update or remove `TestFormatCost` |

## Requirement Traceability Matrix

| Requirement | Design Element |
|-------------|----------------|
| 1.1 | `ParseUsage()` searches both stdout and stderr |
| 1.2 | `premiumRequestsRe` regex, `UsageInfo.PremiumRequests` |
| 1.3 | `apiTimeRe` regex with optional minutes, `parseDuration()` |
| 1.4 | `sessionTimeRe` regex with optional minutes, `parseDuration()` |
| 1.5 | `codeChangesRe` regex, `UsageInfo.LinesAdded/LinesRemoved` |
| 1.6 | `tokenBreakdownRe` with aggregation loop |
| 1.7 | `UsageInfo.CachedTokens` stored separately |
| 1.8 | `parseTokenValue()` handles "k" suffix |
| 1.8a | `parseTokenValue()` handles "m" suffix |
| 1.9 | `ParseUsage()` returns nil when no match |
| 1.10 | Each regex match is independent |
| 1.11 | `CostMetrics` storage in `execute()` |
| 1.12 | `debugLog()` calls on parse failures |
| 2.1 | `CostMetrics.CostUnit` field |
| 2.2 | `cost.UnitUSD`, `cost.UnitCredits`, `cost.UnitPremiumRequests` |
| 2.3 | `SessionEntry.CostUnit` field |
| 2.4 | `cost.InferUnitFromAgent()` |
| 2.4a | Fallback to USD with debug warning |
| 2.5-2.8 | `cost.Format()` switch statement |
| 2.9 | Validation in `ParseUsage()` |
| 3.1-3.3 | `UsageInfo.SessionDuration`, `CostMetrics.SessionDuration` |
| 3.4 | Fallback to `RunResult.Duration` in display |
| 3.5 | Pointer type allows nil |
| 3.6 | Display layer shows both times |
| 4.1-4.4 | All display contexts use `cost.Format()` |
| 4.5 | `cost.Format()` returns "-" for zero/nil |
| 4.6 | Single `cost.Format()` function |
| 4.7 | Display logic uses `LinesAdded/LinesRemoved` |
| 5.1-5.6 | `cost.FormatTotals()` implementation |
| 6.1-6.5 | `SessionEntry.GetCost()` backward compat logic |
