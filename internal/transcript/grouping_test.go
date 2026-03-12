package transcript

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to build a Read tool_use assistant entry with a timestamp.
func makeReadEntry(toolID, filePath, timestamp string) Entry {
	return Entry{
		Type:      "assistant",
		Timestamp: timestamp,
		Message: &Message{
			Role: "assistant",
			Content: []ContentItem{
				{Type: "tool_use", Name: "Read", ID: toolID, Input: map[string]any{"file_path": filePath}},
			},
		},
	}
}

// Helper to build an Edit tool_use assistant entry with a timestamp.
func makeEditEntry(toolID, filePath, timestamp string) Entry {
	return Entry{
		Type:      "assistant",
		Timestamp: timestamp,
		Message: &Message{
			Role: "assistant",
			Content: []ContentItem{
				{Type: "tool_use", Name: "Edit", ID: toolID, Input: map[string]any{"file_path": filePath}},
			},
		},
	}
}

// Helper to build a tool_result user entry.
func makeToolResultEntry(toolID, content string) Entry {
	return Entry{
		Type: "user",
		Message: &Message{
			Role: "user",
			Content: []ContentItem{
				{Type: "tool_result", ToolUseID: toolID, Content: content},
			},
		},
	}
}

func TestExtractReadItems_PropagatesTimestamp(t *testing.T) {
	tests := map[string]struct {
		timestamp string
		wantTS    string
	}{
		"timestamp is propagated": {
			timestamp: "2026-03-12T03:32:05Z",
			wantTS:    "2026-03-12T03:32:05Z",
		},
		"empty timestamp propagated as empty": {
			timestamp: "",
			wantTS:    "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			entry := makeReadEntry("tool-1", "main.go", tc.timestamp)
			resultMap := map[string]toolResultInfo{
				"tool-1": {Content: "file contents"},
			}
			usedIDs := make(map[string]bool)

			items := extractReadItems(&entry, resultMap, usedIDs)

			require.Len(t, items, 1)
			assert.Equal(t, tc.wantTS, items[0].Timestamp)
		})
	}
}

func TestExtractReadItems_MultipleToolUses(t *testing.T) {
	entry := Entry{
		Type:      "assistant",
		Timestamp: "2026-03-12T10:00:00Z",
		Message: &Message{
			Role: "assistant",
			Content: []ContentItem{
				{Type: "tool_use", Name: "Read", ID: "r1", Input: map[string]any{"file_path": "a.go"}},
				{Type: "tool_use", Name: "Read", ID: "r2", Input: map[string]any{"file_path": "b.go"}},
			},
		},
	}
	resultMap := map[string]toolResultInfo{
		"r1": {Content: "a"},
		"r2": {Content: "b"},
	}
	usedIDs := make(map[string]bool)

	items := extractReadItems(&entry, resultMap, usedIDs)

	require.Len(t, items, 2)
	// Both items get the same entry timestamp
	assert.Equal(t, "2026-03-12T10:00:00Z", items[0].Timestamp)
	assert.Equal(t, "2026-03-12T10:00:00Z", items[1].Timestamp)
}

func TestExtractEditItems_PropagatesTimestamp(t *testing.T) {
	tests := map[string]struct {
		timestamp string
		wantTS    string
	}{
		"timestamp is propagated": {
			timestamp: "2026-03-12T04:00:00Z",
			wantTS:    "2026-03-12T04:00:00Z",
		},
		"empty timestamp propagated as empty": {
			timestamp: "",
			wantTS:    "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			entry := makeEditEntry("tool-1", "main.go", tc.timestamp)
			resultMap := map[string]toolResultInfo{
				"tool-1": {Content: "ok"},
			}
			usedIDs := make(map[string]bool)

			items := extractEditItems(&entry, resultMap, usedIDs)

			require.Len(t, items, 1)
			assert.Equal(t, tc.wantTS, items[0].Timestamp)
		})
	}
}

func TestPreprocessEntries_ReadGroupTimestamp(t *testing.T) {
	// Three consecutive reads with different timestamps.
	// The group should use the first read's timestamp.
	entries := []Entry{
		makeReadEntry("r1", "a.go", "2026-03-12T01:00:00Z"),
		makeToolResultEntry("r1", "contents-a"),
		makeReadEntry("r2", "b.go", "2026-03-12T02:00:00Z"),
		makeToolResultEntry("r2", "contents-b"),
		makeReadEntry("r3", "c.go", "2026-03-12T03:00:00Z"),
		makeToolResultEntry("r3", "contents-c"),
	}

	groups := preprocessEntries(entries)

	// Should produce a single read_group
	require.Len(t, groups, 1)
	assert.Equal(t, "read_group", groups[0].Type)
	assert.Equal(t, "2026-03-12T01:00:00Z", groups[0].Timestamp)
	require.Len(t, groups[0].Reads, 3)
}

func TestPreprocessEntries_EditGroupTimestamp(t *testing.T) {
	entries := []Entry{
		makeEditEntry("e1", "x.go", "2026-03-12T05:00:00Z"),
		makeToolResultEntry("e1", "ok"),
		makeEditEntry("e2", "y.go", "2026-03-12T06:00:00Z"),
		makeToolResultEntry("e2", "ok"),
	}

	groups := preprocessEntries(entries)

	require.Len(t, groups, 1)
	assert.Equal(t, "edit_group", groups[0].Type)
	assert.Equal(t, "2026-03-12T05:00:00Z", groups[0].Timestamp)
	require.Len(t, groups[0].Edits, 2)
}

func TestPreprocessEntries_MixedGroupsPreserveTimestamps(t *testing.T) {
	// Read group followed by edit group — each gets its own first-item timestamp.
	entries := []Entry{
		makeReadEntry("r1", "a.go", "2026-03-12T01:00:00Z"),
		makeToolResultEntry("r1", "a"),
		makeReadEntry("r2", "b.go", "2026-03-12T02:00:00Z"),
		makeToolResultEntry("r2", "b"),
		makeEditEntry("e1", "c.go", "2026-03-12T03:00:00Z"),
		makeToolResultEntry("e1", "ok"),
	}

	groups := preprocessEntries(entries)

	require.Len(t, groups, 2)
	assert.Equal(t, "read_group", groups[0].Type)
	assert.Equal(t, "2026-03-12T01:00:00Z", groups[0].Timestamp)
	assert.Equal(t, "edit_group", groups[1].Type)
	assert.Equal(t, "2026-03-12T03:00:00Z", groups[1].Timestamp)
}

func TestPreprocessEntries_EmptyTimestampPropagated(t *testing.T) {
	// Entries without timestamps should still produce groups with empty timestamps.
	entries := []Entry{
		makeReadEntry("r1", "a.go", ""),
		makeToolResultEntry("r1", "a"),
	}

	groups := preprocessEntries(entries)

	require.Len(t, groups, 1)
	assert.Equal(t, "read_group", groups[0].Type)
	assert.Equal(t, "", groups[0].Timestamp)
}

func TestPreprocessEntries_RegularEntriesNoGroupTimestamp(t *testing.T) {
	// Non-grouped entries (regular user/assistant) don't set group-level timestamp.
	entries := []Entry{
		{
			Type:      "user",
			Timestamp: "2026-03-12T10:00:00Z",
			Message: &Message{
				Role:    "user",
				Content: []ContentItem{{Type: "text", Text: "hello"}},
			},
		},
	}

	groups := preprocessEntries(entries)

	require.Len(t, groups, 1)
	assert.Equal(t, "user", groups[0].Type)
	// Regular entries carry timestamp on the Entry itself, not on the group
	assert.Equal(t, "", groups[0].Timestamp)
	assert.Equal(t, "2026-03-12T10:00:00Z", groups[0].Entries[0].Timestamp)
}
