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
					Cost:     &cost.Totals{USD: 0.0523},
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
					Cost:     &cost.Totals{USD: 0.0312},
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
					Cost:     &cost.Totals{USD: 0.0523},
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
					Cost:     &cost.Totals{USD: 0.0312},
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

func TestGenerate_MarkdownLearningsSection(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "report")

	gen := NewGenerator(outputDir)
	data := &ReportData{
		SpecName:       "learnings-test",
		GeneratedAt:    time.Now(),
		BaseCommit:     "abc123",
		OriginalBranch: "main",
		Variants: []VariantReportData{
			{
				ID:     1,
				Branch: "test-branch",
				Status: "completed",
				Diff:   "+code",
			},
			{
				ID:     2,
				Branch: "test-branch-2",
				Status: "completed",
				Diff:   "+more code",
			},
		},
		Comparison: &comparison.Result{
			Recommendation: 1,
			Confidence:     "high",
			Summary:        "Variant 1 is better.",
			Learnings: []comparison.VariantLearning{
				{
					VariantID:      1,
					Category:       comparison.LearningCategoryCodePattern,
					Title:          "Table-driven tests",
					Description:    "Uses table-driven tests for comprehensive coverage.",
					Rationale:      "Makes it easy to add new test cases.",
					FileReferences: []string{"foo_test.go:42", "bar_test.go:100"},
				},
				{
					VariantID:      1,
					Category:       comparison.LearningCategoryErrorHandling,
					Title:          "Sentinel errors",
					Description:    "Uses sentinel errors with errors.Is().",
					Rationale:      "Type-safe error checking across packages.",
					FileReferences: []string{"errors.go:10"},
				},
				{
					VariantID:      2,
					Category:       comparison.LearningCategoryArchitecture,
					Title:          "Clean separation",
					Description:    "Separates business logic from infrastructure.",
					Rationale:      "Easier testing and better maintainability.",
					FileReferences: []string{"service.go:1", "repo.go:1"},
				},
			},
		},
	}

	err := gen.Generate(data)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(outputDir, "report.md"))
	if err != nil {
		t.Fatalf("failed to read report.md: %v", err)
	}

	contentStr := string(content)

	// Verify Learnings section is present [Req 3.1]
	// go-output uses # for top-level sections
	if !strings.Contains(contentStr, "# Learnings") {
		t.Errorf("report.md missing Learnings section header")
	}

	// Verify learnings are grouped by variant [Req 3.2]
	if !strings.Contains(contentStr, "### Variant 1") {
		t.Errorf("report.md missing Variant 1 heading in Learnings")
	}
	if !strings.Contains(contentStr, "### Variant 2") {
		t.Errorf("report.md missing Variant 2 heading in Learnings")
	}

	// Verify learning content is present [Req 3.3]
	checks := []string{
		"[code-pattern]",         // category badge
		"Table-driven tests",     // title
		"table-driven tests for", // description (partial)
		"easy to add new test",   // rationale (partial)
		"`foo_test.go:42`",       // file reference in backticks
		"`bar_test.go:100`",      // another file reference
		"[error-handling]",       // second learning category
		"Sentinel errors",        // second learning title
		"[architecture]",         // variant 2 learning category
		"Clean separation",       // variant 2 learning title
	}
	for _, check := range checks {
		if !strings.Contains(contentStr, check) {
			t.Errorf("report.md missing expected learnings content: %q", check)
		}
	}

	// Verify file references are rendered as relative paths [Req 3.4]
	// They should be in backticks, not as clickable links
	if strings.Contains(contentStr, "[foo_test.go:42]") {
		t.Errorf("file references should not be markdown links")
	}

	// Verify stale reference disclaimer is present [Req 3.6]
	if !strings.Contains(contentStr, "File references are a snapshot") {
		t.Errorf("report.md missing stale reference disclaimer")
	}
	if !strings.Contains(contentStr, "may become outdated if code changes") {
		t.Errorf("report.md missing full disclaimer text")
	}
}

func TestGenerate_MarkdownNoLearnings(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "report")

	gen := NewGenerator(outputDir)
	data := &ReportData{
		SpecName:       "no-learnings",
		GeneratedAt:    time.Now(),
		BaseCommit:     "abc123",
		OriginalBranch: "main",
		Variants: []VariantReportData{
			{
				ID:     1,
				Branch: "test-branch",
				Status: "completed",
				Diff:   "+code",
			},
		},
		Comparison: &comparison.Result{
			Recommendation: 1,
			Confidence:     "high",
			Summary:        "Variant 1 is the only one.",
			Learnings:      nil, // No learnings
		},
	}

	err := gen.Generate(data)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(outputDir, "report.md"))
	if err != nil {
		t.Fatalf("failed to read report.md: %v", err)
	}

	contentStr := string(content)

	// Verify Learnings section is NOT present when no learnings [Req 3.5]
	if strings.Contains(contentStr, "# Learnings") {
		t.Errorf("report.md should not contain Learnings section when there are no learnings")
	}
}

func TestGenerate_MarkdownEmptyLearnings(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "report")

	gen := NewGenerator(outputDir)
	data := &ReportData{
		SpecName:       "empty-learnings",
		GeneratedAt:    time.Now(),
		BaseCommit:     "abc123",
		OriginalBranch: "main",
		Variants: []VariantReportData{
			{
				ID:     1,
				Branch: "test-branch",
				Status: "completed",
				Diff:   "+code",
			},
		},
		Comparison: &comparison.Result{
			Recommendation: 1,
			Confidence:     "high",
			Summary:        "Variant 1 is the only one.",
			Learnings:      []comparison.VariantLearning{}, // Empty slice
		},
	}

	err := gen.Generate(data)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(outputDir, "report.md"))
	if err != nil {
		t.Fatalf("failed to read report.md: %v", err)
	}

	contentStr := string(content)

	// Verify Learnings section is NOT present when learnings slice is empty [Req 3.5]
	if strings.Contains(contentStr, "# Learnings") {
		t.Errorf("report.md should not contain Learnings section when learnings slice is empty")
	}
}

func TestGenerate_MarkdownLearningsAfterImprovements(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "report")

	gen := NewGenerator(outputDir)
	data := &ReportData{
		SpecName:       "ordering-test",
		GeneratedAt:    time.Now(),
		BaseCommit:     "abc123",
		OriginalBranch: "main",
		Variants: []VariantReportData{
			{
				ID:     1,
				Branch: "test-branch",
				Status: "completed",
				Diff:   "+code",
			},
		},
		Comparison: &comparison.Result{
			Recommendation: 1,
			Confidence:     "high",
			Summary:        "Test summary.",
			CrossVariantImprovements: []comparison.CrossVariantImprovement{
				{
					SourceVariantID: 2,
					Description:     "Better error messages",
					Rationale:       "Helps debugging",
					Priority:        "medium",
				},
			},
			Learnings: []comparison.VariantLearning{
				{
					VariantID:      1,
					Category:       comparison.LearningCategoryTesting,
					Title:          "Mock patterns",
					Description:    "Uses interface-based mocking.",
					Rationale:      "Easy to test in isolation.",
					FileReferences: []string{"mock.go:1"},
				},
			},
		},
	}

	err := gen.Generate(data)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(outputDir, "report.md"))
	if err != nil {
		t.Fatalf("failed to read report.md: %v", err)
	}

	contentStr := string(content)

	// Verify both sections exist
	improvementsIdx := strings.Index(contentStr, "Improvements from Other Variants")
	learningsIdx := strings.Index(contentStr, "# Learnings")
	diffsIdx := strings.Index(contentStr, "# Implementation Diffs")

	if improvementsIdx == -1 {
		t.Fatal("report.md missing Improvements section")
	}
	if learningsIdx == -1 {
		t.Fatal("report.md missing Learnings section")
	}
	if diffsIdx == -1 {
		t.Fatal("report.md missing Implementation Diffs section")
	}

	// Verify Learnings comes after Improvements [Req 3.1]
	if learningsIdx < improvementsIdx {
		t.Errorf("Learnings section should come after Improvements from Other Variants section")
	}

	// Verify Learnings comes before Implementation Diffs
	if learningsIdx > diffsIdx {
		t.Errorf("Learnings section should come before Implementation Diffs section")
	}
}

func TestGenerate_HTMLLearningsXSSEscaping(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "report")

	gen := NewGenerator(outputDir)
	data := &ReportData{
		SpecName:       "xss-learnings-test",
		GeneratedAt:    time.Now(),
		BaseCommit:     "abc123",
		OriginalBranch: "main",
		Variants: []VariantReportData{
			{
				ID:     1,
				Branch: "test-branch",
				Status: "completed",
				Diff:   "+code",
			},
		},
		Comparison: &comparison.Result{
			Recommendation: 1,
			Confidence:     "high",
			Summary:        "Test summary.",
			Learnings: []comparison.VariantLearning{
				{
					VariantID:      1,
					Category:       comparison.LearningCategoryCodePattern,
					Title:          "<script>alert('xss-title')</script>",
					Description:    "Test <b>bold</b> injection",
					Rationale:      "Reason with \"quotes\" and <img src=x>",
					FileReferences: []string{"<path>/file.go:42", "normal.go:10"},
				},
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

	contentStr := string(content)

	// Verify dangerous HTML tags are escaped in learnings [Req 4.7]
	dangerous := []string{
		"<script>alert",      // Title injection
		"</script>",          // Script closing
		"<b>bold</b>",        // Description HTML
		"<img src=x>",        // Rationale img tag
		"<path>",             // File reference HTML
	}
	for _, d := range dangerous {
		if strings.Contains(contentStr, d) {
			t.Errorf("index.html contains unescaped dangerous learnings content: %q", d)
		}
	}

	// Verify escaped versions are present
	escaped := []string{
		"&lt;script&gt;",  // Escaped script tag
		"&lt;b&gt;",       // Escaped bold tag
		"&lt;img",         // Escaped img tag
		"&lt;path&gt;",    // Escaped path tag in file reference
	}
	for _, e := range escaped {
		if !strings.Contains(contentStr, e) {
			t.Errorf("index.html missing escaped learnings content: %q", e)
		}
	}
}

func TestGenerate_HTMLLearningsSection(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "report")

	gen := NewGenerator(outputDir)
	data := &ReportData{
		SpecName:       "html-learnings-test",
		GeneratedAt:    time.Now(),
		BaseCommit:     "abc123",
		OriginalBranch: "main",
		Variants: []VariantReportData{
			{
				ID:     1,
				Branch: "test-branch",
				Status: "completed",
				Diff:   "+code",
			},
			{
				ID:     2,
				Branch: "test-branch-2",
				Status: "completed",
				Diff:   "+more code",
			},
		},
		Comparison: &comparison.Result{
			Recommendation: 1,
			Confidence:     "high",
			Summary:        "Variant 1 is better.",
			Learnings: []comparison.VariantLearning{
				{
					VariantID:      1,
					Category:       comparison.LearningCategoryCodePattern,
					Title:          "Table-driven tests",
					Description:    "Uses table-driven tests for comprehensive coverage.",
					Rationale:      "Makes it easy to add new test cases.",
					FileReferences: []string{"foo_test.go:42", "bar_test.go:100"},
				},
				{
					VariantID:      2,
					Category:       comparison.LearningCategoryArchitecture,
					Title:          "Clean separation",
					Description:    "Separates business logic from infrastructure.",
					Rationale:      "Easier testing and better maintainability.",
					FileReferences: []string{"service.go:1"},
				},
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

	contentStr := string(content)

	// Verify Learnings section is present [Req 4.1]
	if !strings.Contains(contentStr, `class="learnings"`) {
		t.Errorf("index.html missing learnings section")
	}
	if !strings.Contains(contentStr, "<h2>Learnings</h2>") {
		t.Errorf("index.html missing Learnings heading")
	}

	// Verify category badges are present [Req 4.2]
	if !strings.Contains(contentStr, `class="category-badge category-code-pattern"`) {
		t.Errorf("index.html missing code-pattern category badge class")
	}
	if !strings.Contains(contentStr, `class="category-badge category-architecture"`) {
		t.Errorf("index.html missing architecture category badge class")
	}

	// Verify learnings are grouped by variant [Req 4.3]
	if !strings.Contains(contentStr, `class="variant-learnings"`) {
		t.Errorf("index.html missing variant-learnings class")
	}

	// Verify file references are in monospace (code tags) [Req 4.4]
	if !strings.Contains(contentStr, "<code>foo_test.go:42</code>") {
		t.Errorf("index.html missing file reference in code tag")
	}

	// Verify learning content
	checks := []string{
		"Table-driven tests",
		"Uses table-driven tests",
		"Makes it easy to add new",
		"Clean separation",
		"service.go:1",
	}
	for _, check := range checks {
		if !strings.Contains(contentStr, check) {
			t.Errorf("index.html missing learnings content: %q", check)
		}
	}

	// Verify stale reference disclaimer is present
	if !strings.Contains(contentStr, "File references are a snapshot") {
		t.Errorf("index.html missing stale reference disclaimer")
	}
}

func TestGenerate_HTMLNoLearnings(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "report")

	gen := NewGenerator(outputDir)
	data := &ReportData{
		SpecName:       "no-learnings",
		GeneratedAt:    time.Now(),
		BaseCommit:     "abc123",
		OriginalBranch: "main",
		Variants: []VariantReportData{
			{
				ID:     1,
				Branch: "test-branch",
				Status: "completed",
				Diff:   "+code",
			},
		},
		Comparison: &comparison.Result{
			Recommendation: 1,
			Confidence:     "high",
			Summary:        "Variant 1 is the only one.",
			Learnings:      nil, // No learnings
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

	// Verify Learnings section is NOT present when no learnings [Req 4.5]
	if strings.Contains(contentStr, `class="learnings"`) {
		t.Errorf("index.html should not contain learnings section when there are no learnings")
	}
	if strings.Contains(contentStr, "<h2>Learnings</h2>") {
		t.Errorf("index.html should not contain Learnings heading when there are no learnings")
	}
}
