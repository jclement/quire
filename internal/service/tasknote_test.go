// Turning a task that is really a note into one.
package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoteTitleFrom(t *testing.T) {
	cases := map[string]string{
		"The sidebar collapses when you resize and the [[Search|search box]] loses focus, which is annoying #ux": "The sidebar collapses when you resize and the",
		"Idea: a [[Frances Bagley]] intro.": "Idea: a Frances Bagley intro",
		"  short  ":                         "short",
		"#ux **bold** `code` thought":       "bold code thought",
	}
	for in, want := range cases {
		if got := NoteTitleFrom(in); got != want {
			t.Errorf("NoteTitleFrom(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTaskToNote(t *testing.T) {
	svc := newTestService(t)
	writeVault(t, svc, map[string]string{
		"projects/web.md": "---\narea: work\n---\n# Web\n\n## Feedback\n\n" +
			"  * [ ] Onboarding feels long, ask [[Sarah Chen]] what she saw #ux 📅 2026-09-10\n" +
			"- [ ] keep me\n",
		"people/sarah-chen.md": "---\ntype: person\n---\n# Sarah Chen\n",
	})
	doc, _ := svc.GetDocument("projects/web.md")
	id := doc.Tasks[0].ID

	note, err := svc.TaskToNote(id, "Onboarding length", "")
	if err != nil {
		t.Fatal(err)
	}
	if note.Path != "notes/onboarding-length.md" || note.Type != "note" || note.Area != "work" {
		t.Errorf("note = %+v", note.DocMeta)
	}
	if !strings.Contains(note.Markdown, "# Onboarding length\n\nOnboarding feels long, ask [[Sarah Chen]] what she saw #ux\n") {
		t.Errorf("note body:\n%s", note.Markdown)
	}

	f, _ := svc.Vault.Read("projects/web.md")
	src := string(f.Raw)
	// The line becomes a plain bullet linking the note, indentation and
	// list marker kept; nothing else moves.
	if !strings.Contains(src, "## Feedback\n\n  * [[Onboarding length]]\n- [ ] keep me\n") {
		t.Errorf("source line not replaced:\n%s", src)
	}
	source, _ := svc.GetDocument("projects/web.md")
	if len(source.Tasks) != 1 {
		t.Errorf("the task should be gone: %+v", source.Tasks)
	}

	// Backlinks: the note is linked from the source, and Sarah from the note.
	back, _ := svc.Index.Backlinks(note.Path)
	if len(back) != 1 || back[0].Path != "projects/web.md" {
		t.Errorf("note backlinks = %+v", back)
	}
	sarah, _ := svc.Index.Backlinks("people/sarah-chen.md")
	if len(sarah) != 1 || sarah[0].Path != note.Path {
		t.Errorf("sarah backlinks = %+v", sarah)
	}
}

// A title another note already has gets a suffixed path; the bullet must
// link the new note, not the old one the bare title would resolve to.
func TestTaskToNoteDuplicateTitle(t *testing.T) {
	svc := newTestService(t)
	writeVault(t, svc, map[string]string{
		"notes/pricing.md":    "# Pricing\n",
		"daily/2026-09-01.md": "- [ ] Pricing thoughts from the call\n",
	})
	doc, _ := svc.GetDocument("daily/2026-09-01.md")
	note, err := svc.TaskToNote(doc.Tasks[0].ID, "Pricing", "personal")
	if err != nil {
		t.Fatal(err)
	}
	if note.Path != "notes/pricing-2.md" || note.Area != "personal" {
		t.Errorf("note = %+v", note.DocMeta)
	}
	f, _ := svc.Vault.Read("daily/2026-09-01.md")
	if strings.TrimSpace(string(f.Raw)) != "- [[notes/pricing-2|Pricing]]" {
		t.Errorf("source = %q", f.Raw)
	}
	back, _ := svc.Index.Backlinks("notes/pricing-2.md")
	if len(back) != 1 {
		t.Errorf("new note should be linked: %+v", back)
	}

	// An empty title falls back to the task's first words.
	writeVault(t, svc, map[string]string{"daily/2026-09-02.md": "- [ ] Remember the wiki migration plan\n"})
	doc, _ = svc.GetDocument("daily/2026-09-02.md")
	auto, err := svc.TaskToNote(doc.Tasks[0].ID, "  ", "")
	if err != nil {
		t.Fatal(err)
	}
	if auto.Title != "Remember the wiki migration plan" {
		t.Errorf("auto title = %q", auto.Title)
	}
}

// The file changed under the index — the task reworded in another editor —
// so its line cannot be found. That must fail before anything is written:
// no note, no edit to the source.
func TestTaskToNoteAfterTheFileChanged(t *testing.T) {
	svc := newTestService(t)
	writeVault(t, svc, map[string]string{"daily/2026-09-01.md": "- [ ] Pricing thoughts from the call\n"})
	doc, _ := svc.GetDocument("daily/2026-09-01.md")
	id := doc.Tasks[0].ID

	changed := "- [ ] Pricing thoughts, reworded elsewhere\n"
	if err := os.WriteFile(filepath.Join(svc.Vault.Dir, "daily", "2026-09-01.md"), []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.TaskToNote(id, "Pricing", ""); err == nil {
		t.Fatal("expected an error for a task whose line is gone")
	}
	if svc.Vault.Exists("notes/pricing.md") {
		t.Error("a note was created for a task that could not be replaced")
	}
	f, _ := svc.Vault.Read("daily/2026-09-01.md")
	if string(f.Raw) != changed {
		t.Errorf("source was touched: %q", f.Raw)
	}
}
