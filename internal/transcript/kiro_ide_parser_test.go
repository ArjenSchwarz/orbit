package transcript

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseKiroIDE(t *testing.T) {
	tests := map[string]struct {
		input       string
		wantEntries int
		wantTypes   []string // expected Type values
		wantRoles   []string // expected Message.Role values
		wantWarns   int
	}{
		"basic conversation": {
			input: `{
				"executionId": "exec-1",
				"chat": [
					{"role": "human", "content": "Hello"},
					{"role": "bot", "content": "Hi there"}
				],
				"metadata": {"modelId": "auto", "startTime": 1000, "endTime": 2000}
			}`,
			wantEntries: 2,
			wantTypes:   []string{"user", "assistant"},
			wantRoles:   []string{"user", "assistant"},
		},
		"system prompt filtering": {
			input: `{
				"executionId": "exec-2",
				"chat": [
					{"role": "human", "content": "<identity>You are an AI assistant...</identity>"},
					{"role": "bot", "content": "I understand"},
					{"role": "human", "content": "What is Go?"},
					{"role": "bot", "content": "Go is a programming language"}
				],
				"metadata": {"modelId": "auto", "startTime": 1000, "endTime": 2000}
			}`,
			wantEntries: 3,
			wantTypes:   []string{"assistant", "user", "assistant"},
			wantRoles:   []string{"assistant", "user", "assistant"},
		},
		"tool messages": {
			input: `{
				"executionId": "exec-3",
				"chat": [
					{"role": "human", "content": "List files"},
					{"role": "bot", "content": "I'll check the directory"},
					{"role": "tool", "content": "file1.go\nfile2.go"}
				],
				"metadata": {"modelId": "auto", "startTime": 1000, "endTime": 2000}
			}`,
			wantEntries: 3,
			wantTypes:   []string{"user", "assistant", "user"},
			wantRoles:   []string{"user", "assistant", "user"},
		},
		"empty chat array": {
			input: `{
				"executionId": "exec-4",
				"chat": [],
				"metadata": {"modelId": "auto", "startTime": 1000, "endTime": 2000}
			}`,
			wantEntries: 0,
		},
		"missing role": {
			input: `{
				"executionId": "exec-5",
				"chat": [
					{"role": "", "content": "No role here"},
					{"role": "bot", "content": "Valid message"}
				],
				"metadata": {"modelId": "auto", "startTime": 1000, "endTime": 2000}
			}`,
			wantEntries: 1,
			wantTypes:   []string{"assistant"},
			wantRoles:   []string{"assistant"},
			wantWarns:   1,
		},
		"empty content messages": {
			input: `{
				"executionId": "exec-6",
				"chat": [
					{"role": "bot", "content": ""},
					{"role": "human", "content": "Hello"},
					{"role": "tool", "content": ""}
				],
				"metadata": {"modelId": "auto", "startTime": 1000, "endTime": 2000}
			}`,
			wantEntries: 1,
			wantTypes:   []string{"user"},
			wantRoles:   []string{"user"},
		},
		"no identity prefix": {
			input: `{
				"executionId": "exec-7",
				"chat": [
					{"role": "human", "content": "Just a normal message"},
					{"role": "bot", "content": "Response"}
				],
				"metadata": {"modelId": "auto", "startTime": 1000, "endTime": 2000}
			}`,
			wantEntries: 2,
			wantTypes:   []string{"user", "assistant"},
			wantRoles:   []string{"user", "assistant"},
		},
		"only system prompt": {
			input: `{
				"executionId": "exec-8",
				"chat": [
					{"role": "human", "content": "<identity>System prompt only</identity>"}
				],
				"metadata": {"modelId": "auto", "startTime": 1000, "endTime": 2000}
			}`,
			wantEntries: 0,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result, err := ParseKiroIDE(strings.NewReader(tc.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(result.Entries) != tc.wantEntries {
				t.Fatalf("expected %d entries, got %d", tc.wantEntries, len(result.Entries))
			}

			for i, wantType := range tc.wantTypes {
				if result.Entries[i].Type != wantType {
					t.Errorf("entry %d: type = %q, want %q", i, result.Entries[i].Type, wantType)
				}
			}
			for i, wantRole := range tc.wantRoles {
				if result.Entries[i].Message == nil {
					t.Fatalf("entry %d: message is nil", i)
				}
				if result.Entries[i].Message.Role != wantRole {
					t.Errorf("entry %d: role = %q, want %q", i, result.Entries[i].Message.Role, wantRole)
				}
			}

			if tc.wantWarns > 0 && len(result.Warnings) != tc.wantWarns {
				t.Errorf("expected %d warnings, got %d", tc.wantWarns, len(result.Warnings))
			}
		})
	}
}

func TestParseKiroIDEContentPreservation(t *testing.T) {
	input := `{
		"executionId": "exec-content",
		"chat": [
			{"role": "human", "content": "What is 2+2?"},
			{"role": "bot", "content": "The answer is 4."},
			{"role": "tool", "content": "tool output here"}
		],
		"metadata": {"modelId": "auto", "startTime": 1000, "endTime": 2000}
	}`

	result, err := ParseKiroIDE(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(result.Entries))
	}

	// Human -> user text
	if text := result.Entries[0].Message.Content[0].Text; text != "What is 2+2?" {
		t.Errorf("entry 0: text = %q, want %q", text, "What is 2+2?")
	}
	if result.Entries[0].Message.Content[0].Type != "text" {
		t.Errorf("entry 0: content type = %q, want %q", result.Entries[0].Message.Content[0].Type, "text")
	}

	// Bot -> assistant text
	if text := result.Entries[1].Message.Content[0].Text; text != "The answer is 4." {
		t.Errorf("entry 1: text = %q, want %q", text, "The answer is 4.")
	}

	// Tool -> user tool_result
	if ct := result.Entries[2].Message.Content[0].Type; ct != "tool_result" {
		t.Errorf("entry 2: content type = %q, want %q", ct, "tool_result")
	}
	if content := result.Entries[2].Message.Content[0].Content; content != "tool output here" {
		t.Errorf("entry 2: content = %q, want %q", content, "tool output here")
	}
}

func TestExtractKiroIDECost(t *testing.T) {
	tests := map[string]struct {
		setupFile func(t *testing.T, dir string) string // returns file path
		wantCost  float64
	}{
		"valid usage summary": {
			setupFile: func(t *testing.T, dir string) string {
				t.Helper()
				p := filepath.Join(dir, "detail.json")
				err := os.WriteFile(p, []byte(`{
					"executionId": "exec-1",
					"usageSummary": [
						{"unit": "credit", "unitPlural": "credits", "usage": 0.05},
						{"unit": "credit", "unitPlural": "credits", "usage": 0.03}
					]
				}`), 0o644)
				if err != nil {
					t.Fatal(err)
				}
				return p
			},
			wantCost: 0.08,
		},
		"mixed units": {
			setupFile: func(t *testing.T, dir string) string {
				t.Helper()
				p := filepath.Join(dir, "detail.json")
				err := os.WriteFile(p, []byte(`{
					"executionId": "exec-2",
					"usageSummary": [
						{"unit": "credit", "unitPlural": "credits", "usage": 0.10},
						{"unit": "token", "unitPlural": "tokens", "usage": 5000},
						{"unit": "credit", "unitPlural": "credits", "usage": 0.05}
					]
				}`), 0o644)
				if err != nil {
					t.Fatal(err)
				}
				return p
			},
			wantCost: 0.15,
		},
		"missing file": {
			setupFile: func(_ *testing.T, dir string) string {
				return filepath.Join(dir, "nonexistent.json")
			},
			wantCost: 0,
		},
		"invalid json": {
			setupFile: func(t *testing.T, dir string) string {
				t.Helper()
				p := filepath.Join(dir, "bad.json")
				err := os.WriteFile(p, []byte(`{not valid json`), 0o644)
				if err != nil {
					t.Fatal(err)
				}
				return p
			},
			wantCost: 0,
		},
		"empty usage summary": {
			setupFile: func(t *testing.T, dir string) string {
				t.Helper()
				p := filepath.Join(dir, "empty.json")
				err := os.WriteFile(p, []byte(`{
					"executionId": "exec-3",
					"usageSummary": []
				}`), 0o644)
				if err != nil {
					t.Fatal(err)
				}
				return p
			},
			wantCost: 0,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := tc.setupFile(t, dir)
			got := extractKiroIDECost(path)

			if math.Abs(got-tc.wantCost) > 0.0001 {
				t.Errorf("cost = %.4f, want %.4f", got, tc.wantCost)
			}
		})
	}
}

func TestConvertKiroIDEToEntries(t *testing.T) {
	tests := map[string]struct {
		chatFile    *KiroIDEChatFile
		wantEntries int
		wantTypes   []string
		wantRoles   []string
		wantWarns   int
	}{
		"human maps to user": {
			chatFile: &KiroIDEChatFile{
				Chat: []KiroIDEMessage{
					{Role: "human", Content: "Hello"},
				},
			},
			wantEntries: 1,
			wantTypes:   []string{"user"},
			wantRoles:   []string{"user"},
		},
		"bot maps to assistant": {
			chatFile: &KiroIDEChatFile{
				Chat: []KiroIDEMessage{
					{Role: "bot", Content: "Hi"},
				},
			},
			wantEntries: 1,
			wantTypes:   []string{"assistant"},
			wantRoles:   []string{"assistant"},
		},
		"tool maps to user with tool_result": {
			chatFile: &KiroIDEChatFile{
				Chat: []KiroIDEMessage{
					{Role: "tool", Content: "output"},
				},
			},
			wantEntries: 1,
			wantTypes:   []string{"user"},
			wantRoles:   []string{"user"},
		},
		"warning on missing role": {
			chatFile: &KiroIDEChatFile{
				Chat: []KiroIDEMessage{
					{Role: "", Content: "no role"},
				},
			},
			wantEntries: 0,
			wantWarns:   1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			entries, warnings := convertKiroIDEToEntries(tc.chatFile)

			if len(entries) != tc.wantEntries {
				t.Fatalf("expected %d entries, got %d", tc.wantEntries, len(entries))
			}
			for i, wantType := range tc.wantTypes {
				if entries[i].Type != wantType {
					t.Errorf("entry %d: type = %q, want %q", i, entries[i].Type, wantType)
				}
			}
			for i, wantRole := range tc.wantRoles {
				if entries[i].Message.Role != wantRole {
					t.Errorf("entry %d: role = %q, want %q", i, entries[i].Message.Role, wantRole)
				}
			}
			if len(warnings) != tc.wantWarns {
				t.Errorf("expected %d warnings, got %d", tc.wantWarns, len(warnings))
			}
		})
	}
}

func TestParseKiroIDEWithCostPath(t *testing.T) {
	dir := t.TempDir()
	costPath := filepath.Join(dir, "detail.json")
	err := os.WriteFile(costPath, []byte(`{
		"executionId": "exec-1",
		"usageSummary": [
			{"unit": "credit", "unitPlural": "credits", "usage": 0.12}
		]
	}`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	input := `{
		"executionId": "exec-1",
		"chat": [
			{"role": "human", "content": "Hello"},
			{"role": "bot", "content": "Hi"}
		],
		"metadata": {"modelId": "auto", "startTime": 1000, "endTime": 2000}
	}`

	result, err := ParseKiroIDEWithCostPath(strings.NewReader(input), costPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Metadata == nil {
		t.Fatal("expected metadata, got nil")
	}
	if result.Metadata.TotalCost == nil {
		t.Fatal("expected TotalCost, got nil")
	}
	if math.Abs(*result.Metadata.TotalCost-0.12) > 0.0001 {
		t.Errorf("TotalCost = %.4f, want %.4f", *result.Metadata.TotalCost, 0.12)
	}
	if result.Metadata.CostUnit != "credits" {
		t.Errorf("CostUnit = %q, want %q", result.Metadata.CostUnit, "credits")
	}
}

func TestParseKiroIDEWithoutCostHasNoMetadata(t *testing.T) {
	input := `{
		"executionId": "exec-1",
		"chat": [
			{"role": "human", "content": "Hello"},
			{"role": "bot", "content": "Hi"}
		],
		"metadata": {"modelId": "auto", "startTime": 1000, "endTime": 2000}
	}`

	result, err := ParseKiroIDE(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Metadata != nil {
		t.Errorf("expected nil metadata for ParseKiroIDE without cost path, got %+v", result.Metadata)
	}
}
