# Requirements: Copilot Usage Tracking

## Introduction

This feature adds usage tracking for GitHub Copilot sessions and fixes cost display issues across all agents. Currently, Orbit captures Copilot CLI output but doesn't parse usage statistics. Additionally, cost display incorrectly shows Kiro credits with a USD `$` prefix, and there's no support for displaying costs in their native units (credits, premium requests, USD).

The implementation will:
1. Parse Copilot CLI output to extract usage metrics (premium requests, tokens, session time, API time, code changes)
2. Fix cost display to show values in their appropriate units
3. Support separate cost aggregation when multiple unit types are present in a run
4. Maintain backward compatibility with existing summary.json files

## Copilot Usage Output Format

The Copilot CLI prints a usage summary to stdout or stderr after session completion. The format is:

```
Total usage est:        0.33 Premium requests
API time spent:         28s
Total session time:     33s
Total code changes:     +0 -0
Breakdown by AI model:
 claude-haiku-4.5        146.4k in, 2.6k out, 88.2k cached (Est. 0.33 Premium requests)
```

### Parsing Rules

**General:**
- Line matching SHALL be case-insensitive (e.g., "Total Usage Est" matches "Total usage est")
- Multiple whitespace characters (spaces, tabs) SHALL be treated as a single delimiter
- Only the period (`.`) decimal separator is supported; comma decimal separators are not parsed
- If multiple usage summaries appear in output, the parser SHALL use the last valid occurrence

**Premium requests:**
- Extract float value before "Premium requests" on the "Total usage est" line
- Pattern: `Total usage est:\s+([\d.]+)\s+Premium requests`

**API time:**
- Extract duration from the "API time spent" line
- Supported formats:
  - `Ns` or `N.NNs` (e.g., "28s", "36.11s") - seconds only
  - `Nm Ns` or `Nm N.NNs` (e.g., "1m 36.11s") - minutes and seconds
- Pattern: `API time spent:\s+(?:(\d+)m\s+)?([\d.]+)s`
- Convert to total seconds: minutes * 60 + seconds

**Session time:**
- Extract duration from the "Total session time" line
- Supported formats:
  - `Ns` or `N.NNNs` (e.g., "33s", "48.964s") - seconds only
  - `Nm Ns` or `Nm N.NNNs` (e.g., "1m 48.964s") - minutes and seconds
- Pattern: `Total session time:\s+(?:(\d+)m\s+)?([\d.]+)s`
- Convert to total seconds: minutes * 60 + seconds

**Code changes:**
- Extract lines added and removed from `+N -M` pattern
- Pattern: `Total code changes:\s+\+(\d+)\s+-(\d+)`

**Token counts:**
- Aggregate all token values from model breakdown lines
- Pattern per line: `([\d.]+)([km])?\s+in,\s+([\d.]+)([km])?\s+out,\s+([\d.]+)([km])?\s+cached`
- Values with "k" suffix SHALL be multiplied by 1,000 (e.g., "146.4k" → 146400)
- Values with "m" suffix SHALL be multiplied by 1,000,000 (e.g., "1.3m" → 1300000)
- Values without suffix are used as-is
- Cached tokens SHALL be stored separately, not added to input tokens

## Struct Changes

The following changes to `CostMetrics` are required:

```go
type CostMetrics struct {
    // Token-based (Claude, Codex, Copilot)
    InputTokens  int
    OutputTokens int
    CachedTokens int     // NEW: for Copilot cached tokens
    TotalTokens  int

    // Credit-based (Kiro)
    Credits float64

    // Premium request-based (Copilot)
    PremiumRequests float64  // CHANGED: int → float64

    // Time metrics
    APIDuration     *time.Duration  // NEW: API time spent (pointer for optional)
    SessionDuration *time.Duration  // NEW: session time (pointer for optional)

    // Code change metrics
    LinesAdded   *int  // NEW: pointer for optional
    LinesRemoved *int  // NEW: pointer for optional

    // Cost unit
    CostUnit string  // NEW: "USD", "credits", or "premium_requests"

    // Universal (deprecated for new code, kept for backward compat)
    CostUSD float64
}
```

The following changes to `SessionEntry` are required:

```go
type SessionEntry struct {
    // ... existing fields unchanged ...
    CostUSD    float64 `json:"cost_usd"`              // Kept for backward compat
    CostValue  float64 `json:"cost_value,omitempty"`  // NEW: actual cost value
    CostUnit   string  `json:"cost_unit,omitempty"`   // NEW: unit type
}
```

**Note:** Pointer types (`*time.Duration`, `*int`) distinguish between "zero" and "not present". A nil pointer means the metric was not available; a zero value means the metric was present and had value zero.

## Requirements

### 1. Copilot Usage Extraction

**User Story:** As a user running Copilot sessions through Orbit, I want usage statistics extracted from the CLI output, so that I can track resource consumption alongside other agents.

**Acceptance Criteria:**

1. <a name="1.1"></a>WHEN a Copilot session completes, the system SHALL parse both stdout and stderr for the usage summary
2. <a name="1.2"></a>The system SHALL extract the "Premium requests" value as a float from the "Total usage est" line
3. <a name="1.3"></a>The system SHALL extract the "API time spent" duration, supporting both seconds-only (e.g., "28s") and minutes-seconds (e.g., "1m 36.11s") formats
4. <a name="1.4"></a>The system SHALL extract the "Total session time" duration, supporting both seconds-only and minutes-seconds formats
5. <a name="1.5"></a>The system SHALL extract the "Total code changes" as lines added and lines removed from the `+N -M` pattern
6. <a name="1.6"></a>The system SHALL aggregate token counts (input, output) by summing values from all model breakdown lines
7. <a name="1.7"></a>The system SHALL aggregate cached token counts separately from input/output tokens
8. <a name="1.8"></a>WHEN parsing a token value with "k" suffix (e.g., "146.4k"), the system SHALL multiply by 1,000
9. <a name="1.8a"></a>WHEN parsing a token value with "m" suffix (e.g., "1.3m"), the system SHALL multiply by 1,000,000
10. <a name="1.9"></a>IF the usage summary is not present in the output, the system SHALL leave optional metrics as nil pointers
11. <a name="1.10"></a>IF some metrics parse successfully but others fail, the system SHALL store the successful values and leave failed ones as nil pointers
12. <a name="1.11"></a>The system SHALL store extracted metrics in the `RunResult.Cost` field using `CostMetrics`
13. <a name="1.12"></a>WHEN parsing fails for a metric, the system SHALL log a debug message including the raw line that failed to parse

### 2. Cost Unit Tracking

**User Story:** As a user viewing run summaries, I want costs displayed in their native units (USD, credits, premium requests), so that I understand actual resource consumption without confusion.

**Acceptance Criteria:**

1. <a name="2.1"></a>The `CostMetrics` struct SHALL include a `CostUnit` field to track the primary cost unit type
2. <a name="2.2"></a>The system SHALL support these unit types: "USD", "credits", "premium_requests"
3. <a name="2.3"></a>The system SHALL store the cost unit in `SessionEntry` within summary.json as `cost_unit`
4. <a name="2.4"></a>WHEN loading a summary.json file without a `cost_unit` field, the system SHALL infer the unit from `agent_type`: "kiro" → "credits", "copilot" → "premium_requests", otherwise → "USD"
5. <a name="2.4a"></a>WHEN both `cost_unit` and `agent_type` fields are missing, the system SHALL default to "USD" and log a debug warning
6. <a name="2.5"></a>The system SHALL NOT display a `$` prefix for non-USD cost units
7. <a name="2.6"></a>WHEN displaying credits, the system SHALL format as "N.NN credits" (e.g., "0.45 credits")
8. <a name="2.7"></a>WHEN displaying premium requests, the system SHALL format as "N.NN premium requests" (e.g., "0.33 premium requests")
9. <a name="2.8"></a>WHEN displaying USD, the system SHALL format as "$N.NN" (e.g., "$1.23")
10. <a name="2.9"></a>The system SHALL validate that extracted cost values are non-negative; negative values SHALL be treated as parse errors

### 3. Session and API Time Tracking

**User Story:** As a user analyzing agent performance, I want session duration and API time tracked for Copilot sessions, so that I can compare execution times across different agents and understand API overhead.

**Acceptance Criteria:**

1. <a name="3.1"></a>The system SHALL extract session time from Copilot's "Total session time" output
2. <a name="3.2"></a>The system SHALL extract API time from Copilot's "API time spent" output
3. <a name="3.3"></a>The system SHALL store both session time and API time in `CostMetrics`
4. <a name="3.4"></a>IF session time is not available in the output, the system SHALL fall back to the measured execution duration from `RunResult.Duration`
5. <a name="3.5"></a>IF API time is not available in the output, the system SHALL set the API time field to nil
6. <a name="3.6"></a>The system SHALL display API time alongside session time where timing metrics are shown

### 4. Cost Display in Reports

**User Story:** As a user reviewing run reports, I want costs displayed consistently across terminal, web UI, and exported formats, so that I get accurate information regardless of how I view the data.

**Acceptance Criteria:**

1. <a name="4.1"></a>The terminal display SHALL format costs according to their unit type using the formats defined in requirement 2
2. <a name="4.2"></a>The web interface SHALL format costs according to their unit type using the formats defined in requirement 2
3. <a name="4.3"></a>The Markdown export SHALL format costs according to their unit type using the formats defined in requirement 2
4. <a name="4.4"></a>The HTML export SHALL format costs according to their unit type using the formats defined in requirement 2
5. <a name="4.5"></a>WHEN cost data is unavailable, the system SHALL display "-" instead of a value
6. <a name="4.6"></a>The system SHALL provide a single `FormatCost(value float64, unit string)` function used by all display contexts
7. <a name="4.7"></a>WHEN code changes data is available, the system SHALL display it as "+N/-M lines"

### 5. Cost Aggregation

**User Story:** As a user with multi-variant or multi-agent runs, I want costs aggregated by unit type, so that I can see meaningful totals without mixing incompatible units.

**Acceptance Criteria:**

1. <a name="5.1"></a>WHEN a run contains costs in multiple unit types, the system SHALL display separate totals for each unit
2. <a name="5.2"></a>The system SHALL display aggregated totals in this order: USD, then credits, then premium requests
3. <a name="5.3"></a>The system SHALL use comma-space as the delimiter between unit totals (e.g., "$1.23, 0.5 credits, 2.1 premium requests")
4. <a name="5.4"></a>WHEN all costs are in a single unit type, the system SHALL display only that total without delimiters
5. <a name="5.5"></a>The system SHALL NOT attempt to convert between different unit types
6. <a name="5.6"></a>The system SHALL omit unit types with zero totals from the aggregated display

### 6. Backward Compatibility

**User Story:** As a user with existing run data, I want my historical summaries to continue displaying correctly, so that I don't lose access to past run information.

**Acceptance Criteria:**

1. <a name="6.1"></a>The system SHALL read summary.json files created before this feature without error
2. <a name="6.2"></a>WHEN a SessionEntry lacks a `cost_unit` field, the system SHALL infer the unit from `agent_type` as defined in requirement 2.4
3. <a name="6.3"></a>The system SHALL NOT require migration of existing summary.json files
4. <a name="6.4"></a>Existing API contracts for RunResult and CostMetrics SHALL remain compatible (new fields only, no removals). **Exception:** `PremiumRequests` changes from `int` to `float64` because Copilot outputs fractional values; this field was previously unused
5. <a name="6.5"></a>Older versions of Orbit that do not recognize the `cost_unit` field SHALL ignore it (standard Go JSON behavior)

## Out of Scope

- Converting between cost units (e.g., credits to USD)
- Historical cost data enrichment for past Copilot sessions
- Per-model token breakdown display (tokens will be aggregated)
- Parsing Copilot JSONL session files for usage data
