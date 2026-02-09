# Decision Log: apsis-serve

## Decision 1: Project Scope - Current Project Only

**Date**: 2026-02-09
**Status**: accepted

### Context

The web interface could either show sessions for only the current project (matching `apsis -l` behaviour) or provide a global view across all projects (like `orbit serve` groups runs by repository).

### Decision

Scope the web interface to the current project directory only.

### Rationale

This matches the existing `apsis -l` behaviour, keeps the implementation simpler, and avoids the need for a project registry. Cross-project browsing can be added as a future enhancement.

### Alternatives Considered

- **All projects with grouping**: Show sessions from all projects grouped by project path - Rejected because it adds significant complexity (project discovery, registry) without a clear immediate need

### Consequences

**Positive:**
- Simpler implementation, fewer moving parts
- Consistent mental model with existing `apsis -l`

**Negative:**
- Users must restart the server or use `--project` flag to browse a different project

---

## Decision 2: Keep `apsis -l` Output Unchanged

**Date**: 2026-02-09
**Status**: accepted

### Context

The session extraction refactor provides an opportunity to change the `apsis -l` output format. The question is whether to make improvements now or keep it stable.

### Decision

Keep `apsis -l` output identical after the extraction. Document any proposed improvements separately for a future change.

### Rationale

Users and scripts may depend on the current output format. Mixing a refactor with behaviour changes increases risk. The extraction should be a pure refactor.

### Alternatives Considered

- **Allow improvements during extraction**: Change output format while refactoring - Rejected because it conflates two concerns and makes the refactor harder to verify
- **Deprecate and replace**: Add a new list subcommand with improved output - Rejected as unnecessary at this stage

### Consequences

**Positive:**
- Zero risk of breaking existing workflows
- Extraction can be verified by comparing output before/after

**Negative:**
- Known improvements to `apsis -l` are deferred

---

## Decision 3: Static Transcript Rendering for v1

**Date**: 2026-02-09
**Status**: accepted

### Context

The transcript view page could render static HTML (snapshot at page load), auto-refresh via HTMX polling, or stream live updates via Server-Sent Events. The `transcript.Follower` type already exists for live monitoring.

### Decision

Use static rendering only for v1. The transcript is rendered once when the page is loaded.

### Rationale

Static rendering is the simplest approach and covers the primary use case (reviewing completed sessions). Live streaming adds complexity (SSE endpoints, partial rendering, state management) that can be built later on the existing `Follower` infrastructure.

### Alternatives Considered

- **HTMX auto-refresh**: Poll and re-render every N seconds - Rejected for v1 because re-rendering large transcripts on every poll is wasteful
- **SSE live streaming**: Push new entries via Server-Sent Events - Rejected for v1 because it requires significant new infrastructure (SSE handler, incremental HTML rendering, client-side append logic)

### Consequences

**Positive:**
- Simple implementation reusing existing `RenderHTMLFragment()`
- No new real-time infrastructure needed

**Negative:**
- Viewing an active session requires manual page refresh to see new content

---

## Decision 4: Print URL Only on Startup

**Date**: 2026-02-09
**Status**: accepted

### Context

The server could automatically open the default browser when started, or simply print the URL and let the user open it manually.

### Decision

Print the listening URL to stderr on startup. Do not auto-open the browser.

### Rationale

Printing is simpler, has no platform-specific dependencies, and works in all environments (SSH sessions, containers, headless servers). Auto-open can be added later as an opt-in flag.

### Alternatives Considered

- **Auto-open browser**: Use `os/exec` with platform-specific commands (`open`, `xdg-open`, `start`) - Rejected because it adds platform-specific code and doesn't work in all environments

### Consequences

**Positive:**
- No platform-specific code
- Works in headless/remote environments

**Negative:**
- One extra step for the user to copy/click the URL

---

## Decision 5: Default Port 8081

**Date**: 2026-02-09
**Status**: accepted

### Context

The web server needs a default port. `orbit serve` defaults to 8080.

### Decision

Default to port 8081.

### Rationale

Using a different port than orbit serve (8080) allows both servers to run simultaneously without conflict. Port 8081 is the next logical choice.

### Alternatives Considered

- **Port 8080**: Same as orbit serve - Rejected because it would conflict if both are running
- **Port 3000**: Common dev server port - Rejected because it conflicts with many other dev tools

### Consequences

**Positive:**
- Can run alongside `orbit serve` without conflict

**Negative:**
- None significant

---

## Decision 6: Canonical Source Identifiers

**Date**: 2026-02-09
**Status**: accepted

### Context

The existing code uses `"kiro ide"` (with a space) as the source string in `SessionInfo`, but URL path parameters cannot contain spaces. The web interface needs URL-safe source identifiers.

### Decision

Define canonical source constants using hyphenated lowercase identifiers: `claude`, `codex`, `copilot`, `kiro-cli`, `kiro-ide`. Normalise `"kiro ide"` to `"kiro-ide"` internally. Preserve the original display format in `apsis -l` output via a display name mapping.

### Rationale

Hyphenated lowercase is URL-safe, consistent, and readable. A display name mapping keeps `apsis -l` output unchanged while the internal representation is clean.

### Alternatives Considered

- **URL-encode spaces**: Use `kiro%20ide` in URLs - Rejected because it's ugly and error-prone
- **Underscore**: Use `kiro_ide` - Rejected because hyphens are the conventional URL separator

### Consequences

**Positive:**
- Clean, consistent source identifiers throughout the codebase
- URL-safe without encoding

**Negative:**
- Requires a mapping layer between internal IDs and display names for `apsis -l` compatibility

---

## Decision 7: Separate `internal/apsisweb/` Package

**Date**: 2026-02-09
**Status**: accepted

### Context

The web server code could live in the existing `internal/web/` package alongside orbit's code, in a sub-package like `internal/web/apsis/`, or in a separate `internal/apsisweb/` package.

### Decision

Create a separate `internal/apsisweb/` package. Import orbit's exported middleware (`SecurityHeaders`, `PathSanitizer`, `IsPathWithinDir`) directly.

### Rationale

Orbit's `internal/web/` has package-level init that parses orbit-specific templates. Mixing apsis templates into the same package creates coupling. The shared code (middleware) is already exported and can be imported directly. Small unexported helpers like `stripPrefix` are ~20 lines and trivial to replicate.

### Alternatives Considered

- **Add to `internal/web/`**: Mix apsis handlers into orbit's web package - Rejected because it couples the two tools and complicates orbit's template init
- **Extract shared `internal/web/shared/`**: Refactor orbit's web code into shared + orbit-specific - Rejected because it's unnecessary refactoring of stable code for the sake of two packages

### Consequences

**Positive:**
- Clean separation of concerns
- No changes to orbit's web package beyond exporting `IsPathWithinDir`
- Each tool's web code is self-contained

**Negative:**
- Minor duplication of static file serving pattern (~20 lines)
- HTMX library vendored in two locations

---

## Decision 8: Transcript Size Guard

**Date**: 2026-02-09
**Status**: accepted

### Context

Transcript files can be tens or hundreds of megabytes. `RenderHTMLFragment()` builds the entire output in memory. Rendering a very large transcript could exhaust server memory and hang the browser.

### Decision

Add a 50MB file size check before rendering. Transcripts exceeding this limit return a user-friendly error page suggesting the CLI instead.

### Rationale

50MB is generous for normal usage (most sessions are well under 10MB) while protecting against pathological cases. The CLI can handle larger files via streaming output. This can be refined later if users hit the limit regularly.

### Alternatives Considered

- **No limit**: Trust the user to not open huge files - Rejected because a single large request could make the server unresponsive
- **Pagination / lazy loading**: Render in chunks - Rejected for v1 because it adds significant complexity for an edge case

### Consequences

**Positive:**
- Protects server memory and browser responsiveness
- Clear error message with actionable alternative

**Negative:**
- Users with very large sessions must use the CLI

---
