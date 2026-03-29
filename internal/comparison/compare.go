package comparison

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
)

// promptRunner is an interface for running custom prompts.
// This abstracts the Claude client to allow testing and future agent flexibility.
type promptRunner interface {
	RunCustomPrompt(ctx context.Context, prompt string) (*agents.RunResult, error)
}

// Comparator generates comparisons between variants using Claude.
type Comparator struct {
	promptRunner promptRunner
	customCmd    string // Empty for built-in Claude comparison
	maxRetries   int
}

// NewComparator creates a new Comparator.
// If customCmd is non-empty, it will be used instead of Claude for comparison.
func NewComparator(runner promptRunner, customCmd string) *Comparator {
	return &Comparator{
		promptRunner: runner,
		customCmd:    customCmd,
		maxRetries:   3,
	}
}

// DefaultTimeout is the maximum duration for a comparison session.
// This prevents indefinite hangs from stalled API connections.
const DefaultTimeout = 30 * time.Minute

// MaxPromptTokens is the maximum estimated token count for comparison prompts.
// Claude has ~200k token context, but we leave room for the response.
const MaxPromptTokens = 150000

// CompareUnified performs comparison with full control over what data is included.
// This is the recommended method - it always includes summaries and optionally includes diffs.
//
// If the agent execution or JSON validation fails but the agent managed to write
// the comparison JSON to OutputPath, the file is loaded as a fallback instead of
// returning an error.
func (c *Comparator) CompareUnified(ctx context.Context, input ComparisonInput) (*Result, error) {
	if len(input.Variants) < 2 {
		return nil, errors.New("at least 2 variants required for comparison")
	}

	// Custom command support is deferred - only Claude comparison is implemented
	if c.customCmd != "" {
		return nil, errors.New("custom comparison commands are not supported")
	}

	originalPrompt := buildComparisonPrompt(input)

	// Check if the prompt fits within context limits
	estimatedTokens := estimatePromptTokens(originalPrompt)
	if estimatedTokens > MaxPromptTokens {
		// If diffs are included and we're over limit, retry without diffs
		if input.IncludeDiff {
			log.Printf("Prompt too large with diffs (%d tokens), retrying without diffs", estimatedTokens)
			input.IncludeDiff = false
			originalPrompt = buildComparisonPrompt(input)
			estimatedTokens = estimatePromptTokens(originalPrompt)
		}

		// If still too large, fail
		if estimatedTokens > MaxPromptTokens {
			return nil, &DiffTooLargeError{
				EstimatedTokens: estimatedTokens,
				MaxTokens:       MaxPromptTokens,
			}
		}
	}

	// Record file state before comparison so we can detect if the agent wrote it
	var fileModTimeBefore time.Time
	if input.OutputPath != "" {
		if info, err := os.Stat(input.OutputPath); err == nil {
			fileModTimeBefore = info.ModTime()
		}
	}

	// Build the set of valid variant IDs from the actual input variants.
	// This ensures validation rejects IDs that aren't in the compared set
	// (e.g., variant 2 is rejected when only variants {1, 3} are compared).
	validIDs := make(map[int]bool, len(input.Variants))
	for _, v := range input.Variants {
		validIDs[v.ID] = true
	}

	result, err := c.runComparison(ctx, originalPrompt, validIDs)
	if err != nil && input.OutputPath != "" {
		// The agent may have written the comparison file before the session
		// failed (e.g., timeout, malformed response). Check for it.
		if fallback, loadErr := c.loadFallbackResult(input.OutputPath, fileModTimeBefore); loadErr == nil {
			log.Printf("Comparison failed (%v) but found written comparison file; using as fallback", err)
			return fallback, nil
		}
	}

	return result, err
}

// loadFallbackResult attempts to load a comparison result from a file that was
// written or updated after modTimeBefore. If modTimeBefore is zero (file didn't
// exist before), any existing file is accepted.
func (c *Comparator) loadFallbackResult(path string, modTimeBefore time.Time) (*Result, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("comparison file not found: %w", err)
	}

	// Only use the file if it was written/modified after we started the comparison
	if !modTimeBefore.IsZero() && !info.ModTime().After(modTimeBefore) {
		return nil, fmt.Errorf("comparison file was not updated during this run")
	}

	return LoadResultFromFile(path)
}

// runComparison executes the comparison prompt with retry logic.
func (c *Comparator) runComparison(ctx context.Context, originalPrompt string, validIDs map[int]bool) (*Result, error) {
	prompt := originalPrompt

	for attempt := 0; attempt < c.maxRetries; attempt++ {
		response, err := c.promptRunner.RunCustomPrompt(ctx, prompt)
		if err != nil {
			return nil, fmt.Errorf("claude execution failed: %w", err)
		}

		result, err := c.parseAndValidate(response.Output, validIDs)
		if err == nil {
			return result, nil
		}

		// On validation failure, retry with clarification
		if attempt < c.maxRetries-1 {
			log.Printf("Comparison JSON validation failed (attempt %d/%d): %v",
				attempt+1, c.maxRetries, err)
			prompt = fmt.Sprintf(`Your previous response was not valid JSON. Error: %s

Please provide the comparison result as valid JSON only, with no additional text.

---

%s`, err.Error(), originalPrompt)
		}
	}

	return nil, fmt.Errorf("comparison failed after %d attempts: JSON validation errors", c.maxRetries)
}

// DiffTooLargeError indicates that the combined diff size exceeds context limits.
type DiffTooLargeError struct {
	EstimatedTokens int
	MaxTokens       int
}

func (e *DiffTooLargeError) Error() string {
	return fmt.Sprintf("combined diff size (%d estimated tokens) exceeds context limit of %d",
		e.EstimatedTokens, e.MaxTokens)
}

// resultRaw mirrors Result but with Learnings as json.RawMessage to allow
// separate, more tolerant parsing of the learnings field.
type resultRaw struct {
	Recommendation           int                       `json:"recommendation"`
	Confidence               string                    `json:"confidence"`
	Summary                  string                    `json:"summary"`
	FileAnalyses             []FileAnalysis            `json:"file_analyses"`
	Observations             []string                  `json:"observations"`
	DocumentationAssessment  []DocAssessment           `json:"documentation_assessment,omitempty"`
	CrossVariantImprovements []CrossVariantImprovement `json:"cross_variant_improvements,omitempty"`
	Learnings                json.RawMessage           `json:"learnings,omitempty"`
}

// parseAndValidate extracts JSON from Claude response and validates structure.
func (c *Comparator) parseAndValidate(response string, validIDs map[int]bool) (*Result, error) {
	jsonStr, err := extractJSON(response)
	if err != nil {
		return nil, fmt.Errorf("extract JSON: %w", err)
	}

	// Use strict parsing for core fields
	decoder := json.NewDecoder(strings.NewReader(jsonStr))
	decoder.DisallowUnknownFields()

	var raw resultRaw
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	// Copy validated core fields to result
	result := &Result{
		Recommendation:           raw.Recommendation,
		Confidence:               raw.Confidence,
		Summary:                  raw.Summary,
		FileAnalyses:             raw.FileAnalyses,
		Observations:             raw.Observations,
		DocumentationAssessment:  raw.DocumentationAssessment,
		CrossVariantImprovements: raw.CrossVariantImprovements,
	}

	// Parse learnings separately with tolerant decoding [Req 6.4]
	// Type mismatches in learnings should not fail the entire comparison
	if len(raw.Learnings) > 0 && string(raw.Learnings) != "null" {
		var learnings []VariantLearning
		if err := json.Unmarshal(raw.Learnings, &learnings); err != nil {
			log.Printf("Warning: failed to parse learnings (non-fatal): %v", err)
			// Continue with empty learnings - graceful degradation
		} else {
			result.Learnings = learnings
		}
	}

	// Validate learnings (non-fatal) [Req 6.2, 6.4]
	// Invalid learnings are filtered out; valid ones are kept
	result.Learnings = validateLearnings(result.Learnings, validIDs)

	// Validate cross-variant improvements (non-fatal)
	// Invalid improvements are filtered out; valid ones are kept
	result.CrossVariantImprovements = validateCrossVariantImprovements(result.CrossVariantImprovements, validIDs)

	// Validate required fields against the actual variant ID set
	if !validIDs[result.Recommendation] {
		return nil, fmt.Errorf("recommendation %d is not in the compared variant set",
			result.Recommendation)
	}
	if result.Confidence == "" {
		return nil, errors.New("missing required field: confidence")
	}
	if result.Confidence != "high" && result.Confidence != "medium" && result.Confidence != "low" {
		return nil, fmt.Errorf("invalid confidence value: %s (must be 'high', 'medium', or 'low')", result.Confidence)
	}
	if result.Summary == "" {
		return nil, errors.New("missing required field: summary")
	}

	return result, nil
}

// extractJSON finds and extracts JSON from Claude's text response.
// Handles cases where JSON is wrapped in markdown code blocks.
func extractJSON(response string) (string, error) {
	response = strings.TrimSpace(response)

	// If it starts with {, assume it's plain JSON
	if strings.HasPrefix(response, "{") {
		// Find the matching closing brace
		return extractJSONObject(response)
	}

	// Look for JSON in markdown code blocks
	// Try ```json first
	if idx := strings.Index(response, "```json"); idx != -1 {
		start := idx + 7 // len("```json")
		end := strings.Index(response[start:], "```")
		if end != -1 {
			return strings.TrimSpace(response[start : start+end]), nil
		}
	}

	// Try plain ``` code blocks
	if idx := strings.Index(response, "```"); idx != -1 {
		start := idx + 3
		end := strings.Index(response[start:], "```")
		if end != -1 {
			content := strings.TrimSpace(response[start : start+end])
			if strings.HasPrefix(content, "{") {
				return content, nil
			}
		}
	}

	// Last resort: find any { and try to extract JSON object
	if idx := strings.Index(response, "{"); idx != -1 {
		return extractJSONObject(response[idx:])
	}

	return "", errors.New("no JSON found in response")
}

// validateLearnings filters learnings to include only valid entries and enforces limits.
// Invalid learnings are logged and discarded. Returns nil if all learnings are invalid.
func validateLearnings(learnings []VariantLearning, validIDs map[int]bool) []VariantLearning {
	if len(learnings) == 0 {
		return nil
	}

	valid := make([]VariantLearning, 0, len(learnings))
	validCategories := map[LearningCategory]bool{
		LearningCategoryCodePattern:   true,
		LearningCategoryArchitecture:  true,
		LearningCategoryTesting:       true,
		LearningCategoryErrorHandling: true,
	}

	// Track per-variant counts for limit enforcement
	variantCounts := make(map[int]int)

	for i, l := range learnings {
		// Trim whitespace from string fields
		l.Title = strings.TrimSpace(l.Title)
		l.Rationale = strings.TrimSpace(l.Rationale)
		l.Description = strings.TrimSpace(l.Description)

		// Check required fields [Req 6.3]
		if l.Title == "" {
			log.Printf("Discarding learning %d: missing title", i)
			continue
		}
		if l.Rationale == "" {
			log.Printf("Discarding learning %d: missing rationale", i)
			continue
		}
		if len(l.FileReferences) == 0 {
			log.Printf("Discarding learning %d: missing file_references", i)
			continue
		}

		// Validate variant ID against the actual compared variant set
		if !validIDs[l.VariantID] {
			log.Printf("Discarding learning %d: variant_id %d not in compared set", i, l.VariantID)
			continue
		}

		// Enforce per-variant limit
		if variantCounts[l.VariantID] >= MaxLearningsPerVariant {
			log.Printf("Discarding learning %d: variant %d already has %d learnings (max %d)",
				i, l.VariantID, variantCounts[l.VariantID], MaxLearningsPerVariant)
			continue
		}

		// Enforce total limit
		if len(valid) >= MaxLearningsTotal {
			log.Printf("Discarding learning %d: total learnings limit reached (%d)", i, MaxLearningsTotal)
			break
		}

		// Enforce file references limit
		if len(l.FileReferences) > MaxFileRefsPerLearning {
			log.Printf("Truncating file references for learning %d: %d -> %d",
				i, len(l.FileReferences), MaxFileRefsPerLearning)
			l.FileReferences = l.FileReferences[:MaxFileRefsPerLearning]
		}

		// Validate category (allow unknown for forward compatibility)
		if !validCategories[l.Category] {
			log.Printf("Learning %d has unknown category %q, using as-is", i, l.Category)
		}

		valid = append(valid, l)
		variantCounts[l.VariantID]++
	}

	if len(valid) == 0 {
		return nil
	}
	return valid
}

// validateCrossVariantImprovements filters improvements to include only entries
// with valid source variant IDs. Invalid entries are logged and discarded.
func validateCrossVariantImprovements(improvements []CrossVariantImprovement, validIDs map[int]bool) []CrossVariantImprovement {
	if len(improvements) == 0 {
		return improvements
	}

	valid := make([]CrossVariantImprovement, 0, len(improvements))
	for i, imp := range improvements {
		if !validIDs[imp.SourceVariantID] {
			log.Printf("Discarding cross-variant improvement %d: source_variant_id %d not in compared set", i, imp.SourceVariantID)
			continue
		}
		if imp.Description == "" {
			log.Printf("Discarding cross-variant improvement %d: missing description", i)
			continue
		}
		valid = append(valid, imp)
	}

	return valid
}

// LoadResultFromFile reads a comparison result JSON file and parses it into a Result.
// This allows recovering comparison results when the agent wrote the file but the session hung.
//
// Uses tolerant parsing identical to the live comparison path: malformed optional
// sections (e.g. learnings with wrong types) are discarded rather than failing the
// entire load. This is intentional — when recovering from a hung session, maximizing
// tolerance for the saved data is more important than strict validation.
func LoadResultFromFile(path string) (*Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read comparison file: %w", err)
	}

	content := strings.TrimSpace(string(data))
	if content == "" {
		return nil, errors.New("comparison file is empty")
	}

	// The file may contain raw JSON or JSON wrapped in markdown code blocks
	// (if the agent wrote it with extra formatting). Use the same extraction logic.
	jsonStr, err := extractJSON(content)
	if err != nil {
		return nil, fmt.Errorf("extract JSON from file: %w", err)
	}

	// Use tolerant parsing: unmarshal into resultRaw so malformed learnings
	// don't fail the entire load (same approach as parseAndValidate).
	var raw resultRaw
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil, fmt.Errorf("parse comparison JSON: %w", err)
	}

	result := &Result{
		Recommendation:           raw.Recommendation,
		Confidence:               raw.Confidence,
		Summary:                  raw.Summary,
		FileAnalyses:             raw.FileAnalyses,
		Observations:             raw.Observations,
		DocumentationAssessment:  raw.DocumentationAssessment,
		CrossVariantImprovements: raw.CrossVariantImprovements,
	}

	// Parse learnings tolerantly — type mismatches are discarded, not fatal.
	if len(raw.Learnings) > 0 && string(raw.Learnings) != "null" {
		var learnings []VariantLearning
		if err := json.Unmarshal(raw.Learnings, &learnings); err != nil {
			log.Printf("Warning: failed to parse learnings from file (non-fatal): %v", err)
		} else {
			result.Learnings = learnings
		}
	}

	// Basic validation
	if result.Recommendation < 1 {
		return nil, errors.New("invalid comparison result: missing recommendation")
	}
	if result.Confidence == "" {
		return nil, errors.New("invalid comparison result: missing confidence")
	}
	if result.Summary == "" {
		return nil, errors.New("invalid comparison result: missing summary")
	}

	return result, nil
}

// extractJSONObject extracts a JSON object starting at the beginning of the string.
// It uses brace matching while respecting string boundaries and escape sequences.
func extractJSONObject(s string) (string, error) {
	depth := 0
	inString := false
	escape := false

	for i, ch := range s {
		if escape {
			escape = false
			continue
		}
		if ch == '\\' && inString {
			escape = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch ch {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				extracted := s[:i+1]
				// Validate that extracted string is valid JSON
				if !json.Valid([]byte(extracted)) {
					return "", errors.New("extracted JSON is not valid")
				}
				return extracted, nil
			}
		}
	}

	return "", errors.New("unbalanced braces in JSON")
}
