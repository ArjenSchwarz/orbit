---
references:
    - specs/copilot-usage-tracking/requirements.md
    - specs/copilot-usage-tracking/design.md
    - specs/copilot-usage-tracking/decision_log.md
---
# Copilot Usage Tracking

## Foundation

- [x] 1. Create cost package with unit constants and Format function <!-- id:xv47eya -->
  - Create internal/cost/format.go with UnitUSD, UnitCredits, UnitPremiumRequests constants
  - Implement Format(value, unit) function per req 2.6-2.8
  - Implement FormatWithPrecision for detailed reports
  - Implement FormatCodeChanges for +N/-M lines display
  - Stream: 1
  - Requirements: [2.1](requirements.md#2.1), [2.2](requirements.md#2.2), [2.5](requirements.md#2.5), [2.6](requirements.md#2.6), [2.7](requirements.md#2.7), [2.8](requirements.md#2.8), [4.5](requirements.md#4.5), [4.6](requirements.md#4.6), [4.7](requirements.md#4.7)

- [x] 2. Add cost package tests <!-- id:xv47eyb -->
  - Test Format() for USD, credits, premium_requests
  - Test zero and negative values return dash
  - Test unknown unit fallback
  - Test FormatWithPrecision with various precisions
  - Test FormatCodeChanges with nil, zero, and positive values
  - Blocked-by: xv47eya (Create cost package with unit constants and Format function)
  - Stream: 1
  - Requirements: [2.5](requirements.md#2.5), [2.6](requirements.md#2.6), [2.7](requirements.md#2.7), [2.8](requirements.md#2.8), [4.5](requirements.md#4.5)

- [x] 3. Add FormatTotals and InferUnitFromAgent to cost package <!-- id:xv47eyc -->
  - Implement Totals struct
  - Implement FormatTotals per req 5.1-5.6
  - Implement InferUnitFromAgent per req 2.4
  - Blocked-by: xv47eyb (Add cost package tests)
  - Stream: 1
  - Requirements: [2.4](requirements.md#2.4), [5.1](requirements.md#5.1), [5.2](requirements.md#5.2), [5.3](requirements.md#5.3), [5.4](requirements.md#5.4), [5.6](requirements.md#5.6)

- [x] 4. Add FormatTotals and InferUnitFromAgent tests <!-- id:xv47eyd -->
  - Test FormatTotals with all units, single unit, mixed units
  - Test order: USD, credits, premium_requests
  - Test omission of zero values
  - Test InferUnitFromAgent for kiro, copilot, claude-code, and unknown
  - Blocked-by: xv47eyc (Add FormatTotals and InferUnitFromAgent to cost package)
  - Stream: 1
  - Requirements: [2.4](requirements.md#2.4), [5.2](requirements.md#5.2), [5.3](requirements.md#5.3), [5.4](requirements.md#5.4), [5.6](requirements.md#5.6)

## Agent Layer

- [x] 5. Update CostMetrics struct in agents package <!-- id:xv47eye -->
  - Add CachedTokens int field
  - Change PremiumRequests from int to float64
  - Add APIDuration *time.Duration
  - Add SessionDuration *time.Duration
  - Add LinesAdded *int
  - Add LinesRemoved *int
  - Add CostUnit string field
  - Add time import if needed
  - Stream: 2
  - Requirements: [1.7](requirements.md#1.7), [2.1](requirements.md#2.1), [3.3](requirements.md#3.3)

- [x] 6. Create Copilot usage parser <!-- id:xv47eyf -->
  - Create internal/agents/copilot/usage.go
  - Define UsageInfo struct
  - Implement ParseUsage(stdout, stderr) function
  - Implement parseDuration helper for Ns and Nm Ns formats
  - Implement parseTokenValue helper with k/m suffix handling
  - Add debug logging for parse failures
  - Blocked-by: xv47eye (Update CostMetrics struct in agents package)
  - Stream: 2
  - Requirements: [1.1](requirements.md#1.1), [1.2](requirements.md#1.2), [1.3](requirements.md#1.3), [1.4](requirements.md#1.4), [1.5](requirements.md#1.5), [1.6](requirements.md#1.6), [1.7](requirements.md#1.7), [1.8](requirements.md#1.8), [1.9](requirements.md#1.9), [1.10](requirements.md#1.10), [1.12](requirements.md#1.12)

- [x] 7. Add Copilot usage parser tests <!-- id:xv47eyg -->
  - Test complete output parsing
  - Test minutes-and-seconds format (1m 36.11s)
  - Test million token suffix (1.3m)
  - Test multiple model lines aggregation
  - Test case insensitivity
  - Test no usage summary returns nil
  - Test partial output (some fields only)
  - Test usage in stderr
  - Test last occurrence used when multiple matches
  - Test tokens without cached field
  - Test malformed number rejection
  - Blocked-by: xv47eyf (Create Copilot usage parser)
  - Stream: 2
  - Requirements: [1.1](requirements.md#1.1), [1.2](requirements.md#1.2), [1.3](requirements.md#1.3), [1.4](requirements.md#1.4), [1.5](requirements.md#1.5), [1.6](requirements.md#1.6), [1.7](requirements.md#1.7), [1.8](requirements.md#1.8), [1.9](requirements.md#1.9), [1.10](requirements.md#1.10)

- [x] 8. Add property-based tests for Copilot parser <!-- id:xv47eyh -->
  - Use rapid for property-based testing
  - Generate valid premium request values and verify parsing
  - Generate varying whitespace and verify tolerance
  - Generate duration values and verify round-trip
  - Blocked-by: xv47eyg (Add Copilot usage parser tests)
  - Stream: 2
  - Requirements: [1.2](requirements.md#1.2), [1.3](requirements.md#1.3), [1.4](requirements.md#1.4)

- [x] 9. Integrate usage parser into Copilot agent <!-- id:xv47eyi -->
  - Modify execute() in internal/agents/copilot/agent.go
  - Call ParseUsage after CLI execution
  - Set result.Cost with extracted metrics
  - Set CostUnit to premium_requests
  - Add debug log for extracted usage
  - Blocked-by: xv47eyg (Add Copilot usage parser tests)
  - Stream: 2
  - Requirements: [1.11](requirements.md#1.11)

- [x] 10. Update Kiro agent to set CostUnit <!-- id:xv47eyj -->
  - Modify extractSessionCredits result handling in agent.go
  - Add CostUnit: credits to CostMetrics
  - Blocked-by: xv47eye (Update CostMetrics struct in agents package)
  - Stream: 2
  - Requirements: [2.1](requirements.md#2.1)

## Storage Layer

- [x] 11. Update SessionEntry struct in logs package <!-- id:xv47eyk -->
  - Add CostValue float64 field with json tag
  - Add CostUnit string field with json tag
  - Keep CostUSD for backward compatibility
  - Import cost package
  - Blocked-by: xv47eyd (Add FormatTotals and InferUnitFromAgent tests)
  - Stream: 3
  - Requirements: [2.3](requirements.md#2.3), [6.4](requirements.md#6.4)

- [x] 12. Add GetCost method to SessionEntry <!-- id:xv47eyl -->
  - Implement GetCost() (float64, string) method
  - Use CostUnit as primary discriminator
  - Fall back to InferUnitFromAgent for legacy entries
  - Add debug warning when both CostUnit and AgentType missing
  - Blocked-by: xv47eyk (Update SessionEntry struct in logs package)
  - Stream: 3
  - Requirements: [2.4](requirements.md#2.4), [6.2](requirements.md#6.2)

- [x] 13. Update Summary struct with CostTotals <!-- id:xv47eym -->
  - Add CostTotals *cost.Totals field
  - Keep TotalCostUSD for backward compatibility
  - Add GetCostTotals() method that computes from sessions if needed
  - Blocked-by: xv47eyl (Add GetCost method to SessionEntry)
  - Stream: 3
  - Requirements: [5.1](requirements.md#5.1), [6.1](requirements.md#6.1)

- [x] 14. Update SaveSession to handle cost units <!-- id:xv47eyn -->
  - Determine costValue and costUnit from CostMetrics
  - Write to both CostUSD and CostValue for backward compat
  - Update CostTotals aggregation by unit
  - Update TotalCostUSD with all costs for old readers
  - Blocked-by: xv47eym (Update Summary struct with CostTotals)
  - Stream: 3
  - Requirements: [2.3](requirements.md#2.3), [6.1](requirements.md#6.1), [6.3](requirements.md#6.3)

- [x] 15. Add backward compatibility tests for SessionEntry <!-- id:xv47eyo -->
  - Test GetCost with new format entries
  - Test GetCost with legacy kiro entries
  - Test GetCost with legacy copilot entries
  - Test GetCost with legacy claude entries
  - Test GetCost with missing agent_type
  - Blocked-by: xv47eyn (Update SaveSession to handle cost units)
  - Stream: 3
  - Requirements: [6.1](requirements.md#6.1), [6.2](requirements.md#6.2), [6.3](requirements.md#6.3)

- [x] 16. Update existing manager tests for new fields <!-- id:xv47eyp -->
  - Update TestManager_SaveSession fixtures with CostUnit
  - Update TestManager_SavePostCompletionSession fixtures
  - Update TestSaveSession_IncludesAgentInfo expected fields
  - Blocked-by: xv47eyn (Update SaveSession to handle cost units)
  - Stream: 3
  - Requirements: [6.4](requirements.md#6.4)

## Display Layer

- [x] 17. Update terminal display to use cost package <!-- id:xv47eyq -->
  - Modify formatCost in internal/orbit/orbit.go
  - Use cost.Format with appropriate unit
  - Add getSessionDuration helper for fallback logic
  - Blocked-by: xv47eyd (Add FormatTotals and InferUnitFromAgent tests), xv47eyi (Integrate usage parser into Copilot agent)
  - Stream: 1
  - Requirements: [3.4](requirements.md#3.4), [4.1](requirements.md#4.1)

- [x] 18. Update web interface cost display <!-- id:xv47eyr -->
  - Add formatCostTotals template helper in handlers.go
  - Update run_detail.html to use GetCostTotals
  - Show individual session costs with proper units
  - Blocked-by: xv47eym (Update Summary struct with CostTotals)
  - Stream: 1
  - Requirements: [4.2](requirements.md#4.2)

- [x] 19. Update report templates to use cost.Format <!-- id:xv47eys -->
  - Modify internal/report/templates.go to use cost.Format
  - Replace or update formatCost function
  - Handle code changes display if shown in reports
  - Blocked-by: xv47eyd (Add FormatTotals and InferUnitFromAgent tests)
  - Stream: 1
  - Requirements: [4.3](requirements.md#4.3), [4.4](requirements.md#4.4)

- [x] 20. Update report generator tests <!-- id:xv47eyt -->
  - Update or remove TestFormatCost if it tests old function
  - Add tests for new cost formatting in reports
  - Blocked-by: xv47eys (Update report templates to use cost.Format)
  - Stream: 1
  - Requirements: [4.3](requirements.md#4.3), [4.4](requirements.md#4.4)

## Validation

- [x] 21. Run full test suite and fix any issues <!-- id:xv47eyu -->
  - Run make test
  - Fix any compilation errors
  - Fix any failing tests
  - Run make lint and fix issues
  - Blocked-by: xv47eyh (Add property-based tests for Copilot parser), xv47eyj (Update Kiro agent to set CostUnit), xv47eyp (Update existing manager tests for new fields), xv47eyq (Update terminal display to use cost package), xv47eyr (Update web interface cost display), xv47eyt (Update report generator tests)
  - Stream: 1

- [x] 22. Manual verification with sample outputs <!-- id:xv47eyv -->
  - Test with mock Copilot output containing all metrics
  - Test with partial output
  - Verify backward compat with existing summary.json files
  - Verify display across terminal, web, and reports
  - Blocked-by: xv47eyu (Run full test suite and fix any issues)
  - Stream: 1
