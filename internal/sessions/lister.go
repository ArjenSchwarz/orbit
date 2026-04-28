package sessions

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents/claudecode"
	"github.com/arjenschwarz/orbit/internal/agents/kiro/logs"
	"github.com/arjenschwarz/orbit/internal/transcript"
	"gopkg.in/yaml.v3"
)

// kiroDiscoverFunc abstracts Kiro session discovery for testing.
type kiroDiscoverFunc func(ctx context.Context, dir string) ([]logs.SessionMetadata, error)

// Lister discovers sessions from all agent types for a project.
type Lister struct {
	homeDir      string
	kiroDiscover kiroDiscoverFunc // nil → logs.DiscoverForDirectory
}

// NewLister creates a Lister.
// Resolves os.UserHomeDir() once. Returns an error if unavailable.
func NewLister() (*Lister, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	return &Lister{homeDir: homeDir}, nil
}

// ListAll returns all sessions for the given project path,
// sorted by creation time (oldest first).
// Warnings are returned for agent sources that failed to list.
func (l *Lister) ListAll(projectPath string) ([]SessionInfo, []ListWarning, error) {
	var warnings []ListWarning

	collectors := []struct {
		source string
		fn     func() ([]SessionInfo, error)
	}{
		{SourceClaude, func() ([]SessionInfo, error) { return l.listClaude(projectPath) }},
		{SourceCopilot, func() ([]SessionInfo, error) {
			sessions, parseWarnings, err := l.listCopilot(projectPath)
			warnings = append(warnings, parseWarnings...)
			return sessions, err
		}},
		{SourceCodex, func() ([]SessionInfo, error) { return l.listCodex(projectPath) }},
		{SourceKiroCLI, func() ([]SessionInfo, error) { return l.listKiro(projectPath) }},
		{SourceKiroIDE, func() ([]SessionInfo, error) { return l.listKiroIDE(projectPath) }},
	}

	var allSessions []SessionInfo

	for _, c := range collectors {
		sessions, err := c.fn()
		if err != nil {
			warnings = append(warnings, ListWarning{Source: c.source, Err: err})
			continue
		}
		allSessions = append(allSessions, sessions...)
	}

	sortSessionsByTimestamp(allSessions)
	return allSessions, warnings, nil
}

// listClaude returns all Claude sessions for a project.
// When projectPath is empty, returns sessions from all projects.
func (l *Lister) listClaude(projectPath string) ([]SessionInfo, error) {
	projectsRoot := filepath.Join(l.homeDir, ".claude", "projects")

	if projectPath == "" {
		return l.listClaudeAllProjects(projectsRoot)
	}

	claudeProjectPath := claudecode.BuildProjectPath(projectPath)
	projectDir := filepath.Join(projectsRoot, claudeProjectPath)

	return l.listClaudeDir(projectDir)
}

// listClaudeAllProjects iterates over all project subdirectories and collects sessions.
func (l *Lister) listClaudeAllProjects(projectsRoot string) ([]SessionInfo, error) {
	if _, err := os.Stat(projectsRoot); os.IsNotExist(err) {
		return nil, nil
	}

	entries, err := os.ReadDir(projectsRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to read projects directory: %w", err)
	}

	var allSessions []SessionInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projectDir := filepath.Join(projectsRoot, entry.Name())
		sessions, err := l.listClaudeDir(projectDir)
		if err != nil {
			// Individual project directory failures (e.g. permission denied) are
			// intentionally skipped so that sessions from other projects are still
			// returned. This parallels how ListAll collects per-source warnings
			// rather than stopping on the first source failure.
			continue
		}
		allSessions = append(allSessions, sessions...)
	}

	return allSessions, nil
}

// listClaudeDir returns all Claude sessions from a single project directory.
func (l *Lister) listClaudeDir(projectDir string) ([]SessionInfo, error) {
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		return nil, nil
	}

	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read project directory: %w", err)
	}

	var sessions []SessionInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}

		sessionID := strings.TrimSuffix(entry.Name(), ".jsonl")
		filePath := filepath.Join(projectDir, entry.Name())

		info, err := entry.Info()
		if err != nil {
			continue
		}

		var createdAt time.Time
		f, err := os.Open(filePath)
		if err == nil {
			createdAt, err = transcript.ParseFirstTimestamp(f)
			_ = f.Close()
			if err != nil {
				createdAt = info.ModTime()
			}
		} else {
			createdAt = info.ModTime()
		}

		sessions = append(sessions, SessionInfo{
			ID:        sessionID,
			CreatedAt: createdAt,
			Size:      info.Size(),
			Source:    SourceClaude,
		})
	}

	return sessions, nil
}

// listCodex returns Codex sessions from ~/.codex/sessions/ filtered by project path.
func (l *Lister) listCodex(projectPath string) ([]SessionInfo, error) {
	codexDir := filepath.Join(l.homeDir, ".codex", "sessions")

	realDir, err := filepath.EvalSymlinks(codexDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var sessions []SessionInfo
	err = walkDirFollowSymlinks(realDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		if info.Size() == 0 {
			return nil
		}

		if projectPath != "" {
			cwd := getCodexSessionCwd(path)
			if cwd == "" {
				return nil
			}
			if normalizePath(cwd) != normalizePath(projectPath) {
				return nil
			}
		}

		filename := filepath.Base(path)
		sessionID := strings.TrimSuffix(filename, ".jsonl")
		if match := uuidPattern.FindString(filename); match != "" {
			sessionID = match
		}

		createdAt, err := getCodexSessionTimestamp(path)
		if err != nil {
			createdAt = info.ModTime()
		}

		sessions = append(sessions, SessionInfo{
			ID:        sessionID,
			CreatedAt: createdAt,
			Size:      info.Size(),
			Source:    SourceCodex,
		})

		return nil
	})

	if err != nil {
		return nil, err
	}

	return sessions, nil
}

// listCopilot returns all Copilot sessions for a project directory. When
// projectPath is empty, sessions with a missing or unparseable workspace.yaml
// are still returned, since workspace metadata is only needed for filtering.
// YAML parse failures are surfaced as warnings rather than silently dropping
// the session.
func (l *Lister) listCopilot(projectPath string) ([]SessionInfo, []ListWarning, error) {
	sessionDir := filepath.Join(l.homeDir, ".copilot", "session-state")

	if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
		return nil, nil, nil
	}

	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read session directory: %w", err)
	}

	var sessions []SessionInfo
	var warnings []ListWarning
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		sessionPath := filepath.Join(sessionDir, entry.Name())
		workspacePath := filepath.Join(sessionPath, "workspace.yaml")
		eventsPath := filepath.Join(sessionPath, "events.jsonl")

		eventsInfo, err := os.Stat(eventsPath)
		if err != nil {
			continue
		}

		if eventsInfo.Size() == 0 {
			continue
		}

		ws, err := parseCopilotWorkspace(workspacePath)
		if err != nil {
			warnings = append(warnings, ListWarning{
				Source: SourceCopilot,
				Err:    fmt.Errorf("session %s: %w", entry.Name(), err),
			})
			ws = nil
		}

		// Workspace metadata is only required when filtering by project.
		// Without metadata we cannot match a project filter, so skip in
		// that case; otherwise include the session.
		if projectPath != "" {
			if ws == nil {
				continue
			}
			matchPath := ws.GitRoot
			if matchPath == "" {
				matchPath = ws.Cwd
			}
			if matchPath != "" && normalizePath(matchPath) != normalizePath(projectPath) {
				continue
			}
		}

		var createdAt time.Time
		if ws != nil && ws.CreatedAt != nil {
			createdAt = *ws.CreatedAt
		} else {
			createdAt = eventsInfo.ModTime()
		}

		sessions = append(sessions, SessionInfo{
			ID:        entry.Name(),
			CreatedAt: createdAt,
			Size:      eventsInfo.Size(),
			Source:    SourceCopilot,
		})
	}

	return sessions, warnings, nil
}

// listKiro returns all Kiro CLI sessions for a project directory.
// When cwd is empty, returns sessions from all directories.
func (l *Lister) listKiro(cwd string) ([]SessionInfo, error) {
	discover := l.kiroDiscover
	if discover == nil {
		discover = logs.DiscoverForDirectory
	}

	var kiroSessions []logs.SessionMetadata
	var err error

	if cwd == "" {
		kiroSessions, err = logs.DiscoverAll(context.Background())
	} else {
		kiroSessions, err = discover(context.Background(), cwd)
	}

	if err != nil {
		if errors.Is(err, logs.ErrDatabaseNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("discover kiro sessions: %w", err)
	}

	result := make([]SessionInfo, len(kiroSessions))
	for i, s := range kiroSessions {
		result[i] = SessionInfo{
			ID:        s.ConversationID,
			CreatedAt: s.CreatedAt,
			Size:      s.Size,
			Source:    SourceKiroCLI,
		}
	}

	return result, nil
}

// listKiroIDE returns all Kiro IDE sessions for a project directory.
func (l *Lister) listKiroIDE(projectPath string) ([]SessionInfo, error) {
	workspaceDir, err := transcript.KiroIDEWorkspaceDir(projectPath)
	if err != nil {
		if errors.Is(err, transcript.ErrKiroIDENotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("kiro ide workspace: %w", err)
	}

	entries, err := os.ReadDir(workspaceDir)
	if err != nil {
		return nil, fmt.Errorf("read kiro ide workspace: %w", err)
	}

	type chatCandidate struct {
		path       string
		entryCount int
		mtime      time.Time
		startTime  int64
		size       int64
	}
	best := make(map[string]*chatCandidate)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".chat") {
			continue
		}

		path := filepath.Join(workspaceDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var header kiroIDEChatHeader
		if err := json.Unmarshal(data, &header); err != nil {
			continue
		}

		if header.ExecutionID == "" {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		var startTime int64
		if header.Metadata != nil {
			startTime = header.Metadata.StartTime
		}

		candidate := &chatCandidate{
			path:       path,
			entryCount: len(header.Chat),
			mtime:      info.ModTime(),
			startTime:  startTime,
			size:       info.Size(),
		}

		existing, ok := best[header.ExecutionID]
		if !ok {
			best[header.ExecutionID] = candidate
			continue
		}

		if candidate.entryCount > existing.entryCount {
			best[header.ExecutionID] = candidate
		} else if candidate.entryCount == existing.entryCount {
			if candidate.mtime.After(existing.mtime) {
				best[header.ExecutionID] = candidate
			} else if candidate.mtime.Equal(existing.mtime) && candidate.path < existing.path {
				best[header.ExecutionID] = candidate
			}
		}
	}

	var sessions []SessionInfo
	for execID, c := range best {
		var createdAt time.Time
		if c.startTime > 0 {
			createdAt = time.UnixMilli(c.startTime)
		} else {
			createdAt = c.mtime
		}
		sessions = append(sessions, SessionInfo{
			ID:        execID,
			CreatedAt: createdAt,
			Size:      c.size,
			Source:    SourceKiroIDE,
		})
	}

	return sessions, nil
}

// --- Supporting functions and types ---

// uuidPattern matches standard UUID format: 8-4-4-4-12 hex digits (case-insensitive)
var uuidPattern = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// kiroIDEChatHeader is a lightweight struct for parsing .chat files during discovery.
type kiroIDEChatHeader struct {
	ExecutionID string            `json:"executionId"`
	Chat        []json.RawMessage `json:"chat"`
	Metadata    *kiroIDEMetadata  `json:"metadata"`
}

type kiroIDEMetadata struct {
	StartTime int64 `json:"startTime"`
}

// copilotWorkspace represents the metadata from a Copilot workspace.yaml file.
type copilotWorkspace struct {
	ID        string     `yaml:"id"`
	Cwd       string     `yaml:"cwd"`
	GitRoot   string     `yaml:"git_root"`
	CreatedAt *time.Time `yaml:"created_at"`
	Summary   string     `yaml:"summary"`
}

// parseCopilotWorkspace parses a Copilot workspace.yaml file.
// Returns (nil, nil) when the file does not exist (callers treat the
// session as having no workspace metadata). Returns a non-nil error when
// the file exists but cannot be read or parsed, so callers can surface a
// warning instead of silently dropping the session.
func parseCopilotWorkspace(path string) (*copilotWorkspace, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var ws copilotWorkspace
	if err := yaml.Unmarshal(data, &ws); err != nil {
		return nil, fmt.Errorf("parse workspace.yaml: %w", err)
	}

	return &ws, nil
}

// codexMetaScanLimit is the maximum number of JSONL lines to scan when
// searching for the session_meta entry. The entry is typically first but
// is not guaranteed to be.
const codexMetaScanLimit = 50

// getCodexSessionTimestamp extracts the timestamp from a Codex session file.
// It scans up to codexMetaScanLimit lines for the session_meta entry.
func getCodexSessionTimestamp(path string) (time.Time, error) {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for i := 0; i < codexMetaScanLimit && scanner.Scan(); i++ {
		var entry struct {
			Timestamp string `json:"timestamp"`
			Type      string `json:"type"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err == nil {
			if entry.Type == "session_meta" && entry.Timestamp != "" {
				if ts, err := time.Parse(time.RFC3339, entry.Timestamp); err == nil {
					return ts, nil
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return time.Time{}, fmt.Errorf("scanning %s: %w", filepath.Base(path), err)
	}

	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

// getCodexSessionCwd extracts the working directory from a Codex session file.
// It scans up to codexMetaScanLimit lines for the session_meta entry.
func getCodexSessionCwd(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for i := 0; i < codexMetaScanLimit && scanner.Scan(); i++ {
		var entry struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err == nil {
			if entry.Type == "session_meta" && len(entry.Payload) > 0 {
				var meta struct {
					Cwd string `json:"cwd"`
				}
				if err := json.Unmarshal(entry.Payload, &meta); err == nil {
					return meta.Cwd
				}
			}
		}
	}
	// scanner.Err() intentionally not checked: a scan error (e.g. line
	// exceeding buffer) is indistinguishable from no session_meta found,
	// and both result in the same empty-string fallback.

	return ""
}

// normalizePath resolves symlinks and cleans a file path for reliable comparison.
func normalizePath(p string) string {
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return filepath.Clean(p)
	}
	return filepath.Clean(resolved)
}

// walkDirFollowSymlinks walks a directory tree, following symlinks with cycle detection.
func walkDirFollowSymlinks(root string, fn fs.WalkDirFunc) error {
	visited := make(map[string]bool)
	return walkDirFollowSymlinksInternal(root, fn, visited)
}

func walkDirFollowSymlinksInternal(root string, fn fs.WalkDirFunc, visited map[string]bool) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	if visited[absRoot] {
		return nil
	}
	visited[absRoot] = true

	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fn(path, d, err)
		}

		if d.Type()&fs.ModeSymlink != 0 {
			realPath, err := filepath.EvalSymlinks(path)
			if err != nil {
				return fn(path, d, err)
			}

			info, err := os.Stat(realPath)
			if err != nil {
				return fn(path, d, err)
			}

			if info.IsDir() {
				absReal, _ := filepath.Abs(realPath)
				if visited[absReal] {
					return nil
				}
				return walkDirFollowSymlinksInternal(realPath, fn, visited)
			}

			return fn(realPath, fs.FileInfoToDirEntry(info), nil)
		}

		return fn(path, d, err)
	})
}

// sortSessionsByTimestamp sorts sessions by creation time (oldest first).
// When timestamps are equal, sources are ordered by priority.
func sortSessionsByTimestamp(sessions []SessionInfo) {
	sourcePriority := map[string]int{
		SourceClaude:  0,
		SourceCopilot: 1,
		SourceCodex:   2,
		SourceKiroCLI: 3,
		SourceKiroIDE: 4,
	}

	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].CreatedAt.Equal(sessions[j].CreatedAt) {
			return sourcePriority[sessions[i].Source] < sourcePriority[sessions[j].Source]
		}
		return sessions[i].CreatedAt.Before(sessions[j].CreatedAt)
	})
}
