// The /api/v1 handlers. Thin: parse → service call → envelope.
package api

import (
	"net/http"
	"strconv"

	"github.com/jclement/quire/internal/service"
	"github.com/jclement/quire/internal/vault"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeData(w, http.StatusOK, map[string]any{
		"status":           "ok",
		"version":          s.Version,
		"update_available": false,
	})
}

// ---- documents ----

func (s *Server) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	docs, err := s.Service.ListDocuments(r.URL.Query().Get("type"), r.URL.Query().Get("q"), limit)
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
	doc, err := s.Service.CreateDocument(docType, body.Title, body.Markdown)
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

// ---- search ----

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	hits, err := s.Service.Search(r.URL.Query().Get("q"), limit)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, hits)
}

// ---- tasks ----

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	view := r.URL.Query().Get("view")
	if view == "" {
		view = "today"
	}
	tasks, err := s.Service.Tasks(view)
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
	}
	if !decodeBody(w, r, &body) {
		return
	}
	task, err := s.Service.CreateTask(body.Text, body.Due, body.Defer)
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

func (s *Server) handleToday(w http.ResponseWriter, r *http.Request) {
	payload, err := s.Service.Today()
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, payload)
}
