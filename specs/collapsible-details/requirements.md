# Collapsible Details Blocks for Session Transcripts

## Introduction

This feature enhances the Apsis transcript renderer by wrapping long tool outputs in collapsible `<details><summary>` blocks. This improves readability of session transcripts, particularly for Task (subagent) and Skill tool calls which often produce verbose output. The feature applies to both Markdown and HTML output formats.

---

## Requirements

### 1. Task and Skill Tool Collapsing

**User Story:** As a user reviewing session transcripts, I want Task and Skill tool calls to be collapsed by default, so that I can quickly scan the transcript without being overwhelmed by subagent output.

**Acceptance Criteria:**

1. <a name="1.1"></a>WHEN rendering a tool_use content block with name "Task", the system SHALL wrap it in a `<details>` element
2. <a name="1.2"></a>WHEN rendering a tool_use content block with name "Skill", the system SHALL wrap it in a `<details>` element
3. <a name="1.3"></a>The summary for Task tools SHALL display the format "🔧 {subagent_type}: {description}" where subagent_type and description are extracted from top-level string fields of the tool input
4. <a name="1.4"></a>IF the Task tool input is not a JSON object, lacks subagent_type or description fields, or those fields are empty strings, the summary SHALL fall back to "🔧 Task"
5. <a name="1.5"></a>The summary for Skill tools SHALL display the format "🔧 Skill: {skill_name}" where skill_name is extracted from the top-level "skill" string field of the tool input
6. <a name="1.6"></a>IF the Skill tool input is not a JSON object, lacks the skill field, or the skill field is an empty string, the summary SHALL fall back to "🔧 Skill"
7. <a name="1.7"></a>Tool name matching SHALL be case-sensitive and exact (e.g., "Task" matches but "task" or "TASK" do not)

### 2. Threshold-Based Collapsing for Other Tools

**User Story:** As a user reviewing session transcripts, I want long tool outputs to be collapsed, so that I can focus on the conversation flow without excessive scrolling.

**Acceptance Criteria:**

1. <a name="2.1"></a>The system SHALL define a collapse threshold constant of 500 runes (approximately 10-15 lines of typical tool input)
2. <a name="2.2"></a>WHEN rendering a tool_use content block (other than Task or Skill) with JSON-serialized input (via `json.Marshal`, no indentation) exceeding 500 runes, the system SHALL wrap it in a `<details>` element
3. <a name="2.3"></a>WHEN rendering a tool_use content block with JSON-serialized input of 500 runes or fewer, the system SHALL render it without collapsing
4. <a name="2.4"></a>The summary for collapsed non-Task/Skill tools SHALL display the format "🔧 Tool: {tool_name}"
5. <a name="2.5"></a>WHEN rendering a tool_use content block with zero-length or nil input, the system SHALL NOT collapse it

### 3. Tool Result Collapsing

**User Story:** As a user reviewing session transcripts, I want tool results to follow the same collapsing rules as their corresponding tool calls, so that the transcript has consistent visual structure.

**Acceptance Criteria:**

1. <a name="3.1"></a>The system SHALL parse the `id` field from tool_use content blocks
2. <a name="3.2"></a>The system SHALL parse the `tool_use_id` field from tool_result content blocks
3. <a name="3.3"></a>The renderer SHALL maintain a map of tool_use `id` to metadata (tool name, summary text) during rendering
4. <a name="3.4"></a>WHEN rendering a tool_result whose tool_use_id matches a Task or Skill tool_use, the system SHALL wrap it in a `<details>` element
5. <a name="3.5"></a>WHEN rendering a tool_result whose tool_use_id matches a Task or Skill tool_use, the summary SHALL display the same format as the corresponding tool_use (e.g., "✅ {subagent_type}: {description}")
6. <a name="3.6"></a>WHEN rendering a tool_result that cannot be matched to a tool_use (unknown tool_use_id), the system SHALL apply threshold-based collapsing (500 runes)
7. <a name="3.7"></a>WHEN rendering a tool_result with content exceeding 500 runes (and not matched to Task/Skill), the system SHALL wrap it in a `<details>` element with summary "✅ Tool Result"
8. <a name="3.8"></a>WHEN rendering a tool_result where `is_error` is true and the result should be collapsed, the system SHALL use "❌" instead of "✅" in the summary
9. <a name="3.9"></a>WHEN rendering a tool_result with content of 500 runes or fewer (and not matched to Task/Skill), the system SHALL render it without collapsing
10. <a name="3.10"></a>WHEN rendering a tool_result with zero-length content, the system SHALL NOT collapse it

### 4. Markdown Output Format

**User Story:** As a user viewing transcripts in Markdown-compatible viewers, I want collapsible blocks to use standard HTML details/summary elements, so that they render correctly in GitHub, VS Code, and other Markdown renderers.

**Acceptance Criteria:**

1. <a name="4.1"></a>Collapsed tool_use blocks in Markdown SHALL use the format:
   ```
   <details>
   <summary>{icon} {summary_text}</summary>

   ```json
   {input}
   ```

   </details>
   ```
2. <a name="4.2"></a>Collapsed tool_result blocks in Markdown SHALL use the format:
   ```
   <details>
   <summary>{icon} {summary_text}</summary>

   ```
   {content}
   ```

   </details>
   ```
3. <a name="4.3"></a>Non-collapsed tool_use blocks SHALL retain the existing heading format: `### 🔧 Tool: \`{name}\``
4. <a name="4.4"></a>Non-collapsed tool_result blocks SHALL retain the existing heading format: `#### ✅ Tool Result` or `#### ❌ Tool Error`

### 5. HTML Output Format

**User Story:** As a user viewing transcripts in a web browser, I want collapsible blocks to be styled consistently with the existing HTML theme, so that the transcript has a professional appearance.

**Acceptance Criteria:**

1. <a name="5.1"></a>The HTML renderer SHALL include CSS styles for a `details.tool-collapsible` class
2. <a name="5.2"></a>Collapsed tool_use blocks in HTML SHALL use `<details class="tool-collapsible">` with a styled summary
3. <a name="5.3"></a>Collapsed tool_result blocks in HTML SHALL use `<details class="tool-collapsible">` with a styled summary
4. <a name="5.4"></a>The `tool-collapsible` CSS SHALL be consistent with existing theme variables (--bg-primary, --border-color, --tool-accent, etc.)
5. <a name="5.5"></a>Non-collapsed tool_use blocks in HTML SHALL retain the existing `div.tool-use` structure
6. <a name="5.6"></a>Non-collapsed tool_result blocks in HTML SHALL retain the existing `div.tool-result` structure

### 6. Data Model Changes

**User Story:** As a developer implementing this feature, I want clear specifications for required data model changes, so that I can parse the necessary fields from JSONL transcripts.

**Acceptance Criteria:**

1. <a name="6.1"></a>The ContentItem struct SHALL include an `ID` field with JSON tag `id` for tool_use blocks
2. <a name="6.2"></a>The ContentItem struct SHALL include a `ToolUseID` field with JSON tag `tool_use_id` for tool_result blocks
3. <a name="6.3"></a>Both fields SHALL be optional (omitempty) to maintain compatibility with existing transcripts

### 7. Backward Compatibility

**User Story:** As a developer using the transcript package, I want existing code to continue working without modification, so that this feature does not introduce breaking changes.

**Acceptance Criteria:**

1. <a name="7.1"></a>The RenderOptions struct SHALL NOT require new mandatory fields
2. <a name="7.2"></a>Existing JSONL files without `id` or `tool_use_id` fields SHALL render correctly
3. <a name="7.3"></a>WHEN a tool_result cannot be matched to a tool_use, the system SHALL apply threshold-based collapsing as a fallback
4. <a name="7.4"></a>The existing truncation behavior (MaxToolInputRunes, MaxToolResultRunes) SHALL be preserved within collapsed blocks
5. <a name="7.5"></a>The collapse decision SHALL be made on the content's rune count BEFORE truncation is applied
6. <a name="7.6"></a>This feature changes output structure for certain tools; consumers parsing rendered output should expect `<details>` elements instead of heading-based formatting for collapsed tools
