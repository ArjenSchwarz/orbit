# Apsis Web Package (internal/apsisweb/)

## Architecture

Separate package from `internal/web/` (orbit's web server) to avoid coupling. Imports shared middleware (`SecurityHeaders`, `PathSanitizer`, `IsPathWithinDir`) from `internal/web/`.

## Key Design Decisions

- **`New()` returns `(*Server, error)`** — unlike orbit's `web.New()` which returns `*Server`, because it creates `sessions.Lister` and `sessions.Resolver` which can fail.
- **Signal handling is NOT in Server** — unlike orbit's server which handles signals internally. Signal handling is in `serveCommand()` (cmd/apsis/main.go) to keep the Server type testable.
- **Template rendering uses per-page pre-parsed templates** — same pattern as orbit: `pageTemplates` map populated in `init()`, each page template includes layout.html.
- **`renderFragment()`** uses the sessions.html template to execute the `sessions_list` named block for HTMX polling.

## All-Projects Scope

`Config.AllProjects` (`apsis serve --all-projects`, a usage error with `--project`) clears `ProjectPath`, so the Lister lists and the Resolver opens with `""`, and both get `WithExtendedOrdering(true)`. Only then does `buildSessionListData` set `SessionView.Project` and `SessionListData.AllProjects`; the `sessions_list` template adds a `.session-project` cell (`-` when unrecorded) and the `all-projects` list class that widens the card grid. Project-scope pages render exactly as before. `Server.Handler()` exposes the router for `httptest` (used by the CLI round-trip test in `cmd/apsis/all_projects_test.go`).

`TestIntegrationEndToEnd` intermittently fails on shutdown (`context deadline exceeded`), also on unmodified upstream; rerun before treating it as a regression.

## Transcript Parsing

The handler dispatches differently based on whether a cost path exists:
- With CostPath: `ParseJSONLWithFormat(reader, FormatKiroIDE, opts)` — explicitly specifies format to thread cost data
- Without CostPath: `Parse(reader)` — auto-detects format

This mirrors the pattern in `cmd/apsis/main.go`.

## Testing

Tests use `newTestServer(t)` helper which creates a real Server with temp project dir. The Lister/Resolver find no sessions in the temp dir, which is fine for testing handler routing, error pages, and template rendering.

Server lifecycle tests use ephemeral ports via `findAvailablePort()`.

### Integration Test

`integration_test.go` uses `t.Setenv("HOME", tmpDir)` to redirect the Lister/Resolver to find mock sessions in a controlled temp directory. This avoids needing dependency injection while testing the full HTTP flow.

Mock Claude sessions must use `"type": "user"` and `"type": "assistant"` (not `"system"` or `"human"`) — the transcript parser's format detection relies on these exact type values. The `message` field must be an object with `role` and `content` array, not a plain string.

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

## Known Flaky Test

- `TestIntegrationEndToEnd` often fails with `shutdown error: context deadline exceeded` after its 5-second `srv.Shutdown`. On 2026-11-05 it failed 4 of 6 runs on an unmodified export of upstream `7259a6c`, so it predates the shared JSON work. Re-run `make test` to get a clean pass, and do not treat this failure as caused by an unrelated change. The cause has not been investigated.
