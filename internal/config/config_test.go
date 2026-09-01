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
