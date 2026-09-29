// Tests for the owner-only rule: credential, sharing and settings routes
// refuse every delegated credential — API token or OAuth client, whatever
// its scope — and admit only the owner in person. The route-by-route
// inventory lives in internal/api (owneronly_test.go), where the routes are
// registered; this file pins the matching and the middleware behaviour.
package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jclement/quire/internal/config"
)

// TestOwnerOnlyMatching pins the path boundaries: a prefix covers its
// subtree but not a sibling that merely shares the spelling, and a
// method-scoped entry leaves the other methods to the ordinary scopes.
func TestOwnerOnlyMatching(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		want         bool
	}{
		{"POST", "/api/v1/tokens", true},
		{"GET", "/api/v1/tokens", true},
		{"DELETE", "/api/v1/tokens/abcd1234", true},
		{"GET", "/api/v1/tokensx", false},
		{"DELETE", "/api/v1/connected-apps/xyz", true},
		{"GET", "/api/v1/audit", true},
		{"POST", "/api/v1/shares", true},
		{"POST", "/api/v1/email/test", true},
		{"PUT", "/api/v1/timezone", true},
		{"GET", "/api/v1/timezone", false},
		{"PUT", "/api/v1/agent-guidance", true},
		{"GET", "/api/v1/agent-guidance", false},
		// Listed before the route exists (calendar-feeds branch).
		{"POST", "/api/v1/calendar/feeds", true},
		{"DELETE", "/api/v1/calendar/feeds/1", true},
		{"GET", "/api/v1/calendar", false},
		{"PUT", "/api/v1/documents/api/v1/tokens", false},
	} {
		if got := OwnerOnly(tc.method, tc.path); got != tc.want {
			t.Errorf("OwnerOnly(%s %s) = %v, want %v", tc.method, tc.path, got, tc.want)
		}
	}
}

// TestOwnerOnlyRefusesDelegatedCredentials is the bug this rule fixed: a
// write-scoped token or OAuth client (the Claude connector) could POST
// /api/v1/tokens and mint itself a credential. Both must now get 403 while
// a passkey session and the auth-none owner still get through — and the
// attempt must land in the audit log.
func TestOwnerOnlyRefusesDelegatedCredentials(t *testing.T) {
	store := newStore(t)
	token, _, err := store.CreateToken("agent", []string{ScopeRead, ScopeWrite, ScopeTasks}, 0)
	if err != nil {
		t.Fatal(err)
	}
	oauth, err := store.MintOAuthTokens("claude", "read write tasks")
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	passkeyMode := store.Middleware(config.AuthPasskey, "", ok)
	noneMode := store.Middleware(config.AuthNone, "", ok)

	send := func(h http.Handler, bearer, cookie string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/v1/tokens", nil)
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: SessionCookie, Value: cookie})
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	for name, bearer := range map[string]string{"API token": token, "OAuth token": oauth.AccessToken} {
		rec := send(passkeyMode, bearer, "")
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "passkey") {
			t.Errorf("%s minting a token = %d %s, want 403 naming the passkey", name, rec.Code, rec.Body.String())
		}
		// A session cookie riding along does not launder the bearer: the
		// bearer is who is asking.
		if rec := send(passkeyMode, bearer, session); rec.Code != http.StatusForbidden {
			t.Errorf("%s plus a session cookie = %d, want 403", name, rec.Code)
		}
	}
	if rec := send(passkeyMode, "", session); rec.Code != http.StatusOK {
		t.Errorf("passkey session = %d, want 200", rec.Code)
	}
	if rec := send(noneMode, "", ""); rec.Code != http.StatusOK {
		t.Errorf("auth-none owner = %d, want 200", rec.Code)
	}

	rows, err := store.ListAudit(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 || rows[0].OK || rows[0].Action != "POST /api/v1/tokens" {
		t.Errorf("every refused attempt should be audited as failed, got %+v", rows)
	}
}
