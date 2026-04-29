package transcript

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

const (
	// maxSeenHashes is the maximum number of entry hashes to track before resetting.
	// At 16 bytes per hash, this uses ~160KB of memory.
	maxSeenHashes = 10000

	// maxScanLineSize is the maximum line size for the scanner (10MB).
	maxScanLineSize = 10 * 1024 * 1024

	// initialScanBuffer is the initial buffer size for the scanner (64KB).
	initialScanBuffer = 64 * 1024

	// PollInterval is the interval between file change checks.
	// Exported for testing configurability.
	PollInterval = 500 * time.Millisecond
)

// scanBufferPool provides reusable scanner buffers to reduce memory allocation
// pressure during high-frequency polling.
var scanBufferPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 0, initialScanBuffer)
		return &buf
	},
}

// Follower monitors a JSONL file and renders new entries incrementally.
type Follower struct {
	filePath string
	output   io.Writer
	warn     io.Writer // Destination for warning messages (defaults to os.Stderr)

	// Deduplication state
	seenHashes    map[[16]byte]struct{}
	renderedCount int // Number of entries already rendered (high-water mark for cap resets)

	// File change detection
	lastMtime time.Time
	lastInode uint64
	lastSize  int64

	// Render state
	initialRenderDone bool // True after first render (header already written)
	opts              RenderOptions
}

// NewFollower creates a new Follower for the given file path.
// Validates file exists at creation time (fails fast per requirement 7.1).
func NewFollower(filePath string, output io.Writer, opts RenderOptions) (*Follower, error) {
	// Validate file exists (requirement 7.1)
	if _, err := os.Stat(filePath); err != nil {
		return nil, fmt.Errorf("file not found: %s", filePath)
	}

	return &Follower{
		filePath:   filePath,
		output:     output,
		warn:       os.Stderr,
		seenHashes: make(map[[16]byte]struct{}),
		opts:       opts,
	}, nil
}

// poll checks for file changes and returns whether the file changed.
// Clears seenHashes on truncation (size decrease) or replacement (inode change).
func (f *Follower) poll() (bool, error) {
	mtime, inode, size, err := getFileInfo(f.filePath)
	if err != nil {
		return false, fmt.Errorf("checking file: %w", err)
	}

	// Detect file truncation or replacement
	truncated := size < f.lastSize
	replaced := f.lastInode != 0 && inode != 0 && inode != f.lastInode

	if truncated || replaced {
		// Clear state and re-render from beginning
		f.seenHashes = make(map[[16]byte]struct{})
		f.initialRenderDone = false
		f.renderedCount = 0
		f.lastMtime = mtime
		f.lastInode = inode
		f.lastSize = size
		return true, nil
	}

	// Check for modification - use mtime change OR size increase as fallback
	// for filesystems with coarse mtime resolution (e.g., 1-second granularity)
	changed := !mtime.Equal(f.lastMtime) || size > f.lastSize
	f.lastMtime = mtime
	f.lastInode = inode
	f.lastSize = size

	return changed, nil
}

// processFile reads, parses, and renders new entries.
// Builds toolMeta/skillDescriptions from all entries, then renders only unseen entries.
func (f *Follower) processFile() error {
	lines, err := readAndHashLines(f.filePath, f.warn)
	if err != nil {
		return err
	}

	// Parse all entries and collect unseen entries with their hashes
	var allEntries []Entry
	var newEntries []Entry
	var newHashes [][16]byte

	for i, lh := range lines {
		var entry Entry
		if err := json.Unmarshal(lh.raw, &entry); err != nil {
			// Already filtered by readAndHashLines, shouldn't happen
			continue
		}
		allEntries = append(allEntries, entry)

		// Entries before renderedCount are already output; skip them even if
		// their hash was evicted during a cap reset.
		if i < f.renderedCount {
			continue
		}

		// Check if this entry is new
		if _, seen := f.seenHashes[lh.hash]; !seen {
			newEntries = append(newEntries, entry)
			newHashes = append(newHashes, lh.hash)
		}
	}

	// No new entries to render
	if len(newEntries) == 0 {
		return nil
	}

	// Build state from ALL entries for correct rendering context
	toolMeta := BuildToolMeta(allEntries)
	skillDescriptions := BuildSkillDescriptionMap(allEntries)

	// Render
	var output string
	if f.initialRenderDone {
		// Incremental: render only new entries without header
		output = RenderEntries(newEntries, toolMeta, skillDescriptions, f.opts)
	} else {
		// Initial: render all with header
		output = RenderMarkdown(allEntries, f.opts)
		f.initialRenderDone = true
	}

	// Write output
	if _, err := f.output.Write([]byte(output)); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	// Flush if output supports it (requirement 4.8)
	if flusher, ok := f.output.(interface{ Flush() error }); ok {
		_ = flusher.Flush()
	}

	// Update seen hashes
	for _, h := range newHashes {
		f.addSeenHash(h)
	}

	// Update high-water mark so cap resets don't replay these entries
	f.renderedCount = len(allEntries)

	return nil
}

// addSeenHash adds a hash to the seen set, resetting if cap exceeded.
func (f *Follower) addSeenHash(h [16]byte) {
	if len(f.seenHashes) >= maxSeenHashes {
		// Reset to avoid unbounded growth. renderedCount prevents replay
		// of already-emitted entries after the map is cleared.
		f.seenHashes = make(map[[16]byte]struct{})
	}
	f.seenHashes[h] = struct{}{}
}

// Run starts the follow loop. Blocks until ctx is cancelled.
// Returns nil on clean shutdown (context cancellation), error on file errors.
func (f *Follower) Run(ctx context.Context) error {
	ticker := time.NewTicker(PollInterval)
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

// lineWithHash holds a raw JSON line and its precomputed hash.
type lineWithHash struct {
	raw  []byte   // Original bytes for parsing
	hash [16]byte // SHA-256 truncated to 16 bytes
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

// readAndHashLines reads a file line by line, returning raw lines and their hashes.
// Skips incomplete JSON at EOF (detected by parse failure on last non-empty line).
// Logs warning to warnWriter for corrupt mid-file lines.
// Uses sync.Pool for scanner buffers to reduce memory allocation pressure.
func readAndHashLines(path string, warnWriter io.Writer) ([]lineWithHash, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	bufPtr := scanBufferPool.Get().(*[]byte)
	buf := *bufPtr
	scanner.Buffer(buf, maxScanLineSize)
	defer func() {
		*bufPtr = buf[:0]
		scanBufferPool.Put(bufPtr)
	}()

	lines := make([]lineWithHash, 0, 128)
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
			// If there's already a pending bad line, it's provably mid-file
			// (this new line follows it), so warn about it now.
			if pendingBadLine != nil {
				_, _ = fmt.Fprintf(warnWriter, "warning: line %d: skipping malformed JSON\n", pendingLineNum)
			}
			pendingBadLine = bytes.Clone(line)
			pendingLineNum = lineNum
			continue
		}

		// Valid JSON - if we had a pending bad line, it was corrupt (not incomplete)
		if pendingBadLine != nil {
			_, _ = fmt.Fprintf(warnWriter, "warning: line %d: skipping malformed JSON\n", pendingLineNum)
			pendingBadLine = nil
		}

		lines = append(lines, lineWithHash{
			raw:  bytes.Clone(line),
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
