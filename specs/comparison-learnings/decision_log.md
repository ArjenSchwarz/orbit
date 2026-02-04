# Decision Log: Comparison Learnings

## Decision 1: Learning Categories

**Date**: 2026-02-03
**Status**: accepted

### Context

The learnings feature needs to categorize insights to help users quickly identify the type of learning. We needed to decide which categories would be most valuable for developer growth.

### Decision

Support four learning categories: `code-pattern`, `architecture`, `testing`, and `error-handling`.

### Rationale

These four categories cover the main areas where developers can improve through code review: writing idiomatic code, structuring systems well, testing effectively, and handling edge cases. The user confirmed all four categories are valuable.

### Alternatives Considered

- **Single uncategorized list**: Simpler but loses the ability to filter/scan by interest area - Rejected for reduced usability
- **More granular categories** (e.g., performance, security, documentation): Could dilute focus and make AI output less consistent - Rejected for simplicity

### Consequences

**Positive:**
- Users can quickly scan for categories they care about
- Four categories is a manageable number for consistent AI output
- Categories align with common code review focus areas

**Negative:**
- Some learnings may not fit neatly into one category
- AI must classify learnings, which adds complexity to the prompt

---

## Decision 2: Learnings Organization

**Date**: 2026-02-03
**Status**: accepted

### Context

Learnings could be organized either by variant (all learnings from variant 1, then all from variant 2) or by category (all code-pattern learnings across variants, then all architecture learnings).

### Decision

Organize learnings by variant, with each variant's learnings listed together.

### Rationale

Per-variant organization makes it easier to see what each implementation approach taught. This aligns with how the rest of the comparison report is structured (variant-centric).

### Alternatives Considered

- **By category**: Would group similar learnings but scatter variant context - Rejected because it loses the implementation approach context

### Consequences

**Positive:**
- Consistent with existing report structure
- Easy to understand "what did variant N teach me"
- Simpler rendering logic (iterate variants, then learnings)

**Negative:**
- Harder to compare similar learnings across variants

---

## Decision 3: File References with Line Numbers

**Date**: 2026-02-03
**Status**: accepted

### Context

Learnings could be abstract insights or tied to specific code locations. We needed to decide whether to include file references.

### Decision

Include file references with optional line numbers in the format `path/to/file.go:123`.

### Rationale

Specific code references allow users to examine the actual implementation, making learnings actionable rather than theoretical. Line numbers are optional since code may shift between commits.

### Alternatives Considered

- **No file references**: Keeps learnings abstract and portable - Rejected because it loses the ability to study the actual code
- **Required line numbers**: More precise but brittle if code changes - Rejected for flexibility

### Consequences

**Positive:**
- Users can navigate directly to relevant code
- Concrete examples are more educational than abstract descriptions
- Optional line numbers provide flexibility

**Negative:**
- Line numbers may become stale if code is modified after report generation
- AI must identify specific file locations, adding prompt complexity

---

## Decision 4: Include Rationale for Each Learning

**Date**: 2026-02-03
**Status**: accepted

### Context

Each learning could be a simple observation or include an explanation of why the pattern matters.

### Decision

Each learning must include a rationale explaining why this pattern is valuable (the broader principle or benefit).

### Rationale

The goal is developer education. Understanding *why* a pattern is good is more valuable than just knowing it exists. The rationale transforms observations into transferable knowledge.

### Alternatives Considered

- **Observation only**: More concise but less educational - Rejected because it doesn't support the learning goal

### Consequences

**Positive:**
- Learnings become transferable to other projects
- Users understand the principle, not just the example
- Higher educational value

**Negative:**
- Longer output, more tokens consumed
- AI must explain reasoning, not just identify patterns

---

## Decision 5: No Maximum Limit on Learnings

**Date**: 2026-02-03
**Status**: accepted

### Context

We could limit learnings to a maximum per variant (e.g., 3-5) to keep reports focused, or allow unlimited learnings.

### Decision

Do not impose a maximum limit on learnings per variant.

### Rationale

The user wants comprehensive insights. The AI prompt guidance on quality (excluding trivial observations) provides natural filtering. Valuable learnings should not be excluded due to an arbitrary cap.

### Alternatives Considered

- **Max 3-5 per variant**: Would ensure focused output but might exclude valuable insights - Rejected to avoid losing information

### Consequences

**Positive:**
- All valuable learnings are captured
- Rich variants can contribute more insights
- No arbitrary information loss

**Negative:**
- Reports may become longer
- AI output may be less consistent in quantity across runs

---

## Decision 6: Graceful Degradation on Parse Errors

**Date**: 2026-02-03
**Status**: accepted

### Context

AI-generated JSON responses may be malformed, missing fields, or fail validation. We needed to decide whether learnings parsing failures should block report generation.

### Decision

Learnings parsing failures SHALL NOT block comparison report generation. The system logs a warning and continues without the Learnings section.

### Rationale

The comparison report's primary value is the recommendation, per-file analysis, and cross-variant improvements. Learnings are supplementary. A parsing error in a supplementary section should not prevent users from accessing the core comparison data.

### Alternatives Considered

- **Fail the entire comparison on parse error**: Would ensure data consistency - Rejected because it blocks access to valuable comparison data
- **Retry with simplified prompt**: Could recover but adds complexity and cost - Rejected for simplicity

### Consequences

**Positive:**
- Users always get comparison results
- Reduces fragility of the comparison pipeline
- Clear logging helps diagnose issues

**Negative:**
- Users may not see learnings due to silent failures
- Must ensure logging is visible enough to notice issues

---

## Decision 7: XSS Protection in HTML Rendering

**Date**: 2026-02-03
**Status**: accepted

### Context

Learnings content is AI-generated text that gets rendered in HTML. AI output is effectively untrusted input.

### Decision

All learning content (title, description, rationale, file references) SHALL be HTML-escaped before rendering.

### Rationale

Go's `html/template` package escapes by default, but this requirement makes the security consideration explicit. AI-generated content could theoretically contain HTML/script injection if the model is manipulated or produces unexpected output.

### Alternatives Considered

- **Trust AI output**: Simpler but creates XSS risk - Rejected for security

### Consequences

**Positive:**
- Prevents potential XSS vulnerabilities
- Follows defense-in-depth principles
- Makes security requirement explicit and testable

**Negative:**
- None significant; Go's template escaping handles this automatically

---

## Decision 8: Stale Reference Disclaimer

**Date**: 2026-02-03
**Status**: accepted

### Context

File references in learnings point to code at the time of analysis. Code may change after the report is generated, making references outdated.

### Decision

Include a disclaimer in the Learnings section: "Note: File references are a snapshot from the time of analysis and may become outdated if code changes."

### Rationale

Setting user expectations is better than silent staleness. Users will understand that references were accurate when generated but may drift over time.

### Alternatives Considered

- **Validate references at render time**: Could check if files exist - Rejected for complexity and because file content may change even if file exists
- **No disclaimer**: Simpler but could confuse users - Rejected for transparency

### Consequences

**Positive:**
- Users understand the limitation
- No runtime validation overhead
- Simple to implement

**Negative:**
- Adds visual noise to the report (minor)

---

## Decision 9: Hard Limits on Learnings (Supersedes Decision 5)

**Date**: 2026-02-03
**Status**: accepted

### Context

Decision 5 stated "no maximum limit on learnings per variant" based on user preference for comprehensive insights. However, design review identified that relying solely on prompt guidelines is insufficient - AI models may produce 20-50 learnings for trivial changes, and unlimited output creates token budget and report size concerns.

### Decision

Enforce hard limits in code:
- Maximum 5 learnings per variant (`MaxLearningsPerVariant`)
- Maximum 20 learnings total (`MaxLearningsTotal`)
- Maximum 5 file references per learning (`MaxFileRefsPerLearning`)

The prompt also requests the AI provide "the most important" learnings, aiming for 3-5 per variant.

### Rationale

Hard limits provide defense-in-depth. Prompt guidelines guide the AI toward quality, but code enforces bounds regardless of AI behavior. This prevents report bloat, token budget overflow, and unbounded AI output.

### Alternatives Considered

- **Prompt-only guidance**: Relies on AI following instructions - Rejected because AI compliance is unreliable
- **Lower limits (3 per variant)**: More conservative - Rejected because 5 allows for richer variants without excessive output
- **Higher limits (10 per variant)**: More permissive - Rejected because it could still produce excessive output

### Consequences

**Positive:**
- Predictable report size
- Token budget stays within bounds
- Reports remain scannable

**Negative:**
- Some valuable learnings may be truncated
- Users expecting "all" learnings may be surprised (mitigated by logging)

---

## Decision 10: Shared Helper for Grouping Learnings

**Date**: 2026-02-03
**Status**: accepted

### Context

Design review identified that `groupLearningsByVariant` was defined twice: once in markdown.go and once in templates.go. Code duplication is a maintenance burden and could lead to inconsistencies.

### Decision

Define `GroupLearningsByVariant` once in `internal/comparison/learnings.go` and export it for use by both renderers. Also provide `SortedVariantIDs` helper for deterministic iteration order.

### Rationale

Single source of truth eliminates duplication and ensures consistent behavior. Placing it in the comparison package makes sense as it operates on comparison types.

### Alternatives Considered

- **Keep duplicated code**: Simpler initially but creates maintenance burden - Rejected

### Consequences

**Positive:**
- Single source of truth
- Deterministic ordering guaranteed
- Easier to test

**Negative:**
- Template function must delegate to package function (minor indirection)

---
