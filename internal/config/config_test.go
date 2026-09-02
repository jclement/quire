package config

import (
	"strings"
	"testing"
)

func TestAuthNoneRequiresLoopback(t *testing.T) {
	t.Setenv("QUIRE_DATA_DIR", t.TempDir())

	cases := []struct {
		addr string
		ok   bool
	}{
		{"127.0.0.1:8321", true},
		{"localhost:8321", true},
		{"[::1]:8321", true},
		{"0.0.0.0:8321", false},
		{"192.168.1.5:8321", false},
		{":8321", false},        // wildcard: fail closed
		{"quire.lan:80", false}, // unresolvable name: fail closed
	}
	for _, tc := range cases {
		t.Setenv("QUIRE_ADDR", tc.addr)
		t.Setenv("QUIRE_AUTH_MODE", "none")
		_, err := Load()
		if tc.ok && err != nil {
			t.Errorf("addr %q: unexpected error %v", tc.addr, err)
		}
		if !tc.ok && (err == nil || !strings.Contains(err.Error(), "loopback")) {
			t.Errorf("addr %q: want loopback error, got %v", tc.addr, err)
		}
	}

	// The same non-loopback address is fine in token-only mode.
	t.Setenv("QUIRE_ADDR", "0.0.0.0:8321")
	t.Setenv("QUIRE_AUTH_MODE", "token-only")
	if _, err := Load(); err != nil {
		t.Errorf("token-only on 0.0.0.0: %v", err)
	}
}

func TestInvalidAuthModeRejected(t *testing.T) {
	t.Setenv("QUIRE_DATA_DIR", t.TempDir())
	t.Setenv("QUIRE_AUTH_MODE", "yolo")
	if _, err := Load(); err == nil {
		t.Errorf("invalid mode accepted")
	}
}

// TestParseTrustedProxies: this list decides whose X-Forwarded-For is
// believed, and believing the wrong one lets a caller forge their own
// identity past the rate limiter. Malformed input must therefore fail
// loudly at startup rather than silently trusting nothing (or everything).
func TestParseTrustedProxies(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		count int
	}{
		{"", 0},
		{"   ", 0},
		{"127.0.0.1", 1},
		{"10.0.0.0/8", 1},
		{"172.16.0.0/12, 127.0.0.1", 2},
		{"  10.0.0.1 , 10.0.0.2  ", 2}, // whitespace tolerated
		{"::1", 1},
		{"2001:db8::/32", 1},
		{"any", 2}, // widens to both default routes
		{"ANY", 2}, // case-insensitive
	} {
		got, err := ParseTrustedProxies(tc.raw)
		if err != nil {
			t.Errorf("ParseTrustedProxies(%q) errored: %v", tc.raw, err)
			continue
		}
		if len(got) != tc.count {
			t.Errorf("ParseTrustedProxies(%q) = %d prefixes, want %d", tc.raw, len(got), tc.count)
		}
	}

	for _, bad := range []string{
		"localhost",      // a name is not an address
		"10.0.0.300",     // not an octet
		"10.0.0.0/99",    // not a prefix length
		"10.0.0.0/",      // truncated
		"127.0.0.1:8080", // a port is not part of an address
		"all",            // near-miss for "any": reject rather than guess
	} {
		if got, err := ParseTrustedProxies(bad); err == nil {
			t.Errorf("ParseTrustedProxies(%q) = %v, want an error", bad, got)
		}
	}
}

// TestTrustedProxiesFromEnv: a bad value must stop the server rather than
// leave it running with a silently empty trust list.
func TestTrustedProxiesFromEnv(t *testing.T) {
	t.Setenv("QUIRE_DATA_DIR", t.TempDir())

	t.Setenv("QUIRE_TRUSTED_PROXIES", "127.0.0.1,10.0.0.0/8")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.TrustedProxies) != 2 {
		t.Errorf("TrustedProxies = %v", cfg.TrustedProxies)
	}

	t.Setenv("QUIRE_TRUSTED_PROXIES", "not-an-address")
	if _, err := Load(); err == nil {
		t.Error("a malformed QUIRE_TRUSTED_PROXIES should refuse to start")
	}
}
