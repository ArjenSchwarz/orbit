package sessions

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/arjenschwarz/orbit/internal/transcript"
)

// execThreadPrefixBytes bounds the readable thread prefix of an exec stream
// identifier, so prefix, separator and 64-hex digest fit in 256 characters.
const execThreadPrefixBytes = 191

// pathDigest is the full SHA-256 hex of a canonical path. Tests replace it to
// force identifier conflicts between distinct paths.
var pathDigest = func(canonicalPath string) string {
	sum := sha256.Sum256([]byte(canonicalPath))
	return hex.EncodeToString(sum[:])
}

// ExecIdentifier derives a Codex exec stream's identifier from its recorded
// thread ID and canonical path (Decision 14): the thread ID with every byte
// outside [A-Za-z0-9_-] replaced by "_" and truncated to 191 bytes, "-", then
// the path digest. It depends on nothing but the stream itself, so appends,
// copies elsewhere and root order cannot change it.
func ExecIdentifier(threadID, canonicalPath string) string {
	prefix := make([]byte, 0, min(len(threadID), execThreadPrefixBytes))
	for i := 0; i < len(threadID) && len(prefix) < execThreadPrefixBytes; i++ {
		c := threadID[i]
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-' {
			prefix = append(prefix, c)
		} else {
			prefix = append(prefix, '_')
		}
	}
	return string(prefix) + "-" + pathDigest(canonicalPath)
}

// CanonicalPath makes path absolute, resolves its symlinks and rewrites each
// component to its on-disk spelling: an exact directory entry wins, otherwise
// a unique case-insensitive match. Case aliases on case-insensitive volumes
// therefore canonicalise to one path.
func CanonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	volume := filepath.VolumeName(resolved)
	current := volume + string(filepath.Separator)
	parts := strings.Split(strings.Trim(resolved[len(volume):], string(filepath.Separator)), string(filepath.Separator))
	for i, part := range parts {
		if part == "" {
			continue
		}
		entries, err := os.ReadDir(current)
		if err != nil {
			// An unlistable directory keeps the remaining spelling as resolved.
			return filepath.Join(append([]string{current}, parts[i:]...)...), nil
		}
		current = filepath.Join(current, onDiskName(entries, part))
	}
	return current, nil
}

// onDiskName returns the directory entry name that part refers to.
func onDiskName(entries []os.DirEntry, part string) string {
	match := ""
	for _, e := range entries {
		name := e.Name()
		if name == part {
			return name
		}
		if strings.EqualFold(name, part) {
			if match != "" {
				return part // ambiguous: keep the given spelling
			}
			match = name
		}
	}
	if match != "" {
		return match
	}
	return part
}

// RepositoryRoot returns the nearest directory at or above dir that holds a
// .git directory, or a .git file starting with "gitdir:" (a worktree). It
// returns "" outside any repository. No git executable is run (Q52).
func RepositoryRoot(dir string) string {
	for {
		gitPath := filepath.Join(dir, ".git")
		if info, err := os.Stat(gitPath); err == nil {
			if info.IsDir() || (info.Mode().IsRegular() && isGitdirFile(gitPath)) {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// isGitdirFile reports whether a .git file points at a git directory.
func isGitdirFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, len("gitdir:"))
	n, _ := io.ReadFull(f, head)
	return bytes.Equal(head[:n], []byte("gitdir:"))
}

// ExecStream describes a Codex exec stream file.
type ExecStream struct {
	Path     string                      // canonical path
	Outcome  transcript.ExecProbeOutcome // what the first JSON object says the file is
	ThreadID string                      // recorded thread ID; set when Outcome is exec
	ID       string                      // derived identifier; set when Outcome is exec
	Project  string                      // containing repository or worktree root; "" outside one
}

// IdentifyExecStream canonicalises path, probes the file's first JSON object
// and derives its identifier and project.
func IdentifyExecStream(path string) (ExecStream, error) {
	canonical, err := CanonicalPath(path)
	if err != nil {
		return ExecStream{}, err
	}
	probe, err := probeExecFile(canonical)
	if err != nil {
		return ExecStream{}, err
	}
	stream := ExecStream{
		Path:     canonical,
		Outcome:  probe.Outcome,
		ThreadID: probe.ThreadID,
		Project:  RepositoryRoot(filepath.Dir(canonical)),
	}
	if probe.Outcome == transcript.ExecProbeExec {
		stream.ID = ExecIdentifier(probe.ThreadID, canonical)
	}
	return stream, nil
}

// execSkipNames are subdirectories recursive discovery never enters; a
// configured root is scanned whatever its name (Q27).
var execSkipNames = map[string]bool{".git": true, "node_modules": true, "testdata": true}

// execCandidate is a .jsonl file found under the configured roots.
type execCandidate struct {
	path string // canonical path
	info os.FileInfo
}

// canonicalExecRoots canonicalises configured roots and drops repeats.
// A root that cannot be resolved, or is not a directory, yields a warning.
func canonicalExecRoots(configured []string) ([]string, []ListWarning) {
	var roots []string
	var warnings []ListWarning
	for _, root := range configured {
		canonical, err := CanonicalPath(root)
		if err == nil {
			var info os.FileInfo
			if info, err = os.Stat(canonical); err == nil && !info.IsDir() {
				err = fmt.Errorf("%s is not a directory", canonical)
			}
		}
		if err != nil {
			warnings = append(warnings, ListWarning{Source: SourceCodexExec, Code: WarningRootUnreadable,
				Err: fmt.Errorf("exec root %s: %w", root, err)})
			continue
		}
		if !slices.Contains(roots, canonical) {
			roots = append(roots, canonical)
		}
	}
	return roots, warnings
}

// withinAnyRoot reports whether a canonical path lies inside a canonical root.
func withinAnyRoot(path string, roots []string) bool {
	return slices.ContainsFunc(roots, func(root string) bool {
		rel, err := filepath.Rel(root, path)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	})
}

// findExecCandidates walks every root without descending directory symlinks
// and returns each canonical .jsonl file once, in path order. A .jsonl
// symlink joins as its target when that is a regular file inside a root.
func findExecCandidates(roots []string) ([]execCandidate, []ListWarning) {
	found := make(map[string]os.FileInfo)
	var warnings []ListWarning
	unreadable := func(path string, err error) {
		warnings = append(warnings, ListWarning{Source: SourceCodexExec, Code: WarningPathUnreadable,
			Err: fmt.Errorf("%s: %w", path, err)})
	}
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if path == root {
					warnings = append(warnings, ListWarning{Source: SourceCodexExec, Code: WarningRootUnreadable,
						Err: fmt.Errorf("exec root %s: %w", root, err)})
				} else {
					unreadable(path, err)
				}
				return nil
			}
			if d.IsDir() {
				if path != root && execSkipNames[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(d.Name(), ".jsonl") {
				return nil
			}
			target := path
			if d.Type()&fs.ModeSymlink != 0 {
				if target, err = CanonicalPath(path); err != nil {
					unreadable(path, err)
					return nil
				}
				if !withinAnyRoot(target, roots) {
					return nil
				}
			}
			info, err := os.Stat(target)
			if err != nil {
				unreadable(path, err)
				return nil
			}
			if info.Mode().IsRegular() {
				found[target] = info
			}
			return nil
		})
	}
	candidates := make([]execCandidate, 0, len(found))
	for path, info := range found {
		candidates = append(candidates, execCandidate{path: path, info: info})
	}
	slices.SortFunc(candidates, func(a, b execCandidate) int { return strings.Compare(a.path, b.path) })
	return candidates, warnings
}

// listCodexExec lists the exec streams under the configured roots
// (requirements 2.5-2.8). A stream whose thread ID a verified rollout records
// is dropped before project filtering, in every scope (Decision 22).
func (l *Lister) listCodexExec(projectPath string, rolloutThreads map[string]bool) ([]SessionInfo, []ListWarning) {
	if len(l.opts.CodexExecRoots) == 0 {
		return nil, nil
	}
	roots, warnings := canonicalExecRoots(l.opts.CodexExecRoots)
	candidates, walkWarnings := findExecCandidates(roots)
	warnings = append(warnings, walkWarnings...)

	type stream struct {
		candidate execCandidate
		threadID  string
	}
	byID := make(map[string][]stream)
	var ids []string
	for _, c := range candidates {
		probe, err := probeExecFile(c.path)
		if err != nil {
			warnings = append(warnings, ListWarning{Source: SourceCodexExec, Code: WarningPathUnreadable, Err: err})
			continue
		}
		switch probe.Outcome {
		case transcript.ExecProbeWithoutThread:
			warnings = append(warnings, ListWarning{Source: SourceCodexExec, Code: WarningMissingThread,
				Err: fmt.Errorf("%s has no thread.started event first; omitted", c.path)})
		case transcript.ExecProbeExec:
			id := ExecIdentifier(probe.ThreadID, c.path)
			if _, seen := byID[id]; !seen {
				ids = append(ids, id)
			}
			byID[id] = append(byID[id], stream{candidate: c, threadID: probe.ThreadID})
		}
	}

	repoRoots := make(map[string]string) // directory -> repository root, for this pass only
	var sessions []SessionInfo
	for _, id := range ids {
		group := byID[id]
		if len(group) > 1 {
			warnings = append(warnings, ListWarning{Source: SourceCodexExec, Code: WarningIdentifierConflict,
				Err: fmt.Errorf("%d streams derive identifier %s; all omitted", len(group), id)})
			continue
		}
		s := group[0]
		if rolloutThreads[strings.ToLower(s.threadID)] {
			continue
		}
		dir := filepath.Dir(s.candidate.path)
		project, known := repoRoots[dir]
		if !known {
			project = RepositoryRoot(dir)
			repoRoots[dir] = project
		}
		if projectPath != "" && (project == "" || normalizePath(project) != normalizePath(projectPath)) {
			continue
		}
		sessions = append(sessions, SessionInfo{
			ID:                 id,
			CreatedAt:          s.candidate.info.ModTime(),
			TimeFromFilesystem: true,
			Size:               s.candidate.info.Size(),
			Source:             SourceCodexExec,
			Project:            project,
			ThreadID:           s.threadID,
		})
	}
	return sessions, warnings
}

// probeExecFile classifies the stream at path by its first JSON object.
func probeExecFile(path string) (transcript.ExecProbe, error) {
	f, err := os.Open(path)
	if err != nil {
		return transcript.ExecProbe{}, err
	}
	defer func() { _ = f.Close() }()
	probe, err := transcript.ProbeExecStream(f)
	if err != nil {
		return transcript.ExecProbe{}, fmt.Errorf("%s: %w", path, err)
	}
	return probe, nil
}

// locateCodexExec finds the stream an identifier names, in either scope:
// it hashes the canonical path of each candidate from a metadata-only walk of
// the roots, probes only a file whose digest matches, and requires the whole
// identifier to match. Project scope does not filter, and suppression does
// not apply, so every derived identifier keeps opening (requirement 2.7).
func (r *Resolver) locateCodexExec(id string) (string, string, error) {
	notFound := fmt.Errorf("session not found: %s", id)
	sep := strings.LastIndexByte(id, '-')
	if len(r.opts.CodexExecRoots) == 0 || sep < 0 || len(id)-sep-1 != sha256.Size*2 {
		return "", "", notFound
	}
	digest := id[sep+1:]
	roots, _ := canonicalExecRoots(r.opts.CodexExecRoots)
	candidates, _ := findExecCandidates(roots)
	var path, threadID string
	matches := 0
	for _, c := range candidates {
		if pathDigest(c.path) != digest {
			continue
		}
		probe, err := probeExecFile(c.path)
		if err != nil || probe.Outcome != transcript.ExecProbeExec || ExecIdentifier(probe.ThreadID, c.path) != id {
			continue
		}
		path, threadID = c.path, probe.ThreadID
		matches++
	}
	if matches != 1 {
		// Conflicting identifiers are omitted from listings, so neither opens.
		return "", "", notFound
	}
	return path, threadID, nil
}

// resolveCodexExec opens the stream an identifier names. The file's
// modification time is its only time, marked as a filesystem time.
func (r *Resolver) resolveCodexExec(id string) (*ResolvedSession, error) {
	path, threadID, err := r.locateCodexExec(id)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	// Recheck containment on the opened path, in case it changed after the walk.
	roots, _ := canonicalExecRoots(r.opts.CodexExecRoots)
	if canonical, err := CanonicalPath(path); err != nil || canonical != path || !withinAnyRoot(canonical, roots) {
		_ = f.Close()
		return nil, fmt.Errorf("session not found: %s", id)
	}
	return &ResolvedSession{Reader: f, Metadata: SessionMetadata{
		Source:             SourceCodexExec,
		ID:                 id,
		CreatedAt:          info.ModTime(),
		TimeFromFilesystem: true,
		Size:               info.Size(),
		Project:            RepositoryRoot(filepath.Dir(path)),
		Path:               path,
		ThreadID:           threadID,
	}}, nil
}
