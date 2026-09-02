package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/jclement/quire/internal/auth"
	"github.com/jclement/quire/internal/index"
	"github.com/jclement/quire/internal/service"
	"github.com/jclement/quire/internal/vault"
)

// newCredentialServer wires a server with an auth store, which is what makes
// the credential routes register at all.
func newCredentialServer(t *testing.T) *httptest.Server {
	t.Helper()
	v, err := vault.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	db, _, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store, err := auth.Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.DB.Close() })

	svc := service.New(v, &index.Index{DB: db, Vault: v})
	svc.Now = func() time.Time { return time.Date(2026, 9, 1, 10, 0, 0, 0, time.Local) }
	s := &Server{Service: svc, Events: NewBroadcaster(), Auth: store, Version: "test"}
	mux := http.NewServeMux()
	s.Routes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func TestTokenLifecycle(t *testing.T) {
	ts := newCredentialServer(t)

	var created service.NewToken
	doJSON(t, "POST", ts.URL+"/api/v1/tokens",
		map[string]any{"name": "laptop", "scopes": []string{"read", "tasks"}, "expires_in_days": 30},
		http.StatusCreated, &created)

	// The plaintext is returned exactly once, and the stored record keeps
	// only the display prefix.
	if created.Plaintext == "" || created.Token.Prefix == "" {
		t.Fatalf("create returned %+v", created)
	}
	if created.Token.ExpiresAt == "" {
		t.Error("expires_in_days did not produce an expiry")
	}

	var listed []service.TokenInfo
	doJSON(t, "GET", ts.URL+"/api/v1/tokens", nil, http.StatusOK, &listed)
	if len(listed) != 1 || listed[0].Name != "laptop" {
		t.Fatalf("list = %+v", listed)
	}
	// The secret must never come back from a list.
	for _, token := range listed {
		if token.Prefix == created.Plaintext {
			t.Error("list leaked the plaintext token")
		}
	}

	// Revoking keeps the row (audit trail) but marks it dead.
	doJSON(t, "DELETE", ts.URL+"/api/v1/tokens/"+created.Token.Prefix, nil, http.StatusNoContent, nil)
	doJSON(t, "GET", ts.URL+"/api/v1/tokens", nil, http.StatusOK, &listed)
	if len(listed) != 1 || listed[0].RevokedAt == "" {
		t.Errorf("after revoke = %+v", listed)
	}
	// Revoking twice is a 404, not a silent success.
	doJSON(t, "DELETE", ts.URL+"/api/v1/tokens/"+created.Token.Prefix, nil, http.StatusNotFound, nil)
}

// TestCreateTokenValidates: a typo'd scope must fail loudly at creation
// rather than minting a credential that silently grants nothing.
func TestCreateTokenValidates(t *testing.T) {
	ts := newCredentialServer(t)
	for _, body := range []map[string]any{
		{"name": "", "scopes": []string{"read"}},
		{"name": "x", "scopes": []string{}},
		{"name": "x", "scopes": []string{"admin"}},
		{"name": "x", "scopes": []string{"read", "root"}},
	} {
		doJSON(t, "POST", ts.URL+"/api/v1/tokens", body, http.StatusBadRequest, nil)
	}
}

func TestConnectedApps(t *testing.T) {
	ts := newCredentialServer(t)

	var apps []service.ConnectedApp
	doJSON(t, "GET", ts.URL+"/api/v1/connected-apps", nil, http.StatusOK, &apps)
	if len(apps) != 0 {
		t.Fatalf("expected no apps, got %+v", apps)
	}
	doJSON(t, "DELETE", ts.URL+"/api/v1/connected-apps/nope", nil, http.StatusNotFound, nil)
}
