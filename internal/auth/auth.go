// Package auth owns auth.db — the one database that is NOT rebuildable —
// and the request authentication middleware. v0.1 implements auth modes
// "none" (loopback-trusted singleton user) and "token-only" (bearer tokens);
// "passkey" is designed (DESIGN.md "Auth modes") but not yet implemented, and
// is rejected at startup rather than silently downgraded.
package auth

import (
	"database/sql"
	"fmt"
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

// OwnerPrincipal is the vault owner with every scope — auth mode "none" and
// tailnet-identified requests act as this.
func OwnerPrincipal() Principal {
	return Principal{Name: "owner", Scopes: map[string]bool{ScopeRead: true, ScopeWrite: true, ScopeTasks: true}}
}

// BearerPrincipal resolves the request's Authorization header to a token
// principal (exported for the tailnet listener, which does its own gating).
func (s *Store) BearerPrincipal(r *http.Request) (Principal, error) {
	return s.authenticateBearer(r)
}

// RequiredScope maps a request to the scope it needs (see requiredScope).
func RequiredScope(r *http.Request) string { return requiredScope(r) }

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
func (s *Store) Middleware(mode config.AuthMode, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !Protected(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		var principal Principal
		switch mode {
		case config.AuthNone:
			principal = OwnerPrincipal()
		case config.AuthTokenOnly:
			p, err := s.authenticateBearer(r)
			if err != nil {
				w.Header().Set("WWW-Authenticate", `Bearer realm="quire"`)
				http.Error(w, `{"error":{"code":"UNAUTHORIZED","message":"valid bearer token required"}}`, http.StatusUnauthorized)
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
					http.Error(w, `{"error":{"code":"UNAUTHORIZED","message":"invalid bearer token"}}`, http.StatusUnauthorized)
					return
				}
				principal = p
				break
			}
			cookie, err := r.Cookie(SessionCookie)
			if err != nil {
				http.Error(w, `{"error":{"code":"UNAUTHORIZED","message":"login required"}}`, http.StatusUnauthorized)
				return
			}
			p, err := s.SessionPrincipal(cookie.Value)
			if err != nil {
				http.Error(w, `{"error":{"code":"UNAUTHORIZED","message":"session expired — log in again"}}`, http.StatusUnauthorized)
				return
			}
			principal = p
		default:
			http.Error(w, `{"error":{"code":"UNAUTHORIZED","message":"auth mode not supported"}}`, http.StatusUnauthorized)
			return
		}

		if scope := requiredScope(r); !principal.Allows(scope) {
			http.Error(w, `{"error":{"code":"FORBIDDEN","message":"token lacks the `+scope+` scope"}}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requiredScope maps a request to the scope it needs. MCP is a POST
// transport carrying reads and writes alike, so it requires whatever the
// token has beyond read — enforced per-tool in a future pass; v0.1 requires
// the tasks scope as the floor for /mcp mutating safely.
func requiredScope(r *http.Request) string {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return ScopeRead
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/tasks") {
		return ScopeTasks
	}
	if r.URL.Path == "/mcp" {
		return ScopeTasks
	}
	return ScopeWrite
}
