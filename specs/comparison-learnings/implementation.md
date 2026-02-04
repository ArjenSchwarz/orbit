# Implementation Explanation: Comparison Learnings

This document explains the comparison-learnings feature implementation at three expertise levels.

---

## Beginner Level

### What Changed / What This Does

When Orbit runs multiple AI agents to implement the same feature (called "variants"), it generates a comparison report showing which implementation is best. Previously, this report only answered "which variant is best?"

The new **Learnings** section answers a different question: "what can I learn from each variant?" Instead of just picking a winner, the report now extracts educational insights—useful coding techniques, architectural patterns, testing approaches, and error handling strategies—that developers can apply to future projects.

Think of it like this: if you asked three different chefs to make the same dish, the old report would tell you which dish tastes best. The new Learnings section tells you "Chef A used an interesting spice combination you might want to try, Chef B has a clever knife technique, and Chef C's sauce preparation method is worth learning."

### Why It Matters

- **Developer growth**: Multi-variant runs become learning opportunities, not just selection tools
- **Knowledge transfer**: Good ideas from non-chosen variants don't get lost
- **Educational value**: Each comparison report teaches something, regardless of which variant wins

### Key Concepts

- **Variant**: One implementation approach, created by an AI agent working in its own git branch
- **Learning category**: A type of insight (code patterns, architecture, testing, error handling)
- **File reference**: A pointer to specific code (like `internal/foo.go:42`) so you can see the actual example

---

## Intermediate Level

### Changes Overview

The feature adds ~3,800 lines across 20 files, touching three main areas:

1. **Data layer** (`internal/comparison/types.go`): New `VariantLearning` struct and `LearningCategory` type
2. **AI prompt** (`internal/comparison/prompt.go`): Extended JSON schema and instructions for extracting learnings
3. **Rendering** (`internal/report/`): Markdown and HTML output with learnings sections

Key files modified:
- `internal/comparison/types.go` - Data structures
- `internal/comparison/compare.go` - Validation logic
- `internal/comparison/learnings.go` - Helper functions
- `internal/comparison/prompt.go` - AI prompt changes
- `internal/report/markdown.go` - Markdown rendering
- `internal/report/templates/index.html` - HTML template
- `internal/report/templates/style.css` - CSS styling

### Implementation Approach

**Pipeline integration**: The feature slots into the existing comparison pipeline without changing its structure:

```
Prompt → AI Analysis → Validation → Report Generation
   ↓         ↓              ↓              ↓
  Added    Parses      Filters         Renders
 learnings learnings   invalid        in HTML/MD
 section   from JSON   entries
```

**Graceful degradation** (Req 6): Learnings extraction is non-fatal. If the AI returns malformed learnings, they're logged and discarded—the report still generates with other comparison data intact.

**Validation layers**:
1. JSON schema validation during parsing
2. Field validation (title, rationale, file_references required)
3. Variant ID range checking
4. Limit enforcement (max 5 per variant, 20 total, 5 file refs per learning)

**Template functions**: Shared helpers (`GroupLearningsByVariant`, `SortedVariantIDs`) in the comparison package are used by both Markdown and HTML renderers, avoiding code duplication.

### Trade-offs

| Decision | Choice | Alternative | Why |
|----------|--------|-------------|-----|
| Organization | By variant | By category | Matches existing report structure, preserves implementation context |
| Categories | Fixed 4 | Dynamic/unlimited | Consistent AI output, covers main code review areas |
| File refs | Optional line numbers | Required/none | Balances precision with code churn reality |
| Limits | Hard limits (5/20) | Soft guidance | Prevents AI from generating unbounded output |
| Unknown categories | Accept (forward compat) | Reject | Allows future category additions without breaking |

---

## Expert Level

### Technical Deep Dive

**JSON Schema Extension**:
```go
"learnings": [
  {
    "variant_id": <number>,
    "category": "<code-pattern|architecture|testing|error-handling>",
    "title": "<string: 5-10 words>",
    "description": "<string>",
    "rationale": "<string>",
    "file_references": ["<path:line>"]
  }
]
```

**Validation implementation** (`compare.go:250-327`):
- Trims whitespace before validation (handles `"   "` as empty)
- Checks variant ID is 1-N (not 0-indexed)
- Uses `map[int]int` for per-variant count tracking
- Truncates excess file references rather than rejecting
- Returns `nil` if all learnings invalid (not empty slice)

**Template function pattern** (`templates.go`):
```go
"groupLearningsByVariant": func(learnings []comparison.VariantLearning) map[int][]comparison.VariantLearning {
    return comparison.GroupLearningsByVariant(learnings)
},
```
This delegates to the shared helper, keeping template functions thin.

**CSS category styling**:
- Uses `.category-{name}` classes for known categories
- Default gray styling via `.category-badge` base class handles unknown categories
- Each category has semantic color (blue=code, purple=architecture, green=testing, red=errors)

### Architecture Impact

**Backward compatibility**: The `Learnings` field uses `omitempty`, so:
- Old reports (without learnings) parse without errors
- New reports with no learnings omit the field entirely
- Existing comparison consumers are unaffected

**Forward compatibility**: Unknown categories are accepted and rendered with default styling. This allows adding new categories (e.g., `performance`, `security`) without code changes—only prompt updates needed.

**Separation of concerns**:
- Comparison package owns data types and validation
- Report package owns rendering
- Prompt package owns AI instructions
- No cross-package coupling beyond the shared `VariantLearning` type

### Potential Issues

**AI output quality**: The prompt includes examples of good vs. trivial learnings, but AI may still generate low-value observations. Mitigation: limit guidance ("3-5 per variant, maximum 5") focuses AI on highest value.

**Stale file references**: Line numbers may drift as code changes. Mitigation: disclaimer text warns users; relative paths remain useful even if lines shift.

**Token consumption**: Learnings add ~500-1000 tokens to AI output. Hard limits (20 total) cap worst case.

**Template iteration order**: Go maps don't guarantee iteration order. The `sortedVariantIDs` helper ensures deterministic rendering.

**XSS safety**: All learning content (title, description, rationale, file references) passes through Go's `html/template` auto-escaping. Tests verify `<script>` tags are escaped.

---

## Completeness Assessment

### Fully Implemented

- [x] All 7 requirement groups (28 acceptance criteria)
- [x] Data types with JSON marshaling (Req 1)
- [x] AI prompt with quality guidelines (Req 2, 5)
- [x] Markdown rendering with grouping (Req 3)
- [x] HTML rendering with styling (Req 4)
- [x] Validation with graceful degradation (Req 6)
- [x] Web interface support via static HTML (Req 7)
- [x] Comprehensive test coverage (unit + integration)
- [x] XSS safety tests
- [x] Documentation (CHANGELOG, README mention)

### Verification Points

Each requirement can be traced to:
1. Implementation code with `[Req X.Y]` comments
2. Test cases verifying the behavior
3. Decision log entries explaining design choices

### No Missing Functionality

All 27 tasks in `tasks.md` are marked complete. The consolidation commits applied improvements from other variants (grid layout, hover effects, agent name in headers) that exceeded the base requirements.
