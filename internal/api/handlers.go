// The /api/v1 handlers. Thin: parse → service call → envelope.
package api

import (
	"net/http"
	"strconv"

	"github.com/jclement/quire/internal/service"
	"github.com/jclement/quire/internal/vault"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	available := false
	if s.UpdateCheck != nil {
		available = s.UpdateCheck()
	}
	writeData(w, http.StatusOK, service.Health{
		Status:          "ok",
		Version:         s.Version,
		UpdateAvailable: available,
		SemanticSearch:  s.Service.SemanticEnabled(),
		Git:             s.Git,
	})
}

// ---- documents ----

func (s *Server) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	docs, err := s.Service.ListDocuments(r.URL.Query().Get("type"), r.URL.Query().Get("q"), r.URL.Query().Get("area"), limit)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, docs)
}

func (s *Server) handleGetDocument(w http.ResponseWriter, r *http.Request) {
	doc, err := s.Service.GetDocument(r.PathValue("path"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, doc)
}

func (s *Server) handleUpdateDocument(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Markdown   string `json:"markdown"`
		BaseSHA256 string `json:"base_sha256"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	doc, err := s.Service.UpdateDocument(r.PathValue("path"), body.Markdown, body.BaseSHA256)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, doc)
}

func (s *Server) handleCreateDocument(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Type     string `json:"type"`
		Title    string `json:"title"`
		Markdown string `json:"markdown"`
		Area     string `json:"area"`
		Template string `json:"template"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	docType := vault.DocType(body.Type)
	switch docType {
	case vault.TypeNote, vault.TypePerson, vault.TypeCompany, vault.TypeProject, vault.TypeMeeting:
	default:
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid document type")
		return
	}
	if body.Title == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "title is required")
		return
	}
	doc, err := s.Service.CreateDocumentWith(docType, body.Title, body.Markdown, service.CreateOptions{Area: body.Area, Template: body.Template})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusCreated, doc)
}

func (s *Server) handleDeleteDocument(w http.ResponseWriter, r *http.Request) {
	if err := s.Service.DeleteDocument(r.PathValue("path")); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRenameDocument(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path         string `json:"path"`
		NewPath      string `json:"new_path"`
		RewriteLinks bool   `json:"rewrite_links"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	result, err := s.Service.RenameDocument(body.Path, body.NewPath, body.RewriteLinks)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

// handleSetFrontmatter applies surgical frontmatter edits (a null value
// removes the key) — the API behind entity linking and properties editing.
func (s *Server) handleSetFrontmatter(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Set        map[string]any `json:"set"`
		BaseSHA256 string         `json:"base_sha256"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	doc, err := s.Service.SetFrontmatter(r.PathValue("path"), body.Set, body.BaseSHA256)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, doc)
}

// handleLink adds or removes a wikilink in a list-valued frontmatter key:
// attendees on a meeting, people at a company, projects for a person.
func (s *Server) handleLink(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path   string `json:"path"`
		Key    string `json:"key"`
		Target string `json:"target"`
		Remove bool   `json:"remove"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	var doc service.Document
	var err error
	if body.Remove {
		doc, err = s.Service.UnlinkEntity(body.Path, body.Key, body.Target)
	} else {
		doc, err = s.Service.LinkEntity(body.Path, body.Key, body.Target)
	}
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, doc)
}

// ---- search ----

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	q := r.URL.Query()
	if q.Get("mode") == "semantic" {
		hits, err := s.Service.SemanticSearch(r.Context(), q.Get("q"), limit, q.Get("area"))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, hits)
		return
	}
	hits, err := s.Service.Search(q.Get("q"), limit)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, hits)
}

// handleRelated: nearest documents by meaning. ?path= rather than a
// /documents/{path}/related route because {path...} must end the pattern.
func (s *Server) handleRelated(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	hits, err := s.Service.RelatedDocuments(r.URL.Query().Get("path"), limit)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, hits)
}

func (s *Server) handleSemanticStatus(w http.ResponseWriter, _ *http.Request) {
	writeData(w, http.StatusOK, s.Service.SemanticStatus())
}

// ---- tasks ----

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	view := r.URL.Query().Get("view")
	if view == "" {
		view = "today"
	}
	tasks, err := s.Service.TasksIn(view, r.URL.Query().Get("area"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "unknown task view")
		return
	}
	writeData(w, http.StatusOK, tasks)
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text  string `json:"text"`
		Due   string `json:"due"`
		Defer string `json:"defer"`
		// The rest of the grammar, all optional; path targets a document
		// other than today's note.
		Path     string `json:"path"`
		Section  string `json:"section"`
		Priority int    `json:"priority"`
		Waiting  bool   `json:"waiting"`
		Recur    string `json:"recur"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	task, err := s.Service.CreateTaskWith(service.TaskSpec{
		Path: body.Path, Text: body.Text, Due: body.Due, Defer: body.Defer,
		Priority: body.Priority, Waiting: body.Waiting, Recur: body.Recur,
		Section: body.Section,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusCreated, task)
}

func (s *Server) handleEditTask(w http.ResponseWriter, r *http.Request) {
	var edit service.TaskEdit
	if !decodeBody(w, r, &edit) {
		return
	}
	task, err := s.Service.EditTask(r.PathValue("id"), edit)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, task)
}

func (s *Server) handleToggleTask(w http.ResponseWriter, r *http.Request) {
	task, err := s.Service.ToggleTask(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, task)
}

// handleRestoreRecurrence writes the missing next occurrence of a repeating
// task that was completed outside quire.
func (s *Server) handleRestoreRecurrence(w http.ResponseWriter, r *http.Request) {
	task, err := s.Service.RestoreRecurrence(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, task)
}

// ---- daily & today ----

func (s *Server) handleGetDaily(w http.ResponseWriter, r *http.Request) {
	doc, err := s.Service.GetDaily(r.PathValue("date"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, doc)
}

func (s *Server) handleEnsureDaily(w http.ResponseWriter, r *http.Request) {
	doc, err := s.Service.EnsureDaily(r.PathValue("date"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, doc)
}

func (s *Server) handleCalendar(w http.ResponseWriter, r *http.Request) {
	payload, err := s.Service.Calendar(r.URL.Query().Get("month"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}
	writeData(w, http.StatusOK, payload)
}

// Agent guidance is stored as a vault document (AGENTS.md); these endpoints
// hide that filename from the Settings UI and are what MCP composes its
// instructions from.
func (s *Server) handleGetGuidance(w http.ResponseWriter, r *http.Request) {
	writeData(w, http.StatusOK, service.AgentGuidanceResponse{
		Path: service.GuidancePath,
		Text: s.Service.AgentGuidance(),
	})
}

func (s *Server) handleSetGuidance(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if _, err := s.Service.SetAgentGuidance(body.Text); err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, service.AgentGuidanceResponse{
		Path: service.GuidancePath,
		Text: s.Service.AgentGuidance(),
	})
}

func (s *Server) handleToday(w http.ResponseWriter, r *http.Request) {
	payload, err := s.Service.TodayIn(r.URL.Query().Get("area"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, payload)
}
