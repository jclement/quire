package auth

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/jclement/quire/internal/config"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DB.Close() })
	return s
}

func TestTokenLifecycle(t *testing.T) {
	s := newStore(t)
	plaintext, tok, err := s.CreateToken("agent", []string{ScopeRead, ScopeTasks}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(plaintext) != 3+64 || plaintext[:3] != "sk_" || tok.Prefix != plaintext[3:11] {
		t.Fatalf("token shape: %q prefix %q", plaintext, tok.Prefix)
	}

	req := httptest.NewRequest("GET", "/api/v1/documents", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	p, err := s.authenticateBearer(req)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Allows(ScopeRead) || !p.Allows(ScopeTasks) || p.Allows(ScopeWrite) {
		t.Errorf("scopes = %+v", p.Scopes)
	}

	if err := s.RevokeToken(tok.Prefix); err != nil {
		t.Fatal(err)
	}
	if _, err := s.authenticateBearer(req); err == nil {
		t.Errorf("revoked token still authenticates")
	}
}

func TestExpiredToken(t *testing.T) {
	s := newStore(t)
	plaintext, _, err := s.CreateToken("shortlived", []string{ScopeRead}, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	if _, err := s.authenticateBearer(req); err == nil {
		t.Errorf("expired token still authenticates")
	}
}

func TestMiddlewareModes(t *testing.T) {
	s := newStore(t)
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })

	get := func(h http.Handler, path, bearer string) int {
		req := httptest.NewRequest("GET", path, nil)
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	noneMode := s.Middleware(config.AuthNone, "", ok)
	if got := get(noneMode, "/api/v1/documents", ""); got != 200 {
		t.Errorf("none mode = %d", got)
	}

	tokenMode := s.Middleware(config.AuthTokenOnly, "", ok)
	if got := get(tokenMode, "/api/v1/documents", ""); got != 401 {
		t.Errorf("token mode without token = %d", got)
	}
	if got := get(tokenMode, "/api/v1/health", ""); got != 200 {
		t.Errorf("health should be open, got %d", got)
	}
	if got := get(tokenMode, "/", ""); got != 200 {
		t.Errorf("SPA should be open, got %d", got)
	}

	plaintext, _, err := s.CreateToken("reader", []string{ScopeRead}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := get(tokenMode, "/api/v1/documents", plaintext); got != 200 {
		t.Errorf("valid token read = %d", got)
	}
	// Read-only token cannot write.
	req := httptest.NewRequest("POST", "/api/v1/documents", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	tokenMode.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Errorf("read-only write = %d", rec.Code)
	}
}
