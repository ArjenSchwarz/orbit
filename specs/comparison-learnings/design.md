# Design: Comparison Learnings

## Overview

This design describes the implementation of a "Learnings" section in Orbit's multi-variant comparison reports. The feature extracts educational insights from each variant's implementation, helping developers learn from different approaches regardless of which variant is recommended.

The implementation follows the existing comparison pipeline pattern: data types define the structure, the AI prompt requests learnings, the comparator validates the response, and the report generators render the output in HTML and Markdown formats.

## Architecture

The learnings feature integrates into the existing comparison pipeline:

```
┌──────────────────┐    ┌──────────────────┐    ┌──────────────────┐
│  Prompt Builder  │───▶│    Comparator    │───▶│ Report Generator │
│  (prompt.go)     │    │   (compare.go)   │    │  (generator.go)  │
└──────────────────┘    └──────────────────┘    └──────────────────┘
        │                        │                       │
        ▼                        ▼                       ▼
   Add learnings           Validate &              Render learnings
   instructions to      filter individual         in HTML/Markdown
   JSON schema          learnings [Req 6]         [Req 3, 4, 7]
```

**Data Flow:**

1. `buildComparisonPrompt()` includes learnings instructions and JSON schema [Req 2]
2. AI generates comparison result with `learnings` array
3. `parseAndValidate()` parses JSON; invalid learnings are filtered [Req 6.3, 6.5]
4. `comparison.Result` includes validated `Learnings []VariantLearning` [Req 1.4]
5. `Generator.Generate()` renders learnings in HTML and Markdown [Req 3, 4]
6. Static HTML is served by web interface [Req 7]

## Components and Interfaces

### 1. VariantLearning Struct [Req 1.1-1.5]

Location: `internal/comparison/types.go`

```go
// LearningCategory defines the type of learning.
type LearningCategory string

const (
    LearningCategoryCodePattern   LearningCategory = "code-pattern"
    LearningCategoryArchitecture  LearningCategory = "architecture"
    LearningCategoryTesting       LearningCategory = "testing"
    LearningCategoryErrorHandling LearningCategory = "error-handling"
)

// Learnings limits to prevent unbounded AI output.
const (
    MaxLearningsPerVariant = 5  // Maximum learnings per variant
    MaxLearningsTotal      = 20 // Maximum total learnings across all variants
    MaxFileRefsPerLearning = 5  // Maximum file references per learning
)

// VariantLearning represents an educational insight from a variant's implementation.
type VariantLearning struct {
    VariantID      int              `json:"variant_id"`
    Category       LearningCategory `json:"category"`
    Title          string           `json:"title"`
    Description    string           `json:"description"`
    Rationale      string           `json:"rationale"`
    FileReferences []string         `json:"file_references"` // e.g., "path/to/file.go:123"
}
```

**Modification to Result struct:**

```go
type Result struct {
    // ... existing fields ...

    // Learnings extracted from each variant's implementation [Req 1.4]
    Learnings []VariantLearning `json:"learnings,omitempty"`
}
```

### 2. Prompt Updates [Req 2.1-2.6, 5.1-5.4]

Location: `internal/comparison/prompt.go`

**JSON Schema Addition:**

```go
// Add to jsonSchema constant
"learnings": [
    {
      "variant_id": <number: which variant this learning is from>,
      "category": "<string: 'code-pattern', 'architecture', 'testing', or 'error-handling'>",
      "title": "<string: brief title for the learning (5-10 words)>",
      "description": "<string: what the pattern/technique is>",
      "rationale": "<string: why this matters and how it could be applied elsewhere>",
      "file_references": ["<string: path/to/file.go:123>"]
    }
  ]
```

**Prompt Instructions Addition:**

Add a new section in `buildComparisonPrompt()` after "Cross-Variant Improvements":

```go
sb.WriteString("### Developer Learnings\n\n")
sb.WriteString("Identify educational insights from EACH variant that could help the user become a better developer.\n")
sb.WriteString("Focus on techniques that are transferable to other projects.\n\n")

sb.WriteString("**Categories:**\n")
sb.WriteString("- `code-pattern`: Idiomatic code, clever algorithms, elegant solutions\n")
sb.WriteString("- `architecture`: Structural decisions, module organization, separation of concerns\n")
sb.WriteString("- `testing`: Test approaches, coverage patterns, mocking techniques\n")
sb.WriteString("- `error-handling`: Defensive coding, edge case handling, resilience patterns\n\n")

sb.WriteString("**For each learning:**\n")
sb.WriteString("- Include specific file references (path/to/file.go:123)\n")
sb.WriteString("- Explain WHY this pattern matters (the broader principle)\n")
sb.WriteString("- Focus on techniques the user could apply in future projects\n\n")

sb.WriteString("**Exclude trivial observations like:**\n")
sb.WriteString("- \"Uses comments\" or \"Has tests\"\n")
sb.WriteString("- Generic observations that apply to any codebase\n")
sb.WriteString("- Implementation details without educational value\n\n")

sb.WriteString("**Good learning examples:**\n")
sb.WriteString("- \"Uses table-driven tests with map[string]struct for unique test case names\"\n")
sb.WriteString("- \"Implements the functional options pattern for flexible configuration\"\n")
sb.WriteString("- \"Uses sentinel errors with errors.Is() for type-safe error checking\"\n\n")

sb.WriteString("**Bad learning examples (too trivial):**\n")
sb.WriteString("- \"Code is well-formatted\"\n")
sb.WriteString("- \"Functions have descriptive names\"\n")
sb.WriteString("- \"Uses if statements for control flow\"\n\n")

sb.WriteString("**Limits:** Provide the most important learnings only. Aim for 3-5 per variant, maximum 5.\n\n")
```

### 3. Validation Updates [Req 6.1-6.5]

Location: `internal/comparison/compare.go`

Add a function to validate and filter learnings:

```go
// validateLearnings filters learnings to include only valid entries and enforces limits.
// Invalid learnings are logged and discarded. Returns nil if all learnings are invalid.
func validateLearnings(learnings []VariantLearning, numVariants int) []VariantLearning {
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

        // Validate variant ID
        if l.VariantID < 1 || l.VariantID > numVariants {
            log.Printf("Discarding learning %d: invalid variant_id %d", i, l.VariantID)
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
```

Modify `parseAndValidate()` to call this after JSON parsing:

```go
func (c *Comparator) parseAndValidate(response string, numVariants int) (*Result, error) {
    // ... existing JSON parsing ...

    // Validate learnings (non-fatal) [Req 6.2, 6.4]
    result.Learnings = validateLearnings(result.Learnings, numVariants)

    // ... existing required field validation ...
}
```

### 4. Markdown Rendering [Req 3.1-3.6]

Location: `internal/report/markdown.go`

Add learnings section after cross-variant improvements:

```go
// In generateMarkdownReport(), after CrossVariantImprovements section:

// Learnings section [Req 3.1]
if data.Comparison != nil && len(data.Comparison.Learnings) > 0 {
    builder.Section("Learnings", func(b *output.Builder) {
        // Disclaimer [Req 3.6]
        b.Text("*Note: File references are a snapshot from the time of analysis and may become outdated if code changes.*")

        // Group learnings by variant [Req 3.2]
        learningsByVariant := groupLearningsByVariant(data.Comparison.Learnings)

        // Render in variant order
        var variantIDs []int
        for id := range learningsByVariant {
            variantIDs = append(variantIDs, id)
        }
        slices.Sort(variantIDs)

        for _, variantID := range variantIDs {
            learnings := learningsByVariant[variantID]
            b.Section(fmt.Sprintf("Variant %d", variantID), func(sb *output.Builder) {
                for _, l := range learnings {
                    // Category badge + title [Req 3.3]
                    sb.Raw(output.FormatMarkdown, []byte(fmt.Sprintf(
                        "#### [%s] %s\n\n", l.Category, l.Title)))

                    // Description
                    sb.Raw(output.FormatMarkdown, []byte(l.Description+"\n\n"))

                    // Rationale
                    sb.Raw(output.FormatMarkdown, []byte(fmt.Sprintf(
                        "**Why it matters:** %s\n\n", l.Rationale)))

                    // File references [Req 3.4]
                    sb.Raw(output.FormatMarkdown, []byte("**Files:** "))
                    refs := make([]string, len(l.FileReferences))
                    for i, ref := range l.FileReferences {
                        refs[i] = fmt.Sprintf("`%s`", ref)
                    }
                    sb.Raw(output.FormatMarkdown, []byte(strings.Join(refs, ", ")+"\n\n"))
                }
            }, output.WithLevel(3))
        }
    })
}
```

### 5. Shared Helper Function

Location: `internal/comparison/learnings.go`

To avoid code duplication, the `GroupLearningsByVariant` function is defined once in the comparison package and used by both Markdown and HTML renderers:

```go
// GroupLearningsByVariant organizes learnings by their variant ID.
// Returns a map with deterministic iteration order via sorted keys.
func GroupLearningsByVariant(learnings []VariantLearning) map[int][]VariantLearning {
    result := make(map[int][]VariantLearning)
    for _, l := range learnings {
        result[l.VariantID] = append(result[l.VariantID], l)
    }
    return result
}

// SortedVariantIDs returns variant IDs from a learnings map in sorted order.
func SortedVariantIDs(learningsByVariant map[int][]VariantLearning) []int {
    ids := make([]int, 0, len(learningsByVariant))
    for id := range learningsByVariant {
        ids = append(ids, id)
    }
    slices.Sort(ids)
    return ids
}
```

The Markdown renderer calls these directly. The HTML template uses a template function that delegates to `GroupLearningsByVariant`.

### 6. HTML Rendering [Req 4.1-4.7]

Location: `internal/report/templates/index.html`

Add after the cross-variant-improvements section:

```html
{{if .Comparison}}
{{if .Comparison.Learnings}}
<section class="learnings">
    <h2>Learnings</h2>
    <p class="learnings-disclaimer">Note: File references are a snapshot from the time of analysis and may become outdated if code changes.</p>

    {{range $variantID, $learnings := groupLearningsByVariant .Comparison.Learnings}}
    <div class="variant-learnings">
        <h3>Variant {{$variantID}}</h3>
        <div class="learnings-list">
            {{range $learnings}}
            <div class="learning">
                <div class="learning-header">
                    <span class="category-badge category-{{.Category}}">{{.Category}}</span>
                    <span class="learning-title">{{.Title}}</span>
                </div>
                <p class="learning-description">{{.Description}}</p>
                <p class="learning-rationale"><strong>Why it matters:</strong> {{.Rationale}}</p>
                <div class="learning-files">
                    <strong>Files:</strong>
                    {{range .FileReferences}}
                    <code>{{.}}</code>
                    {{end}}
                </div>
            </div>
            {{end}}
        </div>
    </div>
    {{end}}
</section>
{{end}}
{{end}}
```

**Template Function:**

Add to `internal/report/templates.go`:

```go
var templateFuncs = template.FuncMap{
    // ... existing functions ...
    "groupLearningsByVariant": func(learnings []comparison.VariantLearning) map[int][]comparison.VariantLearning {
        result := make(map[int][]comparison.VariantLearning)
        for _, l := range learnings {
            result[l.VariantID] = append(result[l.VariantID], l)
        }
        return result
    },
}
```

### 6. CSS Styling [Req 4.2, 4.4, 4.6]

Location: `internal/report/templates/style.css`

```css
/* Learnings section */
.learnings {
    margin-bottom: 2rem;
}

.learnings-disclaimer {
    font-size: 0.9rem;
    color: var(--text-secondary);
    font-style: italic;
    margin-bottom: 1.5rem;
}

.variant-learnings {
    margin-bottom: 1.5rem;
}

.variant-learnings h3 {
    margin-bottom: 1rem;
}

.learnings-list {
    display: flex;
    flex-direction: column;
    gap: 1rem;
}

.learning {
    background-color: var(--bg-card);
    border: 1px solid var(--border-color);
    border-radius: 6px;
    padding: 1rem;
}

.learning-header {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    margin-bottom: 0.75rem;
    flex-wrap: wrap;
}

.learning-title {
    font-weight: 600;
    font-size: 1.05rem;
}

/* Category badges [Req 4.2] */
.category-badge {
    display: inline-block;
    padding: 0.2rem 0.6rem;
    border-radius: 4px;
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.03em;
    /* Default styling for unknown categories */
    background-color: rgba(128, 128, 128, 0.1);
    color: var(--text-secondary);
}

.category-code-pattern {
    background-color: rgba(13, 110, 253, 0.1);
    color: var(--info-color);
}

.category-architecture {
    background-color: rgba(111, 66, 193, 0.1);
    color: #6f42c1;
}

.category-testing {
    background-color: rgba(25, 135, 84, 0.1);
    color: var(--success-color);
}

.category-error-handling {
    background-color: rgba(220, 53, 69, 0.1);
    color: var(--error-color);
}

.learning-description {
    margin: 0 0 0.75rem 0;
}

.learning-rationale {
    margin: 0 0 0.75rem 0;
    font-size: 0.95rem;
    color: var(--text-secondary);
}

.learning-files {
    font-size: 0.9rem;
}

/* File references in monospace [Req 4.4] */
.learning-files code {
    margin-right: 0.5rem;
}

/* Responsive [Req 4.6] */
@media (max-width: 768px) {
    .learning-header {
        flex-direction: column;
        align-items: flex-start;
    }
}
```

## Data Models

### VariantLearning

| Field | Type | JSON | Required | Description |
|-------|------|------|----------|-------------|
| VariantID | int | `variant_id` | Yes | Which variant this learning came from (1-N) |
| Category | LearningCategory | `category` | Yes | One of: code-pattern, architecture, testing, error-handling |
| Title | string | `title` | Yes | Brief title (5-10 words) |
| Description | string | `description` | No | What the pattern/technique is |
| Rationale | string | `rationale` | Yes | Why this matters; how it applies elsewhere |
| FileReferences | []string | `file_references` | Yes | File paths with optional line numbers |

### LearningCategory Constants

| Value | Description |
|-------|-------------|
| `code-pattern` | Idiomatic code, algorithms, elegant solutions |
| `architecture` | Structural decisions, module organization |
| `testing` | Test approaches, coverage patterns, mocking |
| `error-handling` | Defensive coding, edge cases, resilience |

## Error Handling

### Graceful Degradation [Req 6]

The learnings feature uses graceful degradation at multiple levels:

1. **JSON Parsing Failure**: If the `learnings` field is malformed JSON, the entire field is ignored; other comparison fields are still valid [Req 6.2]

2. **Individual Learning Validation**: Each learning is validated independently. Invalid learnings are logged and discarded; valid ones are kept [Req 6.3, 6.5]

3. **Empty Learnings**: If all learnings fail validation, `Result.Learnings` is set to `nil`. Reports omit the section entirely [Req 3.5, 4.5]

4. **Report Generation**: Learnings rendering errors do not fail report generation. If rendering fails, log a warning and skip the section.

### Validation Rules

A learning is **valid** if:
- `variant_id` is between 1 and N (number of variants)
- `title` is non-empty (after trimming whitespace)
- `rationale` is non-empty (after trimming whitespace)
- `file_references` has at least one entry

**Limits enforced:**
- Maximum 5 learnings per variant (`MaxLearningsPerVariant`)
- Maximum 20 learnings total (`MaxLearningsTotal`)
- Maximum 5 file references per learning (`MaxFileRefsPerLearning`)

A learning with an unknown category is **still valid** (forward compatibility).

## Testing Strategy

### Unit Tests

| Component | Test File | Coverage |
|-----------|-----------|----------|
| VariantLearning struct | `types_test.go` | JSON marshaling/unmarshaling |
| validateLearnings | `compare_test.go` | All validation rules, edge cases |
| Prompt generation | `prompt_test.go` | Learnings section inclusion |
| Markdown rendering | `markdown_test.go` | Section structure, grouping |
| HTML template | `generator_test.go` | Template rendering, XSS safety |

### Test Cases for validateLearnings

```go
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
        "invalid variant ID": {
            input: []VariantLearning{{
                VariantID:      5, // > numVariants
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
                        VariantID: 1, Title: fmt.Sprintf("title %d", i),
                        Rationale: "r", FileReferences: []string{"f.go"},
                    }
                }
                return learnings
            }(),
            numVariants: 2,
            wantCount:   5, // MaxLearningsPerVariant
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
        "file references truncated": {
            input: []VariantLearning{{
                VariantID:      1,
                Title:          "title",
                Rationale:      "reason",
                FileReferences: []string{"a.go", "b.go", "c.go", "d.go", "e.go", "f.go", "g.go"},
            }},
            numVariants: 2,
            wantCount:   1,
            // The returned learning should have only 5 file references
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
```

### Integration Tests

1. **End-to-end comparison with learnings**: Run a comparison with mock variants that return learnings; verify they appear in the Result
2. **Report generation**: Verify HTML and Markdown output contain learnings section with correct structure
3. **Graceful degradation**: Verify malformed learnings don't break comparison

### XSS Safety Test [Req 4.7]

```go
func TestLearningsHTMLEscaping(t *testing.T) {
    data := &ReportData{
        Comparison: &comparison.Result{
            Learnings: []comparison.VariantLearning{{
                VariantID:      1,
                Category:       comparison.LearningCategoryCodePattern,
                Title:          "<script>alert('xss')</script>",
                Description:    "Test <b>injection</b>",
                Rationale:      "Reason with \"quotes\"",
                FileReferences: []string{"<path>"},
            }},
        },
    }

    html := renderHTML(data)

    // Verify content is escaped
    if strings.Contains(html, "<script>") {
        t.Error("script tag not escaped")
    }
    if !strings.Contains(html, "&lt;script&gt;") {
        t.Error("expected escaped script tag")
    }
}
```

## Requirements Traceability

| Requirement | Design Element |
|-------------|----------------|
| [1.1] VariantLearning struct | `VariantLearning` in types.go |
| [1.2] Categories | `LearningCategory` constants |
| [1.3] File references format | `FileReferences []string` field |
| [1.4] Learnings in Result | `Result.Learnings` field |
| [1.5] omitempty tag | JSON tag on Learnings field |
| [2.1-2.6] Prompt instructions | `buildComparisonPrompt()` additions |
| [3.1-3.6] Markdown rendering | `generateMarkdownReport()` learnings section |
| [4.1-4.7] HTML rendering | `index.html` template + CSS |
| [5.1-5.4] Quality guidelines | Prompt examples and exclusions |
| [6.1-6.5] Robust handling | `validateLearnings()` function |
| [7.1-7.4] Web interface | Static HTML served by existing web server |
