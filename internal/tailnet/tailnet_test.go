package tailnet

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"tailscale.com/client/tailscale/apitype"
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
	return node.Handler(full, public, store, ownerLogin)
}

func request(h http.Handler, remoteAddr, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	req.RemoteAddr = remoteAddr
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
	gate := node.Handler(full, http.NotFoundHandler(), store, "")

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
