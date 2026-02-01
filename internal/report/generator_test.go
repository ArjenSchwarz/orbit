package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arjenschwarz/orbit/internal/comparison"
	"github.com/arjenschwarz/orbit/internal/cost"
)

func TestGenerate_CreatesIndexHTML(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "report")

	gen := NewGenerator(outputDir)
	data := &ReportData{
		SpecName:       "test-feature",
		GeneratedAt:    time.Date(2026, 1, 11, 10, 30, 0, 0, time.UTC),
		BaseCommit:     "abc123",
		OriginalBranch: "feature/test",
		Variants: []VariantReportData{
			{
				ID:     1,
				Branch: "orbit-impl-1/test-feature",
				Status: "completed",
				Diff:   "+func hello() {}",
				Metrics: VariantMetrics{
					Cost:     0.0523,
					CostUnit: cost.UnitUSD,
					Duration: "3m0s",
					NumTurns: 42,
				},
			},
			{
				ID:     2,
				Branch: "orbit-impl-2/test-feature",
				Status: "completed",
				Diff:   "+func world() {}",
				Metrics: VariantMetrics{
					Cost:     0.0312,
					CostUnit: cost.UnitUSD,
					Duration: "2m15s",
					NumTurns: 35,
				},
			},
		},
		Comparison: &comparison.Result{
			Recommendation: 1,
			Confidence:     "high",
			Summary:        "Variant 1 is cleaner.",
			Observations:   []string{"More concise code", "Better naming"},
			FileAnalyses: []comparison.FileAnalysis{
				{
					Path:       "main.go",
					Variants:   map[int]string{1: "Good", 2: "Okay"},
					Preference: 1,
				},
			},
		},
	}

	err := gen.Generate(data)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	indexPath := filepath.Join(outputDir, "index.html")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Errorf("index.html was not created")
	}

	content, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}

	// Verify key content is present
	checks := []string{
		"test-feature",
		"abc123",
		"feature/test",
		"Variant 1",
		"orbit-impl-1/test-feature",
		"completed",
		"$0.05",
		"high confidence",
		"Variant 1 is cleaner.",
		"More concise code",
		"main.go",
	}
	for _, check := range checks {
		if !strings.Contains(string(content), check) {
			t.Errorf("index.html missing expected content: %q", check)
		}
	}
}

func TestGenerate_EscapesContent(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "report")

	gen := NewGenerator(outputDir)
	data := &ReportData{
		SpecName:       "<script>alert('xss')</script>",
		GeneratedAt:    time.Now(),
		BaseCommit:     "<img src=x>",
		OriginalBranch: "feature/test",
		Variants: []VariantReportData{
			{
				ID:     1,
				Branch: "test-branch",
				Status: "completed",
				Diff:   "<script>malicious</script>",
				Error:  "<b>error</b>",
			},
		},
		Comparison: &comparison.Result{
			Recommendation: 1,
			Confidence:     "high",
			Summary:        "<script>alert('summary')</script>",
			Observations:   []string{"<a href='bad'>click</a>"},
		},
	}

	err := gen.Generate(data)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(outputDir, "index.html"))
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}

	contentStr := string(content)

	// Ensure dangerous HTML tags are escaped (not rendered as HTML)
	// These patterns indicate unescaped user content that could be parsed as HTML
	dangerous := []string{
		"<script>alert",      // User-injected script that could execute
		"</script>",          // Script tag closing
		"<img src=x>",        // User-injected img tag
		"<b>error</b>",       // User-injected HTML tag (should be escaped)
		"<a href='bad'>",     // User-injected link
	}
	for _, d := range dangerous {
		if strings.Contains(contentStr, d) {
			t.Errorf("index.html contains unescaped dangerous content: %q", d)
		}
	}

	// Ensure escaped versions are present (content is displayed, not executed)
	escaped := []string{
		"&lt;script&gt;",   // Escaped script open tag
		"&lt;/script&gt;",  // Escaped script close tag
		"&lt;img",          // Escaped img tag
	}
	for _, e := range escaped {
		if !strings.Contains(contentStr, e) {
			t.Errorf("index.html missing escaped content: %q", e)
		}
	}
}

func TestGenerate_SplitsLargeDiffs(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "report")

	// Create a large diff (>500 lines)
	var largeDiff strings.Builder
	for i := 0; i < 600; i++ {
		largeDiff.WriteString("+line ")
		largeDiff.WriteString(string(rune('0' + i%10)))
		largeDiff.WriteString("\n")
	}

	gen := NewGenerator(outputDir)
	data := &ReportData{
		SpecName:       "large-diff-test",
		GeneratedAt:    time.Now(),
		BaseCommit:     "abc123",
		OriginalBranch: "main",
		Variants: []VariantReportData{
			{
				ID:     1,
				Branch: "test-branch",
				Status: "completed",
				Diff:   largeDiff.String(),
			},
		},
	}

	err := gen.Generate(data)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Check that separate diff file was created
	diffPath := filepath.Join(outputDir, "diffs", "variant-1.html")
	if _, err := os.Stat(diffPath); os.IsNotExist(err) {
		t.Errorf("separate diff file was not created for large diff")
	}

	// Check that index.html links to the diff file
	indexContent, err := os.ReadFile(filepath.Join(outputDir, "index.html"))
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}

	if !strings.Contains(string(indexContent), "diffs/variant-1.html") {
		t.Errorf("index.html does not link to separate diff file")
	}

	// Verify diff content is in the separate file, not inline
	diffContent, err := os.ReadFile(diffPath)
	if err != nil {
		t.Fatalf("failed to read diff file: %v", err)
	}

	// The template escapes + as &#43; in some contexts
	diffStr := string(diffContent)
	hasContent := strings.Contains(diffStr, "+line") || strings.Contains(diffStr, "&#43;line")
	if !hasContent {
		// Show first 1000 chars for debugging
		preview := diffStr
		if len(preview) > 1000 {
			preview = preview[:1000]
		}
		t.Errorf("diff file missing diff content, got: %s", preview)
	}
}

func TestGenerate_IncludesFailedVariants(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "report")

	gen := NewGenerator(outputDir)
	data := &ReportData{
		SpecName:       "mixed-status",
		GeneratedAt:    time.Now(),
		BaseCommit:     "abc123",
		OriginalBranch: "main",
		Variants: []VariantReportData{
			{
				ID:     1,
				Branch: "success-branch",
				Status: "completed",
				Diff:   "+success",
			},
			{
				ID:     2,
				Branch: "failed-branch",
				Status: "failed",
				Error:  "Rate limit exceeded",
			},
		},
	}

	err := gen.Generate(data)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(outputDir, "index.html"))
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}

	// Verify failed variant is included
	if !strings.Contains(string(content), "failed-branch") {
		t.Errorf("index.html missing failed variant branch")
	}
	if !strings.Contains(string(content), "Rate limit exceeded") {
		t.Errorf("index.html missing failed variant error message")
	}
	if !strings.Contains(string(content), "status-failed") {
		t.Errorf("index.html missing failed status class")
	}
}

func TestGenerate_NoComparison(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "report")

	gen := NewGenerator(outputDir)
	data := &ReportData{
		SpecName:       "no-comparison",
		GeneratedAt:    time.Now(),
		BaseCommit:     "abc123",
		OriginalBranch: "main",
		Variants: []VariantReportData{
			{
				ID:     1,
				Branch: "only-branch",
				Status: "completed",
				Diff:   "+code",
			},
		},
		Comparison: nil, // No comparison when single variant or all failed
	}

	err := gen.Generate(data)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(outputDir, "index.html"))
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}

	// Should still render without errors
	if !strings.Contains(string(content), "no-comparison") {
		t.Errorf("index.html missing spec name")
	}
	if !strings.Contains(string(content), "only-branch") {
		t.Errorf("index.html missing variant")
	}
}

func TestCountLines(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"a", 1},
		{"a\n", 2},
		{"a\nb", 2},
		{"a\nb\nc", 3},
		{"a\nb\nc\n", 4},
	}

	for _, tt := range tests {
		got := countLines(tt.input)
		if got != tt.want {
			t.Errorf("countLines(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestFormatCost(t *testing.T) {
	tests := []struct {
		value float64
		unit  string
		want  string
	}{
		{0, cost.UnitUSD, "-"},
		{0.05, cost.UnitUSD, "$0.05"},
		{0.0523, cost.UnitUSD, "$0.05"},
		{1.23, cost.UnitUSD, "$1.23"},
		{0.45, cost.UnitCredits, "0.45 credits"},
		{0.33, cost.UnitPremiumRequests, "0.33 premium requests"},
		{1.5, "", "$1.50"}, // Empty unit defaults to USD
	}

	for _, tt := range tests {
		got := formatCost(tt.value, tt.unit)
		if got != tt.want {
			t.Errorf("formatCost(%v, %q) = %q, want %q", tt.value, tt.unit, got, tt.want)
		}
	}
}

func TestGenerate_CreatesMarkdownReport(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "report")

	gen := NewGenerator(outputDir)
	data := &ReportData{
		SpecName:       "test-feature",
		GeneratedAt:    time.Date(2026, 1, 11, 10, 30, 0, 0, time.UTC),
		BaseCommit:     "abc123def456",
		OriginalBranch: "feature/test",
		VariantCommits: map[int]string{
			1: "commit1abc",
			2: "commit2def",
		},
		Variants: []VariantReportData{
			{
				ID:     1,
				Branch: "orbit-impl-1/test-feature",
				Status: "completed",
				Diff:   "+func hello() {}",
				Agent:  "claude-code",
				Metrics: VariantMetrics{
					Cost:     0.0523,
					CostUnit: cost.UnitUSD,
					Duration: "3m0s",
					NumTurns: 42,
				},
			},
			{
				ID:     2,
				Branch: "orbit-impl-2/test-feature",
				Status: "completed",
				Diff:   "+func world() {}",
				Agent:  "codex",
				Metrics: VariantMetrics{
					Cost:     0.0312,
					CostUnit: cost.UnitUSD,
					Duration: "2m15s",
					NumTurns: 35,
				},
			},
		},
		Comparison: &comparison.Result{
			Recommendation: 1,
			Confidence:     "high",
			Summary:        "Variant 1 is cleaner.",
			Observations:   []string{"More concise code", "Better naming"},
			FileAnalyses: []comparison.FileAnalysis{
				{
					Path:       "main.go",
					Variants:   map[int]string{1: "Good", 2: "Okay"},
					Preference: 1,
				},
			},
		},
	}

	err := gen.Generate(data)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Verify report.md was created
	mdPath := filepath.Join(outputDir, "report.md")
	if _, err := os.Stat(mdPath); os.IsNotExist(err) {
		t.Errorf("report.md was not created")
	}

	content, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("failed to read report.md: %v", err)
	}

	contentStr := string(content)

	// Verify key content is present
	checks := []string{
		"test-feature",                // spec name
		"abc123def456",                // base commit
		"feature/test",                // original branch
		"Variant 1",                   // variant identifier
		"orbit-impl-1/test-feature",   // branch name
		"completed",                   // status
		"$0.05",                       // cost
		"high",                        // confidence
		"Variant 1 is cleaner",        // summary
		"More concise code",           // observation
		"main.go",                     // file analysis path
		"claude-code",                 // agent name
		"```diff",                     // diff code fence
	}
	for _, check := range checks {
		if !strings.Contains(contentStr, check) {
			t.Errorf("report.md missing expected content: %q", check)
		}
	}

	// Verify front matter is present with all required fields [Req 1.7]
	if !strings.Contains(contentStr, "---") {
		t.Errorf("report.md missing YAML front matter")
	}
	if !strings.Contains(contentStr, "title:") {
		t.Errorf("report.md missing title in front matter")
	}
	if !strings.Contains(contentStr, "generated_at:") {
		t.Errorf("report.md missing generated_at in front matter")
	}
	if !strings.Contains(contentStr, "base_commit: abc123def456") {
		t.Errorf("report.md missing base_commit in front matter")
	}
	if !strings.Contains(contentStr, "variant_commits:") {
		t.Errorf("report.md missing variant_commits in front matter")
	}
	if !strings.Contains(contentStr, "1: commit1abc") {
		t.Errorf("report.md missing variant 1 commit in front matter")
	}
	if !strings.Contains(contentStr, "2: commit2def") {
		t.Errorf("report.md missing variant 2 commit in front matter")
	}
}

func TestGenerate_MarkdownLinksToLargeDiffs(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "report")

	// Create a large diff (>500 lines)
	var largeDiff strings.Builder
	for i := 0; i < 600; i++ {
		largeDiff.WriteString("+line ")
		largeDiff.WriteString(string(rune('0' + i%10)))
		largeDiff.WriteString("\n")
	}

	gen := NewGenerator(outputDir)
	data := &ReportData{
		SpecName:       "large-diff-test",
		GeneratedAt:    time.Now(),
		BaseCommit:     "abc123",
		OriginalBranch: "main",
		Variants: []VariantReportData{
			{
				ID:     1,
				Branch: "test-branch",
				Status: "completed",
				Diff:   largeDiff.String(),
			},
		},
	}

	err := gen.Generate(data)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Check that report.md was created and links to the diff file
	mdContent, err := os.ReadFile(filepath.Join(outputDir, "report.md"))
	if err != nil {
		t.Fatalf("failed to read report.md: %v", err)
	}

	contentStr := string(mdContent)

	// Should contain markdown link to diff file
	// The link format may vary slightly based on path separators
	if !strings.Contains(contentStr, "View full diff") {
		t.Errorf("report.md missing 'View full diff' text")
	}
	if !strings.Contains(contentStr, "variant-1.html") {
		t.Errorf("report.md missing link to diff file variant-1.html")
	}
	if !strings.Contains(string(mdContent), ">500 lines") {
		t.Errorf("report.md missing large diff threshold message")
	}
}

func TestGenerate_MarkdownNoComparison(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "report")

	gen := NewGenerator(outputDir)
	data := &ReportData{
		SpecName:       "no-comparison",
		GeneratedAt:    time.Now(),
		BaseCommit:     "abc123",
		OriginalBranch: "main",
		Variants: []VariantReportData{
			{
				ID:     1,
				Branch: "only-branch",
				Status: "completed",
				Diff:   "+code",
			},
		},
		Comparison: nil, // No comparison when single variant or all failed
	}

	err := gen.Generate(data)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(outputDir, "report.md"))
	if err != nil {
		t.Fatalf("failed to read report.md: %v", err)
	}

	// Should still render without errors
	if !strings.Contains(string(content), "no-comparison") {
		t.Errorf("report.md missing spec name")
	}
	if !strings.Contains(string(content), "only-branch") {
		t.Errorf("report.md missing variant branch")
	}
	// Should NOT contain Recommendation section
	if strings.Contains(string(content), "Recommendation") {
		t.Errorf("report.md should not contain Recommendation section when there's no comparison")
	}
}

func TestBoolToCheck(t *testing.T) {
	if boolToCheck(true) != "Yes" {
		t.Errorf("boolToCheck(true) should return 'Yes'")
	}
	if boolToCheck(false) != "No" {
		t.Errorf("boolToCheck(false) should return 'No'")
	}
}
