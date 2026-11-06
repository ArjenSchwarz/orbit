package sessions

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents/claudecode"
	"github.com/arjenschwarz/orbit/internal/agents/kiro/logs"
	"github.com/arjenschwarz/orbit/internal/transcript"
	"github.com/arjenschwarz/orbit/internal/web"
)

// Resolver finds and opens a specific session by source and ID.
type Resolver struct {
	projectPath string
	homeDir     string
	opts        Options
}

// NewResolver creates a Resolver for the given project.
// Resolves os.UserHomeDir() once during construction.
func NewResolver(projectPath string, opts ...Option) (*Resolver, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	return &Resolver{projectPath: projectPath, homeDir: homeDir, opts: applyOptions(opts)}, nil
}

// Resolve locates a session and returns a reader and metadata.
// The caller is responsible for closing the reader.
func (r *Resolver) Resolve(source, sessionID string) (*ResolvedSession, error) {
	if !IsValidSource(source) {
		return nil, fmt.Errorf("unknown source: %s", source)
	}

	switch source {
	case SourceClaude:
		return r.resolveClaude(sessionID)
	case SourceCodex:
		return r.resolveCodex(sessionID)
	case SourceCopilot:
		return r.resolveCopilot(sessionID)
	case SourceKiroCLI:
		return r.resolveKiroCLI(sessionID)
	case SourceKiroIDE:
		return r.resolveKiroIDE(sessionID)
	case SourceCodexExec:
		return r.resolveCodexExec(sessionID)
	default:
		return nil, fmt.Errorf("unknown source: %s", source)
	}
}

func (r *Resolver) resolveClaude(sessionID string) (*ResolvedSession, error) {
	path, err := r.findClaudeSessionPath(sessionID)
	if err != nil {
		return nil, err
	}
	return r.openFileSession(path, SourceClaude, sessionID)
}

// findClaudeSessionPath locates a Claude session file. When projectPath is set,
// it looks in the specific project directory. When empty, it searches all project
// subdirectories under ~/.claude/projects/.
func (r *Resolver) findClaudeSessionPath(sessionID string) (string, error) {
	if strings.ContainsAny(sessionID, "/\\") || strings.Contains(sessionID, "..") {
		return "", fmt.Errorf("session not found: %s", sessionID)
	}

	projectsRoot := filepath.Join(r.homeDir, ".claude", "projects")

	if r.projectPath != "" {
		claudeProjectPath := claudecode.BuildProjectPath(r.projectPath)
		baseDir := filepath.Join(projectsRoot, claudeProjectPath)
		sessionFile := filepath.Join(baseDir, sessionID+".jsonl")
		if !web.IsPathWithinDir(sessionFile, baseDir) {
			return "", fmt.Errorf("session not found: %s", sessionID)
		}
		if _, err := os.Stat(sessionFile); err != nil {
			return "", fmt.Errorf("session not found: %s", sessionID)
		}
		return sessionFile, nil
	}

	// Search all project subdirectories.
	entries, err := os.ReadDir(projectsRoot)
	if err != nil {
		return "", fmt.Errorf("session not found: %s", sessionID)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		baseDir := filepath.Join(projectsRoot, entry.Name())
		sessionFile := filepath.Join(baseDir, sessionID+".jsonl")
		if _, err := os.Stat(sessionFile); err == nil {
			if !web.IsPathWithinDir(sessionFile, projectsRoot) {
				continue
			}
			return sessionFile, nil
		}
	}
	return "", fmt.Errorf("session not found: %s", sessionID)
}

func (r *Resolver) resolveCodex(sessionID string) (*ResolvedSession, error) {
	codexPath, err := selectCodexCandidate(r.homeDir, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to search Codex sessions: %w", err)
	}
	if codexPath == "" {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	baseDir := filepath.Join(r.homeDir, ".codex", "sessions")
	if !web.IsPathWithinDir(codexPath, baseDir) {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	return r.openFileSession(codexPath, SourceCodex, sessionID)
}

func (r *Resolver) resolveCopilot(sessionID string) (*ResolvedSession, error) {
	copilotPath, err := findCopilotSession(r.homeDir, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to search Copilot sessions: %w", err)
	}
	if copilotPath == "" {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	baseDir := filepath.Join(r.homeDir, ".copilot", "session-state")
	if !web.IsPathWithinDir(copilotPath, baseDir) {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	return r.openFileSession(copilotPath, SourceCopilot, sessionID)
}

// resolveKiroCLI opens a Kiro CLI conversation stored under the project's key,
// or under any key in all-projects scope, choosing the row the listing shows.
func (r *Resolver) resolveKiroCLI(sessionID string) (*ResolvedSession, error) {
	var record *logs.SessionRecord
	var err error
	if r.projectPath == "" {
		record, err = logs.GetSessionByID(context.Background(), sessionID)
	} else {
		record, err = logs.GetSessionRecord(context.Background(), sessionID, r.projectPath)
	}
	if err != nil {
		if errors.Is(err, logs.ErrSessionNotFound) || errors.Is(err, logs.ErrDatabaseNotFound) {
			return nil, fmt.Errorf("session not found: %s", sessionID)
		}
		return nil, fmt.Errorf("kiro lookup: %w", err)
	}

	return &ResolvedSession{
		Reader: io.NopCloser(bytes.NewReader(record.Value)),
		Metadata: SessionMetadata{
			Source:            SourceKiroCLI,
			ID:                sessionID,
			Size:              0, // SQLite-backed, size unknown
			Project:           record.Key,
			DatabaseCreatedAt: record.CreatedAt,
		},
	}, nil
}

func (r *Resolver) resolveKiroIDE(sessionID string) (*ResolvedSession, error) {
	bestPath, err := r.locateKiroIDE(sessionID)
	if err != nil {
		return nil, err
	}
	workspaceDir := filepath.Dir(bestPath)

	f, err := os.Open(bestPath)
	if err != nil {
		return nil, err
	}

	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}

	createdAt, fromFilesystem := kiroIDECreatedAt(f, info.ModTime())
	costPath := transcript.KiroIDEExecutionDetailPath(workspaceDir, sessionID)

	return &ResolvedSession{
		Reader: f,
		Metadata: SessionMetadata{
			Source:             SourceKiroIDE,
			ID:                 sessionID,
			Size:               info.Size(),
			CreatedAt:          createdAt,
			TimeFromFilesystem: fromFilesystem,
			CostPath:           costPath,
			Path:               bestPath,
		},
	}, nil
}

// kiroIDECreatedAt extracts the startTime from a Kiro IDE .chat file's metadata,
// matching the behaviour of listKiroIDE. Falls back to modTime if metadata is
// absent or startTime is zero, reporting the fallback. Seeks the reader back to
// the start after parsing.
func kiroIDECreatedAt(rs io.ReadSeeker, modTime time.Time) (createdAt time.Time, fromFilesystem bool) {
	defer func() { _, _ = rs.Seek(0, io.SeekStart) }()
	var header kiroIDEChatHeader
	if err := json.NewDecoder(rs).Decode(&header); err == nil {
		if header.Metadata != nil && header.Metadata.StartTime > 0 {
			return time.UnixMilli(header.Metadata.StartTime), false
		}
	}
	return modTime, true
}

// ResolvePath returns the file path for a session, without opening it.
// This is needed for follow mode which requires a path rather than a reader.
// Returns an error for Kiro CLI sessions (SQLite-backed, no file path).
func (r *Resolver) ResolvePath(source, sessionID string) (string, error) {
	if !IsValidSource(source) {
		return "", fmt.Errorf("unknown source: %s", source)
	}

	switch source {
	case SourceClaude:
		return r.findClaudeSessionPath(sessionID)
	case SourceCodex:
		path, err := selectCodexCandidate(r.homeDir, sessionID)
		if err != nil {
			return "", err
		}
		if path == "" {
			return "", fmt.Errorf("session not found: %s", sessionID)
		}
		baseDir := filepath.Join(r.homeDir, ".codex", "sessions")
		if !web.IsPathWithinDir(path, baseDir) {
			return "", fmt.Errorf("session not found: %s", sessionID)
		}
		return path, nil
	case SourceCopilot:
		path, err := findCopilotSession(r.homeDir, sessionID)
		if err != nil {
			return "", err
		}
		if path == "" {
			return "", fmt.Errorf("session not found: %s", sessionID)
		}
		baseDir := filepath.Join(r.homeDir, ".copilot", "session-state")
		if !web.IsPathWithinDir(path, baseDir) {
			return "", fmt.Errorf("session not found: %s", sessionID)
		}
		return path, nil
	case SourceKiroCLI:
		return "", fmt.Errorf("kiro-cli sessions are SQLite-backed and have no file path")
	case SourceKiroIDE:
		return r.locateKiroIDE(sessionID)
	case SourceCodexExec:
		path, _, err := r.locateCodexExec(sessionID)
		return path, err
	default:
		return "", fmt.Errorf("unknown source: %s", source)
	}
}

// locateKiroIDE returns the .chat file to open for an execution: the best
// candidate in the project's workspace, or across every workspace in
// all-projects scope, under the rule the listing applies.
func (r *Resolver) locateKiroIDE(sessionID string) (string, error) {
	notFound := fmt.Errorf("session not found: %s", sessionID)
	if r.projectPath != "" {
		workspaceDir, err := transcript.KiroIDEWorkspaceDir(r.projectPath)
		if err != nil {
			return "", notFound
		}
		return r.findKiroIDEPath(workspaceDir, sessionID)
	}

	base, err := transcript.KiroIDEBasePath()
	if err != nil {
		return "", notFound
	}
	all, err := kiroIDEAllCandidates(base)
	if err != nil || len(all[sessionID]) == 0 {
		return "", notFound
	}
	return bestKiroIDECandidate(all[sessionID]).path, nil
}

// findKiroIDEPath finds the best .chat file path for a Kiro IDE session in
// one workspace. Symlinks resolving outside the workspace are skipped inside
// the scan, so they cannot shadow legitimate files.
func (r *Resolver) findKiroIDEPath(workspaceDir, sessionID string) (string, error) {
	best, err := kiroIDEWorkspaceCandidates(workspaceDir)
	if err != nil {
		return "", err
	}
	candidate, ok := best[sessionID]
	if !ok {
		return "", fmt.Errorf("session not found: %s", sessionID)
	}
	return candidate.path, nil
}

// openFileSession opens a file and returns a ResolvedSession with metadata.
// It derives CreatedAt using the same source-specific logic as the lister
// (transcript timestamps, session metadata, or workspace metadata) rather
// than relying solely on file modification time.
func (r *Resolver) openFileSession(path, source, sessionID string) (*ResolvedSession, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}

	metadata := SessionMetadata{
		Source: source,
		ID:     sessionID,
		Size:   info.Size(),
		Path:   path,
	}
	resolveFileMetadata(&metadata, f, info.ModTime())

	return &ResolvedSession{
		Reader:   f,
		Metadata: metadata,
	}, nil
}

// resolveFileMetadata derives the session start timestamp using the same
// source-specific logic as the lister, falling back to modTime and marking the
// fallback. It also records a Copilot workspace's project. For Claude, the
// file is seeked back to the start after parsing.
func resolveFileMetadata(metadata *SessionMetadata, f *os.File, modTime time.Time) {
	switch metadata.Source {
	case SourceClaude:
		ts, err := transcript.ParseFirstTimestamp(f)
		_, _ = f.Seek(0, io.SeekStart)
		if err == nil {
			metadata.CreatedAt = ts
			return
		}
	case SourceCodex:
		if head, _ := readCodexHead(metadata.Path); !head.Timestamp.IsZero() {
			metadata.CreatedAt = head.Timestamp
			return
		}
	case SourceCopilot:
		wsPath := filepath.Join(filepath.Dir(metadata.Path), "workspace.yaml")
		if ws, err := parseCopilotWorkspace(wsPath); err == nil && ws != nil {
			metadata.Project = cmp.Or(ws.GitRoot, ws.Cwd)
			if ws.CreatedAt != nil {
				metadata.CreatedAt = *ws.CreatedAt
				return
			}
		}
	}
	metadata.CreatedAt = modTime
	metadata.TimeFromFilesystem = true
}

// selectCodexCandidate returns the first rollout in walk order under
// ~/.codex/sessions/ whose name matches sessionID. UUID IDs are matched
// against UUID substrings in filenames. Non-UUID IDs are matched against the
// full basename without the .jsonl extension, which is the same format that
// listCodex returns for files without a UUID. Matching ignores case. Resolution
// and all-projects listing share this choice (Decision 22).
func selectCodexCandidate(homeDir, sessionID string) (string, error) {
	// Reject IDs containing path separators or traversal sequences.
	if strings.ContainsAny(sessionID, "/\\") || strings.Contains(sessionID, "..") {
		return "", nil
	}

	isUUID := isCodexUUID(sessionID)
	normalizedID := strings.ToLower(sessionID)

	var foundPath string
	err := walkCodexRollouts(homeDir, func(path string, _ fs.DirEntry) error {
		filename := filepath.Base(path)
		var name string
		if isUUID {
			name = uuidPattern.FindString(filename)
		} else {
			name = strings.TrimSuffix(filename, ".jsonl")
		}
		// A match inside a followed directory symlink stops only that inner
		// walk, so keep the first match rather than a later one.
		if foundPath == "" && strings.ToLower(name) == normalizedID {
			foundPath = path
		}
		if foundPath != "" {
			return filepath.SkipAll
		}
		return nil
	})

	return foundPath, err
}

// isCodexUUID reports whether a Codex identifier is a UUID rather than a basename.
func isCodexUUID(id string) bool {
	return len(id) == 36 && uuidPattern.MatchString(id)
}

// findCopilotSession searches ~/.copilot/session-state/ for a session by UUID.
func findCopilotSession(homeDir, sessionID string) (string, error) {
	if len(sessionID) != 36 || !uuidPattern.MatchString(sessionID) {
		return "", nil
	}

	normalizedID := strings.ToLower(sessionID)
	sessionDir := filepath.Join(homeDir, ".copilot", "session-state")

	if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
		return "", nil
	}

	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		return "", err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		if strings.ToLower(entry.Name()) != normalizedID {
			continue
		}

		eventsPath := filepath.Join(sessionDir, entry.Name(), "events.jsonl")
		if _, err := os.Stat(eventsPath); err == nil {
			return eventsPath, nil
		}
	}

	return "", nil
}
