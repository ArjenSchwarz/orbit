package sessions

import (
	"bufio"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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

// kiroIDEBasePathFunc abstracts Kiro IDE base path resolution for testing.
type kiroIDEBasePathFunc func() (string, error)

// Lister discovers sessions from all agent types for a project.
type Lister struct {
	homeDir         string
	opts            Options
	kiroDiscover    kiroDiscoverFunc    // nil → logs.DiscoverForDirectory
	kiroIDEBasePath kiroIDEBasePathFunc // nil → transcript.KiroIDEBasePath
}

// NewLister creates a Lister.
// Resolves os.UserHomeDir() once. Returns an error if unavailable.
func NewLister(opts ...Option) (*Lister, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	return &Lister{homeDir: homeDir, opts: applyOptions(opts)}, nil
}

// ListAll returns all sessions for the given project path, or for every
// project when it is empty, sorted by creation time (oldest first).
// Warnings are returned for agent sources that failed to list.
func (l *Lister) ListAll(projectPath string) ([]SessionInfo, []ListWarning, error) {
	var warnings []ListWarning

	// In all-projects scope each collector lists an identifier found in
	// several places once, choosing the copy the resolver opens, and warns
	// about the others. listCodex runs before listCodexExec and supplies the
	// thread IDs of verified rollouts, which suppress matching exec streams.
	var rolloutThreads map[string]bool
	collectors := []struct {
		source string
		fn     func() ([]SessionInfo, []ListWarning, error)
	}{
		{SourceClaude, func() ([]SessionInfo, []ListWarning, error) { return l.listClaude(projectPath) }},
		{SourceCopilot, func() ([]SessionInfo, []ListWarning, error) { return l.listCopilot(projectPath) }},
		{SourceCodex, func() ([]SessionInfo, []ListWarning, error) {
			sessions, warnings, threads, err := l.listCodexIndexed(projectPath)
			rolloutThreads = threads
			return sessions, warnings, err
		}},
		{SourceKiroCLI, func() ([]SessionInfo, []ListWarning, error) {
			sessions, err := l.listKiro(projectPath)
			return sessions, nil, err
		}},
		{SourceKiroIDE, func() ([]SessionInfo, []ListWarning, error) { return l.listKiroIDE(projectPath) }},
		{SourceCodexExec, func() ([]SessionInfo, []ListWarning, error) {
			sessions, warnings := l.listCodexExec(projectPath, rolloutThreads)
			return sessions, warnings, nil
		}},
	}

	var allSessions []SessionInfo

	for _, c := range collectors {
		sessions, sourceWarnings, err := c.fn()
		warnings = append(warnings, sourceWarnings...)
		if err != nil {
			warnings = append(warnings, ListWarning{Source: c.source, Code: WarningSourceFailed, Err: err})
			continue
		}
		allSessions = append(allSessions, sessions...)
	}

	if l.opts.ExtendedOrdering {
		sortSessionsExtended(allSessions)
	} else {
		sortSessionsByTimestamp(allSessions)
	}
	return allSessions, warnings, nil
}

// listClaude returns all Claude sessions for a project.
// When projectPath is empty, returns sessions from all projects.
func (l *Lister) listClaude(projectPath string) ([]SessionInfo, []ListWarning, error) {
	projectsRoot := filepath.Join(l.homeDir, ".claude", "projects")

	if projectPath == "" {
		return l.listClaudeAllProjects(projectsRoot)
	}

	claudeProjectPath := claudecode.BuildProjectPath(projectPath)
	projectDir := filepath.Join(projectsRoot, claudeProjectPath)

	sessions, err := l.listClaudeDir(projectDir)
	return sessions, nil, err
}

// listClaudeAllProjects iterates over all project subdirectories in sorted
// order and collects sessions. An identifier found in several directories is
// listed from the first one, the directory the resolver opens.
func (l *Lister) listClaudeAllProjects(projectsRoot string) ([]SessionInfo, []ListWarning, error) {
	if _, err := os.Stat(projectsRoot); os.IsNotExist(err) {
		return nil, nil, nil
	}

	entries, err := os.ReadDir(projectsRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read projects directory: %w", err)
	}

	var allSessions []SessionInfo
	var warnings []ListWarning
	listed := make(map[string]bool)
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
		for _, s := range sessions {
			if listed[s.ID] {
				warnings = append(warnings, duplicateWarning(SourceClaude, s.ID, filepath.Join(projectDir, s.ID+".jsonl")))
				continue
			}
			listed[s.ID] = true
			allSessions = append(allSessions, s)
		}
	}

	return allSessions, warnings, nil
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

		filePath := filepath.Join(projectDir, entry.Name())

		// Skip symlinks that escape the project directory.
		if entry.Type()&fs.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(filePath)
			if err != nil || !isWithinDir(resolved, projectDir) {
				continue
			}
		}

		sessionID := strings.TrimSuffix(entry.Name(), ".jsonl")

		info, err := entry.Info()
		if err != nil {
			continue
		}

		session := SessionInfo{
			ID:     sessionID,
			Size:   info.Size(),
			Source: SourceClaude,
		}
		f, err := os.Open(filePath)
		if err == nil {
			session.CreatedAt, err = transcript.ParseFirstTimestamp(f)
			if _, seekErr := f.Seek(0, io.SeekStart); seekErr == nil {
				session.Project, session.Origin = readClaudeHead(f)
			}
			_ = f.Close()
		}
		if err != nil {
			session.CreatedAt = info.ModTime()
			session.TimeFromFilesystem = true
		}

		sessions = append(sessions, session)
	}

	return sessions, nil
}

// Sources returns the sources this Lister can list: every source, except
// codex-exec when no exec roots are configured.
func (l *Lister) Sources() []string {
	if len(l.opts.CodexExecRoots) > 0 {
		return AllSources()
	}
	return slices.DeleteFunc(AllSources(), func(s string) bool { return s == SourceCodexExec })
}

// listCodex returns Codex sessions from ~/.codex/sessions/. Project scope
// keeps every rollout whose recorded cwd is the project. All-projects scope
// lists each identifier once, from the rollout selectCodexCandidate opens,
// which is the first match in the same walk. That rollout is listed only when
// its head is readable and records the identifier; otherwise the identifier is
// omitted with a warning rather than promoting a later copy (Decision 22).
func (l *Lister) listCodex(projectPath string) ([]SessionInfo, []ListWarning, error) {
	sessions, warnings, _, err := l.listCodexIndexed(projectPath)
	return sessions, warnings, err
}

// listCodexIndexed is listCodex that also returns the lower-case recorded IDs
// of the rollouts selectCodexCandidate would open whose heads pass
// codexHeadWarning, in every scope. Exec streams with those thread IDs are
// suppressed (Decision 22). A failed walk returns no index, so it never
// suppresses a stream.
func (l *Lister) listCodexIndexed(projectPath string) ([]SessionInfo, []ListWarning, map[string]bool, error) {
	var sessions []SessionInfo
	var warnings []ListWarning
	selected := make(map[string]bool) // lower-case identifiers already seen in walk order
	threads := make(map[string]bool)
	err := walkCodexRollouts(l.homeDir, func(path string, d fs.DirEntry) error {
		id := codexSessionID(filepath.Base(path))
		key := strings.ToLower(id)
		winner := !selected[key]
		selected[key] = true
		if projectPath == "" && !winner {
			warnings = append(warnings, duplicateWarning(SourceCodex, id, path))
			return nil
		}

		info, err := d.Info()
		if err != nil || info.Size() == 0 {
			return nil
		}

		head, headErr := readCodexHead(path)
		var headWarning *ListWarning
		if winner {
			headWarning = codexHeadWarning(id, path, head, headErr)
			if headWarning == nil && head.ID != "" {
				threads[strings.ToLower(head.ID)] = true
			}
		}
		if projectPath != "" {
			if head.Cwd == "" || normalizePath(head.Cwd) != normalizePath(projectPath) {
				return nil
			}
		} else if headWarning != nil {
			warnings = append(warnings, *headWarning)
			return nil
		}

		session := SessionInfo{
			ID:        id,
			CreatedAt: head.Timestamp,
			Size:      info.Size(),
			Source:    SourceCodex,
			Project:   head.Cwd,
			Model:     head.Model,
			Provider:  head.Provider,
			Origin:    head.Origin,
		}
		if session.CreatedAt.IsZero() {
			session.CreatedAt = info.ModTime()
			session.TimeFromFilesystem = true
		}
		sessions = append(sessions, session)
		return nil
	})
	if err != nil {
		return nil, warnings, nil, err
	}
	return sessions, warnings, threads, nil
}

// codexHeadWarning reports why a selected rollout cannot be listed across
// projects, or returns nil when its head is readable and records a
// session_meta whose id matches the filename UUID (ignoring case).
func codexHeadWarning(id, path string, head codexHead, err error) *ListWarning {
	invalid := func(format string, args ...any) *ListWarning {
		return &ListWarning{Source: SourceCodex, Code: transcript.WarningInvalidRecord,
			Err: fmt.Errorf("session %s at %s: "+format, append([]any{id, path}, args...)...)}
	}
	var pathErr *fs.PathError
	switch {
	case !head.HasMeta && errors.As(err, &pathErr):
		return &ListWarning{Source: SourceCodex, Code: WarningPathUnreadable, Err: fmt.Errorf("session %s: %w", id, err)}
	case !head.HasMeta && err != nil:
		return invalid("%v", err)
	case !head.HasMeta:
		return invalid("no session_meta within the first %d lines", codexMetaScanLimit)
	case isCodexUUID(id) && !strings.EqualFold(head.ID, id):
		return invalid("session_meta records id %q", head.ID)
	}
	return nil
}

// walkCodexRollouts calls fn for each .jsonl file under ~/.codex/sessions in
// walk order. Listing and selectCodexCandidate share it, so the first rollout
// the listing sees for an identifier is the one the resolver opens.
func walkCodexRollouts(homeDir string, fn func(path string, d fs.DirEntry) error) error {
	realDir, err := filepath.EvalSymlinks(filepath.Join(homeDir, ".codex", "sessions"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return walkDirFollowSymlinks(realDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		return fn(path, d)
	})
}

// duplicateWarning reports a copy of a session that all-projects scope does not list.
func duplicateWarning(source, id, path string) ListWarning {
	return ListWarning{Source: source, Code: WarningDuplicateSession,
		Err: fmt.Errorf("another copy of session %s at %s is not listed", id, path)}
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
				Code:   WarningPathUnreadable,
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
			if matchPath == "" || normalizePath(matchPath) != normalizePath(projectPath) {
				continue
			}
		}

		session := SessionInfo{
			ID:     entry.Name(),
			Size:   eventsInfo.Size(),
			Source: SourceCopilot,
		}
		if ws != nil {
			session.Project = cmp.Or(ws.GitRoot, ws.Cwd)
		}
		if ws != nil && ws.CreatedAt != nil {
			session.CreatedAt = *ws.CreatedAt
		} else {
			session.CreatedAt = eventsInfo.ModTime()
			session.TimeFromFilesystem = true
		}

		sessions = append(sessions, session)
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
			Project:   s.Directory,
		}
	}

	return result, nil
}

// listKiroIDE returns all Kiro IDE sessions for a project directory.
// When projectPath is empty, scans all workspace directories.
func (l *Lister) listKiroIDE(projectPath string) ([]SessionInfo, []ListWarning, error) {
	if projectPath == "" {
		return l.listKiroIDEAll()
	}

	workspaceDir, err := transcript.KiroIDEWorkspaceDir(projectPath)
	if err != nil {
		if errors.Is(err, transcript.ErrKiroIDENotFound) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("kiro ide workspace: %w", err)
	}

	sessions, err := l.listKiroIDEWorkspace(workspaceDir)
	return sessions, nil, err
}

// listKiroIDEWorkspace scans a single workspace directory for Kiro IDE sessions.
func (l *Lister) listKiroIDEWorkspace(workspaceDir string) ([]SessionInfo, error) {
	best, err := kiroIDEWorkspaceCandidates(workspaceDir)
	if err != nil {
		return nil, err
	}

	var sessions []SessionInfo
	for _, id := range slices.Sorted(maps.Keys(best)) {
		sessions = append(sessions, kiroIDESessionInfo(id, best[id]))
	}
	return sessions, nil
}

// listKiroIDEAll scans all workspace hash directories under the Kiro IDE base
// path. An execution saved in several workspaces is listed once, from the
// candidate the resolver opens.
func (l *Lister) listKiroIDEAll() ([]SessionInfo, []ListWarning, error) {
	baseFn := l.kiroIDEBasePath
	if baseFn == nil {
		baseFn = transcript.KiroIDEBasePath
	}
	base, err := baseFn()
	if err != nil {
		if errors.Is(err, transcript.ErrKiroIDENotFound) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("kiro ide base path: %w", err)
	}

	all, err := kiroIDEAllCandidates(base)
	if err != nil {
		return nil, nil, err
	}

	var sessions []SessionInfo
	var warnings []ListWarning
	for _, id := range slices.Sorted(maps.Keys(all)) {
		best := bestKiroIDECandidate(all[id])
		sessions = append(sessions, kiroIDESessionInfo(id, best))
		for _, c := range all[id] {
			if c.path != best.path {
				warnings = append(warnings, duplicateWarning(SourceKiroIDE, id, c.path))
			}
		}
	}
	return sessions, warnings, nil
}

// --- Supporting functions and types ---

// uuidPattern matches standard UUID format: 8-4-4-4-12 hex digits (case-insensitive)
var uuidPattern = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

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

// codexHead is what the bounded scan of a Codex rollout's first lines records.
type codexHead struct {
	ID        string    // session_meta payload id
	Cwd       string    // session_meta payload cwd
	Provider  string    // session_meta payload model_provider
	Origin    string    // session_meta payload originator
	Model     string    // first turn_context model, else the session_meta model
	HasMeta   bool      // a session_meta record with a decodable payload was found
	Timestamp time.Time // first session_meta timestamp that parses as RFC3339; zero when none
}

// readCodexHead scans up to codexMetaScanLimit lines of 10 MiB at most. The
// first session_meta whose payload decodes supplies id, cwd, provider,
// originator and the fallback model; the first session_meta whose timestamp
// parses supplies the timestamp; the first turn_context supplies the model
// (Decision 29). It stops once all of them are found. The error reports a file
// that cannot be opened or a failed scan; values found before a failure are kept.
func readCodexHead(path string) (codexHead, error) {
	f, err := os.Open(path)
	if err != nil {
		return codexHead{}, err
	}
	defer func() { _ = f.Close() }()

	var head codexHead
	var metaModel, turnModel string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for i := 0; i < codexMetaScanLimit && scanner.Scan(); i++ {
		var line struct {
			Type      string          `json:"type"`
			Timestamp json.RawMessage `json:"timestamp"`
			Payload   json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}
		switch line.Type {
		case "session_meta":
			var ts string
			if head.Timestamp.IsZero() && json.Unmarshal(line.Timestamp, &ts) == nil && ts != "" {
				if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
					head.Timestamp = parsed
				}
			}
			var meta struct {
				ID            string `json:"id"`
				Cwd           string `json:"cwd"`
				ModelProvider string `json:"model_provider"`
				Originator    string `json:"originator"`
				Model         string `json:"model"`
			}
			if !head.HasMeta && len(line.Payload) > 0 && json.Unmarshal(line.Payload, &meta) == nil {
				head.HasMeta = true
				head.ID, head.Cwd, head.Provider, head.Origin = meta.ID, meta.Cwd, meta.ModelProvider, meta.Originator
				metaModel = meta.Model
			}
		case "turn_context":
			var turn struct {
				Model string `json:"model"`
			}
			if turnModel == "" && len(line.Payload) > 0 && json.Unmarshal(line.Payload, &turn) == nil {
				turnModel = turn.Model
			}
		}
		if head.HasMeta && !head.Timestamp.IsZero() && turnModel != "" {
			break
		}
	}
	head.Model = cmp.Or(turnModel, metaModel)
	if err := scanner.Err(); err != nil {
		return head, fmt.Errorf("scanning %s: %w", filepath.Base(path), err)
	}
	return head, nil
}

// codexSessionID returns the listing identifier of a rollout file: the UUID in
// its name, else its name without the .jsonl extension.
func codexSessionID(filename string) string {
	if match := uuidPattern.FindString(filename); match != "" {
		return match
	}
	return strings.TrimSuffix(filename, ".jsonl")
}

// Bounds of the optional Claude cwd and entrypoint scan (Q57, Q90). The
// timestamp keeps transcript.ParseFirstTimestamp's unbounded scan.
const (
	claudeHeadLines = 50
	claudeHeadBytes = 1 << 20
)

// readClaudeHead returns the first nonempty cwd and entrypoint within the
// first claudeHeadLines lines and claudeHeadBytes bytes of a Claude transcript.
func readClaudeHead(r io.Reader) (cwd, entrypoint string) {
	scanner := bufio.NewScanner(io.LimitReader(r, claudeHeadBytes))
	scanner.Buffer(make([]byte, 0, 64*1024), claudeHeadBytes+1)
	for i := 0; i < claudeHeadLines && scanner.Scan(); i++ {
		var line struct {
			Cwd        string `json:"cwd"`
			Entrypoint string `json:"entrypoint"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}
		cwd = cmp.Or(cwd, line.Cwd)
		entrypoint = cmp.Or(entrypoint, line.Entrypoint)
		if cwd != "" && entrypoint != "" {
			break
		}
	}
	return cwd, entrypoint
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
// Symlinks that resolve outside the root directory are skipped.
// Unresolvable symlinks (broken, permission errors) are silently skipped rather than
// propagated to fn, since callers treat missing sessions as non-fatal.
func walkDirFollowSymlinks(root string, fn fs.WalkDirFunc) error {
	visited := make(map[string]bool)
	boundary, err := filepath.EvalSymlinks(root)
	if err != nil {
		boundary = root
	}
	if abs, err := filepath.Abs(boundary); err == nil {
		boundary = abs
	}
	return walkDirFollowSymlinksInternal(root, boundary, fn, visited)
}

func walkDirFollowSymlinksInternal(root, boundary string, fn fs.WalkDirFunc, visited map[string]bool) error {
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
				return nil // skip unresolvable symlinks
			}

			if !isWithinDir(realPath, boundary) {
				return nil // skip symlinks escaping the boundary
			}

			info, err := os.Stat(realPath)
			if err != nil {
				return nil
			}

			if info.IsDir() {
				absReal, _ := filepath.Abs(realPath)
				if visited[absReal] {
					return nil
				}
				return walkDirFollowSymlinksInternal(realPath, boundary, fn, visited)
			}

			return fn(realPath, fs.FileInfoToDirEntry(info), nil)
		}

		return fn(path, d, err)
	})
}

// isWithinDir reports whether path is within dir.
// Both path and dir are resolved via EvalSymlinks before comparison so that
// symlinked prefixes (e.g. /tmp → /private/tmp on macOS) do not cause
// false negatives.
func isWithinDir(path, dir string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		resolvedDir = dir
	}
	absDir, err := filepath.Abs(resolvedDir)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absDir, absPath)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// sourcePriority orders sources whose sessions have equal times.
var sourcePriority = map[string]int{
	SourceClaude:    0,
	SourceCopilot:   1,
	SourceCodex:     2,
	SourceKiroCLI:   3,
	SourceKiroIDE:   4,
	SourceCodexExec: 5,
}

// sortSessionsByTimestamp sorts sessions by creation time (oldest first).
// When timestamps are equal, sources are ordered by priority. This is the
// legacy order of listings without extended ordering.
func sortSessionsByTimestamp(sessions []SessionInfo) {
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].CreatedAt.Equal(sessions[j].CreatedAt) {
			return sourcePriority[sessions[i].Source] < sourcePriority[sessions[j].Source]
		}
		return sessions[i].CreatedAt.Before(sessions[j].CreatedAt)
	})
}

// sortSessionsExtended sorts sessions by compareSessionsExtended (requirement 4.4).
func sortSessionsExtended(sessions []SessionInfo) {
	slices.SortStableFunc(sessions, compareSessionsExtended)
}

// compareSessionsExtended orders unknown times first, then times ascending,
// then source priority, then identifier, so the last session is the latest.
func compareSessionsExtended(a, b SessionInfo) int {
	if a.CreatedAt.IsZero() != b.CreatedAt.IsZero() {
		if a.CreatedAt.IsZero() {
			return -1
		}
		return 1
	}
	return cmp.Or(
		a.CreatedAt.Compare(b.CreatedAt),
		cmp.Compare(sourcePriority[a.Source], sourcePriority[b.Source]),
		strings.Compare(a.Source, b.Source),
		strings.Compare(a.ID, b.ID),
	)
}
