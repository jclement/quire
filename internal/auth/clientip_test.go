package auth

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/jclement/quire/internal/config"
)

// TestClientIP is a security test as much as a parsing one. The rate limiter
// buckets by whatever this returns, so a caller that can steer it can either
// escape the limiter (fresh bucket per request) or evict someone else.
func TestClientIP(t *testing.T) {
	mustProxies := func(raw string) []netip.Prefix {
		t.Helper()
		p, err := config.ParseTrustedProxies(raw)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}

	for _, tc := range []struct {
		name    string
		peer    string
		xff     string
		realIP  string
		trusted string
		want    string
	}{
		{
			name: "no proxies configured: headers are ignored entirely",
			peer: "203.0.113.9:1234", xff: "1.2.3.4", trusted: "",
			want: "203.0.113.9",
		},
		{
			name: "untrusted peer cannot forge a client address",
			peer: "203.0.113.9:1234", xff: "1.2.3.4", trusted: "10.0.0.0/8",
			want: "203.0.113.9",
		},
		{
			name: "trusted peer's forwarded address is used",
			peer: "172.18.0.2:5000", xff: "198.51.100.7", trusted: "172.16.0.0/12",
			want: "198.51.100.7",
		},
		{
			// The proxy appends; everything left of that is caller-supplied
			// and can be invented, so only the rightmost entry is believed.
			name: "spoofed entries to the left are discarded",
			peer: "172.18.0.2:5000", xff: "9.9.9.9, 8.8.8.8, 198.51.100.7",
			trusted: "172.16.0.0/12", want: "198.51.100.7",
		},
		{
			name: "X-Real-IP is the fallback when there is no XFF",
			peer: "127.0.0.1:5000", realIP: "198.51.100.7", trusted: "127.0.0.1",
			want: "198.51.100.7",
		},
		{
			name: "garbage in the header falls back to the peer",
			peer: "127.0.0.1:5000", xff: "not-an-ip", trusted: "127.0.0.1",
			want: "127.0.0.1",
		},
		{
			name: "empty header falls back to the peer",
			peer: "127.0.0.1:5000", trusted: "127.0.0.1",
			want: "127.0.0.1",
		},
		{
			name: `"any" trusts whatever connects`,
			peer: "203.0.113.9:1234", xff: "198.51.100.7", trusted: "any",
			want: "198.51.100.7",
		},
		{
			name: "IPv6 peer inside an IPv6 trusted prefix",
			peer: "[2001:db8::2]:5000", xff: "2001:db8:1::9", trusted: "2001:db8::/32",
			want: "2001:db8:1::9",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login/finish", nil)
			req.RemoteAddr = tc.peer
			if tc.xff != "" {
				req.Header.Set("X-Forwarded-For", tc.xff)
			}
			if tc.realIP != "" {
				req.Header.Set("X-Real-IP", tc.realIP)
			}
			if got := ClientIP(req, mustProxies(tc.trusted)); got != tc.want {
				t.Errorf("ClientIP = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestRateLimiterIsPerClient: the whole reason for ClientIP. Two clients must
// get independent budgets, or one of them can lock the other out.
func TestRateLimiterIsPerClient(t *testing.T) {
	rl := newRateLimiter()
	for range rateAttempts {
		if !rl.allow("198.51.100.1") {
			t.Fatal("a client should get its full budget")
		}
	}
	if rl.allow("198.51.100.1") {
		t.Error("budget should be exhausted")
	}
	if !rl.allow("198.51.100.2") {
		t.Error("LOCKOUT: a second client was denied because of the first's attempts")
	}
}

// TestRateLimiterIsBounded: the map is keyed by a caller-influenced value on
// a public endpoint, so it must not grow without limit.
func TestRateLimiterIsBounded(t *testing.T) {
	rl := newRateLimiter()
	for i := range maxTrackedClients * 2 {
		rl.allow(netip.AddrFrom4([4]byte{10, byte(i >> 16), byte(i >> 8), byte(i)}).String())
	}
	if len(rl.attempts) > maxTrackedClients {
		t.Errorf("limiter tracked %d clients, cap is %d", len(rl.attempts), maxTrackedClients)
	}
}
