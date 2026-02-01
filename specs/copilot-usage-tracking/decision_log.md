# Decision Log: Copilot Usage Tracking

## Decision 1: Backward Compatibility Strategy

**Date**: 2025-02-01
**Status**: accepted

### Context

Existing summary.json files do not contain cost unit information. When adding cost unit tracking, we need to determine how to handle these legacy files without breaking existing functionality or requiring data migration. Notably, `SessionEntry` already stores `agent_type` which can be used to infer the cost unit.

### Decision

Infer the cost unit from the `agent_type` field for legacy summary.json files that lack the `cost_unit` field:
- "kiro" → "credits"
- "copilot" → "premium_requests"
- All others → "USD"

### Rationale

This approach provides accurate backward compatibility by using existing data. The agent type is already stored in SessionEntry and directly corresponds to the billing model used. This avoids mislabeling Kiro credits as USD.

### Alternatives Considered

- **Always default to USD**: Would incorrectly display Kiro credits with `$` prefix for all historical data
- **Show 'unknown' for legacy**: Would cause visual inconsistency in reports and confuse users
- **Require migration**: Adds unnecessary complexity when inference is reliable

### Consequences

**Positive:**
- Zero migration effort required
- Existing reports display with correct units
- Simple implementation using existing data

**Negative:**
- If agent_type is missing (edge case), falls back to USD which may be incorrect

---

## Decision 2: Missing Cost Data Display

**Date**: 2025-02-01
**Status**: accepted

### Context

Some agents or sessions may not provide cost data. We need a consistent way to display this absence across all output formats.

### Decision

Display "-" (single dash) when cost data is unavailable.

### Rationale

A single dash is a common convention for missing data in tables and reports. It's more compact than "N/A" while being clearly distinct from a zero value. This is preferable to omitting the field entirely (which could be confusing) or showing $0.00 (which would be misleading).

### Alternatives Considered

- **Show "N/A"**: More verbose; "-" is sufficient and more compact for tabular display
- **Omit the cost field entirely**: Users might wonder if it's a bug or missing feature
- **Show $0.00**: Misleading; suggests the session had zero cost rather than unknown cost

### Consequences

**Positive:**
- Clear indication of missing data
- Compact display, works well in tables
- No risk of misinterpretation
- Consistent across all display contexts

**Negative:**
- None significant

---

## Decision 3: Partial Parse Failure Handling

**Date**: 2025-02-01
**Status**: accepted

### Context

When parsing Copilot's usage output, some metrics may parse successfully while others fail (malformed data, format changes, partial output). We need to define behavior for these partial failure scenarios.

### Decision

Extract all metrics that parse successfully and set failed ones to nil. Log debug messages for each parse failure including the raw line that failed.

### Rationale

Partial success is better than total failure. If we can extract premium requests but not session time, that partial data is still valuable. Debug logging enables troubleshooting without cluttering normal output.

### Alternatives Considered

- **All-or-nothing**: Discard all metrics if any parse fails - loses valuable data
- **Silent failure**: Don't log parse failures - makes debugging difficult

### Consequences

**Positive:**
- Maximizes data extraction even with format variations
- Debug logging aids troubleshooting
- Graceful degradation

**Negative:**
- Users may not realize some metrics are missing unless they check debug logs

---

## Decision 4: Cost Aggregation Strategy

**Date**: 2025-02-01
**Status**: accepted

### Context

Multi-variant or multi-agent runs may involve different cost units (USD from Claude, credits from Kiro, premium requests from Copilot). Aggregating these into a single total would require conversion rates that may not exist or may change.

### Decision

Display separate totals for each cost unit type present in a run.

### Rationale

Different cost units represent fundamentally different billing models and cannot be meaningfully combined without exchange rates. Showing separate totals preserves accuracy and lets users understand their consumption in each system's native terms.

### Alternatives Considered

- **Convert to USD where possible**: Requires maintaining conversion rates; rates may not exist for all units; adds complexity and potential for inaccuracy
- **Primary unit only**: Loses information about consumption in other unit types

### Consequences

**Positive:**
- Accurate representation of all costs
- No dependency on external conversion rates
- Users see consumption in native terms for each billing system

**Negative:**
- Slightly more complex display logic
- Users can't easily compare total cost across different unit types

---

## Decision 5: Copilot Metrics Scope

**Date**: 2025-02-01
**Status**: accepted

### Context

Copilot's CLI output includes multiple usage metrics: premium requests, API time, session time, code changes, and per-model token breakdown. We need to decide which metrics to capture and store.

### Decision

Capture all available metrics from the CLI output: premium requests, API time, session time, code changes, and aggregate token counts. Per-model token breakdown will be aggregated into totals rather than stored separately.

### Rationale

Capturing all available data provides maximum value for analysis and reporting. Session time is particularly important as it aligns with metrics tracked for other agents. Aggregating token counts (rather than per-model breakdown) simplifies storage and display while still providing useful information.

### Alternatives Considered

- **Primary metrics only (premium requests + tokens)**: Would lose valuable timing and code change data
- **Premium requests only**: Too limited; misses opportunity to capture useful data
- **Store per-model breakdown**: Adds complexity to data model and display without clear user benefit

### Consequences

**Positive:**
- Rich usage data for analysis
- Consistent with other agents' session time tracking
- Simplified storage compared to per-model breakdown

**Negative:**
- More complex parsing logic
- Per-model attribution lost in aggregation

---

## Decision 6: Usage Data Source

**Date**: 2025-02-01
**Status**: accepted

### Context

Copilot usage data could potentially be extracted from multiple sources: the CLI stdout/stderr output or the JSONL session files stored in ~/.copilot/session-state/.

### Decision

Parse usage data exclusively from CLI stdout/stderr output.

### Rationale

The usage summary shown in the user's example is printed to the terminal after session completion. This is the most reliable and direct source. The JSONL session files don't appear to contain usage metrics based on the existing transcript parser analysis.

### Alternatives Considered

- **Session files + CLI output**: Added complexity for uncertain benefit; session files may not contain usage data
- **CLI output with session file enhancement**: Unclear what additional data session files would provide

### Consequences

**Positive:**
- Simple, direct implementation
- Uses data that's already captured in RunResult.Output/Stderr
- No additional file I/O needed

**Negative:**
- If Copilot changes output format, parsing may break
- Limited to what's printed in the summary (no hidden metrics)

---

## Decision 7: Cost Aggregation Display Order

**Date**: 2025-02-01
**Status**: accepted

### Context

When displaying aggregated costs from multiple unit types in a run summary, we need a consistent order for the different unit types.

### Decision

Display aggregated totals in this fixed order: USD first, then credits, then premium requests. Use comma-space as the delimiter. Omit unit types with zero totals.

Example: "$1.23, 0.5 credits, 2.1 premium requests"

### Rationale

USD is the most common currency and familiar to most users, so it leads. Credits and premium requests follow in order of their introduction to the system. A fixed order ensures consistency across all reports and prevents confusion from varying display orders.

### Alternatives Considered

- **Alphabetical order**: Less intuitive; "credits" would come before "premium_requests" and "USD"
- **Order by value**: Would cause inconsistent ordering between runs
- **Order by frequency in run**: Complex to implement with marginal benefit

### Consequences

**Positive:**
- Consistent, predictable display across all reports
- USD prominence matches user expectations
- Simple implementation

**Negative:**
- None significant

---

## Decision 8: CostValue Field for SessionEntry

**Date**: 2025-02-01
**Status**: accepted

### Context

The existing `SessionEntry.CostUSD` field name is misleading when storing credits or premium requests. Storing 0.45 credits in a field named `CostUSD` creates confusion and potential bugs.

### Decision

Add new `CostValue` and `CostUnit` fields to `SessionEntry` while keeping `CostUSD` for backward compatibility:

```go
CostUSD    float64 `json:"cost_usd"`              // Kept for backward compat
CostValue  float64 `json:"cost_value,omitempty"`  // NEW: actual cost value
CostUnit   string  `json:"cost_unit,omitempty"`   // NEW: unit type
```

When reading:
- If `cost_value` present, use it with `cost_unit`
- Otherwise, use `cost_usd` and infer unit from `agent_type`

When writing:
- Write to both `cost_usd` (for old readers) and `cost_value`/`cost_unit` (for new readers)

### Rationale

This approach maintains full backward compatibility while providing semantically correct field names for new code. Old Orbit versions will read `cost_usd` and display with `$` prefix (the existing bug), while new versions will use `cost_value`/`cost_unit` correctly.

### Alternatives Considered

- **Rename CostUSD to CostValue**: Breaking change, violates requirement 6.4
- **Only add CostUnit, keep CostUSD for values**: Still semantically incorrect, confusing for developers
- **Add schema version field**: Adds complexity without clear benefit for this case

### Consequences

**Positive:**
- Semantically correct field names
- Full backward compatibility
- Clear migration path

**Negative:**
- Slight duplication in storage (cost stored in two fields)
- More complex write logic

---

## Decision 9: Pointer Types for Optional Metrics

**Date**: 2025-02-01
**Status**: accepted

### Context

Some metrics (API time, session time, lines added/removed) may not be available in all scenarios. We need to distinguish between "zero" (metric present, value is 0) and "not present" (metric unavailable).

### Decision

Use pointer types for optional metrics in `CostMetrics`:
- `APIDuration *time.Duration`
- `SessionDuration *time.Duration`
- `LinesAdded *int`
- `LinesRemoved *int`

A nil pointer means "not present"; a zero value means "present and zero".

### Rationale

Go's zero values make it impossible to distinguish "unset" from "zero" for value types. Pointer types provide this distinction explicitly. This is especially important for code changes where "+0 -0" is a valid result that should display as such.

### Alternatives Considered

- **Use -1 as sentinel**: Unconventional, error-prone, doesn't work for durations
- **Separate "present" bool fields**: Doubles the number of fields, clutters the struct
- **Always set to zero if not present**: Cannot distinguish "no changes" from "unknown"

### Consequences

**Positive:**
- Clear distinction between zero and unset
- Explicit handling required at use sites (nil checks)
- Standard Go pattern for optional values

**Negative:**
- More verbose nil checks in code
- Slightly more memory allocation

---

## Decision 10: Forward Compatibility

**Date**: 2025-02-01
**Status**: accepted

### Context

When adding the `cost_unit` field to summary.json, older versions of Orbit that don't recognize this field may read newer summary files. We need to ensure this doesn't cause errors.

### Decision

Rely on Go's standard JSON unmarshaling behavior, which ignores unknown fields by default. No explicit forward compatibility code is needed.

### Rationale

Go's `encoding/json` package ignores unknown fields when unmarshaling into a struct. Older Orbit versions will simply not see the `cost_unit` field and will use their existing logic (which treats costs as USD). This is acceptable because older versions already have the display bug we're fixing.

### Alternatives Considered

- **Explicit schema versioning**: Adds complexity without clear benefit for this case
- **Deprecation warnings**: Not needed since older versions won't see the new field

### Consequences

**Positive:**
- Zero additional code needed for forward compatibility
- Older versions continue to work (with existing display bug)
- Standard Go behavior, no surprises

**Negative:**
- Older versions will continue to show the incorrect display (expected)

---
