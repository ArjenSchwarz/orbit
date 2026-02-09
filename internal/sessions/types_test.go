package sessions

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

func TestAllSources(t *testing.T) {
	sources := AllSources()

	if len(sources) != 5 {
		t.Fatalf("expected 5 sources, got %d", len(sources))
	}

	// Verify all expected sources are present
	expected := []string{SourceClaude, SourceCodex, SourceCopilot, SourceKiroCLI, SourceKiroIDE}
	for i, want := range expected {
		if sources[i] != want {
			t.Errorf("sources[%d] = %q, want %q", i, sources[i], want)
		}
	}

	// Verify returned slice is a copy (modifying it doesn't affect originals)
	sources[0] = "modified"
	if AllSources()[0] == "modified" {
		t.Error("AllSources should return a copy, not the original slice")
	}
}

func TestDisplayName(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{SourceClaude, "claude"},
		{SourceCodex, "codex"},
		{SourceCopilot, "copilot"},
		{SourceKiroCLI, "kiro-cli"},
		{SourceKiroIDE, "kiro ide"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		got := DisplayName(tt.source)
		if got != tt.want {
			t.Errorf("DisplayName(%q) = %q, want %q", tt.source, got, tt.want)
		}
	}
}

func TestIsValidSource(t *testing.T) {
	for _, source := range AllSources() {
		if !IsValidSource(source) {
			t.Errorf("IsValidSource(%q) = false, want true", source)
		}
	}

	invalid := []string{"", "unknown", "Claude", "CLAUDE", "kiro ide", "kiro"}
	for _, source := range invalid {
		if IsValidSource(source) {
			t.Errorf("IsValidSource(%q) = true, want false", source)
		}
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{1, "1 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		got := FormatSize(tt.bytes)
		if got != tt.want {
			t.Errorf("FormatSize(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}

func TestFormatSizeProperty(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		bytes := rapid.Int64Range(0, 1<<50).Draw(rt, "bytes")
		result := FormatSize(bytes)

		// Output is never empty
		if result == "" {
			rt.Fatalf("FormatSize(%d) returned empty string", bytes)
		}

		// Output always contains a size unit
		hasUnit := strings.HasSuffix(result, " B") ||
			strings.HasSuffix(result, "KB") ||
			strings.HasSuffix(result, "MB") ||
			strings.HasSuffix(result, "GB") ||
			strings.HasSuffix(result, "TB") ||
			strings.HasSuffix(result, "PB") ||
			strings.HasSuffix(result, "EB")
		if !hasUnit {
			rt.Fatalf("FormatSize(%d) = %q, missing size unit", bytes, result)
		}
	})
}
