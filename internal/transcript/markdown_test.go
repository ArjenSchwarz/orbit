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

	result := RenderMarkdown(entries, RenderOptions{})

	if !strings.Contains(result, "### 🔧 Tool: `Read`") {
		t.Error("expected tool heading")
	}
	if !strings.Contains(result, "```json") {
		t.Error("expected JSON code block")
	}
	if !strings.Contains(result, "file_path") {
		t.Error("expected input content")
	}
}

func TestRenderMarkdown_ToolResultSuccess(t *testing.T) {
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

	if !strings.Contains(result, "#### ✅ Tool Result") {
		t.Error("expected success heading")
	}
	if !strings.Contains(result, "File contents here") {
		t.Error("expected result content")
	}
}

func TestRenderMarkdown_ToolResultError(t *testing.T) {
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

	if !strings.Contains(result, "#### ❌ Tool Error") {
		t.Error("expected error heading")
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
	if !strings.Contains(markdown, "🔧") {
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
		"script tag":       {"<script>alert(1)</script>", "&lt;script&gt;alert(1)&lt;/script&gt;"},
		"closing summary":  {"</summary>", "&lt;/summary&gt;"},
		"ampersand":        {"foo & bar", "foo &amp; bar"},
		"html entities":    {"<div class=\"x\">", "&lt;div class=&#34;x&#34;&gt;"},
		"normal text":      {"Explore: Search files", "Explore: Search files"},
		"unicode safe":     {"🔧 Tool", "🔧 Tool"},
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
						},
					},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	if !strings.Contains(result, "<details>") {
		t.Error("Task tool should always be wrapped in details")
	}
	if !strings.Contains(result, "<summary>🔧 Explore: Search for config files</summary>") {
		t.Error("expected Task tool summary with subagent_type and description")
	}
	if !strings.Contains(result, "</details>") {
		t.Error("expected closing details tag")
	}
}

func TestRenderMarkdown_TaskToolFallback(t *testing.T) {
	tests := map[string]struct {
		input any
	}{
		"nil input":            {nil},
		"not a map":            {"invalid"},
		"empty map":            {map[string]any{}},
		"missing subagent":     {map[string]any{"description": "test"}},
		"empty subagent":       {map[string]any{"subagent_type": ""}},
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

func TestRenderMarkdown_SkillToolAlwaysCollapses(t *testing.T) {
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

	if !strings.Contains(result, "<details>") {
		t.Error("Skill tool should always be wrapped in details")
	}
	if !strings.Contains(result, "<summary>🔧 Skill: next-task</summary>") {
		t.Error("expected Skill tool summary with skill name")
	}
}

func TestRenderMarkdown_SkillToolFallback(t *testing.T) {
	tests := map[string]struct {
		input any
	}{
		"nil input":      {nil},
		"not a map":      {"invalid"},
		"empty map":      {map[string]any{}},
		"empty skill":    {map[string]any{"skill": ""}},
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

			if !strings.Contains(result, "<details>") {
				t.Error("Skill tool should still be wrapped in details")
			}
			if !strings.Contains(result, "<summary>🔧 Skill</summary>") {
				t.Errorf("expected fallback summary '🔧 Skill' for case %s", name)
			}
		})
	}
}

func TestRenderMarkdown_ToolNameCaseSensitive(t *testing.T) {
	tests := map[string]string{
		"task":  "task",
		"TASK":  "TASK",
		"skill": "skill",
		"SKILL": "SKILL",
	}

	for name, toolName := range tests {
		t.Run(name, func(t *testing.T) {
			// Create input small enough that threshold won't trigger collapse
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
			}

			result := RenderMarkdown(entries, RenderOptions{})

			// Case-insensitive variants should NOT collapse (they use heading format)
			if strings.Contains(result, "<details>") {
				t.Errorf("tool %q should not collapse (case-sensitive matching)", toolName)
			}
			if !strings.Contains(result, "### 🔧 Tool:") {
				t.Errorf("tool %q should use uncollapsed heading format", toolName)
			}
		})
	}
}

func TestRenderMarkdown_ShortToolNoCollapse(t *testing.T) {
	// Create input that is less than 500 runes when serialized
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type:  "tool_use",
						Name:  "Read",
						ID:    "tool_short",
						Input: map[string]any{"file_path": "/tmp/test.txt"},
					},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	// Short tool should NOT be collapsed
	if strings.Contains(result, "<details>") && !strings.Contains(result, "💭 Thinking") {
		t.Error("short tool should not be wrapped in details")
	}
	if !strings.Contains(result, "### 🔧 Tool: `Read`") {
		t.Error("short tool should use heading format")
	}
}

func TestRenderMarkdown_LongToolCollapses(t *testing.T) {
	// Create input that exceeds 500 runes when serialized
	longContent := strings.Repeat("x", 600)
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type:  "tool_use",
						Name:  "Read",
						ID:    "tool_long",
						Input: map[string]any{"file_path": longContent},
					},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	if !strings.Contains(result, "<details>") {
		t.Error("long tool should be wrapped in details")
	}
	if !strings.Contains(result, "<summary>🔧 Tool: Read</summary>") {
		t.Error("expected collapsed tool summary")
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
		"nil input":   nil,
		"empty map":   map[string]any{},
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
	// Tool use in assistant entry, result in user entry
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

	// Result should be collapsed and use the same summary as the tool_use
	if !strings.Contains(result, "<summary>✅ Explore: Search for files</summary>") {
		t.Error("expected tool result to inherit summary from tool_use")
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
	// Error result matching a Task tool
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

	// Error result should use ❌ icon
	if !strings.Contains(result, "<summary>❌ Explore: Search files</summary>") {
		t.Error("error result should use ❌ icon in summary")
	}
}

func TestRenderMarkdown_ZeroLengthResultNoCollapse(t *testing.T) {
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

	// Zero-length result should NOT collapse
	// Should use the uncollapsed format
	if strings.Contains(result, "<details>") && strings.Contains(result, "Tool Result</summary>") {
		t.Error("zero-length result should not collapse")
	}
}

// Cross-entry tool matching tests (Task 11)

func TestRenderMarkdown_CrossEntryToolMatching(t *testing.T) {
	// This is the critical test: tool_use appears in "assistant" entry,
	// but tool_result appears in "user" entry. The map must persist across entries.
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

	// The tool_use should be collapsed with Task summary
	if !strings.Contains(result, "<summary>🔧 Explore: Find config</summary>") {
		t.Error("tool_use should be collapsed with Task summary")
	}

	// The tool_result should inherit the collapse and summary from tool_use
	if !strings.Contains(result, "<summary>✅ Explore: Find config</summary>") {
		t.Error("tool_result should inherit summary from tool_use across entries")
	}
}

func TestRenderMarkdown_ToolResultInUserEntry(t *testing.T) {
	// Verify that tool_result is properly handled in user entries (not assistant)
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

	// Verify the user entry contains the collapsed tool_result
	if !strings.Contains(result, "<summary>✅ Skill: commit</summary>") {
		t.Error("tool_result in user entry should be collapsed with inherited summary")
	}
}

func TestRenderMarkdown_MultipleToolsMatching(t *testing.T) {
	// Multiple tool_use/result pairs should match correctly
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

	// Each result should match its corresponding tool_use
	if !strings.Contains(result, "<summary>✅ Plan: Create plan</summary>") {
		t.Error("first tool_result should match first tool_use")
	}
	if !strings.Contains(result, "<summary>✅ Skill: rune</summary>") {
		t.Error("second tool_result should match second tool_use")
	}
}

// Tests for Markdown output format (Task 15)

func TestRenderMarkdown_DetailsFormat(t *testing.T) {
	// Verify the <details>/<summary> structure is correct
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type: "tool_use",
						Name: "Task",
						ID:   "format_test",
						Input: map[string]any{
							"subagent_type": "Explore",
							"description":   "Test format",
						},
					},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	// Verify structure: <details>\n<summary>...</summary>\n\n...content...\n</details>
	expectedStructure := `<details>
<summary>🔧 Explore: Test format</summary>

` + "```json"

	if !strings.Contains(result, expectedStructure) {
		t.Errorf("expected details structure:\n%s\n\ngot:\n%s", expectedStructure, result)
	}

	// Verify closing tag
	if !strings.Contains(result, "</details>") {
		t.Error("expected closing </details> tag")
	}

	// Verify JSON code block inside details
	if !strings.Contains(result, "```json") || !strings.Contains(result, "```\n\n</details>") {
		t.Error("expected JSON code block inside details")
	}
}

func TestRenderMarkdown_UncollapsedFormat(t *testing.T) {
	// Verify that uncollapsed tools use the heading format
	entries := []Entry{
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{
						Type:  "tool_use",
						Name:  "Read",
						ID:    "uncollapsed_test",
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
						ToolUseID: "uncollapsed_test",
						Content:   "Short result",
						IsError:   false,
					},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	// Verify tool_use uses heading format (not details)
	if !strings.Contains(result, "### 🔧 Tool: `Read`") {
		t.Error("uncollapsed tool_use should use heading format")
	}

	// Verify tool_result uses heading format (not details)
	if !strings.Contains(result, "#### ✅ Tool Result") {
		t.Error("uncollapsed tool_result should use heading format")
	}

	// Should NOT have details tags for these short items
	// (Note: we need to check carefully since thinking blocks also use <details>)
	lines := strings.Split(result, "\n")
	for _, line := range lines {
		if strings.Contains(line, "<details>") && !strings.Contains(result, "💭 Thinking") {
			// If we find a details tag and there's no thinking block, it's an error
			if strings.Contains(line, "Tool") || strings.Contains(line, "Read") {
				t.Error("short tool should not use details tag")
			}
		}
	}
}
