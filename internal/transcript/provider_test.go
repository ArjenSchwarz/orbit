package transcript

import (
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderProvider_AssistantHeaders(t *testing.T) {
	for _, tc := range []struct {
		name, model, provider, label string
	}{
		{"model and provider", "example-model", "example-provider", "example-model (example-provider)"},
		{"provider only", "", "example-provider", "(example-provider)"},
		{"model only", "example-model", "", "example-model"},
		{"neither", "", "", ""},
		{"escaped HTML", "<model>", "<provider>&", "<model> (<provider>&)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entries := []Entry{{
				Type: "assistant", Model: tc.model, Provider: tc.provider,
				Message: &Message{Role: "assistant", Content: []ContentItem{{Type: "text", Text: "Hello"}}},
			}}
			header := "## 🤖 Assistant"
			if tc.label != "" {
				header += " · " + tc.label
			}
			assert.Contains(t, RenderMarkdown(entries, RenderOptions{}), header+"\n")
			for _, render := range []func([]Entry, RenderOptions) string{RenderHTML, RenderHTMLFragment} {
				output := render(entries, RenderOptions{})
				if tc.label == "" {
					assert.NotContains(t, output, `class="message-meta"`)
				} else {
					assert.Contains(t, output, "<span>"+html.EscapeString(tc.label)+"</span>")
				}
			}
		})
	}
}

func TestRenderProvider_ExistingMetadataFreeHeadersUnchanged(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries []Entry
	}{
		{"user", []Entry{{Type: "user", Message: &Message{Role: "user", Content: []ContentItem{{Type: "text", Text: "Hello"}}}}}},
		{"read group", []Entry{makeReadEntry("read_1", "/work/a.go", ""), makeReadEntry("read_2", "/work/b.go", ""), makeToolResultEntry("read_1", "first"), makeToolResultEntry("read_2", "second")}},
		{"edit group", []Entry{makeEditEntry("edit_1", "/work/a.go", ""), makeEditEntry("edit_2", "/work/b.go", ""), makeToolResultEntry("edit_1", "first"), makeToolResultEntry("edit_2", "second")}},
		{"deferred tool result", []Entry{{Type: "assistant", Message: &Message{Role: "assistant", Content: []ContentItem{{Type: "tool_use", ID: "shell_1", Name: "Bash", Input: map[string]any{"command": "echo example"}}}}}, makeToolResultEntry("shell_1", "example")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, render := range []func([]Entry, RenderOptions) string{RenderMarkdown, RenderHTML, RenderHTMLFragment} {
				withoutProvider := render(tc.entries, RenderOptions{})
				entries := append([]Entry(nil), tc.entries...)
				for i := range entries {
					entries[i].Provider = "hidden-provider"
				}
				withProvider := render(entries, RenderOptions{})
				assert.Equal(t, withoutProvider, withProvider)
				assert.NotContains(t, withProvider, "hidden-provider")
			}
		})
	}
}

// Synthetic Kiro IDE chats with metadata.modelId, with and without metadata.modelProvider (Q93).
func TestKiroIDEProvider(t *testing.T) {
	chat := func(metadata string) string {
		return `{"executionId":"example-execution","metadata":` + metadata + `,"chat":[{"role":"human","content":"Hello"},{"role":"bot","content":"Hi"}]}`
	}
	withProvider := chat(`{"modelId":"example-model","modelProvider":"example-provider"}`)
	modelOnly := chat(`{"modelId":"example-model"}`)

	t.Run("chat entries", func(t *testing.T) {
		result, err := ParseKiroIDE(strings.NewReader(withProvider))
		require.NoError(t, err)
		require.NotNil(t, result.Metadata)
		assert.Equal(t, "example-provider", result.Metadata.FirstProvider)
		require.Len(t, result.Entries, 2)
		assert.Empty(t, result.Entries[0].Provider)
		assert.Equal(t, "example-model", result.Entries[1].Model)
		assert.Equal(t, "example-provider", result.Entries[1].Provider)

		assert.Contains(t, RenderMarkdown(result.Entries, RenderOptions{}), "## 🤖 Assistant · example-model (example-provider)\n")
		for _, render := range []func([]Entry, RenderOptions) string{RenderHTML, RenderHTMLFragment} {
			assert.Contains(t, render(result.Entries, RenderOptions{}), "<span>example-model (example-provider)</span>")
		}
	})

	t.Run("execution action entries", func(t *testing.T) {
		costPath := filepath.Join(t.TempDir(), "execution.json")
		actions := `{"actions":[{"actionId":"action-1","actionType":"say","output":{"message":"Starting"}},{"actionId":"action-2","actionType":"runCommand","input":{"command":"ls"},"output":{"output":"ok"}}]}`
		require.NoError(t, os.WriteFile(costPath, []byte(actions), 0o600))
		result, err := ParseKiroIDEWithCostPath(strings.NewReader(withProvider), costPath)
		require.NoError(t, err)
		assert.Equal(t, "example-provider", result.Metadata.FirstProvider)
		for _, entry := range result.Entries {
			if entry.Model != "" {
				assert.Equal(t, "example-provider", entry.Provider, "entries with the model carry its provider")
			} else {
				assert.Empty(t, entry.Provider)
			}
		}
		assert.Equal(t, []string{"", "example-provider", "example-provider", ""}, entryProviders(result.Entries))
	})

	t.Run("without modelProvider", func(t *testing.T) {
		result, err := ParseKiroIDE(strings.NewReader(modelOnly))
		require.NoError(t, err)
		assert.Empty(t, result.Metadata.FirstProvider)
		assert.Equal(t, []string{"", ""}, entryProviders(result.Entries))

		withProviderResult, err := ParseKiroIDE(strings.NewReader(withProvider))
		require.NoError(t, err)
		cleared := append([]Entry(nil), withProviderResult.Entries...)
		for i := range cleared {
			cleared[i].Provider = ""
		}
		for _, render := range []func([]Entry, RenderOptions) string{RenderMarkdown, RenderHTML, RenderHTMLFragment} {
			assert.Equal(t, render(cleared, RenderOptions{}), render(result.Entries, RenderOptions{}))
		}
		assert.Contains(t, RenderMarkdown(result.Entries, RenderOptions{}), "## 🤖 Assistant · example-model\n")
	})
}

func entryProviders(entries []Entry) []string {
	providers := make([]string, len(entries))
	for i, entry := range entries {
		providers[i] = entry.Provider
	}
	return providers
}
