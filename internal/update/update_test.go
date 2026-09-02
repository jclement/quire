package update

import "testing"

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
