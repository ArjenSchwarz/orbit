# Decision Log: Web Interface

## Decision 1: Historical Run Discovery Approach

**Date**: 2025-01-05
**Status**: accepted

### Context

The web interface needs to display runs that were created before the auto-registration feature existed. There are two approaches: automatically scan the filesystem for existing `.orbit/` directories, or require users to manually register historical runs.

### Decision

Require manual registration using `orbit register` for historical runs. Only runs started after this feature is implemented will be auto-registered.

### Rationale

Manual registration is simpler to implement and avoids the complexity of filesystem scanning. It also gives users explicit control over which runs appear in the registry. Auto-discovery could surface unwanted or obsolete log directories.

### Alternatives Considered

- **Auto-discover .orbit directories**: Scan common locations for existing log directories - Rejected because it adds significant implementation complexity and could surface unwanted directories

### Consequences

**Positive:**
- Simpler implementation
- Users have explicit control over registered runs
- No risk of surfacing unwanted or obsolete logs

**Negative:**
- Historical runs require manual action to appear in web interface
- Users may need to run `orbit register` multiple times for multiple projects

---

## Decision 2: htmx Inclusion Strategy

**Date**: 2025-01-05
**Status**: accepted

### Context

The web interface uses htmx for dynamic updates. htmx can either be loaded from a CDN or embedded directly in the binary.

### Decision

Embed htmx directly in the binary using Go's embed package.

### Rationale

Embedding htmx ensures the web interface works without internet access and keeps the deployment as a single binary. The ~14KB size impact is negligible compared to the benefit of zero external dependencies.

### Alternatives Considered

- **CDN link**: Load htmx from a CDN at runtime - Rejected because it requires internet access and creates an external dependency

### Consequences

**Positive:**
- Works offline and in air-gapped environments
- Single binary deployment
- No external dependencies
- Consistent version across all deployments

**Negative:**
- Slightly larger binary size (~14KB)
- Updating htmx requires rebuilding the binary

---

## Decision 3: Missing Log Directory Handling

**Date**: 2025-01-05
**Status**: accepted

### Context

Registry entries may point to log directories that no longer exist (deleted, moved, or on unmounted drives). The system needs to handle this gracefully.

### Decision

Display runs with missing log directories in an error state, showing an error indicator. Do not automatically remove them from the registry.

### Rationale

Automatic removal could cause confusion if the directory is temporarily unavailable (e.g., unmounted network drive). Showing an error state is informative and allows users to decide whether to remove the entry or fix the underlying issue.

### Alternatives Considered

- **Auto-remove stale entries**: Automatically delete registry entries when log directories are missing - Rejected because it could remove entries that are only temporarily unavailable

### Consequences

**Positive:**
- Users can see something is wrong
- Temporarily unavailable directories won't lose their registry entries
- Users can manually clean up when appropriate

**Negative:**
- Registry may accumulate stale entries over time
- No automatic cleanup mechanism

---

## Decision 4: Write Operations in v1

**Date**: 2025-01-05
**Status**: accepted

### Context

The web interface could support write operations like deleting runs from the registry or stopping running processes. However, this adds complexity and security considerations.

### Decision

The web interface will be strictly read-only in v1. No write operations will be exposed.

### Rationale

Read-only access is simpler to implement and secure. Write operations can be added in a future version once authentication is implemented. Users can still manage the registry using CLI commands.

### Alternatives Considered

- **Allow run deletion**: Enable removing runs from registry via web UI - Rejected for v1 to keep the interface simple and secure

### Consequences

**Positive:**
- Simpler implementation
- Reduced security attack surface
- No risk of accidental data loss via web interface

**Negative:**
- Users must use CLI for any management operations
- Cannot remove completed runs from the dashboard

---

## Decision 5: Minimum Mobile Viewport Width

**Date**: 2025-01-05
**Status**: accepted

### Context

Mobile responsiveness requires defining a minimum viewport width to support. This affects layout decisions and testing requirements.

### Decision

Support viewports as narrow as 320px (iPhone SE width).

### Rationale

Supporting 320px ensures compatibility with the smallest commonly-used smartphones. While most modern phones have larger screens, there's no significant implementation overhead to support this width, and it provides better accessibility.

### Alternatives Considered

- **375px (iPhone 12/13 width)**: Modern smartphone minimum - Rejected because 320px support requires minimal additional effort and improves accessibility

### Consequences

**Positive:**
- Maximum device compatibility
- Better accessibility
- Works on older/smaller devices

**Negative:**
- May require more careful layout design for narrow widths
- Testing needs to include 320px viewport

---

## Decision 6: Transcript Rendering Approach

**Date**: 2025-01-05
**Status**: accepted

### Context

The transcript viewer needs to render session transcripts. The codebase already has HTML rendering capabilities in `internal/transcript/html.go` used by the apsis tool.

### Decision

Reuse the existing HTML rendering from `internal/transcript/html.go` for transcript display in the web interface.

### Rationale

Reusing existing code ensures consistency between the apsis HTML export and the web interface. It reduces duplication and maintenance burden. The existing renderer already handles tool calls, outputs, and message formatting.

### Alternatives Considered

- **New web-specific templates**: Create separate templates optimized for web display - Rejected because it duplicates effort and could lead to inconsistent rendering

### Consequences

**Positive:**
- Consistent transcript rendering across tools
- Less code to maintain
- Proven rendering logic

**Negative:**
- Web-specific optimizations may require modifying shared code
- Changes to apsis rendering affect web interface

---

## Decision 7: Defer REST API to v2

**Date**: 2025-01-05
**Status**: accepted

### Context

The original PLAN.md included REST API endpoints (`/api/runs`, `/api/runs/:id/summary`, etc.) for programmatic access. During requirements review, both the design-critic and peer-review-validator agents questioned whether this was necessary for v1, given that the primary use case is the web UI with htmx.

### Decision

Defer the REST API to a future version. The v1 implementation will focus on the htmx-based web interface only.

### Rationale

The web UI using htmx provides all necessary functionality for the primary use case (remote monitoring). Adding a REST API increases implementation scope, testing burden, and attack surface without clear v1 value. The API can be added in v2 when there's demonstrated need for programmatic access.

### Alternatives Considered

- **Keep full API**: Implement all API endpoints as originally planned - Rejected because it adds scope without clear v1 value
- **Minimal API**: Implement only `/api/runs` list endpoint - Rejected because even a minimal API requires schema definition and testing

### Consequences

**Positive:**
- Reduced implementation scope for v1
- Smaller attack surface
- Focus on core functionality

**Negative:**
- No programmatic access in v1
- Scripts/integrations will need to wait for v2

---

## Decision 8: Registry Schema Versioning

**Date**: 2025-01-05
**Status**: accepted

### Context

The run registry uses JSON files to store run metadata. Without schema versioning, future changes to the registry format would be difficult to handle, potentially breaking existing registry files.

### Decision

Include a `schema_version` field in every registry entry, starting at version 1.

### Rationale

Adding schema versioning from the start allows future migrations to be handled gracefully. The implementation can check the version and apply appropriate migrations or compatibility logic when reading older entries.

### Alternatives Considered

- **No versioning**: Handle schema changes ad-hoc when they occur - Rejected because it makes migrations error-prone and unpredictable
- **Version in directory structure**: Use versioned subdirectories like `~/.orbit/runs/v1/` - Rejected because it complicates file discovery

### Consequences

**Positive:**
- Future schema changes can be handled gracefully
- Clear migration path for existing data
- Implementation can support multiple versions simultaneously

**Negative:**
- Slight overhead in every registry file (one extra field)
- Must maintain migration logic as schema evolves

---

## Decision 9: Per-Phase Status Tracking

**Date**: 2025-01-05
**Status**: accepted

### Context

The UI requirements specify displaying per-phase status (pending, running, completed, failed). The existing `Summary` struct only tracks `PhasesCompleted` as a single integer, which doesn't capture individual phase states.

### Decision

Add a `phases` array to the registry entry containing per-phase status information. Each phase object includes: number, status, and run_count.

### Rationale

Per-phase tracking enables the UI to show detailed progress during a run. It also allows showing which specific phase failed rather than just that the run failed. The data structure supports multiple runs per phase (retries).

### Alternatives Considered

- **Derive from files**: Infer phase status from existence of session files - Rejected because it can't distinguish pending from not-started, and doesn't capture "running" state
- **Extend Summary struct**: Add phase array to summary.json - Rejected because summary.json is a snapshot, not real-time status

### Consequences

**Positive:**
- UI can show granular phase progress
- Clear visibility into which phase is running or failed
- Supports retry tracking via run_count

**Negative:**
- Additional registry writes during phase transitions
- More complex data model

---

## Decision 10: Security Headers

**Date**: 2025-01-05
**Status**: accepted

### Context

The web interface serves log content that may contain sensitive information. Security headers help protect against common web vulnerabilities like XSS and clickjacking.

### Decision

Set the following security headers on all responses:
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `Referrer-Policy: no-referrer`
- `Content-Security-Policy: default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'`

### Rationale

These headers are standard security best practices for web applications. Even though the interface is typically localhost-only, users may bind to network interfaces in some scenarios. The CSP allows inline scripts/styles for htmx and embedded CSS while blocking external resources.

### Alternatives Considered

- **No security headers**: Rely on localhost-only binding for security - Rejected because users can bind to network interfaces, and defense in depth is best practice
- **Strict CSP (no inline)**: Require nonces for all scripts - Rejected because htmx attributes would require significant workarounds

### Consequences

**Positive:**
- Protection against XSS and clickjacking
- Defense in depth for network-exposed deployments
- Standard security best practices followed

**Negative:**
- CSP may need adjustment if transcript content includes inline resources
- `unsafe-inline` is required for htmx attributes

---
