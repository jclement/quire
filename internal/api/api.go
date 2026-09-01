// Package api is the HTTP layer: routing, the response envelope, and
// translating service errors to status codes. Handlers hold no SQL and no
// filesystem access — everything goes through the service layer.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jclement/quire/internal/service"
	"github.com/jclement/quire/internal/vault"
)

// Server carries the API's dependencies.
type Server struct {
	Service *service.Service
	Events  *Broadcaster
	Version string
}

// Routes registers all /api/v1 handlers onto mux.
func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/health", s.handleHealth)

	mux.HandleFunc("GET /api/v1/documents", s.handleListDocuments)
	mux.HandleFunc("POST /api/v1/documents", s.handleCreateDocument)
	mux.HandleFunc("GET /api/v1/documents/{path...}", s.handleGetDocument)
	mux.HandleFunc("PUT /api/v1/documents/{path...}", s.handleUpdateDocument)
	mux.HandleFunc("DELETE /api/v1/documents/{path...}", s.handleDeleteDocument)

	mux.HandleFunc("GET /api/v1/search", s.handleSearch)

	mux.HandleFunc("GET /api/v1/tasks", s.handleListTasks)
	mux.HandleFunc("POST /api/v1/tasks", s.handleCreateTask)
	mux.HandleFunc("POST /api/v1/tasks/{id}/toggle", s.handleToggleTask)

	mux.HandleFunc("GET /api/v1/daily/{date}", s.handleGetDaily)
	mux.HandleFunc("POST /api/v1/daily/{date}", s.handleEnsureDaily)
	mux.HandleFunc("GET /api/v1/today", s.handleToday)

	mux.HandleFunc("POST /api/v1/attachments", s.handleUploadAttachment)
	mux.HandleFunc("GET /api/v1/files/{path...}", s.handleServeFile)

	mux.HandleFunc("GET /api/v1/events", s.handleEvents)
}

// ---- envelope ----

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeData(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]any{"data": data}); err != nil {
		slog.Warn("encoding response", "err", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": errorBody{Code: code, Message: message}})
}

// writeServiceError maps the service layer's sentinel errors onto HTTP
// semantics; anything unrecognized is a 500 with a generic message (never
// leak internals).
func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, vault.ErrNotFound):
		writeError(w, http.StatusNotFound, "NOT_FOUND", "not found")
	case errors.Is(err, vault.ErrConflict):
		writeError(w, http.StatusConflict, "CONFLICT", "the file changed on disk — reload and reapply your edit")
	default:
		slog.Error("request failed", "err", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return false
	}
	return true
}
