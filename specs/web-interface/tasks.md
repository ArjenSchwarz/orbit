---
references:
    - specs/web-interface/requirements.md
    - specs/web-interface/design.md
    - specs/web-interface/decision_log.md
---
# Web Interface Implementation

## Phase 1: Registry Package Foundation

- [x] 1. Create registry types (internal/registry/types.go)
  - Define RunStatus, PhaseStatus, Phase, RunEntry structs with JSON tags
  - Requirements: [2.3](requirements.md#2.3), [2.4](requirements.md#2.4), [2.5](requirements.md#2.5), [2.6](requirements.md#2.6), [2.7](requirements.md#2.7)
  - References: internal/registry/types.go
  - [x] 1.1. Write unit tests for registry types
    - Test JSON marshaling/unmarshaling, validate schema_version default
  - [x] 1.2. Implement registry types
    - Create types.go with all type definitions from design

- [x] 2. Implement git URL parsing (internal/registry/git.go)
  - ParseGitRemote for HTTPS and SSH formats, GetRepository with fallback
  - Requirements: [2.8](requirements.md#2.8), [2.9](requirements.md#2.9), [2.10](requirements.md#2.10)
  - References: internal/registry/git.go
  - [x] 2.1. Write property-based tests for git URL parsing
    - Use rapid to test HTTPS/SSH round-trip, edge cases
  - [x] 2.2. Write unit tests for git URL parsing
    - Test HTTPS, SSH, SSH-alt formats, invalid URLs
  - [x] 2.3. Implement ParseGitRemote and GetRepository
    - Parse git remote URLs, fall back to directory name

- [x] 3. Implement registry core operations (internal/registry/registry.go)
  - Registry struct with New, Register, Get, List, FindByLogDir, UpdateStatus, UpdatePhase
  - Requirements: [2.1](requirements.md#2.1), [2.2](requirements.md#2.2), [2.11](requirements.md#2.11), [2.12](requirements.md#2.12), [2.13](requirements.md#2.13)
  - [x] 3.1. Write unit tests for registry operations
    - Test CRUD operations, atomic writes, malformed JSON handling
  - [x] 3.2. Implement Registry struct and New function
    - Create registry directory if needed, return Registry instance
  - [x] 3.3. Implement Register with atomic file operations
    - Write to temp file, rename for atomic update
  - [x] 3.4. Implement Get, List, and FindByLogDir
    - Read JSON files, skip malformed entries with warning
  - [x] 3.5. Implement UpdateStatus and UpdatePhase
    - Read-modify-write with atomic operations

## Phase 2: CLI Refactoring

- [x] 4. Refactor CLI for subcommands (cmd/orbit/)
  - Extract run logic, add subcommand routing with backward compatibility
  - Requirements: [1.1](requirements.md#1.1)
  - [x] 4.1. Write integration tests for CLI subcommand routing
    - Test orbit run, orbit serve, orbit register, backward compatibility
  - [x] 4.2. Extract run command to run.go
    - Move orchestration logic from main.go to runCommand function
  - [x] 4.3. Implement subcommand router in main.go
    - Route to run/serve/register, default to run for backward compatibility

- [x] 5. Extend config for serve options (internal/config/config.go)
  - Add ServePort, ServeBind with defaults and env var support
  - Requirements: [1.2](requirements.md#1.2), [1.3](requirements.md#1.3), [1.4](requirements.md#1.4), [1.5](requirements.md#1.5)
  - References: internal/config/config.go
  - [x] 5.1. Write unit tests for serve config
    - Test defaults, env vars, config file loading
  - [x] 5.2. Add ServePort and ServeBind to Config struct
    - Add fields, SetDefault calls, env var bindings

## Phase 3: Manual Registration Command

- [x] 6. Implement orbit register command (cmd/orbit/register.go)
  - Validate log directory, derive metadata, create/update registry entry
  - Requirements: [4.1](requirements.md#4.1), [4.2](requirements.md#4.2), [4.3](requirements.md#4.3), [4.4](requirements.md#4.4), [4.5](requirements.md#4.5), [4.6](requirements.md#4.6), [4.7](requirements.md#4.7), [4.8](requirements.md#4.8), [4.9](requirements.md#4.9), [4.10](requirements.md#4.10), [4.11](requirements.md#4.11), [4.12](requirements.md#4.12)
  - [x] 6.1. Write unit tests for register command
    - Test valid logs, invalid path, existing entry update, status derivation
  - [x] 6.2. Implement log directory validation
    - Check for phase-*-run-*-session.json files
  - [x] 6.3. Implement metadata derivation
    - Parse summary.json for status, scan files for phases
  - [x] 6.4. Implement registerCommand function
    - Parse flags, validate, derive metadata, register

## Phase 4: Transcript Extension

- [x] 7. Extend transcript renderer with navigation (internal/transcript/html.go)
  - Add NavigationContext, RenderHTMLFragment, renderEntriesToBuilder
  - Requirements: [7.3](requirements.md#7.3), [7.4](requirements.md#7.4), [7.5](requirements.md#7.5)
  - References: internal/transcript/html.go
  - [x] 7.1. Write unit tests for transcript navigation
    - Test RenderHTMLFragment output, navigation HTML generation
  - [x] 7.2. Add NavigationContext to RenderOptions
    - Add struct and field for navigation links
  - [x] 7.3. Refactor RenderHTML to use renderEntriesToBuilder
    - Extract entry rendering to shared function
  - [x] 7.4. Implement RenderHTMLFragment
    - Render without document wrapper, include navigation
  - [x] 7.5. Add renderNavigationHTML helper
    - Generate navigation bar HTML with prev/next/back links

## Phase 5: Web Server

- [x] 8. Implement web server foundation (internal/web/)
  - Server struct, Start, Shutdown with graceful handling
  - Requirements: [1.6](requirements.md#1.6), [1.7](requirements.md#1.7), [1.8](requirements.md#1.8), [1.9](requirements.md#1.9)
  - [x] 8.1. Write unit tests for server lifecycle
    - Test start, shutdown, port conflict handling
  - [x] 8.2. Implement Server struct and New function
    - Create server with config, registry, router
  - [x] 8.3. Implement Start with signal handling
    - Listen, print URL, handle SIGINT/SIGTERM
  - [x] 8.4. Implement Shutdown with graceful drain
    - 5-second timeout for in-flight requests

- [x] 9. Implement security middleware (internal/web/middleware.go)
  - SecurityHeaders, ValidateUUID, PathSanitizer
  - Requirements: [9.1](requirements.md#9.1), [9.3](requirements.md#9.3), [9.4](requirements.md#9.4), [9.5](requirements.md#9.5), [9.6](requirements.md#9.6), [9.7](requirements.md#9.7), [9.8](requirements.md#9.8), [9.9](requirements.md#9.9), [9.10](requirements.md#9.10)
  - [x] 9.1. Write unit tests for middleware
    - Test headers, UUID validation, path traversal rejection
  - [x] 9.2. Implement SecurityHeaders middleware
    - Add X-Content-Type-Options, X-Frame-Options, Referrer-Policy, CSP
  - [x] 9.3. Implement ValidateUUID middleware
    - Regex validation, 404 for invalid UUIDs
  - [x] 9.4. Implement PathSanitizer middleware
    - Reject paths with .., verify within log_dir
  - [x] 9.5. Implement isPathWithinDir helper
    - Resolve symlinks, check relative path does not escape

- [x] 10. Create embedded static assets (internal/web/static/)
  - Embed htmx.min.js, style.css with go:embed
  - Requirements: [10.2](requirements.md#10.2), [10.3](requirements.md#10.3), [10.6](requirements.md#10.6)
  - [x] 10.1. Download and embed htmx.min.js
    - Get htmx 1.9.x, add go:embed directive
  - [x] 10.2. Create style.css with responsive styles
    - CSS variables, dark mode, mobile breakpoints, 44px touch targets
    - Requirements: [8.1](requirements.md#8.1), [8.2](requirements.md#8.2), [8.3](requirements.md#8.3), [8.4](requirements.md#8.4), [8.5](requirements.md#8.5)
  - [x] 10.3. Create static file handler
    - Serve embedded files with correct content types

- [x] 11. Create HTML templates (internal/web/templates/)
  - layout.html, dashboard.html, run_detail.html, transcript.html, error.html
  - Requirements: [10.1](requirements.md#10.1), [10.4](requirements.md#10.4), [10.7](requirements.md#10.7), [10.8](requirements.md#10.8)
  - [x] 11.1. Create layout.html base template
    - HTML structure, viewport meta, htmx script, connection status, noscript
    - Requirements: [8.6](requirements.md#8.6), [8.7](requirements.md#8.7)
  - [x] 11.2. Create dashboard.html template
    - Run list grouped by repository, status indicators, empty state message
    - Requirements: [5.1](requirements.md#5.1), [5.2](requirements.md#5.2), [5.3](requirements.md#5.3), [5.4](requirements.md#5.4), [5.5](requirements.md#5.5), [5.6](requirements.md#5.6), [5.7](requirements.md#5.7), [5.8](requirements.md#5.8)
  - [x] 11.3. Create dashboard_status.html fragment
    - htmx polling fragment for dashboard status updates
  - [x] 11.4. Create run_detail.html template
    - Run info, phase list with links, duration, summary data
    - Requirements: [6.1](requirements.md#6.1), [6.2](requirements.md#6.2), [6.3](requirements.md#6.3), [6.4](requirements.md#6.4), [6.5](requirements.md#6.5), [6.6](requirements.md#6.6)
  - [x] 11.5. Create run_status.html fragment
    - htmx polling fragment for run status updates
    - Requirements: [6.7](requirements.md#6.7)
  - [x] 11.6. Create transcript.html template
    - Wrapper for RenderHTMLFragment content with navigation
  - [x] 11.7. Create error.html template
    - Generic error page for 404 responses
    - Requirements: [6.8](requirements.md#6.8), [6.9](requirements.md#6.9), [7.6](requirements.md#7.6), [7.7](requirements.md#7.7)

- [x] 12. Implement page handlers (internal/web/handlers.go)
  - handleDashboard, handleRunDetail, handleTranscript, status endpoints
  - Requirements: [5.1](requirements.md#5.1), [6.1](requirements.md#6.1), [7.1](requirements.md#7.1), [7.2](requirements.md#7.2)
  - [x] 12.1. Write unit tests for handlers
    - Test rendering, data population, error cases
  - [x] 12.2. Implement handleDashboard
    - Load runs, group by repository, sort by time, check log_dir existence
  - [x] 12.3. Implement handleDashboardStatus
    - Return status fragment for htmx polling
    - Requirements: [11.3](requirements.md#11.3)
  - [x] 12.4. Implement handleRunDetail
    - Load run, phases, summary.json, calculate duration
  - [x] 12.5. Implement handleRunStatus
    - Return status fragment, stop polling on terminal state
    - Requirements: [11.1](requirements.md#11.1), [11.2](requirements.md#11.2)
  - [x] 12.6. Implement handleTranscript
    - Load session file, call RenderHTMLFragment, build navigation
  - [x] 12.7. Wire up router with all handlers and middleware
    - Register routes, apply middleware chain

- [x] 13. Implement orbit serve command (cmd/orbit/serve.go)
  - Parse flags, load config, start server
  - Requirements: [1.1](requirements.md#1.1), [1.2](requirements.md#1.2), [1.3](requirements.md#1.3), [1.4](requirements.md#1.4), [1.5](requirements.md#1.5), [1.6](requirements.md#1.6)
  - [x] 13.1. Write integration tests for serve command
    - Test flag parsing, config loading, server startup
  - [x] 13.2. Implement serveCommand function
    - Parse --port, --bind flags, merge with config, start server

## Phase 6: Auto-Registration Integration

- [x] 14. Integrate registry with orbit run (internal/orbit/orbit.go)
  - Register on start, update status on completion, update phases
  - Requirements: [3.1](requirements.md#3.1), [3.2](requirements.md#3.2), [3.3](requirements.md#3.3), [3.4](requirements.md#3.4), [3.5](requirements.md#3.5), [3.6](requirements.md#3.6), [3.7](requirements.md#3.7)
  - References: internal/orbit/orbit.go
  - [x] 14.1. Write unit tests for auto-registration
    - Test registration on start, status updates, phase updates
  - [x] 14.2. Add registry field to Orbit struct
    - Initialize registry in New function
  - [x] 14.3. Register run on start
    - Create entry with running status, pid, log_dir
  - [x] 14.4. Update phase status during execution
    - Call UpdatePhase when phases start/complete
  - [x] 14.5. Update run status on completion
    - Set completed/failed status, finished_at timestamp
  - [x] 14.6. Handle registration failures gracefully
    - Log warning, continue execution

## Phase 7: Live Updates

- [ ] 15. Implement connection failure handling
  - htmx error events, consecutive failure counter, status indicator
  - Requirements: [11.5](requirements.md#11.5), [11.6](requirements.md#11.6)
  - [ ] 15.1. Add connection status element to layout.html
    - Hidden div with Connection lost message
  - [ ] 15.2. Add htmx error event handlers
    - Track failures, show/hide status on threshold
  - [ ] 15.3. Add CSS for connection status indicator
    - Fixed position, error styling, show on disconnected class
