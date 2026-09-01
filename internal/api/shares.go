// Share-management endpoints (the public share pages themselves live in
// internal/share and are mounted at /s/ outside the API).
package api

import (
	"net/http"

	"github.com/jclement/quire/internal/share"
)

func (s *Server) handleListShares(w http.ResponseWriter, r *http.Request) {
	shares, err := s.Shares.List()
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if shares == nil {
		shares = []share.ShareInfo{}
	}
	writeData(w, http.StatusOK, shares)
}

func (s *Server) handleCreateShare(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path        string `json:"path"`
		ExpiresDays int    `json:"expires_in_days"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	info, err := s.Shares.Create(body.Path, body.ExpiresDays)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusCreated, info)
}

func (s *Server) handleRevokeShare(w http.ResponseWriter, r *http.Request) {
	if err := s.Shares.Revoke(r.PathValue("token")); err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "no active share with that token")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
