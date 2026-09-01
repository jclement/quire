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
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jclement/quire/internal/api"
	"github.com/jclement/quire/internal/config"
	"github.com/jclement/quire/internal/index"
	"github.com/jclement/quire/internal/service"
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
	case "version", "--version", "-v":
		fmt.Println("quire", version)
	default:
		fmt.Fprintf(os.Stderr, "usage: quire [serve|reindex|doctor|version]\n")
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

	apiServer := &api.Server{Service: svc, Events: events, Version: version}
	mux := http.NewServeMux()
	apiServer.Routes(mux)
	mux.Handle("/", webui.Handler())

	if cfg.AuthMode == config.AuthNone {
		slog.Warn("auth mode \"none\": every request is the vault owner (loopback only)")
	}

	server := &http.Server{Addr: cfg.Addr, Handler: securityHeaders(mux)}
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
