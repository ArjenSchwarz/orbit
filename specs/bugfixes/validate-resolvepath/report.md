# Bugfix Report: Validate ResolvePath Results

**Date:** 2026-03-05
**Status:** Fixed

## Description of the Issue

`Resolver.ResolvePath()` returns file paths for follow mode without re-validating that they stay within expected base directories. While `Resolve()` properly checks paths using `web.IsPathWithinDir()` for Codex, Copilot, and Kiro IDE, `ResolvePath()` omits these checks. This means symlinked session files could resolve to paths outside the intended directory.

**Reproduction steps:**
1. Create a symlink inside `~/.codex/sessions/` pointing to a file outside that directory
2. Call `ResolvePath(SourceCodex, sessionID)` with the UUID matching the symlinked file
3. The path is returned without validation, pointing outside `~/.codex/sessions/`

**Impact:** Security — symlinked session files in Codex, Copilot, or Kiro IDE directories could be used to access files outside intended base directories via follow mode.

## Investigation Summary

Compared `Resolve()` and `ResolvePath()` code paths side by side.

- **Symptoms examined:** `ResolvePath` returns paths without `IsPathWithinDir` validation for Codex, Copilot, and Kiro IDE sources
- **Code inspected:** `internal/sessions/resolver.go` — both `Resolve()` and `ResolvePath()` methods, plus `findKiroIDEPath()`
- **Hypotheses tested:** Confirmed that `Resolve()` has `IsPathWithinDir` checks at lines 82 (Codex), 99 (Copilot), and 178 (Kiro IDE), while `ResolvePath()` lacks equivalent checks

## Discovered Root Cause

`ResolvePath()` was added after `Resolve()` for follow mode support and duplicated the lookup logic but omitted the `IsPathWithinDir` security validation.

**Defect type:** Missing validation (second code path bypass)

**Why it occurred:** Classic "second path" bug — a new code path was added that performs the same lookup but skips the security guard present in the original path.

**Contributing factors:** The two methods share lookup logic but not validation logic. The validation was not extracted into shared helper functions.

## Resolution for the Issue

**Changes made:**
- `internal/sessions/resolver.go` — Added `IsPathWithinDir` checks to `ResolvePath()` for Codex and Copilot cases, and to `findKiroIDEPath()` for Kiro IDE

**Approach rationale:** Mirrors the exact validation pattern already used in `Resolve()` for each source. Minimal change, consistent with existing code.

**Alternatives considered:**
- Refactoring to share validation between `Resolve` and `ResolvePath` — Not chosen because it would increase scope beyond the security fix and risk introducing regressions in `Resolve`

## Regression Test

**Test file:** `internal/sessions/resolver_test.go`
**Test names:** `TestResolvePathCodexSymlinkEscape`, `TestResolvePathCopilotSymlinkEscape`, `TestResolvePathKiroIDESymlinkEscape`

**What it verifies:** Each test creates a session file outside the expected base directory, symlinks it into the expected location, and confirms `ResolvePath` (or `findKiroIDEPath`) returns an error instead of the escaped path.

**Run command:** `go test ./internal/sessions/ -run "TestResolvePath.*SymlinkEscape" -v`

## Affected Files

| File | Change |
|------|--------|
| `internal/sessions/resolver.go` | Add `IsPathWithinDir` validation to `ResolvePath` for Codex, Copilot, and `findKiroIDEPath` for Kiro IDE |
| `internal/sessions/resolver_test.go` | Add symlink escape regression tests and valid-path tests for `ResolvePath` |

## Verification

**Automated:**
- [ ] Regression test passes
- [ ] Full test suite passes
- [ ] Linters/validators pass

## Prevention

**Recommendations to avoid similar bugs:**
- When adding a new code path that performs the same lookup as an existing one, audit the original for security checks and replicate them
- Consider extracting shared validation into helper functions so both paths use the same guards

## Related

- Transit ticket: T-325
