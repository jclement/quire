// Package api is the HTTP layer: routing, the response envelope, and
// translating service errors to status codes. Handlers hold no SQL and no
// filesystem access — everything goes through the service layer.
package api

import (
	_ "embed"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jclement/quire/internal/auth"
	"github.com/jclement/quire/internal/service"
	"github.com/jclement/quire/internal/share"
	"github.com/jclement/quire/internal/vault"
)

//go:embed openapi.yaml
var openAPISpec []byte

// Server carries the API's dependencies.
type Server struct {
	Service *service.Service
	Events  *Broadcaster
	Shares  *share.Manager
	// Auth backs the credential-management routes. Nil in tests that only
	// exercise document handlers, so those routes register only when set.
	Auth    *auth.Store
	Version string
	// UpdateCheck reports whether a newer release exists; nil means the
	// check is disabled and health honestly says false.
	UpdateCheck func() bool
}

// Routes registers all /api/v1 handlers onto mux.
func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	mux.HandleFunc("GET /api/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write(openAPISpec)
	})

	mux.HandleFunc("GET /api/v1/documents", s.handleListDocuments)
	mux.HandleFunc("POST /api/v1/documents", s.handleCreateDocument)
	mux.HandleFunc("GET /api/v1/documents/{path...}", s.handleGetDocument)
	mux.HandleFunc("PUT /api/v1/documents/{path...}", s.handleUpdateDocument)
	mux.HandleFunc("DELETE /api/v1/documents/{path...}", s.handleDeleteDocument)

	mux.HandleFunc("POST /api/v1/rename", s.handleRenameDocument)
	mux.HandleFunc("PATCH /api/v1/documents/{path...}", s.handleSetFrontmatter)
	mux.HandleFunc("POST /api/v1/link", s.handleLink)

	mux.HandleFunc("GET /api/v1/search", s.handleSearch)

	mux.HandleFunc("GET /api/v1/tasks", s.handleListTasks)
	mux.HandleFunc("POST /api/v1/tasks", s.handleCreateTask)
	mux.HandleFunc("POST /api/v1/tasks/{id}/toggle", s.handleToggleTask)
	mux.HandleFunc("PATCH /api/v1/tasks/{id}", s.handleEditTask)

	mux.HandleFunc("GET /api/v1/daily/{date}", s.handleGetDaily)
	mux.HandleFunc("POST /api/v1/daily/{date}", s.handleEnsureDaily)
	mux.HandleFunc("GET /api/v1/today", s.handleToday)
	mux.HandleFunc("GET /api/v1/calendar", s.handleCalendar)

	mux.HandleFunc("GET /api/v1/agent-guidance", s.handleGetGuidance)
	mux.HandleFunc("PUT /api/v1/agent-guidance", s.handleSetGuidance)

	mux.HandleFunc("POST /api/v1/attachments", s.handleUploadAttachment)
	mux.HandleFunc("POST /api/v1/capture", s.handleCapture)
	mux.HandleFunc("GET /api/v1/files/{path...}", s.handleServeFile)

	mux.HandleFunc("GET /api/v1/events", s.handleEvents)

	if s.Shares != nil {
		mux.HandleFunc("GET /api/v1/shares", s.handleListShares)
		mux.HandleFunc("POST /api/v1/shares", s.handleCreateShare)
		mux.HandleFunc("DELETE /api/v1/shares/{token}", s.handleRevokeShare)
	}

	if s.Auth != nil {
		mux.HandleFunc("GET /api/v1/tokens", s.handleListTokens)
		mux.HandleFunc("POST /api/v1/tokens", s.handleCreateToken)
		mux.HandleFunc("DELETE /api/v1/tokens/{prefix}", s.handleRevokeToken)
		mux.HandleFunc("GET /api/v1/connected-apps", s.handleListConnectedApps)
		mux.HandleFunc("DELETE /api/v1/connected-apps/{id}", s.handleDisconnectApp)
	}
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
