# Bugfix Report: Variant Base Commit Drift

**Date:** 2026-02-17
**Status:** Fixed

## Description of the Issue

When running `orbit run --variants`, each variant worktree is created from `main`. If `main` advances between worktree creation (e.g., because another process pushes), different variants may start from different base commits, leading to inconsistent comparison results.

**Reproduction steps:**
1. Run `orbit run --variants 3` on a repository
2. While variants are being created, push a new commit to the current branch from another terminal
3. Observe that the variant branches may point to different base commits

**Impact:** Variant comparison results become unreliable when variants start from different commits, defeating the purpose of controlled multi-variant comparison.

## Investigation Summary

- **Symptoms examined:** `Setup()` in `manager.go` captures `headCommit` once but does not pass it to `CreateBranch`
- **Code inspected:** `internal/variants/manager.go` (Setup method), `internal/variants/git.go` (GitClient interface and Git implementation)
- **Hypotheses tested:** The `git branch <name>` command (without a commit argument) always creates a branch at the current HEAD, not at a pinned commit

## Discovered Root Cause

`Git.CreateBranch()` runs `git branch <name>` which creates a branch at whatever HEAD is at the moment of the call. The `Setup()` method captures HEAD once via `GetHeadCommit()`, but the captured commit is only stored in metadata -- it is never passed to `CreateBranch()`.

**Defect type:** Race condition / missing parameter

**Why it occurred:** The original implementation assumed HEAD would remain stable throughout the Setup loop. This is true in most single-user scenarios but fails when external processes modify the branch.

**Contributing factors:** The `CreateBranch` interface only accepted a branch name, with no way to specify a target commit.

## Resolution for the Issue

**Changes made:**
- `internal/variants/git.go:30` - Changed `CreateBranch(name string) error` to `CreateBranch(name, commit string) error` in the `GitClient` interface
- `internal/variants/git.go:115-121` - Updated `Git.CreateBranch` implementation to pass the commit SHA to `git branch <name> <commit>` when non-empty
- `internal/variants/manager.go:258` - Pass the captured `headCommit` to `CreateBranch` in the Setup loop

**Approach rationale:** Adding a commit parameter to `CreateBranch` is the minimal change that eliminates the race. The `git branch <name> <commit>` command is the standard git mechanism for creating a branch at a specific point.

**Alternatives considered:**
- Adding a separate `CreateBranchAt(name, commit string)` method - rejected because it would leave the unsafe `CreateBranch` method available and add interface bloat
- Detaching HEAD before the loop - rejected because it is more invasive and could interfere with other operations

## Regression Test

**Test file:** `internal/variants/git_test.go`
**Test name:** `TestCreateBranch_AtSpecificCommit`

**What it verifies:** Creates a branch at a specific commit after HEAD has advanced, then confirms the branch points to the pinned commit rather than the current HEAD.

**Test file:** `internal/variants/manager_test.go`
**Test name:** `TestSetup_PinsBranchesToCapturedCommit`

**What it verifies:** All variant branches created during Setup receive the captured HEAD commit, ensuring they all share the same base.

**Run command:** `go test ./internal/variants/ -run "TestCreateBranch_AtSpecificCommit|TestSetup_PinsBranchesToCapturedCommit"`

## Affected Files

| File | Change |
|------|--------|
| `internal/variants/git.go` | Added `commit` parameter to `CreateBranch` in interface and implementation |
| `internal/variants/manager.go` | Pass `headCommit` to `CreateBranch` call in Setup |
| `internal/variants/mock_git.go` | Updated `MockGit.CreateBranch` signature |
| `internal/variants/manager_test.go` | Updated mock signature, added `TestSetup_PinsBranchesToCapturedCommit` |
| `internal/variants/git_test.go` | Updated existing tests, added `TestCreateBranch_AtSpecificCommit` |
| `internal/consolidation/consolidator_test.go` | Updated mock signature |
| `internal/status/gatherer_test.go` | Updated mock signature |

## Verification

**Automated:**
- [x] Regression test passes
- [x] Full test suite passes
- [x] Linters/validators pass

## Prevention

**Recommendations to avoid similar bugs:**
- When capturing state for use in a loop, pass the captured value explicitly rather than relying on implicit "current" state
- For git operations that accept an optional commit/ref, always expose that parameter in the interface to avoid implicit HEAD coupling

## Related

- Transit ticket: T-112
