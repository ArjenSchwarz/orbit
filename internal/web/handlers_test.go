package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arjenschwarz/orbit/internal/registry"
)

func TestHandleDashboard(t *testing.T) {
	reg, err := registry.New(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	// Create a test run
	entry := registry.NewRunEntry()
	entry.Name = "test-run"
	entry.Repository = "owner/repo"
	entry.LogDir = t.TempDir()
	entry.Status = registry.StatusCompleted
	entry.StartedAt = time.Now().Add(-1 * time.Hour)
	entry.Branch = "main"
	if err := reg.Register(entry); err != nil {
		t.Fatalf("failed to register entry: %v", err)
	}

	server := New(Config{
		Port:     8080,
		Bind:     "localhost",
		Registry: reg,
	})

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	server.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "test-run") {
		t.Error("expected response to contain run name")
	}
	if !strings.Contains(body, "owner/repo") {
		t.Error("expected response to contain repository")
	}
}

func TestHandleDashboardEmpty(t *testing.T) {
	reg, err := registry.New(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	server := New(Config{
		Port:     8080,
		Bind:     "localhost",
		Registry: reg,
	})

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	server.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "No runs registered") {
		t.Error("expected empty state message")
	}
}

func TestHandleRunDetail(t *testing.T) {
	regDir := t.TempDir()
	reg, err := registry.New(regDir)
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	// Create log directory
	logDir := t.TempDir()

	// Create a test run
	entry := registry.NewRunEntry()
	entry.Name = "test-run"
	entry.Repository = "owner/repo"
	entry.LogDir = logDir
	entry.Status = registry.StatusCompleted
	entry.StartedAt = time.Now().Add(-1 * time.Hour)
	finishedAt := time.Now()
	entry.FinishedAt = &finishedAt
	entry.Branch = "feature/test"
	entry.Phases = []registry.Phase{
		{Number: 1, Status: registry.PhaseStatusCompleted, RunCount: 1},
		{Number: 2, Status: registry.PhaseStatusCompleted, RunCount: 1},
	}
	if err := reg.Register(entry); err != nil {
		t.Fatalf("failed to register entry: %v", err)
	}

	server := New(Config{
		Port:     8080,
		Bind:     "localhost",
		Registry: reg,
	})

	req := httptest.NewRequest("GET", "/runs/"+entry.ID, nil)
	req.SetPathValue("id", entry.ID)
	rec := httptest.NewRecorder()

	server.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "test-run") {
		t.Error("expected response to contain run name")
	}
	if !strings.Contains(body, "feature/test") {
		t.Error("expected response to contain branch")
	}
	if !strings.Contains(body, "Phase 1") {
		t.Error("expected response to contain Phase 1")
	}
}

func TestHandleRunDetailNotFound(t *testing.T) {
	reg, err := registry.New(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	server := New(Config{
		Port:     8080,
		Bind:     "localhost",
		Registry: reg,
	})

	// Valid UUID but not in registry
	validID := "550e8400-e29b-41d4-a716-446655440000"
	req := httptest.NewRequest("GET", "/runs/"+validID, nil)
	req.SetPathValue("id", validID)
	rec := httptest.NewRecorder()

	server.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}
}

func TestHandleRunDetailMissingLogDir(t *testing.T) {
	reg, err := registry.New(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	// Create a test run with nonexistent log dir
	entry := registry.NewRunEntry()
	entry.Name = "test-run"
	entry.Repository = "owner/repo"
	entry.LogDir = "/nonexistent/path"
	entry.Status = registry.StatusCompleted
	entry.StartedAt = time.Now()
	entry.Branch = "main"
	if err := reg.Register(entry); err != nil {
		t.Fatalf("failed to register entry: %v", err)
	}

	server := New(Config{
		Port:     8080,
		Bind:     "localhost",
		Registry: reg,
	})

	req := httptest.NewRequest("GET", "/runs/"+entry.ID, nil)
	req.SetPathValue("id", entry.ID)
	rec := httptest.NewRecorder()

	server.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "no longer exists") {
		t.Error("expected missing log directory message")
	}
}

func TestHandleTranscript(t *testing.T) {
	reg, err := registry.New(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	// Create log directory with a transcript file
	logDir := t.TempDir()
	transcriptFile := filepath.Join(logDir, "phase-1-transcript.jsonl")
	transcriptContent := `{"type":"summary","cwd":"/test","session_id":"test-session"}
{"type":"user","message":{"role":"user","content":[{"type":"text","text":"Hello"}]}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Hi there!"}]}}`
	if err := os.WriteFile(transcriptFile, []byte(transcriptContent), 0644); err != nil {
		t.Fatalf("failed to write transcript: %v", err)
	}

	// Create a test run
	entry := registry.NewRunEntry()
	entry.Name = "test-run"
	entry.Repository = "owner/repo"
	entry.LogDir = logDir
	entry.Status = registry.StatusCompleted
	entry.StartedAt = time.Now()
	entry.Branch = "main"
	entry.Phases = []registry.Phase{
		{Number: 1, Status: registry.PhaseStatusCompleted, RunCount: 1},
	}
	if err := reg.Register(entry); err != nil {
		t.Fatalf("failed to register entry: %v", err)
	}

	server := New(Config{
		Port:     8080,
		Bind:     "localhost",
		Registry: reg,
	})

	req := httptest.NewRequest("GET", "/runs/"+entry.ID+"/transcript/1", nil)
	req.SetPathValue("id", entry.ID)
	req.SetPathValue("phase", "1")
	rec := httptest.NewRecorder()

	server.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Phase 1") {
		t.Error("expected response to contain phase number")
	}
}

func TestHandleTranscriptNotFound(t *testing.T) {
	reg, err := registry.New(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	// Create log directory without transcript
	logDir := t.TempDir()

	// Create a test run
	entry := registry.NewRunEntry()
	entry.Name = "test-run"
	entry.Repository = "owner/repo"
	entry.LogDir = logDir
	entry.Status = registry.StatusCompleted
	entry.StartedAt = time.Now()
	entry.Branch = "main"
	if err := reg.Register(entry); err != nil {
		t.Fatalf("failed to register entry: %v", err)
	}

	server := New(Config{
		Port:     8080,
		Bind:     "localhost",
		Registry: reg,
	})

	req := httptest.NewRequest("GET", "/runs/"+entry.ID+"/transcript/1", nil)
	req.SetPathValue("id", entry.ID)
	req.SetPathValue("phase", "1")
	rec := httptest.NewRecorder()

	server.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}
}

func TestHandleTranscriptInvalidPhase(t *testing.T) {
	reg, err := registry.New(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	// Create a test run
	entry := registry.NewRunEntry()
	entry.Name = "test-run"
	entry.Repository = "owner/repo"
	entry.LogDir = t.TempDir()
	entry.Status = registry.StatusCompleted
	entry.StartedAt = time.Now()
	entry.Branch = "main"
	if err := reg.Register(entry); err != nil {
		t.Fatalf("failed to register entry: %v", err)
	}

	server := New(Config{
		Port:     8080,
		Bind:     "localhost",
		Registry: reg,
	})

	// Test with non-numeric phase
	req := httptest.NewRequest("GET", "/runs/"+entry.ID+"/transcript/abc", nil)
	req.SetPathValue("id", entry.ID)
	req.SetPathValue("phase", "abc")
	rec := httptest.NewRecorder()

	server.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}
}

func TestHandleStaticFiles(t *testing.T) {
	reg, err := registry.New(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	server := New(Config{
		Port:     8080,
		Bind:     "localhost",
		Registry: reg,
	})

	tests := []struct {
		path        string
		contentType string
	}{
		{"/static/htmx.min.js", "application/javascript"},
		{"/static/style.css", "text/css"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			rec := httptest.NewRecorder()

			server.router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("expected status 200, got %d", rec.Code)
			}

			contentType := rec.Header().Get("Content-Type")
			if !strings.HasPrefix(contentType, tt.contentType) {
				t.Errorf("expected content-type %s, got %s", tt.contentType, contentType)
			}
		})
	}
}
