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

func TestConvertKiroIDEActionsToEntries(t *testing.T) {
	chatFile := &KiroIDEChatFile{
		Chat: []KiroIDEMessage{
			{Role: "human", Content: "<identity>system prompt</identity>"},
			{Role: "human", Content: "Fix the bug"},
		},
	}

	t.Run("say action becomes assistant text", func(t *testing.T) {
		actions := []KiroIDEAction{
			{ActionID: "a1", ActionType: "say", ActionState: "Success", Output: map[string]any{"message": "I'll fix that for you."}},
		}
		entries, warnings := convertKiroIDEActionsToEntries(actions, chatFile)
		if len(warnings) != 0 {
			t.Errorf("expected 0 warnings, got %d", len(warnings))
		}
		// 1 user message from chat + 1 say action
		if len(entries) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(entries))
		}
		// First entry: user message from chat
		if entries[0].Type != "user" {
			t.Errorf("entry 0: type = %q, want %q", entries[0].Type, "user")
		}
		if entries[0].Message.Content[0].Text != "Fix the bug" {
			t.Errorf("entry 0: text = %q, want %q", entries[0].Message.Content[0].Text, "Fix the bug")
		}
		// Second entry: say action
		if entries[1].Type != "assistant" {
			t.Errorf("entry 1: type = %q, want %q", entries[1].Type, "assistant")
		}
		if entries[1].Message.Content[0].Text != "I'll fix that for you." {
			t.Errorf("entry 1: text = %q, want %q", entries[1].Message.Content[0].Text, "I'll fix that for you.")
		}
	})

	t.Run("readFiles action produces tool_use and tool_result", func(t *testing.T) {
		actions := []KiroIDEAction{
			{
				ActionID: "tool-1", ActionType: "readFiles", ActionState: "Accepted",
				Input: map[string]any{
					"files": []any{
						map[string]any{"path": "main.go"},
						map[string]any{"path": "go.mod"},
					},
				},
			},
		}
		entries, _ := convertKiroIDEActionsToEntries(actions, chatFile)
		// 1 user + 1 tool_use + 1 tool_result = 3
		if len(entries) != 3 {
			t.Fatalf("expected 3 entries, got %d", len(entries))
		}
		toolUse := entries[1]
		if toolUse.Type != "assistant" {
			t.Errorf("tool_use entry type = %q, want %q", toolUse.Type, "assistant")
		}
		if toolUse.Message.Content[0].Type != "tool_use" {
			t.Errorf("content type = %q, want %q", toolUse.Message.Content[0].Type, "tool_use")
		}
		if toolUse.Message.Content[0].Name != "Read" {
			t.Errorf("tool name = %q, want %q", toolUse.Message.Content[0].Name, "Read")
		}
		if toolUse.Message.Content[0].ID != "tool-1" {
			t.Errorf("tool ID = %q, want %q", toolUse.Message.Content[0].ID, "tool-1")
		}
		toolResult := entries[2]
		if toolResult.Message.Content[0].Type != "tool_result" {
			t.Errorf("result type = %q, want %q", toolResult.Message.Content[0].Type, "tool_result")
		}
		if toolResult.Message.Content[0].ToolUseID != "tool-1" {
			t.Errorf("tool_use_id = %q, want %q", toolResult.Message.Content[0].ToolUseID, "tool-1")
		}
		want := "Read 2 file(s): main.go, go.mod"
		if toolResult.Message.Content[0].Content != want {
			t.Errorf("result content = %q, want %q", toolResult.Message.Content[0].Content, want)
		}
	})

	t.Run("runCommand with output and exit code", func(t *testing.T) {
		actions := []KiroIDEAction{
			{
				ActionID: "cmd-1", ActionType: "runCommand", ActionState: "Success",
				Input:  map[string]any{"command": "go test ./..."},
				Output: map[string]any{"output": "PASS\nok  mypackage 0.1s\n", "exitCode": float64(0)},
			},
		}
		entries, _ := convertKiroIDEActionsToEntries(actions, chatFile)
		// 1 user + 1 tool_use + 1 tool_result = 3
		if len(entries) != 3 {
			t.Fatalf("expected 3 entries, got %d", len(entries))
		}
		toolUse := entries[1]
		if toolUse.Message.Content[0].Name != "Bash" {
			t.Errorf("tool name = %q, want %q", toolUse.Message.Content[0].Name, "Bash")
		}
		inputMap, ok := toolUse.Message.Content[0].Input.(map[string]any)
		if !ok {
			t.Fatal("expected input to be map[string]any")
		}
		if inputMap["command"] != "go test ./..." {
			t.Errorf("command = %q, want %q", inputMap["command"], "go test ./...")
		}
		toolResult := entries[2]
		if toolResult.Message.Content[0].Content != "PASS\nok  mypackage 0.1s\n" {
			t.Errorf("result content = %q, want test output", toolResult.Message.Content[0].Content)
		}
		if toolResult.Message.Content[0].IsError {
			t.Error("expected IsError = false for exit code 0")
		}
	})

	t.Run("runCommand with non-zero exit code", func(t *testing.T) {
		actions := []KiroIDEAction{
			{
				ActionID: "cmd-2", ActionType: "runCommand", ActionState: "Error",
				Input:  map[string]any{"command": "go build ./..."},
				Output: map[string]any{"output": "build failed\n", "exitCode": float64(1)},
			},
		}
		entries, _ := convertKiroIDEActionsToEntries(actions, chatFile)
		toolResult := entries[2]
		if !toolResult.Message.Content[0].IsError {
			t.Error("expected IsError = true for exit code 1")
		}
		got := toolResult.Message.Content[0].Content
		if !strings.Contains(got, "build failed") {
			t.Errorf("result content = %q, want to contain %q", got, "build failed")
		}
		if !strings.Contains(got, "Exit code: 1") {
			t.Errorf("result content = %q, want to contain %q", got, "Exit code: 1")
		}
	})

	t.Run("replace action maps to Edit with file path result", func(t *testing.T) {
		actions := []KiroIDEAction{
			{
				ActionID: "edit-1", ActionType: "replace", ActionState: "Accepted",
				Input: map[string]any{"file": "main.go"},
			},
		}
		entries, _ := convertKiroIDEActionsToEntries(actions, chatFile)
		toolUse := entries[1]
		if toolUse.Message.Content[0].Name != "Edit" {
			t.Errorf("tool name = %q, want %q", toolUse.Message.Content[0].Name, "Edit")
		}
		toolResult := entries[2]
		if toolResult.Message.Content[0].Content != "main.go" {
			t.Errorf("result content = %q, want %q", toolResult.Message.Content[0].Content, "main.go")
		}
	})

	t.Run("create action maps to Write with file path result", func(t *testing.T) {
		actions := []KiroIDEAction{
			{
				ActionID: "write-1", ActionType: "create", ActionState: "Accepted",
				Input: map[string]any{"file": "new_file.go"},
			},
		}
		entries, _ := convertKiroIDEActionsToEntries(actions, chatFile)
		toolUse := entries[1]
		if toolUse.Message.Content[0].Name != "Write" {
			t.Errorf("tool name = %q, want %q", toolUse.Message.Content[0].Name, "Write")
		}
		toolResult := entries[2]
		if toolResult.Message.Content[0].Content != "new_file.go" {
			t.Errorf("result content = %q, want %q", toolResult.Message.Content[0].Content, "new_file.go")
		}
	})

	t.Run("append action maps to Edit with file path result", func(t *testing.T) {
		actions := []KiroIDEAction{
			{
				ActionID: "append-1", ActionType: "append", ActionState: "Accepted",
				Input: map[string]any{"file": "main.go"},
			},
		}
		entries, _ := convertKiroIDEActionsToEntries(actions, chatFile)
		toolUse := entries[1]
		if toolUse.Message.Content[0].Name != "Edit" {
			t.Errorf("tool name = %q, want %q", toolUse.Message.Content[0].Name, "Edit")
		}
		toolResult := entries[2]
		if toolResult.Message.Content[0].Content != "main.go" {
			t.Errorf("result content = %q, want %q", toolResult.Message.Content[0].Content, "main.go")
		}
	})

	t.Run("search action maps to Grep", func(t *testing.T) {
		actions := []KiroIDEAction{
			{
				ActionID: "search-1", ActionType: "search", ActionState: "Accepted",
				Input: map[string]any{"query": "func main", "why": "Find entry point"},
			},
		}
		entries, _ := convertKiroIDEActionsToEntries(actions, chatFile)
		toolUse := entries[1]
		if toolUse.Message.Content[0].Name != "Grep" {
			t.Errorf("tool name = %q, want %q", toolUse.Message.Content[0].Name, "Grep")
		}
		inputMap := toolUse.Message.Content[0].Input.(map[string]any)
		if inputMap["query"] != "func main" {
			t.Errorf("query = %q, want %q", inputMap["query"], "func main")
		}
		if inputMap["reason"] != "Find entry point" {
			t.Errorf("reason = %q, want %q", inputMap["reason"], "Find entry point")
		}
		toolResult := entries[2]
		if toolResult.Message.Content[0].Content != "func main" {
			t.Errorf("result content = %q, want %q", toolResult.Message.Content[0].Content, "func main")
		}
	})

	t.Run("taskStatus becomes assistant text", func(t *testing.T) {
		actions := []KiroIDEAction{
			{
				ActionID: "ts-1", ActionType: "taskStatus", ActionState: "Success",
				TaskID: "1.1 Setup", TaskStatus: "in_progress",
			},
		}
		entries, _ := convertKiroIDEActionsToEntries(actions, chatFile)
		// 1 user + 1 taskStatus text = 2
		if len(entries) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(entries))
		}
		if entries[1].Type != "assistant" {
			t.Errorf("type = %q, want %q", entries[1].Type, "assistant")
		}
		want := `Task "1.1 Setup": in_progress`
		if entries[1].Message.Content[0].Text != want {
			t.Errorf("text = %q, want %q", entries[1].Message.Content[0].Text, want)
		}
	})

	t.Run("rejected action shows error result", func(t *testing.T) {
		actions := []KiroIDEAction{
			{
				ActionID: "rej-1", ActionType: "runCommand", ActionState: "Rejected",
				Input: map[string]any{"command": "rm -rf /"},
			},
		}
		entries, _ := convertKiroIDEActionsToEntries(actions, chatFile)
		toolResult := entries[2]
		if !toolResult.Message.Content[0].IsError {
			t.Error("expected IsError = true for rejected action")
		}
		if toolResult.Message.Content[0].Content != "Action rejected by user" {
			t.Errorf("content = %q, want %q", toolResult.Message.Content[0].Content, "Action rejected by user")
		}
	})

	t.Run("error with errorMessage", func(t *testing.T) {
		actions := []KiroIDEAction{
			{
				ActionID: "err-1", ActionType: "replace", ActionState: "Error",
				ErrorMessage: "String 'foo' not found in main.go",
			},
		}
		entries, _ := convertKiroIDEActionsToEntries(actions, chatFile)
		toolResult := entries[2]
		if !toolResult.Message.Content[0].IsError {
			t.Error("expected IsError = true for error action")
		}
		if toolResult.Message.Content[0].Content != "String 'foo' not found in main.go" {
			t.Errorf("content = %q, want error message", toolResult.Message.Content[0].Content)
		}
	})

	t.Run("skips internal actions", func(t *testing.T) {
		actions := []KiroIDEAction{
			{ActionID: "m1", ActionType: "model", ActionState: "Success"},
			{ActionID: "s1", ActionType: "steering", ActionState: "Success"},
			{ActionID: "i1", ActionType: "intentClassification", ActionState: "Success"},
			{ActionID: "sp1", ActionType: "specAgent", ActionState: "Success"},
			{ActionID: "say1", ActionType: "say", ActionState: "Success", Output: map[string]any{"message": "Done"}},
		}
		entries, _ := convertKiroIDEActionsToEntries(actions, chatFile)
		// 1 user message + 1 say = 2 (skipping 4 internal actions)
		if len(entries) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(entries))
		}
	})

	t.Run("unknown action type produces warning", func(t *testing.T) {
		actions := []KiroIDEAction{
			{ActionID: "u1", ActionType: "newFeature", ActionState: "Success"},
		}
		_, warnings := convertKiroIDEActionsToEntries(actions, chatFile)
		if len(warnings) != 1 {
			t.Fatalf("expected 1 warning, got %d", len(warnings))
		}
		if !strings.Contains(warnings[0].Message, "newFeature") {
			t.Errorf("warning = %q, want to mention action type", warnings[0].Message)
		}
	})

	t.Run("userInput action becomes user entry", func(t *testing.T) {
		actions := []KiroIDEAction{
			{
				ActionID: "ui-1", ActionType: "userInput", ActionState: "Success",
				Output: map[string]any{
					"questions": []any{
						map[string]any{
							"id":       "ui-1",
							"question": "Ready to proceed?",
							"response": map[string]any{"type": "next-phase"},
						},
					},
				},
			},
		}
		entries, _ := convertKiroIDEActionsToEntries(actions, chatFile)
		// 1 user from chat + 1 userInput = 2
		if len(entries) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(entries))
		}
		got := entries[1].Message.Content[0].Text
		if !strings.Contains(got, "Ready to proceed?") {
			t.Errorf("text = %q, want to contain question", got)
		}
		if !strings.Contains(got, "[Response: next-phase]") {
			t.Errorf("text = %q, want to contain response type", got)
		}
	})

	t.Run("mixed action sequence", func(t *testing.T) {
		actions := []KiroIDEAction{
			{ActionID: "m1", ActionType: "model", ActionState: "Success"},
			{ActionID: "s1", ActionType: "say", ActionState: "Success", Output: map[string]any{"message": "Let me check the code."}},
			{ActionID: "r1", ActionType: "readFiles", ActionState: "Accepted", Input: map[string]any{"files": []any{map[string]any{"path": "main.go"}}}},
			{ActionID: "e1", ActionType: "replace", ActionState: "Accepted", Input: map[string]any{"file": "main.go"}},
			{ActionID: "c1", ActionType: "runCommand", ActionState: "Success", Input: map[string]any{"command": "go test"}, Output: map[string]any{"output": "PASS\n", "exitCode": float64(0)}},
			{ActionID: "s2", ActionType: "say", ActionState: "Success", Output: map[string]any{"message": "All done!"}},
		}
		entries, warnings := convertKiroIDEActionsToEntries(actions, chatFile)
		if len(warnings) != 0 {
			t.Errorf("expected 0 warnings, got %d", len(warnings))
		}
		// 1 user + 1 say + 2 readFiles + 2 replace + 2 runCommand + 1 say = 9
		if len(entries) != 9 {
			t.Fatalf("expected 9 entries, got %d", len(entries))
		}
		// Verify ordering: user, say, read(use), read(result), edit(use), edit(result), bash(use), bash(result), say
		wantTypes := []string{"user", "assistant", "assistant", "user", "assistant", "user", "assistant", "user", "assistant"}
		for i, wt := range wantTypes {
			if entries[i].Type != wt {
				t.Errorf("entry %d: type = %q, want %q", i, entries[i].Type, wt)
			}
		}
	})
}

func TestParseKiroIDEWithActionsFromCostPath(t *testing.T) {
	dir := t.TempDir()
	costPath := filepath.Join(dir, "detail.json")
	err := os.WriteFile(costPath, []byte(`{
		"executionId": "exec-1",
		"usageSummary": [
			{"unit": "credit", "unitPlural": "credits", "usage": 0.05}
		],
		"actions": [
			{
				"type": "AgentExecutionAction",
				"executionId": "exec-1",
				"actionId": "s1",
				"actionType": "say",
				"actionState": "Success",
				"output": {"message": "I'll help with that."}
			},
			{
				"type": "AgentExecutionAction",
				"executionId": "exec-1",
				"actionId": "r1",
				"actionType": "readFiles",
				"actionState": "Accepted",
				"input": {"files": [{"path": "main.go"}]}
			},
			{
				"type": "AgentExecutionAction",
				"executionId": "exec-1",
				"actionId": "c1",
				"actionType": "runCommand",
				"actionState": "Success",
				"input": {"command": "go test ./..."},
				"output": {"output": "PASS\n", "exitCode": 0}
			}
		]
	}`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	input := `{
		"executionId": "exec-1",
		"chat": [
			{"role": "human", "content": "<identity>system</identity>"},
			{"role": "human", "content": "Fix the tests"},
			{"role": "bot", "content": "I'll help with that."},
			{"role": "tool", "content": ""}
		],
		"metadata": {"modelId": "auto", "startTime": 1000, "endTime": 2000}
	}`

	result, err := ParseKiroIDEWithCostPath(strings.NewReader(input), costPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should use action-based entries, not chat-based
	// 1 user ("Fix the tests") + 1 say + 2 readFiles + 2 runCommand = 6
	if len(result.Entries) != 6 {
		t.Fatalf("expected 6 entries, got %d", len(result.Entries))
	}

	// First entry should be user message from chat
	if result.Entries[0].Message.Content[0].Text != "Fix the tests" {
		t.Errorf("first entry text = %q, want %q", result.Entries[0].Message.Content[0].Text, "Fix the tests")
	}

	// Second entry should be say action (assistant text)
	if result.Entries[1].Type != "assistant" {
		t.Errorf("entry 1 type = %q, want %q", result.Entries[1].Type, "assistant")
	}

	// Third entry should be readFiles tool_use
	if result.Entries[2].Message.Content[0].Name != "Read" {
		t.Errorf("entry 2 tool name = %q, want %q", result.Entries[2].Message.Content[0].Name, "Read")
	}

	// Cost metadata should still be present
	if result.Metadata == nil || result.Metadata.TotalCost == nil {
		t.Fatal("expected cost metadata")
	}
	if math.Abs(*result.Metadata.TotalCost-0.05) > 0.0001 {
		t.Errorf("cost = %.4f, want 0.05", *result.Metadata.TotalCost)
	}
}

func TestParseKiroIDEFallsBackToChatWhenNoActions(t *testing.T) {
	dir := t.TempDir()
	costPath := filepath.Join(dir, "detail.json")
	err := os.WriteFile(costPath, []byte(`{
		"executionId": "exec-1",
		"usageSummary": [
			{"unit": "credit", "unitPlural": "credits", "usage": 0.05}
		],
		"actions": []
	}`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	input := `{
		"executionId": "exec-1",
		"chat": [
			{"role": "human", "content": "Hello"},
			{"role": "bot", "content": "Hi there"}
		],
		"metadata": {"modelId": "auto", "startTime": 1000, "endTime": 2000}
	}`

	result, err := ParseKiroIDEWithCostPath(strings.NewReader(input), costPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should fall back to chat-based entries
	if len(result.Entries) != 2 {
		t.Fatalf("expected 2 entries (chat fallback), got %d", len(result.Entries))
	}
	if result.Entries[0].Type != "user" {
		t.Errorf("entry 0 type = %q, want %q", result.Entries[0].Type, "user")
	}
	if result.Entries[1].Type != "assistant" {
		t.Errorf("entry 1 type = %q, want %q", result.Entries[1].Type, "assistant")
	}
}
