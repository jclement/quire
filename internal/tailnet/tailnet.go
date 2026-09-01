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

// Handler wraps the app for the tailnet listener.
//
//   - Tailnet peers act as the vault owner (or, when an Authorization header
//     is present, as that token — an agent holding a read-only token keeps
//     its reduced blast radius even on-tailnet).
//   - Non-tailnet (funnel) visitors see only /s/* — everything else 404s, so
//     the public internet cannot even learn quire is here.
//   - ownerLogin, when non-empty, restricts tailnet access to that login.
func (n *Node) Handler(full http.Handler, public http.Handler, store *auth.Store, ownerLogin string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		who, err := n.whois(r.Context(), r.RemoteAddr)
		if err != nil || who == nil || who.Node == nil {
			// Public internet via funnel: share pages only.
			if strings.HasPrefix(r.URL.Path, "/s/") {
				public.ServeHTTP(w, r)
				return
			}
			http.NotFound(w, r)
			return
		}

		if ownerLogin != "" {
			login := ""
			if who.UserProfile != nil {
				login = who.UserProfile.LoginName
			}
			if !strings.EqualFold(login, ownerLogin) {
				http.Error(w, `{"error":{"code":"FORBIDDEN","message":"this quire belongs to someone else on the tailnet"}}`, http.StatusForbidden)
				return
			}
		}

		if auth.Protected(r.URL.Path) {
			principal := auth.OwnerPrincipal()
			if r.Header.Get("Authorization") != "" {
				principal, err = store.BearerPrincipal(r)
				if err != nil {
					http.Error(w, `{"error":{"code":"UNAUTHORIZED","message":"invalid bearer token"}}`, http.StatusUnauthorized)
					return
				}
			}
			if scope := auth.RequiredScope(r); !principal.Allows(scope) {
				http.Error(w, `{"error":{"code":"FORBIDDEN","message":"token lacks the `+scope+` scope"}}`, http.StatusForbidden)
				return
			}
		}
		full.ServeHTTP(w, r)
	})
}
