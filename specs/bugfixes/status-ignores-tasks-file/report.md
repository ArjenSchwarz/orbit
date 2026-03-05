# Bugfix Report: Status Ignores Custom Tasks File Path

**Date:** 2026-03-05
**Status:** Fixed
**Ticket:** T-133

## Description of the Issue

`orbit status` always constructs the tasks file path as `{worktree}/specs/{specName}/tasks.md`, hardcoding both the directory structure and filename casing. When a run used `--tasks-file` pointing to a different location, or the spec uses uppercase `TASKS.md`, the status command shows "Task progress unavailable" despite valid tasks.

**Reproduction steps:**
1. Create a spec with `TASKS.md` (uppercase) or use `--tasks-file /custom/path/tasks.md`
2. Run `orbit run --variants 2`
3. Run `orbit status` while the run is in progress
4. Observe "Task progress unavailable" for all variants

**Impact:** Medium severity. Users cannot monitor task progress during variant runs when using uppercase task filenames or custom task file paths. The feature appears broken even though the run itself works correctly.

## Investigation Summary

- **Symptoms examined:** `gatherTaskProgress` returns nil (task progress unavailable) when tasks file is uppercase or at custom path
- **Code inspected:** `internal/status/gatherer.go`, `cmd/orbit/status.go`, `cmd/orbit/run.go` (detectTasksFile), `internal/variants/types.go`, `internal/orbit/variants.go`
- **Hypotheses tested:** Checked if `VariantsMetadata` stores the tasks file path (it does not), checked if `Gatherer` receives any tasks file information (it does not)

## Discovered Root Cause

`internal/status/gatherer.go:216` constructs the tasks file path as:
```go
tasksFile := filepath.Join(worktreePath, "specs", g.specName, "tasks.md")
```

This hardcodes two assumptions:
1. The tasks file is always at `specs/{specName}/tasks.md` (ignoring `--tasks-file`)
2. The filename is always lowercase `tasks.md` (ignoring `TASKS.md`)

**Defect type:** Hardcoded path assumption

**Why it occurred:** The status gatherer was implemented with the default convention path only, without considering that the run command supports custom task file paths and case-insensitive detection.

**Contributing factors:** `VariantsMetadata` (persisted in `variants.json`) does not store the tasks file path used during the run, so the status command has no way to recover this information.

## Resolution for the Issue

**Changes made:**
- `internal/variants/types.go` — Added `TasksFileRel` field to `VariantsMetadata` (persisted in `variants.json` with `omitempty` for backward compat)
- `internal/variants/manager.go` — Added `SetTasksFile(relPath)` method to store the tasks file path in metadata
- `internal/orbit/variants.go` — After variant setup, stores the tasks file relative path in metadata via `SetTasksFile`; added `tasksFileRel()` helper to compute the relative path
- `internal/status/gatherer.go` — Added `tasksFileRel` field and `SetTasksFileRel` setter; replaced hardcoded path with `resolveTasksFile()` which first tries the stored path, then falls back to auto-detection of both `tasks.md` and `TASKS.md`
- `cmd/orbit/status.go` — Passes `metadata.TasksFileRel` to the gatherer when available

**Approach rationale:** Two-layer fix: (1) persist the tasks file path in variant metadata so it survives across processes, and (2) add fallback auto-detection of both filename casings for backward compatibility with existing `variants.json` files that lack the field.

**Alternatives considered:**
- Only add TASKS.md fallback without metadata storage — would not fix the `--tasks-file` custom path case
- Add tasksFileRel as a required parameter to `NewGatherer` — would break the existing API and all callers unnecessarily; a setter is cleaner since the field is optional

## Regression Test

**Test file:** `internal/status/gatherer_test.go`
**Test names:**
- `TestGatherTaskProgress_UppercaseTasksFile` — verifies TASKS.md is found
- `TestGatherTaskProgress_CustomTasksFile` — verifies custom --tasks-file path works
- `TestGatherTaskProgress_StandardPath` — no regression for default tasks.md
- `TestGatherTaskProgress_MetadataTasksFileTakesPrecedence` — custom path takes precedence over default

**Run command:** `go test ./internal/status/ -run TestGatherTaskProgress`

## Affected Files

| File | Change |
|------|--------|
| `internal/status/gatherer.go` | Use stored tasks file path with fallback detection |
| `internal/variants/types.go` | Add TasksFileRel to VariantsMetadata |
| `internal/orbit/variants.go` | Store tasks file path in metadata during variant setup |
| `cmd/orbit/status.go` | Pass tasks file path from metadata to gatherer |

## Verification

**Automated:**
- [x] Regression test passes
- [x] Full test suite passes
- [x] Linters/validators pass

## Prevention

**Recommendations to avoid similar bugs:**
- When a value can come from user configuration, avoid hardcoding the default assumption. Instead, store the actual configured value and fall back to detection logic.
- The `detectTasksFile` function in `cmd/orbit/run.go` already checks both casings; status should mirror that logic or share it.
