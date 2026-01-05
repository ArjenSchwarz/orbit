# Web Interface Design

## Overview

This document describes the technical design for adding a web-based interface to Orbit for monitoring runs and browsing logs remotely. The implementation adds three main capabilities:

1. **Run Registry** (`internal/registry/`) - A system for tracking known runs in `~/.orbit/runs/`
2. **Web Server** (`internal/web/`) - An HTTP server that serves the web interface
3. **CLI Commands** - `orbit serve` and `orbit register` subcommands

The design prioritizes simplicity, reusing existing code (especially `internal/transcript/html.go`), and maintaining the single-binary deployment model by embedding all static assets.

## Architecture

### Package Structure

```
cmd/orbit/
  ├── main.go           # Refactored to use subcommands
  ├── run.go            # NEW: orbit run command (extracted from current main.go)
  ├── serve.go          # NEW: orbit serve command
  └── register.go       # NEW: orbit register command

internal/
  ├── registry/
  │   ├── registry.go   # NEW: Run registration and discovery
  │   ├── git.go        # NEW: Git URL parsing for repository field
  │   └── types.go      # NEW: Registry entry types
  │
  ├── web/
  │   ├── server.go     # NEW: HTTP server setup and lifecycle
  │   ├── handlers.go   # NEW: Page handlers (dashboard, detail, transcript)
  │   ├── middleware.go # NEW: Security headers, path validation
  │   └── static/       # NEW: Embedded static assets
  │       ├── htmx.min.js
  │       └── style.css
  │
  ├── transcript/
  │   └── html.go       # MODIFIED: Add navigation context support
  │
  ├── config/
  │   └── config.go     # MODIFIED: Add serve-port and serve-bind
  │
  └── orbit/
      └── orbit.go      # MODIFIED: Add registry integration
```

### CLI Subcommand Refactoring

The current `cmd/orbit/main.go` implements the orchestration logic directly. This design introduces subcommands, requiring a refactor:

**Current structure (no subcommands):**
```
orbit --tasks-file specs/foo/tasks.md
```

**New structure (with subcommands):**
```
orbit run --tasks-file specs/foo/tasks.md
orbit serve --port 8080
orbit register ./specs/foo/.orbit
```

**Implementation approach:**

1. **main.go** becomes a command router:
   - Checks first argument for subcommand (`run`, `serve`, `register`)
   - Falls back to `run` when first argument is a flag or missing (backward compatibility)
   - Routes to appropriate handler

2. **run.go** receives extracted orchestration code:
   - All current `main.go` logic moves here
   - Function signature: `func runCommand(args []string) error`

3. **Backward compatibility preserved:**
   - `orbit --tasks-file foo` continues to work (implicit `run`)
   - `orbit run --tasks-file foo` is the explicit form

```go
// main.go (simplified)
func main() {
    if len(os.Args) < 2 || strings.HasPrefix(os.Args[1], "-") {
        // No subcommand or starts with flag: implicit "run"
        runCommand(os.Args[1:])
        return
    }

    switch os.Args[1] {
    case "run":
        runCommand(os.Args[2:])
    case "serve":
        serveCommand(os.Args[2:])
    case "register":
        registerCommand(os.Args[2:])
    default:
        // Unknown subcommand, treat as implicit "run"
        runCommand(os.Args[1:])
    }
}
```

### Component Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                         CLI Layer                               │
│  ┌──────────────┐  ┌───────────────┐  ┌──────────────────────┐  │
│  │ orbit run    │  │ orbit serve   │  │ orbit register       │  │
│  │ (modified)   │  │ (new)         │  │ (new)                │  │
│  └──────┬───────┘  └───────┬───────┘  └──────────┬───────────┘  │
└─────────┼──────────────────┼─────────────────────┼──────────────┘
          │                  │                     │
          ▼                  ▼                     ▼
┌─────────────────┐  ┌───────────────┐  ┌──────────────────────┐
│   Registry      │◀─┤  Web Server   │  │   Registry           │
│   (write)       │  │  (read)       │  │   (write)            │
└────────┬────────┘  └───────┬───────┘  └──────────┬───────────┘
         │                   │                     │
         ▼                   ▼                     ▼
┌─────────────────────────────────────────────────────────────────┐
│                    ~/.orbit/runs/*.json                         │
│                    (Registry Storage)                           │
└─────────────────────────────────────────────────────────────────┘
```

### Request Flow

```
Browser Request
       │
       ▼
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│  Middleware  │────▶│   Router     │────▶│   Handler    │
│  - Security  │     │  - Path      │     │  - Dashboard │
│  - Headers   │     │    matching  │     │  - Detail    │
│  - Validate  │     │              │     │  - Transcript│
└──────────────┘     └──────────────┘     └──────┬───────┘
                                                  │
       ┌──────────────────────────────────────────┤
       │                                          │
       ▼                                          ▼
┌──────────────┐                          ┌──────────────┐
│   Registry   │                          │  Log Files   │
│   (list/get) │                          │  (read)      │
└──────────────┘                          └──────────────┘
       │                                          │
       └──────────────────┬───────────────────────┘
                          │
                          ▼
                   ┌──────────────┐
                   │   Template   │
                   │   Render     │
                   └──────────────┘
                          │
                          ▼
                   HTML Response
```

## Components and Interfaces

### Registry Package (`internal/registry/`)

#### Types

```go
// RunStatus represents the state of a run.
type RunStatus string

const (
    StatusRunning   RunStatus = "running"
    StatusCompleted RunStatus = "completed"
    StatusFailed    RunStatus = "failed"
)

// PhaseStatus represents the state of a phase within a run.
type PhaseStatus string

const (
    PhaseStatusPending   PhaseStatus = "pending"
    PhaseStatusRunning   PhaseStatus = "running"
    PhaseStatusCompleted PhaseStatus = "completed"
    PhaseStatusFailed    PhaseStatus = "failed"
)

// Phase represents a single phase within a run.
type Phase struct {
    Number   int         `json:"number"`
    Status   PhaseStatus `json:"status"`
    RunCount int         `json:"run_count"`
}

// RunEntry represents a registered run in the registry.
type RunEntry struct {
    ID            string     `json:"id"`              // UUID v4
    SchemaVersion int        `json:"schema_version"`  // Always 1 for v1
    Name          string     `json:"name"`            // Display name
    Repository    string     `json:"repository"`      // owner/repo format
    LogDir        string     `json:"log_dir"`         // Absolute path
    Status        RunStatus  `json:"status"`
    StartedAt     time.Time  `json:"started_at"`
    FinishedAt    *time.Time `json:"finished_at,omitempty"`
    Branch        string     `json:"branch"`
    PID           *int       `json:"pid,omitempty"`   // nil for manual registrations
    Phases        []Phase    `json:"phases,omitempty"`
}
```

#### Registry Interface

```go
// Registry manages run entries in ~/.orbit/runs/.
type Registry struct {
    dir string // ~/.orbit/runs/
}

// New creates a new Registry instance.
// Creates the registry directory if it doesn't exist.
func New() (*Registry, error)

// Register creates or updates a run entry.
// Uses atomic write (temp file + rename).
func (r *Registry) Register(entry *RunEntry) error

// Get retrieves a run entry by ID.
// Returns nil, nil if not found.
func (r *Registry) Get(id string) (*RunEntry, error)

// List returns all run entries.
// Skips malformed JSON files with a logged warning.
func (r *Registry) List() ([]*RunEntry, error)

// FindByLogDir finds an entry by its log directory path.
// Returns nil, nil if not found.
func (r *Registry) FindByLogDir(logDir string) (*RunEntry, error)

// UpdateStatus updates the status of a run.
func (r *Registry) UpdateStatus(id string, status RunStatus) error

// UpdatePhase updates or adds a phase status.
func (r *Registry) UpdatePhase(id string, phase Phase) error
```

#### Git URL Parsing

```go
// ParseGitRemote extracts owner/repo from a git remote URL.
// Supports HTTPS, SSH, and SSH (alt) formats.
// Returns empty string on parse failure.
func ParseGitRemote(url string) string

// GetRepository returns the repository identifier for the current directory.
// Falls back to directory name if git remote parsing fails.
func GetRepository(workingDir string) string
```

### Web Package (`internal/web/`)

#### Server

```go
// Config holds web server configuration.
type Config struct {
    Port     int    // Default: 8080
    Bind     string // Default: "localhost"
    Registry *registry.Registry
}

// Server is the HTTP server for the web interface.
type Server struct {
    config   Config
    router   *http.ServeMux
    server   *http.Server
    registry *registry.Registry
}

// New creates a new web server.
func New(config Config) *Server

// Start begins listening and serving requests.
// Blocks until shutdown.
func (s *Server) Start() error

// Shutdown gracefully stops the server.
// Waits up to 5 seconds for in-flight requests.
func (s *Server) Shutdown(ctx context.Context) error
```

#### Routes

| Path | Handler | Description |
|------|---------|-------------|
| `GET /` | `handleDashboard` | List all runs, grouped by repository |
| `GET /runs/{id}` | `handleRunDetail` | Show run details and phase list |
| `GET /runs/{id}/status` | `handleRunStatus` | HTML fragment for htmx polling |
| `GET /runs/{id}/transcript/{phase}` | `handleTranscript` | Show transcript for phase |
| `GET /runs/{id}/transcript/{phase}/{run}` | `handleTranscript` | Transcript for specific run |
| `GET /dashboard/status` | `handleDashboardStatus` | HTML fragment for dashboard polling |
| `GET /static/*` | `handleStatic` | Embedded static files (htmx, CSS) |

#### Middleware

```go
// SecurityHeaders adds security headers to all responses.
// - X-Content-Type-Options: nosniff
// - X-Frame-Options: DENY
// - Referrer-Policy: no-referrer
// - Content-Security-Policy (for HTML responses)
func SecurityHeaders(next http.Handler) http.Handler

// ValidateUUID validates that :id path parameters are valid UUID v4.
// Returns 404 for invalid UUIDs.
func ValidateUUID(next http.Handler) http.Handler

// PathSanitizer rejects requests with path traversal attempts.
// Returns 404 for paths containing "..".
func PathSanitizer(next http.Handler) http.Handler
```

### Template Structure

Templates are embedded using `//go:embed` and organized as follows:

```
internal/web/templates/
├── layout.html         # Base template with header, CSS, htmx
├── dashboard.html      # Run list, grouped by repository
├── dashboard_status.html  # Polling fragment for dashboard
├── run_detail.html     # Run information and phase list
├── run_status.html     # Polling fragment for run detail
├── transcript.html     # Wrapper for transcript with navigation
└── error.html          # Error pages (404, etc.)
```

#### Template Data Models

```go
// DashboardData is passed to dashboard.html.
type DashboardData struct {
    Repositories []RepositoryGroup
    Empty        bool  // true if no runs registered
}

type RepositoryGroup struct {
    Name string
    Runs []RunSummary
}

type RunSummary struct {
    ID        string
    Name      string
    Branch    string
    Status    string  // running, completed, failed, missing
    StatusCSS string  // status-running, status-completed, etc.
    StartedAt string  // Formatted time
    Missing   bool    // true if log_dir doesn't exist
}

// RunDetailData is passed to run_detail.html.
type RunDetailData struct {
    Run      *registry.RunEntry
    Phases   []PhaseView
    Duration string  // e.g., "2h 15m"
    Summary  *logs.Summary  // nil if summary.json not found
    Missing  bool
}

type PhaseView struct {
    Number    int
    Status    string
    StatusCSS string
    RunCount  int
    HasTranscript bool
}

// TranscriptData is passed to transcript.html.
type TranscriptData struct {
    RunID       string
    RunName     string
    Phase       int
    Run         int
    Content     template.HTML  // From transcript.RenderHTMLFragment
    PrevPhase   *int  // nil if first phase
    NextPhase   *int  // nil if last phase
}
```

### Config Extension

Add to `internal/config/config.go`:

```go
type Config struct {
    // ... existing fields ...
    ServePort int
    ServeBind string
}

// In Load():
v.SetDefault("serve-port", 8080)
v.SetDefault("serve-bind", "localhost")

// Environment variables:
// ORBIT_SERVE_PORT, ORBIT_SERVE_BIND
```

### Transcript HTML Extension

Modify `internal/transcript/html.go` to support navigation and fragment rendering:

```go
// RenderOptions contains options for rendering.
type RenderOptions struct {
    Title      string
    SessionID  string
    ProjectDir string
    // NEW fields for navigation
    Navigation *NavigationContext
}

// NavigationContext provides previous/next phase links.
type NavigationContext struct {
    PrevURL  string  // Empty if no previous
    PrevText string  // e.g., "Phase 1"
    NextURL  string  // Empty if no next
    NextText string  // e.g., "Phase 3"
    BackURL  string  // Back to run detail
    BackText string  // e.g., "Back to Run"
}

// RenderHTML renders a complete HTML document (used by apsis).
// Navigation is included when Navigation is set.
func RenderHTML(entries []Entry, opts RenderOptions) string {
    // ... existing document wrapper code ...

    // After opening <body>, before <header>:
    if opts.Navigation != nil {
        sb.WriteString(renderNavigationHTML(opts.Navigation))
    }

    // ... existing content rendering ...

    // After </main>, before </body>:
    if opts.Navigation != nil {
        sb.WriteString(renderNavigationHTML(opts.Navigation))
    }
}

// RenderHTMLFragment renders just the content without document wrapper.
// Returns HTML that can be embedded in an existing page template.
// Includes navigation at top/bottom when Navigation is set.
// Does NOT include <!DOCTYPE>, <html>, <head>, or <body> tags.
func RenderHTMLFragment(entries []Entry, opts RenderOptions) string {
    var sb strings.Builder

    // Add navigation at top if provided
    if opts.Navigation != nil {
        sb.WriteString(renderNavigationHTML(opts.Navigation))
    }

    // Render all entries (shared with RenderHTML)
    renderEntriesToBuilder(&sb, entries, opts)

    // Add navigation at bottom if provided
    if opts.Navigation != nil {
        sb.WriteString(renderNavigationHTML(opts.Navigation))
    }

    return sb.String()
}

// renderEntriesToBuilder writes entry HTML to the builder.
// Extracted to share between RenderHTML and RenderHTMLFragment.
func renderEntriesToBuilder(sb *strings.Builder, entries []Entry, opts RenderOptions)
```

**Note:** The web interface uses `RenderHTMLFragment` to embed transcript content within its own templates (which provide the page wrapper, navigation header, and footer). The existing `RenderHTML` function remains unchanged for `apsis` CLI usage but gains navigation support.

## Data Models

### Registry Entry Schema (v1)

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "schema_version": 1,
  "name": "feature-auth",
  "repository": "ArjenSchwarz/orbit",
  "log_dir": "/Users/arjen/projects/orbit/specs/feature-auth/.orbit",
  "status": "completed",
  "started_at": "2025-01-05T10:30:00Z",
  "finished_at": "2025-01-05T12:45:00Z",
  "branch": "feature/auth",
  "pid": null,
  "phases": [
    {"number": 1, "status": "completed", "run_count": 1},
    {"number": 2, "status": "completed", "run_count": 2},
    {"number": 3, "status": "completed", "run_count": 1}
  ]
}
```

### Relationship to Existing Summary

The registry entry complements but does not replace `summary.json` in log directories:

| Field | Registry Entry | summary.json |
|-------|---------------|--------------|
| Run status | Real-time during run | Snapshot at completion |
| Phase status | Per-phase tracking | Only `phases_completed` count |
| Cost tracking | Not stored | `total_cost_usd` |
| Session details | Not stored | Full `sessions` array |
| Purpose | Discovery and monitoring | Detailed run history |

**Why a separate registry is necessary:**

1. **Discovery**: The web server needs to discover all runs without scanning the entire filesystem. The registry provides a centralized index at `~/.orbit/runs/` that can be enumerated efficiently.

2. **Real-time status**: `summary.json` is only written when a run completes. The registry is updated as phases start/complete, enabling live monitoring of in-progress runs.

3. **Cross-project visibility**: `summary.json` files are scattered across project directories. The registry provides a single location to list runs across all projects.

4. **Metadata not in summary**: Fields like `repository`, `branch`, and `pid` are not stored in `summary.json` but are needed for the web UI.

The web interface reads from both: registry for listing/status/discovery, summary.json for detailed metrics (cost, session history) when viewing run details.

## Error Handling

### Registry Errors

| Error Condition | Handling |
|-----------------|----------|
| `~/.orbit/runs/` doesn't exist | Create directory on first write |
| Malformed JSON file | Skip entry, log warning, continue |
| Temp file creation fails | Return error, caller handles |
| Rename fails | Clean up temp file, return error |
| File read permission denied | Skip entry, log warning |

### Web Server Errors

| Error Condition | HTTP Status | Response |
|-----------------|-------------|----------|
| Invalid UUID format | 404 | Generic "not found" page |
| Run ID not in registry | 404 | Generic "not found" page |
| Log directory missing | 200 | Page with "missing" status indicator |
| Transcript file not found | 404 | "Transcript not found" message |
| Port already in use | N/A | Exit with error message |
| Bind address unavailable | N/A | Exit with error message |

### Registration Errors

| Error Condition | Handling |
|-----------------|----------|
| Invalid path (not a directory) | Exit with error message |
| No valid orbit logs found | Exit with error message |
| Registry write fails | Exit with error message |
| Git remote parsing fails | Use directory name as fallback |

## Testing Strategy

### Unit Tests

#### Registry Package

| Test | Coverage |
|------|----------|
| `TestRegister` | Create new entry, atomic write |
| `TestRegisterUpdate` | Update existing entry |
| `TestGet` | Retrieve by ID |
| `TestGetNotFound` | Return nil for missing ID |
| `TestList` | List all entries |
| `TestListMalformed` | Skip bad JSON, continue |
| `TestFindByLogDir` | Find by path |
| `TestParseGitRemote` | HTTPS, SSH, SSH-alt formats |
| `TestParseGitRemoteFallback` | Invalid URL returns empty |
| `TestGetRepository` | Integration with git command |
| `TestAtomicWrite` | Verify temp+rename pattern |

#### Web Package

| Test | Coverage |
|------|----------|
| `TestSecurityHeaders` | All headers present |
| `TestValidateUUID` | Valid and invalid UUIDs |
| `TestPathSanitizer` | Path traversal rejection |
| `TestHandleDashboard` | Template rendering |
| `TestHandleRunDetail` | Template with phases |
| `TestHandleTranscript` | Transcript rendering |
| `TestHandleNotFound` | 404 responses |
| `TestSymlinkBlocking` | Symlinks outside log_dir |

#### Config Extension

| Test | Coverage |
|------|----------|
| `TestServeDefaults` | Default port and bind |
| `TestServeEnvOverride` | Environment variable override |
| `TestServeConfigFile` | Config file values |

### Integration Tests

| Test | Coverage |
|------|----------|
| `TestServerStartStop` | Lifecycle management |
| `TestGracefulShutdown` | In-flight request completion |
| `TestPortInUse` | Error on port conflict |
| `TestRegistryPersistence` | Data survives restart |
| `TestAutoRegistration` | orbit run creates entry |
| `TestPhaseUpdates` | Phase status updates |

### Property-Based Tests

Git URL parsing is a good candidate for property-based testing:

```go
// TestPropertyGitURLRoundTrip verifies that parsed URLs
// consistently produce the same owner/repo output.
func TestPropertyGitURLRoundTrip(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        owner := rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9-]*`).Draw(t, "owner")
        repo := rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9-]*`).Draw(t, "repo")

        httpsURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)
        sshURL := fmt.Sprintf("git@github.com:%s/%s.git", owner, repo)

        expected := owner + "/" + repo

        if got := ParseGitRemote(httpsURL); got != expected {
            t.Fatalf("HTTPS: got %q, want %q", got, expected)
        }
        if got := ParseGitRemote(sshURL); got != expected {
            t.Fatalf("SSH: got %q, want %q", got, expected)
        }
    })
}
```

### Manual Testing

| Scenario | Steps |
|----------|-------|
| Mobile responsiveness | Test on 320px, 375px, 768px viewports |
| Dark mode | Test with system dark mode enabled |
| Live polling | Start run, verify 5s refresh in detail view |
| Missing log directory | Delete log_dir, verify "missing" status |
| Large transcript | Test with 10,000+ line transcript |
| No JavaScript | Disable JS, verify navigation works |

## Security Considerations

### Path Validation

All file access follows this validation chain:

1. **UUID validation** - Regex pattern for valid UUID v4
2. **Path sanitization** - Reject any path containing `..`
3. **Registry check** - Verify run ID exists in registry
4. **Boundary check** - Verify requested path is within registered `log_dir`
5. **Symlink check** - Resolve symlinks and verify they stay within `log_dir`

```go
// isPathWithinDir checks if path is within dir after resolving symlinks.
func isPathWithinDir(path, dir string) bool {
    resolved, err := filepath.EvalSymlinks(path)
    if err != nil {
        return false
    }
    resolvedDir, err := filepath.EvalSymlinks(dir)
    if err != nil {
        return false
    }
    rel, err := filepath.Rel(resolvedDir, resolved)
    if err != nil {
        return false
    }
    return !strings.HasPrefix(rel, "..")
}
```

### Content Security Policy

The CSP header allows:
- `default-src 'self'` - Only load resources from same origin
- `script-src 'self' 'unsafe-inline'` - Allow htmx inline attributes
- `style-src 'self' 'unsafe-inline'` - Allow inline styles

The `unsafe-inline` is required because htmx uses HTML attributes like `hx-get` which technically count as inline scripts. This is acceptable because:
1. We control all HTML generation (server-side templates)
2. User content in transcripts is properly escaped
3. The interface is typically localhost-only

## Implementation Notes

### htmx Integration

htmx polling is configured in templates:

```html
<!-- Run detail status polling (5 seconds) -->
<div id="run-status"
     hx-get="/runs/{{.Run.ID}}/status"
     hx-trigger="every 5s"
     hx-swap="outerHTML"
     class="status-{{.Run.Status}}">
    {{.Run.Status}}
</div>

<!-- Stop polling when terminal state -->
{{if eq .Run.Status "running"}}
<div hx-get="/runs/{{.Run.ID}}/status"
     hx-trigger="every 5s"
     hx-swap="outerHTML">
{{else}}
<div><!-- No polling for completed/failed -->
{{end}}
```

### Connection Failure Handling (Requirement 11.5)

htmx provides events for handling connection failures. The templates use these to track consecutive failures and display a reconnection message:

```html
<!-- In layout.html -->
<script>
    let consecutiveFailures = 0;
    const maxFailures = 3;

    document.body.addEventListener('htmx:sendError', function(evt) {
        consecutiveFailures++;
        if (consecutiveFailures >= maxFailures) {
            document.getElementById('connection-status').classList.add('disconnected');
        }
    });

    document.body.addEventListener('htmx:afterRequest', function(evt) {
        if (evt.detail.successful) {
            consecutiveFailures = 0;
            document.getElementById('connection-status').classList.remove('disconnected');
        }
    });
</script>

<div id="connection-status" class="connection-status">
    Connection lost - retrying...
</div>
```

CSS shows the status element only when disconnected:

```css
.connection-status {
    display: none;
    position: fixed;
    bottom: 1rem;
    right: 1rem;
    padding: 0.75rem 1rem;
    background-color: var(--error-color);
    color: white;
    border-radius: 4px;
}

.connection-status.disconnected {
    display: block;
}
```

### No-JavaScript Fallback (Requirement 10.8)

For users with JavaScript disabled, auto-refresh is unavailable. Templates include manual refresh links:

```html
<!-- In run_detail.html -->
<noscript>
    <div class="no-js-notice">
        Auto-refresh requires JavaScript.
        <a href="/runs/{{.Run.ID}}">Refresh manually</a>
    </div>
</noscript>

<!-- In dashboard.html -->
<noscript>
    <div class="no-js-notice">
        Auto-refresh requires JavaScript.
        <a href="/">Refresh manually</a>
    </div>
</noscript>
```

### Graceful Shutdown

```go
func (s *Server) Start() error {
    // Set up signal handling
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

    go func() {
        <-sigChan
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        s.Shutdown(ctx)
    }()

    log.Printf("Starting server at http://%s:%d", s.config.Bind, s.config.Port)
    return s.server.ListenAndServe()
}
```

### Large Transcript Handling

Session transcripts can grow large (10,000+ lines). The design handles this through:

1. **Server-side rendering**: The entire transcript is rendered server-side before sending to the client, avoiding client-side JavaScript performance issues.

2. **HTML `<details>` collapsible sections**: Tool outputs and Read file contents are collapsed by default, reducing initial render time and visual complexity.

3. **No pagination in v1**: Full transcripts are served in a single page. This is acceptable because:
   - Server-side rendering handles the conversion efficiently
   - Browser rendering of static HTML is fast even for large documents
   - Users can use browser find (Ctrl+F) to search the entire transcript

4. **Future enhancement (deferred)**: If performance becomes an issue with extremely large transcripts (50,000+ lines), virtual scrolling or pagination can be added as a v2 feature.

### CSS Approach

The web interface uses a minimal custom CSS file (~5KB) rather than Tailwind or other frameworks:

- Matches the existing style from `transcript/html.go`
- Supports dark mode via `prefers-color-scheme`
- Uses CSS variables for theming
- Mobile-first with media queries for larger screens

## Traceability Matrix

| Requirement | Design Element |
|-------------|----------------|
| 1.1 (orbit serve) | `cmd/orbit/serve.go` |
| 1.2-1.5 (config) | `internal/config/config.go` extension |
| 1.7 (graceful shutdown) | `Server.Shutdown()` with 5s timeout |
| 1.8-1.9 (port errors) | `Server.Start()` error handling |
| 2.1-2.13 (registry) | `internal/registry/` package |
| 3.1-3.7 (auto-registration) | `internal/orbit/orbit.go` integration |
| 4.1-4.12 (register cmd) | `cmd/orbit/register.go` |
| 5.1-5.8 (dashboard) | `handleDashboard`, `dashboard.html` |
| 6.1-6.9 (run detail) | `handleRunDetail`, `run_detail.html` |
| 7.1-7.7 (transcript) | `handleTranscript`, `RenderHTMLFragment`, transcript navigation |
| 8.1-8.7 (mobile) | CSS media queries, viewport meta |
| 9.1-9.10 (security) | Middleware, path validation |
| 10.1-10.7 (frontend) | Templates, embedded htmx |
| 10.8 (no-JS fallback) | `<noscript>` manual refresh links |
| 11.1-11.4 (live updates) | htmx polling, status fragments |
| 11.5-11.6 (connection failure) | htmx error events, `connection-status` element |
