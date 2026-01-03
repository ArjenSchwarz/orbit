# Orbit Web Interface - Design Plan

## Problem Statement

Users need a way to monitor orbit runs and browse logs/results remotely (e.g., from a phone when controlling a remote machine). The current CLI-only interface requires terminal access.

## Requirements

1. **View current runs** - See in-progress orchestration status
2. **View past runs** - Browse historical runs and their results
3. **Browse logs** - Read transcripts, session files, summaries
4. **Remote access** - Work well on mobile devices
5. **File isolation** - Server reads files, not the browser directly

## Architectural Options

### Option A: `orbit serve` Subcommand

Add a `serve` subcommand to orbit that starts a web server.

```bash
orbit serve [--port 8080] [--bind 0.0.0.0]
```

**Pros:**
- Single binary, no new tools to install
- Shares existing code for reading logs/transcripts
- Natural fit with existing CLI

**Cons:**
- Need a registration mechanism for runs to be discoverable

### Option B: Separate `orbit-web` Binary

Create a new binary specifically for the web interface.

```bash
orbit-web [--port 8080]
```

**Pros:**
- Separation of concerns
- Can run independently of orchestration
- Smaller attack surface if exposing to network

**Cons:**
- Another binary to build/install
- Code duplication or shared internal packages

### Option C: Embedded Server During Runs (Live Only)

Start a web server automatically when `orbit run` executes.

**Pros:**
- Zero configuration for live monitoring
- Server lifetime matches run lifetime

**Cons:**
- Can't view past runs
- No historical browsing
- Would need separate solution for history

---

**Recommendation:** Option A (`orbit serve`) - keeps everything in one tool, maximizes code reuse.

## Run Registration Mechanism

For the server to discover runs, we need a registration system.

### Approach 1: Global Registry File

Store run metadata in `~/.orbit/runs.json`:

```json
{
  "runs": [
    {
      "id": "abc123",
      "spec_path": "/home/user/project/specs/feature-x",
      "log_dir": "/home/user/project/specs/feature-x/.orbit/2024-01-03-150405",
      "status": "running",
      "started_at": "2024-01-03T15:04:05Z",
      "branch": "feature/feature-x"
    }
  ]
}
```

**Pros:**
- Simple to implement
- Easy to query

**Cons:**
- Need file locking for concurrent access
- Registry can get stale if runs crash

### Approach 2: Directory-Based Registration

Use `~/.orbit/runs/` with one file per run:

```
~/.orbit/runs/
├── abc123.json        # Active run metadata
├── def456.json        # Another run
└── ...
```

Each file contains:
```json
{
  "log_dir": "/path/to/.orbit/2024-01-03-150405",
  "status": "running",
  "started_at": "2024-01-03T15:04:05Z",
  "pid": 12345
}
```

**Pros:**
- No locking issues (atomic file operations)
- Can detect stale runs via PID checking
- Natural filesystem operations

**Cons:**
- More files to manage
- Need cleanup strategy

### Approach 3: SQLite Database

Use `~/.orbit/orbit.db` for all run tracking.

**Pros:**
- ACID transactions
- Query flexibility
- Built-in for Go (mattn/go-sqlite3)

**Cons:**
- CGO dependency (unless using modernc.org/sqlite)
- More complex than needed?

---

**Recommendation:** Approach 2 (directory-based) - simple, robust, no locking issues, easy cleanup.

## Web Interface Architecture

### Backend Components

```
cmd/orbit/main.go
  └── serve.go              # New: serve subcommand

internal/
  └── web/
      ├── server.go         # HTTP server setup, routing
      ├── handlers.go       # Request handlers
      ├── api.go            # REST API endpoints
      └── templates/        # HTML templates (embedded)
          ├── layout.html
          ├── runs.html
          ├── run_detail.html
          └── transcript.html

  └── registry/
      ├── registry.go       # Run registration/discovery
      └── cleanup.go        # Stale run cleanup
```

### API Endpoints

```
GET  /                      # Dashboard - list all runs
GET  /api/runs              # JSON list of all runs
GET  /api/runs/:id          # Single run details
GET  /api/runs/:id/summary  # Summary.json content
GET  /api/runs/:id/phases   # Phase list with status
GET  /api/runs/:id/sessions # Session list
GET  /api/runs/:id/transcript/:phase  # Transcript content
GET  /runs/:id              # Run detail page
GET  /runs/:id/transcript/:phase      # Transcript viewer
```

### Frontend Approach Options

#### Frontend Option 1: Server-Side Rendering (SSR) with htmx

Use Go templates + htmx for interactivity without heavy JavaScript.

**Pros:**
- Minimal JavaScript
- Fast initial load
- Works great on mobile
- Easy to implement

**Cons:**
- Less "app-like" feel
- Limited offline capability

#### Frontend Option 2: Static HTML + Vanilla JS

Serve static HTML with fetch() calls to API.

**Pros:**
- No framework dependencies
- Simple to understand
- Good performance

**Cons:**
- More boilerplate code
- Manual DOM manipulation

#### Frontend Option 3: SPA Framework (React/Vue/Svelte)

Build a full single-page application.

**Pros:**
- Rich interactivity
- Modern UI patterns

**Cons:**
- Build complexity
- Larger bundle size
- Overkill for this use case

---

**Recommendation:** Option 1 (SSR + htmx) - simplest approach, works well on mobile, minimal dependencies.

## Data Flow

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│  orbit run      │────▶│  ~/.orbit/runs/  │◀────│  orbit serve    │
│  (registers)    │     │  (registry)      │     │  (discovers)    │
└─────────────────┘     └──────────────────┘     └────────┬────────┘
        │                                                  │
        │                                                  │
        ▼                                                  ▼
┌─────────────────┐                              ┌─────────────────┐
│  .orbit/        │◀─────────────────────────────│  Web Browser    │
│  (log files)    │         reads                │  (phone/desktop)│
└─────────────────┘                              └─────────────────┘
```

1. `orbit run` registers itself in `~/.orbit/runs/` at startup
2. Updates status periodically or on phase completion
3. `orbit serve` scans registry for known runs
4. Server reads log files directly when serving requests
5. Browser only talks to server, never touches filesystem

## Live Updates

For showing live progress of running orchestrations:

### Option 1: Polling

Browser polls `/api/runs/:id` every N seconds.

**Pros:** Simple, works everywhere
**Cons:** Latency, unnecessary requests

### Option 2: Server-Sent Events (SSE)

Server pushes updates to browser.

```go
GET /api/runs/:id/stream
```

**Pros:** Real-time, efficient, works with htmx
**Cons:** Need connection management

### Option 3: WebSockets

Full duplex communication.

**Pros:** Most flexible
**Cons:** Overkill for read-only updates

---

**Recommendation:** Start with polling (simple), add SSE later if needed.

## Mobile Considerations

- Responsive design (CSS media queries or Tailwind)
- Touch-friendly tap targets (44px minimum)
- Readable transcript formatting
- Collapsible sections for long content
- Dark mode support (follows system preference)

## Security Considerations

- Default bind to `localhost` only
- Optional `--bind` flag for network access
- No authentication by default (trusted network assumption)
- Future: optional basic auth or token auth
- No write operations exposed (read-only interface)

## Implementation Phases

### Phase 1: Core Infrastructure
- [ ] Create run registry system (`internal/registry/`)
- [ ] Modify `orbit run` to register/deregister runs
- [ ] Add `orbit serve` subcommand skeleton
- [ ] Basic HTTP server with routing

### Phase 2: API Layer
- [ ] Implement REST API endpoints
- [ ] Run list endpoint
- [ ] Run detail endpoint
- [ ] Transcript serving endpoint

### Phase 3: Web UI
- [ ] HTML templates with embedded static files
- [ ] Dashboard page (run list)
- [ ] Run detail page
- [ ] Transcript viewer
- [ ] Basic responsive CSS

### Phase 4: Polish
- [ ] Auto-refresh for running jobs (polling)
- [ ] Mobile-optimized styling
- [ ] Dark mode
- [ ] Error handling and empty states

### Phase 5: Future Enhancements
- [ ] SSE for live updates
- [ ] Search/filter runs
- [ ] Cost analytics dashboard
- [ ] Optional authentication

## Alternative Approaches Considered

### File Server + Static Site Generation

Generate static HTML whenever logs update, serve with any web server.

**Why not:** Requires external web server, more moving parts.

### Separate Long-Running Daemon

A daemon process that watches filesystem and maintains state.

**Why not:** More operational complexity, needs process management.

### Cloud-Based Dashboard

Push logs to a cloud service for viewing.

**Why not:** Privacy concerns, external dependency, overkill for local use.

## Questions for Discussion

1. Should `orbit serve` discover runs automatically from `~/.orbit/runs/` OR require explicit paths?

2. Should we support viewing arbitrary `.orbit/` directories (not just registered runs)?

3. Is htmx acceptable as a dependency, or prefer zero JavaScript dependencies?

4. Should the server auto-open browser on startup?

5. What level of authentication (if any) should be included in v1?
