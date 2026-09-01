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
	"github.com/jclement/quire/internal/index"
	"github.com/jclement/quire/internal/mcp"
	"github.com/jclement/quire/internal/service"
	"github.com/jclement/quire/internal/share"
	"github.com/jclement/quire/internal/tailnet"
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
		fmt.Fprintf(os.Stderr, "usage: quire [serve|reindex|doctor|token|task add|search|today|version]\n")
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

	shares := share.NewManager(authStore, svc, cfg.BaseURL)
	apiServer := &api.Server{Service: svc, Events: events, Shares: shares, Version: version}
	mux := http.NewServeMux()
	apiServer.Routes(mux)
	mux.Handle("/mcp", mcp.Handler(svc, version))
	shares.Routes(mux)
	mux.Handle("/", webui.Handler())

	if cfg.AuthMode == config.AuthNone {
		slog.Warn("auth mode \"none\": every request is the vault owner (loopback only)")
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
		authHTTP.Routes(mux)
		slog.Info("passkey auth enabled", "rp_id", baseURL.Hostname())
	}

	if cfg.TailscaleEnabled() {
		go serveTailnet(ctx, cfg, mux, shares, authStore)
	}

	server := &http.Server{Addr: cfg.Addr, Handler: securityHeaders(authStore.Middleware(cfg.AuthMode, mux))}
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
	return nil
}

// serveTailnet joins the tailnet and serves the app there over HTTPS,
// authenticated by tailnet identity. With funnel on, the same listener also
// takes public traffic, which the gate restricts to /s/* share pages.
// Tailnet failures never take down the local listener — they log and retry
// is a restart away.
func serveTailnet(ctx context.Context, cfg config.Config, mux http.Handler, shares *share.Manager, authStore *auth.Store) {
	node, err := tailnet.Start(ctx, cfg)
	if err != nil {
		slog.Error("tailscale failed to start; continuing without it", "err", err)
		return
	}
	defer node.Close()

	ln, err := node.Listen(cfg.TSFunnel)
	if err != nil {
		slog.Error("tailscale listener failed", "funnel", cfg.TSFunnel, "err", err)
		return
	}

	// Share links should advertise the tailnet HTTPS name (the funnel URL is
	// the same name) unless the user pinned an explicit base URL.
	if node.DNSName != "" && os.Getenv("QUIRE_BASE_URL") == "" {
		shares.SetBaseURL("https://" + node.DNSName)
	}

	publicMux := http.NewServeMux()
	shares.Routes(publicMux)

	server := &http.Server{Handler: securityHeaders(node.Handler(mux, publicMux, authStore, cfg.TSOwner))}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	slog.Info("quire on the tailnet", "url", "https://"+node.DNSName, "funnel", cfg.TSFunnel)
	if err := server.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		slog.Error("tailscale server stopped", "err", err)
	}
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
	if dangling == 0 {
		fmt.Println("vault healthy: no dangling links")
	} else {
		fmt.Printf("%d dangling link(s)\n", dangling)
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
		next.ServeHTTP(w, r)
	})
}
