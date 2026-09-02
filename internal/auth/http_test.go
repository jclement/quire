// Tests for the passkey HTTP surface — the login system, which had no test
// coverage at all. The WebAuthn ceremonies themselves need a real
// authenticator and are covered end to end in the browser; everything
// around them is exercised here: recovery codes, sessions, cookie flags,
// the management routes, and the rate limit that stands between an online
// attacker and a 50-bit recovery code.
package auth

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func newAuthHTTP(t *testing.T) (*HTTPConfig, *http.ServeMux, *Store) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.DB.Close() })
	passkeys, err := NewPasskeys(store, "quire", "quire.example.com", []string{"https://quire.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	h := &HTTPConfig{Passkeys: passkeys, SecureCookies: true}
	mux := http.NewServeMux()
	h.Routes(mux)
	return h, mux, store
}

// do issues a request, optionally with a session cookie, from a given peer.
func do(mux *http.ServeMux, method, path, body, session, peer string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if session != "" {
		req.AddCookie(&http.Cookie{Name: SessionCookie, Value: session})
	}
	if peer != "" {
		req.RemoteAddr = peer
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestAuthStatus(t *testing.T) {
	_, mux, store := newAuthHTTP(t)

	rec := do(mux, "GET", "/api/v1/auth/status", "", "", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"registered":false`) {
		t.Fatalf("fresh instance status = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"authenticated":false`) {
		t.Error("nobody is authenticated on a fresh instance")
	}

	// A valid session flips authenticated, and nothing else.
	session, err := store.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	rec = do(mux, "GET", "/api/v1/auth/status", "", session, "")
	if !strings.Contains(rec.Body.String(), `"authenticated":true`) {
		t.Errorf("status with a session = %s", rec.Body.String())
	}
	// A forged one does not.
	rec = do(mux, "GET", "/api/v1/auth/status", "", "not-a-session", "")
	if !strings.Contains(rec.Body.String(), `"authenticated":false`) {
		t.Errorf("forged session authenticated: %s", rec.Body.String())
	}
}

// TestRecoveryFlow: recovery codes are the way back in when every passkey is
// gone, so they must work exactly once each and be useless to a guesser.
func TestRecoveryFlow(t *testing.T) {
	_, mux, store := newAuthHTTP(t)
	codes, err := store.generateRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != 8 {
		t.Fatalf("expected 8 recovery codes, got %d", len(codes))
	}

	rec := do(mux, "POST", "/api/v1/auth/recover", `{"code":"`+codes[0]+`"}`, "", "")
	if rec.Code != 200 {
		t.Fatalf("redeeming a valid code = %d %s", rec.Code, rec.Body.String())
	}
	// The response must both set a usable session and tell the client to go
	// register a passkey, since the code that got them in is now burnt.
	if !strings.Contains(rec.Body.String(), `"register_passkey":true`) {
		t.Error("recovery should direct the user to register a passkey")
	}
	cookie := sessionCookie(t, rec)
	if _, err := store.SessionPrincipal(cookie.Value); err != nil {
		t.Errorf("recovery session is not valid: %v", err)
	}
	// Cookie hardening: readable by script, or sent cross-site, would undo
	// the point of server-side sessions.
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || !cookie.Secure {
		t.Errorf("session cookie = HttpOnly:%v SameSite:%v Secure:%v",
			cookie.HttpOnly, cookie.SameSite, cookie.Secure)
	}

	// Single use.
	if rec := do(mux, "POST", "/api/v1/auth/recover", `{"code":"`+codes[0]+`"}`, "", ""); rec.Code != 401 {
		t.Errorf("reusing a recovery code = %d, want 401", rec.Code)
	}
	// Other codes still live.
	if rec := do(mux, "POST", "/api/v1/auth/recover", `{"code":"`+codes[1]+`"}`, "", ""); rec.Code != 200 {
		t.Errorf("second code = %d, want 200", rec.Code)
	}
	// Guesses and malformed bodies get nothing.
	for _, body := range []string{
		`{"code":"aaaa-bbbb"}`,
		`{"code":""}`,
		`{"code":"' OR 1=1 --"}`,
		`not json`,
	} {
		if rec := do(mux, "POST", "/api/v1/auth/recover", body, "", ""); rec.Code == 200 {
			t.Errorf("recover accepted %s", body)
		}
	}
}

// TestRecoveryIsRateLimited: an 8-code pool of 50-bit codes is only strong
// online if guessing is slow.
func TestRecoveryIsRateLimited(t *testing.T) {
	_, mux, _ := newAuthHTTP(t)
	const attacker = "203.0.113.5:9999"

	var limited bool
	for range rateAttempts + 2 {
		rec := do(mux, "POST", "/api/v1/auth/recover", `{"code":"aaaa-bbbb"}`, "", attacker)
		if rec.Code == http.StatusTooManyRequests {
			limited = true
			if rec.Header().Get("Retry-After") == "" {
				t.Error("a 429 must carry Retry-After")
			}
			break
		}
	}
	if !limited {
		t.Fatalf("recovery was never rate limited within %d attempts", rateAttempts+2)
	}
	// A different client is unaffected — the limiter buckets per client, so
	// one attacker must not lock the owner out.
	rec := do(mux, "POST", "/api/v1/auth/recover", `{"code":"aaaa-bbbb"}`, "", "198.51.100.7:1234")
	if rec.Code == http.StatusTooManyRequests {
		t.Error("LOCKOUT: one client's attempts rate-limited another")
	}
}

func TestLogoutRevokesTheSession(t *testing.T) {
	_, mux, store := newAuthHTTP(t)
	session, err := store.CreateSession()
	if err != nil {
		t.Fatal(err)
	}

	rec := do(mux, "POST", "/api/v1/auth/logout", "", session, "")
	if rec.Code != 200 {
		t.Fatalf("logout = %d", rec.Code)
	}
	// Server-side revocation is the whole reason sessions are not JWTs:
	// the token must be dead immediately, not merely cleared in the browser.
	if _, err := store.SessionPrincipal(session); err == nil {
		t.Error("session still valid after logout")
	}
	if cookie := sessionCookie(t, rec); cookie.MaxAge >= 0 {
		t.Errorf("logout should expire the cookie, got MaxAge %d", cookie.MaxAge)
	}
	// Logging out without a session is harmless, not a 500.
	if rec := do(mux, "POST", "/api/v1/auth/logout", "", "", ""); rec.Code != 200 {
		t.Errorf("logout with no session = %d", rec.Code)
	}
}

// TestPasskeyManagementNeedsASession: listing or deleting passkeys is
// owner-only, and these handlers gate themselves rather than relying on the
// API middleware (they sit under /api/v1/auth/, which is unprotected).
func TestPasskeyManagementNeedsASession(t *testing.T) {
	_, mux, store := newAuthHTTP(t)

	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/v1/auth/passkeys"},
		{"DELETE", "/api/v1/auth/passkeys/1"},
	} {
		for _, session := range []string{"", "forged", "' OR 1=1 --"} {
			rec := do(mux, tc.method, tc.path, "", session, "")
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("%s %s with session %q = %d, want 401", tc.method, tc.path, session, rec.Code)
			}
		}
	}

	// With a session, listing works and is empty rather than null — the
	// client types it as an array.
	session, err := store.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	rec := do(mux, "GET", "/api/v1/auth/passkeys", "", session, "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"data":[]`) {
		t.Errorf("listing passkeys = %d %s", rec.Code, rec.Body.String())
	}
}

// TestLoginBeginWithNoPasskeys: an instance nobody has claimed cannot start
// a login ceremony, and says so rather than panicking.
func TestLoginBeginWithNoPasskeys(t *testing.T) {
	_, mux, _ := newAuthHTTP(t)
	rec := do(mux, "POST", "/api/v1/auth/login/begin", "", "", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("login/begin with no passkeys = %d, want 400", rec.Code)
	}
}

// sessionCookie pulls the session cookie out of a response.
func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == SessionCookie {
			return cookie
		}
	}
	t.Fatalf("no %s cookie in response", SessionCookie)
	return nil
}
