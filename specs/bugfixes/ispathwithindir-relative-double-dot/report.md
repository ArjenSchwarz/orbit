# Bugfix Report: IsPathWithinDir Rejects Valid In-Tree Paths Whose Relative Segment Starts With ".."

**Date:** 2026-04-28
**Status:** Fixed
**Transit:** T-702

## Description of the Issue

`internal/web/middleware.go:IsPathWithinDir` decides whether `path` lives under `dir`
by computing `filepath.Rel(resolvedDir, resolved)` and then checking
`!strings.HasPrefix(rel, "..")`. That prefix check treats any relative path
starting with the two-character sequence `".."` as outside the directory,
including legitimate child paths whose first segment merely begins with `".."`,
for example `..cache/file.txt` or `..session.jsonl`.

**Reproduction steps:**
1. Create a directory `tmp/`.
2. Inside it, create `tmp/..cache/file.txt` (a normal file in a normally named child directory).
3. Call `IsPathWithinDir("tmp/..cache/file.txt", "tmp")`.
4. Observe that it returns `false` even though the file lives under `tmp`.

**Impact:** Severity moderate. Callers in `internal/web/handlers.go` (transcript serving)
and `internal/sessions/resolver.go` (session resolution across all agents) treat the
`false` return as "outside allowed dir" and surface `not found` for legitimate files.
The bug manifests for any user whose project tree contains a directory or file whose
name begins with two dots — uncommon but legal on every supported OS.

## Investigation Summary

- **Symptoms examined:** Ticket reports false negatives on names beginning with `..`.
- **Code inspected:** `internal/web/middleware.go` (`IsPathWithinDir`),
  `internal/web/middleware_test.go` (existing coverage),
  callers in `internal/web/handlers.go` and `internal/sessions/resolver.go`.
- **Hypotheses tested:**
  - The symlink resolution path was not at fault — it correctly resolves both ends.
  - `filepath.Rel` returns the literal path — `..cache/file.txt` is preserved verbatim.
  - The defect lives in the final classification step: `strings.HasPrefix(rel, "..")`
    matches both the parent-directory segment `".."` and any longer name beginning
    with two dots.

## Discovered Root Cause

**Defect type:** Logic error — improper substring/prefix check used as a path-segment check.

`filepath.Rel` produces a relative path whose first segment is `".."` only when the
target sits outside `dir`. To detect that, the first path segment must be compared
exactly. Using `strings.HasPrefix(rel, "..")` instead checks for any string that
begins with `..` — including names like `..cache` or `..session.jsonl` — which are
valid in-tree filenames.

**Why it occurred:** The author conflated "first segment equals `..`" with "starts with `..`".
Both happen to be true for traversal cases, but the prefix form also matches valid names.

**Contributing factors:**
- Tests exercised only the basic in/out cases; no test used a name starting with `..`,
  so the bug never surfaced.
- The check is used by safety-critical callers (path validation), so the conservative
  failure mode (reject) hid the bug as a "file not found" rather than a security or
  panic-style symptom.

## Resolution for the Issue

**Changes made:**
- `internal/web/middleware.go:106` — Replace `!strings.HasPrefix(rel, "..")` with an
  explicit check that rejects only when `rel == ".."` or `rel` starts with
  `".." + string(filepath.Separator)`. This matches the parent-directory segment as
  a discrete path component instead of as a prefix.

**Approach rationale:** The fix is a minimal, local change to the same classification
line. It uses `filepath.Separator` so it stays correct on Windows (`\`) and Unix (`/`).
It preserves the existing symlink-resolution safeguards (everything before the final
return is unchanged) and remains O(1).

**Alternatives considered:**
- Splitting `rel` with `filepath.SplitList` or `strings.Split` and inspecting the
  first segment — Rejected. More allocations and code for no extra correctness;
  `filepath.Rel`'s contract guarantees the leading-`..` form for outside paths.
- Using `filepath.IsLocal(rel)` (Go 1.20+) — Rejected. `IsLocal` also rejects
  absolute paths and paths containing reserved Windows names, which would change
  behaviour beyond the scope of this bugfix and could surprise existing callers.

## Regression Test

**Test file:** `internal/web/middleware_test.go`
**Test cases added to `TestIsPathWithinDir`:**
- `child dir name starts with double dot` — `tmp/..cache/file.txt` inside `tmp` must return `true`.
- `file name starts with double dot` — `tmp/..session.jsonl` inside `tmp` must return `true`.

**What they verify:** Names whose first character pair is `".."` but which are not
the parent-directory segment are correctly classified as in-tree.

**Run command:** `go test ./internal/web/ -run TestIsPathWithinDir -v`

## Affected Files

| File | Change |
|------|--------|
| `internal/web/middleware.go` | Tighten the leading-`..` check in `IsPathWithinDir` so only the literal `..` segment counts as outside the directory. |
| `internal/web/middleware_test.go` | Add two regression cases covering names that begin with `..`. |

## Verification

**Automated:**
- [x] Regression tests pass
- [x] Full test suite passes
- [x] Linters/validators pass

**Manual verification:**
- Existing `path traversal attempt` and symlink-escape tests still pass, confirming
  that genuine traversal and symlink-escape cases remain rejected.

## Prevention

**Recommendations to avoid similar bugs:**
- When checking for a leading path segment, compare the first segment explicitly or
  match `segment` followed by `filepath.Separator` — never use `strings.HasPrefix`
  alone for path-segment classification.
- When introducing a path-validation primitive, include a test case for a filename
  whose first characters coincide with the special segment being detected (e.g. a
  file named `..foo` when the check is for the `..` segment).

## Related

- Transit ticket: T-702
- Caller sites for `IsPathWithinDir`:
  - `internal/web/handlers.go:483`
  - `internal/sessions/resolver.go` (multiple call sites)
