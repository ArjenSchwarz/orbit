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
serve-port: 8080
serve-bind: localhost
```

This follows the existing flat key pattern used by other config options (`post-command`, `date-subdirs`, etc.).

Configuration priority: CLI flags > environment variables (`ORBIT_SERVE_PORT`, `ORBIT_SERVE_BIND`) > config file > defaults

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
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "feature-x",
  "repository": "ArjenSchwarz/orbit",
  "log_dir": "/path/to/.orbit/2024-01-03-150405",
  "status": "running",
  "started_at": "2024-01-03T15:04:05Z",
  "branch": "feature/feature-x",
  "pid": 12345
}
```

Run IDs use standard UUID v4 format. Registry filenames match the ID (e.g., `550e8400-e29b-41d4-a716-446655440000.json`).

The `repository` field enables grouping runs by project in the web interface. It is derived from:
1. Git remote origin URL (preferred)
2. Directory name as fallback

**Git URL parsing logic:**

Extract `owner/repo` from common remote URL formats:
- HTTPS: `https://github.com/owner/repo.git` → `owner/repo`
- SSH: `git@github.com:owner/repo.git` → `owner/repo`
- SSH (alt): `ssh://git@github.com/owner/repo.git` → `owner/repo`

Strip `.git` suffix if present. For non-standard URLs or parsing failures, fall back to the working directory name.

**Benefits:**
- No file locking issues (atomic file operations)
- Can detect stale runs via PID checking
- Natural filesystem operations
- Easy cleanup via file deletion

**Concurrency handling:**
- Use atomic write pattern: write to temp file, then rename
- Each run has a unique ID, so no conflicts between different runs
- Same run updating its own file is sequential (single process)
- `orbit serve` reads are eventually consistent (acceptable for UI refresh)

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

### Network Security
- Default bind to `localhost` only
- Optional `--bind` flag for network access
- No authentication by default (trusted network assumption)
- Future: optional basic auth or token auth
- No write operations exposed (read-only interface)

### Input Validation
- Run IDs must be standard UUID v4 format (e.g., `550e8400-e29b-41d4-a716-446655440000`)
- Use `filepath.Clean()` on all path components
- Reject path components containing `..` or absolute paths
- Validate phase numbers as positive integers

### File Access Controls
- Only serve files from directories in the registry
- Verify requested paths are within registered `log_dir` boundaries
- Never follow symlinks outside registered directories

### API Security
- Add input validation middleware for all endpoints
- Return generic 404 for invalid/unauthorized paths (no information leakage)
- Rate limiting consideration for future if exposed to network

### Registry Integrity
- Atomic writes must handle partial failures (cleanup temp files on error)
- PID-based stale detection has limitations due to OS PID reuse; combine with timestamp checks
- Consider file locking or heartbeat mechanism for long-running processes

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

## Testing Strategy

### Unit Tests
- Registry operations (register, list, delete, stale detection)
- Git URL parsing for repository extraction
- Input validation functions (UUID validation, path sanitization)
- Config loading for serve options

### Integration Tests
- HTTP endpoints return correct responses
- File serving respects directory boundaries
- Registry persistence across restarts

### Manual/E2E Testing
- Mobile responsiveness verification
- Live polling during active runs
- Cross-browser testing (Safari mobile, Chrome, Firefox)

## Performance Considerations

### Initial Implementation
- Simple file-based reads on each request (sufficient for typical usage)
- No caching in v1

### Future Optimizations (if needed)
- In-memory cache of registry with file-watch invalidation
- Pagination for run list if users accumulate many runs
- Lazy loading of transcript content
- Summary.json caching with mtime checking

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

## Open Design Questions

The following items need to be resolved during detailed specification:

### Git URL Parsing
- Handle enterprise Git platforms (GitLab, Bitbucket, self-hosted)
- Support alternative SSH formats and custom ports
- Graceful handling of repositories with unusual remote configurations

### API Response Strategy
- Clarify when endpoints return JSON vs HTML fragments
- Define content negotiation approach (Accept header vs separate endpoints)
- Document htmx-specific response requirements

### Mobile UX Details
- Specific viewport meta tag configuration
- Minimum supported screen widths
- Touch gesture handling for transcript navigation
- Offline/poor-connectivity behavior

### Auto-Registration Timing
- Exact point in `orbit run` lifecycle when registration occurs
- How to handle registration failures (retry, warn, fail?)
- Cleanup behavior when runs are interrupted (SIGINT, SIGTERM)

### Cost Analytics
- Consider promoting from Phase 5 to Phase 4
- Define specific metrics to display (per-run, per-repo, trends)
- Data aggregation and storage requirements

## Design Decisions

| Decision | Choice |
|----------|--------|
| Discovery | Auto-discover all registered runs from `~/.orbit/runs/` |
| Ad-hoc viewing | Use `orbit register` command to add runs to registry |
| Frontend | Server-side rendering with htmx |
| Authentication | None for v1 (localhost-only by default) |
