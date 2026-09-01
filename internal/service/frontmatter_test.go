package service

import (
	"strings"
	"testing"
)

// Linking must be surgical: only the touched key's line changes, and every
// comment, key order and quoting style around it survives.
func TestSetFrontmatterIsSurgical(t *testing.T) {
	s := newTestService(t)
	original := "---\n# who this is\ntype: person\nemail: 'sarah@acme.example'\ntags: [customer]\n---\n# Sarah Chen\n\nProse stays.\n"
	if _, err := s.UpdateDocument("people/sarah-chen.md", original, ""); err != nil {
		t.Fatal(err)
	}

	doc, err := s.SetFrontmatter("people/sarah-chen.md", map[string]any{"company": "[[Acme]]"}, "")
	if err != nil {
		t.Fatal(err)
	}
	want := "---\n# who this is\ntype: person\nemail: 'sarah@acme.example'\ntags: [customer]\ncompany: \"[[Acme]]\"\n---\n# Sarah Chen\n\nProse stays.\n"
	if doc.Markdown != want {
		t.Errorf("set:\ngot  %q\nwant %q", doc.Markdown, want)
	}

	// A null removes just that key.
	doc, err = s.SetFrontmatter("people/sarah-chen.md", map[string]any{"email": nil}, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(doc.Markdown, "email:") || !strings.Contains(doc.Markdown, "# who this is") {
		t.Errorf("remove:\n%s", doc.Markdown)
	}

	// Stale base hash is a conflict, not a clobber.
	if _, err := s.SetFrontmatter("people/sarah-chen.md", map[string]any{"title": "VP"}, "stale"); err == nil {
		t.Errorf("stale write accepted")
	}
}

func TestLinkAndUnlinkEntity(t *testing.T) {
	s := newTestService(t)
	if _, err := s.UpdateDocument("meetings/sync.md", "---\ntype: meeting\ndate: 2026-09-01T14:00\n---\n# Sync\n", ""); err != nil {
		t.Fatal(err)
	}

	doc, err := s.LinkEntity("meetings/sync.md", "people", "Sarah Chen")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.Markdown, `people: ["[[Sarah Chen]]"]`) {
		t.Errorf("first link:\n%s", doc.Markdown)
	}

	doc, err = s.LinkEntity("meetings/sync.md", "people", "[[Dan Roe]]")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.Markdown, `people: ["[[Sarah Chen]]", "[[Dan Roe]]"]`) {
		t.Errorf("second link:\n%s", doc.Markdown)
	}

	// Linking the same person again is a no-op, however it is spelled.
	before := doc.Markdown
	doc, err = s.LinkEntity("meetings/sync.md", "people", "sarah chen")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Markdown != before {
		t.Errorf("duplicate link changed the file:\n%s", doc.Markdown)
	}

	// The link resolves, so the meeting shows up in Sarah's backlinks.
	if _, err := s.CreateDocument("person", "Sarah Chen", ""); err == nil {
		back, err := s.GetDocument("people/sarah-chen.md")
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, b := range back.Backlinks {
			if b.Path == "meetings/sync.md" {
				found = true
			}
		}
		if !found {
			t.Errorf("frontmatter link did not create a backlink: %+v", back.Backlinks)
		}
	}

	doc, err = s.UnlinkEntity("meetings/sync.md", "people", "Sarah Chen")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.Markdown, `people: ["[[Dan Roe]]"]`) {
		t.Errorf("unlink:\n%s", doc.Markdown)
	}

	// Removing the last one drops the key entirely.
	doc, err = s.UnlinkEntity("meetings/sync.md", "people", "Dan Roe")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(doc.Markdown, "people:") {
		t.Errorf("empty list should remove the key:\n%s", doc.Markdown)
	}
}
