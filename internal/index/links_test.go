package index

import "testing"

// TestLinkTarget covers Obsidian's link forms. Heading and block links used
// to resolve to nothing at all: `quire doctor` called them dangling and the
// target page grew no backlink, which is the relationship model failing
// silently on any vault that uses them.
func TestLinkTarget(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"Sarah Chen", "sarah chen"},
		{"  Sarah Chen  ", "sarah chen"},
		{"Project Apollo#Timeline", "project apollo"},
		{"Project Apollo#Timeline#Nested", "project apollo"},
		{"Project Apollo^block-id", "project apollo"},
		{"Project Apollo #Timeline", "project apollo"}, // space before the anchor
		{"PROJECT APOLLO", "project apollo"},
		{"#Heading", ""}, // a jump within the current page
		{"^block", ""},   // likewise
		{"", ""},
	} {
		if got := linkTarget(tc.raw); got != tc.want {
			t.Errorf("linkTarget(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

// TestNormalizeNameKeepsHash: document names are normalized by a different
// function on purpose. Stripping at "#" there would rename "C# Notes" to
// "C", making the page unlinkable.
func TestNormalizeNameKeepsHash(t *testing.T) {
	if got := normalizeName("C# Notes"); got != "c# notes" {
		t.Errorf("normalizeName(%q) = %q — a language name must survive", "C# Notes", got)
	}
}
