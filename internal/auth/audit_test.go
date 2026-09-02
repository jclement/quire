package auth

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/jclement/quire/internal/config"
)

func TestAuditRecordAndList(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.DB.Close() })

	for i := range 3 {
		if err := store.RecordAudit(AuditRecord{Principal: "claude", Action: "mcp:create_task", Path: "daily/x.md", Detail: "task " + string(rune('a'+i)), OK: true}); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := store.ListAudit(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0].Detail != "task c" {
		t.Fatalf("newest first expected, got %+v", rows)
	}
	if rows[0].Principal != "claude" || !rows[0].OK || rows[0].At == "" {
		t.Errorf("row = %+v", rows[0])
	}
}

// TestAuditScope: agents are audited, the owner's own browser is not — the
// log answers "what did the agents do", and autosaves would drown it.
func TestAuditScope(t *testing.T) {
	if Audited(OwnerPrincipal()) {
		t.Error("the owner's session must not be audited")
	}
	if !Audited(Principal{Name: "claude", Scopes: map[string]bool{ScopeWrite: true}}) {
		t.Error("a token principal must be audited")
	}
	if !Audited(Principal{Name: "oauth:abc"}) {
		t.Error("an OAuth client must be audited")
	}
}

// TestMiddlewareAuditsAgentWrites: a REST write by a token lands in the
// log with its outcome; a read does not; the owner's writes do not.
func TestMiddlewareAuditsAgentWrites(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.DB.Close() })
	token, _, err := store.CreateToken("agent", []string{ScopeRead, ScopeWrite}, 0)
	if err != nil {
		t.Fatal(err)
	}

	mw := store.Middleware(config.AuthTokenOnly, "", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/documents/notes/bad.md" {
			w.WriteHeader(http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	do := func(method, path string) {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		mw.ServeHTTP(httptest.NewRecorder(), req)
	}
	do("GET", "/api/v1/documents")               // not audited
	do("PUT", "/api/v1/documents/notes/good.md") // audited, ok
	do("PUT", "/api/v1/documents/notes/bad.md")  // audited, failed
	do("POST", "/mcp")                           // MCP audits itself per tool

	rows, err := store.ListAudit(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 audit rows, got %d: %+v", len(rows), rows)
	}
	if rows[0].Path != "notes/bad.md" || rows[0].OK {
		t.Errorf("failed write should be recorded as not ok: %+v", rows[0])
	}
	if rows[1].Path != "notes/good.md" || !rows[1].OK || rows[1].Principal != "token:agent" {
		t.Errorf("successful write row = %+v", rows[1])
	}
}
