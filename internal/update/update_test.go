package update

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestIsNewer covers the comparison the Settings notice depends on. The
// failure mode that matters is a false positive: nagging about an update
// that does not exist is worse than staying quiet, so anything unparseable
// must compare as "not newer".
func TestIsNewer(t *testing.T) {
	for _, tc := range []struct {
		current, latest string
		want            bool
	}{
		{"0.1.1", "0.1.2", true},
		{"0.1.1", "0.2.0", true},
		{"0.1.1", "1.0.0", true},
		{"v0.1.1", "v0.1.2", true}, // leading v on both
		{"0.1.1", "v0.1.2", true},  // and on one
		{"0.9.0", "0.10.0", true},  // numeric, not lexical
		{"0.1.2", "0.1.2", false},
		{"0.1.2", "0.1.1", false}, // downgrade is not an update
		{"1.0.0", "0.9.9", false},
		{"0.1.2", "0.1.2-rc1", false}, // pre-release of the same version
		{"dev", "0.1.2", false},       // dev builds never nag
		{"0.1.2", "", false},
		{"", "0.1.2", false},
		{"0.1", "0.1.2", false}, // malformed current
		{"0.1.2", "not-a-version", false},
		{"0.1.2", "0.1.x", false},
	} {
		if got := IsNewer(tc.current, tc.latest); got != tc.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", tc.current, tc.latest, got, tc.want)
		}
	}
}

// TestNilCheckerIsQuiet: Start returns nil for a dev build, and callers wire
// Available unconditionally, so the nil case must not panic.
func TestNilCheckerIsQuiet(t *testing.T) {
	var c *Checker
	if c.Available() {
		t.Error("a nil checker must report no update")
	}
	if Start(t.Context(), "dev") != nil {
		t.Error("dev builds must not start a checker")
	}
}

// TestCheckOnce exercises the HTTP path. The rule that matters is that a
// failure of any kind stays quiet: this runs in the background against a
// third party, and there is no version of "GitHub was slow" that should
// affect the app.
func TestCheckOnce(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		body    string
		current string
		want    bool
	}{
		{"a newer release", 200, `{"tag_name":"v9.0.0"}`, "0.1.0", true},
		{"the same release", 200, `{"tag_name":"v0.1.0"}`, "0.1.0", false},
		{"an older release", 200, `{"tag_name":"v0.0.1"}`, "0.1.0", false},
		{"rate limited", 403, `{"message":"rate limited"}`, "0.1.0", false},
		{"not found", 404, ``, "0.1.0", false},
		{"server error", 500, ``, "0.1.0", false},
		{"malformed json", 200, `not json at all`, "0.1.0", false},
		{"no tag in the payload", 200, `{}`, "0.1.0", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			c := &Checker{current: tc.current, url: server.URL}
			c.checkOnce(t.Context())

			if got := c.Available(); got != tc.want {
				t.Errorf("Available() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestCheckOnceSurvivesAnUnreachableHost: no panic, no hang, no update
// claimed — the app must not care that the check failed.
func TestCheckOnceSurvivesAnUnreachableHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := server.URL
	server.Close() // nothing is listening now

	c := &Checker{current: "0.1.0", url: url}
	c.checkOnce(t.Context())
	if c.Available() {
		t.Error("an unreachable host should not report an update")
	}
}
