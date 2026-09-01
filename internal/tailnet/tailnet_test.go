package tailnet

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/ipn"
	"tailscale.com/tailcfg"

	"github.com/jclement/quire/internal/auth"
)

// fakeNode builds a Node whose WhoIs is table-driven by remote address:
// "100.*" addresses resolve to a tailnet identity, everything else (funnel)
// fails — mirroring production behavior without a tailnet.
func fakeNode(logins map[string]string) *Node {
	return &Node{whois: func(_ context.Context, remoteAddr string) (*apitype.WhoIsResponse, error) {
		login, ok := logins[remoteAddr]
		if !ok {
			return nil, fmt.Errorf("peer not found")
		}
		return &apitype.WhoIsResponse{
			Node:        &tailcfg.Node{},
			UserProfile: &tailcfg.UserProfile{LoginName: login},
		}, nil
	}}
}

func gateFor(t *testing.T, ownerLogin string) http.Handler {
	t.Helper()
	store, err := auth.Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.DB.Close() })

	full := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "full") })
	public := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "share") })

	node := fakeNode(map[string]string{
		"100.64.0.1:1234": "jeff@example.com",
		"100.64.0.2:1234": "intruder@example.com",
	})
	return node.Handler(GateConfig{Full: full, Public: public, Store: store, OwnerLogin: ownerLogin})
}

func request(h http.Handler, remoteAddr, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	req.RemoteAddr = remoteAddr
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// funnelRequest models what actually arrives over Tailscale Funnel: the
// connection is proxied through Tailscale's ingress, so it carries a TAILNET
// source address that WhoIs resolves — the mark on the connection is the only
// thing that distinguishes it. This shape is why the gate leaked: inferring
// "public" from WhoIs failing let anonymous internet traffic in as the owner.
func funnelRequest(h http.Handler, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	req.RemoteAddr = "100.64.0.1:1234" // resolvable by the fake WhoIs
	// The real type Tailscale hands us for public traffic.
	req = req.WithContext(ConnContext(req.Context(), &ipn.FunnelConn{}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestTailnetPeerGetsFullApp(t *testing.T) {
	gate := gateFor(t, "")
	for _, path := range []string{"/", "/api/v1/documents", "/s/abc123", "/mcp"} {
		rec := request(gate, "100.64.0.1:1234", path)
		if rec.Code != 200 || rec.Body.String() != "full" {
			t.Errorf("%s from tailnet = %d %q", path, rec.Code, rec.Body.String())
		}
	}
}

func TestFunnelVisitorSeesOnlyShares(t *testing.T) {
	gate := gateFor(t, "")
	rec := request(gate, "203.0.113.9:5555", "/s/abc123")
	if rec.Code != 200 || rec.Body.String() != "share" {
		t.Errorf("share from internet = %d %q", rec.Code, rec.Body.String())
	}
	// Everything else does not exist as far as the internet can tell.
	for _, path := range []string{"/", "/api/v1/documents", "/api/v1/health", "/mcp", "/today"} {
		rec := request(gate, "203.0.113.9:5555", path)
		if rec.Code != 404 {
			t.Errorf("%s from internet = %d, want 404", path, rec.Code)
		}
	}
}

func TestOwnerRestriction(t *testing.T) {
	gate := gateFor(t, "jeff@example.com")
	if rec := request(gate, "100.64.0.1:1234", "/api/v1/documents"); rec.Code != 200 {
		t.Errorf("owner = %d", rec.Code)
	}
	if rec := request(gate, "100.64.0.2:1234", "/api/v1/documents"); rec.Code != 403 {
		t.Errorf("non-owner tailnet peer = %d, want 403", rec.Code)
	}
}

func TestBearerTokenScopesHonoredOnTailnet(t *testing.T) {
	store, err := auth.Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.DB.Close() })
	plaintext, _, err := store.CreateToken("readonly-agent", []string{auth.ScopeRead}, 0)
	if err != nil {
		t.Fatal(err)
	}

	full := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "full") })
	node := fakeNode(map[string]string{"100.64.0.1:1234": "jeff@example.com"})
	gate := node.Handler(GateConfig{Full: full, Public: http.NotFoundHandler(), Store: store})

	// A deliberately scoped-down token keeps its reduced power on-tailnet.
	req := httptest.NewRequest("POST", "/api/v1/documents", nil)
	req.RemoteAddr = "100.64.0.1:1234"
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	gate.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Errorf("read-only token write on tailnet = %d, want 403", rec.Code)
	}

	// An invalid token is rejected, not silently upgraded to owner.
	req.Header.Set("Authorization", "Bearer sk_bogus")
	rec = httptest.NewRecorder()
	gate.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Errorf("bogus token on tailnet = %d, want 401", rec.Code)
	}
}

// A funnel connection whose peer address WhoIs happily resolves must still be
// treated as the public internet. Without this, an anonymous request reached
// the API as the vault owner.
func TestFunnelConnectionIsNeverTreatedAsTailnet(t *testing.T) {
	gate := gateFor(t, "")
	for _, path := range []string{"/api/v1/documents", "/api/v1/today", "/", "/today", "/api/v1/health"} {
		if rec := funnelRequest(gate, path); rec.Code != 404 {
			t.Errorf("funnel %s = %d (body %q), want 404 — the vault must not be reachable from the internet",
				path, rec.Code, rec.Body.String())
		}
	}
	// Share pages remain the one public surface.
	if rec := funnelRequest(gate, "/s/abc123"); rec.Code != 200 || rec.Body.String() != "share" {
		t.Errorf("funnel share page = %d %q", rec.Code, rec.Body.String())
	}
}

// A source outside the tailnet range is public even with no funnel mark —
// an independent check, in case the mark is ever absent.
func TestNonTailnetAddressIsPublic(t *testing.T) {
	gate := gateFor(t, "")
	if rec := request(gate, "203.0.113.9:5555", "/api/v1/documents"); rec.Code != 404 {
		t.Errorf("public address reached the API: %d", rec.Code)
	}
}

// With PublicMCP on, funnel visitors may reach /mcp (credentialed), the
// OAuth endpoints, and metadata — and still nothing else. The gate also owns
// the identity headers: smuggled values must be stripped.
func TestPublicMCPGating(t *testing.T) {
	store, err := auth.Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.DB.Close() })
	plaintext, _, err := store.CreateToken("agent", []string{auth.ScopeRead, auth.ScopeWrite}, 0)
	if err != nil {
		t.Fatal(err)
	}

	var sawLogin, sawPublic string
	echo := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawLogin = r.Header.Get(auth.HeaderTailnetLogin)
		sawPublic = r.Header.Get(auth.HeaderPublicRequest)
		fmt.Fprint(w, "public")
	})
	node := fakeNode(map[string]string{"100.64.0.1:1234": "jeff@example.com"})
	gate := node.Handler(GateConfig{
		Full: echo, Public: echo, Store: store,
		PublicMCP:    true,
		MCPChallenge: `Bearer resource_metadata="https://q.example/.well-known/oauth-protected-resource"`,
	})

	do := func(remote, path, bearer string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", path, nil)
		req.RemoteAddr = remote
		// Smuggling attempt: must be stripped before handlers see it.
		req.Header.Set(auth.HeaderTailnetLogin, "evil@example.com")
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		rec := httptest.NewRecorder()
		gate.ServeHTTP(rec, req)
		return rec
	}

	// Funnel + no credential → 401 with the discovery challenge.
	rec := do("203.0.113.9:5555", "/mcp", "")
	if rec.Code != 401 || rec.Header().Get("WWW-Authenticate") == "" {
		t.Errorf("public /mcp uncredentialed = %d, WWW-Authenticate=%q", rec.Code, rec.Header().Get("WWW-Authenticate"))
	}
	// Funnel + valid token → served, marked public, no tailnet identity.
	rec = do("203.0.113.9:5555", "/mcp", plaintext)
	if rec.Code != 200 || sawPublic != "1" || sawLogin != "" {
		t.Errorf("public /mcp with token = %d public=%q login=%q", rec.Code, sawPublic, sawLogin)
	}
	// OAuth + metadata reachable over funnel; API still invisible.
	for path, want := range map[string]int{
		"/oauth/token": 200, "/.well-known/oauth-authorization-server": 200,
		"/api/v1/documents": 404, "/": 404,
	} {
		if rec := do("203.0.113.9:5555", path, ""); rec.Code != want {
			t.Errorf("public %s = %d, want %d", path, rec.Code, want)
		}
	}
	// Tailnet peer: identity header is the verified login, not the smuggle.
	rec = do("100.64.0.1:1234", "/", "")
	if rec.Code != 200 || sawLogin != "jeff@example.com" || sawPublic != "" {
		t.Errorf("tailnet identity = %d login=%q public=%q", rec.Code, sawLogin, sawPublic)
	}
}
