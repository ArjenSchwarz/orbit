package web

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/arjenschwarz/orbit/internal/registry"
)

func TestNew(t *testing.T) {
	reg, err := registry.New(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	config := Config{
		Port:     8080,
		Bind:     "localhost",
		Registry: reg,
	}

	server := New(config)
	if server == nil {
		t.Fatal("expected non-nil server")
	}

	if server.config.Port != 8080 {
		t.Errorf("expected port 8080, got %d", server.config.Port)
	}

	if server.config.Bind != "localhost" {
		t.Errorf("expected bind localhost, got %s", server.config.Bind)
	}

	if server.registry != reg {
		t.Error("expected registry to be set")
	}
}

func TestServerStartStop(t *testing.T) {
	reg, err := registry.New(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	// Find an available port
	port := findAvailablePort(t)

	config := Config{
		Port:     port,
		Bind:     "127.0.0.1",
		Registry: reg,
	}

	server := New(config)

	// Start server in background
	errChan := make(chan error, 1)
	go func() {
		errChan <- server.Start()
	}()

	// Wait for server to be ready
	waitForServer(t, fmt.Sprintf("127.0.0.1:%d", port))

	// Verify server is responding
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil {
		t.Fatalf("failed to connect to server: %v", err)
	}
	_ = resp.Body.Close()

	// Shutdown gracefully
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		t.Errorf("shutdown error: %v", err)
	}

	// Wait for Start to return
	select {
	case err := <-errChan:
		if err != nil && err != http.ErrServerClosed {
			t.Errorf("unexpected server error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("server did not stop in time")
	}
}

func TestServerPortConflict(t *testing.T) {
	reg, err := registry.New(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	// Find an available port and occupy it
	port := findAvailablePort(t)
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("failed to occupy port: %v", err)
	}
	defer func() { _ = listener.Close() }()

	config := Config{
		Port:     port,
		Bind:     "127.0.0.1",
		Registry: reg,
	}

	server := New(config)

	// Start should fail with port in use error
	err = server.Start()
	if err == nil {
		t.Error("expected error when port is in use")
	}
}

func TestServerGracefulShutdown(t *testing.T) {
	reg, err := registry.New(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	port := findAvailablePort(t)

	config := Config{
		Port:     port,
		Bind:     "127.0.0.1",
		Registry: reg,
	}

	server := New(config)

	// Add a slow handler for testing graceful shutdown
	server.router.HandleFunc("GET /slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("done"))
	})

	// Start server
	errChan := make(chan error, 1)
	go func() {
		errChan <- server.Start()
	}()

	waitForServer(t, fmt.Sprintf("127.0.0.1:%d", port))

	// Start a slow request
	respChan := make(chan *http.Response, 1)
	go func() {
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/slow", port))
		if err != nil {
			t.Logf("request error: %v", err)
			respChan <- nil
			return
		}
		respChan <- resp
	}()

	// Give the request time to start
	time.Sleep(100 * time.Millisecond)

	// Initiate shutdown (should wait for in-flight request)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	shutdownStart := time.Now()
	if err := server.Shutdown(ctx); err != nil {
		t.Errorf("shutdown error: %v", err)
	}
	shutdownDuration := time.Since(shutdownStart)

	// Shutdown should have waited for the slow request
	if shutdownDuration < 400*time.Millisecond {
		t.Errorf("shutdown was too fast (%v), should have waited for in-flight request", shutdownDuration)
	}

	// Check that the slow request completed
	resp := <-respChan
	if resp == nil {
		t.Log("slow request was interrupted (acceptable in some scenarios)")
	} else {
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
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
