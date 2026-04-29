package sessions

import (
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
}

// NewResolver creates a Resolver for the given project.
// Resolves os.UserHomeDir() once during construction.
func NewResolver(projectPath string) (*Resolver, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	return &Resolver{projectPath: projectPath, homeDir: homeDir}, nil
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
	codexPath, err := findCodexSession(r.homeDir, sessionID)
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

func (r *Resolver) resolveKiroCLI(sessionID string) (*ResolvedSession, error) {
	reader, err := logs.GetSession(context.Background(), sessionID, r.projectPath)
	if err != nil {
		if errors.Is(err, logs.ErrSessionNotFound) || errors.Is(err, logs.ErrDatabaseNotFound) {
			return nil, fmt.Errorf("session not found: %s", sessionID)
		}
		return nil, fmt.Errorf("kiro lookup: %w", err)
	}

	return &ResolvedSession{
		Reader: io.NopCloser(reader),
		Metadata: SessionMetadata{
			Source: SourceKiroCLI,
			ID:     sessionID,
			Size:   0, // SQLite-backed, size unknown
		},
	}, nil
}

func (r *Resolver) resolveKiroIDE(sessionID string) (*ResolvedSession, error) {
	workspaceDir, err := transcript.KiroIDEWorkspaceDir(r.projectPath)
	if err != nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	bestPath, err := r.findKiroIDEPath(workspaceDir, sessionID)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(bestPath)
	if err != nil {
		return nil, err
	}

	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}

	createdAt := kiroIDECreatedAt(f, info.ModTime())
	costPath := transcript.KiroIDEExecutionDetailPath(workspaceDir, sessionID)

	return &ResolvedSession{
		Reader: f,
		Metadata: SessionMetadata{
			Source:    SourceKiroIDE,
			ID:        sessionID,
			Size:      info.Size(),
			CreatedAt: createdAt,
			CostPath:  costPath,
		},
	}, nil
}

// kiroIDECreatedAt extracts the startTime from a Kiro IDE .chat file's metadata,
// matching the behaviour of listKiroIDE. Falls back to modTime if metadata is
// absent or startTime is zero. Seeks the reader back to the start after parsing.
func kiroIDECreatedAt(rs io.ReadSeeker, modTime time.Time) time.Time {
	defer func() { _, _ = rs.Seek(0, io.SeekStart) }()
	var header kiroIDEChatHeader
	if err := json.NewDecoder(rs).Decode(&header); err == nil {
		if header.Metadata != nil && header.Metadata.StartTime > 0 {
			return time.UnixMilli(header.Metadata.StartTime)
		}
	}
	return modTime
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
		path, err := findCodexSession(r.homeDir, sessionID)
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
		workspaceDir, err := transcript.KiroIDEWorkspaceDir(r.projectPath)
		if err != nil {
			return "", fmt.Errorf("session not found: %s", sessionID)
		}
		return r.findKiroIDEPath(workspaceDir, sessionID)
	default:
		return "", fmt.Errorf("unknown source: %s", source)
	}
}

// findKiroIDEPath finds the best .chat file path for a Kiro IDE session.
// Each candidate is validated with IsPathWithinDir inside the scan loop,
// so symlinks pointing outside the workspace are skipped without shadowing
// legitimate files.
func (r *Resolver) findKiroIDEPath(workspaceDir, sessionID string) (string, error) {
	entries, err := os.ReadDir(workspaceDir)
	if err != nil {
		return "", fmt.Errorf("read kiro ide workspace: %w", err)
	}

	var bestPath string
	var bestCount int
	var bestMtime time.Time

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".chat") {
			continue
		}
		path := filepath.Join(workspaceDir, entry.Name())
		if !web.IsPathWithinDir(path, workspaceDir) {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var header kiroIDEChatHeader
		if err := json.Unmarshal(data, &header); err != nil || header.ExecutionID != sessionID {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		count := len(header.Chat)
		mtime := info.ModTime()
		if bestPath == "" || count > bestCount || (count == bestCount && mtime.After(bestMtime)) {
			bestPath = path
			bestCount = count
			bestMtime = mtime
		}
	}

	if bestPath == "" {
		return "", fmt.Errorf("session not found: %s", sessionID)
	}

	if !web.IsPathWithinDir(bestPath, workspaceDir) {
		return "", fmt.Errorf("session not found: %s", sessionID)
	}
	return bestPath, nil
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

	createdAt := r.resolveFileCreatedAt(f, path, source, info.ModTime())

	return &ResolvedSession{
		Reader: f,
		Metadata: SessionMetadata{
			Source:    source,
			ID:        sessionID,
			Size:      info.Size(),
			CreatedAt: createdAt,
		},
	}, nil
}

// resolveFileCreatedAt derives the session start timestamp using the same
// source-specific logic as the lister. Falls back to modTime on failure.
// For Claude, the file is seeked back to the start after parsing.
func (r *Resolver) resolveFileCreatedAt(f *os.File, path, source string, modTime time.Time) time.Time {
	switch source {
	case SourceClaude:
		if ts, err := transcript.ParseFirstTimestamp(f); err == nil {
			_, _ = f.Seek(0, io.SeekStart)
			return ts
		}
		_, _ = f.Seek(0, io.SeekStart)
	case SourceCodex:
		if ts, err := getCodexSessionTimestamp(path); err == nil {
			return ts
		}
	case SourceCopilot:
		wsPath := filepath.Join(filepath.Dir(path), "workspace.yaml")
		if ws, err := parseCopilotWorkspace(wsPath); err == nil && ws != nil && ws.CreatedAt != nil {
			return *ws.CreatedAt
		}
	case SourceKiroCLI, SourceKiroIDE:
		// Kiro CLI uses SQLite (handled by resolveKiroSession, not file-based).
		// Kiro IDE embeds startTime in JSON but requires full parsing; fall
		// through to modTime for now.
	}
	return modTime
}

// findCodexSession searches ~/.codex/sessions/ for a session by UUID or
// filename-based ID. UUID IDs are matched against UUID substrings in filenames.
// Non-UUID IDs are matched against the full basename without the .jsonl extension,
// which is the same format that listCodex returns for files without a UUID.
func findCodexSession(homeDir, sessionID string) (string, error) {
	// Reject IDs containing path separators or traversal sequences.
	if strings.ContainsAny(sessionID, "/\\") || strings.Contains(sessionID, "..") {
		return "", nil
	}

	isUUID := len(sessionID) == 36 && uuidPattern.MatchString(sessionID)
	normalizedID := strings.ToLower(sessionID)
	codexDir := filepath.Join(homeDir, ".codex", "sessions")

	realDir, err := filepath.EvalSymlinks(codexDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	var foundPath string
	err = walkDirFollowSymlinks(realDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}

		filename := filepath.Base(path)

		if isUUID {
			// Match UUID substring within the filename.
			if match := uuidPattern.FindString(filename); strings.ToLower(match) == normalizedID {
				foundPath = path
				return filepath.SkipAll
			}
		} else {
			// Match the full basename (without .jsonl) for non-UUID IDs.
			if strings.ToLower(strings.TrimSuffix(filename, ".jsonl")) == normalizedID {
				foundPath = path
				return filepath.SkipAll
			}
		}
		return nil
	})

	return foundPath, err
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
