# Orbit Web Interface - Design Plan

## Problem Statement

Users need a way to monitor orbit runs and browse logs/results remotely (e.g., from a phone when controlling a remote machine). The current CLI-only interface requires terminal access.

## Requirements

1. **View current runs** - See in-progress orchestration status
2. **View past runs** - Browse historical runs and their results
3. **Browse logs** - Read transcripts, session files, summaries
4. **Remote access** - Work well on mobile devices
5. **File isolation** - Server reads files, not the browser directly

## Architecture

### New Commands

**`orbit serve`** - Starts the web server

```bash
orbit serve [--port 8080] [--bind localhost]
```

- Single binary, no new tools to install
- Shares existing code for reading logs/transcripts
- Auto-discovers all registered runs from `~/.orbit/runs/`
- Port and bind configurable via CLI flags or `.orbit.yaml`

**Configuration** (in `.orbit.yaml` or `~/.orbit.yaml`):

```yaml
serve:
  port: 8080
  bind: localhost
```

Configuration priority: CLI flags > environment variables > config file > defaults

**`orbit register`** - Manually registers a run directory

```bash
orbit register /path/to/.orbit/2024-01-03-150405
orbit register .                    # Register current directory's .orbit/
orbit register --name "my-feature"  # Optional display name
```

- Adds existing log directories to the registry
- Useful for historical runs or runs started before web interface existed
- Validates that the directory contains valid orbit logs

## Run Registration Mechanism

Directory-based registration using `~/.orbit/runs/` with one file per run:

```
~/.orbit/runs/
├── abc123.json        # Run metadata
├── def456.json        # Another run
└── ...
```

Each file contains:
```json
{
  "id": "abc123",
  "name": "feature-x",
  "repository": "ArjenSchwarz/orbit",
  "log_dir": "/path/to/.orbit/2024-01-03-150405",
  "status": "running",
  "started_at": "2024-01-03T15:04:05Z",
  "branch": "feature/feature-x",
  "pid": 12345
}
```

The `repository` field enables grouping runs by project in the web interface. It is derived from:
1. Git remote origin URL (preferred)
2. Directory name as fallback

**Benefits:**
- No file locking issues (atomic file operations)
- Can detect stale runs via PID checking
- Natural filesystem operations
- Easy cleanup via file deletion

**Registration sources:**
1. `orbit run` - auto-registers at startup, updates on completion
2. `orbit register` - manual registration of existing directories

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

### Frontend Approach

Server-side rendering with Go templates + htmx for interactivity.

**htmx** is a lightweight (~14kb) JavaScript library that enables dynamic behavior via HTML attributes. Instead of fetching JSON and manipulating the DOM with JavaScript, htmx fetches HTML fragments from the server and swaps them into the page.

```html
<!-- Auto-refresh run status every 5 seconds -->
<div hx-get="/api/runs/abc123/status"
     hx-trigger="every 5s"
     hx-swap="innerHTML">
  Status: running...
</div>

<!-- Load transcript on click -->
<a hx-get="/runs/abc123/transcript/1"
   hx-target="#content">
  View Phase 1
</a>
```

**Benefits:**
- Minimal JavaScript, no build step
- Fast initial load, works great on mobile
- Server does all rendering (reuse Go templates)
- Progressive enhancement

## Data Flow

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│  orbit run      │────▶│                  │◀────│  orbit serve    │
│  (auto-register)│     │  ~/.orbit/runs/  │     │  (discovers)    │
└─────────────────┘     │    (registry)    │     └────────┬────────┘
                        │                  │               │
┌─────────────────┐     │                  │               │
│  orbit register │────▶│                  │               │
│  (manual)       │     └──────────────────┘               │
└─────────────────┘                                        │
        │                                                  │
        ▼                                                  ▼
┌─────────────────┐                              ┌─────────────────┐
│  .orbit/        │◀─────────────────────────────│  Web Browser    │
│  (log files)    │         reads                │  (phone/desktop)│
└─────────────────┘                              └─────────────────┘
```

1. `orbit run` auto-registers in `~/.orbit/runs/` at startup, updates on completion
2. `orbit register` manually adds existing log directories to registry
3. `orbit serve` scans registry for all known runs
4. Server reads log files directly when serving requests
5. Browser only talks to server, never touches filesystem

## Live Updates

For showing live progress of running orchestrations, use htmx polling:

```html
<div hx-get="/api/runs/abc123/status"
     hx-trigger="every 5s"
     hx-swap="innerHTML">
  <!-- Status updates automatically -->
</div>
```

Simple, works everywhere, and htmx handles it declaratively. Can upgrade to Server-Sent Events (SSE) later if needed for lower latency.

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
- [ ] Modify `orbit run` to auto-register/deregister runs
- [ ] Add `orbit register` subcommand for manual registration
- [ ] Add `orbit serve` subcommand skeleton
- [ ] Basic HTTP server with routing

### Phase 2: API Layer
- [ ] Implement REST API endpoints
- [ ] Run list endpoint
- [ ] Run detail endpoint
- [ ] Transcript serving endpoint

### Phase 3: Web UI
- [ ] HTML templates with embedded static files (including htmx)
- [ ] Dashboard page (run list)
- [ ] Run detail page
- [ ] Transcript viewer
- [ ] Basic responsive CSS

### Phase 4: Polish
- [ ] Auto-refresh for running jobs (htmx polling)
- [ ] Mobile-optimized styling
- [ ] Dark mode (system preference)
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

## Design Decisions

| Decision | Choice |
|----------|--------|
| Discovery | Auto-discover all registered runs from `~/.orbit/runs/` |
| Ad-hoc viewing | Use `orbit register` command to add runs to registry |
| Frontend | Server-side rendering with htmx |
| Authentication | None for v1 (localhost-only by default) |
