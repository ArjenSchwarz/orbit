// Package web provides the HTTP server for the Orbit web interface.
package web

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/arjenschwarz/orbit/internal/registry"
)

// Config holds web server configuration.
type Config struct {
	Port     int    // Default: 8080
	Bind     string // Default: "localhost"
	Registry *registry.Registry
}

// Server is the HTTP server for the web interface.
type Server struct {
	config   Config
	router   *http.ServeMux
	server   *http.Server
	registry *registry.Registry
}

// New creates a new web server.
func New(config Config) *Server {
	router := http.NewServeMux()

	// Create the server
	s := &Server{
		config:   config,
		router:   router,
		registry: config.Registry,
	}

	// Set up routes
	s.setupRoutes()

	// Create http.Server
	s.server = &http.Server{
		Addr:    fmt.Sprintf("%s:%d", config.Bind, config.Port),
		Handler: router,
	}

	return s
}

// setupRoutes configures the HTTP routes.
func (s *Server) setupRoutes() {
	// Apply middleware stack to all routes
	// Note: Go 1.22+ ServeMux handles path parameters with {name} syntax

	// Static files
	staticHandler := http.StripPrefix("/static/", newStaticHandler())
	s.router.Handle("GET /static/", SecurityHeaders(staticHandler))

	// Transcript CSS (served from transcript package to avoid duplication)
	s.router.Handle("GET /static/transcript.css", SecurityHeaders(http.HandlerFunc(s.handleTranscriptCSS)))

	// Dashboard
	s.router.Handle("GET /", SecurityHeaders(http.HandlerFunc(s.handleDashboard)))
	s.router.Handle("GET /dashboard/status", SecurityHeaders(http.HandlerFunc(s.handleDashboardStatus)))

	// Run detail
	s.router.Handle("GET /runs/{id}", SecurityHeaders(PathSanitizer(
		ValidateUUID("id")(http.HandlerFunc(s.handleRunDetail)),
	)))
	s.router.Handle("GET /runs/{id}/status", SecurityHeaders(PathSanitizer(
		ValidateUUID("id")(http.HandlerFunc(s.handleRunStatus)),
	)))

	// Transcript viewer
	s.router.Handle("GET /runs/{id}/transcript/{phase}", SecurityHeaders(PathSanitizer(
		ValidateUUID("id")(http.HandlerFunc(s.handleTranscript)),
	)))
	s.router.Handle("GET /runs/{id}/transcript/{phase}/{run}", SecurityHeaders(PathSanitizer(
		ValidateUUID("id")(http.HandlerFunc(s.handleTranscript)),
	)))
}

// Start begins listening and serving requests.
// It sets up signal handling for graceful shutdown.
// Blocks until shutdown.
func (s *Server) Start() error {
	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Channel to signal shutdown completion
	shutdownComplete := make(chan struct{})

	go func() {
		sig := <-sigChan
		log.Printf("Received signal %v, shutting down...", sig)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := s.Shutdown(ctx); err != nil {
			log.Printf("Shutdown error: %v", err)
		}
		close(shutdownComplete)
	}()

	log.Printf("Starting server at http://%s:%d", s.config.Bind, s.config.Port)

	err := s.server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	// Wait for shutdown handler to complete if it was triggered
	select {
	case <-shutdownComplete:
	default:
	}

	return nil
}

// Shutdown gracefully stops the server.
// Waits up to 5 seconds for in-flight requests.
func (s *Server) Shutdown(ctx context.Context) error {
	log.Println("Shutting down server...")
	return s.server.Shutdown(ctx)
}
