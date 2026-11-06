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

## Transcript Parsing

The handler calls `sessions.ParseResolved(resolved)` with default options: it threads the Kiro IDE cost path, forces the Kiro IDE and codex-exec parsers for those sources and auto-detects the rest. The CLI uses the same helper (after its own `-a` dispatch) and adds the shared-JSON-only Codex flags; the web never sets them, so header-only Codex rollouts still fail with a 500 and model changes keep legacy grouping.

## Codex Exec Streams

`Config.CodexExecRoots` (`apsis serve --codex-exec-dir`, or `APSIS_CODEX_EXEC_DIRS`) goes to both the Lister and the Resolver. The filter pills come from `lister.Sources()`, so the `codex-exec` pill appears only with roots (Q46). Exec rows set `SessionView.FileTime`; the template then wraps the `<time>` and a `file modified` label in a `.session-date` span. The label must stay outside `<time>` because the layout script replaces `<time>` text (Q117). Exec transcript views add `TranscriptViewData.ThreadID`, `FileTime` and `Warnings` (`line N: message`); other sources never set them (Q47). `.source-codex-exec` has light and dark badge rules; `CSSVersion` is 3.

The listing warning banner lives inside the `sessions_list` fragment, so HTMX polls refresh it and the full page shows it once (Q54).

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

## Shutdown Flake (Fixed)

`TestIntegrationEndToEnd` used to fail with `shutdown error: context deadline exceeded` (4 of 6 runs on unmodified upstream `7259a6c`). Cause: `http.Server.Shutdown` treats a connection in `StateNew` (dialled, no request yet) as active for 5 seconds, and `http.DefaultClient` can dial a spare keep-alive connection it never uses. The test now calls `http.DefaultClient.CloseIdleConnections()` before `Shutdown` (Q119). Any new test that shuts down a real server after `http.Get` calls needs the same step.
