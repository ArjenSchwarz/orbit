# Comparison Learnings

## Introduction

This feature adds a "Learnings" section to Orbit's multi-variant comparison reports. When comparing implementation variants, each variant may demonstrate valuable techniques, patterns, or approaches that could help the user become a better developer. The learnings section captures these insights with specific code references and explanations of why each pattern matters.

Currently, comparison reports focus on recommending the best variant and identifying improvements that could be merged. The learnings section shifts focus from "which is better" to "what can I learn from each approach" - making multi-variant runs educational regardless of which variant is chosen.

## Requirements

### 1. Learnings Data Structure

**User Story:** As a developer reviewing comparison reports, I want each learning to include structured metadata, so that I can quickly understand what was learned, where to find it, and why it matters.

**Acceptance Criteria:**

1. <a name="1.1"></a>The system SHALL define a `VariantLearning` struct containing: variant ID, category, title, description, rationale, and file references
2. <a name="1.2"></a>The system SHALL support the following learning categories: `code-pattern`, `architecture`, `testing`, `error-handling`
3. <a name="1.3"></a>The `file_references` field SHALL be an array of file paths relative to the repository root, optionally with line numbers in the format `path/to/file.go:123`
4. <a name="1.4"></a>The system SHALL add a `Learnings` field to the `comparison.Result` struct as `[]VariantLearning`
5. <a name="1.5"></a>The `Learnings` field SHALL use the `omitempty` JSON tag to ensure backwards compatibility when parsing older comparison results

### 2. AI Prompt for Learning Extraction

**User Story:** As a developer, I want the AI comparison to automatically identify educational insights from each variant, so that I receive learnings without manual analysis.

**Acceptance Criteria:**

1. <a name="2.1"></a>The comparison prompt SHALL include instructions for identifying learnings from each variant
2. <a name="2.2"></a>The prompt SHALL request learnings across all four categories: code patterns, architecture, testing strategies, and error handling
3. <a name="2.3"></a>The prompt SHALL instruct the AI to include specific file references for each learning
4. <a name="2.4"></a>The prompt SHALL instruct the AI to explain why each learning matters (the broader principle or benefit)
5. <a name="2.5"></a>The prompt SHALL NOT impose a maximum limit on learnings per variant
6. <a name="2.6"></a>The JSON schema in the prompt SHALL be updated to include the learnings array structure

### 3. Report Rendering - Markdown

**User Story:** As a developer viewing comparison reports in GitHub or a text editor, I want learnings displayed in a readable markdown format organized by variant, so that I can easily browse insights from each implementation.

**Acceptance Criteria:**

1. <a name="3.1"></a>The markdown report SHALL include a "Learnings" section after the "Improvements from Other Variants" section
2. <a name="3.2"></a>WHEN learnings exist for a variant, the section SHALL display them grouped under a variant heading
3. <a name="3.3"></a>Each learning SHALL display: category badge, title, description, rationale, and file references
4. <a name="3.4"></a>File references SHALL be rendered as relative paths (not clickable links in markdown)
5. <a name="3.5"></a>IF no learnings exist for any variant, the "Learnings" section SHALL be omitted entirely
6. <a name="3.6"></a>The Learnings section SHALL include a disclaimer: "Note: File references are a snapshot from the time of analysis and may become outdated if code changes."

### 4. Report Rendering - HTML

**User Story:** As a developer viewing comparison reports in a browser, I want learnings displayed with visual styling and navigation, so that I can quickly scan and understand the insights.

**Acceptance Criteria:**

1. <a name="4.1"></a>The HTML report SHALL include a "Learnings" section with consistent styling matching existing sections
2. <a name="4.2"></a>Each learning category SHALL have a distinct visual badge (e.g., colored label)
3. <a name="4.3"></a>Learnings SHALL be grouped by variant with clear visual separation
4. <a name="4.4"></a>File references SHALL be displayed in monospace font
5. <a name="4.5"></a>IF no learnings exist for any variant, the "Learnings" section SHALL be omitted entirely
6. <a name="4.6"></a>The HTML template SHALL be responsive and readable on mobile devices
7. <a name="4.7"></a>All learning content (title, description, rationale, file references) rendered in HTML SHALL be properly escaped to prevent XSS vulnerabilities

### 5. Learning Quality Guidelines

**User Story:** As a developer, I want learnings to be genuinely educational rather than trivial observations, so that the section provides real value for my growth.

**Acceptance Criteria:**

1. <a name="5.1"></a>The AI prompt SHALL instruct that learnings should represent techniques the user could apply in future projects
2. <a name="5.2"></a>The AI prompt SHALL instruct to exclude trivial observations (e.g., "uses comments", "has tests")
3. <a name="5.3"></a>The AI prompt SHALL instruct that each learning should teach a transferable skill or pattern
4. <a name="5.4"></a>The AI prompt SHALL provide examples of good learnings vs. trivial observations

### 6. Robust Response Handling

**User Story:** As a developer, I want the comparison report to generate successfully even when learnings extraction fails, so that a single parsing error doesn't prevent me from seeing the comparison results.

**Acceptance Criteria:**

1. <a name="6.1"></a>The system SHALL validate learnings against the expected JSON schema
2. <a name="6.2"></a>IF learnings fail to parse or validate, the system SHALL log a warning and continue report generation without the Learnings section
3. <a name="6.3"></a>Individual learnings missing required fields (title, rationale, file_references) SHALL be discarded with a logged warning
4. <a name="6.4"></a>The comparison report generation SHALL NOT fail due to learnings-related errors
5. <a name="6.5"></a>WHEN learnings are partially valid, the system SHALL include only the valid learnings in the report

### 7. Web Interface Support

**User Story:** As a developer using Orbit's web interface (`orbit serve`), I want to view learnings in the browser alongside other comparison data, so that I have a consistent experience across all viewing methods.

**Acceptance Criteria:**

1. <a name="7.1"></a>The web interface SHALL render the Learnings section when viewing comparison reports
2. <a name="7.2"></a>The web interface SHALL use CSS classes for category badges consistent with the static HTML report
3. <a name="7.3"></a>WHEN no learnings exist, the web interface SHALL NOT render an empty Learnings section or heading
4. <a name="7.4"></a>The web interface learnings display SHALL be responsive and match the styling of other report sections
