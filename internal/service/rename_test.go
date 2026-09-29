package service

import (
	"errors"
	"testing"

	"github.com/jclement/quire/internal/index"
	"github.com/jclement/quire/internal/vault"
)

// TestRenameKeepsSelfLinks: inbound links used to be rewritten first — the
// document's links to itself included, in the old file — and then the new
// file was written from content read before that, so the moved document
// carried the dead name.
func TestRenameKeepsSelfLinks(t *testing.T) {
	s := newTestService(t)
	if _, err := s.UpdateDocument("projects/plan.md", "# Reporting Plan\n\nBack to [[plan]].\n", ""); err != nil {
		t.Fatal(err)
	}
	result, err := s.RenameDocument("projects/plan.md", "projects/plan-2026.md", true)
	if err != nil {
		t.Fatal(err)
	}
	want := "# Reporting Plan\n\nBack to [[plan-2026|plan]].\n"
	if result.Document.Markdown != want {
		t.Errorf("moved document:\ngot  %q\nwant %q", result.Document.Markdown, want)
	}
	if s.Vault.Exists("projects/plan.md") {
		t.Errorf("old file still exists")
	}
}

// TestRenameKeepsAnOldFileEditedMidMove: the old path used to be deleted
// unconditionally, dropping any edit that landed after the move read it.
func TestRenameKeepsAnOldFileEditedMidMove(t *testing.T) {
	s := newTestService(t)
	if _, err := s.UpdateDocument("notes/a.md", "# A\n\nfirst draft\n", ""); err != nil {
		t.Fatal(err)
	}
	// Once the new copy is indexed, an editor saves into the old path.
	edited := false
	s.Index.Notify = func(ev index.Event) {
		if ev.Path != "notes/b.md" || edited {
			return
		}
		edited = true
		f, _ := s.Vault.Read("notes/a.md")
		if _, err := s.Vault.Write("notes/a.md", []byte("# A\n\nsecond draft\n"), f.SHA256); err != nil {
			t.Errorf("mid-move edit: %v", err)
		}
	}

	if _, err := s.RenameDocument("notes/a.md", "notes/b.md", true); !errors.Is(err, vault.ErrConflict) {
		t.Errorf("rename over a mid-move edit: got %v, want ErrConflict", err)
	}
	old, err := s.Vault.Read("notes/a.md")
	if err != nil {
		t.Fatalf("the edited old file was deleted: %v", err)
	}
	if string(old.Raw) != "# A\n\nsecond draft\n" {
		t.Errorf("old file = %q", old.Raw)
	}
}
