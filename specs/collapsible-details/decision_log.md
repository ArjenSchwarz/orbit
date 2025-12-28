# Decision Log: Collapsible Details Blocks

## Decision 1: No User Control for Collapsing

**Date**: 2025-12-27
**Status**: accepted

### Context

When adding collapsible blocks to transcripts, we considered whether users should have the ability to disable this behavior via CLI flags or render options.

### Decision

Always apply collapsing logic as specified. No user control mechanism will be provided.

### Rationale

The collapsing behavior improves readability in all cases. Adding user control would increase complexity without clear benefit. Users who need full expanded output can manually expand the details blocks in their viewer.

### Alternatives Considered

- **Add --no-collapse flag**: CLI flag to disable collapsing - Rejected because it adds complexity without clear use case
- **Add render option**: Programmatic control via RenderOptions struct - Rejected for same reason; if needed later, can be added as non-breaking change

### Consequences

**Positive:**
- Simpler implementation
- Consistent output format
- No additional CLI flag documentation needed

**Negative:**
- Users cannot get fully-expanded output directly from the tool

---

## Decision 2: JSON-Serialized Length for Threshold

**Date**: 2025-12-27
**Status**: accepted

### Context

The 500-character threshold for collapsing non-Task/Skill tools needs a clear definition of what content is measured.

### Decision

Measure the JSON-serialized input length (the formatted JSON string) for threshold comparison.

### Rationale

This matches what users see in the output. The JSON-serialized form is already being generated for rendering, so measuring it avoids additional processing. It also provides a more accurate reflection of visual length in the transcript.

### Alternatives Considered

- **Raw content length**: Measure string content before formatting - Rejected because it doesn't reflect actual rendered length and would require additional logic to handle different input types

### Consequences

**Positive:**
- Consistent with rendered output
- No additional processing needed
- Predictable behavior for users

**Negative:**
- Formatting (indentation, spacing) affects the measurement

---

## Decision 3: Only Task and Skill Always Collapse

**Date**: 2025-12-27
**Status**: accepted

### Context

We considered whether tools other than Task and Skill should always be collapsed regardless of content length.

### Decision

Only Task and Skill tools always collapse. All other tools use threshold-based collapsing.

### Rationale

Task and Skill are unique in that they spawn subagents and produce substantial, often verbose output. Other tools like Read, Write, Grep, etc. have variable output lengths that are reasonably handled by the threshold mechanism.

### Alternatives Considered

- **Add more tools to always-collapse list**: Include tools like Bash, Grep - Rejected because their output length is highly variable and threshold-based handling is more appropriate

### Consequences

**Positive:**
- Simple, maintainable rule
- Other tools remain visible when output is short
- Consistent with user expectations

**Negative:**
- None identified

---

## Decision 4: Fallback to Threshold for Unmatched Results

**Date**: 2025-12-27
**Status**: accepted

### Context

Tool results are linked to tool uses via tool_use_id. When a tool_result cannot be matched to its corresponding tool_use (e.g., incomplete transcript, parsing issues), we need a fallback behavior.

### Decision

When a tool_result cannot be matched to a tool_use, apply threshold-based collapsing (500 runes).

### Rationale

This provides reasonable behavior for incomplete transcripts while still collapsing verbose output. It's the safest fallback that maintains readability without incorrectly assuming the result came from a Task/Skill tool.

### Alternatives Considered

- **Never collapse unmatched**: Only collapse if confirmed Task/Skill - Rejected because long unmatched results would still harm readability
- **Always collapse unmatched**: Collapse any unmatched result - Rejected because it could hide short, useful output unnecessarily

### Consequences

**Positive:**
- Graceful degradation for incomplete transcripts
- Maintains readability for long outputs
- Short outputs remain visible

**Negative:**
- May not correctly identify Task/Skill results in some edge cases

---

## Decision 5: Use Runes for Threshold Measurement

**Date**: 2025-12-27
**Status**: accepted

### Context

The collapse threshold needs a unit of measurement. The existing truncation logic uses runes (Unicode code points) via `utf8.RuneCountInString()`. The initial requirements specified "characters" which is ambiguous in Go.

### Decision

Use runes for threshold measurement, consistent with existing truncation logic. The threshold is 500 runes.

### Rationale

Consistency with existing code (`MaxToolInputRunes`, `MaxToolResultRunes`). In Go, `len(s)` returns bytes, not characters, which could cause incorrect behavior with multi-byte UTF-8 characters. Rune counting provides consistent behavior across all Unicode content.

### Alternatives Considered

- **Byte count**: Use `len(s)` - Rejected because it's inconsistent with existing code and can split multi-byte characters
- **Grapheme clusters**: More accurate for display width - Rejected because it adds complexity and existing code uses runes

### Consequences

**Positive:**
- Consistent with existing truncation behavior
- Correct handling of Unicode content
- No surprising behavior with non-ASCII text

**Negative:**
- Slightly more expensive than byte counting (negligible)

---

## Decision 6: Pre-Truncation Collapse Decision

**Date**: 2025-12-27
**Status**: accepted

### Context

The order of operations between truncation and collapse decision needed clarification. Content may be truncated by existing logic before rendering.

### Decision

The collapse decision is made based on the content's rune count BEFORE any truncation is applied. Truncation is then applied to content within the collapsed block.

### Rationale

Users expect large outputs to be collapsed even if they will be truncated. A 5000-rune result should still collapse even though it will be truncated to 3000 runes for display.

### Alternatives Considered

- **Post-truncation decision**: Collapse based on truncated length - Rejected because it would incorrectly show large results as uncollapsed if truncated below threshold

### Consequences

**Positive:**
- Intuitive behavior for users
- Large outputs always collapse regardless of truncation

**Negative:**
- None identified

---

## Decision 7: Acknowledge Output Format Change

**Date**: 2025-12-27
**Status**: accepted

### Context

This feature changes the output structure for certain tools. Previously all tools used heading-based formatting; now some will use `<details>` elements. This could break downstream consumers that parse the output.

### Decision

Acknowledge this as an intentional output format change in the requirements. The change improves readability and is considered acceptable.

### Rationale

The benefit of improved readability outweighs the cost to downstream consumers. Consumers parsing Markdown/HTML output were never guaranteed a stable format.

### Alternatives Considered

- **Version the output format**: Add format version header - Rejected as overengineering for this use case
- **Add compatibility mode**: Flag to use old format - Already rejected in Decision 1

### Consequences

**Positive:**
- Clear documentation of the change
- Users know to expect different output

**Negative:**
- Scripts that grep for `### 🔧 Tool:` may break

---

## Decision 8: Render-Level Tool Metadata Map Scope

**Date**: 2025-12-27
**Status**: accepted

### Context

During design review, it was discovered that in Claude JSONL transcripts, `tool_use` blocks appear in "assistant" type entries while `tool_result` blocks appear in "user" type entries. The initial design scoped the metadata map to `formatAssistantMessage`, which would never find matching results.

### Decision

The tool metadata map is initialized at the render function level (`RenderMarkdown`/`RenderHTML`) and passed to both `formatAssistantMessage` and `formatUserMessage`. Both functions can populate and read from the shared map.

### Rationale

The JSONL structure requires cross-entry matching. A render-level scope ensures the map persists across all entries in a single rendering pass.

### Alternatives Considered

- **Two-pass rendering**: First pass builds map, second pass renders - Rejected because it doubles processing time and adds complexity
- **Entry-level scope with result buffering**: Buffer tool_results until matching tool_use found - Rejected because results always follow their tool_use chronologically

### Consequences

**Positive:**
- Correct matching of tool_result to tool_use across entries
- Maintains single-pass rendering efficiency
- Simple implementation with shared map parameter

**Negative:**
- Function signatures change to accept map parameter
- Map grows with number of tools in transcript (memory bounded by transcript size)

---

## Decision 9: Escape Summary Text for Security

**Date**: 2025-12-27
**Status**: accepted

### Context

Summary text in `<details>` elements is derived from tool inputs (subagent_type, description, skill). Malicious or malformed inputs could contain HTML entities or structure-breaking sequences like `</summary>`.

### Decision

Apply `html.EscapeString()` to all user-derived summary text components before inserting them into both HTML and Markdown output. Markdown uses inline HTML for `<details>` elements, so escaping is required for both formats.

### Rationale

Defense in depth against XSS and structural corruption. Even though transcripts are typically from trusted sources, escaping is cheap and prevents edge cases.

### Alternatives Considered

- **No escaping**: Trust input - Rejected because it creates potential security vulnerabilities
- **Sanitize instead of escape**: Remove dangerous content - Rejected because escaping is simpler and preserves content

### Consequences

**Positive:**
- Prevents XSS attacks in HTML output
- Prevents structural corruption in Markdown
- Consistent with existing html.EscapeString usage in the codebase

**Negative:**
- Summary text with `<`, `>`, `&` will show as escaped entities

---
