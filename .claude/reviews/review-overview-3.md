# PR Review Overview - Iteration 3

**PR**: #64 | **Branch**: feature/compare-improvements | **Date**: 2026-02-10

## Valid Issues

### Code-Level Issues

#### Issue 1: Reuse tolerant parsing when loading saved comparison JSON
- **File**: `internal/comparison/compare.go:393`
- **Reviewer**: @chatgpt-codex-connector (P1)
- **Comment**: `LoadResultFromFile` unmarshals directly into `Result`, so a type mismatch in optional sections (e.g. malformed `learnings`) fails the whole load. The live path uses `parseAndValidate` which treats malformed learnings as non-fatal.
- **Validation**: Valid. `LoadResultFromFile` should use the same `resultRaw` + tolerant learnings parsing that `parseAndValidate` uses, since this function is specifically for recovery scenarios where tolerance matters most.

#### Issue 2: Validate loaded recommendation against completed variants
- **File**: `cmd/orbit/compare.go:131`
- **Reviewer**: @chatgpt-codex-connector (P2)
- **Comment**: The loaded recommendation is accepted without checking that the variant ID exists in the completed-variant set. Stale or edited JSON could produce misleading reports.
- **Validation**: Valid. A simple bounds check against the number of completed variants after loading prevents nonsensical recommendations.

### PR-Level Issues

#### Issue 3: Add agent compliance check
- **Type**: review comment
- **Reviewer**: @claude
- **Comment**: Consider detecting when the agent didn't write the expected JSON file and logging a warning.
- **Validation**: Valid. A simple `os.Stat` check after `CompareUnified` returns would surface when the safety net didn't work.

#### Issue 4: Improve config skip comment
- **Type**: review comment
- **Reviewer**: @claude
- **Comment**: Clarify *why* config isn't needed for `--from-file`.
- **Validation**: Valid. Minor comment improvement.

## Invalid/Skipped Issues

### Issue A: File path security hardening
- **Location**: `cmd/orbit/compare.go:154`
- **Reviewer**: @claude
- **Comment**: Suggested path validation to prevent symlink/traversal attacks.
- **Reason**: The path is constructed from `filepath.Join(specDir, ".orbit", "comparison.json")` where specDir comes from `filepath.Join("specs", specName)` - all internal values. Defense-in-depth is overkill here.

### Issue B: Track --from-file usage metrics
- **Reviewer**: @claude
- **Comment**: Consider tracking how often `--from-file` is used.
- **Reason**: Over-engineering for a CLI tool. Not worth adding.
