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

func TestTagsAndDailyNotesBefore(t *testing.T) {
	ix := newTestIndex(t)
	tags, err := ix.Tags()
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) == 0 {
		t.Fatal("the test vault carries tags; none came back")
	}
	// Most-used first, ties alphabetical.
	for i := 1; i < len(tags); i++ {
		if tags[i-1].Count < tags[i].Count {
			t.Errorf("tags not sorted by count: %+v", tags)
		}
	}

	for _, rel := range []string{"daily/2020-01-30.md", "daily/2020-01-31.md", "daily/2020-02-01.md"} {
		if _, err := ix.Vault.Write(rel, []byte("# "+rel+"\n"), ""); err != nil {
			t.Fatal(err)
		}
		if _, err := ix.IndexFile(rel); err != nil {
			t.Fatal(err)
		}
	}
	before, err := ix.DailyNotesBefore("2020-02-01", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 2 || before[0].Path != "daily/2020-01-31.md" || before[1].Path != "daily/2020-01-30.md" {
		t.Errorf("DailyNotesBefore = %v", paths(before))
	}
	one, _ := ix.DailyNotesBefore("2020-02-01", 1)
	if len(one) != 1 {
		t.Errorf("limit not honoured: %d", len(one))
	}
}

func paths(rows []DocRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Path
	}
	return out
}
