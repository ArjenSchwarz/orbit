package transcript

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"
)

// lineWithHash holds a raw JSON line and its precomputed hash.
type lineWithHash struct {
	raw  []byte    // Original bytes for parsing
	hash [16]byte  // SHA-256 truncated to 16 bytes
}

// hashLine computes a truncated SHA-256 hash of a JSON line.
// Returns first 16 bytes (128 bits) for memory efficiency while
// maintaining sufficient collision resistance.
func hashLine(line []byte) [16]byte {
	full := sha256.Sum256(line)
	var truncated [16]byte
	copy(truncated[:], full[:16])
	return truncated
}

// getFileInfo returns mtime, inode, and size for file change detection.
// On Unix systems, inode is retrieved via syscall.Stat_t.
// On non-Unix platforms, inode is set to 0 (disabling replacement detection).
func getFileInfo(path string) (mtime time.Time, inode uint64, size int64, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, 0, 0, err
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		// Fallback: use 0 for inode (disables file replacement detection)
		return info.ModTime(), 0, info.Size(), nil
	}

	return info.ModTime(), stat.Ino, info.Size(), nil
}

// readAndHashLines reads a file line by line, returning raw lines and their hashes.
// Skips incomplete JSON at EOF (detected by parse failure on last non-empty line).
// Logs warning to warnWriter for corrupt mid-file lines.
func readAndHashLines(path string, warnWriter io.Writer) ([]lineWithHash, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024) // 10MB max line

	var lines []lineWithHash
	var pendingBadLine []byte // Track if previous line failed to parse
	var pendingLineNum int

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		// Try to parse to detect incomplete JSON
		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			// Mark as pending bad line - might be incomplete (EOF) or corrupt (mid-file)
			pendingBadLine = append([]byte{}, line...) // Copy
			pendingLineNum = lineNum
			continue
		}

		// Valid JSON - if we had a pending bad line, it was corrupt (not incomplete)
		if pendingBadLine != nil {
			_, _ = fmt.Fprintf(warnWriter, "warning: line %d: skipping malformed JSON\n", pendingLineNum)
			pendingBadLine = nil
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
