# Web Interface Requirements

## Introduction

This feature adds a web-based interface to Orbit for monitoring runs and browsing logs remotely. Users can view in-progress and completed orchestration runs, browse session transcripts, and access summaries from any device with a web browser, including mobile phones.

The interface consists of:
- An `orbit serve` command that starts a local web server
- An `orbit register` command for manually adding existing log directories to the registry
- A run registry system that tracks known runs in `~/.orbit/runs/`
- Auto-registration of runs when `orbit run` executes

The web interface is read-only and designed for monitoring, not control.

---

## Requirements

### 1. Web Server Command

**User Story:** As a user, I want to start a web server with a simple command, so that I can access Orbit logs from my browser without installing additional tools.

**Acceptance Criteria:**

1. <a name="1.1"></a>The system SHALL provide an `orbit serve` subcommand that starts an HTTP server
2. <a name="1.2"></a>The server SHALL accept a `--port` flag with a default value of 8080
3. <a name="1.3"></a>The server SHALL accept a `--bind` flag with a default value of "localhost"
4. <a name="1.4"></a>The server SHALL read `serve-port` and `serve-bind` from `.orbit.yaml` configuration files
5. <a name="1.5"></a>The configuration priority SHALL be: CLI flags > environment variables (`ORBIT_SERVE_PORT`, `ORBIT_SERVE_BIND`) > config file > defaults
6. <a name="1.6"></a>The server SHALL display the URL where it is accessible when started
7. <a name="1.7"></a>WHEN SIGINT or SIGTERM is received, the server SHALL complete in-flight requests (max 5 seconds) before shutting down
8. <a name="1.8"></a>IF the specified port is already in use, THEN the server SHALL exit with error message "port {port} is already in use"
9. <a name="1.9"></a>IF the server cannot bind to the specified address, THEN it SHALL exit with a descriptive error message

### 2. Run Registry System

**User Story:** As a user, I want Orbit to maintain a registry of all known runs, so that the web interface can discover and display them without manual configuration.

**Acceptance Criteria:**

1. <a name="2.1"></a>The system SHALL store run metadata as individual JSON files in `~/.orbit/runs/`
2. <a name="2.2"></a>Each registry file SHALL use the run's UUID v4 as its filename with `.json` extension
3. <a name="2.3"></a>Each registry entry SHALL contain the following required fields: id (UUID v4), name (string), repository (string), log_dir (string), status (enum), started_at (RFC3339 timestamp), branch (string)
4. <a name="2.4"></a>Each registry entry SHALL contain the following optional fields: pid (integer, null for manual registrations), finished_at (RFC3339 timestamp, null if not completed), phases (array of phase objects)
5. <a name="2.5"></a>Each registry entry SHALL contain a schema_version field with initial value of 1
6. <a name="2.6"></a>The status field SHALL be one of: "running", "completed", "failed"
7. <a name="2.7"></a>Each phase object in the phases array SHALL contain: number (integer), status (enum: "pending", "running", "completed", "failed"), run_count (integer)
8. <a name="2.8"></a>The repository field SHALL be derived from the git remote origin URL when available
9. <a name="2.9"></a>The system SHALL parse git URLs in HTTPS format (`https://github.com/owner/repo.git`) and SSH format (`git@github.com:owner/repo.git`) to extract `owner/repo`
10. <a name="2.10"></a>IF git URL parsing fails, THEN the system SHALL use the working directory name as the repository value
11. <a name="2.11"></a>The system SHALL use atomic file operations (write to temp file, then rename) for registry updates
12. <a name="2.12"></a>IF a registry JSON file is malformed, THEN the system SHALL skip that entry and log a warning
13. <a name="2.13"></a>IF the `~/.orbit/runs/` directory does not exist, THEN the system SHALL create it on first registry write

### 3. Auto-Registration

**User Story:** As a user, I want runs to be automatically registered when I start them, so that they appear in the web interface without manual intervention.

**Acceptance Criteria:**

1. <a name="3.1"></a>WHEN `orbit run` starts (after initializing the log directory), the system SHALL create a registry entry with status "running"
2. <a name="3.2"></a>WHEN `orbit run` completes successfully, the system SHALL update the registry entry: status to "completed", finished_at to current timestamp
3. <a name="3.3"></a>WHEN `orbit run` fails, the system SHALL update the registry entry: status to "failed", finished_at to current timestamp
4. <a name="3.4"></a>The registry entry SHALL include the process ID (pid) of the running orbit process
5. <a name="3.5"></a>WHEN a phase starts, the system SHALL update the phases array with the phase status set to "running"
6. <a name="3.6"></a>WHEN a phase completes, the system SHALL update the phases array with the phase status set to "completed" or "failed"
7. <a name="3.7"></a>IF registration fails, THEN `orbit run` SHALL log a warning to stderr and continue execution

### 4. Manual Registration Command

**User Story:** As a user, I want to manually register existing log directories, so that I can view historical runs that were created before the web interface existed.

**Acceptance Criteria:**

1. <a name="4.1"></a>The system SHALL provide an `orbit register` subcommand
2. <a name="4.2"></a>The command SHALL accept a path argument pointing to an orbit log directory
3. <a name="4.3"></a>WHEN the path is ".", the system SHALL look for `.orbit/` in the current directory
4. <a name="4.4"></a>The command SHALL accept an optional `--name` flag to set a display name
5. <a name="4.5"></a>A directory SHALL be considered valid orbit logs IF it contains at least one `phase-*-run-*-session.json` file
6. <a name="4.6"></a>IF the specified path does not contain valid orbit logs, THEN the command SHALL exit with error message "no valid orbit logs found in {path}"
7. <a name="4.7"></a>IF a run with the same log_dir already exists in the registry, THEN the command SHALL update the existing entry, preserving its id
8. <a name="4.8"></a>IF summary.json exists and contains `"status": "success"`, THEN the registered run status SHALL be "completed"
9. <a name="4.9"></a>IF summary.json exists and contains `"status": "failed"`, THEN the registered run status SHALL be "failed"
10. <a name="4.10"></a>IF summary.json does not exist or has no status, THEN the registered run status SHALL be "completed" (assumed historical run)
11. <a name="4.11"></a>The pid field SHALL be null for manually registered runs
12. <a name="4.12"></a>The phases array SHALL be derived by scanning for `phase-N-run-M-session.json` files in the log directory

### 5. Dashboard View

**User Story:** As a user, I want to see a list of all my Orbit runs at a glance, so that I can quickly find and access the run I'm interested in.

**Acceptance Criteria:**

1. <a name="5.1"></a>The root path (`/`) SHALL display a dashboard listing all registered runs
2. <a name="5.2"></a>Each run entry SHALL display: name, repository, branch, status, and start time
3. <a name="5.3"></a>Runs SHALL be grouped by repository
4. <a name="5.4"></a>Runs SHALL be sorted by start time with most recent first within each repository group
5. <a name="5.5"></a>Each run entry SHALL link to its detail page at `/runs/{id}`
6. <a name="5.6"></a>Run status SHALL be indicated by a CSS class `status-{value}` where value is running, completed, failed, or missing
7. <a name="5.7"></a>IF a run's log directory no longer exists, THEN the run SHALL be displayed with status "missing" and CSS class `status-missing`
8. <a name="5.8"></a>IF there are no registered runs, THEN the dashboard SHALL display a message explaining how to register runs

### 6. Run Detail View

**User Story:** As a user, I want to see detailed information about a specific run, so that I can understand its progress and access its logs.

**Acceptance Criteria:**

1. <a name="6.1"></a>The path `/runs/{id}` SHALL display detailed information for a specific run
2. <a name="6.2"></a>The detail view SHALL display: run name, repository, branch, status, start time
3. <a name="6.3"></a>IF the run has a finished_at timestamp, THEN the detail view SHALL display the duration as `finished_at - started_at`
4. <a name="6.4"></a>The detail view SHALL list all phases from the phases array with their status
5. <a name="6.5"></a>Each phase SHALL link to its transcript viewer at `/runs/{id}/transcript/{phase}`
6. <a name="6.6"></a>IF a summary.json exists, THEN the detail view SHALL display: total phases, completed phases, and any error message
7. <a name="6.7"></a>IF the run status is "running", THEN the status section SHALL auto-refresh using htmx polling
8. <a name="6.8"></a>IF the run ID is not a valid UUID v4, THEN the server SHALL return HTTP 404
9. <a name="6.9"></a>IF the run ID is not found in the registry, THEN the server SHALL return HTTP 404

### 7. Transcript Viewer

**User Story:** As a user, I want to read session transcripts in a formatted view, so that I can understand what happened during each phase without parsing raw log files.

**Acceptance Criteria:**

1. <a name="7.1"></a>The path `/runs/{id}/transcript/{phase}` SHALL display the transcript for phase N, run 1 (most recent run of that phase)
2. <a name="7.2"></a>The path `/runs/{id}/transcript/{phase}/{run}` SHALL display the transcript for a specific phase and run number
3. <a name="7.3"></a>The transcript SHALL be rendered using the existing `internal/transcript/html.go` HTML rendering with its current styling and configuration
4. <a name="7.4"></a>The `internal/transcript/html.go` renderer SHALL be extended to accept optional navigation context (previous/next phase links)
5. <a name="7.5"></a>WHEN navigation context is provided, the renderer SHALL display navigation links at the top and bottom of the transcript
6. <a name="7.6"></a>IF the phase transcript file does not exist, THEN the system SHALL return HTTP 404 with message "Transcript not found for phase {N}"
7. <a name="7.7"></a>IF the phase parameter is not a positive integer, THEN the server SHALL return HTTP 404

### 8. Mobile Responsiveness

**User Story:** As a user accessing Orbit from my phone, I want the interface to be usable on a small screen, so that I can monitor runs remotely without needing a desktop.

**Acceptance Criteria:**

1. <a name="8.1"></a>The interface SHALL be usable on viewports as narrow as 320px (iPhone SE)
2. <a name="8.2"></a>Touch targets (buttons, links) SHALL have a minimum size of 44x44 CSS pixels
3. <a name="8.3"></a>The interface SHALL use a single-column layout on viewports narrower than 768px
4. <a name="8.4"></a>Text content SHALL wrap without requiring horizontal scrolling
5. <a name="8.5"></a>The interface SHALL support dark mode using `@media (prefers-color-scheme: dark)`
6. <a name="8.6"></a>The viewport meta tag SHALL be set to `width=device-width, initial-scale=1`
7. <a name="8.7"></a>Pinch-to-zoom SHALL remain enabled for accessibility

### 9. Security

**User Story:** As a user, I want the web interface to be secure by default, so that I don't accidentally expose sensitive log data to unauthorized access.

**Acceptance Criteria:**

1. <a name="9.1"></a>The server SHALL bind to localhost (127.0.0.1) only by default
2. <a name="9.2"></a>The interface SHALL be read-only with no write operations exposed via HTTP
3. <a name="9.3"></a>Run IDs SHALL be validated as UUID v4 format using regex pattern `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`
4. <a name="9.4"></a>Path components SHALL be sanitized using `filepath.Clean()` and requests containing `..` SHALL be rejected with HTTP 404
5. <a name="9.5"></a>The server SHALL only serve files from directories registered in the registry
6. <a name="9.6"></a>The server SHALL verify that requested file paths are within the registered log_dir using `filepath.Rel()` and rejecting paths that start with `..`
7. <a name="9.7"></a>The server SHALL NOT follow symlinks that resolve to paths outside the registered log_dir
8. <a name="9.8"></a>Invalid or unauthorized path requests SHALL return HTTP 404 to prevent path enumeration
9. <a name="9.9"></a>The server SHALL set the following HTTP headers on all responses: `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`
10. <a name="9.10"></a>HTML responses SHALL include a Content-Security-Policy header: `default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'`

### 10. Frontend Architecture

**User Story:** As a maintainer, I want the frontend to be simple and dependency-free, so that the codebase remains maintainable and the binary stays self-contained.

**Acceptance Criteria:**

1. <a name="10.1"></a>The frontend SHALL use server-side rendering with Go html/template
2. <a name="10.2"></a>The frontend SHALL use htmx for dynamic updates without full page reloads
3. <a name="10.3"></a>htmx SHALL be embedded in the binary using Go embed (no external CDN)
4. <a name="10.4"></a>HTML templates SHALL be embedded in the binary using Go embed
5. <a name="10.5"></a>The frontend SHALL NOT require a JavaScript build step
6. <a name="10.6"></a>CSS styling SHALL be embedded as static files using Go embed
7. <a name="10.7"></a>WITH JavaScript disabled, users SHALL be able to navigate between pages and view content
8. <a name="10.8"></a>WITH JavaScript disabled, auto-refresh features SHALL be unavailable but a manual refresh link SHALL be provided

### 11. Live Updates

**User Story:** As a user watching a running orchestration, I want the interface to update automatically, so that I can see progress without manually refreshing the page.

**Acceptance Criteria:**

1. <a name="11.1"></a>The run detail view SHALL poll `/runs/{id}/status` every 5 seconds when status is "running"
2. <a name="11.2"></a>Polling SHALL stop when the status response indicates "completed" or "failed"
3. <a name="11.3"></a>The dashboard SHALL poll `/dashboard/status` every 10 seconds to refresh run statuses
4. <a name="11.4"></a>Status updates SHALL use htmx to replace only the status elements, not the full page
5. <a name="11.5"></a>IF the server is unreachable for 3 consecutive poll attempts, THEN the interface SHALL display "Connection lost - retrying..." in the status area
6. <a name="11.6"></a>WHEN connection is restored, the error indicator SHALL be removed automatically

---

## Future Enhancements (Out of Scope for v1)

The following features are explicitly deferred to future versions:

- **REST API**: JSON endpoints for programmatic access (`/api/runs`, etc.)
- **Server-Sent Events (SSE)**: Real-time push updates instead of polling
- **Authentication**: Optional basic auth or token-based authentication
- **Search and filtering**: Find runs by name, branch, or date range
- **Cost analytics dashboard**: Token usage and cost tracking visualization
- **Run deletion**: Remove runs from registry via web interface
