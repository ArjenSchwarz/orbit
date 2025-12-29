package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderHTML_BasicStructure(t *testing.T) {
	entries := []Entry{}
	result := RenderHTML(entries, RenderOptions{})

	// Check for valid HTML structure
	if !strings.Contains(result, "<!DOCTYPE html>") {
		t.Error("expected DOCTYPE declaration")
	}
	if !strings.Contains(result, "<html lang=\"en\">") {
		t.Error("expected html tag with lang attribute")
	}
	if !strings.Contains(result, "<head>") {
		t.Error("expected head tag")
	}
	if !strings.Contains(result, "<body>") {
		t.Error("expected body tag")
	}
	if !strings.Contains(result, "</html>") {
		t.Error("expected closing html tag")
	}
}

func TestRenderHTML_EmbeddedCSS(t *testing.T) {
	entries := []Entry{}
	result := RenderHTML(entries, RenderOptions{})

	if !strings.Contains(result, "<style>") {
		t.Error("expected embedded style tag")
	}
	if !strings.Contains(result, ".message") {
		t.Error("expected message class in CSS")
	}
	if !strings.Contains(result, ".user") {
		t.Error("expected user class in CSS")
	}
	if !strings.Contains(result, ".assistant") {
		t.Error("expected assistant class in CSS")
	}
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

	if !strings.Contains(result, `class="message user"`) {
		t.Error("expected user message class")
	}
	if !strings.Contains(result, "👤") {
		t.Error("expected user icon")
	}
	if !strings.Contains(result, "Hello, Claude!") {
		t.Error("expected message content")
	}
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

	if !strings.Contains(result, `class="message assistant"`) {
		t.Error("expected assistant message class")
	}
	if !strings.Contains(result, "🤖") {
		t.Error("expected assistant icon")
	}
	if !strings.Contains(result, "Hello! How can I help?") {
		t.Error("expected message content")
	}
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

	if !strings.Contains(result, `<div class="thinking-block">`) {
		t.Error("expected thinking block div")
	}
	if !strings.Contains(result, `<div class="thinking-header">💭 Thinking</div>`) {
		t.Error("expected thinking header")
	}
	if !strings.Contains(result, "Let me think about this...") {
		t.Error("expected thinking content")
	}
	if !strings.Contains(result, `<div class="thinking-content markdown-content">`) {
		t.Error("expected thinking content div with markdown class")
	}
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
	if !strings.Contains(result, `<details class="tool-collapsible">`) {
		t.Error("expected combined tool block to use details.tool-collapsible")
	}
	if !strings.Contains(result, "🔧") {
		t.Error("expected tool icon")
	}
	if !strings.Contains(result, "Bash: Print hello") {
		t.Error("expected tool name and description in summary")
	}
	if !strings.Contains(result, "Command:") {
		t.Error("expected Command input section")
	}
	if !strings.Contains(result, "Result:") {
		t.Error("expected Result section")
	}
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
	if !strings.Contains(result, `<details class="tool-collapsible">`) {
		t.Error("expected unmatched tool_result to use details.tool-collapsible")
	}
	if !strings.Contains(result, "✅") {
		t.Error("expected success icon")
	}
	if !strings.Contains(result, "Tool Result") {
		t.Error("expected Tool Result summary")
	}
	if !strings.Contains(result, "File contents here") {
		t.Error("expected result content")
	}
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
	if !strings.Contains(result, `<details class="tool-collapsible error">`) {
		t.Error("expected error class on collapsible")
	}
	if !strings.Contains(result, "❌") {
		t.Error("expected error icon")
	}
	if !strings.Contains(result, "Tool Error") {
		t.Error("expected Tool Error summary")
	}
	if !strings.Contains(result, "Error: file not found") {
		t.Error("expected error content")
	}
}

func TestRenderHTML_TitleCustomization(t *testing.T) {
	entries := []Entry{}

	opts := RenderOptions{
		Title:     "Phase 1 Session Transcript",
		SessionID: "test-session-123",
	}

	result := RenderHTML(entries, opts)

	if !strings.Contains(result, "<title>Phase 1 Session Transcript</title>") {
		t.Error("expected custom title in head")
	}
	if !strings.Contains(result, "<h1>Phase 1 Session Transcript</h1>") {
		t.Error("expected custom title in header")
	}
	if !strings.Contains(result, `<code>test-session-123</code>`) {
		t.Error("expected session ID in code tag")
	}
}

func TestRenderHTML_DefaultTitle(t *testing.T) {
	entries := []Entry{}
	result := RenderHTML(entries, RenderOptions{})

	if !strings.Contains(result, "<title>Session Transcript</title>") {
		t.Error("expected default title")
	}
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

	// Should escape HTML entities
	if strings.Contains(result, "<script>") {
		t.Error("script tag should be escaped")
	}
	if !strings.Contains(result, "&lt;script&gt;") {
		t.Error("expected escaped script tag")
	}
}

func TestRenderHTML_SpecialCharactersInTitle(t *testing.T) {
	entries := []Entry{}

	opts := RenderOptions{
		Title:     "Test <Title> & More",
		SessionID: "session&<>\"",
	}

	result := RenderHTML(entries, opts)

	if strings.Contains(result, "<Title>") {
		t.Error("title should be escaped")
	}
	if !strings.Contains(result, "&lt;Title&gt;") {
		t.Error("expected escaped title")
	}
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

	if strings.Contains(result, "Should be skipped") {
		t.Error("unknown content type should be skipped")
	}
	if !strings.Contains(result, "Should be visible") {
		t.Error("known content type should be visible")
	}
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

	if strings.Contains(result, "queue-operation") {
		t.Error("unknown entry type should be skipped")
	}
	if !strings.Contains(result, "Should be visible") {
		t.Error("known entry type should be visible")
	}
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
	if strings.Contains(result, `class="message user"`) {
		t.Error("empty user message should not be rendered")
	}
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
	if strings.Contains(result, `class="message user"`) {
		t.Error("nil message should not be rendered")
	}
}

func TestRenderHTML_DarkModeSupport(t *testing.T) {
	entries := []Entry{}
	result := RenderHTML(entries, RenderOptions{})

	if !strings.Contains(result, "@media (prefers-color-scheme: dark)") {
		t.Error("expected dark mode media query")
	}
}

func TestRenderHTML_ResponsiveViewport(t *testing.T) {
	entries := []Entry{}
	result := RenderHTML(entries, RenderOptions{})

	if !strings.Contains(result, `<meta name="viewport"`) {
		t.Error("expected viewport meta tag")
	}
}

func TestRenderHTML_UTF8Charset(t *testing.T) {
	entries := []Entry{}
	result := RenderHTML(entries, RenderOptions{})

	if !strings.Contains(result, `<meta charset="UTF-8">`) {
		t.Error("expected UTF-8 charset")
	}
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
	if !strings.Contains(htmlResult, "🤖") && !strings.Contains(htmlResult, "✅") {
		t.Error("expected valid emoji in output")
	}
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

	if !strings.Contains(result, `<details class="tool-collapsible">`) {
		t.Error("expected subagent Task tool to use details.tool-collapsible")
	}
	if !strings.Contains(result, "Explore: Search for config files") {
		t.Error("expected summary to contain subagent_type and description")
	}
	if !strings.Contains(result, "🤖🔧") {
		t.Error("expected robot and tool icons in summary for subagent")
	}
	if !strings.Contains(result, "<strong>Prompt:</strong>") {
		t.Error("expected Prompt section in subagent block")
	}
	if !strings.Contains(result, "<strong>Result:</strong>") {
		t.Error("expected Result section in subagent block")
	}
}

func TestRenderHTML_SkillToolCollapses(t *testing.T) {
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

	if !strings.Contains(result, `<details class="tool-collapsible">`) {
		t.Error("expected Skill tool to use details.tool-collapsible")
	}
	if !strings.Contains(result, "Skill: next-task") {
		t.Error("expected summary to contain skill name")
	}
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
	if !strings.Contains(result, `<details class="tool-collapsible">`) {
		t.Error("expected combined tool block to use details.tool-collapsible")
	}
	if !strings.Contains(result, "Bash") {
		t.Error("expected tool name in summary")
	}
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

	if !strings.Contains(result, `<details class="tool-collapsible">`) {
		t.Error("combined tool block should use details.tool-collapsible")
	}
	if !strings.Contains(result, "Write") {
		t.Error("expected summary to contain tool name")
	}
}

func TestRenderHTML_CSSIncluded(t *testing.T) {
	entries := []Entry{}
	result := RenderHTML(entries, RenderOptions{})

	if !strings.Contains(result, "details.tool-collapsible") {
		t.Error("expected CSS for details.tool-collapsible")
	}
	if !strings.Contains(result, ".tool-content") {
		t.Error("expected CSS for .tool-content")
	}
	if !strings.Contains(result, "details.tool-collapsible.error") {
		t.Error("expected CSS for .error variant")
	}
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
	if !strings.Contains(result, "✅") {
		t.Error("expected success icon in result")
	}
	if !strings.Contains(result, "🤖🔧") {
		t.Error("expected robot and tool icons for subagent")
	}
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
	if !strings.Contains(result, `<details class="tool-collapsible">`) {
		t.Error("expected combined tool block to use details.tool-collapsible")
	}
	// Should have exactly 1 collapsible block (the combined tool call + result)
	detailsCount := strings.Count(result, `<details class="tool-collapsible">`)
	if detailsCount != 1 {
		t.Errorf("expected 1 collapsible block for combined tool call + result, got %d", detailsCount)
	}
}

func TestRenderHTML_CrossEntryToolMatching(t *testing.T) {
	// Verify that tool_result in user entry matches tool_use in assistant entry
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
						Content:   "Commit successful",
						IsError:   false,
					},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	// Both should be collapsed and show proper summaries
	if !strings.Contains(result, "Skill: commit") {
		t.Error("expected Skill summary in tool_use")
	}
	// Result should inherit the summary from tool_use
	if !strings.Contains(result, "✅") {
		t.Error("expected success icon in matched result")
	}
	// Both should be in details blocks
	detailsCount := strings.Count(result, `<details class="tool-collapsible">`)
	if detailsCount != 2 {
		t.Errorf("expected 2 collapsible blocks for Skill tool and result, got %d", detailsCount)
	}
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
	if !strings.Contains(result, `<details class="tool-collapsible error">`) {
		t.Error("expected error class on collapsible result")
	}
	if !strings.Contains(result, "❌") {
		t.Error("expected error icon in result")
	}
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
	testGoldenCollapsibleHTML(t, "skill_tool", []string{
		`<details class="tool-collapsible">`,
		`<summary><span class="icon">🔧</span> Skill: next-task</summary>`,
		`<summary><span class="icon">✅</span> Skill: next-task</summary>`,
		"Task completed: Updated to next phase",
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
		if !strings.Contains(result, pattern) {
			t.Errorf("expected pattern not found: %q\nIn output:\n%s", pattern, result)
		}
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
	if !strings.Contains(result, `<details class="tool-collapsible">`) {
		t.Error("expected unmatched tool_result to use details.tool-collapsible")
	}
	if !strings.Contains(result, "Tool Result") {
		t.Error("expected unmatched tool_result to have 'Tool Result' summary")
	}
	if !strings.Contains(result, "hello") {
		t.Error("expected tool result content to be rendered")
	}
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
	if !strings.Contains(result, `<details class="tool-collapsible">`) {
		t.Error("long tool should be collapsed")
	}

	// Should NOT be truncated (no truncation marker)
	if strings.Contains(result, "... (truncated)") {
		t.Error("content should not be truncated with <details> blocks")
	}

	// Verify the FULL result content IS present (no truncation)
	if !strings.Contains(result, longResult) {
		t.Error("full result should be present, not truncated")
	}
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
	if !strings.Contains(result, `<details class="tool-collapsible">`) {
		t.Error("result exceeding threshold should be collapsed")
	}

	// Content should NOT be truncated (it's under MaxToolResultRunes)
	if strings.Contains(result, "... (truncated)") {
		t.Error("content under MaxToolResultRunes should not be truncated")
	}
}
