// Package tailnet runs quire as its own node on a Tailscale tailnet via
// tsnet: automatic registration with an auth key, HTTPS at
// https://<hostname>.<tailnet>.ts.net, and requests authenticated by tailnet
// identity (WhoIs) — no passwords, no tokens needed on-tailnet.
//
// With funnel enabled, the same listener also accepts public internet
// traffic; the per-request gate gives those visitors exactly one surface:
// /s/* share pages. WhoIs distinguishes the two — a tailnet peer resolves to
// an identity, a funnel connection does not.
package tailnet

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/tsnet"

	"github.com/jclement/quire/internal/auth"
	"github.com/jclement/quire/internal/config"
)

// Node is a running tsnet instance.
type Node struct {
	srv *tsnet.Server
	// whois is injected for tests; production is LocalClient().WhoIs.
	whois func(ctx context.Context, remoteAddr string) (*apitype.WhoIsResponse, error)

	// DNSName is the node's MagicDNS name ("quire.tailXXXX.ts.net").
	DNSName string
}

// Start registers/joins the tailnet and returns the running node. Blocks
// until the node is up (first run with a fresh auth key can take a few
// seconds; re-runs use persisted state and are fast).
func Start(ctx context.Context, cfg config.Config) (*Node, error) {
	srv := &tsnet.Server{
		Dir:      cfg.StateDir() + "/tsnet",
		Hostname: cfg.TSHostname,
		AuthKey:  cfg.TSAuthKey,
		UserLogf: func(format string, args ...any) { slog.Info("tailscale: " + fmt.Sprintf(format, args...)) },
	}
	status, err := srv.Up(ctx)
	if err != nil {
		return nil, fmt.Errorf("joining tailnet: %w", err)
	}
	lc, err := srv.LocalClient()
	if err != nil {
		return nil, fmt.Errorf("tailscale local client: %w", err)
	}
	dnsName := ""
	if status.Self != nil {
		dnsName = strings.TrimSuffix(status.Self.DNSName, ".")
	}
	return &Node{srv: srv, whois: lc.WhoIs, DNSName: dnsName}, nil
}

// Close shuts the node down.
func (n *Node) Close() error { return n.srv.Close() }

// Listen returns the HTTPS listener: funnel-enabled (tailnet + public on
// :443) or tailnet-only TLS. Certificates are handled by Tailscale.
func (n *Node) Listen(funnel bool) (net.Listener, error) {
	if funnel {
		return n.srv.ListenFunnel("tcp", ":443")
	}
	return n.srv.ListenTLS("tcp", ":443")
}

// GateConfig configures the per-request tailnet/public gate.
type GateConfig struct {
	Full   http.Handler // the whole app, served to tailnet peers
	Public http.Handler // the funnel-visible subset (shares; MCP/OAuth when enabled)
	Store  *auth.Store
	// OwnerLogin, when non-empty, restricts tailnet access to that login.
	OwnerLogin string
	// PublicMCP additionally exposes /mcp and the OAuth endpoints to funnel
	// visitors (each still enforcing its own credentials).
	PublicMCP bool
	// MCPChallenge is the WWW-Authenticate value for /mcp 401s (OAuth
	// discovery).
	MCPChallenge string
}

// publicAllowed is the explicit funnel allowlist — defense in depth on top
// of the public mux only registering these routes.
func (g GateConfig) publicAllowed(path string) bool {
	if strings.HasPrefix(path, "/s/") {
		return true
	}
	if !g.PublicMCP {
		return false
	}
	return path == "/mcp" || strings.HasPrefix(path, "/oauth/") || strings.HasPrefix(path, "/.well-known/")
}

// Handler wraps the app for the tailnet listener.
//
//   - Tailnet peers act as the vault owner (or, when an Authorization header
//     is present, as that token — an agent holding a read-only token keeps
//     its reduced blast radius even on-tailnet).
//   - Non-tailnet (funnel) visitors see only the public allowlist —
//     everything else 404s, so the public internet cannot even learn quire
//     is here.
//   - The gate owns the X-Quire-* identity headers: inbound values are
//     always stripped, then set from verified state, so downstream handlers
//     (the OAuth consent page) can trust them.
func (n *Node) Handler(cfg GateConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Del(auth.HeaderTailnetLogin)
		r.Header.Del(auth.HeaderPublicRequest)

		who, err := n.whois(r.Context(), r.RemoteAddr)
		if err != nil || who == nil || who.Node == nil {
			if !cfg.publicAllowed(r.URL.Path) {
				http.NotFound(w, r)
				return
			}
			r.Header.Set(auth.HeaderPublicRequest, "1")
			if r.URL.Path == "/mcp" {
				// Public MCP demands a credential on every request; the 401
				// challenge is how connectors discover the OAuth server.
				principal, err := cfg.Store.BearerPrincipal(r)
				if err != nil || !principal.Allows(auth.RequiredScope(r)) {
					w.Header().Set("WWW-Authenticate", cfg.MCPChallenge)
					http.Error(w, `{"error":{"code":"UNAUTHORIZED","message":"authorization required"}}`, http.StatusUnauthorized)
					return
				}
			}
			cfg.Public.ServeHTTP(w, r)
			return
		}

		login := "member"
		if who.UserProfile != nil && who.UserProfile.LoginName != "" {
			login = who.UserProfile.LoginName
		}
		if cfg.OwnerLogin != "" && !strings.EqualFold(login, cfg.OwnerLogin) {
			http.Error(w, `{"error":{"code":"FORBIDDEN","message":"this quire belongs to someone else on the tailnet"}}`, http.StatusForbidden)
			return
		}
		r.Header.Set(auth.HeaderTailnetLogin, login)

		if auth.Protected(r.URL.Path) {
			principal := auth.OwnerPrincipal()
			if r.Header.Get("Authorization") != "" {
				principal, err = cfg.Store.BearerPrincipal(r)
				if err != nil {
					if r.URL.Path == "/mcp" {
						w.Header().Set("WWW-Authenticate", cfg.MCPChallenge)
					}
					http.Error(w, `{"error":{"code":"UNAUTHORIZED","message":"invalid bearer token"}}`, http.StatusUnauthorized)
					return
				}
			}
			if scope := auth.RequiredScope(r); !principal.Allows(scope) {
				http.Error(w, `{"error":{"code":"FORBIDDEN","message":"token lacks the `+scope+` scope"}}`, http.StatusForbidden)
				return
			}
		}
		cfg.Full.ServeHTTP(w, r)
	})
}
