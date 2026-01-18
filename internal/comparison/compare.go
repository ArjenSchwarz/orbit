package comparison

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/arjenschwarz/orbit/internal/agents"
)

// promptRunner is an interface for running custom prompts.
// This abstracts the Claude client to allow testing and future agent flexibility.
type promptRunner interface {
	RunCustomPrompt(prompt string) (*agents.RunResult, error)
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

// MaxPromptTokens is the maximum estimated token count for comparison prompts.
// Claude has ~200k token context, but we leave room for the response.
const MaxPromptTokens = 150000

// Compare analyzes variants and returns structured results.
// Uses JSON validation with retry on malformed responses.
func (c *Comparator) Compare(ctx context.Context, specName string, variants []VariantData) (*Result, error) {
	if len(variants) < 2 {
		return nil, errors.New("at least 2 variants required for comparison")
	}

	// Validate that variants have non-empty diffs
	for _, v := range variants {
		if strings.TrimSpace(v.Diff) == "" {
			return nil, fmt.Errorf("variant %d has empty diff", v.ID)
		}
	}

	// Custom command support is deferred - only Claude comparison is implemented
	if c.customCmd != "" {
		return nil, errors.New("custom comparison commands are not supported")
	}

	originalPrompt := buildPrompt(specName, variants)

	// Check that the prompt fits within context limits (Requirement 5.8)
	estimatedTokens := estimatePromptTokens(originalPrompt)
	if estimatedTokens > MaxPromptTokens {
		return nil, fmt.Errorf("combined diff size (%d estimated tokens) exceeds context limit of %d; variants are too large to compare",
			estimatedTokens, MaxPromptTokens)
	}
	prompt := originalPrompt

	for attempt := 0; attempt < c.maxRetries; attempt++ {
		response, err := c.promptRunner.RunCustomPrompt(prompt)
		if err != nil {
			return nil, fmt.Errorf("claude execution failed: %w", err)
		}

		result, err := c.parseAndValidate(response.Output, len(variants))
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

// parseAndValidate extracts JSON from Claude response and validates structure.
func (c *Comparator) parseAndValidate(response string, numVariants int) (*Result, error) {
	jsonStr, err := extractJSON(response)
	if err != nil {
		return nil, fmt.Errorf("extract JSON: %w", err)
	}

	// Use strict parsing to catch unknown fields
	decoder := json.NewDecoder(strings.NewReader(jsonStr))
	decoder.DisallowUnknownFields()

	var result Result
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	// Validate required fields with range checks
	if result.Recommendation < 1 || result.Recommendation > numVariants {
		return nil, fmt.Errorf("recommendation must be between 1 and %d, got %d",
			numVariants, result.Recommendation)
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

	return &result, nil
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
