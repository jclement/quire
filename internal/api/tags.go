// Tags, the journal's history, and the audit log — three small read
// endpoints added together because Settings and the sidebar grew them in
// the same change.
package api

import (
	"net/http"
	"strconv"

	"github.com/jclement/quire/internal/service"
)

func (s *Server) handleListAreas(w http.ResponseWriter, r *http.Request) {
	areas, err := s.Service.Areas()
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, areas)
}

func (s *Server) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	templates, err := s.Service.Templates()
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, templates)
}

// handleInstallStarterTemplates writes the starter set into templates/,
// skipping any that already exist. Explicitly requested, never automatic:
// quire does not drop files into a vault unasked.
func (s *Server) handleInstallStarterTemplates(w http.ResponseWriter, r *http.Request) {
	written, err := s.Service.InstallStarterTemplates()
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"written": written})
}

func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.Service.Tags()
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, tags)
}

// handleListDaily is the journal's pager: existing daily notes before
// ?before=YYYY-MM-DD (default: today), newest first, ?limit at a time.
func (s *Server) handleListDaily(w http.ResponseWriter, r *http.Request) {
	before := r.URL.Query().Get("before")
	if before == "" {
		before = s.Service.Now().Format("2006-01-02")
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	docs, err := s.Service.DailyNotesBefore(before, limit)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, docs)
}

func (s *Server) handleListAudit(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	rows, err := s.Auth.ListAudit(limit)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	out := make([]service.AuditEntry, 0, len(rows))
	for _, rec := range rows {
		out = append(out, service.AuditEntry{
			ID: rec.ID, At: rec.At, Principal: rec.Principal, Action: rec.Action,
			Path: rec.Path, Detail: rec.Detail, OK: rec.OK,
		})
	}
	writeData(w, http.StatusOK, out)
}
