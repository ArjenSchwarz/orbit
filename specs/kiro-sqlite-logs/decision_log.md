# Decision Log: Kiro SQLite Log Parsing

## Decision 1: Replace Export Mechanism Entirely

**Date**: 2026-01-30
**Status**: accepted

### Context

The current Kiro agent implementation uses a workaround to retrieve session logs: after each phase completes, it executes `/chat save` via the CLI to export the session to a JSON file. This is documented as a "hack" in the multi-agent decision log. Now that we know Kiro stores all conversations in an SQLite database, we need to decide whether to replace this mechanism entirely or maintain both approaches.

### Decision

Replace the `/chat save` export mechanism entirely with direct SQLite database access.

### Rationale

The SQLite database is Kiro's native storage and provides direct access to conversation data without requiring additional CLI invocations. This eliminates the complexity of post-phase exports, reduces potential failure points, and aligns with how other agents (Claude Code, Codex) store sessions in accessible locations.

### Alternatives Considered

- **Coexist with both mechanisms**: Keep SQLite for reads, export as fallback - Rejected because it adds complexity and the export mechanism has no advantages when SQLite is available
- **SQLite preferred with export fallback**: Use SQLite when available, fall back to export if DB missing - Rejected because if SQLite is missing, something is wrong with the Kiro installation and export would likely fail too

### Consequences

**Positive:**
- Simpler implementation with single source of truth
- No need to maintain export code paths
- Faster session access (no CLI invocation needed)
- Matches how other agents work

**Negative:**
- No backwards compatibility with existing exported session files (accepted as requirement)
- Dependency on Kiro's internal database schema (could break with Kiro updates)

---

## Decision 2: Full Apsis Support for Kiro Sessions

**Date**: 2026-01-30
**Status**: accepted

### Context

Apsis currently supports listing and converting sessions for Claude Code and Codex. With SQLite access to Kiro sessions, we need to decide the level of Apsis integration.

### Decision

Provide full Apsis support for Kiro sessions, including listing with `apsis -l` and conversion by session ID.

### Rationale

Consistency across agents improves user experience. Developers should be able to use the same Apsis commands regardless of which agent produced the transcript. The existing transcript parsing infrastructure already handles Kiro's JSON format.

### Alternatives Considered

- **Convert only**: Allow conversion but no listing - Rejected because it creates inconsistent UX between agents
- **Orbit only**: Only use SQLite in Orbit, leave Apsis unchanged - Rejected because it limits the utility of the SQLite integration

### Consequences

**Positive:**
- Consistent user experience across all agents
- Kiro sessions are discoverable and accessible via Apsis
- Full feature parity with Claude Code and Codex

**Negative:**
- Slightly more complex Apsis implementation
- Need to handle cases where Kiro database is unavailable

---

## Decision 3: Auto-detect OS for Database Path

**Date**: 2026-01-30
**Status**: accepted

### Context

Kiro stores its SQLite database in different locations depending on the operating system. We need to decide how to resolve the correct path.

### Decision

Auto-detect the operating system at runtime and use the standard path for that OS. No configuration override will be provided.

### Rationale

Simplicity is preferred. The standard paths are well-defined and users shouldn't need to override them in normal usage. Adding configuration options increases complexity without clear benefit.

### Alternatives Considered

- **Auto-detect with KIRO_DB_PATH override**: Allow environment variable override - Rejected as unnecessary complexity for unlikely use cases
- **Full configuration support**: Support via .orbit.yaml and env vars - Rejected as over-engineering

### Consequences

**Positive:**
- Simple implementation
- No configuration burden on users
- Works out of the box for standard Kiro installations

**Negative:**
- Cannot handle non-standard Kiro installations
- If Kiro changes default paths, code changes required

---

## Decision 4: Filter Sessions by Current Working Directory

**Date**: 2026-01-30
**Status**: accepted

### Context

The SQLite database contains sessions from all projects (the `key` column is the working directory path). When listing or discovering sessions, we need to decide how to handle multiple projects.

### Decision

Filter sessions to only show those matching the current working directory.

### Rationale

This matches the behavior of Claude Code and Codex session discovery, which are project-scoped. Users typically want to see sessions relevant to their current project, not all sessions across all projects.

### Alternatives Considered

- **Show all with grouping**: List all sessions grouped by project - Rejected as it changes the UX model and could be overwhelming
- **Both modes with --all flag**: Default to current dir, --all shows everything - Rejected as adding complexity for minimal benefit

### Consequences

**Positive:**
- Consistent with other agent behaviors
- Focused results relevant to current work
- Simpler implementation

**Negative:**
- Cannot easily see sessions from other projects
- Must change directory to access different project's sessions

---

## Decision 5: No Backwards Compatibility with Exported Sessions

**Date**: 2026-01-30
**Status**: accepted

### Context

Existing Orbit runs with Kiro may have exported session JSON files in `.orbit/` directories. We need to decide whether to maintain compatibility with these files.

### Decision

Do not maintain backwards compatibility with existing exported session files. Only use SQLite going forward.

### Rationale

The exported files were a workaround, not a supported feature. The SQLite database contains the authoritative data. Maintaining two code paths for reading sessions adds complexity without significant benefit, as the export mechanism was primarily for Orbit's internal use rather than user-facing session storage.

### Alternatives Considered

- **Read existing exports**: Continue reading old JSON files if they exist - Rejected as it maintains legacy code paths
- **Migration tool**: Provide tool to import old exports - Rejected as the data already exists in SQLite

### Consequences

**Positive:**
- Simpler implementation with single code path
- Clean break from legacy workaround
- No technical debt from export mechanism

**Negative:**
- Old exported files become orphaned (but data is in SQLite)
- Cannot replay old sessions if SQLite was cleared

---

## Decision 6: Path Normalization with Symlink Fallback

**Date**: 2026-01-30
**Status**: accepted

### Context

Session discovery requires matching the user's current working directory against the `key` column in the database. Paths can vary due to symlinks, relative paths, and trailing slashes.

### Decision

Normalize paths using `filepath.Abs` + `filepath.Clean`, then attempt a second lookup using symlink-resolved path (via `filepath.EvalSymlinks`) if it differs from the normalized path. When the same session is found via both paths, keep the one with the most recent `UpdatedAt`.

### Rationale

This handles both direct paths and symlinked project directories without requiring users to know which path Kiro recorded. Trying both ensures maximum compatibility while maintaining reasonable performance (at most 2 queries). Keeping the most recent ensures we don't discard updated session metadata.

### Alternatives Considered

- **Normalize only**: Use filepath.Abs + filepath.Clean, ignore symlinks - Rejected because users may access projects via symlinks
- **Exact match**: Use working directory as-is - Rejected because path variations would cause missed matches
- **First-match-wins dedup**: Keep first discovered match - Rejected because it may discard more recent metadata

### Consequences

**Positive:**
- Handles common path variations robustly
- Works transparently for users regardless of how they navigate to projects
- Always returns most up-to-date session metadata

**Negative:**
- May perform two queries when symlink resolution produces different path

---

## Decision 7: Session ID Lookup Scoped to Current Directory

**Date**: 2026-01-30
**Status**: accepted

### Context

When a user runs `apsis <session-id>`, should it search all directories in the database or only the current working directory?

### Decision

Session ID lookup requires being in the correct project directory, consistent with session listing behavior.

### Rationale

This maintains consistency with how session listing works and matches the behavior of Claude Code and Codex session resolution. Users must be in the project directory to see or access its sessions.

### Alternatives Considered

- **Search all directories**: Find session anywhere in database - Rejected for consistency with listing behavior

### Consequences

**Positive:**
- Consistent behavior between listing and lookup
- Clear mental model: project sessions require being in project directory
- Matches other agent behaviors

**Negative:**
- Less convenient when user knows session ID but is in wrong directory
- No way to access sessions from other projects without changing directory

---

## Decision 8: 5-Second Busy Timeout for Concurrent Access

**Date**: 2026-01-30
**Status**: accepted

### Context

Kiro CLI may be actively writing to the SQLite database while Orbit or Apsis attempts to read. SQLite locks can cause read failures.

### Decision

Configure a 5-second busy timeout to handle concurrent database access gracefully.

### Rationale

5 seconds is long enough to wait out typical write operations but short enough to fail reasonably fast if something is truly stuck. This is a standard practice for SQLite concurrent access.

### Alternatives Considered

- **No timeout**: Fail immediately if database is locked - Rejected because it would cause spurious failures during active Kiro sessions

### Consequences

**Positive:**
- Resilient to concurrent Kiro writes
- Standard SQLite best practice
- Transparent to users in normal operation

**Negative:**
- May delay operations by up to 5 seconds in worst case
- Does not solve fundamental concurrent access issues, only mitigates them

---

## Decision 9: Connection-Per-Operation Pattern

**Date**: 2026-01-30
**Status**: accepted

### Context

Database connections need lifecycle management. Options include connection pooling, singleton connection, or opening/closing per operation.

### Decision

Open a new database connection for each public API call (`DiscoverForDirectory`, `GetSession`), and close it before returning.

### Rationale

- Operations are infrequent (listing sessions, fetching transcripts)
- Connection overhead is minimal for read-only SQLite access
- Eliminates resource leak concerns
- Simplifies implementation (no pool management)
- `GetSession` reads blob into memory before returning, enabling clean connection closure

### Alternatives Considered

- **Connection pool**: Maintain pool of connections - Rejected as over-engineering for low-frequency operations
- **Singleton connection**: Single shared connection - Rejected due to lifecycle complexity and testing difficulties

### Consequences

**Positive:**
- No resource leaks possible
- Clean, predictable behavior
- Easy to test

**Negative:**
- Small overhead per operation (connection setup, schema verification)
- Acceptable given operation frequency

---

## Decision 10: Read JSON Blob Fully Into Memory

**Date**: 2026-01-30
**Status**: accepted

### Context

`GetSession` returns JSON data to be parsed. The design could stream data or load it entirely into memory.

### Decision

Read the entire JSON blob into memory and return a `bytes.Reader`.

### Rationale

- SQLite TEXT columns are not streamable - entire value must be read
- JSON blobs are typically 100KB-1MB (observed from real data)
- Memory usage is bounded and predictable
- Allows connection to close before returning
- `bytes.Reader` satisfies `io.Reader` interface for parser compatibility

### Alternatives Considered

- **Stream from open connection**: Keep connection open while caller reads - Rejected because it creates resource management burden on caller and potential leaks

### Consequences

**Positive:**
- Clean connection lifecycle
- Simple implementation
- Compatible with existing parser interface

**Negative:**
- Memory spike for large sessions (acceptable given typical sizes)

---

## Decision 11: DB Struct for Dependency Injection

**Date**: 2026-01-30
**Status**: accepted

### Context

Unit tests need to use test databases rather than the real Kiro database. The design needs a way to inject test paths.

### Decision

Create a `DB` struct that holds the database path, with `DefaultDB()` for production and `NewTestDB(path)` for testing.

### Rationale

- Enables test isolation without environment variable manipulation
- Keeps public API simple (convenience functions use `DefaultDB()`)
- Explicit dependency makes testing straightforward
- No global state or singletons

### Alternatives Considered

- **Environment variable**: Override path via env var - Rejected because it's less explicit and harder to isolate in parallel tests
- **Interface abstraction**: Define database interface - Rejected as over-abstraction for single implementation

### Consequences

**Positive:**
- Clean test isolation
- Explicit dependencies
- Easy to mock or stub for different scenarios

**Negative:**
- Slightly more complex API (two ways to access: convenience functions vs DB methods)

---

## Decision 12: Use modernc.org/sqlite (Pure Go)

**Date**: 2026-01-30
**Status**: accepted

### Context

Need to choose an SQLite driver for Go. Main options are `mattn/go-sqlite3` (CGO) and `modernc.org/sqlite` (pure Go).

### Decision

Use `modernc.org/sqlite` pure Go driver.

### Rationale

- Orbit is currently CGO-free; adding CGO would complicate builds
- CGO complicates cross-compilation and CI pipelines
- Pure Go enables single-binary distribution
- Performance difference is negligible for our use case (reading small blobs)

### Alternatives Considered

- **mattn/go-sqlite3**: More mature, faster - Rejected because it requires CGO
- **ncruces/go-sqlite3**: WebAssembly-based - Rejected as less mature than modernc

### Consequences

**Positive:**
- Maintains CGO-free build
- Simpler CI/CD
- Single binary distribution

**Negative:**
- Slightly larger binary (~5-10MB)
- Marginally slower (not significant for this use case)

---
