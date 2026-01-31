package transcript

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetLastDisplayableEntry(t *testing.T) {
	testdataDir := filepath.Join("testdata", "last_entry")

	tests := []struct {
		name           string
		fixture        string
		wantNil        bool
		wantErr        bool
		wantToolName   string
		wantTextPrefix string
	}{
		{
			name:         "finds tool_use entry at end",
			fixture:      "tool_use.jsonl",
			wantToolName: "Read",
		},
		{
			name:           "finds text entry at end",
			fixture:        "text_only.jsonl",
			wantTextPrefix: "Go is a statically typed",
		},
		{
			name:         "prioritizes tool_use when both present in same entry",
			fixture:      "mixed_tool_and_text.jsonl",
			wantToolName: "Glob",
		},
		{
			name:    "skips thinking-only entries, returns nil",
			fixture: "thinking_only.jsonl",
			wantNil: true,
		},
		{
			name:           "skips meta entry, returns previous displayable",
			fixture:        "meta_entry.jsonl",
			wantTextPrefix: "I found the answer",
		},
		{
			name:         "handles incomplete JSON at end",
			fixture:      "incomplete_last_line.jsonl",
			wantToolName: "Read",
		},
		{
			name:    "returns nil for only system/user messages",
			fixture: "system_only.jsonl",
			wantNil: true,
		},
		{
			name:    "returns nil for empty file",
			fixture: "../empty.jsonl",
			wantNil: true,
		},
		{
			name:    "returns error for missing file",
			fixture: "nonexistent.jsonl",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(testdataDir, tt.fixture)
			entry, err := GetLastDisplayableEntry(path)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantNil {
				if entry != nil {
					t.Errorf("expected nil entry, got %+v", entry)
				}
				return
			}

			if entry == nil {
				t.Fatal("expected entry, got nil")
			}

			if tt.wantToolName != "" {
				found := false
				for _, c := range entry.Message.Content {
					if c.Type == "tool_use" && c.Name == tt.wantToolName {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected tool_use with name %q, not found in entry", tt.wantToolName)
				}
			}

			if tt.wantTextPrefix != "" {
				found := false
				for _, c := range entry.Message.Content {
					if c.Type == "text" && strings.HasPrefix(c.Text, tt.wantTextPrefix) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected text starting with %q, not found in entry", tt.wantTextPrefix)
				}
			}
		})
	}
}

func TestGetLastDisplayableEntry_LargeFile(t *testing.T) {
	// Create a temp file larger than initialChunkSize (64KB) to test window expansion
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "large.jsonl")

	// Each line is ~100 bytes, so we need ~700 lines to exceed 64KB
	// Add 800 user messages (about 80KB) to ensure we exceed the 64KB initial chunk
	var content strings.Builder
	for i := 0; i < 800; i++ {
		// Create lines that are about 100 bytes each
		content.WriteString(`{"type":"user","message":{"role":"user","content":"This is a filler message number ` +
			fmt.Sprintf("%04d", i) + ` with some extra padding to make it longer"}}` + "\n")
	}
	// Add the target entry at the end
	content.WriteString(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"TargetTool","input":{"file_path":"/target/file.go"}}]}}` + "\n")

	data := []byte(content.String())
	// Verify file is larger than 64KB to ensure window expansion is needed
	if len(data) < 64*1024 {
		t.Fatalf("test file should be > 64KB to test window expansion, got %d bytes", len(data))
	}

	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	entry, err := GetLastDisplayableEntry(tmpFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry == nil {
		t.Fatal("expected entry, got nil")
	}

	found := false
	for _, c := range entry.Message.Content {
		if c.Type == "tool_use" && c.Name == "TargetTool" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find TargetTool entry")
	}
}

func TestFormatToolUse(t *testing.T) {
	tests := []struct {
		name   string
		tool   string
		input  any
		expect string
	}{
		{
			name:   "file_path parameter",
			tool:   "Read",
			input:  map[string]any{"file_path": "/path/to/file.go"},
			expect: "Read: /path/to/file.go",
		},
		{
			name:   "path parameter",
			tool:   "Glob",
			input:  map[string]any{"path": "/src", "pattern": "*.go"},
			expect: "Glob: /src",
		},
		{
			name:   "command parameter",
			tool:   "Bash",
			input:  map[string]any{"command": "go test ./..."},
			expect: "Bash: go test ./...",
		},
		{
			name:   "pattern parameter",
			tool:   "Grep",
			input:  map[string]any{"pattern": "TODO", "output_mode": "content"},
			expect: "Grep: TODO",
		},
		{
			name:   "query parameter",
			tool:   "WebSearch",
			input:  map[string]any{"query": "golang best practices"},
			expect: "WebSearch: golang best practices",
		},
		{
			name:   "url parameter",
			tool:   "WebFetch",
			input:  map[string]any{"url": "https://example.com", "prompt": "Summarize"},
			expect: "WebFetch: https://example.com",
		},
		{
			name:   "prompt parameter",
			tool:   "Task",
			input:  map[string]any{"prompt": "Implement feature X"},
			expect: "Task: Implement feature X",
		},
		{
			name:   "priority order - file_path over path",
			tool:   "Edit",
			input:  map[string]any{"file_path": "/file.go", "path": "/other"},
			expect: "Edit: /file.go",
		},
		{
			name:   "priority order - path over command",
			tool:   "Custom",
			input:  map[string]any{"path": "/mypath", "command": "echo"},
			expect: "Custom: /mypath",
		},
		{
			name:   "truncation at 60 chars",
			tool:   "Read",
			input:  map[string]any{"file_path": "/very/long/path/that/exceeds/sixty/characters/in/total/length/file.go"},
			expect: "Read: /very/long/path/that/exceeds/sixty/characters/in/total/le...",
		},
		{
			name:   "no matching parameters",
			tool:   "Unknown",
			input:  map[string]any{"other_param": "value"},
			expect: "Unknown: value",
		},
		{
			name:   "no parameters",
			tool:   "Empty",
			input:  map[string]any{},
			expect: "Empty",
		},
		{
			name:   "nil input",
			tool:   "NilInput",
			input:  nil,
			expect: "NilInput",
		},
		{
			name:   "non-string parameter values",
			tool:   "Numbers",
			input:  map[string]any{"count": 42, "enabled": true},
			expect: "Numbers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatToolUse(tt.tool, tt.input)
			if got != tt.expect {
				t.Errorf("FormatToolUse(%q, %v) = %q, want %q", tt.tool, tt.input, got, tt.expect)
			}
		})
	}
}

func TestFormatLastAction(t *testing.T) {
	tests := []struct {
		name   string
		entry  *Entry
		expect string
	}{
		{
			name:   "nil entry",
			entry:  nil,
			expect: "",
		},
		{
			name:   "nil message",
			entry:  &Entry{Message: nil},
			expect: "",
		},
		{
			name: "tool_use entry",
			entry: &Entry{
				Message: &Message{
					Role: "assistant",
					Content: []ContentItem{
						{Type: "tool_use", Name: "Read", Input: map[string]any{"file_path": "/test.go"}},
					},
				},
			},
			expect: "Read: /test.go",
		},
		{
			name: "text entry",
			entry: &Entry{
				Message: &Message{
					Role: "assistant",
					Content: []ContentItem{
						{Type: "text", Text: "Here is the answer"},
					},
				},
			},
			expect: "Here is the answer",
		},
		{
			name: "tool_use takes priority over text",
			entry: &Entry{
				Message: &Message{
					Role: "assistant",
					Content: []ContentItem{
						{Type: "text", Text: "I'll search for that"},
						{Type: "tool_use", Name: "Grep", Input: map[string]any{"pattern": "func main"}},
					},
				},
			},
			expect: "Grep: func main",
		},
		{
			name: "text truncation at 80 chars",
			entry: &Entry{
				Message: &Message{
					Role: "assistant",
					Content: []ContentItem{
						{Type: "text", Text: "This is a very long text response that exceeds the eighty character limit and should be truncated with ellipsis."},
					},
				},
			},
			expect: "This is a very long text response that exceeds the eighty character limit and...",
		},
		{
			name: "empty text content",
			entry: &Entry{
				Message: &Message{
					Role: "assistant",
					Content: []ContentItem{
						{Type: "text", Text: ""},
					},
				},
			},
			expect: "",
		},
		{
			name: "thinking content is ignored",
			entry: &Entry{
				Message: &Message{
					Role: "assistant",
					Content: []ContentItem{
						{Type: "thinking", Thinking: "Let me think..."},
					},
				},
			},
			expect: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatLastAction(tt.entry)
			if got != tt.expect {
				t.Errorf("FormatLastAction() = %q, want %q", got, tt.expect)
			}
		})
	}
}

func TestExtractKeyInput(t *testing.T) {
	tests := []struct {
		name   string
		input  any
		expect string
	}{
		{
			name:   "nil input",
			input:  nil,
			expect: "",
		},
		{
			name:   "non-map input",
			input:  "string",
			expect: "",
		},
		{
			name:   "empty map",
			input:  map[string]any{},
			expect: "",
		},
		{
			name:   "file_path priority",
			input:  map[string]any{"file_path": "/a.go", "path": "/b.go"},
			expect: "/a.go",
		},
		{
			name:   "empty string skipped",
			input:  map[string]any{"file_path": "", "path": "/valid.go"},
			expect: "/valid.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractKeyInput(tt.input)
			if got != tt.expect {
				t.Errorf("extractKeyInput(%v) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestIsDisplayableEntry(t *testing.T) {
	tests := []struct {
		name   string
		entry  *Entry
		expect bool
	}{
		{
			name:   "meta entry",
			entry:  &Entry{IsMeta: true, Message: &Message{Role: "assistant", Content: []ContentItem{{Type: "text", Text: "test"}}}},
			expect: false,
		},
		{
			name:   "nil message",
			entry:  &Entry{Message: nil},
			expect: false,
		},
		{
			name:   "user role",
			entry:  &Entry{Message: &Message{Role: "user", Content: []ContentItem{{Type: "text", Text: "test"}}}},
			expect: false,
		},
		{
			name:   "system role",
			entry:  &Entry{Message: &Message{Role: "system", Content: []ContentItem{{Type: "text", Text: "test"}}}},
			expect: false,
		},
		{
			name:   "assistant with text",
			entry:  &Entry{Message: &Message{Role: "assistant", Content: []ContentItem{{Type: "text", Text: "test"}}}},
			expect: true,
		},
		{
			name:   "assistant with tool_use",
			entry:  &Entry{Message: &Message{Role: "assistant", Content: []ContentItem{{Type: "tool_use", Name: "Read"}}}},
			expect: true,
		},
		{
			name:   "assistant with only thinking",
			entry:  &Entry{Message: &Message{Role: "assistant", Content: []ContentItem{{Type: "thinking", Thinking: "..."}}}},
			expect: false,
		},
		{
			name:   "assistant with only tool_result",
			entry:  &Entry{Message: &Message{Role: "assistant", Content: []ContentItem{{Type: "tool_result", Content: "result"}}}},
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsDisplayableEntry(tt.entry)
			if got != tt.expect {
				t.Errorf("IsDisplayableEntry() = %v, want %v", got, tt.expect)
			}
		})
	}
}
