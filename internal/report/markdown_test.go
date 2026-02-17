package report

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/arjenschwarz/orbit/internal/comparison"
)

func TestGenerateMarkdownReport_IncludesLearnings(t *testing.T) {
	// Create temporary directory for output
	tmpDir, err := os.MkdirTemp("", "report-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("Failed to remove temp dir: %v", err)
		}
	}()

	gen := NewGenerator(tmpDir)

	// Create test data with learnings
	data := &ReportData{
		SpecName:    "test-spec",
		BaseCommit:  "abc1234",
		GeneratedAt: time.Now(),
		Variants: []VariantReportData{
			{ID: 1, Branch: "feature/v1"},
		},
		Comparison: &comparison.Result{
			Recommendation: 1,
			Confidence:     "high",
			Summary:        "Test summary",
			Learnings: []comparison.VariantLearning{
				{
					VariantID:      1,
					Category:       comparison.LearningCategoryCodePattern,
					Title:          "Use sync.Pool",
					Description:    "Reduces GC pressure",
					Rationale:      "Performance optimization",
					FileReferences: []string{"pool.go:42"},
				},
			},
		},
	}

	// Generate report
	if err := gen.generateMarkdownReport(data); err != nil {
		t.Fatalf("generateMarkdownReport failed: %v", err)
	}

	// Read generated file
	contentBytes, err := os.ReadFile(filepath.Join(tmpDir, "report.md"))
	if err != nil {
		t.Fatalf("Failed to read report.md: %v", err)
	}
	content := string(contentBytes)

	// Verify learnings section exists
	assert.Contains(t, content, "# Learnings", "Report should contain 'Learnings' section")

	// Verify disclaimer
	assert.Contains(t, content, "snapshot from the time of analysis", "Report should contain staleness disclaimer")

	// Verify learning content
	assert.Contains(t, content, "Use sync.Pool", "Report should contain learning title")
	assert.Contains(t, content, "Performance optimization", "Report should contain learning rationale")
	assert.Contains(t, content, "`pool.go:42`", "Report should contain file reference")
}

func TestGenerateMarkdownReport_OmitsEmptyLearnings(t *testing.T) {
	// Create temporary directory for output
	tmpDir, err := os.MkdirTemp("", "report-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("Failed to remove temp dir: %v", err)
		}
	}()

	gen := NewGenerator(tmpDir)

	// Create test data without learnings
	data := &ReportData{
		SpecName:    "test-spec",
		BaseCommit:  "abc1234",
		GeneratedAt: time.Now(),
		Variants: []VariantReportData{
			{ID: 1, Branch: "feature/v1"},
		},
		Comparison: &comparison.Result{
			Recommendation: 1,
			Confidence:     "high",
			Summary:        "Test summary",
			Learnings:      nil, // No learnings
		},
	}

	// Generate report
	if err := gen.generateMarkdownReport(data); err != nil {
		t.Fatalf("generateMarkdownReport failed: %v", err)
	}

	// Read generated file
	contentBytes, err := os.ReadFile(filepath.Join(tmpDir, "report.md"))
	if err != nil {
		t.Fatalf("Failed to read report.md: %v", err)
	}
	content := string(contentBytes)

	// Verify learnings section does NOT exist
	assert.NotContains(t, content, "## Learnings", "Report should NOT contain 'Learnings' section when no learnings exist")
}

func TestGenerateMarkdownReport_GroupsLearningsByVariant(t *testing.T) {
	// Create temporary directory for output
	tmpDir, err := os.MkdirTemp("", "report-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("Failed to remove temp dir: %v", err)
		}
	}()

	gen := NewGenerator(tmpDir)

	// Create test data with learnings from multiple variants
	data := &ReportData{
		SpecName:    "test-spec",
		BaseCommit:  "abc1234",
		GeneratedAt: time.Now(),
		Variants: []VariantReportData{
			{ID: 1, Branch: "feature/v1"},
			{ID: 2, Branch: "feature/v2"},
		},
		Comparison: &comparison.Result{
			Recommendation: 1,
			Confidence:     "high",
			Summary:        "Test summary",
			Learnings: []comparison.VariantLearning{
				{
					VariantID:      1,
					Category:       comparison.LearningCategoryCodePattern,
					Title:          "V1 Pattern",
					Rationale:      "V1 rationale",
					FileReferences: []string{"v1.go:10"},
				},
				{
					VariantID:      2,
					Category:       comparison.LearningCategoryTesting,
					Title:          "V2 Testing",
					Rationale:      "V2 rationale",
					FileReferences: []string{"v2_test.go:20"},
				},
			},
		},
	}

	// Generate report
	if err := gen.generateMarkdownReport(data); err != nil {
		t.Fatalf("generateMarkdownReport failed: %v", err)
	}

	// Read generated file
	contentBytes, err := os.ReadFile(filepath.Join(tmpDir, "report.md"))
	if err != nil {
		t.Fatalf("Failed to read report.md: %v", err)
	}
	content := string(contentBytes)

	// Verify both variant sections exist
	assert.Contains(t, content, "### Variant 1", "Report should contain 'Variant 1' section")
	assert.Contains(t, content, "### Variant 2", "Report should contain 'Variant 2' section")

	// Verify variant 1 content
	assert.Contains(t, content, "V1 Pattern", "Report should contain V1 Pattern learning")

	// Verify variant 2 content
	assert.Contains(t, content, "V2 Testing", "Report should contain V2 Testing learning")
}

func TestGenerateMarkdownReport_IncludesCategoryBadges(t *testing.T) {
	// Create temporary directory for output
	tmpDir, err := os.MkdirTemp("", "report-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("Failed to remove temp dir: %v", err)
		}
	}()

	gen := NewGenerator(tmpDir)

	// Create test data with different categories
	data := &ReportData{
		SpecName:    "test-spec",
		BaseCommit:  "abc1234",
		GeneratedAt: time.Now(),
		Variants: []VariantReportData{
			{ID: 1, Branch: "feature/v1"},
		},
		Comparison: &comparison.Result{
			Recommendation: 1,
			Confidence:     "high",
			Summary:        "Test summary",
			Learnings: []comparison.VariantLearning{
				{
					VariantID:      1,
					Category:       comparison.LearningCategoryArchitecture,
					Title:          "Dependency Injection",
					Rationale:      "Improves testability",
					FileReferences: []string{"di.go:10"},
				},
			},
		},
	}

	// Generate report
	if err := gen.generateMarkdownReport(data); err != nil {
		t.Fatalf("generateMarkdownReport failed: %v", err)
	}

	// Read generated file
	contentBytes, err := os.ReadFile(filepath.Join(tmpDir, "report.md"))
	if err != nil {
		t.Fatalf("Failed to read report.md: %v", err)
	}
	content := string(contentBytes)

	// Verify category badge format
	assert.Contains(t, content, "[architecture]", "Report should contain [architecture] category badge")
}
