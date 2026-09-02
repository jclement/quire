// quire — a self-hosted personal knowledge & work hub in one binary.
// Command dispatch lives here: serve (the app), reindex (rebuild index.db
// from markdown), doctor (vault health), version.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jclement/quire/internal/api"
	"github.com/jclement/quire/internal/auth"
	"github.com/jclement/quire/internal/cli"
	"github.com/jclement/quire/internal/config"
	"github.com/jclement/quire/internal/gitback"
	"github.com/jclement/quire/internal/index"
	"github.com/jclement/quire/internal/mcp"
	"github.com/jclement/quire/internal/oauth"
	"github.com/jclement/quire/internal/service"
	"github.com/jclement/quire/internal/share"
	"github.com/jclement/quire/internal/update"
	"github.com/jclement/quire/internal/vault"
	"github.com/jclement/quire/internal/webui"
)

// version is stamped by GoReleaser via -ldflags; "dev" otherwise.
var version = "dev"

func main() {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	var err error
	switch cmd {
	case "serve":
		err = runServe()
	case "reindex":
		err = runReindex()
	case "doctor":
		err = runDoctor()
	case "token":
		err = runToken(os.Args[2:])
	case "backup":
		err = runBackup(os.Args[2:])
	case "digest":
		err = runDigest()
	case "task":
		// `quire task add ...` — the subcommand shape people type.
		if len(os.Args) < 3 {
			err = cli.Run(nil)
		} else {
			err = cli.Run(os.Args[2:])
		}
	case "search", "today":
		err = cli.Run(os.Args[1:])
	case "version", "--version", "-v":
		fmt.Println("quire", version)
	default:
		fmt.Fprintf(os.Stderr, "usage: quire [serve|reindex|doctor|backup|digest|token|task add|search|today|version]\n")
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "quire:", err)
		os.Exit(1)
	}
}

// setup performs the shared boot sequence: config, logging, vault, index.
func setup() (config.Config, *service.Service, error) {
	cfg, err := config.Load()
	if err != nil {
		return config.Config{}, nil, err
	}

	level := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	v, err := vault.New(cfg.VaultDir())
	if err != nil {
		return config.Config{}, nil, err
	}
	db, _, err := index.Open(filepath.Join(cfg.StateDir(), "index.db"))
	if err != nil {
		return config.Config{}, nil, err
	}
	ix := &index.Index{DB: db, Vault: v}
	return cfg, service.New(v, ix), nil
}

func runServe() error {
	cfg, svc, err := setup()
	if err != nil {
		return err
	}
	authStore, err := auth.Open(filepath.Join(cfg.StateDir(), "auth.db"))
	if err != nil {
		return err
	}

	events := api.NewBroadcaster()
	svc.Index.Notify = events.Publish

	// Startup scan catches anything changed while quire was down, then the
	// watcher takes over for live changes.
	start := time.Now()
	if err := svc.Index.FullScan(); err != nil {
		return fmt.Errorf("initial scan: %w", err)
	}
	slog.Info("index ready", "took", time.Since(start).Round(time.Millisecond))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		if err := svc.Index.Watch(ctx); err != nil {
			slog.Error("watcher stopped", "err", err)
		}
	}()

	// Git-backed vault: every index change pokes the debounced committer.
	var committer *gitback.Committer
	if cfg.Git {
		committer, err = gitback.Start(ctx, cfg.VaultDir())
		if err != nil {
			slog.Error("vault git backing failed; continuing without it", "err", err)
			committer = nil
		} else {
			svc.Index.Notify = func(ev index.Event) {
				events.Publish(ev)
				committer.Poke()
			}
		}
	}

	shares := share.NewManager(authStore, svc, cfg.BaseURL)
	apiServer := &api.Server{
		Service: svc, Events: events, Shares: shares, Auth: authStore, Version: version,
	}
	if cfg.UpdateCheck {
		checker := update.Start(ctx, version)
		apiServer.UpdateCheck = checker.Available
	}
	mux := http.NewServeMux()
	apiServer.Routes(mux)
	mcpHandler := mcp.Handler(svc, version)
	mux.Handle("/mcp", mcpHandler)
	shares.Routes(mux)
	mux.Handle("/", webui.Handler())

	// OAuth 2.1 authorization server for remote MCP clients. Consent needs
	// the vault owner: a passkey session, or the loopback-only auth-none
	// listener, which by definition nobody remote can reach.
	isOwner := func(r *http.Request) bool {
		if cookie, err := r.Cookie(auth.SessionCookie); err == nil {
			if _, err := authStore.SessionPrincipal(cookie.Value); err == nil {
				return true
			}
		}
		// Loopback-only mode has no other notion of a user.
		return cfg.AuthMode == config.AuthNone
	}
	oauthServer := oauth.New(authStore, cfg.BaseURL, isOwner)
	oauthServer.Routes(mux)

	if cfg.AuthMode == config.AuthNone {
		slog.Warn("auth mode \"none\": every request is the vault owner (loopback only)")
	}
	// Consent is a browser flow, so it needs a browser-shaped credential.
	// token-only has none, which makes /oauth/authorize unapprovable — the
	// connector fails at the last step with nothing to explain it.
	if cfg.AuthMode == config.AuthTokenOnly {
		slog.Warn("auth mode \"token-only\": bearer tokens work, but OAuth consent cannot be approved " +
			"(no browser login) — use QUIRE_AUTH_MODE=passkey if you want claude.ai connectors")
	}
	// Every URL quire hands out — the OAuth discovery challenge on /mcp,
	// share links — comes from base_url. Left at its default while listening
	// elsewhere, it silently points clients at a host that isn't there.
	if cfg.BaseURL == config.DefaultBaseURL && cfg.Addr != "127.0.0.1:8321" &&
		cfg.Addr != "localhost:8321" {
		slog.Warn("QUIRE_BASE_URL is unset, so share links and MCP OAuth discovery will advertise "+
			config.DefaultBaseURL+" — set it to the URL clients actually reach this instance at",
			"listening_on", cfg.Addr)
	}
	if cfg.AuthMode == config.AuthPasskey {
		baseURL, err := url.Parse(cfg.BaseURL)
		if err != nil || baseURL.Hostname() == "" {
			return fmt.Errorf("auth mode \"passkey\" needs a valid QUIRE_BASE_URL (passkeys bind to its hostname), got %q", cfg.BaseURL)
		}
		passkeys, err := auth.NewPasskeys(authStore, "quire", baseURL.Hostname(), []string{cfg.BaseURL})
		if err != nil {
			return err
		}
		authHTTP := &auth.HTTPConfig{Passkeys: passkeys, SecureCookies: baseURL.Scheme == "https"}
		// An un-bootstrapped instance is claimable by whoever reaches it
		// first, so gate the first registration behind a code only someone
		// with server access can read. Loopback listeners skip it: there is
		// no remote to race.
		count, err := authStore.PasskeyCount()
		if err != nil {
			return err
		}
		if count == 0 && !config.IsLoopback(cfg.Addr) {
			code, err := auth.NewEnrollCode()
			if err != nil {
				return err
			}
			authHTTP.EnrollCode = code
			slog.Warn("no passkey registered yet — this instance is unclaimed. "+
				"Enter this enrollment code in the browser to register the owner passkey. "+
				"It changes on every restart and stops working once a passkey exists.",
				"enrollment_code", code)
		}
		authHTTP.Routes(mux)
		slog.Info("passkey auth enabled", "rp_id", baseURL.Hostname())
	} else {
		// Without passkey mode there is no auth surface; answer explicitly
		// (the SPA's auth gate keys off this 404) instead of letting the SPA
		// fallback serve HTML for an API path.
		mux.HandleFunc("/api/v1/auth/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":{"code":"NOT_FOUND","message":"authentication is not enabled on this instance"}}`)
		})
	}

	scheduleDigest(ctx, cfg, svc)

	server := &http.Server{Addr: cfg.Addr, Handler: securityHeaders(authStore.Middleware(cfg.AuthMode, oauthServer.WWWAuthenticate(), mux))}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	slog.Info("quire serving", "addr", cfg.Addr, "vault", cfg.VaultDir(), "auth", string(cfg.AuthMode), "version", version)
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	if committer != nil {
		committer.Wait(10 * time.Second)
	}
	return nil
}

func runReindex() error {
	cfg, svc, err := setup()
	if err != nil {
		return err
	}
	// Prove the disposability guarantee the honest way: drop everything and
	// rebuild from markdown alone.
	if _, err := svc.Index.DB.Exec("DELETE FROM documents"); err != nil {
		return err
	}
	for _, table := range []string{"docnames", "links", "tags", "tasks", "task_links", "fts"} {
		if _, err := svc.Index.DB.Exec("DELETE FROM " + table); err != nil {
			return err
		}
	}
	start := time.Now()
	if err := svc.Index.FullScan(); err != nil {
		return err
	}
	var docs, tasks int
	_ = svc.Index.DB.QueryRow("SELECT COUNT(*) FROM documents").Scan(&docs)
	_ = svc.Index.DB.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&tasks)
	fmt.Printf("reindexed %s: %d documents, %d tasks in %s\n", cfg.VaultDir(), docs, tasks, time.Since(start).Round(time.Millisecond))
	return nil
}

func runDoctor() error {
	_, svc, err := setup()
	if err != nil {
		return err
	}
	if err := svc.Index.FullScan(); err != nil {
		return err
	}

	// Dangling wikilinks: link targets no document answers to.
	rows, err := svc.Index.DB.Query(`
		SELECT DISTINCT l.src_path, l.target_raw FROM links l
		WHERE NOT EXISTS (SELECT 1 FROM docnames n WHERE n.name = l.target_norm)
		ORDER BY l.src_path`)
	if err != nil {
		return err
	}
	defer rows.Close()
	dangling := 0
	for rows.Next() {
		var src, target string
		if err := rows.Scan(&src, &target); err != nil {
			return err
		}
		fmt.Printf("dangling link: [[%s]] in %s\n", target, src)
		dangling++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	// Unreferenced attachments: files no document mentions. Reported, never
	// deleted — a reference may live outside the vault or be coming back.
	unreferenced := 0
	attErr := filepath.WalkDir(filepath.Join(svc.Vault.Dir, "attachments"), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(svc.Vault.Dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		var refs int
		if err := svc.Index.DB.QueryRow(
			"SELECT COUNT(*) FROM fts WHERE body LIKE '%' || ? || '%'", rel).Scan(&refs); err != nil {
			return err
		}
		if refs == 0 {
			fmt.Printf("unreferenced attachment: %s\n", rel)
			unreferenced++
		}
		return nil
	})
	if attErr != nil && !os.IsNotExist(attErr) {
		return attErr
	}

	if dangling == 0 && unreferenced == 0 {
		fmt.Println("vault healthy: no dangling links, no unreferenced attachments")
	} else {
		fmt.Printf("%d dangling link(s), %d unreferenced attachment(s)\n", dangling, unreferenced)
	}
	return nil
}

// runToken drives token lifecycle: create <name> [scopes...], list, revoke
// <prefix>. Scopes default to read; pass any of read, write, tasks.
func runToken(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	store, err := auth.Open(filepath.Join(cfg.StateDir(), "auth.db"))
	if err != nil {
		return err
	}

	if len(args) == 0 {
		return fmt.Errorf("usage: quire token [create <name> [read|write|tasks ...] | list | revoke <prefix>]")
	}
	switch args[0] {
	case "create":
		if len(args) < 2 {
			return fmt.Errorf("usage: quire token create <name> [read|write|tasks ...]")
		}
		plaintext, t, err := store.CreateToken(args[1], args[2:], 0)
		if err != nil {
			return err
		}
		fmt.Printf("token %q created (scopes: %v)\n\n  %s\n\nThis is the only time it will be shown.\n", t.Name, t.Scopes, plaintext)
	case "list":
		tokens, err := store.ListTokens()
		if err != nil {
			return err
		}
		if len(tokens) == 0 {
			fmt.Println("no tokens")
			return nil
		}
		for _, t := range tokens {
			status := "active"
			if t.RevokedAt != "" {
				status = "revoked " + t.RevokedAt
			}
			lastUsed := t.LastUsedAt
			if lastUsed == "" {
				lastUsed = "never"
			}
			fmt.Printf("sk_%s…  %-20s %-24s %s  last used %s\n", t.Prefix, t.Name, strings.Join(t.Scopes, ","), status, lastUsed)
		}
	case "revoke":
		if len(args) < 2 {
			return fmt.Errorf("usage: quire token revoke <prefix>")
		}
		if err := store.RevokeToken(strings.TrimPrefix(args[1], "sk_")); err != nil {
			return err
		}
		fmt.Println("revoked")
	default:
		return fmt.Errorf("unknown token subcommand %q", args[0])
	}
	return nil
}

// securityHeaders applies the house baseline to every response.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		// The SPA's own bundle only. 'unsafe-inline' for styles is forced by
		// CodeMirror and Mermaid, which inject stylesheets at runtime; the
		// pre-paint theme script in index.html forces it for scripts too.
		// Share pages override this with a far stricter policy of their own.
		if !strings.HasPrefix(r.URL.Path, "/s/") {
			h.Set("Content-Security-Policy",
				"default-src 'self'; img-src 'self' data: blob:; "+
					"style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; "+
					// Fonts are inlined into the bundle as data: URIs; without
					// this the whole UI silently falls back to system faces.
					"font-src 'self' data:; "+
					"connect-src 'self'; base-uri 'self'; frame-ancestors 'none'; "+
					"object-src 'none'; form-action 'self'")
		}
		next.ServeHTTP(w, r)
	})
}
