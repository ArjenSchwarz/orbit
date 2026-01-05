package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/arjenschwarz/orbit/internal/config"
	"github.com/arjenschwarz/orbit/internal/registry"
	"github.com/arjenschwarz/orbit/internal/web"
)

// serveCommand starts the web server.
func serveCommand(args []string) error {
	regDir, err := getRegistryDir()
	if err != nil {
		return fmt.Errorf("failed to get registry directory: %w", err)
	}

	return serveCommandWithConfig(args, regDir)
}

// serveCommandWithConfig is the internal implementation that allows overriding
// the registry directory for testing.
func serveCommandWithConfig(args []string, regDir string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)

	portFlag := fs.Int("port", 0, "Port to listen on (default from config or 8080)")
	bindFlag := fs.String("bind", "", "Address to bind to (default from config or localhost)")
	showVersion := fs.Bool("version", false, "Show version and exit")
	showHelp := fs.Bool("help", false, "Show help and exit")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: orbit serve [options]\n\n")
		fmt.Fprintf(os.Stderr, "Start the Orbit web interface for viewing run status and transcripts.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nConfiguration:\n")
		fmt.Fprintf(os.Stderr, "  Port and bind can be configured via:\n")
		fmt.Fprintf(os.Stderr, "  - Environment: ORBIT_SERVE_PORT, ORBIT_SERVE_BIND\n")
		fmt.Fprintf(os.Stderr, "  - Config file: serve-port, serve-bind in ~/.orbit.yaml or .orbit.yaml\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  orbit serve                    # Start with default settings\n")
		fmt.Fprintf(os.Stderr, "  orbit serve --port 3000        # Start on port 3000\n")
		fmt.Fprintf(os.Stderr, "  orbit serve --bind 0.0.0.0     # Listen on all interfaces\n")
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	if *showHelp {
		fs.Usage()
		return nil
	}

	if *showVersion {
		fmt.Printf("orbit serve version %s\n", version)
		return nil
	}

	// Load configuration
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	cfg := config.Load(workingDir)

	// Apply defaults from config, with CLI flags as overrides
	port := cfg.ServePort
	if *portFlag != 0 {
		port = *portFlag
	}

	bind := cfg.ServeBind
	if *bindFlag != "" {
		bind = *bindFlag
	}

	// Validate port range
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", port)
	}

	// Create registry
	reg, err := registry.New(regDir)
	if err != nil {
		return fmt.Errorf("failed to initialize registry: %w", err)
	}

	// Create and start server
	webConfig := web.Config{
		Port:     port,
		Bind:     bind,
		Registry: reg,
	}

	server := web.New(webConfig)

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- server.Start()
	}()

	// Wait for signal or error
	select {
	case sig := <-sigChan:
		log.Printf("Received signal %v, shutting down...", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(ctx)
	case err := <-errChan:
		return err
	}
}
