// Package apsisweb provides the HTTP server for the Apsis web interface.
package apsisweb

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/arjenschwarz/orbit/internal/sessions"
	"github.com/arjenschwarz/orbit/internal/web"
)

//go:embed templates/*
var templatesFS embed.FS

// CSSVersion is used for cache busting. Bump when CSS changes.
const CSSVersion = "1"

// Config holds web server configuration.
type Config struct {
	Port        int    // Default: 8081
	Bind        string // Default: "localhost"
	ProjectPath string // Resolved project directory
}

// Server is the HTTP server for the apsis web interface.
type Server struct {
	config   Config
	router   *http.ServeMux
	server   *http.Server
	lister   *sessions.Lister
	resolver *sessions.Resolver
}

// New creates a new web server.
func New(config Config) (*Server, error) {
	lister, err := sessions.NewLister()
	if err != nil {
		return nil, fmt.Errorf("create lister: %w", err)
	}

	resolver, err := sessions.NewResolver(config.ProjectPath)
	if err != nil {
		return nil, fmt.Errorf("create resolver: %w", err)
	}

	router := http.NewServeMux()

	s := &Server{
		config:   config,
		router:   router,
		lister:   lister,
		resolver: resolver,
	}

	s.setupRoutes()

	s.server = &http.Server{
		Addr:              fmt.Sprintf("%s:%d", config.Bind, config.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return s, nil
}

// setupRoutes configures the HTTP routes.
func (s *Server) setupRoutes() {
	static := http.StripPrefix("/static/", newStaticHandler())

	// Static assets
	s.router.Handle("GET /static/", web.SecurityHeaders(static))
	s.router.Handle("GET /static/transcript.css",
		web.SecurityHeaders(http.HandlerFunc(s.handleTranscriptCSS)))

	// Pages
	s.router.Handle("GET /",
		web.SecurityHeaders(web.PathSanitizer(http.HandlerFunc(s.handleSessionList))))
	s.router.Handle("GET /api/sessions",
		web.SecurityHeaders(web.PathSanitizer(http.HandlerFunc(s.handleSessionListFragment))))
	s.router.Handle("GET /sessions/{source}/{id}",
		web.SecurityHeaders(web.PathSanitizer(
			ValidateSource("source")(
				SanitizeSessionID("id")(
					http.HandlerFunc(s.handleTranscript))))))
}

// Start begins listening and serving requests.
// Does not handle signals — the caller is responsible for calling Shutdown.
func (s *Server) Start() error {
	err := s.server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return err
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// Template data types

// TemplateData is the base data for all templates.
type TemplateData struct {
	Title      string
	CSSVersion string
}

// SessionListData is passed to sessions.html.
type SessionListData struct {
	TemplateData
	Sessions []SessionView
	Warnings []string
	Sources  []string
	Empty    bool
}

// SessionView is a single session formatted for display.
type SessionView struct {
	ID          string
	DisplayID   string
	Source      string
	SourceClass string
	CreatedAt   string
	Size        string
	URL         string
}

// TranscriptViewData is passed to transcript.html.
type TranscriptViewData struct {
	TemplateData
	SessionID string
	Source    string
	Content   template.HTML
	CreatedAt string
	Size      string
}

// ErrorData is passed to error.html.
type ErrorData struct {
	TemplateData
	Code    int
	Message string
}

// Template rendering

var templateFuncs = template.FuncMap{
	"sourceClass": func(source string) string {
		return "source-" + source
	},
}

var pageTemplates = make(map[string]*template.Template)

func init() {
	pages := []string{"sessions.html", "transcript.html", "error.html"}
	for _, page := range pages {
		tmpl := template.Must(
			template.New("").Funcs(templateFuncs).
				ParseFS(templatesFS, "templates/layout.html", "templates/"+page))
		pageTemplates[page] = tmpl
	}
}

func (s *Server) renderTemplate(w http.ResponseWriter, page string, data any) {
	tmpl := pageTemplates[page]
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func (s *Server) renderFragment(w http.ResponseWriter, name string, data any) {
	tmpl := pageTemplates["sessions.html"]
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("fragment error: %v", err)
	}
}
