# Apsis Follow Mode Design

## Overview

This document describes the technical design for adding follow mode to the apsis CLI tool. Follow mode (`-F`/`--follow`) continuously monitors a JSONL transcript file and outputs new entries as they are appended, similar to `tail -f`.

The design integrates with the existing apsis architecture by adding a new follow loop in `cmd/apsis/main.go` and a new `Follower` component in `internal/transcript/` that handles file monitoring, entry deduplication, and incremental rendering.

### Requirements Traceability

| Requirement | Design Component |
|-------------|------------------|
| 1.1-1.4 Follow Mode Flag | `Config.Follow` field, flag parsing in `parseFlags()` |
| 2.1-2.4 Input Source Support | `resolveFollowInput()` function |
| 3.1-3.8 File Monitoring | `Follower.poll()` method with mtime/inode tracking |
| 4.1-4.8 Incremental Output | `Follower.processFile()` with hash-based seen set |
| 5.1-5.7 Output Restrictions | `validateFollowMode()` function |
| 6.1-6.4 Termination Handling | Signal handler with `signal.NotifyContext()` |
| 7.1-7.6 Error Handling | Error types and handling in `Follower` methods |

---

## Architecture

### Component Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                        cmd/apsis/main.go                         │
├─────────────────────────────────────────────────────────────────┤
│  parseFlags() ─────► Config{Follow: true}                        │
│       │                                                          │
│       ▼                                                          │
│  run() ────────────► validateFollowMode()                        │
│       │                     │                                    │
│       │              [error if -o or -f html]                    │
│       │                                                          │
│       ▼                                                          │
│  resolveFollowInput() ──► file path (not io.Reader)              │
│       │                                                          │
│       ▼                                                          │
│  runFollow() ◄────── signal.NotifyContext(SIGINT)                │
│       │                                                          │
│       ▼                                                          │
│  transcript.NewFollower(filePath, os.Stdout)                     │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                  internal/transcript/follow.go                   │
├─────────────────────────────────────────────────────────────────┤
│  Follower struct {                                               │
│      filePath    string                                          │
│      output      io.Writer                                       │
│      seenHashes  map[[16]byte]struct{}                           │
│      lastMtime   time.Time                                       │
│      lastInode   uint64                                          │
│      lastSize    int64                                           │
│  }                                                               │
│                                                                  │
│  Run(ctx) ─────────► poll loop (500ms ticker)                    │
│       │                     │                                    │
│       │              [check mtime/inode]                         │
│       │                     │                                    │
│       ▼                     ▼                                    │
│  processFile() ◄──── [if changed]                                │
│       │                                                          │
│       ├──► Parse() ──► []Entry + toolMeta accumulation           │
│       │                                                          │
│       ├──► hash each entry's raw JSON line                       │
│       │                                                          │
│       ├──► filter to unseen entries                              │
│       │                                                          │
│       ├──► RenderMarkdown(newEntries, opts) ──► output           │
│       │                                                          │
│       └──► flush stdout                                          │
└─────────────────────────────────────────────────────────────────┘
```

### Data Flow

```
┌──────────┐     ┌─────────────────┐     ┌───────────┐     ┌────────────┐
│  JSONL   │────►│  Read Lines +   │────►│  Filter   │────►│  Render    │
│  File    │     │  Hash + Parse   │     │  (unseen) │     │  (new only)│
└──────────┘     └─────────────────┘     └───────────┘     └────────────┘
     │                   │                     │                  │
     │                   ▼                     ▼                  ▼
     │           rawLine → hash         seenHashes set     RenderEntries()
     │           rawLine → Entry        (updated)          (no header)
     │           toolMeta accumulated
     │
     └─────── stat() for mtime/inode/size detection
```

**Key insight**: We read raw lines first, compute hash from raw bytes, then parse. This ensures deterministic hashing before JSON parsing loses the original byte representation.

---

## Components and Interfaces

### Config Extension (cmd/apsis/main.go)

```go
// Config holds CLI configuration.
type Config struct {
    // ... existing fields ...
    Follow  bool   // -F, --follow
}
```

### Follower Component (internal/transcript/follow.go)

```go
// Follower monitors a JSONL file and renders new entries incrementally.
type Follower struct {
    filePath   string
    output     io.Writer

    // Deduplication state
    seenHashes map[[16]byte]struct{}

    // File change detection
    lastMtime  time.Time
    lastInode  uint64
    lastSize   int64

    // Render state
    initialRenderDone bool  // True after first render (header already written)
    opts              RenderOptions
}

// NewFollower creates a new Follower for the given file path.
// Validates file exists at creation time (fails fast per requirement 7.1).
func NewFollower(filePath string, output io.Writer, opts RenderOptions) (*Follower, error)

// Run starts the follow loop. Blocks until ctx is cancelled.
// Returns nil on clean shutdown (SIGINT), error on file errors.
func (f *Follower) Run(ctx context.Context) error
```

### Internal Methods

```go
// poll checks for file changes and processes if changed.
// Returns (changed bool, err error).
func (f *Follower) poll() (bool, error)

// processFile reads, hashes, parses, and renders new entries.
// Core algorithm:
//   1. Open file, read line by line with bufio.Scanner
//   2. For each line: compute hash, then parse into Entry
//   3. Accumulate toolMeta/skillDescriptions from ALL entries
//   4. Collect entries whose hash is not in seenHashes
//   5. Render only the new entries (without header)
//   6. Update seenHashes with new entry hashes
func (f *Follower) processFile() error

// readAndHashLines reads file line by line, returning raw lines and their hashes.
// Skips incomplete JSON at EOF (detected by parse failure on last non-empty line).
// Logs warning to warnWriter for corrupt mid-file lines.
func readAndHashLines(path string, warnWriter io.Writer) ([]lineWithHash, error)

type lineWithHash struct {
    raw   []byte    // Original bytes for parsing
    hash  [16]byte  // Pre-computed hash
}

// hashLine computes a truncated SHA-256 hash of a JSON line.
func hashLine(line []byte) [16]byte

// getFileInfo returns mtime, inode, and size for change detection.
// Uses syscall.Stat_t on Unix for inode access.
func getFileInfo(path string) (mtime time.Time, inode uint64, size int64, err error)
```

### Modified Functions (cmd/apsis/main.go)

```go
// validateFollowMode checks for incompatible flag combinations.
// Returns error if -F is used with -o or -f html.
func validateFollowMode(cfg *Config) error

// resolveFollowInput resolves input to a file path (not io.Reader).
// Returns error for stdin input.
func resolveFollowInput(input string, projectPath string) (string, error)

// runFollow executes follow mode with signal handling.
func runFollow(ctx context.Context, filePath string, opts RenderOptions) error
```

---

## Data Models

### Hash-Based Entry Identification

Entries are identified by a truncated SHA-256 hash of their raw JSON line content:

```go
// [16]byte provides 128 bits of collision resistance
// Sufficient for typical session sizes (thousands of entries)
// Memory: 16 bytes per entry vs 32 bytes for full SHA-256

func hashLine(line []byte) [16]byte {
    full := sha256.Sum256(line)
    var truncated [16]byte
    copy(truncated[:], full[:16])
    return truncated
}
```

### Seen Hash Set Management

To prevent unbounded memory growth for very long sessions:

```go
const maxSeenHashes = 10000  // ~160KB of hash storage

// addSeenHash adds a hash to the seen set, resetting if cap exceeded.
func (f *Follower) addSeenHash(h [16]byte) {
    if len(f.seenHashes) >= maxSeenHashes {
        // Reset to avoid unbounded growth
        // This causes re-rendering of recent entries, which is acceptable
        f.seenHashes = make(map[[16]byte]struct{})
        f.initialRenderDone = false  // Re-render header on next batch
    }
    f.seenHashes[h] = struct{}{}
}
```

**Behavior when cap exceeded**: The seen set is cleared and `initialRenderDone` is reset. On the next poll, all entries will appear "new" and be re-rendered with the header. This is safe because:
1. Output is append-only - duplicate rendering just adds content
2. 10,000 entries is a very long session (~hours of interaction)
3. The alternative (crash or silent memory growth) is worse

**Known behavior**: Duplicate identical JSON lines (same content appearing twice in the file) will only be rendered once. This is rare for transcripts and is acceptable.

### File State Tracking

```go
type fileState struct {
    mtime time.Time  // Modification time for change detection
    inode uint64     // Inode for file replacement detection
    size  int64      // Size for truncation detection
}
```

### Renderer State and Incremental Rendering

The existing `RenderMarkdown()` function is not suitable for follow mode because:
1. It renders a header on every call
2. It builds `toolMeta` internally from the passed entries

**Solution**: Add a new `RenderEntries()` function for incremental rendering:

```go
// RenderEntries renders entries without header, using pre-built state.
// This is the incremental rendering function for follow mode.
func RenderEntries(entries []Entry, toolMeta map[string]toolMetadata,
                   skillDescriptions map[string]string, opts RenderOptions) string

// buildToolMeta accumulates tool metadata from entries.
// Exported for use by Follower.
func buildToolMeta(entries []Entry) map[string]toolMetadata
```

**Follow mode rendering flow**:
1. **Initial render**: Call `RenderMarkdown()` with all entries (includes header)
2. **Subsequent renders**: Call `RenderEntries()` with only new entries, passing accumulated state (no header)

```go
// In processFile():
if f.initialRenderDone {
    // Incremental: render only new entries, no header
    output := RenderEntries(newEntries, toolMeta, skillDescriptions, f.opts)
    f.output.Write([]byte(output))
} else {
    // Initial: render all with header
    output := RenderMarkdown(allEntries, f.opts)
    f.output.Write([]byte(output))
    f.initialRenderDone = true
}
```

The `toolMeta` and `skillDescriptions` maps are rebuilt on each poll by processing ALL parsed entries, ensuring new entries that reference earlier tool_use IDs render correctly.

---

## Error Handling

### Error Types and Responses

| Error Condition | Detection | Response | Requirement |
|-----------------|-----------|----------|-------------|
| File not found at startup | `os.Stat()` in `NewFollower()` returns `os.ErrNotExist` | Print error to stderr, exit 1 | 7.1 |
| File unreadable during follow | `os.Open()` or `os.Stat()` fails | Print error to stderr, exit 1 | 7.2 |
| Session ID not found | `resolveFollowInput()` returns error | Print error to stderr, exit 1 | 7.3 |
| Stdin with follow mode | `input == ""` check | Print "Cannot follow stdin input..." to stderr, exit 1 | 2.3, 2.4 |
| `-o` with follow mode | `cfg.Output != ""` check | Print "Cannot use --output with --follow..." to stderr, exit 1 | 5.2, 5.3 |
| HTML format with follow mode | `cfg.Format == "html"` check | Print "HTML output is not supported..." to stderr, exit 1 | 5.5, 5.6 |
| Incomplete JSON at EOF | `json.Unmarshal()` fails on last line | Skip line, retry next poll | 7.5, 7.6 |
| File truncated | `size < lastSize` | Clear seenHashes, re-render from start | 3.5 |
| File replaced (inode change) | `inode != lastInode` | Clear seenHashes, re-render from start | 3.6, 3.7 |
| SIGINT received | `ctx.Done()` in poll loop | Complete in-progress write, exit 130 | 6.1-6.4 |

### Error Message Format

All error messages follow the pattern:
```
Error: <descriptive message>
```

Written to stderr (requirement 7.4).

### File Must Exist at Startup

Per requirement 7.1, the file must exist when follow mode starts. We do **not** support waiting for a file to appear.

**Rationale**:
1. **Fail fast**: If the user specifies a non-existent session, it's likely a typo. Waiting silently would be confusing.
2. **Scope**: Waiting for file creation is a different feature (watch directory for new files).
3. **Workaround exists**: Users can use shell: `while [ ! -f file.jsonl ]; do sleep 1; done; apsis -F file.jsonl`

The file is validated in `NewFollower()` which returns an error if the file doesn't exist. This error propagates to `run()` which prints it and exits with code 1.

---

## Testing Strategy

### Unit Tests

| Component | Test Cases | Method |
|-----------|------------|--------|
| `hashLine()` | Deterministic output, different inputs produce different hashes | Table-driven |
| `getFileInfo()` | Returns correct mtime/inode/size, handles missing file | Table-driven with temp files |
| `validateFollowMode()` | Rejects -o, rejects -f html, accepts valid combinations | Table-driven |
| `resolveFollowInput()` | File path resolution, session ID resolution, stdin rejection | Table-driven with mock filesystem |

### Integration Tests

| Scenario | Setup | Verification |
|----------|-------|--------------|
| Basic follow | Create file, start follower, append entries | New entries appear in output |
| File truncation | Start follower, truncate file, append new entries | Output contains only new entries (no duplicates) |
| File replacement | Start follower, delete and recreate file | Output contains new file's entries |
| Incomplete JSON | Append partial line, wait, complete line | Entry appears after completion |
| Signal handling | Start follower, send SIGINT | Clean exit with code 130 |

### Test Helpers

```go
// testFollower creates a Follower with a temp file for testing.
// Returns the follower, file path, and cleanup function.
func testFollower(t *testing.T) (*Follower, string, func())

// appendToFile appends a JSONL entry to the test file.
func appendToFile(t *testing.T, path string, entry Entry)

// waitForOutput polls the output buffer until expected content appears.
// Uses polling with short sleep (10ms) rather than blocking.
// Timeout should be > 500ms (poll interval) + processing time.
func waitForOutput(t *testing.T, buf *bytes.Buffer, expected string, timeout time.Duration) bool
```

### Testing Timing and Race Conditions

**Problem**: The follower polls every 500ms. Tests that append entries and immediately check output will flake.

**Solution**: Test architecture that accounts for polling:

```go
func TestFollowerBasicFollow(t *testing.T) {
    f, path, cleanup := testFollower(t)
    defer cleanup()

    var buf bytes.Buffer
    f.output = &buf

    // Start follower in background
    ctx, cancel := context.WithCancel(context.Background())
    done := make(chan error, 1)
    go func() {
        done <- f.Run(ctx)
    }()

    // Append entry
    appendToFile(t, path, testEntry("hello"))

    // Wait for output with generous timeout (2x poll interval + margin)
    if !waitForOutput(t, &buf, "hello", 1500*time.Millisecond) {
        t.Fatal("expected output not found within timeout")
    }

    // Clean shutdown
    cancel()
    if err := <-done; err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
}

func waitForOutput(t *testing.T, buf *bytes.Buffer, expected string, timeout time.Duration) bool {
    t.Helper()
    deadline := time.Now().Add(timeout)
    for time.Now().Before(deadline) {
        if strings.Contains(buf.String(), expected) {
            return true
        }
        time.Sleep(10 * time.Millisecond)
    }
    return false
}
```

**Key testing principles**:
1. Timeout = 2× poll interval + 500ms margin = 1500ms minimum
2. Use polling checks, not blocking waits
3. Run follower in goroutine with cancellable context
4. Check for content containment, not exact match (avoids header timing issues)

### Acceptance Criteria Coverage

| Requirement | Test File | Test Function |
|-------------|-----------|---------------|
| 1.1-1.4 | `main_test.go` | `TestFollowFlagParsing` |
| 2.1-2.4 | `main_test.go` | `TestResolveFollowInput` |
| 3.1-3.8 | `follow_test.go` | `TestFollowerPoll`, `TestFollowerTruncation`, `TestFollowerReplacement` |
| 4.1-4.8 | `follow_test.go` | `TestFollowerIncrementalOutput`, `TestFollowerHashDedup` |
| 5.1-5.7 | `main_test.go` | `TestValidateFollowMode` |
| 6.1-6.4 | `follow_test.go` | `TestFollowerSignalHandling` |
| 7.1-7.6 | `follow_test.go` | `TestFollowerErrorHandling`, `TestFollowerIncompleteJSON` |

---

## Implementation Notes

### Platform-Specific Inode Access

```go
// Unix (Linux, macOS)
func getFileInfo(path string) (time.Time, uint64, int64, error) {
    info, err := os.Stat(path)
    if err != nil {
        return time.Time{}, 0, 0, err
    }

    stat, ok := info.Sys().(*syscall.Stat_t)
    if !ok {
        // Fallback: use 0 for inode (disable replacement detection)
        return info.ModTime(), 0, info.Size(), nil
    }

    return info.ModTime(), stat.Ino, info.Size(), nil
}
```

On Windows, `syscall.Stat_t` is not available. The implementation will use a fallback that sets inode to 0, disabling file replacement detection.

**Why this is acceptable on Windows**:

1. **Windows file locking**: Windows typically holds exclusive locks on files being written. An agent writing to a session file would prevent deletion/replacement while running. File replacement is less common than on Unix.

2. **Atomic replacement is rare**: Unix tools often use atomic replacement (write temp → rename) for crash safety. Windows applications more commonly use in-place writes with `FILE_SHARE_READ` access.

3. **Truncation detection still works**: If an agent crashes and restarts, creating a new file, the size will likely differ. Size comparison catches most practical replacement scenarios.

4. **Hash-based dedup is the safety net**: Even if we miss a replacement, the hash-based seen set will detect duplicate content and only render truly new entries. The worst case is re-rendering all entries from a replaced file (which is correct behavior).

5. **Primary platform**: Claude Code, Codex, and Copilot primarily run on macOS and Linux where inode detection works correctly.

### Stdout Flushing

Go's `os.Stdout` writes directly to file descriptor 1 without userspace buffering. When piped, the kernel buffers data until the receiving process reads it, but the write from our process completes immediately.

**Correction**: `file.Sync()` performs `fsync()` which is for durability to disk, not for pipe visibility. For stdout to a terminal or pipe, no explicit flushing is needed - `Write()` is sufficient.

However, if `output` is a `bufio.Writer` (for testing), we need to flush:

```go
// After rendering new entries
if bw, ok := f.output.(*bufio.Writer); ok {
    _ = bw.Flush()
}
// For os.Stdout or *os.File, Write() is already unbuffered
```

For production use with `os.Stdout`, the write completes synchronously and is immediately available to piped processes.

### Concurrent File Access

The JSONL file is being written by an agent process while we read it. This is safe because:

1. **POSIX append semantics**: Writes with `O_APPEND` are atomic up to `PIPE_BUF` (typically 4KB). JSONL lines are usually smaller. Even without `O_APPEND`, the kernel ensures writes don't interleave at the byte level.

2. **Line-based protocol**: Each JSONL entry is a complete line ending with `\n`. We read complete lines only - `bufio.Scanner` waits for `\n` before returning.

3. **Open per poll**: We open the file fresh on each poll with `os.Open()`, getting a new file descriptor. This avoids stale file handle issues if the file is replaced.

4. **Read-only access**: We only read; no locking required. The writer holds no exclusive lock on Unix (agents append without locking).

5. **Incomplete line handling**: If we read while a line is being written, we get a partial line that fails to parse. We skip it and catch the complete line on the next poll.

**File handle strategy**: Open, read, close on each poll rather than keeping a persistent handle. This is slightly less efficient but more robust:
- Handles file replacement (new inode)
- Avoids stale NFS handles
- Simpler error recovery

### Poll Loop Structure

```go
func (f *Follower) Run(ctx context.Context) error {
    ticker := time.NewTicker(500 * time.Millisecond)
    defer ticker.Stop()

    // Initial render
    if err := f.processFile(); err != nil {
        return err
    }

    for {
        select {
        case <-ctx.Done():
            return nil // Clean shutdown
        case <-ticker.C:
            changed, err := f.poll()
            if err != nil {
                return err
            }
            if changed {
                if err := f.processFile(); err != nil {
                    return err
                }
            }
        }
    }
}
```

### Incomplete JSON Handling

When the file is being actively written, we may read a partial JSON line. The handling mechanism:

```go
func readAndHashLines(path string, warnWriter io.Writer) ([]lineWithHash, error) {
    f, err := os.Open(path)
    if err != nil {
        return nil, fmt.Errorf("opening %s: %w", path, err)
    }
    defer f.Close()

    scanner := bufio.NewScanner(f)
    buf := make([]byte, 0, 64*1024)
    scanner.Buffer(buf, 10*1024*1024) // 10MB max line

    var lines []lineWithHash
    var pendingBadLine bool  // Track if previous line failed to parse
    var pendingLineNum int

    lineNum := 0
    for scanner.Scan() {
        lineNum++
        line := bytes.TrimSpace(scanner.Bytes()) // Handle \r\n line endings
        if len(line) == 0 {
            continue
        }

        // Try to parse to detect incomplete JSON
        var entry Entry
        if err := json.Unmarshal(line, &entry); err != nil {
            // Mark as pending bad line - might be incomplete (EOF) or corrupt (mid-file)
            pendingBadLine = true
            pendingLineNum = lineNum
            continue
        }

        // Valid JSON - if we had a pending bad line, it was corrupt (not incomplete)
        if pendingBadLine {
            fmt.Fprintf(warnWriter, "warning: line %d: skipping malformed JSON\n", pendingLineNum)
            pendingBadLine = false
        }

        lines = append(lines, lineWithHash{
            raw:  append([]byte{}, line...), // Copy
            hash: hashLine(line),
        })
    }

    // If last line failed to parse, it might be incomplete (still being written)
    // We skip it silently and will retry on next poll (requirement 7.5, 7.6)
    // Note: we don't warn here because incomplete final lines are expected

    if err := scanner.Err(); err != nil {
        return nil, fmt.Errorf("reading %s: %w", path, err)
    }
    return lines, nil
}
```

**Key behaviors**:
- Lines that fail to parse mid-file are skipped (corrupt data, matches existing parser)
- The last line that fails to parse is assumed incomplete and skipped silently
- On next poll (500ms), the line will be complete and will parse successfully

### Incremental Rendering Approach

The design re-parses the entire file on each poll but only renders new entries. This approach was chosen because:

1. **Stateful renderer**: The markdown renderer builds `toolMeta` and `skillDescriptions` maps that require processing all prior entries to correctly render new entries that reference earlier content.

2. **Simplicity**: Avoids complex state serialization between polls.

3. **Robustness**: Handles truncation and replacement scenarios naturally.

4. **Performance**: Needs validation during implementation. Estimate based on:
   - `bufio.Scanner` reads at ~1GB/s for sequential I/O
   - `json.Unmarshal` for simple structs: ~50μs per entry (conservative)
   - SHA-256 hash: ~2μs per KB
   - For 1000 entries averaging 1KB each: ~55ms total

   This is an estimate; actual benchmarks will be added during implementation. If performance is inadequate for very long sessions (10,000+ entries), we can optimize by caching parsed entries between polls.

---

## Known Limitations

| Limitation | Description | Mitigation |
|------------|-------------|------------|
| **File size** | Re-parses entire file each poll (O(N)) | Design targets files under 50MB. For larger files, consider one-time conversion instead of follow mode. |
| **Memory cap reset** | After 10,000 unique entries, seen set resets causing re-render | Rare in practice; re-render is safe (output is append-only). |
| **Duplicate suppression** | Identical JSON lines only rendered once | Rare for transcripts; documented behavior. |
| **Windows inode** | File replacement detection disabled on Windows | Truncation detection still works; hash dedup catches most cases. |
| **Parse errors** | Corrupt mid-file lines skipped with warning | Warning logged to stderr; does not block processing. |

---

## File Changes Summary

| File | Change Type | Description |
|------|-------------|-------------|
| `cmd/apsis/main.go` | Modify | Add Follow flag, validateFollowMode(), resolveFollowInput(), runFollow() |
| `internal/transcript/follow.go` | New | Follower struct and methods, readAndHashLines(), lineWithHash type |
| `internal/transcript/markdown.go` | Modify | Add RenderEntries() for incremental rendering without header, export buildToolMeta() |
| `internal/transcript/follow_test.go` | New | Unit and integration tests for Follower |
| `cmd/apsis/main_test.go` | Modify | Add tests for follow mode flags and validation |
