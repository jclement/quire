package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jclement/quire/internal/config"
)

func TestSessionLifecycle(t *testing.T) {
	s := newStore(t)
	token, err := s.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.SessionPrincipal(token)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Allows(ScopeWrite) {
		t.Errorf("session principal = %+v", p)
	}
	s.DeleteSession(token)
	if _, err := s.SessionPrincipal(token); err == nil {
		t.Errorf("deleted session still valid")
	}
	if _, err := s.SessionPrincipal("bogus"); err == nil {
		t.Errorf("bogus session valid")
	}
}

func TestRecoveryCodes(t *testing.T) {
	s := newStore(t)
	codes, err := s.generateRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != 8 {
		t.Fatalf("codes = %v", codes)
	}
	for _, c := range codes {
		if len(c) != 9 || !strings.Contains(c, "-") {
			t.Errorf("code shape %q", c)
		}
	}

	// Redeem once: works, yields a session. Twice: burned.
	token, err := s.RedeemRecoveryCode(codes[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionPrincipal(token); err != nil {
		t.Errorf("recovery session invalid: %v", err)
	}
	if _, err := s.RedeemRecoveryCode(codes[0]); err == nil {
		t.Errorf("recovery code redeemed twice")
	}
	if _, err := s.RedeemRecoveryCode("nope-nope"); err == nil {
		t.Errorf("invalid code accepted")
	}
}

func TestPasskeyModeMiddleware(t *testing.T) {
	s := newStore(t)
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mw := s.Middleware(config.AuthPasskey, "", ok)

	// No credentials → 401 on protected paths, but auth + SPA + health open.
	get := func(path, cookie string) int {
		req := httptest.NewRequest("GET", path, nil)
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: SessionCookie, Value: cookie})
		}
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)
		return rec.Code
	}
	if got := get("/api/v1/documents", ""); got != 401 {
		t.Errorf("unauthenticated = %d", got)
	}
	if got := get("/api/v1/auth/status", ""); got != 200 {
		t.Errorf("auth endpoint blocked = %d", got)
	}
	if got := get("/api/v1/health", ""); got != 200 {
		t.Errorf("health blocked = %d", got)
	}
	if got := get("/", ""); got != 200 {
		t.Errorf("SPA blocked = %d", got)
	}

	// Session cookie works.
	token, err := s.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	if got := get("/api/v1/documents", token); got != 200 {
		t.Errorf("session request = %d", got)
	}
	if got := get("/api/v1/documents", "wrong"); got != 401 {
		t.Errorf("bad session = %d", got)
	}

	// Bearer token also works in passkey mode (agents), with scopes.
	plaintext, _, err := s.CreateToken("agent", []string{ScopeRead}, 0)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/api/v1/documents", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Errorf("bearer in passkey mode = %d", rec.Code)
	}
}

// Register/begin is open only while no passkey exists (bootstrap), then
// demands a session — the invariant that keeps a fresh instance claimable
// but a claimed one closed.
func TestRegisterBootstrapGating(t *testing.T) {
	s := newStore(t)
	pk, err := NewPasskeys(s, "quire", "quire.example.ts.net", []string{"https://quire.example.ts.net"})
	if err != nil {
		t.Fatal(err)
	}
	h := &HTTPConfig{Passkeys: pk, SecureCookies: true}
	mux := http.NewServeMux()
	h.Routes(mux)

	post := func(path, cookie string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", path, strings.NewReader("{}"))
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: SessionCookie, Value: cookie})
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	// Bootstrap: begin works with no session and returns WebAuthn options.
	rec := post("/api/v1/auth/register/begin", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "challenge") {
		t.Fatalf("bootstrap begin = %d %s", rec.Code, rec.Body.String())
	}

	// Simulate a claimed instance: store a fake passkey row.
	if _, err := s.DB.Exec(`INSERT INTO passkeys (id, name, credential_json, created_at) VALUES ('x', 'test', '{}', 'now')`); err != nil {
		t.Fatal(err)
	}
	if rec := post("/api/v1/auth/register/begin", ""); rec.Code != 401 {
		t.Errorf("claimed instance, no session = %d", rec.Code)
	}
	token, _ := s.CreateSession()
	if rec := post("/api/v1/auth/register/begin", token); rec.Code != 200 {
		t.Errorf("claimed instance with session = %d", rec.Code)
	}

	// Login/begin works on a claimed instance without auth.
	if rec := post("/api/v1/auth/login/begin", ""); rec.Code != 200 {
		t.Errorf("login begin = %d", rec.Code)
	}
}
