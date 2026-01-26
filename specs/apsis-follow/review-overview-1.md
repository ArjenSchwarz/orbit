# PR Review Overview - Iteration 1

**PR**: #33 | **Branch**: feature/apsis-follow | **Date**: 2026-01-26

## Valid Issues

### Code-Level Issues

*No code-level review comments found.*

### PR-Level Issues

#### Issue 1: Flaky Test - Mtime Detection
- **Type**: review comment (PR-level)
- **Reviewer**: @claude
- **Comment**:
  > Test TestFollower_Poll_MtimeChange is failing because it appends to a file and immediately checks mtime. On filesystems with coarse-grained mtime resolution (1 second on many Linux filesystems), the modification time may not change if the write happens within the same second.
- **Validation**: Valid. The `poll()` method at `internal/transcript/follow.go:100` only checks mtime changes and truncation (size decrease). Size *increase* is not detected as a change, so if the append happens within the same second (same mtime), the test could fail. Fix: Also detect size increase as a change.

## Invalid/Skipped Issues

### Issue A: Windows Inode Handling
- **Location**: `internal/transcript/fileinfo_windows.go:11`
- **Reviewer**: @claude
- **Comment**:
  > Windows implementation always returns inode 0, which means file replacement detection will not work on Windows.
- **Reason**: Nice-to-have enhancement, not a blocking issue. Windows file replacement detection can be addressed in a follow-up.

### Issue B: Magic Number for Poll Interval
- **Location**: `internal/transcript/follow.go:29`
- **Reviewer**: @claude
- **Comment**:
  > Poll interval is hardcoded at 500ms. Consider making it configurable via flag for power users.
- **Reason**: Nice-to-have enhancement. The interval is exported as a constant for testing configurability. CLI flag can be added in a follow-up.

### Issue C: Buffer Flushing
- **Location**: `internal/transcript/follow.go:162-164`
- **Reviewer**: @claude
- **Comment**:
  > Line 162-164 attempts to flush output, but os.Stdout does not implement Flush().
- **Reason**: This is by design. The code checks if the writer implements `Flush() error` and only calls it if available. os.Stdout doesn't implement this interface, so the flush is a no-op for stdout but works for bufio.Writer wrappers. No fix needed.

### Issue D: Deduplication Reset Logging
- **Reviewer**: @claude
- **Comment**:
  > When the hash map is reset at 10,000 entries, users might see duplicate content without knowing why.
- **Reason**: Nice-to-have enhancement. The 10,000 entry cap is unlikely to be hit in normal usage. Can be addressed in a follow-up.

### Issue E: Codex Review Comment
- **Type**: automated review
- **Reviewer**: @chatgpt-codex-connector
- **Comment**: Standard Codex review notification
- **Reason**: No actionable items. The review body only contains setup information.
