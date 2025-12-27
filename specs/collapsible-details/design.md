# Collapsible Details Blocks - Design Document

## Overview

This design document describes the implementation of collapsible `<details>` blocks in the Apsis transcript renderer. The feature wraps verbose tool outputs (particularly Task and Skill tools) in collapsible sections to improve transcript readability.

**Scope:** Modifications to the `internal/transcript` package affecting `types.go`, `markdown.go`, and `html.go`.

**Key Design Decisions:** See `decision_log.md` for rationale on threshold values, rune vs byte counting, and other decisions.

---

## Architecture

The implementation follows the existing single-pass rendering architecture with one critical addition: a **render-level** tool metadata map that persists across all entries to track tool_use IDs for result matching.

**Key Insight:** In Claude JSONL transcripts, `tool_use` appears in "assistant" entries while `tool_result` appears in "user" entries. The map must be shared across both entry types.

```
┌─────────────────────────────────────────────────────────────────┐
│                    RenderMarkdown / RenderHTML                   │
├─────────────────────────────────────────────────────────────────┤
│                                                                   │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │            Tool Metadata Map (render-level scope)            │ │
│  │                  map[id]→{name, summary}                     │ │
│  └─────────────────────────────────────────────────────────────┘ │
│                              │                                    │
│         ┌────────────────────┼────────────────────┐              │
│         ▼                    ▼                    ▼              │
│  ┌─────────────┐    ┌─────────────────┐    ┌─────────────┐      │
│  │ Entry N     │    │ Entry N+1       │    │ Entry N+2   │      │
│  │ (assistant) │    │ (user)          │    │ (assistant) │      │
│  │ tool_use    │───▶│ tool_result     │    │ text        │      │
│  │ populates   │    │ reads from      │    │             │      │
│  │ map         │    │ map             │    │             │      │
│  └─────────────┘    └─────────────────┘    └─────────────┘      │
│                                                                   │
└─────────────────────────────────────────────────────────────────┘
```

### Processing Flow

1. Parse JSONL entries (unchanged)
2. Initialize tool metadata map at render function level
3. For each entry (both "user" and "assistant" types):
   - For `tool_use`: extract ID, determine if should collapse, store metadata in map
   - For `tool_result`: lookup tool_use_id in map, inherit collapse decision or apply threshold
4. Render with appropriate format (collapsed or uncollapsed)

---

## Components and Interfaces

### 1. Data Model Changes (`types.go`)

Add two fields to `ContentItem`:

```go
type ContentItem struct {
    Type      string `json:"type"`
    Text      string `json:"text,omitempty"`
    Thinking  string `json:"thinking,omitempty"`
    Name      string `json:"name,omitempty"`
    Input     any    `json:"input,omitempty"`
    Content   string `json:"content,omitempty"`
    IsError   bool   `json:"is_error,omitempty"`
    ID        string `json:"id,omitempty"`          // NEW: tool_use ID
    ToolUseID string `json:"tool_use_id,omitempty"` // NEW: links tool_result to tool_use
}
```

**Requirement Traceability:** [6.1], [6.2], [6.3]

The `UnmarshalJSON` method must be updated to parse these new fields from the alias struct.

### 2. Shared Helper Functions (`markdown.go`)

Add these package-level functions and constants:

```go
const (
    // CollapseThresholdRunes is the threshold for collapsing non-Task/Skill tools.
    CollapseThresholdRunes = 500
)

// toolMetadata stores information about a tool_use for result matching.
type toolMetadata struct {
    Name    string // Tool name (e.g., "Task", "Read")
    Summary string // Summary text for collapsed display
}

// getToolSummary extracts a readable summary for Task and Skill tools.
// Returns empty string for other tools or if extraction fails.
// Uses defensive type assertions with comma-ok idiom.
func getToolSummary(name string, input any) string {
    switch name {
    case "Task":
        inputMap, ok := input.(map[string]any)
        if !ok {
            return ""
        }
        subType, _ := inputMap["subagent_type"].(string)
        desc, _ := inputMap["description"].(string)
        // Handle partial fields gracefully (no trailing colon)
        if subType != "" && desc != "" {
            return subType + ": " + desc
        }
        if subType != "" {
            return subType
        }
        return ""
    case "Skill":
        inputMap, ok := input.(map[string]any)
        if !ok {
            return ""
        }
        skill, _ := inputMap["skill"].(string)
        if skill != "" {
            return "Skill: " + skill
        }
        return ""
    }
    return ""
}

// shouldCollapse determines if a tool_use or tool_result should be wrapped
// in a <details> element based on tool name and content rune count.
func shouldCollapse(name string, runeCount int) bool {
    if name == "Task" || name == "Skill" {
        return true
    }
    return runeCount > CollapseThresholdRunes
}

// runeCount returns the number of runes in a string.
// Wrapper around utf8.RuneCountInString for clarity.
func runeCount(s string) int {
    return utf8.RuneCountInString(s)
}

// escapeSummary escapes summary text for safe inclusion in HTML/Markdown.
// Applies html.EscapeString to prevent XSS and structural corruption.
func escapeSummary(s string) string {
    return html.EscapeString(s)
}
```

**Requirement Traceability:** [1.3], [1.4], [1.5], [1.6], [1.7], [2.1], [2.2], [2.3]

### 3. Markdown Renderer Changes (`markdown.go`)

**Critical:** The tool metadata map must be scoped at the `RenderMarkdown` level, not per-entry. This is because `tool_use` appears in "assistant" entries while `tool_result` appears in "user" entries.

Modify `RenderMarkdown` to:

1. Initialize a `toolMetadata` map before the entry loop
2. Pass the map to both `formatAssistantMessage` and `formatUserMessage`

Modify `formatAssistantMessage` signature and behavior:

```go
func formatAssistantMessage(entry *Entry, toolMeta map[string]toolMetadata) string
```

For `tool_use` blocks:
- Serialize input with `json.Marshal` (compact, no indent) for threshold measurement
- Store metadata in map keyed by `item.ID`
- Check `shouldCollapse(item.Name, runeCount(compactJSON))`
- Render with `<details>` wrapper if collapsed (using indented JSON for display)
- Apply `escapeSummary()` to all summary text

Modify `formatUserMessage` to handle `tool_result`:

```go
func formatUserMessage(entry *Entry, toolMeta map[string]toolMetadata) string
```

For `tool_result` blocks in user messages:
- Lookup `item.ToolUseID` in metadata map
- If found and Task/Skill: inherit collapse behavior and summary
- If not found: apply threshold-based collapsing
- Render with `<details>` wrapper if collapsed
- Apply `escapeSummary()` to all summary text

**Requirement Traceability:** [1.1], [1.2], [2.4], [2.5], [3.3], [3.4], [3.5], [3.6], [3.7], [3.8], [3.9], [3.10], [4.1], [4.2], [4.3], [4.4]

### 4. HTML Renderer Changes (`html.go`)

Add CSS for collapsible tool blocks:

```css
details.tool-collapsible {
    margin: 1rem 0;
    background-color: var(--bg-primary);
    border: 1px solid var(--border-color);
    border-radius: 6px;
}

details.tool-collapsible summary {
    cursor: pointer;
    padding: 0.5rem 0.75rem;
    background-color: var(--bg-code);
    font-weight: 500;
    display: flex;
    align-items: center;
    gap: 0.5rem;
}

details.tool-collapsible summary .icon {
    color: var(--tool-accent);
}

details.tool-collapsible.error summary .icon {
    color: var(--error-color);
}

details.tool-collapsible .tool-content {
    padding: 0.75rem;
    border-top: 1px solid var(--border-color);
}

details.tool-collapsible .tool-content pre {
    margin: 0;
    overflow-x: auto;
    white-space: pre-wrap;
    word-wrap: break-word;
}

details.tool-collapsible .tool-content code {
    font-family: "SF Mono", Monaco, "Cascadia Code", monospace;
    font-size: 0.85rem;
    line-height: 1.5;
}
```

Apply same architectural changes as Markdown renderer:

1. Modify `RenderHTML` to initialize metadata map before entry loop
2. Pass map to both `formatAssistantMessageHTML` and `formatUserMessageHTML`
3. Modify `formatUserMessageHTML` to handle `tool_result` blocks
4. Apply `html.EscapeString()` to all summary text components

```go
func formatAssistantMessageHTML(entry *Entry, toolMeta map[string]toolMetadata) string
func formatUserMessageHTML(entry *Entry, toolMeta map[string]toolMetadata) string
```

**Requirement Traceability:** [5.1], [5.2], [5.3], [5.4], [5.5], [5.6]

---

## Data Models

### Tool Metadata Map

```go
// Internal structure for tracking tool_use metadata during rendering
type toolMetadata struct {
    Name    string // Tool name for collapse decision
    Summary string // Pre-computed summary text
}

// Map type used in formatAssistantMessage functions
map[string]toolMetadata  // key: tool_use ID
```

### Summary Text Formats

| Tool Type | Summary Format | Example |
|-----------|---------------|---------|
| Task (with fields) | `🔧 {subagent_type}: {description}` | `🔧 Explore: Search for config files` |
| Task (fallback) | `🔧 Task` | `🔧 Task` |
| Skill (with field) | `🔧 Skill: {skill_name}` | `🔧 Skill: next-task` |
| Skill (fallback) | `🔧 Skill` | `🔧 Skill` |
| Other (collapsed) | `🔧 Tool: {name}` | `🔧 Tool: Read` |
| Result (success) | `✅ {summary_from_tool_use}` or `✅ Tool Result` | `✅ Explore: Search for config files` |
| Result (error) | `❌ {summary_from_tool_use}` or `❌ Tool Error` | `❌ Tool Error` |

---

## Error Handling

### JSON Parsing Errors

| Scenario | Behavior | Requirement |
|----------|----------|-------------|
| `input` is nil | Do not collapse (zero-length) | [2.5] |
| `input` is not JSON-serializable | Skip input display, do not collapse | Graceful degradation |
| Task input not a map | Fall back to "🔧 Task" | [1.4] |
| Task input missing `subagent_type` | Fall back to "🔧 Task" | [1.4] |
| `subagent_type` is empty string | Fall back to "🔧 Task" | [1.4] |
| Skill input missing `skill` field | Fall back to "🔧 Skill" | [1.6] |

### ID Matching Errors

| Scenario | Behavior | Requirement |
|----------|----------|-------------|
| `tool_use_id` not in map | Apply threshold-based collapsing | [3.6], [7.3] |
| `tool_use_id` is empty | Apply threshold-based collapsing | [3.6] |
| `id` field missing from tool_use | Skip storing in map (result won't match) | Graceful degradation |

### Content Length Edge Cases

| Scenario | Behavior | Requirement |
|----------|----------|-------------|
| Content exactly 500 runes | Do not collapse (≤ 500) | [2.3], [3.9] |
| Content is 501 runes | Collapse | [2.2], [3.7] |
| Zero-length content | Do not collapse | [2.5], [3.10] |
| Content is nil | Do not collapse | [2.5] |

---

## Testing Strategy

### Unit Tests

Tests should follow the existing table-driven pattern with map-based test cases.

#### Markdown Tests (`markdown_test.go`)

| Test Name | Description | Requirements |
|-----------|-------------|--------------|
| `TestRenderMarkdown_TaskToolAlwaysCollapses` | Task tool with valid input collapses | [1.1], [1.3] |
| `TestRenderMarkdown_TaskToolFallback` | Task with missing fields uses fallback | [1.4] |
| `TestRenderMarkdown_SkillToolAlwaysCollapses` | Skill tool with valid input collapses | [1.2], [1.5] |
| `TestRenderMarkdown_SkillToolFallback` | Skill with missing field uses fallback | [1.6] |
| `TestRenderMarkdown_ToolNameCaseSensitive` | "task" and "TASK" do not collapse | [1.7] |
| `TestRenderMarkdown_ShortToolNoCollapse` | < 500 runes does not collapse | [2.3] |
| `TestRenderMarkdown_LongToolCollapses` | > 500 runes collapses | [2.2] |
| `TestRenderMarkdown_ExactThresholdNoCollapse` | 500 runes does not collapse | [2.3] |
| `TestRenderMarkdown_ZeroLengthNoCollapse` | Empty input does not collapse | [2.5] |
| `TestRenderMarkdown_ResultMatchesToolUse` | Result inherits Task/Skill collapse | [3.4], [3.5] |
| `TestRenderMarkdown_UnmatchedResultThreshold` | Unmatched result uses threshold | [3.6] |
| `TestRenderMarkdown_ResultErrorIcon` | Error results use ❌ | [3.8] |
| `TestRenderMarkdown_ZeroLengthResultNoCollapse` | Empty result does not collapse | [3.10] |
| `TestRenderMarkdown_DetailsFormat` | Verify `<details>` structure | [4.1], [4.2] |
| `TestRenderMarkdown_UncollapsedFormat` | Verify heading format unchanged | [4.3], [4.4] |

#### HTML Tests (`html_test.go`)

| Test Name | Description | Requirements |
|-----------|-------------|--------------|
| `TestRenderHTML_TaskToolCollapses` | Task tool uses `details.tool-collapsible` | [5.2] |
| `TestRenderHTML_SkillToolCollapses` | Skill tool collapses | [5.2] |
| `TestRenderHTML_ShortToolNoCollapse` | Short tool uses `div.tool-use` | [5.5] |
| `TestRenderHTML_LongToolCollapses` | Long tool collapses | [5.2] |
| `TestRenderHTML_ResultCollapses` | Result uses `details.tool-collapsible` | [5.3] |
| `TestRenderHTML_ShortResultNoCollapse` | Short result uses `div.tool-result` | [5.6] |
| `TestRenderHTML_CSSIncluded` | CSS contains `.tool-collapsible` styles | [5.1], [5.4] |

#### Helper Function Tests

| Test Name | Description |
|-----------|-------------|
| `TestGetToolSummary_Task` | Extracts subagent_type and description |
| `TestGetToolSummary_TaskPartialFields` | Only subagent_type present produces "Explore" not "Explore: " |
| `TestGetToolSummary_TaskFallback` | Returns empty for malformed input |
| `TestGetToolSummary_Skill` | Extracts skill name |
| `TestGetToolSummary_OtherTool` | Returns empty for non-Task/Skill |
| `TestShouldCollapse_AlwaysTools` | Task/Skill always return true |
| `TestShouldCollapse_Threshold` | Correct behavior at 499, 500, 501 runes |
| `TestRuneCount_Unicode` | Correct count for multi-byte characters |
| `TestEscapeSummary_XSS` | Escapes `</summary>` and script tags |

#### Parser Tests (`parser_test.go`)

| Test Name | Description | Requirements |
|-----------|-------------|--------------|
| `TestParseJSONL_IDField` | Parses `id` from tool_use | [3.1], [6.1] |
| `TestParseJSONL_ToolUseIDField` | Parses `tool_use_id` from tool_result | [3.2], [6.2] |
| `TestParseJSONL_MissingIDFields` | Handles missing ID fields gracefully | [6.3], [7.2] |

#### Cross-Entry Tests (Critical)

| Test Name | Description | Requirements |
|-----------|-------------|--------------|
| `TestRenderMarkdown_CrossEntryToolMatching` | tool_use in assistant entry, tool_result in next user entry | [3.3], [3.4], [3.5] |
| `TestRenderMarkdown_ToolResultInUserEntry` | tool_result appears in "user" type entry, not assistant | [3.4] |
| `TestRenderHTML_CrossEntryToolMatching` | Same cross-entry behavior for HTML | [5.3] |
| `TestRenderMarkdown_MultipleToolsMatching` | Multiple tool_use/result pairs match correctly | [3.3] |

### Integration Tests

Use golden file testing with real JSONL samples:

1. Create `testdata/collapsible/` directory with sample JSONL files
2. Create corresponding `.golden` files for expected Markdown/HTML output
3. Test both formats produce expected collapsible structure
4. Include sample with cross-entry tool_use/tool_result pairs

### Backward Compatibility Tests

| Test Name | Description | Requirements |
|-----------|-------------|--------------|
| `TestBackwardCompat_NoIDFields` | Old JSONL without id/tool_use_id renders | [7.2] |
| `TestBackwardCompat_TruncationPreserved` | Long content still truncated | [7.4] |
| `TestBackwardCompat_PreTruncationDecision` | Collapse based on original length | [7.5] |

---

## Requirement Traceability Matrix

| Req | Component | Test |
|-----|-----------|------|
| 1.1 | `formatAssistantMessage` | `TestRenderMarkdown_TaskToolAlwaysCollapses` |
| 1.2 | `formatAssistantMessage` | `TestRenderMarkdown_SkillToolAlwaysCollapses` |
| 1.3 | `getToolSummary` | `TestGetToolSummary_Task` |
| 1.4 | `getToolSummary` | `TestGetToolSummary_TaskFallback` |
| 1.5 | `getToolSummary` | `TestGetToolSummary_Skill` |
| 1.6 | `getToolSummary` | `TestGetToolSummary_OtherTool` |
| 1.7 | `shouldCollapse` | `TestRenderMarkdown_ToolNameCaseSensitive` |
| 2.1 | `CollapseThresholdRunes` | `TestShouldCollapse_Threshold` |
| 2.2 | `shouldCollapse` | `TestRenderMarkdown_LongToolCollapses` |
| 2.3 | `shouldCollapse` | `TestRenderMarkdown_ShortToolNoCollapse` |
| 2.4 | `formatAssistantMessage` | `TestRenderMarkdown_LongToolCollapses` |
| 2.5 | `formatAssistantMessage` | `TestRenderMarkdown_ZeroLengthNoCollapse` |
| 3.1 | `ContentItem.ID` | `TestParseJSONL_IDField` |
| 3.2 | `ContentItem.ToolUseID` | `TestParseJSONL_ToolUseIDField` |
| 3.3 | `formatAssistantMessage` | `TestRenderMarkdown_ResultMatchesToolUse` |
| 3.4 | `formatAssistantMessage` | `TestRenderMarkdown_ResultMatchesToolUse` |
| 3.5 | `formatAssistantMessage` | `TestRenderMarkdown_ResultMatchesToolUse` |
| 3.6 | `formatAssistantMessage` | `TestRenderMarkdown_UnmatchedResultThreshold` |
| 3.7 | `formatAssistantMessage` | `TestRenderMarkdown_UnmatchedResultThreshold` |
| 3.8 | `formatAssistantMessage` | `TestRenderMarkdown_ResultErrorIcon` |
| 3.9 | `formatAssistantMessage` | `TestRenderMarkdown_ShortToolNoCollapse` |
| 3.10 | `formatAssistantMessage` | `TestRenderMarkdown_ZeroLengthResultNoCollapse` |
| 4.1 | `formatAssistantMessage` | `TestRenderMarkdown_DetailsFormat` |
| 4.2 | `formatAssistantMessage` | `TestRenderMarkdown_DetailsFormat` |
| 4.3 | `formatAssistantMessage` | `TestRenderMarkdown_UncollapsedFormat` |
| 4.4 | `formatAssistantMessage` | `TestRenderMarkdown_UncollapsedFormat` |
| 5.1 | `htmlCSS` | `TestRenderHTML_CSSIncluded` |
| 5.2 | `formatAssistantMessageHTML` | `TestRenderHTML_TaskToolCollapses` |
| 5.3 | `formatAssistantMessageHTML` | `TestRenderHTML_ResultCollapses` |
| 5.4 | `htmlCSS` | `TestRenderHTML_CSSIncluded` |
| 5.5 | `formatAssistantMessageHTML` | `TestRenderHTML_ShortToolNoCollapse` |
| 5.6 | `formatAssistantMessageHTML` | `TestRenderHTML_ShortResultNoCollapse` |
| 6.1 | `ContentItem` | `TestParseJSONL_IDField` |
| 6.2 | `ContentItem` | `TestParseJSONL_ToolUseIDField` |
| 6.3 | `ContentItem` | `TestParseJSONL_MissingIDFields` |
| 7.1 | `RenderOptions` | N/A (no changes to struct) |
| 7.2 | `RenderMarkdown`, `RenderHTML` | `TestBackwardCompat_NoIDFields` |
| 7.3 | `formatAssistantMessage` | `TestRenderMarkdown_UnmatchedResultThreshold` |
| 7.4 | `formatAssistantMessage` | `TestBackwardCompat_TruncationPreserved` |
| 7.5 | `formatAssistantMessage` | `TestBackwardCompat_PreTruncationDecision` |
| 7.6 | N/A (documentation) | N/A |
