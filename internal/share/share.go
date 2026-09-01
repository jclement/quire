// Package share serves public read-only share pages at /s/<token> — clean
// standalone HTML, no SPA, no auth. A share exposes exactly one document
// plus the attachments that document references. This is the only surface
// Tailscale Funnel exposes to the public internet.
package share

import (
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jclement/quire/internal/auth"
	"github.com/jclement/quire/internal/service"
)

// Manager creates and serves shares. baseURL is where share links point —
// updated at runtime once the funnel hostname is known.
type Manager struct {
	Auth    *auth.Store
	Service *service.Service
	baseURL atomic.Value // string
}

// NewManager returns a Manager building share URLs against baseURL.
func NewManager(authStore *auth.Store, svc *service.Service, baseURL string) *Manager {
	m := &Manager{Auth: authStore, Service: svc}
	m.baseURL.Store(strings.TrimRight(baseURL, "/"))
	return m
}

// SetBaseURL swaps the URL shares are advertised at (e.g. the funnel DNS
// name once tsnet is up).
func (m *Manager) SetBaseURL(u string) { m.baseURL.Store(strings.TrimRight(u, "/")) }

// ShareInfo is a share plus its public URL.
type ShareInfo struct {
	auth.Share
	URL string `json:"url"`
}

func (m *Manager) info(sh auth.Share) ShareInfo {
	return ShareInfo{Share: sh, URL: m.baseURL.Load().(string) + "/s/" + sh.Token}
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
	if err != nil || !strings.Contains(doc.Markdown, rel) {
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
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
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
