# Implementation Explanation: apsis-serve

## Beginner Level

### What Changed

This feature adds a web browser interface to the `apsis` tool. Previously, you could only list AI coding sessions in a terminal with `apsis -l` and view transcripts as text files. Now you can run `apsis serve` and open a web page in your browser to browse and read session transcripts with search, filtering, and auto-refresh.

### Why It Matters

Developers using AI coding agents (Claude Code, Codex, Copilot, Kiro) accumulate session transcripts. Finding and reading a specific session in the terminal is cumbersome. The web interface lets you filter by agent type, search by session ID, and read formatted transcripts in your browser — from any device on your network if needed.

### Key Concepts

- **Session**: A recorded conversation between a developer and an AI coding agent. Stored as files (JSONL format) or in databases (SQLite for Kiro CLI).
- **Transcript**: The formatted, human-readable version of a session.
- **HTMX**: A small JavaScript library that makes web pages update dynamically without writing complex JavaScript. The session list refreshes every 15 seconds using HTMX polling.
- **Embedded assets**: The CSS, JavaScript, and HTML templates are compiled into the binary using Go's `embed` package, so `apsis serve` works as a single file with no external dependencies.

### How the Code Is Organized

The work splits into two main parts:
1. **Extracting session discovery** — the logic that finds and lists sessions was moved out of `cmd/apsis/main.go` into a reusable `internal/sessions/` package. Both the CLI (`apsis -l`) and the web server use this shared code.
2. **Building the web server** — a new `internal/apsisweb/` package handles HTTP requests, renders HTML templates, and serves static files.

---

## Intermediate Level

### Changes Overview

**5 commits, 31 files changed, ~5700 lines added, ~3400 lines removed**

| Component | Files | Purpose |
|-----------|-------|---------|
| `internal/sessions/` | types.go, lister.go, resolver.go + tests | Session discovery and resolution extracted from main.go |
| `internal/apsisweb/` | server.go, handlers.go, middleware.go, static.go + templates, CSS, tests | HTTP server with HTMX-powered UI |
| `cmd/apsis/main.go` | Modified | Serve subcommand + wiring to sessions package |
| `internal/web/` | middleware.go, handlers.go | Export `IsPathWithinDir` (was unexported) |

### Implementation Approach

**Session extraction** follows a pure refactor pattern: move code into a package, replace call sites, verify identical output. The `Lister` collects sessions from all 5 agent types, returns non-fatal warnings for sources that fail, and sorts oldest-first. The `Resolver` opens a specific session by source+ID, performing `IsPathWithinDir` validation before opening files.

**Web server** follows the same patterns as `orbit serve`:
- `http.ServeMux` with Go 1.22 method-based routing (`GET /sessions/{source}/{id}`)
- Middleware chain: `SecurityHeaders → PathSanitizer → ValidateSource → SanitizeSessionID → handler`
- Templates parsed at `init()` into a `pageTemplates` map; fragments rendered separately for HTMX
- Client-side filtering and search via vanilla JavaScript, state preserved across HTMX polls via closure variables

**Subcommand detection** uses `os.Args[1]` check before `flag.Parse()` with a separate `flag.FlagSet` for serve-specific flags. Config priority: CLI flag > env var > default.

### Trade-offs

- **Separate `internal/apsisweb/` vs extending `internal/web/`**: Chose separation to avoid coupling orbit and apsis web code. Minor duplication of static file serving (~20 lines) and HTMX vendored in two places.
- **Static rendering for transcripts**: No live streaming in v1. Simpler implementation that reuses `RenderHTMLFragment()`. Active sessions require manual page refresh.
- **`New()` returns error**: Unlike orbit's `web.New()`, because creating `Lister`/`Resolver` can fail (home directory lookup). Signal handling is in `serveCommand()` rather than the Server type, keeping it testable.

---

## Expert Level

### Technical Deep Dive

**Session resolution security model**: Each `Resolve` method constructs a file path from the session ID and validates it with `web.IsPathWithinDir(resolvedPath, baseDir)` before opening. The `SanitizeSessionID` middleware rejects `..`, `/`, `\` after URL decoding. This creates defense in depth: middleware blocks obvious traversal, resolver validates the resolved path against the expected directory via symlink-safe evaluation (`filepath.EvalSymlinks`).

**Kiro IDE handling**: `.chat` files are JSON (not JSONL) with an `executionId` field. Multiple `.chat` files can share an `executionId`; the resolver picks the one with the most chat entries (then most recent mtime as tiebreaker). Cost data comes from a separate execution detail file, threaded via `Metadata.CostPath` to `ParseJSONLWithFormat`.

**Kiro CLI sessions** are SQLite-backed (`logs.GetSession` returns an `io.Reader`). No file path exists, so `IsPathWithinDir` is not applicable, `Metadata.Size` is 0, and the 50MB size guard is bypassed. `ResolvePath` returns an error for Kiro CLI.

**HTMX polling correctness**: The `sessions_list` fragment includes both the session list and the empty state. Filter/search state is held in JavaScript closure variables and reapplied via `htmx:afterSwap` event listener. The HTMX polling div is always rendered (even when empty) to ensure auto-refresh works when sessions appear later.

**CSP header**: Applied only to `text/html` responses via `cspResponseWriter` in orbit's `SecurityHeaders` middleware. The wrapper intercepts `WriteHeader` and checks `Content-Type` before writing. Apsis handlers set `Content-Type: text/html` before template execution, so CSP is correctly applied.

### Architecture Impact

The `internal/sessions/` package creates a clean data access layer that decouples session discovery from both the CLI and web presentation. Future features (cross-project browsing, live streaming, API endpoints) can build on this abstraction without modifying existing code.

The `internal/apsisweb/` package mirrors `internal/web/` patterns but is independent. Shared security middleware is imported, not duplicated. If the shared surface grows, a `internal/web/shared/` extraction could be done later.

### Potential Issues

- **Memory usage**: Large transcripts are fully parsed and rendered into a single HTML string. The 50MB guard mitigates this but doesn't cover Kiro CLI sessions (size unknown). A very large Kiro CLI session could cause high memory usage.
- **Concurrent polling**: Each HTMX poll triggers a full `ListAll()` which reads directories and opens files for timestamp extraction. Under high polling frequency this is negligible, but if many browser tabs are open, the I/O could add up.
- **Port conflicts**: Default port 8081 could conflict with other dev tools. The `--port` flag and `APSIS_SERVE_PORT` env var provide workarounds.

---

## Completeness Assessment

### Fully Implemented
- All 8 requirement groups (1.x through 8.x) are addressed
- All 20 tasks from tasks.md are completed
- Security model (headers, path validation, input sanitization) matches spec
- Responsive design with dark mode, 44px touch targets, 320px minimum viewport
- HTMX auto-refresh with connection loss detection and filter state preservation
- Integration test covering end-to-end flow

### Spec Divergences (Documented)
- `New()` returns `(*Server, error)` instead of `*Server` — documented in agent-notes and decision log
- Signal handling in `serveCommand()` instead of `Server.Start()` — documented in agent-notes

### Not Tested (By Design)
- Requirements 5.x (CSS/responsive) — verified by inspection, not automatable in Go tests
- Requirements 8.x (connection resilience JS) — browser-level behavior
- Client-side filtering and search (3.5, 3.6, 3.10) — JavaScript behavior
