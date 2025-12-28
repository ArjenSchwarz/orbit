# Collapsible Details Blocks for Session Transcripts

## Summary

Add `<details><summary>` blocks to both Markdown and HTML transcript renderers to improve readability of long tool outputs, particularly for Task (subagent) and Skill tool calls.

## Requirements

1. **Task and Skill tools**: Always wrap in `<details>` blocks
   - Summary for Task: `"🔧 [SubagentType]: [Description]"` (e.g., "🔧 Explore: Explore session log parsing")
   - Summary for Skill: `"🔧 Skill: [skill name]"` (e.g., "🔧 Skill: next-task")

2. **Other tools**: Wrap in `<details>` if content > 500 characters

3. **Thinking blocks**: Already use `<details>` - no change needed

## Files to Modify

### `internal/transcript/types.go`

Add fields to `ContentItem` for tool linking:

```go
type ContentItem struct {
    // ... existing fields ...
    ID        string `json:"id,omitempty"`          // tool_use ID
    ToolUseID string `json:"tool_use_id,omitempty"` // links tool_result to tool_use
}
```

### `internal/transcript/markdown.go`

1. Add constant:
   ```go
   const CollapseThreshold = 500
   ```

2. Add helper to extract Task/Skill summary info from input:
   ```go
   func getToolSummary(name string, input any) string
   ```

3. Modify `formatAssistantMessage()`:
   - Track tool_use IDs and their names in a map
   - For tool_use: wrap in `<details>` if Task/Skill or content > 500 chars
   - For tool_result: look up corresponding tool_use, wrap if Task/Skill or content > 500 chars

### `internal/transcript/html.go`

1. Add CSS for collapsible tool sections:
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

   details.tool-collapsible .tool-content {
       padding: 0.75rem;
       border-top: 1px solid var(--border-color);
   }
   ```

2. Modify `formatAssistantMessageHTML()`:
   - Reuse `getToolSummary()` helper from markdown.go (or make it a shared helper)
   - Track tool_use IDs and their names in a map
   - For tool_use: wrap in `<details class="tool-collapsible">` if Task/Skill or content > 500 chars
   - For tool_result: look up corresponding tool_use, wrap if Task/Skill or content > 500 chars

## Implementation Details

### Tool Use Rendering

**Task tool:**
```markdown
<details>
<summary>🔧 Explore: Explore session log parsing</summary>

```json
{...input...}
```

</details>
```

**Skill tool:**
```markdown
<details>
<summary>🔧 Skill: next-task</summary>

```json
{...input...}
```

</details>
```

**Other tools (> 500 chars):**
```markdown
<details>
<summary>🔧 Tool: Read</summary>

```json
{...long input...}
```

</details>
```

**Other tools (< 500 chars) - unchanged:**
```markdown
### 🔧 Tool: `Read`

```json
{"file_path": "/tmp/example.txt"}
```
```

### Tool Result Rendering

**Task/Skill results:**
```markdown
<details>
<summary>✅ Explore: Explore session log parsing</summary>

```
[subagent output]
```

</details>
```

**Other results (> 500 chars):**
```markdown
<details>
<summary>✅ Tool Result</summary>

```
[long output]
```

</details>
```

**Other results (< 500 chars) - unchanged:**
```markdown
#### ✅ Tool Result

```
Hello from the file!
```
```

### HTML Rendering

**Task tool (always collapses):**
```html
<details class="tool-collapsible">
    <summary>
        <span class="icon">🔧</span>
        <span>Explore: Explore session log parsing</span>
    </summary>
    <div class="tool-content">
        <pre><code>{...input...}</code></pre>
    </div>
</details>
```

**Skill tool (always collapses):**
```html
<details class="tool-collapsible">
    <summary>
        <span class="icon">🔧</span>
        <span>Skill: next-task</span>
    </summary>
    <div class="tool-content">
        <pre><code>{...input...}</code></pre>
    </div>
</details>
```

**Other tools (> 500 chars):**
```html
<details class="tool-collapsible">
    <summary>
        <span class="icon">🔧</span>
        <span>Tool: Read</span>
    </summary>
    <div class="tool-content">
        <pre><code>{...long input...}</code></pre>
    </div>
</details>
```

**Other tools (< 500 chars) - unchanged:**
```html
<div class="tool-use">
    <div class="tool-use-header">
        <span class="icon">🔧</span>
        <span>Tool: <code>Read</code></span>
    </div>
    <div class="tool-input">
        <pre><code>{"file_path": "/tmp/example.txt"}</code></pre>
    </div>
</div>
```

**Task/Skill results:**
```html
<details class="tool-collapsible">
    <summary>
        <span class="icon">✅</span>
        <span>Explore: Explore session log parsing</span>
    </summary>
    <div class="tool-content">
        <pre><code>[subagent output]</code></pre>
    </div>
</details>
```

**Other results (> 500 chars):**
```html
<details class="tool-collapsible">
    <summary>
        <span class="icon">✅</span>
        <span>Tool Result</span>
    </summary>
    <div class="tool-content">
        <pre><code>[long output]</code></pre>
    </div>
</details>
```

**Other results (< 500 chars) - unchanged:**
```html
<div class="tool-result">
    <div class="tool-result-header success">
        <span>✅ Tool Result</span>
    </div>
    <div class="tool-result-content">
        <pre><code>Hello from the file!</code></pre>
    </div>
</div>
```

## Test Cases

### Markdown Tests

- Task tool with subagent output (always collapses)
- Skill tool (always collapses)
- Short Read tool (< 500 chars, no collapse)
- Long Read tool (> 500 chars, collapses)
- Tool error results (maintain ❌ indicator)

### HTML Tests

- Task tool with subagent output (always collapses with `details.tool-collapsible`)
- Skill tool (always collapses)
- Short Read tool (< 500 chars, renders as `div.tool-use`)
- Long Read tool (> 500 chars, collapses)
- Tool error results (maintain error styling with ❌ indicator)
- Collapsed tool results use correct success/error icon

## Shared Helper

Extract `getToolSummary()` as a package-level function usable by both renderers:

```go
// getToolSummary extracts a readable summary for Task and Skill tools.
// Returns empty string for other tools.
func getToolSummary(name string, input any) string {
    switch name {
    case "Task":
        if m, ok := input.(map[string]any); ok {
            subagentType, _ := m["subagent_type"].(string)
            desc, _ := m["description"].(string)
            if subagentType != "" && desc != "" {
                return fmt.Sprintf("%s: %s", subagentType, desc)
            }
            if subagentType != "" {
                return subagentType
            }
        }
    case "Skill":
        if m, ok := input.(map[string]any); ok {
            skill, _ := m["skill"].(string)
            if skill != "" {
                return fmt.Sprintf("Skill: %s", skill)
            }
        }
    }
    return ""
}

// shouldCollapse determines if a tool use/result should be collapsed.
func shouldCollapse(name string, contentLen int) bool {
    if name == "Task" || name == "Skill" {
        return true
    }
    return contentLen > CollapseThreshold
}
```
