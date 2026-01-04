package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/arjenschwarz/orbit/internal/logs"
	"github.com/arjenschwarz/orbit/internal/registry"
	"github.com/arjenschwarz/orbit/internal/transcript"
)

//go:embed templates/*
var templatesFS embed.FS

// TemplateData is the base data for all templates.
type TemplateData struct {
	Title      string
	CurrentURL string
}

// DashboardData is passed to dashboard.html.
type DashboardData struct {
	TemplateData
	Repositories []RepositoryGroup
	Empty        bool
}

// RepositoryGroup groups runs by repository.
type RepositoryGroup struct {
	Name string
	Runs []RunSummary
}

// RunSummary is a summary of a run for the dashboard.
type RunSummary struct {
	ID          string
	Name        string
	Branch      string
	Status      string
	StartedAt   string
	startedTime time.Time // unexported, used for sorting
}

// RunDetailData is passed to run_detail.html.
type RunDetailData struct {
	TemplateData
	Run        *registry.RunEntry
	Phases     []PhaseView
	Duration   string
	StartedAt  string
	FinishedAt string
	Summary    *logs.Summary
	Missing    bool
}

// PhaseView represents a phase for display.
type PhaseView struct {
	Number        int
	Status        string
	RunCount      int
	HasTranscript bool
}

// TranscriptData is passed to transcript.html.
type TranscriptData struct {
	TemplateData
	RunID     string
	RunName   string
	Phase     int
	RunNumber int
	Content   template.HTML
	PrevPhase *int
	NextPhase *int
}

// ErrorData is passed to error.html.
type ErrorData struct {
	TemplateData
	Code    int
	Message string
}

// templates holds the parsed templates.
var templates *template.Template

func init() {
	var err error
	templates, err = template.ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		panic("failed to parse templates: " + err.Error())
	}
}

// handleDashboard renders the dashboard page.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	// Only handle exact root path
	if r.URL.Path != "/" {
		s.handleNotFound(w, r)
		return
	}

	entries, err := s.registry.List()
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to load runs")
		return
	}

	data := s.buildDashboardData(entries, r.URL.Path)
	s.renderTemplate(w, "layout.html", "dashboard.html", data)
}

// handleDashboardStatus renders the dashboard status fragment for htmx polling.
func (s *Server) handleDashboardStatus(w http.ResponseWriter, r *http.Request) {
	entries, err := s.registry.List()
	if err != nil {
		http.Error(w, "Failed to load runs", http.StatusInternalServerError)
		return
	}

	data := s.buildDashboardData(entries, r.URL.Path)
	s.renderFragment(w, "dashboard_status.html", data)
}

// buildDashboardData creates dashboard data from registry entries.
func (s *Server) buildDashboardData(entries []*registry.RunEntry, currentURL string) DashboardData {
	if len(entries) == 0 {
		return DashboardData{
			TemplateData: TemplateData{
				Title:      "Dashboard",
				CurrentURL: currentURL,
			},
			Empty: true,
		}
	}

	// Group runs by repository
	repoMap := make(map[string][]RunSummary)
	for _, entry := range entries {
		status := string(entry.Status)

		// Check if log directory exists
		if _, err := os.Stat(entry.LogDir); os.IsNotExist(err) {
			status = "missing"
		}

		summary := RunSummary{
			ID:          entry.ID,
			Name:        entry.Name,
			Branch:      entry.Branch,
			Status:      status,
			StartedAt:   entry.StartedAt.Format("Jan 2, 15:04"),
			startedTime: entry.StartedAt,
		}
		repoMap[entry.Repository] = append(repoMap[entry.Repository], summary)
	}

	// Sort runs within each repository by start time (newest first)
	for repo := range repoMap {
		sort.Slice(repoMap[repo], func(i, j int) bool {
			return repoMap[repo][i].startedTime.After(repoMap[repo][j].startedTime)
		})
	}

	// Create sorted list of repositories
	var repos []string
	for repo := range repoMap {
		repos = append(repos, repo)
	}
	sort.Strings(repos)

	var groups []RepositoryGroup
	for _, repo := range repos {
		groups = append(groups, RepositoryGroup{
			Name: repo,
			Runs: repoMap[repo],
		})
	}

	return DashboardData{
		TemplateData: TemplateData{
			Title:      "Dashboard",
			CurrentURL: currentURL,
		},
		Repositories: groups,
		Empty:        false,
	}
}

// handleRunDetail renders the run detail page.
func (s *Server) handleRunDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	entry, err := s.registry.Get(id)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to load run")
		return
	}
	if entry == nil {
		s.handleNotFound(w, r)
		return
	}

	data := s.buildRunDetailData(entry, r.URL.Path)
	s.renderTemplate(w, "layout.html", "run_detail.html", data)
}

// handleRunStatus renders the run status fragment for htmx polling.
func (s *Server) handleRunStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	entry, err := s.registry.Get(id)
	if err != nil {
		http.Error(w, "Failed to load run", http.StatusInternalServerError)
		return
	}
	if entry == nil {
		http.NotFound(w, r)
		return
	}

	data := RunDetailData{
		Run: entry,
	}
	s.renderFragment(w, "run_status.html", data)
}

// buildRunDetailData creates run detail data from a registry entry.
func (s *Server) buildRunDetailData(entry *registry.RunEntry, currentURL string) RunDetailData {
	data := RunDetailData{
		TemplateData: TemplateData{
			Title:      entry.Name,
			CurrentURL: currentURL,
		},
		Run:       entry,
		StartedAt: entry.StartedAt.Format("Jan 2, 2006 15:04:05"),
	}

	// Check if log directory exists
	if _, err := os.Stat(entry.LogDir); os.IsNotExist(err) {
		data.Missing = true
	}

	// Calculate duration if finished
	if entry.FinishedAt != nil {
		data.FinishedAt = entry.FinishedAt.Format("Jan 2, 2006 15:04:05")
		duration := entry.FinishedAt.Sub(entry.StartedAt)
		data.Duration = formatDuration(duration)
	}

	// Load summary.json if it exists
	summaryPath := filepath.Join(entry.LogDir, "summary.json")
	if summaryData, err := os.ReadFile(summaryPath); err == nil {
		var summary logs.Summary
		if json.Unmarshal(summaryData, &summary) == nil {
			data.Summary = &summary
		}
	}

	// Build phase views
	for _, phase := range entry.Phases {
		hasTranscript := s.hasTranscriptFile(entry.LogDir, phase.Number)
		data.Phases = append(data.Phases, PhaseView{
			Number:        phase.Number,
			Status:        string(phase.Status),
			RunCount:      phase.RunCount,
			HasTranscript: hasTranscript,
		})
	}

	return data
}

// hasTranscriptFile checks if a transcript file exists for a phase.
func (s *Server) hasTranscriptFile(logDir string, phase int) bool {
	patterns := []string{
		fmt.Sprintf("phase-%d-transcript.jsonl", phase),
		fmt.Sprintf("phase-%d-run-*-transcript.jsonl", phase),
	}

	for _, pattern := range patterns {
		matches, _ := filepath.Glob(filepath.Join(logDir, pattern))
		if len(matches) > 0 {
			return true
		}
	}
	return false
}

// handleTranscript renders the transcript page.
func (s *Server) handleTranscript(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	phaseStr := r.PathValue("phase")
	runStr := r.PathValue("run")

	// Parse phase number
	phase, err := strconv.Atoi(phaseStr)
	if err != nil || phase < 1 {
		s.handleNotFound(w, r)
		return
	}

	// Parse optional run number (default to 1)
	runNumber := 1
	if runStr != "" {
		runNumber, err = strconv.Atoi(runStr)
		if err != nil || runNumber < 1 {
			s.handleNotFound(w, r)
			return
		}
	}

	entry, err := s.registry.Get(id)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to load run")
		return
	}
	if entry == nil {
		s.handleNotFound(w, r)
		return
	}

	// Find transcript file
	transcriptPath := s.findTranscriptFile(entry.LogDir, phase, runNumber)
	if transcriptPath == "" {
		s.handleNotFound(w, r)
		return
	}

	// Validate path is within log directory
	if !isPathWithinDir(transcriptPath, entry.LogDir) {
		s.handleNotFound(w, r)
		return
	}

	// Parse and render transcript
	file, err := os.Open(transcriptPath)
	if err != nil {
		s.handleNotFound(w, r)
		return
	}
	defer func() { _ = file.Close() }()

	result, err := transcript.ParseJSONL(file)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to parse transcript")
		return
	}

	// Build navigation
	var prevPhase, nextPhase *int
	for _, p := range entry.Phases {
		if p.Number == phase-1 {
			prev := p.Number
			prevPhase = &prev
		}
		if p.Number == phase+1 {
			next := p.Number
			nextPhase = &next
		}
	}

	// Render transcript content
	opts := transcript.RenderOptions{
		Title:      fmt.Sprintf("Phase %d Transcript", phase),
		ProjectDir: entry.LogDir,
	}
	content := transcript.RenderHTMLFragment(result.Entries, opts)

	data := TranscriptData{
		TemplateData: TemplateData{
			Title:      fmt.Sprintf("%s - Phase %d", entry.Name, phase),
			CurrentURL: r.URL.Path,
		},
		RunID:     entry.ID,
		RunName:   entry.Name,
		Phase:     phase,
		RunNumber: runNumber,
		Content:   template.HTML(content),
		PrevPhase: prevPhase,
		NextPhase: nextPhase,
	}

	s.renderTemplate(w, "layout.html", "transcript.html", data)
}

// findTranscriptFile finds the transcript file for a phase and run.
func (s *Server) findTranscriptFile(logDir string, phase, runNumber int) string {
	// Try run-specific file first
	runPath := filepath.Join(logDir, fmt.Sprintf("phase-%d-run-%d-transcript.jsonl", phase, runNumber))
	if _, err := os.Stat(runPath); err == nil {
		return runPath
	}

	// Try generic phase file
	phasePath := filepath.Join(logDir, fmt.Sprintf("phase-%d-transcript.jsonl", phase))
	if _, err := os.Stat(phasePath); err == nil {
		return phasePath
	}

	return ""
}

// handleNotFound renders the 404 page.
func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	s.handleError(w, r, http.StatusNotFound, "Page not found")
}

// handleError renders an error page.
func (s *Server) handleError(w http.ResponseWriter, r *http.Request, code int, message string) {
	w.WriteHeader(code)

	data := ErrorData{
		TemplateData: TemplateData{
			Title:      fmt.Sprintf("%d Error", code),
			CurrentURL: r.URL.Path,
		},
		Code:    code,
		Message: message,
	}

	s.renderTemplate(w, "layout.html", "error.html", data)
}

// pageTemplates holds pre-parsed templates for each page.
var pageTemplates = make(map[string]*template.Template)

func init() {
	pages := []string{"dashboard.html", "run_detail.html", "transcript.html", "error.html"}
	for _, page := range pages {
		tmpl := template.Must(template.ParseFS(templatesFS, "templates/layout.html", "templates/"+page))
		pageTemplates[page] = tmpl
	}
}

// renderTemplate renders a template with layout.
func (s *Server) renderTemplate(w http.ResponseWriter, _, page string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	tmpl, ok := pageTemplates[page]
	if !ok {
		http.Error(w, "Template not found: "+page, http.StatusInternalServerError)
		return
	}

	// Execute the layout template, which calls {{template "content" .}}
	if err := tmpl.ExecuteTemplate(w, "layout.html", data); err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

// renderFragment renders a template fragment (no layout).
func (s *Server) renderFragment(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := templates.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)

	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
