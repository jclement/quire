// Credential management: API tokens and connected OAuth apps. Both were
// reachable only from the CLI (tokens) or not at all (apps) before this —
// which meant there was no way to answer "what currently has access to my
// vault?" from the app itself. That question is the point of these routes.
//
// These endpoints are inside /api/v1 and so are authenticated like any
// other, meaning a token can mint another token. That is intentional and
// matches the CLI: possession of a write credential is already full access.
package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/jclement/quire/internal/auth"
	"github.com/jclement/quire/internal/service"
)

// tokenInfo converts the store's shape to the wire shape.
func tokenInfo(t auth.Token) service.TokenInfo {
	scopes := t.Scopes
	if scopes == nil {
		scopes = []string{}
	}
	return service.TokenInfo{
		ID:         t.ID,
		Name:       t.Name,
		Prefix:     t.Prefix,
		Scopes:     scopes,
		CreatedAt:  t.CreatedAt,
		ExpiresAt:  t.ExpiresAt,
		RevokedAt:  t.RevokedAt,
		LastUsedAt: t.LastUsedAt,
	}
}

func (s *Server) handleListTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := s.Auth.ListTokens()
	if err != nil {
		writeServiceError(w, err)
		return
	}
	out := make([]service.TokenInfo, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, tokenInfo(t))
	}
	writeData(w, http.StatusOK, out)
}

func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string   `json:"name"`
		Scopes      []string `json:"scopes"`
		ExpiresDays int      `json:"expires_in_days"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name is required")
		return
	}
	if len(body.Scopes) == 0 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "at least one scope is required")
		return
	}
	// Reject unknown scopes rather than silently minting a token that grants
	// nothing — a typo'd scope should fail loudly at creation, not later.
	for _, scope := range body.Scopes {
		switch scope {
		case auth.ScopeRead, auth.ScopeWrite, auth.ScopeTasks:
		default:
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "unknown scope "+scope)
			return
		}
	}
	var ttl time.Duration
	if body.ExpiresDays > 0 {
		ttl = time.Duration(body.ExpiresDays) * 24 * time.Hour
	}
	plaintext, token, err := s.Auth.CreateToken(body.Name, body.Scopes, ttl)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusCreated, service.NewToken{Token: tokenInfo(token), Plaintext: plaintext})
}

func (s *Server) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	// Revoke by prefix (what the UI displays), but accept the numeric id too
	// so the route is usable from a script that listed them.
	prefix := r.PathValue("prefix")
	if id, err := strconv.ParseInt(prefix, 10, 64); err == nil {
		tokens, listErr := s.Auth.ListTokens()
		if listErr != nil {
			writeServiceError(w, listErr)
			return
		}
		for _, t := range tokens {
			if t.ID == id {
				prefix = t.Prefix
				break
			}
		}
	}
	if err := s.Auth.RevokeToken(prefix); err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "no active token with that prefix")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListConnectedApps(w http.ResponseWriter, r *http.Request) {
	apps, err := s.Auth.ListConnectedApps()
	if err != nil {
		writeServiceError(w, err)
		return
	}
	out := make([]service.ConnectedApp, 0, len(apps))
	for _, app := range apps {
		out = append(out, service.ConnectedApp{
			ClientID:    app.ClientID,
			Name:        app.Name,
			Scopes:      app.Scopes,
			ConsentedAt: app.ConsentedAt,
			LastUsedAt:  app.LastUsedAt,
			ActiveGrant: app.ActiveGrant,
		})
	}
	writeData(w, http.StatusOK, out)
}

func (s *Server) handleDisconnectApp(w http.ResponseWriter, r *http.Request) {
	if err := s.Auth.DisconnectApp(r.PathValue("id")); err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "no connected app with that id")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
