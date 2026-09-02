package auth

import (
	"net/http/httptest"
	"path/filepath"
	"testing"
)

// TestConnectedApps covers the question Settings exists to answer — what is
// attached to this vault — and the action it exists to offer: cutting one off.
func TestConnectedApps(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.DB.Close() })

	client, err := store.RegisterOAuthClient("Claude", []string{"https://claude.ai/cb"})
	if err != nil {
		t.Fatal(err)
	}

	// A merely-registered client is not a connected app: anyone can register
	// via DCR, so listing those would make the page meaningless.
	apps, err := store.ListConnectedApps()
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 0 {
		t.Fatalf("unconsented client listed as connected: %+v", apps)
	}

	// Consent happens when a code is minted.
	if _, err := store.MintOAuthCode(client.ID, "https://claude.ai/cb", "challenge", "read write tasks"); err != nil {
		t.Fatal(err)
	}
	tokens, err := store.MintOAuthTokens(client.ID, "read write tasks")
	if err != nil {
		t.Fatal(err)
	}

	apps, err = store.ListConnectedApps()
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 {
		t.Fatalf("connected apps = %+v", apps)
	}
	if apps[0].Name != "Claude" || !apps[0].ActiveGrant {
		t.Errorf("app = %+v, want an active grant named Claude", apps[0])
	}
	if len(apps[0].Scopes) != 3 {
		t.Errorf("scopes = %v, want the three granted", apps[0].Scopes)
	}

	// The access token works right up until we disconnect.
	req := httptest.NewRequest("GET", "/api/v1/documents", nil)
	req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	if _, err := store.authenticateBearer(req); err != nil {
		t.Fatalf("oauth token should authenticate before disconnect: %v", err)
	}

	if err := store.DisconnectApp(client.ID); err != nil {
		t.Fatal(err)
	}
	// Disconnecting must kill the live credential, not just hide the row.
	if _, err := store.authenticateBearer(req); err == nil {
		t.Error("access token still valid after disconnect")
	}
	// And the client itself is gone, so a reconnect has to pass consent again.
	if _, err := store.GetOAuthClient(client.ID); err == nil {
		t.Error("client record survived disconnect")
	}
	apps, _ = store.ListConnectedApps()
	if len(apps) != 0 {
		t.Errorf("app still listed after disconnect: %+v", apps)
	}
	// Disconnecting something that is not there is an error, not a no-op.
	if err := store.DisconnectApp(client.ID); err == nil {
		t.Error("disconnecting an unknown app should fail")
	}
}
