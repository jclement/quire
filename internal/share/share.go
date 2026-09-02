// Package share serves public read-only share pages at /s/<token> — clean
// standalone HTML, no SPA, no auth. A share exposes exactly one document
// plus the attachments that document references. Share pages are the one
// part of quire that is deliberately readable without credentials.
package share

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jclement/quire/internal/auth"
	"github.com/jclement/quire/internal/service"
)

// Manager creates and serves shares. baseURL is where share links point: it
// is whatever URL the outside world reaches this instance at, which quire
// cannot discover for itself behind a proxy or tunnel — hence config.
type Manager struct {
	Auth    *auth.Store
	Service *service.Service
	baseURL string
}

// NewManager returns a Manager building share URLs against baseURL.
func NewManager(authStore *auth.Store, svc *service.Service, baseURL string) *Manager {
	return &Manager{Auth: authStore, Service: svc, baseURL: strings.TrimRight(baseURL, "/")}
}

// ShareInfo is re-exported from the service package, which owns every
// API-visible shape (and generates the frontend's types from them).
type ShareInfo = service.ShareInfo

func (m *Manager) info(sh auth.Share) ShareInfo {
	return ShareInfo{
		Token:        sh.Token,
		DocPath:      sh.DocPath,
		URL:          m.baseURL + "/s/" + sh.Token,
		CreatedAt:    sh.CreatedAt,
		ExpiresAt:    sh.ExpiresAt,
		RevokedAt:    sh.RevokedAt,
		ViewCount:    sh.ViewCount,
		LastViewedAt: sh.LastViewedAt,
	}
}

// Create shares the document at path. expiresDays <= 0 means no expiry.
// The document must exist — sharing a typo'd path helps no one.
func (m *Manager) Create(path string, expiresDays int) (ShareInfo, error) {
	if _, err := m.Service.GetDocument(path); err != nil {
		return ShareInfo{}, err
	}
	var ttl time.Duration
	if expiresDays > 0 {
		ttl = time.Duration(expiresDays) * 24 * time.Hour
	}
	sh, err := m.Auth.CreateShare(path, ttl)
	if err != nil {
		return ShareInfo{}, err
	}
	return m.info(sh), nil
}

// List returns all shares with their URLs.
func (m *Manager) List() ([]ShareInfo, error) {
	shares, err := m.Auth.ListShares()
	if err != nil {
		return nil, err
	}
	out := make([]ShareInfo, 0, len(shares))
	for _, sh := range shares {
		out = append(out, m.info(sh))
	}
	return out, nil
}

// Revoke revokes a share token.
func (m *Manager) Revoke(token string) error { return m.Auth.RevokeShare(token) }

// Routes registers the public /s/ handlers on mux.
func (m *Manager) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /s/{token}", m.handlePage)
	mux.HandleFunc("GET /s/{token}/{path...}", m.handleFile)
}

func (m *Manager) handlePage(w http.ResponseWriter, r *http.Request) {
	sh, err := m.Auth.ResolveShare(r.PathValue("token"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	doc, err := m.Service.GetDocument(sh.DocPath)
	if err != nil {
		// The underlying file is gone; the share silently 404s rather than
		// admitting it ever existed.
		http.NotFound(w, r)
		return
	}
	page, err := renderPage(doc.Title, sh.Token, doc.Markdown)
	if err != nil {
		slog.Error("rendering share page", "token", sh.Token, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Robots-Tag", "noindex")
	w.Header().Set("Referrer-Policy", "no-referrer")
	// Share pages are the one surface anonymous strangers render, and they
	// carry no JavaScript at all — so they get a policy that says exactly
	// that. If a markdown-injected script ever survived escaping, this is
	// what stops it running.
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; img-src 'self' data:; style-src 'unsafe-inline'; "+
			"base-uri 'self'; form-action 'none'; frame-ancestors 'none'")
	_, _ = w.Write(page)
}

// handleFile serves an attachment through a share, but only one the shared
// document actually references — a share is a window onto one document, not
// onto the vault.
func (m *Manager) handleFile(w http.ResponseWriter, r *http.Request) {
	sh, err := m.Auth.ResolveShare(r.PathValue("token"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	rel := r.PathValue("path")
	if strings.HasSuffix(rel, ".md") {
		http.NotFound(w, r)
		return
	}
	doc, err := m.Service.GetDocument(sh.DocPath)
	if err != nil || !referencedFiles(doc.Markdown)[rel] {
		http.NotFound(w, r)
		return
	}
	data, err := m.Service.ReadAttachment(rel)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	serveAttachment(w, rel, data)
}

func serveAttachment(w http.ResponseWriter, rel string, data []byte) {
	ctype := contentTypeFor(rel)
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	if ctype == "image/svg+xml" {
		// Same policy as the app's files handler: inline styles and data:
		// fonts/images (an Excalidraw render), nothing that can reach out.
		w.Header().Set("Content-Security-Policy",
			"default-src 'none'; style-src 'unsafe-inline'; font-src data:; img-src data:")
	}
	_, _ = w.Write(data)
}

func contentTypeFor(rel string) string {
	switch strings.ToLower(rel[strings.LastIndex(rel, ".")+1:]) {
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	case "svg":
		return "image/svg+xml"
	case "pdf":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}
