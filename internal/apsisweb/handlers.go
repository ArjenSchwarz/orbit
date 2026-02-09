package apsisweb

import (
	"fmt"
	"html/template"
	"net/http"
	"slices"

	"github.com/arjenschwarz/orbit/internal/sessions"
	"github.com/arjenschwarz/orbit/internal/transcript"
)

// handleSessionList renders the session list page.
func (s *Server) handleSessionList(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		s.handleNotFound(w, r)
		return
	}

	data, err := s.buildSessionListData()
	if err != nil {
		s.renderError(w, http.StatusInternalServerError, "Failed to list sessions")
		return
	}
	s.renderTemplate(w, "sessions.html", data)
}

// handleSessionListFragment renders the session list fragment for HTMX polling.
func (s *Server) handleSessionListFragment(w http.ResponseWriter, r *http.Request) {
	data, err := s.buildSessionListData()
	if err != nil {
		http.Error(w, "Failed to list sessions", http.StatusInternalServerError)
		return
	}
	s.renderFragment(w, "sessions_list", data)
}

// handleTranscript renders the transcript page.
func (s *Server) handleTranscript(w http.ResponseWriter, r *http.Request) {
	source := r.PathValue("source")
	id := r.PathValue("id")

	resolved, err := s.resolver.Resolve(source, id)
	if err != nil {
		s.handleNotFound(w, r)
		return
	}
	defer func() { _ = resolved.Reader.Close() }()

	// Check file size (50MB limit)
	const maxSize = 50 * 1024 * 1024
	if resolved.Metadata.Size > maxSize {
		s.renderError(w, http.StatusRequestEntityTooLarge,
			"This transcript is too large to render in the browser. Use the CLI instead: apsis "+id)
		return
	}

	// Parse transcript — thread cost path for Kiro IDE sessions
	var result *transcript.ParseResult
	if resolved.Metadata.CostPath != "" {
		opts := transcript.ParseOptions{KiroIDECostPath: resolved.Metadata.CostPath}
		result, err = transcript.ParseJSONLWithFormat(resolved.Reader, transcript.FormatKiroIDE, opts)
	} else {
		result, err = transcript.Parse(resolved.Reader)
	}
	if err != nil {
		s.renderError(w, http.StatusInternalServerError, "Failed to parse transcript")
		return
	}

	opts := transcript.RenderOptions{
		SessionID: id,
	}
	if result.Metadata != nil {
		opts.TotalCost = result.Metadata.TotalCost
		opts.CostUnit = result.Metadata.CostUnit
	}
	content := transcript.RenderHTMLFragment(result.Entries, opts)

	data := TranscriptViewData{
		TemplateData: TemplateData{Title: "Transcript", CSSVersion: CSSVersion},
		SessionID:    id,
		Source:       source,
		Content:      template.HTML(content),
		CreatedAt:    resolved.Metadata.CreatedAt.Format("Jan 2, 2006 3:04 PM"),
		Size:         sessions.FormatSize(resolved.Metadata.Size),
	}
	s.renderTemplate(w, "transcript.html", data)
}

// handleTranscriptCSS serves the transcript CSS from the transcript package.
func (s *Server) handleTranscriptCSS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = fmt.Fprint(w, transcript.TranscriptCSS())
}

// buildSessionListData builds the session list data shared by full page and HTMX fragment.
func (s *Server) buildSessionListData() (SessionListData, error) {
	sessionList, warnings, err := s.lister.ListAll(s.config.ProjectPath)
	if err != nil {
		return SessionListData{}, err
	}

	var warningMessages []string
	for _, w := range warnings {
		warningMessages = append(warningMessages,
			fmt.Sprintf("Could not load %s sessions: %s", sessions.DisplayName(w.Source), w.Err))
	}

	// Reverse to newest-first for web display
	slices.Reverse(sessionList)

	var views []SessionView
	for _, si := range sessionList {
		displayID := si.ID
		if len(displayID) > 40 {
			displayID = displayID[:37] + "..."
		}
		views = append(views, SessionView{
			ID:          si.ID,
			DisplayID:   displayID,
			Source:      si.Source,
			SourceClass: "source-" + si.Source,
			CreatedAt:   si.CreatedAt.Format("Jan 2, 2006 3:04 PM"),
			Size:        sessions.FormatSize(si.Size),
			URL:         fmt.Sprintf("/sessions/%s/%s", si.Source, si.ID),
		})
	}

	return SessionListData{
		TemplateData: TemplateData{Title: "Sessions", CSSVersion: CSSVersion},
		Sessions:     views,
		Warnings:     warningMessages,
		Sources:      sessions.AllSources(),
		Empty:        len(views) == 0,
	}, nil
}

// renderError renders an error page with the given HTTP status code.
func (s *Server) renderError(w http.ResponseWriter, code int, message string) {
	w.WriteHeader(code)
	s.renderTemplate(w, "error.html", ErrorData{
		TemplateData: TemplateData{Title: "Error", CSSVersion: CSSVersion},
		Code:         code,
		Message:      message,
	})
}

// handleNotFound renders the 404 page.
func (s *Server) handleNotFound(w http.ResponseWriter, _ *http.Request) {
	s.renderError(w, http.StatusNotFound, "Page not found")
}
