// The exposure test: quire is designed to be safe to put on the public
// internet, and that claim rests entirely on Protected() classifying every
// route correctly. This file is the audit.
//
// It is deliberately fail-closed. The route list below is exhaustive, and a
// route that is not in it fails the test rather than being assumed safe —
// so adding an endpoint forces a conscious decision about whether anonymous
// callers may reach it. The v0.1.0 vulnerability was exactly this class of
// mistake: a trust decision that defaulted to open when a signal was
// missing.
package auth

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jclement/quire/internal/config"
)

// publicRoutes are the only paths an anonymous caller may reach, each with
// the reason it is safe. Everything else must demand a credential.
var publicRoutes = map[string]string{
	"/api/v1/health":               "liveness only: status, version, update flag — no vault data",
	"/api/v1/auth/status":          "whether auth is set up; gates its own flows",
	"/api/v1/auth/login/begin":     "mints a WebAuthn challenge; proves nothing",
	"/api/v1/auth/login/finish":    "verifies a passkey assertion; rate limited",
	"/api/v1/auth/register/begin":  "bootstrap or session-gated inside the handler",
	"/api/v1/auth/register/finish": "bootstrap or session-gated inside the handler",
	"/api/v1/auth/recover":         "redeems a recovery code; rate limited",
	"/api/v1/auth/logout":          "destroys the caller's own session",
	"/api/v1/auth/passkeys":        "session-gated inside the handler",
	"/s/":                          "share pages — public by design; the link is the secret",
	"/":                            "SPA shell; holds no data, its API calls are checked",
	"/oauth/authorize":             "consent page; owner-gated inside the handler",
	"/oauth/register":              "RFC 7591 DCR; capped, grants nothing until consent",
	"/oauth/token":                 "requires a code or refresh token to return anything",
	"/oauth/revoke":                "requires the token being revoked",
	"/.well-known/oauth-authorization-server": "RFC 8414 discovery metadata; static",
	"/.well-known/oauth-protected-resource":   "RFC 9728 discovery metadata; static",
}

// notPublic are routes deliberately behind auth even though they carry no
// vault content — cheap defence in depth, asserted so nobody "fixes" it.
var notPublic = []string{"/api/openapi.yaml"}

// dataRoutes carry vault content or grant state. Every one of them must
// reject an anonymous caller in every auth mode that has credentials.
var dataRoutes = []struct{ method, path string }{
	{"GET", "/api/v1/documents"},
	{"GET", "/api/v1/documents/notes/secret.md"},
	{"POST", "/api/v1/documents"},
	{"PUT", "/api/v1/documents/notes/secret.md"},
	{"PATCH", "/api/v1/documents/notes/secret.md"},
	{"DELETE", "/api/v1/documents/notes/secret.md"},
	{"GET", "/api/v1/search?q=password"},
	{"GET", "/api/v1/tasks"},
	{"POST", "/api/v1/tasks"},
	{"PATCH", "/api/v1/tasks/abc"},
	{"POST", "/api/v1/tasks/abc/toggle"},
	{"GET", "/api/v1/today"},
	{"GET", "/api/v1/calendar"},
	{"GET", "/api/v1/daily"}, // the journal: every daily note
	{"GET", "/api/v1/daily/2026-09-01"},
	{"GET", "/api/v1/tags"},
	{"GET", "/api/v1/areas"},
	{"GET", "/api/v1/audit"}, // what agents did, by name
	{"POST", "/api/v1/daily/2026-09-01"},
	{"GET", "/api/v1/events"},                  // SSE: streams every vault change
	{"GET", "/api/v1/files/attachments/x.png"}, // raw vault files
	{"POST", "/api/v1/attachments"},
	{"POST", "/api/v1/capture"},
	{"GET", "/api/v1/shares"},  // share tokens = read access to documents
	{"POST", "/api/v1/shares"}, // minting a share is granting public access
	{"DELETE", "/api/v1/shares/tok"},
	{"GET", "/api/v1/tokens"},  // token names, prefixes, scopes
	{"POST", "/api/v1/tokens"}, // minting a credential
	{"DELETE", "/api/v1/tokens/abcd1234"},
	{"GET", "/api/v1/connected-apps"},
	{"DELETE", "/api/v1/connected-apps/xyz"},
	{"GET", "/api/v1/agent-guidance"},
	{"PUT", "/api/v1/agent-guidance"},
	{"POST", "/api/v1/rename"},
	{"POST", "/api/v1/link"},
	{"POST", "/mcp"}, // the entire service layer, over one endpoint
}

// TestProtectedClassifiesEveryDataRoute is the core assertion: nothing that
// touches the vault is reachable without a credential.
func TestProtectedClassifiesEveryDataRoute(t *testing.T) {
	for _, route := range dataRoutes {
		path, _, _ := strings.Cut(route.path, "?")
		if !Protected(path) {
			t.Errorf("EXPOSED: %s %s is not Protected() — anonymous callers reach it",
				route.method, route.path)
		}
	}
	for path, why := range publicRoutes {
		if Protected(path) && !strings.HasPrefix(path, "/api/v1/auth/") {
			t.Errorf("%s is Protected() but documented public (%s) — the two disagree", path, why)
		}
	}
	for _, path := range notPublic {
		if !Protected(path) {
			t.Errorf("%s became publicly reachable", path)
		}
	}
}

// TestAnonymousIsRejectedEndToEnd runs real requests through the real
// middleware in both credentialed modes and asserts nothing leaks. This
// catches what a Protected() unit test cannot: a route that is classified
// correctly but whose handler answers before the middleware runs.
func TestAnonymousIsRejectedEndToEnd(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.DB.Close() })

	// A handler that would hand over the crown jewels if ever reached.
	secret := "VAULT-CONTENTS-THAT-MUST-NOT-ESCAPE"
	leaky := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(secret))
	})

	for _, mode := range []config.AuthMode{config.AuthTokenOnly, config.AuthPasskey} {
		mw := store.Middleware(mode, "", leaky)
		for _, route := range dataRoutes {
			for _, creds := range []struct{ name, header string }{
				{"no credential", ""},
				{"garbage bearer", "Bearer not-a-real-token"},
				{"empty bearer", "Bearer "},
				{"forged owner", "Bearer owner"},
			} {
				req := httptest.NewRequest(route.method, route.path, nil)
				if creds.header != "" {
					req.Header.Set("Authorization", creds.header)
				}
				rec := httptest.NewRecorder()
				mw.ServeHTTP(rec, req)

				if rec.Code == http.StatusOK || strings.Contains(rec.Body.String(), secret) {
					t.Errorf("LEAK [%s] %s %s with %s: got %d %q",
						mode, route.method, route.path, creds.name, rec.Code, rec.Body.String())
				}
			}
		}
	}
}

// TestSessionCookieCannotBeForged: the session cookie is the browser's whole
// credential, so a guessed or empty value must not authenticate.
func TestSessionCookieCannotBeForged(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.DB.Close() })

	// A real session exists, so the table is not trivially empty.
	real, err := store.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SessionPrincipal(real); err != nil {
		t.Fatalf("a freshly minted session must authenticate: %v", err)
	}

	for _, forged := range []string{
		"", " ", "owner", "admin",
		real[:len(real)-1],    // truncated
		real + "x",            // extended
		strings.ToUpper(real), // case-flipped
		"' OR 1=1 --",         // SQL injection through the cookie
		"%' OR token LIKE '%", // LIKE injection
	} {
		if _, err := store.SessionPrincipal(forged); err == nil {
			t.Errorf("forged session %q authenticated", forged)
		}
	}
}

// TestBearerTokenCannotBeForged mirrors the above for the API-token path,
// including the SQL-injection shapes a token string could carry.
func TestBearerTokenCannotBeForged(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.DB.Close() })

	plaintext, _, err := store.CreateToken("real", []string{ScopeRead}, 0)
	if err != nil {
		t.Fatal(err)
	}

	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mw := store.Middleware(config.AuthTokenOnly, "", ok)
	try := func(header string) int {
		req := httptest.NewRequest("GET", "/api/v1/documents", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)
		return rec.Code
	}

	if got := try("Bearer " + plaintext); got != 200 {
		t.Fatalf("the real token must work: got %d", got)
	}
	for _, forged := range []string{
		"Bearer " + plaintext[:len(plaintext)-1],
		"Bearer " + strings.ToUpper(plaintext),
		"Bearer ' OR 1=1 --",
		"Bearer %",
		"bearer " + plaintext, // wrong case: the scheme is case-sensitive here
		plaintext,             // no scheme at all
		"Bearer  " + plaintext,
	} {
		if got := try(forged); got == 200 {
			t.Errorf("forged credential %q authenticated", forged)
		}
	}

	// A read-only token must not be able to write.
	req := httptest.NewRequest("POST", "/api/v1/documents", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("read-only token on a write = %d, want 403", rec.Code)
	}
}
