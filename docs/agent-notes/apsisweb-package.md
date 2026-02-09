# Apsis Web Package (internal/apsisweb/)

## Architecture

Separate package from `internal/web/` (orbit's web server) to avoid coupling. Imports shared middleware (`SecurityHeaders`, `PathSanitizer`, `IsPathWithinDir`) from `internal/web/`.

## Key Design Decisions

- **`New()` returns `(*Server, error)`** — unlike orbit's `web.New()` which returns `*Server`, because it creates `sessions.Lister` and `sessions.Resolver` which can fail.
- **Signal handling is NOT in Server** — unlike orbit's server which handles signals internally. Signal handling is in `serveCommand()` (cmd/apsis/main.go) to keep the Server type testable.
- **Template rendering uses per-page pre-parsed templates** — same pattern as orbit: `pageTemplates` map populated in `init()`, each page template includes layout.html.
- **`renderFragment()`** uses the sessions.html template to execute the `sessions_list` named block for HTMX polling.

## Transcript Parsing

The handler dispatches differently based on whether a cost path exists:
- With CostPath: `ParseJSONLWithFormat(reader, FormatKiroIDE, opts)` — explicitly specifies format to thread cost data
- Without CostPath: `Parse(reader)` — auto-detects format

This mirrors the pattern in `cmd/apsis/main.go`.

## Testing

Tests use `newTestServer(t)` helper which creates a real Server with temp project dir. The Lister/Resolver find no sessions in the temp dir, which is fine for testing handler routing, error pages, and template rendering.

Server lifecycle tests use ephemeral ports via `findAvailablePort()`.

## File Layout

```
internal/apsisweb/
  server.go          - Server struct, Config, routing, template init/rendering
  handlers.go        - Session list, transcript, error handlers, buildSessionListData
  middleware.go      - ValidateSource, SanitizeSessionID
  static.go          - Embedded static file handler, stripPrefix
  static/
    style.css        - CSS with variables, dark mode, agent badges, responsive
    htmx.min.js      - Vendored copy from internal/web/static/
  templates/
    layout.html      - Base layout with HTMX, connection indicator, noscript
    sessions.html    - Session list with filter/search JS, HTMX polling
    transcript.html  - Transcript viewer with metadata card
    error.html       - Error page with code and message
```
