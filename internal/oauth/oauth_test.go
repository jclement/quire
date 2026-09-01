package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jclement/quire/internal/auth"
	"github.com/jclement/quire/internal/config"
)

// newTestServer wires the OAuth server the way main does, with an isOwner
// that trusts the tailnet identity header (as the gate would inject it).
func newTestServer(t *testing.T) (*Server, *http.ServeMux, *auth.Store) {
	t.Helper()
	store, err := auth.Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.DB.Close() })

	isOwner := func(r *http.Request) bool { return r.Header.Get(auth.HeaderTailnetLogin) != "" }
	s := New(store, "https://quire.example.ts.net", isOwner)
	mux := http.NewServeMux()
	s.Routes(mux)
	return s, mux, store
}

func postForm(mux *http.ServeMux, path string, form url.Values, owner bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if owner {
		req.Header.Set(auth.HeaderTailnetLogin, "jeff@example.com")
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// TestFullConnectorFlow walks the exact dance a claude.ai connector does:
// metadata discovery → DCR → authorize + consent → code exchange with PKCE →
// authenticated request → refresh rotation → reuse detection.
func TestFullConnectorFlow(t *testing.T) {
	s, mux, store := newTestServer(t)

	// 1. Discovery.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/.well-known/oauth-authorization-server", nil))
	var meta map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &meta); err != nil || meta["issuer"] != "https://quire.example.ts.net" {
		t.Fatalf("metadata = %d %s", rec.Code, rec.Body.String())
	}

	// 2. Dynamic client registration.
	regBody := `{"client_name":"Claude","redirect_uris":["https://claude.ai/api/mcp/auth_callback"]}`
	req := httptest.NewRequest("POST", "/oauth/register", strings.NewReader(regBody))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != 201 {
		t.Fatalf("register = %d %s", rec.Code, rec.Body.String())
	}
	var client struct {
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &client); err != nil || client.ClientID == "" {
		t.Fatal("no client_id")
	}

	// 3. Authorize: PKCE pair, consent page, approval.
	verifier := "test-verifier-string-that-is-long-enough-for-pkce"
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	authorizeURL := "/oauth/authorize?response_type=code&client_id=" + client.ClientID +
		"&redirect_uri=" + url.QueryEscape("https://claude.ai/api/mcp/auth_callback") +
		"&state=xyz&code_challenge=" + challenge + "&code_challenge_method=S256&scope=read+write+tasks"

	// Unauthenticated (funnel) → the "approve from your tailnet" page.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", authorizeURL, nil))
	if rec.Code != 403 || !strings.Contains(rec.Body.String(), "tailnet") {
		t.Fatalf("unauthenticated authorize = %d", rec.Code)
	}

	// Owner (tailnet) → consent form with a nonce.
	req = httptest.NewRequest("GET", authorizeURL, nil)
	req.Header.Set(auth.HeaderTailnetLogin, "jeff@example.com")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Claude") {
		t.Fatalf("consent form = %d", rec.Code)
	}
	nonce := regexp.MustCompile(`name="nonce" value="([0-9a-f]+)"`).FindStringSubmatch(rec.Body.String())
	if nonce == nil {
		t.Fatal("no nonce in consent form")
	}

	// Approve.
	rec = postForm(mux, "/oauth/authorize", url.Values{
		"nonce": {nonce[1]}, "client_id": {client.ClientID},
		"redirect_uri": {"https://claude.ai/api/mcp/auth_callback"},
		"state":        {"xyz"}, "code_challenge": {challenge},
		"scope": {"read write tasks"}, "decision": {"approve"},
	}, true)
	if rec.Code != 302 {
		t.Fatalf("approve = %d %s", rec.Code, rec.Body.String())
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil || loc.Query().Get("state") != "xyz" || loc.Query().Get("code") == "" {
		t.Fatalf("redirect = %q", rec.Header().Get("Location"))
	}
	code := loc.Query().Get("code")

	// 4. Token exchange with the PKCE verifier.
	rec = postForm(mux, "/oauth/token", url.Values{
		"grant_type": {"authorization_code"}, "code": {code},
		"client_id":     {client.ClientID},
		"redirect_uri":  {"https://claude.ai/api/mcp/auth_callback"},
		"code_verifier": {verifier},
	}, false)
	if rec.Code != 200 {
		t.Fatalf("token = %d %s", rec.Code, rec.Body.String())
	}
	var tokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tokens); err != nil ||
		!strings.HasPrefix(tokens.AccessToken, "oaq_") || tokens.TokenType != "Bearer" {
		t.Fatalf("token body = %s", rec.Body.String())
	}

	// Codes are single-use.
	rec = postForm(mux, "/oauth/token", url.Values{
		"grant_type": {"authorization_code"}, "code": {code},
		"client_id":     {client.ClientID},
		"redirect_uri":  {"https://claude.ai/api/mcp/auth_callback"},
		"code_verifier": {verifier},
	}, false)
	if rec.Code != 400 {
		t.Errorf("code reuse = %d", rec.Code)
	}

	// 5. The access token authenticates like any bearer, with its scopes.
	apiReq := httptest.NewRequest("GET", "/api/v1/documents", nil)
	apiReq.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	principal, err := store.BearerPrincipal(apiReq)
	if err != nil || !principal.Allows(auth.ScopeWrite) || !strings.HasPrefix(principal.Name, "oauth:") {
		t.Fatalf("oauth bearer principal = %+v, %v", principal, err)
	}
	// And it passes the standard middleware in token-only mode.
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mw := store.Middleware(config.AuthTokenOnly, s.WWWAuthenticate(), ok)
	rec = httptest.NewRecorder()
	mw.ServeHTTP(rec, apiReq)
	if rec.Code != 200 {
		t.Errorf("oauth token through middleware = %d", rec.Code)
	}

	// 6. Refresh rotation: new pair; old refresh works only briefly (grace),
	// and the rotated-out access token is replaced.
	rec = postForm(mux, "/oauth/token", url.Values{
		"grant_type": {"refresh_token"}, "refresh_token": {tokens.RefreshToken},
	}, false)
	if rec.Code != 200 {
		t.Fatalf("refresh = %d %s", rec.Code, rec.Body.String())
	}
	var rotated struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &rotated)
	if rotated.RefreshToken == tokens.RefreshToken || rotated.AccessToken == tokens.AccessToken {
		t.Errorf("rotation reused credentials")
	}

	// 7. Revocation kills the grant.
	rec = postForm(mux, "/oauth/revoke", url.Values{"token": {rotated.AccessToken}}, false)
	if rec.Code != 200 {
		t.Fatalf("revoke = %d", rec.Code)
	}
	apiReq.Header.Set("Authorization", "Bearer "+rotated.AccessToken)
	if _, err := store.BearerPrincipal(apiReq); err == nil {
		t.Errorf("revoked access token still valid")
	}
}

func TestAuthorizeRejectsBadRequests(t *testing.T) {
	_, mux, store := newTestServer(t)
	client, err := store.RegisterOAuthClient("Test", []string{"https://ok.example/cb"})
	if err != nil {
		t.Fatal(err)
	}

	get := func(query string) int {
		req := httptest.NewRequest("GET", "/oauth/authorize?"+query, nil)
		req.Header.Set(auth.HeaderTailnetLogin, "jeff@example.com")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}

	base := "response_type=code&client_id=" + client.ID + "&code_challenge=abc&code_challenge_method=S256"
	// Unregistered redirect must never be redirected to.
	if got := get(base + "&redirect_uri=" + url.QueryEscape("https://evil.example/cb")); got != 400 {
		t.Errorf("unregistered redirect = %d", got)
	}
	// PKCE is mandatory.
	if got := get("response_type=code&client_id=" + client.ID + "&redirect_uri=" + url.QueryEscape("https://ok.example/cb")); got != 400 {
		t.Errorf("missing PKCE = %d", got)
	}
	// Unknown scope rejected.
	if got := get(base + "&redirect_uri=" + url.QueryEscape("https://ok.example/cb") + "&scope=admin"); got != 400 {
		t.Errorf("bad scope = %d", got)
	}
}

func TestRegisterValidatesRedirects(t *testing.T) {
	_, mux, _ := newTestServer(t)
	for body, want := range map[string]int{
		`{"client_name":"x","redirect_uris":["https://ok.example/cb"]}`:  201,
		`{"client_name":"x","redirect_uris":["http://localhost:33418"]}`: 201, // native loopback
		`{"client_name":"x","redirect_uris":["http://evil.example/cb"]}`: 400, // plain http, non-loopback
		`{"client_name":"x","redirect_uris":[]}`:                         400,
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", "/oauth/register", strings.NewReader(body)))
		if rec.Code != want {
			t.Errorf("register %s = %d, want %d", body, rec.Code, want)
		}
	}
}
