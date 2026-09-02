// Working out who is actually calling, when a proxy sits in front.
//
// quire's rate limiter buckets by client IP. Behind a tunnel or reverse
// proxy every request arrives from the proxy, so without this the limiter
// degrades into one global 10-per-5-minutes bucket and a stranger hammering
// /login locks the owner out of their own vault.
//
// The fix is not "always read X-Forwarded-For" — that header is caller-
// supplied, and trusting it unconditionally lets an attacker mint a fresh
// bucket per request and skip the limiter entirely. So it is trusted only
// when the immediate peer is a configured proxy, and only its rightmost
// entry, which is the one that proxy appended.
package auth

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ClientIP returns the address to attribute a request to. trusted is the set
// of proxy addresses whose forwarding headers may be believed; when it is
// empty, or the peer is not in it, the direct peer address wins.
func ClientIP(r *http.Request, trusted []netip.Prefix) string {
	peer := hostOf(r.RemoteAddr)
	if len(trusted) == 0 {
		return peer
	}
	addr, err := netip.ParseAddr(peer)
	if err != nil || !trustedPeer(addr, trusted) {
		return peer
	}
	// Rightmost X-Forwarded-For entry: everything to its left was supplied
	// by whoever called the proxy and can be invented freely. Anything the
	// proxy itself appended is the last element.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		candidate := strings.TrimSpace(parts[len(parts)-1])
		if _, err := netip.ParseAddr(candidate); err == nil {
			return candidate
		}
	}
	if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); real != "" {
		if _, err := netip.ParseAddr(real); err == nil {
			return real
		}
	}
	return peer
}

func trustedPeer(addr netip.Addr, trusted []netip.Prefix) bool {
	addr = addr.Unmap()
	for _, prefix := range trusted {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// hostOf strips the port from a RemoteAddr, tolerating an address that has
// none.
func hostOf(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}
