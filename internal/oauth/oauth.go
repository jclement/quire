// Package oauth is quire's OAuth 2.1 authorization server for remote MCP
// clients (claude.ai connectors, Claude Desktop): RFC 8414 metadata, RFC
// 7591 dynamic client registration, PKCE-only authorization code flow with a
// server-rendered consent page, rotating refresh tokens, RFC 7009
// revocation. Public clients only (token_endpoint_auth_method "none").
//
// Consent requires the vault owner: a tailnet-identified request (the gate
// injects the identity header), a passkey session, or loopback auth-none.
// Over funnel, an unauthenticated authorize renders a "open this from your
// tailnet" page instead — MagicDNS means the same URL works there.
package oauth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jclement/quire/internal/auth"
)

// Scopes an OAuth client may request; also the consent default.
var allowedScopes = map[string]bool{
	auth.ScopeRead: true, auth.ScopeWrite: true, auth.ScopeTasks: true, "share": true,
}

const defaultScopes = "read write tasks"

// Server is the authorization-server state.
type Server struct {
	Store *auth.Store
	// IsOwner authenticates the human at the authorize endpoint.
	IsOwner func(r *http.Request) bool

	issuer atomic.Value // string; settable once the tailnet name is known

	// In-flight consent nonces (CSRF): the GET mints one, the POST burns it.
	mu     sync.Mutex
	nonces map[string]time.Time
}

// New builds the server with an initial issuer (updated via SetIssuer when
// the tailnet comes up).
func New(store *auth.Store, issuer string, isOwner func(r *http.Request) bool) *Server {
	s := &Server{Store: store, IsOwner: isOwner, nonces: map[string]time.Time{}}
	s.issuer.Store(strings.TrimRight(issuer, "/"))
	return s
}

// SetIssuer swaps the advertised issuer/origin.
func (s *Server) SetIssuer(issuer string) { s.issuer.Store(strings.TrimRight(issuer, "/")) }

// Issuer returns the current issuer origin.
func (s *Server) Issuer() string { return s.issuer.Load().(string) }

// Routes registers all OAuth endpoints.
func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", s.handleASMetadata)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", s.handleResourceMetadata)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp", s.handleResourceMetadata)
	mux.HandleFunc("POST /oauth/register", s.handleRegister)
	mux.HandleFunc("GET /oauth/authorize", s.handleAuthorizeForm)
	mux.HandleFunc("POST /oauth/authorize", s.handleAuthorizeDecision)
	mux.HandleFunc("POST /oauth/token", s.handleToken)
	mux.HandleFunc("POST /oauth/revoke", s.handleRevoke)
}

// WWWAuthenticate is the challenge value a 401 from /mcp must carry — it is
// the entire OAuth discovery mechanism for connectors.
func (s *Server) WWWAuthenticate() string {
	return fmt.Sprintf(`Bearer resource_metadata=%q`, s.Issuer()+"/.well-known/oauth-protected-resource")
}

// ---- metadata ----

func (s *Server) handleASMetadata(w http.ResponseWriter, r *http.Request) {
	issuer := s.Issuer()
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                issuer,
		"authorization_endpoint":                issuer + "/oauth/authorize",
		"token_endpoint":                        issuer + "/oauth/token",
		"registration_endpoint":                 issuer + "/oauth/register",
		"revocation_endpoint":                   issuer + "/oauth/revoke",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"scopes_supported":                      []string{"read", "write", "tasks", "share"},
	})
}

func (s *Server) handleResourceMetadata(w http.ResponseWriter, r *http.Request) {
	issuer := s.Issuer()
	writeJSON(w, http.StatusOK, map[string]any{
		"resource":              issuer + "/mcp",
		"authorization_servers": []string{issuer},
		"bearer_methods_supported": []string{
			"header",
		},
	})
}

// ---- dynamic client registration (RFC 7591) ----

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ClientName   string   `json:"client_name"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata", "invalid JSON body")
		return
	}
	for _, uri := range body.RedirectURIs {
		if !validRedirectURI(uri) {
			oauthError(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect URIs must be https (or a loopback/custom-scheme native URI)")
			return
		}
	}
	client, err := s.Store.RegisterOAuthClient(body.ClientName, body.RedirectURIs)
	if err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id":                  client.ID,
		"client_name":                client.Name,
		"redirect_uris":              client.RedirectURIs,
		"token_endpoint_auth_method": "none",
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
	})
}

// validRedirectURI: https for web clients; loopback http and custom schemes
// for native apps (OAuth 2.1 §8.4).
func validRedirectURI(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return false
	}
	switch u.Scheme {
	case "https":
		return u.Host != ""
	case "http":
		host := u.Hostname()
		return host == "localhost" || host == "127.0.0.1" || host == "::1"
	default:
		// Custom scheme (com.example.app:/callback) — native clients.
		return !strings.Contains(u.Scheme, " ")
	}
}

// ---- authorize ----

type authorizeRequest struct {
	client      auth.OAuthClient
	redirectURI string
	state       string
	challenge   string
	scopes      string
}

// parseAuthorize validates the query per OAuth 2.1 (PKCE S256 required).
// Errors that can't be safely redirected render locally.
func (s *Server) parseAuthorize(r *http.Request) (authorizeRequest, string) {
	q := r.URL.Query()
	client, err := s.Store.GetOAuthClient(q.Get("client_id"))
	if err != nil {
		return authorizeRequest{}, "unknown client_id"
	}
	redirectURI := q.Get("redirect_uri")
	if !client.AllowsRedirect(redirectURI) {
		return authorizeRequest{}, "redirect_uri is not registered for this client"
	}
	if q.Get("response_type") != "code" {
		return authorizeRequest{}, "response_type must be code"
	}
	if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
		return authorizeRequest{}, "PKCE with S256 is required"
	}
	scopes := strings.Fields(q.Get("scope"))
	if len(scopes) == 0 {
		scopes = strings.Fields(defaultScopes)
	}
	for _, sc := range scopes {
		if !allowedScopes[sc] {
			return authorizeRequest{}, "unsupported scope " + sc
		}
	}
	return authorizeRequest{
		client:      client,
		redirectURI: redirectURI,
		state:       q.Get("state"),
		challenge:   q.Get("code_challenge"),
		scopes:      strings.Join(scopes, " "),
	}, ""
}

func (s *Server) handleAuthorizeForm(w http.ResponseWriter, r *http.Request) {
	req, problem := s.parseAuthorize(r)
	if problem != "" {
		renderPage(w, http.StatusBadRequest, "Can't authorize", template.HTML("<p>"+template.HTMLEscapeString(problem)+"</p>"))
		return
	}
	if !s.IsOwner(r) {
		renderPage(w, http.StatusForbidden, "Approve from your tailnet",
			`<p>This authorization must be approved by the vault owner.</p>
			 <p>Open this same address from a device on your tailnet (the URL is
			 identical — MagicDNS routes you directly), or sign in first if this
			 quire uses passkeys.</p>`)
		return
	}

	nonce := s.mintNonce()
	body := fmt.Sprintf(`
	  <p><strong>%s</strong> is asking to connect to your quire.</p>
	  <p>It will be able to act with these scopes: <code>%s</code>.</p>
	  <form method="post" action="/oauth/authorize">
	    <input type="hidden" name="nonce" value="%s">
	    <input type="hidden" name="client_id" value="%s">
	    <input type="hidden" name="redirect_uri" value="%s">
	    <input type="hidden" name="state" value="%s">
	    <input type="hidden" name="code_challenge" value="%s">
	    <input type="hidden" name="scope" value="%s">
	    <button name="decision" value="approve" class="approve">Approve</button>
	    <button name="decision" value="deny" class="deny">Deny</button>
	  </form>`,
		template.HTMLEscapeString(displayName(req.client)),
		template.HTMLEscapeString(req.scopes),
		nonce,
		template.HTMLEscapeString(req.client.ID),
		template.HTMLEscapeString(req.redirectURI),
		template.HTMLEscapeString(req.state),
		template.HTMLEscapeString(req.challenge),
		template.HTMLEscapeString(req.scopes))
	renderPage(w, http.StatusOK, "Authorize "+displayName(req.client), template.HTML(body))
}

func (s *Server) handleAuthorizeDecision(w http.ResponseWriter, r *http.Request) {
	if !s.IsOwner(r) {
		renderPage(w, http.StatusForbidden, "Not authorized", `<p>Only the vault owner can approve this.</p>`)
		return
	}
	if err := r.ParseForm(); err != nil || !s.burnNonce(r.PostFormValue("nonce")) {
		renderPage(w, http.StatusBadRequest, "Expired", `<p>This consent form expired — start the authorization again.</p>`)
		return
	}

	client, err := s.Store.GetOAuthClient(r.PostFormValue("client_id"))
	redirectURI := r.PostFormValue("redirect_uri")
	if err != nil || !client.AllowsRedirect(redirectURI) {
		renderPage(w, http.StatusBadRequest, "Can't authorize", `<p>Invalid client or redirect.</p>`)
		return
	}
	target, err := url.Parse(redirectURI)
	if err != nil {
		renderPage(w, http.StatusBadRequest, "Can't authorize", `<p>Invalid redirect.</p>`)
		return
	}
	q := target.Query()
	if state := r.PostFormValue("state"); state != "" {
		q.Set("state", state)
	}

	if r.PostFormValue("decision") != "approve" {
		q.Set("error", "access_denied")
		target.RawQuery = q.Encode()
		http.Redirect(w, r, target.String(), http.StatusFound)
		return
	}

	code, err := s.Store.MintOAuthCode(client.ID, redirectURI, r.PostFormValue("code_challenge"), r.PostFormValue("scope"))
	if err != nil {
		slog.Error("minting oauth code", "err", err)
		renderPage(w, http.StatusInternalServerError, "Error", `<p>Something went wrong — try again.</p>`)
		return
	}
	q.Set("code", code)
	target.RawQuery = q.Encode()
	http.Redirect(w, r, target.String(), http.StatusFound)
}

// ---- token ----

func (s *Server) handleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request", "invalid form body")
		return
	}
	var pair auth.OAuthTokenPair
	var err error
	switch r.PostFormValue("grant_type") {
	case "authorization_code":
		scopes, redeemErr := s.Store.RedeemOAuthCode(
			r.PostFormValue("code"),
			r.PostFormValue("client_id"),
			r.PostFormValue("redirect_uri"),
			r.PostFormValue("code_verifier"))
		if redeemErr != nil {
			oauthError(w, http.StatusBadRequest, "invalid_grant", redeemErr.Error())
			return
		}
		pair, err = s.Store.MintOAuthTokens(r.PostFormValue("client_id"), scopes)
	case "refresh_token":
		pair, err = s.Store.RotateOAuthTokens(r.PostFormValue("refresh_token"))
	default:
		oauthError(w, http.StatusBadRequest, "unsupported_grant_type", "use authorization_code or refresh_token")
		return
	}
	if err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_grant", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  pair.AccessToken,
		"token_type":    "Bearer",
		"expires_in":    pair.AccessExpiresIn,
		"refresh_token": pair.RefreshToken,
		"scope":         pair.Scopes,
	})
}

func (s *Server) handleRevoke(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request", "invalid form body")
		return
	}
	s.Store.RevokeOAuthToken(r.PostFormValue("token"))
	w.WriteHeader(http.StatusOK) // RFC 7009: 200 even for unknown tokens
}

// ---- consent nonces ----

func (s *Server) mintNonce() string {
	raw := make([]byte, 16)
	_, _ = rand.Read(raw)
	nonce := hex.EncodeToString(raw)
	s.mu.Lock()
	defer s.mu.Unlock()
	for n, t := range s.nonces {
		if time.Since(t) > 10*time.Minute {
			delete(s.nonces, n)
		}
	}
	s.nonces[nonce] = time.Now()
	return nonce
}

func (s *Server) burnNonce(nonce string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.nonces[nonce]
	delete(s.nonces, nonce)
	return ok && time.Since(t) <= 10*time.Minute
}

// ---- helpers ----

func displayName(c auth.OAuthClient) string {
	if c.Name != "" {
		return c.Name
	}
	return "An application"
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func oauthError(w http.ResponseWriter, status int, code, description string) {
	writeJSON(w, status, map[string]string{"error": code, "error_description": description})
}

var consentTemplate = template.Must(template.New("consent").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex"><title>{{.Title}} · quire</title>
<style>
:root { color-scheme: light dark; }
body { font: 16px/1.6 ui-sans-serif, -apple-system, sans-serif; max-width: 26rem;
  margin: 4rem auto; padding: 0 1.25rem; }
code { background: rgba(125,125,125,.15); padding: .1em .3em; border-radius: 4px; }
button { font: inherit; padding: .5rem 1.5rem; border-radius: 6px; border: 1px solid
  rgba(125,125,125,.4); cursor: pointer; margin-right: .5rem; margin-top: 1rem; }
button.approve { background: #4662d7; border-color: #4662d7; color: white; }
h1 { font-size: 1.3rem; }
footer { margin-top: 3rem; font-size: .8rem; opacity: .6; }
</style></head><body>
<h1>{{.Title}}</h1>
{{.Body}}
<footer>quire</footer>
</body></html>`))

func renderPage(w http.ResponseWriter, status int, title string, body template.HTML) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = consentTemplate.Execute(w, struct {
		Title string
		Body  template.HTML
	}{title, body})
}
