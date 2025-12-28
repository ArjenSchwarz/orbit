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

	if !strings.Contains(result, `<details class="thinking">`) {
		t.Error("expected thinking details tag")
	}
	if !strings.Contains(result, "<summary>💭 Thinking</summary>") {
		t.Error("expected thinking summary")
	}
	if !strings.Contains(result, "Let me think about this...") {
		t.Error("expected thinking content")
	}
	if !strings.Contains(result, "</details>") {
		t.Error("expected closing details tag")
	}
}

func TestRenderHTML_ToolUse(t *testing.T) {
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type:  "tool_use",
						Name:  "Read",
						Input: map[string]any{"file_path": "/tmp/test.txt"},
					},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	if !strings.Contains(result, `class="tool-use"`) {
		t.Error("expected tool-use class")
	}
	if !strings.Contains(result, "🔧") {
		t.Error("expected tool icon")
	}
	if !strings.Contains(result, "<code>Read</code>") {
		t.Error("expected tool name in code tag")
	}
	if !strings.Contains(result, "file_path") {
		t.Error("expected input content")
	}
	if !strings.Contains(result, "<pre><code>") {
		t.Error("expected code block")
	}
}

func TestRenderHTML_ToolResultSuccess(t *testing.T) {
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

	if !strings.Contains(result, `class="tool-result"`) {
		t.Error("expected tool-result class")
	}
	if !strings.Contains(result, `class="tool-result-header success"`) {
		t.Error("expected success header class")
	}
	if !strings.Contains(result, "✅") {
		t.Error("expected success icon")
	}
	if !strings.Contains(result, "File contents here") {
		t.Error("expected result content")
	}
}

func TestRenderHTML_ToolResultError(t *testing.T) {
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

	if !strings.Contains(result, `class="tool-result-header error"`) {
		t.Error("expected error header class")
	}
	if !strings.Contains(result, "❌") {
		t.Error("expected error icon")
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

	html := RenderHTML(parseResult.Entries, RenderOptions{})

	// The output should be valid UTF-8 (no broken characters)
	if !strings.Contains(html, "🔧") {
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
						},
					},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	if !strings.Contains(result, `<details class="tool-collapsible">`) {
		t.Error("expected Task tool to use details.tool-collapsible")
	}
	if !strings.Contains(result, "Explore: Search for config files") {
		t.Error("expected summary to contain subagent_type and description")
	}
	if !strings.Contains(result, "🔧") {
		t.Error("expected tool icon in summary")
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
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type:  "tool_use",
						Name:  "Read",
						ID:    "read-123",
						Input: map[string]any{"file_path": "/tmp/test.txt"},
					},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	// Short tool input should use div.tool-use, not details
	if strings.Contains(result, `<details class="tool-collapsible">`) {
		t.Error("short tool input should not use details.tool-collapsible")
	}
	if !strings.Contains(result, `class="tool-use"`) {
		t.Error("expected div.tool-use for short tool input")
	}
}

func TestRenderHTML_LongToolCollapses(t *testing.T) {
	// Create input exceeding 500 runes threshold
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
						Input: map[string]any{"content": longContent},
					},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	if !strings.Contains(result, `<details class="tool-collapsible">`) {
		t.Error("long tool input should use details.tool-collapsible")
	}
	if !strings.Contains(result, "Tool: Write") {
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
	// Tool result for a Task tool should collapse
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

	// Count details.tool-collapsible occurrences (should be 2: one for tool_use, one for tool_result)
	detailsCount := strings.Count(result, `<details class="tool-collapsible">`)
	if detailsCount != 2 {
		t.Errorf("expected 2 collapsible blocks, got %d", detailsCount)
	}
	if !strings.Contains(result, "✅") {
		t.Error("expected success icon in result")
	}
}

func TestRenderHTML_ShortResultNoCollapse(t *testing.T) {
	// Short tool result for non-Task/Skill tool should not collapse
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type:  "tool_use",
						Name:  "Read",
						ID:    "read-456",
						Input: map[string]any{"file_path": "/tmp/test.txt"},
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
						ToolUseID: "read-456",
						Content:   "Short content",
						IsError:   false,
					},
				},
			},
		},
	}

	result := RenderHTML(entries, RenderOptions{})

	// The tool_result should use div.tool-result, not details
	if !strings.Contains(result, `class="tool-result"`) {
		t.Error("expected div.tool-result for short result")
	}
	// Should have exactly 0 collapsible blocks (Read tool is short, result is short)
	detailsCount := strings.Count(result, `<details class="tool-collapsible">`)
	if detailsCount != 0 {
		t.Errorf("expected 0 collapsible blocks for short content, got %d", detailsCount)
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
	testGoldenCollapsibleHTML(t, "task_tool", []string{
		`<details class="tool-collapsible">`,
		`<summary><span class="icon">🔧</span> Explore: Search for config files</summary>`,
		`<summary><span class="icon">✅</span> Explore: Search for config files</summary>`,
		"subagent_type",
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
	testGoldenCollapsibleHTML(t, "long_output", []string{
		`<details class="tool-collapsible">`,
		`<summary><span class="icon">🔧</span> Tool: Read</summary>`,
		`<summary><span class="icon">✅</span> Tool: Read</summary>`,
		"Line 1: This is a very long file content",
	})
}

func TestRenderHTML_GoldenCollapsible_ShortOutput(t *testing.T) {
	testGoldenCollapsibleHTML(t, "short_output", []string{
		`class="tool-use"`,
		`<code>Read</code>`,
		`class="tool-result"`,
		`class="tool-result-header success"`,
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
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type:  "tool_use",
						Name:  "Read",
						ID:    "", // Empty ID (old format)
						Input: map[string]any{"file_path": "/tmp/test.txt"},
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
						Content:   "File contents here",
						IsError:   false,
					},
				},
			},
		},
	}

	// Should render without panicking
	result := RenderHTML(entries, RenderOptions{})

	// Tool use should render with div.tool-use (short input)
	if !strings.Contains(result, `class="tool-use"`) {
		t.Error("expected tool_use with div.tool-use")
	}
	if !strings.Contains(result, `<code>Read</code>`) {
		t.Error("expected tool name in code tag")
	}

	// Tool result should render with div.tool-result (short unmatched)
	if !strings.Contains(result, `class="tool-result"`) {
		t.Error("expected tool_result with div.tool-result")
	}
	if !strings.Contains(result, "File contents here") {
		t.Error("expected tool result content to be rendered")
	}
}

func TestBackwardCompat_TruncationPreserved_HTML(t *testing.T) {
	// Test that truncation still works within collapsed blocks
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

	// Should be truncated
	if !strings.Contains(result, "... (truncated)") {
		t.Error("long content should be truncated")
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
