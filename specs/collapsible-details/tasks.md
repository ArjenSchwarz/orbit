---
references:
    - requirements.md
    - design.md
    - decision_log.md
---
# Collapsible Details Blocks

## Phase 1: Data Model Changes

- [x] 1. Add ID and ToolUseID fields to ContentItem struct
  - Add ID string field with json tag id,omitempty
  - Add ToolUseID string field with json tag tool_use_id,omitempty
  - File: internal/transcript/types.go
  - Requirements: [6.1](requirements.md#6.1), [6.2](requirements.md#6.2), [6.3](requirements.md#6.3)
  - References: internal/transcript/types.go

- [x] 2. Update ContentItem UnmarshalJSON to parse new fields
  - Update contentItemAlias struct to include ID and ToolUseID fields
  - Copy ID and ToolUseID to ContentItem after unmarshaling
  - Requirements: [3.1](requirements.md#3.1), [3.2](requirements.md#3.2)
  - References: internal/transcript/types.go

- [x] 3. Add parser tests for ID fields
  - Add TestParseJSONL_IDField test
  - Add TestParseJSONL_ToolUseIDField test
  - Add TestParseJSONL_MissingIDFields test
  - Requirements: [3.1](requirements.md#3.1), [3.2](requirements.md#3.2), [6.3](requirements.md#6.3), [7.2](requirements.md#7.2)
  - References: internal/transcript/parser_test.go

## Phase 2: Helper Functions

- [x] 4. Add collapse threshold constant and toolMetadata type
  - Add CollapseThresholdRunes = 500 constant
  - Add toolMetadata struct with Name and Summary fields
  - File: internal/transcript/markdown.go
  - Requirements: [2.1](requirements.md#2.1)
  - References: internal/transcript/markdown.go

- [x] 5. Write tests for helper functions
  - TestGetToolSummary_Task - extracts subagent_type and description
  - TestGetToolSummary_TaskPartialFields - handles missing description gracefully
  - TestGetToolSummary_TaskFallback - returns empty for malformed input
  - TestGetToolSummary_Skill - extracts skill name
  - TestGetToolSummary_OtherTool - returns empty for non-Task/Skill
  - TestShouldCollapse_AlwaysTools - Task/Skill always return true
  - TestShouldCollapse_Threshold - correct behavior at 499, 500, 501 runes
  - TestEscapeSummary_XSS - escapes dangerous HTML
  - Requirements: [1.3](requirements.md#1.3), [1.4](requirements.md#1.4), [1.5](requirements.md#1.5), [1.6](requirements.md#1.6), [1.7](requirements.md#1.7), [2.2](requirements.md#2.2), [2.3](requirements.md#2.3)
  - References: internal/transcript/markdown_test.go

- [x] 6. Implement getToolSummary function
  - Handle Task tool: extract subagent_type and description from map
  - Handle partial fields: Explore without trailing colon when description empty
  - Handle Skill tool: extract skill field from map
  - Return empty string for other tools or extraction failures
  - Use defensive comma-ok type assertions
  - Requirements: [1.3](requirements.md#1.3), [1.4](requirements.md#1.4), [1.5](requirements.md#1.5), [1.6](requirements.md#1.6)
  - References: internal/transcript/markdown.go

- [x] 7. Implement shouldCollapse function
  - Return true for Task and Skill tools regardless of size
  - Return true if rune count exceeds CollapseThresholdRunes
  - Case-sensitive exact match for tool names
  - Requirements: [1.1](requirements.md#1.1), [1.2](requirements.md#1.2), [1.7](requirements.md#1.7), [2.2](requirements.md#2.2), [2.3](requirements.md#2.3)
  - References: internal/transcript/markdown.go

- [x] 8. Implement escapeSummary function
  - Wrap html.EscapeString for summary text
  - Prevents XSS and structural corruption in details elements
  - Requirements: [5.4](requirements.md#5.4)
  - References: internal/transcript/markdown.go

## Phase 3: Markdown Renderer

- [x] 9. Write Markdown renderer tests for collapsible tools
  - TestRenderMarkdown_TaskToolAlwaysCollapses
  - TestRenderMarkdown_TaskToolFallback
  - TestRenderMarkdown_SkillToolAlwaysCollapses
  - TestRenderMarkdown_SkillToolFallback
  - TestRenderMarkdown_ToolNameCaseSensitive
  - TestRenderMarkdown_ShortToolNoCollapse
  - TestRenderMarkdown_LongToolCollapses
  - TestRenderMarkdown_ExactThresholdNoCollapse
  - TestRenderMarkdown_ZeroLengthNoCollapse
  - Requirements: [1.1](requirements.md#1.1), [1.2](requirements.md#1.2), [1.4](requirements.md#1.4), [1.6](requirements.md#1.6), [1.7](requirements.md#1.7), [2.2](requirements.md#2.2), [2.3](requirements.md#2.3), [2.5](requirements.md#2.5)
  - References: internal/transcript/markdown_test.go

- [x] 10. Write Markdown renderer tests for tool results
  - TestRenderMarkdown_ResultMatchesToolUse
  - TestRenderMarkdown_UnmatchedResultThreshold
  - TestRenderMarkdown_ResultErrorIcon
  - TestRenderMarkdown_ZeroLengthResultNoCollapse
  - Requirements: [3.4](requirements.md#3.4), [3.5](requirements.md#3.5), [3.6](requirements.md#3.6), [3.7](requirements.md#3.7), [3.8](requirements.md#3.8), [3.10](requirements.md#3.10)
  - References: internal/transcript/markdown_test.go

- [x] 11. Write cross-entry tool matching tests
  - TestRenderMarkdown_CrossEntryToolMatching - tool_use in assistant, tool_result in user entry
  - TestRenderMarkdown_ToolResultInUserEntry
  - TestRenderMarkdown_MultipleToolsMatching
  - Requirements: [3.3](requirements.md#3.3), [3.4](requirements.md#3.4), [3.5](requirements.md#3.5)
  - References: internal/transcript/markdown_test.go

- [x] 12. Modify RenderMarkdown to initialize and pass toolMetadata map
  - Initialize toolMetadata map before entry loop
  - Pass map to formatAssistantMessage
  - Pass map to formatUserMessage
  - Requirements: [3.3](requirements.md#3.3)
  - References: internal/transcript/markdown.go

- [x] 13. Modify formatAssistantMessage to handle collapsible tool_use
  - Update function signature to accept toolMeta map
  - For tool_use: serialize with json.Marshal for threshold, store metadata in map
  - Check shouldCollapse and render with details wrapper if needed
  - Use json.MarshalIndent for display, escapeSummary for summary text
  - Handle zero-length input case
  - Requirements: [1.1](requirements.md#1.1), [1.2](requirements.md#1.2), [2.2](requirements.md#2.2), [2.3](requirements.md#2.3), [2.4](requirements.md#2.4), [2.5](requirements.md#2.5), [4.1](requirements.md#4.1), [4.3](requirements.md#4.3)
  - References: internal/transcript/markdown.go

- [x] 14. Modify formatUserMessage to handle tool_result blocks
  - Update function signature to accept toolMeta map
  - Iterate content items looking for tool_result type
  - Lookup ToolUseID in map to inherit collapse behavior
  - Apply threshold-based collapsing if not matched
  - Use escapeSummary and correct icon based on IsError
  - Requirements: [3.4](requirements.md#3.4), [3.5](requirements.md#3.5), [3.6](requirements.md#3.6), [3.7](requirements.md#3.7), [3.8](requirements.md#3.8), [3.9](requirements.md#3.9), [3.10](requirements.md#3.10), [4.2](requirements.md#4.2), [4.4](requirements.md#4.4)
  - References: internal/transcript/markdown.go

- [x] 15. Write tests for Markdown output format
  - TestRenderMarkdown_DetailsFormat - verify details/summary structure
  - TestRenderMarkdown_UncollapsedFormat - verify heading format unchanged
  - Requirements: [4.1](requirements.md#4.1), [4.2](requirements.md#4.2), [4.3](requirements.md#4.3), [4.4](requirements.md#4.4)
  - References: internal/transcript/markdown_test.go

## Phase 4: HTML Renderer

- [ ] 16. Add CSS styles for tool-collapsible class
  - Add details.tool-collapsible styles to htmlCSS constant
  - Style summary with cursor pointer and flex layout
  - Add .tool-content styles for collapsed content
  - Add .error variant for error icon color
  - Use existing theme variables for consistency
  - Requirements: [5.1](requirements.md#5.1), [5.4](requirements.md#5.4)
  - References: internal/transcript/html.go

- [ ] 17. Write HTML renderer tests for collapsible tools
  - TestRenderHTML_TaskToolCollapses
  - TestRenderHTML_SkillToolCollapses
  - TestRenderHTML_ShortToolNoCollapse
  - TestRenderHTML_LongToolCollapses
  - TestRenderHTML_CSSIncluded
  - Requirements: [5.1](requirements.md#5.1), [5.2](requirements.md#5.2), [5.4](requirements.md#5.4), [5.5](requirements.md#5.5)
  - References: internal/transcript/html_test.go

- [ ] 18. Write HTML renderer tests for tool results
  - TestRenderHTML_ResultCollapses
  - TestRenderHTML_ShortResultNoCollapse
  - TestRenderHTML_CrossEntryToolMatching
  - Requirements: [5.3](requirements.md#5.3), [5.6](requirements.md#5.6)
  - References: internal/transcript/html_test.go

- [ ] 19. Modify RenderHTML to initialize and pass toolMetadata map
  - Initialize toolMetadata map before entry loop
  - Pass map to formatAssistantMessageHTML
  - Pass map to formatUserMessageHTML
  - Requirements: [3.3](requirements.md#3.3)
  - References: internal/transcript/html.go

- [ ] 20. Modify formatAssistantMessageHTML to handle collapsible tool_use
  - Update function signature to accept toolMeta map
  - For tool_use: check shouldCollapse and render with details.tool-collapsible if needed
  - Store metadata in map keyed by ID
  - Apply html.EscapeString to all summary components
  - Requirements: [5.2](requirements.md#5.2), [5.5](requirements.md#5.5)
  - References: internal/transcript/html.go

- [ ] 21. Modify formatUserMessageHTML to handle tool_result blocks
  - Update function signature to accept toolMeta map
  - Handle tool_result content items
  - Lookup ToolUseID to inherit collapse behavior
  - Apply threshold-based collapsing if not matched
  - Use correct icon and error class based on IsError
  - Requirements: [5.3](requirements.md#5.3), [5.6](requirements.md#5.6)
  - References: internal/transcript/html.go

## Phase 5: Integration Tests

- [ ] 22. Create test data for collapsible blocks
  - Create testdata/collapsible/ directory
  - Add JSONL sample with Task tool_use and tool_result in separate entries
  - Add JSONL sample with Skill tool
  - Add JSONL sample with long tool output exceeding threshold
  - Add JSONL sample with short tool output below threshold
  - Requirements: [1.1](requirements.md#1.1), [1.2](requirements.md#1.2), [2.2](requirements.md#2.2), [2.3](requirements.md#2.3), [3.4](requirements.md#3.4)
  - References: internal/transcript/testdata/collapsible/

- [ ] 23. Create golden files for expected output
  - Create .golden files for Markdown output
  - Create .golden files for HTML output
  - Include both collapsed and uncollapsed tool examples
  - Requirements: [4.1](requirements.md#4.1), [4.2](requirements.md#4.2), [5.2](requirements.md#5.2), [5.3](requirements.md#5.3)
  - References: internal/transcript/testdata/collapsible/

- [ ] 24. Write golden file integration tests
  - TestRenderMarkdown_GoldenCollapsible
  - TestRenderHTML_GoldenCollapsible
  - Compare rendered output against golden files
  - Requirements: [4.1](requirements.md#4.1), [4.2](requirements.md#4.2), [5.2](requirements.md#5.2), [5.3](requirements.md#5.3)
  - References: internal/transcript/markdown_test.go, internal/transcript/html_test.go

- [ ] 25. Write backward compatibility tests
  - TestBackwardCompat_NoIDFields - verify old JSONL without id/tool_use_id renders
  - TestBackwardCompat_TruncationPreserved - verify MaxToolInputRunes still applies
  - TestBackwardCompat_PreTruncationDecision - verify collapse based on original length
  - Requirements: [7.2](requirements.md#7.2), [7.4](requirements.md#7.4), [7.5](requirements.md#7.5)
  - References: internal/transcript/markdown_test.go, internal/transcript/html_test.go

- [ ] 26. Run linter and fix any issues
  - Run make lint to check for issues
  - Fix any golangci-lint warnings
  - Run make modernize to apply modern Go idioms
  - References: Makefile
