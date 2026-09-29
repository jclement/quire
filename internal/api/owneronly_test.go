// The route inventory for the owner-only rule (auth.OwnerOnly). Scopes say
// whether a caller may touch the vault; this says which routes no delegated
// credential may touch at all — minting tokens, disconnecting apps,
// publishing shares, changing settings. A write-scoped agent could do all
// of that until the rule existed, because every one of those routes was
// "just a write".
//
// Like auth's exposure test it fails closed: every route Routes registers
// must appear in routeAccess, so a new endpoint forces a decision about
// whether an agent may call it rather than defaulting to "write scope is
// enough".
package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jclement/quire/internal/auth"
	"github.com/jclement/quire/internal/config"
	"github.com/jclement/quire/internal/share"
)

// ownerOnly and agent classify a route in routeAccess.
const (
	ownerOnly = "owner"
	agent     = "agent"
)

// routeAccess is every pattern Routes registers, with who may call it.
var routeAccess = map[string]string{
	"GET /api/v1/health":                         agent,
	"GET /api/openapi.yaml":                      agent,
	"GET /api/v1/documents":                      agent,
	"POST /api/v1/documents":                     agent,
	"GET /api/v1/documents/{path...}":            agent,
	"PUT /api/v1/documents/{path...}":            agent,
	"DELETE /api/v1/documents/{path...}":         agent,
	"POST /api/v1/rename":                        agent,
	"PATCH /api/v1/documents/{path...}":          agent,
	"POST /api/v1/link":                          agent,
	"GET /api/v1/search":                         agent,
	"GET /api/v1/related":                        agent,
	"GET /api/v1/semantic/status":                agent,
	"GET /api/v1/timezone":                       agent,
	"PUT /api/v1/timezone":                       ownerOnly,
	"GET /api/v1/email":                          ownerOnly,
	"POST /api/v1/email/test":                    ownerOnly,
	"GET /api/v1/tags":                           agent,
	"GET /api/v1/unwritten":                      agent,
	"GET /api/v1/areas":                          agent,
	"PUT /api/v1/areas":                          ownerOnly,
	"GET /api/v1/templates":                      agent,
	"POST /api/v1/templates/starter":             agent, // writes vault documents, nothing more
	"GET /api/v1/tasks":                          agent,
	"POST /api/v1/tasks":                         agent,
	"POST /api/v1/tasks/{id}/toggle":             agent,
	"POST /api/v1/tasks/{id}/restore-recurrence": agent,
	"PATCH /api/v1/tasks/{id}":                   agent,
	"GET /api/v1/daily":                          agent,
	"GET /api/v1/daily/{date}":                   agent,
	"POST /api/v1/daily/{date}":                  agent,
	"GET /api/v1/weekly/{week}":                  agent,
	"POST /api/v1/weekly/{week}":                 agent,
	"GET /api/v1/today":                          agent,
	"GET /api/v1/calendar":                       agent,
	"GET /api/v1/agent-guidance":                 agent,
	"PUT /api/v1/agent-guidance":                 ownerOnly,
	"POST /api/v1/attachments":                   agent,
	"POST /api/v1/capture":                       agent,
	"POST /api/v1/capture/note":                  agent,
	"POST /api/v1/drawings":                      agent,
	"PUT /api/v1/drawings/{path...}":             agent,
	"GET /api/v1/files/{path...}":                agent,
	"GET /api/v1/events":                         agent,
	"GET /api/v1/shares":                         ownerOnly,
	"POST /api/v1/shares":                        ownerOnly,
	"DELETE /api/v1/shares/{token}":              ownerOnly,
	"GET /api/v1/tokens":                         ownerOnly,
	"POST /api/v1/tokens":                        ownerOnly,
	"DELETE /api/v1/tokens/{prefix}":             ownerOnly,
	"GET /api/v1/audit":                          ownerOnly,
	"GET /api/v1/connected-apps":                 ownerOnly,
	"DELETE /api/v1/connected-apps/{id}":         ownerOnly,
	"POST /api/v1/aliases":                       agent,
	"GET /api/v1/decisions":                      agent,
	"GET /api/v1/work-items":                     agent,
	"PUT /api/v1/work-items":                     ownerOnly, // a setting: where work-item ids link to
	"GET /api/v1/waiting":                        agent,
	"POST /api/v1/notes/from-task":               agent, // vault work, same as the task_to_note tool
	"GET /api/v1/calendar/feeds":                 ownerOnly,
	"POST /api/v1/calendar/feeds":                ownerOnly,
	"DELETE /api/v1/calendar/feeds/{id}":         ownerOnly,
	"POST /api/v1/calendar/refresh":              ownerOnly, // Settings button; drives outbound fetches
	"GET /api/v1/calendar/events":                agent,
	"POST /api/v1/calendar/events/note":          agent, // same as create_meeting_from_event
	"GET /api/v1/meeting-prep":                   agent,
}

// routeRecorder collects the patterns Routes registers.
type routeRecorder struct{ patterns []string }

func (r *routeRecorder) HandleFunc(pattern string, _ func(http.ResponseWriter, *http.Request)) {
	r.patterns = append(r.patterns, pattern)
}

// registeredRoutes returns every pattern a fully-wired Server registers —
// auth and shares set, since those are what make the admin routes exist.
func registeredRoutes(t *testing.T, store *auth.Store) []string {
	t.Helper()
	s := &Server{Auth: store, Shares: share.NewManager(store, nil, "https://quire.example.com")}
	var recorder routeRecorder
	s.Routes(&recorder)
	return recorder.patterns
}

// concretePath turns a pattern's path into one a request can carry.
func concretePath(pattern string) string {
	path := pattern
	path = strings.ReplaceAll(path, "{path...}", "notes/x.md")
	for strings.Contains(path, "{") {
		start := strings.Index(path, "{")
		end := strings.Index(path, "}")
		path = path[:start] + "x" + path[end+1:]
	}
	return path
}

// TestEveryRouteIsClassified keeps routeAccess exhaustive and honest: no
// registered route is missing from it, none it names has gone, and each
// classification agrees with auth.OwnerOnly.
func TestEveryRouteIsClassified(t *testing.T) {
	store, err := auth.Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.DB.Close() })

	registered := map[string]bool{}
	for _, pattern := range registeredRoutes(t, store) {
		registered[pattern] = true
		access, ok := routeAccess[pattern]
		if !ok {
			t.Errorf("UNCLASSIFIED: %s — add it to routeAccess; if it manages credentials, "+
				"sharing or settings it belongs in auth's ownerOnlyRoutes too", pattern)
			continue
		}
		method, path, _ := strings.Cut(pattern, " ")
		if got := auth.OwnerOnly(method, concretePath(path)); got != (access == ownerOnly) {
			t.Errorf("%s is classified %s here but auth.OwnerOnly says %v", pattern, access, got)
		}
	}
	for pattern := range routeAccess {
		if !registered[pattern] {
			t.Errorf("routeAccess lists %s, which is no longer registered", pattern)
		}
	}
}

// TestOwnerOnlyRoutesEndToEnd sends every registered route through the real
// middleware as each kind of caller. Owner-only routes must refuse a
// full-scope API token and a full-scope OAuth token and admit a passkey
// session; every other route must still admit those same agents, so the
// rule cannot quietly grow over the vault surface agents exist to use.
func TestOwnerOnlyRoutesEndToEnd(t *testing.T) {
	store, err := auth.Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.DB.Close() })

	token, _, err := store.CreateToken("agent", []string{auth.ScopeRead, auth.ScopeWrite, auth.ScopeTasks}, 0)
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
	reached := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mw := store.Middleware(config.AuthPasskey, "", reached)

	callers := []struct {
		name, bearer, cookie string
		delegated            bool
	}{
		{"API token", token, "", true},
		{"OAuth token", oauth.AccessToken, "", true},
		{"passkey session", "", session, false},
	}
	for _, pattern := range registeredRoutes(t, store) {
		method, path, _ := strings.Cut(pattern, " ")
		for _, caller := range callers {
			req := httptest.NewRequest(method, concretePath(path), nil)
			if caller.bearer != "" {
				req.Header.Set("Authorization", "Bearer "+caller.bearer)
			}
			if caller.cookie != "" {
				req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: caller.cookie})
			}
			rec := httptest.NewRecorder()
			mw.ServeHTTP(rec, req)

			want := http.StatusOK
			if caller.delegated && routeAccess[pattern] == ownerOnly {
				want = http.StatusForbidden
			}
			if rec.Code != want {
				t.Errorf("%s as %s = %d, want %d", pattern, caller.name, rec.Code, want)
			}
		}
	}
}
