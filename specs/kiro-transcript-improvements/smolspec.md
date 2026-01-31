# Kiro Transcript Improvements

## Overview

Enhance Kiro log parsing in Apsis to display session cost information and correctly parse all tool result variants. Currently, the `usage_info` credits are extracted but not displayed, and the `Json` variant in tool results (containing bash command output with exit_status/stderr/stdout) is silently ignored, resulting in missing tool output in transcripts.

## Requirements

- The system MUST display total session cost at the top of Kiro transcripts (after session ID, before content separator)
- The system MUST parse the `Json` variant in Kiro tool result content alongside the existing `Text` variant
- The system MUST format Json tool results to show stdout/stderr content in a readable format
- The system MUST display cost with 2 decimal places, rounded half-up (e.g., "0.14 credits")
- The system MUST only display cost when `TotalCost` pointer is non-nil and value is > 0.005 (rounds to 0.01+)
- The system MUST concatenate Text and Json content when both are present in the same tool result
- The system SHOULD prefix stderr output with "stderr:" when non-empty
- The system SHOULD note non-zero exit status in tool result output (e.g., "[exit: 1]")

## Implementation Approach

### 1. Add Metadata to ParseResult (`internal/transcript/parser.go`)

Add a `Metadata` field to carry format-specific information:

```go
// ParseResultMetadata contains optional format-specific metadata.
type ParseResultMetadata struct {
    TotalCost *float64 // nil = not available, pointer to value = available
    CostUnit  string   // e.g., "credits"
}

type ParseResult struct {
    Entries  []Entry
    Warnings []ParseWarning
    Metadata *ParseResultMetadata // nil for formats without metadata
}
```

### 2. Extend RenderOptions (`internal/transcript/types.go`)

Add cost fields to the existing `RenderOptions` struct (no signature changes to render functions):

```go
type RenderOptions struct {
    Title      string
    SessionID  string
    ProjectDir string
    Navigation *NavigationContext
    TotalCost  *float64 // nil = don't display, pointer = display if > 0.005
    CostUnit   string   // e.g., "credits" (default if empty)
}
```

**Data flow**: Callers copy from `ParseResult.Metadata` to `RenderOptions`:
- `cmd/apsis/main.go`: Update `convert()` to copy metadata to opts before rendering
- `internal/logs/manager.go`: Update `SavePhaseLog()` to copy metadata to opts
- `internal/web/handlers.go`: Update transcript handler to copy metadata to opts

### 3. Parse Json Variant (`internal/transcript/kiro_types.go`, `internal/transcript/kiro_parser.go`)

Add struct for Json variant:

```go
// KiroJsonOutput represents structured command output in Json variant.
type KiroJsonOutput struct {
    ExitStatus string `json:"exit_status"` // Always string in observed logs
    Stdout     string `json:"stdout"`
    Stderr     string `json:"stderr"`
}

// KiroResultContent represents content in a tool result.
type KiroResultContent struct {
    Text string          `json:"Text,omitempty"`
    Json *KiroJsonOutput `json:"Json,omitempty"`
}
```

Update `convertKiroUserMessage()` to handle both variants:

```go
for _, content := range result.Content {
    if content.Text != "" {
        text += content.Text
    }
    if content.Json != nil {
        text += formatJsonOutput(content.Json)
    }
}
```

Format function:
```go
func formatJsonOutput(j *KiroJsonOutput) string {
    var parts []string
    if j.Stdout != "" {
        parts = append(parts, j.Stdout)
    }
    if j.Stderr != "" {
        parts = append(parts, "stderr: "+j.Stderr)
    }
    if j.ExitStatus != "" && j.ExitStatus != "0" {
        parts = append(parts, fmt.Sprintf("[exit: %s]", j.ExitStatus))
    }
    return strings.Join(parts, "\n")
}
```

### 4. Populate Cost in Kiro Parser (`internal/transcript/kiro_parser.go`)

Modify `ParseKiro()` to populate metadata using existing `extractKiroCredits()`:

```go
func ParseKiro(r io.Reader) (*ParseResult, error) {
    // ... existing parsing ...

    totalCost := extractKiroCredits(&session)
    var metadata *ParseResultMetadata
    if totalCost > 0 {
        metadata = &ParseResultMetadata{
            TotalCost: &totalCost,
            CostUnit:  "credits",
        }
    }

    return &ParseResult{
        Entries:  entries,
        Warnings: warnings,
        Metadata: metadata,
    }, nil
}
```

### 5. Display Cost in Renderers (`internal/transcript/markdown.go`, `internal/transcript/html.go`)

Update header section in `RenderMarkdown()`:

```go
if opts.SessionID != "" {
    sb.WriteString(fmt.Sprintf("**Session ID:** `%s`\n\n", opts.SessionID))
}

if opts.TotalCost != nil && *opts.TotalCost >= 0.005 {
    unit := opts.CostUnit
    if unit == "" {
        unit = "credits"
    }
    sb.WriteString(fmt.Sprintf("**Cost:** %.2f %s\n\n", *opts.TotalCost, unit))
}

sb.WriteString("---\n\n")
```

Same pattern for `RenderHTML()` and `RenderHTMLFragment()`.

**Follow mode**: Cost display works in follow mode because `RenderOptions` carries the cost. The initial header is rendered with cost when the follower starts (if metadata is available from initial parse).

**Existing Patterns to Follow:**
- `RenderOptions` struct pattern for passing render configuration (`types.go:202-208`)
- `ParseResult` struct for returning parse results with warnings (`parser.go:226-230`)
- `extractKiroCredits()` for credit summing logic (`kiro_parser.go:163-175`)

**Dependencies:**
- `ParseKiro()` function
- `RenderMarkdown()`, `RenderHTML()`, `RenderHTMLFragment()` functions
- `KiroResultContent` and related types
- Callers: `cmd/apsis/main.go`, `internal/logs/manager.go`, `internal/web/handlers.go`, `internal/transcript/follow.go`

**Out of Scope:**
- Cost display for Claude/Codex/Copilot formats (they don't have equivalent metadata in their logs)
- Per-request cost breakdown
- Currency conversion or cost aggregation across sessions

## Risks and Assumptions

- **Risk**: Adding `Metadata` field to `ParseResult` could affect callers that destructure the struct | **Mitigation**: Go allows adding fields to structs without breaking existing code; callers using named fields or ignoring extra fields are unaffected
- **Risk**: Json variant may have additional fields not yet seen | **Mitigation**: Parse known fields only; unknown fields ignored by Go's JSON unmarshaler by default
- **Risk**: `exit_status` might be integer in some cases | **Mitigation**: Use string type as observed; if integer appears, JSON unmarshal will fail gracefully and we can add `any` type handling later
- **Assumption**: All Kiro sessions have `usage_info` in `user_turn_metadata` when cost tracking is enabled | **Validation**: Handle nil/empty gracefully by not setting metadata
- **Assumption**: Credit values should be summed across all usage_info entries | **Validation**: Confirmed by existing `extractKiroCredits()` implementation
- **Assumption**: 2 decimal places is sufficient precision for credit display | **Validation**: Observed values like 0.139 round reasonably to 0.14
- **Prerequisite**: Sample Kiro log available at `samples/kiro-cli-log.json` for testing
