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
