---
references:
    - specs/apsis-serve/requirements.md
    - specs/apsis-serve/design.md
    - specs/apsis-serve/decision_log.md
---
# apsis-serve

## Pre-work

- [x] 1. Export isPathWithinDir in internal/web/middleware.go <!-- id:99ijhi4 -->
  - Rename isPathWithinDir to IsPathWithinDir in internal/web/middleware.go
  - Update the single call site in internal/web/handlers.go (line ~483)
  - Run make test to verify orbit tests still pass
  - Stream: 1
  - Requirements: [6.7](requirements.md#6.7)
  - References: internal/web/middleware.go, internal/web/handlers.go

## Session Extraction

- [x] 2. Create internal/sessions/types.go with data types and source constants <!-- id:99ijhi5 -->
  - Create internal/sessions/ package directory
  - Define SessionInfo struct (ID, CreatedAt, Size, Source)
  - Define SessionMetadata, ResolvedSession, ListWarning structs
  - Define source constants: SourceClaude, SourceCodex, SourceCopilot, SourceKiroCLI, SourceKiroIDE
  - Implement AllSources(), DisplayName(), IsValidSource(), FormatSize()
  - DisplayName maps: kiro-ide -> kiro ide; all others identity
  - FormatSize extracted from main.go formatSize()
  - Stream: 1
  - Requirements: [1.1](requirements.md#1.1), [1.7](requirements.md#1.7), [1.8](requirements.md#1.8)
  - References: cmd/apsis/main.go

- [x] 3. Write property-based and unit tests for sessions types <!-- id:99ijhi6 -->
  - Create internal/sessions/types_test.go
  - Property-based tests for FormatSize using pgregory.net/rapid
  - Unit tests for AllSources(), DisplayName(), IsValidSource()
  - Blocked-by: 99ijhi5 (Create internal/sessions/types.go with data types and source constants)
  - Stream: 1
  - Requirements: [1.7](requirements.md#1.7), [1.8](requirements.md#1.8)
  - References: internal/sessions/types.go

- [x] 4. Create internal/sessions/lister.go — extract session listing logic <!-- id:99ijhi7 -->
  - Create Lister struct with homeDir field
  - Implement NewLister() (*Lister, error)
  - Implement ListAll(projectPath) ([]SessionInfo, []ListWarning, error)
  - Extract per-agent list functions from main.go
  - Extract supporting functions and types
  - Use canonical source constants in all list functions
  - Blocked-by: 99ijhi5 (Create internal/sessions/types.go with data types and source constants)
  - Stream: 1
  - Requirements: [1.1](requirements.md#1.1), [1.2](requirements.md#1.2), [1.5](requirements.md#1.5), [1.6](requirements.md#1.6)
  - References: cmd/apsis/main.go, internal/sessions/types.go

- [x] 5. Create internal/sessions/resolver.go — extract session resolution logic <!-- id:99ijhi8 -->
  - Create Resolver struct with projectPath and homeDir fields
  - Implement NewResolver(projectPath string) (*Resolver, error)
  - Implement Resolve(source, sessionID string) (*ResolvedSession, error)
  - Extract find/resolve functions from main.go
  - Call web.IsPathWithinDir() before opening resolved files
  - Populate Metadata.Size via os.Stat for file-backed sessions
  - Set CostPath for Kiro IDE sessions
  - Blocked-by: 99ijhi5 (Create internal/sessions/types.go with data types and source constants)
  - Stream: 1
  - Requirements: [1.1](requirements.md#1.1), [1.3](requirements.md#1.3), [6.5](requirements.md#6.5)
  - References: cmd/apsis/main.go, internal/sessions/types.go, internal/web/middleware.go

- [x] 6. Write tests for sessions Lister <!-- id:99ijhi9 -->
  - Create internal/sessions/lister_test.go
  - Test ListAll with no sessions, single source, multiple sources
  - Test one source failing returns warning while others succeed
  - Test sort order: oldest-first
  - Use temp directories with mock .jsonl files
  - Blocked-by: 99ijhi7 (Create internal/sessions/lister.go — extract session listing logic)
  - Stream: 1
  - Requirements: [1.2](requirements.md#1.2), [1.5](requirements.md#1.5), [1.6](requirements.md#1.6)
  - References: internal/sessions/lister.go

- [x] 7. Write tests for sessions Resolver <!-- id:99ijhia -->
  - Create internal/sessions/resolver_test.go
  - Test Resolve with Claude source using test fixture
  - Test unknown source and non-existent session return errors
  - Test metadata fields populated correctly
  - Test path validation with IsPathWithinDir
  - Blocked-by: 99ijhi8 (Create internal/sessions/resolver.go — extract session resolution logic)
  - Stream: 1
  - Requirements: [1.3](requirements.md#1.3), [6.5](requirements.md#6.5)
  - References: internal/sessions/resolver.go

- [x] 8. Wire cmd/apsis/main.go to use sessions package and remove extracted code <!-- id:99ijhib -->
  - Replace listSessions() to use sessions.NewLister()
  - Replace resolveInput() to use sessions.NewResolver()
  - Use sessions.DisplayName() and sessions.FormatSize()
  - Delete all extracted functions and types from main.go
  - Verify apsis -l output identical before and after (req 1.4)
  - Run make test and make lint
  - Blocked-by: 99ijhi7 (Create internal/sessions/lister.go — extract session listing logic), 99ijhi8 (Create internal/sessions/resolver.go — extract session resolution logic), 99ijhi9 (Write tests for sessions Lister), 99ijhia (Write tests for sessions Resolver)
  - Stream: 1
  - Requirements: [1.1](requirements.md#1.1), [1.4](requirements.md#1.4)
  - References: cmd/apsis/main.go, internal/sessions/lister.go, internal/sessions/resolver.go

## Web Server

- [x] 9. Create internal/apsisweb/middleware.go with ValidateSource and SanitizeSessionID <!-- id:99ijhic -->
  - Implement ValidateSource(paramName) func(http.Handler) http.Handler
  - Implement SanitizeSessionID(paramName) func(http.Handler) http.Handler
  - Follow same pattern as orbit ValidateUUID
  - Blocked-by: 99ijhi5 (Create internal/sessions/types.go with data types and source constants)
  - Stream: 2
  - Requirements: [4.7](requirements.md#4.7), [4.8](requirements.md#4.8), [6.4](requirements.md#6.4)
  - References: internal/web/middleware.go, internal/sessions/types.go

- [x] 10. Write tests for apsisweb middleware <!-- id:99ijhid -->
  - Create internal/apsisweb/middleware_test.go
  - Test ValidateSource with valid and invalid sources
  - Test SanitizeSessionID with clean IDs and path traversal attempts
  - Test URL-decoded path traversal rejection
  - Blocked-by: 99ijhic (Create internal/apsisweb/middleware.go with ValidateSource and SanitizeSessionID)
  - Stream: 2
  - Requirements: [4.7](requirements.md#4.7), [4.8](requirements.md#4.8)
  - References: internal/apsisweb/middleware.go

- [x] 11. Create internal/apsisweb/static.go with embedded static file serving <!-- id:99ijhie -->
  - Create embed.FS for static/ directory
  - Implement newStaticHandler() and stripPrefix()
  - Set Cache-Control and Content-Type headers
  - Stream: 2
  - Requirements: [7.1](requirements.md#7.1), [7.3](requirements.md#7.3)
  - References: internal/web/static.go

- [x] 12. Create static assets: style.css and vendored htmx.min.js <!-- id:99ijhif -->
  - Create internal/apsisweb/static/ directory
  - Create style.css with CSS variables, dark mode, agent badges, responsive layout
  - Copy htmx.min.js from internal/web/static/htmx.min.js
  - Stream: 2
  - Requirements: [5.1](requirements.md#5.1), [5.2](requirements.md#5.2), [5.3](requirements.md#5.3), [5.4](requirements.md#5.4), [5.5](requirements.md#5.5), [5.6](requirements.md#5.6), [5.7](requirements.md#5.7), [7.6](requirements.md#7.6)
  - References: internal/web/static/style.css, internal/web/static/htmx.min.js

- [x] 13. Create HTML templates: layout.html, sessions.html, transcript.html, error.html <!-- id:99ijhig -->
  - Create internal/apsisweb/templates/ directory
  - layout.html with HTMX, CSS cache-busting, connection indicator, noscript notice
  - sessions.html with filter pills, HTMX polling, search, session cards
  - transcript.html with back link, metadata card, 900px transcript wrapper
  - error.html with code, message, back link
  - Blocked-by: 99ijhif (Create static assets: style.css and vendored htmx.min.js)
  - Stream: 2
  - Requirements: [3.1](requirements.md#3.1), [3.2](requirements.md#3.2), [3.3](requirements.md#3.3), [3.4](requirements.md#3.4), [3.5](requirements.md#3.5), [3.6](requirements.md#3.6), [3.7](requirements.md#3.7), [3.8](requirements.md#3.8), [3.9](requirements.md#3.9), [3.10](requirements.md#3.10), [3.11](requirements.md#3.11), [3.12](requirements.md#3.12), [4.1](requirements.md#4.1), [4.3](requirements.md#4.3), [4.4](requirements.md#4.4), [4.5](requirements.md#4.5), [7.2](requirements.md#7.2), [7.4](requirements.md#7.4), [8.1](requirements.md#8.1), [8.2](requirements.md#8.2), [8.3](requirements.md#8.3), [8.4](requirements.md#8.4)
  - References: internal/web/templates/layout.html, internal/web/templates/dashboard.html

- [x] 14. Create internal/apsisweb/server.go with Server, Config, routing, and lifecycle <!-- id:99ijhih -->
  - Define Config, template data types, Server struct
  - Implement New(), setupRoutes(), Start(), Shutdown()
  - Set HTTP timeouts: ReadHeader 10s, Write 120s, Idle 60s
  - Template rendering with init(), renderTemplate(), renderFragment()
  - Embed templates via go:embed
  - Blocked-by: 99ijhi7 (Create internal/sessions/lister.go — extract session listing logic), 99ijhi8 (Create internal/sessions/resolver.go — extract session resolution logic), 99ijhic (Create internal/apsisweb/middleware.go with ValidateSource and SanitizeSessionID), 99ijhie (Create internal/apsisweb/static.go with embedded static file serving), 99ijhig (Create HTML templates: layout.html, sessions.html, transcript.html, error.html)
  - Stream: 2
  - Requirements: [2.1](requirements.md#2.1), [2.10](requirements.md#2.10), [7.2](requirements.md#7.2)
  - References: internal/web/server.go, internal/web/handlers.go

- [x] 15. Create internal/apsisweb/handlers.go with session list, transcript, and error handlers <!-- id:99ijhii -->
  - Implement handleSessionList, handleSessionListFragment
  - Implement handleTranscript with 50MB guard and Kiro IDE cost threading
  - Implement handleTranscriptCSS, renderError, handleNotFound
  - Implement buildSessionListData with newest-first sort and ID truncation
  - Blocked-by: 99ijhih (Create internal/apsisweb/server.go with Server, Config, routing, and lifecycle)
  - Stream: 2
  - Requirements: [3.1](requirements.md#3.1), [3.3](requirements.md#3.3), [3.7](requirements.md#3.7), [3.8](requirements.md#3.8), [3.9](requirements.md#3.9), [3.12](requirements.md#3.12), [4.1](requirements.md#4.1), [4.2](requirements.md#4.2), [4.6](requirements.md#4.6), [4.9](requirements.md#4.9), [7.5](requirements.md#7.5)
  - References: internal/web/handlers.go

- [x] 16. Write tests for apsisweb handlers <!-- id:99ijhij -->
  - Create internal/apsisweb/handlers_test.go
  - Test session list rendering, empty state, warnings
  - Test transcript rendering, 404, 413 for oversized files
  - Test transcript CSS content type and cache headers
  - Test exact root path routing
  - Blocked-by: 99ijhii (Create internal/apsisweb/handlers.go with session list, transcript, and error handlers)
  - Stream: 2
  - Requirements: [3.1](requirements.md#3.1), [3.8](requirements.md#3.8), [3.9](requirements.md#3.9), [3.12](requirements.md#3.12), [4.1](requirements.md#4.1), [4.6](requirements.md#4.6), [4.9](requirements.md#4.9)
  - References: internal/apsisweb/handlers.go

- [x] 17. Write tests for apsisweb server lifecycle <!-- id:99ijhik -->
  - Create internal/apsisweb/server_test.go
  - Test New creates server with correct config and timeouts
  - Test server starts and responds on ephemeral port
  - Test graceful shutdown
  - Blocked-by: 99ijhih (Create internal/apsisweb/server.go with Server, Config, routing, and lifecycle)
  - Stream: 2
  - Requirements: [2.1](requirements.md#2.1), [2.2](requirements.md#2.2), [2.10](requirements.md#2.10)
  - References: internal/web/server.go, internal/apsisweb/server.go

## CLI Integration

- [ ] 18. Add serveCommand to cmd/apsis/main.go with flag parsing and signal handling <!-- id:99ijhil -->
  - Detect serve subcommand before flag.Parse()
  - Implement serveCommand() with separate flag.FlagSet
  - Add --port, --bind, --project flags with env var fallback
  - Implement resolve helpers for config priority
  - Signal handling with 5-second shutdown timeout
  - Update printUsage() to mention serve subcommand
  - Blocked-by: 99ijhib (Wire cmd/apsis/main.go to use sessions package and remove extracted code), 99ijhih (Create internal/apsisweb/server.go with Server, Config, routing, and lifecycle)
  - Stream: 1
  - Requirements: [2.1](requirements.md#2.1), [2.2](requirements.md#2.2), [2.3](requirements.md#2.3), [2.4](requirements.md#2.4), [2.5](requirements.md#2.5), [2.6](requirements.md#2.6), [2.7](requirements.md#2.7), [2.8](requirements.md#2.8), [2.9](requirements.md#2.9), [2.11](requirements.md#2.11)
  - References: cmd/apsis/main.go, cmd/orbit/serve.go

## Integration

- [ ] 19. Write integration test for end-to-end serve flow <!-- id:99ijhim -->
  - Create internal/apsisweb/integration_test.go
  - Test with temp dir containing mock Claude .jsonl session
  - Verify session list, transcript rendering, 404s, security headers
  - Test server startup and clean shutdown
  - Blocked-by: 99ijhil (Add serveCommand to cmd/apsis/main.go with flag parsing and signal handling)
  - Stream: 1
  - Requirements: [3.1](requirements.md#3.1), [4.1](requirements.md#4.1), [4.6](requirements.md#4.6), [6.2](requirements.md#6.2)
  - References: internal/apsisweb/server.go, internal/apsisweb/handlers.go

- [ ] 20. Run final verification: make test, make lint, make build <!-- id:99ijhin -->
  - Run make test — all tests pass
  - Run make lint — no lint errors
  - Run make build — both binaries build
  - Verify apsis -l output unchanged (req 1.4)
  - Blocked-by: 99ijhim (Write integration test for end-to-end serve flow)
  - Stream: 1
  - Requirements: [1.4](requirements.md#1.4)
  - References: Makefile
