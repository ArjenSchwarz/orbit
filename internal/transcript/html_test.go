package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenderHTML_BasicStructure(t *testing.T) {
	entries := []Entry{}
	result := RenderHTML(entries, RenderOptions{})

	// Check for valid HTML structure
	assert.Contains(t, result, "<!DOCTYPE html>", "expected DOCTYPE declaration")
	assert.Contains(t, result, "<html lang=\"en\">", "expected html tag with lang attribute")
	assert.Contains(t, result, "<head>", "expected head tag")
	assert.Contains(t, result, "<body>", "expected body tag")
	assert.Contains(t, result, "</html>", "expected closing html tag")
}

func TestRenderHTML_EmbeddedCSS(t *testing.T) {
	entries := []Entry{}
	result := RenderHTML(entries, RenderOptions{})

	assert.Contains(t, result, "<style>", "expected embedded style tag")
	assert.Contains(t, result, ".message", "expected message class in CSS")
	assert.Contains(t, result, ".user", "expected user class in CSS")
	assert.Contains(t, result, ".assistant", "expected assistant class in CSS")
}

func TestRenderHTML_UserMessage(t *testing.T) {
	entries := []Entry{
		{
			Type: "user",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{Type: "text", Text: "Hello, Claude!"},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	assert.Contains(t, result, `class="message user"`, "expected user message class")
	assert.Contains(t, result, "👤", "expected user icon")
	assert.Contains(t, result, "Hello, Claude!", "expected message content")
}

func TestRenderHTML_AssistantMessage(t *testing.T) {
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{Type: "text", Text: "Hello! How can I help?"},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	assert.Contains(t, result, `class="message assistant"`, "expected assistant message class")
	assert.Contains(t, result, "🤖", "expected assistant icon")
	assert.Contains(t, result, "Hello! How can I help?", "expected message content")
}

func TestRenderHTML_ThinkingBlock(t *testing.T) {
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{Type: "thinking", Thinking: "Let me think about this..."},
					{Type: "text", Text: "Here's my answer."},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	assert.Contains(t, result, `<div class="thinking-block">`, "expected thinking block div")
	assert.Contains(t, result, `<div class="thinking-header">💭 Thinking</div>`, "expected thinking header")
	assert.Contains(t, result, "Let me think about this...", "expected thinking content")
	assert.Contains(t, result, `<div class="thinking-content markdown-content">`, "expected thinking content div with markdown class")
}

func TestRenderHTML_ToolUse(t *testing.T) {
	// Test that non-Task/Skill tool_use stores metadata but doesn't render until result
	// Use Bash tool with matching tool_result to test combined rendering
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type:  "tool_use",
						Name:  "Bash",
						ID:    "bash-test",
						Input: map[string]any{"command": "echo hello", "description": "Print hello"},
					},
				},
			},
		},
		{
			Type: "user",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{
						Type:      "tool_result",
						ToolUseID: "bash-test",
						Content:   "hello",
						IsError:   false,
					},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	// Combined tool call + result should be in a collapsible block
	assert.Contains(t, result, `<details class="tool-collapsible">`, "expected combined tool block to use details.tool-collapsible")
	assert.Contains(t, result, "🔧", "expected tool icon")
	assert.Contains(t, result, "Bash: Print hello", "expected tool name and description in summary")
	assert.Contains(t, result, "Command:", "expected Command input section")
	assert.Contains(t, result, "Result:", "expected Result section")
}

func TestRenderHTML_ToolResultSuccess(t *testing.T) {
	// Unmatched tool_result (legacy/standalone) renders in collapsible block
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{Type: "tool_result", Content: "File contents here", IsError: false},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	// Unmatched results render in collapsible blocks
	assert.Contains(t, result, `<details class="tool-collapsible">`, "expected unmatched tool_result to use details.tool-collapsible")
	assert.Contains(t, result, "✅", "expected success icon")
	assert.Contains(t, result, "Tool Result", "expected Tool Result summary")
	assert.Contains(t, result, "File contents here", "expected result content")
}

func TestRenderHTML_ToolResultError(t *testing.T) {
	// Unmatched error tool_result renders in collapsible block with error class
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{Type: "tool_result", Content: "Error: file not found", IsError: true},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	// Error results have error class on the collapsible
	assert.Contains(t, result, `<details class="tool-collapsible error">`, "expected error class on collapsible")
	assert.Contains(t, result, "❌", "expected error icon")
	assert.Contains(t, result, "Tool Error", "expected Tool Error summary")
	assert.Contains(t, result, "Error: file not found", "expected error content")
}

func TestRenderHTML_TitleCustomization(t *testing.T) {
	entries := []Entry{}

	opts := RenderOptions{
		Title:     "Phase 1 Session Transcript",
		SessionID: "test-session-123",
	}

	result := RenderHTML(entries, opts)

	assert.Contains(t, result, "<title>Phase 1 Session Transcript</title>", "expected custom title in head")
	assert.Contains(t, result, "<h1>Phase 1 Session Transcript</h1>", "expected custom title in header")
	assert.Contains(t, result, `<code>test-session-123</code>`, "expected session ID in code tag")
}

func TestRenderHTML_DefaultTitle(t *testing.T) {
	entries := []Entry{}
	result := RenderHTML(entries, RenderOptions{})

	assert.Contains(t, result, "<title>Session Transcript</title>", "expected default title")
}

func TestRenderHTML_CostDisplay(t *testing.T) {
	tests := map[string]struct {
		totalCost *float64
		costUnit  string
		wantCost  bool
		expected  string
	}{
		"cost displayed with credits": {
			totalCost: ptrHTML(0.14),
			costUnit:  "credits",
			wantCost:  true,
			expected:  "Cost: 0.14 credits",
		},
		"cost displayed with default unit": {
			totalCost: ptrHTML(0.14),
			costUnit:  "",
			wantCost:  true,
			expected:  "Cost: 0.14 credits",
		},
		"cost displayed with custom unit": {
			totalCost: ptrHTML(1.50),
			costUnit:  "USD",
			wantCost:  true,
			expected:  "Cost: 1.50 USD",
		},
		"cost rounded to 2 decimals": {
			totalCost: ptrHTML(0.139),
			costUnit:  "credits",
			wantCost:  true,
			expected:  "Cost: 0.14 credits",
		},
		"cost at threshold displayed": {
			totalCost: ptrHTML(0.005),
			costUnit:  "credits",
			wantCost:  true,
			expected:  "Cost: 0.01 credits",
		},
		"cost below threshold not displayed": {
			totalCost: ptrHTML(0.004),
			costUnit:  "credits",
			wantCost:  false,
		},
		"nil cost not displayed": {
			totalCost: nil,
			costUnit:  "credits",
			wantCost:  false,
		},
		"zero cost not displayed": {
			totalCost: ptrHTML(0.0),
			costUnit:  "credits",
			wantCost:  false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			entries := []Entry{}
			opts := RenderOptions{
				TotalCost: tc.totalCost,
				CostUnit:  tc.costUnit,
			}

			result := RenderHTML(entries, opts)

			if tc.wantCost {
				assert.Contains(t, result, tc.expected, "expected %q in output, got:\n%s", tc.expected, result)
				// Verify cost is displayed with proper HTML element
				assert.Contains(t, result, `<p class="session-cost">`, "expected session-cost paragraph in output")
			} else {
				// Check that no actual cost paragraph is rendered (CSS has .session-cost class definition)
				assert.NotContains(t, result, `<p class="session-cost">`, "expected no cost paragraph in output, got:\n%s", result)
			}
		})
	}
}

// ptrHTML returns a pointer to the given float64 value.
func ptrHTML(v float64) *float64 {
	return &v
}

func TestRenderHTML_HTMLEscaping(t *testing.T) {
	entries := []Entry{
		{
			Type: "user",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{Type: "text", Text: "<script>alert('xss')</script>"},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	// User-provided content should be escaped — the XSS payload must not appear as raw HTML
	assert.NotContains(t, result, "<script>alert", "user-provided script content should be escaped")
	assert.Contains(t, result, "&lt;script&gt;", "expected escaped script tag")
}

func TestRenderHTML_SpecialCharactersInTitle(t *testing.T) {
	entries := []Entry{}

	opts := RenderOptions{
		Title:     "Test <Title> & More",
		SessionID: "session&<>\"",
	}

	result := RenderHTML(entries, opts)

	assert.NotContains(t, result, "<Title>", "title should be escaped")
	assert.Contains(t, result, "&lt;Title&gt;", "expected escaped title")
}

func TestRenderHTML_UnknownContentTypes(t *testing.T) {
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{Type: "unknown_type", Text: "Should be skipped"},
					{Type: "text", Text: "Should be visible"},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	assert.NotContains(t, result, "Should be skipped", "unknown content type should be skipped")
	assert.Contains(t, result, "Should be visible", "known content type should be visible")
}

func TestRenderHTML_UnknownEntryTypes(t *testing.T) {
	entries := []Entry{
		{
			Type: "queue-operation",
			Message: &Message{
				Role:    "system",
				Content: []ContentItem{{Type: "text", Text: "Should be skipped"}},
			},
		},
		{
			Type: "user",
			Message: &Message{
				Role:    "user",
				Content: []ContentItem{{Type: "text", Text: "Should be visible"}},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	assert.NotContains(t, result, "queue-operation", "unknown entry type should be skipped")
	assert.Contains(t, result, "Should be visible", "known entry type should be visible")
}

func TestRenderHTML_EmptyUserMessage(t *testing.T) {
	entries := []Entry{
		{
			Type: "user",
			Message: &Message{
				Role:    "user",
				Content: []ContentItem{},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	// Should not render empty user message section
	assert.NotContains(t, result, `class="message user"`, "empty user message should not be rendered")
}

func TestRenderHTML_NilMessage(t *testing.T) {
	entries := []Entry{
		{
			Type:    "user",
			Message: nil,
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	// Should not panic and should not render message section
	assert.NotContains(t, result, `class="message user"`, "nil message should not be rendered")
}

func TestRenderHTML_DarkModeSupport(t *testing.T) {
	entries := []Entry{}
	result := RenderHTML(entries, RenderOptions{})

	assert.Contains(t, result, "@media (prefers-color-scheme: dark)", "expected dark mode media query")
}

func TestRenderHTML_ResponsiveViewport(t *testing.T) {
	entries := []Entry{}
	result := RenderHTML(entries, RenderOptions{})

	assert.Contains(t, result, `<meta name="viewport"`, "expected viewport meta tag")
}

func TestRenderHTML_UTF8Charset(t *testing.T) {
	entries := []Entry{}
	result := RenderHTML(entries, RenderOptions{})

	assert.Contains(t, result, `<meta charset="UTF-8">`, "expected UTF-8 charset")
}

func TestRenderHTML_TruncationRuneBoundary(t *testing.T) {
	// Use the same test file as markdown tests
	f, err := os.Open(filepath.Join("testdata", "unicode.jsonl"))
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer func() { _ = f.Close() }()

	parseResult, err := ParseJSONL(f)
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	htmlResult := RenderHTML(parseResult.Entries, RenderOptions{})

	// The output should be valid UTF-8 (no broken characters)
	// Check for common emoji that should be preserved
	hasEmoji := strings.Contains(htmlResult, "🤖") || strings.Contains(htmlResult, "✅")
	assert.True(t, hasEmoji, "expected valid emoji in output")
}

func TestRenderHTML_MultipleMessages(t *testing.T) {
	entries := []Entry{
		{
			Type: "user",
			Message: &Message{
				Role:    "user",
				Content: []ContentItem{{Type: "text", Text: "First message"}},
			},
		},
		{
			Type: "assistant",
			Message: &Message{
				Role:    "assistant",
				Content: []ContentItem{{Type: "text", Text: "Response"}},
			},
		},
		{
			Type: "user",
			Message: &Message{
				Role:    "user",
				Content: []ContentItem{{Type: "text", Text: "Second message"}},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	// Count message sections
	userCount := strings.Count(result, `class="message user"`)
	assistantCount := strings.Count(result, `class="message assistant"`)

	if userCount != 2 {
		t.Errorf("expected 2 user messages, got %d", userCount)
	}
	if assistantCount != 1 {
		t.Errorf("expected 1 assistant message, got %d", assistantCount)
	}
}

func TestRenderHTML_TaskToolCollapses(t *testing.T) {
	// Subagent Task tools defer rendering to tool_result (combined block)
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type: "tool_use",
						Name: "Task",
						ID:   "task-123",
						Input: map[string]any{
							"subagent_type": "Explore",
							"description":   "Search for config files",
							"prompt":        "Find all configuration files",
						},
					},
				},
			},
		},
		{
			Type: "user",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{
						Type:      "tool_result",
						ToolUseID: "task-123",
						Content:   "Found config files",
						IsError:   false,
					},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	assert.Contains(t, result, `<details class="tool-collapsible">`, "expected subagent Task tool to use details.tool-collapsible")
	assert.Contains(t, result, "Explore: Search for config files", "expected summary to contain subagent_type and description")
	assert.Contains(t, result, "🤖🔧", "expected robot and tool icons in summary for subagent")
	assert.Contains(t, result, "<strong>Prompt:</strong>", "expected Prompt section in subagent block")
	assert.Contains(t, result, "<strong>Result:</strong>", "expected Result section in subagent block")
}

func TestRenderHTML_SkillToolRendersSimple(t *testing.T) {
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type:  "tool_use",
						Name:  "Skill",
						ID:    "skill-123",
						Input: map[string]any{"skill": "next-task"},
					},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	// Skill should render as a simple div, not a collapsible details
	assert.Contains(t, result, `<div class="tool-use">`, "expected Skill tool to use div.tool-use")
	assert.Contains(t, result, "Skill: next-task", "expected content to contain skill name")
	// Should NOT have collapsible details
	assert.NotContains(t, result, `<details class="tool-collapsible">`, "Skill tool should not use collapsible details")
}

func TestRenderHTML_SkillToolWithDescription(t *testing.T) {
	// When a skill has a meta entry with sourceToolUseID linking to it,
	// the skill description should be rendered as collapsible content.
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type: "tool_use",
						Name: "Skill",
						ID:   "skill_123",
						Input: map[string]any{
							"skill": "permission-analyzer",
						},
					},
				},
			},
		},
		{
			Type:            "user",
			IsMeta:          true,
			SourceToolUseID: "skill_123",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{
						Type: "text",
						Text: "This skill analyzes permissions and generates config.",
					},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	// Skill with description should be collapsible
	assert.Contains(t, result, `<details class="tool-collapsible">`, "expected Skill with description to be wrapped in details")
	assert.Contains(t, result, "Skill: permission-analyzer", "expected Skill tool to show skill name in summary")
	assert.Contains(t, result, "This skill analyzes permissions", "expected skill description to be rendered")
}

func TestRenderHTML_ShortToolNoCollapse(t *testing.T) {
	// In the new combined format, all non-Task/Skill tools use collapsible blocks
	// even for short content, because they combine tool_use + tool_result
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type:  "tool_use",
						Name:  "Bash",
						ID:    "bash-123",
						Input: map[string]any{"command": "echo hello"},
					},
				},
			},
		},
		{
			Type: "user",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{
						Type:      "tool_result",
						ToolUseID: "bash-123",
						Content:   "hello",
						IsError:   false,
					},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	// Combined tool call + result always uses collapsible block
	assert.Contains(t, result, `<details class="tool-collapsible">`, "expected combined tool block to use details.tool-collapsible")
	assert.Contains(t, result, "Bash", "expected tool name in summary")
}

func TestRenderHTML_LongToolCollapses(t *testing.T) {
	// All non-Task/Skill tools now use combined collapsible blocks
	longContent := strings.Repeat("a", 600)
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type:  "tool_use",
						Name:  "Write",
						ID:    "write-123",
						Input: map[string]any{"content": longContent, "file_path": "/path/to/file.txt"},
					},
				},
			},
		},
		{
			Type: "user",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{
						Type:      "tool_result",
						ToolUseID: "write-123",
						Content:   "File written successfully",
						IsError:   false,
					},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	assert.Contains(t, result, `<details class="tool-collapsible">`, "combined tool block should use details.tool-collapsible")
	assert.Contains(t, result, "Write", "expected summary to contain tool name")
}

func TestRenderHTML_CSSIncluded(t *testing.T) {
	entries := []Entry{}
	result := RenderHTML(entries, RenderOptions{})

	assert.Contains(t, result, "details.tool-collapsible", "expected CSS for details.tool-collapsible")
	assert.Contains(t, result, ".tool-content", "expected CSS for .tool-content")
	assert.Contains(t, result, "details.tool-collapsible.error", "expected CSS for .error variant")
}

func TestRenderHTML_ResultCollapses(t *testing.T) {
	// Subagent Task tools combine tool_use and tool_result into a single block
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type: "tool_use",
						Name: "Task",
						ID:   "task-456",
						Input: map[string]any{
							"subagent_type": "Explore",
							"description":   "Find files",
							"prompt":        "Find all files matching the pattern",
						},
					},
				},
			},
		},
		{
			Type: "user",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{
						Type:      "tool_result",
						ToolUseID: "task-456",
						Content:   "Found 10 files matching the pattern",
						IsError:   false,
					},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	// Subagent combines into 1 block with both Prompt and Result sections
	detailsCount := strings.Count(result, `<details class="tool-collapsible">`)
	if detailsCount != 1 {
		t.Errorf("expected 1 combined collapsible block for subagent, got %d", detailsCount)
	}
	assert.Contains(t, result, "✅", "expected success icon in result")
	assert.Contains(t, result, "🤖🔧", "expected robot and tool icons for subagent")
}

func TestRenderHTML_ShortResultNoCollapse(t *testing.T) {
	// In the new combined format, all non-Task/Skill tools use collapsible blocks
	// The combined block contains both the tool call and result
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type:  "tool_use",
						Name:  "Bash",
						ID:    "bash-456",
						Input: map[string]any{"command": "echo hello"},
					},
				},
			},
		},
		{
			Type: "user",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{
						Type:      "tool_result",
						ToolUseID: "bash-456",
						Content:   "Short content",
						IsError:   false,
					},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	// Combined tool call + result uses collapsible block
	assert.Contains(t, result, `<details class="tool-collapsible">`, "expected combined tool block to use details.tool-collapsible")
	// Should have exactly 1 collapsible block (the combined tool call + result)
	detailsCount := strings.Count(result, `<details class="tool-collapsible">`)
	if detailsCount != 1 {
		t.Errorf("expected 1 collapsible block for combined tool call + result, got %d", detailsCount)
	}
}

func TestRenderHTML_CrossEntryToolMatching(t *testing.T) {
	// Verify that tool_result in user entry matches tool_use in assistant entry
	// For Skill tools, the result is skipped (only tool_use renders)
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type: "tool_use",
						Name: "Skill",
						ID:   "skill-789",
						Input: map[string]any{
							"skill": "commit",
						},
					},
				},
			},
		},
		{
			Type: "user",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{
						Type:      "tool_result",
						ToolUseID: "skill-789",
						Content:   "Launching skill: commit",
						IsError:   false,
					},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	// Skill should render as simple div with skill name
	assert.Contains(t, result, "Skill: commit", "expected Skill summary in tool_use")
	assert.Contains(t, result, `<div class="tool-use">`, "expected Skill to use div.tool-use")
	// Skill result should be skipped (not rendered)
	assert.NotContains(t, result, "Launching skill: commit", "Skill result should not be rendered")
}

func TestRenderHTML_ResultErrorWithCollapse(t *testing.T) {
	// Error result for Task tool should collapse with error class
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type: "tool_use",
						Name: "Task",
						ID:   "task-error",
						Input: map[string]any{
							"subagent_type": "Explore",
							"description":   "Find missing files",
						},
					},
				},
			},
		},
		{
			Type: "user",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{
						Type:      "tool_result",
						ToolUseID: "task-error",
						Content:   "Error: No files found",
						IsError:   true,
					},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	// Error result should have error class
	assert.Contains(t, result, `<details class="tool-collapsible error">`, "expected error class on collapsible result")
	assert.Contains(t, result, "❌", "expected error icon in result")
}

// Golden file integration tests for collapsible blocks (HTML)

func TestRenderHTML_GoldenCollapsible_TaskTool(t *testing.T) {
	// Subagent Task tools now render with 🤖🔧 and Prompt/Result sections
	testGoldenCollapsibleHTML(t, "task_tool", []string{
		`<details class="tool-collapsible">`,
		`🤖🔧 Explore: Search for config files`,
		`<strong>Prompt:</strong>`,
		`<strong>Result:</strong>`,
		"Found 3 config files",
	})
}

func TestRenderHTML_GoldenCollapsible_SkillTool(t *testing.T) {
	// Skill tools now render as simple div (not collapsible) and result is skipped
	testGoldenCollapsibleHTML(t, "skill_tool", []string{
		`<div class="tool-use">`,
		`Skill: next-task`,
	})
}

func TestRenderHTML_GoldenCollapsible_LongOutput(t *testing.T) {
	// Bash tool with long input - now uses combined collapsible format
	testGoldenCollapsibleHTML(t, "long_output", []string{
		`<details class="tool-collapsible">`,
		"🔧 Bash:",
		"Command:",
		"Result:",
		"Line 1: This is a very long file content",
	})
}

func TestRenderHTML_GoldenCollapsible_ShortOutput(t *testing.T) {
	// Bash tool with short input - now uses combined collapsible format (same as long)
	testGoldenCollapsibleHTML(t, "short_output", []string{
		`<details class="tool-collapsible">`,
		"🔧 Bash",
		"Command:",
		"Result:",
		"Hello, World!",
	})
}

func testGoldenCollapsibleHTML(t *testing.T, name string, expectedPatterns []string) {
	t.Helper()

	// Read JSONL input
	jsonlPath := filepath.Join("testdata", "collapsible", name+".jsonl")
	f, err := os.Open(jsonlPath)
	if err != nil {
		t.Fatalf("failed to open test file %s: %v", jsonlPath, err)
	}
	defer func() { _ = f.Close() }()

	parseResult, err := ParseJSONL(f)
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	// Render to HTML
	result := RenderHTML(parseResult.Entries, RenderOptions{})

	// Verify expected patterns are present
	for _, pattern := range expectedPatterns {
		assert.Contains(t, result, pattern, "expected pattern not found: %q\nIn output:\n%s", pattern, result)
	}
}

// Backward compatibility tests (Task 25) for HTML

func TestBackwardCompat_NoIDFields_HTML(t *testing.T) {
	// Test that old JSONL without id/tool_use_id fields renders correctly
	// When IDs don't match, tool_result renders as standalone collapsible
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type:  "tool_use",
						Name:  "Bash",
						ID:    "", // Empty ID (old format)
						Input: map[string]any{"command": "echo hello"},
					},
				},
			},
		},
		{
			Type: "user",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{
						Type:      "tool_result",
						ToolUseID: "", // Empty ToolUseID (old format)
						Content:   "hello",
						IsError:   false,
					},
				},
			},
		},
	}

	// Should render without panicking
	result := RenderHTML(entries, RenderOptions{})

	// With no matching IDs, tool_use stores metadata but doesn't render
	// tool_result renders as standalone collapsible with "Tool Result" summary
	assert.Contains(t, result, `<details class="tool-collapsible">`, "expected unmatched tool_result to use details.tool-collapsible")
	assert.Contains(t, result, "Tool Result", "expected unmatched tool_result to have 'Tool Result' summary")
	assert.Contains(t, result, "hello", "expected tool result content to be rendered")
}

func TestBackwardCompat_TruncationPreserved_HTML(t *testing.T) {
	// Test that long content is NOT truncated (truncation removed with <details> blocks)
	longInput := strings.Repeat("x", MaxToolInputRunes+500)
	longResult := strings.Repeat("y", MaxToolResultRunes+500)

	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type:  "tool_use",
						Name:  "Write",
						ID:    "trunc_001",
						Input: map[string]any{"content": longInput},
					},
				},
			},
		},
		{
			Type: "user",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{
						Type:      "tool_result",
						ToolUseID: "trunc_001",
						Content:   longResult,
						IsError:   false,
					},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	// Should be collapsed
	assert.Contains(t, result, `<details class="tool-collapsible">`, "long tool should be collapsed")

	// Should NOT be truncated (no truncation marker)
	assert.NotContains(t, result, "... (truncated)", "content should not be truncated with <details> blocks")

	// Verify the FULL result content IS present (no truncation)
	assert.Contains(t, result, longResult, "full result should be present, not truncated")
}

func TestBackwardCompat_PreTruncationDecision_HTML(t *testing.T) {
	// Test that collapse decision is made BEFORE truncation
	content := strings.Repeat("z", CollapseThresholdRunes+100) // 600 runes

	entries := []Entry{
		{
			Type: "user",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{
						Type:      "tool_result",
						ToolUseID: "unmatched_id",
						Content:   content,
						IsError:   false,
					},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	// Should be collapsed because original content exceeds threshold
	assert.Contains(t, result, `<details class="tool-collapsible">`, "result exceeding threshold should be collapsed")

	// Content should NOT be truncated (it's under MaxToolResultRunes)
	assert.NotContains(t, result, "... (truncated)", "content under MaxToolResultRunes should not be truncated")
}

// Navigation tests for RenderHTMLFragment

func TestRenderHTMLFragment_BasicStructure(t *testing.T) {
	entries := []Entry{
		{
			Type: "user",
			Message: &Message{
				Role:    "user",
				Content: []ContentItem{{Type: "text", Text: "Hello"}},
			},
		},
	}

	result := RenderHTMLFragment(entries, RenderOptions{})

	// Should NOT contain document wrapper elements
	assert.NotContains(t, result, "<!DOCTYPE html>", "fragment should not contain DOCTYPE")
	assert.NotContains(t, result, "<html", "fragment should not contain html tag")
	assert.NotContains(t, result, "<head>", "fragment should not contain head tag")
	assert.NotContains(t, result, "<body>", "fragment should not contain body tag")

	// Should contain the message content
	assert.Contains(t, result, "Hello", "fragment should contain message content")
	assert.Contains(t, result, `class="message user"`, "fragment should contain user message section")
}

func TestRenderHTMLFragment_WithNavigation(t *testing.T) {
	entries := []Entry{
		{
			Type: "user",
			Message: &Message{
				Role:    "user",
				Content: []ContentItem{{Type: "text", Text: "Test message"}},
			},
		},
	}

	nav := &NavigationContext{
		PrevURL:  "/runs/abc/transcript/1",
		PrevText: "Phase 1",
		NextURL:  "/runs/abc/transcript/3",
		NextText: "Phase 3",
		BackURL:  "/runs/abc",
		BackText: "Back to Run",
	}

	result := RenderHTMLFragment(entries, RenderOptions{Navigation: nav})

	// Should contain navigation at top and bottom
	navCount := strings.Count(result, `class="transcript-nav"`)
	if navCount != 2 {
		t.Errorf("expected 2 navigation blocks (top and bottom), got %d", navCount)
	}

	// Should contain prev link
	assert.Contains(t, result, `href="/runs/abc/transcript/1"`, "expected prev link")
	assert.Contains(t, result, "Phase 1", "expected prev text")

	// Should contain next link
	assert.Contains(t, result, `href="/runs/abc/transcript/3"`, "expected next link")
	assert.Contains(t, result, "Phase 3", "expected next text")

	// Should contain back link
	assert.Contains(t, result, `href="/runs/abc"`, "expected back link")
	assert.Contains(t, result, "Back to Run", "expected back text")
}

func TestRenderHTMLFragment_NavigationPrevOnly(t *testing.T) {
	entries := []Entry{}

	nav := &NavigationContext{
		PrevURL:  "/runs/abc/transcript/1",
		PrevText: "Phase 1",
		BackURL:  "/runs/abc",
		BackText: "Back to Run",
	}

	result := RenderHTMLFragment(entries, RenderOptions{Navigation: nav})

	// Should contain prev link
	assert.Contains(t, result, `href="/runs/abc/transcript/1"`, "expected prev link")

	// Should NOT contain next link (empty NextURL)
	assert.NotContains(t, result, "nav-next", "should not contain next link element when NextURL is empty")
}

func TestRenderHTMLFragment_NavigationNextOnly(t *testing.T) {
	entries := []Entry{}

	nav := &NavigationContext{
		NextURL:  "/runs/abc/transcript/2",
		NextText: "Phase 2",
		BackURL:  "/runs/abc",
		BackText: "Back to Run",
	}

	result := RenderHTMLFragment(entries, RenderOptions{Navigation: nav})

	// Should NOT contain prev link (empty PrevURL)
	assert.NotContains(t, result, "nav-prev", "should not contain prev link element when PrevURL is empty")

	// Should contain next link
	assert.Contains(t, result, `href="/runs/abc/transcript/2"`, "expected next link")
}

func TestRenderHTMLFragment_NoNavigation(t *testing.T) {
	entries := []Entry{
		{
			Type: "user",
			Message: &Message{
				Role:    "user",
				Content: []ContentItem{{Type: "text", Text: "Test"}},
			},
		},
	}

	result := RenderHTMLFragment(entries, RenderOptions{})

	// Should NOT contain navigation
	assert.NotContains(t, result, `class="transcript-nav"`, "should not contain navigation when Navigation is nil")
}

func TestRenderHTML_WithNavigation(t *testing.T) {
	entries := []Entry{
		{
			Type: "user",
			Message: &Message{
				Role:    "user",
				Content: []ContentItem{{Type: "text", Text: "Test message"}},
			},
		},
	}

	nav := &NavigationContext{
		PrevURL:  "/runs/abc/transcript/1",
		PrevText: "Phase 1",
		NextURL:  "/runs/abc/transcript/3",
		NextText: "Phase 3",
		BackURL:  "/runs/abc",
		BackText: "Back to Run",
	}

	result := RenderHTML(entries, RenderOptions{Navigation: nav})

	// Should contain document wrapper
	assert.Contains(t, result, "<!DOCTYPE html>", "expected DOCTYPE in full HTML")

	// Should contain navigation
	navCount := strings.Count(result, `class="transcript-nav"`)
	if navCount != 2 {
		t.Errorf("expected 2 navigation blocks, got %d", navCount)
	}

	// Navigation should be between header and main content
	assert.Contains(t, result, "</header>", "expected header in full HTML")
}

func TestRenderNavigationHTML_HTMLEscaping(t *testing.T) {
	entries := []Entry{}

	// Use potentially dangerous characters in navigation
	nav := &NavigationContext{
		PrevURL:  "/runs/<script>/transcript/1",
		PrevText: "Phase <1>",
		NextURL:  "/runs/abc&xyz/transcript/2",
		NextText: "Phase \"2\"",
		BackURL:  "/runs/test",
		BackText: "Back & Return",
	}

	result := RenderHTMLFragment(entries, RenderOptions{Navigation: nav})

	// Should escape HTML entities
	assert.NotContains(t, result, "<script>", "URL should be escaped")
	assert.NotContains(t, result, "Phase <1>", "text should be escaped")
	assert.Contains(t, result, "&amp;", "expected escaped HTML entities")
	assert.Contains(t, result, "&lt;", "expected escaped HTML entities")
}

// --- HTML Metadata Rendering Tests (Task 16) ---

func TestRenderHTML_UserMessageWithTimestamp(t *testing.T) {
	entries := []Entry{
		{
			Type:      "user",
			Timestamp: "2026-03-12T03:32:05Z",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{Type: "text", Text: "Hello"},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	assert.Contains(t, result, `class="message-meta"`, "expected metadata span in user header")
	assert.Contains(t, result, `<time datetime="2026-03-12T03:32:05Z"`, "expected time element with datetime attr")
	assert.Contains(t, result, `>2026-03-12T03:32:05Z</time>`, "expected RFC3339 fallback text in time element")
	// User messages should NOT show model
	assert.NotContains(t, result, `<span>claude-opus</span>`, "user messages should not show model")
}

func TestRenderHTML_AssistantMessageWithTimestampAndModel(t *testing.T) {
	entries := []Entry{
		{
			Type:      "assistant",
			Timestamp: "2026-03-12T03:32:05Z",
			Model:     "claude-opus",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{Type: "text", Text: "Hello!"},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	assert.Contains(t, result, `class="message-meta"`, "expected metadata span in assistant header")
	assert.Contains(t, result, `<time datetime="2026-03-12T03:32:05Z"`, "expected time element")
	assert.Contains(t, result, `<span>claude-opus</span>`, "expected model span")
	assert.Contains(t, result, `class="meta-separator"`, "expected separator between timestamp and model")
}

func TestRenderHTML_AssistantMessageTimestampOnly(t *testing.T) {
	entries := []Entry{
		{
			Type:      "assistant",
			Timestamp: "2026-03-12T03:32:05Z",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{Type: "text", Text: "Hello!"},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	assert.Contains(t, result, `class="message-meta"`, "expected metadata span")
	assert.Contains(t, result, `<time datetime=`, "expected time element")
	assert.NotContains(t, result, `class="meta-separator"`, "no separator when only timestamp present")
}

func TestRenderHTML_NoMetadataRendersCleanHeader(t *testing.T) {
	entries := []Entry{
		{
			Type: "user",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{Type: "text", Text: "Hello"},
				},
			},
		},
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{Type: "text", Text: "Hi!"},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	// Headers should render without metadata spans when no timestamp/model
	assert.NotContains(t, result, `class="message-meta"`, "no metadata span when no metadata available")
	assert.NotContains(t, result, `<time `, "no time element when no timestamp")
	// But headers themselves should still be present
	assert.Contains(t, result, "👤", "expected user icon")
	assert.Contains(t, result, "🤖", "expected assistant icon")
}

func TestRenderHTML_SlashCommandWithTimestamp(t *testing.T) {
	entries := []Entry{
		{
			Type:      "user",
			Timestamp: "2026-03-12T03:32:05Z",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{Type: "text", Text: "/help"},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	assert.Contains(t, result, `class="message-meta"`, "expected metadata span in slash command header")
	assert.Contains(t, result, `<time datetime=`, "expected time element in slash command")
}

func TestRenderHTML_ReadGroupWithTimestamp(t *testing.T) {
	// Create entries that will be grouped as a read group by preprocessEntries.
	// A read group is: assistant entry with tool_use Read, followed by user entry with tool_result.
	entries := []Entry{
		{
			Type:      "assistant",
			Timestamp: "2026-03-12T03:32:05Z",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type: "tool_use",
						ID:   "read-1",
						Name: "Read",
						Input: map[string]any{
							"file_path": "/tmp/test.go",
						},
					},
				},
			},
		},
		{
			Type:      "user",
			Timestamp: "2026-03-12T03:32:06Z",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{
						Type:      "tool_result",
						ToolUseID: "read-1",
						Content:   "file contents here",
					},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	// The read group header should contain metadata from the first entry's timestamp
	assert.Contains(t, result, `class="message-meta"`, "expected metadata span in read group header")
	assert.Contains(t, result, `<time datetime=`, "expected time element in read group")
}

func TestRenderHTML_EditGroupWithTimestamp(t *testing.T) {
	entries := []Entry{
		{
			Type:      "assistant",
			Timestamp: "2026-03-12T03:32:05Z",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type: "tool_use",
						ID:   "edit-1",
						Name: "Edit",
						Input: map[string]any{
							"file_path": "/tmp/test.go",
							"new_string": "new content",
							"old_string": "old content",
						},
					},
				},
			},
		},
		{
			Type:      "user",
			Timestamp: "2026-03-12T03:32:06Z",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{
						Type:      "tool_result",
						ToolUseID: "edit-1",
						Content:   "ok",
					},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	assert.Contains(t, result, `class="message-meta"`, "expected metadata span in edit group header")
	assert.Contains(t, result, `<time datetime=`, "expected time element in edit group")
}

func TestRenderHTML_TimeElementHasValidDatetimeAttr(t *testing.T) {
	entries := []Entry{
		{
			Type:      "assistant",
			Timestamp: "2026-03-12T14:32:05+11:00",
			Model:     "gpt-4",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{Type: "text", Text: "Response"},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	// datetime attribute should be UTC
	assert.Contains(t, result, `datetime="2026-03-12T03:32:05Z"`, "datetime attr should be UTC ISO 8601")
	// Fallback text should be local timezone RFC3339
	assert.Contains(t, result, `>2026-03-12T03:32:05Z</time>`, "fallback text should be RFC3339")
}

func TestRenderHTML_StandaloneContainsFormatLocalDatesScript(t *testing.T) {
	entries := []Entry{
		{
			Type:      "user",
			Timestamp: "2026-03-12T03:32:05Z",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{Type: "text", Text: "Hello"},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	assert.Contains(t, result, "<script>", "standalone HTML should contain inline script")
	assert.Contains(t, result, "Intl.DateTimeFormat", "script should use Intl.DateTimeFormat for locale formatting")
	assert.Contains(t, result, `querySelectorAll`, "script should find time elements")
	assert.Contains(t, result, `datetime`, "script should read datetime attribute")
}

func TestRenderHTMLFragment_NoStandaloneScript(t *testing.T) {
	entries := []Entry{
		{
			Type:      "user",
			Timestamp: "2026-03-12T03:32:05Z",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{Type: "text", Text: "Hello"},
				},
			},
		},
	}

	result := RenderHTMLFragment(entries, RenderOptions{})

	// Fragment should NOT include the standalone script — the web layout provides its own
	assert.NotContains(t, result, "<script>", "fragment should not contain inline script")
}

func TestRenderHTML_MetadataConsistentAcrossMessageTypes(t *testing.T) {
	// Verify that metadata styling is consistent: all message types use the same
	// message-meta class and time element structure (requirement 3.5).
	ts := "2026-03-12T03:32:05Z"
	entries := []Entry{
		{
			Type:      "user",
			Timestamp: ts,
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{Type: "text", Text: "Question"},
				},
			},
		},
		{
			Type:      "assistant",
			Timestamp: ts,
			Model:     "claude-opus",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{Type: "text", Text: "Answer"},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	// Both user and assistant should use the same message-meta class
	metaCount := strings.Count(result, `class="message-meta"`)
	assert.Equal(t, 2, metaCount, "expected message-meta span in both user and assistant headers")

	// Both should use <time> elements
	timeCount := strings.Count(result, `<time datetime=`)
	assert.Equal(t, 2, timeCount, "expected time element in both headers")
}

func TestRenderHTML_ModelHTMLEscaping(t *testing.T) {
	entries := []Entry{
		{
			Type:      "assistant",
			Timestamp: "2026-03-12T03:32:05Z",
			Model:     `model<script>alert("xss")</script>`,
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{Type: "text", Text: "Response"},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	assert.NotContains(t, result, `<script>alert`, "model name should be HTML-escaped")
	assert.Contains(t, result, `&lt;script&gt;`, "expected escaped model name")
}

func TestRenderHTML_CSSContainsMetadataStyles(t *testing.T) {
	entries := []Entry{}
	result := RenderHTML(entries, RenderOptions{})

	assert.Contains(t, result, ".message-meta", "CSS should contain message-meta class")
	assert.Contains(t, result, ".meta-separator", "CSS should contain meta-separator class")
}
