package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/arjenschwarz/orbit/internal/registry"
)

func TestServeCommand(t *testing.T) {
	// Create temp registry directory
	tmpDir := t.TempDir()
	regDir := filepath.Join(tmpDir, "runs")
	if err := os.MkdirAll(regDir, 0755); err != nil {
		t.Fatalf("failed to create registry dir: %v", err)
	}

	// Create a test entry
	reg, err := registry.New(regDir)
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	entry := registry.NewRunEntry()
	entry.Name = "test-run"
	entry.Repository = "test/repo"
	entry.Branch = "main"
	entry.LogDir = t.TempDir()
	entry.Status = registry.StatusCompleted
	if err := reg.Register(entry); err != nil {
		t.Fatalf("failed to register entry: %v", err)
	}

	// Find available port
	port := findAvailablePort(t)

	// Start server in goroutine
	done := make(chan error, 1)
	go func() {
		// Override registry directory via environment
		t.Setenv("ORBIT_REGISTRY_DIR", regDir)
		done <- serveCommandWithConfig([]string{
			fmt.Sprintf("--port=%d", port),
			"--bind=127.0.0.1",
		}, regDir)
	}()

	// Wait for server to start
	waitForServer(t, fmt.Sprintf("127.0.0.1:%d", port))

	// Test that server responds
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil {
		t.Fatalf("failed to connect to server: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Server will need to be stopped via signal in production
	// For tests, we just check it started successfully
}

func TestServeCommandVersion(t *testing.T) {
	// Test --version flag (should not start server)
	err := serveCommandWithConfig([]string{"--version"}, t.TempDir())
	if err != nil {
		t.Errorf("version flag should not return error, got: %v", err)
	}
}

func TestServeCommandHelp(t *testing.T) {
	// Test --help flag (should not start server)
	err := serveCommandWithConfig([]string{"--help"}, t.TempDir())
	if err != nil {
		t.Errorf("help flag should not return error, got: %v", err)
	}
}

func TestServeCommandPortParsing(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name:    "valid port",
			args:    []string{"--port=8080"},
			wantErr: false,
		},
		{
			name:    "port too low",
			args:    []string{"--port=0"},
			wantErr: true,
		},
		{
			name:    "port too high",
			args:    []string{"--port=70000"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use a context with timeout to prevent hanging
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			errCh := make(chan error, 1)
			go func() {
				errCh <- serveCommandWithConfig(tt.args, t.TempDir())
			}()

			select {
			case err := <-errCh:
				if (err != nil) != tt.wantErr {
					t.Errorf("serveCommand() error = %v, wantErr %v", err, tt.wantErr)
				}
			case <-ctx.Done():
				// Server started successfully (timed out waiting for it to exit)
				if tt.wantErr {
					t.Errorf("expected error for args %v but server started", tt.args)
				}
			}
		})
	}
}

// findAvailablePort finds an available TCP port for testing.
func findAvailablePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return port
}

// waitForServer waits for the server to start accepting connections.
func waitForServer(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("server did not start in time at %s", addr)
}
