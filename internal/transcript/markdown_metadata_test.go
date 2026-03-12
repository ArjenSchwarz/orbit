package transcript

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Tests for Markdown metadata rendering (Task 14/15).
// These verify that message headers include timestamp and model metadata
// per requirements 1.1, 1.2, 1.5, 2.1, 2.2, 3.1, 3.3, 3.5, 3.6.

func TestRenderMarkdown_UserMessageWithTimestamp(t *testing.T) {
	entries := []Entry{
		{
			Type:      "user",
			Timestamp: "2026-03-12T03:32:05Z",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{Type: "text", Text: "Hello!"},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	// User messages show timestamp only, never model (req 2.1)
	assert.Contains(t, result, "## 👤 User · 2026-03-12T03:32:05Z")
	assert.Contains(t, result, "Hello!")
}

func TestRenderMarkdown_AssistantMessageWithTimestampAndModel(t *testing.T) {
	entries := []Entry{
		{
			Type:      "assistant",
			Timestamp: "2026-03-12T03:32:05Z",
			Model:     "claude-opus",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{Type: "text", Text: "Here's my answer."},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	// Assistant messages show both timestamp and model (req 2.1, 3.1)
	assert.Contains(t, result, "## 🤖 Assistant · 2026-03-12T03:32:05Z · claude-opus")
	assert.Contains(t, result, "Here's my answer.")
}

func TestRenderMarkdown_AssistantMessageWithTimestampOnly(t *testing.T) {
	entries := []Entry{
		{
			Type:      "assistant",
			Timestamp: "2026-03-12T03:32:05Z",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{Type: "text", Text: "Response without model info."},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	// Timestamp shown, no model suffix (req 2.2 — omit silently)
	assert.Contains(t, result, "## 🤖 Assistant · 2026-03-12T03:32:05Z")
	assert.NotContains(t, result, "## 🤖 Assistant · 2026-03-12T03:32:05Z ·")
}

func TestRenderMarkdown_AssistantMessageWithModelOnly(t *testing.T) {
	entries := []Entry{
		{
			Type:  "assistant",
			Model: "gpt-4o",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{Type: "text", Text: "Response with model but no timestamp."},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	// Model shown even without timestamp (req 3.1)
	assert.Contains(t, result, "## 🤖 Assistant · gpt-4o")
}

func TestRenderMarkdown_NoMetadata_HeaderUnchanged(t *testing.T) {
	entries := []Entry{
		{
			Type: "user",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{Type: "text", Text: "No metadata here."},
				},
			},
		},
		{
			Type: "assistant",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{Type: "text", Text: "Also no metadata."},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	// Headers should be exactly as before — no trailing separator (req 1.5, 4.1)
	assert.Contains(t, result, "## 👤 User\n")
	assert.Contains(t, result, "## 🤖 Assistant\n")
	// Ensure no stray separators
	assert.NotContains(t, result, "## 👤 User ·")
	assert.NotContains(t, result, "## 🤖 Assistant ·")
}

func TestRenderMarkdown_InvalidTimestamp_OmittedSilently(t *testing.T) {
	entries := []Entry{
		{
			Type:      "user",
			Timestamp: "not-a-date",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{Type: "text", Text: "Bad timestamp."},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	// Invalid timestamp should be silently omitted (req 1.5)
	assert.Contains(t, result, "## 👤 User\n")
	assert.NotContains(t, result, "not-a-date")
}

func TestRenderMarkdown_TimestampConvertedToLocalTimezone(t *testing.T) {
	// TestMain sets time.Local to UTC+0 ("TEST" zone)
	entries := []Entry{
		{
			Type:      "user",
			Timestamp: "2026-03-12T14:32:05+11:00",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{Type: "text", Text: "Offset timestamp."},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	// Should be converted to local timezone (UTC+0 in test) per req 1.2
	assert.Contains(t, result, "## 👤 User · 2026-03-12T03:32:05Z")
	// Original offset should not appear
	assert.NotContains(t, result, "+11:00")
}

func TestRenderMarkdown_SlashCommandWithTimestamp(t *testing.T) {
	entries := []Entry{
		{
			Type:      "user",
			Timestamp: "2026-03-12T03:32:05Z",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{Type: "text", Text: "<command-message>catchup</command-message>\n<command-name>/catchup</command-name>"},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	// Slash command user header should include timestamp (req 1.1)
	assert.Contains(t, result, "## 👤 User · 2026-03-12T03:32:05Z")
}

func TestRenderMarkdown_ReadGroupWithTimestamp(t *testing.T) {
	// Consecutive Read calls are grouped; the group header should show
	// the first entry's timestamp (req 3.6)
	entries := []Entry{
		makeReadEntry("read_1", "/src/main.go", "2026-03-12T03:00:00Z"),
		makeReadEntry("read_2", "/src/util.go", "2026-03-12T03:01:00Z"),
		makeToolResultEntry("read_1", "package main"),
		makeToolResultEntry("read_2", "package util"),
	}

	result := RenderMarkdown(entries, RenderOptions{})

	// Group header should use first entry's timestamp
	assert.Contains(t, result, "## 🤖 Assistant · 2026-03-12T03:00:00Z")
	// Should NOT show the second entry's timestamp in the header
	lines := strings.Split(result, "\n")
	headerCount := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "## 🤖 Assistant") {
			headerCount++
		}
	}
	// Should be a single grouped header, not two separate ones
	assert.Equal(t, 1, headerCount, "expected single grouped assistant header")
}

func TestRenderMarkdown_EditGroupWithTimestamp(t *testing.T) {
	// Consecutive Edit calls are grouped; the group header should show
	// the first entry's timestamp (req 3.6)
	entries := []Entry{
		makeEditEntry("edit_1", "/src/main.go", "2026-03-12T04:00:00Z"),
		makeEditEntry("edit_2", "/src/util.go", "2026-03-12T04:01:00Z"),
		{
			Type: "user",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{Type: "tool_result", ToolUseID: "edit_1", Content: "OK"},
					{Type: "tool_result", ToolUseID: "edit_2", Content: "OK"},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	// Group header should use first entry's timestamp
	assert.Contains(t, result, "## 🤖 Assistant · 2026-03-12T04:00:00Z")
}

func TestRenderMarkdown_ReadGroupNoTimestamp_HeaderUnchanged(t *testing.T) {
	// Read group with no timestamps should render header as before
	entries := []Entry{
		makeReadEntry("read_no_ts", "/src/main.go", ""),
		makeToolResultEntry("read_no_ts", "package main"),
	}

	result := RenderMarkdown(entries, RenderOptions{})

	assert.Contains(t, result, "## 🤖 Assistant\n")
	assert.NotContains(t, result, "## 🤖 Assistant ·")
}

func TestRenderMarkdown_UserMessageModelNotShown(t *testing.T) {
	// Even if a user entry somehow has a Model field, it should NOT be displayed
	// (req 2.1: model shown on assistant messages only)
	entries := []Entry{
		{
			Type:      "user",
			Timestamp: "2026-03-12T03:32:05Z",
			Model:     "should-not-appear",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{Type: "text", Text: "User message."},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	assert.Contains(t, result, "## 👤 User · 2026-03-12T03:32:05Z")
	assert.NotContains(t, result, "should-not-appear")
}

func TestRenderMarkdown_MetadataConsistentSeparator(t *testing.T) {
	// Verify the ` · ` separator is used consistently (req 3.3)
	entries := []Entry{
		{
			Type:      "assistant",
			Timestamp: "2026-03-12T03:32:05Z",
			Model:     "claude-opus",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{Type: "text", Text: "Test."},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	// The header line should use ` · ` as separator between all parts
	for _, line := range strings.Split(result, "\n") {
		if strings.HasPrefix(line, "## 🤖 Assistant") {
			assert.Equal(t, "## 🤖 Assistant · 2026-03-12T03:32:05Z · claude-opus", line)
			break
		}
	}
}

func TestRenderMarkdown_MultipleMessagesWithMixedMetadata(t *testing.T) {
	// End-to-end: mix of messages with and without metadata
	entries := []Entry{
		{
			Type:      "user",
			Timestamp: "2026-03-12T01:00:00Z",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{Type: "text", Text: "First question."},
				},
			},
		},
		{
			Type:      "assistant",
			Timestamp: "2026-03-12T01:00:05Z",
			Model:     "claude-opus",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{Type: "text", Text: "First answer."},
				},
			},
		},
		{
			Type: "user",
			Message: &Message{
				Role: "user",
				Content: []ContentItem{
					{Type: "text", Text: "Second question (no metadata)."},
				},
			},
		},
		{
			Type:      "assistant",
			Timestamp: "2026-03-12T01:01:00Z",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{Type: "text", Text: "Second answer (no model)."},
				},
			},
		},
	}

	result := RenderMarkdown(entries, RenderOptions{})

	assert.Contains(t, result, "## 👤 User · 2026-03-12T01:00:00Z")
	assert.Contains(t, result, "## 🤖 Assistant · 2026-03-12T01:00:05Z · claude-opus")
	assert.Contains(t, result, "## 👤 User\n")
	assert.Contains(t, result, "## 🤖 Assistant · 2026-03-12T01:01:00Z\n")
}

func TestRenderEntries_MetadataIncluded(t *testing.T) {
	// Verify RenderEntries (used by follow mode) also includes metadata
	entries := []Entry{
		{
			Type:      "assistant",
			Timestamp: "2026-03-12T03:32:05Z",
			Model:     "claude-opus",
			Message: &Message{
				Role: "assistant",
				Content: []ContentItem{
					{Type: "text", Text: "Follow mode response."},
				},
			},
		},
	}

	toolMeta := BuildToolMeta(entries)
	skillDescriptions := BuildSkillDescriptionMap(entries)
	result := RenderEntries(entries, toolMeta, skillDescriptions, RenderOptions{})

	assert.Contains(t, result, "## 🤖 Assistant · 2026-03-12T03:32:05Z · claude-opus")
}
