# Requirements: apsis-serve

## Introduction

Apsis currently provides `apsis -l` to list AI coding agent sessions for the current project as plain text output. While functional, this lacks the ability to browse transcripts, filter by agent type, or access session data from other devices. This feature adds an `apsis serve` subcommand that launches a local web server for browsing and viewing session transcripts, following the patterns established by `orbit serve`.

The web interface is scoped to the current project directory (matching `apsis -l` behaviour). It renders transcripts using the existing HTML rendering pipeline. Live streaming of active sessions is out of scope for v1.

---

### 1. Session Listing Extraction

**User Story:** As a developer, I want session discovery logic available as a reusable package, so that both the CLI and web interface can list sessions without code duplication.

**Acceptance Criteria:**

1. <a name="1.1"></a>The system SHALL extract session discovery logic from `cmd/apsis/main.go` into an `internal/sessions/` package
2. <a name="1.2"></a>The `internal/sessions/` package SHALL expose a `Lister` type with a `ListAll(projectPath string)` method that returns `([]SessionInfo, []ListWarning, error)` -- sessions found, per-source warnings, and a fatal error if the entire listing fails
3. <a name="1.3"></a>The `internal/sessions/` package SHALL expose a `Resolver` type with a `Resolve(source, sessionID string)` method that returns a `ResolvedSession` containing an `io.ReadCloser`, `SessionMetadata` (Source, ID, CreatedAt, Size, CostPath), and error
4. <a name="1.4"></a>The existing `apsis -l` command SHALL produce identical output after the extraction
5. <a name="1.5"></a>The `Lister` SHALL discover sessions from all five supported agent types: Claude Code, Codex, Copilot, Kiro CLI, and Kiro IDE
6. <a name="1.6"></a>The `Lister` SHALL return sessions sorted by creation time (oldest first) to match existing `apsis -l` behaviour
7. <a name="1.7"></a>The `internal/sessions/` package SHALL define canonical source constants using hyphenated lowercase identifiers: `claude`, `codex`, `copilot`, `kiro-cli`, `kiro-ide`
8. <a name="1.8"></a>Each source constant SHALL have a corresponding display name for CLI output (e.g., `kiro-ide` displays as `kiro ide` in `apsis -l`). All agents SHALL use the display name mapping for consistency, even when the internal identifier and display name are identical

---

### 2. Web Server Subcommand

**User Story:** As a developer, I want to run `apsis serve` to start a local web server, so that I can browse session transcripts in a web browser from any device on my network.

**Acceptance Criteria:**

1. <a name="2.1"></a>The system SHALL add a `serve` subcommand to the apsis CLI that starts an HTTP server
2. <a name="2.2"></a>The server SHALL default to port 8081 and bind address `localhost`
3. <a name="2.3"></a>The server SHALL accept `--port` and `--bind` flags to override defaults
4. <a name="2.4"></a>The server SHALL accept `--project` flag to specify the project directory, defaulting to the current working directory
5. <a name="2.5"></a>The server SHALL support `APSIS_SERVE_PORT` and `APSIS_SERVE_BIND` environment variables (CLI flags take priority)
6. <a name="2.6"></a>The server SHALL print the listening URL to stderr on startup (e.g., `Listening on http://localhost:8081`)
7. <a name="2.7"></a>The server SHALL handle SIGINT and SIGTERM for graceful shutdown with a 5-second timeout
8. <a name="2.8"></a>The `serve` subcommand SHALL be detected by checking `os.Args[1]` before `flag.Parse()`, using a separate `flag.FlagSet` for serve-specific flags
9. <a name="2.9"></a>WHEN `--bind` is set to `0.0.0.0`, the server SHALL print a warning to stderr: "Warning: Server is accessible from the network. Session data may contain sensitive information."
10. <a name="2.10"></a>The server SHALL set HTTP timeouts: `ReadHeaderTimeout: 10s`, `WriteTimeout: 120s`, `IdleTimeout: 60s`
11. <a name="2.11"></a>`apsis serve --help` SHALL display serve-specific help text. Unknown flags SHALL produce an error.

---

### 3. Session List Page

**User Story:** As a developer, I want a web page listing all sessions for my project, so that I can quickly find and open the transcript I need.

**Acceptance Criteria:**

1. <a name="3.1"></a>The root URL (`GET /`) SHALL display a page listing all sessions for the configured project
2. <a name="3.2"></a>Each session entry SHALL display the agent source (as a colour-coded badge), session ID, creation timestamp, and file size
3. <a name="3.3"></a>Sessions SHALL be sorted newest-first by default (the web handler reverses the Lister's oldest-first sort)
4. <a name="3.4"></a>Each session entry SHALL be a clickable link to the transcript view page
5. <a name="3.5"></a>The page SHALL include filter toggles for each agent type (claude, codex, copilot, kiro-cli, kiro-ide) plus an "all" option
6. <a name="3.6"></a>Filtering SHALL be performed client-side without a server round-trip
7. <a name="3.7"></a>The session list SHALL auto-refresh via HTMX polling every 15 seconds, targeting only the session list container (not the filter controls or search input)
8. <a name="3.8"></a>WHEN no sessions are found, the page SHALL display an empty state message indicating no sessions exist for this project
9. <a name="3.9"></a>Session IDs longer than 40 characters SHALL be truncated in the display with the full ID available via a title tooltip
10. <a name="3.10"></a>The page SHALL include a text search input that filters sessions by ID (client-side)
11. <a name="3.11"></a>Client-side filter and search state SHALL be preserved across HTMX poll refreshes
12. <a name="3.12"></a>WHEN some agent sources fail during listing, the page SHALL display a warning banner indicating which sources could not be loaded, while still showing sessions from successful sources

---

### 4. Transcript View Page

**User Story:** As a developer, I want to view a session transcript rendered as HTML in my browser, so that I can read through the conversation with proper formatting and collapsible tool calls.

**Acceptance Criteria:**

1. <a name="4.1"></a>The URL `GET /sessions/{source}/{id}` SHALL render the transcript for the specified session
2. <a name="4.2"></a>The transcript SHALL be rendered using the existing `transcript.RenderHTMLFragment()` function
3. <a name="4.3"></a>The page SHALL display session metadata above the transcript: agent source badge, full session ID, creation date, file size, and cost (when available from transcript parse metadata)
4. <a name="4.4"></a>The page SHALL include a "Back to sessions" navigation link
5. <a name="4.5"></a>The transcript content SHALL be displayed in a max-width 900px centred container
6. <a name="4.6"></a>WHEN the session ID or source is invalid, the server SHALL return a 404 error page
7. <a name="4.7"></a>The `{source}` path parameter SHALL be validated against the known agent types: `claude`, `codex`, `copilot`, `kiro-cli`, `kiro-ide`
8. <a name="4.8"></a>The `{id}` path parameter SHALL be rejected IF it contains path traversal characters (`..`, `/`, `\`) after URL decoding
9. <a name="4.9"></a>WHEN a transcript file exceeds 50MB, the server SHALL return a user-friendly error page explaining the file is too large to render in the browser, and suggest using the CLI instead (`apsis <session-id>`)

---

### 5. Responsive Design and Dark Mode

**User Story:** As a developer, I want the web interface to work on my phone and respect my system's dark mode preference, so that I can browse transcripts from any device comfortably.

**Acceptance Criteria:**

1. <a name="5.1"></a>The interface SHALL use CSS variables for theming, following the same variable naming convention as `orbit serve`
2. <a name="5.2"></a>The interface SHALL support dark mode via `@media (prefers-color-scheme: dark)`
3. <a name="5.3"></a>All interactive elements SHALL have a minimum touch target of 44x44 pixels
4. <a name="5.4"></a>The layout SHALL adapt to viewports as narrow as 320px (iPhone SE)
5. <a name="5.5"></a>WHEN the viewport is narrower than 768px, session cards SHALL stack to a single-column layout
6. <a name="5.6"></a>WHEN the viewport is narrower than 768px, filter pills SHALL wrap to multiple lines
7. <a name="5.7"></a>The interface SHALL use the same system font stack as `orbit serve` (`-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, ...`)

---

### 6. Security

**User Story:** As a developer, I want the web server to be secure by default, so that I don't expose sensitive session data unintentionally.

**Acceptance Criteria:**

1. <a name="6.1"></a>The server SHALL bind to `localhost` by default, preventing access from other machines
2. <a name="6.2"></a>The server SHALL apply security headers on all responses: `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`
3. <a name="6.3"></a>The server SHALL apply a Content-Security-Policy header on HTML responses: `default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'`
4. <a name="6.4"></a>The server SHALL reject requests containing `..` in the URL path (after URL decoding)
5. <a name="6.5"></a>The server SHALL validate that transcript file paths resolve to locations within expected directories using symlink-safe path validation
6. <a name="6.6"></a>The server SHALL reuse the exported `SecurityHeaders` and `PathSanitizer` middleware from the `internal/web` package
7. <a name="6.7"></a>The `internal/web` package SHALL export `IsPathWithinDir` (currently unexported `isPathWithinDir`) for use by the apsis web package

---

### 7. Static Assets and Templates

**User Story:** As a developer, I want all web assets embedded in the binary, so that `apsis serve` works as a single binary with no external file dependencies.

**Acceptance Criteria:**

1. <a name="7.1"></a>All static assets (CSS, JavaScript) SHALL be embedded in the binary via Go's `embed` package
2. <a name="7.2"></a>All HTML templates SHALL be embedded in the binary via Go's `embed` package
3. <a name="7.3"></a>Static assets SHALL be served with `Cache-Control: public, max-age=86400` headers
4. <a name="7.4"></a>The CSS SHALL include a version query parameter for cache busting
5. <a name="7.5"></a>Transcript CSS SHALL be served from the `transcript` package via `transcript.TranscriptCSS()`, not duplicated
6. <a name="7.6"></a>The HTMX library SHALL be vendored in the static assets directory, using the same vendored version as orbit serve

---

### 8. Connection Resilience

**User Story:** As a developer, I want the web interface to gracefully handle connection issues, so that I know when auto-refresh has stopped working.

**Acceptance Criteria:**

1. <a name="8.1"></a>The interface SHALL track consecutive HTMX polling failures
2. <a name="8.2"></a>WHEN 3 or more consecutive polling requests fail, the interface SHALL display a "Connection lost" indicator
3. <a name="8.3"></a>WHEN a polling request succeeds after failures, the indicator SHALL be hidden
4. <a name="8.4"></a>Basic navigation (session list, transcript viewing) SHALL function without JavaScript via manual page refresh. A noscript notice SHALL explain that filtering, search, and auto-refresh require JavaScript.

---

## Out of Scope

- **Live session streaming** (SSE or WebSocket for real-time transcript updates)
- **Cross-project browsing** (showing sessions from multiple projects)
- **Full-text search** within transcript content
- **Auto-opening the browser** on server start
- **Changes to existing `apsis -l` output format** (document proposed improvements separately)
