package comparison

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
)

// Tests for LoadResultFromFile

func TestLoadResultFromFile_ValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/comparison.json"

	content := `{
		"recommendation": 2,
		"confidence": "high",
		"summary": "Variant 2 is better.",
		"file_analyses": [{"path": "main.go", "variants": {"1": "ok", "2": "great"}, "preference": 2}],
		"observations": ["Variant 2 has cleaner code"],
		"cross_variant_improvements": [{"source_variant_id": 1, "description": "Better tests", "rationale": "More coverage", "priority": "medium"}]
	}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := LoadResultFromFile(path)
	if err != nil {
		t.Fatalf("LoadResultFromFile failed: %v", err)
	}

	if result.Recommendation != 2 {
		t.Errorf("expected recommendation 2, got %d", result.Recommendation)
	}
	if result.Confidence != "high" {
		t.Errorf("expected confidence 'high', got %q", result.Confidence)
	}
	if result.Summary != "Variant 2 is better." {
		t.Errorf("unexpected summary: %q", result.Summary)
	}
	if len(result.FileAnalyses) != 1 {
		t.Errorf("expected 1 file analysis, got %d", len(result.FileAnalyses))
	}
	if len(result.CrossVariantImprovements) != 1 {
		t.Errorf("expected 1 cross-variant improvement, got %d", len(result.CrossVariantImprovements))
	}
}

func TestLoadResultFromFile_MarkdownWrappedJSON(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/comparison.json"

	content := "```json\n" + `{"recommendation": 1, "confidence": "medium", "summary": "Variant 1 wins.", "file_analyses": [], "observations": []}` + "\n```\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := LoadResultFromFile(path)
	if err != nil {
		t.Fatalf("LoadResultFromFile failed: %v", err)
	}

	if result.Recommendation != 1 {
		t.Errorf("expected recommendation 1, got %d", result.Recommendation)
	}
}

func TestLoadResultFromFile_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/comparison.json"

	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadResultFromFile(path)
	if err == nil {
		t.Fatal("expected error for empty file")
	}
	if !containsString(err.Error(), "empty") {
		t.Errorf("expected error about empty file, got: %v", err)
	}
}

func TestLoadResultFromFile_FileNotFound(t *testing.T) {
	_, err := LoadResultFromFile("/nonexistent/path/comparison.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadResultFromFile_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/comparison.json"

	if err := os.WriteFile(path, []byte("not json at all"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadResultFromFile(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadResultFromFile_MissingRequiredFields(t *testing.T) {
	tests := map[string]struct {
		json    string
		wantErr string
	}{
		"missing recommendation": {
			json:    `{"recommendation": 0, "confidence": "high", "summary": "test"}`,
			wantErr: "missing recommendation",
		},
		"missing confidence": {
			json:    `{"recommendation": 1, "confidence": "", "summary": "test"}`,
			wantErr: "missing confidence",
		},
		"missing summary": {
			json:    `{"recommendation": 1, "confidence": "high", "summary": ""}`,
			wantErr: "missing summary",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := dir + "/comparison.json"

			if err := os.WriteFile(path, []byte(tc.json), 0o644); err != nil {
				t.Fatal(err)
			}

			_, err := LoadResultFromFile(path)
			if err == nil {
				t.Fatal("expected error for missing required field")
			}
			if !containsString(err.Error(), tc.wantErr) {
				t.Errorf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestLoadResultFromFile_WithLearnings(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/comparison.json"

	content := `{
		"recommendation": 1,
		"confidence": "high",
		"summary": "Test with learnings.",
		"file_analyses": [],
		"observations": [],
		"learnings": [
			{
				"variant_id": 1,
				"category": "code-pattern",
				"title": "Table-driven tests",
				"description": "Uses map for test cases",
				"rationale": "Unique names",
				"file_references": ["test.go:42"]
			}
		]
	}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := LoadResultFromFile(path)
	if err != nil {
		t.Fatalf("LoadResultFromFile failed: %v", err)
	}

	if len(result.Learnings) != 1 {
		t.Errorf("expected 1 learning, got %d", len(result.Learnings))
	}
}

func TestLoadResultFromFile_MalformedLearningsTolerated(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/comparison.json"

	// Learnings with variant_id as string instead of int — should be tolerated
	content := `{
		"recommendation": 1,
		"confidence": "high",
		"summary": "Test tolerant parsing.",
		"file_analyses": [],
		"observations": ["obs1"],
		"learnings": [
			{
				"variant_id": "1",
				"category": "code-pattern",
				"title": "Good pattern",
				"rationale": "Why it matters",
				"file_references": ["file.go:10"]
			}
		]
	}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := LoadResultFromFile(path)
	if err != nil {
		t.Fatalf("LoadResultFromFile should not fail on malformed learnings: %v", err)
	}

	// Core fields should be intact
	if result.Recommendation != 1 {
		t.Errorf("expected recommendation 1, got %d", result.Recommendation)
	}
	if result.Confidence != "high" {
		t.Errorf("expected confidence 'high', got %q", result.Confidence)
	}
	if len(result.Observations) != 1 {
		t.Errorf("expected 1 observation, got %d", len(result.Observations))
	}
	// Malformed learnings are discarded gracefully
}

func TestBuildComparisonPrompt_IncludesMetrics(t *testing.T) {
	input := ComparisonInput{
		SpecName: "test-spec",
		Variants: []VariantData{
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
		},
		IncludeDiff: true,
	}

	prompt := buildComparisonPrompt(input)

	// Check metrics table is present
	if !containsString(prompt, "## Metrics") {
		t.Error("prompt should contain metrics section")
	}

	// Check duration formatting (cost is intentionally excluded from comparison)
	if !containsString(prompt, "3m 30s") {
		t.Error("prompt should contain formatted duration for variant 1")
	}
	if !containsString(prompt, "5m 15s") {
		t.Error("prompt should contain formatted duration for variant 2")
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

func TestCompareUnified_RejectsOversizedPrompt(t *testing.T) {
	comp := NewComparator(nil, "")

	// Create variants with commit messages large enough to exceed the token limit
	// even without diffs. MaxPromptTokens is 150000, ~4 chars/token = ~600000 chars.
	largeMsg := make([]byte, 400000)
	for i := range largeMsg {
		largeMsg[i] = 'x'
	}

	ctx := context.Background()
	input := ComparisonInput{
		SpecName: "test-spec",
		Variants: []VariantData{
			{ID: 1, CommitMessages: []string{string(largeMsg)}},
			{ID: 2, CommitMessages: []string{string(largeMsg)}},
		},
		IncludeDiff: false,
	}

	_, err := comp.CompareUnified(ctx, input)
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

func TestValidateLearnings(t *testing.T) {
	tests := map[string]struct {
		input       []VariantLearning
		numVariants int
		wantCount   int
		wantNil     bool
	}{
		"valid learning": {
			input: []VariantLearning{{
				VariantID:      1,
				Category:       LearningCategoryCodePattern,
				Title:          "Table-driven tests",
				Rationale:      "Ensures unique names",
				FileReferences: []string{"foo_test.go:42"},
			}},
			numVariants: 2,
			wantCount:   1,
		},
		"missing title": {
			input: []VariantLearning{{
				VariantID:      1,
				Rationale:      "reason",
				FileReferences: []string{"file.go"},
			}},
			numVariants: 2,
			wantNil:     true,
		},
		"missing rationale": {
			input: []VariantLearning{{
				VariantID:      1,
				Title:          "title",
				FileReferences: []string{"file.go"},
			}},
			numVariants: 2,
			wantNil:     true,
		},
		"missing file references": {
			input: []VariantLearning{{
				VariantID: 1,
				Title:     "title",
				Rationale: "reason",
			}},
			numVariants: 2,
			wantNil:     true,
		},
		"empty file references": {
			input: []VariantLearning{{
				VariantID:      1,
				Title:          "title",
				Rationale:      "reason",
				FileReferences: []string{},
			}},
			numVariants: 2,
			wantNil:     true,
		},
		"invalid variant ID too high": {
			input: []VariantLearning{{
				VariantID:      5, // > numVariants
				Title:          "title",
				Rationale:      "reason",
				FileReferences: []string{"file.go"},
			}},
			numVariants: 2,
			wantNil:     true,
		},
		"invalid variant ID zero": {
			input: []VariantLearning{{
				VariantID:      0,
				Title:          "title",
				Rationale:      "reason",
				FileReferences: []string{"file.go"},
			}},
			numVariants: 2,
			wantNil:     true,
		},
		"invalid variant ID negative": {
			input: []VariantLearning{{
				VariantID:      -1,
				Title:          "title",
				Rationale:      "reason",
				FileReferences: []string{"file.go"},
			}},
			numVariants: 2,
			wantNil:     true,
		},
		"unknown category allowed": {
			input: []VariantLearning{{
				VariantID:      1,
				Category:       "performance", // unknown
				Title:          "title",
				Rationale:      "reason",
				FileReferences: []string{"file.go"},
			}},
			numVariants: 2,
			wantCount:   1,
		},
		"partial valid": {
			input: []VariantLearning{
				{VariantID: 1, Title: "valid", Rationale: "r", FileReferences: []string{"f.go"}},
				{VariantID: 1, Title: "", Rationale: "r", FileReferences: []string{"f.go"}}, // invalid
			},
			numVariants: 2,
			wantCount:   1,
		},
		"per-variant limit enforced": {
			input: func() []VariantLearning {
				learnings := make([]VariantLearning, 8)
				for i := range 8 {
					learnings[i] = VariantLearning{
						VariantID:      1,
						Title:          "title " + itoa(i),
						Rationale:      "r",
						FileReferences: []string{"f.go"},
					}
				}
				return learnings
			}(),
			numVariants: 2,
			wantCount:   MaxLearningsPerVariant, // 5
		},
		"total limit enforced": {
			input: func() []VariantLearning {
				learnings := make([]VariantLearning, 30)
				for i := range 30 {
					// Distribute across 6 variants to avoid per-variant limit
					learnings[i] = VariantLearning{
						VariantID:      (i % 6) + 1,
						Title:          "title " + itoa(i),
						Rationale:      "r",
						FileReferences: []string{"f.go"},
					}
				}
				return learnings
			}(),
			numVariants: 6,
			wantCount:   MaxLearningsTotal, // 20
		},
		"whitespace-only title rejected": {
			input: []VariantLearning{{
				VariantID:      1,
				Title:          "   ",
				Rationale:      "reason",
				FileReferences: []string{"file.go"},
			}},
			numVariants: 2,
			wantNil:     true,
		},
		"whitespace-only rationale rejected": {
			input: []VariantLearning{{
				VariantID:      1,
				Title:          "title",
				Rationale:      "  \t  ",
				FileReferences: []string{"file.go"},
			}},
			numVariants: 2,
			wantNil:     true,
		},
		"nil input": {
			input:       nil,
			numVariants: 2,
			wantNil:     true,
		},
		"empty input": {
			input:       []VariantLearning{},
			numVariants: 2,
			wantNil:     true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := validateLearnings(tc.input, tc.numVariants)
			if tc.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
				return
			}
			if len(got) != tc.wantCount {
				t.Errorf("expected %d learnings, got %d", tc.wantCount, len(got))
			}
		})
	}
}

func TestValidateLearnings_FileReferenceTruncation(t *testing.T) {
	input := []VariantLearning{{
		VariantID:      1,
		Title:          "title",
		Rationale:      "reason",
		FileReferences: []string{"a.go", "b.go", "c.go", "d.go", "e.go", "f.go", "g.go"},
	}}

	got := validateLearnings(input, 2)
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 learning, got %d", len(got))
	}
	if len(got[0].FileReferences) != MaxFileRefsPerLearning {
		t.Errorf("expected %d file references (truncated), got %d",
			MaxFileRefsPerLearning, len(got[0].FileReferences))
	}
}

func TestValidateLearnings_WhitespaceTrimming(t *testing.T) {
	input := []VariantLearning{{
		VariantID:      1,
		Title:          "  title with spaces  ",
		Description:    "  description  ",
		Rationale:      "  rationale  ",
		FileReferences: []string{"file.go"},
	}}

	got := validateLearnings(input, 2)
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got[0].Title != "title with spaces" {
		t.Errorf("expected trimmed title, got %q", got[0].Title)
	}
	if got[0].Description != "description" {
		t.Errorf("expected trimmed description, got %q", got[0].Description)
	}
	if got[0].Rationale != "rationale" {
		t.Errorf("expected trimmed rationale, got %q", got[0].Rationale)
	}
}

func TestParseAndValidate_WithLearnings(t *testing.T) {
	comp := NewComparator(nil, "")

	// JSON with valid learnings
	validJSON := `{
		"recommendation": 1,
		"confidence": "high",
		"summary": "Variant 1 is the best choice.",
		"file_analyses": [],
		"observations": [],
		"learnings": [
			{
				"variant_id": 1,
				"category": "code-pattern",
				"title": "Table-driven tests",
				"description": "Uses map[string]struct for test cases",
				"rationale": "Ensures unique test names",
				"file_references": ["foo_test.go:42"]
			},
			{
				"variant_id": 2,
				"category": "architecture",
				"title": "Clean separation",
				"description": "Separates concerns well",
				"rationale": "Makes code easier to maintain",
				"file_references": ["internal/pkg/module.go"]
			}
		]
	}`

	result, err := comp.parseAndValidate(validJSON, 2)
	if err != nil {
		t.Fatalf("parseAndValidate failed: %v", err)
	}

	if len(result.Learnings) != 2 {
		t.Errorf("expected 2 learnings, got %d", len(result.Learnings))
	}
	if result.Learnings[0].Title != "Table-driven tests" {
		t.Errorf("unexpected first learning title: %s", result.Learnings[0].Title)
	}
	if result.Learnings[1].Category != LearningCategoryArchitecture {
		t.Errorf("unexpected second learning category: %s", result.Learnings[1].Category)
	}
}

func TestParseAndValidate_FiltersInvalidLearnings(t *testing.T) {
	comp := NewComparator(nil, "")

	// JSON with mix of valid and invalid learnings
	jsonWithInvalid := `{
		"recommendation": 1,
		"confidence": "medium",
		"summary": "Test summary",
		"file_analyses": [],
		"observations": [],
		"learnings": [
			{
				"variant_id": 1,
				"category": "testing",
				"title": "Valid learning",
				"rationale": "Has all required fields",
				"file_references": ["test.go"]
			},
			{
				"variant_id": 1,
				"category": "code-pattern",
				"title": "",
				"rationale": "Missing title",
				"file_references": ["test.go"]
			},
			{
				"variant_id": 99,
				"category": "architecture",
				"title": "Invalid variant",
				"rationale": "Variant 99 doesn't exist",
				"file_references": ["test.go"]
			}
		]
	}`

	result, err := comp.parseAndValidate(jsonWithInvalid, 2)
	if err != nil {
		t.Fatalf("parseAndValidate failed: %v", err)
	}

	// Should only have 1 valid learning
	if len(result.Learnings) != 1 {
		t.Errorf("expected 1 valid learning, got %d", len(result.Learnings))
	}
	if result.Learnings[0].Title != "Valid learning" {
		t.Errorf("unexpected learning title: %s", result.Learnings[0].Title)
	}
}

func TestParseAndValidate_NoLearnings(t *testing.T) {
	comp := NewComparator(nil, "")

	// JSON without learnings field (backwards compatibility)
	jsonWithoutLearnings := `{
		"recommendation": 2,
		"confidence": "low",
		"summary": "Variant 2 is marginally better",
		"file_analyses": [],
		"observations": []
	}`

	result, err := comp.parseAndValidate(jsonWithoutLearnings, 2)
	if err != nil {
		t.Fatalf("parseAndValidate failed: %v", err)
	}

	// Learnings should be nil (omitempty behavior)
	if result.Learnings != nil {
		t.Errorf("expected nil learnings, got %v", result.Learnings)
	}
}

// TestParseAndValidate_MalformedLearnings verifies that malformed learnings
// are handled gracefully without failing the entire comparison. [Req 6.4]
func TestParseAndValidate_MalformedLearnings(t *testing.T) {
	comp := NewComparator(nil, "")

	tests := map[string]struct {
		json string
		desc string
	}{
		"variant_id as string": {
			json: `{
				"recommendation": 1,
				"confidence": "high",
				"summary": "Variant 1 is better",
				"file_analyses": [],
				"observations": [],
				"learnings": [
					{
						"variant_id": "1",
						"category": "code-pattern",
						"title": "Good pattern",
						"description": "A useful pattern",
						"rationale": "Why it matters",
						"file_references": ["file.go:10"]
					}
				]
			}`,
			desc: "AI returns variant_id as string instead of int",
		},
		"file_references as string": {
			json: `{
				"recommendation": 1,
				"confidence": "high",
				"summary": "Variant 1 is better",
				"file_analyses": [],
				"observations": [],
				"learnings": [
					{
						"variant_id": 1,
						"category": "code-pattern",
						"title": "Good pattern",
						"description": "A useful pattern",
						"rationale": "Why it matters",
						"file_references": "file.go:10"
					}
				]
			}`,
			desc: "AI returns file_references as string instead of array",
		},
		"learnings as object": {
			json: `{
				"recommendation": 1,
				"confidence": "high",
				"summary": "Variant 1 is better",
				"file_analyses": [],
				"observations": [],
				"learnings": {
					"variant_id": 1,
					"category": "code-pattern"
				}
			}`,
			desc: "AI returns learnings as object instead of array",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result, err := comp.parseAndValidate(tc.json, 2)

			// Core comparison should succeed even with malformed learnings
			if err != nil {
				t.Fatalf("parseAndValidate should not fail for malformed learnings (%s): %v", tc.desc, err)
			}

			// Verify core fields are parsed correctly
			if result.Recommendation != 1 {
				t.Errorf("expected recommendation 1, got %d", result.Recommendation)
			}
			if result.Confidence != "high" {
				t.Errorf("expected confidence 'high', got %q", result.Confidence)
			}

			// Malformed learnings should be gracefully discarded (nil or empty)
			if len(result.Learnings) > 0 {
				t.Errorf("expected no learnings after malformed input, got %d", len(result.Learnings))
			}
		})
	}
}

// Integration Tests for Comparison with Learnings

// mockPromptRunner implements the promptRunner interface for testing.
type mockPromptRunner struct {
	response string
	err      error
}

func (m *mockPromptRunner) RunCustomPrompt(prompt string) (*agents.RunResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &agents.RunResult{Output: m.response}, nil
}

// TestIntegration_ComparisonWithLearnings tests the full comparison flow
// from prompt to result, verifying learnings are correctly extracted.
func TestIntegration_ComparisonWithLearnings(t *testing.T) {
	// Mock AI response with learnings
	aiResponse := `{
		"recommendation": 1,
		"confidence": "high",
		"summary": "Variant 1 demonstrates cleaner architecture and better test coverage.",
		"file_analyses": [
			{
				"path": "internal/service/handler.go",
				"variants": {"1": "Clean separation of concerns", "2": "Mixed responsibilities"},
				"preference": 1
			}
		],
		"observations": [
			"Variant 1 uses table-driven tests",
			"Variant 2 has fewer test cases"
		],
		"learnings": [
			{
				"variant_id": 1,
				"category": "code-pattern",
				"title": "Table-driven tests with map[string]struct",
				"description": "Uses map[string]struct{} for test cases to ensure unique names.",
				"rationale": "Guarantees each test case has a unique name, making failures easier to identify.",
				"file_references": ["internal/service/handler_test.go:42", "internal/service/handler_test.go:120"]
			},
			{
				"variant_id": 1,
				"category": "error-handling",
				"title": "Sentinel errors with errors.Is()",
				"description": "Defines package-level sentinel errors and checks them with errors.Is().",
				"rationale": "Enables type-safe error checking across package boundaries without string matching.",
				"file_references": ["internal/service/errors.go:10"]
			},
			{
				"variant_id": 2,
				"category": "architecture",
				"title": "Functional options pattern",
				"description": "Uses functional options for flexible struct configuration.",
				"rationale": "Allows adding new options without breaking existing callers.",
				"file_references": ["internal/config/options.go:15", "internal/config/options.go:30"]
			}
		]
	}`

	runner := &mockPromptRunner{response: aiResponse}
	comp := NewComparator(runner, "")

	ctx := context.Background()
	input := ComparisonInput{
		SpecName: "test-feature",
		Variants: []VariantData{
			{ID: 1, CommitMessages: []string{"implement handler"}},
			{ID: 2, CommitMessages: []string{"different impl"}},
		},
	}

	result, err := comp.CompareUnified(ctx, input)
	if err != nil {
		t.Fatalf("CompareUnified() failed: %v", err)
	}

	// Verify basic comparison fields
	if result.Recommendation != 1 {
		t.Errorf("expected recommendation 1, got %d", result.Recommendation)
	}
	if result.Confidence != "high" {
		t.Errorf("expected confidence 'high', got %q", result.Confidence)
	}

	// Verify learnings were extracted
	if len(result.Learnings) != 3 {
		t.Fatalf("expected 3 learnings, got %d", len(result.Learnings))
	}

	// Verify first learning
	l1 := result.Learnings[0]
	if l1.VariantID != 1 {
		t.Errorf("learning 0: expected variant_id 1, got %d", l1.VariantID)
	}
	if l1.Category != LearningCategoryCodePattern {
		t.Errorf("learning 0: expected category %q, got %q", LearningCategoryCodePattern, l1.Category)
	}
	if l1.Title != "Table-driven tests with map[string]struct" {
		t.Errorf("learning 0: unexpected title: %q", l1.Title)
	}
	if len(l1.FileReferences) != 2 {
		t.Errorf("learning 0: expected 2 file references, got %d", len(l1.FileReferences))
	}

	// Verify second learning has error-handling category
	l2 := result.Learnings[1]
	if l2.Category != LearningCategoryErrorHandling {
		t.Errorf("learning 1: expected category %q, got %q", LearningCategoryErrorHandling, l2.Category)
	}

	// Verify third learning is from variant 2
	l3 := result.Learnings[2]
	if l3.VariantID != 2 {
		t.Errorf("learning 2: expected variant_id 2, got %d", l3.VariantID)
	}
	if l3.Category != LearningCategoryArchitecture {
		t.Errorf("learning 2: expected category %q, got %q", LearningCategoryArchitecture, l3.Category)
	}
}

// TestIntegration_ComparisonWithLearningsGroupedByVariant tests that learnings
// can be grouped by variant for rendering.
func TestIntegration_ComparisonWithLearningsGroupedByVariant(t *testing.T) {
	aiResponse := `{
		"recommendation": 2,
		"confidence": "medium",
		"summary": "Variant 2 is slightly better.",
		"file_analyses": [],
		"observations": [],
		"learnings": [
			{"variant_id": 1, "category": "testing", "title": "L1", "rationale": "R1", "file_references": ["f1.go"]},
			{"variant_id": 2, "category": "architecture", "title": "L2", "rationale": "R2", "file_references": ["f2.go"]},
			{"variant_id": 1, "category": "code-pattern", "title": "L3", "rationale": "R3", "file_references": ["f3.go"]},
			{"variant_id": 3, "category": "error-handling", "title": "L4", "rationale": "R4", "file_references": ["f4.go"]}
		]
	}`

	runner := &mockPromptRunner{response: aiResponse}
	comp := NewComparator(runner, "")

	ctx := context.Background()
	input := ComparisonInput{
		SpecName: "test-feature",
		Variants: []VariantData{
			{ID: 1, CommitMessages: []string{"v1"}},
			{ID: 2, CommitMessages: []string{"v2"}},
			{ID: 3, CommitMessages: []string{"v3"}},
		},
	}

	result, err := comp.CompareUnified(ctx, input)
	if err != nil {
		t.Fatalf("CompareUnified() failed: %v", err)
	}

	// Group learnings by variant
	grouped := GroupLearningsByVariant(result.Learnings)

	// Verify grouping
	if len(grouped) != 3 {
		t.Errorf("expected 3 variant groups, got %d", len(grouped))
	}
	if len(grouped[1]) != 2 {
		t.Errorf("variant 1 should have 2 learnings, got %d", len(grouped[1]))
	}
	if len(grouped[2]) != 1 {
		t.Errorf("variant 2 should have 1 learning, got %d", len(grouped[2]))
	}
	if len(grouped[3]) != 1 {
		t.Errorf("variant 3 should have 1 learning, got %d", len(grouped[3]))
	}

	// Verify ordering via SortedVariantIDs
	sortedIDs := SortedVariantIDs(grouped)
	if len(sortedIDs) != 3 || sortedIDs[0] != 1 || sortedIDs[1] != 2 || sortedIDs[2] != 3 {
		t.Errorf("expected sorted IDs [1, 2, 3], got %v", sortedIDs)
	}
}

// TestIntegration_GracefulDegradation_MalformedLearnings tests that malformed
// learnings do not break comparison - the comparison still succeeds but learnings
// are filtered or omitted [Req 6.2, 6.4].
func TestIntegration_GracefulDegradation_MalformedLearnings(t *testing.T) {
	tests := map[string]struct {
		response        string
		wantLearnings   int
		wantRecommend   int
	}{
		"all learnings missing required fields": {
			response: `{
				"recommendation": 1,
				"confidence": "high",
				"summary": "Test summary.",
				"file_analyses": [],
				"observations": [],
				"learnings": [
					{"variant_id": 1, "category": "testing", "title": "", "rationale": "R1", "file_references": ["f.go"]},
					{"variant_id": 1, "category": "testing", "title": "T2", "rationale": "", "file_references": ["f.go"]},
					{"variant_id": 1, "category": "testing", "title": "T3", "rationale": "R3", "file_references": []}
				]
			}`,
			wantLearnings: 0, // All invalid
			wantRecommend: 1,
		},
		"mix of valid and invalid learnings": {
			response: `{
				"recommendation": 2,
				"confidence": "medium",
				"summary": "Variant 2 wins.",
				"file_analyses": [],
				"observations": [],
				"learnings": [
					{"variant_id": 1, "category": "testing", "title": "Valid One", "rationale": "R1", "file_references": ["f.go"]},
					{"variant_id": 99, "category": "testing", "title": "Invalid Variant", "rationale": "R2", "file_references": ["f.go"]},
					{"variant_id": 2, "category": "architecture", "title": "Valid Two", "rationale": "R3", "file_references": ["g.go"]}
				]
			}`,
			wantLearnings: 2, // Only 2 valid
			wantRecommend: 2,
		},
		"learnings field completely missing (backwards compat)": {
			response: `{
				"recommendation": 1,
				"confidence": "low",
				"summary": "Old response format.",
				"file_analyses": [],
				"observations": []
			}`,
			wantLearnings: 0, // nil/absent
			wantRecommend: 1,
		},
		"learnings field is null": {
			response: `{
				"recommendation": 1,
				"confidence": "high",
				"summary": "Null learnings.",
				"file_analyses": [],
				"observations": [],
				"learnings": null
			}`,
			wantLearnings: 0,
			wantRecommend: 1,
		},
		"learnings field is empty array": {
			response: `{
				"recommendation": 1,
				"confidence": "high",
				"summary": "Empty learnings.",
				"file_analyses": [],
				"observations": [],
				"learnings": []
			}`,
			wantLearnings: 0,
			wantRecommend: 1,
		},
		"learnings with unknown category (forward compat)": {
			response: `{
				"recommendation": 1,
				"confidence": "high",
				"summary": "Unknown category.",
				"file_analyses": [],
				"observations": [],
				"learnings": [
					{"variant_id": 1, "category": "performance", "title": "Fast Algorithm", "rationale": "O(1)", "file_references": ["algo.go"]}
				]
			}`,
			wantLearnings: 1, // Unknown category is allowed
			wantRecommend: 1,
		},
		"per-variant limit exceeded": {
			response: `{
				"recommendation": 1,
				"confidence": "high",
				"summary": "Too many learnings from one variant.",
				"file_analyses": [],
				"observations": [],
				"learnings": [
					{"variant_id": 1, "category": "testing", "title": "L1", "rationale": "R", "file_references": ["f.go"]},
					{"variant_id": 1, "category": "testing", "title": "L2", "rationale": "R", "file_references": ["f.go"]},
					{"variant_id": 1, "category": "testing", "title": "L3", "rationale": "R", "file_references": ["f.go"]},
					{"variant_id": 1, "category": "testing", "title": "L4", "rationale": "R", "file_references": ["f.go"]},
					{"variant_id": 1, "category": "testing", "title": "L5", "rationale": "R", "file_references": ["f.go"]},
					{"variant_id": 1, "category": "testing", "title": "L6", "rationale": "R", "file_references": ["f.go"]},
					{"variant_id": 1, "category": "testing", "title": "L7", "rationale": "R", "file_references": ["f.go"]}
				]
			}`,
			wantLearnings: MaxLearningsPerVariant, // Capped at limit
			wantRecommend: 1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			runner := &mockPromptRunner{response: tc.response}
			comp := NewComparator(runner, "")

			ctx := context.Background()
			input := ComparisonInput{
				SpecName: "test-feature",
				Variants: []VariantData{
					{ID: 1, CommitMessages: []string{"v1"}},
					{ID: 2, CommitMessages: []string{"v2"}},
				},
			}

			result, err := comp.CompareUnified(ctx, input)
			if err != nil {
				t.Fatalf("CompareUnified() should not fail on malformed learnings: %v", err)
			}

			// Core comparison fields should be valid
			if result.Recommendation != tc.wantRecommend {
				t.Errorf("expected recommendation %d, got %d", tc.wantRecommend, result.Recommendation)
			}

			// Learnings count should match expected
			gotLearnings := len(result.Learnings)
			if gotLearnings != tc.wantLearnings {
				t.Errorf("expected %d learnings, got %d", tc.wantLearnings, gotLearnings)
			}
		})
	}
}

// TestIntegration_GracefulDegradation_WhitespaceOnlyFields tests that learnings
// with whitespace-only required fields are rejected.
// Tests for CompareUnified fallback to comparison file

func TestCompareUnified_FallbackToFile_WhenAgentFails(t *testing.T) {
	dir := t.TempDir()
	outputPath := dir + "/comparison.json"

	// Agent writes the comparison file during execution, then fails
	validJSON := `{
		"recommendation": 1,
		"confidence": "high",
		"summary": "Variant 1 is better.",
		"file_analyses": [],
		"observations": ["Good code"]
	}`
	runner := &fileWritingMockRunner{
		outputPath:  outputPath,
		fileContent: validJSON,
		err:         fmt.Errorf("agent execution failed"),
	}
	comp := NewComparator(runner, "")

	ctx := context.Background()
	input := ComparisonInput{
		SpecName:   "test-spec",
		Variants:   []VariantData{{ID: 1, CommitMessages: []string{"c1"}}, {ID: 2, CommitMessages: []string{"c2"}}},
		OutputPath: outputPath,
	}

	result, err := comp.CompareUnified(ctx, input)
	if err != nil {
		t.Fatalf("expected fallback to succeed, got error: %v", err)
	}
	if result.Recommendation != 1 {
		t.Errorf("expected recommendation 1, got %d", result.Recommendation)
	}
	if result.Confidence != "high" {
		t.Errorf("expected confidence 'high', got %q", result.Confidence)
	}
	if result.Summary != "Variant 1 is better." {
		t.Errorf("unexpected summary: %q", result.Summary)
	}
}

func TestCompareUnified_FallbackToFile_WhenJSONValidationFails(t *testing.T) {
	dir := t.TempDir()
	outputPath := dir + "/comparison.json"

	// Agent returns garbled response that fails JSON validation, but writes
	// a valid file to disk during execution
	validJSON := `{
		"recommendation": 2,
		"confidence": "medium",
		"summary": "Variant 2 wins.",
		"file_analyses": [],
		"observations": []
	}`
	runner := &fileWritingMockRunner{
		outputPath:  outputPath,
		fileContent: validJSON,
		// Return garbled response (not an error, but invalid JSON)
		response: "not valid json at all",
	}
	comp := NewComparator(runner, "")

	ctx := context.Background()
	input := ComparisonInput{
		SpecName:   "test-spec",
		Variants:   []VariantData{{ID: 1, CommitMessages: []string{"c1"}}, {ID: 2, CommitMessages: []string{"c2"}}},
		OutputPath: outputPath,
	}

	result, err := comp.CompareUnified(ctx, input)
	if err != nil {
		t.Fatalf("expected fallback to succeed, got error: %v", err)
	}
	if result.Recommendation != 2 {
		t.Errorf("expected recommendation 2, got %d", result.Recommendation)
	}
}

func TestCompareUnified_NoFallback_WhenFileNotWritten(t *testing.T) {
	dir := t.TempDir()
	outputPath := dir + "/comparison.json"
	// Do NOT write the file - it doesn't exist

	runner := &mockPromptRunner{err: fmt.Errorf("agent execution failed")}
	comp := NewComparator(runner, "")

	ctx := context.Background()
	input := ComparisonInput{
		SpecName:   "test-spec",
		Variants:   []VariantData{{ID: 1, CommitMessages: []string{"c1"}}, {ID: 2, CommitMessages: []string{"c2"}}},
		OutputPath: outputPath,
	}

	_, err := comp.CompareUnified(ctx, input)
	if err == nil {
		t.Fatal("expected error when no fallback file exists")
	}
	if !containsString(err.Error(), "agent execution failed") {
		t.Errorf("expected original error, got: %v", err)
	}
}

func TestCompareUnified_NoFallback_WhenNoOutputPath(t *testing.T) {
	runner := &mockPromptRunner{err: fmt.Errorf("agent execution failed")}
	comp := NewComparator(runner, "")

	ctx := context.Background()
	input := ComparisonInput{
		SpecName:   "test-spec",
		Variants:   []VariantData{{ID: 1, CommitMessages: []string{"c1"}}, {ID: 2, CommitMessages: []string{"c2"}}},
		OutputPath: "", // No output path
	}

	_, err := comp.CompareUnified(ctx, input)
	if err == nil {
		t.Fatal("expected error when no output path set")
	}
}

func TestCompareUnified_NoFallback_WhenFileIsStale(t *testing.T) {
	dir := t.TempDir()
	outputPath := dir + "/comparison.json"

	// Write a file BEFORE comparison starts (simulating a pre-existing stale file)
	staleJSON := `{
		"recommendation": 1,
		"confidence": "low",
		"summary": "Old stale result.",
		"file_analyses": [],
		"observations": []
	}`
	if err := os.WriteFile(outputPath, []byte(staleJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	// Sleep briefly to ensure mod time difference is detectable
	time.Sleep(10 * time.Millisecond)

	// Use a multi-call runner that:
	// 1. Doesn't update the file (simulates agent NOT writing a new file)
	// 2. Returns an error
	runner := &mockPromptRunner{err: fmt.Errorf("agent execution failed")}
	comp := NewComparator(runner, "")

	ctx := context.Background()
	input := ComparisonInput{
		SpecName:   "test-spec",
		Variants:   []VariantData{{ID: 1, CommitMessages: []string{"c1"}}, {ID: 2, CommitMessages: []string{"c2"}}},
		OutputPath: outputPath,
	}

	_, err := comp.CompareUnified(ctx, input)
	if err == nil {
		t.Fatal("expected error when file is stale (not updated during comparison)")
	}
	if !containsString(err.Error(), "agent execution failed") {
		t.Errorf("expected original error, got: %v", err)
	}
}

func TestCompareUnified_FallbackToFile_WhenFileUpdatedDuringComparison(t *testing.T) {
	dir := t.TempDir()
	outputPath := dir + "/comparison.json"

	// Write an initial stale file
	staleJSON := `{
		"recommendation": 1,
		"confidence": "low",
		"summary": "Old result.",
		"file_analyses": [],
		"observations": []
	}`
	if err := os.WriteFile(outputPath, []byte(staleJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	// Sleep to ensure time difference
	time.Sleep(10 * time.Millisecond)

	// Use a runner that updates the file during execution then fails
	newJSON := `{
		"recommendation": 2,
		"confidence": "high",
		"summary": "Updated result.",
		"file_analyses": [],
		"observations": ["Fresh observation"]
	}`
	runner := &fileWritingMockRunner{
		outputPath: outputPath,
		fileContent: newJSON,
		err:        fmt.Errorf("session timed out"),
	}
	comp := NewComparator(runner, "")

	ctx := context.Background()
	input := ComparisonInput{
		SpecName:   "test-spec",
		Variants:   []VariantData{{ID: 1, CommitMessages: []string{"c1"}}, {ID: 2, CommitMessages: []string{"c2"}}},
		OutputPath: outputPath,
	}

	result, err := comp.CompareUnified(ctx, input)
	if err != nil {
		t.Fatalf("expected fallback to succeed, got error: %v", err)
	}
	if result.Recommendation != 2 {
		t.Errorf("expected recommendation 2 (from updated file), got %d", result.Recommendation)
	}
	if result.Summary != "Updated result." {
		t.Errorf("expected updated summary, got %q", result.Summary)
	}
}

func TestCompareUnified_NoFallback_WhenFileIsInvalid(t *testing.T) {
	dir := t.TempDir()
	outputPath := dir + "/comparison.json"

	// Agent fails
	runner := &mockPromptRunner{err: fmt.Errorf("agent execution failed")}
	comp := NewComparator(runner, "")

	// Write an invalid comparison file
	if err := os.WriteFile(outputPath, []byte("not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	input := ComparisonInput{
		SpecName:   "test-spec",
		Variants:   []VariantData{{ID: 1, CommitMessages: []string{"c1"}}, {ID: 2, CommitMessages: []string{"c2"}}},
		OutputPath: outputPath,
	}

	_, err := comp.CompareUnified(ctx, input)
	if err == nil {
		t.Fatal("expected error when fallback file is invalid")
	}
}

// fileWritingMockRunner is a mock that writes a file during RunCustomPrompt
// then returns an error or a garbled response, simulating an agent that writes
// comparison.json but the session fails afterward.
type fileWritingMockRunner struct {
	outputPath  string
	fileContent string
	response    string // If set, return this as output (may be invalid JSON)
	err         error  // If set, return this error
}

func (m *fileWritingMockRunner) RunCustomPrompt(prompt string) (*agents.RunResult, error) {
	// Simulate the agent writing the comparison file before the error
	if err := os.WriteFile(m.outputPath, []byte(m.fileContent), 0o644); err != nil {
		return nil, fmt.Errorf("mock: failed to write file: %w", err)
	}
	if m.err != nil {
		return nil, m.err
	}
	return &agents.RunResult{Output: m.response}, nil
}

func TestIntegration_GracefulDegradation_WhitespaceOnlyFields(t *testing.T) {
	aiResponse := `{
		"recommendation": 1,
		"confidence": "high",
		"summary": "Testing whitespace.",
		"file_analyses": [],
		"observations": [],
		"learnings": [
			{"variant_id": 1, "category": "testing", "title": "   ", "rationale": "Valid rationale", "file_references": ["f.go"]},
			{"variant_id": 1, "category": "testing", "title": "Valid title", "rationale": "  \t  ", "file_references": ["f.go"]},
			{"variant_id": 1, "category": "testing", "title": "  Valid trimmed  ", "rationale": "  Also trimmed  ", "file_references": ["f.go"]}
		]
	}`

	runner := &mockPromptRunner{response: aiResponse}
	comp := NewComparator(runner, "")

	ctx := context.Background()
	input := ComparisonInput{
		SpecName: "test-feature",
		Variants: []VariantData{
			{ID: 1, CommitMessages: []string{"v1"}},
			{ID: 2, CommitMessages: []string{"v2"}},
		},
	}

	result, err := comp.CompareUnified(ctx, input)
	if err != nil {
		t.Fatalf("CompareUnified() failed: %v", err)
	}

	// Only the third learning should be valid (both title and rationale non-empty after trim)
	if len(result.Learnings) != 1 {
		t.Errorf("expected 1 valid learning, got %d", len(result.Learnings))
	}

	if len(result.Learnings) > 0 {
		// Verify whitespace was trimmed
		if result.Learnings[0].Title != "Valid trimmed" {
			t.Errorf("expected trimmed title, got %q", result.Learnings[0].Title)
		}
		if result.Learnings[0].Rationale != "Also trimmed" {
			t.Errorf("expected trimmed rationale, got %q", result.Learnings[0].Rationale)
		}
	}
}
