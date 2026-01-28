# PR Review Overview - Iteration 1

**PR**: #35 | **Branch**: feature/enhanced-status | **Date**: 2025-01-27

## Valid Issues

### Code-Level Issues

#### Issue 1: Summary.json read from wrong location

- **File**: `internal/status/gatherer.go:175`
- **Reviewer**: @chatgpt-codex-connector
- **Comment**: "This reads `summary.json` from the worktree spec directory (`<worktree>/specs/<spec>/.orbit/summary.json`), but variants write their summaries under the main spec log directory (`specs/<spec>/.orbit/logs/variant-<id>/summary.json` as set up in `internal/orbit/orbit.go:1334-1339`). In real runs that means `GetLiveTranscriptPath` can't find a session ID and `orbit status` will always show "Waiting for activity…" even when a Claude transcript exists."
- **Validation**: Valid. The code at orbit.go:1336-1338 shows variant logs are stored at `specs/<spec>/.orbit/logs/variant-<id>/` with `summary.json` there, not in the worktree. The current code reads from the wrong path and will fail to find session IDs.

#### Issue 2: int64 vs int type mismatch in last_entry.go

- **File**: `internal/transcript/last_entry.go:81`
- **Reviewer**: @copilot-pull-request-reviewer
- **Comment**: "`chunkSize` is an `int64`, but it's used as the capacity argument to `make` and as the slice bound (`(*bufPtr)[:chunkSize]`). Both require an `int`, so this won't compile as written."
- **Validation**: Invalid. The code compiles and runs. Go automatically handles int64 to int conversion in slice capacity when the value fits. Tests pass, including large file tests.

#### Issue 3: Markdown format used for terminal text output

- **File**: `cmd/orbit/status.go:185`
- **Reviewer**: @copilot-pull-request-reviewer
- **Comment**: "`renderTerminal` uses `output.WithFormat(output.Markdown())` even though the flag/help text calls this mode "text". This will emit Markdown formatting to stdout."
- **Validation**: Valid. The function is named `renderTerminal` and described as "text" mode but uses `output.Markdown()`. This should use `output.Text()` for consistency with the naming and expected plain-text terminal output.

#### Issue 4: Test doesn't exercise window expansion

- **File**: `internal/transcript/last_entry_test.go:139`
- **Reviewer**: @copilot-pull-request-reviewer
- **Comment**: "`TestGetLastDisplayableEntry_LargeFile` claims to test "window expansion", but the generated file is far smaller than `initialChunkSize` (64KB), so the code path that grows `chunkSize` is never exercised."
- **Validation**: Valid. The test creates ~100 small entries which is well under 64KB. The test should create data exceeding 64KB to actually test the window expansion logic.

#### Issue 5: Incorrect Claude transcript path in decision log

- **File**: `specs/enhanced-status/decision_log.md:190`
- **Reviewer**: @copilot-pull-request-reviewer
- **Comment**: "The Claude transcript path here includes an extra `/sessions/` segment (`~/.claude/projects/{hash}/sessions/{session-id}.jsonl`), but the implementation uses `~/.claude/projects/{project-hash}/{session-id}.jsonl`."
- **Validation**: Valid. The decision log documents an incorrect path with an extra `/sessions/` segment that doesn't match the actual implementation.

## Invalid/Skipped Issues

### Issue A: int64 type conversion

- **Location**: `internal/transcript/last_entry.go:81`
- **Reviewer**: @copilot-pull-request-reviewer
- **Comment**: Suggested `chunkSize` needs explicit int conversion
- **Reason**: Invalid - Go compiles this code fine. The `make([]byte, 0, chunkSize)` call where chunkSize is int64 works because Go handles the conversion implicitly when the value fits in int. Tests pass and code runs correctly.

### Issue B: Diagnostic warnings about `min` undefined

- **Location**: `errors.go:145`, `client_test.go:182,231`
- **Reviewer**: System diagnostics
- **Reason**: False positive from IDE. Go 1.21+ has `min` as a builtin. The project uses Go 1.25 and builds/tests successfully.
