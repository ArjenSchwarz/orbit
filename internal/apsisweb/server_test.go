package apsisweb

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestNewCreatesServerWithConfig(t *testing.T) {
	s, err := New(Config{
		Port:        8081,
		Bind:        "localhost",
		ProjectPath: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if s.config.Port != 8081 {
		t.Errorf("expected port 8081, got %d", s.config.Port)
	}
	if s.config.Bind != "localhost" {
		t.Errorf("expected bind localhost, got %s", s.config.Bind)
	}
	if s.lister == nil {
		t.Error("expected lister to be set")
	}
	if s.resolver == nil {
		t.Error("expected resolver to be set")
	}
}

func TestNewSetsHTTPTimeouts(t *testing.T) {
	s, err := New(Config{
		Port:        0,
		Bind:        "127.0.0.1",
		ProjectPath: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if s.server.ReadHeaderTimeout != 10*time.Second {
		t.Errorf("expected ReadHeaderTimeout 10s, got %v", s.server.ReadHeaderTimeout)
	}
	if s.server.WriteTimeout != 120*time.Second {
		t.Errorf("expected WriteTimeout 120s, got %v", s.server.WriteTimeout)
	}
	if s.server.IdleTimeout != 60*time.Second {
		t.Errorf("expected IdleTimeout 60s, got %v", s.server.IdleTimeout)
	}
}

func TestServerStartAndResponds(t *testing.T) {
	port := findAvailablePort(t)

	s, err := New(Config{
		Port:        port,
		Bind:        "127.0.0.1",
		ProjectPath: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	errChan := make(chan error, 1)
	go func() {
		errChan <- s.Start()
	}()

	waitForServer(t, fmt.Sprintf("127.0.0.1:%d", port))

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Errorf("shutdown error: %v", err)
	}

	select {
	case err := <-errChan:
		if err != nil && err != http.ErrServerClosed {
			t.Errorf("unexpected server error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("server did not stop in time")
	}
}

func TestServerGracefulShutdown(t *testing.T) {
	port := findAvailablePort(t)

	s, err := New(Config{
		Port:        port,
		Bind:        "127.0.0.1",
		ProjectPath: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	// Add a slow handler for testing
	s.router.HandleFunc("GET /slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	errChan := make(chan error, 1)
	go func() {
		errChan <- s.Start()
	}()

	waitForServer(t, fmt.Sprintf("127.0.0.1:%d", port))

	// Start a slow request
	respChan := make(chan *http.Response, 1)
	go func() {
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/slow", port))
		if err != nil {
			respChan <- nil
			return
		}
		respChan <- resp
	}()

	time.Sleep(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	shutdownStart := time.Now()
	if err := s.Shutdown(ctx); err != nil {
		t.Errorf("shutdown error: %v", err)
	}
	shutdownDuration := time.Since(shutdownStart)

	if shutdownDuration < 400*time.Millisecond {
		t.Errorf("shutdown was too fast (%v), should have waited for in-flight request", shutdownDuration)
	}

	resp := <-respChan
	if resp != nil {
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
	}
}

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
