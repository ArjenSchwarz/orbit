# Apsis Copilot Support

## Overview

Add full GitHub Copilot CLI session support to Apsis. Currently, Apsis can parse Copilot `events.jsonl` files when given a direct path, but lacks session discovery (`apsis -l`), session ID lookup (`apsis <uuid>`), project filtering, extended thinking display, and handling of all event types. This brings Copilot support to parity with Claude Code, Codex, and Kiro.

## Requirements

### Session Discovery
- The system MUST list Copilot sessions when running `apsis -l`
- The system MUST read session metadata from `~/.copilot/session-state/{uuid}/workspace.yaml`
- The system MUST display sessions with `[copilot]` source prefix in listings
- The system MUST extract `created_at` timestamp from `workspace.yaml` for sorting
- The system MUST filter sessions by project directory using exact path match on `git_root` field (falling back to `cwd` if `git_root` is empty)
- The system MUST skip session directories missing `events.jsonl` or `workspace.yaml`

### Session Lookup
- The system MUST resolve Copilot session UUIDs to file paths (e.g., `apsis b310b03c-...`)
- The system MUST look up sessions in `~/.copilot/session-state/{uuid}/events.jsonl`
- The system MUST support case-insensitive UUID matching
- The system MUST return empty string (not error) when session not found, matching `findCodexSession()` pattern

### Event Type Handling
- The system MUST add `session.model_change`, `skill.invoked`, and `abort` to the format detection map
- The system MUST parse events with unknown types without error (for forward compatibility)

### Extended Thinking Display
- The system MUST parse `reasoningText` field from `assistant.message` events
- The system MUST render extended thinking before text content in assistant turns
- The system MUST use the existing thinking block format (collapsible `<details>` in markdown)
- The system MUST ignore `reasoningOpaque` field (encrypted, not human-readable)

## Implementation Approach

### 1. Add Session Discovery (`cmd/apsis/main.go`)

Create `listCopilotSessions()` following the pattern of `listKiroSessions()` (lines 580-627):

```go
func listCopilotSessions(projectPath string) ([]SessionInfo, error) {
    homeDir, _ := os.UserHomeDir()
    sessionDir := filepath.Join(homeDir, ".copilot", "session-state")

    entries, err := os.ReadDir(sessionDir)
    // For each UUID directory:
    //   - Parse workspace.yaml for metadata
    //   - Check events.jsonl exists
    //   - Filter by projectPath matching git_root (or cwd if git_root empty)
    //   - Return SessionInfo with Source: "copilot"
}
```

Add `findCopilotSession()` following `findCodexSession()` pattern (lines 344-383):
- Look up UUID in `~/.copilot/session-state/{uuid}/events.jsonl`
- Return `("", nil)` if not found (not an error)
- Support case-insensitive UUID matching

Update `listAllSessions()` (line 629) to call `listCopilotSessions()` and merge results.

Update `resolveInput()` (around line 230) to try `findCopilotSession()` after Codex lookup.

Update `sortSessionsByTimestamp()` (line 670): Copilot sessions sort after Claude but before Codex/Kiro in tie situations.

### 2. Add Workspace YAML Parsing (`cmd/apsis/main.go`)

The `workspace.yaml` file has this structure (verified from actual sessions):

```yaml
id: b310b03c-e860-461a-840c-aafb44b812f8
cwd: /Users/arjen/projects/personal/orbit
git_root: /Users/arjen/projects/personal/orbit
repository: ArjenSchwarz/orbit
branch: main
summary_count: 0
created_at: 2026-01-31T21:23:32.449Z
updated_at: 2026-01-31T21:23:38.155Z
summary: dewcribr this repository
```

Add struct for parsing (use pointer for optional fields):

```go
type CopilotWorkspace struct {
    ID        string     `yaml:"id"`
    Cwd       string     `yaml:"cwd"`
    GitRoot   string     `yaml:"git_root"`
    CreatedAt *time.Time `yaml:"created_at"`
    Summary   string     `yaml:"summary"`
}
```

Uses existing `gopkg.in/yaml.v3` dependency (already in go.mod).

### 3. Update Format Detection (`internal/transcript/parser.go`)

Add missing event types to `copilotTypes` map (lines 31-43). Event types verified from scanning all sessions in `~/.copilot/session-state/`:

```go
var copilotTypes = map[string]bool{
    // ... existing types ...
    "session.model_change": true,
    "skill.invoked":        true,
    "abort":                true,
}
```

### 4. Add Extended Thinking Support

**`internal/transcript/copilot_types.go`**: Add reasoning fields to `CopilotData` struct (around line 35):

```go
type CopilotData struct {
    // ... existing fields ...

    // assistant.message reasoning fields
    ReasoningText   string `json:"reasoningText,omitempty"`
    ReasoningOpaque string `json:"reasoningOpaque,omitempty"` // Ignored
}
```

**`internal/transcript/copilot_parser.go`**: Update `convertCopilotToEntries()` case for `assistant.message` (around line 99). Add thinking content BEFORE text content:

```go
case "assistant.message":
    // Add thinking content first (if present)
    if event.Data.ReasoningText != "" {
        currentTurnContent = append(currentTurnContent, ContentItem{
            Type:     "thinking",
            Thinking: event.Data.ReasoningText,
        })
    }

    // Add text content (existing code)
    if event.Data.Content != "" {
        currentTurnContent = append(currentTurnContent, ContentItem{
            Type: "text",
            Text: event.Data.Content,
        })
    }
    // ... existing tool handling ...
```

The thinking block will render using existing logic in `internal/transcript/markdown.go` (lines 89-99, `renderThinking()` function).

### Dependencies

- Existing: `listKiroSessions()`, `findCodexSession()` patterns in `cmd/apsis/main.go`
- Existing: `copilotTypes` map in `internal/transcript/parser.go`
- Existing: Thinking block rendering in `internal/transcript/markdown.go`
- Existing: `gopkg.in/yaml.v3` in go.mod

### Out of Scope

- Cost/token display (Copilot doesn't expose this in logs like Kiro does)
- Skill invocation details rendering (parse for detection only)
- Model change display in transcripts (parse for detection only)
- Session resume by ID (Copilot CLI limitation - only supports `--continue`)
- Rendering `abort` events specially (parse for detection only)

## Risks and Assumptions

- **Risk**: `workspace.yaml` format may vary across Copilot versions | **Mitigation**: Use pointer types for optional fields; fall back to file modification time if `created_at` missing or unparseable
- **Risk**: Session directory exists but missing required files | **Mitigation**: Skip directories without both `events.jsonl` and `workspace.yaml`; log warning for debugging
- **Assumption**: `reasoningText` appears in `assistant.message` events alongside `content` | **Validation**: Verified in actual session logs
- **Assumption**: Event types `session.model_change`, `skill.invoked`, `abort` are the only missing types | **Validation**: Verified by scanning all event types across 32 real sessions
- **Assumption**: Session UUIDs are standard format (8-4-4-4-12 hex digits) | **Validation**: Use existing `uuidPattern` regex from line 342
- **Prerequisite**: Copilot CLI installed with existing sessions in `~/.copilot/session-state/`
