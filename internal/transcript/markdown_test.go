package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderMarkdown_UserMessage(t *testing.T) {
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

	result := RenderMarkdown(entries, RenderOptions{})

	if !strings.Contains(result, "## 👤 User") {
		t.Error("expected User heading")
	}
	if !strings.Contains(result, "Hello, Claude!") {
		t.Error("expected message content")
	}
	if !strings.Contains(result, "---") {
		t.Error("expected horizontal rule")
	}
}

func TestRenderMarkdown_AssistantMessage(t *testing.T) {
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

	result := RenderMarkdown(entries, RenderOptions{})

	if !strings.Contains(result, "## 🤖 Assistant") {
		t.Error("expected Assistant heading")
	}
	if !strings.Contains(result, "Hello! How can I help?") {
		t.Error("expected message content")
	}
}

func TestRenderMarkdown_ThinkingBlock(t *testing.T) {
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

	result := RenderMarkdown(entries, RenderOptions{})

	if !strings.Contains(result, "<details>") {
		t.Error("expected details tag")
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

func TestRenderMarkdown_ToolUse(t *testing.T) {
	// Non-Task/Skill tools now render as combined collapsible blocks with their results
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
						Input: map[string]any{"command": "ls -la", "description": "List files"},
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
						Content:   "file1.txt\nfile2.txt",
						IsError:   false,
					},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	// Combined tool call + result in collapsible block
	if !strings.Contains(result, "<details>") {
		t.Error("expected details block for combined tool")
	}
	if !strings.Contains(result, "Bash: List files") {
		t.Error("expected tool name and description in summary")
	}
	if !strings.Contains(result, "**Command:**") {
		t.Error("expected Command section")
	}
	if !strings.Contains(result, "**Result:**") {
		t.Error("expected Result section")
	}
}

func TestRenderMarkdown_ToolResultSuccess(t *testing.T) {
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

	result := RenderMarkdown(entries, RenderOptions{})

	// Unmatched results render in collapsible blocks
	if !strings.Contains(result, "<details>") {
		t.Error("expected details block for unmatched tool_result")
	}
	if !strings.Contains(result, "✅ Tool Result") {
		t.Error("expected success summary")
	}
	if !strings.Contains(result, "File contents here") {
		t.Error("expected result content")
	}
}

func TestRenderMarkdown_ToolResultError(t *testing.T) {
	// Unmatched error tool_result renders in collapsible block
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

	result := RenderMarkdown(entries, RenderOptions{})

	// Unmatched error results render in collapsible blocks
	if !strings.Contains(result, "<details>") {
		t.Error("expected details block for unmatched tool_result")
	}
	if !strings.Contains(result, "❌ Tool Error") {
		t.Error("expected error summary")
	}
	if !strings.Contains(result, "Error: file not found") {
		t.Error("expected error content")
	}
}

func TestRenderMarkdown_TruncationRuneBoundary(t *testing.T) {
	// Create content with multi-byte UTF-8 characters
	// Each emoji is 4 bytes but 1 rune
	f, err := os.Open(filepath.Join("testdata", "unicode.jsonl"))
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer func() { _ = f.Close() }()

	parseResult, err := ParseJSONL(f)
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	markdown := RenderMarkdown(parseResult.Entries, RenderOptions{})

	// The output should be valid UTF-8 (no broken characters)
	// Check for common emoji that should be preserved
	if !strings.Contains(markdown, "🤖") && !strings.Contains(markdown, "✅") {
		t.Error("expected valid emoji in output")
	}
}

func TestRenderMarkdown_TitleCustomization(t *testing.T) {
	entries := []Entry{}

	opts := RenderOptions{
		Title:     "Phase 1 Session Transcript",
		SessionID: "test-session-123",
	}

	result := RenderMarkdown(entries, opts)

	if !strings.Contains(result, "# Phase 1 Session Transcript") {
		t.Error("expected custom title")
	}
	if !strings.Contains(result, "**Session ID:** `test-session-123`") {
		t.Error("expected session ID")
	}
}

func TestRenderMarkdown_DefaultTitle(t *testing.T) {
	entries := []Entry{}
	result := RenderMarkdown(entries, RenderOptions{})

	if !strings.Contains(result, "# Session Transcript") {
		t.Error("expected default title")
	}
}

func TestRenderMarkdown_HorizontalRulesBetweenMessages(t *testing.T) {
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

	result := RenderMarkdown(entries, RenderOptions{})

	// Count horizontal rules (should have one after header and one after each message)
	count := strings.Count(result, "---")
	// Header separator + 3 messages = 4 separators
	if count != 4 {
		t.Errorf("expected 4 horizontal rules, got %d", count)
	}
}

func TestRenderMarkdown_UnknownContentTypes(t *testing.T) {
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

	result := RenderMarkdown(entries, RenderOptions{})

	if strings.Contains(result, "Should be skipped") {
		t.Error("unknown content type should be skipped")
	}
	if !strings.Contains(result, "Should be visible") {
		t.Error("known content type should be visible")
	}
}

func TestTruncateString_ShortString(t *testing.T) {
	input := "Hello, World!"
	result := truncateString(input, 100)
	if result != input {
		t.Errorf("short string should not be truncated: got %q", result)
	}
}

func TestTruncateString_ExactLength(t *testing.T) {
	input := "Hello"
	result := truncateString(input, 5)
	if result != input {
		t.Errorf("string at exact length should not be truncated: got %q", result)
	}
}

func TestTruncateString_Truncated(t *testing.T) {
	input := "Hello, World!"
	result := truncateString(input, 5)
	if !strings.HasPrefix(result, "Hello") {
		t.Errorf("expected prefix 'Hello', got %q", result)
	}
	if !strings.Contains(result, "... (truncated)") {
		t.Error("expected truncation marker")
	}
}

func TestTruncateString_UTF8Safe(t *testing.T) {
	// String with multi-byte characters
	// Each emoji is 1 rune but 4 bytes
	input := "🎉🎊🎁🎄🎅" // 5 emojis = 5 runes
	result := truncateString(input, 3)

	// Should truncate to 3 runes (3 emojis)
	if !strings.HasPrefix(result, "🎉🎊🎁") {
		t.Errorf("expected '🎉🎊🎁' prefix, got %q", result)
	}
	if !strings.Contains(result, "... (truncated)") {
		t.Error("expected truncation marker")
	}
	// Should NOT contain broken UTF-8
	if strings.Contains(result, "\uFFFD") {
		t.Error("result contains replacement character (broken UTF-8)")
	}
}

func TestTruncateString_MixedUTF8(t *testing.T) {
	// Mix of single-byte ASCII and multi-byte characters
	input := "a🎉b🎊c" // 5 runes total
	result := truncateString(input, 3)

	// Should truncate to 3 runes: "a🎉b"
	if !strings.HasPrefix(result, "a🎉b") {
		t.Errorf("expected 'a🎉b' prefix, got %q", result)
	}
}

func TestGetToolSummary_Task(t *testing.T) {
	input := map[string]any{
		"subagent_type": "Explore",
		"description":   "Search for config files",
	}
	result := getToolSummary("Task", input)
	expected := "Explore: Search for config files"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestGetToolSummary_TaskPartialFields(t *testing.T) {
	// Only subagent_type present, no description
	input := map[string]any{
		"subagent_type": "Explore",
	}
	result := getToolSummary("Task", input)
	// Should return "Explore" without trailing colon
	expected := "Explore"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestGetToolSummary_TaskFallback(t *testing.T) {
	tests := map[string]struct {
		input any
	}{
		"nil input":          {nil},
		"not a map":          {"invalid"},
		"empty map":          {map[string]any{}},
		"empty subagent":     {map[string]any{"subagent_type": ""}},
		"wrong type for key": {map[string]any{"subagent_type": 123}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := getToolSummary("Task", tc.input)
			if result != "" {
				t.Errorf("expected empty string, got %q", result)
			}
		})
	}
}

func TestGetToolSummary_Skill(t *testing.T) {
	input := map[string]any{
		"skill": "next-task",
	}
	result := getToolSummary("Skill", input)
	expected := "Skill: next-task"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestGetToolSummary_OtherTool(t *testing.T) {
	tests := map[string]struct {
		toolName string
		input    any
	}{
		"Read tool":      {"Read", map[string]any{"file_path": "/tmp/test"}},
		"Bash tool":      {"Bash", map[string]any{"command": "ls"}},
		"lowercase task": {"task", map[string]any{"subagent_type": "Explore"}},
		"uppercase TASK": {"TASK", map[string]any{"subagent_type": "Explore"}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := getToolSummary(tc.toolName, tc.input)
			if result != "" {
				t.Errorf("expected empty string for non-Task/Skill tool, got %q", result)
			}
		})
	}
}

func TestShouldCollapse_AlwaysTools(t *testing.T) {
	tests := map[string]struct {
		toolName  string
		runeCount int
	}{
		"Task zero":     {"Task", 0},
		"Task small":    {"Task", 100},
		"Task at limit": {"Task", 500},
		"Task large":    {"Task", 1000},
		"Skill zero":    {"Skill", 0},
		"Skill small":   {"Skill", 100},
		"Skill large":   {"Skill", 5000},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if !shouldCollapse(tc.toolName, tc.runeCount) {
				t.Errorf("%s with %d runes should always collapse", tc.toolName, tc.runeCount)
			}
		})
	}
}

func TestShouldCollapse_Threshold(t *testing.T) {
	tests := map[string]struct {
		runeCount int
		expected  bool
	}{
		"499 runes - should not collapse": {499, false},
		"500 runes - should not collapse": {500, false},
		"501 runes - should collapse":     {501, true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := shouldCollapse("Read", tc.runeCount)
			if result != tc.expected {
				t.Errorf("shouldCollapse(Read, %d) = %v, want %v", tc.runeCount, result, tc.expected)
			}
		})
	}
}

func TestEscapeSummary_XSS(t *testing.T) {
	tests := map[string]struct {
		input    string
		expected string
	}{
		"script tag":      {"<script>alert(1)</script>", "&lt;script&gt;alert(1)&lt;/script&gt;"},
		"closing summary": {"</summary>", "&lt;/summary&gt;"},
		"ampersand":       {"foo & bar", "foo &amp; bar"},
		"html entities":   {"<div class=\"x\">", "&lt;div class=&#34;x&#34;&gt;"},
		"normal text":     {"Explore: Search files", "Explore: Search files"},
		"unicode safe":    {"🔧 Tool", "🔧 Tool"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := escapeSummary(tc.input)
			if result != tc.expected {
				t.Errorf("escapeSummary(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

// Tests for collapsible tool_use blocks (Task 9)

func TestRenderMarkdown_TaskToolAlwaysCollapses(t *testing.T) {
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
						ID:   "tool_123",
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
						ToolUseID: "tool_123",
						Content:   "Found config files",
						IsError:   false,
					},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	if !strings.Contains(result, "<details>") {
		t.Error("subagent Task tool should always be wrapped in details")
	}
	if !strings.Contains(result, "<summary>✅ 🤖🔧 Explore: Search for config files</summary>") {
		t.Error("expected subagent Task summary with robot emoji, tool emoji, subagent_type and description")
	}
	if !strings.Contains(result, "</details>") {
		t.Error("expected closing details tag")
	}
	if !strings.Contains(result, "**Prompt:**") {
		t.Error("expected Prompt section in subagent block")
	}
	if !strings.Contains(result, "**Result:**") {
		t.Error("expected Result section in subagent block")
	}
}

func TestRenderMarkdown_TaskToolFallback(t *testing.T) {
	tests := map[string]struct {
		input any
	}{
		"nil input":        {nil},
		"not a map":        {"invalid"},
		"empty map":        {map[string]any{}},
		"missing subagent": {map[string]any{"description": "test"}},
		"empty subagent":   {map[string]any{"subagent_type": ""}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			entries := []Entry{
				{
					Type: "assistant",
					Message: &Message{
						Role: "assistant",
						Content: []ContentItem{
							{
								Type:  "tool_use",
								Name:  "Task",
								ID:    "tool_123",
								Input: tc.input,
							},
						},
					},
				},
			}

			result := RenderMarkdown(entries, RenderOptions{})

			if !strings.Contains(result, "<details>") {
				t.Error("Task tool should still be wrapped in details")
			}
			if !strings.Contains(result, "<summary>🔧 Task</summary>") {
				t.Errorf("expected fallback summary '🔧 Task' for case %s", name)
			}
		})
	}
}

func TestRenderMarkdown_SkillToolRendersSimple(t *testing.T) {
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type: "tool_use",
						Name: "Skill",
						ID:   "tool_456",
						Input: map[string]any{
							"skill": "next-task",
						},
					},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	// Skill should render as simple line, not collapsible
	if !strings.Contains(result, "🔧 Skill: next-task") {
		t.Error("expected Skill tool to show skill name")
	}
	// Should NOT be wrapped in details
	if strings.Contains(result, "<details>") {
		t.Error("Skill tool should not be wrapped in details")
	}
}

func TestRenderMarkdown_SkillToolFallback(t *testing.T) {
	tests := map[string]struct {
		input any
	}{
		"nil input":   {nil},
		"not a map":   {"invalid"},
		"empty map":   {map[string]any{}},
		"empty skill": {map[string]any{"skill": ""}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			entries := []Entry{
				{
					Type: "assistant",
					Message: &Message{
						Role: "assistant",
						Content: []ContentItem{
							{
								Type:  "tool_use",
								Name:  "Skill",
								ID:    "tool_456",
								Input: tc.input,
							},
						},
					},
				},
			}

			result := RenderMarkdown(entries, RenderOptions{})

			// Skill should render as simple line with fallback name
			if !strings.Contains(result, "🔧 Skill") {
				t.Errorf("expected fallback '🔧 Skill' for case %s", name)
			}
			// Should NOT be wrapped in details
			if strings.Contains(result, "<details>") {
				t.Error("Skill tool should not be wrapped in details")
			}
		})
	}
}

func TestRenderMarkdown_SkillToolWithDescription(t *testing.T) {
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

	result := RenderMarkdown(entries, RenderOptions{})

	// Skill with description should be collapsible
	if !strings.Contains(result, "<details>") {
		t.Error("expected Skill with description to be wrapped in details")
	}
	if !strings.Contains(result, "🔧 Skill: permission-analyzer") {
		t.Error("expected Skill tool to show skill name in summary")
	}
	if !strings.Contains(result, "This skill analyzes permissions") {
		t.Error("expected skill description to be rendered")
	}
}

func TestRenderMarkdown_ToolNameCaseSensitive(t *testing.T) {
	// In the new combined format, case-insensitive tool names (not exact "Task" or "Skill")
	// are treated as regular tools and use combined collapsible format when matched with result
	tests := map[string]string{
		"task":  "task",
		"TASK":  "TASK",
		"skill": "skill",
		"SKILL": "SKILL",
	}

	for name, toolName := range tests {
		t.Run(name, func(t *testing.T) {
			entries := []Entry{
				{
					Type: "assistant",
					Message: &Message{
						Role: "assistant",
						Content: []ContentItem{
							{
								Type:  "tool_use",
								Name:  toolName,
								ID:    "tool_789",
								Input: map[string]any{"small": "input"},
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
								ToolUseID: "tool_789",
								Content:   "result",
								IsError:   false,
							},
						},
					},
				},
			}

			result := RenderMarkdown(entries, RenderOptions{})

			// Case-insensitive variants use combined collapsible format
			if !strings.Contains(result, "<details>") {
				t.Errorf("tool %q should use combined collapsible format", toolName)
			}
			if !strings.Contains(result, toolName) {
				t.Errorf("expected tool name %q in output", toolName)
			}
		})
	}
}

func TestRenderMarkdown_ShortToolNoCollapse(t *testing.T) {
	// In the new combined format, all non-Task/Skill tools use collapsible blocks
	// when matched with their results
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type:  "tool_use",
						Name:  "Bash",
						ID:    "tool_short",
						Input: map[string]any{"command": "ls"},
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
						ToolUseID: "tool_short",
						Content:   "file.txt",
						IsError:   false,
					},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	// Combined tool call + result uses collapsible format
	if !strings.Contains(result, "<details>") {
		t.Error("combined tool block should use details")
	}
	if !strings.Contains(result, "Bash") {
		t.Error("expected tool name in output")
	}
}

func TestRenderMarkdown_LongToolCollapses(t *testing.T) {
	// All non-Task/Skill tools now use combined collapsible blocks
	longContent := strings.Repeat("x", 600)
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type:  "tool_use",
						Name:  "Bash",
						ID:    "tool_long",
						Input: map[string]any{"command": longContent},
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
						ToolUseID: "tool_long",
						Content:   "output",
						IsError:   false,
					},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	if !strings.Contains(result, "<details>") {
		t.Error("combined tool block should use details")
	}
	if !strings.Contains(result, "Bash") {
		t.Error("expected tool name in output")
	}
}

func TestRenderMarkdown_ExactThresholdNoCollapse(t *testing.T) {
	// Create input that is exactly 500 runes when JSON-serialized
	// JSON format: {"data":"xxx..."} = 11 chars overhead + content
	// We need total serialized length to be exactly 500 runes
	// {"data":"..."} has 11 chars of overhead, so we need 489 chars of content
	content := strings.Repeat("a", 489)
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type:  "tool_use",
						Name:  "Bash",
						ID:    "tool_exact",
						Input: map[string]any{"data": content},
					},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	// At exactly 500 runes, should NOT collapse (threshold is > 500)
	if strings.Contains(result, "<details>") && strings.Contains(result, "🔧 Tool: Bash") {
		t.Error("tool at exactly 500 runes should not collapse")
	}
}

func TestRenderMarkdown_ZeroLengthNoCollapse(t *testing.T) {
	tests := map[string]any{
		"nil input": nil,
		"empty map": map[string]any{},
	}

	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			entries := []Entry{
				{
					Type: "assistant",
					Message: &Message{
						Role: "assistant",
						Content: []ContentItem{
							{
								Type:  "tool_use",
								Name:  "Write",
								ID:    "tool_zero",
								Input: input,
							},
						},
					},
				},
			}

			result := RenderMarkdown(entries, RenderOptions{})

			// Zero-length input should NOT collapse
			if strings.Contains(result, "<details>") && strings.Contains(result, "🔧 Tool: Write") {
				t.Errorf("zero-length input (%s) should not collapse", name)
			}
		})
	}
}

// Tests for tool_result blocks (Task 10)

func TestRenderMarkdown_ResultMatchesToolUse(t *testing.T) {
	// Subagent Task tool use in assistant entry, result in user entry - combined
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type: "tool_use",
						Name: "Task",
						ID:   "task_001",
						Input: map[string]any{
							"subagent_type": "Explore",
							"description":   "Search for files",
							"prompt":        "Find all relevant files",
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
						ToolUseID: "task_001",
						Content:   "Found 5 files",
						IsError:   false,
					},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	// Subagent result should be combined with robot emoji
	if !strings.Contains(result, "<summary>✅ 🤖🔧 Explore: Search for files</summary>") {
		t.Error("expected subagent tool result with robot emoji in summary")
	}
}

func TestRenderMarkdown_UnmatchedResultThreshold(t *testing.T) {
	// Result without matching tool_use, long content should collapse
	longContent := strings.Repeat("x", 600)
	entries := []Entry{
		{
			Type: "user",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{
						Type:      "tool_result",
						ToolUseID: "unknown_id",
						Content:   longContent,
						IsError:   false,
					},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	if !strings.Contains(result, "<details>") {
		t.Error("long unmatched result should be collapsed")
	}
	if !strings.Contains(result, "<summary>✅ Tool Result</summary>") {
		t.Error("unmatched result should use generic 'Tool Result' summary")
	}
}

func TestRenderMarkdown_ResultErrorIcon(t *testing.T) {
	// Error result matching a subagent Task tool
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type: "tool_use",
						Name: "Task",
						ID:   "task_err",
						Input: map[string]any{
							"subagent_type": "Explore",
							"description":   "Search files",
							"prompt":        "Find all files",
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
						ToolUseID: "task_err",
						Content:   "Error: something went wrong",
						IsError:   true,
					},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	// Error result for subagent should use ❌ icon with robot emoji
	if !strings.Contains(result, "<summary>❌ 🤖🔧 Explore: Search files</summary>") {
		t.Error("error result should use ❌ icon with robot emoji in summary")
	}
}

func TestRenderMarkdown_ZeroLengthResultNoCollapse(t *testing.T) {
	// Unmatched tool_result with empty content still uses collapsible format
	// (consistent behavior for all unmatched results)
	entries := []Entry{
		{
			Type: "user",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{
						Type:      "tool_result",
						ToolUseID: "some_id",
						Content:   "",
						IsError:   false,
					},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	// Unmatched results use collapsible format even when empty
	if !strings.Contains(result, "<details>") {
		t.Error("unmatched tool_result should use collapsible format")
	}
	if !strings.Contains(result, "Tool Result") {
		t.Error("expected Tool Result summary")
	}
}

// Cross-entry tool matching tests (Task 11)

func TestRenderMarkdown_CrossEntryToolMatching(t *testing.T) {
	// This is the critical test: tool_use appears in "assistant" entry,
	// but tool_result appears in "user" entry. The map must persist across entries.
	// For subagent Tasks, they're combined into a single block.
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type: "tool_use",
						Name: "Task",
						ID:   "cross_entry_001",
						Input: map[string]any{
							"subagent_type": "Explore",
							"description":   "Find config",
							"prompt":        "Find config files",
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
						ToolUseID: "cross_entry_001",
						Content:   "Config found at /etc/app.conf",
						IsError:   false,
					},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	// Subagent combines tool_use and tool_result into one block with robot emoji
	if !strings.Contains(result, "<summary>✅ 🤖🔧 Explore: Find config</summary>") {
		t.Error("subagent should have combined block with robot emoji")
	}

	// Should have Prompt and Result sections
	if !strings.Contains(result, "**Prompt:**") {
		t.Error("expected Prompt section in combined block")
	}
	if !strings.Contains(result, "**Result:**") {
		t.Error("expected Result section in combined block")
	}
}

func TestRenderMarkdown_ToolResultInUserEntry(t *testing.T) {
	// Verify that tool_result is properly handled in user entries (not assistant)
	// For Skill, the tool_result is skipped (only tool_use renders)
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type: "tool_use",
						Name: "Skill",
						ID:   "skill_user_entry",
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
						ToolUseID: "skill_user_entry",
						Content:   "Commit created successfully",
						IsError:   false,
					},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	// Skill tool_use should render as simple line
	if !strings.Contains(result, "🔧 Skill: commit") {
		t.Error("Skill tool_use should render with skill name")
	}
	// Skill result should be skipped
	if strings.Contains(result, "Commit created successfully") {
		t.Error("Skill tool_result should not be rendered")
	}
}

func TestRenderMarkdown_MultipleToolsMatching(t *testing.T) {
	// Multiple tool_use/result pairs should match correctly
	// Subagent Task combines, Skill renders as simple line (result skipped)
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type: "tool_use",
						Name: "Task",
						ID:   "task_a",
						Input: map[string]any{
							"subagent_type": "Plan",
							"description":   "Create plan",
							"prompt":        "Create a plan",
						},
					},
					{
						Type: "tool_use",
						Name: "Skill",
						ID:   "skill_b",
						Input: map[string]any{
							"skill": "rune",
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
						ToolUseID: "task_a",
						Content:   "Plan created",
						IsError:   false,
					},
					{
						Type:      "tool_result",
						ToolUseID: "skill_b",
						Content:   "Rune executed",
						IsError:   false,
					},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	// Subagent Task result should be combined with robot emoji
	if !strings.Contains(result, "<summary>✅ 🤖🔧 Plan: Create plan</summary>") {
		t.Error("subagent Task result should have robot emoji")
	}
	// Skill should render as simple line (not collapsible, result skipped)
	if !strings.Contains(result, "🔧 Skill: rune") {
		t.Error("Skill should render with skill name")
	}
	// Skill result should be skipped
	if strings.Contains(result, "Rune executed") {
		t.Error("Skill result should not be rendered")
	}
}

// Tests for Markdown output format (Task 15)

func TestRenderMarkdown_SkillSimpleFormat(t *testing.T) {
	// Verify that Skill renders as a simple line, not collapsible
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type: "tool_use",
						Name: "Skill",
						ID:   "format_test",
						Input: map[string]any{
							"skill": "next-task",
						},
					},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	// Skill should render as simple line
	if !strings.Contains(result, "🔧 Skill: next-task") {
		t.Error("expected Skill to render with skill name")
	}

	// Should NOT use details/summary
	if strings.Contains(result, "<details>") {
		t.Error("Skill should not use details element")
	}

	// Should NOT show JSON
	if strings.Contains(result, "```json") {
		t.Error("Skill should not show JSON input")
	}
}

func TestRenderMarkdown_UncollapsedFormat(t *testing.T) {
	// In the new combined format, all non-Task/Skill tools use collapsible blocks
	// when matched with their results
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type:  "tool_use",
						Name:  "Bash",
						ID:    "uncollapsed_test",
						Input: map[string]any{"command": "ls"},
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
						ToolUseID: "uncollapsed_test",
						Content:   "Short result",
						IsError:   false,
					},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	// Combined tool call + result uses collapsible format
	if !strings.Contains(result, "<details>") {
		t.Error("combined tool block should use details")
	}
	if !strings.Contains(result, "Bash") {
		t.Error("expected tool name in output")
	}
	if !strings.Contains(result, "**Command:**") {
		t.Error("expected Command section")
	}
	if !strings.Contains(result, "**Result:**") {
		t.Error("expected Result section")
	}
}

// Golden file integration tests for collapsible blocks

func TestRenderMarkdown_GoldenCollapsible_TaskTool(t *testing.T) {
	testGoldenCollapsible(t, "task_tool")
}

func TestRenderMarkdown_GoldenCollapsible_SkillTool(t *testing.T) {
	testGoldenCollapsible(t, "skill_tool")
}

func TestRenderMarkdown_GoldenCollapsible_LongOutput(t *testing.T) {
	testGoldenCollapsible(t, "long_output")
}

func TestRenderMarkdown_GoldenCollapsible_ShortOutput(t *testing.T) {
	testGoldenCollapsible(t, "short_output")
}

// Golden file tests for Codex format (Phase 3)

func TestRenderMarkdown_GoldenCodex_Basic(t *testing.T) {
	testGoldenCodex(t, "basic")
}

func TestRenderMarkdown_GoldenCodex_Reasoning(t *testing.T) {
	testGoldenCodex(t, "reasoning")
}

func testGoldenCodex(t *testing.T, name string) {
	t.Helper()

	// Read JSONL input
	jsonlPath := filepath.Join("testdata", "codex", name+".jsonl")
	f, err := os.Open(jsonlPath)
	if err != nil {
		t.Fatalf("failed to open test file %s: %v", jsonlPath, err)
	}
	defer func() { _ = f.Close() }()

	parseResult, err := ParseJSONL(f)
	if err != nil {
		t.Fatalf("ParseJSONL returned error: %v", err)
	}

	// Render to Markdown
	actual := RenderMarkdown(parseResult.Entries, RenderOptions{})

	// Read golden file
	goldenPath := filepath.Join("testdata", "codex", name+".md.golden")
	goldenBytes, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("failed to read golden file %s: %v", goldenPath, err)
	}
	expected := string(goldenBytes)

	// Compare
	if actual != expected {
		t.Errorf("output does not match golden file\n--- expected ---\n%s\n--- actual ---\n%s", expected, actual)
	}
}

func testGoldenCollapsible(t *testing.T, name string) {
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

	// Render to Markdown
	actual := RenderMarkdown(parseResult.Entries, RenderOptions{})

	// Read golden file
	goldenPath := filepath.Join("testdata", "collapsible", name+".md.golden")
	goldenBytes, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("failed to read golden file %s: %v", goldenPath, err)
	}
	expected := string(goldenBytes)

	// Compare
	if actual != expected {
		t.Errorf("output does not match golden file\n--- expected ---\n%s\n--- actual ---\n%s", expected, actual)
	}
}

// Backward compatibility tests (Task 25)

func TestBackwardCompat_NoIDFields(t *testing.T) {
	// Test that old JSONL without id/tool_use_id fields renders correctly
	// This simulates transcripts from before the ID fields were added
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
						Input: map[string]any{"command": "ls"},
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
						Content:   "File list here",
						IsError:   false,
					},
				},
			},
		},
	}

	// Should render without panicking
	result := RenderMarkdown(entries, RenderOptions{})

	// With no matching IDs, tool_use stores metadata but doesn't render
	// tool_result renders as standalone collapsible with "Tool Result" summary
	if !strings.Contains(result, "<details>") {
		t.Error("expected unmatched tool_result to use collapsible format")
	}
	if !strings.Contains(result, "Tool Result") {
		t.Error("expected unmatched tool_result to have 'Tool Result' summary")
	}
	if !strings.Contains(result, "File list here") {
		t.Error("expected tool result content to be rendered")
	}
}

func TestBackwardCompat_TruncationPreserved(t *testing.T) {
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

	result := RenderMarkdown(entries, RenderOptions{})

	// Should be collapsed (combined tool block)
	if !strings.Contains(result, "<details>") {
		t.Error("combined tool block should be collapsed")
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

func TestBackwardCompat_PreTruncationDecision(t *testing.T) {
	// Test that collapse decision is made BEFORE truncation
	// A 600-rune result should collapse even though after truncation it might be shorter
	// (This matters for results that are > threshold but < MaxToolResultRunes)

	// Create content that exceeds collapse threshold but is under truncation limit
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

	result := RenderMarkdown(entries, RenderOptions{})

	// Should be collapsed because original content exceeds threshold
	if !strings.Contains(result, "<details>") {
		t.Error("result exceeding threshold should be collapsed")
	}
	if !strings.Contains(result, "<summary>✅ Tool Result</summary>") {
		t.Error("expected collapsed result summary")
	}

	// Content should NOT be truncated (it's under MaxToolResultRunes)
	if strings.Contains(result, "... (truncated)") {
		t.Error("content under MaxToolResultRunes should not be truncated")
	}
}

func TestRenderMarkdown_EditGroupErrorMessagePreserved(t *testing.T) {
	// Test that Edit tool error messages are preserved when there's no structuredPatch
	// This tests the fix for the regression where failed Edit operations showed empty blocks.
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type: "tool_use",
						Name: "Edit",
						ID:   "edit_fail",
						Input: map[string]any{
							"file_path":  "/path/to/file.go",
							"old_string": "not found",
							"new_string": "replacement",
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
						ToolUseID: "edit_fail",
						Content:   "Error: old_string not found in file",
						IsError:   true,
					},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	// Should show error icon
	if !strings.Contains(result, "❌") {
		t.Error("expected error icon for failed edit")
	}

	// Should show Edit tool summary
	if !strings.Contains(result, "🔧 Edit:") {
		t.Error("expected Edit tool in summary")
	}

	// Error message should be preserved and rendered as fallback content
	if !strings.Contains(result, "Error: old_string not found in file") {
		t.Error("expected error message to be preserved in output")
	}
}

func TestRenderMarkdown_EditGroupLegacyFormatPreserved(t *testing.T) {
	// Test that Edit tool results from older logs without structuredPatch are displayed
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type: "tool_use",
						Name: "Edit",
						ID:   "edit_legacy",
						Input: map[string]any{
							"file_path":  "/path/to/file.go",
							"old_string": "old content",
							"new_string": "new content",
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
						ToolUseID: "edit_legacy",
						Content:   "Successfully edited /path/to/file.go",
						IsError:   false,
					},
				},
			},
			// No ToolUseResult field - simulating legacy format
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	// Should show success icon
	if !strings.Contains(result, "✅") {
		t.Error("expected success icon for successful edit")
	}

	// Legacy content should be preserved and rendered as fallback
	if !strings.Contains(result, "Successfully edited /path/to/file.go") {
		t.Error("expected legacy format content to be preserved in output")
	}
}
