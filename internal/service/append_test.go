package service

import (
	"testing"
)

func TestAppendToDocument(t *testing.T) {
	s := newTestService(t)
	original := "# Meeting\n\n## Notes\n\nsome notes\n\n## Action Items\n\n- [ ] existing\n\n## Decisions\n\nnone yet\n"
	if _, err := s.UpdateDocument("meetings/m.md", original, ""); err != nil {
		t.Fatal(err)
	}

	// Append into a middle section: lands inside it, before the next heading.
	doc, err := s.AppendToDocument("meetings/m.md", "- [ ] new item 📅 2026-09-03", "Action Items")
	if err != nil {
		t.Fatal(err)
	}
	want := "# Meeting\n\n## Notes\n\nsome notes\n\n## Action Items\n\n- [ ] existing\n- [ ] new item 📅 2026-09-03\n\n## Decisions\n\nnone yet\n"
	if doc.Markdown != want {
		t.Errorf("section append:\ngot  %q\nwant %q", doc.Markdown, want)
	}

	// Plain append: end of file.
	doc, err = s.AppendToDocument("meetings/m.md", "postscript", "")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Markdown != want+"postscript\n" {
		t.Errorf("file append:\ngot %q", doc.Markdown)
	}

	// Unknown section errors rather than guessing.
	if _, err := s.AppendToDocument("meetings/m.md", "x", "Nope"); err == nil {
		t.Errorf("unknown section should error")
	}
}
