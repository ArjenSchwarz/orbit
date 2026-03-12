package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Integration tests: parse sample transcripts → render to Markdown and HTML → verify metadata.

func TestIntegration_ClaudeMetadata(t *testing.T) {
	// Claude transcripts have timestamps but no model (req 2.4).
	entries := parseFixture(t, "integration/claude.jsonl", FormatClaude)

	md := RenderMarkdown(entries, RenderOptions{})
	html := RenderHTML(entries, RenderOptions{})

	// Timestamps must appear in both outputs (date may shift due to timezone conversion).
	assertContains(t, md, " · ", "markdown should contain metadata separator (timestamp)")
	assertContains(t, html, "<time datetime=", "html should contain <time> element")

	// Model must NOT appear (req 2.4 — Claude transcripts don't include model info).
	for _, line := range strings.Split(md, "\n") {
		if strings.HasPrefix(line, "## 🤖 Assistant") {
			parts := strings.Split(line, " · ")
			// Should be at most 2 parts: header and timestamp, never 3 (which would mean model).
			if len(parts) > 2 {
				t.Errorf("assistant header has too many metadata parts (model leaked?): %s", line)
			}
		}
	}
}

func TestIntegration_CodexMetadata(t *testing.T) {
	// Codex transcripts have timestamps and session-level model.
	entries := parseFixture(t, "integration/codex.jsonl", FormatCodex)

	md := RenderMarkdown(entries, RenderOptions{})
	html := RenderHTML(entries, RenderOptions{})

	// Timestamps on both user and assistant messages.
	assertContains(t, md, " · ", "markdown should contain metadata separator (timestamp)")
	assertContains(t, html, "<time datetime=", "html should contain <time> element")

	// Model on assistant messages only.
	assertContains(t, md, "o3-mini", "markdown should contain model")
	assertContains(t, html, "o3-mini", "html should contain model")

	// User messages should not show model.
	for _, line := range strings.Split(md, "\n") {
		if strings.HasPrefix(line, "## 👤 User") && strings.Contains(line, "o3-mini") {
			t.Error("user message header should not contain model")
		}
	}
}

func TestIntegration_CopilotMetadata(t *testing.T) {
	// Copilot transcripts have timestamps and model from session.model_change.
	entries := parseFixture(t, "integration/copilot.jsonl", FormatCopilot)

	md := RenderMarkdown(entries, RenderOptions{})
	html := RenderHTML(entries, RenderOptions{})

	// Timestamps present.
	assertContains(t, md, " · ", "markdown should contain metadata separator (timestamp)")
	assertContains(t, html, "<time datetime=", "html should contain <time> element")

	// Model on assistant messages.
	assertContains(t, md, "gpt-4o", "markdown should contain model")
	assertContains(t, html, "gpt-4o", "html should contain model")
}

func TestIntegration_KiroCLIMetadata(t *testing.T) {
	// Kiro CLI transcripts have per-message timestamps and model.
	f, err := os.Open(filepath.Join("testdata", "integration", "kiro.json"))
	if err != nil {
		t.Fatalf("failed to open fixture: %v", err)
	}
	defer func() { _ = f.Close() }()

	result, err := ParseKiro(f)
	if err != nil {
		t.Fatalf("ParseKiro error: %v", err)
	}

	md := RenderMarkdown(result.Entries, RenderOptions{})
	html := RenderHTML(result.Entries, RenderOptions{})

	// Timestamps present.
	assertContains(t, md, " · ", "markdown should contain metadata separator (timestamp)")
	assertContains(t, html, "<time datetime=", "html should contain <time> element")

	// Model on assistant messages.
	assertContains(t, md, "claude-sonnet-4", "markdown should contain model")
	assertContains(t, html, "claude-sonnet-4", "html should contain model")
}

func TestIntegration_KiroIDEMetadata(t *testing.T) {
	// Kiro IDE: session-level timestamp on first message only (req 1.7),
	// model on all assistant messages.
	f, err := os.Open(filepath.Join("testdata", "integration", "kiro_ide.json"))
	if err != nil {
		t.Fatalf("failed to open fixture: %v", err)
	}
	defer func() { _ = f.Close() }()

	result, err := ParseKiroIDE(f)
	if err != nil {
		t.Fatalf("ParseKiroIDE error: %v", err)
	}

	if len(result.Entries) < 4 {
		t.Fatalf("expected at least 4 entries, got %d", len(result.Entries))
	}

	// First entry should have a timestamp.
	if result.Entries[0].Timestamp == "" {
		t.Error("first entry should have a timestamp (session start time)")
	}

	// Subsequent entries should NOT have timestamps (session-level only).
	for i := 1; i < len(result.Entries); i++ {
		if result.Entries[i].Timestamp != "" {
			t.Errorf("entry %d should not have a timestamp (Kiro IDE is session-level only)", i)
		}
	}

	// All assistant entries should have model.
	for i, e := range result.Entries {
		if e.Type == "assistant" && e.Model == "" {
			t.Errorf("assistant entry %d should have model", i)
		}
	}

	md := RenderMarkdown(result.Entries, RenderOptions{})
	html := RenderHTML(result.Entries, RenderOptions{})

	// Model appears in output.
	assertContains(t, md, "claude-sonnet-4", "markdown should contain model")
	assertContains(t, html, "claude-sonnet-4", "html should contain model")
}

func TestIntegration_SessionMetadataPreserved(t *testing.T) {
	// Verify session-level metadata (cost, session ID) still renders at top (req 4.2).
	// Session metadata is passed via RenderOptions, not parsed from entries.
	entries := parseFixture(t, "integration/claude.jsonl", FormatClaude)

	cost := 0.42
	opts := RenderOptions{
		SessionID: "sess-abc-123",
		TotalCost: &cost,
	}

	md := RenderMarkdown(entries, opts)
	html := RenderHTML(entries, opts)

	// Session metadata should appear in output.
	assertContains(t, md, "0.42", "markdown should contain cost value")
	assertContains(t, md, "sess-abc-123", "markdown should contain session ID")
	assertContains(t, html, "0.42", "html should contain cost value")
	assertContains(t, html, "sess-abc-123", "html should contain session ID")

	// Message-level timestamps should also be present.
	assertContains(t, md, " · ", "markdown should contain message timestamp separator")
}

func TestIntegration_HTMLTimeElements(t *testing.T) {
	// Verify HTML output contains proper <time datetime="..."> elements with valid attributes.
	entries := parseFixture(t, "integration/claude.jsonl", FormatClaude)
	html := RenderHTML(entries, RenderOptions{})

	// Should have <time> elements with datetime attributes.
	assertContains(t, html, `<time datetime="`, "html should contain <time> with datetime attribute")

	// The datetime attribute should contain a valid ISO 8601 value (UTC).
	assertContains(t, html, `datetime="2026-03-`, "datetime should contain ISO 8601 timestamp")

	// Fallback text should be present inside the <time> element.
	assertContains(t, html, `</time>`, "html should have closing </time> tag")
}

func TestIntegration_StandaloneHTMLScript(t *testing.T) {
	// Standalone HTML (RenderHTML) should include the JS for locale formatting.
	entries := parseFixture(t, "integration/claude.jsonl", FormatClaude)
	html := RenderHTML(entries, RenderOptions{})

	assertContains(t, html, "Intl.DateTimeFormat", "standalone HTML should include locale formatting script")

	// Fragment (RenderHTMLFragment) should NOT include the standalone script.
	fragment := RenderHTMLFragment(entries, RenderOptions{})
	assertNotContains(t, fragment, "Intl.DateTimeFormat", "HTML fragment should not include standalone script")
}

// --- helpers ---

func parseFixture(t *testing.T, path string, format Format) []Entry {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", path))
	if err != nil {
		t.Fatalf("failed to open fixture %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()

	var result *ParseResult
	switch format {
	case FormatCopilot:
		result, err = ParseCopilot(f)
	default:
		result, err = ParseJSONLWithFormat(f, format)
	}
	if err != nil {
		t.Fatalf("parse error for %s: %v", path, err)
	}
	return result.Entries
}

func assertContains(t *testing.T, s, substr, msg string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("%s: output does not contain %q", msg, substr)
	}
}

func assertNotContains(t *testing.T, s, substr, msg string) {
	t.Helper()
	if strings.Contains(s, substr) {
		t.Errorf("%s: output unexpectedly contains %q", msg, substr)
	}
}
