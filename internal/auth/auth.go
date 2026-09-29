// Package auth owns auth.db — the one database that is NOT rebuildable —
// and the request authentication middleware. v0.1 implements auth modes
// "none" (loopback-trusted singleton user) and "token-only" (bearer tokens);
// "passkey" is designed (DESIGN.md "Auth modes") but not yet implemented, and
// is rejected at startup rather than silently downgraded.
package auth

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/jclement/quire/internal/config"
)

// Scopes are coarse on purpose (DESIGN.md decision 6).
const (
	ScopeRead  = "read"
	ScopeWrite = "write"
	ScopeTasks = "tasks" // write, but only to tasks — the "agent manages my todos" token
)

// Principal is who a request acts as; handlers and services only ever see
// this, never the transport-level credential.
type Principal struct {
	Name   string
	Scopes map[string]bool
}

// Allows reports whether the principal may perform an operation needing the
// given scope. The write scope implies tasks.
func (p Principal) Allows(scope string) bool {
	if p.Scopes[scope] {
		return true
	}
	return scope == ScopeTasks && p.Scopes[ScopeWrite]
}

// IsOwner reports whether the principal is the owner in person — a passkey
// session, or the loopback-only auth-none listener — rather than a
// credential the owner delegated (an API token or an OAuth client). The
// name is the marker: only OwnerPrincipal carries it, and delegated
// principals are always prefixed ("token:", "oauth:"), so none can collide.
func (p Principal) IsOwner() bool { return p.Name == ownerName }

// principalKey carries the authenticated principal down to handlers. It is
// unexported so nothing outside this package can forge one into a context.
type principalKey struct{}

// WithPrincipal returns ctx carrying p.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom returns the principal the middleware authenticated for r.
// The false return means the request never passed the middleware — callers
// must treat that as "no access", never as "owner".
func PrincipalFrom(r *http.Request) (Principal, bool) {
	p, ok := r.Context().Value(principalKey{}).(Principal)
	return p, ok
}

// Store wraps auth.db.
type Store struct {
	DB *sql.DB
}

// migrations are additive and applied in order; PRAGMA user_version records
// how far this database has migrated. auth.db is precious: schema changes
// must migrate, never drop (unlike index.db).
var migrations = []string{
	// v1: API tokens
	`CREATE TABLE IF NOT EXISTS api_tokens (
		id           INTEGER PRIMARY KEY,
		name         TEXT NOT NULL,
		prefix       TEXT NOT NULL,             -- first 8 chars after sk_, for display
		hash         TEXT NOT NULL UNIQUE,      -- sha256 of the full token
		scopes       TEXT NOT NULL,             -- comma-separated
		created_at   TEXT NOT NULL,
		expires_at   TEXT NOT NULL DEFAULT '',  -- RFC3339 or '' for never
		revoked_at   TEXT NOT NULL DEFAULT '',
		last_used_at TEXT NOT NULL DEFAULT ''
	);`,
	// v2: share links
	`CREATE TABLE IF NOT EXISTS shares (
		token          TEXT PRIMARY KEY,
		doc_path       TEXT NOT NULL,
		created_at     TEXT NOT NULL,
		expires_at     TEXT NOT NULL DEFAULT '',
		revoked_at     TEXT NOT NULL DEFAULT '',
		view_count     INTEGER NOT NULL DEFAULT 0,
		last_viewed_at TEXT NOT NULL DEFAULT ''
	);`,
	// v3: passkeys, recovery codes, sessions
	`CREATE TABLE IF NOT EXISTS passkeys (
		id              TEXT PRIMARY KEY, -- hex credential id
		name            TEXT NOT NULL DEFAULT '',
		credential_json TEXT NOT NULL,
		created_at      TEXT NOT NULL,
		last_used_at    TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE IF NOT EXISTS recovery_codes (
		hash    TEXT PRIMARY KEY, -- argon2id
		used_at TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE IF NOT EXISTS sessions (
		token_hash   TEXT PRIMARY KEY, -- sha256
		created_at   TEXT NOT NULL,
		expires_at   TEXT NOT NULL,
		last_seen_at TEXT NOT NULL DEFAULT ''
	);`,
	// v4: OAuth 2.1 authorization server (dynamic client registration, PKCE
	// codes, rotating refresh tokens) for remote MCP clients.
	`CREATE TABLE IF NOT EXISTS oauth_clients (
		id            TEXT PRIMARY KEY,
		name          TEXT NOT NULL DEFAULT '',
		redirect_uris TEXT NOT NULL,            -- JSON array
		created_at    TEXT NOT NULL,
		consented_at  TEXT NOT NULL DEFAULT ''  -- unconsented clients are capped
	);
	CREATE TABLE IF NOT EXISTS oauth_codes (
		code_hash    TEXT PRIMARY KEY,
		client_id    TEXT NOT NULL,
		redirect_uri TEXT NOT NULL,
		challenge    TEXT NOT NULL,             -- PKCE S256 code_challenge
		scopes       TEXT NOT NULL,
		expires_at   TEXT NOT NULL,
		used_at      TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE IF NOT EXISTS oauth_tokens (
		id                 INTEGER PRIMARY KEY,
		client_id          TEXT NOT NULL,
		access_hash        TEXT NOT NULL UNIQUE,
		refresh_hash       TEXT NOT NULL UNIQUE,
		prev_refresh_hash  TEXT NOT NULL DEFAULT '', -- rotation reuse-grace
		rotated_at         TEXT NOT NULL DEFAULT '',
		scopes             TEXT NOT NULL,
		access_expires_at  TEXT NOT NULL,
		refresh_expires_at TEXT NOT NULL,
		revoked_at         TEXT NOT NULL DEFAULT '',
		created_at         TEXT NOT NULL,
		last_used_at       TEXT NOT NULL DEFAULT ''
	);`,
	// v5: the audit log of agent actions (API tokens and OAuth clients).
	`CREATE TABLE IF NOT EXISTS audit_log (
		id        INTEGER PRIMARY KEY,
		at        TEXT NOT NULL,
		principal TEXT NOT NULL,
		action    TEXT NOT NULL,
		path      TEXT NOT NULL DEFAULT '',
		detail    TEXT NOT NULL DEFAULT '',
		ok        INTEGER NOT NULL DEFAULT 1
	);
	CREATE INDEX IF NOT EXISTS audit_log_at ON audit_log(at);`,
}

// Open opens (creating if needed) auth.db at path.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("opening auth db: %w", err)
	}
	db.SetMaxOpenConns(1)

	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return nil, fmt.Errorf("auth schema version: %w", err)
	}
	for v := version; v < len(migrations); v++ {
		if _, err := db.Exec(migrations[v]); err != nil {
			return nil, fmt.Errorf("auth migration %d: %w", v+1, err)
		}
		if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", v+1)); err != nil {
			return nil, err
		}
	}
	return &Store{DB: db}, nil
}

// ownerName is the principal name of the owner in person; see IsOwner.
const ownerName = "owner"

// OwnerPrincipal is the vault owner with every scope — auth mode "none" and
// passkey sessions act as this.
func OwnerPrincipal() Principal {
	return Principal{Name: ownerName, Scopes: map[string]bool{ScopeRead: true, ScopeWrite: true, ScopeTasks: true}}
}

// Protected reports whether a path is subject to authentication at all.
// The SPA shell and share pages are not; neither are the auth endpoints
// themselves (they gate their own flows) or health.
func Protected(path string) bool {
	if path == "/api/v1/health" || strings.HasPrefix(path, "/api/v1/auth/") {
		return false
	}
	return strings.HasPrefix(path, "/api/") || path == "/mcp"
}

// Middleware authenticates requests according to mode and enforces scopes.
// Unauthenticated paths: /api/v1/health and the SPA (everything outside
// /api and /mcp — the SPA itself holds no data; its API calls are checked).
// mcpChallenge, when non-empty, is set as WWW-Authenticate on /mcp 401s so
// OAuth-capable clients can discover the authorization server.
func (s *Store) Middleware(mode config.AuthMode, mcpChallenge string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !Protected(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		unauthorized := func(msg string) {
			if r.URL.Path == "/mcp" && mcpChallenge != "" {
				w.Header().Set("WWW-Authenticate", mcpChallenge)
			}
			http.Error(w, `{"error":{"code":"UNAUTHORIZED","message":"`+msg+`"}}`, http.StatusUnauthorized)
		}

		var principal Principal
		switch mode {
		case config.AuthNone:
			principal = OwnerPrincipal()
		case config.AuthTokenOnly:
			p, err := s.authenticateBearer(r)
			if err != nil {
				unauthorized("valid bearer token required")
				return
			}
			principal = p
		case config.AuthPasskey:
			// Bearer tokens (agents, scripts) and session cookies (humans)
			// both work; an explicit-but-invalid bearer is rejected rather
			// than falling through to the cookie.
			if r.Header.Get("Authorization") != "" {
				p, err := s.authenticateBearer(r)
				if err != nil {
					unauthorized("invalid bearer token")
					return
				}
				principal = p
				break
			}
			cookie, err := r.Cookie(SessionCookie)
			if err != nil {
				unauthorized("login required")
				return
			}
			p, err := s.SessionPrincipal(cookie.Value)
			if err != nil {
				unauthorized("session expired — log in again")
				return
			}
			principal = p
		default:
			unauthorized("auth mode not supported")
			return
		}

		// Checked before scopes: no scope is enough, so a read token asking
		// to list credentials hears the same answer as a write token.
		if OwnerOnly(r.Method, r.URL.Path) && !principal.IsOwner() {
			s.auditOwnerOnlyRefusal(principal, r)
			http.Error(w, `{"error":{"code":"FORBIDDEN","message":"`+ownerOnlyMessage+`"}}`, http.StatusForbidden)
			return
		}
		if scope := requiredScope(r); scope != "" && !principal.Allows(scope) {
			http.Error(w, `{"error":{"code":"FORBIDDEN","message":"token lacks the `+scope+` scope"}}`, http.StatusForbidden)
			return
		}
		// Hand the principal down so per-tool scope checks (MCP) can see it.
		r = r.WithContext(WithPrincipal(r.Context(), principal))

		// REST writes by agents are audited with their outcome. MCP audits
		// itself per tool (a single POST /mcp says nothing useful), so it is
		// excluded here.
		if Audited(principal) && r.Method != http.MethodGet && r.Method != http.MethodHead && r.URL.Path != "/mcp" {
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)
			if err := s.RecordAudit(AuditRecord{
				Principal: principal.Name,
				Action:    r.Method + " " + r.URL.Path,
				Path:      strings.TrimPrefix(r.URL.Path, "/api/v1/documents/"),
				OK:        sw.status < 400,
			}); err != nil {
				slog.Warn("audit", "err", err)
			}
			return
		}
		next.ServeHTTP(w, r)
	})
}

// statusWriter captures the response status for the audit row.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Flush keeps SSE and streaming responses working through the wrapper.
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// ---- Owner-only routes ----

// ownerOnlyMessage tells a refused agent (and whoever reads its transcript)
// why, and where the thing it wanted is actually done.
const ownerOnlyMessage = "this endpoint manages credentials, sharing or settings and only the owner " +
	"signed in with a passkey may use it — API tokens and connected apps are refused whatever their scope. " +
	"Do this in the quire web UI under Settings."

// ownerOnlyRoute is one entry in ownerOnlyRoutes. An empty method means
// every method; path matches itself and anything beneath it.
type ownerOnlyRoute struct {
	method string
	path   string
}

// ownerOnlyRoutes are the administration surface: credentials, sharing,
// settings and the audit trail. Scopes answer "may this caller touch the
// vault", and the write scope is exactly what an agent needs — so before
// this list, a write-scoped token or OAuth client could mint itself a
// broader token, revoke the owner's others, disconnect apps, publish any
// document to the internet, or rewrite its own guidance. None of that is
// vault work; all of it is the owner's.
//
// The passkey routes (/api/v1/auth/passkeys, register) are not listed: they
// sit under /api/v1/auth/, which this middleware never sees, and gate
// themselves on the session cookie alone (HTTPConfig.hasSession) — which
// is owner-only by construction, since no bearer carries one.
//
// internal/api's route inventory test classifies every registered route
// against this list, so a new route cannot land unclassified.
var ownerOnlyRoutes = []ownerOnlyRoute{
	{"", "/api/v1/tokens"},         // minting/listing/revoking credentials
	{"", "/api/v1/connected-apps"}, // OAuth grants
	// Agents write the audit log; reading it back would let one check what
	// the owner can see of it, and gives it nothing it needs to do vault work.
	{"", "/api/v1/audit"},
	// A public link is exfiltration with a URL. MCP exposes no sharing tool,
	// so the web UI is the only legitimate caller of any of these.
	{"", "/api/v1/shares"},
	// Status reveals the SMTP setup; the test endpoint sends mail.
	{"", "/api/v1/email"},
	{http.MethodPut, "/api/v1/timezone"},
	{http.MethodPut, "/api/v1/areas"},
	// The guidance is appended to every agent's MCP instructions; an agent
	// that could write it could persist instructions to all future agents.
	{http.MethodPut, "/api/v1/agent-guidance"},
	// The work-item URL template turns ids in every document into links; an
	// agent that could set it could point them anywhere.
	{http.MethodPut, "/api/v1/work-items"},
	// Calendar feed subscriptions are configuration that makes the server
	// fetch arbitrary URLs, and the feed URLs themselves are secrets.
	{"", "/api/v1/calendar/feeds"},
	// Refetching the feeds is a Settings button; no MCP tool needs it, and
	// an agent has no business driving outbound fetches.
	{http.MethodPost, "/api/v1/calendar/refresh"},
}

// OwnerOnly reports whether method+path is administration only the owner
// in person may perform (see ownerOnlyRoutes).
func OwnerOnly(method, path string) bool {
	for _, route := range ownerOnlyRoutes {
		if route.method != "" && route.method != method {
			continue
		}
		if path == route.path || strings.HasPrefix(path, route.path+"/") {
			return true
		}
	}
	return false
}

// auditOwnerOnlyRefusal records an agent reaching for the admin surface.
// Refusals are otherwise unaudited, but this one is exactly what the owner
// would want to see: something tried to grant itself access.
func (s *Store) auditOwnerOnlyRefusal(principal Principal, r *http.Request) {
	if !Audited(principal) {
		return
	}
	if err := s.RecordAudit(AuditRecord{
		Principal: principal.Name,
		Action:    r.Method + " " + r.URL.Path,
		Detail:    "refused: owner-only",
		OK:        false,
	}); err != nil {
		slog.Warn("audit", "err", err)
	}
}

// requiredScope maps a request to the scope it needs, or "" when the route
// enforces its own.
//
// /mcp is the "" case: it is one POST transport carrying reads and writes
// alike, so any single blanket scope is wrong. internal/mcp instead
// registers each tool against the scope that tool actually needs, so a
// caller sees exactly the tools it may use — a read-only token gets the
// read tools, a tasks token gets the task tools, and neither is handed the
// document-write tools it would have received before, when passing this
// gate granted the whole toolset.
func requiredScope(r *http.Request) string {
	if r.URL.Path == "/mcp" {
		return ""
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return ScopeRead
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/tasks") {
		return ScopeTasks
	}
	return ScopeWrite
}
