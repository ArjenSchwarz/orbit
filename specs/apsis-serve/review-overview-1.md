# PR Review Overview - Iteration 1

**PR**: #62 | **Branch**: feature/apsis-serve | **Date**: 2026-02-09

## Valid Issues

### Issue 1: TestHandleTranscriptOversized tests the wrong thing
- **Type**: PR-level review comment
- **Reviewer**: @claude
- **Comment**: "This test claims to test oversized file handling but actually tests 404 handling instead. The comment admits it can't test what it claims to test."
- **Validation**: Valid. The test name says "Oversized" but calls `handleNotFound` and checks for 404. It's a duplicate of the 404 test and violates CLAUDE.md testing guidelines. Should either be fixed to actually test the 50MB guard or removed.

### Issue 2: Missing length limit on session ID validation
- **Type**: PR-level review comment
- **Reviewer**: @claude
- **Comment**: "While middleware validates session IDs, there's no length limits. UUIDs are 36 chars, but the code accepts arbitrary length strings."
- **Validation**: Valid. Adding a reasonable max length (256 chars) to `SanitizeSessionID` is a quick, sensible guard. Session IDs are UUIDs (36 chars) or similar short identifiers.

## Invalid/Skipped Issues

### Issue A: XSS via template.HTML
- **Location**: `internal/apsisweb/handlers.go:90`
- **Reviewer**: @claude
- **Comment**: "The transcript content from transcript.RenderHTMLFragment() is marked as template.HTML, which bypasses Go's auto-escaping."
- **Reason**: By design. `RenderHTMLFragment` produces HTML output — that's its purpose. It already uses `html.EscapeString` on all user-controlled content. Wrapping known-safe HTML in `template.HTML` is standard Go practice.

### Issue B: Symlink following in session listing
- **Location**: `internal/sessions/lister.go:480-526`
- **Reviewer**: @claude
- **Comment**: "An attacker could create symlinks pointing outside the expected directories."
- **Reason**: Intentional. Codex sessions use symlinks by design (the codex sessions directory itself is often a symlink). Cycle detection is already present. Adding `IsPathWithinDir` would break legitimate Codex session discovery. The lister only reads session metadata, never executes content.

### Issue C: Memory usage for large .chat files
- **Location**: `internal/sessions/lister.go` (Kiro IDE listing)
- **Reviewer**: @claude
- **Comment**: "os.ReadFile() loads entire file into memory. For Kiro IDE sessions, this happens for every .chat file during discovery."
- **Reason**: Over-engineering for this context. Kiro IDE .chat files are typically small JSON files (< 1MB). The 50MB guard is for transcript rendering, not listing. Adding streaming JSON parsing for header extraction adds significant complexity for a negligible benefit in a local dev tool.

### Issue D: No rate limiting on session listing
- **Location**: `internal/apsisweb/handlers.go:105-144`
- **Reviewer**: @claude
- **Comment**: "Multiple clients could DoS the server."
- **Reason**: This is a local development tool running on localhost. Rate limiting adds unnecessary complexity. If someone is DoS-ing their own machine, that's not our problem.

### Issue E: Silent failure on template rendering
- **Location**: `internal/apsisweb/server.go:177-179`
- **Reviewer**: @claude
- **Comment**: "Template execution errors are only logged, not returned to caller."
- **Reason**: At the point `ExecuteTemplate` is called, HTTP headers have already been sent. You can't reliably change the response status. Logging is the correct approach. The suggested fix using `Written()` is not a standard `http.ResponseWriter` method.

### Issue F: CSP too permissive
- **Location**: `internal/web/middleware.go:46`
- **Reviewer**: @claude
- **Comment**: "'unsafe-inline' allows inline scripts and styles, which weakens XSS protections."
- **Reason**: Required for HTMX and inline scripts/styles. Moving to nonces or external files would add significant complexity for marginal benefit on a localhost-only tool. Follow-up material at best.

### Issue G: Codex usage limit notice
- **Type**: PR-level comment (bot noise)
- **Reviewer**: @chatgpt-codex-connector
- **Reason**: Automated bot message about Codex usage limits. Not actionable.
