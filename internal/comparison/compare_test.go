package comparison

import (
	"context"
	"testing"
	"time"
)

func TestBuildPrompt_IncludesAllVariants(t *testing.T) {
	variants := []VariantData{
		{ID: 1, Diff: "diff for variant 1"},
		{ID: 2, Diff: "diff for variant 2"},
		{ID: 3, Diff: "diff for variant 3"},
	}

	prompt := buildPrompt("test-spec", variants)

	// Check that all variants are mentioned
	for _, v := range variants {
		if !containsString(prompt, v.Diff) {
			t.Errorf("prompt should contain diff for variant %d", v.ID)
		}
	}

	// Check header mentions correct count
	if !containsString(prompt, "3 implementation variants") {
		t.Error("prompt should mention 3 implementation variants")
	}

	// Check spec name is included
	if !containsString(prompt, "test-spec") {
		t.Error("prompt should include spec name")
	}
}

func TestBuildPrompt_IncludesMetrics(t *testing.T) {
	variants := []VariantData{
		{
			ID:   1,
			Diff: "diff 1",
			Metrics: VariantMetrics{
				Cost:     0.0523,
				Duration: 3*time.Minute + 30*time.Second,
				NumTurns: 42,
			},
		},
		{
			ID:   2,
			Diff: "diff 2",
			Metrics: VariantMetrics{
				Cost:     0.0812,
				Duration: 5*time.Minute + 15*time.Second,
				NumTurns: 58,
			},
		},
	}

	prompt := buildPrompt("test-spec", variants)

	// Check metrics table is present
	if !containsString(prompt, "## Metrics") {
		t.Error("prompt should contain metrics section")
	}

	// Check cost formatting
	if !containsString(prompt, "$0.0523") {
		t.Error("prompt should contain formatted cost for variant 1")
	}
	if !containsString(prompt, "$0.0812") {
		t.Error("prompt should contain formatted cost for variant 2")
	}

	// Check turn counts
	if !containsString(prompt, "42") {
		t.Error("prompt should contain turn count for variant 1")
	}
	if !containsString(prompt, "58") {
		t.Error("prompt should contain turn count for variant 2")
	}
}

func TestParseAndValidate_ValidJSON(t *testing.T) {
	comp := NewComparator(nil, "")

	validJSON := `{
		"recommendation": 1,
		"confidence": "high",
		"summary": "Variant 1 is the best choice.",
		"file_analyses": [
			{
				"path": "main.go",
				"variants": {"1": "clean", "2": "messy"},
				"preference": 1
			}
		],
		"observations": ["Variant 1 has cleaner code"]
	}`

	result, err := comp.parseAndValidate(validJSON, 2)
	if err != nil {
		t.Fatalf("parseAndValidate failed: %v", err)
	}

	if result.Recommendation != 1 {
		t.Errorf("expected recommendation 1, got %d", result.Recommendation)
	}
	if result.Confidence != "high" {
		t.Errorf("expected confidence 'high', got %s", result.Confidence)
	}
	if result.Summary != "Variant 1 is the best choice." {
		t.Errorf("unexpected summary: %s", result.Summary)
	}
	if len(result.FileAnalyses) != 1 {
		t.Errorf("expected 1 file analysis, got %d", len(result.FileAnalyses))
	}
	if len(result.Observations) != 1 {
		t.Errorf("expected 1 observation, got %d", len(result.Observations))
	}
}

func TestParseAndValidate_MissingFields(t *testing.T) {
	comp := NewComparator(nil, "")

	tests := []struct {
		name    string
		json    string
		wantErr string
	}{
		{
			name:    "missing confidence",
			json:    `{"recommendation": 1, "summary": "test"}`,
			wantErr: "missing required field: confidence",
		},
		{
			name:    "missing summary",
			json:    `{"recommendation": 1, "confidence": "high"}`,
			wantErr: "missing required field: summary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := comp.parseAndValidate(tt.json, 2)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !containsString(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestParseAndValidate_InvalidConfidence(t *testing.T) {
	comp := NewComparator(nil, "")

	json := `{
		"recommendation": 1,
		"confidence": "very_high",
		"summary": "test summary"
	}`

	_, err := comp.parseAndValidate(json, 2)
	if err == nil {
		t.Fatal("expected error for invalid confidence, got nil")
	}
	if !containsString(err.Error(), "invalid confidence value") {
		t.Errorf("expected error about invalid confidence, got: %v", err)
	}
}

func TestParseAndValidate_RecommendationOutOfRange(t *testing.T) {
	comp := NewComparator(nil, "")

	tests := []struct {
		name           string
		recommendation int
		numVariants    int
	}{
		{"recommendation too low", 0, 2},
		{"recommendation too high", 3, 2},
		{"negative recommendation", -1, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			json := `{
				"recommendation": ` + itoa(tt.recommendation) + `,
				"confidence": "high",
				"summary": "test"
			}`

			_, err := comp.parseAndValidate(json, tt.numVariants)
			if err == nil {
				t.Fatal("expected error for out-of-range recommendation, got nil")
			}
			if !containsString(err.Error(), "recommendation must be between") {
				t.Errorf("expected range error, got: %v", err)
			}
		})
	}
}

func TestExtractJSON_PlainJSON(t *testing.T) {
	input := `{"recommendation": 1, "confidence": "high", "summary": "test"}`

	result, err := extractJSON(input)
	if err != nil {
		t.Fatalf("extractJSON failed: %v", err)
	}
	if result != input {
		t.Errorf("expected %q, got %q", input, result)
	}
}

func TestExtractJSON_MarkdownCodeBlock(t *testing.T) {
	input := "Here is my analysis:\n```json\n{\"recommendation\": 1}\n```\nThat's it."

	result, err := extractJSON(input)
	if err != nil {
		t.Fatalf("extractJSON failed: %v", err)
	}
	if result != `{"recommendation": 1}` {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestExtractJSON_PlainCodeBlock(t *testing.T) {
	input := "Here is my analysis:\n```\n{\"recommendation\": 2}\n```"

	result, err := extractJSON(input)
	if err != nil {
		t.Fatalf("extractJSON failed: %v", err)
	}
	if result != `{"recommendation": 2}` {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestExtractJSON_JSONInText(t *testing.T) {
	input := `Here is the comparison:
{
  "recommendation": 1,
  "confidence": "high"
}
That's my analysis.`

	result, err := extractJSON(input)
	if err != nil {
		t.Fatalf("extractJSON failed: %v", err)
	}

	// Should extract the JSON object
	if !containsString(result, "recommendation") {
		t.Errorf("expected JSON to contain recommendation: %q", result)
	}
}

func TestExtractJSON_NoJSON(t *testing.T) {
	input := "This response has no JSON at all."

	_, err := extractJSON(input)
	if err == nil {
		t.Fatal("expected error for no JSON, got nil")
	}
}

func TestExtractJSON_NestedObjects(t *testing.T) {
	input := `{
		"recommendation": 1,
		"nested": {
			"inner": {
				"value": "test"
			}
		}
	}`

	result, err := extractJSON(input)
	if err != nil {
		t.Fatalf("extractJSON failed: %v", err)
	}
	if !containsString(result, "nested") {
		t.Errorf("expected nested object to be preserved")
	}
}

func TestExtractJSON_StringWithBraces(t *testing.T) {
	input := `{"text": "a string with {braces} inside"}`

	result, err := extractJSON(input)
	if err != nil {
		t.Fatalf("extractJSON failed: %v", err)
	}
	if result != input {
		t.Errorf("expected %q, got %q", input, result)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{30 * time.Second, "30s"},
		{time.Minute, "1m 0s"},
		{3*time.Minute + 45*time.Second, "3m 45s"},
		{10*time.Minute + 5*time.Second, "10m 5s"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatDuration(tt.duration)
			if result != tt.expected {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, result, tt.expected)
			}
		})
	}
}

func TestEstimatePromptTokens(t *testing.T) {
	// Simple heuristic test: ~4 chars per token
	prompt := "This is a test prompt with about 40 chars."
	tokens := estimatePromptTokens(prompt)

	// Should be roughly len/4
	expected := len(prompt) / 4
	if tokens != expected {
		t.Errorf("estimatePromptTokens = %d, want %d", tokens, expected)
	}
}

func TestCompare_RejectsOversizedPrompt(t *testing.T) {
	comp := NewComparator(nil, "")

	// Create variants with diffs large enough to exceed the token limit
	// MaxPromptTokens is 150000, which at ~4 chars/token is ~600000 chars
	largeDiff := make([]byte, 400000) // 400k chars = ~100k tokens each
	for i := range largeDiff {
		largeDiff[i] = 'x'
	}

	variants := []VariantData{
		{ID: 1, Diff: string(largeDiff)},
		{ID: 2, Diff: string(largeDiff)},
	}

	ctx := context.Background()
	_, err := comp.Compare(ctx, "test-spec", variants)
	if err == nil {
		t.Fatal("expected error for oversized prompt, got nil")
	}

	if !containsString(err.Error(), "exceeds context limit") {
		t.Errorf("expected context limit error, got: %v", err)
	}
}

// Helper functions

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
